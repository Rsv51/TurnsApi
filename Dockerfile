# 多阶段构建 Dockerfile for TurnsAPI
# 第一阶段：构建阶段
FROM golang:1.21-alpine AS builder

# 设置工作目录
WORKDIR /app

# 安装必要的工具
RUN apk add --no-cache git ca-certificates tzdata

# 复制 go mod 文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o turnsapi ./cmd/turnsapi

# 第二阶段：运行阶段
FROM alpine:latest

# 安装必要的运行时依赖
RUN apk --no-cache add ca-certificates tzdata

# 设置时区
ENV TZ=Asia/Shanghai

# 创建非root用户
RUN addgroup -g 1001 -S turnsapi && \
    adduser -u 1001 -S turnsapi -G turnsapi

# 设置工作目录
WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /app/turnsapi .

# 创建必要的目录
RUN mkdir -p config logs web/static web/templates && \
    chown -R turnsapi:turnsapi /app

# 复制配置文件和静态资源
COPY --chown=turnsapi:turnsapi config/config.example.yaml ./config/
COPY --chown=turnsapi:turnsapi web/ ./web/

# 暴露端口
EXPOSE 8080

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# 启动命令
CMD ["./turnsapi", "-config", "config/config.yaml"]
