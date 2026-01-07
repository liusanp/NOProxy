from pydantic_settings import BaseSettings
from typing import Dict, Optional


class Settings(BaseSettings):
    # 服务器配置
    HOST: str = "0.0.0.0"
    PORT: int = 8000
    DEBUG: bool = True

    # 访问密码
    ACCESS_PASSWORD: str = "changeme"
    ADMIN_PASSWORD: str = "admin123"

    # 目标网站配置
    TARGET_BASE_URL: str = "https://91porn.com"
    VIDEO_LIST_PATH: str = "/v.php?category=rf&viewtype=basic"

    # Playwright配置
    HEADLESS: bool = True  # 设为True使用无头模式（适合Docker）
    BROWSER_TYPE: str = "chromium"

    # 浏览器启动模式: "auto" 自动启动, "cdp" 连接已运行的Chrome
    BROWSER_MODE: str = "cdp"
    CDP_URL: str = "http://127.0.0.1:9222"  # CDP模式的连接地址

    # 网络代理配置 (可选，格式: http://host:port 或 socks5://host:port)
    BROWSER_PROXY: Optional[str] = None  # 例如: "http://127.0.0.1:7890" 或 "socks5://127.0.0.1:1080"

    # 代理服务配置
    PROXY_BASE_URL: str = "http://localhost:8000"

    # 选择器配置 (适配91porn)
    SELECTORS: Dict[str, str] = {
        "video_item": ".listchannel .well",  # 视频列表项
        "video_title": ".video-title",        # 标题
        "video_thumbnail": "img.img-responsive",  # 缩略图
        "video_link": "a",                    # 链接
        "video_duration": ".duration",        # 时长
        "m3u8_source": "video source, video", # m3u8源
    }

    # 缓存配置
    CACHE_ENABLED: bool = True
    CACHE_TTL: int = 300  # 秒

    # 视频文件缓存配置
    VIDEO_CACHE_ENABLED: bool = True  # 是否启用视频本地缓存
    VIDEO_CACHE_DIR: str = "cache/videos"  # 视频缓存目录
    AUTO_PRECACHE: bool = True  # 是否自动预缓存列表中的视频
    PRECACHE_CONCURRENT: int = 2  # 预缓存并发数

    class Config:
        env_file = ".env"
        env_file_encoding = "utf-8"


settings = Settings()
