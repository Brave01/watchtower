# ===== 构建阶段：后端 =====
FROM golang:1.25-alpine AS backend-builder
WORKDIR /build

# 依赖缓存层
COPY watchtower-backend/go.mod watchtower-backend/go.sum ./
RUN go mod download

# 复制源码并编译
COPY watchtower-backend/ ./
RUN go build -ldflags="-s -w" -o watchtower ./cmd/server/

# ===== 运行阶段 =====
FROM alpine:3.19
RUN apk add --no-cache ca-certificates iputils
WORKDIR /app
COPY --from=backend-builder /build/watchtower .
COPY --from=backend-builder /build/configs ./configs
EXPOSE 3972
VOLUME ["/app/data"]
CMD ["./watchtower"]
