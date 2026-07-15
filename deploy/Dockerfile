# ===== 构建阶段：前端 =====
FROM node:18-alpine AS frontend-builder
ENV NPM_CONFIG_REGISTRY=https://registry.npmmirror.com
WORKDIR /build/frontend
COPY frontend/ ./
RUN npm install && npm run build
# Vite outDir: ../internal/webui/static/dist → 产物在 /build/internal/webui/static/dist

# ===== 构建阶段：后端 =====
FROM golang:1.25-alpine AS backend-builder
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /build/internal/webui/static/dist ./internal/webui/static/dist
RUN go build -ldflags="-s -w" -o watchtower .

# ===== 运行阶段 =====
FROM alpine:3.19
RUN apk add --no-cache ca-certificates iputils tzdata && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=backend-builder /build/watchtower .
COPY --from=backend-builder /build/configs ./configs
COPY --from=backend-builder /build/internal/webui/static/dist ./internal/webui/static/dist
VOLUME ["/app/data"]
CMD ["./watchtower"]