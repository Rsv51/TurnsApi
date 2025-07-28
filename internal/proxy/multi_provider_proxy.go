package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"turnsapi/internal"
	"turnsapi/internal/keymanager"
	"turnsapi/internal/logger"
	"turnsapi/internal/providers"
	"turnsapi/internal/router"

	"github.com/gin-gonic/gin"
)

// MultiProviderProxy 多提供商代理
type MultiProviderProxy struct {
	config          *internal.Config
	keyManager      *keymanager.MultiGroupKeyManager
	providerManager *providers.ProviderManager
	providerRouter  *router.ProviderRouter
	requestLogger   *logger.RequestLogger
}

// NewMultiProviderProxy 创建多提供商代理
func NewMultiProviderProxy(
	config *internal.Config,
	keyManager *keymanager.MultiGroupKeyManager,
	requestLogger *logger.RequestLogger,
) *MultiProviderProxy {
	// 创建提供商管理器
	factory := providers.NewDefaultProviderFactory()
	providerManager := providers.NewProviderManager(factory)
	
	// 创建提供商路由器
	providerRouter := router.NewProviderRouter(config, providerManager)

	return &MultiProviderProxy{
		config:          config,
		keyManager:      keyManager,
		providerManager: providerManager,
		providerRouter:  providerRouter,
		requestLogger:   requestLogger,
	}
}

// RemoveProvider 从提供商管理器中移除分组
func (mp *MultiProviderProxy) RemoveProvider(groupID string) {
	mp.providerManager.RemoveProvider(groupID)
}

// HandleChatCompletion 处理聊天完成请求
func (p *MultiProviderProxy) HandleChatCompletion(c *gin.Context) {
	startTime := time.Now()

	// 解析请求
	var req providers.ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("Failed to parse request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request format",
				"type":    "invalid_request_error",
				"code":    "invalid_json",
			},
		})
		return
	}

	// 检查必需字段
	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Model is required",
				"type":    "invalid_request_error",
				"code":    "missing_model",
			},
		})
		return
	}

	if len(req.Messages) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Messages are required",
				"type":    "invalid_request_error",
				"code":    "missing_messages",
			},
		})
		return
	}

	// 获取代理密钥信息以检查权限
	var allowedGroups []string
	if keyInfo, exists := c.Get("key_info"); exists {
		if proxyKey, ok := keyInfo.(*logger.ProxyKey); ok {
			allowedGroups = proxyKey.AllowedGroups
		}
	}

	// 路由到合适的提供商
	routeReq := &router.RouteRequest{
		Model:         req.Model,
		AllowedGroups: allowedGroups, // 传递代理密钥的权限限制
	}

	// 检查是否有显式指定的提供商分组
	if providerGroup := c.GetHeader("X-Provider-Group"); providerGroup != "" {
		routeReq.ProviderGroup = providerGroup
	}

	// 使用智能路由重试机制
	success := p.handleRequestWithRetry(c, &req, routeReq, startTime)
	if !success {
		// 如果所有重试都失败了，返回错误
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"message": "All provider groups failed to process the request",
				"type":    "service_unavailable",
				"code":    "all_providers_failed",
			},
		})
	}
}

