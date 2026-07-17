# Watchtower (瞭望塔) — AI 上下文文档

## 项目概述

Watchtower (瞭望塔) — Go + Vue 3 全栈单页应用，Linux 服务大盘管理平台。

## 项目结构

```
server_controller_manager/
├── watchtower-backend/              # Go 后端代码
│   ├── cmd/server/main.go           # 入口：配置加载 → 组件初始化 → 路由注册
│   ├── configs/
│   │   ├── config.yaml              # 非敏感配置（端口、ES 地址、探针间隔）
│   │   ├── .env                     # 敏感配置（ES密码/JWT密钥/管理员hash）
│   │   ├── config.example.yaml      # 配置示例
│   │   └── .env.example             # 环境变量示例
│   ├── data/
│   │   └── server.db                # SQLite 数据库文件（自动创建）
│   ├── internal/
│   │   ├── store/                   # SQLite 存储层
│   │   │   ├── interface.go         # Store 接口定义
│   │   │   ├── model.go             # 数据模型（已废弃，由 model 包接管）
│   │   │   └── sqlite.go            # 建表、CRUD、自动迁移
│   │   ├── model/                   # 数据模型定义
│   │   │   ├── alert_rule.go        # 告警规则模型
│   │   │   ├── constants.go         # 状态常量
│   │   │   └── ...                  # 其他模型
│   │   ├── auth/                    # JWT 认证
│   │   │   ├── jwt.go               # HS256 签发/校验
│   │   │   ├── middleware.go        # Cookie 中间件（豁免路径 prefix 匹配）
│   │   │   └── handlers.go          # 登录/登出/Me/修改密码
│   │   ├── middleware/              # HTTP 中间件
│   │   │   ├── auth.go              # 认证中间件封装
│   │   │   └── cors.go              # CORS 中间件
│   │   ├── dashboard/               # 拨测模块
│   │   │   ├── prober.go            # ICMP/TCP/HTTP/SSH 探测
│   │   │   ├── scheduler.go         # 定时调度器
│   │   │   ├── collector.go         # SSH 采集 CPU/内存/磁盘
│   │   │   └── ssh_ws.go            # SSH WebSocket（BinaryMessage）
│   │   ├── core/                    # 核心配置
│   │   │   ├── config.go            # 配置结构体 + 默认值
│   │   │   └── config_loader.go     # YAML + .env 加载
│   │   └── app/                     # 应用层
│   │       ├── enter.go             # App 顶层封装
│   │       └── v1/
│   │           ├── enter.go         # v1 路由注册入口
│   │           ├── user/            # 用户认证 API
│   │           ├── dashboard/       # 大盘/主机/角色 API
│   │           └── logmonitor/      # 日志监控 API
│   │
│   └── watchtower-frontend/         # Vue 3 + Vite SPA 源码
│       ├── src/
│       │   ├── main.js              # 入口
│       │   ├── App.vue              # 根组件（侧边栏 + 登录/改密/登出）
│       │   ├── api.js               # fetch 封装 + 认证 API
│       │   ├── style.css            # 全局样式
│       │   ├── views/
│       │   │   ├── Login.vue        # 登录页
│       │   │   ├── Dashboard.vue    # 统计卡片 + 主机网格
│       │   │   ├── Hosts.vue        # 主机 CRUD + 角色分配 + SSH 终端
│       │   │   ├── Diagram.vue      # 架构图（Vue Flow）
│       │   │   └── LogMonitor.vue   # 日志监控
│       │   └── components/
│       │       ├── ServerNode.vue    # 8 Handle 自定义节点
│       │       └── ExportDialog.vue  # Excel 导出命名弹窗
│       ├── public/                  # 静态资源（favicon.ico）
│       ├── index.html
│       ├── vite.config.js           # 开发代理配置（支持 WS 代理）
│       └── package.json
│
├── deploy/                          # 部署文件
│   ├── Dockerfile                   # 多阶段构建（前端 node:18 + 后端 go:1.25 + nginx:alpine）
│   ├── docker-compose.yml           # 前后端容器编排（frontend:3972:80, backend:3972）
│   ├── nginx.conf                   # Nginx 反向代理（含 /api/ssh/ws WebSocket 支持）
│   └── deploy.md                    # 部署文档
│
└── docs/                            # 项目文档
    ├── ai-context.md                # AI 上下文（本文件）
    ├── architecture-api.md          # 架构与 API 文档
    └── user-guide.md                # 用户手册
```

