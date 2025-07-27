package logger

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Database 数据库管理器
type Database struct {
	db *sql.DB
}

// NewDatabase 创建新的数据库管理器
func NewDatabase(dbPath string) (*Database, error) {
	// 确保数据库目录存在
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// 打开数据库连接
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	database := &Database{db: db}

	// 初始化数据库表
	if err := database.initTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize tables: %w", err)
	}

	// 执行数据库迁移
	if err := database.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return database, nil
}

// Close 关闭数据库连接
func (d *Database) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// initTables 初始化数据库表
func (d *Database) initTables() error {
	createTableSQL := `
	-- 代理服务API密钥表
	CREATE TABLE IF NOT EXISTS proxy_keys (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		key TEXT NOT NULL UNIQUE,
		allowed_groups TEXT, -- JSON数组，存储允许访问的分组ID
		is_active BOOLEAN NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_used_at DATETIME
	);

	-- 请求日志表
	CREATE TABLE IF NOT EXISTS request_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		proxy_key_name TEXT NOT NULL,
		proxy_key_id TEXT NOT NULL,
		provider_group TEXT NOT NULL DEFAULT '',
		openrouter_key TEXT NOT NULL,
		model TEXT NOT NULL,
		request_body TEXT NOT NULL,
		response_body TEXT,
		status_code INTEGER NOT NULL,
		is_stream BOOLEAN NOT NULL DEFAULT 0,
		duration INTEGER NOT NULL DEFAULT 0,
		tokens_used INTEGER NOT NULL DEFAULT 0,
		error TEXT,
		client_ip TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (proxy_key_id) REFERENCES proxy_keys(id)
	);

	-- 索引
	CREATE INDEX IF NOT EXISTS idx_proxy_keys_name ON proxy_keys(name);
	CREATE INDEX IF NOT EXISTS idx_proxy_keys_key ON proxy_keys(key);
	CREATE INDEX IF NOT EXISTS idx_proxy_keys_is_active ON proxy_keys(is_active);
	
	CREATE INDEX IF NOT EXISTS idx_request_logs_proxy_key_id ON request_logs(proxy_key_id);
	CREATE INDEX IF NOT EXISTS idx_request_logs_proxy_key_name ON request_logs(proxy_key_name);
	CREATE INDEX IF NOT EXISTS idx_request_logs_provider_group ON request_logs(provider_group);
	CREATE INDEX IF NOT EXISTS idx_request_logs_model ON request_logs(model);
	CREATE INDEX IF NOT EXISTS idx_request_logs_created_at ON request_logs(created_at);
	CREATE INDEX IF NOT EXISTS idx_request_logs_status_code ON request_logs(status_code);
	`

	_, err := d.db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	// 执行数据库迁移
	if err := d.migrateDatabase(); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	log.Println("Database tables initialized successfully")
	return nil
}

// migrateDatabase 执行数据库迁移
func (d *Database) migrateDatabase() error {
	// 检查proxy_keys表是否有allowed_groups列
	var columnExists bool
	err := d.db.QueryRow(`
		SELECT COUNT(*) > 0
		FROM pragma_table_info('proxy_keys')
		WHERE name = 'allowed_groups'
	`).Scan(&columnExists)

	if err != nil {
		return fmt.Errorf("failed to check column existence: %w", err)
	}

	// 如果列不存在，添加它
	if !columnExists {
		log.Println("Adding allowed_groups column to proxy_keys table...")
		_, err = d.db.Exec(`ALTER TABLE proxy_keys ADD COLUMN allowed_groups TEXT`)
		if err != nil {
			return fmt.Errorf("failed to add allowed_groups column: %w", err)
		}
		log.Println("Successfully added allowed_groups column")
	}

	// 检查request_logs表是否有client_ip列
	err = d.db.QueryRow(`
		SELECT COUNT(*) > 0
		FROM pragma_table_info('request_logs')
		WHERE name = 'client_ip'
	`).Scan(&columnExists)

	if err != nil {
		return fmt.Errorf("failed to check client_ip column existence: %w", err)
	}

	// 如果列不存在，添加它
	if !columnExists {
		log.Println("Adding client_ip column to request_logs table...")
		_, err = d.db.Exec(`ALTER TABLE request_logs ADD COLUMN client_ip TEXT NOT NULL DEFAULT ''`)
		if err != nil {
			return fmt.Errorf("failed to add client_ip column: %w", err)
		}
		log.Println("Successfully added client_ip column")
	}

	return nil
}

