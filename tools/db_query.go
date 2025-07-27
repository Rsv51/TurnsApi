package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("用法: go run tools/db_query.go <数据库路径>")
	}

	dbPath := os.Args[1]
	
	// 打开数据库
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer db.Close()

	fmt.Println("=== TurnsAPI 数据库查询工具 ===")
	fmt.Printf("数据库路径: %s\n\n", dbPath)

	// 查询分组信息
	fmt.Println("1. 分组信息:")
	groupsSQL := `
	SELECT group_id, name, provider_type, base_url, enabled, 
		   timeout_seconds, max_retries, rotation_strategy,
		   created_at, updated_at
	FROM provider_groups 
	ORDER BY group_id`

	rows, err := db.Query(groupsSQL)
	if err != nil {
		log.Fatalf("查询分组失败: %v", err)
	}
	defer rows.Close()

	fmt.Printf("%-25s %-20s %-15s %-30s %-8s %-8s %-12s %-15s\n", 
		"分组ID", "名称", "提供商类型", "Base URL", "启用", "超时", "重试次数", "轮换策略")
	fmt.Println(strings.Repeat("-", 140))

	groupCount := 0
	for rows.Next() {
		var groupID, name, providerType, baseURL, rotationStrategy, createdAt, updatedAt string
		var enabled bool
		var timeoutSeconds, maxRetries int

		err = rows.Scan(&groupID, &name, &providerType, &baseURL, &enabled, 
			&timeoutSeconds, &maxRetries, &rotationStrategy, &createdAt, &updatedAt)
		if err != nil {
			log.Printf("扫描分组数据失败: %v", err)
			continue
		}

		enabledStr := "否"
		if enabled {
			enabledStr = "是"
		}

		// 截断长URL
		displayURL := baseURL
		if len(displayURL) > 28 {
			displayURL = displayURL[:25] + "..."
		}

		fmt.Printf("%-25s %-20s %-15s %-30s %-8s %-8d %-12d %-15s\n", 
			groupID, name, providerType, displayURL, enabledStr, 
			timeoutSeconds, maxRetries, rotationStrategy)
		
		groupCount++
	}

	fmt.Printf("\n总计: %d 个分组\n\n", groupCount)

	// 查询API密钥信息
	fmt.Println("2. API密钥统计:")
	keysSQL := `
	SELECT pg.group_id, pg.name, COUNT(pak.api_key) as key_count
	FROM provider_groups pg
	LEFT JOIN provider_api_keys pak ON pg.group_id = pak.group_id
	GROUP BY pg.group_id, pg.name
	ORDER BY pg.group_id`

	keyRows, err := db.Query(keysSQL)
	if err != nil {
		log.Fatalf("查询API密钥失败: %v", err)
	}
	defer keyRows.Close()

	fmt.Printf("%-25s %-20s %-10s\n", "分组ID", "名称", "密钥数量")
	fmt.Println(strings.Repeat("-", 60))

	totalKeys := 0
	for keyRows.Next() {
		var groupID, name string
		var keyCount int

		err = keyRows.Scan(&groupID, &name, &keyCount)
		if err != nil {
			log.Printf("扫描密钥数据失败: %v", err)
			continue
		}

		fmt.Printf("%-25s %-20s %-10d\n", groupID, name, keyCount)
		totalKeys += keyCount
	}

	fmt.Printf("\n总计: %d 个API密钥\n\n", totalKeys)

	// 查询启用的分组
	fmt.Println("3. 启用状态统计:")
	var enabledCount, disabledCount int
	
	err = db.QueryRow("SELECT COUNT(*) FROM provider_groups WHERE enabled = 1").Scan(&enabledCount)
	if err != nil {
		log.Printf("查询启用分组数量失败: %v", err)
	}
	
	err = db.QueryRow("SELECT COUNT(*) FROM provider_groups WHERE enabled = 0").Scan(&disabledCount)
	if err != nil {
		log.Printf("查询禁用分组数量失败: %v", err)
	}

	fmt.Printf("启用的分组: %d\n", enabledCount)
	fmt.Printf("禁用的分组: %d\n", disabledCount)
	fmt.Printf("总分组数量: %d\n\n", enabledCount+disabledCount)

	// 查询最近更新的分组
	fmt.Println("4. 最近更新的分组:")
	recentSQL := `
	SELECT group_id, name, updated_at
	FROM provider_groups 
	ORDER BY updated_at DESC 
	LIMIT 5`

	recentRows, err := db.Query(recentSQL)
	if err != nil {
		log.Printf("查询最近更新失败: %v", err)
	} else {
		defer recentRows.Close()

		fmt.Printf("%-25s %-20s %-20s\n", "分组ID", "名称", "更新时间")
		fmt.Println(strings.Repeat("-", 70))

		for recentRows.Next() {
			var groupID, name, updatedAt string
			err = recentRows.Scan(&groupID, &name, &updatedAt)
			if err != nil {
				log.Printf("扫描最近更新数据失败: %v", err)
				continue
			}

			fmt.Printf("%-25s %-20s %-20s\n", groupID, name, updatedAt)
		}
	}

	fmt.Println("\n=== 查询完成 ===")
}
