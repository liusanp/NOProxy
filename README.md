# NOProxy

视频代理服务，支持 Go 和 Python 后端实现。

## 功能

- 视频列表浏览
- 视频流代理
- M3U8/MP4 支持
- 自动 Cookie 管理
- 本地缓存（视频、封面图、列表信息）
- **SQLite 缓存索引**（高效查询已缓存视频）
- 离线浏览支持（获取失败时使用缓存）
- 已缓存视频管理页面
- **浏览器连接断开自动重连**
- **增强反检测脚本**

## 快速开始

### Docker 部署（推荐）

#### Go 版本（推荐）

```bash
docker-compose -f docker-compose-go.yml up -d
```

#### Python 版本

```bash
docker-compose up -d
```

访问 http://localhost:8000

服务说明：
- `chrome`: browserless/chrome 浏览器服务，端口 9222
- `backend`: 后端服务，端口 8000

缓存目录已默认映射到 `./cache`。

#### 使用代理（可选）

```bash
BROWSER_PROXY=http://proxy:port docker-compose -f docker-compose-go.yml up -d
```

### 本地开发

**Go 后端（推荐）**

```bash
cd backend-go
go mod download
go run .
```

**Python 后端**

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
| `CDP_URL` | CDP 连接地址 | http://chrome:3000 (Docker) |
| `BROWSER_PROXY` | 浏览器代理 | - |

### 缓存配置

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `VIDEO_CACHE_ENABLED` | 启用本地缓存 | true |
| `VIDEO_CACHE_DIR` | 缓存目录 | cache/videos |
| `CACHE_DB_PATH` | 缓存数据库路径 | {VIDEO_CACHE_DIR}/cache.db |
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
- **缓存索引**：使用 SQLite 数据库存储缓存元数据，支持高效分页查询

### SQLite 缓存索引

Go 版本使用 SQLite 数据库（`cache/videos/cache.db`）存储缓存元数据，相比文件系统遍历有以下优势：

| 操作 | 文件系统遍历 | SQLite 查询 |
|------|-------------|-------------|
| 列表查询 | O(n×m) 文件操作 | O(log n) 索引查询 |
| 分页 | 全量加载后切片 | SQL LIMIT/OFFSET |
| 获取总大小 | 遍历所有文件 | SQL SUM() |
| 搜索 | 不支持 | SQL WHERE/LIKE |

首次启动时会自动从现有缓存文件同步数据到数据库。

### 自动预缓存

启用 `AUTO_PRECACHE=true` 后，获取视频列表时会自动在后台预缓存列表中的视频：

- 使用新标签页获取视频详情，不干扰主页面浏览
- 自动跳过已缓存或正在下载的视频
- 通过 `PRECACHE_CONCURRENT` 控制并发数，避免过载

### 浏览器连接管理

Go 版本支持浏览器连接断开自动重连：

- 检测到 CDP 连接断开时自动重新初始化
- 无需手动重启服务
- 适用于列表获取和视频详情获取

### 反检测功能

内置增强反检测脚本，覆盖以下检测点：

- WebDriver 标志隐藏
- 插件和 MimeType 模拟
- Chrome 对象完整模拟
- WebGL 渲染器信息伪装
- Selenium/WebDriver 相关属性清理
- Headless 特征隐藏
- iframe 内 navigator 处理
- Function.toString 检测防护
- 更多...

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
├── backend-go/           # Go 后端（推荐）
│   ├── services/         # 核心服务
│   │   ├── scraper.go    # 页面解析服务
│   │   ├── video_cache.go # 视频缓存服务
│   │   ├── cache_db.go   # SQLite 缓存索引
│   │   └── proxy.go      # 代理服务
│   ├── routers/          # API 路由
│   ├── models/           # 数据模型
│   ├── config/           # 配置管理
│   └── main.go           # 入口文件
├── backend/              # Python 后端（已弃用）
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
│   ├── cache.db          # SQLite 缓存索引
│   ├── list_page_1.json  # 列表缓存
│   ├── {viewkey}.jpg     # 封面图缓存
│   ├── {viewkey}.mp4     # MP4 视频缓存
│   ├── {viewkey}/        # M3U8 视频缓存目录
│   │   ├── video.m3u8
│   │   ├── 0.ts, 1.ts...
│   │   └── detail.json
│   └── ...
├── Dockerfile
├── docker-compose.yml        # Python 版本
└── docker-compose-go.yml     # Go 版本（推荐）
```

## 版本对比

| 特性 | Go 版本 | Python 版本 |
|------|---------|-------------|
| 状态 | **推荐** | 已弃用 |
| 性能 | 更高 | 一般 |
| 内存占用 | 更低 | 较高 |
| 缓存查询 | SQLite 索引 | 文件系统遍历 |
| 浏览器重连 | 自动 | 不支持 |
| 反检测脚本 | 增强版 | 基础版 |

## License

MIT