// migrate 执行数据库迁移
func (d *Database) migrate() error {
	// 检查是否需要添加provider_group字段
	var columnExists bool
	err := d.db.QueryRow(`
		SELECT COUNT(*) > 0
		FROM pragma_table_info('request_logs')
		WHERE name = 'provider_group'
	`).Scan(&columnExists)

	if err != nil {
		return fmt.Errorf("failed to check provider_group column: %w", err)
	}

	// 如果字段不存在，添加它
	if !columnExists {
		log.Printf("Adding provider_group column to request_logs table...")
		_, err = d.db.Exec(`ALTER TABLE request_logs ADD COLUMN provider_group TEXT NOT NULL DEFAULT ''`)
		if err != nil {
			return fmt.Errorf("failed to add provider_group column: %w", err)
		}

		// 添加索引
		_, err = d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_request_logs_provider_group ON request_logs(provider_group)`)
		if err != nil {
			return fmt.Errorf("failed to create provider_group index: %w", err)
		}

		log.Printf("Successfully added provider_group column and index")
	}

	return nil
}

// InsertRequestLog 插入请求日志
func (d *Database) InsertRequestLog(log *RequestLog) error {
	query := `
	INSERT INTO request_logs (
		proxy_key_name, proxy_key_id, provider_group, openrouter_key, model, request_body, response_body,
		status_code, is_stream, duration, tokens_used, error, client_ip, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := d.db.Exec(query,
		log.ProxyKeyName, log.ProxyKeyID, log.ProviderGroup, log.OpenRouterKey, log.Model,
		log.RequestBody, log.ResponseBody, log.StatusCode, log.IsStream,
		log.Duration, log.TokensUsed, log.Error, log.ClientIP, log.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert request log: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	log.ID = id
	return nil
}

// GetRequestLogs 获取请求日志列表
func (d *Database) GetRequestLogs(proxyKeyName, providerGroup string, limit, offset int) ([]*RequestLogSummary, error) {
	var query string
	var args []interface{}
	var conditions []string

	// 构建WHERE条件
	if proxyKeyName != "" {
		conditions = append(conditions, "proxy_key_name = ?")
		args = append(args, proxyKeyName)
	}

	if providerGroup != "" {
		conditions = append(conditions, "provider_group = ?")
		args = append(args, providerGroup)
	}

	// 构建查询语句
	query = `
	SELECT id, proxy_key_name, proxy_key_id, provider_group, openrouter_key, model, status_code,
		   is_stream, duration, tokens_used, error, client_ip, created_at
	FROM request_logs`

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query request logs: %w", err)
	}
	defer rows.Close()

	var logs []*RequestLogSummary
	for rows.Next() {
		log := &RequestLogSummary{}
		err := rows.Scan(
			&log.ID, &log.ProxyKeyName, &log.ProxyKeyID, &log.ProviderGroup, &log.OpenRouterKey,
			&log.Model, &log.StatusCode, &log.IsStream, &log.Duration,
			&log.TokensUsed, &log.Error, &log.ClientIP, &log.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan request log: %w", err)
		}
		logs = append(logs, log)
	}

	return logs, nil
}

// GetRequestLogsWithFilter 根据筛选条件获取请求日志列表
func (d *Database) GetRequestLogsWithFilter(filter *LogFilter) ([]*RequestLogSummary, error) {
	var query string
	var args []interface{}
	var conditions []string

	// 构建WHERE条件
	if filter.ProxyKeyName != "" {
		conditions = append(conditions, "proxy_key_name = ?")
		args = append(args, filter.ProxyKeyName)
	}

	if filter.ProviderGroup != "" {
		conditions = append(conditions, "provider_group = ?")
		args = append(args, filter.ProviderGroup)
	}

	if filter.Model != "" {
		conditions = append(conditions, "model = ?")
		args = append(args, filter.Model)
	}

	if filter.Status != "" {
		if filter.Status == "200" {
			conditions = append(conditions, "status_code = 200")
		} else if filter.Status == "error" {
			conditions = append(conditions, "status_code != 200")
		}
	}

	if filter.Stream != "" {
		if filter.Stream == "true" {
			conditions = append(conditions, "is_stream = 1")
		} else if filter.Stream == "false" {
			conditions = append(conditions, "is_stream = 0")
		}
	}

	// 构建查询语句
	query = `
	SELECT id, proxy_key_name, proxy_key_id, provider_group, openrouter_key, model, status_code,
		   is_stream, duration, tokens_used, error, client_ip, created_at
	FROM request_logs`

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, filter.Limit, filter.Offset)

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query request logs with filter: %w", err)
	}
	defer rows.Close()

	var logs []*RequestLogSummary
	for rows.Next() {
		log := &RequestLogSummary{}
		err := rows.Scan(
			&log.ID, &log.ProxyKeyName, &log.ProxyKeyID, &log.ProviderGroup, &log.OpenRouterKey,
			&log.Model, &log.StatusCode, &log.IsStream, &log.Duration,
			&log.TokensUsed, &log.Error, &log.ClientIP, &log.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan request log: %w", err)
		}
		logs = append(logs, log)
	}

	return logs, nil
}

