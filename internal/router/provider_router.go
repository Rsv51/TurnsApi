package router

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"turnsapi/internal"
	"turnsapi/internal/providers"
)

// GroupFailureInfo 分组失败信息
type GroupFailureInfo struct {
	FailureCount int
	LastFailure  time.Time
}

// ProviderRouter 提供商路由器
type ProviderRouter struct {
	config          *internal.Config
	providerManager *providers.ProviderManager
	failureTracker  map[string]map[string]*GroupFailureInfo // model -> groupID -> failure info
	mutex           sync.RWMutex
}

// NewProviderRouter 创建提供商路由器
func NewProviderRouter(config *internal.Config, providerManager *providers.ProviderManager) *ProviderRouter {
	return &ProviderRouter{
		config:          config,
		providerManager: providerManager,
		failureTracker:  make(map[string]map[string]*GroupFailureInfo),
	}
}

// RouteRequest 路由请求结构
type RouteRequest struct {
	Model         string   `json:"model"`
	ProviderGroup string   `json:"provider_group,omitempty"` // 可选的显式提供商分组
	AllowedGroups []string `json:"allowed_groups,omitempty"` // 代理密钥允许访问的分组
}

// RouteResult 路由结果
type RouteResult struct {
	GroupID      string
	Group        *internal.UserGroup
	Provider     providers.Provider
	ProviderConfig *providers.ProviderConfig
}

// Route 根据请求路由到合适的提供商
func (pr *ProviderRouter) Route(req *RouteRequest) (*RouteResult, error) {
	var group *internal.UserGroup
	var groupID string

	// 1. 如果显式指定了提供商分组，优先使用
	if req.ProviderGroup != "" {
		var exists bool
		group, exists = pr.config.GetGroupByID(req.ProviderGroup)
		if !exists {
			return nil, fmt.Errorf("specified provider group '%s' not found", req.ProviderGroup)
		}
		if !group.Enabled {
			return nil, fmt.Errorf("specified provider group '%s' is disabled", req.ProviderGroup)
		}

		// 检查代理密钥是否有权限访问指定分组
		if !pr.hasGroupAccess(req.AllowedGroups, req.ProviderGroup) {
			return nil, fmt.Errorf("access denied to provider group '%s'", req.ProviderGroup)
		}

		groupID = req.ProviderGroup
	} else {
		// 2. 根据模型名称和代理密钥权限自动路由
		group, groupID = pr.routeByModelWithPermissions(req.Model, req.AllowedGroups)
		if group == nil {
			return nil, fmt.Errorf("no suitable provider group found for model '%s' with current permissions", req.Model)
		}
	}

	// 3. 创建提供商配置
	providerConfig, err := pr.createProviderConfig(groupID, group)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider config for group '%s': %w", groupID, err)
	}

	// 4. 获取提供商实例
	provider, err := pr.providerManager.GetProvider(groupID, providerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider for group '%s': %w", groupID, err)
	}

	return &RouteResult{
		GroupID:        groupID,
		Group:          group,
		Provider:       provider,
		ProviderConfig: providerConfig,
	}, nil
}

// routeByModel 根据模型名称路由
func (pr *ProviderRouter) routeByModel(modelName string) (*internal.UserGroup, string) {
	// 1. 首先检查是否有分组明确支持该模型
	for groupID, group := range pr.config.UserGroups {
		if !group.Enabled {
			continue
		}
		
		// 如果分组指定了模型列表，检查是否包含该模型
		if len(group.Models) > 0 {
			for _, model := range group.Models {
				if model == modelName {
					return group, groupID
				}
			}
		}
	}

	// 2. 如果没有明确支持，尝试基于模型名称的模式匹配
	return pr.routeByModelPattern(modelName)
}

// routeByModelPattern 根据模型名称模式路由
func (pr *ProviderRouter) routeByModelPattern(modelName string) (*internal.UserGroup, string) {
	modelLower := strings.ToLower(modelName)

	// 定义模型名称模式到提供商类型的映射
	patterns := map[string]string{
		"gpt":     "openai",
		"claude":  "anthropic",
		"gemini":  "gemini",
		"o1":      "openai",
		"davinci": "openai",
		"turbo":   "openai",
	}

	// 查找匹配的模式
	var targetProviderType string
	for pattern, providerType := range patterns {
		if strings.Contains(modelLower, pattern) {
			targetProviderType = providerType
			break
		}
	}

	if targetProviderType == "" {
		// 如果没有匹配的模式，返回第一个启用的分组
		return pr.getFirstEnabledGroup()
	}

	// 查找匹配提供商类型的分组
	for groupID, group := range pr.config.UserGroups {
		if group.Enabled && group.ProviderType == targetProviderType {
			// 如果分组没有指定模型列表，或者模型列表为空，则认为支持所有该类型的模型
			if len(group.Models) == 0 {
				return group, groupID
			}
		}
	}

	// 如果没有找到匹配的分组，返回第一个启用的分组
	return pr.getFirstEnabledGroup()
}

