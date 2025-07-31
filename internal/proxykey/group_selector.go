package proxykey

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// GroupSelector 分组选择器，用于在多个允许分组之间进行选择
type GroupSelector struct {
	config           *GroupSelectionConfig
	allowedGroups    []string
	currentIndex     int
	groupUsageCount  map[string]int64
	lastUsedTime     map[string]time.Time
	mutex            sync.RWMutex
	weightedSelector *WeightedSelector
}

// WeightedSelector 权重选择器
type WeightedSelector struct {
	groups  []string
	weights []int
	total   int
}

// NewGroupSelector 创建新的分组选择器
func NewGroupSelector(allowedGroups []string, config *GroupSelectionConfig) *GroupSelector {
	if config == nil {
		config = &GroupSelectionConfig{
			Strategy: GroupSelectionRoundRobin,
		}
	}

	selector := &GroupSelector{
		config:          config,
		allowedGroups:   allowedGroups,
		groupUsageCount: make(map[string]int64),
		lastUsedTime:    make(map[string]time.Time),
	}

	// 如果是权重策略，初始化权重选择器
	if config.Strategy == GroupSelectionWeighted && len(config.GroupWeights) > 0 {
		selector.initWeightedSelector()
	}

	return selector
}

// initWeightedSelector 初始化权重选择器
func (gs *GroupSelector) initWeightedSelector() {
	weightMap := make(map[string]int)
	for _, gw := range gs.config.GroupWeights {
		weightMap[gw.GroupID] = gw.Weight
	}

	var groups []string
	var weights []int
	total := 0

	for _, groupID := range gs.allowedGroups {
		weight := weightMap[groupID]
		if weight <= 0 {
			weight = 1 // 默认权重为1
		}
		groups = append(groups, groupID)
		weights = append(weights, weight)
		total += weight
	}

	gs.weightedSelector = &WeightedSelector{
		groups:  groups,
		weights: weights,
		total:   total,
	}
}

// SelectGroup 选择下一个分组
func (gs *GroupSelector) SelectGroup() (string, error) {
	gs.mutex.Lock()
	defer gs.mutex.Unlock()

	if len(gs.allowedGroups) == 0 {
		return "", fmt.Errorf("no allowed groups available")
	}

	if len(gs.allowedGroups) == 1 {
		return gs.allowedGroups[0], nil
	}

	switch gs.config.Strategy {
	case GroupSelectionRoundRobin:
		return gs.selectRoundRobin(), nil
	case GroupSelectionWeighted:
		return gs.selectWeighted(), nil
	case GroupSelectionRandom:
		return gs.selectRandom(), nil
	case GroupSelectionFailover:
		return gs.selectFailover(), nil
	default:
		return gs.selectRoundRobin(), nil
	}
}

// selectRoundRobin 轮询选择
func (gs *GroupSelector) selectRoundRobin() string {
	selectedGroup := gs.allowedGroups[gs.currentIndex]
	gs.currentIndex = (gs.currentIndex + 1) % len(gs.allowedGroups)
	gs.updateUsageStats(selectedGroup)
	return selectedGroup
}

// selectWeighted 权重选择
func (gs *GroupSelector) selectWeighted() string {
	if gs.weightedSelector == nil {
		return gs.selectRoundRobin()
	}

	// 生成随机数
	randNum := rand.Intn(gs.weightedSelector.total)
	
	// 根据权重选择分组
	currentSum := 0
	for i, weight := range gs.weightedSelector.weights {
		currentSum += weight
		if randNum < currentSum {
			selectedGroup := gs.weightedSelector.groups[i]
			gs.updateUsageStats(selectedGroup)
			return selectedGroup
		}
	}

	// 如果出现意外情况，返回第一个分组
	selectedGroup := gs.weightedSelector.groups[0]
	gs.updateUsageStats(selectedGroup)
	return selectedGroup
}

// selectRandom 随机选择
func (gs *GroupSelector) selectRandom() string {
	index := rand.Intn(len(gs.allowedGroups))
	selectedGroup := gs.allowedGroups[index]
	gs.updateUsageStats(selectedGroup)
	return selectedGroup
}

// selectFailover 故障转移选择（按顺序优先级）
func (gs *GroupSelector) selectFailover() string {
	// 故障转移策略：按allowedGroups的顺序选择第一个可用的分组
	// 这里简化实现，直接返回第一个分组
	// 在实际应用中，可以结合健康检查来判断分组是否可用
	selectedGroup := gs.allowedGroups[0]
	gs.updateUsageStats(selectedGroup)
	return selectedGroup
}

// updateUsageStats 更新使用统计
func (gs *GroupSelector) updateUsageStats(groupID string) {
	gs.groupUsageCount[groupID]++
	gs.lastUsedTime[groupID] = time.Now()
}

// GetUsageStats 获取使用统计
func (gs *GroupSelector) GetUsageStats() map[string]GroupUsageStats {
	gs.mutex.RLock()
	defer gs.mutex.RUnlock()

	stats := make(map[string]GroupUsageStats)
	for _, groupID := range gs.allowedGroups {
		stats[groupID] = GroupUsageStats{
			GroupID:    groupID,
			UsageCount: gs.groupUsageCount[groupID],
			LastUsed:   gs.lastUsedTime[groupID],
		}
	}
	return stats
}

// GroupUsageStats 分组使用统计
type GroupUsageStats struct {
	GroupID    string    `json:"group_id"`
	UsageCount int64     `json:"usage_count"`
	LastUsed   time.Time `json:"last_used"`
}

// UpdateConfig 更新分组选择配置
func (gs *GroupSelector) UpdateConfig(config *GroupSelectionConfig) {
	gs.mutex.Lock()
	defer gs.mutex.Unlock()

	gs.config = config
	if config.Strategy == GroupSelectionWeighted && len(config.GroupWeights) > 0 {
		gs.initWeightedSelector()
	} else {
		gs.weightedSelector = nil
	}
}

// UpdateAllowedGroups 更新允许的分组列表
func (gs *GroupSelector) UpdateAllowedGroups(allowedGroups []string) {
	gs.mutex.Lock()
	defer gs.mutex.Unlock()

	gs.allowedGroups = allowedGroups
	gs.currentIndex = 0

	// 清理不再允许的分组的统计信息
	newGroupUsageCount := make(map[string]int64)
	newLastUsedTime := make(map[string]time.Time)
	
	for _, groupID := range allowedGroups {
		if count, exists := gs.groupUsageCount[groupID]; exists {
			newGroupUsageCount[groupID] = count
		}
		if lastUsed, exists := gs.lastUsedTime[groupID]; exists {
			newLastUsedTime[groupID] = lastUsed
		}
	}

	gs.groupUsageCount = newGroupUsageCount
	gs.lastUsedTime = newLastUsedTime

	// 如果是权重策略，重新初始化权重选择器
	if gs.config.Strategy == GroupSelectionWeighted && len(gs.config.GroupWeights) > 0 {
		gs.initWeightedSelector()
	}
}
