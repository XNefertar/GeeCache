package geecache

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// 模拟数据大小
const (
	KeySize   = 16
	ValueSize = 1024 // 1KB
)

// 生成随机字符串
func randomString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(rand.Intn(26) + 'a')
	}
	return string(b)
}

// 1. 基准测试：底层存储引擎的并发写入 (测试 Sharding 效果)
// 直接测试 cache 结构体，绕过 Group 和 Singleflight
func BenchmarkCoreAddParallel(b *testing.B) {
	// 初始化分片缓存，分配足够大的内存避免频繁淘汰
	c := newCache(int64(b.N * ValueSize))

	val := ByteView{b: []byte(randomString(ValueSize))}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		id := rand.Int()
		counter := 0
		for pb.Next() {
			// 使用随机 Key 触发分片哈希
			key := fmt.Sprintf("key-%d-%d", id, counter)
			c.add(key, val, time.Minute)
			counter++
		}
	})
}

// 2. 基准测试：底层存储引擎的并发读取 (测试 Sharding + RWMutex 效果)
func BenchmarkCoreGetParallel(b *testing.B) {
	c := newCache(int64(1024 * 1024 * 1024)) // 1GB
	val := ByteView{b: []byte(randomString(ValueSize))}

	// 预填充数据
	keys := make([]string, 10000)
	for i := 0; i < 10000; i++ {
		keys[i] = fmt.Sprintf("key-%d", i)
		c.add(keys[i], val, time.Minute)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := keys[i%10000]
			_, _ = c.get(key)
			i++
		}
	})
}

// 3. 基准测试：Group 层面的完整流程 (Hit 场景)
// 包含 Singleflight 检查、Group 路由等开销
func BenchmarkGroupGetHitParallel(b *testing.B) {
	// 模拟 Getter
	getter := GetterFunc(func(key string) ([]byte, error) {
		return []byte(randomString(ValueSize)), nil
	})

	g, _ := NewGroup("bench_hit", 1024*1024*1024, getter)

	// 预填充
	keys := make([]string, 10000)
	for i := 0; i < 10000; i++ {
		keys[i] = fmt.Sprintf("key-%d", i)
		// 触发一次加载
		_, _ = g.Get(keys[i])
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := keys[i%10000]
			_, _ = g.Get(key)
			i++
		}
	})
}

// 4. 基准测试：Group 层面的完整流程 (Miss 场景 - 模拟缓存击穿保护)
// 这将测试 Singleflight 的合并效果
func BenchmarkGroupGetMissParallel(b *testing.B) {
	getter := GetterFunc(func(key string) ([]byte, error) {
		// 模拟慢查询
		time.Sleep(time.Millisecond)
		// 🆕 返回错误，防止结果被缓存
		// 这样每次请求都会走 Miss -> Singleflight -> Load 流程
		// 从而真实测试 Singleflight 的合并等待性能
		return nil, fmt.Errorf("no cache")
	})

	g, _ := NewGroup("bench_miss", 1024*1024*1024, getter)

	key := "hot-key-miss"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// 所有并发请求都打同一个 Key，测试 Singleflight
			_, _ = g.Get(key)
		}
	})
}