// handleRequestWithRetry 处理请求并支持智能重试
func (p *MultiProviderProxy) handleRequestWithRetry(
	c *gin.Context,
	req *providers.ChatCompletionRequest,
	routeReq *router.RouteRequest,
	startTime time.Time,
) bool {
	const maxRetries = 3

	for attempt := 0; attempt < maxRetries; attempt++ {
		// 获取路由结果
		routeResult, err := p.providerRouter.RouteWithRetry(routeReq)
		if err != nil {
			log.Printf("Failed to route request (attempt %d/%d): %v", attempt+1, maxRetries, err)
			if attempt == maxRetries-1 {
				// 最后一次尝试失败
				return false
			}
			continue
		}

		// 获取API密钥
		apiKey, err := p.keyManager.GetNextKeyForGroup(routeResult.GroupID)
		if err != nil {
			log.Printf("Failed to get API key for group %s (attempt %d/%d): %v", routeResult.GroupID, attempt+1, maxRetries, err)
			// 报告失败
			p.providerRouter.ReportFailure(req.Model, routeResult.GroupID)
			if attempt == maxRetries-1 {
				return false
			}
			continue
		}

		// 更新提供商配置中的API密钥
		p.providerRouter.UpdateProviderConfig(routeResult.ProviderConfig, apiKey)

		// 尝试处理请求
		var success bool
		if req.Stream {
			success = p.handleStreamingRequestWithRetry(c, req, routeResult, apiKey, startTime)
		} else {
			success = p.handleNonStreamingRequestWithRetry(c, req, routeResult, apiKey, startTime)
		}

		if success {
			// 报告成功
			p.providerRouter.ReportSuccess(req.Model, routeResult.GroupID)
			return true
		} else {
			// 报告失败
			p.providerRouter.ReportFailure(req.Model, routeResult.GroupID)
			log.Printf("Request failed for group %s (attempt %d/%d)", routeResult.GroupID, attempt+1, maxRetries)
		}
	}

	return false
}

// handleNonStreamingRequestWithRetry 处理非流式请求（支持重试）
func (p *MultiProviderProxy) handleNonStreamingRequestWithRetry(
	c *gin.Context,
	req *providers.ChatCompletionRequest,
	routeResult *router.RouteResult,
	apiKey string,
	startTime time.Time,
) bool {
	return p.handleNonStreamingRequest(c, req, routeResult, apiKey, startTime)
}

// handleNonStreamingRequest 处理非流式请求
func (p *MultiProviderProxy) handleNonStreamingRequest(
	c *gin.Context,
	req *providers.ChatCompletionRequest,
	routeResult *router.RouteResult,
	apiKey string,
	startTime time.Time,
) bool {
	// 创建带有长超时的context，避免请求超时
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// 应用分组的请求参数覆盖
	req.ApplyRequestParams(routeResult.ProviderConfig.RequestParams)

	// 应用模型名称映射
	originalModel := req.Model
	req.Model = p.providerRouter.ResolveModelName(req.Model, routeResult.GroupID)

	// 发送请求到提供商
	response, err := routeResult.Provider.ChatCompletion(ctx, req)

	// 恢复原始模型名称用于日志记录
	req.Model = originalModel
	if err != nil {
		log.Printf("Provider request failed: %v", err)
		p.keyManager.ReportError(routeResult.GroupID, apiKey, err.Error())
		
		// 记录错误日志
		if p.requestLogger != nil {
			proxyKeyName, proxyKeyID := p.getProxyKeyInfo(c)
			reqBody, _ := json.Marshal(req)
			clientIP := logger.GetClientIP(c)
			p.requestLogger.LogRequest(proxyKeyName, proxyKeyID, routeResult.GroupID, apiKey, req.Model, string(reqBody), "", clientIP, 502, false, time.Since(startTime), err)
		}
		
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"message": "Failed to connect to provider",
				"type":    "connection_error",
				"code":    "upstream_error",
			},
		})
		return false
	}

	// 报告成功
	p.keyManager.ReportSuccess(routeResult.GroupID, apiKey)

	// 记录成功日志
	if p.requestLogger != nil {
		proxyKeyName, proxyKeyID := p.getProxyKeyInfo(c)
		reqBody, _ := json.Marshal(req)
		respBody, _ := json.Marshal(response)
		clientIP := logger.GetClientIP(c)
		p.requestLogger.LogRequest(proxyKeyName, proxyKeyID, routeResult.GroupID, apiKey, req.Model, string(reqBody), string(respBody), clientIP, 200, false, time.Since(startTime), nil)
	}

	// 返回响应
	c.JSON(http.StatusOK, response)
	return true
}

