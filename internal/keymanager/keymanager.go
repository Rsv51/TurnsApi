package keymanager

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// KeyStatus API密钥状态
type KeyStatus struct {
	Key           string    `json:"key"`
	Name          string    `json:"name,omitempty"`
	Description   string    `json:"description,omitempty"`
	IsActive      bool      `json:"is_active"`
	LastUsed      time.Time `json:"last_used"`
	UsageCount    int64     `json:"usage_count"`
	ErrorCount    int64     `json:"error_count"`
	LastError     string    `json:"last_error,omitempty"`
	LastErrorTime time.Time `json:"last_error_time,omitempty"`
	AllowedModels []string  `json:"allowed_models,omitempty"`
}

// KeyInfo API密钥信息
type KeyInfo struct {
	Key           string   `json:"key"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	IsActive      bool     `json:"is_active"`
	AllowedModels []string `json:"allowed_models"`
}

// KeyManager API密钥管理器
type KeyManager struct {
	keys              []string
	keyInfos          map[string]*KeyInfo
	keyStatuses       map[string]*KeyStatus
	rotationStrategy  string
	currentIndex      int
	mutex             sync.RWMutex
	healthCheckTicker *time.Ticker
	ctx               context.Context
	cancel            context.CancelFunc
	configPath        string // 配置文件路径
}

// NewKeyManager 创建新的密钥管理器
func NewKeyManager(keys []string, rotationStrategy string, healthCheckInterval time.Duration, configPath string) *KeyManager {
	ctx, cancel := context.WithCancel(context.Background())

	km := &KeyManager{
		keys:             keys,
		keyInfos:         make(map[string]*KeyInfo),
		keyStatuses:      make(map[string]*KeyStatus),
		rotationStrategy: rotationStrategy,
		currentIndex:     0,
		ctx:              ctx,
		cancel:           cancel,
		configPath:       configPath,
	}

	// 初始化密钥信息和状态
	for _, key := range keys {
		km.keyInfos[key] = &KeyInfo{
			Key:           key,
			Name:          fmt.Sprintf("Key-%s", key[len(key)-8:]), // 使用密钥后8位作为默认名称
			Description:   "",
			IsActive:      true,
			AllowedModels: []string{}, // 空数组表示允许所有模型
		}
		km.keyStatuses[key] = &KeyStatus{
			Key:           key,
			Name:          km.keyInfos[key].Name,
			Description:   km.keyInfos[key].Description,
			IsActive:      true,
			LastUsed:      time.Time{},
			UsageCount:    0,
			ErrorCount:    0,
			AllowedModels: km.keyInfos[key].AllowedModels,
		}
	}

	// 启动健康检查
	if healthCheckInterval > 0 {
		km.healthCheckTicker = time.NewTicker(healthCheckInterval)
		go km.startHealthCheck()
	}

	return km
}

// GetNextKey 获取下一个可用的API密钥
func (km *KeyManager) GetNextKey() (string, error) {
	km.mutex.Lock()
	defer km.mutex.Unlock()

	activeKeys := km.getActiveKeys()
	if len(activeKeys) == 0 {
		return "", fmt.Errorf("no active API keys available")
	}

	var selectedKey string

	switch km.rotationStrategy {
	case "round_robin":
		selectedKey = km.roundRobinSelection(activeKeys)
	case "random":
		selectedKey = km.randomSelection(activeKeys)
	case "least_used":
		selectedKey = km.leastUsedSelection(activeKeys)
	default:
		selectedKey = km.roundRobinSelection(activeKeys)
	}

	// 更新使用统计
	if status, exists := km.keyStatuses[selectedKey]; exists {
		status.LastUsed = time.Now()
		status.UsageCount++
	}

	return selectedKey, nil
}

// getActiveKeys 获取所有活跃的密钥
func (km *KeyManager) getActiveKeys() []string {
	var activeKeys []string
	for _, key := range km.keys {
		if status, exists := km.keyStatuses[key]; exists && status.IsActive {
			activeKeys = append(activeKeys, key)
		}
	}
	return activeKeys
}

// roundRobinSelection 轮询选择
func (km *KeyManager) roundRobinSelection(activeKeys []string) string {
	if len(activeKeys) == 0 {
		return ""
	}

	// 找到当前索引对应的密钥在活跃密钥中的位置
	currentKey := ""
	if km.currentIndex < len(km.keys) {
		currentKey = km.keys[km.currentIndex]
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
	for i, key := range km.keys {
		if key == selectedKey {
			km.currentIndex = (i + 1) % len(km.keys)
			break
		}
	}

	return selectedKey
}

// randomSelection 随机选择
func (km *KeyManager) randomSelection(activeKeys []string) string {
	if len(activeKeys) == 0 {
		return ""
	}
	return activeKeys[rand.Intn(len(activeKeys))]
}

// leastUsedSelection 最少使用选择
func (km *KeyManager) leastUsedSelection(activeKeys []string) string {
	if len(activeKeys) == 0 {
		return ""
	}

	var leastUsedKey string
	var minUsage int64 = -1

	for _, key := range activeKeys {
		if status, exists := km.keyStatuses[key]; exists {
			if minUsage == -1 || status.UsageCount < minUsage {
				minUsage = status.UsageCount
				leastUsedKey = key
			}
		}
	}

	return leastUsedKey
}

// ReportError 报告密钥错误
func (km *KeyManager) ReportError(key string, errorMsg string) {
	km.mutex.Lock()
	defer km.mutex.Unlock()

	if status, exists := km.keyStatuses[key]; exists {
		status.ErrorCount++
		status.LastError = errorMsg
		status.LastErrorTime = time.Now()

		// 如果错误次数过多，暂时禁用该密钥
		if status.ErrorCount >= 5 {
			status.IsActive = false
			log.Printf("API key disabled due to too many errors: %s", km.maskKey(key))
		}
	}
}

// ReportSuccess 报告密钥成功使用
func (km *KeyManager) ReportSuccess(key string) {
	km.mutex.Lock()
	defer km.mutex.Unlock()

	if status, exists := km.keyStatuses[key]; exists {
		// 成功使用后，如果密钥被禁用，可以重新启用
		if !status.IsActive && status.ErrorCount > 0 {
			status.IsActive = true
			status.ErrorCount = 0
			status.LastError = ""
			log.Printf("API key re-enabled after successful use: %s", km.maskKey(key))
		}
	}
}

// GetKeyStatuses 获取所有密钥状态
func (km *KeyManager) GetKeyStatuses() map[string]*KeyStatus {
	km.mutex.RLock()
	defer km.mutex.RUnlock()

	// 创建副本以避免并发访问问题
	statuses := make(map[string]*KeyStatus)
	for key, status := range km.keyStatuses {
		statusCopy := *status
		statusCopy.Key = km.maskKey(key) // 隐藏密钥的敏感部分
		statuses[km.maskKey(key)] = &statusCopy
	}

	return statuses
}

// maskKey 隐藏密钥的敏感部分
func (km *KeyManager) maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// startHealthCheck 启动健康检查
func (km *KeyManager) startHealthCheck() {
	for {
		select {
		case <-km.ctx.Done():
			return
		case <-km.healthCheckTicker.C:
			km.performHealthCheck()
		}
	}
}

// performHealthCheck 执行健康检查
func (km *KeyManager) performHealthCheck() {
	km.mutex.Lock()
	defer km.mutex.Unlock()

	// 这里可以实现实际的健康检查逻辑
	// 例如向OpenRouter API发送测试请求
	log.Println("Performing health check for API keys...")

	// 重置长时间未使用的错误计数
	now := time.Now()
	for _, status := range km.keyStatuses {
		if !status.IsActive && now.Sub(status.LastErrorTime) > 10*time.Minute {
			status.IsActive = true
			status.ErrorCount = 0
			status.LastError = ""
			log.Printf("API key re-enabled after cooldown: %s", km.maskKey(status.Key))
		}
	}
}

// AddKey 添加新的API密钥
func (km *KeyManager) AddKey(key, name, description string, allowedModels []string) error {
	km.mutex.Lock()
	defer km.mutex.Unlock()

	// 检查密钥是否已存在
	for _, existingKey := range km.keys {
		if existingKey == key {
			return fmt.Errorf("API密钥已存在")
		}
	}

	// 添加密钥到列表
	km.keys = append(km.keys, key)

	// 如果没有提供名称，使用默认名称
	if name == "" {
		name = fmt.Sprintf("Key-%s", key[len(key)-8:])
	}

	// 初始化密钥信息
	km.keyInfos[key] = &KeyInfo{
		Key:           key,
		Name:          name,
		Description:   description,
		IsActive:      true,
		AllowedModels: allowedModels,
	}

	// 初始化密钥状态
	km.keyStatuses[key] = &KeyStatus{
		Key:           key,
		Name:          name,
		Description:   description,
		IsActive:      true,
		LastUsed:      time.Time{},
		UsageCount:    0,
		ErrorCount:    0,
		AllowedModels: allowedModels,
	}

	log.Printf("添加新的API密钥: %s (名称: %s)", km.maskKey(key), name)

	// 更新配置文件
	if err := km.updateConfigFile(); err != nil {
		log.Printf("更新配置文件失败: %v", err)
	}

	return nil
}

// AddKeysInBatch 批量添加API密钥
func (km *KeyManager) AddKeysInBatch(keys []string) (int, []string, error) {
	km.mutex.Lock()
	defer km.mutex.Unlock()

	var addedKeys []string
	var errors []string
	addedCount := 0

	for i, key := range keys {
		// 跳过空密钥
		if key == "" {
			continue
		}

		// 检查密钥是否已存在
		exists := false
		for _, existingKey := range km.keys {
			if existingKey == key {
				errors = append(errors, fmt.Sprintf("密钥 %d: 已存在", i+1))
				exists = true
				break
			}
		}

		if exists {
			continue
		}

		// 添加密钥到列表
		km.keys = append(km.keys, key)

		// 生成默认名称
		name := fmt.Sprintf("Key-%s", key[len(key)-8:])

		// 初始化密钥信息
		km.keyInfos[key] = &KeyInfo{
			Key:           key,
			Name:          name,
			Description:   fmt.Sprintf("批量添加的密钥 #%d", i+1),
			IsActive:      true,
			AllowedModels: []string{}, // 空表示允许所有模型
		}

		// 初始化密钥状态
		km.keyStatuses[key] = &KeyStatus{
			Key:           key,
			Name:          name,
			Description:   fmt.Sprintf("批量添加的密钥 #%d", i+1),
			IsActive:      true,
			LastUsed:      time.Time{},
			UsageCount:    0,
			ErrorCount:    0,
			AllowedModels: []string{},
		}

		addedKeys = append(addedKeys, key)
		addedCount++
		log.Printf("批量添加API密钥: %s (名称: %s)", km.maskKey(key), name)
	}

	// 如果有密钥被添加，更新配置文件
	if addedCount > 0 {
		if err := km.updateConfigFile(); err != nil {
			log.Printf("批量添加后更新配置文件失败: %v", err)
		}
	}

	return addedCount, errors, nil
}

// UpdateKey 更新API密钥信息
func (km *KeyManager) UpdateKey(keyID, name, description string, isActive *bool, allowedModels []string) error {
	km.mutex.Lock()
	defer km.mutex.Unlock()

	// 在这个简单实现中，keyID就是密钥本身
	// 在更复杂的实现中，可能需要维护ID到密钥的映射
	if status, exists := km.keyStatuses[keyID]; exists {
		if info, infoExists := km.keyInfos[keyID]; infoExists {
			// 更新密钥信息
			if name != "" {
				info.Name = name
				status.Name = name
			}
			if description != "" {
				info.Description = description
				status.Description = description
			}
			if allowedModels != nil {
				info.AllowedModels = allowedModels
				status.AllowedModels = allowedModels
			}
			if isActive != nil {
				info.IsActive = *isActive
				status.IsActive = *isActive
				log.Printf("更新API密钥状态: %s, 活跃: %v", km.maskKey(keyID), *isActive)
			}
			log.Printf("更新API密钥信息: %s (名称: %s)", km.maskKey(keyID), info.Name)
		}
		return nil
	}

	return fmt.Errorf("API密钥不存在")
}

// DeleteKey 删除API密钥
func (km *KeyManager) DeleteKey(keyID string) error {
	km.mutex.Lock()
	defer km.mutex.Unlock()

	// 查找并删除密钥
	for i, key := range km.keys {
		if key == keyID {
			// 从切片中删除
			km.keys = append(km.keys[:i], km.keys[i+1:]...)
			// 删除状态和信息
			delete(km.keyStatuses, keyID)
			delete(km.keyInfos, keyID)
			log.Printf("删除API密钥: %s", km.maskKey(keyID))

			// 更新配置文件
			if err := km.updateConfigFile(); err != nil {
				log.Printf("更新配置文件失败: %v", err)
			}

			return nil
		}
	}

	return fmt.Errorf("API密钥不存在")
}

// IsModelAllowed 检查指定密钥是否允许使用指定模型
func (km *KeyManager) IsModelAllowed(key, model string) bool {
	km.mutex.RLock()
	defer km.mutex.RUnlock()

	if info, exists := km.keyInfos[key]; exists {
		// 如果AllowedModels为空，表示允许所有模型
		if len(info.AllowedModels) == 0 {
			return true
		}
		// 检查模型是否在允许列表中
		for _, allowedModel := range info.AllowedModels {
			if allowedModel == model {
				return true
			}
		}
		return false
	}
	// 如果密钥不存在，默认不允许
	return false
}

// GetAllAllowedModels 获取所有密钥允许的模型列表（去重）
func (km *KeyManager) GetAllAllowedModels() []string {
	km.mutex.RLock()
	defer km.mutex.RUnlock()

	modelSet := make(map[string]bool)
	hasUnlimitedKey := false

	for _, info := range km.keyInfos {
		if !info.IsActive {
			continue
		}
		// 如果有密钥允许所有模型，则返回空列表表示无限制
		if len(info.AllowedModels) == 0 {
			hasUnlimitedKey = true
			break
		}
		for _, model := range info.AllowedModels {
			modelSet[model] = true
		}
	}

	// 如果有无限制的密钥，返回空列表
	if hasUnlimitedKey {
		return []string{}
	}

	// 转换为切片
	models := make([]string, 0, len(modelSet))
	for model := range modelSet {
		models = append(models, model)
	}
	return models
}

// Close 关闭密钥管理器
func (km *KeyManager) Close() {
	if km.cancel != nil {
		km.cancel()
	}
	if km.healthCheckTicker != nil {
		km.healthCheckTicker.Stop()
	}
}

// ConfigFile 配置文件结构
type ConfigFile struct {
	APIKeys struct {
		Keys []string `yaml:"keys"`
	} `yaml:"api_keys"`
}

// updateConfigFile 更新配置文件
func (km *KeyManager) updateConfigFile() error {
	if km.configPath == "" {
		return fmt.Errorf("配置文件路径未设置")
	}

	// 读取现有配置文件
	data, err := os.ReadFile(km.configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 解析YAML到通用结构
	var config yaml.Node
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 将当前密钥列表转换为YAML节点
	var keysNode yaml.Node
	keysNode.Kind = yaml.SequenceNode
	for _, key := range km.keys {
		var keyNode yaml.Node
		keyNode.Kind = yaml.ScalarNode
		keyNode.Value = key
		keysNode.Content = append(keysNode.Content, &keyNode)
	}

	// 查找并更新api_keys.keys节点
	if config.Kind == yaml.DocumentNode && len(config.Content) > 0 {
		root := config.Content[0]
		if root.Kind == yaml.MappingNode {
			for i := 0; i < len(root.Content); i += 2 {
				if root.Content[i].Value == "api_keys" {
					apiKeysNode := root.Content[i+1]
					if apiKeysNode.Kind == yaml.MappingNode {
						for j := 0; j < len(apiKeysNode.Content); j += 2 {
							if apiKeysNode.Content[j].Value == "keys" {
								apiKeysNode.Content[j+1] = &keysNode
								break
							}
						}
					}
					break
				}
			}
		}
	}

	// 写回配置文件
	newData, err := yaml.Marshal(&config)
	if err != nil {
		return fmt.Errorf("序列化配置文件失败: %v", err)
	}

	if err := os.WriteFile(km.configPath, newData, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	log.Printf("配置文件已更新: %s，当前密钥数量: %d", km.configPath, len(km.keys))
	return nil
}
