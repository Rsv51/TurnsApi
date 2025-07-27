package keymanager

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"turnsapi/internal"
)

// GroupKeyManager 分组密钥管理器
type GroupKeyManager struct {
	groupID          string
	groupName        string
	keys             []string
	keyInfos         map[string]*KeyInfo
	keyStatuses      map[string]*KeyStatus
	rotationStrategy string
	currentIndex     int
	mutex            sync.RWMutex
}

// NewGroupKeyManager 创建分组密钥管理器
func NewGroupKeyManager(groupID, groupName string, keys []string, rotationStrategy string) *GroupKeyManager {
	gkm := &GroupKeyManager{
		groupID:          groupID,
		groupName:        groupName,
		keys:             keys,
		keyInfos:         make(map[string]*KeyInfo),
		keyStatuses:      make(map[string]*KeyStatus),
		rotationStrategy: rotationStrategy,
		currentIndex:     0,
	}

	// 初始化密钥信息和状态
	for _, key := range keys {
		gkm.keyInfos[key] = &KeyInfo{
			Key:           key,
			Name:          fmt.Sprintf("%s-Key-%s", groupName, key[len(key)-8:]),
			Description:   fmt.Sprintf("密钥来自分组: %s", groupName),
			IsActive:      true,
			AllowedModels: []string{},
		}
		gkm.keyStatuses[key] = &KeyStatus{
			Key:           key,
			Name:          gkm.keyInfos[key].Name,
			Description:   gkm.keyInfos[key].Description,
			IsActive:      true,
			LastUsed:      time.Time{},
			UsageCount:    0,
			ErrorCount:    0,
			AllowedModels: gkm.keyInfos[key].AllowedModels,
		}
	}

	return gkm
}

// GetNextKey 获取下一个可用的API密钥
func (gkm *GroupKeyManager) GetNextKey() (string, error) {
	gkm.mutex.Lock()
	defer gkm.mutex.Unlock()

	activeKeys := gkm.getActiveKeys()
	if len(activeKeys) == 0 {
		return "", fmt.Errorf("no active API keys available in group %s", gkm.groupID)
	}

	var selectedKey string

	switch gkm.rotationStrategy {
	case "round_robin":
		selectedKey = gkm.roundRobinSelection(activeKeys)
	case "random":
		selectedKey = gkm.randomSelection(activeKeys)
	case "least_used":
		selectedKey = gkm.leastUsedSelection(activeKeys)
	default:
		selectedKey = gkm.roundRobinSelection(activeKeys)
	}

	// 更新使用统计
	if status, exists := gkm.keyStatuses[selectedKey]; exists {
		status.LastUsed = time.Now()
		status.UsageCount++
	}

	return selectedKey, nil
}

// getActiveKeys 获取所有活跃的密钥
func (gkm *GroupKeyManager) getActiveKeys() []string {
	var activeKeys []string
	for _, key := range gkm.keys {
		if status, exists := gkm.keyStatuses[key]; exists && status.IsActive {
			activeKeys = append(activeKeys, key)
		}
	}
	return activeKeys
}

// roundRobinSelection 轮询选择
func (gkm *GroupKeyManager) roundRobinSelection(activeKeys []string) string {
	if len(activeKeys) == 0 {
		return ""
	}

	// 找到当前索引对应的密钥在活跃密钥中的位置
	currentKey := ""
	if gkm.currentIndex < len(gkm.keys) {
		currentKey = gkm.keys[gkm.currentIndex]
	}

	// 查找当前密钥在活跃密钥中的位置
	currentPos := -1
	for i, key := range activeKeys {
		if key == currentKey {
			currentPos = i
			break
		}
	}

	// 选择下一个密钥
	nextPos := (currentPos + 1) % len(activeKeys)
	selectedKey := activeKeys[nextPos]

	// 更新全局索引
	for i, key := range gkm.keys {
		if key == selectedKey {
			gkm.currentIndex = (i + 1) % len(gkm.keys)
			break
		}
	}

	return selectedKey
}

// randomSelection 随机选择
func (gkm *GroupKeyManager) randomSelection(activeKeys []string) string {
	if len(activeKeys) == 0 {
		return ""
	}
	return activeKeys[randomInt(len(activeKeys))]
}

