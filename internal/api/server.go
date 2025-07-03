package api

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"turnsapi/internal"
	"turnsapi/internal/auth"
	"turnsapi/internal/keymanager"
	"turnsapi/internal/proxy"
	"turnsapi/internal/proxykey"

	"github.com/gin-gonic/gin"
)

// Server HTTP服务器
type Server struct {
	config          *internal.Config
	keyManager      *keymanager.KeyManager
	proxy           *proxy.OpenRouterProxy
	authManager     *auth.AuthManager
	proxyKeyManager *proxykey.Manager
	router          *gin.Engine
	httpServer      *http.Server

	// 模型列表缓存
	modelsCacheData []byte
	modelsCacheTime time.Time
	modelsCacheTTL  time.Duration
}

// NewServer 创建新的HTTP服务器
func NewServer(config *internal.Config, keyManager *keymanager.KeyManager) *Server {
	// 设置Gin模式
	if config.Logging.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	server := &Server{
		config:          config,
		keyManager:      keyManager,
		authManager:     auth.NewAuthManager(config),
		proxyKeyManager: proxykey.NewManager(),
		router:          gin.New(),
		modelsCacheTTL:  10 * time.Minute, // 模型列表缓存10分钟
	}

	// 创建代理
	server.proxy = proxy.NewOpenRouterProxy(config, keyManager)

	// 设置代理密钥管理器到认证管理器
	server.authManager.SetProxyKeyManager(server.proxyKeyManager)

	// 设置中间件
	server.setupMiddleware()

	// 设置路由
	server.setupRoutes()

	return server
}

// setupMiddleware 设置中间件
func (s *Server) setupMiddleware() {
	// 日志中间件
	s.router.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("%s - [%s] \"%s %s %s %d %s \"%s\" %s\"\n",
			param.ClientIP,
			param.TimeStamp.Format(time.RFC1123),
			param.Method,
			param.Path,
			param.Request.Proto,
			param.StatusCode,
			param.Latency,
			param.Request.UserAgent(),
			param.ErrorMessage,
		)
	}))

	// 恢复中间件
	s.router.Use(gin.Recovery())

	// CORS中间件
	s.router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})
}

// setupRoutes 设置路由
func (s *Server) setupRoutes() {
	// 认证路由（不需要认证）
	auth := s.router.Group("/auth")
	{
		auth.GET("/login", s.handleLoginPage)
		auth.POST("/login", s.handleLogin)
		auth.POST("/logout", s.handleLogout)
	}

	// API路由组（需要API密钥认证）
	api := s.router.Group("/api/v1")
	api.Use(s.authManager.APIKeyAuthMiddleware())
	{
		// OpenRouter兼容的聊天完成端点
		api.POST("/chat/completions", s.handleChatCompletions)
		// 模型列表端点
		api.GET("/models", s.handleModels)
	}

	// 管理路由组（需要认证）
	admin := s.router.Group("/admin")
	admin.Use(s.authManager.AuthMiddleware())
	{
		admin.GET("/status", s.handleStatus)
		admin.GET("/keys", s.handleKeysStatus)
		// API密钥管理
		admin.POST("/keys", s.handleAddKey)
		admin.POST("/keys/batch", s.handleAddKeysBatch)
		admin.PUT("/keys/:id", s.handleUpdateKey)
		admin.DELETE("/keys/:id", s.handleDeleteKey)
		// 代理服务API密钥管理
		admin.GET("/proxy-keys", s.handleProxyKeys)
		admin.POST("/proxy-keys", s.handleGenerateProxyKey)
		admin.DELETE("/proxy-keys/:id", s.handleDeleteProxyKey)
		// 获取完整模型列表（用于管理界面）
		admin.GET("/available-models", s.handleAvailableModels)
	}

	// 静态文件
	s.router.Static("/static", "./web/static")
	s.router.LoadHTMLGlob("web/templates/*")

	// Web界面（需要Web认证）
	s.router.GET("/", s.authManager.WebAuthMiddleware(), s.handleIndex)
	s.router.GET("/dashboard", s.authManager.WebAuthMiddleware(), s.handleDashboard)

	// 健康检查（不需要认证）
	s.router.GET("/health", s.handleHealth)
}

// handleChatCompletions 处理聊天完成请求
func (s *Server) handleChatCompletions(c *gin.Context) {
	s.proxy.HandleChatCompletions(c)
}