// handleStreamingRequestWithRetry 处理流式请求（支持重试）
func (p *MultiProviderProxy) handleStreamingRequestWithRetry(
	c *gin.Context,
	req *providers.ChatCompletionRequest,
	routeResult *router.RouteResult,
	apiKey string,
	startTime time.Time,
) bool {
	return p.handleStreamingRequest(c, req, routeResult, apiKey, startTime)
}

// handleStreamingRequest 处理流式请求
func (p *MultiProviderProxy) handleStreamingRequest(
	c *gin.Context,
	req *providers.ChatCompletionRequest,
	routeResult *router.RouteResult,
	apiKey string,
	startTime time.Time,
) bool {
	// 创建带有长超时的context，避免流式请求超时
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// 应用分组的请求参数覆盖
	req.ApplyRequestParams(routeResult.ProviderConfig.RequestParams)

	// 应用模型名称映射
	originalModel := req.Model
	req.Model = p.providerRouter.ResolveModelName(req.Model, routeResult.GroupID)

	// 设置流式响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	// 获取流式响应
	streamChan, err := routeResult.Provider.ChatCompletionStream(ctx, req)

	// 恢复原始模型名称用于日志记录
	req.Model = originalModel
	if err != nil {
		log.Printf("Provider streaming request failed: %v", err)
		p.keyManager.ReportError(routeResult.GroupID, apiKey, err.Error())
		
		// 记录错误日志
		if p.requestLogger != nil {
			proxyKeyName, proxyKeyID := p.getProxyKeyInfo(c)
			reqBody, _ := json.Marshal(req)
			clientIP := logger.GetClientIP(c)
			p.requestLogger.LogRequest(proxyKeyName, proxyKeyID, routeResult.GroupID, apiKey, req.Model, string(reqBody), "", clientIP, 502, true, time.Since(startTime), err)
		}
		
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"message": "Failed to connect to provider",
				"type":    "connection_error",
				"code":    "upstream_error",
			},
		})
		return false
	}

	// 获取响应写入器
	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Printf("Streaming not supported")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Streaming not supported",
				"type":    "internal_error",
			},
		})
		return false
	}

	// 处理流式数据
	hasData := false
	responseBuffer := make([]byte, 0, 1024)

	for streamResp := range streamChan {
		if streamResp.Error != nil {
			log.Printf("Stream error: %v", streamResp.Error)
			p.keyManager.ReportError(routeResult.GroupID, apiKey, streamResp.Error.Error())
			break
		}

		if len(streamResp.Data) > 0 {
			hasData = true
			w.Write(streamResp.Data)
			flusher.Flush()
			
			// 收集响应数据用于日志记录（限制大小）
			if len(responseBuffer) < 10000 {
				responseBuffer = append(responseBuffer, streamResp.Data...)
			}
		}

		if streamResp.Done {
			break
		}
	}

	duration := time.Since(startTime)

	// 如果接收到数据，报告成功
	if hasData {
		p.keyManager.ReportSuccess(routeResult.GroupID, apiKey)

		// 记录成功日志
		if p.requestLogger != nil {
			proxyKeyName, proxyKeyID := p.getProxyKeyInfo(c)
			reqBody, _ := json.Marshal(req)
			clientIP := logger.GetClientIP(c)
			p.requestLogger.LogRequest(proxyKeyName, proxyKeyID, routeResult.GroupID, apiKey, req.Model, string(reqBody), string(responseBuffer), clientIP, 200, true, duration, nil)
		}
		return true
	}

	return false
}

// getProxyKeyInfo 获取代理密钥信息
func (p *MultiProviderProxy) getProxyKeyInfo(c *gin.Context) (string, string) {
	if name, exists := c.Get("proxy_key_name"); exists {
		if nameStr, ok := name.(string); ok {
			if id, exists := c.Get("proxy_key_id"); exists {
				if idStr, ok := id.(string); ok {
					return nameStr, idStr
				}
			}
			return nameStr, "unknown"
		}
	}
	return "Unknown", "unknown"
}

