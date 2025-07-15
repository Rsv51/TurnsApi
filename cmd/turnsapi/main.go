package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"turnsapi/internal"
	"turnsapi/internal/api"
	"turnsapi/internal/keymanager"
	"turnsapi/internal/logger"
)

var (
	configPath = flag.String("config", "config/config.yaml", "配置文件路径")
	version    = "1.0.0"
)

func main() {
	flag.Parse()

	log.Printf("TurnsAPI v%s 启动中...", version)

	// 加载配置
	config, err := internal.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 验证配置
	if len(config.APIKeys.Keys) == 0 {
		log.Fatal("配置文件中未找到API密钥，请在config/config.yaml中配置您的OpenRouter API密钥")
	}

	// 检查API密钥格式
	validKeys := make([]string, 0)
	for _, key := range config.APIKeys.Keys {
		if key != "" && key != "sk-or-v1-your-api-key-1" && len(key) > 10 {
			validKeys = append(validKeys, key)
		}
	}

	if len(validKeys) == 0 {
		log.Fatal("未找到有效的API密钥，请确保在config/config.yaml中配置了真实的OpenRouter API密钥")
	}

	log.Printf("加载了 %d 个有效的API密钥", len(validKeys))

	// 创建日志目录
	if config.Logging.File != "" {
		if err := os.MkdirAll("logs", 0755); err != nil {
			log.Printf("创建日志目录失败: %v", err)
		}
	}

	// 创建密钥管理器
	keyManager := keymanager.NewKeyManager(
		validKeys,
		config.APIKeys.RotationStrategy,
		config.APIKeys.HealthCheckInterval,
		"config/config.yaml", // 配置文件路径
	)
	defer keyManager.Close()

	log.Printf("密钥管理器初始化完成，轮询策略: %s", config.APIKeys.RotationStrategy)

	// 启动日志清理任务
	go startLogCleanupTask(config)

	// 创建HTTP服务器
	server := api.NewServer(config, keyManager)

	// 启动服务器
	go func() {
		log.Printf("HTTP服务器启动在 %s", config.GetAddress())
		if err := server.Start(); err != nil {
			log.Fatalf("服务器启动失败: %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("正在关闭服务器...")

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Stop(ctx); err != nil {
		log.Printf("服务器关闭失败: %v", err)
	} else {
		log.Println("服务器已优雅关闭")
	}
}

// startLogCleanupTask 启动日志清理任务
func startLogCleanupTask(config *internal.Config) {
	// 每天凌晨2点执行清理任务
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// 立即执行一次清理（如果需要）
	performLogCleanup(config)

	for {
		select {
		case <-ticker.C:
			performLogCleanup(config)
		}
	}
}

// performLogCleanup 执行日志清理
func performLogCleanup(config *internal.Config) {
	if config.Database.RetentionDays <= 0 {
		return // 如果保留天数为0或负数，不执行清理
	}

	requestLogger, err := logger.NewRequestLogger(config.Database.Path)
	if err != nil {
		log.Printf("Failed to create request logger for cleanup: %v", err)
		return
	}
	defer requestLogger.Close()

	if err := requestLogger.CleanupOldLogs(config.Database.RetentionDays); err != nil {
		log.Printf("Failed to cleanup old logs: %v", err)
	} else {
		log.Printf("Log cleanup completed successfully")
	}
}