## API 路由（全部通过标准库 net/http ServeMux 注册）

```
# 认证
POST   /api/auth/login              # JWT 登录（豁免认证）
POST   /api/auth/logout             # 登出
GET    /api/auth/me                 # 当前用户（豁免认证，自校验 Cookie）
PUT    /api/auth/change-password    # 修改密码（持久化到数据库）
GET    /login                       # 登录页（豁免认证，向后兼容）
GET    /common/*                    # 静态文件（豁免认证）

# 大盘 & 主机
GET    /api/dashboard               # 大盘数据（hosts+roles+stats）
GET    /api/hosts                   # 主机列表
POST   /api/hosts                   # 添加主机
POST   /api/hosts/update            # 更新主机
DELETE /api/hosts?id=xxx            # 删除主机
POST   /api/hosts/batch             # 批量添加
POST   /api/hosts/maintenance       # 维护模式切换
GET    /api/hosts/export            # Excel 导出

# 角色
GET    /api/roles                   # 角色列表
POST   /api/roles                   # 添加角色
DELETE /api/roles?id=xxx            # 删除角色
POST   /api/roles/batch             # 批量添加

# 分配
POST   /api/assign                  # 分配角色
DELETE /api/assign?host_id=xxx&role_id=yyy  # 取消分配
POST   /api/assign/batch            # 批量分配
GET    /api/refresh                 # 手动触发探测

# SSH
GET    /api/ssh-credential          # 凭据列表
POST   /api/ssh-credential          # 添加凭据
DELETE /api/ssh-credential?id=xxx   # 删除凭据
WS     /api/ssh/ws                  # SSH 终端（BinaryMessage）

# 日志监控
GET    /api/health                  # 健康检查（豁免认证）
GET    /api/stats                   # 统计信息
GET    /api/es/config               # 获取 ES 配置 + 连接状态
POST   /api/es/config               # 保存 ES 配置
GET    /api/rules                   # 告警规则列表
POST   /api/rules                   # 添加规则
POST   /api/rules/update            # 更新规则（支持部分更新）
POST   /api/rules/delete?id=xxx     # 删除规则
GET    /api/webhook/config          # Webhook 配置列表
POST   /api/webhook/config          # 新增/更新 Webhook
DELETE /api/webhook/config?id=1     # 删除 Webhook
POST   /api/webhook/test            # 测试 Webhook
GET    /api/webhook/limited-alerts              # 限流缓存
GET    /api/webhook/limited-alerts/history      # 限流历史（分页）
POST   /api/webhook/limited-alerts/clear        # 清除限流记录
POST   /api/webhook/limited-alerts/cleanup      # 清理过期记录
WS     /ws                          # 实时日志
```

## 响应格式

```json
// 成功
{ "success": true, "data": {...}, "message": "..." }
// 认证相关
{ "ok": true, "username": "admin" }
// 失败
{ "error": "..." }
```

## 数据库表结构

### hosts
- `id` TEXT PK, `ip` TEXT, `hostname` TEXT, `cpu` TEXT, `memory` TEXT, `disk` TEXT, `status` INTEGER, `maintenance` INTEGER, `last_check_time` TEXT

### roles
- `id` TEXT PK, `name` TEXT, `type` TEXT (ICMP/TCP/HTTP/SSH), `port` INTEGER, `path` TEXT, `timeout` INTEGER

### assignments
- `host_id` TEXT PK, `role_id` TEXT PK, `status` INTEGER, `status_code` INTEGER, `last_check_time` TEXT, `error_message` TEXT, `override_port` INTEGER, `override_path` TEXT, `consecutive_failures` INTEGER