// handleModels 处理模型列表请求
func (s *Server) handleModels(c *gin.Context) {
	s.proxy.HandleModels(c)
}

// handleLoginPage 显示登录页面
func (s *Server) handleLoginPage(c *gin.Context) {
	// 如果认证未启用，重定向到首页
	if !s.config.Auth.Enabled {
		c.Redirect(http.StatusFound, "/")
		return
	}

	// 如果已经登录，重定向到仪表板
	if token, err := c.Cookie("auth_token"); err == nil && token != "" {
		if _, valid := s.authManager.ValidateToken(token); valid {
			c.Redirect(http.StatusFound, "/dashboard")
			return
		}
	}

	c.HTML(http.StatusOK, "login.html", gin.H{
		"title": "TurnsAPI - 登录",
	})
}

// handleLogin 处理登录请求
func (s *Server) handleLogin(c *gin.Context) {
	if !s.config.Auth.Enabled {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	var loginReq struct {
		Username string `json:"username" form:"username"`
		Password string `json:"password" form:"password"`
	}

	if err := c.ShouldBind(&loginReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format",
			"code":  "invalid_request",
		})
		return
	}

	session, err := s.authManager.Login(loginReq.Username, loginReq.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid username or password",
			"code":  "invalid_credentials",
		})
		return
	}

	// 设置cookie
	c.SetCookie("auth_token", session.Token, int(s.config.Auth.SessionTimeout.Seconds()), "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"token":   session.Token,
		"expires": session.ExpiresAt,
	})
}

// handleLogout 处理登出请求
func (s *Server) handleLogout(c *gin.Context) {
	token, err := c.Cookie("auth_token")
	if err == nil && token != "" {
		s.authManager.Logout(token)
	}

	// 清除cookie
	c.SetCookie("auth_token", "", -1, "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

// handleAddKey 添加API密钥
func (s *Server) handleAddKey(c *gin.Context) {
	var req struct {
		Key           string   `json:"key" binding:"required"`
		Name          string   `json:"name"`
		Description   string   `json:"description"`
		AllowedModels []string `json:"allowed_models"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format",
			"code":  "invalid_request",
		})
		return
	}

	// 添加密钥到密钥管理器
	if err := s.keyManager.AddKey(req.Key, req.Name, req.Description, req.AllowedModels); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
			"code":  "add_key_failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "API密钥添加成功",
	})
}

// handleAddKeysBatch 批量添加API密钥
func (s *Server) handleAddKeysBatch(c *gin.Context) {
	var req struct {
		Keys []string `json:"keys" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format",
			"code":  "invalid_request",
		})
		return
	}

	if len(req.Keys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "密钥列表不能为空",
			"code":  "empty_keys",
		})
		return
	}

	// 批量添加密钥
	addedCount, errors, err := s.keyManager.AddKeysInBatch(req.Keys)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
			"code":  "batch_add_failed",
		})
		return
	}

	response := gin.H{
		"success":       true,
		"message":       fmt.Sprintf("批量添加完成，成功添加 %d 个密钥", addedCount),
		"added_count":   addedCount,
		"total_count":   len(req.Keys),
		"skipped_count": len(req.Keys) - addedCount,
	}

	if len(errors) > 0 {
		response["errors"] = errors
	}

	c.JSON(http.StatusOK, response)
}

