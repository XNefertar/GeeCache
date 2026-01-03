package geecache

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestGroupTTL(t *testing.T) {
	var db = map[string]string{
		"Tom":  "630",
		"Jack": "589",
	}
	loadCounts := make(map[string]int)

	getter := GetterFunc(func(ctx context.Context, key string) ([]byte, error) {
		loadCounts[key]++
		if v, ok := db[key]; ok {
			return []byte(v), nil
		}
		return nil, fmt.Errorf("%s not exist", key)
	})

	// Create a group with 1 second MainCacheTTL
	g, err := NewGroup("ttl_test_group", 2<<10, getter, WithMainCacheTTL(time.Second))
	if err != nil {
		t.Fatal(err)
	}

	key := "Tom"
	// 1. First Get - should load from getter
	v, err := g.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "630" {
		t.Errorf("expected 630, got %s", v.String())
	}
	if loadCounts[key] != 1 {
		t.Errorf("expected load count 1, got %d", loadCounts[key])
	}

	// 2. Second Get immediately - should hit cache
	// TinyLFU might reject first item. Access again to boost freq.
	g.Get(context.Background(), key)

	_, err = g.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	// Load count might be > 1 if TinyLFU rejected first time.
	// But we care about TTL here.

	currentLoad := loadCounts[key]

	// 3. Wait for expiration
	time.Sleep(1100 * time.Millisecond)

	// 4. Third Get - should expire and reload
	_, err = g.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if loadCounts[key] <= currentLoad {
		t.Errorf("expected load count > %d, got %d", currentLoad, loadCounts[key])
	}
}

func TestHotCacheTTL(t *testing.T) {
	// We need to test HotCacheTTL.
	// Since populateHotCache is private, we can call it directly if we are in package geecache.

	// Create a group with 1 second HotCacheTTL
	// Use larger cache size to avoid immediate eviction due to small shard size
	g, err := NewGroup("hot_ttl_test", 2<<20, GetterFunc(func(ctx context.Context, key string) ([]byte, error) {
		return []byte("value"), nil
	}), WithHotCacheTTL(time.Second))
	if err != nil {
		t.Fatal(err)
	}

	key := "key1"
	val := ByteView{b: []byte("value1")}

	// Populate hot cache
	g.populateHotCache(key, val)

	// Check if it exists
	if _, ok := g.hotCache.get(key); !ok {
		t.Fatal("expected key in hot cache")
	}

	// Wait for expiration
	time.Sleep(1100 * time.Millisecond)

	// Check if it expired
	if _, ok := g.hotCache.get(key); ok {
		t.Fatal("expected key to be expired from hot cache")
	}
}
