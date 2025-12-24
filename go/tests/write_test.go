package tests

import (
	"fmt"
	"geecache"
	"sync"
	"testing"
	"time"
)

func TestWriteStrategies(t *testing.T) {
	var mu sync.Mutex
	var db = make(map[string]string)
	var getter = geecache.GetterFunc(func(key string) ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		if v, ok := db[key]; ok {
			return []byte(v), nil
		}
		return nil, fmt.Errorf("key not found")
	})
	var setter = geecache.SetterFunc(func(key string, value []byte) error {
		mu.Lock()
		defer mu.Unlock()
		db[key] = string(value)
		return nil
	})

	g, err := geecache.NewGroup("write_test", 2<<20, getter)
	if err != nil {
		t.Fatal(err)
	}
	g.RegisterSetter(setter)

	// Test Write-Through
	key1 := "key1"
	val1 := "val1"
	err = g.Set(key1, []byte(val1), geecache.StrategyWriteThrough)
	if err != nil {
		t.Fatal(err)
	}

	// Check DB
	mu.Lock()
	got1 := db[key1]
	mu.Unlock()
	if got1 != val1 {
		t.Fatalf("Write-Through failed to update DB. got %s, want %s", got1, val1)
	}
	// Check Cache
	if v, err := g.Get(key1); err != nil || string(v.ByteSlice()) != val1 {
		t.Fatalf("Write-Through failed to update Cache. got %v, want %s", v, val1)
	}

	// Test Write-Back
	key2 := "key2"
	val2 := "val2"
	err = g.Set(key2, []byte(val2), geecache.StrategyWriteBack)
	if err != nil {
		t.Fatal(err)
	}

	// Check Cache immediately
	if v, err := g.Get(key2); err != nil || string(v.ByteSlice()) != val2 {
		t.Fatalf("Write-Back failed to update Cache immediately. got %v, want %s", v, val2)
	}

	// Wait for async DB update
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	got2 := db[key2]
	mu.Unlock()
	if got2 != val2 {
		t.Fatalf("Write-Back failed to update DB asynchronously. got %s, want %s", got2, val2)
	}
}
