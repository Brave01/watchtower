# 安全加固记录

本文件记录了 Watchtower / 瞭望塔项目所做的安全审查和加固工作。

---

## 加固清单

| # | 项目 | 严重程度 | 状态 | 版本 |
|---|------|---------|------|------|
| 1 | CORS 白名单校验 | 高危 | 已完成 | - |
| 2 | Nginx 安全响应头 | 高危 | 已完成 | - |
| 3 | SSH 凭据加密存储 | 高危 | 已完成 | - |
| 4 | 登录接口 IP 频率限制 | 中危 | 已完成 | - |
| 5 | SSH HostKey 校验可配置 | 中危 | 已完成 | - |

HTTPS 配置暂未处理，需配合域名证书和反向代理使用。

---

## 1. CORS 白名单校验

**风险描述**: 旧版 CORS 中间件无条件信任所有 Origin（`Access-Control-Allow-Origin: *`），任何第三方网站均可跨域读取 API 响应。

**修复方案**:

- CORS 中间件接受可选 `allowedOrigins` 参数
- 非空时校验 Origin 头是否在白名单内，仅匹配的 Origin 被允许
- 空列表时保持向后兼容（允许所有来源，适用于开发环境）
- 配置项 `server.cors_origins` 使用逗号分隔的 Origin 白名单

**涉及文件**:

- [internal/middleware/cors.go](file:///Users/tangran/server_controller_manager/watchtower-backend/internal/middleware/cors.go)
- [internal/core/config.go](file:///Users/tangran/server_controller_manager/watchtower-backend/internal/core/config.go)
- [cmd/server/main.go](file:///Users/tangran/server_controller_manager/watchtower-backend/cmd/server/main.go)

---

## 2. Nginx 安全响应头

**风险描述**: Nginx 未设置安全响应头，存在 MIME 嗅探、点击劫持、XSS 等攻击风险。

**修复方案**: 在 `server` 块中添加以下响应头：

| 响应头 | 值 | 作用 |
|--------|---|------|
| `X-Content-Type-Options` | `nosniff` | 防止浏览器 MIME 类型嗅探 |
| `X-Frame-Options` | `DENY` | 防止点击劫持，禁止页面被嵌入 iframe |
| `X-XSS-Protection` | `1; mode=block` | 启用浏览器 XSS 过滤器 |
| `Referrer-Policy` | `no-referrer-when-downgrade` | HTTPS→HTTP 时不发送 Referrer |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=()` | 限制敏感 API 权限 |

**涉及文件**:

- [deploy/nginx.conf](file:///Users/tangran/server_controller_manager/deploy/nginx.conf)

---

## 3. SSH 凭据 AES-256-GCM 加密存储

**风险描述**: SSH 凭据（密码/私钥）以明文存储在 SQLite 数据库中，数据库文件泄露将导致所有服务器凭据暴露。

**修复方案**:

- 新增 `internal/crypto` 包，提供基于 AES-256-GCM 的加解密
- 存储 SSH 凭据时自动加密 `password` 和 `private_key` 字段，读取时自动解密
- 密钥从环境变量 `SSH_CRED_KEY` 读取，通过 SHA-256 派生 32 字节密钥
- 启动时自动检测并迁移已有明文凭据（INSERT OR REPLACE，向后兼容）
- 未配置 `SSH_CRED_KEY` 时加解密功能自动跳过，不影响现有功能

**加密流程**:

```
明文 → AES-256-GCM 加密（随机 nonce）→ hex 编码 → 存储
读取 → hex 解码 → AES-256-GCM 解密 → 明文
```

**配置方式**:

```bash
# .env 或环境变量
SSH_CRED_KEY=your-secure-encryption-key
```

**涉及文件**:

- [internal/crypto/crypto.go](file:///Users/tangran/server_controller_manager/watchtower-backend/internal/crypto/crypto.go)（新增）
- [internal/store/sqlite.go](file:///Users/tangran/server_controller_manager/watchtower-backend/internal/store/sqlite.go)
- [cmd/server/main.go](file:///Users/tangran/server_controller_manager/watchtower-backend/cmd/server/main.go)

---

## 4. 登录接口 IP 频率限制

**风险描述**: `/api/auth/login` 接口无频率限制，攻击者可进行暴力密码猜测。

**修复方案**:

- 基于 IP 的失败计数器，使用 `sync.Mutex` 保证并发安全
- 规则：**5 分钟内 5 次失败 → 锁定 5 分钟**
- 支持反向代理头 `X-Forwarded-For` / `X-Real-IP`
- 登录成功后自动清空该 IP 的失败记录
- 锁定期间返回 HTTP 429 (Too Many Requests)

**涉及文件**:

- [internal/auth/handlers.go](file:///Users/tangran/server_controller_manager/watchtower-backend/internal/auth/handlers.go)

---

## 5. SSH HostKey 校验可配置

**风险描述**: 所有 SSH 连接均使用 `ssh.InsecureIgnoreHostKey()` 跳过主机密钥校验，存在中间人攻击风险。

**修复方案**:

- 新增 `internal/dashboard/hostkey.go`，抽取统一的主机密钥校验回调变量
- 配置项 `ssh.host_key_check`（默认 `false`，保持向后兼容）
- `true` 时启用基本校验（检查密钥不为空），生产环境可在此处接入 known_hosts 验证
- 所有 SSH 连接点统一使用该配置：采集（collector）、探测（prober）、WebSocket 终端（ssh_ws）

**配置方式**:

```yaml
# config.yaml
ssh:
  host_key_check: false   # true=启用，false=跳过
```

**涉及文件**:

- [internal/dashboard/hostkey.go](file:///Users/tangran/server_controller_manager/watchtower-backend/internal/dashboard/hostkey.go)（新增）
- [internal/core/config.go](file:///Users/tangran/server_controller_manager/watchtower-backend/internal/core/config.go)
- [configs/config.yaml](file:///Users/tangran/server_controller_manager/watchtower-backend/configs/config.yaml)
- [internal/dashboard/collector.go](file:///Users/tangran/server_controller_manager/watchtower-backend/internal/dashboard/collector.go)
- [internal/dashboard/prober.go](file:///Users/tangran/server_controller_manager/watchtower-backend/internal/dashboard/prober.go)
- [internal/dashboard/ssh_ws.go](file:///Users/tangran/server_controller_manager/watchtower-backend/internal/dashboard/ssh_ws.go)
- [cmd/server/main.go](file:///Users/tangran/server_controller_manager/watchtower-backend/cmd/server/main.go)

---

## 补充建议（未实现）

以下项目建议在后续版本中逐步实施：

1. **HTTPS 支持** — 配置 SSL 证书，全站 HTTPS
2. **会话超时缩短** — 当前 JWT 有效期 24h，可缩短至 2-4h，配合 refresh token
3. **审计日志** — 记录敏感操作（登录、凭据修改、主机删除）的日志
4. **CSRF Token** — 由于当前使用 Cookie 认证 + SameSite=Lax，CSRF 风险较低但可进一步加固
5. **数据库加密** — 若数据库文件仍可能泄露，可考虑 SQLite 全库加密（如 sqlcipher）
