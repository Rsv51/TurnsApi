# TurnsAPI 启动脚本

Write-Host "TurnsAPI 启动脚本" -ForegroundColor Green
Write-Host "=================" -ForegroundColor Green

# 检查配置文件
if (-not (Test-Path "config/config.yaml")) {
    Write-Host "错误: 配置文件 config/config.yaml 不存在" -ForegroundColor Red
    exit 1
}

# 检查可执行文件
if (-not (Test-Path "turnsapi.exe")) {
    Write-Host "可执行文件不存在，正在构建..." -ForegroundColor Yellow
    go build -o turnsapi.exe cmd/turnsapi/main.go
    if ($LASTEXITCODE -ne 0) {
        Write-Host "构建失败" -ForegroundColor Red
        exit 1
    }
    Write-Host "构建成功" -ForegroundColor Green
}

# 创建日志目录
if (-not (Test-Path "logs")) {
    New-Item -ItemType Directory -Path "logs" -Force | Out-Null
    Write-Host "创建日志目录: logs/" -ForegroundColor Cyan
}

# 检查配置文件中的API密钥
Write-Host "`n检查配置文件..." -ForegroundColor Yellow
$configContent = Get-Content "config/config.yaml" -Raw
if ($configContent -match "sk-or-v1-your-api-key") {
    Write-Host "警告: 检测到示例API密钥，请在 config/config.yaml 中配置真实的 OpenRouter API 密钥" -ForegroundColor Red
    Write-Host "示例密钥格式: sk-or-v1-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" -ForegroundColor Yellow
    
    $continue = Read-Host "`n是否继续启动服务？(y/N)"
    if ($continue -ne "y" -and $continue -ne "Y") {
        Write-Host "已取消启动" -ForegroundColor Yellow
        exit 0
    }
}

Write-Host "`n启动 TurnsAPI 服务..." -ForegroundColor Green
Write-Host "配置文件: config/config.yaml" -ForegroundColor Cyan
Write-Host "日志目录: logs/" -ForegroundColor Cyan
Write-Host "`n按 Ctrl+C 停止服务" -ForegroundColor Yellow
Write-Host "===================" -ForegroundColor Green

# 启动服务
./turnsapi.exe -config config/config.yaml
