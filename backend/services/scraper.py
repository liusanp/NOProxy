import asyncio
import json
import re
import time
from pathlib import Path
from typing import List, Optional
from urllib.parse import urljoin, urlparse, parse_qs
from playwright.async_api import async_playwright, Browser, Page, BrowserContext
from ..config import settings
from ..models.video import VideoItem, VideoDetail


# Cookies存储文件
COOKIES_FILE = Path(__file__).parent.parent / "cookies.json"


class VideoListResult:
    """视频列表结果"""
    def __init__(self, videos: List[VideoItem], total_pages: int = 1):
        self.videos = videos
        self.total_pages = total_pages


class ScraperService:
    """Playwright解析服务"""

    def __init__(self):
        self._playwright = None
        self._browser: Optional[Browser] = None
        self._context: Optional[BrowserContext] = None
        self._page: Optional[Page] = None
        self._lock = asyncio.Lock()
        self._current_page_num = 0  # 当前所在页码
        self._pending_requests = 0  # 正在进行的请求数

    def load_cookies(self) -> list:
        """从文件加载cookies"""
        if COOKIES_FILE.exists():
            try:
                with open(COOKIES_FILE, "r") as f:
                    return json.load(f)
            except:
                pass
        return []

    def save_cookies(self, cookies: list):
        """保存cookies到文件"""
        with open(COOKIES_FILE, "w") as f:
            json.dump(cookies, f)

    async def initialize(self):
        """初始化Playwright浏览器"""
        if self._browser is None:
            self._playwright = await async_playwright().start()

            if settings.BROWSER_MODE == "cdp":
                # CDP模式：连接到已运行的Chrome
                try:
                    print(f"尝试连接到已运行的Chrome ({settings.CDP_URL})...")
                    self._browser = await self._playwright.chromium.connect_over_cdp(settings.CDP_URL)
                    self._context = self._browser.contexts[0]

                    if self._context.pages:
                        self._page = self._context.pages[0]
                    else:
                        self._page = await self._context.new_page()

                    # 添加默认 cookie
                    await self._context.add_cookies([{
                        "name": "language",
                        "value": "cn_CN",
                        "domain": ".91porn.com",
                        "path": "/"
                    }])

                    print("成功连接到Chrome!")
                except Exception as e:
                    print(f"连接Chrome失败: {e}")
                    print(f"请先运行: google-chrome --remote-debugging-port=9222")
                    raise Exception(f"CDP连接失败: {e}")
            else:
                # Auto模式：自动启动浏览器
                print(f"启动浏览器 (headless={settings.HEADLESS})...")

                # 启动配置
                launch_options = {
                    "headless": settings.HEADLESS,
                    "args": [
                        "--disable-features=TranslateUI",
                        "--disable-background-networking",
                        "--disable-dev-shm-usage",
                        "--no-sandbox",
                    ],
                }

                # 如果配置了代理
                if settings.BROWSER_PROXY:
                    launch_options["proxy"] = {"server": settings.BROWSER_PROXY}
                    print(f"使用代理: {settings.BROWSER_PROXY}")

                self._browser = await self._playwright.chromium.launch(**launch_options)
                self._context = await self._browser.new_context(
                    viewport={"width": 1920, "height": 1080},
                    user_agent="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
                    locale="zh-CN",
                    timezone_id="Asia/Shanghai",
                )

                # 增强反检测脚本
                await self._context.add_init_script("""
                    // 隐藏 webdriver
                    Object.defineProperty(navigator, 'webdriver', { get: () => undefined });

                    // 模拟插件
                    Object.defineProperty(navigator, 'plugins', {
                        get: () => {
                            const plugins = [
                                { name: 'Chrome PDF Plugin', filename: 'internal-pdf-viewer' },
                                { name: 'Chrome PDF Viewer', filename: 'mhjfbmdgcfjbbpaeojofohoefgiehjai' },
                                { name: 'Native Client', filename: 'internal-nacl-plugin' }
                            ];
                            plugins.length = 3;
                            return plugins;
                        }
                    });

                    // 语言
                    Object.defineProperty(navigator, 'languages', { get: () => ['zh-CN', 'zh', 'en-US', 'en'] });

                    // Chrome 对象
                    window.chrome = {
                        runtime: {},
                        loadTimes: function() {},
                        csi: function() {},
                        app: {}
                    };

                    // 权限查询
                    const originalQuery = window.navigator.permissions.query;
                    window.navigator.permissions.query = (parameters) => (
                        parameters.name === 'notifications' ?
                            Promise.resolve({ state: Notification.permission }) :
                            originalQuery(parameters)
                    );

                    // WebGL 渲染器
                    const getParameter = WebGLRenderingContext.prototype.getParameter;
                    WebGLRenderingContext.prototype.getParameter = function(parameter) {
                        if (parameter === 37445) return 'Intel Inc.';
                        if (parameter === 37446) return 'Intel Iris OpenGL Engine';
                        return getParameter.apply(this, arguments);
                    };

                    // 隐藏自动化特征
                    delete navigator.__proto__.webdriver;
                """)

                # 添加默认 cookie
                await self._context.add_cookies([{
                    "name": "language",
                    "value": "cn_CN",
                    "domain": ".91porn.com",
                    "path": "/"
                }])

                self._page = await self._context.new_page()
                print("浏览器启动成功!")

    async def _inject_stealth(self, page: Page):
        """注入stealth脚本"""
        await page.add_init_script("""
            Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
            Object.defineProperty(navigator, 'plugins', { get: () => [1, 2, 3, 4, 5] });
            Object.defineProperty(navigator, 'languages', { get: () => ['zh-CN', 'zh', 'en'] });
            window.chrome = { runtime: {} };
            const originalQuery = window.navigator.permissions.query;
            window.navigator.permissions.query = (parameters) => (
                parameters.name === 'notifications' ?
                    Promise.resolve({ state: Notification.permission }) :
                    originalQuery(parameters)
            );
        """)

    async def set_cookies_from_string(self, cookie_string: str):
        """从字符串设置cookies (格式: name=value; name2=value2)"""
        cookies = []
        for item in cookie_string.split(";"):
            item = item.strip()
            if "=" in item:
                name, value = item.split("=", 1)
                cookies.append({
                    "name": name.strip(),
                    "value": value.strip(),
                    "domain": ".91porn.com",
                    "path": "/"
                })

        if cookies:
            self.save_cookies(cookies)
            if self._context:
                await self._context.add_cookies(cookies)
            print(f"已设置 {len(cookies)} 个cookies")
        return len(cookies)

    async def _save_current_cookies(self):
        """保存当前浏览器的cookies"""
        if self._context:
            try:
                cookies = await self._context.cookies()
                # 只保存91porn相关的cookies
                filtered = [c for c in cookies if "91porn" in c.get("domain", "")]
                if filtered:
                    self.save_cookies(filtered)
                    print(f"自动保存了 {len(filtered)} 个cookies")
            except Exception as e:
                print(f"保存cookies失败: {e}")

    async def close(self):
        """关闭浏览器"""
        if self._page:
            await self._page.close()
            self._page = None
        if self._context:
            await self._context.close()
            self._context = None
        if self._browser:
            await self._browser.close()
            self._browser = None
        if self._playwright:
            await self._playwright.stop()
            self._playwright = None

    async def _get_page(self) -> Page:
        """获取页面（复用同一个页面）"""
        async with self._lock:
            if self._page is None:
                await self.initialize()
            return self._page

    async def get_video_list(self, page_num: int = 1) -> VideoListResult:
        """获取视频列表"""
        page = await self._get_page()
        videos = []
        total_pages = 1

        try:
            # 构建URL并直接访问
            list_url = urljoin(settings.TARGET_BASE_URL, settings.VIDEO_LIST_PATH)
            list_url = f"{list_url}&page={page_num}"
            print(f"正在访问第{page_num}页: {list_url}")

            # 使用domcontentloaded而不是networkidle，避免超时
            await page.goto(list_url, wait_until="domcontentloaded", timeout=30000)

            # 等待页面加载，如果遇到Cloudflare验证，用户可以手动完成
            print("等待页面加载...如果看到验证页面请手动完成")
            await asyncio.sleep(5)

            # 检查是否遇到Cloudflare，等待用户验证
            for i in range(30):  # 最多等待30秒让用户完成验证
                title = await page.title()
                if "cloudflare" in title.lower() or "just a moment" in title.lower() or "blocked" in title.lower():
                    print(f"检测到验证页面，等待用户完成验证... ({i+1}/30)")
                    await asyncio.sleep(1)
                else:
                    break

            self._current_page_num = page_num

            # 保存当前cookies
            await self._save_current_cookies()

            # 再次检查是否遇到Cloudflare
            title = await page.title()
            print(f"页面标题: {title}")

            if "cloudflare" in title.lower() or "just a moment" in title.lower():
                print("警告: 遇到Cloudflare验证页面，请在设置中更新cookies")
                self._current_page_num = 0  # 重置页码
                return VideoListResult(videos=[], total_pages=1)

            # 获取总页数
            total_pages = await self._get_total_pages(page)
            print(f"总页数: {total_pages}")

            # 使用 JavaScript 直接提取视频列表数据
            # 先选择主列表的容器 .col-xs-12.col-sm-4.col-md-3.col-lg-3
            videos_data = await page.evaluate("""
                () => {
                    const videos = [];
                    const seen = new Set();

                    // 选择主列表的列容器
                    const columns = document.querySelectorAll('.col-xs-12.col-sm-4.col-md-3.col-lg-3');
                    console.log('找到列容器数:', columns.length);

                    for (const col of columns) {
                        // 在列容器内找视频卡片
                        const card = col.querySelector('.well.well-sm.videos-text-align');
                        if (!card) continue;

                        // 找链接
                        const link = card.querySelector('a[href*="viewkey"]');
                        if (!link) continue;

                        const href = link.href;
                        const match = href.match(/viewkey=([a-zA-Z0-9]+)/);
                        if (!match) continue;

                        const videoId = match[1];
                        if (seen.has(videoId)) continue;

                        // 在卡片内找图片
                        const img = card.querySelector('.thumb-overlay img, img.img-responsive');
                        let thumbnail = img ? img.src : null;

                        // 获取标题
                        const titleEl = card.querySelector('.video-title');
                        let title = titleEl ? titleEl.innerText?.trim() : (link.title || 'Video');

                        // 获取时长
                        const durationEl = card.querySelector('.duration');
                        const duration = durationEl ? durationEl.innerText?.trim() : null;

                        seen.add(videoId);
                        videos.push({
                            id: videoId,
                            title: title,
                            thumbnail: thumbnail,
                            url: href,
                            duration: duration
                        });
                    }

                    return videos;
                }
            """)

            print(f"JavaScript 提取到 {len(videos_data)} 个视频")

            for v in videos_data:
                videos.append(VideoItem(
                    id=v['id'],
                    title=v['title'] or f"Video",
                    thumbnail=v['thumbnail'],
                    url=v['url'],
                    duration=v['duration']
                ))
                # print(f"视频 {v['id']} 缩略图: {v['thumbnail']}")

        except Exception as e:
            print(f"获取视频列表失败: {e}")
            import traceback
            traceback.print_exc()

        return VideoListResult(videos=videos, total_pages=total_pages)

    async def _get_total_pages(self, page: Page) -> int:
        """从分页控件获取总页数"""
        total_pages = 1
        try:
            # 方法1: 查找分页链接中的最大页码
            pagination_links = await page.query_selector_all(".pagination a, .pagingnav a")
            max_page = 1
            for link in pagination_links:
                text = await link.inner_text()
                text = text.strip()
                if text.isdigit():
                    max_page = max(max_page, int(text))
            if max_page > 1:
                total_pages = max_page

            # 方法2: 查找"共X页"文本
            if total_pages == 1:
                content = await page.content()
                import re
                match = re.search(r'共\s*(\d+)\s*页', content)
                if match:
                    total_pages = int(match.group(1))

            # 方法3: 查找最后一页链接
            if total_pages == 1:
                last_link = await page.query_selector(".pagination li:last-child a, .pagingnav a:last-child")
                if last_link:
                    href = await last_link.get_attribute("href")
                    if href:
                        match = re.search(r'page=(\d+)', href)
                        if match:
                            total_pages = int(match.group(1))

        except Exception as e:
            print(f"获取总页数失败: {e}")

        return total_pages

    async def _parse_video_item(self, element, index: int) -> Optional[VideoItem]:
        """解析单个视频项"""
        # 获取链接元素 - 可能是元素本身或子元素
        tag_name = await element.evaluate("el => el.tagName.toLowerCase()")

        if tag_name == "a":
            link_el = element
            # 如果是 <a> 标签，尝试获取父容器来找图片
            parent_el = await element.evaluate_handle("el => el.closest('.well, .col-xs-12, .video-item') || el.parentElement")
        else:
            link_el = await element.query_selector("a")
            parent_el = element

        if not link_el:
            return None

        href = await link_el.get_attribute("href")
        if not href or "viewkey" not in href:
            return None

        url = urljoin(settings.TARGET_BASE_URL, href)

        # 从URL中提取视频ID (viewkey参数)
        parsed = urlparse(url)
        query_params = parse_qs(parsed.query)
        video_id = query_params.get("viewkey", [str(index)])[0]

        # 获取缩略图 - 优先从父容器查找
        thumbnail = None
        img_el = await parent_el.query_selector("img") if parent_el else None
        if not img_el:
            img_el = await element.query_selector("img")
        if not img_el:
            img_el = await link_el.query_selector("img")

        if img_el:
            thumbnail = await img_el.get_attribute("src")
            if not thumbnail or "loading" in thumbnail.lower() or "placeholder" in thumbnail.lower():
                thumbnail = await img_el.get_attribute("data-src")
            if not thumbnail:
                thumbnail = await img_el.get_attribute("data-original")
            # 过滤掉明显的占位图
            if thumbnail and ("data:image" in thumbnail or "blank" in thumbnail.lower() or "spacer" in thumbnail.lower()):
                thumbnail = await img_el.get_attribute("data-src") or await img_el.get_attribute("data-original")
            if thumbnail and not thumbnail.startswith("http"):
                thumbnail = urljoin(settings.TARGET_BASE_URL, thumbnail)
            print(f"视频 {video_id} 缩略图: {thumbnail}")

        # 获取标题
        title = f"Video {index + 1}"
        # 尝试多种标题选择器
        title_el = await element.query_selector(".video-title")
        if not title_el:
            title_el = await element.query_selector("span.video-title")
        if not title_el:
            # 尝试从链接的title属性获取
            title = await link_el.get_attribute("title")
        if title_el:
            title = await title_el.inner_text()

        if not title:
            title = f"Video {index + 1}"

        # 获取时长
        duration = None
        duration_el = await element.query_selector(".duration")
        if duration_el:
            duration = await duration_el.inner_text()

        return VideoItem(
            id=video_id,
            title=title.strip(),
            thumbnail=thumbnail,
            url=url,
            duration=duration
        )

    async def get_video_detail(self, video_url: str) -> Optional[VideoDetail]:
        """获取视频详情和m3u8链接"""
        page = await self._get_page()
        detail = None

        self._pending_requests += 1  # 增加请求计数

        try:
            # 设置请求拦截来捕获m3u8请求
            m3u8_urls = []

            async def handle_request(request):
                url = request.url
                if ".m3u8" in url or "m3u8" in url:
                    m3u8_urls.append(url)

            page.on("request", handle_request)

            print(f"正在访问视频页: {video_url}")

            # goto 可能因为页面重定向等原因失败，但页面内容可能已加载
            try:
                await page.goto(video_url, wait_until="domcontentloaded", timeout=30000)
            except Exception as goto_error:
                print(f"页面导航异常 (可能正常): {goto_error}")
                # 等待页面稳定
                await asyncio.sleep(2)

            # 等待视频加载
            await asyncio.sleep(3)

            # 尝试点击播放按钮（如果有的话）
            try:
                play_btn = await page.query_selector(".vjs-big-play-button, .play-button, #player")
                if play_btn:
                    await play_btn.click()
                    await asyncio.sleep(2)
            except:
                pass

            # 尝试多种方式获取视频链接
            video_src = None

            # 方法1: 从 .video-container 下的 source 标签获取 (优先)
            source_el = await page.query_selector(".video-container source")
            if source_el:
                video_src = await source_el.get_attribute("src")
                print(f"从 .video-container source 找到: {video_src}")

            # 方法2: 从 .video-container 下的 video 标签获取
            if not video_src:
                video_el = await page.query_selector(".video-container video")
                if video_el:
                    video_src = await video_el.get_attribute("src")
                    print(f"从 .video-container video 找到: {video_src}")

            # 方法3: 从拦截的请求中获取 m3u8
            if not video_src and m3u8_urls:
                for url in m3u8_urls:
                    if "index.m3u8" in url or "/hls/" in url:
                        video_src = url
                        break
                if not video_src:
                    video_src = m3u8_urls[0]
                print(f"从请求拦截找到: {video_src}")

            # 方法4: 从页面内容中提取 mp4 或 m3u8 链接
            if not video_src:
                content = await page.content()
                # 先尝试 mp4
                mp4_pattern = r'https?://[^\s"\'<>]+\.mp4[^\s"\'<>]*'
                matches = re.findall(mp4_pattern, content)
                if matches:
                    video_src = matches[0]
                    print(f"从页面内容找到mp4: {video_src}")
                else:
                    # 再尝试 m3u8
                    m3u8_pattern = r'https?://[^\s"\'<>]+\.m3u8[^\s"\'<>]*'
                    matches = re.findall(m3u8_pattern, content)
                    if matches:
                        video_src = matches[0]
                        print(f"从页面内容找到m3u8: {video_src}")

            # 方法5: 从任意 video source 标签获取
            if not video_src:
                source_el = await page.query_selector("video source")
                if source_el:
                    video_src = await source_el.get_attribute("src")
                    print(f"从 video source 找到: {video_src}")

            # 方法6: 从任意 video 标签的 src 获取
            if not video_src:
                video_el = await page.query_selector("video")
                if video_el:
                    video_src = await video_el.get_attribute("src")
                    print(f"从 video src 找到: {video_src}")

            print(f"最终视频链接: {video_src}")

            # 修复链接格式问题 (如 .com//mp43/ -> .com/mp43/)
            if video_src:
                video_src = re.sub(r'\.com//+', '.com/', video_src)
                print(f"修复后链接: {video_src}")

            # 获取标题
            title = await page.title()
            title_el = await page.query_selector("h4, .video-title, #viewvideo-title")
            if title_el:
                title = await title_el.inner_text()

            # 获取缩略图
            thumbnail = None
            video_el = await page.query_selector("video")
            if video_el:
                thumbnail = await video_el.get_attribute("poster")

            # 提取视频ID
            parsed = urlparse(video_url)
            query_params = parse_qs(parsed.query)
            video_id = query_params.get("viewkey", ["unknown"])[0]

            detail = VideoDetail(
                id=video_id,
                title=title.strip() if title else "未知标题",
                thumbnail=thumbnail,
                m3u8_url=video_src,
                original_url=video_url
            )

            # 获取完视频链接后延迟返回列表页，避免视频继续播放浪费资源
            # 延迟一段时间确保前端已开始加载视频
            scraper = self  # 保存引用

            async def delayed_go_back():
                await asyncio.sleep(10)  # 等待10秒让前端有时间加载
                # 检查是否还有其他请求正在进行
                if scraper._pending_requests > 0:
                    print(f"有 {scraper._pending_requests} 个请求正在进行，暂不返回列表页")
                    return
                try:
                    print("返回列表页...")
                    await page.go_back(wait_until="domcontentloaded", timeout=10000)
                except Exception as back_error:
                    print(f"返回列表页失败: {back_error}")
                    try:
                        list_url = urljoin(settings.TARGET_BASE_URL, settings.VIDEO_LIST_PATH)
                        await page.goto(list_url, wait_until="domcontentloaded", timeout=10000)
                    except:
                        pass

            # 异步执行，不阻塞返回
            asyncio.create_task(delayed_go_back())

        except Exception as e:
            print(f"获取视频详情失败: {e}")
            import traceback
            traceback.print_exc()
        finally:
            self._pending_requests -= 1  # 减少请求计数

        return detail


# 全局单例
scraper_service = ScraperService()
