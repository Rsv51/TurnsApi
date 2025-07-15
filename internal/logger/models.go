package logger

import (
	"time"
)

// RequestLog 请求日志结构
type RequestLog struct {
	ID             int64     `json:"id" db:"id"`
	ProxyKeyName   string    `json:"proxy_key_name" db:"proxy_key_name"`     // 代理服务API密钥名称
	ProxyKeyID     string    `json:"proxy_key_id" db:"proxy_key_id"`         // 代理服务API密钥ID
	OpenRouterKey  string    `json:"openrouter_key" db:"openrouter_key"`     // 使用的OpenRouter密钥（脱敏）
	Model          string    `json:"model" db:"model"`
	RequestBody    string    `json:"request_body" db:"request_body"`
	ResponseBody   string    `json:"response_body" db:"response_body"`
	StatusCode     int       `json:"status_code" db:"status_code"`
	IsStream       bool      `json:"is_stream" db:"is_stream"`
	Duration       int64     `json:"duration" db:"duration"` // 毫秒
	TokensUsed     int       `json:"tokens_used" db:"tokens_used"`
	Error          string    `json:"error" db:"error"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// RequestLogSummary 请求日志摘要（用于列表显示）
type RequestLogSummary struct {
	ID            int64     `json:"id"`
	ProxyKeyName  string    `json:"proxy_key_name"`
	ProxyKeyID    string    `json:"proxy_key_id"`
	OpenRouterKey string    `json:"openrouter_key"`
	Model         string    `json:"model"`
	StatusCode    int       `json:"status_code"`
	IsStream      bool      `json:"is_stream"`
	Duration      int64     `json:"duration"`
	TokensUsed    int       `json:"tokens_used"`
	Error         string    `json:"error"`
	CreatedAt     time.Time `json:"created_at"`
}

// ProxyKey 代理服务API密钥结构
type ProxyKey struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Key         string    `json:"key" db:"key"`
	IsActive    bool      `json:"is_active" db:"is_active"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
	LastUsedAt  *time.Time `json:"last_used_at" db:"last_used_at"`
}

// ProxyKeyStats 代理密钥统计
type ProxyKeyStats struct {
	ProxyKeyName    string  `json:"proxy_key_name"`
	ProxyKeyID      string  `json:"proxy_key_id"`
	TotalRequests   int64   `json:"total_requests"`
	SuccessRequests int64   `json:"success_requests"`
	ErrorRequests   int64   `json:"error_requests"`
	TotalTokens     int64   `json:"total_tokens"`
	AvgDuration     float64 `json:"avg_duration"`
}

// ModelStats 模型统计
type ModelStats struct {
	Model        string `json:"model"`
	TotalRequests int64  `json:"total_requests"`
	TotalTokens  int64  `json:"total_tokens"`
	AvgDuration  float64 `json:"avg_duration"`
}