// getFirstEnabledGroup 获取第一个启用的分组
func (pr *ProviderRouter) getFirstEnabledGroup() (*internal.UserGroup, string) {
	for groupID, group := range pr.config.UserGroups {
		if group.Enabled {
			return group, groupID
		}
	}
	return nil, ""
}

// createProviderConfig 创建提供商配置
func (pr *ProviderRouter) createProviderConfig(groupID string, group *internal.UserGroup) (*providers.ProviderConfig, error) {
	if len(group.APIKeys) == 0 {
		return nil, fmt.Errorf("no API keys configured for group '%s'", groupID)
	}

	// 这里暂时使用第一个API密钥，实际使用时会通过KeyManager获取
	apiKey := group.APIKeys[0]

	config := &providers.ProviderConfig{
		BaseURL:       group.BaseURL,
		APIKey:        apiKey,
		Timeout:       group.Timeout,
		MaxRetries:    group.MaxRetries,
		Headers:       make(map[string]string),
		ProviderType:  group.ProviderType,
		RequestParams: make(map[string]interface{}),
	}

	// 复制头部信息
	for key, value := range group.Headers {
		config.Headers[key] = value
	}

	// 复制请求参数覆盖
	for key, value := range group.RequestParams {
		config.RequestParams[key] = value
	}

	return config, nil
}

// GetGroupsForModel 获取支持特定模型的所有分组（按优先级排序，仅限于允许的分组范围内）
func (pr *ProviderRouter) GetGroupsForModel(modelName string, allowedGroups []string) []string {
	pr.mutex.RLock()
	defer pr.mutex.RUnlock()

	var candidateGroups []string

	// 获取有权限访问的分组列表
	accessibleGroups := pr.getAccessibleGroups(allowedGroups)
	if len(accessibleGroups) == 0 {
		return candidateGroups // 返回空列表
	}

	// 1. 首先检查明确支持该模型的分组（仅在允许的分组范围内）
	for _, groupID := range accessibleGroups {
		group := pr.config.UserGroups[groupID]
		if !group.Enabled {
			continue
		}

		// 检查是否明确支持该模型
		if len(group.Models) > 0 {
			for _, model := range group.Models {
				if model == modelName {
					candidateGroups = append(candidateGroups, groupID)
					break
				}
			}
		}
	}

	// 2. 如果没有明确支持的分组，尝试基于模型名称的模式匹配（仅在允许的分组范围内）
	if len(candidateGroups) == 0 {
		targetProviderType := pr.inferProviderTypeFromModel(modelName)
		if targetProviderType != "" {
			for _, groupID := range accessibleGroups {
				group := pr.config.UserGroups[groupID]
				if !group.Enabled {
					continue
				}

				if group.ProviderType == targetProviderType {
					// 如果分组没有指定模型列表，或者模型列表为空，则认为支持所有该类型的模型
					if len(group.Models) == 0 {
						candidateGroups = append(candidateGroups, groupID)
					}
				}
			}
		}
	}

	// 3. 按失败次数排序（失败次数少的优先）
	return pr.sortGroupsByFailureCount(modelName, candidateGroups)
}

// getAccessibleGroups 获取有权限访问的分组列表
func (pr *ProviderRouter) getAccessibleGroups(allowedGroups []string) []string {
	var accessibleGroups []string

	// 如果allowedGroups为空或nil，表示可以访问所有分组
	if len(allowedGroups) == 0 {
		for groupID, group := range pr.config.UserGroups {
			if group.Enabled {
				accessibleGroups = append(accessibleGroups, groupID)
			}
		}
		return accessibleGroups
	}

	// 否则只返回允许访问的分组
	for _, groupID := range allowedGroups {
		if group, exists := pr.config.UserGroups[groupID]; exists && group.Enabled {
			accessibleGroups = append(accessibleGroups, groupID)
		}
	}

	return accessibleGroups
}

