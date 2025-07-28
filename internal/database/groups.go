package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// UserGroup 用户分组配置（避免循环导入）
type UserGroup struct {
	Name             string            `yaml:"name" json:"name"`
	ProviderType     string            `yaml:"provider_type" json:"provider_type"`
	BaseURL          string            `yaml:"base_url" json:"base_url"`
	Enabled          bool              `yaml:"enabled" json:"enabled"`
	Timeout          time.Duration     `yaml:"timeout" json:"timeout"`
	MaxRetries       int               `yaml:"max_retries" json:"max_retries"`
	RotationStrategy string            `yaml:"rotation_strategy" json:"rotation_strategy"`
	APIKeys          []string          `yaml:"api_keys" json:"api_keys"`
	Models           []string          `yaml:"models,omitempty" json:"models,omitempty"`
	Headers          map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	RequestParams    map[string]interface{} `yaml:"request_params,omitempty" json:"request_params,omitempty"` // JSON请求参数覆盖
}

// GroupsDB 分组数据库管理器
type GroupsDB struct {
	db *sql.DB
}

// NewGroupsDB 创建新的分组数据库管理器
func NewGroupsDB(dbPath string) (*GroupsDB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	groupsDB := &GroupsDB{db: db}
	
	// 初始化表结构
	if err := groupsDB.initTables(); err != nil {
		return nil, fmt.Errorf("failed to initialize tables: %w", err)
	}

	return groupsDB, nil
}

// initTables 初始化数据库表
func (gdb *GroupsDB) initTables() error {
	// 创建分组表
	createGroupsTable := `
	CREATE TABLE IF NOT EXISTS provider_groups (
		group_id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		provider_type TEXT NOT NULL,
		base_url TEXT NOT NULL,
		enabled BOOLEAN NOT NULL DEFAULT 1,
		timeout_seconds INTEGER NOT NULL DEFAULT 30,
		max_retries INTEGER NOT NULL DEFAULT 3,
		rotation_strategy TEXT NOT NULL DEFAULT 'round_robin',
		models TEXT, -- JSON array of supported models
		headers TEXT, -- JSON object of custom headers
		request_params TEXT, -- JSON object of request parameters override
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	// 创建API密钥表
	createKeysTable := `
	CREATE TABLE IF NOT EXISTS provider_api_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		group_id TEXT NOT NULL,
		api_key TEXT NOT NULL,
		key_order INTEGER NOT NULL DEFAULT 0,
		is_valid BOOLEAN DEFAULT NULL,
		last_validated_at DATETIME DEFAULT NULL,
		validation_error TEXT DEFAULT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (group_id) REFERENCES provider_groups(group_id) ON DELETE CASCADE
	);`

	// 创建索引
	createIndexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_provider_groups_enabled ON provider_groups(enabled);`,
		`CREATE INDEX IF NOT EXISTS idx_provider_groups_type ON provider_groups(provider_type);`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_group_id ON provider_api_keys(group_id);`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_order ON provider_api_keys(group_id, key_order);`,
	}

	// 执行表创建
	if _, err := gdb.db.Exec(createGroupsTable); err != nil {
		return fmt.Errorf("failed to create provider_groups table: %w", err)
	}

	if _, err := gdb.db.Exec(createKeysTable); err != nil {
		return fmt.Errorf("failed to create provider_api_keys table: %w", err)
	}

	// 执行数据库迁移，为现有表添加新字段
	if err := gdb.migrateAPIKeysTable(); err != nil {
		return fmt.Errorf("failed to migrate provider_api_keys table: %w", err)
	}

	// 执行数据库迁移，为分组表添加request_params字段
	if err := gdb.migrateRequestParamsField(); err != nil {
		return fmt.Errorf("failed to migrate request_params field: %w", err)
	}

	// 创建索引
	for _, indexSQL := range createIndexes {
		if _, err := gdb.db.Exec(indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	log.Println("Provider groups database tables initialized successfully")
	return nil
}

// migrateAPIKeysTable 迁移API密钥表，添加验证相关字段
func (gdb *GroupsDB) migrateAPIKeysTable() error {
	// 检查字段是否已存在
	checkColumnSQL := `PRAGMA table_info(provider_api_keys);`
	rows, err := gdb.db.Query(checkColumnSQL)
	if err != nil {
		return fmt.Errorf("failed to check table info: %w", err)
	}
	defer rows.Close()

	existingColumns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue interface{}

		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("failed to scan column info: %w", err)
		}
		existingColumns[name] = true
	}

	// 添加缺失的字段
	migrations := []string{}

	if !existingColumns["is_valid"] {
		migrations = append(migrations, "ALTER TABLE provider_api_keys ADD COLUMN is_valid BOOLEAN DEFAULT NULL;")
	}

	if !existingColumns["last_validated_at"] {
		migrations = append(migrations, "ALTER TABLE provider_api_keys ADD COLUMN last_validated_at DATETIME DEFAULT NULL;")
	}

	if !existingColumns["validation_error"] {
		migrations = append(migrations, "ALTER TABLE provider_api_keys ADD COLUMN validation_error TEXT DEFAULT NULL;")
	}

	// 执行迁移
	for _, migration := range migrations {
		if _, err := gdb.db.Exec(migration); err != nil {
			return fmt.Errorf("failed to execute migration '%s': %w", migration, err)
		}
		log.Printf("Executed migration: %s", migration)
	}

	if len(migrations) > 0 {
		log.Printf("API keys table migration completed, added %d new columns", len(migrations))
	}

	return nil
}

