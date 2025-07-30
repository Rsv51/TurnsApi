package logger

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestLogger 请求日志记录器
type RequestLogger struct {
	db *Database
}

// NewRequestLogger 创建新的请求日志记录器
func NewRequestLogger(dbPath string) (*RequestLogger, error) {
	db, err := NewDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	return &RequestLogger{
		db: db,
	}, nil
}

// Close 关闭日志记录器
func (r *RequestLogger) Close() error {
	return r.db.Close()
}

// LogRequest 记录请求日志
func (r *RequestLogger) LogRequest(
	proxyKeyName, proxyKeyID, providerGroup, openRouterKey, model, requestBody, responseBody, clientIP string,
	statusCode int, isStream bool, duration time.Duration, err error,
) {
	// 创建日志记录
	requestLog := &RequestLog{
		ProxyKeyName:  proxyKeyName,
		ProxyKeyID:    proxyKeyID,
		ProviderGroup: providerGroup,
		OpenRouterKey: r.maskAPIKey(openRouterKey),
		Model:         model,
		RequestBody:   requestBody,
		ResponseBody:  responseBody,
		StatusCode:    statusCode,
		IsStream:      isStream,
		Duration:      duration.Milliseconds(),
		TokensUsed:    r.extractTokensUsed(responseBody),
		ClientIP:      clientIP,
		CreatedAt:     time.Now(),
	}

	// 如果有错误，记录错误信息
	if err != nil {
		requestLog.Error = err.Error()
	}

	// 插入数据库
	if insertErr := r.db.InsertRequestLog(requestLog); insertErr != nil {
		log.Printf("Failed to insert request log: %v", insertErr)
	}
}

// GetRequestLogs 获取请求日志列表
func (r *RequestLogger) GetRequestLogs(proxyKeyName, providerGroup string, limit, offset int) ([]*RequestLogSummary, error) {
	return r.db.GetRequestLogs(proxyKeyName, providerGroup, limit, offset)
}

// GetRequestLogsWithFilter 根据筛选条件获取请求日志列表
func (r *RequestLogger) GetRequestLogsWithFilter(filter *LogFilter) ([]*RequestLogSummary, error) {
	return r.db.GetRequestLogsWithFilter(filter)
}

// GetRequestCountWithFilter 根据筛选条件获取请求总数
func (r *RequestLogger) GetRequestCountWithFilter(filter *LogFilter) (int64, error) {
	return r.db.GetRequestCountWithFilter(filter)
}

// GetRequestLogDetail 获取请求日志详情
func (r *RequestLogger) GetRequestLogDetail(id int64) (*RequestLog, error) {
	return r.db.GetRequestLogDetail(id)
}

// GetProxyKeyStats 获取代理密钥统计
func (r *RequestLogger) GetProxyKeyStats() ([]*ProxyKeyStats, error) {
	return r.db.GetProxyKeyStats()
}

// GetModelStats 获取模型统计
func (r *RequestLogger) GetModelStats() ([]*ModelStats, error) {
	return r.db.GetModelStats()
}

// GetRequestCount 获取请求总数
func (r *RequestLogger) GetRequestCount(proxyKeyName, providerGroup string) (int64, error) {
	return r.db.GetRequestCount(proxyKeyName, providerGroup)
}

// GetTotalTokensStats 获取总token数统计
func (r *RequestLogger) GetTotalTokensStats() (*TotalTokensStats, error) {
	return r.db.GetTotalTokensStats()
}

// InsertProxyKey 插入代理密钥
func (r *RequestLogger) InsertProxyKey(key *ProxyKey) error {
	return r.db.InsertProxyKey(key)
}

// GetProxyKey 根据密钥获取代理密钥信息
func (r *RequestLogger) GetProxyKey(keyValue string) (*ProxyKey, error) {
	return r.db.GetProxyKey(keyValue)
}

// GetAllProxyKeys 获取所有代理密钥
func (r *RequestLogger) GetAllProxyKeys() ([]*ProxyKey, error) {
	return r.db.GetAllProxyKeys()
}

// UpdateProxyKey 更新代理密钥信息
func (r *RequestLogger) UpdateProxyKey(key *ProxyKey) error {
	return r.db.UpdateProxyKey(key)
}

// UpdateProxyKeyLastUsed 更新代理密钥最后使用时间
func (r *RequestLogger) UpdateProxyKeyLastUsed(keyID string) error {
	return r.db.UpdateProxyKeyLastUsed(keyID)
}

// DeleteProxyKey 删除代理密钥
func (r *RequestLogger) DeleteProxyKey(keyID string) error {
	return r.db.DeleteProxyKey(keyID)
}

// CleanupOldLogs 清理旧日志
func (r *RequestLogger) CleanupOldLogs(retentionDays int) error {
	return r.db.CleanupOldLogs(retentionDays)
}

// DeleteRequestLogs 批量删除请求日志
func (r *RequestLogger) DeleteRequestLogs(ids []int64) (int64, error) {
	return r.db.DeleteRequestLogs(ids)
}

