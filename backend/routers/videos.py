from fastapi import APIRouter, HTTPException, Query
from ..models.video import VideoListResponse, VideoDetail
from ..services.scraper import scraper_service

router = APIRouter(prefix="/api/videos", tags=["videos"])

# 简单的内存缓存
_cache = {}
_total_pages_cache = {"total_pages": 1}


@router.get("", response_model=VideoListResponse)
async def get_video_list(page: int = Query(1, ge=1, description="页码")):
    """获取视频列表"""
    cache_key = f"list_{page}"

    if cache_key in _cache:
        return _cache[cache_key]

    try:
        result = await scraper_service.get_video_list(page_num=page)

        # 更新总页数缓存
        if result.total_pages > 1:
            _total_pages_cache["total_pages"] = result.total_pages

        response = VideoListResponse(
            videos=result.videos,
            total=len(result.videos),
            page=page,
            total_pages=_total_pages_cache["total_pages"]
        )

        # 只有成功获取到数据时才缓存
        if result.videos:
            _cache[cache_key] = response

        return response

    except Exception as e:
        raise HTTPException(status_code=500, detail=f"获取视频列表失败: {str(e)}")


@router.get("/{video_id}", response_model=VideoDetail)
async def get_video_detail(video_id: str):
    """获取视频详情"""
    cache_key = f"detail_{video_id}"

    if cache_key in _cache:
        return _cache[cache_key]

    try:
        # 构建91porn视频页URL (使用viewkey参数)
        from ..config import settings

        video_url = f"{settings.TARGET_BASE_URL}/view_video.php?viewkey={video_id}"
        detail = await scraper_service.get_video_detail(video_url)

        if not detail:
            raise HTTPException(status_code=404, detail="视频不存在")

        _cache[cache_key] = detail
        return detail

    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"获取视频详情失败: {str(e)}")


@router.delete("/cache")
async def clear_cache():
    """清除缓存"""
    _cache.clear()
    _total_pages_cache["total_pages"] = 1
    return {"message": "缓存已清除"}
