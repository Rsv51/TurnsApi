package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"turnsapi/internal"
	"turnsapi/internal/auth"
	"turnsapi/internal/health"
	"turnsapi/internal/keymanager"
	"turnsapi/internal/logger"
	"turnsapi/internal/providers"
	"turnsapi/internal/proxy"
	"turnsapi/internal/proxykey"

	"github.com/gin-gonic/gin"
)

// MultiProviderServer 多提供商HTTP服务器
type MultiProviderServer struct {
	configManager       *internal.ConfigManager
	config              *internal.Config
	keyManager          *keymanager.MultiGroupKeyManager
	proxy               *proxy.MultiProviderProxy
	authManager         *auth.AuthManager
	proxyKeyManager     *proxykey.Manager
	requestLogger       *logger.RequestLogger
	healthChecker       *health.MultiProviderHealthChecker
	router              *gin.Engine
	httpServer          *http.Server
	startTime           time.Time
}

// NewMultiProviderServer 创建新的多提供商服务器
func NewMultiProviderServer(configManager *internal.ConfigManager, keyManager *keymanager.MultiGroupKeyManager) *MultiProviderServer {
	config := configManager.GetConfig()

	log.Printf("=== 开始创建MultiProviderServer ===")
	log.Printf("配置的服务器模式: '%s', 日志级别: '%s'", config.Server.Mode, config.Logging.Level)

	// 设置Gin模式
	// 优先使用Server.Mode配置，如果未设置则根据日志级别判断
	var ginMode string
	switch config.Server.Mode {
	case "debug":
		ginMode = gin.DebugMode
	case "release":
		ginMode = gin.ReleaseMode
	case "test":
		ginMode = gin.TestMode
	default:
		// 向后兼容：如果Mode未设置或无效，则根据日志级别判断
		if config.Logging.Level == "debug" {
			ginMode = gin.DebugMode
		} else {
			ginMode = gin.ReleaseMode
		}
	}

	// 设置环境变量（Gin优先检查环境变量）
	os.Setenv("GIN_MODE", ginMode)
	gin.SetMode(ginMode)
	log.Printf("Gin模式设置为: %s", ginMode)

	// 创建请求日志记录器
	requestLogger, err := logger.NewRequestLogger(config.Database.Path)
	if err != nil {
		log.Printf("Failed to create request logger: %v", err)
	}

	// 创建代理密钥管理器
	proxyKeyManager := proxykey.NewManagerWithDB(requestLogger)

	server := &MultiProviderServer{
		configManager:   configManager,
		config:          config,
		keyManager:      keyManager,
		authManager:     auth.NewAuthManager(config),
		proxyKeyManager: proxyKeyManager,
		requestLogger:   requestLogger,
		router:          gin.New(),
		startTime:       time.Now(),
	}

	// 创建多提供商代理
	server.proxy = proxy.NewMultiProviderProxy(config, keyManager, requestLogger)

	// 创建健康检查器
	factory := providers.NewDefaultProviderFactory()
	providerManager := providers.NewProviderManager(factory)
	server.healthChecker = health.NewMultiProviderHealthChecker(config, keyManager, providerManager, server.proxy.GetProviderRouter())

	// 设置代理密钥管理器到认证管理器
	server.authManager.SetProxyKeyManager(server.proxyKeyManager)

	// 设置中间件
	server.setupMiddleware()

	// 设置路由
	server.setupRoutes()

	return server
}

// setupMiddleware 设置中间件
func (s *MultiProviderServer) setupMiddleware() {
	// 日志中间件
	s.router.Use(gin.Logger())
	s.router.Use(gin.Recovery())

	// CORS中间件
	s.router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Provider-Group")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})
}

// setupRoutes 设置路由
func (s *MultiProviderServer) setupRoutes() {
	// API路由（需要API密钥认证）
	api := s.router.Group("/v1")
	api.Use(s.authManager.APIKeyAuthMiddleware())
	{
		api.POST("/chat/completions", s.handleChatCompletions)
		api.GET("/models", s.handleModels)
	}

	// 兼容OpenAI API路径
	s.router.POST("/chat/completions", s.authManager.APIKeyAuthMiddleware(), s.handleChatCompletions)
	s.router.GET("/models", s.authManager.APIKeyAuthMiddleware(), s.handleModels)

	// 管理API（需要HTTP Basic认证）
	admin := s.router.Group("/admin")
	admin.Use(s.authManager.AuthMiddleware())
	{
		// 系统状态
		admin.GET("/status", s.handleStatus)
		
		// 健康检查
		admin.GET("/health/system", s.handleSystemHealth)
		admin.GET("/health/providers", s.handleProvidersHealth)
		admin.GET("/health/providers/:groupId", s.handleProviderHealth)
		
		// 密钥管理
		admin.GET("/groups", s.handleGroupsStatus)
		admin.GET("/groups/:groupId/keys", s.handleGroupKeysStatus)
		
		// 模型管理
		admin.GET("/models", s.handleAllModels)
		admin.GET("/models/:groupId", s.handleGroupModels)
		admin.POST("/models/test", s.handleTestModels)
		admin.GET("/models/available/:groupId", s.handleAvailableModels)
		admin.POST("/models/available/by-type", s.handleAvailableModelsByType)
		admin.POST("/keys/validate/:groupId", s.handleValidateKeys)
		admin.POST("/keys/validate", s.handleValidateKeysWithoutGroup)
		admin.GET("/keys/status", s.handleKeysStatus)
		admin.GET("/keys/validation/:groupId", s.handleGetKeyValidationStatus)
		
		// 日志管理
		admin.GET("/logs", s.handleLogs)
		admin.GET("/logs/:id", s.handleLogDetail)
		admin.GET("/logs/stats/api-keys", s.handleAPIKeyStats)
		admin.GET("/logs/stats/models", s.handleModelStats)
		
		// 代理密钥管理
		admin.GET("/proxy-keys", s.handleProxyKeys)
		admin.POST("/proxy-keys", s.handleGenerateProxyKey)
		admin.DELETE("/proxy-keys/:id", s.handleDeleteProxyKey)

		// 分组管理
		admin.GET("/groups/manage", s.handleGroupsManage)
		admin.POST("/groups", s.handleCreateGroup)
		admin.PUT("/groups/:groupId", s.handleUpdateGroup)
		admin.DELETE("/groups/:groupId", s.handleDeleteGroup)
		admin.POST("/groups/:groupId/toggle", s.handleToggleGroup)
	}

	// Web认证
	s.router.GET("/auth/login", s.authManager.HandleLoginPage)
	s.router.POST("/auth/login", s.authManager.HandleLogin)
	s.router.POST("/auth/logout", s.authManager.HandleLogout)

	// 静态文件
	s.router.Static("/static", "./web/static")
	s.router.LoadHTMLGlob("web/templates/*")

	// Web界面（需要Web认证）
	s.router.GET("/", s.authManager.WebAuthMiddleware(), s.handleIndex)
	s.router.GET("/dashboard", s.authManager.WebAuthMiddleware(), s.handleMultiProviderDashboard)
	s.router.GET("/logs", s.authManager.WebAuthMiddleware(), s.handleLogsPage)
	s.router.GET("/groups", s.authManager.WebAuthMiddleware(), s.handleGroupsManagePage)

	// 健康检查（不需要认证）
	s.router.GET("/health", s.handleHealth)
}

