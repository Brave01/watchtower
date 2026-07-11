# SCM — AI 上下文文档

## 项目概述

Server Controller Manager (SCM) — Go + Vue 3 全栈单页应用，Linux 服务大盘管理平台。

## 项目结构

```
watchtower/
├── main.go                    # 入口：配置加载 → 组件初始化 → 路由注册
├── logmonitor_init.go         # 日志监控初始化（旧版入口）
├── configs/
│   ├── config.yaml            # 非敏感配置
│   ├── .env                   # 敏感配置（ES密码/JWT密钥/管理员hash）
│   ├── config.example.yaml    # 配置示例
│   └── .env.example           # 环境变量示例
├── internal/
│   ├── store/                 # SQLite 存储层
│   │   ├── interface.go       # Store 接口
│   │   ├── models.go          # 数据模型
│   │   └── sqlite.go          # 建表、CRUD、自动迁移、seedDefaults
│   ├── auth/                  # JWT 认证
│   │   ├── jwt.go             # HS256 签发/校验
│   │   ├── middleware.go      # Cookie 中间件
│   │   └── handlers.go        # 登录/登出/Me
│   ├── dashboard/             # 拨测模块
│   │   ├── prober.go          # ICMP/TCP/HTTP/SSH
│   │   ├── scheduler.go       # 定时调度器
│   │   └── ssh_ws.go          # SSH WebSocket（BinaryMessage）
│   ├── handler/               # HTTP 路由与处理器
│   │   ├── auth.go            # 认证路由
│   │   ├── dashboard.go       # 大盘/主机/角色/分配 CRUD
│   │   ├── logmonitor.go      # 日志监控 API
│   │   └── es_pipeline.go     # ESPipeline 动态控制
│   ├── logmonitor/            # 日志监控子系统
│   │   ├── parser/parser.go   # ES JSON 解析器
│   │   ├── filter/filter.go   # 告警规则过滤
│   │   ├── dedup/dedup.go     # MD5 + TTL 去重
│   │   ├── ws/hub.go          # WebSocket Hub
│   │   ├── es/client.go       # ES 客户端（支持 size 参数）
│   │   └── webhook/feishu.go  # Webhook 客户端
│   └── webui/
│       ├── assets.go          # go:embed
│       └── static/dist/       # Vite 构建产物
├── frontend/                  # Vue 3 + Vite SPA
│   └── src/
│       ├── views/
│       │   ├── Dashboard.vue  # 大盘
│       │   ├── Hosts.vue      # 主机 CRUD + SSH
│       │   ├── Diagram.vue    # 架构图（Vue Flow）
│       │   └── LogMonitor.vue # 日志监控
│       └── components/
│           └── ServerNode.vue # 8 Handle 自定义节点
└── deploy/                      # 部署文件
    ├── Dockerfile
    ├── docker-compose.yml
    └── deploy.md
```

## API 路由（全部通过标准库 net/http ServeMux 注册）

```
# 认证
POST /api/auth/login             # JWT 登录
POST /api/auth/logout            # 登出
GET  /api/auth/me                # 当前用户
GET  /login                      # 登录页（豁免认证）
GET  /                           # SPA 主页
GET  /common/*                   # 静态文件（豁免认证）
GET  /assets/*                   # Vite 构建产物（豁免认证）

# 大盘 & 主机
GET  /api/dashboard              # 大盘数据（hosts+roles+stats）
GET  /api/hosts                  # 主机列表
POST /api/hosts                  # 添加主机
POST /api/hosts/update           # 更新主机
DELETE /api/hosts?id=xxx         # 删除主机
POST /api/hosts/batch            # 批量添加
POST /api/hosts/maintenance      # 维护模式切换
GET  /api/hosts/export           # Excel 导出

# 角色
GET  /api/roles                  # 角色列表
POST /api/roles                  # 添加角色
DELETE /api/roles?id=xxx         # 删除角色
POST /api/roles/batch            # 批量添加

# 分配
POST /api/assign                 # 分配角色
DELETE /api/assign?host_id=xxx&role_id=yyy  # 取消分配
POST /api/assign/batch           # 批量分配
GET  /api/refresh                # 手动触发探测

# SSH
GET  /api/ssh-credential         # 凭据列表
POST /api/ssh-credential         # 添加凭据
DELETE /api/ssh-credential?id=xxx # 删除凭据
WS   /api/ssh/ws                 # SSH 终端（BinaryMessage）

# 日志监控
GET  /api/health                 # 健康检查
GET  /api/stats                  # 统计信息
GET  /api/es/config              # 获取 ES 配置 + 连接状态
POST /api/es/config              # 保存 ES 配置
GET  /api/rules                  # 告警规则列表
POST /api/rules                  # 添加规则
POST /api/rules/update           # 更新规则（支持部分更新）
POST /api/rules/delete?id=xxx    # 删除规则
GET  /api/webhook/config         # Webhook 配置列表
POST /api/webhook/config         # 新增/更新 Webhook
DELETE /api/webhook/config?id=1  # 删除 Webhook
POST /api/webhook/test           # 测试 Webhook
GET  /api/webhook/limited-alerts           # 限流缓存（内存+数据库）
GET  /api/webhook/limited-alerts/history   # 限流历史（分页）
POST /api/webhook/limited-alerts/clear     # 清除限流记录
POST /api/webhook/limited-alerts/cleanup   # 清理过期记录
WS   /ws                         # 实时日志
```

## 响应格式

```json
// 成功
{ "success": true, "data": {...}, "message": "..." }
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
3. **ICMP 分离**：作为存活探测，不混入普通角色列表；前端 LED/Label 独立展示
4. **认证豁免**：静态文件、登录页、健康检查路径豁免中间件校验
5. **前端 SPA**：Vite 构建产物嵌入 Go 二进制（`//go:embed`），可通过 GOOS/GOARCH 交叉编译
6. **SSH BinaryMessage**：WebSocket 使用 BinaryMessage 避免非 UTF-8 乱码
7. **ESPipeline 动态控制**：ES 客户端支持运行时启动/停止
8. **Webhook 多客户端**：每个 Webhook 独立管理，独立的限流器、模板、@配置
9. **ES size 参数**：每次查询最大日志数通过配置控制，不在查询体中硬编码

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
