package keymanager

import (
	"testing"
	"time"

	"turnsapi/internal"
)

// TestMultiGroupKeyManagerDeduplication 测试多分组密钥管理器的去重功能
func TestMultiGroupKeyManagerDeduplication(t *testing.T) {
	// 创建测试配置
	config := &internal.Config{
		UserGroups: map[string]*internal.UserGroup{
			"group1": {
				Name:         "Test Group 1",
				ProviderType: "openai",
				BaseURL:      "https://api.openai.com/v1",
				Enabled:      true,
				APIKeys:      []string{"key1", "key2", "key3"},
				Timeout:      30 * time.Second,
				MaxRetries:   3,
			},
			"group2": {
				Name:         "Test Group 2",
				ProviderType: "anthropic",
				BaseURL:      "https://api.anthropic.com/v1",
				Enabled:      true,
				APIKeys:      []string{"key4", "key5"},
				Timeout:      30 * time.Second,
				MaxRetries:   3,
			},
		},
	}

	// 创建多分组密钥管理器
	mgkm := NewMultiGroupKeyManager(config)
	defer mgkm.Close()

	// 测试1: 检查单个密钥重复
	t.Run("CheckSingleKeyDuplication", func(t *testing.T) {
		// 检查已存在的密钥
		duplicates := mgkm.CheckSingleKeyDuplication("key1")
		if len(duplicates) == 0 {
			t.Error("Expected to find duplicates for key1, but found none")
		}
		if len(duplicates) != 1 || duplicates[0] != "group1" {
			t.Errorf("Expected key1 to be found in group1, got: %v", duplicates)
		}

		// 检查不存在的密钥
		duplicates = mgkm.CheckSingleKeyDuplication("nonexistent")
		if len(duplicates) != 0 {
			t.Errorf("Expected no duplicates for nonexistent key, got: %v", duplicates)
		}

		// 检查空密钥
		duplicates = mgkm.CheckSingleKeyDuplication("")
		if len(duplicates) != 0 {
			t.Errorf("Expected no duplicates for empty key, got: %v", duplicates)
		}
	})

	// 测试2: 检查多个密钥重复
	t.Run("CheckKeyDuplication", func(t *testing.T) {
		testKeys := []string{"key1", "key4", "newkey1", "key2", "newkey2"}
		duplicates := mgkm.CheckKeyDuplication(testKeys)

		expectedDuplicates := map[string][]string{
			"key1": {"group1"},
			"key4": {"group2"},
			"key2": {"group1"},
		}

		if len(duplicates) != len(expectedDuplicates) {
			t.Errorf("Expected %d duplicates, got %d", len(expectedDuplicates), len(duplicates))
		}

		for key, expectedGroups := range expectedDuplicates {
			if groups, exists := duplicates[key]; !exists {
				t.Errorf("Expected to find duplicates for key %s", key)
			} else if len(groups) != len(expectedGroups) || groups[0] != expectedGroups[0] {
				t.Errorf("Expected key %s to be in groups %v, got %v", key, expectedGroups, groups)
			}
		}
	})

	// 测试3: 验证分组密钥（仅检查分组内重复）
	t.Run("ValidateKeysForGroup", func(t *testing.T) {
		// 测试新分组，应该允许跨分组重复
		testKeys := []string{"key1", "newkey1", "key4", "newkey2", "newkey1"} // newkey1内部重复
		validKeys, groupDuplicates, internalDuplicates := mgkm.ValidateKeysForGroup("group3", testKeys)

		// 检查有效密钥 - key1和key4现在允许跨分组重复，只有newkey1的第二次出现被过滤
		expectedValidKeys := []string{"key1", "newkey1", "key4", "newkey2"}
		if len(validKeys) != len(expectedValidKeys) {
			t.Errorf("Expected %d valid keys, got %d: %v", len(expectedValidKeys), len(validKeys), validKeys)
		}

		// 检查分组内重复 - 新分组没有现有密钥，所以应该为0
		if len(groupDuplicates) != 0 {
			t.Errorf("Expected 0 group duplicates, got %d: %v", len(groupDuplicates), groupDuplicates)
		}

		// 检查内部重复 - newkey1重复
		if len(internalDuplicates) != 1 {
			t.Errorf("Expected 1 internal duplicate, got %d: %v", len(internalDuplicates), internalDuplicates)
		}
		
		// 测试现有分组内重复
		testKeysExisting := []string{"key1", "newkey3"} // key1在group1中已存在
		validKeys, groupDuplicates, internalDuplicates = mgkm.ValidateKeysForGroup("group1", testKeysExisting)
		
		// key1应该被标记为分组内重复
		if len(validKeys) != 1 || validKeys[0] != "newkey3" {
			t.Errorf("Expected 1 valid key (newkey3), got: %v", validKeys)
		}
		
		if len(groupDuplicates) != 1 {
			t.Errorf("Expected 1 group duplicate, got %d: %v", len(groupDuplicates), groupDuplicates)
		}
	})

	// 测试4: 获取所有分组的密钥
	t.Run("GetAllKeysAcrossGroups", func(t *testing.T) {
		allKeys := mgkm.GetAllKeysAcrossGroups()

		expectedKeys := map[string][]string{
			"key1": {"group1"},
			"key2": {"group1"},
			"key3": {"group1"},
			"key4": {"group2"},
			"key5": {"group2"},
		}

		if len(allKeys) != len(expectedKeys) {
			t.Errorf("Expected %d unique keys, got %d", len(expectedKeys), len(allKeys))
		}

		for key, expectedGroups := range expectedKeys {
			if groups, exists := allKeys[key]; !exists {
				t.Errorf("Expected to find key %s", key)
			} else if len(groups) != len(expectedGroups) || groups[0] != expectedGroups[0] {
				t.Errorf("Expected key %s to be in groups %v, got %v", key, expectedGroups, groups)
			}
		}
	})
}

