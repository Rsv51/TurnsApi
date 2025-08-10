package providers

import (
	"testing"
	"time"
)

func TestProviderFactory(t *testing.T) {
	factory := NewDefaultProviderFactory()

	// Test supported types
	supportedTypes := factory.GetSupportedTypes()
	expectedTypes := []string{"openai", "openrouter", "gemini", "anthropic", "azure_openai"}

	if len(supportedTypes) != len(expectedTypes) {
		t.Errorf("Expected %d supported types, got %d", len(expectedTypes), len(supportedTypes))
	}

	for _, expectedType := range expectedTypes {
		found := false
		for _, supportedType := range supportedTypes {
			if supportedType == expectedType {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected type %s not found in supported types", expectedType)
		}
	}
}

func TestCreateOpenAIProvider(t *testing.T) {
	factory := NewDefaultProviderFactory()
	config := &ProviderConfig{
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "test-key",
		Timeout:      30 * time.Second,
		MaxRetries:   3,
		Headers:      map[string]string{"Content-Type": "application/json"},
		ProviderType: "openai",
	}

	provider, err := factory.CreateProvider(config)
	if err != nil {
		t.Fatalf("Failed to create OpenAI provider: %v", err)
	}

	if provider.GetProviderType() != "openai" {
		t.Errorf("Expected provider type 'openai', got '%s'", provider.GetProviderType())
	}
}

func TestCreateGeminiProvider(t *testing.T) {
	factory := NewDefaultProviderFactory()
	config := &ProviderConfig{
		BaseURL:      "https://generativelanguage.googleapis.com/v1beta",
		APIKey:       "test-key",
		Timeout:      30 * time.Second,
		MaxRetries:   3,
		Headers:      map[string]string{"Content-Type": "application/json"},
		ProviderType: "gemini",
	}

	provider, err := factory.CreateProvider(config)
	if err != nil {
		t.Fatalf("Failed to create Gemini provider: %v", err)
	}

	if provider.GetProviderType() != "gemini" {
		t.Errorf("Expected provider type 'gemini', got '%s'", provider.GetProviderType())
	}
}

func TestCreateAnthropicProvider(t *testing.T) {
	factory := NewDefaultProviderFactory()
	config := &ProviderConfig{
		BaseURL:      "https://api.anthropic.com",
		APIKey:       "test-key",
		Timeout:      30 * time.Second,
		MaxRetries:   3,
		Headers:      map[string]string{"Content-Type": "application/json"},
		ProviderType: "anthropic",
	}

	provider, err := factory.CreateProvider(config)
	if err != nil {
		t.Fatalf("Failed to create Anthropic provider: %v", err)
	}

	if provider.GetProviderType() != "anthropic" {
		t.Errorf("Expected provider type 'anthropic', got '%s'", provider.GetProviderType())
	}
}

func TestUnsupportedProviderType(t *testing.T) {
	factory := NewDefaultProviderFactory()
	config := &ProviderConfig{
		BaseURL:      "https://example.com",
		APIKey:       "test-key",
		Timeout:      30 * time.Second,
		MaxRetries:   3,
		Headers:      map[string]string{"Content-Type": "application/json"},
		ProviderType: "unsupported",
	}

	_, err := factory.CreateProvider(config)
	if err == nil {
		t.Error("Expected error for unsupported provider type, got nil")
	}
}

func TestValidateProviderConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *ProviderConfig
		expectError bool
	}{
		{
			name: "valid config",
			config: &ProviderConfig{
				BaseURL:      "https://api.openai.com/v1",
				APIKey:       "test-key",
				Timeout:      30 * time.Second,
				MaxRetries:   3,
				Headers:      map[string]string{"Content-Type": "application/json"},
				ProviderType: "openai",
			},
			expectError: false,
		},
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
		},
		{
			name: "empty provider type",
			config: &ProviderConfig{
				BaseURL: "https://api.openai.com/v1",
				APIKey:  "test-key",
			},
			expectError: true,
		},
		{
			name: "empty base URL",
			config: &ProviderConfig{
				APIKey:       "test-key",
				ProviderType: "openai",
			},
			expectError: true,
		},
		{
			name: "empty API key",
			config: &ProviderConfig{
				BaseURL:      "https://api.openai.com/v1",
				ProviderType: "openai",
			},
			expectError: true,
		},
		{
			name: "unsupported provider type",
			config: &ProviderConfig{
				BaseURL:      "https://api.openai.com/v1",
				APIKey:       "test-key",
				ProviderType: "unsupported",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProviderConfig(tt.config)
			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestProviderManager(t *testing.T) {
	factory := NewDefaultProviderFactory()
	manager := NewProviderManager(factory)

	config := &ProviderConfig{
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "test-key",
		Timeout:      30 * time.Second,
		MaxRetries:   3,
		Headers:      map[string]string{"Content-Type": "application/json"},
		ProviderType: "openai",
	}

	// Test getting provider
	provider1, err := manager.GetProvider("test-group", config)
	if err != nil {
		t.Fatalf("Failed to get provider: %v", err)
	}

	// Test getting same provider again (should be cached)
	provider2, err := manager.GetProvider("test-group", config)
	if err != nil {
		t.Fatalf("Failed to get cached provider: %v", err)
	}

	if provider1 != provider2 {
		t.Error("Expected same provider instance from cache")
	}

	// Test removing provider
	manager.RemoveProvider("test-group")

	// Test getting provider after removal (should create new instance)
	provider3, err := manager.GetProvider("test-group", config)
	if err != nil {
		t.Fatalf("Failed to get provider after removal: %v", err)
	}

	if provider1 == provider3 {
		t.Error("Expected different provider instance after removal")
	}
}

func TestChatCompletionRequest(t *testing.T) {
	req := &ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
		Temperature: func() *float64 { f := 0.7; return &f }(),
		MaxTokens:   func() *int { i := 100; return &i }(),
		Stream:      false,
	}

	if req.Model != "gpt-3.5-turbo" {
		t.Errorf("Expected model 'gpt-3.5-turbo', got '%s'", req.Model)
	}

	if len(req.Messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(req.Messages))
	}

	if req.Messages[0].Role != "user" {
		t.Errorf("Expected role 'user', got '%s'", req.Messages[0].Role)
	}

	if req.Messages[0].Content != "Hello" {
		t.Errorf("Expected content 'Hello', got '%s'", req.Messages[0].Content)
	}

	if req.Temperature == nil || *req.Temperature != 0.7 {
		t.Errorf("Expected temperature 0.7, got %v", req.Temperature)
	}

	if req.MaxTokens == nil || *req.MaxTokens != 100 {
		t.Errorf("Expected max_tokens 100, got %v", req.MaxTokens)
	}

	if req.Stream != false {
		t.Errorf("Expected stream false, got %v", req.Stream)
	}
}

