# 深度合并设计文档

## 1. 目标

将当前两个独立进程（`log-monitor` 日志监控 + `dashboard` 服务拨测）**合并为一个单进程、单端口、统一存储、统一前端**的全栈应用。

| 维度 | 当前状态 | 合并后 |
|------|---------|--------|
| 进程数 | 2 个独立二进制 | 1 个二进制 |
| 端口 | `:3971` + `:3972` | 统一 1 个端口 |
| 前端 | 2 套独立 SPA | 1 套统一 SPA |
| 存储 | log-monitor 用 SQLite，dashboard 用内存 | 统一存储层，支持 SQLite / MySQL |
| go.mod | 3 个 module | 1 个 module |

---

## 2. 目录结构

```
watchtower/
├── main.go                          # 单一入口
├── go.mod / go.sum                  # 单一 module
├── configs/
│   └── config.yaml                  # 合并后的配置
│
├── internal/
│   ├── store/                       # 统一存储层（新）
│   │   ├── interface.go             #   Store 接口定义
│   │   ├── sqlite.go                #   SQLite 实现
│   │   ├── mysql.go                 #   MySQL 实现
│   │   └── models.go                #   统一数据模型
│   │
│   ├── logmonitor/                  # 从 log-monitor/internal/ 迁移
│   │   ├── es/client.go             #   ES 客户端（零改动）
│   │   ├── parser/parser.go         #   日志解析器（零改动）
│   │   ├── filter/filter.go         #   过滤引擎（零改动）
│   │   ├── dedup/dedup.go           #   去重引擎（零改动）
│   │   └── webhook/feishu.go        #   飞书 Webhook（零改动）
│   │
│   ├── dashboard/                   # 从 lvneng/ 迁移
│   │   ├── types.go                 #   数据模型（零改动）
│   │   ├── prober.go                #   拨测引擎（零改动）
│   │   ├── scheduler.go             #   定时调度（仅改 store 调用）
│   │   └── ssh_ws.go                #   SSH 终端代理（零改动）
│   │
│   ├── handler/                     # 统一 API 路由注册（新）
│   │   ├── logmonitor.go            #   日志监控 API
│   │   ├── dashboard.go             #   服务拨测 API
│   │   └── auth.go                  #   认证路由
│   │
│   ├── auth/                        # 从 common/auth/ 迁移（零改动）
│   └── webui/                       # 统一前端资源（新）
│       ├── assets.go                #   go:embed
│       └── static/                  #   统一 SPA
│
└── deploy/                          # 单容器部署
```

### 各模块改动程度

| 包 | 改动 | 说明 |
|----|------|------|
| `internal/logmonitor/` | **零改动** | 完全从原项目复制 |
| `internal/dashboard/prober.go` | **零改动** | 完全从原项目复制 |
| `internal/dashboard/ssh_ws.go` | **零改动** | 完全从原项目复制 |
| `internal/dashboard/scheduler.go` | **小改** | `store.Xxx()` 替换为 `internal/store.Xxx()` |
| `internal/auth/` | **零改动** | 从 `common/auth` 复制 |
| `internal/store/` | **全新** | 需要设计实现 |
| `internal/handler/` | **重新封装** | 将原有 API handler 逻辑重新组装 |
| `main.go` | **全新** | 组装所有模块 |

---

## 3. 统一存储层设计

### 3.1 现状差异

| 维度 | log-monitor | dashboard |
|------|------------|-----------|
| 存储 | SQLite（`modernc.org/sqlite`） | 内存 Map（`sync.RWMutex`） |
| 数据 | alert_rules, webhook_config, limited_alert_logs | hosts, roles, assignments, ssh_credentials |
| 问题 | 重启不丢数据 | 重启全丢 |

### 3.2 接口定义

```go
// internal/store/interface.go
type Store interface {
    // 告警规则
    ListAlertRules() ([]AlertRule, error)
    SaveAlertRule(*AlertRule) error
    DeleteAlertRule(id string) error

    // Webhook 配置
    GetWebhookConfig() (*WebhookConfig, error)
    SaveWebhookConfig(*WebhookConfig) error

    // 限流日志
    SaveLimitedAlert(*LimitedAlert) error
    ListLimitedAlerts(limit, offset int) ([]LimitedAlert, error)
    LoadLimitedAlertsForRetry(limit int) ([]LimitedAlert, error)
    ClearLimitedAlerts() error
    DeleteOldLimitedAlerts(before time.Time) (int64, error)

    // 主机
    ListHosts() ([]Host, error)
    GetHost(id string) (*Host, error)
    AddHost(*Host) error
    UpdateHostStatus(id string, status int, checkTime time.Time) error
    UpdateHostMaintenance(id string, maintenance bool) error
    DeleteHost(id string) error

    // 角色
    ListRoles() ([]Role, error)
    GetRole(id string) (*Role, error)
    AddRole(*Role) error

    // 指派
    ListAssignments() ([]Assignment, error)
    GetAssignment(hostID, roleID string) (*Assignment, error)
    AddAssignment(*Assignment) error
    UpdateAssignmentStatus(hostID, roleID string, status, statusCode int, errMsg string, checkTime time.Time) error
    UpdateAssignmentConsecutiveFailures(hostID, roleID string, failures int) error

    // SSH 凭据
    ListSSHCredentials() ([]SSHCredential, error)
    GetSSHCredential(id string) (*SSHCredential, error)
    AddSSHCredential(*SSHCredential) (string, error)
    DeleteSSHCredential(id string) error

    // 用户
    GetUser(username string) (*User, error)

    Close() error
}
```

### 3.3 实现策略