// handleChatCompletions 处理聊天完成请求
func (s *MultiProviderServer) handleChatCompletions(c *gin.Context) {
	// 增加请求计数
	s.healthChecker.IncrementRequestCount()
	s.proxy.HandleChatCompletion(c)
}

// handleModels 处理模型列表请求
func (s *MultiProviderServer) handleModels(c *gin.Context) {
	// 获取代理密钥信息
	keyInfo, exists := c.Get("key_info")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"message": "Authentication required",
				"type":    "authentication_error",
				"code":    "missing_key_info",
			},
		})
		return
	}

	// 转换为ProxyKey类型
	proxyKey, ok := keyInfo.(*logger.ProxyKey)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Invalid key information",
				"type":    "internal_error",
				"code":    "invalid_key_info",
			},
		})
		return
	}

	// 检查是否指定了特定的提供商分组
	groupID := c.Query("provider_group")

	if groupID != "" {
		// 检查代理密钥是否有访问指定分组的权限
		if !s.hasGroupAccess(proxyKey, groupID) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"message": fmt.Sprintf("Access denied to provider group '%s'", groupID),
					"type":    "permission_error",
					"code":    "group_access_denied",
				},
			})
			return
		}
	}

	// 获取并返回标准OpenAI格式的模型列表
	s.handleOpenAIModels(c, proxyKey, groupID)
}



// handleOpenAIModels 处理OpenAI格式的模型列表请求
func (s *MultiProviderServer) handleOpenAIModels(c *gin.Context, proxyKey *logger.ProxyKey, groupID string) {
	// 调试日志
	log.Printf("代理密钥权限: ID=%s, AllowedGroups=%v", proxyKey.ID, proxyKey.AllowedGroups)

	// 获取所有启用的分组
	enabledGroups := s.proxy.GetProviderRouter().GetAvailableGroups()
	log.Printf("启用的分组: %v", func() []string {
		var groups []string
		for id := range enabledGroups {
			groups = append(groups, id)
		}
		return groups
	}())

	// 根据代理密钥权限和查询参数过滤分组
	var accessibleGroups map[string]*internal.UserGroup

	if groupID != "" {
		// 如果指定了特定分组，只返回该分组的模型
		if group, exists := enabledGroups[groupID]; exists {
			accessibleGroups = map[string]*internal.UserGroup{groupID: group}
		} else {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"message": fmt.Sprintf("Provider group '%s' not found", groupID),
					"type":    "not_found",
					"code":    "group_not_found",
				},
			})
			return
		}
	} else {
		// 根据代理密钥权限过滤分组
		accessibleGroups = make(map[string]*internal.UserGroup)

		if len(proxyKey.AllowedGroups) == 0 {
			// 如果没有限制，可以访问所有启用的分组
			accessibleGroups = enabledGroups
		} else {
			// 只包含有权限访问的分组
			for _, allowedGroupID := range proxyKey.AllowedGroups {
				if group, exists := enabledGroups[allowedGroupID]; exists {
					accessibleGroups[allowedGroupID] = group
				}
			}
		}
	}

	// 收集所有可访问分组的模型
	var allModels []map[string]interface{}

	for currentGroupID, group := range accessibleGroups {
		log.Printf("处理分组: ID=%s, Name=%s, ProviderType=%s", currentGroupID, group.Name, group.ProviderType)
		models := s.getModelsForGroup(currentGroupID, group)
		log.Printf("分组 %s 返回了 %d 个模型", currentGroupID, len(models))
		allModels = append(allModels, models...)
	}

	// 返回标准OpenAI格式
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   allModels,
	})
}

// getModelsForGroup 获取指定分组的模型列表
func (s *MultiProviderServer) getModelsForGroup(groupID string, group *internal.UserGroup) []map[string]interface{} {
	var models []map[string]interface{}

	// 如果分组配置了特定的模型列表，使用配置的模型
	if len(group.Models) > 0 {
		log.Printf("分组 %s 配置了 %d 个特定模型: %v", groupID, len(group.Models), group.Models)
		for _, modelID := range group.Models {
			models = append(models, map[string]interface{}{
				"id":       modelID,
				"object":   "model",
				"created":  1640995200, // 默认时间戳
				"owned_by": s.getOwnerByModelID(modelID),
			})
		}
		return models
	}

	// 如果没有配置特定模型，根据分组ID或提供商类型返回预定义模型
	log.Printf("分组 %s 没有配置特定模型，使用预定义模型列表", groupID)

	// 优先根据分组ID判断，然后根据提供商类型
	switch groupID {
	case "openrouter":
		// OpenRouter分组返回OpenRouter模型
		models = append(models, s.getOpenRouterModels()...)
	case "moda":
		// Moda分组返回OpenAI模型（因为它使用OpenAI格式）
		models = append(models, s.getOpenAIModels()...)
	default:
		// 根据提供商类型返回预定义的模型列表
		switch group.ProviderType {
		case "openai":
			models = append(models, s.getOpenAIModels()...)
		case "openrouter":
			models = append(models, s.getOpenRouterModels()...)
		case "anthropic":
			models = append(models, s.getAnthropicModels()...)
		case "gemini":
			models = append(models, s.getGeminiModels()...)
		default:
			// 对于未知类型，返回通用模型
			models = append(models, map[string]interface{}{
				"id":       fmt.Sprintf("%s-default", groupID),
				"object":   "model",
				"created":  1640995200, // 2022-01-01
				"owned_by": groupID,
			})
		}
	}

	return models
}

// getOwnerByModelID 根据模型ID推断所有者
func (s *MultiProviderServer) getOwnerByModelID(modelID string) string {
	if strings.Contains(modelID, "qwen") {
		return "alibaba"
	}
	if strings.Contains(modelID, "moonshotai") || strings.Contains(modelID, "kimi") {
		return "moonshot"
	}
	if strings.Contains(modelID, "deepseek") {
		return "deepseek"
	}
	if strings.Contains(modelID, "gpt") || strings.Contains(modelID, "openai") {
		return "openai"
	}
	if strings.Contains(modelID, "claude") || strings.Contains(modelID, "anthropic") {
		return "anthropic"
	}
	if strings.Contains(modelID, "gemini") || strings.Contains(modelID, "google") {
		return "google"
	}
	if strings.Contains(modelID, "llama") || strings.Contains(modelID, "meta") {
		return "meta"
	}

	// 默认返回openai
	return "openai"
}

// getOpenAIModels 获取OpenAI模型列表
func (s *MultiProviderServer) getOpenAIModels() []map[string]interface{} {
	return []map[string]interface{}{
		
	}
}

// getOpenRouterModels 获取OpenRouter模型列表
func (s *MultiProviderServer) getOpenRouterModels() []map[string]interface{} {
	return []map[string]interface{}{
		
	}
}

// getAnthropicModels 获取Anthropic模型列表
func (s *MultiProviderServer) getAnthropicModels() []map[string]interface{} {
	return []map[string]interface{}{
		
	}
}

// getGeminiModels 获取Gemini模型列表
func (s *MultiProviderServer) getGeminiModels() []map[string]interface{} {
	return []map[string]interface{}{
	}
}

