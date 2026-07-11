## 1. 架构设计

\`\`\`mermaid
flowchart TD
  "Browser (SPA)" --> "Go Server (HTTP+WS)"
  "Go Server" --> "SQLite / MySQL"
  "Go Server" --> "Elasticsearch"
  "Go Server" --> "飞书 Webhook"
\`\`\`

## 2. 技术选型
- **前端**：Vue 3 (CDN) + Tailwind CSS (CDN) + xterm.js (CDN)
- **构建方式**：单文件 HTML，通过 Go `//go:embed` 编译进二进制
- **后端**：Go 标准库 + gorilla/websocket
- **存储**：SQLite (modernc.org/sqlite) / MySQL (go-sql-driver)
- **认证**：JWT (golang-jwt/jwt/v5) + bcrypt

## 3. 路由定义
| 路由 | 用途 |
|------|------|
| `/` | SPA 根页面（需认证） |
| `/login` | 登录页面 |
| `/common/*` | 静态资源（CSS/JS） |
| `/ws` | WebSocket 实时日志流 |
| `/api/auth/*` | 认证 API |
| `/api/health` | 健康检查 |
| `/api/stats` | 日志监控统计 |
| `/api/rules` | 告警规则 CRUD |
| `/api/webhook/*` | 飞书 Webhook 配置/测试/日志 |
| `/api/dashboard` | 大盘数据 |
| `/api/hosts` | 主机 CRUD |
| `/api/roles` | 角色 CRUD |
| `/api/assign` | 角色指派 |
| `/api/ssh-credential` | SSH 凭据 CRUD |
| `/api/ssh/ws` | SSH WebSocket 终端 |
| `/api/refresh` | 触发拨测刷新 |

## 4. 前端设计

### 4.1 组件架构
单一 HTML 文件，用 Vue 3 CDN 构建：
- `App` 根组件：导航 + Tab 切换
- `DashboardPage`：主机列表、角色筛选、统计概览
- `LogMonitorPage`：实时日志流、规则管理、飞书配置、统计面板
- `SSHTerminal`：xterm.js WebSocket 终端（弹窗/独立面板）

### 4.2 API 调用
通过 `fetch()` 调用后端 REST API，WebSocket 使用原生 WebSocket API。
认证通过 cookie 传递 JWT token。
