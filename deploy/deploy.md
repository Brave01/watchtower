# Watchtower (瞭望塔) 部署文档

## 目录

1. [二进制本地部署](#1-二进制本地部署)
2. [Docker 部署](#2-docker-部署)
3. [Kubernetes 部署](#3-kubernetes-部署)
4. [配置说明](#4-配置说明)
5. [常见问题](#5-常见问题)

---

## 1. 二进制本地部署

### 1.1 前置条件

- Go 1.21+
- Node.js 18+（仅构建前端时需要）
- 目标系统：Linux（推荐）或 macOS

### 1.2 编译构建

```bash
# 克隆代码
git clone <repo>
cd watchtower

# 构建前端（如无需修改前端代码可跳过）
cd frontend
npm install
npm run build
cd ..

# 编译 Go 二进制
go build -ldflags="-s -w" -o watchtower .

# 交叉编译（部署到 Linux 服务器）
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o watchtower-linux .
```

### 1.3 部署运行

```bash
# 创建配置目录
mkdir -p /opt/watchtower/configs /opt/watchtower/data

# 复制二进制
cp watchtower /opt/watchtower/

# 复制配置文件
cp configs/config.yaml /opt/watchtower/configs/
cp configs/.env /opt/watchtower/configs/   # 编辑此文件，填写正确密码

# 创建 systemd 服务
cat > /etc/systemd/system/watchtower.service << 'EOF'
[Unit]
Description=Watchtower
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/watchtower
ExecStart=/opt/watchtower/watchtower
Restart=always
RestartSec=5
User=root
EnvironmentFile=/opt/watchtower/configs/.env

[Install]
WantedBy=multi-user.target
EOF

# 启动
systemctl daemon-reload
systemctl enable watchtower
systemctl start watchtower

# 查看日志
journalctl -u watchtower -f
```

### 1.4 验证

```bash
curl http://localhost:3972/api/health
# 预期响应: {"status":"ok"}
```

浏览器访问 `http://<服务器IP>:3972`，默认账号密码 `admin` / `admin`。

---

## 2. Docker 部署

### 2.1 项目目录中已包含的文件

- `Dockerfile` — 多阶段构建镜像
- `docker-compose.yml` — 一键启动服务

### 2.2 Dockerfile

```dockerfile
# ===== 构建阶段：前端 =====
FROM node:18-alpine AS frontend-builder
WORKDIR /build/frontend
COPY frontend/ ./
RUN npm install && npm run build
# Vite outDir: ../internal/webui/static/dist → 产物在 /build/internal/webui/static/dist

# ===== 构建阶段：后端 =====
FROM golang:1.21-alpine AS backend-builder
WORKDIR /build
COPY . .
COPY --from=frontend-builder /build/internal/webui/static/dist ./internal/webui/static/dist
RUN go build -ldflags="-s -w" -o watchtower .

# ===== 运行阶段 =====
FROM alpine:3.19
RUN apk add --no-cache ca-certificates iputils
WORKDIR /app
COPY --from=backend-builder /build/watchtower .
COPY --from=backend-builder /build/configs ./configs
EXPOSE 3972
VOLUME ["/app/data"]
CMD ["./watchtower"]
```

### 2.3 docker-compose.yml

```yaml
version: '3.8'

services:
  watchtower:
    build:
      context: ..
      dockerfile: deploy/Dockerfile
    container_name: watchtower
    restart: always
    ports:
      - "3972:3972"
    volumes:
      - ./data:/app/data
      - ./configs/.env:/app/configs/.env
      - ./configs/config.yaml:/app/configs/config.yaml
    environment:
      - TZ=Asia/Shanghai
```

### 2.4 构建和启动

```bash
# 构建镜像
docker-compose build

# 启动服务
docker-compose up -d

# 查看日志
docker-compose logs -f

# 停止服务
docker-compose down

# 数据持久化在 ./data/ 目录
```

### 2.5 自定义配置

编辑 `configs/config.yaml` 和 `configs/.env` 后重新启动：

```bash
docker-compose restart
```

---

## 3. Kubernetes 部署

### 3.1 镜像构建与推送

```bash
# 构建镜像
docker build -f deploy/Dockerfile -t your-registry/watchtower:latest .

# 推送到镜像仓库
docker push your-registry/watchtower:latest
```

### 3.2 K8s 资源清单

建议所有资源放在同一个 `watchtower.yaml` 文件中，按顺序 apply。

#### 3.2.1 Namespace（可选）

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: watchtower
```

#### 3.2.2 ConfigMap（非敏感配置）

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: watchtower-config
  namespace: watchtower
data:
  config.yaml: |
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

#### 3.2.3 Secret（敏感配置）

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: watchtower-secret
  namespace: watchtower
type: Opaque
stringData:
  .env: |
    ES_PASSWORD="your-es-password"
    AUTH_JWT_SECRET="change-me-to-a-long-random-string"
    ADMIN_USER=admin
    ADMIN_PASSWORD_HASH='$2b$12$...bcrypt-hash...'
```

#### 3.2.4 PersistentVolumeClaim

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: watchtower-data
  namespace: watchtower
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
```

#### 3.2.5 Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: watchtower
  namespace: watchtower
  labels:
    app: watchtower
spec:
  replicas: 1
  selector:
    matchLabels:
      app: watchtower
  template:
    metadata:
      labels:
        app: watchtower
    spec:
      containers:
      - name: watchtower
        image: your-registry/watchtower:latest
        ports:
        - containerPort: 3972
          name: http
        volumeMounts:
        - name: config
          mountPath: /app/configs/config.yaml
          subPath: config.yaml
        - name: secret
          mountPath: /app/configs/.env
          subPath: .env
        - name: data
          mountPath: /app/data
        env:
        - name: TZ
          value: Asia/Shanghai
        resources:
          requests:
            cpu: "100m"
            memory: "128Mi"
          limits:
            cpu: "500m"
            memory: "512Mi"
      volumes:
      - name: config
        configMap:
          name: watchtower-config
      - name: secret
        secret:
          name: watchtower-secret
      - name: data
        persistentVolumeClaim:
          claimName: watchtower-data
```

#### 3.2.6 Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: watchtower
  namespace: watchtower
spec:
  selector:
    app: watchtower
  ports:
  - port: 3972
    targetPort: 3972
    name: http
  type: ClusterIP
```

#### 3.2.7 Ingress（可选）

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: watchtower
  namespace: watchtower
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
spec:
  ingressClassName: nginx
  rules:
  - host: watchtower.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: watchtower
            port:
              number: 3972
```

### 3.3 部署到集群

```bash
# 创建命名空间
kubectl create namespace watchtower

# 应用所有资源
kubectl apply -f watchtower.yaml

# 查看状态
kubectl -n watchtower get pods
kubectl -n watchtower get svc

# 查看日志
kubectl -n watchtower logs -l app=watchtower -f

# 端口转发（本地访问）
kubectl -n watchtower port-forward svc/watchtower 3972:3972
```

### 3.4 扩容/缩容

```bash
kubectl -n watchtower scale deployment/watchtower --replicas=3
```

> **注意**：由于使用 SQLite 文件数据库，多副本时需确保 PVC 的 ReadWriteOnce 模式只挂载到一个 Pod。如需高可用，建议迁移到外部数据库或使用 NFS 等共享存储。

---

## 4. 配置说明

### 4.1 config.yaml 配置项

```yaml
server:
  port: 3972                           # 服务监听端口

log_monitor:
  elasticsearch:
    address: ""                        # ES 地址（Web UI 中可覆盖）
    username: ""                       # ES 用户名
    index: ""                          # ES 索引
    interval: 10                       # 轮询间隔（秒）
    size: 100                          # 每次查询最大日志数
    query:
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
    url: ""                            # 默认飞书 Webhook URL
    max_retries: 3                     # 最大重试次数

dashboard:
  probe_interval: "15s"                # 探针调度间隔

auth:
  admin_user: "admin"                  # 管理员用户名

store_cfg:
  driver: sqlite
  path: "./data/server.db"             # SQLite 数据库路径
```

### 4.2 .env 环境变量

```env
ES_PASSWORD="your-es-password"         # ES 密码
AUTH_JWT_SECRET="change-me"            # JWT 签名密钥（必改）
ADMIN_USER=admin                       # 管理员用户名
ADMIN_PASSWORD_HASH=''                 # 管理员密码 bcrypt 哈希（留空自动生成）
```

### 4.3 环境变量加载优先级

1. `.env` 文件（configs/.env）
2. 系统环境变量 / Docker/K8s 环境变量
3. 默认值（仅开发使用）

---

## 5. 常见问题

### 5.1 ICMP 探测需要 root 权限

部分系统上 `ping` 命令需要 root 权限。解决方案：

```bash
# 设置 ping 的 suid 位
chmod u+s /bin/ping

# 或使用 Docker 时添加 CAP_NET_RAW
# docker run --cap-add=NET_RAW ...
```

### 5.2 提示无法 ping：socket: Operation not permitted

在 Docker 容器中运行时，ping 需要 `CAP_NET_RAW` 和 `CAP_NET_ADMIN` 权限：

```yaml
# docker-compose.yml 中
services:
  watchtower:
    build:
      context: ..
      dockerfile: deploy/Dockerfile
    cap_add:
      - NET_RAW
      - NET_ADMIN
```

```yaml
# K8s Deployment 中
securityContext:
  capabilities:
    add: ["NET_RAW", "NET_ADMIN"]
```

### 5.3 SQLite 数据库文件权限

确保运行 Watchtower 的用户对 `data/` 目录及其文件有读写权限：

```bash
chown -R <user>:<group> /opt/watchtower/data/
chmod 755 /opt/watchtower/data/
```

### 5.4 修改前端后需要重新构建

```bash
cd frontend
npm run build
cd ..
go build -o watchtower .
```

前端构建产物通过 `//go:embed` 嵌入 Go 二进制中，修改前端后必须重新编译 Go 程序才能生效。

### 5.5 K8s 存储注意事项

- `ReadWriteOnce` 的 PVC 不支持跨节点挂载同一副本
- 如需多副本部署，建议使用 NFS、CephFS 等支持 `ReadWriteMany` 的存储
- 或考虑将 SQLite 迁移到外部数据库（当前版本暂不支持）

### 5.6 首次登录后请修改默认密码