- **SQLite 实现**：继续使用 `modernc.org/sqlite`，在原有 alert_rules / webhook_config / limited_alert_logs 表基础上，新增 hosts / roles / assignments / ssh_credentials / users 表。数据库路径由 `config.yaml` 的 `store.path` 控制。
- **MySQL 实现**：使用 `github.com/go-sql-driver/mysql`，DDL 与 SQLite 保持相同表结构。连接参数由 `config.yaml` 的 `store.mysql` 分节配置。
- **dashboard 数据持久化**：原内存存储中主机状态、探测结果等也落盘，彻底解决重启丢失的问题。

---

## 4. 配置文件

```yaml
server:
  port: 8080

store:
  driver: sqlite                    # sqlite | mysql
  path: ./data/server.db            # SQLite 路径
  mysql:
    host: "localhost"
    port: 3306
    user: "scm"
    database: "server_controller"

log_monitor:
  elasticsearch:
    address: "http://your-es-server:9200"
    username: "elastic"
    index: "k8s-logs-*"
    interval: 10
    query: { ... }
  feishu_webhook:
    url: ""
    max_retries: 3
  alerts: []

dashboard:
  probe_interval: "15s"
  default_roles:
    - name: "icmp", type: "ICMP"
    - name: "redis", type: "TCP", port: 6379

auth:
  admin_user: "admin"
```

> 敏感信息（ES 密码、MySQL 密码、JWT 密钥、管理员密码哈希）通过环境变量注入，不入配置文件。

---

## 5. 统一前端

### 导航结构

```
| Server Controller Manager | [日志监控] [服务大盘] | admin ▼ | [退出] |
```

| 主 Tab | 子页面 | 来源 |
|--------|--------|------|
| **日志监控** | 实时日志 / 告警配置 / 飞书配置 / 统计 | log-monitor 4 标签 |
| **服务大盘** | 大盘概览 / 主机管理 / 角色模版 / SSH 凭据 / SSH 终端 | dashboard |

以 Vue 3 为基底，将 log-monitor 的原生 JS 页面重写为 Vue 组件。

### API 路由（无冲突）

log-monitor 路由：`/api/health`, `/api/stats`, `/api/rules*`, `/api/webhook/*`, `/ws`
dashboard 路由：`/api/dashboard`, `/api/hosts*`, `/api/roles*`, `/api/assign*`, `/api/refresh`, `/api/ssh-credential*`, `/api/ssh/ws`

两组路由路径**完全无冲突**，可直接注册在同一个 `http.ServeMux` 上。

---

## 6. main.go 组装流程

```
main()
  ├── 加载 config.yaml
  ├── 初始化 Store（SQLite 或 MySQL）
  │     ├── 建表迁移
  │     └── 写入默认角色
  ├── 初始化 log-monitor 组件
  │     ├── ES Client, Parser, Deduplicator
  │     ├── Filter Engine（加载告警规则）
  │     └── Webhook Client
  ├── 初始化 dashboard 组件
  │     ├── Scheduler（定时拨测，从 Store 读取主机/角色/指派）
  │     └── SSH Credential 管理
  ├── 注册路由（统一 mux）
  │     ├── 认证路由
  │     ├── 静态文件（统一 SPA）
  │     ├── log-monitor API
  │     └── dashboard API
  ├── 启动后台协程
  │     ├── ES 定时查询 + 限流日志重试 + 清理
  │     └── dashboard 定时拨测
  └── 启动 HTTP 服务（优雅关闭）
```

---

## 7. 依赖汇总

| 依赖 | 用途 |
|------|------|
| `github.com/gorilla/websocket` | WS 实时日志 + SSH 终端 |
| `github.com/elastic/go-elasticsearch/v8` | ES 客户端 |
| `modernc.org/sqlite` | SQLite 驱动 |
| `github.com/go-sql-driver/mysql` | 【新增】MySQL 驱动 |
| `github.com/google/uuid` | UUID 生成 |
| `golang.org/x/crypto` | bcrypt + SSH |
| `gopkg.in/yaml.v3` | 配置解析 |

---

## 8. 实施步骤

| 阶段 | 内容 | 涉及文件 |
|------|------|---------|
| 1 | 创建新 `go.mod`，搭建 `internal/store/` 接口 + SQLite 实现 | `go.mod`, `store/*.go` |
| 2 | 迁移 `log-monitor/internal/` → `internal/logmonitor/` | 复制 15 个文件 |
| 3 | 迁移 `lvneng/model/`、`viewmodel/`、`view/` → `internal/dashboard/`，scheduler 改 store 调用 | 复制 7 个文件 |
| 4 | 迁移 `common/auth/` → `internal/auth/`，`common/webui/` → `internal/webui/` | 复制 6 个文件 |
| 5 | 编写 `internal/handler/` 封装 API 路由 | 新建 3 个文件 |
| 6 | 编写 `main.go` 组装逻辑 | 1 个文件 |
| 7 | 编写统一前端 SPA | 3 个静态文件 |
| 8 | 更新部署配置 | Docker/K8s |

---

## 9. 风险点

1. **拨测高频写入压力**：原内存存储改为数据库后，定时拨测（每 15s 全量探测）的每次状态更新都产生数据库写入。建议：单次轮询内使用批量事务提交，避免逐条写入。

2. **SSH 凭据读取**：`scheduler.probeAll()` 中反复调用 `GetFirstSSHCredential()`，改为数据库后应在循环外一次性加载。

3. **前端合并**：log-monitor 前端为原生 JS，dashboard 为 Vue 3。重写为统一 Vue 组件是最大工作量部分，可后置。
