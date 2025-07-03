# TurnsAPI 部署指南

本指南将帮助您快速部署和配置 TurnsAPI 服务。

## 🚀 快速开始

### 1. 准备工作

确保您已经：
- 安装了 Go 1.21 或更高版本
- 获得了有效的 OpenRouter API 密钥
- 下载或克隆了 TurnsAPI 项目

### 2. 配置 API 密钥

```bash
# 复制示例配置文件
cp config/config.example.yaml config/config.yaml

# 编辑配置文件，添加您的 API 密钥
notepad config/config.yaml  # Windows
# 或
nano config/config.yaml     # Linux/macOS
```

在配置文件中，将示例密钥替换为您的真实密钥：

```yaml
api_keys:
  keys:
    - "sk-or-v1-your-real-api-key-1"
    - "sk-or-v1-your-real-api-key-2"
    # 添加更多密钥...
```

### 3. 构建和运行

#### Windows

```powershell
# 使用启动脚本（推荐）
.\start.ps1

# 或手动构建和运行
go build -o turnsapi.exe cmd/turnsapi/main.go
.\turnsapi.exe -config config/config.yaml
```

#### Linux/macOS

```bash
# 构建
go build -o turnsapi cmd/turnsapi/main.go

# 运行
./turnsapi -config config/config.yaml
```

### 4. 验证部署

```bash
# 检查服务健康状态
curl http://localhost:8080/health

# 访问 Web 界面
# 浏览器打开: http://localhost:8080
```

## 🔧 高级配置

### 端口配置

如果需要更改服务端口，编辑 `config/config.yaml`：

```yaml
server:
  port: "8080"    # 更改为您需要的端口
  host: "0.0.0.0" # 监听所有网卡
```

### 性能调优

```yaml
openrouter:
  timeout: 30s      # 根据网络情况调整超时时间
  max_retries: 3    # 根据需要调整重试次数

api_keys:
  rotation_strategy: "round_robin"  # 推荐使用轮询策略
  health_check_interval: 60s        # 健康检查间隔
```

### 日志配置

```yaml
logging:
  level: "info"                 # 生产环境建议使用 info 或 warn
  file: "logs/turnsapi.log"     # 日志文件路径
  max_size: 100                 # 日志文件大小限制
  max_backups: 3                # 保留的日志文件数量
  max_age: 28                   # 日志保留天数
```

## 🐳 Docker 部署

### 创建 Dockerfile

```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o turnsapi cmd/turnsapi/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/turnsapi .
COPY --from=builder /app/config ./config
COPY --from=builder /app/web ./web

EXPOSE 8080
CMD ["./turnsapi", "-config", "config/config.yaml"]
```

### 构建和运行 Docker 容器

```bash
# 构建镜像
docker build -t turnsapi .

# 运行容器
docker run -d \
  --name turnsapi \
  -p 8080:8080 \
  -v $(pwd)/config:/root/config \
  -v $(pwd)/logs:/root/logs \
  turnsapi
```

## 🌐 生产环境部署

### 使用 systemd（Linux）

创建服务文件 `/etc/systemd/system/turnsapi.service`：

```ini
[Unit]
Description=TurnsAPI Service
After=network.target

[Service]
Type=simple
User=turnsapi
WorkingDirectory=/opt/turnsapi
ExecStart=/opt/turnsapi/turnsapi -config /opt/turnsapi/config/config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

启动服务：

```bash
sudo systemctl daemon-reload
sudo systemctl enable turnsapi
sudo systemctl start turnsapi
sudo systemctl status turnsapi
```

### 使用 Nginx 反向代理

Nginx 配置示例：

```nginx
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # 支持流式响应
        proxy_buffering off;
        proxy_cache off;
    }
}
```

### 使用 PM2（Node.js 环境）

虽然 TurnsAPI 是 Go 程序，但可以使用 PM2 管理：

```bash
# 安装 PM2
npm install -g pm2

# 创建 ecosystem.config.js
cat > ecosystem.config.js << EOF
module.exports = {
  apps: [{
    name: 'turnsapi',
    script: './turnsapi',
    args: '-config config/config.yaml',
    instances: 1,
    autorestart: true,
    watch: false,
    max_memory_restart: '1G',
    env: {
      NODE_ENV: 'production'
    }
  }]
}
EOF

# 启动服务
pm2 start ecosystem.config.js
pm2 save
pm2 startup
```

## 🔒 安全配置

### 1. 防火墙配置

```bash
# Ubuntu/Debian
sudo ufw allow 8080/tcp

# CentOS/RHEL
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --reload
```

### 2. API 密钥安全

- 不要在代码中硬编码 API 密钥
- 使用环境变量或安全的配置管理
- 定期轮换 API 密钥
- 监控 API 密钥使用情况

### 3. 访问控制

考虑在 Nginx 或其他反向代理中添加访问控制：

```nginx
# IP 白名单
allow 192.168.1.0/24;
allow 10.0.0.0/8;
deny all;

# 基本认证
auth_basic "Restricted Access";
auth_basic_user_file /etc/nginx/.htpasswd;
```

## 📊 监控和维护

### 1. 健康检查

设置定期健康检查：

```bash
# 创建健康检查脚本
cat > health_check.sh << EOF
#!/bin/bash
if curl -f http://localhost:8080/health > /dev/null 2>&1; then
    echo "Service is healthy"
    exit 0
else
    echo "Service is unhealthy"
    exit 1
fi
EOF

chmod +x health_check.sh

# 添加到 crontab
echo "*/5 * * * * /path/to/health_check.sh" | crontab -
```

### 2. 日志轮转

配置 logrotate：

```bash
cat > /etc/logrotate.d/turnsapi << EOF
/opt/turnsapi/logs/*.log {
    daily
    missingok
    rotate 30
    compress
    delaycompress
    notifempty
    create 644 turnsapi turnsapi
    postrotate
        systemctl reload turnsapi
    endscript
}
EOF
```

### 3. 性能监控

使用 Prometheus 和 Grafana 监控服务性能（需要添加相应的 metrics 端点）。

## 🚨 故障排除

### 常见问题

1. **端口被占用**
   ```bash
   # 查找占用端口的进程
   netstat -tulpn | grep :8080
   # 或
   lsof -i :8080
   ```

2. **权限问题**
   ```bash
   # 确保用户有执行权限
   chmod +x turnsapi
   
   # 确保配置文件可读
   chmod 644 config/config.yaml
   ```

3. **API 密钥无效**
   - 检查密钥格式是否正确
   - 验证密钥是否有效且未过期
   - 查看日志文件获取详细错误信息

### 日志分析

```bash
# 查看实时日志
tail -f logs/turnsapi.log

# 查看错误日志
grep "ERROR" logs/turnsapi.log

# 查看特定时间段的日志
grep "2024-01-01" logs/turnsapi.log
```

## 📞 获取帮助

如果遇到问题：

1. 查看日志文件获取详细错误信息
2. 检查配置文件格式是否正确
3. 验证网络连接和 API 密钥
4. 提交 Issue 到项目仓库
