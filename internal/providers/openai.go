package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OpenAIProvider OpenAI格式提供商
type OpenAIProvider struct {
	*BaseProvider
}

// NewOpenAIProvider 创建OpenAI提供商
func NewOpenAIProvider(config *ProviderConfig) *OpenAIProvider {
	return &OpenAIProvider{
		BaseProvider: NewBaseProvider(config),
	}
}

// ChatCompletion 发送聊天完成请求
func (p *OpenAIProvider) ChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// OpenAI格式不需要转换，直接使用
	endpoint := fmt.Sprintf("%s/chat/completions", p.Config.BaseURL)
	
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// 设置头部
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.Config.APIKey)
	
	// 设置自定义头部
	for key, value := range p.Config.Headers {
		if key != "Authorization" { // 避免覆盖Authorization头
			httpReq.Header.Set(key, value)
		}
	}
	
	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	var response ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	return &response, nil
}

// ChatCompletionStream 发送流式聊天完成请求
func (p *OpenAIProvider) ChatCompletionStream(ctx context.Context, req *ChatCompletionRequest) (<-chan StreamResponse, error) {
	// 确保设置stream为true
	req.Stream = true
	
	endpoint := fmt.Sprintf("%s/chat/completions", p.Config.BaseURL)
	
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// 设置头部
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.Config.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Cache-Control", "no-cache")
	
	// 设置自定义头部
	for key, value := range p.Config.Headers {
		if key != "Authorization" {
			httpReq.Header.Set(key, value)
		}
	}
	
	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	streamChan := make(chan StreamResponse, 10)
	
	go func() {
		defer close(streamChan)
		defer resp.Body.Close()
		
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			
			// 发送原始数据行
			streamChan <- StreamResponse{
				Data: []byte(line + "\n"),
				Done: false,
			}
			
			// 检查是否结束
			if strings.Contains(line, "[DONE]") {
				streamChan <- StreamResponse{
					Done: true,
				}
				return
			}
		}
		
		if err := scanner.Err(); err != nil {
			streamChan <- StreamResponse{
				Error: err,
				Done:  true,
			}
		}
	}()
	
	return streamChan, nil
}

// GetModels 获取可用模型列表
func (p *OpenAIProvider) GetModels(ctx context.Context) (interface{}, error) {
	endpoint := fmt.Sprintf("%s/models", p.Config.BaseURL)
	
	httpReq, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	httpReq.Header.Set("Authorization", "Bearer "+p.Config.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	
	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	var models interface{}
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	return models, nil
}

// HealthCheck 健康检查
func (p *OpenAIProvider) HealthCheck(ctx context.Context) error {
	// 创建一个简单的健康检查请求，只检查连接性
	req, err := http.NewRequestWithContext(ctx, "GET", p.Config.BaseURL+"/models", nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	// 添加认证头
	req.Header.Set("Authorization", "Bearer "+p.Config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	// 添加自定义头
	for key, value := range p.Config.Headers {
		req.Header.Set(key, value)
	}

	// 发送请求
	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send health check request: %w", err)
	}
	defer resp.Body.Close()

	// 只要返回状态码是 2xx 就认为健康
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	// 如果状态码不是 2xx，读取错误信息
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("health check failed with status %d: %s", resp.StatusCode, string(body))
}

// TransformRequest OpenAI格式不需要转换
func (p *OpenAIProvider) TransformRequest(req *ChatCompletionRequest) (interface{}, error) {
	return req, nil
}

// TransformResponse OpenAI格式不需要转换
func (p *OpenAIProvider) TransformResponse(resp interface{}) (*ChatCompletionResponse, error) {
	if response, ok := resp.(*ChatCompletionResponse); ok {
		return response, nil
	}
	return nil, fmt.Errorf("invalid response type")
}

// CreateHTTPRequest 创建HTTP请求
func (p *OpenAIProvider) CreateHTTPRequest(ctx context.Context, endpoint string, body interface{}) (*http.Request, error) {
	var bodyReader io.Reader
	
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyReader = bytes.NewBuffer(jsonBody)
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bodyReader)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.Config.APIKey)
	
	for key, value := range p.Config.Headers {
		if key != "Authorization" {
			req.Header.Set(key, value)
		}
	}
	
	return req, nil
}

// ParseHTTPResponse 解析HTTP响应
func (p *OpenAIProvider) ParseHTTPResponse(resp *http.Response) (interface{}, error) {
	var response ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &response, nil
}
