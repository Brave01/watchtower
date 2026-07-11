# Server Controller Manager (SCM) — 架构与 API 文档

## 1. 项目概述

SCM（Server Controller Manager）是一个服务大盘管理平台，提供以下核心功能：

- **主机存活探测**：通过 ICMP ping 定期检测主机的在线状态
- **服务拨测**：支持 TCP、HTTP、SSH 等多种协议的服务级别健康检查
- **SSH 终端**：基于 WebSocket + xterm.js 的浏览器端远程终端
- **架构图**：基于 Vue Flow 的可视化拓扑编辑器，支持连线、分组、导出 PDF
- **日志监控**：对接 Elasticsearch 采集日志，经解析、过滤、去重后通过 Webhook 推送告警

### 技术栈

| 层级 | 技术 | 说明 |
|------|------|------|
| 后端 | Go（标准库 `net/http`） | 纯标准库路由，无第三方 Web 框架 |
| 前端 | Vue 3 + Vue Flow + Vite | SPA，Vite 构建输出到 `internal/webui/static/dist/` |
| 存储 | SQLite（`modernc.org/sqlite`） | 嵌入式数据库，单文件 |
| 认证 | JWT HS256 | 基于 Cookie 的会话管理，24h 过期 |
| WebSocket | gorilla/websocket | SSH 终端（BinaryMessage）与实时日志推送 |
| 日志采集 | Elasticsearch 客户端 | 定时轮询，支持 K8s 日志格式 |
| 告警推送 | 飞书/钉钉/企业微信 Webhook | 支持消息模板、@提及、限流、多客户端 |

---

## 2. 项目结构

```
/Users/tangran/watchtower/
├── main.go                          # 程序入口：配置加载、组件初始化、HTTP 路由注册
├── logmonitor_init.go               # 日志监控组件初始化（旧版入口）
├── go.mod / go.sum                  # Go 模块依赖
│
├── configs/
│   ├── config.yaml                  # 非敏感配置（端口 3972、ES 地址索引、探针间隔 15s）
│   ├── .env                         # 敏感配置（ES 密码、JWT 密钥、管理员密码 hash）
│   ├── config.example.yaml          # 配置示例
│   └── .env.example                 # 环境变量示例
│
├── data/
│   └── server.db                    # SQLite 数据库文件（自动创建）
│
├── internal/
│   ├── store/                       # 数据存储层
│   │   ├── interface.go             # Store 接口定义（含 DeleteAssignment）
│   │   ├── models.go                # 数据模型定义（Host、Role、Assignment、ESConfig 等）
│   │   └── sqlite.go                # SQLite 实现（建表、CRUD、种子数据）
│   │
│   ├── auth/                        # JWT 认证子系统
│   │   ├── jwt.go                   # HS256 JWT 签发与校验
│   │   ├── middleware.go            # HTTP 中间件（Cookie 校验、豁免路径）
│   │   └── handlers.go              # 登录/登出/Me 处理器
│   │
│   ├── dashboard/                   # 拨测模块
│   │   ├── prober.go                # 四种探测实现 + ResolveProbeParams
│   │   ├── scheduler.go             # 定时调度器（15s 间隔，MaxRetries=2）
│   │   └── ssh_ws.go                # SSH WebSocket 终端（BinaryMessage）
│   │
│   ├── handler/                     # HTTP 路由注册与处理器
│   │   ├── auth.go                  # 认证路由（/api/auth/*、/login、/）
│   │   ├── dashboard.go             # 大盘 API 含主机/角色/分配 CRUD + DELETE
│   │   ├── logmonitor.go            # 日志监控 API（含 ES 配置、规则、Webhook）
│   │   └── es_pipeline.go           # ESPipeline：ES 客户端动态控制 + WS 广播日志
│   │
│   ├── logmonitor/                  # 日志监控子系统
│   │   ├── parser/parser.go         # ES JSON 日志解析器
│   │   ├── filter/filter.go         # 告警规则过滤引擎
│   │   ├── dedup/dedup.go           # MD5 + TTL 去重
│   │   ├── ws/hub.go                # WebSocket 广播 Hub
│   │   ├── es/client.go             # Elasticsearch 客户端（支持 size 配置）
│   │   └── webhook/feishu.go        # Webhook 客户端（限流、模板、@提及）
│   │
│   └── webui/
│       ├── assets.go                # //go:embed 声明
│       └── static/
│           ├── index.html           # 旧版 SPA（保留 fallback）
│           ├── login.html           # 登录页
│           ├── dist/                # Vite 构建产物（Go embed）
│           ├── nav.js / theme.css
│
├── frontend/                        # Vue 3 + Vite SPA 源码
│   ├── package.json
│   ├── vite.config.js
│   ├── index.html
│   └── src/
│       ├── main.js                  # 入口
│       ├── App.vue                  # 根组件（侧边栏 + tab 切换）
│       ├── api.js                   # fetch 封装 + {success,data} 自动解包
│       ├── style.css                # 全局样式
│       ├── views/
│       │   ├── Dashboard.vue        # 统计卡片 + 主机网格
│       │   ├── Hosts.vue            # 主机 CRUD + 角色分配 + SSH 终端
│       │   ├── Diagram.vue          # Vue Flow 架构图
│       │   └── LogMonitor.vue       # 日志监控（规则/ES配置/实时日志/限流缓存）
│       └── components/
│           └── ServerNode.vue       # 8 Handle 自定义节点
│
└── deploy/                          # 部署文件
    ├── Dockerfile                    # Docker 构建文件
    ├── docker-compose.yml            # Docker Compose 配置
    └── deploy.md                     # 部署文档
```

---

## 3. 数据库表结构

