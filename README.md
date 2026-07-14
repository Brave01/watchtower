# Watchtower (瞭望塔)

> 多服务器集中管理与监控平台

## 功能

| 模块          | 说明                                       |
| ----------- | ---------------------------------------- |
| **服务大盘**    | 主机状态总览（在线/离线/维护/角色异常），支持 Excel 导出        |
| **主机管理**    | 增删改查，支持磁盘分区、CPU/内存信息、维护模式、批量导入           |
| **角色管理**    | 自定义探测角色（ICMP / TCP / HTTP / SSH），灵活分配至主机 |
| **SSH 终端**  | 浏览器内嵌 xterm.js 终端，支持凭据保存与快捷连接            |
| **架构图**     | 基于 Vue Flow 的拓扑可视化，支持连线/标注/分组/导出 PDF     |
| **日志监控**    | 连接 Elasticsearch 实时拉取日志，WebSocket 推送至浏览器 |
| **告警规则**    | 关键词/正则匹配，冷却去重，多 Webhook 推送               |
| **Webhook** | 飞书/钉钉/企业微信/自定义，支持限流、@提及、自定义模板            |
| **SQLite**  | 内嵌数据库，无需外部依赖                             |

## 快速开始

### 二进制启动

```bash
# 克隆
git clone https://github.com/Brave01/watchtower.git
cd watchtower

# 配置
cp configs/config.yaml configs/config.yaml  # 按需修改
# 编辑 configs/.env 设置 ES_PASSWORD、AUTH_JWT_SECRET 等

# 运行
go build -o watchtower .
./watchtower
```

### Docker 启动

```bash
docker compose -f deploy/docker-compose.yml up -d
```

### 访问

打开浏览器访问 `http://localhost:3972`，默认账号 `admin` / `admin`。

## 配置

| 文件                    | 说明                                    |
| --------------------- | ------------------------------------- |
| `configs/config.yaml` | 服务端口、ES 地址、探针间隔等                      |
| `configs/.env`        | 敏感信息：ES\_PASSWORD、AUTH\_JWT\_SECRET 等 |

## 构建前端

```bash
cd frontend
npm install
npm run build   # 产出到 internal/webui/static/dist/
```

## 技术栈

- **后端**：Go 1.25+，标准库 `net/http`
- **前端**：Vue 3 + Vite
- **数据库**：SQLite（modernc.org/sqlite）
- **WebSocket**：gorilla/websocket
- **部署**：Docker / Docker Compose / 二进制 / Kubernetes

## 项目结构

```
.
├── main.go                    # 入口
├── configs/                   # 配置文件
├── internal/
│   ├── auth/                  # JWT 认证中间件
│   ├── dashboard/             # 拨测调度器 & 探针
│   ├── handler/               # HTTP 路由 & WS
│   ├── logmonitor/            # ES 日志拉取、去重、过滤、Webhook
│   ├── store/                 # SQLite 数据层
│   └── webui/                 # 嵌入式前端静态资源
├── frontend/                  # Vue 3 前端源码
├── deploy/                    # Docker/K8s 部署文件
└── docs/                      # 详细文档
```