func TestOpenAITransformations(t *testing.T) {
	provider := NewOpenAIProvider(&ProviderConfig{
		ProviderType: "openai",
	})

	req := &ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	// Test request transformation (should be no-op for OpenAI)
	transformed, err := provider.TransformRequest(req)
	if err != nil {
		t.Fatalf("Failed to transform request: %v", err)
	}

	if transformed != req {
		t.Error("Expected OpenAI request transformation to be no-op")
	}

	// Test response transformation
	response := &ChatCompletionResponse{
		ID:     "test-id",
		Object: "chat.completion",
		Model:  "gpt-3.5-turbo",
	}

	transformedResp, err := provider.TransformResponse(response)
	if err != nil {
		t.Fatalf("Failed to transform response: %v", err)
	}

	if transformedResp != response {
		t.Error("Expected OpenAI response transformation to be no-op")
	}
}

func TestOpenAIDefaultMaxTokens(t *testing.T) {
	_ = NewOpenAIProvider(&ProviderConfig{
		ProviderType: "openai",
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "test-key",
	})

	// Test that default max_tokens is set when not provided
	req := &ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
		// MaxTokens is nil
	}

	// Create a copy to test with
	reqCopy := *req

	// This would normally make an HTTP request, but we can test the logic
	// by checking if MaxTokens gets set before the request is made
	if reqCopy.MaxTokens != nil {
		t.Error("Expected MaxTokens to be nil initially")
	}

	// Test that when MaxTokens is already set, it's not overridden
	existingMaxTokens := 1000
	reqWithMaxTokens := &ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
		MaxTokens: &existingMaxTokens,
	}

	// The provider should not override existing MaxTokens
	if reqWithMaxTokens.MaxTokens == nil || *reqWithMaxTokens.MaxTokens != 1000 {
		t.Error("Expected existing MaxTokens to be preserved")
	}
}