数据库文件位于 `data/server.db`，使用 SQLite。所有表在首次启动时通过自动迁移（`migrate()`）创建。

### 3.1 hosts — 主机

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT PK | 主机 ID（UUID） |
| ip | TEXT NOT NULL | IP 地址 |
| hostname | TEXT NOT NULL | 主机名 |
| cpu | TEXT | CPU 信息（如 "8核"） |
| memory | TEXT | 内存信息（如 "32G"） |
| disk | TEXT | 磁盘信息（格式：`mount:size,mount:size`，如 `/:100,/data:200`） |
| status | INTEGER | 综合状态（0=Unknown, 1=Up, 2=Down, 3=Warning, 4=Muted） |
| maintenance | INTEGER | 维护模式（1=开启） |
| last_check_time | TEXT | 最后一次检查时间 |

### 3.2 roles — 服务角色

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT PK | 角色 ID（UUID） |
| name | TEXT NOT NULL | 角色名称（如 Redis, MySQL） |
| type | TEXT NOT NULL | 探测类型（ICMP/TCP/HTTP/SSH） |
| port | INTEGER | 服务端口 |
| path | TEXT | HTTP 探测路径 |
| timeout | INTEGER | 超时时间（秒，默认 5） |

### 3.3 assignments — 角色分配

| 字段 | 类型 | 说明 |
|------|------|------|
| host_id | TEXT NOT NULL | 主机 ID（联合主键） |
| role_id | TEXT NOT NULL | 角色 ID（联合主键） |
| status | INTEGER | 探测状态 |
| status_code | INTEGER | HTTP 状态码 |
| last_check_time | TEXT | 最后检查时间 |
| error_message | TEXT | 错误信息 |
| override_port | INTEGER | 覆盖端口（可选） |
| override_path | TEXT | 覆盖路径（可选） |
| consecutive_failures | INTEGER | 连续失败次数 |

### 3.4 ssh_credentials — SSH 凭据

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT PK | 凭据 ID（UUID） |
| label | TEXT | 显示标签 |
| username | TEXT NOT NULL | SSH 用户名 |
| auth_method | TEXT NOT NULL | 认证方式（password/key） |
| password | TEXT | 密码 |
| private_key | TEXT | 私钥内容 |

### 3.5 日志监控表

#### alert_rules — 告警规则

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT PK | 规则 ID（UUID） |
| name | TEXT NOT NULL | 规则名称 |
| enabled | INTEGER | 是否启用（1=启用，0=停用） |
| keywords | TEXT | 匹配关键词（JSON 数组或逗号分隔） |
| exclude_keywords | TEXT | 排除关键词 |
| level | TEXT | 日志级别过滤（空=不限制） |
| regex_pattern | TEXT | 正则表达式匹配 |
| cooldown | INTEGER | 冷却时间（秒，默认 300） |
| message_template | TEXT | 告警消息模板 |
| webhook_id | INTEGER | 关联的 Webhook 配置 ID（0=默认） |
| created_at | TEXT | 创建时间 |
| updated_at | TEXT | 更新时间 |

#### webhook_config — Webhook 推送配置

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK AUTOINCREMENT | 配置 ID |
| name | TEXT | 配置名称 |
| platform | TEXT | 平台类型（feishu/dingtalk/wechat/custom） |
| url | TEXT | Webhook URL |
| secret | TEXT | 签名密钥 |
| enabled | INTEGER | 是否启用（1=启用） |
| max_retries | INTEGER | 最大重试次数（默认 3） |
| mention_type | TEXT | @提及类型（none/all/specific） |
| mention_users | TEXT | 指定用户 ID 列表（逗号分隔） |
| rate_limit | INTEGER | 每分钟限流次数（0=不限） |
| rate_limit_per_second | INTEGER | 每秒限流次数（0=不限） |
| ring_buffer_size | INTEGER | 限流溢出环形缓冲区大小（默认 10000） |
| template | TEXT | 告警消息模板 |

#### es_config — ES 连接配置

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | 固定为 1（单行配置） |
| address | TEXT | ES 地址 |
| username | TEXT | ES 用户名 |
| password | TEXT | ES 密码 |
| index | TEXT | 索引名 |
| interval | INTEGER | 轮询间隔（秒，默认 15） |
| size | INTEGER | 每次查询最大日志数（默认 100） |
| query | TEXT | 查询体（JSON） |
| enabled | INTEGER | 是否启用（1=启用） |

#### limited_alert_logs — 限流溢出日志记录

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK AUTOINCREMENT | 记录 ID |
| rule_name | TEXT NOT NULL | 触发规则名称 |
| message | TEXT NOT NULL | 日志消息 |
| level | TEXT | 日志级别 |
| source | TEXT | 日志来源 |
| timestamp | TEXT | 原始时间戳 |
| limited_at | TEXT | 限流时间 |
| summary | TEXT | 摘要（保存模板信息） |

#### users — 用户

| 字段 | 类型 | 说明 |
|------|------|------|
| username | TEXT PK | 用户名 |
| password_hash | TEXT NOT NULL | bcrypt 密码哈希 |

### 3.6 默认种子数据

首次启动时，如果 `roles` 表为空，自动插入以下默认角色：

| 角色 ID | 名称 | 类型 | 端口 | 路径 | 超时 |
|---------|------|------|------|------|------|
| role-icmp | ICMP | ICMP | 0 | — | 5s |

> 其他旧版种子角色（Redis、MySQL、Nginx 等）已移除，需用户按需手动创建。

---

## 4. 完整 API 文档

所有 API 路由通过标准库 `net/http` 的 `http.ServeMux` 注册。认证通过 JWT Cookie 中间件保护，响应统一格式为 JSON。