// GetProviderRouter 获取提供商路由器
func (p *MultiProviderProxy) GetProviderRouter() *router.ProviderRouter {
	return p.providerRouter
}

// GetProviderManager 获取提供商管理器
func (p *MultiProviderProxy) GetProviderManager() *providers.ProviderManager {
	return p.providerManager
}

// HandleModels 处理模型列表请求
func (p *MultiProviderProxy) HandleModels(c *gin.Context) {
	// 检查是否指定了特定的提供商分组
	groupID := c.Query("provider_group")

	if groupID != "" {
		// 获取特定分组的模型
		p.handleGroupModels(c, groupID)
	} else {
		// 获取所有分组的模型
		p.handleAllModels(c)
	}
}

// handleGroupModels 处理特定分组的模型列表请求
func (p *MultiProviderProxy) handleGroupModels(c *gin.Context, groupID string) {
	// 获取分组信息
	group, exists := p.providerRouter.GetGroupInfo(groupID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Provider group '%s' not found", groupID),
				"type":    "invalid_request_error",
				"code":    "group_not_found",
			},
		})
		return
	}

	if !group.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Provider group '%s' is disabled", groupID),
				"type":    "invalid_request_error",
				"code":    "group_disabled",
			},
		})
		return
	}

	// 获取API密钥
	apiKey, err := p.keyManager.GetNextKeyForGroup(groupID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": gin.H{
				"message": "No available API keys for this group",
				"type":    "service_unavailable",
				"code":    "no_api_keys",
			},
		})
		return
	}

	// 创建提供商配置
	providerConfig := &providers.ProviderConfig{
		BaseURL:       group.BaseURL,
		APIKey:        apiKey,
		Timeout:       group.Timeout,
		MaxRetries:    group.MaxRetries,
		Headers:       group.Headers,
		ProviderType:  group.ProviderType,
		RequestParams: group.RequestParams,
	}

	// 获取提供商实例
	provider, err := p.providerManager.GetProvider(groupID, providerConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to get provider instance",
				"type":    "internal_error",
				"code":    "provider_error",
			},
		})
		return
	}

	// 获取模型列表
	ctx := c.Request.Context()
	rawModels, err := provider.GetModels(ctx)
	if err != nil {
		log.Printf("Failed to get models from provider %s: %v", groupID, err)
		p.keyManager.ReportError(groupID, apiKey, err.Error())
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"message": "Failed to get models from provider",
				"type":    "connection_error",
				"code":    "upstream_error",
			},
		})
		return
	}

	// 报告成功
	p.keyManager.ReportSuccess(groupID, apiKey)

	// 标准化模型数据格式
	standardizedModels := p.standardizeModelsResponse(rawModels, group.ProviderType)

	// 添加模型别名到模型列表中
	var enhancedModels interface{}
	if modelSlice, ok := standardizedModels.([]map[string]interface{}); ok {
		enhancedModels = p.addModelAliases(modelSlice, groupID)
	} else {
		enhancedModels = standardizedModels
	}

	// 为了与前端期望的格式一致，将单个提供商的响应包装成与所有提供商相同的格式
	response := gin.H{
		"object": "list",
		"data": map[string]interface{}{
			groupID: map[string]interface{}{
				"group_name":    group.Name,
				"provider_type": group.ProviderType,
				"models":        enhancedModels,
			},
		},
	}

	// 返回模型列表
	c.JSON(http.StatusOK, response)
}

