package providers

import (
	"context"
	"io"
	"net/http"
	"time"
)

// ChatMessage 聊天消息结构，支持多模态内容
type ChatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // 支持字符串或多模态内容数组
}

// MessageContent 消息内容结构（用于多模态）
type MessageContent struct {
	Type     string            `json:"type"`     // "text" 或 "image_url"
	Text     string            `json:"text,omitempty"`
	ImageURL *MessageImageURL  `json:"image_url,omitempty"`
}

// MessageImageURL 图像URL结构
type MessageImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // "low", "high", "auto"
}

// ChatCompletionRequest 聊天完成请求结构
type ChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
	TopP        *float64      `json:"top_p,omitempty"`
	Stop        []string      `json:"stop,omitempty"`
}

// ChatCompletionResponse 聊天完成响应结构
type ChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// StreamResponse 流式响应结构
type StreamResponse struct {
	Data  []byte
	Error error
	Done  bool
}

// ProviderConfig 提供商配置
type ProviderConfig struct {
	BaseURL          string
	APIKey           string
	Timeout          time.Duration
	MaxRetries       int
	Headers          map[string]string
	ProviderType     string
}

// Provider 提供商接口
type Provider interface {
	// GetProviderType 获取提供商类型
	GetProviderType() string
	
	// ChatCompletion 发送聊天完成请求
	ChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error)
	
	// ChatCompletionStream 发送流式聊天完成请求
	ChatCompletionStream(ctx context.Context, req *ChatCompletionRequest) (<-chan StreamResponse, error)
	
	// GetModels 获取可用模型列表
	GetModels(ctx context.Context) (interface{}, error)
	
	// HealthCheck 健康检查
	HealthCheck(ctx context.Context) error
	
	// TransformRequest 将标准请求转换为提供商特定格式
	TransformRequest(req *ChatCompletionRequest) (interface{}, error)
	
	// TransformResponse 将提供商响应转换为标准格式
	TransformResponse(resp interface{}) (*ChatCompletionResponse, error)
	
	// CreateHTTPRequest 创建HTTP请求
	CreateHTTPRequest(ctx context.Context, endpoint string, body interface{}) (*http.Request, error)
	
	// ParseHTTPResponse 解析HTTP响应
	ParseHTTPResponse(resp *http.Response) (interface{}, error)
}

// ProviderFactory 提供商工厂接口
type ProviderFactory interface {
	CreateProvider(config *ProviderConfig) (Provider, error)
	GetSupportedTypes() []string
}

// BaseProvider 基础提供商实现
type BaseProvider struct {
	Config     *ProviderConfig
	HTTPClient *http.Client
}

// NewBaseProvider 创建基础提供商
func NewBaseProvider(config *ProviderConfig) *BaseProvider {
	return &BaseProvider{
		Config: config,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Minute, // 硬编码为10分钟超时
		},
	}
}

// GetProviderType 获取提供商类型
func (bp *BaseProvider) GetProviderType() string {
	return bp.Config.ProviderType
}

// CreateHTTPRequest 创建HTTP请求
func (bp *BaseProvider) CreateHTTPRequest(ctx context.Context, endpoint string, body interface{}) (*http.Request, error) {
	var bodyReader io.Reader
	
	if body != nil {
		// 这里需要根据具体实现来序列化body
		// 在具体的提供商实现中会重写这个方法
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bodyReader)
	if err != nil {
		return nil, err
	}
	
	// 设置通用头部
	for key, value := range bp.Config.Headers {
		req.Header.Set(key, value)
	}
	
	return req, nil
}

// HealthCheck 默认健康检查实现
func (bp *BaseProvider) HealthCheck(ctx context.Context) error {
	// 默认实现，具体提供商可以重写
	return nil
}