### 4.1 认证接口

#### POST /api/auth/login

登录并获取会话 Cookie。

- **豁免认证**：是
- **请求体**：
  ```json
  { "username": "admin", "password": "admin" }
  ```
- **成功响应**（200）：
  ```json
  { "ok": true, "username": "admin" }
  ```
- **失败响应**（401）：
  ```json
  { "error": "invalid username or password" }
  ```
- **副作用**：设置 `scm_session` Cookie（HttpOnly、SameSite=Lax、24h 过期）

#### POST /api/auth/logout

清除当前会话。

- **豁免认证**：否
- **响应**：
  ```json
  { "ok": true }
  ```
- **副作用**：清除 `scm_session` Cookie

#### GET /api/auth/me

获取当前登录状态。

- **豁免认证**：是（自校验 Cookie）
- **已登录响应**：
  ```json
  { "authenticated": true, "username": "admin" }
  ```
- **未登录响应**：
  ```json
  { "authenticated": false }
  ```

#### GET /login

返回登录页面 HTML。

- **豁免认证**：是

#### GET /

返回 SPA 主页面 HTML。

- **豁免认证**：否（未登录跳转到 `/login?redirect=...`）

#### GET /common/\*

提供前端静态文件。

- **豁免认证**：是

#### GET /assets/\*

提供 Vite 构建产物（`dist/assets/*`）。
- **豁免认证**：是

---

### 4.2 大盘与拨测接口

#### GET /api/dashboard

获取大盘概览数据。

- **响应**：
  ```json
  {
    "success": true,
    "data": {
      "hosts": [
        {
          "host": { "id": "...", "ip": "10.0.0.1", "hostname": "node1", "cpu": "8核", "memory": "32G", "disk": "/:100,/data:200", "status": 1, ... },
          "roles": [
            { "host_id": "...", "role_id": "...", "status": 1, "status_code": 200,
              "role": { "id": "...", "name": "Nginx", "type": "HTTP", "port": 80, ... },
              ... }
          ],
          "is_alive": true
        }
      ],
      "roles": [ /* 非 ICMP 角色列表 */ ],
      "stats": { "total": 5, "healthy": 3, "unhealthy": 1, "maintenance": 1, "role_unhealthy": 2 }
    }
  }
  ```
- **说明**：ICMP 存活状态独立为 `is_alive`，不混入普通角色列表；`role_unhealthy` 统计角色异常数

#### GET /api/hosts

列出所有主机（裸 `[]Host`，不含角色数据）。

- **响应**：
  ```json
  { "success": true, "data": [ { "id": "...", "ip": "...", "hostname": "...", "cpu": "...", "memory": "...", "disk": "...", ... } ] }
  ```

#### POST /api/hosts

添加主机。

- **请求体**：
  ```json
  { "ip": "10.0.0.1", "hostname": "node1", "cpu": "8核", "memory": "32G", "disk": "/:100,/data:200" }
  ```
- **响应**（201）：
  ```json
  { "success": true, "message": "主机已添加", "data": { "id": "...", ... } }
  ```
- **说明**：自动绑定 ICMP 探测角色，并触发一次拨测

#### POST /api/hosts/update

更新主机信息。

- **请求体**：
  ```json
  { "id": "xxx", "ip": "10.0.0.1", "hostname": "node1", "cpu": "8核", "memory": "32G", "disk": "/:100" }
  ```
- **响应**：
  ```json
  { "success": true, "message": "主机已更新" }
  ```

#### DELETE /api/hosts?id=xxx

删除主机。

- **响应**：
  ```json
  { "success": true, "message": "主机已删除" }
  ```
- **说明**：同时删除该主机的所有分配记录

#### POST /api/hosts/batch

批量添加主机。

- **请求体**：
  ```json
  {
    "hosts": [
      { "hostname": "node1", "ip": "10.0.0.1", "cpu": "8核", "memory": "32G", "disk": "/:100" }
    ]
  }
  ```
- **响应**（201）：
  ```json
  {
    "success": true, "message": "批量添加完成",
    "data": { "count": 1, "hosts": [...], "errors": [] }
  }
  ```

#### POST /api/hosts/maintenance

切换主机维护模式。

- **请求体**：
  ```json
  { "host_id": "xxx" }
  ```
- **响应**：
  ```json
  { "success": true, "message": "维护模式已开启/已关闭", "data": { ... } }
  ```

#### GET /api/hosts/export

导出 Excel（xlsx）文件。

- **响应**：`Content-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`
- **说明**：
  - Sheet "主机列表"：主机名、IP、状态（中文）、**CPU(核)**、**内存(GB)**、磁盘（挂载点+容量）、角色、维护模式
  - 不再使用分区独立 Sheet，所有磁盘信息合并到主机列表行的磁盘列中

#### GET /api/roles

列出所有角色。

- **响应**：
  ```json
  { "success": true, "data": [ { "id": "...", "name": "Redis", "type": "TCP", "port": 6379, ... } ] }
  ```

#### POST /api/roles

添加角色。

- **请求体**：
  ```json
  { "name": "MyService", "type": "HTTP", "port": 8080, "path": "/health", "timeout": 5 }
  ```
- **响应**（201）：
  ```json
  { "success": true, "message": "角色已添加", "data": { "id": "...", ... } }
  ```
- **说明**：`type` 必须为 `ICMP`、`TCP`、`HTTP` 或 `SSH`；TCP/HTTP/SSH 需指定端口；`timeout` 默认 5

#### DELETE /api/roles?id=xxx

删除角色。

- **响应**：
  ```json
  { "success": true, "message": "角色已删除" }
  ```

#### POST /api/roles/batch