// handleAllModels 处理所有分组的模型列表请求
func (p *MultiProviderProxy) handleAllModels(c *gin.Context) {
	allModels := make(map[string]interface{})

	// 获取所有启用的分组
	enabledGroups := p.providerRouter.GetAvailableGroups()

	for groupID, group := range enabledGroups {
		// 获取API密钥
		apiKey, err := p.keyManager.GetNextKeyForGroup(groupID)
		if err != nil {
			log.Printf("Failed to get API key for group %s: %v", groupID, err)
			continue
		}

		// 创建提供商配置
		providerConfig := &providers.ProviderConfig{
			BaseURL:       group.BaseURL,
			APIKey:        apiKey,
			Timeout:       group.Timeout,
			MaxRetries:    group.MaxRetries,
			Headers:       group.Headers,
			ProviderType:  group.ProviderType,
			RequestParams: group.RequestParams,
		}

		// 获取提供商实例
		provider, err := p.providerManager.GetProvider(groupID, providerConfig)
		if err != nil {
			log.Printf("Failed to get provider for group %s: %v", groupID, err)
			continue
		}

		// 获取模型列表
		ctx := c.Request.Context()
		rawModels, err := provider.GetModels(ctx)
		if err != nil {
			log.Printf("Failed to get models from provider %s: %v", groupID, err)
			p.keyManager.ReportError(groupID, apiKey, err.Error())
			continue
		}

		// 报告成功
		p.keyManager.ReportSuccess(groupID, apiKey)

		// 标准化模型数据格式
		standardizedModels := p.standardizeModelsResponse(rawModels, group.ProviderType)

		// 添加模型别名到模型列表中
		var enhancedModels interface{}
		if modelSlice, ok := standardizedModels.([]map[string]interface{}); ok {
			enhancedModels = p.addModelAliases(modelSlice, groupID)
		} else {
			enhancedModels = standardizedModels
		}

		// 添加到结果中
		allModels[groupID] = map[string]interface{}{
			"group_name":    group.Name,
			"provider_type": group.ProviderType,
			"models":        enhancedModels,
		}
	}

	// 返回所有模型
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   allModels,
	})
}

// StandardizeModelsResponse 标准化不同提供商的模型响应格式（公开方法）
func (p *MultiProviderProxy) StandardizeModelsResponse(rawModels interface{}, providerType string) interface{} {
	return p.standardizeModelsResponse(rawModels, providerType)
}

// standardizeModelsResponse 标准化不同提供商的模型响应格式
func (p *MultiProviderProxy) standardizeModelsResponse(rawModels interface{}, providerType string) interface{} {
	switch providerType {
	case "openai", "azure_openai":
		// OpenAI格式已经是标准格式
		return rawModels

	case "gemini":
		// Gemini格式需要转换
		return p.standardizeGeminiModels(rawModels)

	case "anthropic":
		// Anthropic格式需要转换
		return p.standardizeAnthropicModels(rawModels)

	default:
		// 默认尝试OpenAI格式
		return rawModels
	}
}

// standardizeGeminiModels 标准化Gemini模型响应
func (p *MultiProviderProxy) standardizeGeminiModels(rawModels interface{}) interface{} {
	// 尝试解析Gemini响应格式
	if modelsMap, ok := rawModels.(map[string]interface{}); ok {
		if modelsArray, exists := modelsMap["models"]; exists {
			if models, ok := modelsArray.([]interface{}); ok {
				// 转换为OpenAI格式
				standardModels := make([]map[string]interface{}, 0)
				for _, model := range models {
					if modelMap, ok := model.(map[string]interface{}); ok {
						// 提取模型名称
						var modelID string
						if name, exists := modelMap["name"]; exists {
							if nameStr, ok := name.(string); ok {
								// Gemini模型名称格式: "models/gemini-pro"
								parts := strings.Split(nameStr, "/")
								if len(parts) > 1 {
									modelID = parts[len(parts)-1]
								} else {
									modelID = nameStr
								}
							}
						}

						if modelID != "" {
							standardModel := map[string]interface{}{
								"id":       modelID,
								"object":   "model",
								"owned_by": "google",
							}

							// 添加其他可用信息
							if displayName, exists := modelMap["displayName"]; exists {
								standardModel["display_name"] = displayName
							}
							if description, exists := modelMap["description"]; exists {
								standardModel["description"] = description
							}

							standardModels = append(standardModels, standardModel)
						}
					}
				}

				return map[string]interface{}{
					"object": "list",
					"data":   standardModels,
				}
			}
		}
	}

	// 如果解析失败，返回空列表
	return map[string]interface{}{
		"object": "list",
		"data":   []interface{}{},
	}
}