// migrateRequestParamsField 迁移分组表，添加request_params字段
func (gdb *GroupsDB) migrateRequestParamsField() error {
	// 检查字段是否已存在
	checkColumnSQL := `PRAGMA table_info(provider_groups);`
	rows, err := gdb.db.Query(checkColumnSQL)
	if err != nil {
		return fmt.Errorf("failed to check table info: %w", err)
	}
	defer rows.Close()

	existingColumns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue interface{}

		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("failed to scan column info: %w", err)
		}
		existingColumns[name] = true
	}

	var migrations []string

	// 检查并添加request_params字段
	if !existingColumns["request_params"] {
		migrations = append(migrations, "ALTER TABLE provider_groups ADD COLUMN request_params TEXT DEFAULT NULL;")
	}

	// 执行迁移
	for _, migration := range migrations {
		if _, err := gdb.db.Exec(migration); err != nil {
			return fmt.Errorf("failed to execute migration '%s': %w", migration, err)
		}
		log.Printf("Executed migration: %s", migration)
	}

	if len(migrations) > 0 {
		log.Printf("Provider groups table migration completed, added %d new columns", len(migrations))
	}

	return nil
}

// UpdateAPIKeyValidation 更新API密钥的验证状态
func (gdb *GroupsDB) UpdateAPIKeyValidation(groupID, apiKey string, isValid bool, validationError string) error {
	updateSQL := `
		UPDATE provider_api_keys
		SET is_valid = ?, last_validated_at = CURRENT_TIMESTAMP, validation_error = ?
		WHERE group_id = ? AND api_key = ?`

	_, err := gdb.db.Exec(updateSQL, isValid, validationError, groupID, apiKey)
	if err != nil {
		return fmt.Errorf("failed to update API key validation status: %w", err)
	}

	return nil
}

// GetAPIKeyValidationStatus 获取API密钥的验证状态
func (gdb *GroupsDB) GetAPIKeyValidationStatus(groupID string) (map[string]map[string]interface{}, error) {
	querySQL := `
		SELECT api_key, is_valid, last_validated_at, validation_error
		FROM provider_api_keys
		WHERE group_id = ?
		ORDER BY key_order`

	rows, err := gdb.db.Query(querySQL, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to query API key validation status: %w", err)
	}
	defer rows.Close()

	result := make(map[string]map[string]interface{})

	for rows.Next() {
		var apiKey string
		var isValid *bool
		var lastValidatedAt *string
		var validationError *string

		if err := rows.Scan(&apiKey, &isValid, &lastValidatedAt, &validationError); err != nil {
			return nil, fmt.Errorf("failed to scan validation status: %w", err)
		}

		status := map[string]interface{}{
			"is_valid":           isValid,
			"last_validated_at":  lastValidatedAt,
			"validation_error":   validationError,
		}

		result[apiKey] = status
	}

	return result, nil
}