// ClearAllRequestLogs 清空所有请求日志
func (r *RequestLogger) ClearAllRequestLogs() (int64, error) {
	return r.db.ClearAllRequestLogs()
}

// GetAllRequestLogsForExport 获取所有请求日志用于导出
func (r *RequestLogger) GetAllRequestLogsForExport(proxyKeyName, providerGroup string) ([]*RequestLog, error) {
	return r.db.GetAllRequestLogsForExport(proxyKeyName, providerGroup)
}

// GetAllRequestLogsForExportWithFilter 根据筛选条件获取所有请求日志用于导出
func (r *RequestLogger) GetAllRequestLogsForExportWithFilter(filter *LogFilter) ([]*RequestLog, error) {
	return r.db.GetAllRequestLogsForExportWithFilter(filter)
}

// maskAPIKey 遮蔽API密钥敏感信息
func (r *RequestLogger) maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return strings.Repeat("*", len(apiKey))
	}
	return apiKey[:4] + strings.Repeat("*", len(apiKey)-8) + apiKey[len(apiKey)-4:]
}

// extractTokensUsed 从响应中提取使用的token数量
func (r *RequestLogger) extractTokensUsed(responseBody string) int {
	if responseBody == "" {
		return 0
	}

	// 首先尝试解析JSON响应（非流式）
	var response map[string]interface{}
	if err := json.Unmarshal([]byte(responseBody), &response); err == nil {
		// 查找usage字段
		if usage, ok := response["usage"].(map[string]interface{}); ok {
			if totalTokens, ok := usage["total_tokens"].(float64); ok {
				return int(totalTokens)
			}
		}
		return 0
	}

	// 如果JSON解析失败，尝试从流式响应中提取token数
	return r.extractTokensFromStream(responseBody)
}

// extractTokensFromStream 从流式响应中提取token数量
func (r *RequestLogger) extractTokensFromStream(streamBody string) int {
	if streamBody == "" {
		return 0
	}

	lines := strings.Split(streamBody, "\n")
	totalTokens := 0

	// 从后往前遍历，因为token统计通常在最后几个chunk中
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])

		// 跳过非数据行
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		dataStr := strings.TrimPrefix(line, "data: ")

		// 跳过[DONE]标记、空行和处理状态信息
		if dataStr == "[DONE]" || dataStr == "" ||
			strings.Contains(dataStr, "OPENROUTER PROCESSING") ||
			strings.Contains(dataStr, "PROCESSING") {
			continue
		}

		// 尝试解析JSON数据
		var chunkData map[string]interface{}
		if err := json.Unmarshal([]byte(dataStr), &chunkData); err != nil {
			// 如果JSON解析失败，记录调试信息但继续处理
			log.Printf("Failed to parse JSON chunk: %s, error: %v", dataStr[:min(100, len(dataStr))], err)
			continue
		}

		// 查找usage字段（OpenAI格式）
		if usage, ok := chunkData["usage"].(map[string]interface{}); ok {
			if tokens, ok := usage["total_tokens"].(float64); ok {
				totalTokens = int(tokens)
				log.Printf("Found tokens in stream: %d", totalTokens)
				break // 找到token统计就退出
			}
		}

		// 查找Gemini原生格式的usageMetadata字段
		if usageMetadata, ok := chunkData["usageMetadata"].(map[string]interface{}); ok {
			if tokens, ok := usageMetadata["totalTokenCount"].(float64); ok {
				totalTokens = int(tokens)
				log.Printf("Found Gemini tokens in stream: %d", totalTokens)
				break // 找到token统计就退出
			}
		}

		// 查找Anthropic格式的usage字段
		if usage, ok := chunkData["usage"].(map[string]interface{}); ok {
			if inputTokens, ok1 := usage["input_tokens"].(float64); ok1 {
				if outputTokens, ok2 := usage["output_tokens"].(float64); ok2 {
					totalTokens = int(inputTokens + outputTokens)
					log.Printf("Found Anthropic tokens in stream: %d", totalTokens)
					break
				}
			}
		}
	}

	if totalTokens == 0 {
		log.Printf("No tokens found in stream response, response length: %d", len(streamBody))
	}

	return totalTokens
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetClientIP 获取客户端真实IP地址
func GetClientIP(c *gin.Context) string {
	// 优先从X-Forwarded-For头获取
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For可能包含多个IP，取第一个
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// 从X-Real-IP头获取
	if xri := c.GetHeader("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// 从RemoteAddr获取
	if ip := c.ClientIP(); ip != "" {
		return ip
	}

	// 最后从Request.RemoteAddr获取
	if remoteAddr := c.Request.RemoteAddr; remoteAddr != "" {
		if idx := strings.LastIndex(remoteAddr, ":"); idx != -1 {
			return remoteAddr[:idx]
		}
		return remoteAddr
	}

	return "unknown"
}
