import os
import asyncio
import aiohttp
import aiofiles
import json
import re
from urllib.parse import urljoin, urlparse
from typing import Optional, Dict, List, Any
from pathlib import Path
from ..config import settings


class VideoCacheService:
    """视频本地缓存服务"""

    def __init__(self):
        self._download_tasks: Dict[str, asyncio.Task] = {}
        self._download_progress: Dict[str, dict] = {}
        self._session: Optional[aiohttp.ClientSession] = None
        self._cache_dir = Path(settings.VIDEO_CACHE_DIR)

    async def _get_session(self) -> aiohttp.ClientSession:
        """获取HTTP会话"""
        if self._session is None or self._session.closed:
            self._session = aiohttp.ClientSession(
                headers={
                    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
                    "Accept": "*/*",
                    "Referer": settings.TARGET_BASE_URL,
                },
                timeout=aiohttp.ClientTimeout(total=300, connect=10),
            )
        return self._session

    def _get_video_cache_dir(self, viewkey: str) -> Path:
        """获取视频缓存目录"""
        return self._cache_dir / viewkey

    def _get_mp4_cache_path(self, viewkey: str) -> Path:
        """获取MP4缓存路径"""
        return self._cache_dir / f"{viewkey}.mp4"

    def _get_thumbnail_cache_path(self, viewkey: str) -> Path:
        """获取封面图缓存路径"""
        return self._cache_dir / f"{viewkey}.jpg"

    def _get_list_cache_path(self, page: int) -> Path:
        """获取列表缓存路径"""
        return self._cache_dir / f"list_page_{page}.json"

    def _ensure_cache_dir(self, viewkey: str) -> Path:
        """确保缓存目录存在"""
        cache_dir = self._get_video_cache_dir(viewkey)
        cache_dir.mkdir(parents=True, exist_ok=True)
        return cache_dir

    def is_cached(self, viewkey: str) -> bool:
        """检查视频是否已完整缓存"""
        # 检查MP4
        mp4_path = self._get_mp4_cache_path(viewkey)
        if mp4_path.exists():
            return True

        # 检查M3U8
        cache_dir = self._get_video_cache_dir(viewkey)
        m3u8_path = cache_dir / "video.m3u8"
        complete_marker = cache_dir / ".complete"

        return m3u8_path.exists() and complete_marker.exists()

    def is_downloading(self, viewkey: str) -> bool:
        """检查视频是否正在下载"""
        return viewkey in self._download_tasks and not self._download_tasks[viewkey].done()

    def get_download_progress(self, viewkey: str) -> Optional[dict]:
        """获取下载进度"""
        return self._download_progress.get(viewkey)

    async def get_cached_m3u8(self, viewkey: str) -> Optional[str]:
        """获取缓存的m3u8内容（重写为本地分片路径）"""
        cache_dir = self._get_video_cache_dir(viewkey)
        m3u8_path = cache_dir / "video.m3u8"

        if not m3u8_path.exists():
            return None

        async with aiofiles.open(m3u8_path, "r") as f:
            return await f.read()

    async def get_cached_segment(self, viewkey: str, segment_name: str) -> Optional[bytes]:
        """获取缓存的分片"""
        cache_dir = self._get_video_cache_dir(viewkey)
        segment_path = cache_dir / segment_name

        if not segment_path.exists():
            return None

        async with aiofiles.open(segment_path, "rb") as f:
            return await f.read()

    def get_cached_mp4_path(self, viewkey: str) -> Optional[Path]:
        """获取缓存的MP4路径"""
        mp4_path = self._get_mp4_cache_path(viewkey)
        if mp4_path.exists():
            return mp4_path
        return None

    def get_cached_thumbnail_path(self, viewkey: str) -> Optional[Path]:
        """获取缓存的封面图路径"""
        thumb_path = self._get_thumbnail_cache_path(viewkey)
        if thumb_path.exists():
            return thumb_path
        return None

    async def download_thumbnail(self, viewkey: str, thumbnail_url: str) -> bool:
        """下载并缓存封面图"""
        if not thumbnail_url:
            return False

        try:
            self._cache_dir.mkdir(parents=True, exist_ok=True)
            thumb_path = self._get_thumbnail_cache_path(viewkey)

            if thumb_path.exists():
                return True

            session = await self._get_session()
            async with session.get(thumbnail_url) as resp:
                if resp.status == 200:
                    content = await resp.read()
                    async with aiofiles.open(thumb_path, "wb") as f:
                        await f.write(content)
                    print(f"[Cache] 已缓存封面图: {viewkey}")
                    return True
        except Exception as e:
            print(f"[Cache] 下载封面图失败 {viewkey}: {e}")
        return False

    async def get_cached_list(self, page: int, max_age: int = None) -> Optional[dict]:
        """获取缓存的视频列表

        Args:
            page: 页码
            max_age: 最大缓存有效期（秒），None 表示不检查时间

        Returns:
            缓存数据，如果不存在或已过期返回 None
        """
        list_path = self._get_list_cache_path(page)

        if not list_path.exists():
            return None

        try:
            # 检查缓存时间
            if max_age is not None:
                import time
                file_mtime = list_path.stat().st_mtime
                if time.time() - file_mtime > max_age:
                    print(f"[Cache] 列表缓存已过期: 第{page}页")
                    return None

            async with aiofiles.open(list_path, "r", encoding="utf-8") as f:
                data = json.loads(await f.read())
                print(f"[Cache] 读取列表缓存: 第{page}页")
                return data
        except Exception as e:
            print(f"[Cache] 读取列表缓存失败 page={page}: {e}")
            return None

    async def save_list_cache(self, page: int, data: dict):
        """保存视频列表到缓存"""
        try:
            self._cache_dir.mkdir(parents=True, exist_ok=True)
            list_path = self._get_list_cache_path(page)

            async with aiofiles.open(list_path, "w", encoding="utf-8") as f:
                await f.write(json.dumps(data, ensure_ascii=False, indent=2))
            print(f"[Cache] 已保存列表缓存: 第{page}页")
        except Exception as e:
            print(f"[Cache] 保存列表缓存失败 page={page}: {e}")

    def _get_detail_path(self, viewkey: str) -> Path:
        """获取详情缓存文件路径"""
        # M3U8格式存在目录下，MP4格式存在同级目录
        cache_dir = self._get_video_cache_dir(viewkey)
        if cache_dir.exists():
            return cache_dir / "detail.json"
        # MP4格式的详情文件
        return self._cache_dir / f"{viewkey}.detail.json"

    async def get_cached_detail(self, viewkey: str) -> Optional[Any]:
        """获取缓存的视频详情"""
        from ..models.video import VideoDetail

        # 检查M3U8格式的详情
        cache_dir = self._get_video_cache_dir(viewkey)
        detail_path = cache_dir / "detail.json"

        if not detail_path.exists():
            # 检查MP4格式的详情
            detail_path = self._cache_dir / f"{viewkey}.detail.json"

        if not detail_path.exists():
            return None

        try:
            async with aiofiles.open(detail_path, "r", encoding="utf-8") as f:
                data = json.loads(await f.read())
                return VideoDetail(**data)
        except Exception as e:
            print(f"[Cache] 读取详情失败 {viewkey}: {e}")
            return None

    async def save_detail(self, viewkey: str, detail: Any):
        """保存视频详情到缓存"""
        # 确定保存路径
        cache_dir = self._get_video_cache_dir(viewkey)
        if cache_dir.exists():
            detail_path = cache_dir / "detail.json"
        else:
            self._cache_dir.mkdir(parents=True, exist_ok=True)
            detail_path = self._cache_dir / f"{viewkey}.detail.json"

        try:
            # 转换为字典
            if hasattr(detail, "model_dump"):
                data = detail.model_dump()
            elif hasattr(detail, "dict"):
                data = detail.dict()
            else:
                data = dict(detail)

            async with aiofiles.open(detail_path, "w", encoding="utf-8") as f:
                await f.write(json.dumps(data, ensure_ascii=False, indent=2))
            print(f"[Cache] 已保存详情: {viewkey}")
        except Exception as e:
            print(f"[Cache] 保存详情失败 {viewkey}: {e}")

    async def start_cache_download(self, viewkey: str, m3u8_url: str, m3u8_content: str, detail: Any = None):
        """启动后台下载任务（M3U8格式）"""
        if not settings.VIDEO_CACHE_ENABLED:
            return

        if self.is_cached(viewkey) or self.is_downloading(viewkey):
            return

        task = asyncio.create_task(self._download_m3u8_video(viewkey, m3u8_url, m3u8_content, detail))
        self._download_tasks[viewkey] = task

    async def start_mp4_cache_download(self, viewkey: str, mp4_url: str, detail: Any = None):
        """启动后台下载任务（MP4格式）"""
        if not settings.VIDEO_CACHE_ENABLED:
            return

        if self.is_cached(viewkey) or self.is_downloading(viewkey):
            return

        task = asyncio.create_task(self._download_mp4_video(viewkey, mp4_url, detail))
        self._download_tasks[viewkey] = task

    async def _download_m3u8_video(self, viewkey: str, m3u8_url: str, m3u8_content: str, detail: Any = None):
        """下载M3U8视频的所有分片"""
        try:
            print(f"[Cache] 开始下载视频: {viewkey}")
            cache_dir = self._ensure_cache_dir(viewkey)

            # 同时下载封面图
            if detail and hasattr(detail, 'thumbnail') and detail.thumbnail:
                await self.download_thumbnail(viewkey, detail.thumbnail)

            # 解析m3u8获取分片URL列表
            segments = self._parse_m3u8_segments(m3u8_content, m3u8_url)

            self._download_progress[viewkey] = {
                "total": len(segments),
                "downloaded": 0,
                "status": "downloading",
            }

            session = await self._get_session()
            local_m3u8_lines = []
            segment_index = 0

            for line in m3u8_content.split("\n"):
                line = line.strip()
                if not line:
                    local_m3u8_lines.append(line)
                    continue

                if line.startswith("#"):
                    # 处理带有URI的标签
                    if "URI=" in line:
                        line = self._rewrite_uri_for_local(line, viewkey, segment_index)
                    local_m3u8_lines.append(line)
                    continue

                # 这是一个分片URL
                segment_url = segments[segment_index]["url"]
                segment_name = f"{segment_index}.ts"

                # 下载分片
                try:
                    async with session.get(segment_url) as resp:
                        if resp.status == 200:
                            content = await resp.read()
                            segment_path = cache_dir / segment_name
                            async with aiofiles.open(segment_path, "wb") as f:
                                await f.write(content)
                            print(f"[Cache] {viewkey}: 已下载分片 {segment_index + 1}/{len(segments)}")
                except Exception as e:
                    print(f"[Cache] 分片下载失败 {segment_url}: {e}")

                # 写入本地分片名称
                local_m3u8_lines.append(segment_name)
                segment_index += 1

                self._download_progress[viewkey]["downloaded"] = segment_index

            # 保存本地m3u8
            m3u8_path = cache_dir / "video.m3u8"
            async with aiofiles.open(m3u8_path, "w") as f:
                await f.write("\n".join(local_m3u8_lines))

            # 创建完成标记
            complete_marker = cache_dir / ".complete"
            async with aiofiles.open(complete_marker, "w") as f:
                await f.write("complete")

            # 保存视频详情
            if detail:
                await self.save_detail(viewkey, detail)

            self._download_progress[viewkey]["status"] = "complete"
            print(f"[Cache] 视频下载完成: {viewkey}")

        except Exception as e:
            print(f"[Cache] 视频下载失败 {viewkey}: {e}")
            self._download_progress[viewkey] = {
                "status": "error",
                "error": str(e),
            }
        finally:
            if viewkey in self._download_tasks:
                del self._download_tasks[viewkey]

    async def _download_mp4_video(self, viewkey: str, mp4_url: str, detail: Any = None):
        """下载MP4视频"""
        try:
            print(f"[Cache] 开始下载MP4: {viewkey}")
            self._cache_dir.mkdir(parents=True, exist_ok=True)

            # 同时下载封面图
            if detail and hasattr(detail, 'thumbnail') and detail.thumbnail:
                await self.download_thumbnail(viewkey, detail.thumbnail)

            mp4_path = self._get_mp4_cache_path(viewkey)
            temp_path = self._cache_dir / f"{viewkey}.mp4.tmp"

            session = await self._get_session()

            self._download_progress[viewkey] = {
                "status": "downloading",
                "downloaded": 0,
                "total": 0,
            }

            async with session.get(mp4_url) as resp:
                if resp.status != 200:
                    raise Exception(f"下载失败: HTTP {resp.status}")

                total_size = int(resp.headers.get("Content-Length", 0))
                self._download_progress[viewkey]["total"] = total_size

                async with aiofiles.open(temp_path, "wb") as f:
                    downloaded = 0
                    async for chunk in resp.content.iter_chunked(512 * 1024):
                        await f.write(chunk)
                        downloaded += len(chunk)
                        self._download_progress[viewkey]["downloaded"] = downloaded

            # 重命名为最终文件
            temp_path.rename(mp4_path)

            # 保存视频详情
            if detail:
                await self.save_detail(viewkey, detail)

            self._download_progress[viewkey]["status"] = "complete"
            print(f"[Cache] MP4下载完成: {viewkey}")

        except Exception as e:
            print(f"[Cache] MP4下载失败 {viewkey}: {e}")
            self._download_progress[viewkey] = {
                "status": "error",
                "error": str(e),
            }
            # 清理临时文件
            temp_path = self._cache_dir / f"{viewkey}.mp4.tmp"
            if temp_path.exists():
                temp_path.unlink()
        finally:
            if viewkey in self._download_tasks:
                del self._download_tasks[viewkey]

    def _parse_m3u8_segments(self, content: str, base_url: str) -> List[dict]:
        """解析m3u8文件获取分片URL列表"""
        segments = []
        base = self._get_base_url(base_url)

        for line in content.split("\n"):
            line = line.strip()
            if not line or line.startswith("#"):
                continue

            # 转换为绝对URL
            if not line.startswith("http"):
                segment_url = urljoin(base, line)
            else:
                segment_url = line

            segments.append({"url": segment_url})

        return segments

    def _get_base_url(self, url: str) -> str:
        """获取URL的基础路径"""
        parsed = urlparse(url)
        path = parsed.path
        if "/" in path:
            path = path.rsplit("/", 1)[0] + "/"
        return f"{parsed.scheme}://{parsed.netloc}{path}"

    def _rewrite_uri_for_local(self, line: str, viewkey: str, segment_index: int) -> str:
        """重写标签中的URI为本地路径"""
        # 简单处理：将URI替换为本地分片路径
        uri_match = re.search(r'URI="([^"]+)"', line)
        if uri_match:
            # 对于key文件等，暂时保留原始URL
            pass
        return line

    async def list_cached_videos(self) -> List[dict]:
        """列出所有已缓存的视频"""
        if not self._cache_dir.exists():
            return []

        cached = []

        # 检查M3U8格式缓存
        for item in self._cache_dir.iterdir():
            if item.is_dir():
                complete_marker = item / ".complete"
                if complete_marker.exists():
                    # 计算目录大小
                    size = sum(f.stat().st_size for f in item.iterdir() if f.is_file())
                    cached.append({
                        "viewkey": item.name,
                        "type": "m3u8",
                        "size": size,
                    })
            elif item.suffix == ".mp4":
                cached.append({
                    "viewkey": item.stem,
                    "type": "mp4",
                    "size": item.stat().st_size,
                })

        return cached

    async def delete_cached_video(self, viewkey: str) -> bool:
        """删除指定视频的缓存"""
        deleted = False

        # 删除M3U8缓存目录
        cache_dir = self._get_video_cache_dir(viewkey)
        if cache_dir.exists():
            import shutil
            shutil.rmtree(cache_dir)
            deleted = True

        # 删除MP4缓存
        mp4_path = self._get_mp4_cache_path(viewkey)
        if mp4_path.exists():
            mp4_path.unlink()
            deleted = True

        return deleted

    async def clear_all_cache(self) -> int:
        """清除所有缓存，返回删除的数量"""
        if not self._cache_dir.exists():
            return 0

        import shutil
        count = len(list(self._cache_dir.iterdir()))
        shutil.rmtree(self._cache_dir)
        self._cache_dir.mkdir(parents=True, exist_ok=True)
        return count

    def get_cache_size(self) -> int:
        """获取缓存总大小（字节）"""
        if not self._cache_dir.exists():
            return 0

        total = 0
        for item in self._cache_dir.rglob("*"):
            if item.is_file():
                total += item.stat().st_size
        return total

    async def close(self):
        """关闭会话"""
        if self._session and not self._session.closed:
            await self._session.close()


# 全局单例
video_cache_service = VideoCacheService()
