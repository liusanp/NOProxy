from fastapi import APIRouter, HTTPException, Query
from ..models.video import VideoListResponse, VideoDetail, VideoItem
from ..services.scraper import scraper_service
from ..services.video_cache import video_cache_service
from ..services.proxy import proxy_service
from ..config import settings
import asyncio

router = APIRouter(prefix="/api/videos", tags=["videos"])

_total_pages_cache = {"total_pages": 1}
_precache_queue = set()  # 正在预缓存的视频ID

# 列表缓存有效期：3小时
LIST_CACHE_MAX_AGE = 3 * 60 * 60


async def _download_thumbnails(videos):
    """后台下载视频封面图"""
    for video in videos:
        if video.thumbnail:
            await video_cache_service.download_thumbnail(video.id, video.thumbnail)


async def _precache_video(video_id: str):
    """预缓存单个视频"""
    # 检查是否已缓存或正在下载
    if video_cache_service.is_cached(video_id):
        return
    if video_cache_service.is_downloading(video_id):
        return
    if video_id in _precache_queue:
        return

    _precache_queue.add(video_id)
    try:
        video_url = f"{settings.TARGET_BASE_URL}/view_video.php?viewkey={video_id}"
        detail = await scraper_service.get_video_detail_in_new_tab(video_url)

        if not detail or not detail.m3u8_url:
            print(f"[预缓存] 跳过 {video_id}: 无法获取视频链接")
            return

        # 再次检查（可能在获取详情期间用户已开始播放）
        if video_cache_service.is_cached(video_id) or video_cache_service.is_downloading(video_id):
            return

        # 启动缓存下载
        video_src = detail.m3u8_url
        is_mp4 = ".mp4" in video_src.lower() or ".m3u8" not in video_src.lower()

        if is_mp4:
            await video_cache_service.start_mp4_cache_download(video_id, video_src, detail)
        else:
            # 获取 m3u8 内容
            session = await proxy_service.get_session()
            async with session.get(video_src) as resp:
                original_m3u8 = await resp.text()
            await video_cache_service.start_cache_download(video_id, video_src, original_m3u8, detail)

        print(f"[预缓存] 已启动: {video_id}")

    except Exception as e:
        print(f"[预缓存] 失败 {video_id}: {e}")
    finally:
        _precache_queue.discard(video_id)


async def _precache_videos(videos):
    """后台预缓存视频列表"""
    # 使用信号量限制并发数
    semaphore = asyncio.Semaphore(settings.PRECACHE_CONCURRENT)

    async def precache_with_limit(video):
        async with semaphore:
            await _precache_video(video.id)

    # 并发预缓存
    tasks = [precache_with_limit(v) for v in videos]
    await asyncio.gather(*tasks, return_exceptions=True)


@router.get("", response_model=VideoListResponse)
async def get_video_list(page: int = Query(1, ge=1, description="页码")):
    """获取视频列表"""
    # 优先使用有效期内的缓存（3小时）
    if settings.VIDEO_CACHE_ENABLED:
        fresh_cache = await video_cache_service.get_cached_list(page, max_age=LIST_CACHE_MAX_AGE)
        if fresh_cache:
            videos = [VideoItem(**v) for v in fresh_cache.get("videos", [])]
            response = VideoListResponse(
                videos=videos,
                total=fresh_cache.get("total", len(videos)),
                page=fresh_cache.get("page", page),
                total_pages=fresh_cache.get("total_pages", 1)
            )
            if response.total_pages > 1:
                _total_pages_cache["total_pages"] = response.total_pages
            return response

    # 缓存过期或不存在，尝试从网站获取
    result = None
    fetch_error = None
    try:
        result = await scraper_service.get_video_list(page_num=page)
    except Exception as e:
        fetch_error = e
        print(f"获取视频列表失败: {e}")

    # 获取成功且有数据
    if result and result.videos:
        # 更新总页数缓存
        if result.total_pages > 1:
            _total_pages_cache["total_pages"] = result.total_pages

        response = VideoListResponse(
            videos=result.videos,
            total=len(result.videos),
            page=page,
            total_pages=_total_pages_cache["total_pages"]
        )

        # 保存到文件缓存
        if settings.VIDEO_CACHE_ENABLED:
            await video_cache_service.save_list_cache(page, {
                "videos": [v.model_dump() for v in result.videos],
                "total": len(result.videos),
                "page": page,
                "total_pages": _total_pages_cache["total_pages"]
            })
            # 后台异步下载封面图
            asyncio.create_task(_download_thumbnails(result.videos))
            # 后台异步预缓存视频
            if settings.AUTO_PRECACHE:
                asyncio.create_task(_precache_videos(result.videos))

        return response

    # 获取失败或无数据，尝试使用过期的缓存作为兜底
    if settings.VIDEO_CACHE_ENABLED:
        file_cached = await video_cache_service.get_cached_list(page)  # 不检查时间
        if file_cached:
            videos = [VideoItem(**v) for v in file_cached.get("videos", [])]
            response = VideoListResponse(
                videos=videos,
                total=file_cached.get("total", len(videos)),
                page=file_cached.get("page", page),
                total_pages=file_cached.get("total_pages", 1)
            )
            # 更新总页数
            if response.total_pages > 1:
                _total_pages_cache["total_pages"] = response.total_pages
            print(f"[Cache] 使用过期缓存兜底: 第{page}页, {len(videos)}个视频")
            return response

    # 既无法获取也无缓存
    if fetch_error:
        raise HTTPException(status_code=500, detail=f"获取视频列表失败: {str(fetch_error)}")
    else:
        raise HTTPException(status_code=404, detail="暂无视频数据")


@router.get("/{video_id}", response_model=VideoDetail)
async def get_video_detail(video_id: str):
    """获取视频详情"""
    from ..services.video_cache import video_cache_service

    # 如果视频文件已缓存，优先使用持久化的详情缓存
    if video_cache_service.is_cached(video_id):
        cached_detail = await video_cache_service.get_cached_detail(video_id)
        if cached_detail:
            return cached_detail

    # 视频未缓存，每次都重新获取详情（不使用内存缓存）
    try:
        video_url = f"{settings.TARGET_BASE_URL}/view_video.php?viewkey={video_id}"
        detail = await scraper_service.get_video_detail(video_url)

        if not detail:
            raise HTTPException(status_code=404, detail="视频不存在")

        return detail

    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"获取视频详情失败: {str(e)}")


@router.delete("/cache")
async def clear_cache():
    """清除缓存"""
    _total_pages_cache["total_pages"] = 1
    return {"message": "缓存已清除"}
