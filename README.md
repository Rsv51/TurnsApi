# TurnsAPI - OpenRouter API 代理服务

TurnsAPI 是一个用 Go 语言开发的高性能 API 代理服务，专门用于转发大模型请求到 OpenRouter API。它提供了智能的 API 密钥轮询系统、流式响应支持和实时监控功能。

## 🚀 主要特性

- **智能密钥轮询**: 支持轮询、随机和最少使用三种轮询策略
- **流式响应支持**: 完全支持 Server-Sent Events (SSE) 流式响应
- **高可用性**: 自动故障转移和重试机制
- **实时监控**: Web 界面实时监控 API 密钥状态和服务性能
- **请求日志记录**: 完整记录所有API请求和响应信息，支持按密钥分类存储
- **日志分析**: 提供详细的统计分析，包括API密钥使用情况和模型调用统计
- **安全认证**: 内置用户名密码认证系统，保护 API 和管理界面
- **错误处理**: 智能错误处理和 API 密钥健康检查
- **易于配置**: 基于 YAML 的配置文件

## 📋 系统要求

- Go 1.21 或更高版本
- 有效的 OpenRouter API 密钥

## 🛠️ 安装和配置

### 1. 克隆项目

```bash
git clone <repository-url>
cd TurnsApi
```

### 2. 安装依赖

```bash
go mod tidy
```

### 3. 配置 API 密钥

编辑 `config/config.yaml` 文件，添加您的 OpenRouter API 密钥：

```yaml
api_keys:
  keys:
    - "sk-or-v1-your-real-api-key-1"
    - "sk-or-v1-your-real-api-key-2"
    - "sk-or-v1-your-real-api-key-3"
    # 添加更多密钥...
```

### 4. 构建和运行

#### 方式一：Docker 运行（推荐）

```bash
# 1. 创建必要的目录
mkdir -p config logs data

# 2. 创建配置文件
cp config/config.example.yaml config/config.yaml
# 编辑 config/config.yaml，添加您的 OpenRouter API 密钥

# 3. 使用 Docker 运行
docker run -d \
  --name turnsapi \
  -p 8080:8080 \
  -v $(pwd)/config:/app/config \
  -v $(pwd)/logs:/app/logs \
  -v $(pwd)/data:/app/data \
  bradleylzh/turnsapi:latest

# 4. 查看运行状态
docker ps
docker logs turnsapi
```

#### 方式二：本地构建运行

```bash
# 快速构建和测试
chmod +x build_and_test.sh
./build_and_test.sh

# 或者手动构建
CGO_ENABLED=1 go build -o turnsapi cmd/turnsapi/main.go

# 运行
./turnsapi -config config/config.yaml
```

或者直接运行：

```bash
go run cmd/turnsapi/main.go -config config/config.yaml
```

### 5. 验证安装

访问 http://localhost:8080 确认服务正常运行，然后访问 http://localhost:8080/logs 查看日志记录功能。

## 🔧 配置说明

### 服务器配置

```yaml
server:
  port: 8080        # 服务端口
  host: "0.0.0.0"   # 监听地址
```

### 认证配置

```yaml
auth:
  enabled: true                 # 是否启用认证
  username: "admin"             # 管理员用户名
  password: "turnsapi123"       # 管理员密码（请修改）
  session_timeout: "24h"        # 会话超时时间
```

### OpenRouter 配置

```yaml
openrouter:
  base_url: "https://openrouter.ai/api/v1"  # OpenRouter API 基础 URL
  timeout: 30s                              # 请求超时时间
  max_retries: 3                            # 最大重试次数
```

### API 密钥配置

```yaml
api_keys:
  keys:
    - "your-api-key-1"
    - "your-api-key-2"
  rotation_strategy: "round_robin"    # 轮询策略: round_robin, random, least_used
  health_check_interval: 60s          # 健康检查间隔
```

### 日志配置

```yaml
logging:
  level: "info"           # 日志级别: debug, info, warn, error
  file: "logs/turnsapi.log"
  max_size: 100           # 日志文件最大大小 (MB)
  max_backups: 3          # 保留的日志文件数量
  max_age: 28             # 日志文件保留天数
```

### 数据库配置

```yaml
database:
  path: "data/turnsapi.db"    # SQLite数据库文件路径
  retention_days: 30          # 请求日志保留天数
```

## 📡 API 使用

### 认证

如果启用了认证，需要先登录获取访问令牌：

```bash
# 登录获取令牌
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "turnsapi123"
  }'
```

响应示例：
```json
{
  "success": true,
  "token": "your-access-token",
  "expires": "2024-01-02T12:00:00Z"
}
```

### 聊天完成 API

**端点**: `POST /api/v1/chat/completions`

**请求示例**:

```bash
curl -X POST http://localhost:8080/api/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-access-token" \
  -d '{
    "model": "minimax/minimax-m1",
    "messages": [
      {
        "role": "user",
        "content": "Hello, how are you?"
      }
    ],
    "stream": false
  }'
```

**流式请求示例**:

```bash
curl -X POST http://localhost:8080/api/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-access-token" \
  -d '{
    "model": "minimax/minimax-m1",
    "messages": [
      {
        "role": "user",
        "content": "Tell me a story"
      }
    ],
    "stream": true
  }'
```

### 支持的参数