// hasGroupAccess 检查代理密钥是否有访问指定分组的权限
func (s *MultiProviderServer) hasGroupAccess(proxyKey *logger.ProxyKey, groupID string) bool {
	// 如果AllowedGroups为空，表示可以访问所有分组
	if len(proxyKey.AllowedGroups) == 0 {
		return true
	}

	// 检查是否在允许的分组列表中
	for _, allowedGroup := range proxyKey.AllowedGroups {
		if allowedGroup == groupID {
			return true
		}
	}

	return false
}





// handleSystemHealth 处理系统健康检查
func (s *MultiProviderServer) handleSystemHealth(c *gin.Context) {
	health := s.healthChecker.GetSystemHealth()
	c.JSON(http.StatusOK, health)
}

// handleProvidersHealth 处理所有提供商健康检查
func (s *MultiProviderServer) handleProvidersHealth(c *gin.Context) {
	health := s.healthChecker.GetSystemHealth()
	c.JSON(http.StatusOK, health)
}

// handleProviderHealth 处理特定提供商健康检查
func (s *MultiProviderServer) handleProviderHealth(c *gin.Context) {
	groupID := c.Param("groupId")
	health := s.healthChecker.CheckProviderHealth(groupID)
	c.JSON(http.StatusOK, health)
}

// handleStatus 处理状态查询
func (s *MultiProviderServer) handleStatus(c *gin.Context) {
	systemHealth := s.healthChecker.GetSystemHealth()
	
	c.JSON(http.StatusOK, gin.H{
		"status":         systemHealth.Status,
		"timestamp":      time.Now(),
		"uptime":         systemHealth.Uptime,
		"total_groups":   systemHealth.TotalGroups,
		"healthy_groups": systemHealth.HealthyGroups,
		"total_keys":     systemHealth.TotalKeys,
		"active_keys":    systemHealth.ActiveKeys,
	})
}



// handleGroupsStatus 处理分组状态查询
func (s *MultiProviderServer) handleGroupsStatus(c *gin.Context) {
	// 从数据库获取分组信息（包含创建时间，按创建时间倒序）
	groupsWithMetadata, err := s.configManager.GetGroupsWithMetadata()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to load groups: " + err.Error(),
		})
		return
	}

	groups := make(map[string]interface{})

	for groupID, groupInfo := range groupsWithMetadata {
		// 添加总密钥数
		if apiKeys, ok := groupInfo["api_keys"].([]string); ok {
			groupInfo["total_keys"] = len(apiKeys)
		} else {
			groupInfo["total_keys"] = 0
		}

		// 获取健康状态，如果没有健康检查记录则默认为健康
		if healthStatus, exists := s.healthChecker.GetProviderHealth(groupID); exists {
			groupInfo["healthy"] = healthStatus.Healthy
			groupInfo["last_check"] = healthStatus.LastCheck
			groupInfo["response_time"] = healthStatus.ResponseTime
			groupInfo["last_error"] = healthStatus.LastError
		} else {
			// 新分组默认为健康状态
			groupInfo["healthy"] = true
			groupInfo["last_check"] = nil
			groupInfo["response_time"] = 0
			groupInfo["last_error"] = ""
		}

		groups[groupID] = groupInfo
	}

	c.JSON(http.StatusOK, gin.H{
		"groups": groups,
	})
}

// handleGroupKeysStatus 处理特定分组的密钥状态查询
func (s *MultiProviderServer) handleGroupKeysStatus(c *gin.Context) {
	groupID := c.Param("groupId")
	
	groupStatus, exists := s.keyManager.GetGroupStatus(groupID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Group not found",
		})
		return
	}
	
	c.JSON(http.StatusOK, groupStatus)
}

// handleAllModels 处理所有模型列表请求 - 返回分组配置中选择的模型
func (s *MultiProviderServer) handleAllModels(c *gin.Context) {
	allGroups := s.configManager.GetAllGroups()
	allModels := make(map[string]interface{})

	for groupID, group := range allGroups {
		if !group.Enabled {
			continue // 跳过禁用的分组
		}

		// 构建模型列表 - 使用分组配置中的模型
		var modelList []map[string]interface{}

		if len(group.Models) > 0 {
			// 如果分组配置了特定模型，使用配置的模型
			for _, modelID := range group.Models {
				modelList = append(modelList, map[string]interface{}{
					"id":       modelID,
					"object":   "model",
					"owned_by": s.getProviderOwner(group.ProviderType),
				})
			}
		} else {
			// 如果没有配置特定模型，表示支持所有模型，返回一个通用提示
			modelList = append(modelList, map[string]interface{}{
				"id":       "all-models-supported",
				"object":   "model",
				"owned_by": s.getProviderOwner(group.ProviderType),
				"note":     "This provider supports all available models",
			})
		}

		// 添加到结果中
		allModels[groupID] = map[string]interface{}{
			"group_name":    group.Name,
			"provider_type": group.ProviderType,
			"models": map[string]interface{}{
				"object": "list",
				"data":   modelList,
			},
		}
	}

	// 返回所有模型
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   allModels,
	})
}

// handleGroupModels 处理特定分组的模型列表请求 - 返回分组配置中选择的模型
func (s *MultiProviderServer) handleGroupModels(c *gin.Context) {
	groupID := c.Param("groupId")

	group, exists := s.configManager.GetGroup(groupID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "Group not found",
				"type":    "not_found",
				"code":    "group_not_found",
			},
		})
		return
	}

	// 构建模型列表 - 使用分组配置中的模型
	var modelList []map[string]interface{}

	if len(group.Models) > 0 {
		// 如果分组配置了特定模型，使用配置的模型
		for _, modelID := range group.Models {
			modelList = append(modelList, map[string]interface{}{
				"id":       modelID,
				"object":   "model",
				"owned_by": s.getProviderOwner(group.ProviderType),
			})
		}
	} else {
		// 如果没有配置特定模型，表示支持所有模型，返回一个通用提示
		modelList = append(modelList, map[string]interface{}{
			"id":       "all-models-supported",
			"object":   "model",
			"owned_by": s.getProviderOwner(group.ProviderType),
			"note":     "This provider supports all available models",
		})
	}

	// 为了与前端期望的格式一致，将单个提供商的响应包装成与所有提供商相同的格式
	response := gin.H{
		"object": "list",
		"data": map[string]interface{}{
			groupID: map[string]interface{}{
				"group_name":    group.Name,
				"provider_type": group.ProviderType,
				"models": map[string]interface{}{
					"object": "list",
					"data":   modelList,
				},
			},
		},
	}

	c.JSON(http.StatusOK, response)
}

// getProviderOwner 根据提供商类型返回所有者信息
func (s *MultiProviderServer) getProviderOwner(providerType string) string {
	switch providerType {
	case "openai":
		return "openai"
	case "azure_openai":
		return "openai"
	case "anthropic":
		return "anthropic"
	case "gemini":
		return "google"
	case "openrouter":
		return "openrouter"
	default:
		return providerType
	}
}

// handleAvailableModels 处理获取提供商所有可用模型的请求（用于分组管理页面的模型选择）
func (s *MultiProviderServer) handleAvailableModels(c *gin.Context) {
	groupID := c.Param("groupId")

	// 直接调用proxy的HandleModels方法来获取提供商的所有可用模型
	c.Request.URL.RawQuery = fmt.Sprintf("provider_group=%s", groupID)
	s.proxy.HandleModels(c)
}

