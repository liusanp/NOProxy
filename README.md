# NOProxy

视频代理服务，基于 FastAPI + Playwright。

## 功能

- 视频列表浏览
- 视频流代理
- M3U8/MP4 支持
- 自动 Cookie 管理
- 本地缓存（视频、封面图、列表信息）
- 离线浏览支持（获取失败时使用缓存）
- 已缓存视频管理页面

## 快速开始

### Docker 部署（推荐）

```bash
docker-compose up -d
```

访问 http://localhost:8000

#### 映射缓存目录（可选）

如需持久化视频缓存，可在 `docker-compose.yml` 中添加 volumes 映射：

```yaml
services:
  noproxy:
    build: .
    ports:
      - "8000:8000"
    volumes:
      - ./cache:/app/cache  # 映射视频缓存目录
    environment:
      - VIDEO_CACHE_ENABLED=true
      # ... 其他配置
```

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

### 基础配置

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `HOST` | 服务监听地址 | 0.0.0.0 |
| `PORT` | 服务端口 | 8000 |
| `ACCESS_PASSWORD` | 访问密码 | changeme |
| `ADMIN_PASSWORD` | 管理员密码 | admin123 |
| `TARGET_BASE_URL` | 目标网站地址 | - |
| `VIDEO_LIST_PATH` | 视频列表路径 | /videos |

### 密码说明

- **访问密码**：普通用户登录，可浏览视频和查看缓存列表
- **管理员密码**：管理员登录，额外拥有删除单个缓存和清空全部缓存的权限

### 浏览器配置

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `BROWSER_MODE` | 浏览器模式 (auto/cdp) | cdp |
| `CDP_URL` | CDP 连接地址 | http://127.0.0.1:9222 |
| `HEADLESS` | 无头模式 | true |
| `BROWSER_PROXY` | 浏览器代理 | - |

### 缓存配置

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `VIDEO_CACHE_ENABLED` | 启用本地缓存 | true |
| `VIDEO_CACHE_DIR` | 缓存目录 | cache/videos |
| `VIDEO_LIST_CACHE_TTL` | 视频列表缓存有效期（秒） | 43200 (12小时) |
| `CACHE_PAGE_SIZE` | 已缓存视频列表每页数量 | 20 |
| `AUTO_PRECACHE` | 自动预缓存列表视频 | true |
| `PRECACHE_CONCURRENT` | 预缓存并发数 | 2 |

### 缓存说明

启用缓存后，以下内容会自动保存到本地：

- **视频列表**：每页列表信息持久化保存
  - 默认12小时内优先使用缓存，不请求网站（可通过 `VIDEO_LIST_CACHE_TTL` 配置）
  - 超过有效期会重新获取
  - 获取失败时使用过期缓存兜底
- **封面图**：获取列表时后台自动下载，文件名为 `{viewkey}.jpg`
- **视频文件**：首次播放时后台自动下载（M3U8 或 MP4）
- **视频详情**：视频缓存成功后，详情也会持久化保存

### 自动预缓存

启用 `AUTO_PRECACHE=true` 后，获取视频列表时会自动在后台预缓存列表中的视频：

- 使用新标签页获取视频详情，不干扰主页面浏览
- 自动跳过已缓存或正在下载的视频
- 通过 `PRECACHE_CONCURRENT` 控制并发数，避免过载

### 缓存管理 API

| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/cache` | GET | 列出所有缓存视频和总大小 |
| `/api/cache/{viewkey}` | GET | 查看指定视频缓存状态 |
| `/api/cache/{viewkey}` | DELETE | 删除指定视频缓存（需管理员权限） |
| `/api/cache` | DELETE | 清空所有缓存（需管理员权限） |

### 图片代理 API

| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/stream/image/{viewkey}` | GET | 获取封面图（优先本地缓存） |
| `/api/stream/image/{viewkey}?url=xxx` | GET | 获取封面图并缓存 |

## 前端页面

| 路径 | 说明 |
|------|------|
| `/` | 视频列表页，浏览和搜索视频 |
| `/play/:id` | 视频播放页 |
| `/cached` | 已缓存视频页，查看和管理本地缓存的视频 |

### 已缓存页面功能

- 显示所有已缓存的视频列表
- 显示缓存总大小和视频数量
- 每个视频显示类型（MP4/M3U8）和文件大小
- 支持单个删除或清空全部缓存
- 点击视频可直接播放（使用本地缓存）

## 项目结构

```
├── backend/              # FastAPI 后端
│   ├── services/         # 核心服务
│   ├── routers/          # API 路由
│   └── models/           # 数据模型
├── frontend/             # Vue 前端
│   └── src/
│       ├── views/        # 页面组件
│       │   ├── VideoList.vue
│       │   ├── VideoPlayer.vue
│       │   └── CachedVideos.vue
│       ├── components/   # 公共组件
│       └── api/          # API 接口
├── cache/videos/         # 缓存目录
│   ├── list_page_1.json  # 列表缓存
│   ├── {viewkey}.jpg     # 封面图缓存
│   ├── {viewkey}.mp4     # MP4 视频缓存
│   ├── {viewkey}/        # M3U8 视频缓存目录
│   │   ├── video.m3u8
│   │   ├── 0.ts, 1.ts...
│   │   └── detail.json
│   └── ...
└── Dockerfile
```

## License

MIT