批量添加角色。

- **请求体**：
  ```json
  {
    "roles": [
      { "name": "ServiceA", "type": "TCP", "port": 8080, ... }
    ]
  }
  ```
- **响应**（201）：
  ```json
  { "success": true, "message": "批量添加完成", "data": { "count": 1, "roles": [...], "errors": [] } }
  ```

#### POST /api/assign

为主机分配角色。

- **请求体**：
  ```json
  { "host_id": "xxx", "role_id": "yyyy", "port": 3306, "path": "/" }
  ```
- **说明**：`port` 和 `path` 可选，用于覆盖角色默认值；不能重复分配（返回 400）
- **响应**（201）：
  ```json
  { "success": true, "message": "角色已指派" }
  ```
- **副作用**：调用 `Scheduler.ProbeHost(hostID)` 立即探测

#### DELETE /api/assign?host_id=xxx&role_id=yyy

删除角色分配。

- **响应**：
  ```json
  { "success": true, "message": "角色已取消指派" }
  ```

#### POST /api/assign/batch

批量分配角色。

- **请求体**：
  ```json
  { "host_ids": ["host1", "host2"], "role_id": "role-xxx", "port": 3306, "path": "/" }
  ```
- **响应**：
  ```json
  { "success": true, "message": "批量指派成功", "data": { "count": 2 } }
  ```

#### GET /api/refresh

触发一次拨测刷新。

- **响应**：
  ```json
  { "success": true, "message": "探测已触发" }
  ```

#### GET /api/ssh-credential

列出 SSH 凭据（脱敏返回）。

- **响应**：
  ```json
  {
    "success": true,
    "data": [
      { "id": "...", "label": "server1", "username": "root", "auth_method": "password",
        "password": "******", "private_key": "" }
    ]
  }
  ```

#### POST /api/ssh-credential

添加 SSH 凭据。

- **请求体**：
  ```json
  { "label": "server1", "username": "root", "auth_method": "password", "password": "mypass" }
  ```
- **响应**：
  ```json
  { "success": true, "message": "SSH 凭据已添加", "data": { "id": "xxx" } }
  ```

#### DELETE /api/ssh-credential?id=xxx

删除 SSH 凭据。

- **响应**：
  ```json
  { "success": true, "message": "凭据已删除" }
  ```

#### WS /api/ssh/ws?host_id=xxx&cred_id=yyy

SSH 终端 WebSocket 连接。

- **查询参数**：`host_id`（必需）、`cred_id`（可选，使用已保存凭据）、`host`（IP）、`port`（默认 22）、`user`、`pass`
- **数据传输**：使用 **BinaryMessage** 传输 SSH stdout 数据
  - 后端：`conn.WriteMessage(websocket.BinaryMessage, buf[:n])`
  - 前端：`termSocket.binaryType = 'arraybuffer'`，`terminal.write(new Uint8Array(e.data))`
- **认证流程**（手动模式）：首条 WebSocket 消息发送 JSON：
  ```json
  { "type": "manual", "username": "root", "auth_method": "password", "password": "..." }
  ```
- **终端 resize**：发送 JSON：
  ```json
  { "type": "resize", "cols": 120, "rows": 40 }
  ```
- **说明**：双通道管道 — xterm.js ↔ WebSocket (BinaryMessage) ↔ SSH stdin/stdout

---

### 4.3 日志监控接口

#### GET /api/health

健康检查。豁免认证。

- **响应**：
  ```json
  { "status": "ok" }
  ```

#### GET /api/stats

日志监控统计信息。

- **响应**：
  ```json
  {
    "ws_clients": 0,
    "dedup_size": 0,
    "rule_count": 5,
    "rate_limit": { "remaining_minute": 18, "remaining_second": 4, "limit_per_minute": 20, "limit_per_second": 5, "total_sent": 100, "total_limited": 5 },
    "webhook_stats": [ /* 各 Webhook 客户端的限流统计 */ ]
  }
  ```

#### GET /api/es/config

获取 ES 配置与连接状态。

- **响应**：
  ```json
  {
    "config": { "id": 1, "address": "http://your-es-server:9200", "username": "elastic", "index": "k8s-logs-*", "interval": 10, "size": 100, "query": "{}", "enabled": true },
    "status": "connected"
  }
  ```

#### POST /api/es/config

保存 ES 配置。

- **请求体**：
  ```json
  {
    "address": "http://your-es-server:9200",
    "username": "elastic",
    "password": "xxx",
    "index": "k8s-logs-*",
    "interval": 10,
    "size": 100,
    "query": "{}",
    "enabled": true
  }
  ```
- **响应**：
  ```json
  { "success": true, "message": "配置已保存" }
  ```
- **说明**：保存后自动重启 ESPipeline，连接新 ES 实例

#### GET /api/rules

列出所有告警规则。

- **响应**：
  ```json
  { "rules": [ { "id": "...", "name": "...", "enabled": true, "level": "error", "webhook_id": 1, ... } ] }
  ```

#### POST /api/rules

添加告警规则。

- **请求体**：
  ```json
  {
    "name": "Error 检测",
    "enabled": true,
    "keywords": "[\"error\", \"ERROR\"]",
    "exclude_keywords": "",
    "level": "",
    "regex_pattern": "",
    "cooldown": 300,
    "message_template": "告警: {rule_name}\\n级别: {level}\\n消息: {message}",
    "webhook_id": 0
  }
  ```
- **响应**：
  ```json
  { "message": "ok" }
  ```

#### POST /api/rules/update

更新告警规则（支持部分更新，如仅切换 enabled 状态）。

- **请求体**：同 POST /api/rules（需包含 `id`）
- **响应**：
  ```json
  { "message": "ok", "id": "xxx" }
  ```