// handleAvailableModelsByType 根据提供商类型和配置获取可用模型（用于新建分组时的模型选择）
func (s *MultiProviderServer) handleAvailableModelsByType(c *gin.Context) {
	var req struct {
		ProviderType string   `json:"provider_type" binding:"required"`
		BaseURL      string   `json:"base_url" binding:"required"`
		APIKeys      []string `json:"api_keys" binding:"required"`
		MaxRetries   int      `json:"max_retries"`
		Timeout      int      `json:"timeout_seconds"`
		Headers      map[string]string `json:"headers"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request: " + err.Error(),
		})
		return
	}

	// 验证API密钥不为空
	validKeys := make([]string, 0)
	for _, key := range req.APIKeys {
		if strings.TrimSpace(key) != "" {
			validKeys = append(validKeys, strings.TrimSpace(key))
		}
	}

	if len(validKeys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "At least one valid API key is required",
		})
		return
	}

	// 设置默认值
	if req.MaxRetries == 0 {
		req.MaxRetries = 3
	}
	if req.Timeout == 0 {
		req.Timeout = 30
	}

	// 创建临时分组配置
	tempGroup := &internal.UserGroup{
		Name:         "temp-test-group",
		ProviderType: req.ProviderType,
		BaseURL:      req.BaseURL,
		APIKeys:      validKeys,
		Enabled:      true,
		Timeout:      time.Duration(req.Timeout) * time.Second,
		MaxRetries:   req.MaxRetries,
		Headers:      req.Headers,
	}

	// 创建临时提供商实例
	factory := providers.NewDefaultProviderFactory()
	config := &providers.ProviderConfig{
		BaseURL:      tempGroup.BaseURL,
		APIKey:       tempGroup.APIKeys[0], // 使用第一个API密钥进行测试
		Timeout:      tempGroup.Timeout,
		MaxRetries:   tempGroup.MaxRetries,
		Headers:      tempGroup.Headers,
		ProviderType: tempGroup.ProviderType,
	}

	provider, err := factory.CreateProvider(config)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Failed to create provider: " + err.Error(),
		})
		return
	}

	// 获取模型列表
	ctx := c.Request.Context()
	rawModels, err := provider.GetModels(ctx)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Failed to get models: " + err.Error(),
		})
		return
	}

	// 标准化模型数据格式
	standardizedModels := s.proxy.StandardizeModelsResponse(rawModels, tempGroup.ProviderType)

	// 返回模型列表，格式与其他API保持一致
	response := gin.H{
		"object": "list",
		"data": map[string]interface{}{
			"temp-group": map[string]interface{}{
				"group_name":    "临时测试分组",
				"provider_type": tempGroup.ProviderType,
				"models":        standardizedModels,
			},
		},
	}

	c.JSON(http.StatusOK, response)
}

// handleValidateKeys 处理密钥有效性验证请求
func (s *MultiProviderServer) handleValidateKeys(c *gin.Context) {
	groupID := c.Param("groupId")

	group, exists := s.configManager.GetGroup(groupID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Group not found",
		})
		return
	}

	// 获取要验证的密钥列表
	var req struct {
		APIKeys []string `json:"api_keys"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request data: " + err.Error(),
		})
		return
	}

	// 选择用于测试的模型（优先使用配置的第一个模型，否则使用默认模型）
	var testModel string
	if len(group.Models) > 0 {
		testModel = group.Models[0]
	} else {
		// 根据提供商类型选择默认测试模型
		switch group.ProviderType {
		case "openai", "azure_openai":
			testModel = "gpt-3.5-turbo"
		case "anthropic":
			testModel = "claude-3-haiku-20240307"
		case "gemini":
			testModel = "gemini-2.5-flash"
		default:
			testModel = "gpt-3.5-turbo" // 默认模型
		}
	}

	log.Printf("🔍 开始批量验证密钥: 分组=%s, 提供商=%s, 密钥数量=%d, 测试模型=%s",
		groupID, group.ProviderType, len(req.APIKeys), testModel)

	// 顺序验证每个密钥，避免限流
	results := make([]map[string]interface{}, len(req.APIKeys))
	log.Printf("⚙️ 采用顺序验证模式，每个密钥间隔10秒，避免API限流")

	for i, apiKey := range req.APIKeys {
		if strings.TrimSpace(apiKey) == "" {
			log.Printf("⚠️ 跳过空密钥 (索引: %d)", i)
			results[i] = map[string]interface{}{
				"index":   i,
				"api_key": apiKey,
				"valid":   false,
				"error":   "Empty API key",
			}
			continue
		}

		log.Printf("🎯 开始验证密钥 %d/%d: %s", i+1, len(req.APIKeys), s.maskKey(apiKey))

		// 验证密钥，最多重试3次
		valid, err := s.validateKeyWithRetry(groupID, apiKey, testModel, group, 3)

		// 更新数据库中的验证状态
		validationError := ""
		if err != nil {
			validationError = err.Error()
		}

		// 记录验证结果
		if valid {
			log.Printf("✅ 密钥验证成功 %d/%d: %s", i+1, len(req.APIKeys), s.maskKey(apiKey))
		} else {
			log.Printf("❌ 密钥验证失败 %d/%d: %s - %s", i+1, len(req.APIKeys), s.maskKey(apiKey), validationError)
		}

		// 异步更新数据库，避免阻塞验证流程
		go func(gID, apiKey string, isValid bool, errMsg string) {
			if updateErr := s.configManager.UpdateAPIKeyValidation(gID, apiKey, isValid, errMsg); updateErr != nil {
				log.Printf("❌ 更新数据库验证状态失败 %s: %v", s.maskKey(apiKey), updateErr)
			} else {
				log.Printf("💾 数据库验证状态已更新: %s (有效: %v)", s.maskKey(apiKey), isValid)
			}
		}(groupID, apiKey, valid, validationError)

		results[i] = map[string]interface{}{
			"index":   i,
			"api_key": apiKey,
			"valid":   valid,
			"error":   validationError,
		}

		// 如果不是最后一个密钥，等待10秒再验证下一个
		if i < len(req.APIKeys)-1 {
			log.Printf("⏳ 等待10秒后验证下一个密钥，避免API限流...")
			time.Sleep(10 * time.Second)
		}
	}

	// 所有验证已完成（顺序执行）
	log.Printf("✅ 所有密钥验证已完成")

	// 统计结果
	validCount := 0
	invalidCount := 0
	for _, result := range results {
		if result["valid"].(bool) {
			validCount++
		} else {
			invalidCount++
		}
	}

	log.Printf("📊 验证结果统计: 总计=%d, 有效=%d, 无效=%d, 成功率=%.1f%%",
		len(req.APIKeys), validCount, invalidCount,
		float64(validCount)/float64(len(req.APIKeys))*100)

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"test_model":    testModel,
		"total_keys":    len(req.APIKeys),
		"valid_keys":    validCount,
		"invalid_keys":  invalidCount,
		"results":       results,
	})
}

