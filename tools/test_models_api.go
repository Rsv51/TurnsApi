package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("用法: go run tools/test_models_api.go <测试类型>\n测试类型: all | single")
	}

	testType := os.Args[1]
	baseURL := "http://localhost:8080"
	
	var url string
	switch testType {
	case "all":
		url = baseURL + "/admin/models"
		fmt.Println("=== 测试所有提供商模型API ===")
	case "single":
		url = baseURL + "/admin/models?provider_group=moda"
		fmt.Println("=== 测试单个提供商模型API (moda) ===")
	default:
		log.Fatal("无效的测试类型，请使用 'all' 或 'single'")
	}

	fmt.Printf("请求URL: %s\n\n", url)

	// 创建HTTP请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatalf("创建请求失败: %v", err)
	}

	// 添加基本认证
	req.SetBasicAuth("admin", "turnsapi123")

	// 发送HTTP请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("读取响应失败: %v", err)
	}

	fmt.Printf("HTTP状态码: %d\n", resp.StatusCode)
	fmt.Printf("响应长度: %d 字节\n\n", len(body))

	if resp.StatusCode != 200 {
		fmt.Printf("错误响应:\n%s\n", string(body))
		return
	}

	// 解析JSON响应
	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		log.Fatalf("解析JSON失败: %v", err)
	}

	// 美化输出JSON
	prettyJSON, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		log.Fatalf("格式化JSON失败: %v", err)
	}

	fmt.Println("响应结构:")
	fmt.Println(string(prettyJSON))

	// 分析响应结构
	fmt.Println("\n=== 响应结构分析 ===")
	
	if object, ok := response["object"]; ok {
		fmt.Printf("object: %v\n", object)
	}

	if data, ok := response["data"]; ok {
		switch d := data.(type) {
		case map[string]interface{}:
			fmt.Printf("data类型: 对象 (包含 %d 个分组)\n", len(d))
			for groupID, groupData := range d {
				fmt.Printf("  分组ID: %s\n", groupID)
				if group, ok := groupData.(map[string]interface{}); ok {
					if name, exists := group["group_name"]; exists {
						fmt.Printf("    分组名称: %v\n", name)
					}
					if providerType, exists := group["provider_type"]; exists {
						fmt.Printf("    提供商类型: %v\n", providerType)
					}
					if models, exists := group["models"]; exists {
						if modelsMap, ok := models.(map[string]interface{}); ok {
							if modelsData, exists := modelsMap["data"]; exists {
								if modelsList, ok := modelsData.([]interface{}); ok {
									fmt.Printf("    模型数量: %d\n", len(modelsList))
									if len(modelsList) > 0 {
										fmt.Printf("    示例模型: ")
										if firstModel, ok := modelsList[0].(map[string]interface{}); ok {
											if id, exists := firstModel["id"]; exists {
												fmt.Printf("%v", id)
											}
											if ownedBy, exists := firstModel["owned_by"]; exists {
												fmt.Printf(" (owned_by: %v)", ownedBy)
											}
										}
										fmt.Println()
									}
								}
							}
						}
					}
				}
				fmt.Println()
			}
		case []interface{}:
			fmt.Printf("data类型: 数组 (包含 %d 个项目)\n", len(d))
		default:
			fmt.Printf("data类型: %T\n", d)
		}
	}

	fmt.Println("=== 测试完成 ===")
}
