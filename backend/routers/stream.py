from fastapi import APIRouter, HTTPException, Response, Request
from fastapi.responses import StreamingResponse, FileResponse
from base64 import urlsafe_b64decode, urlsafe_b64encode
from ..services.proxy import proxy_service
from ..services.scraper import scraper_service
from ..services.video_cache import video_cache_service
from ..config import settings
import aiohttp
import asyncio

router = APIRouter(prefix="/api/stream", tags=["stream"])

# 缓存视频 URL
_video_cache = {}

# 全局连接池（复用连接）
_connector = None
_session = None


async def get_session():
    """获取或创建全局 aiohttp session"""
    global _connector, _session
    if _session is None or _session.closed:
        # 创建优化的连接器
        _connector = aiohttp.TCPConnector(
            limit=100,  # 最大连接数
            limit_per_host=10,  # 每个主机最大连接数
            ttl_dns_cache=300,  # DNS缓存时间
            enable_cleanup_closed=True,
        )
        _session = aiohttp.ClientSession(
            connector=_connector,
            timeout=aiohttp.ClientTimeout(
                total=None,  # 不限制总时间（长视频需要）
                connect=10,  # 连接超时10秒
                sock_read=30,  # 读取超时30秒
            ),
        )
    return _session


@router.get("/{video_id}")
async def get_stream(video_id: str, request: Request):
    """获取视频流代理"""
    print(f"=== 收到流请求: video_id={video_id} ===")

    # 检查本地缓存
    if settings.VIDEO_CACHE_ENABLED and video_cache_service.is_cached(video_id):
        print(f"[Cache] 使用本地缓存: {video_id}")

        # 检查是MP4还是M3U8缓存
        mp4_path = video_cache_service.get_cached_mp4_path(video_id)
        if mp4_path:
            print(f"[Cache] 返回缓存的MP4: {mp4_path}")
            return await serve_cached_mp4(mp4_path, request)

        # 返回缓存的M3U8
        m3u8_content = await video_cache_service.get_cached_m3u8(video_id)
        if m3u8_content:
            # 重写m3u8中的分片路径为代理URL
            proxy_base = settings.PROXY_BASE_URL
            rewritten_m3u8 = _rewrite_cached_m3u8(m3u8_content, video_id, proxy_base)
            return Response(
                content=rewritten_m3u8,
                media_type="application/vnd.apple.mpegurl",
                headers={
                    "Access-Control-Allow-Origin": "*",
                    "Cache-Control": "no-cache",
                }
            )

    cache_key = f"video_{video_id}"
    detail = None

    # 检查URL缓存（同时缓存了detail）
    if cache_key in _video_cache:
        cached = _video_cache[cache_key]
        video_url = cached["url"]
        detail = cached.get("detail")
        print(f"使用缓存的URL: {video_url}")
    else:
        # 构建视频页URL
        page_url = f"{settings.TARGET_BASE_URL}/view_video.php?viewkey={video_id}"
        print(f"获取视频详情: {page_url}")
        detail = await scraper_service.get_video_detail(page_url)

        if not detail or not detail.m3u8_url:
            print("错误: 无法获取视频流URL")
            raise HTTPException(status_code=404, detail="无法获取视频流")

        video_url = detail.m3u8_url
        _video_cache[cache_key] = {"url": video_url, "detail": detail}
        print(f"获取到视频URL: {video_url}")

    # 判断是 MP4 还是 M3U8
    is_mp4 = ".mp4" in video_url.lower() or not ".m3u8" in video_url.lower()

    if is_mp4:
        print("检测到MP4格式，使用流式代理")
        # 启动后台缓存下载
        if settings.VIDEO_CACHE_ENABLED and detail:
            await video_cache_service.start_mp4_cache_download(video_id, video_url, detail)
        return await proxy_mp4_stream(video_url, request)
    else:
        print("检测到M3U8格式，重写并代理")
        try:
            proxy_base = settings.PROXY_BASE_URL
            m3u8_content = await proxy_service.fetch_m3u8(video_url, proxy_base)

            # 启动后台缓存下载（传入原始m3u8内容用于下载）
            if settings.VIDEO_CACHE_ENABLED and detail:
                # 获取原始m3u8内容（未重写的）
                session = await proxy_service.get_session()
                async with session.get(video_url) as resp:
                    original_m3u8 = await resp.text()
                await video_cache_service.start_cache_download(video_id, video_url, original_m3u8, detail)

            return Response(
                content=m3u8_content,
                media_type="application/vnd.apple.mpegurl",
                headers={
                    "Access-Control-Allow-Origin": "*",
                    "Cache-Control": "no-cache",
                }
            )
        except Exception as e:
            print(f"M3U8处理失败: {e}，尝试作为MP4代理")
            return await proxy_mp4_stream(video_url, request)