// TestKeyManagerDeduplication 测试单个密钥管理器的去重功能
func TestKeyManagerDeduplication(t *testing.T) {
	// 创建密钥管理器
	keys := []string{"key1", "key2", "key3"}
	km := NewKeyManager(keys, "round_robin", 0, "")
	defer km.Close()

	// 测试1: 检查密钥重复
	t.Run("CheckKeyDuplication", func(t *testing.T) {
		// 检查已存在的密钥
		duplicates := km.CheckKeyDuplication([]string{"key1", "newkey", "key2"})
		expected := []string{"key1", "key2"}
		
		if len(duplicates) != len(expected) {
			t.Errorf("Expected %d duplicates, got %d: %v", len(expected), len(duplicates), duplicates)
		}

		for i, key := range expected {
			if i >= len(duplicates) || duplicates[i] != key {
				t.Errorf("Expected duplicate %s at position %d, got %v", key, i, duplicates)
			}
		}

		// 检查不存在的密钥
		duplicates = km.CheckKeyDuplication([]string{"newkey1", "newkey2"})
		if len(duplicates) != 0 {
			t.Errorf("Expected no duplicates for new keys, got: %v", duplicates)
		}
	})

	// 测试2: 添加重复密钥应该失败
	t.Run("AddDuplicateKey", func(t *testing.T) {
		err := km.AddKey("key1", "Duplicate Key", "This should fail", []string{})
		if err == nil {
			t.Error("Expected error when adding duplicate key, but got none")
		}
		if err.Error() != "API密钥已存在" {
			t.Errorf("Expected specific error message, got: %s", err.Error())
		}
	})

	// 测试3: 添加新密钥应该成功
	t.Run("AddNewKey", func(t *testing.T) {
		err := km.AddKey("newkey", "New Key", "This should succeed", []string{})
		if err != nil {
			t.Errorf("Expected no error when adding new key, got: %s", err.Error())
		}

		// 验证密钥已添加
		duplicates := km.CheckKeyDuplication([]string{"newkey"})
		if len(duplicates) != 1 {
			t.Error("Expected new key to be found after adding")
		}
	})
}

// TestBatchKeyDeduplication 测试批量密钥去重功能
func TestBatchKeyDeduplication(t *testing.T) {
	// 创建密钥管理器
	keys := []string{"existing1", "existing2"}
	km := NewKeyManager(keys, "round_robin", 0, "")
	defer km.Close()

	t.Run("AddKeysInBatchWithDuplicates", func(t *testing.T) {
		// 测试包含各种重复情况的批量添加
		testKeys := []string{
			"existing1",  // 与现有密钥重复
			"new1",       // 新密钥
			"new2",       // 新密钥
			"existing2",  // 与现有密钥重复
			"new1",       // 内部重复
			"new3",       // 新密钥
			"",           // 空密钥
			"new3",       // 内部重复
		}

		addedCount, errors, err := km.AddKeysInBatch(testKeys)
		if err != nil {
			t.Errorf("Unexpected error: %s", err.Error())
		}

		// 应该只添加了 new1, new2, new3 (3个密钥)
		expectedAdded := 3
		if addedCount != expectedAdded {
			t.Errorf("Expected %d keys to be added, got %d", expectedAdded, addedCount)
		}

		// 应该有错误信息
		if len(errors) == 0 {
			t.Error("Expected some errors due to duplicates, but got none")
		}

		// 验证新密钥确实被添加了
		duplicates := km.CheckKeyDuplication([]string{"new1", "new2", "new3"})
		if len(duplicates) != 3 {
			t.Errorf("Expected all 3 new keys to be found, got %d: %v", len(duplicates), duplicates)
		}
	})
}