#### POST /api/rules/delete?id=xxx

删除告警规则。

- **响应**：
  ```json
  { "message": "deleted" }
  ```

#### GET /api/webhook/config

获取所有 Webhook 配置。

- **响应**：
  ```json
  { "webhooks": [ { "id": 1, "name": "...", "platform": "feishu", "url": "...", "enabled": true, "rate_limit": 20, "rate_limit_per_second": 5, ... } ] }
  ```

#### POST /api/webhook/config

保存（新增或更新）Webhook 配置。

- **请求体**：
  ```json
  {
    "id": 0,
    "name": "告警群",
    "platform": "feishu",
    "url": "https://open.feishu.cn/open-apis/bot/v2/hook/xxx",
    "secret": "",
    "enabled": true,
    "max_retries": 3,
    "mention_type": "none",
    "mention_users": "",
    "rate_limit": 60,
    "rate_limit_per_second": 5,
    "ring_buffer_size": 10000,
    "template": ""
  }
  ```
- **说明**：`id=0` 为新增，`id>0` 为更新；更新时自动同步内存中的 Webhook 客户端
- **响应**：
  ```json
  { "message": "ok", "id": 1 }
  ```

#### DELETE /api/webhook/config?id=1

删除 Webhook 配置。

- **响应**：
  ```json
  { "message": "deleted" }
  ```

#### POST /api/webhook/test

测试 Webhook 推送。

- **请求体**：
  ```json
  {
    "webhook_id": 1,
    "rule_name": "测试告警",
    "message": "这是一条测试消息",
    "level": "INFO",
    "source": "webhook-test",
    "template": ""
  }
  ```
- **响应**：
  ```json
  { "success": true, "message": "测试消息已发送" }
  ```

#### GET /api/webhook/limited-alerts

获取限流缓存列表（包括内存缓冲区与数据库记录）。

- **响应**：
  ```json
  {
    "alerts": [ /* 内存缓冲区中的限流记录 */ ],
    "db_alerts": [ /* 数据库中的限流溢出记录 */ ],
    "db_total": 100
  }
  ```

#### GET /api/webhook/limited-alerts/history?page=1&page_size=20

获取限流历史记录（分页）。

- **响应**：
  ```json
  {
    "records": [ /* 分页记录 */ ],
    "total": 100,
    "page": 1,
    "page_size": 20,
    "total_pages": 5
  }
  ```

#### POST /api/webhook/limited-alerts/clear

清除所有限流记录（内存 + 数据库）。

- **响应**：
  ```json
  { "message": "cleared" }
  ```

#### POST /api/webhook/limited-alerts/cleanup

清理 24 小时前的过期限流记录。

- **响应**：
  ```json
  { "message": "cleanup done", "deleted": 50 }
  ```

#### WS /ws

实时日志 WebSocket 连接。

- **推送格式**：
  ```json
  { "type": "raw_log", "data": { "level": "ERROR", "message": "...", "source": "...", "timestamp": "..." } }
  { "type": "log_match", "data": { "rule_name": "...", "parsed_log": { ... }, "is_alert": true, "is_cooldown": false } }
  ```
- **说明**：通过 `Broadcast` 方法向所有连接的客户端推送日志数据

---

## 5. 关键数据模型

### 5.1 Host（主机）

```go
type Host struct {
    ID            string    `json:"id"`              // UUID
    IP            string    `json:"ip"`              // IP 地址
    Hostname      string    `json:"hostname"`        // 主机名
    CPU           string    `json:"cpu"`             // CPU 信息（如 "8核"）
    Memory        string    `json:"memory"`          // 内存信息（如 "32G"）
    Disk          string    `json:"disk"`            // 磁盘信息（格式 "mount:size,mount:size"）
    Status        int       `json:"status"`          // 状态：0=Unknown, 1=Up, 2=Down, 3=Warning, 4=Muted
    Maintenance   bool      `json:"maintenance"`     // 维护模式
    LastCheckTime time.Time `json:"last_check_time"` // 最后检查时间
}
```

### 5.2 Role（服务角色）

```go
type Role struct {
    ID      string `json:"id"`      // UUID
    Name    string `json:"name"`    // 角色名称（如 Redis, MySQL）
    Type    string `json:"type"`    // 探测类型：ICMP / TCP / HTTP / SSH
    Port    int    `json:"port"`    // 服务端口
    Path    string `json:"path"`    // HTTP 探测路径（如 "/health"）
    Timeout int    `json:"timeout"` // 超时秒数（默认 5）
}
```

### 5.3 Assignment（角色分配）

```go
type Assignment struct {
    HostID              string    `json:"host_id"`
    RoleID              string    `json:"role_id"`
    Role                *Role     `json:"role,omitempty"`
    Status              int       `json:"status"`
    StatusCode          int       `json:"status_code"`
    LastCheckTime       time.Time `json:"last_check_time"`
    ErrorMessage        string    `json:"error_message,omitempty"`
    OverridePort        *int      `json:"override_port,omitempty"`
    OverridePath        string    `json:"override_path,omitempty"`
    ConsecutiveFailures int       `json:"consecutive_failures"`
}
```

### 5.4 HostWithRoles（主机与角色聚合）

```go
type HostWithRoles struct {
    Host    Host         `json:"host"`
    Roles   []Assignment `json:"roles"`
    IsAlive bool         `json:"is_alive"`
}
```

### 5.5 ProbeResult（探测结果）

```go
type ProbeResult struct {
    Status     int    // 状态：1=Up, 2=Down
    StatusCode int    // HTTP 状态码（仅 HTTP 探测）
    Error      string // 错误信息
}
```

