# 多阶段构建 Dockerfile for TurnsAPI
# 第一阶段：构建阶段
FROM golang:1.23-alpine AS builder

# 设置工作目录
WORKDIR /app

# 安装必要的工具和SQLite开发库
RUN apk add --no-cache git ca-certificates tzdata gcc musl-dev sqlite-dev

# 复制 go mod 文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 设置编译环境变量
ENV CGO_ENABLED=1
ENV GOOS=linux
ENV CGO_CFLAGS="-D_LARGEFILE64_SOURCE"

# 构建应用 (启用 CGO 以支持 SQLite)
RUN go build -a -ldflags '-extldflags "-static"' -o turnsapi ./cmd/turnsapi

# 第二阶段：运行阶段
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata sqlite-dev
ENV TZ=Asia/Shanghai

RUN addgroup -g 1001 -S turnsapi && \
    adduser -u 1001 -S turnsapi -G turnsapi

WORKDIR /app

# 1) 先把默认配置和静态资源复制进来
COPY --from=builder /app/turnsapi .
COPY --chown=turnsapi:turnsapi config/config.example.yaml ./config/
COPY --chown=turnsapi:turnsapi web/ ./web/

# 2) 再创建目录并复制默认配置
RUN mkdir -p config logs data web/static web/templates && \
    cp config/config.example.yaml config/config.yaml && \
    chown -R turnsapi:turnsapi /app

ENV GIN_MODE=release
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

CMD ["./turnsapi", "-config", "config/config.yaml"]

# 复制静态资源
COPY --chown=turnsapi:turnsapi config/config.example.yaml ./config/
COPY --chown=turnsapi:turnsapi web/ ./web/

ENV GIN_MODE=release
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

CMD ["./turnsapi", "-config", "config/config.yaml"]

# 设置生产环境变量
ENV GIN_MODE=release

# 暴露端口
EXPOSE 8080

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# 启动命令
# 注意：生产环境请确保挂载正确的config.yaml文件，并设置mode为release
CMD ["./turnsapi", "-config", "config/config.yaml"]
