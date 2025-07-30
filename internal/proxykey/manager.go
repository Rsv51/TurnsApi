package proxykey

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"turnsapi/internal/logger"
)

// ProxyKey 代理服务API密钥
type ProxyKey struct {
	ID            string    `json:"id"`
	Key           string    `json:"key"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	AllowedGroups []string  `json:"allowed_groups"` // 允许访问的分组ID列表
	CreatedAt     time.Time `json:"created_at"`
	LastUsed      time.Time `json:"last_used"`
	UsageCount    int64     `json:"usage_count"`
	IsActive      bool      `json:"is_active"`
}

// Manager 代理密钥管理器
type Manager struct {
	keys          map[string]*ProxyKey
	requestLogger *logger.RequestLogger
	mu            sync.RWMutex
}

// NewManager 创建新的代理密钥管理器
func NewManager() *Manager {
	return &Manager{
		keys: make(map[string]*ProxyKey),
	}
}

// NewManagerWithDB 创建带数据库支持的代理密钥管理器
func NewManagerWithDB(requestLogger *logger.RequestLogger) *Manager {
	m := &Manager{
		keys:          make(map[string]*ProxyKey),
		requestLogger: requestLogger,
	}

	// 从数据库加载现有密钥
	if err := m.loadKeysFromDB(); err != nil {
		log.Printf("Failed to load proxy keys from database: %v", err)
	}

	return m
}

// loadKeysFromDB 从数据库加载代理密钥
func (m *Manager) loadKeysFromDB() error {
	if m.requestLogger == nil {
		return nil
	}

	dbKeys, err := m.requestLogger.GetAllProxyKeys()
	if err != nil {
		return fmt.Errorf("failed to get proxy keys from database: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, dbKey := range dbKeys {
		// 转换数据库模型到内存模型
		key := &ProxyKey{
			ID:            dbKey.ID,
			Key:           dbKey.Key,
			Name:          dbKey.Name,
			Description:   dbKey.Description,
			AllowedGroups: dbKey.AllowedGroups,
			CreatedAt:     dbKey.CreatedAt,
			IsActive:      dbKey.IsActive,
		}

		if dbKey.LastUsedAt != nil {
			key.LastUsed = *dbKey.LastUsedAt
		}

		m.keys[key.ID] = key
	}

	log.Printf("Loaded %d proxy keys from database", len(dbKeys))
	return nil
}

// GenerateKey 生成新的代理API密钥
func (m *Manager) GenerateKey(name, description string, allowedGroups []string) (*ProxyKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 生成随机密钥
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}

	keyStr := "tapi-" + hex.EncodeToString(keyBytes)
	id := generateID()
	now := time.Now()

	key := &ProxyKey{
		ID:            id,
		Key:           keyStr,
		Name:          name,
		Description:   description,
		AllowedGroups: allowedGroups,
		CreatedAt:     now,
		IsActive:      true,
	}

	// 保存到数据库
	if m.requestLogger != nil {
		dbKey := &logger.ProxyKey{
			ID:            id,
			Name:          name,
			Description:   description,
			Key:           keyStr,
			AllowedGroups: allowedGroups,
			IsActive:      true,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		if err := m.requestLogger.InsertProxyKey(dbKey); err != nil {
			return nil, fmt.Errorf("failed to save proxy key to database: %w", err)
		}
	}

	m.keys[id] = key
	return key, nil
}

// ValidateKey 验证代理API密钥
func (m *Manager) ValidateKey(keyStr string) (interface{}, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, key := range m.keys {
		if key.Key == keyStr && key.IsActive {
			// 返回logger.ProxyKey类型以便认证中间件使用
			dbKey := &logger.ProxyKey{
				ID:            key.ID,
				Name:          key.Name,
				Description:   key.Description,
				Key:           key.Key,
				AllowedGroups: key.AllowedGroups,
				IsActive:      key.IsActive,
				CreatedAt:     key.CreatedAt,
				UpdatedAt:     key.CreatedAt,
			}
			if !key.LastUsed.IsZero() {
				dbKey.LastUsedAt = &key.LastUsed
			}
			return dbKey, true
		}
	}
	return nil, false
}

// ValidateKeyForGroup 验证代理API密钥是否可以访问指定分组
func (m *Manager) ValidateKeyForGroup(keyStr, groupID string) (interface{}, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, key := range m.keys {
		if key.Key == keyStr && key.IsActive {
			// 检查分组访问权限
			if len(key.AllowedGroups) > 0 {
				hasAccess := false
				for _, allowedGroup := range key.AllowedGroups {
					if allowedGroup == groupID {
						hasAccess = true
						break
					}
				}
				if !hasAccess {
					return nil, false // 没有访问权限
				}
			}
			// 如果AllowedGroups为空，表示可以访问所有分组

			// 返回logger.ProxyKey类型以便认证中间件使用
			dbKey := &logger.ProxyKey{
				ID:            key.ID,
				Name:          key.Name,
				Description:   key.Description,
				Key:           key.Key,
				AllowedGroups: key.AllowedGroups,
				IsActive:      key.IsActive,
				CreatedAt:     key.CreatedAt,
				UpdatedAt:     key.CreatedAt,
			}
			if !key.LastUsed.IsZero() {
				dbKey.LastUsedAt = &key.LastUsed
			}
			return dbKey, true
		}
	}
	return nil, false
}

// UpdateUsage 更新密钥使用统计
func (m *Manager) UpdateUsage(keyStr string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, key := range m.keys {
		if key.Key == keyStr {
			key.LastUsed = time.Now()
			key.UsageCount++

			// 更新数据库中的最后使用时间
			if m.requestLogger != nil {
				if err := m.requestLogger.UpdateProxyKeyLastUsed(key.ID); err != nil {
					log.Printf("Failed to update proxy key last used time in database: %v", err)
				}
			}
			break
		}
	}
}

// GetAllKeys 获取所有代理密钥
func (m *Manager) GetAllKeys() []*ProxyKey {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]*ProxyKey, 0, len(m.keys))
	for _, key := range m.keys {
		keys = append(keys, key)
	}
	return keys
}

// DeleteKey 删除代理密钥
func (m *Manager) DeleteKey(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.keys[id]; !exists {
		return fmt.Errorf("key not found")
	}

	// 从数据库删除
	if m.requestLogger != nil {
		if err := m.requestLogger.DeleteProxyKey(id); err != nil {
			return fmt.Errorf("failed to delete proxy key from database: %w", err)
		}
	}

	delete(m.keys, id)
	return nil
}

// UpdateKey 更新代理密钥信息
func (m *Manager) UpdateKey(id string, name, description string, isActive bool, allowedGroups []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key, exists := m.keys[id]
	if !exists {
		return fmt.Errorf("key not found")
	}

	// 更新内存中的密钥信息
	key.Name = name
	key.Description = description
	key.IsActive = isActive
	key.AllowedGroups = allowedGroups

	// 更新数据库中的密钥信息
	if m.requestLogger != nil {
		dbKey := &logger.ProxyKey{
			ID:            key.ID,
			Name:          name,
			Description:   description,
			Key:           key.Key,
			AllowedGroups: allowedGroups,
			IsActive:      isActive,
			CreatedAt:     key.CreatedAt,
			UpdatedAt:     time.Now(),
		}

		if err := m.requestLogger.UpdateProxyKey(dbKey); err != nil {
			return fmt.Errorf("failed to update proxy key in database: %w", err)
		}
	}

	return nil
}

// generateID 生成唯一ID
func generateID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
