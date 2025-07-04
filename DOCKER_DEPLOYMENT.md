# TurnsAPI Docker 部署指南

本文档详细介绍如何使用 Docker 部署 TurnsAPI 服务。

## 📋 前置要求

### 必需软件
- **Docker**: 版本 20.10 或更高
- **Docker Compose**: 版本 1.29 或更高（可选，推荐）

### 系统要求
- **内存**: 至少 512MB RAM
- **存储**: 至少 1GB 可用空间
- **网络**: 能够访问 OpenRouter API

## 🚀 快速开始

### 1. 准备配置文件

```bash
# 复制配置模板
cp config/config.example.yaml config/config.yaml

# 编辑配置文件（重要！）
nano config/config.yaml  # Linux/macOS
# 或
notepad config/config.yaml  # Windows
```

**⚠️ 重要**: 必须将配置文件中的示例 API 密钥替换为您的真实 OpenRouter API 密钥。

### 2. 选择部署方式

#### 方式一：Docker Compose（推荐）

```bash
# 构建并启动服务
docker-compose up -d

# 查看服务状态
docker-compose ps

# 查看日志
docker-compose logs -f turnsapi
```

#### 方式二：纯 Docker

```bash
# 构建镜像
docker build -t turnsapi .

# 运行容器
docker run -d \
  --name turnsapi \
  -p 8080:8080 \
  -v $(pwd)/config/config.yaml:/app/config/config.yaml:ro \
  -v $(pwd)/logs:/app/logs \
  --restart unless-stopped \
  turnsapi
```

### 3. 验证部署

```bash
# 检查服务健康状态
curl http://localhost:8080/health

# 访问管理界面
# 浏览器打开: http://localhost:8080/admin
```

## 🛠️ 使用部署脚本

项目提供了自动化部署脚本，简化部署过程：

### Linux/macOS

```bash
# 赋予执行权限
chmod +x deploy.sh

# 使用 Docker Compose 部署
./deploy.sh compose

# 使用 Docker 部署
./deploy.sh docker

# 查看服务状态
./deploy.sh status

# 查看日志
./deploy.sh logs

# 停止服务
./deploy.sh stop

# 重启服务
./deploy.sh restart
```

### Windows PowerShell

```powershell
# 使用 Docker Compose 部署
.\deploy.ps1 compose

# 使用 Docker 部署
.\deploy.ps1 docker

# 查看服务状态
.\deploy.ps1 status

# 查看日志
.\deploy.ps1 logs

# 停止服务
.\deploy.ps1 stop

# 重启服务
.\deploy.ps1 restart
```

## 📁 文件结构说明

```
TurnsApi/
├── Dockerfile              # Docker 镜像构建文件
├── docker-compose.yml      # Docker Compose 配置
├── .dockerignore           # Docker 构建忽略文件
├── deploy.sh              # Linux/macOS 部署脚本
├── deploy.ps1             # Windows 部署脚本
├── config/
│   ├── config.example.yaml # 配置模板
│   └── config.yaml         # 实际配置（需要创建）
├── logs/                   # 日志目录（自动创建）
└── web/                    # 静态文件目录
```

## 🔧 配置说明

### Docker Compose 配置

`docker-compose.yml` 文件包含以下配置：

- **端口映射**: 8080:8080
- **卷挂载**: 
  - 配置文件（只读）
  - 日志目录（读写）
  - 静态文件目录（只读）
- **健康检查**: 自动检测服务状态
- **重启策略**: 除非手动停止，否则自动重启

### Dockerfile 特性

- **多阶段构建**: 优化镜像大小
- **非 root 用户**: 提高安全性
- **健康检查**: 内置健康检查机制
- **时区设置**: 默认使用 Asia/Shanghai

## 🔍 监控和维护

### 查看服务状态

```bash
# Docker Compose
docker-compose ps

# Docker
docker ps | grep turnsapi

# 详细状态
docker inspect turnsapi
```

### 查看日志

```bash
# 实时日志
docker-compose logs -f turnsapi

# 最近日志
docker-compose logs --tail=100 turnsapi

# 本地日志文件
tail -f logs/turnsapi.log
```

### 健康检查

```bash
# 手动健康检查
curl http://localhost:8080/health

# Docker 健康检查状态
docker inspect turnsapi | grep -A 10 Health
```

### 资源使用情况

```bash
# 查看容器资源使用
docker stats turnsapi

# 查看镜像大小
docker images | grep turnsapi
```

## 🔄 更新和升级

### 更新服务

```bash
# 停止服务
docker-compose down

# 拉取最新代码（如果有）
git pull

# 重新构建并启动
docker-compose up -d --build
```

### 备份和恢复

```bash
# 备份配置文件
cp config/config.yaml config/config.yaml.backup

# 备份日志
tar -czf logs-backup-$(date +%Y%m%d).tar.gz logs/

# 导出 Docker 镜像
docker save turnsapi:latest | gzip > turnsapi-image.tar.gz

# 导入 Docker 镜像
docker load < turnsapi-image.tar.gz
```

## 🚨 故障排除

### 常见问题

1. **容器无法启动**
   ```bash
   # 查看详细错误
   docker-compose logs turnsapi
   
   # 检查配置文件
   docker-compose config
   ```

2. **端口被占用**
   ```bash
   # 查找占用进程
   netstat -tlnp | grep :8080
   
   # 修改端口（在 docker-compose.yml 中）
   ports:
     - "8081:8080"  # 改为 8081
   ```

3. **配置文件挂载失败**
   ```bash
   # 检查文件路径
   ls -la config/config.yaml
   
   # 检查文件权限
   chmod 644 config/config.yaml
   ```

4. **健康检查失败**
   ```bash
   # 进入容器检查
   docker exec -it turnsapi sh
   
   # 手动测试健康检查
   wget --spider http://localhost:8080/health
   ```

### 性能优化

1. **限制容器资源**
   ```yaml
   # 在 docker-compose.yml 中添加
   deploy:
     resources:
       limits:
         memory: 512M
         cpus: '0.5'
   ```

2. **优化日志配置**
   ```yaml
   # 限制日志大小
   logging:
     driver: "json-file"
     options:
       max-size: "10m"
       max-file: "3"
   ```

## 🔒 安全建议

1. **使用非 root 用户**: Dockerfile 已配置
2. **限制网络访问**: 使用防火墙或反向代理
3. **定期更新镜像**: 保持基础镜像最新
4. **监控日志**: 定期检查异常访问
5. **备份配置**: 定期备份重要配置文件

## 📞 获取帮助

如果遇到问题：

1. 查看容器日志: `docker-compose logs turnsapi`
2. 检查配置文件格式和内容
3. 验证 API 密钥是否有效
4. 查看 [DEPLOYMENT.md](DEPLOYMENT.md) 获取更多信息
5. 提交 Issue 到项目仓库

---

**提示**: 首次部署建议使用 Docker Compose 方式，它提供了最完整的配置和最简单的管理方式。