def _rewrite_cached_m3u8(content: str, viewkey: str, proxy_base: str) -> str:
    """重写缓存的m3u8文件，将本地分片路径改为代理URL"""
    lines = content.split("\n")
    new_lines = []

    for line in lines:
        line = line.strip()
        if not line:
            new_lines.append(line)
            continue

        if line.startswith("#"):
            new_lines.append(line)
            continue

        # 非注释行是分片文件名（如 0.ts, 1.ts）
        # 转换为代理URL: /api/stream/cached-segment/{viewkey}/{segment_name}
        proxy_url = f"{proxy_base}/api/stream/cached-segment/{viewkey}/{line}"
        new_lines.append(proxy_url)

    return "\n".join(new_lines)


async def serve_cached_mp4(mp4_path, request: Request):
    """服务缓存的MP4文件，支持Range请求"""
    import os
    import aiofiles

    file_size = os.path.getsize(mp4_path)
    range_header = request.headers.get("range")

    if range_header:
        # 解析Range头
        range_match = range_header.replace("bytes=", "").split("-")
        start = int(range_match[0]) if range_match[0] else 0
        end = int(range_match[1]) if range_match[1] else file_size - 1

        content_length = end - start + 1

        async def stream_file():
            async with aiofiles.open(mp4_path, "rb") as f:
                await f.seek(start)
                remaining = content_length
                chunk_size = 512 * 1024
                while remaining > 0:
                    read_size = min(chunk_size, remaining)
                    chunk = await f.read(read_size)
                    if not chunk:
                        break
                    remaining -= len(chunk)
                    yield chunk

        return StreamingResponse(
            stream_file(),
            status_code=206,
            headers={
                "Content-Type": "video/mp4",
                "Content-Length": str(content_length),
                "Content-Range": f"bytes {start}-{end}/{file_size}",
                "Accept-Ranges": "bytes",
                "Access-Control-Allow-Origin": "*",
            }
        )
    else:
        # 完整文件请求
        return FileResponse(
            mp4_path,
            media_type="video/mp4",
            headers={
                "Accept-Ranges": "bytes",
                "Access-Control-Allow-Origin": "*",
            }
        )


async def proxy_mp4_stream(url: str, request: Request):
    """代理MP4视频流，支持Range请求"""
    print(f"=== 代理MP4流: {url} ===")

    headers = {
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
        "Referer": settings.TARGET_BASE_URL,
        "Accept": "*/*",
        "Accept-Encoding": "identity",  # 不压缩，直接传输
    }

    # 传递Range头支持视频拖动
    range_header = request.headers.get("range")
    if range_header:
        headers["Range"] = range_header
        print(f"Range请求: {range_header}")

    session = await get_session()

    try:
        resp = await session.get(url, headers=headers)

        content_length = resp.headers.get("Content-Length", "")
        content_type = resp.headers.get("Content-Type", "video/mp4")
        content_range = resp.headers.get("Content-Range", "")
        status_code = resp.status

        print(f"上游响应: status={status_code}, content-type={content_type}, length={content_length}")

        # 使用更大的缓冲区 (512KB) 提高传输效率
        CHUNK_SIZE = 512 * 1024

        async def stream_response():
            try:
                async for chunk in resp.content.iter_chunked(CHUNK_SIZE):
                    yield chunk
            finally:
                resp.release()

        response_headers = {
            "Access-Control-Allow-Origin": "*",
            "Accept-Ranges": "bytes",
            "Content-Type": content_type,
            "Cache-Control": "public, max-age=3600",  # 允许浏览器缓存
        }

        if content_length:
            response_headers["Content-Length"] = content_length

        # 如果上游返回了Content-Range，传递给客户端
        if content_range:
            response_headers["Content-Range"] = content_range

        return StreamingResponse(
            stream_response(),
            status_code=status_code,  # 使用上游的状态码 (200 或 206)
            headers=response_headers
        )
    except Exception as e:
        print(f"MP4代理失败: {e}")
        raise HTTPException(status_code=500, detail=f"MP4代理失败: {str(e)}")


