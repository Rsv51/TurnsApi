# TurnsAPI - 多提供商 API 代理服务

TurnsAPI 是一个用 Go 语言开发的高性能多提供商 API 代理服务，支持 OpenAI、Google Gemini、Anthropic Claude、Azure OpenAI 等多个大模型提供商。它提供了智能的 API 密钥轮询系统、流式响应支持和实时监控功能。

## 🚀 主要特性

- **多提供商支持**: 支持 OpenAI、Google Gemini、Anthropic Claude、Azure OpenAI 等多个提供商
- **原生接口响应**: 支持返回提供商原生响应格式，暴露Gemini `/v1/beta` 原生API端点
- **RPM限制功能**: 支持分组级别的每分钟请求数限制，防止API配额过度消耗
- **智能密钥轮询**: 支持轮询、随机和最少使用三种轮询策略
- **智能路由重试**: 自动故障转移和智能重试机制，提高请求成功率
- **模型重命名映射**: 支持为不同分组的模型设置别名，统一模型名称管理
- **JSON参数覆盖**: 支持分组级别的请求参数覆盖（temperature、max_tokens等）
- **流式响应支持**: 完全支持 Server-Sent Events (SSE) 流式响应
- **高可用性**: 自动故障转移和重试机制
- **实时监控**: Web 界面实时监控 API 密钥状态和服务性能
- **请求日志记录**: 完整记录所有API请求和响应信息，支持按密钥分类存储
- **日志分析**: 提供详细的统计分析，包括API密钥使用情况和模型调用统计
- **安全认证**: 内置用户名密码认证系统，保护 API 和管理界面
- **错误处理**: 智能错误处理和 API 密钥健康检查
- **生产就绪**: 支持 release 模式，优化生产环境性能
- **易于配置**: 基于 YAML 的配置文件和Web界面管理

## 🌐 支持的提供商

| 提供商 | 类型 | 支持的模型 |
|--------|------|------------|
| **OpenAI** | `openai` | GPT-3.5, GPT-4, GPT-4 Turbo, GPT-4o 等 |
| **Google Gemini** | `gemini` | Gemini Pro, Gemini Pro Vision, Gemini 1.5 等 |
| **Anthropic Claude** | `anthropic` | Claude 3 Haiku, Claude 3 Sonnet, Claude 3 Opus 等 |
| **Azure OpenAI** | `azure_openai` | Azure 部署的 OpenAI 模型 |
| **OpenRouter** | `openai` | 支持所有 OpenRouter 可用模型 |
| **自定义兼容服务** | `openai` | 任何 OpenAI API 兼容的服务 |

## 📋 系统要求

- Go 1.23 或更高版本
- 有效的 API 密钥（支持多个提供商）

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
  -e GIN_MODE=release \
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
  port: "8080"      # 服务端口
  host: "0.0.0.0"   # 监听地址
  mode: "release"   # 服务器模式: debug, release, test (生产环境建议使用release)
```

**服务器模式说明：**
- `debug`: 开发模式，显示详细的路由信息和调试日志
- `release`: 生产模式，隐藏调试信息，提供更好的性能
- `test`: 测试模式，用于单元测试环境

### 认证配置

```yaml
auth:
  enabled: true                 # 是否启用认证
  username: "admin"             # 管理员用户名
  password: "turnsapi123"       # 管理员密码（请修改）
  session_timeout: "24h"        # 会话超时时间
```

### 多提供商配置

```yaml
user_groups:
  # OpenAI 官方 API
  openai_official:
    name: "OpenAI Official"
    provider_type: "openai"
    base_url: "https://api.openai.com/v1"
    enabled: true
    timeout: 30s
    max_retries: 3
    rotation_strategy: "round_robin"
    api_keys:
      - "sk-your-openai-key-1"
      - "sk-your-openai-key-2"
    models:
      - "gpt-3.5-turbo"
      - "gpt-4"
      - "gpt-4o"
    headers:
      Content-Type: "application/json"
    # JSON请求参数覆盖（可选）
    request_params:
      temperature: 0.7
      max_tokens: 2000
      top_p: 0.9
    # 模型重命名映射（可选）
    model_mappings:
      gpt4: "gpt-4"                    # 将 gpt4 映射到 gpt-4
      gpt35: "gpt-3.5-turbo"          # 将 gpt35 映射到 gpt-3.5-turbo
      chatgpt: "gpt-3.5-turbo"        # 将 chatgpt 映射到 gpt-3.5-turbo

  # Google Gemini API
  google_gemini:
    name: "Google Gemini"
    provider_type: "gemini"
    base_url: "https://generativelanguage.googleapis.com/v1beta"
    enabled: true
    timeout: 30s
    max_retries: 3
    rotation_strategy: "random"
    api_keys:
      - "your-gemini-api-key"
    models:
      - "gemini-pro"
      - "gemini-1.5-pro"
      - "gemini-2.5-pro"
    headers:
      Content-Type: "application/json"
    # 启用原生接口响应格式（可选）
    use_native_response: true
    # RPM限制：每分钟最多60次请求（可选，0表示无限制）
    rpm_limit: 60

  # Anthropic Claude API
  anthropic_claude:
    name: "Anthropic Claude"
    provider_type: "anthropic"
    base_url: "https://api.anthropic.com"
    api_version: "2023-06-01"
    enabled: true
    timeout: 30s
    max_retries: 3
    rotation_strategy: "least_used"
    api_keys:
      - "your-anthropic-key"
    models:
      - "claude-3-sonnet-20240229"
      - "claude-3-opus-20240229"
    headers:
      Content-Type: "application/json"