// leastUsedSelection 最少使用选择
func (gkm *GroupKeyManager) leastUsedSelection(activeKeys []string) string {
	if len(activeKeys) == 0 {
		return ""
	}

	var leastUsedKey string
	var minUsage int64 = -1

	for _, key := range activeKeys {
		if status, exists := gkm.keyStatuses[key]; exists {
			if minUsage == -1 || status.UsageCount < minUsage {
				minUsage = status.UsageCount
				leastUsedKey = key
			}
		}
	}

	return leastUsedKey
}

// ReportSuccess 报告密钥使用成功
func (gkm *GroupKeyManager) ReportSuccess(apiKey string) {
	gkm.mutex.Lock()
	defer gkm.mutex.Unlock()

	if status, exists := gkm.keyStatuses[apiKey]; exists {
		status.LastUsed = time.Now()
		// 成功使用不增加错误计数，但可以重置连续错误状态
		if status.ErrorCount > 0 {
			log.Printf("密钥 %s (分组: %s) 恢复正常", gkm.maskKey(apiKey), gkm.groupID)
		}
	}
}

// ReportError 报告密钥使用错误
func (gkm *GroupKeyManager) ReportError(apiKey string, errorMsg string) {
	gkm.mutex.Lock()
	defer gkm.mutex.Unlock()

	if status, exists := gkm.keyStatuses[apiKey]; exists {
		status.ErrorCount++
		status.LastError = errorMsg
		status.LastErrorTime = time.Now()

		log.Printf("密钥 %s (分组: %s) 发生错误: %s (错误次数: %d)", 
			gkm.maskKey(apiKey), gkm.groupID, errorMsg, status.ErrorCount)

		// 如果错误次数过多，可以考虑暂时禁用密钥
		if status.ErrorCount >= 5 {
			status.IsActive = false
			log.Printf("密钥 %s (分组: %s) 因错误过多被暂时禁用", gkm.maskKey(apiKey), gkm.groupID)
		}
	}
}

// GetKeyStatuses 获取所有密钥状态
func (gkm *GroupKeyManager) GetKeyStatuses() map[string]*KeyStatus {
	gkm.mutex.RLock()
	defer gkm.mutex.RUnlock()

	statuses := make(map[string]*KeyStatus)
	for key, status := range gkm.keyStatuses {
		// 创建副本以避免并发修改
		statusCopy := *status
		statuses[key] = &statusCopy
	}
	return statuses
}

// GetGroupInfo 获取分组信息
func (gkm *GroupKeyManager) GetGroupInfo() map[string]interface{} {
	gkm.mutex.RLock()
	defer gkm.mutex.RUnlock()

	activeCount := 0
	totalCount := len(gkm.keys)
	
	for _, status := range gkm.keyStatuses {
		if status.IsActive {
			activeCount++
		}
	}

	return map[string]interface{}{
		"group_id":          gkm.groupID,
		"group_name":        gkm.groupName,
		"total_keys":        totalCount,
		"active_keys":       activeCount,
		"rotation_strategy": gkm.rotationStrategy,
	}
}

// maskKey 掩码密钥显示
func (gkm *GroupKeyManager) maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// randomInt 生成随机整数
func randomInt(max int) int {
	return int(time.Now().UnixNano()) % max
}

// MultiGroupKeyManager 多分组密钥管理器
type MultiGroupKeyManager struct {
	config           *internal.Config
	groupManagers    map[string]*GroupKeyManager
	mutex            sync.RWMutex
	ctx              context.Context
	cancel           context.CancelFunc
}

// NewMultiGroupKeyManager 创建多分组密钥管理器
func NewMultiGroupKeyManager(config *internal.Config) *MultiGroupKeyManager {
	ctx, cancel := context.WithCancel(context.Background())

	mgkm := &MultiGroupKeyManager{
		config:        config,
		groupManagers: make(map[string]*GroupKeyManager),
		ctx:           ctx,
		cancel:        cancel,
	}

	// 初始化所有分组的密钥管理器
	for groupID, group := range config.UserGroups {
		if group.Enabled && len(group.APIKeys) > 0 {
			groupManager := NewGroupKeyManager(groupID, group.Name, group.APIKeys, group.RotationStrategy)
			mgkm.groupManagers[groupID] = groupManager
		}
	}

	// 移除了定时健康检查

	return mgkm
}