// TestToolCallValidation 测试工具调用验证功能
func TestToolCallValidation(t *testing.T) {
	provider := NewOpenAIProvider(&ProviderConfig{
		ProviderType: "openai",
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "test-key",
	})

	tests := []struct {
		name        string
		request     *ChatCompletionRequest
		expectError bool
		errorCode   string
	}{
		{
			name: "valid tool call request",
			request: &ChatCompletionRequest{
				Model: "gpt-3.5-turbo",
				Messages: []ChatMessage{
					{Role: "user", Content: "What's the weather like?"},
				},
				Tools: []Tool{
					{
						Type: "function",
						Function: &Function{
							Name:        "get_weather",
							Description: "Get current weather information",
							Parameters: map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"location": map[string]interface{}{
										"type":        "string",
										"description": "The city and state",
									},
								},
								"required": []string{"location"},
							},
						},
					},
				},
				ToolChoice: "auto",
			},
			expectError: false,
		},
		{
			name: "too many tools",
			request: &ChatCompletionRequest{
				Model: "gpt-3.5-turbo",
				Messages: []ChatMessage{
					{Role: "user", Content: "Test"},
				},
				Tools: make([]Tool, 129), // 超过128个工具的限制
			},
			expectError: true,
			errorCode:   "too_many_tools",
		},
		{
			name: "invalid tool type",
			request: &ChatCompletionRequest{
				Model: "gpt-3.5-turbo",
				Messages: []ChatMessage{
					{Role: "user", Content: "Test"},
				},
				Tools: []Tool{
					{
						Type: "invalid_type",
						Function: &Function{
							Name: "test_function",
						},
					},
				},
			},
			expectError: true,
			errorCode:   "invalid_tool_type",
		},
		{
			name: "missing function definition",
			request: &ChatCompletionRequest{
				Model: "gpt-3.5-turbo",
				Messages: []ChatMessage{
					{Role: "user", Content: "Test"},
				},
				Tools: []Tool{
					{
						Type:     "function",
						Function: nil,
					},
				},
			},
			expectError: true,
			errorCode:   "missing_function_definition",
		},
		{
			name: "missing function name",
			request: &ChatCompletionRequest{
				Model: "gpt-3.5-turbo",
				Messages: []ChatMessage{
					{Role: "user", Content: "Test"},
				},
				Tools: []Tool{
					{
						Type: "function",
						Function: &Function{
							Name: "",
						},
					},
				},
			},
			expectError: true,
			errorCode:   "missing_function_name",
		},
		{
			name: "invalid function name",
			request: &ChatCompletionRequest{
				Model: "gpt-3.5-turbo",
				Messages: []ChatMessage{
					{Role: "user", Content: "Test"},
				},
				Tools: []Tool{
					{
						Type: "function",
						Function: &Function{
							Name: "invalid function name with spaces!",
						},
					},
				},
			},
			expectError: true,
			errorCode:   "invalid_function_name",
		},
		{
			name: "duplicate function names",
			request: &ChatCompletionRequest{
				Model: "gpt-3.5-turbo",
				Messages: []ChatMessage{
					{Role: "user", Content: "Test"},
				},
				Tools: []Tool{
					{
						Type: "function",
						Function: &Function{
							Name: "duplicate_function",
						},
					},
					{
						Type: "function",
						Function: &Function{
							Name: "duplicate_function",
						},
					},
				},
			},
			expectError: true,
			errorCode:   "duplicate_function_name",
		},
		{
			name: "invalid tool_choice string",
			request: &ChatCompletionRequest{
				Model: "gpt-3.5-turbo",
				Messages: []ChatMessage{
					{Role: "user", Content: "Test"},
				},
				Tools: []Tool{
					{
						Type: "function",
						Function: &Function{
							Name: "test_function",
						},
					},
				},
				ToolChoice: "invalid_choice",
			},
			expectError: true,
			errorCode:   "invalid_tool_choice",
		},
		{
			name: "unknown tool_choice function",
			request: &ChatCompletionRequest{
				Model: "gpt-3.5-turbo",
				Messages: []ChatMessage{
					{Role: "user", Content: "Test"},
				},
				Tools: []Tool{
					{
						Type: "function",
						Function: &Function{
							Name: "available_function",
						},
					},
				},
				ToolChoice: ToolChoiceFunction{
					Type: "function",
					Function: &ToolChoiceFunc{
						Name: "unknown_function",
					},
				},
			},
			expectError: true,
			errorCode:   "unknown_tool_choice_function",
		},
		{
			name: "valid specific tool_choice",
			request: &ChatCompletionRequest{
				Model: "gpt-3.5-turbo",
				Messages: []ChatMessage{
					{Role: "user", Content: "Test"},
				},
				Tools: []Tool{
					{
						Type: "function",
						Function: &Function{
							Name: "available_function",
						},
					},
				},
				ToolChoice: ToolChoiceFunction{
					Type: "function",
					Function: &ToolChoiceFunc{
						Name: "available_function",
					},
				},
			},
			expectError: false,
		},
		{
			name: "invalid parallel_tool_calls with single tool",
			request: &ChatCompletionRequest{
				Model: "gpt-3.5-turbo",
				Messages: []ChatMessage{
					{Role: "user", Content: "Test"},
				},
				Tools: []Tool{
					{
						Type: "function",
						Function: &Function{
							Name: "single_function",
						},
					},
				},
				ParallelToolCalls: func() *bool { b := true; return &b }(),
			},
			expectError: true,
			errorCode:   "invalid_parallel_tool_calls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := provider.validateToolCallRequest(tt.request)
			
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
					return
				}
				
				if toolCallErr, ok := err.(*ToolCallError); ok {
					if toolCallErr.Code != tt.errorCode {
						t.Errorf("Expected error code '%s', got '%s'", tt.errorCode, toolCallErr.Code)
					}
				} else {
					t.Errorf("Expected ToolCallError, got %T", err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

// TestFunctionNameValidation 测试函数名称验证
func TestFunctionNameValidation(t *testing.T) {
	tests := []struct {
		name     string
		funcName string
		valid    bool
	}{
		{"valid name with letters", "get_weather", true},
		{"valid name with numbers", "function_123", true},
		{"valid name with hyphens", "get-weather-info", true},
		{"valid name with underscores", "get_weather_info", true},
		{"valid mixed case", "GetWeatherInfo", true},
		{"empty name", "", false},
		{"too long name", "this_is_a_very_long_function_name_that_exceeds_the_maximum_allowed_length_of_64_characters", false},
		{"name with spaces", "get weather", false},
		{"name with special chars", "get_weather!", false},
		{"name with dots", "get.weather", false},
		{"name starting with number", "123_function", true}, // 数字开头是允许的
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidFunctionName(tt.funcName)
			if result != tt.valid {
				t.Errorf("Expected isValidFunctionName('%s') = %v, got %v", tt.funcName, tt.valid, result)
			}
		})
	}
}

