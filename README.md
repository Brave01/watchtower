# Watchtower (瞭望塔)

> 多服务器集中管理与监控平台

## 功能

| 模块 | 说明 |
|------|------|
| **服务大盘** | 主机状态总览（在线/离线/维护/角色异常），支持 Excel 导出 |
| **主机管理** | 增删改查，支持磁盘分区、CPU/内存信息、维护模式、批量导入 |
| **角色管理** | 自定义探测角色（ICMP / TCP / HTTP / SSH），灵活分配至主机 |
| **SSH 终端** | 浏览器内嵌 xterm.js 终端，支持凭据保存与快捷连接 |
| **架构图** | 基于 Vue Flow 的拓扑可视化，支持连线/标注/分组/导出 PDF |
| **日志监控** | 连接 Elasticsearch 实时拉取日志，WebSocket 推送至浏览器 |
| **告警规则** | 关键词/正则匹配，冷却去重，多 Webhook 推送 |
| **Webhook** | 飞书/钉钉/企业微信/自定义，支持限流、@提及、自定义模板 |
| **SQLite** | 内嵌数据库，无需外部依赖 |
| **修改密码** | 登录后可在 UI 中修改密码，持久化到数据库 |

## 快速开始

### 开发模式（前后端分离）

```bash
# 终端 1：启动 Go 后端（端口 3972）
cd watchtower-backend
go build -o watchtower .
./watchtower

# 终端 2：启动前端开发服务器（端口 5173，支持热更新）
cd watchtower-frontend
npm install
npx vite --host
```

打开浏览器访问 `http://localhost:5173`，默认账号 `admin` / `admin`。

### 二进制启动（生产模式）

```bash
# 构建前端
cd watchtower-frontend
npm install && npm run build

# 编译后端
cd ../watchtower-backend
go build -o watchtower .

# 运行（后端 Serve 前端构建产物 + API）
cd ../deploy
docker compose up -d
```

### Docker 启动

```bash
cd deploy
docker compose up -d
# 访问 http://localhost:3972
```

### 访问

打开浏览器访问 `http://localhost:3972`，默认账号 `admin` / `admin`。

> **注意**：首次使用默认密码登录后，请立即在侧边栏底部点击锁图标修改密码。

## 配置

| 文件 | 说明 |
|------|------|
| `watchtower-backend/configs/config.yaml` | 服务端口、ES 地址、探针间隔等 |
| `watchtower-backend/configs/.env` | 敏感信息：ES\_PASSWORD、AUTH\_JWT\_SECRET 等 |
| `deploy/Dockerfile` | Docker 构建文件 |
| `deploy/docker-compose.yml` | Docker Compose 部署配置 |

## 技术栈

- **后端**：Go 1.25+，标准库 `net/http`
- **前端**：Vue 3 + Vite
- **数据库**：SQLite（modernc.org/sqlite）
- **WebSocket**：gorilla/websocket
- **部署**：Docker / Docker Compose / 二进制

## 项目结构

```
.
├── watchtower-backend/          # Go 后端代码
│   ├── main.go                  # 入口
│   ├── configs/                 # 配置文件
│   ├── internal/
│   │   ├── auth/                # JWT 认证中间件
│   │   ├── dashboard/           # 拨测调度器 & 探针
│   │   ├── handler/             # HTTP 路由 & WS
│   │   ├── logmonitor/          # ES 日志拉取、去重、过滤、Webhook
│   │   ├── store/               # SQLite 数据层
│   │   └── webui/               # 嵌入式前端静态资源
│   └── data/                    # SQLite 数据库文件
│
├── watchtower-frontend/         # Vue 3 前端源码
│   ├── src/
│   │   ├── views/               # 页面组件
│   │   ├── components/          # 通用组件
│   │   ├── App.vue              # 根组件（含登录/登出/改密）
│   │   ├── api.js               # API 封装
│   │   └── style.css            # 全局样式
│   ├── public/                  # 静态资源
│   └── package.json
│
├── deploy/                      # 构建部署文件
│   ├── Dockerfile               # 多阶段构建
│   ├── docker-compose.yml       # 前后端容器编排
│   └── nginx.conf               # Nginx 反向代理配置
│
└── docs/                        # 详细文档
    ├── ai-context.md
    ├── architecture-api.md
    └── user-guide.md
```
