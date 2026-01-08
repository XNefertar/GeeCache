package main

import (
	"context"
	"fmt"
	"geecache"
	"geecache/pkg/mockdb"
	"log"
	"math/rand"
	"time"
)

// 配置参数
const (
	DBSize           = 100000                // 模拟 10 万条数据
	ValueSize        = 1024                  // 每条数据 1KB
	DBLatency        = 50 * time.Millisecond // 模拟数据库查询耗时 50ms (这是一个常见的慢查询时间)
	TestRequestCount = 1000                  // 测试请求总数
)

func main() {
	// 1. 初始化数据库
	fmt.Printf("1. [Init] 正在构建包含 %d 条数据的模拟数据库 (单条数据 %d 字节)...\n", DBSize, ValueSize)
	db := mockdb.New(mockdb.Config{
		Size:      DBSize,
		ValueSize: ValueSize,
		Latency:   DBLatency,
	})
	fmt.Println("   [Init] 数据库构建完成")

	// 2. 初始化缓存
	// 缓存大小设为 50MB，足以容纳一部分热点数据
	fmt.Println("2. [Init] 初始化 GeeCache (MaxBytes: 50MB)...")
	group, err := geecache.NewGroup("perf_test_group", 50*1024*1024, geecache.GetterFunc(
		func(ctx context.Context, key string) ([]byte, error) {
			return db.Get(key)
		}))
	if err != nil {
		log.Fatal(err)
	}

	// 3. 生成随机测试集
	// 我们生成 1000 个随机 Key，用于接下来的两轮测试
	// 为了保证这一千个请求是“有且只有 1000 个不同的Key”，我们接下来重放这 1000 个 Key
	fmt.Printf("3. [Init] 生成 %d 个随机测试请求 Key...\n", TestRequestCount)
	testKeys := make([]string, TestRequestCount)
	for i := 0; i < TestRequestCount; i++ {
		testKeys[i] = fmt.Sprintf("key_%d", rand.Intn(DBSize))
	}

	ctx := context.Background()

	// ----------------------------------------------------------------
	// 阶段一：冷启动测试 (模拟缓存穿透/未命中)
	// ----------------------------------------------------------------
	fmt.Println("\n========================================")
	fmt.Println("🔥 阶段一：冷启动测试 (预期：全回源，性能低)")
	fmt.Println("   说明：缓存为空，所有请求都需要去数据库查，带 50ms 延迟")
	fmt.Println("========================================")

	start := time.Now()
	for _, key := range testKeys {
		_, err := group.Get(ctx, key)
		if err != nil {
			log.Printf("Error getting %s: %v", key, err)
		}
	}
	durationCold := time.Since(start)

	avgCold := durationCold / time.Duration(TestRequestCount)
	qpsCold := float64(TestRequestCount) / durationCold.Seconds()

	fmt.Printf(">> 耗时总计: %v\n", durationCold)
	fmt.Printf(">> 平均响应: %v / req\n", avgCold)
	fmt.Printf(">> QPS:      %.2f\n", qpsCold)

	// ----------------------------------------------------------------
	// 阶段二：热缓存测试 (模拟命中)
	// ----------------------------------------------------------------
	fmt.Println("\n========================================")
	fmt.Println("🚀 阶段二：热缓存测试 (预期：全命中，性能高)")
	fmt.Println("   说明：请求相同的 %d 个 Key，预期全部命中内存缓存", TestRequestCount)
	fmt.Println("========================================")

	start = time.Now()
	for _, key := range testKeys {
		_, err := group.Get(ctx, key)
		if err != nil {
			log.Printf("Error getting %s: %v", key, err)
		}
	}
	durationHot := time.Since(start)

	avgHot := durationHot / time.Duration(TestRequestCount)
	qpsHot := float64(TestRequestCount) / durationHot.Seconds()

	fmt.Printf(">> 耗时总计: %v\n", durationHot)
	fmt.Printf(">> 平均响应: %v / req\n", avgHot)
	fmt.Printf(">> QPS:      %.2f\n", qpsHot)

	// ----------------------------------------------------------------
	// 结果对比
	// ----------------------------------------------------------------
	fmt.Println("\n========================================")
	fmt.Println("📊 最终结果量化报告")
	fmt.Println("========================================")

	speedup := float64(durationCold) / float64(durationHot)

	fmt.Printf("1. 数据库延迟模拟: %v\n", DBLatency)
	fmt.Printf("2. 缓存命中提升:   %.2f 倍\n", speedup)
	fmt.Printf("3. 延迟降低:       %v -> %v\n", avgCold, avgHot)
	fmt.Println("========================================")
}
