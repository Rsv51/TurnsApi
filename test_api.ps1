# TurnsAPI 测试脚本
# 使用前请确保在 config/config.yaml 中配置了有效的 OpenRouter API 密钥

Write-Host "TurnsAPI 测试脚本" -ForegroundColor Green
Write-Host "==================" -ForegroundColor Green

# 检查服务是否运行
Write-Host "`n1. 检查服务健康状态..." -ForegroundColor Yellow
try {
    $healthResponse = Invoke-RestMethod -Uri "http://localhost:8080/health" -Method GET -TimeoutSec 5
    Write-Host "✓ 服务健康检查通过" -ForegroundColor Green
    Write-Host "  状态: $($healthResponse.status)" -ForegroundColor Cyan
} catch {
    Write-Host "✗ 服务健康检查失败: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "请确保 TurnsAPI 服务正在运行" -ForegroundColor Yellow
    exit 1
}

# 测试登录认证
Write-Host "`n2. 测试登录认证..." -ForegroundColor Yellow
$loginData = @{
    username = "admin"
    password = "turnsapi123"
} | ConvertTo-Json

try {
    $loginResponse = Invoke-RestMethod -Uri "http://localhost:8080/auth/login" -Method POST -Body $loginData -ContentType "application/json"
    if ($loginResponse.success) {
        Write-Host "✓ 登录认证测试通过" -ForegroundColor Green
        $authToken = $loginResponse.token
        Write-Host "  获得认证令牌: $($authToken.Substring(0,16))..." -ForegroundColor Cyan
    } else {
        Write-Host "✗ 登录认证失败" -ForegroundColor Red
        exit 1
    }
} catch {
    Write-Host "✗ 登录认证测试失败: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "  可能认证功能未启用，继续其他测试..." -ForegroundColor Yellow
    $authToken = $null
}

# 检查服务状态
Write-Host "`n3. 检查服务状态..." -ForegroundColor Yellow
try {
    $headers = @{}
    if ($authToken) {
        $headers["Authorization"] = "Bearer $authToken"
    }
    $statusResponse = Invoke-RestMethod -Uri "http://localhost:8080/admin/status" -Method GET -Headers $headers
    Write-Host "✓ 服务状态检查通过" -ForegroundColor Green
    Write-Host "  活跃密钥: $($statusResponse.active_keys)/$($statusResponse.total_keys)" -ForegroundColor Cyan
} catch {
    Write-Host "✗ 服务状态检查失败: $($_.Exception.Message)" -ForegroundColor Red
}

# 检查密钥状态
Write-Host "`n4. 检查API密钥状态..." -ForegroundColor Yellow
try {
    $headers = @{}
    if ($authToken) {
        $headers["Authorization"] = "Bearer $authToken"
    }
    $keysResponse = Invoke-RestMethod -Uri "http://localhost:8080/admin/keys" -Method GET -Headers $headers
    Write-Host "✓ API密钥状态检查通过" -ForegroundColor Green
    $activeKeys = ($keysResponse.keys.PSObject.Properties | Where-Object { $_.Value.is_active -eq $true }).Count
    $totalKeys = $keysResponse.keys.PSObject.Properties.Count
    Write-Host "  活跃密钥数量: $activeKeys/$totalKeys" -ForegroundColor Cyan
} catch {
    Write-Host "✗ API密钥状态检查失败: $($_.Exception.Message)" -ForegroundColor Red
}

# 测试非流式聊天完成
Write-Host "`n5. 测试非流式聊天完成..." -ForegroundColor Yellow
$chatRequest = @{
    model = "minimax/minimax-m1"
    messages = @(
        @{
            role = "user"
            content = "Hello! Please respond with just 'Test successful' if you can see this message."
        }
    )
    stream = $false
    max_tokens = 50
} | ConvertTo-Json -Depth 3

