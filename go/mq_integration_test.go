package geecache

import (
	"geecache/mq"
	"testing"
	"time"
)

func TestMQIntegration(t *testing.T) {
	// 1. Setup Group
	getter := GetterFunc(func(key string) ([]byte, error) {
		return []byte("value"), nil
	})
	g := NewGroup("mq_test", 2<<20, getter)

	// 2. Setup MQ
	q := mq.NewMemoryMQ()
	err := g.RegisterMQ(q, "mq_test_topic")
	if err != nil {
		t.Fatalf("failed to register mq: %v", err)
	}

	// 3. Populate Cache
	key := "key1"
	_, err = g.Get(key) // Load into L2
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := g.mainCache.get(key); !ok {
		t.Fatal("cache should be populated")
	}

	// 4. Simulate DB Update & Publish Invalidation
	// (In real world, this happens in the application layer)
	err = q.Publish("mq_test_topic", key)
	if err != nil {
		t.Fatal(err)
	}

	// 5. Wait for async processing
	time.Sleep(200 * time.Millisecond)

	// 6. Verify Cache is Empty
	if _, ok := g.mainCache.get(key); ok {
		t.Fatal("cache should be removed after MQ message")
	}
}
