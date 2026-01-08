package main

import (
	"context"
	"fmt"
	"geecache"
	"geecache/geecachehttp"
	"log"
	"math/rand"
	"net/http"
	"time"
)

// 配置
const (
	DBSize   = 10000 // 1万条数据
	ItemSize = 512   // 512字节
	Port     = 9999
	DBDelay  = 10 * time.Millisecond // 模拟 10ms 的 DB 延迟
)

// MockDB 逻辑
type MockDB struct {
	data map[string][]byte
}

func NewMockDB() *MockDB {
	db := &MockDB{data: make(map[string][]byte, DBSize)}
	val := make([]byte, ItemSize)
	rand.Read(val)
	for i := 0; i < DBSize; i++ {
		key := fmt.Sprintf("key_%d", i)
		db.data[key] = val
	}
	return db
}

func (db *MockDB) Get(key string) ([]byte, error) {
	// 模拟 DB 延迟
	time.Sleep(DBDelay)
	if v, ok := db.data[key]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("not found")
}

func main() {
	// 1. 初始化 DB
	db := NewMockDB()

	// 2. 初始化 GeeCache Group
	// 缓存大小设置大一点，尽量让测试期间能命中
	geecache.NewGroup("benchmark", 50*1024*1024, geecache.GetterFunc(
		func(ctx context.Context, key string) ([]byte, error) {
			log.Printf("[DB] Loading %s...", key)
			return db.Get(key)
		}))

	// 3. 启动 HTTP 服务
	addr := fmt.Sprintf("localhost:%d", Port)
	peers := geecachehttp.NewHTTPPool(addr)

	log.Printf("GeeCache is running at %s", addr)
	log.Println("Test URL format: /_geecache/benchmark/key_N")

	// 需要将 geecachehttp 的 Handler 注册到 http 中
	// 因为 geecachehttp.HTTPPool 实现了 ServeHTTP
	err := http.ListenAndServe(addr, peers)
	if err != nil {
		log.Fatal(err)
	}
}