// GetRequestCountWithFilter 根据筛选条件获取请求总数
func (d *Database) GetRequestCountWithFilter(filter *LogFilter) (int64, error) {
	var query string
	var args []interface{}
	var conditions []string

	// 构建WHERE条件（与GetRequestLogsWithFilter保持一致）
	if filter.ProxyKeyName != "" {
		conditions = append(conditions, "proxy_key_name = ?")
		args = append(args, filter.ProxyKeyName)
	}

	if filter.ProviderGroup != "" {
		conditions = append(conditions, "provider_group = ?")
		args = append(args, filter.ProviderGroup)
	}

	if filter.Model != "" {
		conditions = append(conditions, "model = ?")
		args = append(args, filter.Model)
	}

	if filter.Status != "" {
		if filter.Status == "200" {
			conditions = append(conditions, "status_code = 200")
		} else if filter.Status == "error" {
			conditions = append(conditions, "status_code != 200")
		}
	}

	if filter.Stream != "" {
		if filter.Stream == "true" {
			conditions = append(conditions, "is_stream = 1")
		} else if filter.Stream == "false" {
			conditions = append(conditions, "is_stream = 0")
		}
	}

	// 构建查询语句
	query = "SELECT COUNT(*) FROM request_logs"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	var count int64
	err := d.db.QueryRow(query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get request count with filter: %w", err)
	}

	return count, nil
}

// GetRequestLogDetail 获取请求日志详情
func (d *Database) GetRequestLogDetail(id int64) (*RequestLog, error) {
	query := `
	SELECT id, proxy_key_name, proxy_key_id, provider_group, openrouter_key, model, request_body, response_body,
		   status_code, is_stream, duration, tokens_used, error, client_ip, created_at
	FROM request_logs
	WHERE id = ?
	`

	log := &RequestLog{}
	err := d.db.QueryRow(query, id).Scan(
		&log.ID, &log.ProxyKeyName, &log.ProxyKeyID, &log.ProviderGroup, &log.OpenRouterKey, &log.Model,
		&log.RequestBody, &log.ResponseBody, &log.StatusCode, &log.IsStream,
		&log.Duration, &log.TokensUsed, &log.Error, &log.ClientIP, &log.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("request log not found")
		}
		return nil, fmt.Errorf("failed to get request log detail: %w", err)
	}

	return log, nil
}