| 参数 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `model` | string | 是 | 模型名称，如 `minimax/minimax-m1` |
| `messages` | array | 是 | 对话消息数组 |
| `stream` | boolean | 否 | 是否启用流式响应 |
| `temperature` | number | 否 | 温度参数 (0-2) |
| `max_tokens` | integer | 否 | 最大生成 token 数 |
| `top_p` | number | 否 | Top-p 采样参数 |
| `stop` | string/array | 否 | 停止词 |

## 🖥️ Web 界面

### 访问地址

- **登录页面**: http://localhost:8080/auth/login
- **首页**: http://localhost:8080/ （需要登录）
- **仪表板**: http://localhost:8080/dashboard （需要登录）
- **请求日志**: http://localhost:8080/logs （需要登录）
- **API 状态**: http://localhost:8080/admin/status （需要认证）
- **密钥状态**: http://localhost:8080/admin/keys （需要认证）

### 功能特性

- 实时显示 API 密钥状态
- 服务性能监控
- 使用统计和错误统计
- **请求日志查看**: 详细的API请求和响应日志记录
- **统计分析**: API密钥使用统计和模型调用分析
- 自动刷新功能

## 🔍 监控和管理

### 健康检查

```bash
curl http://localhost:8080/health
```

### 服务状态

```bash
curl http://localhost:8080/admin/status
```

### 密钥状态

```bash
curl http://localhost:8080/admin/keys
```

### 请求日志查询

```bash
# 获取所有请求日志
curl http://localhost:8080/admin/logs

# 按API密钥筛选日志
curl "http://localhost:8080/admin/logs?api_key=sk-or****1234"

# 分页查询日志
curl "http://localhost:8080/admin/logs?limit=20&offset=0"

# 获取日志详情
curl http://localhost:8080/admin/logs/123

# 获取API密钥统计
curl http://localhost:8080/admin/logs/stats/api-keys

# 获取模型使用统计
curl http://localhost:8080/admin/logs/stats/models
```

## � Docker 使用说明

### Docker 命令详解

```bash
# 基本运行命令
docker run -d \
  --name turnsapi \                    # 容器名称
  -p 8080:8080 \                      # 端口映射 (主机:容器)
  -v $(pwd)/config:/app/config \      # 配置文件挂载
  -v $(pwd)/logs:/app/logs \          # 日志目录挂载
  -v $(pwd)/data:/app/data \          # 数据库目录挂载
  bradleylzh/turnsapi:latest          # 镜像地址

# 查看容器状态
docker ps

# 查看容器日志
docker logs turnsapi

# 实时查看日志
docker logs -f turnsapi

# 停止容器
docker stop turnsapi

# 重启容器
docker restart turnsapi

# 删除容器
docker rm turnsapi

# 更新到最新版本
docker pull bradleylzh/turnsapi:latest
docker stop turnsapi
docker rm turnsapi
# 然后重新运行上面的 docker run 命令
```

### Docker Compose 部署

创建 `docker-compose.yml` 文件：

```yaml
version: '3.8'

services:
  turnsapi:
    image: bradleylzh/turnsapi:latest
    container_name: turnsapi
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./config:/app/config
      - ./logs:/app/logs
      - ./data:/app/data
    environment:
      - TZ=Asia/Shanghai
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
```

使用 Docker Compose：

```bash
# 启动服务
docker-compose up -d

# 查看状态
docker-compose ps

# 查看日志
docker-compose logs -f

# 停止服务
docker-compose down
```

### 数据持久化

Docker 运行时会自动创建以下目录映射：

- `./config` → `/app/config` (配置文件)
- `./logs` → `/app/logs` (应用日志)
- `./data` → `/app/data` (SQLite数据库)

确保这些目录存在并有适当的权限：

```bash
mkdir -p config logs data
chmod 755 config logs data
```

## 🚨 故障排除

### 常见问题

1. **服务启动失败**
   - 检查配置文件格式是否正确
   - 确保端口未被占用
   - 验证 API 密钥格式

2. **Docker 相关问题**
   - 确保 Docker 已正确安装并运行
   - 检查端口 8080 是否被占用：`netstat -tlnp | grep 8080`
   - 验证目录挂载权限：`ls -la config logs data`
   - 查看容器日志：`docker logs turnsapi`

3. **API 请求失败**
   - 检查 API 密钥是否有效
   - 确认网络连接正常
   - 查看日志文件获取详细错误信息

4. **流式响应异常**
   - 确保客户端支持 Server-Sent Events
   - 检查防火墙和代理设置

5. **数据库问题**
   - 确保 `data` 目录有写入权限
   - 检查 SQLite 数据库文件是否正常创建
   - 查看应用日志中的数据库相关错误

### 日志查看

```bash
# 查看实时日志
tail -f logs/turnsapi.log

# 查看错误日志
grep "ERROR" logs/turnsapi.log
```

## 🔒 安全注意事项

### 生产环境部署

1. **修改默认密码**: 请务必修改配置文件中的默认用户名和密码
2. **使用强密码**: 建议使用包含大小写字母、数字和特殊字符的强密码
3. **启用 HTTPS**: 在生产环境中建议使用反向代理（如 Nginx）启用 HTTPS
4. **定期更新密码**: 建议定期更新管理员密码
5. **网络安全**: 限制管理界面的访问 IP 范围

### 认证配置示例

```yaml
auth:
  enabled: true
  username: "your-admin-username"
  password: "your-strong-password-123!"
  session_timeout: "8h"
```

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

MIT License

## 📞 支持

如有问题，请提交 Issue 或联系维护者。
