package tests

import (
	"fmt"
	"geecache"
	"geecache/bridge"
	"math/rand"
	"os"
	"testing"
)

// BenchmarkL3_Integration 测试 L3 (LSM) 集成性能
// 场景：L1/L2 缓存很小，大量请求穿透到 L3
func BenchmarkL3_Integration(b *testing.B) {
	dbPath := "/tmp/geecache_bench_l3"
	os.RemoveAll(dbPath)
	defer os.RemoveAll(dbPath)

	lsm, err := bridge.NewLSMStore(dbPath)
	if err != nil {
		b.Fatal(err)
	}

	// Mock DB getter (should not be reached if L3 hit)
	getter := geecache.GetterFunc(func(key string) ([]byte, error) {
		return nil, fmt.Errorf("db miss")
	})

	// 设置极小的 L1/L2 缓存 (1KB)，强制请求落到 L3
	g := geecache.NewGroup("bench_l3", 1024, getter)
	g.SetCentralCache(lsm)

	// 预填充 L3
	val := []byte("value-data-1234567890") // ~20 bytes
	numKeys := 10000
	keys := make([]string, numKeys)
	for i := 0; i < numKeys; i++ {
		keys[i] = fmt.Sprintf("key-%d", i)
		lsm.Set(keys[i], val)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		// 使用局部随机源避免全局锁竞争，模拟完全随机的访问模式
		r := rand.New(rand.NewSource(int64(rand.Int())))
		for pb.Next() {
			key := keys[r.Intn(numKeys)]
			// 这将频繁触发 L3 读取
			view, err := g.Get(key)
			if err != nil {
				b.Error(err)
			}
			if len(view.ByteSlice()) == 0 {
				b.Errorf("key %s not found", key)
			}
		}
	})
}
