package internal

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

// UserGroup 用户自定义分组配置
type UserGroup struct {
	Name             string                 `yaml:"name"`
	ProviderType     string                 `yaml:"provider_type"`
	BaseURL          string                 `yaml:"base_url"`
	Enabled          bool                   `yaml:"enabled"`
	Timeout          time.Duration          `yaml:"timeout"`
	MaxRetries       int                    `yaml:"max_retries"`
	RotationStrategy string                 `yaml:"rotation_strategy"`
	Models           []string               `yaml:"models"`
	APIKeys          []string               `yaml:"api_keys"`
	Headers          map[string]string      `yaml:"headers,omitempty"`
	RequestParams    map[string]interface{} `yaml:"request_params,omitempty"` // JSON请求参数覆盖
	ModelMappings    map[string]string      `yaml:"model_mappings,omitempty"` // 模型名称映射：别名 -> 原始模型名
}

// GlobalSettings 全局设置
type GlobalSettings struct {
	DefaultRotationStrategy string        `yaml:"default_rotation_strategy"`
	DefaultTimeout          time.Duration `yaml:"default_timeout"`
	DefaultMaxRetries       int           `yaml:"default_max_retries"`
}

// Monitoring 监控配置
type Monitoring struct {
	Enabled         bool   `yaml:"enabled"`
	MetricsEndpoint string `yaml:"metrics_endpoint"`
	HealthEndpoint  string `yaml:"health_endpoint"`
}