// GetNextKeyForGroup 获取指定分组的下一个可用密钥
func (mgkm *MultiGroupKeyManager) GetNextKeyForGroup(groupID string) (string, error) {
	mgkm.mutex.RLock()
	groupManager, exists := mgkm.groupManagers[groupID]
	mgkm.mutex.RUnlock()

	if !exists {
		return "", fmt.Errorf("group %s not found or not enabled", groupID)
	}

	return groupManager.GetNextKey()
}

// GetNextKeyForModel 根据模型名称获取合适分组的下一个可用密钥
func (mgkm *MultiGroupKeyManager) GetNextKeyForModel(modelName string) (string, string, error) {
	// 查找支持该模型的分组
	group, groupID := mgkm.config.GetGroupByModel(modelName)
	if group == nil {
		return "", "", fmt.Errorf("no enabled group found for model %s", modelName)
	}

	key, err := mgkm.GetNextKeyForGroup(groupID)
	if err != nil {
		return "", "", fmt.Errorf("failed to get key for group %s: %w", groupID, err)
	}

	return key, groupID, nil
}

// ReportSuccess 报告密钥使用成功
func (mgkm *MultiGroupKeyManager) ReportSuccess(groupID, apiKey string) {
	mgkm.mutex.RLock()
	groupManager, exists := mgkm.groupManagers[groupID]
	mgkm.mutex.RUnlock()

	if exists {
		groupManager.ReportSuccess(apiKey)
	}
}

// ReportError 报告密钥使用错误
func (mgkm *MultiGroupKeyManager) ReportError(groupID, apiKey string, errorMsg string) {
	mgkm.mutex.RLock()
	groupManager, exists := mgkm.groupManagers[groupID]
	mgkm.mutex.RUnlock()

	if exists {
		groupManager.ReportError(apiKey, errorMsg)
	}
}

// GetAllGroupStatuses 获取所有分组的状态
func (mgkm *MultiGroupKeyManager) GetAllGroupStatuses() map[string]interface{} {
	mgkm.mutex.RLock()
	defer mgkm.mutex.RUnlock()

	statuses := make(map[string]interface{})

	for groupID, groupManager := range mgkm.groupManagers {
		groupInfo := groupManager.GetGroupInfo()
		groupInfo["key_statuses"] = groupManager.GetKeyStatuses()
		statuses[groupID] = groupInfo
	}

	return statuses
}

// GetGroupStatus 获取指定分组的状态
func (mgkm *MultiGroupKeyManager) GetGroupStatus(groupID string) (interface{}, bool) {
	mgkm.mutex.RLock()
	groupManager, exists := mgkm.groupManagers[groupID]
	mgkm.mutex.RUnlock()

	if !exists {
		return nil, false
	}

	groupInfo := groupManager.GetGroupInfo()
	groupInfo["key_statuses"] = groupManager.GetKeyStatuses()
	return groupInfo, true
}

// UpdateGroupConfig 更新分组配置
func (mgkm *MultiGroupKeyManager) UpdateGroupConfig(groupID string, group *internal.UserGroup) error {
	mgkm.mutex.Lock()
	defer mgkm.mutex.Unlock()

	if group == nil {
		// 删除分组管理器
		delete(mgkm.groupManagers, groupID)
		log.Printf("删除分组 %s 的密钥管理器", groupID)
	} else if group.Enabled && len(group.APIKeys) > 0 {
		// 创建或更新分组管理器
		groupManager := NewGroupKeyManager(groupID, group.Name, group.APIKeys, group.RotationStrategy)
		mgkm.groupManagers[groupID] = groupManager
		log.Printf("更新分组 %s 的密钥管理器", groupID)
	} else {
		// 删除分组管理器（禁用或无密钥）
		delete(mgkm.groupManagers, groupID)
		log.Printf("删除分组 %s 的密钥管理器（禁用或无密钥）", groupID)
	}

	return nil
}

// 移除了定时健康检查方法

// Close 关闭管理器
func (mgkm *MultiGroupKeyManager) Close() {
	if mgkm.cancel != nil {
		mgkm.cancel()
	}
}