### 5.6 SSHCredential（SSH 凭据）

```go
type SSHCredential struct {
    ID         string `json:"id"`
    Label      string `json:"label"`
    Username   string `json:"username"`
    AuthMethod string `json:"auth_method"`
    Password   string `json:"password,omitempty"`
    PrivateKey string `json:"private_key,omitempty"`
}
```

### 5.7 ESConfig

```go
type ESConfig struct {
    ID       int    `json:"id"`
    Address  string `json:"address"`
    Username string `json:"username"`
    Password string `json:"password,omitempty"`
    Index    string `json:"index"`
    Interval int    `json:"interval"`
    Size     int    `json:"size"`
    Query    string `json:"query"`
    Enabled  bool   `json:"enabled"`
}
```

### 5.8 状态常量

| 常量名 | 值 | 含义 |
|--------|----|------|
| HostStatusUnknown | 0 | 未知（未探测） |
| HostStatusUp | 1 | 正常（Up） |
| HostStatusDown | 2 | 异常（Down） |
| HostStatusWarning | 3 | 警告（重试中） |
| HostStatusMuted | 4 | 静音（预留） |

---

## 6. 认证流程

### 6.1 认证方式

SCM 使用 **JWT HS256**（HMAC-SHA256）进行无状态会话管理，由 `internal/auth/jwt.go` 实现。

- **签名密钥**：通过环境变量 `AUTH_JWT_SECRET` 配置（默认 `dev-secret`）
- **Token 结构**：Base64Url(header) + "." + Base64Url(claims) + "." + Base64Url(signature)
- **Claims 内容**：
  ```json
  { "sub": "admin", "iat": 1234567890, "exp": 1234567890 }
  ```

### 6.2 Cookie 管理

由 `internal/auth/middleware.go` 实现：

| 属性 | 值 |
|------|----|
| Cookie 名称 | `scm_session` |
| HttpOnly | 是 |
| SameSite | Lax |
| 有效期 | 24 小时 |

### 6.3 中间件逻辑

```go
// 1. 检查请求路径是否在豁免列表中
// 2. 如果豁免 → 直接放行
// 3. 否则从 Cookie 中获取 scm_session
// 4. 校验 JWT 签名和有效期
// 5. 校验失败：
//    - /api/ 开头的请求 → 返回 401 JSON
//    - 其余请求 → 302 跳转到 /login?redirect=...
```

### 6.4 豁免路径

| 路径 | 原因 |
|------|------|
| `/login` | 登录页面 |
| `/api/auth/*` | 登录、登出、Me |
| `/common/*` | 静态文件 |
| `/api/health` | 健康检查 |
| `/assets/*` | Vite 构建产物 |

### 6.5 管理员账号

- **默认账号**：`admin` / `admin`
- **密码存储**：bcrypt 加密
- **配置方式**：
  - 环境变量 `ADMIN_USER`：设置用户名
  - 环境变量 `ADMIN_PASSWORD_HASH`：设置密码的 bcrypt 哈希
  - 如果未配置，默认使用 `admin` 明文密码自动生成 bcrypt 哈希

### 6.6 登录流程

```
用户 → POST /api/auth/login {username, password}
  → bcrypt.CompareHashAndPassword 校验密码
  → JWT Sign(secret, username, 24h)
  → Set-Cookie: scm_session=<token>; HttpOnly; SameSite=Lax; Max-Age=86400
  → 返回 {ok: true, username: "admin"}
```

---

## 7. 拨测机制

### 7.1 调度器

由 `internal/dashboard/scheduler.go` 实现：

- **间隔**：`config.yaml` 中 `dashboard.probe_interval` 配置（默认 15 秒）
- **调度方式**：启动时立即执行一次，之后按固定间隔定时执行
- **手动触发**：`GET /api/refresh` 或 `Scheduler.Trigger()`
- **并发模型**：所有探测任务通过 goroutine 并发执行，结果通过 channel 收集

### 7.2 四种探测方式

#### ICMP 探测（`ProbeICMP`）

```go
// 查找系统 ping 命令（/sbin/ping, /usr/sbin/ping, /bin/ping, /usr/bin/ping）
// 执行: ping -c 3 -W <timeout> <ip>
// 成功条件: 命令返回 0
```

#### TCP 探测（`ProbeTCP`）

```go
// 使用 net.DialTimeout 建立 TCP 连接
// 成功条件: 在超时时间内连接成功
// 用途: Redis(6379), MySQL(3306), PostgreSQL(5432), K8s(6443), RabbitMQ(5672)
```

#### HTTP 探测（`ProbeHTTP`）

```go
// 使用 http.Client.Get 发起 HTTP GET 请求
// 禁用重定向（CheckRedirect: ErrUseLastResponse）
// 成功条件: 状态码 200-399
// 用途: Nginx(80/)
```

#### SSH 探测（`ProbeSSH`）

```go
// 使用 golang.org/x/crypto/ssh 建立 SSH 连接
// 支持密码认证和密钥认证
// 成功条件: 登录成功后执行 whoami 命令成功
// 用途: SSH(22)
```

### 7.3 端口解析

```go
func ResolveProbeParams(role *Role, assignment *Assignment) (port int, path string)
```

- 默认使用 `role.Port`
- 若 `assignment.OverridePort != nil`，使用覆盖端口
- 角色创建时建议填写正确的端口（HTTP→80、SSH→22），前端表单选择类型后自动填充

### 7.4 失败重试策略

```
连续失败 0 次 → 状态保持 Up
第 1 次失败   → 状态设为 Warning，记录 "第1次重试中"
第 2 次失败   → 状态设为 Down（标红），记录错误信息
              （MaxRetries = 2，即连续失败 2 次才标红）
成功后       → 重置连续失败次数为 0
```

