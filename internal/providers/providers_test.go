package providers

import (
	"context"
	"testing"
	"time"
)

func TestProviderFactory(t *testing.T) {
	factory := NewDefaultProviderFactory()

	// Test supported types
	supportedTypes := factory.GetSupportedTypes()
	expectedTypes := []string{"openai", "gemini", "anthropic", "azure_openai"}

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
		APIVersion:   "2023-06-01",
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