// validateKeyWithRetry 带重试机制的密钥验证
func (s *MultiProviderServer) validateKeyWithRetry(groupID, apiKey, testModel string, group *internal.UserGroup, maxRetries int) (bool, error) {
	var lastErr error
	maskedKey := s.maskKey(apiKey)

	log.Printf("🔑 开始验证密钥: %s (分组: %s, 提供商: %s, 模型: %s)", maskedKey, groupID, group.ProviderType, testModel)

	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Printf("🔄 密钥验证尝试 %d/%d: %s", attempt, maxRetries, maskedKey)

		// 创建提供商配置，强制使用300秒超时进行验证
		providerConfig := &providers.ProviderConfig{
			BaseURL:      group.BaseURL,
			APIKey:       apiKey,
			Timeout:      time.Duration(300) * time.Second, // 强制300秒超时，忽略分组配置
			MaxRetries:   1,
			Headers:      group.Headers,
			ProviderType: group.ProviderType,
		}

		log.Printf("📋 提供商配置: BaseURL=%s, ProviderType=%s, Timeout=300s (强制设置)",
			func() string {
				if group.BaseURL != "" {
					return group.BaseURL
				}
				return "默认"
			}(), group.ProviderType)
		log.Printf("📝 注意: 分组原始超时=%v, 验证时强制使用300s", group.Timeout)

		// 获取提供商实例
		providerID := fmt.Sprintf("%s_validate_%s_%d", groupID, apiKey[:min(8, len(apiKey))], attempt)
		log.Printf("🏭 创建提供商实例: %s", providerID)

		provider, err := s.proxy.GetProviderManager().GetProvider(providerID, providerConfig)
		if err != nil {
			lastErr = fmt.Errorf("failed to create provider (attempt %d/%d): %w", attempt, maxRetries, err)
			log.Printf("❌ 创建提供商失败 (尝试 %d/%d): %v", attempt, maxRetries, err)
			continue
		}

		log.Printf("✅ 提供商实例创建成功")

		// 验证密钥
		log.Printf("🚀 发送测试请求到 %s 模型...", testModel)
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)

		startTime := time.Now()
		response, err := provider.ChatCompletion(ctx, &providers.ChatCompletionRequest{
			Model:    testModel,
			Messages: []providers.ChatMessage{{Role: "user", Content: "test"}},
			// 移除MaxTokens限制，让提供商使用默认值
		})
		duration := time.Since(startTime)
		cancel()

		if err == nil {
			// 验证成功
			log.Printf("✅ 密钥验证成功: %s (耗时: %v)", maskedKey, duration)
			if response != nil && len(response.Choices) > 0 {
				log.Printf("📝 响应内容长度: %d 字符", len(response.Choices[0].Message.Content))
			}
			return true, nil
		}

		lastErr = fmt.Errorf("validation failed (attempt %d/%d): %w", attempt, maxRetries, err)
		log.Printf("❌ 密钥验证失败 (尝试 %d/%d, 耗时: %v): %v", attempt, maxRetries, duration, err)

		// 如果不是最后一次尝试，等待一小段时间再重试
		if attempt < maxRetries {
			waitTime := time.Duration(attempt) * 500 * time.Millisecond
			log.Printf("⏳ 等待 %v 后重试...", waitTime)
			time.Sleep(waitTime) // 递增等待时间
		}
	}

	// 所有重试都失败
	log.Printf("💥 密钥验证最终失败: %s (已尝试 %d 次)", maskedKey, maxRetries)
	return false, lastErr
}

// maskKey 遮蔽API密钥的敏感部分
func (s *MultiProviderServer) maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// handleKeysStatus 处理获取所有分组密钥状态的请求
func (s *MultiProviderServer) handleKeysStatus(c *gin.Context) {
	allGroups := s.configManager.GetAllGroups()
	groupsStatus := make(map[string]interface{})

	for groupID, group := range allGroups {
		if !group.Enabled {
			continue // 跳过禁用的分组
		}

		// 选择用于测试的模型
		var testModel string
		if len(group.Models) > 0 {
			testModel = group.Models[0]
		} else {
			// 根据提供商类型选择默认测试模型
			switch group.ProviderType {
			case "openai", "azure_openai":
				testModel = "gpt-3.5-turbo"
			case "anthropic":
				testModel = "claude-3-haiku-20240307"
			case "gemini":
				testModel = "gemini-2.5-flash"
			default:
				testModel = "gpt-3.5-turbo"
			}
		}

		// 验证每个密钥
		validCount := 0
		invalidCount := 0
		keyResults := make([]map[string]interface{}, 0, len(group.APIKeys))

		for i, apiKey := range group.APIKeys {
			if strings.TrimSpace(apiKey) == "" {
				invalidCount++
				keyResults = append(keyResults, map[string]interface{}{
					"index": i,
					"valid": false,
					"error": "Empty API key",
				})
				continue
			}

			// 创建提供商配置
			providerConfig := &providers.ProviderConfig{
				BaseURL:      group.BaseURL,
				APIKey:       apiKey,
				Timeout:      time.Duration(5) * time.Second, // 更短的超时用于状态检查
				MaxRetries:   1,
				Headers:      group.Headers,
				ProviderType: group.ProviderType,
			}

			// 获取提供商实例
			provider, err := s.proxy.GetProviderManager().GetProvider(fmt.Sprintf("%s_status_%d", groupID, i), providerConfig)
			if err != nil {
				invalidCount++
				keyResults = append(keyResults, map[string]interface{}{
					"index": i,
					"valid": false,
					"error": "Failed to create provider: " + err.Error(),
				})
				continue
			}

			// 验证密钥
			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			_, err = provider.ChatCompletion(ctx, &providers.ChatCompletionRequest{
				Model:    testModel,
				Messages: []providers.ChatMessage{{Role: "user", Content: "test"}},
				// 移除MaxTokens限制，让提供商使用默认值
			})
			cancel()

			if err != nil {
				invalidCount++
				keyResults = append(keyResults, map[string]interface{}{
					"index": i,
					"valid": false,
					"error": err.Error(),
				})
			} else {
				validCount++
				keyResults = append(keyResults, map[string]interface{}{
					"index": i,
					"valid": true,
					"error": "",
				})
			}
		}

		groupsStatus[groupID] = map[string]interface{}{
			"group_name":    group.Name,
			"provider_type": group.ProviderType,
			"test_model":    testModel,
			"total_keys":    len(group.APIKeys),
			"valid_keys":    validCount,
			"invalid_keys":  invalidCount,
			"key_results":   keyResults,
			"last_checked":  time.Now().Format("2006-01-02 15:04:05"),
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    groupsStatus,
	})
}