// GetProxyKeyStats 获取代理密钥统计
func (d *Database) GetProxyKeyStats() ([]*ProxyKeyStats, error) {
	query := `
	SELECT 
		proxy_key_name,
		proxy_key_id,
		COUNT(*) as total_requests,
		SUM(CASE WHEN status_code = 200 THEN 1 ELSE 0 END) as success_requests,
		SUM(CASE WHEN status_code != 200 THEN 1 ELSE 0 END) as error_requests,
		SUM(tokens_used) as total_tokens,
		AVG(duration) as avg_duration
	FROM request_logs 
	GROUP BY proxy_key_name, proxy_key_id
	ORDER BY total_requests DESC
	`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query proxy key stats: %w", err)
	}
	defer rows.Close()

	var stats []*ProxyKeyStats
	for rows.Next() {
		stat := &ProxyKeyStats{}
		err := rows.Scan(
			&stat.ProxyKeyName, &stat.ProxyKeyID, &stat.TotalRequests, &stat.SuccessRequests,
			&stat.ErrorRequests, &stat.TotalTokens, &stat.AvgDuration,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan proxy key stats: %w", err)
		}
		stats = append(stats, stat)
	}

	return stats, nil
}

// GetModelStats 获取模型统计
func (d *Database) GetModelStats() ([]*ModelStats, error) {
	query := `
	SELECT 
		model,
		COUNT(*) as total_requests,
		SUM(tokens_used) as total_tokens,
		AVG(duration) as avg_duration
	FROM request_logs 
	WHERE status_code = 200
	GROUP BY model
	ORDER BY total_requests DESC
	`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query model stats: %w", err)
	}
	defer rows.Close()

	var stats []*ModelStats
	for rows.Next() {
		stat := &ModelStats{}
		err := rows.Scan(
			&stat.Model, &stat.TotalRequests, &stat.TotalTokens, &stat.AvgDuration,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan model stats: %w", err)
		}
		stats = append(stats, stat)
	}

	return stats, nil
}

// GetRequestCount 获取请求总数
func (d *Database) GetRequestCount(proxyKeyName, providerGroup string) (int64, error) {
	var query string
	var args []interface{}
	var conditions []string

	// 构建WHERE条件
	if proxyKeyName != "" {
		conditions = append(conditions, "proxy_key_name = ?")
		args = append(args, proxyKeyName)
	}

	if providerGroup != "" {
		conditions = append(conditions, "provider_group = ?")
		args = append(args, providerGroup)
	}

	// 构建查询语句
	query = "SELECT COUNT(*) FROM request_logs"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	var count int64
	err := d.db.QueryRow(query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get request count: %w", err)
	}

	return count, nil
}

