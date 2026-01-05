from fastapi import APIRouter, HTTPException, Response, Request
from fastapi.responses import StreamingResponse
from base64 import urlsafe_b64decode, urlsafe_b64encode
from ..services.proxy import proxy_service
from ..services.scraper import scraper_service
from ..config import settings
import aiohttp

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

    cache_key = f"video_{video_id}"

    # 检查缓存
    if cache_key in _video_cache:
        video_url = _video_cache[cache_key]
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
        _video_cache[cache_key] = video_url
        print(f"获取到视频URL: {video_url}")

    # 判断是 MP4 还是 M3U8
    is_mp4 = ".mp4" in video_url.lower() or not ".m3u8" in video_url.lower()

    if is_mp4:
        print("检测到MP4格式，使用流式代理")
        return await proxy_mp4_stream(video_url, request)
    else:
        print("检测到M3U8格式，重写并代理")
        try:
            proxy_base = settings.PROXY_BASE_URL
            m3u8_content = await proxy_service.fetch_m3u8(video_url, proxy_base)
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
    """清除m3u8缓存"""
    _m3u8_cache.clear()
    return {"message": "流缓存已清除"}