// handleTestModels 处理测试模型加载请求
func (s *MultiProviderServer) handleTestModels(c *gin.Context) {
	var testGroup struct {
		Name             string   `json:"name"`
		ProviderType     string   `json:"provider_type"`
		BaseURL          string   `json:"base_url"`
		Enabled          bool     `json:"enabled"`
		Timeout          int      `json:"timeout"`
		MaxRetries       int      `json:"max_retries"`
		RotationStrategy string   `json:"rotation_strategy"`
		APIKeys          []string `json:"api_keys"`
	}

	if err := c.ShouldBindJSON(&testGroup); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request data: " + err.Error(),
		})
		return
	}

	// 验证必需字段
	if testGroup.ProviderType == "" || testGroup.BaseURL == "" || len(testGroup.APIKeys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Provider type, base URL, and at least one API key are required",
		})
		return
	}

	// 创建临时的UserGroup配置
	tempGroup := &internal.UserGroup{
		Name:             testGroup.Name,
		ProviderType:     testGroup.ProviderType,
		BaseURL:          testGroup.BaseURL,
		Enabled:          testGroup.Enabled,
		Timeout:          time.Duration(testGroup.Timeout) * time.Second,
		MaxRetries:       testGroup.MaxRetries,
		RotationStrategy: testGroup.RotationStrategy,
		APIKeys:          testGroup.APIKeys,
	}

	// 使用第一个API密钥来测试模型加载
	if len(testGroup.APIKeys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "No API keys provided",
		})
		return
	}

	// 创建提供商配置
	providerConfig := &providers.ProviderConfig{
		BaseURL:      tempGroup.BaseURL,
		APIKey:       testGroup.APIKeys[0], // 使用第一个密钥进行测试
		Timeout:      tempGroup.Timeout,
		MaxRetries:   tempGroup.MaxRetries,
		ProviderType: tempGroup.ProviderType,
	}

	// 获取提供商实例
	provider, err := s.proxy.GetProviderManager().GetProvider("test", providerConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to create provider instance: " + err.Error(),
		})
		return
	}

	// 获取模型列表
	ctx := c.Request.Context()
	rawModels, err := provider.GetModels(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": "Failed to load models: " + err.Error(),
		})
		return
	}

	// 标准化模型数据格式
	standardizedModels := s.proxy.StandardizeModelsResponse(rawModels, tempGroup.ProviderType)

	// 返回模型列表
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"models":  standardizedModels,
	})
}

// handleIndex 处理首页
func (s *MultiProviderServer) handleIndex(c *gin.Context) {
	c.HTML(http.StatusOK, "index.html", gin.H{
		"title": "TurnsAPI - 多提供商代理服务",
	})
}

// handleMultiProviderDashboard 处理多提供商仪表板页面
func (s *MultiProviderServer) handleMultiProviderDashboard(c *gin.Context) {
	c.HTML(http.StatusOK, "multi_provider_dashboard.html", gin.H{
		"title": "多提供商仪表板 - TurnsAPI",
	})
}

// handleHealth 处理健康检查
func (s *MultiProviderServer) handleHealth(c *gin.Context) {
	systemHealth := s.healthChecker.GetSystemHealth()
	
	status := "healthy"
	if systemHealth.Status != "healthy" {
		status = systemHealth.Status
	}
	
	c.JSON(http.StatusOK, gin.H{
		"status":    status,
		"timestamp": time.Now(),
	})
}

// Start 启动服务器
func (s *MultiProviderServer) Start() error {
	s.httpServer = &http.Server{
		Addr:    s.config.GetAddress(),
		Handler: s.router,
	}

	log.Printf("Starting multi-provider server on %s", s.config.GetAddress())
	return s.httpServer.ListenAndServe()
}

// Stop 停止服务器
func (s *MultiProviderServer) Stop(ctx context.Context) error {
	// 关闭健康检查器
	if s.healthChecker != nil {
		s.healthChecker.Close()
	}
	
	// 关闭密钥管理器
	if s.keyManager != nil {
		s.keyManager.Close()
	}
	
	// 关闭请求日志记录器
	if s.requestLogger != nil {
		if err := s.requestLogger.Close(); err != nil {
			log.Printf("Failed to close request logger: %v", err)
		}
	}

	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// handleLogs 处理日志查询
func (s *MultiProviderServer) handleLogs(c *gin.Context) {
	if s.requestLogger == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Request logger not available",
		})
		return
	}

	// 获取查询参数
	limit := 50
	offset := 0
	proxyKeyName := c.Query("proxy_key_name")
	providerGroup := c.Query("provider_group")

	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// 获取日志列表
	logs, err := s.requestLogger.GetRequestLogs(proxyKeyName, providerGroup, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get logs: " + err.Error(),
		})
		return
	}

	// 获取总数
	totalCount, err := s.requestLogger.GetRequestCount(proxyKeyName, providerGroup)
	if err != nil {
		log.Printf("Failed to get logs count: %v", err)
		totalCount = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"logs":        logs,
		"total_count": totalCount,
	})
}

// handleLogDetail 处理日志详情查询
func (s *MultiProviderServer) handleLogDetail(c *gin.Context) {
	if s.requestLogger == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Request logger not available",
		})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid log ID",
		})
		return
	}

	logDetail, err := s.requestLogger.GetRequestLogDetail(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Log not found: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"log":     logDetail,
	})
}

// handleAPIKeyStats 处理API密钥统计
func (s *MultiProviderServer) handleAPIKeyStats(c *gin.Context) {
	if s.requestLogger == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Request logger not available",
		})
		return
	}

	stats, err := s.requestLogger.GetProxyKeyStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get API key stats: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"stats":   stats,
	})
}

// handleModelStats 处理模型统计
func (s *MultiProviderServer) handleModelStats(c *gin.Context) {
	if s.requestLogger == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Request logger not available",
		})
		return
	}

	stats, err := s.requestLogger.GetModelStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get model stats: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"stats":   stats,
	})
}

// handleProxyKeys 处理代理密钥列表查询（支持分页和搜索）
func (s *MultiProviderServer) handleProxyKeys(c *gin.Context) {
	// 获取查询参数
	page := 1
	pageSize := 10
	search := c.Query("search")

	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	// 获取所有密钥
	allKeys := s.proxyKeyManager.GetAllKeys()

	// 搜索过滤
	var filteredKeys []*proxykey.ProxyKey
	if search != "" {
		searchLower := strings.ToLower(search)
		for _, key := range allKeys {
			if strings.Contains(strings.ToLower(key.Name), searchLower) ||
			   strings.Contains(strings.ToLower(key.Description), searchLower) ||
			   strings.Contains(strings.ToLower(key.Key), searchLower) {
				filteredKeys = append(filteredKeys, key)
			}
		}
	} else {
		filteredKeys = allKeys
	}

	// 计算分页
	total := len(filteredKeys)
	totalPages := (total + pageSize - 1) / pageSize

	// 计算起始和结束索引
	start := (page - 1) * pageSize
	end := start + pageSize

	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	// 获取当前页的数据
	var pageKeys []*proxykey.ProxyKey
	if start < end {
		pageKeys = filteredKeys[start:end]
	} else {
		pageKeys = []*proxykey.ProxyKey{}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"keys":        pageKeys,
		"pagination": gin.H{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": totalPages,
			"has_prev":    page > 1,
			"has_next":    page < totalPages,
		},
		"search": search,
	})
}

// handleGenerateProxyKey 处理生成代理密钥
func (s *MultiProviderServer) handleGenerateProxyKey(c *gin.Context) {
	var req struct {
		Name          string   `json:"name" binding:"required"`
		Description   string   `json:"description"`
		AllowedGroups []string `json:"allowedGroups"` // 允许访问的分组ID列表
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format",
		})
		return
	}

	key, err := s.proxyKeyManager.GenerateKey(req.Name, req.Description, req.AllowedGroups)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to generate key",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"key":     key,
	})
}