try {
    $headers = @{"Content-Type" = "application/json"}
    if ($authToken) {
        $headers["Authorization"] = "Bearer $authToken"
    }
    $chatResponse = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/chat/completions" -Method POST -Body $chatRequest -Headers $headers -TimeoutSec 30
    Write-Host "✓ 非流式聊天完成测试通过" -ForegroundColor Green
    if ($chatResponse.choices -and $chatResponse.choices.Count -gt 0) {
        $responseContent = $chatResponse.choices[0].message.content
        Write-Host "  响应内容: $responseContent" -ForegroundColor Cyan
    }
} catch {
    Write-Host "✗ 非流式聊天完成测试失败: $($_.Exception.Message)" -ForegroundColor Red
    if ($_.Exception.Response) {
        $errorStream = $_.Exception.Response.GetResponseStream()
        $reader = New-Object System.IO.StreamReader($errorStream)
        $errorBody = $reader.ReadToEnd()
        Write-Host "  错误详情: $errorBody" -ForegroundColor Red
    }
}

# 测试流式聊天完成（简单测试）
Write-Host "`n6. 测试流式聊天完成..." -ForegroundColor Yellow
$streamRequest = @{
    model = "minimax/minimax-m1"
    messages = @(
        @{
            role = "user"
            content = "Count from 1 to 3, one number per line."
        }
    )
    stream = $true
    max_tokens = 20
} | ConvertTo-Json -Depth 3

try {
    # 使用 curl 测试流式响应（PowerShell 的 Invoke-RestMethod 不太适合处理 SSE）
    Write-Host "  使用 curl 测试流式响应..." -ForegroundColor Cyan
    $authHeader = if ($authToken) { "-H `"Authorization: Bearer $authToken`"" } else { "" }
    $curlCommand = "curl -X POST http://localhost:8080/api/v1/chat/completions -H `"Content-Type: application/json`" $authHeader -d `"$($streamRequest.Replace('`"', '\`"'))`" --max-time 10"
    Write-Host "  命令: $curlCommand" -ForegroundColor Gray
    Write-Host "✓ 流式聊天完成测试配置完成（需要手动使用 curl 测试）" -ForegroundColor Green
} catch {
    Write-Host "✗ 流式聊天完成测试失败: $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host "`n==================" -ForegroundColor Green
Write-Host "测试完成！" -ForegroundColor Green
Write-Host "`n访问以下地址查看 Web 界面:" -ForegroundColor Yellow
Write-Host "  首页: http://localhost:8080/" -ForegroundColor Cyan
Write-Host "  仪表板: http://localhost:8080/dashboard" -ForegroundColor Cyan
Write-Host "  API状态: http://localhost:8080/admin/status" -ForegroundColor Cyan

Write-Host "`n手动测试流式响应命令:" -ForegroundColor Yellow
if ($authToken) {
    Write-Host "curl -X POST http://localhost:8080/api/v1/chat/completions \\" -ForegroundColor Cyan
    Write-Host "  -H `"Content-Type: application/json`" \\" -ForegroundColor Cyan
    Write-Host "  -H `"Authorization: Bearer $authToken`" \\" -ForegroundColor Cyan
    Write-Host "  -d '{" -ForegroundColor Cyan
    Write-Host "    `"model`": `"minimax/minimax-m1`"," -ForegroundColor Cyan
    Write-Host "    `"messages`": [{`"role`": `"user`", `"content`": `"Hello`"}]," -ForegroundColor Cyan
    Write-Host "    `"stream`": true" -ForegroundColor Cyan
    Write-Host "  }'" -ForegroundColor Cyan
} else {
    Write-Host "curl -X POST http://localhost:8080/api/v1/chat/completions \\" -ForegroundColor Cyan
    Write-Host "  -H `"Content-Type: application/json`" \\" -ForegroundColor Cyan
    Write-Host "  -d '{" -ForegroundColor Cyan
    Write-Host "    `"model`": `"minimax/minimax-m1`"," -ForegroundColor Cyan
    Write-Host "    `"messages`": [{`"role`": `"user`", `"content`": `"Hello`"}]," -ForegroundColor Cyan
    Write-Host "    `"stream`": true" -ForegroundColor Cyan
    Write-Host "  }'" -ForegroundColor Cyan
}