```

**轮询策略说明：**
- `round_robin`: 轮询使用密钥
- `random`: 随机选择密钥
- `least_used`: 选择使用次数最少的密钥

**高级功能配置：**

1. **JSON请求参数覆盖** (`request_params`)：
   - 为分组设置默认的请求参数，会覆盖客户端请求中的相应参数
   - 支持的参数：`temperature`、`max_tokens`、`top_p`、`stop`
   - 用于统一不同分组的模型行为

2. **模型重命名映射** (`model_mappings`)：
   - 为分组中的模型设置别名，实现模型名称的统一管理
   - 格式：`别名: 实际模型名`
   - 客户端使用别名请求时，系统自动转换为实际模型名
   - 支持多个别名映射到同一个实际模型

3. **原生接口响应** (`use_native_response`)：
   - 启用后，API将返回提供商的原生响应格式，而不是OpenAI兼容格式
   - 对于Gemini提供商，会暴露 `/v1/beta` 端点，支持Gemini原生API调用
   - 流式响应会直接返回提供商的原始SSE数据流
   - 适用于需要使用提供商特定功能的场景

4. **RPM限制** (`rpm_limit`)：
   - 设置每分钟最大请求数限制，防止API配额过度消耗
   - 设置为0表示无限制
   - 超出限制时会返回429错误
   - 支持分组级别的独立限制

5. **智能路由重试机制**：
   - 当某个分组连续失败3次后，会被临时阻止5分钟
   - 系统自动选择失败次数最少的分组进行重试
   - 成功请求会重置该分组的失败计数
   - 确保在用户权限范围内进行智能路由

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

### 提供商选择

可以通过以下方式指定使用的提供商：

1. **通过请求头指定**：
```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-access-token" \
  -H "X-Provider-Group: openai_official" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

2. **自动路由**：系统会根据模型名称自动选择合适的提供商
3. **默认分组**：如果未指定，使用第一个启用的分组
4. **模型别名**：可以使用配置的模型别名，系统自动转换为实际模型名

### 模型重命名功能

如果在分组配置中设置了模型映射，可以使用别名来请求模型：

```bash
# 假设配置了映射：gpt4 -> gpt-4
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-access-token" \
  -d '{
    "model": "gpt4",  # 使用别名，系统自动转换为 gpt-4
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### JSON参数覆盖功能

分组级别的参数覆盖会自动应用到该分组的所有请求：

```bash
# 即使客户端设置了 temperature: 1.0，
# 如果分组配置了 temperature: 0.7，最终会使用 0.7
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-access-token" \
  -H "X-Provider-Group: openai_official" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello!"}],
    "temperature": 1.0  # 会被分组配置覆盖
  }'
```

### 聊天完成 API

**端点**: `POST /v1/chat/completions`

**请求示例**:

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-access-token" \
  -d '{
    "model": "gpt-4",
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
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-access-token" \
  -d '{
    "model": "gpt-4",
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
| `model` | string | 是 | 模型名称，如 `gpt-4`, `claude-3-sonnet-20240229` |
| `messages` | array | 是 | 对话消息数组 |
| `stream` | boolean | 否 | 是否启用流式响应 |
| `temperature` | number | 否 | 温度参数 (0-2) |
| `max_tokens` | integer | 否 | 最大生成 token 数 |
| `top_p` | number | 否 | Top-p 采样参数 |
| `stop` | string/array | 否 | 停止词 |

### Gemini 原生 API 端点

当分组配置启用了"原生接口响应"时，可以使用Gemini原生API格式进行调用：

#### 非流式请求

**端点**: `POST /v1/beta/models/{model}/generateContent`

```bash
curl -X POST http://localhost:8080/v1/beta/models/gemini-2.5-pro/generateContent \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-access-token" \
  -d '{
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "text": "Hello, how are you?"
          }
        ]
      }
    ],
    "generationConfig": {
      "temperature": 0.7,
      "maxOutputTokens": 1000,
      "topP": 0.9
    }
  }'