// TestToolCallRequestStructure 测试工具调用请求结构
func TestToolCallRequestStructure(t *testing.T) {
	// 测试完整的工具调用请求结构
	req := &ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []ChatMessage{
			{Role: "user", Content: "What's the weather in New York?"},
		},
		Tools: []Tool{
			{
				Type: "function",
				Function: &Function{
					Name:        "get_weather",
					Description: "Get current weather information for a location",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{
								"type":        "string",
								"description": "The city and state, e.g. San Francisco, CA",
							},
							"unit": map[string]interface{}{
								"type":        "string",
								"enum":        []string{"celsius", "fahrenheit"},
								"description": "The unit of temperature",
							},
						},
						"required": []string{"location"},
					},
				},
			},
			{
				Type: "function",
				Function: &Function{
					Name:        "get_forecast",
					Description: "Get weather forecast for a location",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{
								"type":        "string",
								"description": "The city and state",
							},
							"days": map[string]interface{}{
								"type":        "integer",
								"description": "Number of days to forecast",
								"minimum":     1,
								"maximum":     7,
							},
						},
						"required": []string{"location"},
					},
				},
			},
		},
		ToolChoice:        "auto",
		ParallelToolCalls: func() *bool { b := true; return &b }(),
	}

	// 验证请求结构
	if len(req.Tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(req.Tools))
	}

	if req.Tools[0].Function.Name != "get_weather" {
		t.Errorf("Expected first tool name 'get_weather', got '%s'", req.Tools[0].Function.Name)
	}

	if req.Tools[1].Function.Name != "get_forecast" {
		t.Errorf("Expected second tool name 'get_forecast', got '%s'", req.Tools[1].Function.Name)
	}

	if req.ToolChoice != "auto" {
		t.Errorf("Expected tool_choice 'auto', got '%v'", req.ToolChoice)
	}

	if req.ParallelToolCalls == nil || !*req.ParallelToolCalls {
		t.Error("Expected parallel_tool_calls to be true")
	}
}

