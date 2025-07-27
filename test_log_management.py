#!/usr/bin/env python3
"""
测试日志管理功能的脚本
"""

import requests
import json
import time

BASE_URL = "http://localhost:8080"

def test_log_management():
    """测试日志管理功能"""
    
    print("=== 测试日志管理功能 ===")
    
    # 1. 获取日志列表
    print("\n1. 获取日志列表...")
    response = requests.get(f"{BASE_URL}/admin/logs")
    if response.status_code == 200:
        data = response.json()
        if data.get('success'):
            logs = data.get('logs', [])
            print(f"   成功获取 {len(logs)} 条日志")
            if logs:
                log_ids = [log['id'] for log in logs[:3]]  # 取前3条用于测试
                print(f"   测试用日志ID: {log_ids}")
            else:
                print("   没有日志数据，无法测试删除功能")
                return
        else:
            print(f"   获取日志失败: {data.get('error')}")
            return
    else:
        print(f"   请求失败: {response.status_code}")
        return
    
    # 2. 测试批量删除（如果有日志的话）
    if logs and len(logs) >= 2:
        print("\n2. 测试批量删除...")
        test_ids = [logs[0]['id'], logs[1]['id']]
        delete_data = {"ids": test_ids}
        
        response = requests.delete(
            f"{BASE_URL}/admin/logs/batch",
            headers={"Content-Type": "application/json"},
            data=json.dumps(delete_data)
        )
        
        if response.status_code == 200:
            data = response.json()
            if data.get('success'):
                print(f"   成功删除 {data.get('deleted_count')} 条日志")
            else:
                print(f"   删除失败: {data.get('error')}")
        else:
            print(f"   删除请求失败: {response.status_code}")
    
    # 3. 测试导出功能
    print("\n3. 测试导出功能...")
    response = requests.get(f"{BASE_URL}/admin/logs/export?format=csv")
    
    if response.status_code == 200:
        if response.headers.get('content-type') == 'text/csv':
            print(f"   成功导出CSV文件，大小: {len(response.content)} 字节")
        else:
            print(f"   导出格式不正确: {response.headers.get('content-type')}")
    else:
        print(f"   导出请求失败: {response.status_code}")
    
    # 4. 测试JSON导出
    print("\n4. 测试JSON导出...")
    response = requests.get(f"{BASE_URL}/admin/logs/export?format=json")
    
    if response.status_code == 200:
        try:
            data = response.json()
            if data.get('success'):
                print(f"   成功导出JSON，包含 {data.get('count')} 条记录")
            else:
                print(f"   JSON导出失败: {data.get('error')}")
        except json.JSONDecodeError:
            print("   JSON格式错误")
    else:
        print(f"   JSON导出请求失败: {response.status_code}")
    
    print("\n=== 测试完成 ===")

if __name__ == "__main__":
    try:
        test_log_management()
    except requests.exceptions.ConnectionError:
        print("错误: 无法连接到服务器，请确保服务器正在运行在 http://localhost:8080")
    except Exception as e:
        print(f"测试过程中发生错误: {e}")