@router.get("/segment/{encoded_url:path}")
async def get_segment(encoded_url: str):
    """代理获取ts分片或其他资源"""
    try:
        # 解码原始URL
        original_url = urlsafe_b64decode(encoded_url.encode()).decode()

        # 判断是m3u8还是其他资源
        if ".m3u8" in original_url:
            # 如果是子m3u8，也需要重写
            proxy_base = settings.PROXY_BASE_URL
            content = await proxy_service.fetch_m3u8(original_url, proxy_base)
            return Response(
                content=content,
                media_type="application/vnd.apple.mpegurl",
                headers={
                    "Access-Control-Allow-Origin": "*",
                    "Cache-Control": "no-cache",
                }
            )
        else:
            # 获取分片
            content, content_type = await proxy_service.fetch_segment(original_url)

            return Response(
                content=content,
                media_type=content_type,
                headers={
                    "Access-Control-Allow-Origin": "*",
                    "Cache-Control": "max-age=3600",
                }
            )

    except Exception as e:
        raise HTTPException(status_code=500, detail=f"获取资源失败: {str(e)}")


@router.get("/cached-segment/{viewkey}/{segment_name}")
async def get_cached_segment(viewkey: str, segment_name: str):
    """获取本地缓存的分片"""
    content = await video_cache_service.get_cached_segment(viewkey, segment_name)

    if content is None:
        raise HTTPException(status_code=404, detail="缓存分片不存在")

    return Response(
        content=content,
        media_type="video/MP2T",
        headers={
            "Access-Control-Allow-Origin": "*",
            "Cache-Control": "max-age=86400",
        }
    )


@router.get("/direct")
async def get_direct_stream(url: str):
    """直接获取m3u8内容（通过URL参数）"""
    try:
        proxy_base = settings.PROXY_BASE_URL
        m3u8_content = await proxy_service.fetch_m3u8(url, proxy_base)

        return Response(
            content=m3u8_content,
            media_type="application/vnd.apple.mpegurl",
            headers={
                "Access-Control-Allow-Origin": "*",
                "Cache-Control": "no-cache",
            }
        )

    except Exception as e:
        raise HTTPException(status_code=500, detail=f"获取视频流失败: {str(e)}")


@router.delete("/cache")
async def clear_stream_cache():
    """清除URL缓存"""
    _video_cache.clear()
    return {"message": "流缓存已清除"}


@router.get("/image/{video_id}")
async def get_image(video_id: str, url: str = None):
    """获取视频封面图代理"""
    # 优先使用本地缓存
    if settings.VIDEO_CACHE_ENABLED:
        thumb_path = video_cache_service.get_cached_thumbnail_path(video_id)
        if thumb_path:
            return FileResponse(
                thumb_path,
                media_type="image/jpeg",
                headers={
                    "Access-Control-Allow-Origin": "*",
                    "Cache-Control": "public, max-age=86400",
                }
            )

    # 没有缓存且没有提供URL，返回404
    if not url:
        raise HTTPException(status_code=404, detail="封面图未缓存且未提供原始URL")

    # 代理远程图片
    try:
        session = await get_session()
        headers = {
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
            "Referer": settings.TARGET_BASE_URL,
        }

        async with session.get(url, headers=headers) as resp:
            if resp.status != 200:
                raise HTTPException(status_code=resp.status, detail="获取图片失败")

            content = await resp.read()
            content_type = resp.headers.get("Content-Type", "image/jpeg")

            # 后台缓存图片
            if settings.VIDEO_CACHE_ENABLED:
                asyncio.create_task(video_cache_service.download_thumbnail(video_id, url))

            return Response(
                content=content,
                media_type=content_type,
                headers={
                    "Access-Control-Allow-Origin": "*",
                    "Cache-Control": "public, max-age=86400",
                }
            )
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"获取图片失败: {str(e)}")