// handleUpdateKey 更新API密钥
func (s *Server) handleUpdateKey(c *gin.Context) {
	keyID := c.Param("id")
	var req struct {
		Name          string   `json:"name"`
		Description   string   `json:"description"`
		IsActive      *bool    `json:"is_active"`
		AllowedModels []string `json:"allowed_models"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format",
			"code":  "invalid_request",
		})
		return
	}

	// 更新密钥信息
	if err := s.keyManager.UpdateKey(keyID, req.Name, req.Description, req.IsActive, req.AllowedModels); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
			"code":  "update_key_failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "API密钥更新成功",
	})
}

// handleDeleteKey 删除API密钥
func (s *Server) handleDeleteKey(c *gin.Context) {
	keyID := c.Param("id")

	if err := s.keyManager.DeleteKey(keyID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
			"code":  "delete_key_failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "API密钥删除成功",
	})
}

// handleProxyKeys 获取代理服务API密钥列表
func (s *Server) handleProxyKeys(c *gin.Context) {
	keys := s.proxyKeyManager.GetAllKeys()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"keys":    keys,
	})
}

// handleGenerateProxyKey 生成代理服务API密钥
func (s *Server) handleGenerateProxyKey(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format",
			"code":  "invalid_request",
		})
		return
	}

	key, err := s.proxyKeyManager.GenerateKey(req.Name, req.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to generate proxy key",
			"code":  "generate_key_failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"key":     key,
		"message": "代理服务API密钥生成成功",
	})
}

// handleDeleteProxyKey 删除代理服务API密钥
func (s *Server) handleDeleteProxyKey(c *gin.Context) {
	keyID := c.Param("id")

	if err := s.proxyKeyManager.DeleteKey(keyID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
			"code":  "delete_proxy_key_failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "代理服务API密钥删除成功",
	})
}

// handleStatus 处理状态查询
func (s *Server) handleStatus(c *gin.Context) {
	keyStatuses := s.keyManager.GetKeyStatuses()

	activeCount := 0
	totalCount := len(keyStatuses)

	for _, status := range keyStatuses {
		if status.IsActive {
			activeCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":      "ok",
		"timestamp":   time.Now(),
		"active_keys": activeCount,
		"total_keys":  totalCount,
		"uptime":      time.Since(time.Now()), // 这里应该记录实际的启动时间
	})
}

// handleKeysStatus 处理密钥状态查询
func (s *Server) handleKeysStatus(c *gin.Context) {
	keyStatuses := s.keyManager.GetKeyStatuses()
	c.JSON(http.StatusOK, gin.H{
		"keys": keyStatuses,
	})
}

// handleIndex 处理首页
func (s *Server) handleIndex(c *gin.Context) {
	c.HTML(http.StatusOK, "index.html", gin.H{
		"title": "TurnsAPI - OpenRouter Proxy",
	})
}

// handleDashboard 处理仪表板页面
func (s *Server) handleDashboard(c *gin.Context) {
	keyStatuses := s.keyManager.GetKeyStatuses()

	activeCount := 0
	totalUsage := int64(0)
	totalErrors := int64(0)

	for _, status := range keyStatuses {
		if status.IsActive {
			activeCount++
		}
		totalUsage += status.UsageCount
		totalErrors += status.ErrorCount
	}

	c.HTML(http.StatusOK, "dashboard.html", gin.H{
		"title":        "Dashboard - TurnsAPI",
		"keys":         keyStatuses,
		"active_count": activeCount,
		"total_count":  len(keyStatuses),
		"total_usage":  totalUsage,
		"total_errors": totalErrors,
	})
}

// handleHealth 处理健康检查
func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now(),
	})
}

// Start 启动服务器
func (s *Server) Start() error {
	s.httpServer = &http.Server{
		Addr:    s.config.GetAddress(),
		Handler: s.router,
	}

	log.Printf("Starting server on %s", s.config.GetAddress())
	return s.httpServer.ListenAndServe()
}

// Stop 停止服务器
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// handleAvailableModels 获取完整的模型列表（用于管理界面）
func (s *Server) handleAvailableModels(c *gin.Context) {
	// 检查缓存
	if s.modelsCacheData != nil && time.Since(s.modelsCacheTime) < s.modelsCacheTTL {
		c.Data(http.StatusOK, "application/json", s.modelsCacheData)
		return
	}

	// 获取API密钥
	apiKey, err := s.keyManager.GetNextKey()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "No available API keys",
			"code":  "no_api_keys",
		})
		return
	}

	// 创建请求到OpenRouter
	req, err := http.NewRequest("GET", s.config.OpenRouter.BaseURL+"/models", nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create request",
			"code":  "request_creation_failed",
		})
		return
	}

	// 设置请求头
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.keyManager.ReportError(apiKey, err.Error())
		c.JSON(http.StatusBadGateway, gin.H{
			"error": "Failed to connect to OpenRouter",
			"code":  "upstream_connection_failed",
		})
		return
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.keyManager.ReportError(apiKey, err.Error())
		c.JSON(http.StatusBadGateway, gin.H{
			"error": "Failed to read response",
			"code":  "response_read_failed",
		})
		return
	}

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		s.keyManager.ReportError(apiKey, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)))
		c.Data(resp.StatusCode, "application/json", body)
		return
	}

	// 报告成功
	s.keyManager.ReportSuccess(apiKey)

	// 更新缓存
	s.modelsCacheData = body
	s.modelsCacheTime = time.Now()

	// 返回完整的模型列表（不过滤）
	c.Data(http.StatusOK, "application/json", body)
}