// standardizeAnthropicModels 标准化Anthropic模型响应
func (p *MultiProviderProxy) standardizeAnthropicModels(rawModels interface{}) interface{} {
	// Anthropic通常不提供模型列表API，返回预定义的模型
	predefinedModels := []map[string]interface{}{
		{
			"id":       "claude-3-sonnet-20240229",
			"object":   "model",
			"owned_by": "anthropic",
		},
		{
			"id":       "claude-3-opus-20240229",
			"object":   "model",
			"owned_by": "anthropic",
		},
		{
			"id":       "claude-3-haiku-20240307",
			"object":   "model",
			"owned_by": "anthropic",
		},
		{
			"id":       "claude-2.1",
			"object":   "model",
			"owned_by": "anthropic",
		},
		{
			"id":       "claude-2.0",
			"object":   "model",
			"owned_by": "anthropic",
		},
	}

	return map[string]interface{}{
		"object": "list",
		"data":   predefinedModels,
	}
}

// getProviderGroup 获取提供商分组信息
func (p *MultiProviderProxy) getProviderGroup(c *gin.Context, model string) string {
	// 尝试从上下文中获取分组信息
	if groupID, exists := c.Get("provider_group"); exists {
		if groupStr, ok := groupID.(string); ok {
			return groupStr
		}
	}

	// 如果上下文中没有，尝试根据模型推断分组
	if group, groupID := p.config.GetGroupByModel(model); group != nil {
		return groupID
	}

	// 默认返回空字符串
	return ""
}

// addModelAliases 为模型列表添加别名信息
func (p *MultiProviderProxy) addModelAliases(models []map[string]interface{}, groupID string) []map[string]interface{} {
	group, exists := p.config.UserGroups[groupID]
	if !exists || len(group.ModelMappings) == 0 {
		return models
	}

	var enhancedModels []map[string]interface{}

	// 处理每个原始模型
	for _, model := range models {
		modelID, ok := model["id"].(string)
		if !ok {
			enhancedModels = append(enhancedModels, model)
			continue
		}

		// 检查是否有别名映射到这个原始模型
		var aliases []string
		for alias, originalModel := range group.ModelMappings {
			if originalModel == modelID {
				aliases = append(aliases, alias)
			}
		}

		if len(aliases) > 0 {
			// 如果有别名，为每个别名创建条目
			for _, alias := range aliases {
				aliasModel := make(map[string]interface{})
				for k, v := range model {
					aliasModel[k] = v
				}
				aliasModel["id"] = alias
				aliasModel["original_model"] = modelID
				aliasModel["is_alias"] = true
				enhancedModels = append(enhancedModels, aliasModel)
			}

			// 也保留原始模型，但标记它有别名
			originalModel := make(map[string]interface{})
			for k, v := range model {
				originalModel[k] = v
			}
			originalModel["has_aliases"] = aliases
			originalModel["is_original"] = true
			enhancedModels = append(enhancedModels, originalModel)
		} else {
			// 没有别名的模型直接添加
			enhancedModels = append(enhancedModels, model)
		}
	}

	// 添加那些没有对应原始模型的别名（可能是跨分组映射）
	for alias, originalModel := range group.ModelMappings {
		// 检查原始模型是否在当前模型列表中
		found := false
		for _, model := range models {
			if modelID, ok := model["id"].(string); ok && modelID == originalModel {
				found = true
				break
			}
		}

		// 如果原始模型不在当前列表中，创建一个别名条目
		if !found {
			aliasModel := map[string]interface{}{
				"id":             alias,
				"object":         "model",
				"created":        0,
				"owned_by":       "alias",
				"original_model": originalModel,
				"is_alias":       true,
				"cross_group":    true, // 标记为跨分组映射
			}
			enhancedModels = append(enhancedModels, aliasModel)
		}
	}

	return enhancedModels
}
