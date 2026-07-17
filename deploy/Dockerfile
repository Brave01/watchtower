# ===== 构建阶段：前端 =====
FROM node:18-alpine AS frontend-builder
ENV NPM_CONFIG_REGISTRY=https://registry.npmmirror.com
# ws 端口，默认 为空，开发环境走 Vite 代理口，生产环境配置为前端暴露端口
ENV VITE_WS_PORT=3972
WORKDIR /build
COPY watchtower-frontend/ ./watchtower-frontend
RUN cd watchtower-frontend && npm install && npm run build

# ===== 构建阶段：后端 =====
FROM golang:1.25-alpine AS backend-builder
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /build
COPY watchtower-backend/go.mod watchtower-backend/go.sum ./
RUN go mod download
COPY watchtower-backend/ .
RUN go build -ldflags="-s -w" -o watchtower ./cmd/server

# ===== 运行阶段：前端 Nginx =====
FROM nginx:alpine AS frontend-server
COPY --from=frontend-builder /build/watchtower-frontend/dist /usr/share/nginx/html
COPY deploy/nginx.conf /etc/nginx/conf.d/default.conf

# ===== 运行阶段：后端 =====
FROM alpine:3.19 AS backend-server
RUN apk add --no-cache ca-certificates iputils tzdata && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=backend-builder /build/watchtower .
COPY --from=backend-builder /build/configs ./configs
VOLUME ["/app/data"]
CMD ["./watchtower"]