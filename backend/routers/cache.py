from fastapi import APIRouter, HTTPException, Header, Query
from typing import Optional
from ..services.video_cache import video_cache_service
from ..config import settings

router = APIRouter(prefix="/api/cache", tags=["cache"])


def verify_admin(admin_token: Optional[str] = None):
    """验证管理员权限"""
    if admin_token != settings.ADMIN_PASSWORD:
        raise HTTPException(status_code=403, detail="需要管理员权限")


@router.get("")
async def list_cached_videos(
    page: int = Query(1, ge=1, description="页码"),
    page_size: int = Query(None, ge=1, le=100, description="每页数量")
):
    """列出已缓存的视频（分页）"""
    if page_size is None:
        page_size = settings.CACHE_PAGE_SIZE

    cached = await video_cache_service.list_cached_videos()
    total_size = video_cache_service.get_cache_size()
    total_count = len(cached)
    total_pages = (total_count + page_size - 1) // page_size if total_count > 0 else 1

    # 分页
    start = (page - 1) * page_size
    end = start + page_size
    paged_videos = cached[start:end]

    return {
        "enabled": settings.VIDEO_CACHE_ENABLED,
        "cache_dir": settings.VIDEO_CACHE_DIR,
        "total_size": total_size,
        "total_size_mb": round(total_size / (1024 * 1024), 2),
        "videos": paged_videos,
        "total": total_count,
        "page": page,
        "page_size": page_size,
        "total_pages": total_pages,
    }


@router.get("/{viewkey}")
async def get_cache_status(viewkey: str):
    """获取指定视频的缓存状态"""
    is_cached = video_cache_service.is_cached(viewkey)
    is_downloading = video_cache_service.is_downloading(viewkey)
    progress = video_cache_service.get_download_progress(viewkey)

    return {
        "viewkey": viewkey,
        "is_cached": is_cached,
        "is_downloading": is_downloading,
        "progress": progress,
    }


@router.delete("/{viewkey}")
async def delete_cached_video(viewkey: str, x_admin_token: Optional[str] = Header(None)):
    """删除指定视频的缓存（需要管理员权限）"""
    verify_admin(x_admin_token)

    deleted = await video_cache_service.delete_cached_video(viewkey)

    if not deleted:
        raise HTTPException(status_code=404, detail="缓存不存在")

    return {"message": f"已删除视频缓存: {viewkey}"}


@router.delete("")
async def clear_all_cache(x_admin_token: Optional[str] = Header(None)):
    """清除所有视频缓存（需要管理员权限）"""
    verify_admin(x_admin_token)

    count = await video_cache_service.clear_all_cache()
    return {"message": f"已清除 {count} 个视频缓存"}