```

#### 流式请求

**端点**: `POST /v1/beta/models/{model}/streamGenerateContent`

```bash
curl -X POST http://localhost:8080/v1/beta/models/gemini-2.5-pro/streamGenerateContent \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-access-token" \
  -d '{
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "text": "Tell me a story"
          }
        ]
      }
    ],
    "generationConfig": {
      "temperature": 0.8,
      "maxOutputTokens": 2000
    }
  }'
```

#### Gemini 原生响应格式

```json
{
  "candidates": [
    {
      "content": {
        "parts": [
          {
            "text": "Hello! I'm doing well, thank you for asking..."
          }
        ],
        "role": "model"
      },
      "finishReason": "STOP",
      "index": 0
    }
  ],
  "usageMetadata": {
    "promptTokenCount": 10,
    "candidatesTokenCount": 25,
    "totalTokenCount": 35
  }
}
```

#### Gemini 原生模型列表

**端点**: `GET /v1/beta/models`

```bash
curl -X GET http://localhost:8080/v1/beta/models \
  -H "Authorization: Bearer your-access-token"
```

### 模型列表 API

**端点**: `GET /v1/models`

```bash
curl -X GET http://localhost:8080/v1/models \
  -H "Authorization: Bearer your-access-token"
```

可以通过查询参数指定提供商分组：
```bash
curl -X GET "http://localhost:8080/v1/models?provider_group=openai_official" \
  -H "Authorization: Bearer your-access-token"
```

## 🖥️ Web 界面

### 访问地址

http://localhost:8080

### 功能特性

- **多提供商管理**: 统一管理多个AI提供商的配置
- **实时监控**: 显示各提供商的API密钥状态和健康状况
- **分组管理**: 创建、编辑、删除提供商分组
- **密钥验证**: 实时验证API密钥的有效性
- **模型测试**: 测试各提供商的模型可用性
- **模型重命名配置**: 通过Web界面配置模型别名映射
- **参数覆盖设置**: 可视化配置分组级别的JSON请求参数覆盖
- **请求日志查看**: 详细的API请求和响应日志记录
- **统计分析**: API密钥使用统计和模型调用分析
- **自动刷新功能**: 实时更新状态信息
- **智能表单**: 下拉选择实际模型名称，避免输入错误

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

### 认证配置示例

```yaml
auth:
  enabled: true
  username: "your-admin-username"
  password: "your-strong-password-123!"
  session_timeout: "8h"
```

## 📝 更新日志

### v2.2.0 (最新版本)
- 🌐 **原生接口响应**: 支持返回提供商原生响应格式，不进行OpenAI兼容转换
- 🔗 **Gemini原生API端点**: 暴露 `/v1/beta` 端点，完全兼容Google Gemini原生API调用
- ⚡ **RPM限制功能**: 支持分组级别的每分钟请求数限制，防止配额过度消耗
- 🎛️ **Web界面增强**: 新增原生响应和RPM限制配置选项
- 🔄 **流式响应优化**: 原生模式下直接返回提供商的原始SSE数据流
- 🛠️ **数据库迁移**: 自动迁移数据库结构，支持新配置字段
- 🐛 **类型修复**: 修复前端数字字段类型转换问题

### v2.1.0
- 🏷️ **模型重命名映射**: 支持为不同分组的模型设置别名，统一模型名称管理
- ⚙️ **JSON参数覆盖**: 支持分组级别的请求参数覆盖（temperature、max_tokens等）
- 🔄 **智能路由重试**: 增强的故障转移机制，自动选择最佳分组进行重试
- 🎯 **智能表单**: Web界面优化，实际模型名称支持下拉选择
- 💾 **配置持久化**: 模型映射和参数覆盖配置正确保存到数据库
- 🐛 **Bug修复**: 修复配置数据传递和显示问题

### v2.0.0
- ✨ **多提供商支持**: 新增对 OpenAI、Google Gemini、Anthropic Claude、Azure OpenAI 等多个提供商的支持
- 🚀 **生产模式优化**: 添加 `server.mode` 配置，支持 debug/release/test 模式
- 🔧 **配置升级**: 从单一 OpenRouter 配置升级为多提供商分组配置
- 📊 **增强监控**: 新增多提供商健康检查和状态监控
- 🎛️ **管理界面**: 全新的分组管理界面，支持动态配置
- 🔑 **密钥验证**: 实时验证各提供商API密钥的有效性
- 🧪 **模型测试**: 支持测试各提供商的模型可用性
- 📈 **向后兼容**: 完全兼容旧版本配置文件

### v1.x.x (旧版本)
- 基础的 OpenRouter API 代理功能
- 单一提供商支持
- 基本的密钥轮询和监控

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

MIT License

## 📞 支持

如有问题，请提交 Issue 或联系维护者。
