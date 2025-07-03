package proxykey

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// ProxyKey 代理服务API密钥
type ProxyKey struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	LastUsed    time.Time `json:"last_used"`
	UsageCount  int64     `json:"usage_count"`
	IsActive    bool      `json:"is_active"`
}

// Manager 代理密钥管理器
type Manager struct {
	keys map[string]*ProxyKey
	mu   sync.RWMutex
}

// NewManager 创建新的代理密钥管理器
func NewManager() *Manager {
	return &Manager{
		keys: make(map[string]*ProxyKey),
	}
}

// GenerateKey 生成新的代理API密钥
func (m *Manager) GenerateKey(name, description string) (*ProxyKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 生成随机密钥
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}

	keyStr := "tapi-" + hex.EncodeToString(keyBytes)
	id := generateID()

	key := &ProxyKey{
		ID:          id,
		Key:         keyStr,
		Name:        name,
		Description: description,
		CreatedAt:   time.Now(),
		IsActive:    true,
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
			return key, true
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

	delete(m.keys, id)
	return nil
}

// UpdateKey 更新代理密钥信息
func (m *Manager) UpdateKey(id string, name, description string, isActive bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key, exists := m.keys[id]
	if !exists {
		return fmt.Errorf("key not found")
	}

	key.Name = name
	key.Description = description
	key.IsActive = isActive
	return nil
}

// generateID 生成唯一ID
func generateID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