// InsertProxyKey 插入代理密钥
func (d *Database) InsertProxyKey(key *ProxyKey) error {
	// 将AllowedGroups转换为JSON字符串
	allowedGroupsJSON := "[]"
	if key.AllowedGroups != nil && len(key.AllowedGroups) > 0 {
		if jsonBytes, err := json.Marshal(key.AllowedGroups); err == nil {
			allowedGroupsJSON = string(jsonBytes)
		} else {
			log.Printf("Failed to marshal AllowedGroups: %v", err)
		}
	}





	query := `
	INSERT INTO proxy_keys (id, name, description, key, allowed_groups, is_active, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := d.db.Exec(query,
		key.ID, key.Name, key.Description, key.Key, allowedGroupsJSON, key.IsActive,
		key.CreatedAt, key.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert proxy key: %w", err)
	}

	return nil
}

// GetProxyKey 根据密钥获取代理密钥信息
func (d *Database) GetProxyKey(keyValue string) (*ProxyKey, error) {
	query := `
	SELECT id, name, description, key, allowed_groups, is_active, created_at, updated_at, last_used_at
	FROM proxy_keys
	WHERE key = ? AND is_active = 1
	`

	key := &ProxyKey{}
	var allowedGroupsJSON string
	err := d.db.QueryRow(query, keyValue).Scan(
		&key.ID, &key.Name, &key.Description, &key.Key, &allowedGroupsJSON, &key.IsActive,
		&key.CreatedAt, &key.UpdatedAt, &key.LastUsedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("proxy key not found")
		}
		return nil, fmt.Errorf("failed to get proxy key: %w", err)
	}

	// 解析AllowedGroups JSON
	if allowedGroupsJSON != "" {
		if err := json.Unmarshal([]byte(allowedGroupsJSON), &key.AllowedGroups); err != nil {
			key.AllowedGroups = []string{} // 解析失败时使用空数组
		}
	} else {
		key.AllowedGroups = []string{}
	}

	return key, nil
}

// GetAllProxyKeys 获取所有代理密钥
func (d *Database) GetAllProxyKeys() ([]*ProxyKey, error) {
	query := `
	SELECT id, name, description, key, allowed_groups, is_active, created_at, updated_at, last_used_at
	FROM proxy_keys
	ORDER BY created_at DESC
	`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query proxy keys: %w", err)
	}
	defer rows.Close()

	var keys []*ProxyKey
	for rows.Next() {
		key := &ProxyKey{}
		var allowedGroupsJSON string
		err := rows.Scan(
			&key.ID, &key.Name, &key.Description, &key.Key, &allowedGroupsJSON, &key.IsActive,
			&key.CreatedAt, &key.UpdatedAt, &key.LastUsedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan proxy key: %w", err)
		}

		// 解析AllowedGroups JSON
		if allowedGroupsJSON != "" {
			if err := json.Unmarshal([]byte(allowedGroupsJSON), &key.AllowedGroups); err != nil {
				key.AllowedGroups = []string{} // 解析失败时使用空数组
			}
		} else {
			key.AllowedGroups = []string{}
		}

		keys = append(keys, key)
	}

	return keys, nil
}

// UpdateProxyKeyLastUsed 更新代理密钥最后使用时间
func (d *Database) UpdateProxyKeyLastUsed(keyID string) error {
	query := `UPDATE proxy_keys SET last_used_at = ?, updated_at = ? WHERE id = ?`
	
	now := time.Now()
	_, err := d.db.Exec(query, now, now, keyID)
	if err != nil {
		return fmt.Errorf("failed to update proxy key last used: %w", err)
	}

	return nil
}

// DeleteProxyKey 删除代理密钥
func (d *Database) DeleteProxyKey(keyID string) error {
	query := `DELETE FROM proxy_keys WHERE id = ?`
	
	_, err := d.db.Exec(query, keyID)
	if err != nil {
		return fmt.Errorf("failed to delete proxy key: %w", err)
	}

	return nil
}

// CleanupOldLogs 清理旧日志（保留指定天数的日志）
func (d *Database) CleanupOldLogs(retentionDays int) error {
	query := `DELETE FROM request_logs WHERE created_at < datetime('now', '-' || ? || ' days')`

	result, err := d.db.Exec(query, retentionDays)
	if err != nil {
		return fmt.Errorf("failed to cleanup old logs: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	log.Printf("Cleaned up %d old log records", rowsAffected)
	return nil
}

// DeleteRequestLogs 批量删除指定ID的请求日志
func (d *Database) DeleteRequestLogs(ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	// 构建IN子句的占位符
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf("DELETE FROM request_logs WHERE id IN (%s)", strings.Join(placeholders, ","))

	result, err := d.db.Exec(query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to delete request logs: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected, nil
}

// ClearAllRequestLogs 清空所有请求日志
func (d *Database) ClearAllRequestLogs() (int64, error) {
	query := `DELETE FROM request_logs`

	result, err := d.db.Exec(query)
	if err != nil {
		return 0, fmt.Errorf("failed to clear all request logs: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected, nil
}

// GetAllRequestLogsForExport 获取所有请求日志用于导出（包含完整信息）
func (d *Database) GetAllRequestLogsForExport(proxyKeyName, providerGroup string) ([]*RequestLog, error) {
	var query string
	var args []interface{}
	var conditions []string

	// 构建WHERE条件
	if proxyKeyName != "" {
		conditions = append(conditions, "proxy_key_name = ?")
		args = append(args, proxyKeyName)
	}

	if providerGroup != "" {
		conditions = append(conditions, "provider_group = ?")
		args = append(args, providerGroup)
	}

	// 构建查询语句
	query = `
	SELECT id, proxy_key_name, proxy_key_id, provider_group, openrouter_key, model, request_body, response_body,
		   status_code, is_stream, duration, tokens_used, error, client_ip, created_at
	FROM request_logs`

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY created_at DESC"

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query request logs for export: %w", err)
	}
	defer rows.Close()

	var logs []*RequestLog
	for rows.Next() {
		log := &RequestLog{}
		err := rows.Scan(
			&log.ID, &log.ProxyKeyName, &log.ProxyKeyID, &log.ProviderGroup, &log.OpenRouterKey,
			&log.Model, &log.RequestBody, &log.ResponseBody, &log.StatusCode, &log.IsStream,
			&log.Duration, &log.TokensUsed, &log.Error, &log.ClientIP, &log.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan request log: %w", err)
		}
		logs = append(logs, log)
	}

	return logs, nil
}

// GetAllRequestLogsForExportWithFilter 根据筛选条件获取所有请求日志用于导出（包含完整信息）
func (d *Database) GetAllRequestLogsForExportWithFilter(filter *LogFilter) ([]*RequestLog, error) {
	var query string
	var args []interface{}
	var conditions []string

	// 构建WHERE条件（与GetRequestLogsWithFilter保持一致）
	if filter.ProxyKeyName != "" {
		conditions = append(conditions, "proxy_key_name = ?")
		args = append(args, filter.ProxyKeyName)
	}

	if filter.ProviderGroup != "" {
		conditions = append(conditions, "provider_group = ?")
		args = append(args, filter.ProviderGroup)
	}

	if filter.Model != "" {
		conditions = append(conditions, "model = ?")
		args = append(args, filter.Model)
	}

	if filter.Status != "" {
		if filter.Status == "200" {
			conditions = append(conditions, "status_code = 200")
		} else if filter.Status == "error" {
			conditions = append(conditions, "status_code != 200")
		}
	}

	if filter.Stream != "" {
		if filter.Stream == "true" {
			conditions = append(conditions, "is_stream = 1")
		} else if filter.Stream == "false" {
			conditions = append(conditions, "is_stream = 0")
		}
	}

	// 构建查询语句
	query = `
	SELECT id, proxy_key_name, proxy_key_id, provider_group, openrouter_key, model, request_body, response_body,
		   status_code, is_stream, duration, tokens_used, error, client_ip, created_at
	FROM request_logs`

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY created_at DESC"

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query request logs for export with filter: %w", err)
	}
	defer rows.Close()

	var logs []*RequestLog
	for rows.Next() {
		log := &RequestLog{}
		err := rows.Scan(
			&log.ID, &log.ProxyKeyName, &log.ProxyKeyID, &log.ProviderGroup, &log.OpenRouterKey,
			&log.Model, &log.RequestBody, &log.ResponseBody, &log.StatusCode, &log.IsStream,
			&log.Duration, &log.TokensUsed, &log.Error, &log.ClientIP, &log.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan request log: %w", err)
		}
		logs = append(logs, log)
	}

	return logs, nil
}