package internal

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

// Config 应用程序配置结构
type Config struct {
	Server struct {
		Port string `yaml:"port"`
		Host string `yaml:"host"`
	} `yaml:"server"`

	Auth struct {
		Enabled        bool          `yaml:"enabled"`
		Username       string        `yaml:"username"`
		Password       string        `yaml:"password"`
		SessionTimeout time.Duration `yaml:"session_timeout"`
	} `yaml:"auth"`

	OpenRouter struct {
		BaseURL    string        `yaml:"base_url"`
		Timeout    time.Duration `yaml:"timeout"`
		MaxRetries int           `yaml:"max_retries"`
	} `yaml:"openrouter"`

	APIKeys struct {
		Keys                []string      `yaml:"keys"`
		RotationStrategy    string        `yaml:"rotation_strategy"`
		HealthCheckInterval time.Duration `yaml:"health_check_interval"`
	} `yaml:"api_keys"`

	Logging struct {
		Level      string `yaml:"level"`
		File       string `yaml:"file"`
		MaxSize    int    `yaml:"max_size"`
		MaxBackups int    `yaml:"max_backups"`
		MaxAge     int    `yaml:"max_age"`
	} `yaml:"logging"`

	Database struct {
		Path           string `yaml:"path"`
		RetentionDays  int    `yaml:"retention_days"`
	} `yaml:"database"`
}

// LoadConfig 从文件加载配置
func LoadConfig(configPath string) (*Config, error) {
	config := &Config{}

	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(config); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	// 设置默认值
	if config.Server.Port == "" {
		config.Server.Port = "8080"
	}
	if config.Server.Host == "" {
		config.Server.Host = "0.0.0.0"
	}
	if config.OpenRouter.BaseURL == "" {
		config.OpenRouter.BaseURL = "https://openrouter.ai/api/v1"
	}
	if config.OpenRouter.Timeout == 0 {
		config.OpenRouter.Timeout = 30 * time.Second
	}
	if config.OpenRouter.MaxRetries == 0 {
		config.OpenRouter.MaxRetries = 3
	}
	if config.APIKeys.RotationStrategy == "" {
		config.APIKeys.RotationStrategy = "round_robin"
	}
	if config.APIKeys.HealthCheckInterval == 0 {
		config.APIKeys.HealthCheckInterval = 60 * time.Second
	}
	if config.Logging.Level == "" {
		config.Logging.Level = "info"
	}
	if config.Auth.Username == "" {
		config.Auth.Username = "admin"
	}
	if config.Auth.Password == "" {
		config.Auth.Password = "turnsapi123"
	}
	if config.Auth.SessionTimeout == 0 {
		config.Auth.SessionTimeout = 24 * time.Hour
	}
	if config.Database.Path == "" {
		config.Database.Path = "data/turnsapi.db"
	}
	if config.Database.RetentionDays == 0 {
		config.Database.RetentionDays = 30
	}

	return config, nil
}

// GetAddress 获取服务器监听地址
func (c *Config) GetAddress() string {
	return fmt.Sprintf("%s:%s", c.Server.Host, c.Server.Port)
}
