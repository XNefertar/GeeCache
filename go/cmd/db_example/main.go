package main

import (
	"context"
	"fmt"
	"geecache"
	"log"
	"time"
)

// 模拟一个数据库 (可以是 MySQL/PostgreSQL 等)
var mockMysqlDB = map[string]string{
	"user:1001": `{"name":"Tom", "age":20, "score":630}`,
	"user:1002": `{"name":"Jack", "age":25, "score":589}`,
	"user:1003": `{"name":"Sam", "age":22, "score":567}`,
}

// simulateDBQuery 模拟缓慢的数据库查询
func simulateDBQuery(key string) ([]byte, error) {
	log.Printf("[DB] 正在从数据库查询 key: %s (耗时 500ms...)", key)
	time.Sleep(500 * time.Millisecond) // 模拟网络延迟和查询耗时
	if v, ok := mockMysqlDB[key]; ok {
		return []byte(v), nil
	}
	return nil, fmt.Errorf("record not found")
}

func main() {
	// 1. 初始化 GeeCache Group
	// 这里的 GetterFunc 是连接你的数据库和缓存的桥梁
	// 类似于 "Read-Through" 模式：如果缓存没命中，自动回调这个函数去数据库查
	groupName := "user_profiles"
	group, err := geecache.NewGroup(groupName, 2<<20, geecache.GetterFunc(
		func(ctx context.Context, key string) ([]byte, error) {
			return simulateDBQuery(key)
		}))
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// 2. 交互式演示
	keysToQuery := []string{"user:1001", "user:1001", "user:1002", "user:1001"}

	fmt.Println("=== 开始模拟数据库接入测试 ===")

	for i, key := range keysToQuery {
		fmt.Printf("\n--- 请求 #%d: 获取 %s ---\n", i+1, key)
		start := time.Now()

		// 调用 Group.Get，逻辑被封装在 GeeCache 内部
		val, err := group.Get(ctx, key)

		duration := time.Since(start)

		if err != nil {
			fmt.Printf("❌ 获取失败: %v\n", err)
		} else {
			// 简单的判断：如果很快就是缓存命中，如果慢就是回源
			source := "🔥 缓存命中 (Cache Hit)"
			if duration > 100*time.Millisecond {
				source = "🐢 数据库回源 (DB Fetch)"
			}
			fmt.Printf("✅ 获取成功: %s\n", val)
			fmt.Printf("⏱️  耗时: %v  => %s\n", duration, source)
		}
	}

	// 3. 统计信息
	// 注意：这里需要根据你项目中实际暴露的统计接口来调整，或者查看 GetGroup 内部状态
	fmt.Println("\n=== 测试结束 ===")
	// 如果 geecache 包有暴露统计信息可以打印
}