// sortGroupsByFailureCount 按失败次数对分组进行排序
func (pr *ProviderRouter) sortGroupsByFailureCount(modelName string, groups []string) []string {
	if len(groups) <= 1 {
		return groups
	}

	// 获取每个分组的失败次数
	type groupScore struct {
		groupID      string
		failureCount int
		lastFailure  time.Time
	}

	var scores []groupScore
	modelFailures, exists := pr.failureTracker[modelName]

	for _, groupID := range groups {
		score := groupScore{groupID: groupID}
		if exists {
			if info, hasFailure := modelFailures[groupID]; hasFailure {
				score.failureCount = info.FailureCount
				score.lastFailure = info.LastFailure
			}
		}
		scores = append(scores, score)
	}

	// 简单排序：失败次数少的优先，失败次数相同的按最后失败时间排序
	for i := 0; i < len(scores)-1; i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[i].failureCount > scores[j].failureCount ||
				(scores[i].failureCount == scores[j].failureCount && scores[i].lastFailure.After(scores[j].lastFailure)) {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	result := make([]string, len(scores))
	for i, score := range scores {
		result[i] = score.groupID
	}

	return result
}

// RouteWithRetry 智能路由，支持失败重试
func (pr *ProviderRouter) RouteWithRetry(req *RouteRequest) (*RouteResult, error) {
	// 如果显式指定了提供商分组，直接使用
	if req.ProviderGroup != "" {
		group, exists := pr.config.UserGroups[req.ProviderGroup]
		if !exists {
			return nil, fmt.Errorf("specified provider group '%s' not found", req.ProviderGroup)
		}
		if !group.Enabled {
			return nil, fmt.Errorf("specified provider group '%s' is disabled", req.ProviderGroup)
		}

		// 创建提供商配置
		providerConfig, err := pr.createProviderConfig(req.ProviderGroup, group)
		if err != nil {
			return nil, fmt.Errorf("failed to create provider config for group '%s': %w", req.ProviderGroup, err)
		}

		// 获取提供商实例
		provider, err := pr.providerManager.GetProvider(req.ProviderGroup, providerConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to get provider for group '%s': %w", req.ProviderGroup, err)
		}

		return &RouteResult{
			GroupID:        req.ProviderGroup,
			Group:          group,
			Provider:       provider,
			ProviderConfig: providerConfig,
		}, nil
	}

	// 获取支持该模型的所有分组（按优先级排序）
	candidateGroups := pr.GetGroupsForModel(req.Model, req.AllowedGroups)
	if len(candidateGroups) == 0 {
		return nil, fmt.Errorf("no suitable provider group found for model '%s' with current permissions", req.Model)
	}

	// 尝试每个候选分组
	for _, groupID := range candidateGroups {
		// 检查该分组是否已经失败太多次
		if pr.isGroupTemporarilyBlocked(req.Model, groupID) {
			continue
		}

		group := pr.config.UserGroups[groupID]

		// 创建提供商配置
		providerConfig, err := pr.createProviderConfig(groupID, group)
		if err != nil {
			continue
		}

		// 获取提供商实例
		provider, err := pr.providerManager.GetProvider(groupID, providerConfig)
		if err != nil {
			continue
		}

		return &RouteResult{
			GroupID:        groupID,
			Group:          group,
			Provider:       provider,
			ProviderConfig: providerConfig,
		}, nil
	}

	return nil, fmt.Errorf("all suitable provider groups are temporarily unavailable for model '%s'", req.Model)
}

// isGroupTemporarilyBlocked 检查分组是否因失败次数过多而被临时阻止
func (pr *ProviderRouter) isGroupTemporarilyBlocked(modelName, groupID string) bool {
	pr.mutex.RLock()
	defer pr.mutex.RUnlock()

	modelFailures, exists := pr.failureTracker[modelName]
	if !exists {
		return false
	}

	info, hasFailure := modelFailures[groupID]
	if !hasFailure {
		return false
	}

	// 如果失败次数达到3次，则临时阻止（可以配置）
	const maxFailures = 3
	const blockDuration = 5 * time.Minute // 阻止5分钟

	if info.FailureCount >= maxFailures {
		// 检查是否已经过了阻止时间
		if time.Since(info.LastFailure) < blockDuration {
			return true
		} else {
			// 重置失败计数
			info.FailureCount = 0
		}
	}

	return false
}

// ReportFailure 报告分组失败
func (pr *ProviderRouter) ReportFailure(modelName, groupID string) {
	pr.mutex.Lock()
	defer pr.mutex.Unlock()

	if pr.failureTracker[modelName] == nil {
		pr.failureTracker[modelName] = make(map[string]*GroupFailureInfo)
	}

	if pr.failureTracker[modelName][groupID] == nil {
		pr.failureTracker[modelName][groupID] = &GroupFailureInfo{}
	}

	info := pr.failureTracker[modelName][groupID]
	info.FailureCount++
	info.LastFailure = time.Now()
}

// ReportSuccess 报告分组成功
func (pr *ProviderRouter) ReportSuccess(modelName, groupID string) {
	pr.mutex.Lock()
	defer pr.mutex.Unlock()

	if pr.failureTracker[modelName] != nil {
		if info, exists := pr.failureTracker[modelName][groupID]; exists {
			// 成功后重置失败计数
			info.FailureCount = 0
		}
	}
}

// GetAvailableGroups 获取所有可用的分组
func (pr *ProviderRouter) GetAvailableGroups() map[string]*internal.UserGroup {
	return pr.config.GetEnabledGroups()
}

// GetGroupInfo 获取分组信息
func (pr *ProviderRouter) GetGroupInfo(groupID string) (*internal.UserGroup, bool) {
	return pr.config.GetGroupByID(groupID)
}

// ValidateModel 验证模型是否被任何分组支持
func (pr *ProviderRouter) ValidateModel(modelName string) bool {
	group, _ := pr.routeByModel(modelName)
	return group != nil
}

// hasGroupAccess 检查代理密钥是否有权限访问指定分组
func (pr *ProviderRouter) hasGroupAccess(allowedGroups []string, groupID string) bool {
	// 如果没有限制，可以访问所有分组
	if len(allowedGroups) == 0 {
		return true
	}

	// 检查分组是否在允许列表中
	for _, allowedGroup := range allowedGroups {
		if allowedGroup == groupID {
			return true
		}
	}

	return false
}

// routeByModelWithPermissions 根据模型名称和权限路由
func (pr *ProviderRouter) routeByModelWithPermissions(modelName string, allowedGroups []string) (*internal.UserGroup, string) {
	// 首先尝试精确匹配模型
	for groupID, group := range pr.config.UserGroups {
		if !group.Enabled {
			continue
		}

		// 检查权限
		if !pr.hasGroupAccess(allowedGroups, groupID) {
			continue
		}

		// 检查模型是否在分组的模型列表中
		for _, model := range group.Models {
			if model == modelName {
				return group, groupID
			}
		}
	}

	// 如果没有精确匹配，尝试根据模型名称推断提供商类型
	targetProviderType := pr.inferProviderTypeFromModel(modelName)
	if targetProviderType == "" {
		// 如果无法推断，返回第一个有权限的启用分组
		return pr.getFirstEnabledGroupWithPermissions(allowedGroups)
	}

	// 查找匹配提供商类型的分组
	for groupID, group := range pr.config.UserGroups {
		if !group.Enabled {
			continue
		}

		// 检查权限
		if !pr.hasGroupAccess(allowedGroups, groupID) {
			continue
		}

		if group.ProviderType == targetProviderType {
			// 如果分组没有指定模型列表，或者模型列表为空，则认为支持所有该类型的模型
			if len(group.Models) == 0 {
				return group, groupID
			}
		}
	}

	// 如果没有找到匹配的分组，返回第一个有权限的启用分组
	return pr.getFirstEnabledGroupWithPermissions(allowedGroups)
}

// getFirstEnabledGroupWithPermissions 获取第一个有权限的启用分组
func (pr *ProviderRouter) getFirstEnabledGroupWithPermissions(allowedGroups []string) (*internal.UserGroup, string) {
	for groupID, group := range pr.config.UserGroups {
		if group.Enabled && pr.hasGroupAccess(allowedGroups, groupID) {
			return group, groupID
		}
	}
	return nil, ""
}

// inferProviderTypeFromModel 从模型名称推断提供商类型
func (pr *ProviderRouter) inferProviderTypeFromModel(modelName string) string {
	modelLower := strings.ToLower(modelName)

	// 定义模型名称模式到提供商类型的映射
	patterns := map[string]string{
		"gpt":     "openai",
		"claude":  "anthropic",
		"gemini":  "gemini",
		"o1":      "openai",
		"davinci": "openai",
		"turbo":   "openai",
	}

	// 查找匹配的模式
	for pattern, providerType := range patterns {
		if strings.Contains(modelLower, pattern) {
			return providerType
		}
	}

	return ""
}

// GetSupportedModels 获取所有支持的模型列表
func (pr *ProviderRouter) GetSupportedModels() []string {
	modelSet := make(map[string]bool)
	
	for _, group := range pr.config.UserGroups {
		if !group.Enabled {
			continue
		}
		
		// 如果分组指定了模型列表，添加这些模型
		if len(group.Models) > 0 {
			for _, model := range group.Models {
				modelSet[model] = true
			}
		}
	}
	
	// 转换为切片
	models := make([]string, 0, len(modelSet))
	for model := range modelSet {
		models = append(models, model)
	}
	
	return models
}

// GetProviderTypeForGroup 获取分组的提供商类型
func (pr *ProviderRouter) GetProviderTypeForGroup(groupID string) (string, error) {
	group, exists := pr.config.GetGroupByID(groupID)
	if !exists {
		return "", fmt.Errorf("group '%s' not found", groupID)
	}
	
	return group.ProviderType, nil
}

// UpdateProviderConfig 更新提供商配置中的API密钥
func (pr *ProviderRouter) UpdateProviderConfig(config *providers.ProviderConfig, apiKey string) {
	config.APIKey = apiKey
}
