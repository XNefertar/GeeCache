package singleflight

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDo(t *testing.T) {
	var g Group
	var n int32 = 0 // 原子计数器，记录 fn 被调用的次数

	// 模拟一个耗时的数据库查询函数
	// 每次调用，计数器 +1
	fn := func() (interface{}, error) {
		atomic.AddInt32(&n, 1)
		time.Sleep(100 * time.Millisecond) // 模拟慢查询，耗时 100ms
		return "bar", nil
	}

	// 模拟并发请求
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 1000 个协程同时请求同一个 Key "key"
			val, err := g.Do("key", fn)

			// 验证结果必须正确
			if err != nil {
				t.Errorf("Do error: %v", err)
			}
			if val.(string) != "bar" {
				t.Errorf("Do got = %v; want %v", val, "bar")
			}
		}()
	}

	wg.Wait()

	// 核心验证：
	// 虽然有 1000 个并发请求，但 fn 应该只被调用了 1 次！
	if n != 1 {
		t.Errorf("fn expected be called 1 time, but called %d times", n)
	}
}