// handleDeleteProxyKey 处理删除代理密钥
func (s *MultiProviderServer) handleDeleteProxyKey(c *gin.Context) {
	id := c.Param("id")

	err := s.proxyKeyManager.DeleteKey(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Key not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

// handleLogsPage 处理日志页面
func (s *MultiProviderServer) handleLogsPage(c *gin.Context) {
	c.HTML(http.StatusOK, "logs.html", gin.H{
		"title": "请求日志 - TurnsAPI",
	})
}

// handleGroupsManagePage 处理分组管理页面
func (s *MultiProviderServer) handleGroupsManagePage(c *gin.Context) {
	c.HTML(http.StatusOK, "groups_manage.html", gin.H{
		"title": "分组管理 - TurnsAPI",
	})
}

// handleGroupsManage 处理分组管理API
func (s *MultiProviderServer) handleGroupsManage(c *gin.Context) {
	groups := make(map[string]interface{})

	allGroups := s.configManager.GetAllGroups()
	for groupID, group := range allGroups {
		groupInfo := map[string]interface{}{
			"group_id":          groupID,
			"group_name":        group.Name,
			"provider_type":     group.ProviderType,
			"base_url":          group.BaseURL,
			"enabled":           group.Enabled,
			"timeout":           group.Timeout.Seconds(),
			"max_retries":       group.MaxRetries,
			"rotation_strategy": group.RotationStrategy,
			"api_keys":          group.APIKeys,
			"models":            group.Models,
			"headers":           group.Headers,
		}

		// 获取健康状态，如果没有健康检查记录则默认为健康
		if healthStatus, exists := s.healthChecker.GetProviderHealth(groupID); exists {
			groupInfo["healthy"] = healthStatus.Healthy
			groupInfo["last_check"] = healthStatus.LastCheck
			groupInfo["response_time"] = healthStatus.ResponseTime
			groupInfo["last_error"] = healthStatus.LastError
		} else {
			// 新分组默认为健康状态
			groupInfo["healthy"] = true
			groupInfo["last_check"] = nil
			groupInfo["response_time"] = 0
			groupInfo["last_error"] = ""
		}

		groups[groupID] = groupInfo
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"groups":  groups,
	})
}

// handleCreateGroup 处理创建分组
func (s *MultiProviderServer) handleCreateGroup(c *gin.Context) {
	var req struct {
		GroupID          string            `json:"group_id" binding:"required"`
		Name             string            `json:"name" binding:"required"`
		ProviderType     string            `json:"provider_type" binding:"required"`
		BaseURL          string            `json:"base_url" binding:"required"`
		Enabled          bool              `json:"enabled"`
		Timeout          float64           `json:"timeout"`
		MaxRetries       int               `json:"max_retries"`
		RotationStrategy string            `json:"rotation_strategy"`
		APIKeys          []string          `json:"api_keys"`
		Models           []string          `json:"models"`
		Headers          map[string]string `json:"headers"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request format: " + err.Error(),
		})
		return
	}

	// 检查分组ID是否已存在
	if _, exists := s.configManager.GetGroup(req.GroupID); exists {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "Group ID already exists",
		})
		return
	}

	// 验证提供商类型
	supportedTypes := []string{"openai", "gemini", "anthropic", "azure_openai"}
	supported := false
	for _, supportedType := range supportedTypes {
		if req.ProviderType == supportedType {
			supported = true
			break
		}
	}

	if !supported {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": fmt.Sprintf("Unsupported provider type: %s", req.ProviderType),
		})
		return
	}

	// 设置默认值
	if req.Timeout == 0 {
		req.Timeout = 30
	}
	if req.MaxRetries == 0 {
		req.MaxRetries = 3
	}
	if req.RotationStrategy == "" {
		req.RotationStrategy = "round_robin"
	}
	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}
	if req.Headers["Content-Type"] == "" {
		req.Headers["Content-Type"] = "application/json"
	}

	// 创建新的用户分组
	newGroup := &internal.UserGroup{
		Name:             req.Name,
		ProviderType:     req.ProviderType,
		BaseURL:          req.BaseURL,
		Enabled:          req.Enabled,
		Timeout:          time.Duration(req.Timeout) * time.Second,
		MaxRetries:       req.MaxRetries,
		RotationStrategy: req.RotationStrategy,
		APIKeys:          req.APIKeys,
		Models:           req.Models,
		Headers:          req.Headers,
	}

	// 保存到配置管理器（会同时更新数据库和内存）
	if err := s.configManager.SaveGroup(req.GroupID, newGroup); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to save group: " + err.Error(),
		})
		return
	}

	// 更新密钥管理器
	if err := s.keyManager.UpdateGroupConfig(req.GroupID, newGroup); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update key manager: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Group created successfully",
		"group_id": req.GroupID,
	})
}

// handleUpdateGroup 处理更新分组
func (s *MultiProviderServer) handleUpdateGroup(c *gin.Context) {
	groupID := c.Param("groupId")

	// 检查分组是否存在
	existingGroup, exists := s.configManager.GetGroup(groupID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Group not found",
		})
		return
	}

	var req struct {
		Name             string            `json:"name"`
		ProviderType     string            `json:"provider_type"`
		BaseURL          string            `json:"base_url"`
		Enabled          *bool             `json:"enabled"`
		Timeout          *float64          `json:"timeout"`
		MaxRetries       *int              `json:"max_retries"`
		RotationStrategy string            `json:"rotation_strategy"`
		APIKeys          []string          `json:"api_keys"`
		Models           []string          `json:"models"`
		Headers          map[string]string `json:"headers"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request format: " + err.Error(),
		})
		return
	}

	// 更新字段（只更新提供的字段）
	if req.Name != "" {
		existingGroup.Name = req.Name
	}
	if req.ProviderType != "" {
		// 验证提供商类型
		supportedTypes := []string{"openai", "gemini", "anthropic", "azure_openai"}
		supported := false
		for _, supportedType := range supportedTypes {
			if req.ProviderType == supportedType {
				supported = true
				break
			}
		}

		if !supported {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": fmt.Sprintf("Unsupported provider type: %s", req.ProviderType),
			})
			return
		}
		existingGroup.ProviderType = req.ProviderType
	}
	if req.BaseURL != "" {
		existingGroup.BaseURL = req.BaseURL
	}
	if req.Enabled != nil {
		existingGroup.Enabled = *req.Enabled
	}
	if req.Timeout != nil {
		existingGroup.Timeout = time.Duration(*req.Timeout) * time.Second
	}
	if req.MaxRetries != nil {
		existingGroup.MaxRetries = *req.MaxRetries
	}
	if req.RotationStrategy != "" {
		existingGroup.RotationStrategy = req.RotationStrategy
	}
	if req.APIKeys != nil {
		existingGroup.APIKeys = req.APIKeys
	}
	if req.Models != nil {
		existingGroup.Models = req.Models
	}
	if req.Headers != nil {
		existingGroup.Headers = req.Headers
	}

	// 保存到配置管理器
	if err := s.configManager.UpdateGroup(groupID, existingGroup); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update group: " + err.Error(),
		})
		return
	}

	// 更新密钥管理器
	if err := s.keyManager.UpdateGroupConfig(groupID, existingGroup); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update key manager: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Group updated successfully",
	})
}