### 7.5 综合状态计算

```
主机综合状态 = MAX(所有分配角色的状态)
  → 如果任一角色 Down → 主机 Down
  → 如果任一角色 Warning → 主机 Warning
  → 全部 Up → 主机 Up
  → 无角色 → Unknown
维护模式下的主机跳过探测，不影响综合状态
```

### 7.6 ICMP 特殊处理

- **自动绑定**：添加主机时自动创建 ICMP 角色分配
- **角色 ID**：固定为 `role-icmp`
- **显示分离**：ICMP 状态单独记录为 `is_alive`，不混入普通角色列表
- **前端展示**：LED 颜色始终显示 ICMP 状态；卡片标签分两行 — `存活: 在线/离线`（ICMP）+ `nginx: 正常/异常`（角色探针）

---

## 8. SSH 终端

### 8.1 连接流程

```
用户点击 SSH 按钮
  → 选择凭据或手动输入
  → 前端连接 WebSocket: /api/ssh/ws?host_id=xxx&cred_id=yyy
  → 服务端查询主机 IP
  → 如果 cred_id 已提供：
      → 从数据库加载凭据，自动认证
  → 如果 cred_id 未提供：
      → 等待前端发送 JSON 认证信息
  → 建立 SSH 连接 → 请求 PTY（xterm-256color, 40×120）
  → 启动 Shell → 双通道数据传输（BinaryMessage）
```

### 8.2 数据传输格式

- **后端**：`conn.WriteMessage(websocket.BinaryMessage, buf[:n])`
- **前端**：`termSocket.binaryType = 'arraybuffer'`，`onmessage` 用 `new Uint8Array(e.data)` 写入 xterm
- **更改为 BinaryMessage 的原因**：SSH 原始字节包含非 UTF-8 序列，TextMessage 会导致乱码

### 8.3 认证方式

- **已保存凭据**：通过 `cred_id` 参数指定，自动从数据库加载
- **手动认证**：通过 WebSocket 发送 JSON：
  ```json
  { "type": "manual", "username": "root", "auth_method": "password", "password": "..." }
  { "type": "manual", "username": "root", "auth_method": "key", "private_key": "-----BEGIN..." }
  ```

### 8.4 凭据模式 UI

凭据连接时，`termCredMode = true`，终端窗口隐藏手动输入框，只显示提示"已使用 SSH 凭据自动连接"和断开按钮。

### 8.5 终端 Resize

```json
{ "type": "resize", "cols": 120, "rows": 40 }
```

服务端调用 `session.WindowChange(rows, cols)`。

---

## 9. 架构图功能

### 9.1 技术选型

使用 **@vue-flow/core** v1.41+ 实现，替代旧版手写 SVG 方案。

### 9.2 数据存储

- **存储位置**：浏览器 `localStorage`，key 为 `scm-diagram-data`
- **数据结构**：
  ```json
  {
    "nodes": [ { "id": "...", "type": "custom", "position": {"x":100, "y":200}, "data": {"label":"Nginx", "ip":"...", "role":"nginx", "status":"online", "tags":[]} } ],
    "edges": [ { "id": "...", "source": "...", "target": "...", "sourceHandle": "s-tc", "targetHandle": "t-tc", "type": "smoothstep", "style": {}, "label": "", "animated": false } ],
    "textlabel": [ { ... } ],
    "group": [ { ... } ]
  }
  ```

### 9.3 自定义节点（ServerNode.vue）

每个服务节点有 **8 个 Handle 连接点**，每个点位同时有 `source` 和 `target` 两种类型：

| 点位 ID | 位置 | 说明 |
|---------|------|------|
| TL | 左上角 | `t-tl`(target) + `s-tl`(source) |
| TC | 上边中间 | `t-tc` + `s-tc` |
| TR | 右上角 | `t-tr` + `s-tr` |
| RC | 右边中间 | `t-rc` + `s-rc` |
| BR | 右下角 | `t-br` + `s-br` |
| BC | 下边中间 | `t-bc` + `s-bc` |
| BL | 左下角 | `t-bl` + `s-bl` |
| LC | 左边中间 | `t-lc` + `s-lc` |

Handle 默认隐藏（`opacity: 0`），hover 时显示，连线模式下全局显示。

### 9.4 功能特性

| 功能 | 实现方式 |
|------|---------|
| 四种交互模式 | `diagMode` 状态（select/connect/text/group） |
| 节点拖拽 | Vue Flow 内置 draggable |
| 连线 | connect 模式下点击 source Handle → target Handle |
| 连线类型 | smoothstep（默认）/ bezier / straight / step |
| 样式切换 | 实线/虚线、箭头/无箭头 |
| 文字标注 | text 模式下点击画布空白处添加 |
| 分组框 | group 模式拖拽生成，可调整大小、选择填充色/边框色/名称/虚实线 |
| 右键菜单 | 删除节点/边/文字/分组 |
| 导出 PDF | 隐藏 Controls+Background → `html-to-image.toPng` → `jsPDF.addImage`（A4 横向，纯白背景） |
| 保存/恢复 | `localStorage`（key=`scm-diagram-data`） |
| 一键清除 | 清空画布所有内容 |

### 9.5 角色数据

- 节点从 `/api/dashboard` 加载 `HostWithRoles`
- 自动提取第一个非 ICMP 角色的 `role.name` 作为 `data.role`
- 节点显示角色名和服务状态

---

## 10. 日志监控

### 10.1 整体流程

