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
		group_selection_config TEXT, -- JSON对象，存储分组选择配置
		is_active BOOLEAN NOT NULL DEFAULT 1,
		usage_count INTEGER NOT NULL DEFAULT 0, -- 使用次数
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
		tokens_estimated BOOLEAN NOT NULL DEFAULT 0,
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

	// 迁移proxy_keys表
	if err := d.migrateProxyKeysTable(); err != nil {
		return fmt.Errorf("failed to migrate proxy_keys table: %w", err)
	}

	log.Println("Database tables initialized successfully")
	return nil
}

// migrateProxyKeysTable 迁移proxy_keys表，添加usage_count字段
func (d *Database) migrateProxyKeysTable() error {
	// 检查usage_count字段是否存在
	checkSQL := `PRAGMA table_info(proxy_keys)`
	rows, err := d.db.Query(checkSQL)
	if err != nil {
		return fmt.Errorf("failed to check table info: %w", err)
	}
	defer rows.Close()

	hasUsageCount := false
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, dfltValue, pk interface{}

		if err := rows.Scan(&cid, &name, &dataType, &notNull, &dfltValue, &pk); err != nil {
			continue
		}

		if name == "usage_count" {
			hasUsageCount = true
			break
		}
	}

	// 如果没有usage_count字段，则添加
	if !hasUsageCount {
		alterSQL := `ALTER TABLE proxy_keys ADD COLUMN usage_count INTEGER NOT NULL DEFAULT 0`
		_, err := d.db.Exec(alterSQL)
		if err != nil {
			return fmt.Errorf("failed to add usage_count column: %w", err)
		}
		log.Println("Added usage_count column to proxy_keys table")
	}

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

	// 检查proxy_keys表是否有group_selection_config列
	err = d.db.QueryRow(`
		SELECT COUNT(*) > 0
		FROM pragma_table_info('proxy_keys')
		WHERE name = 'group_selection_config'
	`).Scan(&columnExists)

	if err != nil {
		return fmt.Errorf("failed to check group_selection_config column existence: %w", err)
	}

	// 如果列不存在，添加它
	if !columnExists {
		log.Println("Adding group_selection_config column to proxy_keys table...")
		_, err = d.db.Exec(`ALTER TABLE proxy_keys ADD COLUMN group_selection_config TEXT`)
		if err != nil {
			return fmt.Errorf("failed to add group_selection_config column: %w", err)
		}
		log.Println("Successfully added group_selection_config column")
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

	// 检查request_logs表是否有tokens_estimated列
	err = d.db.QueryRow(`
		SELECT COUNT(*) > 0
		FROM pragma_table_info('request_logs')
		WHERE name = 'tokens_estimated'
	`).Scan(&columnExists)

	if err != nil {
		return fmt.Errorf("failed to check tokens_estimated column existence: %w", err)
	}

	// 如果列不存在，添加它
	if !columnExists {
		log.Println("Adding tokens_estimated column to request_logs table...")
		_, err = d.db.Exec(`ALTER TABLE request_logs ADD COLUMN tokens_estimated BOOLEAN NOT NULL DEFAULT 0`)
		if err != nil {
			return fmt.Errorf("failed to add tokens_estimated column: %w", err)
		}
		log.Println("Successfully added tokens_estimated column")
	}

	// 检查request_logs表是否有工具调用相关字段
	toolCallFields := []string{"has_tool_calls", "tool_calls_count", "tool_names"}
	for _, field := range toolCallFields {
		err = d.db.QueryRow(`
			SELECT COUNT(*) > 0
			FROM pragma_table_info('request_logs')
			WHERE name = ?
		`, field).Scan(&columnExists)

		if err != nil {
			return fmt.Errorf("failed to check %s column existence: %w", field, err)
		}

		// 如果列不存在，添加它
		if !columnExists {
			var alterSQL string
			switch field {
			case "has_tool_calls":
				alterSQL = `ALTER TABLE request_logs ADD COLUMN has_tool_calls BOOLEAN NOT NULL DEFAULT 0`
			case "tool_calls_count":
				alterSQL = `ALTER TABLE request_logs ADD COLUMN tool_calls_count INTEGER NOT NULL DEFAULT 0`
			case "tool_names":
				alterSQL = `ALTER TABLE request_logs ADD COLUMN tool_names TEXT NOT NULL DEFAULT ''`
			}

			log.Printf("Adding %s column to request_logs table...", field)
			_, err = d.db.Exec(alterSQL)
			if err != nil {
				return fmt.Errorf("failed to add %s column: %w", field, err)
			}
			log.Printf("Successfully added %s column", field)
		}
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
		status_code, is_stream, duration, tokens_used, tokens_estimated, error, client_ip, created_at,
		has_tool_calls, tool_calls_count, tool_names
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := d.db.Exec(query,
		log.ProxyKeyName, log.ProxyKeyID, log.ProviderGroup, log.OpenRouterKey, log.Model,
		log.RequestBody, log.ResponseBody, log.StatusCode, log.IsStream,
		log.Duration, log.TokensUsed, log.TokensEstimated, log.Error, log.ClientIP, log.CreatedAt,
		log.HasToolCalls, log.ToolCallsCount, log.ToolNames,
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
		   is_stream, duration, tokens_used, tokens_estimated, error, client_ip, created_at,
		   has_tool_calls, tool_calls_count, tool_names
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
			&log.TokensUsed, &log.TokensEstimated, &log.Error, &log.ClientIP, &log.CreatedAt,
			&log.HasToolCalls, &log.ToolCallsCount, &log.ToolNames,
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
		   is_stream, duration, tokens_used, tokens_estimated, error, client_ip, created_at,
		   has_tool_calls, tool_calls_count, tool_names
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
			&log.TokensUsed, &log.TokensEstimated, &log.Error, &log.ClientIP, &log.CreatedAt,
			&log.HasToolCalls, &log.ToolCallsCount, &log.ToolNames,
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
		   status_code, is_stream, duration, tokens_used, tokens_estimated, error, client_ip, created_at,
		   has_tool_calls, tool_calls_count, tool_names
	FROM request_logs
	WHERE id = ?
	`

	log := &RequestLog{}
	err := d.db.QueryRow(query, id).Scan(
		&log.ID, &log.ProxyKeyName, &log.ProxyKeyID, &log.ProviderGroup, &log.OpenRouterKey, &log.Model,
		&log.RequestBody, &log.ResponseBody, &log.StatusCode, &log.IsStream,
		&log.Duration, &log.TokensUsed, &log.TokensEstimated, &log.Error, &log.ClientIP, &log.CreatedAt,
		&log.HasToolCalls, &log.ToolCallsCount, &log.ToolNames,
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

// GetModelStatsWithFilter 基于筛选与时间范围的模型统计
func (d *Database) GetModelStatsWithFilter(filter *LogFilter) ([]*ModelStats, error) {
	var (
		conds []string
		args  []interface{}
	)
	
	// 基础条件：只统计成功的请求
	conds = append(conds, "status_code = 200")
	
	if filter != nil {
		if filter.ProxyKeyName != "" {
			conds = append(conds, "proxy_key_name = ?")
			args = append(args, filter.ProxyKeyName)
		}
		if filter.ProviderGroup != "" {
			conds = append(conds, "provider_group = ?")
			args = append(args, filter.ProviderGroup)
		}
		if filter.Model != "" {
			conds = append(conds, "model = ?")
			args = append(args, filter.Model)
		}
		if filter.Stream != "" {
			if filter.Stream == "true" {
				conds = append(conds, "is_stream = 1")
			} else if filter.Stream == "false" {
				conds = append(conds, "is_stream = 0")
			}
		}
		if filter.StartTime != nil {
			conds = append(conds, "created_at >= ?")
			args = append(args, filter.StartTime.Format("2006-01-02 15:04:05"))
		}
		if filter.EndTime != nil {
			conds = append(conds, "created_at <= ?")
			args = append(args, filter.EndTime.Format("2006-01-02 15:04:05"))
		}
	}
	
	query := `
	SELECT
		model,
		COUNT(*) as total_requests,
		SUM(tokens_used) as total_tokens,
		AVG(duration) as avg_duration
	FROM request_logs`
	
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	
	query += " GROUP BY model ORDER BY total_requests DESC"

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query model stats with filter: %w", err)
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

// GetTotalTokensStats 获取总token数统计
func (d *Database) GetTotalTokensStats() (*TotalTokensStats, error) {
	query := `
	SELECT
		SUM(tokens_used) as total_tokens,
		SUM(CASE WHEN status_code = 200 THEN tokens_used ELSE 0 END) as success_tokens,
		COUNT(*) as total_requests,
		SUM(CASE WHEN status_code = 200 THEN 1 ELSE 0 END) as success_requests
	FROM request_logs
	`

	stats := &TotalTokensStats{}
	err := d.db.QueryRow(query).Scan(
		&stats.TotalTokens, &stats.SuccessTokens, &stats.TotalRequests, &stats.SuccessRequests,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query total tokens stats: %w", err)
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
	INSERT INTO proxy_keys (id, name, description, key, allowed_groups, group_selection_config, is_active, usage_count, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := d.db.Exec(query,
		key.ID, key.Name, key.Description, key.Key, allowedGroupsJSON, key.GroupSelectionConfig, key.IsActive, key.UsageCount,
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
	SELECT id, name, description, key, allowed_groups, group_selection_config, is_active, usage_count, created_at, updated_at, last_used_at
	FROM proxy_keys
	WHERE key = ? AND is_active = 1
	`

	key := &ProxyKey{}
	var allowedGroupsJSON string
	var groupSelectionConfigJSON sql.NullString
	err := d.db.QueryRow(query, keyValue).Scan(
		&key.ID, &key.Name, &key.Description, &key.Key, &allowedGroupsJSON, &groupSelectionConfigJSON, &key.IsActive,
		&key.UsageCount, &key.CreatedAt, &key.UpdatedAt, &key.LastUsedAt,
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

	// 处理GroupSelectionConfig（可能为NULL）
	if groupSelectionConfigJSON.Valid {
		key.GroupSelectionConfig = groupSelectionConfigJSON.String
	} else {
		key.GroupSelectionConfig = ""
	}

	return key, nil
}

// GetAllProxyKeys 获取所有代理密钥
func (d *Database) GetAllProxyKeys() ([]*ProxyKey, error) {
	query := `
	SELECT id, name, description, key, allowed_groups, group_selection_config, is_active, usage_count, created_at, updated_at, last_used_at
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
		var groupSelectionConfigJSON sql.NullString
		if err := rows.Scan(
			&key.ID, &key.Name, &key.Description, &key.Key, &allowedGroupsJSON, &groupSelectionConfigJSON, &key.IsActive,
			&key.UsageCount, &key.CreatedAt, &key.UpdatedAt, &key.LastUsedAt,
		); err != nil {
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

		// 处理GroupSelectionConfig（可能为NULL）
		if groupSelectionConfigJSON.Valid {
			key.GroupSelectionConfig = groupSelectionConfigJSON.String
		} else {
			key.GroupSelectionConfig = ""
		}

		keys = append(keys, key)
	}

	return keys, nil
}

// UpdateProxyKey 更新代理密钥信息
func (d *Database) UpdateProxyKey(key *ProxyKey) error {
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
	UPDATE proxy_keys
	SET name = ?, description = ?, allowed_groups = ?, group_selection_config = ?, is_active = ?, usage_count = ?, updated_at = ?
	WHERE id = ?
	`

	now := time.Now()
	_, err := d.db.Exec(query,
		key.Name, key.Description, allowedGroupsJSON, key.GroupSelectionConfig, key.IsActive, key.UsageCount, now, key.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update proxy key: %w", err)
	}

	return nil
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

// UpdateProxyKeyUsage 更新代理密钥使用次数
func (d *Database) UpdateProxyKeyUsage(keyID string) error {
	query := `UPDATE proxy_keys SET usage_count = usage_count + 1, last_used_at = ?, updated_at = ? WHERE id = ?`

	now := time.Now()
	_, err := d.db.Exec(query, now, now, keyID)
	if err != nil {
		return fmt.Errorf("failed to update proxy key usage: %w", err)
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
		   status_code, is_stream, duration, tokens_used, tokens_estimated, error, client_ip, created_at,
		   has_tool_calls, tool_calls_count, tool_names
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
			&log.Duration, &log.TokensUsed, &log.TokensEstimated, &log.Error, &log.ClientIP, &log.CreatedAt,
			&log.HasToolCalls, &log.ToolCallsCount, &log.ToolNames,
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
		   status_code, is_stream, duration, tokens_used, tokens_estimated, error, client_ip, created_at,
		   has_tool_calls, tool_calls_count, tool_names
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
			&log.Duration, &log.TokensUsed, &log.TokensEstimated, &log.Error, &log.ClientIP, &log.CreatedAt,
			&log.HasToolCalls, &log.ToolCallsCount, &log.ToolNames,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan request log: %w", err)
		}
		logs = append(logs, log)
	}

	return logs, nil
}

 // GetStatusStats 基于筛选与时间范围的状态分布聚合
 func (d *Database) GetStatusStats(filter *LogFilter) (*StatusStats, error) {
 	var (
 		conds []string
 		args  []interface{}
 	)
 	if filter != nil {
 		if filter.ProxyKeyName != "" {
 			conds = append(conds, "proxy_key_name = ?")
 			args = append(args, filter.ProxyKeyName)
 		}
 		if filter.ProviderGroup != "" {
 			conds = append(conds, "provider_group = ?")
 			args = append(args, filter.ProviderGroup)
 		}
 		if filter.Model != "" {
 			conds = append(conds, "model = ?")
 			args = append(args, filter.Model)
 		}
 		if filter.Stream != "" {
 			if filter.Stream == "true" {
 				conds = append(conds, "is_stream = 1")
 			} else if filter.Stream == "false" {
 				conds = append(conds, "is_stream = 0")
 			}
 		}
 		if filter.Status != "" {
 			if filter.Status == "200" {
 				conds = append(conds, "status_code = 200")
 			} else if filter.Status == "error" {
 				conds = append(conds, "status_code != 200")
 			}
 		}
 		if filter.StartTime != nil {
 			conds = append(conds, "created_at >= ?")
 			args = append(args, filter.StartTime.Format("2006-01-02 15:04:05"))
 		}
 		if filter.EndTime != nil {
 			conds = append(conds, "created_at <= ?")
 			args = append(args, filter.EndTime.Format("2006-01-02 15:04:05"))
 		}
 	}
 	query := `
 		SELECT
 			SUM(CASE WHEN status_code = 200 THEN 1 ELSE 0 END) AS success_count,
 			SUM(CASE WHEN status_code != 200 THEN 1 ELSE 0 END) AS error_count
 		FROM request_logs`
 	if len(conds) > 0 {
 		query += " WHERE " + strings.Join(conds, " AND ")
 	}
 	var res StatusStats
 	if err := d.db.QueryRow(query, args...).Scan(&res.Success, &res.Error); err != nil {
 		return nil, fmt.Errorf("failed to query status stats: %w", err)
 	}
 	return &res, nil
 }
 
 // GetTokensTimeline 基于筛选与时间范围的tokens时间序列；≤24h按小时，否则按天
 func (d *Database) GetTokensTimeline(filter *LogFilter) ([]*TimelinePoint, error) {
 	var (
 		conds []string
 		args  []interface{}
 	)
 	var start, end time.Time
 	hasRange := false
 	if filter != nil {
 		if filter.ProxyKeyName != "" {
 			conds = append(conds, "proxy_key_name = ?")
 			args = append(args, filter.ProxyKeyName)
 		}
 		if filter.ProviderGroup != "" {
 			conds = append(conds, "provider_group = ?")
 			args = append(args, filter.ProviderGroup)
 		}
 		if filter.Model != "" {
 			conds = append(conds, "model = ?")
 			args = append(args, filter.Model)
 		}
 		if filter.Stream != "" {
 			if filter.Stream == "true" {
 				conds = append(conds, "is_stream = 1")
 			} else if filter.Stream == "false" {
 				conds = append(conds, "is_stream = 0")
 			}
 		}
 		// 注意：不要用 Status 限制到 success-only，这里要返回 total 与 success 两条序列
 		if filter.StartTime != nil {
 			start = *filter.StartTime
 			conds = append(conds, "created_at >= ?")
 			args = append(args, start.Format("2006-01-02 15:04:05"))
 			hasRange = true
 		}
 		if filter.EndTime != nil {
 			end = *filter.EndTime
 			conds = append(conds, "created_at <= ?")
 			args = append(args, end.Format("2006-01-02 15:04:05"))
 			hasRange = true
 		}
 	}
 	// 自动选择粒度
 	bucket := "%Y-%m-%d"
 	if hasRange {
 		if end.IsZero() {
 			end = time.Now()
 		}
 		if start.IsZero() {
 			// 默认取最近24h
 			start = end.Add(-24 * time.Hour)
 		}
 		if end.Sub(start) <= 24*time.Hour {
 			bucket = "%Y-%m-%d %H:00"
 		}
 	}
 	query := fmt.Sprintf(`
 		SELECT
 			strftime('%s', created_at) AS bucket_time,
 			SUM(tokens_used) AS total_tokens,
 			SUM(CASE WHEN status_code = 200 THEN tokens_used ELSE 0 END) AS success_tokens
 		FROM request_logs`, bucket)
 	if len(conds) > 0 {
 		query += " WHERE " + strings.Join(conds, " AND ")
 	}
 	query += " GROUP BY bucket_time ORDER BY bucket_time ASC"
 
 	rows, err := d.db.Query(query, args...)
 	if err != nil {
 		return nil, fmt.Errorf("failed to query tokens timeline: %w", err)
 	}
 	defer rows.Close()
 
 	var out []*TimelinePoint
 	for rows.Next() {
 		var t TimelinePoint
 		if err := rows.Scan(&t.Date, &t.Total, &t.Success); err != nil {
 			return nil, fmt.Errorf("failed to scan timeline row: %w", err)
 		}
 		out = append(out, &t)
 	}
 	return out, nil
 }
 
 // GetGroupTokensStats 基于筛选与时间范围的分组tokens聚合（按 total desc）
 func (d *Database) GetGroupTokensStats(filter *LogFilter) ([]*GroupTokensStat, error) {
 	var (
 		conds []string
 		args  []interface{}
 	)
 	if filter != nil {
 		if filter.ProxyKeyName != "" {
 			conds = append(conds, "proxy_key_name = ?")
 			args = append(args, filter.ProxyKeyName)
 		}
 		if filter.ProviderGroup != "" {
 			conds = append(conds, "provider_group = ?")
 			args = append(args, filter.ProviderGroup)
 		}
 		if filter.Model != "" {
 			conds = append(conds, "model = ?")
 			args = append(args, filter.Model)
 		}
 		if filter.Stream != "" {
 			if filter.Stream == "true" {
 				conds = append(conds, "is_stream = 1")
 			} else if filter.Stream == "false" {
 				conds = append(conds, "is_stream = 0")
 			}
 		}
 		if filter.Status != "" {
 			if filter.Status == "200" {
 				conds = append(conds, "status_code = 200")
 			} else if filter.Status == "error" {
 				conds = append(conds, "status_code != 200")
 			}
 		}
 		if filter.StartTime != nil {
 			conds = append(conds, "created_at >= ?")
 			args = append(args, filter.StartTime.Format("2006-01-02 15:04:05"))
 		}
 		if filter.EndTime != nil {
 			conds = append(conds, "created_at <= ?")
 			args = append(args, filter.EndTime.Format("2006-01-02 15:04:05"))
 		}
 	}
 	query := `
 		SELECT
 			COALESCE(provider_group, '') AS grp,
 			SUM(tokens_used) AS total_tokens,
 			SUM(CASE WHEN status_code = 200 THEN tokens_used ELSE 0 END) AS success_tokens
 		FROM request_logs`
 	if len(conds) > 0 {
 		query += " WHERE " + strings.Join(conds, " AND ")
 	}
 	query += " GROUP BY grp ORDER BY total_tokens DESC"
 
 	rows, err := d.db.Query(query, args...)
 	if err != nil {
 		return nil, fmt.Errorf("failed to query group tokens stats: %w", err)
 	}
 	defer rows.Close()
 
 	var out []*GroupTokensStat
 	for rows.Next() {
 		var g GroupTokensStat
 		if err := rows.Scan(&g.Group, &g.Total, &g.Success); err != nil {
 			return nil, fmt.Errorf("failed to scan group tokens stat: %w", err)
 		}
 		if g.Group == "" {
 			g.Group = "-"
 		}
 		out = append(out, &g)
 	}
 	return out, nil
 }