// handleDeleteGroup 处理删除分组
func (s *MultiProviderServer) handleDeleteGroup(c *gin.Context) {
	groupID := c.Param("groupId")

	// 检查分组是否存在
	_, exists := s.configManager.GetGroup(groupID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Group not found",
		})
		return
	}

	// 检查是否是最后一个启用的分组
	enabledCount := s.configManager.GetEnabledGroupCount()
	currentGroup, _ := s.configManager.GetGroup(groupID)

	if enabledCount <= 1 && currentGroup.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Cannot delete the last enabled group",
		})
		return
	}

	// 从配置管理器中删除（会同时删除数据库和内存中的数据）
	if err := s.configManager.DeleteGroup(groupID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to delete group: " + err.Error(),
		})
		return
	}

	// 更新密钥管理器（传递nil表示删除）
	if err := s.keyManager.UpdateGroupConfig(groupID, nil); err != nil {
		log.Printf("警告: 删除分组 %s 时更新密钥管理器失败: %v", groupID, err)
	}

	// 从健康检查器中移除分组
	s.healthChecker.RemoveGroup(groupID)

	// 从提供商管理器中移除分组
	s.proxy.RemoveProvider(groupID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Group deleted successfully",
	})
}

// handleToggleGroup 处理切换分组启用状态
func (s *MultiProviderServer) handleToggleGroup(c *gin.Context) {
	groupID := c.Param("groupId")

	// 使用配置管理器的切换方法（包含所有业务逻辑和数据库更新）
	if err := s.configManager.ToggleGroup(groupID); err != nil {
		if err.Error() == "group not found: "+groupID {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Group not found",
			})
		} else if err.Error() == "cannot disable the last enabled group" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Cannot disable the last enabled group",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to toggle group: " + err.Error(),
			})
		}
		return
	}

	// 获取更新后的分组状态
	group, _ := s.configManager.GetGroup(groupID)

	// 更新密钥管理器
	if err := s.keyManager.UpdateGroupConfig(groupID, group); err != nil {
		log.Printf("警告: 切换分组 %s 状态时更新密钥管理器失败: %v", groupID, err)
	}

	action := "enabled"
	if !group.Enabled {
		action = "disabled"
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Group %s successfully", action),
		"enabled": group.Enabled,
	})
}

// handleValidateKeysWithoutGroup 处理不需要groupId的密钥验证请求（用于编辑分组时）
func (s *MultiProviderServer) handleValidateKeysWithoutGroup(c *gin.Context) {
	// 获取要验证的分组配置和密钥列表
	var req struct {
		Name             string            `json:"name"`
		ProviderType     string            `json:"provider_type"`
		BaseURL          string            `json:"base_url"`
		Enabled          bool              `json:"enabled"`
		Timeout          int               `json:"timeout"`
		MaxRetries       int               `json:"max_retries"`
		RotationStrategy string            `json:"rotation_strategy"`
		APIKeys          []string          `json:"api_keys"`
		Headers          map[string]string `json:"headers"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request data: " + err.Error(),
		})
		return
	}

	// 验证必需字段
	if req.ProviderType == "" || req.BaseURL == "" || len(req.APIKeys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Provider type, base URL, and at least one API key are required",
		})
		return
	}

	// 创建临时的UserGroup配置
	tempGroup := &internal.UserGroup{
		Name:             req.Name,
		ProviderType:     req.ProviderType,
		BaseURL:          req.BaseURL,
		Enabled:          req.Enabled,
		Timeout:          time.Duration(300) * time.Second, // 强制设置为300秒，避免超时
		MaxRetries:       req.MaxRetries,
		RotationStrategy: req.RotationStrategy,
		APIKeys:          req.APIKeys,
		Headers:          req.Headers,
	}

	// 获取测试模型
	var testModel string
	// 根据提供商类型选择默认测试模型
	switch req.ProviderType {
	case "openai", "azure_openai":
		testModel = "gpt-3.5-turbo"
	case "anthropic":
		testModel = "claude-3-haiku-20240307"
	case "gemini":
		testModel = "gemini-2.5-flash"
	default:
		testModel = "gpt-3.5-turbo" // 默认模型
	}

	log.Printf("🔍 开始临时分组密钥验证: 名称=%s, 提供商=%s, 密钥数量=%d, 测试模型=%s",
		req.Name, req.ProviderType, len(req.APIKeys), testModel)

	// 顺序验证密钥，避免限流
	results := make([]map[string]interface{}, len(req.APIKeys))
	log.Printf("⚙️ 采用顺序验证模式，每个密钥间隔10秒，避免API限流")

	for i, apiKey := range req.APIKeys {
		if strings.TrimSpace(apiKey) == "" {
			log.Printf("⚠️ 跳过空密钥 (索引: %d)", i)
			results[i] = map[string]interface{}{
				"index":   i,
				"api_key": apiKey,
				"valid":   false,
				"error":   "Empty API key",
			}
			continue
		}

		log.Printf("🎯 开始验证临时分组密钥 %d/%d: %s", i+1, len(req.APIKeys), s.maskKey(apiKey))

		// 验证密钥，最多重试3次
		valid, err := s.validateKeyWithRetry("temp", apiKey, testModel, tempGroup, 3)

		validationError := ""
		if err != nil {
			validationError = err.Error()
		}

		// 记录验证结果
		if valid {
			log.Printf("✅ 临时分组密钥验证成功 %d/%d: %s", i+1, len(req.APIKeys), s.maskKey(apiKey))
		} else {
			log.Printf("❌ 临时分组密钥验证失败 %d/%d: %s - %s", i+1, len(req.APIKeys), s.maskKey(apiKey), validationError)
		}

		results[i] = map[string]interface{}{
			"index":   i,
			"api_key": apiKey,
			"valid":   valid,
			"error":   validationError,
		}

		// 如果不是最后一个密钥，等待10秒再验证下一个
		if i < len(req.APIKeys)-1 {
			log.Printf("⏳ 等待10秒后验证下一个密钥，避免API限流...")
			time.Sleep(10 * time.Second)
		}
	}

	// 所有验证已完成（顺序执行）
	log.Printf("✅ 所有临时分组密钥验证已完成")

	// 统计结果
	validCount := 0
	invalidCount := 0
	for _, result := range results {
		if result["valid"].(bool) {
			validCount++
		} else {
			invalidCount++
		}
	}

	log.Printf("📊 临时分组验证结果统计: 总计=%d, 有效=%d, 无效=%d, 成功率=%.1f%%",
		len(req.APIKeys), validCount, invalidCount,
		float64(validCount)/float64(len(req.APIKeys))*100)

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"test_model":    testModel,
		"total_keys":    len(req.APIKeys),
		"valid_keys":    validCount,
		"invalid_keys":  invalidCount,
		"results":       results,
	})
}

// handleGetKeyValidationStatus 获取API密钥验证状态
func (s *MultiProviderServer) handleGetKeyValidationStatus(c *gin.Context) {
	groupID := c.Param("groupId")

	// 检查分组是否存在
	_, exists := s.configManager.GetGroup(groupID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Group not found",
		})
		return
	}

	// 获取验证状态
	validationStatus, err := s.configManager.GetAPIKeyValidationStatus(groupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to get validation status: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":           true,
		"group_id":          groupID,
		"validation_status": validationStatus,
	})
}