// SaveGroup 保存分组配置
func (gdb *GroupsDB) SaveGroup(groupID string, group *UserGroup) error {
	tx, err := gdb.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 序列化models、headers和request_params为JSON
	modelsJSON, err := json.Marshal(group.Models)
	if err != nil {
		return fmt.Errorf("failed to marshal models: %w", err)
	}

	headersJSON, err := json.Marshal(group.Headers)
	if err != nil {
		return fmt.Errorf("failed to marshal headers: %w", err)
	}

	requestParamsJSON, err := json.Marshal(group.RequestParams)
	if err != nil {
		return fmt.Errorf("failed to marshal request_params: %w", err)
	}

	// 插入或更新分组信息
	upsertGroupSQL := `
	INSERT INTO provider_groups (
		group_id, name, provider_type, base_url, enabled,
		timeout_seconds, max_retries, rotation_strategy, models, headers, request_params, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(group_id) DO UPDATE SET
		name = excluded.name,
		provider_type = excluded.provider_type,
		base_url = excluded.base_url,
		enabled = excluded.enabled,
		timeout_seconds = excluded.timeout_seconds,
		max_retries = excluded.max_retries,
		rotation_strategy = excluded.rotation_strategy,
		models = excluded.models,
		headers = excluded.headers,
		request_params = excluded.request_params,
		updated_at = CURRENT_TIMESTAMP;`

	_, err = tx.Exec(upsertGroupSQL,
		groupID, group.Name, group.ProviderType, group.BaseURL,
		group.Enabled, int(group.Timeout.Seconds()), group.MaxRetries,
		group.RotationStrategy, string(modelsJSON), string(headersJSON), string(requestParamsJSON))
	if err != nil {
		return fmt.Errorf("failed to save group: %w", err)
	}

	// 删除现有的API密钥
	if _, err = tx.Exec("DELETE FROM provider_api_keys WHERE group_id = ?", groupID); err != nil {
		return fmt.Errorf("failed to delete existing API keys: %w", err)
	}

	// 插入新的API密钥
	insertKeySQL := "INSERT INTO provider_api_keys (group_id, api_key, key_order) VALUES (?, ?, ?)"
	for i, apiKey := range group.APIKeys {
		if _, err = tx.Exec(insertKeySQL, groupID, apiKey, i); err != nil {
			return fmt.Errorf("failed to save API key: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("分组 %s 已保存到数据库", groupID)
	return nil
}

// LoadGroup 加载单个分组配置
func (gdb *GroupsDB) LoadGroup(groupID string) (*UserGroup, error) {
	// 查询分组信息
	groupSQL := `
	SELECT name, provider_type, base_url, enabled,
		   timeout_seconds, max_retries, rotation_strategy, models, headers, request_params
	FROM provider_groups WHERE group_id = ?`

	var group UserGroup
	var modelsJSON, headersJSON string
	var requestParamsJSON *string // 使用指针来处理NULL值
	var timeoutSeconds int

	err := gdb.db.QueryRow(groupSQL, groupID).Scan(
		&group.Name, &group.ProviderType, &group.BaseURL,
		&group.Enabled, &timeoutSeconds, &group.MaxRetries, &group.RotationStrategy,
		&modelsJSON, &headersJSON, &requestParamsJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("group not found: %s", groupID)
		}
		return nil, fmt.Errorf("failed to load group: %w", err)
	}

	// 设置超时时间
	group.Timeout = time.Duration(timeoutSeconds) * time.Second

	// 反序列化models、headers和request_params
	if err = json.Unmarshal([]byte(modelsJSON), &group.Models); err != nil {
		return nil, fmt.Errorf("failed to unmarshal models: %w", err)
	}

	if err = json.Unmarshal([]byte(headersJSON), &group.Headers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal headers: %w", err)
	}

	// 处理request_params，可能为NULL
	if requestParamsJSON != nil && *requestParamsJSON != "" && *requestParamsJSON != "null" {
		if err = json.Unmarshal([]byte(*requestParamsJSON), &group.RequestParams); err != nil {
			return nil, fmt.Errorf("failed to unmarshal request_params: %w", err)
		}
	} else {
		group.RequestParams = make(map[string]interface{})
	}

	// 查询API密钥
	keysSQL := "SELECT api_key FROM provider_api_keys WHERE group_id = ? ORDER BY key_order"
	rows, err := gdb.db.Query(keysSQL, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to load API keys: %w", err)
	}
	defer rows.Close()

	var apiKeys []string
	for rows.Next() {
		var apiKey string
		if err = rows.Scan(&apiKey); err != nil {
			return nil, fmt.Errorf("failed to scan API key: %w", err)
		}
		apiKeys = append(apiKeys, apiKey)
	}

	group.APIKeys = apiKeys
	return &group, nil
}

// LoadAllGroups 加载所有分组配置
func (gdb *GroupsDB) LoadAllGroups() (map[string]*UserGroup, error) {
	// 查询所有分组
	groupsSQL := `
	SELECT group_id, name, provider_type, base_url, enabled,
		   timeout_seconds, max_retries, rotation_strategy, models, headers, request_params
	FROM provider_groups ORDER BY created_at DESC`

	rows, err := gdb.db.Query(groupsSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to query groups: %w", err)
	}
	defer rows.Close()

	groups := make(map[string]*UserGroup)

	for rows.Next() {
		var groupID string
		var group UserGroup
		var modelsJSON, headersJSON string
		var requestParamsJSON *string // 使用指针来处理NULL值
		var timeoutSeconds int

		err = rows.Scan(&groupID, &group.Name, &group.ProviderType, &group.BaseURL,
			&group.Enabled, &timeoutSeconds, &group.MaxRetries,
			&group.RotationStrategy, &modelsJSON, &headersJSON, &requestParamsJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to scan group: %w", err)
		}

		// 设置超时时间
		group.Timeout = time.Duration(timeoutSeconds) * time.Second

		// 反序列化models、headers和request_params
		if err = json.Unmarshal([]byte(modelsJSON), &group.Models); err != nil {
			log.Printf("警告: 分组 %s 的models反序列化失败: %v", groupID, err)
			group.Models = []string{}
		}

		if err = json.Unmarshal([]byte(headersJSON), &group.Headers); err != nil {
			log.Printf("警告: 分组 %s 的headers反序列化失败: %v", groupID, err)
			group.Headers = make(map[string]string)
		}

		// 处理request_params，可能为NULL
		if requestParamsJSON != nil && *requestParamsJSON != "" && *requestParamsJSON != "null" {
			if err = json.Unmarshal([]byte(*requestParamsJSON), &group.RequestParams); err != nil {
				log.Printf("警告: 分组 %s 的request_params反序列化失败: %v", groupID, err)
				group.RequestParams = make(map[string]interface{})
			}
		} else {
			group.RequestParams = make(map[string]interface{})
		}

		groups[groupID] = &group
	}

	// 为每个分组加载API密钥
	for groupID, group := range groups {
		keysSQL := "SELECT api_key FROM provider_api_keys WHERE group_id = ? ORDER BY key_order"
		keyRows, err := gdb.db.Query(keysSQL, groupID)
		if err != nil {
			return nil, fmt.Errorf("failed to load API keys for group %s: %w", groupID, err)
		}

		var apiKeys []string
		for keyRows.Next() {
			var apiKey string
			if err = keyRows.Scan(&apiKey); err != nil {
				keyRows.Close()
				return nil, fmt.Errorf("failed to scan API key: %w", err)
			}
			apiKeys = append(apiKeys, apiKey)
		}
		keyRows.Close()

		group.APIKeys = apiKeys
	}

	return groups, nil
}

// GetGroupsWithMetadata 获取分组配置及元数据（包括创建时间）
func (gdb *GroupsDB) GetGroupsWithMetadata() (map[string]map[string]interface{}, error) {
	// 查询所有分组及元数据
	groupsSQL := `
	SELECT group_id, name, provider_type, base_url, enabled,
		   timeout_seconds, max_retries, rotation_strategy, models, headers,
		   created_at, updated_at
	FROM provider_groups ORDER BY created_at DESC`

	rows, err := gdb.db.Query(groupsSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to query groups: %w", err)
	}
	defer rows.Close()

	groups := make(map[string]map[string]interface{})

	for rows.Next() {
		var groupID, name, providerType, baseURL, rotationStrategy, modelsJSON, headersJSON string
		var enabled bool
		var timeoutSeconds, maxRetries int
		var createdAt, updatedAt time.Time

		err = rows.Scan(&groupID, &name, &providerType, &baseURL, &enabled,
			&timeoutSeconds, &maxRetries, &rotationStrategy, &modelsJSON, &headersJSON,
			&createdAt, &updatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan group: %w", err)
		}

		// 解析模型和头部信息
		var models []string
		var headers map[string]string

		if err = json.Unmarshal([]byte(modelsJSON), &models); err != nil {
			log.Printf("警告: 分组 %s 的models反序列化失败: %v", groupID, err)
			models = []string{}
		}

		if err = json.Unmarshal([]byte(headersJSON), &headers); err != nil {
			log.Printf("警告: 分组 %s 的headers反序列化失败: %v", groupID, err)
			headers = make(map[string]string)
		}

		// 查询API密钥
		keysSQL := "SELECT api_key FROM provider_api_keys WHERE group_id = ? ORDER BY key_order"
		keyRows, err := gdb.db.Query(keysSQL, groupID)
		if err != nil {
			return nil, fmt.Errorf("failed to load API keys for group %s: %w", groupID, err)
		}

		var apiKeys []string
		for keyRows.Next() {
			var apiKey string
			if err = keyRows.Scan(&apiKey); err != nil {
				keyRows.Close()
				return nil, fmt.Errorf("failed to scan API key: %w", err)
			}
			apiKeys = append(apiKeys, apiKey)
		}
		keyRows.Close()

		// 构建分组信息
		groupInfo := map[string]interface{}{
			"group_id":          groupID,
			"group_name":        name,
			"provider_type":     providerType,
			"base_url":          baseURL,
			"enabled":           enabled,
			"timeout":           time.Duration(timeoutSeconds) * time.Second,
			"max_retries":       maxRetries,
			"rotation_strategy": rotationStrategy,
			"api_keys":          apiKeys,
			"models":            models,
			"headers":           headers,
			"created_at":        createdAt,
			"updated_at":        updatedAt,
		}

		groups[groupID] = groupInfo
	}

	return groups, nil
}

// DeleteGroup 删除分组配置
func (gdb *GroupsDB) DeleteGroup(groupID string) error {
	tx, err := gdb.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 删除API密钥（由于外键约束，会自动删除）
	if _, err = tx.Exec("DELETE FROM provider_api_keys WHERE group_id = ?", groupID); err != nil {
		return fmt.Errorf("failed to delete API keys: %w", err)
	}

	// 删除分组
	result, err := tx.Exec("DELETE FROM provider_groups WHERE group_id = ?", groupID)
	if err != nil {
		return fmt.Errorf("failed to delete group: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("group not found: %s", groupID)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("分组 %s 已从数据库中删除", groupID)
	return nil
}

// GetGroupCount 获取分组总数
func (gdb *GroupsDB) GetGroupCount() (int, error) {
	var count int
	err := gdb.db.QueryRow("SELECT COUNT(*) FROM provider_groups").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get group count: %w", err)
	}
	return count, nil
}

// GetEnabledGroupCount 获取启用的分组数量
func (gdb *GroupsDB) GetEnabledGroupCount() (int, error) {
	var count int
	err := gdb.db.QueryRow("SELECT COUNT(*) FROM provider_groups WHERE enabled = 1").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get enabled group count: %w", err)
	}
	return count, nil
}

// Close 关闭数据库连接
func (gdb *GroupsDB) Close() error {
	if gdb.db != nil {
		return gdb.db.Close()
	}
	return nil
}
