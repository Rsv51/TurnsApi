package router

import (
	"fmt"
	"strings"

	"turnsapi/internal"
	"turnsapi/internal/providers"
)

// ProviderRouter 提供商路由器
type ProviderRouter struct {
	config          *internal.Config
	providerManager *providers.ProviderManager
}

// NewProviderRouter 创建提供商路由器
func NewProviderRouter(config *internal.Config, providerManager *providers.ProviderManager) *ProviderRouter {
	return &ProviderRouter{
		config:          config,
		providerManager: providerManager,
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
		BaseURL:      group.BaseURL,
		APIKey:       apiKey,
		Timeout:      group.Timeout,
		MaxRetries:   group.MaxRetries,
		Headers:      make(map[string]string),
		ProviderType: group.ProviderType,
	}

	// 复制头部信息
	for key, value := range group.Headers {
		config.Headers[key] = value
	}

	return config, nil
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
