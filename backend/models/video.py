from pydantic import BaseModel
from typing import Optional, List


class VideoItem(BaseModel):
    """视频列表项"""
    id: str
    title: str
    thumbnail: Optional[str] = None
    url: str
    duration: Optional[str] = None


class VideoDetail(BaseModel):
    """视频详情"""
    id: str
    title: str
    thumbnail: Optional[str] = None
    m3u8_url: Optional[str] = None
    original_url: str


class VideoListResponse(BaseModel):
    """视频列表响应"""
    videos: List[VideoItem]
    total: int
    page: int = 1
    total_pages: int = 1


class StreamInfo(BaseModel):
    """流信息"""
    video_id: str
    m3u8_url: str
    proxy_url: str
