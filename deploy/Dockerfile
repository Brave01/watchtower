# ===== 构建阶段：前端 =====
FROM node:18-alpine AS frontend-builder
WORKDIR /build/frontend
COPY frontend/ ./
RUN npm install && npm run build

# ===== 构建阶段：后端 =====
FROM golang:1.21-alpine AS backend-builder
WORKDIR /build
COPY . .
COPY --from=frontend-builder /build/frontend/dist ./frontend/dist
RUN go build -ldflags="-s -w" -o scm .

# ===== 运行阶段 =====
FROM alpine:3.19
RUN apk add --no-cache ca-certificates iputils
WORKDIR /app
COPY --from=backend-builder /build/scm .
COPY --from=backend-builder /build/configs ./configs
EXPOSE 3972
VOLUME ["/app/data"]
CMD ["./scm"]