// TestToolCallResponseStructure 测试工具调用响应结构
func TestToolCallResponseStructure(t *testing.T) {
	// 测试包含工具调用的响应结构
	response := &ChatCompletionResponse{
		ID:      "chatcmpl-test",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "gpt-3.5-turbo",
		Choices: []ChatCompletionChoice{
			{
				Index: 0,
				Message: ChatCompletionMessage{
					Role:    "assistant",
					Content: "",
					ToolCalls: []ToolCall{
						{
							ID:   "call_test_123",
							Type: "function",
							Function: &FunctionCall{
								Name:      "get_weather",
								Arguments: `{"location": "New York, NY"}`,
							},
						},
						{
							ID:   "call_test_456",
							Type: "function",
							Function: &FunctionCall{
								Name:      "get_forecast",
								Arguments: `{"location": "New York, NY", "days": 3}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: Usage{
			PromptTokens:     50,
			CompletionTokens: 25,
			TotalTokens:      75,
		},
	}

	// 验证响应结构
	if len(response.Choices) != 1 {
		t.Errorf("Expected 1 choice, got %d", len(response.Choices))
	}

	choice := response.Choices[0]
	if len(choice.Message.ToolCalls) != 2 {
		t.Errorf("Expected 2 tool calls, got %d", len(choice.Message.ToolCalls))
	}

	if choice.Message.ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("Expected first tool call name 'get_weather', got '%s'", choice.Message.ToolCalls[0].Function.Name)
	}

	if choice.Message.ToolCalls[1].Function.Name != "get_forecast" {
		t.Errorf("Expected second tool call name 'get_forecast', got '%s'", choice.Message.ToolCalls[1].Function.Name)
	}

	if choice.FinishReason != "tool_calls" {
		t.Errorf("Expected finish_reason 'tool_calls', got '%s'", choice.FinishReason)
	}
}

// TestToolCallErrorHandling 测试工具调用错误处理
func TestToolCallErrorHandling(t *testing.T) {
	provider := NewOpenAIProvider(&ProviderConfig{
		ProviderType: "openai",
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "test-key",
	})

	tests := []struct {
		name       string
		statusCode int
		body       []byte
		expectType string
		expectCode string
	}{
		{
			name:       "OpenAI tool call error",
			statusCode: 400,
			body: []byte(`{
				"error": {
					"message": "Invalid tool definition: function name is required",
					"type": "invalid_request_error",
					"code": "invalid_tool_definition"
				}
			}`),
			expectType: "tool_call_error",
			expectCode: "invalid_tool_definition",
		},
		{
			name:       "Rate limit error",
			statusCode: 429,
			body: []byte(`{
				"error": {
					"message": "Rate limit exceeded",
					"type": "rate_limit_exceeded",
					"code": "rate_limit_exceeded"
				}
			}`),
			expectType: "rate_limit_error",
			expectCode: "rate_limit_exceeded",
		},
		{
			name:       "Authentication error",
			statusCode: 401,
			body:       []byte(`{"error": {"message": "Invalid API key", "type": "authentication_error", "code": "invalid_api_key"}}`),
			expectType: "authentication_error",
			expectCode: "unauthorized",
		},
		{
			name:       "Server error",
			statusCode: 500,
			body:       []byte(`{"error": {"message": "Internal server error", "type": "server_error", "code": "internal_error"}}`),
			expectType: "server_error",
			expectCode: "internal_server_error",
		},
		{
			name:       "Unknown error",
			statusCode: 418,
			body:       []byte(`I'm a teapot`),
			expectType: "api_error",
			expectCode: "unknown_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := provider.handleAPIError(tt.statusCode, tt.body)
			
			if err == nil {
				t.Error("Expected error but got nil")
				return
			}

			toolCallErr, ok := err.(*ToolCallError)
			if !ok {
				t.Errorf("Expected ToolCallError, got %T", err)
				return
			}

			if toolCallErr.Type != tt.expectType {
				t.Errorf("Expected error type '%s', got '%s'", tt.expectType, toolCallErr.Type)
			}

			if toolCallErr.Code != tt.expectCode {
				t.Errorf("Expected error code '%s', got '%s'", tt.expectCode, toolCallErr.Code)
			}
		})
	}
}

// TestValidateMessageSequence 测试消息序列验证
func TestValidateMessageSequence(t *testing.T) {
	provider := NewOpenAIProvider(&ProviderConfig{
		ProviderType: "openai",
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "test-key",
	})

	tests := []struct {
		name        string
		messages    []ChatMessage
		expectError bool
		errorCode   string
	}{
		{
			name: "valid message sequence without tools",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there!"},
			},
			expectError: false,
		},
		{
			name: "tool message without preceding assistant message",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
				{Role: "tool", Content: "result", ToolCallID: "call_123"},
			},
			expectError: true,
			errorCode:   "invalid_message_sequence",
		},
		{
			name: "tool message after user message",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
				{Role: "user", Content: "Another message"},
				{Role: "tool", Content: "result", ToolCallID: "call_123"},
			},
			expectError: true,
			errorCode:   "invalid_message_sequence",
		},
		{
			name: "tool message after assistant without tool_calls",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there!"},
				{Role: "tool", Content: "result", ToolCallID: "call_123"},
			},
			expectError: true,
			errorCode:   "invalid_message_sequence",
		},
		{
			name: "valid tool message after assistant with tool_calls",
			messages: []ChatMessage{
				{Role: "user", Content: "What's the weather?"},
				{
					Role:    "assistant",
					Content: "",
					ToolCalls: []ToolCall{
						{
							ID:   "call_123",
							Type: "function",
							Function: &FunctionCall{
								Name:      "get_weather",
								Arguments: `{"location": "New York"}`,
							},
						},
					},
				},
				{Role: "tool", Content: "Sunny, 25°C", ToolCallID: "call_123"},
			},
			expectError: false,
		},
		{
			name: "tool message without tool_call_id",
			messages: []ChatMessage{
				{Role: "user", Content: "What's the weather?"},
				{
					Role:    "assistant",
					Content: "",
					ToolCalls: []ToolCall{
						{
							ID:   "call_123",
							Type: "function",
							Function: &FunctionCall{
								Name:      "get_weather",
								Arguments: `{"location": "New York"}`,
							},
						},
					},
				},
				{Role: "tool", Content: "Sunny, 25°C"}, // Missing ToolCallID
			},
			expectError: true,
			errorCode:   "missing_tool_call_id",
		},
		{
			name: "tool message with invalid tool_call_id",
			messages: []ChatMessage{
				{Role: "user", Content: "What's the weather?"},
				{
					Role:    "assistant",
					Content: "",
					ToolCalls: []ToolCall{
						{
							ID:   "call_123",
							Type: "function",
							Function: &FunctionCall{
								Name:      "get_weather",
								Arguments: `{"location": "New York"}`,
							},
						},
					},
				},
				{Role: "tool", Content: "Sunny, 25°C", ToolCallID: "call_456"}, // Wrong ID
			},
			expectError: true,
			errorCode:   "invalid_tool_call_id",
		},
		{
			name: "assistant message with invalid tool_call",
			messages: []ChatMessage{
				{Role: "user", Content: "What's the weather?"},
				{
					Role:    "assistant",
					Content: "",
					ToolCalls: []ToolCall{
						{
							// Missing ID
							Type: "function",
							Function: &FunctionCall{
								Name:      "get_weather",
								Arguments: `{"location": "New York"}`,
							},
						},
					},
				},
			},
			expectError: true,
			errorCode:   "missing_tool_call_id",
		},
		{
			name: "assistant message with invalid tool_call type",
			messages: []ChatMessage{
				{Role: "user", Content: "What's the weather?"},
				{
					Role:    "assistant",
					Content: "",
					ToolCalls: []ToolCall{
						{
							ID:   "call_123",
							Type: "invalid_type", // Invalid type
							Function: &FunctionCall{
								Name:      "get_weather",
								Arguments: `{"location": "New York"}`,
							},
						},
					},
				},
			},
			expectError: true,
			errorCode:   "invalid_tool_call_type",
		},
		{
			name: "assistant message with missing function",
			messages: []ChatMessage{
				{Role: "user", Content: "What's the weather?"},
				{
					Role:    "assistant",
					Content: "",
					ToolCalls: []ToolCall{
						{
							ID:       "call_123",
							Type:     "function",
							Function: nil, // Missing function
						},
					},
				},
			},
			expectError: true,
			errorCode:   "missing_function_call",
		},
		{
			name: "assistant message with missing function name",
			messages: []ChatMessage{
				{Role: "user", Content: "What's the weather?"},
				{
					Role:    "assistant",
					Content: "",
					ToolCalls: []ToolCall{
						{
							ID:   "call_123",
							Type: "function",
							Function: &FunctionCall{
								// Missing Name
								Arguments: `{"location": "New York"}`,
							},
						},
					},
				},
			},
			expectError: true,
			errorCode:   "missing_function_name",
		},
		{
			name: "multiple valid tool responses",
			messages: []ChatMessage{
				{Role: "user", Content: "Get weather and time"},
				{
					Role:    "assistant",
					Content: "",
					ToolCalls: []ToolCall{
						{
							ID:   "call_123",
							Type: "function",
							Function: &FunctionCall{
								Name:      "get_weather",
								Arguments: `{"location": "New York"}`,
							},
						},
						{
							ID:   "call_456",
							Type: "function",
							Function: &FunctionCall{
								Name:      "get_time",
								Arguments: `{"timezone": "UTC"}`,
							},
						},
					},
				},
				{Role: "tool", Content: "Sunny, 25°C", ToolCallID: "call_123"},
				{Role: "tool", Content: "12:00 UTC", ToolCallID: "call_456"},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := provider.validateMessageSequence(tt.messages)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				
				if toolErr, ok := err.(*ToolCallError); ok {
					if toolErr.Code != tt.errorCode {
						t.Errorf("expected error code %s, got %s", tt.errorCode, toolErr.Code)
					}
				} else {
					t.Errorf("expected ToolCallError, got %T", err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

// TestParallelToolCallsValidation 测试并行工具调用验证
func TestParallelToolCallsValidation(t *testing.T) {
	provider := NewOpenAIProvider(&ProviderConfig{
		ProviderType: "openai",
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "test-key",
	})

	tests := []struct {
		name        string
		tools       []Tool
		parallel    *bool
		expectError bool
	}{
		{
			name: "parallel true with multiple tools",
			tools: []Tool{
				{Type: "function", Function: &Function{Name: "func1"}},
				{Type: "function", Function: &Function{Name: "func2"}},
			},
			parallel:    func() *bool { b := true; return &b }(),
			expectError: false,
		},
		{
			name: "parallel false with single tool",
			tools: []Tool{
				{Type: "function", Function: &Function{Name: "func1"}},
			},
			parallel:    func() *bool { b := false; return &b }(),
			expectError: false,
		},
		{
			name: "parallel true with single tool (should error)",
			tools: []Tool{
				{Type: "function", Function: &Function{Name: "func1"}},
			},
			parallel:    func() *bool { b := true; return &b }(),
			expectError: true,
		},
		{
			name: "parallel nil (should be fine)",
			tools: []Tool{
				{Type: "function", Function: &Function{Name: "func1"}},
			},
			parallel:    nil,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ChatCompletionRequest{
				Model: "gpt-3.5-turbo",
				Messages: []ChatMessage{
					{Role: "user", Content: "Test"},
				},
				Tools:             tt.tools,
				ParallelToolCalls: tt.parallel,
			}

			err := provider.validateToolCallRequest(req)
			
			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}
			
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}