```
Elasticsearch
  └─ 定时轮询（可配置间隔，通过 size 限制每次查询日志数）
      └─ 原始 JSON 日志
          ├─ Parser 解析
          │   ├─ 字段映射（level/message/timestamp/source）
          │   ├─ K8s 元信息提取（namespace/pod/container）
          │   └─ 日志级别识别（FATAL/ERROR/WARN/INFO/DEBUG）
          │
          ├─ Deduper 去重（MD5 + 5 分钟 TTL）
          │
          ├─ Filter 规则匹配
          │   ├─ 关键词匹配（支持多个）
          │   ├─ 正则匹配
          │   ├─ 级别过滤
          │   ├─ 排除关键词
          │   └─ 指纹冷却（相同来源+相同消息）
          │       ├─ 匹配 → IsAlert=true → Webhook 推送
          │       └─ 不匹配 → 仅通过 WebSocket 广播
          │
          └─ WebSocket Hub 广播
              ├─ type: "raw_log" → 未匹配的原始日志（含级别、时间戳）
              └─ type: "log_match" → 匹配结果（含告警成功/失败信息、令牌剩余）
```

### 10.2 Elasticsearch 客户端（`es/client.go`）

- **连接**：支持 HTTPS，跳过 TLS 验证
- **认证**：基本认证（用户名+密码）
- **轮询**：按配置间隔（默认 10 秒）定时查询，`size` 参数控制每次查询最大日志数
- **查询模板**：支持 `{interval}` 占位符替换，实现滚动时间窗口
- **超时**：连接超时 5 秒，响应超时 10 秒

### 10.3 日志解析器（`parser/parser.go`）

- **字段映射**：将 ES JSON 中的字段自动映射到标准字段：

  | 标准字段 | 来源字段（按优先级） |
  |----------|-------------------|
  | level | `log_level` / `severity` / `level` / `stream` |
  | message | `message` / `msg` / `log` / `content` |
  | timestamp | `@timestamp` / `timestamp` / `time` |
  | source | `host` / `hostname` / `server` / `service` / `kubernetes.namespace_name` |

- **K8s 元信息**：自动提取 `kubernetes` 对象中的 `namespace_name`、`pod_name`、`container_name`、`node_name`、`host`
- **级别推断**：从消息内容中关键词识别（FATAL/ERROR/WARN/INFO/DEBUG/TRACE）

### 10.4 告警规则过滤（`filter/filter.go`）

- **匹配逻辑**：`(关键词 OR 正则) AND 级别 AND NOT 排除关键词`
- **指纹冷却**：
  - 指纹 = `{来源}:{消息内容 MD5 前 16 位}`
  - 相同指纹在冷却期内不重复告警（默认 5 分钟）
  - 不同指纹（新错误类型）立即告警
- **运行时状态**：每条规则独立维护指纹时间映射，支持动态增删改

### 10.5 去重（`dedup/dedup.go`）

- **算法**：MD5（全量 JSON）
- **TTL**：5 分钟
- **容量**：最大 10 万条，超出时淘汰最旧记录
- **清理**：后台定时清理过期记录（每 TTL/2 检查一次）

### 10.6 Webhook 推送（`webhook/feishu.go`）

- **平台支持**：飞书（feishu）、钉钉（dingtalk）、企业微信（wechat）、自定义（custom）
- **消息格式**：飞书富文本（post），支持标题、正文、@提及
- **消息模板**：支持变量替换 `{rule_name}` `{level}` `{source}` `{timestamp}` `{message}`
- **重试机制**：指数退避重试（1s、2s、4s...），遇到飞书频率限制错误（code=11232）等待 15 秒
- **限流**：双令牌桶（每分钟 + 每秒），支持独立配置
- **多客户端**：支持多个 Webhook 客户端独立管理，每个有独立的限流和模板配置

### 10.7 限流溢出处理

```
Webhook 发送
  → 令牌桶检查
  ├─ 通过 → 立即发送，日志输出 [INFO] [ESPipeline] 告警成功 [令牌剩余: 18/4]
  └─ 受限 → 推入环形缓冲区（默认 10000 条）
              ├─ 缓冲区未满 → 暂存
              └─ 缓冲区已满 → 溢出回调 → 写入 limited_alert_logs 表
                                └─ 后台 DrainLoop（每 10 秒检查）
                                    └─ 从 DB 加载 → 按限流速率逐条重发
```

### 10.8 实时日志 WebSocket（`ws/hub.go`）

- **端点**：`/ws`
- **推送事件**：

  | type | data | 说明 |
  |------|------|------|
  | `raw_log` | `ParsedLog` | 未匹配规则但通过去重的日志 |
  | `log_match` | `FilteredLog` / `alert result` | 匹配规则的日志（含告警成功/失败信息、级别、令牌剩余） |

- **广播方式**：每次有新日志时向所有连接的客户端推送

---

## 附：配置说明

### config.yaml（非敏感配置）

```yaml
server:
  port: 3972

log_monitor:
  elasticsearch:
    address: "http://your-es-server:9200"
    username: "elastic"
    index: "k8s-logs-*"
    interval: 10
    query:
      size: 100
      sort:
        - "@timestamp":
            order: desc
      query:
        bool:
          filter:
            - range:
                "@timestamp":
                  gte: "now-{interval}s"
                  lte: "now"
  feishu_webhook:
    url: ""
    max_retries: 3

dashboard:
  probe_interval: "15s"

auth:
  admin_user: "admin"

store_cfg:
  driver: sqlite
  path: "./data/server.db"
```

### .env（敏感配置）

```env
ES_PASSWORD="your-es-password"
AUTH_JWT_SECRET="change-me-to-a-long-random-string"
ADMIN_USER=admin
ADMIN_PASSWORD_HASH='$2b$12$...bcrypt-hash...'
```