// Config 应用程序配置结构
type Config struct {
	Server struct {
		Port string `yaml:"port"`
		Host string `yaml:"host"`
		Mode string `yaml:"mode"`
	} `yaml:"server"`

	Auth struct {
		Enabled        bool          `yaml:"enabled"`
		Username       string        `yaml:"username"`
		Password       string        `yaml:"password"`
		SessionTimeout time.Duration `yaml:"session_timeout"`
	} `yaml:"auth"`

	// 新的用户分组配置
	UserGroups map[string]*UserGroup `yaml:"user_groups,omitempty"`

	// 全局设置
	GlobalSettings *GlobalSettings `yaml:"global_settings,omitempty"`

	// 监控配置
	Monitoring *Monitoring `yaml:"monitoring,omitempty"`

	// 向后兼容的旧配置结构
	OpenRouter struct {
		BaseURL    string        `yaml:"base_url"`
		Timeout    time.Duration `yaml:"timeout"`
		MaxRetries int           `yaml:"max_retries"`
	} `yaml:"openrouter,omitempty"`

	APIKeys struct {
		Keys                []string      `yaml:"keys"`
		RotationStrategy    string        `yaml:"rotation_strategy"`
	} `yaml:"api_keys,omitempty"`

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

	// 设置基本默认值
	if config.Server.Port == "" {
		config.Server.Port = "8080"
	}
	if config.Server.Host == "" {
		config.Server.Host = "0.0.0.0"
	}
	if config.Server.Mode == "" {
		config.Server.Mode = "release"  // 默认使用release模式
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

	// 设置全局设置默认值
	if config.GlobalSettings == nil {
		config.GlobalSettings = &GlobalSettings{}
	}
	if config.GlobalSettings.DefaultRotationStrategy == "" {
		config.GlobalSettings.DefaultRotationStrategy = "round_robin"
	}
	if config.GlobalSettings.DefaultTimeout == 0 {
		config.GlobalSettings.DefaultTimeout = 30 * time.Second
	}
	if config.GlobalSettings.DefaultMaxRetries == 0 {
		config.GlobalSettings.DefaultMaxRetries = 3
	}

	// 设置监控配置默认值
	if config.Monitoring == nil {
		config.Monitoring = &Monitoring{}
	}
	if config.Monitoring.MetricsEndpoint == "" {
		config.Monitoring.MetricsEndpoint = "/metrics"
	}
	if config.Monitoring.HealthEndpoint == "" {
		config.Monitoring.HealthEndpoint = "/health"
	}

	// 向后兼容处理：如果没有用户分组配置但有旧的OpenRouter配置，则创建默认分组
	if len(config.UserGroups) == 0 && (len(config.APIKeys.Keys) > 0 || config.OpenRouter.BaseURL != "") {
		config.UserGroups = make(map[string]*UserGroup)

		// 设置OpenRouter默认值
		baseURL := config.OpenRouter.BaseURL
		if baseURL == "" {
			baseURL = "https://openrouter.ai/api/v1"
		}

		timeout := config.OpenRouter.Timeout
		if timeout == 0 {
			timeout = config.GlobalSettings.DefaultTimeout
		}

		maxRetries := config.OpenRouter.MaxRetries
		if maxRetries == 0 {
			maxRetries = config.GlobalSettings.DefaultMaxRetries
		}

		rotationStrategy := config.APIKeys.RotationStrategy
		if rotationStrategy == "" {
			rotationStrategy = config.GlobalSettings.DefaultRotationStrategy
		}

		// 创建默认的OpenRouter分组
		config.UserGroups["openrouter_default"] = &UserGroup{
			Name:             "OpenRouter (默认)",
			ProviderType:     "openai",
			BaseURL:          baseURL,
			Enabled:          true,
			Timeout:          timeout,
			MaxRetries:       maxRetries,
			RotationStrategy: rotationStrategy,
			APIKeys:          config.APIKeys.Keys,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		}
	}

	// 为每个用户分组设置默认值
	for groupID, group := range config.UserGroups {
		if group.Timeout == 0 {
			group.Timeout = config.GlobalSettings.DefaultTimeout
		}
		if group.MaxRetries == 0 {
			group.MaxRetries = config.GlobalSettings.DefaultMaxRetries
		}
		if group.RotationStrategy == "" {
			group.RotationStrategy = config.GlobalSettings.DefaultRotationStrategy
		}
		if group.Headers == nil {
			group.Headers = make(map[string]string)
		}
		if group.Headers["Content-Type"] == "" {
			group.Headers["Content-Type"] = "application/json"
		}

		// 为特定提供商设置默认头部
		switch group.ProviderType {
		case "anthropic":
			// Anthropic版本现在在提供商内部处理
			if group.Headers["anthropic-version"] == "" {
				group.Headers["anthropic-version"] = "2023-06-01"
			}
		}

		config.UserGroups[groupID] = group
	}

	return config, nil
}

// GetAddress 获取服务器监听地址
func (c *Config) GetAddress() string {
	return fmt.Sprintf("%s:%s", c.Server.Host, c.Server.Port)
}

// GetEnabledGroups 获取所有启用的用户分组
func (c *Config) GetEnabledGroups() map[string]*UserGroup {
	enabled := make(map[string]*UserGroup)
	for groupID, group := range c.UserGroups {
		if group.Enabled {
			enabled[groupID] = group
		}
	}
	return enabled
}

// GetGroupByID 根据ID获取用户分组
func (c *Config) GetGroupByID(groupID string) (*UserGroup, bool) {
	group, exists := c.UserGroups[groupID]
	return group, exists
}

// GetGroupByModel 根据模型名称获取匹配的用户分组
func (c *Config) GetGroupByModel(modelName string) (*UserGroup, string) {
	for groupID, group := range c.UserGroups {
		if !group.Enabled {
			continue
		}

		// 如果分组没有指定模型列表，则认为支持所有模型
		if len(group.Models) == 0 {
			return group, groupID
		}

		// 检查模型是否在分组的模型列表中
		for _, model := range group.Models {
			if model == modelName {
				return group, groupID
			}
		}
	}
	return nil, ""
}

// GetGroupByBaseURL 根据BaseURL获取用户分组
func (c *Config) GetGroupByBaseURL(baseURL string) (*UserGroup, string) {
	for groupID, group := range c.UserGroups {
		if group.BaseURL == baseURL {
			return group, groupID
		}
	}
	return nil, ""
}

// IsLegacyConfig 检查是否为旧版配置
func (c *Config) IsLegacyConfig() bool {
	return len(c.UserGroups) == 1 && c.UserGroups["openrouter_default"] != nil
}
