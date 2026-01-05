# NOProxy

视频代理服务，基于 FastAPI + Playwright。

## 功能

- 视频列表浏览
- 视频流代理
- M3U8/MP4 支持
- 自动 Cookie 管理

## 快速开始

### Docker 部署（推荐）

```bash
docker-compose up -d
```

访问 http://localhost:8000

### 本地开发

**后端**

```bash
cd backend
pip install -r requirements.txt
playwright install chromium
uvicorn main:app --reload
```

**前端**

```bash
cd frontend
npm install
npm run dev
```

## 配置

复制 `.env.example` 为 `.env` 并修改：

```bash
cp .env.example .env
```

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `HOST` | 服务监听地址 | 0.0.0.0 |
| `PORT` | 服务端口 | 8000 |
| `ACCESS_PASSWORD` | 访问密码 | changeme |
| `TARGET_BASE_URL` | 目标网站地址 | - |
| `VIDEO_LIST_PATH` | 视频列表路径 | /videos |
| `BROWSER_MODE` | 浏览器模式 (auto/cdp) | cdp |
| `CDP_URL` | CDP 连接地址 | http://127.0.0.1:9222 |
| `HEADLESS` | 无头模式 | true |
| `BROWSER_PROXY` | 浏览器代理 | - |

## 项目结构

```
├── backend/          # FastAPI 后端
│   ├── services/     # 核心服务
│   ├── routers/      # API 路由
│   └── models/       # 数据模型
├── frontend/         # 前端
├── docker/           # Docker 配置
└── Dockerfile
```

## License

MIT
