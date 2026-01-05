from contextlib import asynccontextmanager
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.staticfiles import StaticFiles
from fastapi.responses import FileResponse
from pathlib import Path

from .config import settings
from .routers import videos, stream
from .services.scraper import scraper_service
from .services.proxy import proxy_service


# 前端构建目录
frontend_dist = Path(__file__).parent.parent / "frontend" / "dist"


@asynccontextmanager
async def lifespan(app: FastAPI):
    """应用生命周期管理"""
    # 启动时初始化
    print("正在初始化Playwright...")
    await scraper_service.initialize()
    print("Playwright初始化完成")

    yield

    # 关闭时清理
    print("正在关闭服务...")
    await scraper_service.close()
    await proxy_service.close()
    print("服务已关闭")


# 创建FastAPI应用
app = FastAPI(
    title="NOProxy - 视频代理服务",
    description="使用Playwright解析视频网站，提供视频列表和m3u8代理播放",
    version="1.0.0",
    lifespan=lifespan
)

# 配置CORS
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# 注册 API 路由
app.include_router(videos.router)
app.include_router(stream.router)


@app.get("/health")
async def health_check():
    """健康检查"""
    return {"status": "healthy"}


# 提供前端静态文件服务
if frontend_dist.exists():
    # 挂载静态资源目录 (js, css, images 等)
    app.mount("/assets", StaticFiles(directory=str(frontend_dist / "assets")), name="assets")

    # 根路径返回 index.html
    @app.get("/")
    async def serve_index():
        return FileResponse(str(frontend_dist / "index.html"))

    # 其他非 API 路由返回 index.html (支持 Vue Router 的 history 模式)
    @app.get("/{full_path:path}")
    async def serve_spa(full_path: str):
        # 返回 index.html
        return FileResponse(str(frontend_dist / "index.html"))


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(
        "backend.main:app",
        host=settings.HOST,
        port=settings.PORT,
        reload=settings.DEBUG
    )