### ssh_credentials
- `id` TEXT PK, `label` TEXT, `username` TEXT, `auth_method` TEXT, `password` TEXT, `private_key` TEXT

### alert_rules
- `id` TEXT PK, `name` TEXT, `enabled` INTEGER, `keywords` TEXT, `exclude_keywords` TEXT, `level` TEXT, `regex_pattern` TEXT, `cooldown` INTEGER, `message_template` TEXT, `webhook_id` INTEGER, `created_at` TEXT, `updated_at` TEXT

### webhook_config
- `id` INTEGER PK AUTOINCREMENT, `name` TEXT, `platform` TEXT, `url` TEXT, `secret` TEXT, `enabled` INTEGER, `max_retries` INTEGER, `mention_type` TEXT, `mention_users` TEXT, `rate_limit` INTEGER, `rate_limit_per_second` INTEGER, `ring_buffer_size` INTEGER, `template` TEXT

### es_config
- `id` INTEGER PK, `address` TEXT, `username` TEXT, `password` TEXT, `"index"` TEXT, `interval` INTEGER DEFAULT 15, `size` INTEGER DEFAULT 100, `query` TEXT DEFAULT '{}', `enabled` INTEGER

### limited_alert_logs
- `id` INTEGER PK AUTOINCREMENT, `rule_name` TEXT, `message` TEXT, `level` TEXT, `source` TEXT, `timestamp` TEXT, `limited_at` TEXT, `summary` TEXT

### users
- `username` TEXT PK, `password_hash` TEXT

## 状态常量
- `HostStatusUnknown`=0, `HostStatusUp`=1, `HostStatusDown`=2, `HostStatusWarning`=3, `HostStatusMuted`=4

## 关键架构决策

1. **标准库路由**：无第三方 Web 框架，纯 `net/http` ServeMux
2. **SQLite 自动迁移**：首次启动自动建表，旧表通过 ALTER TABLE 补充新列（忽略错误）
3. **前后端分离**：前端独立运行在 Vite 开发服务器（热更新），生产环境通过 Nginx 反向代理
4. **端口配置优先级**：环境变量 `PORT` > `config.yaml server.port` > 默认 3972
5. **ICMP 分离**：作为存活探测，不混入普通角色列表；前端 LED/Label 独立展示
6. **认证豁免**：`/login`、`/api/auth/*`、`/common/*`、`/api/health` 豁免中间件校验（prefix 匹配）
7. **密码持久化**：登录密码从数据库校验，修改密码通过 API 持久化到 SQLite
8. **SSH BinaryMessage**：WebSocket 使用 BinaryMessage 避免非 UTF-8 乱码
9. **ESPipeline 动态控制**：ES 客户端支持运行时启动/停止
10. **Webhook 多客户端**：每个 Webhook 独立管理，独立的限流器、模板、@配置
11. **ES size 参数**：每次查询最大日志数通过配置控制，不在查询体中硬编码
12. **WebSocket 端口**：开发环境走 Vite 代理（同端口），生产环境通过环境变量 `VITE_WS_PORT` 配置直连后端端口

## Excel 导出格式
- 单 sheet "主机列表"
- 列：主机名、IP、状态（中文）、CPU(核)、内存(GB)、磁盘（挂载点:容量）、角色、维护模式
- 不再使用分区独立 Sheet

## 使用到的第三方 Go 库
- `modernc.org/sqlite` — 纯 Go SQLite 驱动（无 CGo）
- `golang.org/x/crypto` — bcrypt（密码）、ssh（连接）
- `gorilla/websocket` — WebSocket
- `github.com/xuri/excelize/v2` — Excel 导出
- `github.com/golang-jwt/jwt/v5` — JWT 签名

## 使用到的前端依赖
- `vue@3` — 前端框架
- `@vue-flow/core@^1.41.0` — 架构图
- `xterm` + `xterm-addon-fit` — SSH 终端
- `html-to-image` + `jspdf` — 架构图导出 PDF
