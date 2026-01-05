import aiohttp
from urllib.parse import urljoin, urlparse
from typing import Optional
import re


class ProxyService:
    """M3U8代理服务"""

    def __init__(self):
        self._session: Optional[aiohttp.ClientSession] = None

    async def get_session(self) -> aiohttp.ClientSession:
        """获取HTTP会话"""
        if self._session is None or self._session.closed:
            self._session = aiohttp.ClientSession(
                headers={
                    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
                    "Accept": "*/*",
                    "Accept-Language": "en-US,en;q=0.9",
                    "Cookie": "language=cn_CN",
                }
            )
        return self._session

    async def close(self):
        """关闭会话"""
        if self._session and not self._session.closed:
            await self._session.close()

    async def fetch_m3u8(self, m3u8_url: str, proxy_base_url: str) -> str:
        """获取并重写m3u8文件"""
        session = await self.get_session()

        print(f"正在获取m3u8: {m3u8_url}")

        async with session.get(m3u8_url) as response:
            if response.status != 200:
                raise Exception(f"获取m3u8失败: {response.status}")

            content = await response.text()
            print(f"m3u8原始内容前500字符:\n{content[:500]}")

        # 检查是否真的是m3u8格式
        if not content.strip().startswith("#EXTM3U"):
            print("警告: 内容不是标准m3u8格式")
            # 可能是重定向URL
            if content.strip().startswith("http"):
                print(f"检测到重定向URL: {content.strip()}")
                return await self.fetch_m3u8(content.strip().split()[0], proxy_base_url)
            # 不是m3u8格式，抛出异常让调用方处理为MP4
            raise Exception("内容不是m3u8格式，可能是MP4文件")

        # 重写m3u8内容
        result = self._rewrite_m3u8(content, m3u8_url, proxy_base_url)
        print(f"m3u8重写后内容前500字符:\n{result[:500]}")
        return result

    def _rewrite_m3u8(self, content: str, original_url: str, proxy_base_url: str) -> str:
        """重写m3u8文件中的URL"""
        lines = content.split("\n")
        new_lines = []
        base_url = self._get_base_url(original_url)

        for line in lines:
            line = line.strip()

            if not line:
                new_lines.append(line)
                continue

            # 跳过注释行但保留
            if line.startswith("#"):
                # 处理 #EXT-X-KEY 等包含URI的行
                if "URI=" in line:
                    line = self._rewrite_uri_in_tag(line, base_url, proxy_base_url)
                new_lines.append(line)
                continue

            # 非注释行都当作资源URL处理
            # 转换为绝对URL
            if not line.startswith("http"):
                absolute_url = urljoin(base_url, line)
            else:
                absolute_url = line

            # 生成代理URL
            proxy_url = self._create_proxy_url(absolute_url, proxy_base_url)
            new_lines.append(proxy_url)

        return "\n".join(new_lines)

    def _rewrite_uri_in_tag(self, line: str, base_url: str, proxy_base_url: str) -> str:
        """重写标签中的URI"""
        uri_match = re.search(r'URI="([^"]+)"', line)
        if uri_match:
            original_uri = uri_match.group(1)
            if not original_uri.startswith("http"):
                absolute_uri = urljoin(base_url, original_uri)
            else:
                absolute_uri = original_uri
            proxy_uri = self._create_proxy_url(absolute_uri, proxy_base_url)
            line = line.replace(f'URI="{original_uri}"', f'URI="{proxy_uri}"')
        return line

    def _get_base_url(self, url: str) -> str:
        """获取URL的基础路径"""
        parsed = urlparse(url)
        path = parsed.path
        if "/" in path:
            path = path.rsplit("/", 1)[0] + "/"
        return f"{parsed.scheme}://{parsed.netloc}{path}"

    def _create_proxy_url(self, original_url: str, proxy_base_url: str) -> str:
        """创建代理URL"""
        from base64 import urlsafe_b64encode
        encoded = urlsafe_b64encode(original_url.encode()).decode()
        return f"{proxy_base_url}/api/stream/segment/{encoded}"

    async def fetch_segment(self, segment_url: str) -> tuple[bytes, str]:
        """获取ts分片或其他资源"""
        session = await self.get_session()

        async with session.get(segment_url) as response:
            if response.status != 200:
                raise Exception(f"获取分片失败: {response.status}")

            content = await response.read()
            content_type = response.headers.get("Content-Type", "video/MP2T")

        return content, content_type


# 全局单例
proxy_service = ProxyService()
