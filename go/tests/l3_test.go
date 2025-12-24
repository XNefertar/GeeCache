package tests

import (
	"geecache"
	"testing"
)

type MockCentralCache struct {
	data map[string][]byte
}

func (m *MockCentralCache) Get(key string) ([]byte, error) {
	if v, ok := m.data[key]; ok {
		return v, nil
	}
	return nil, nil // Miss
}

func (m *MockCentralCache) Set(key string, value []byte) error {
	m.data[key] = value
	return nil
}

func (m *MockCentralCache) Delete(key string) error {
	delete(m.data, key)
	return nil
}

func TestL3Cache(t *testing.T) {
	db := map[string]string{
		"key1": "value1",
	}
	getter := geecache.GetterFunc(func(key string) ([]byte, error) {
		if v, ok := db[key]; ok {
			return []byte(v), nil
		}
		return nil, nil
	})

	g, err := geecache.NewGroup("l3_test", 2<<10, getter)
	if err != nil {
		t.Fatal(err)
	}
	l3 := &MockCentralCache{data: make(map[string][]byte)}
	g.SetCentralCache(l3)

	// 1. Get key1 (Miss L3, Hit DB, Populate L3)
	v, err := g.Get("key1")
	if err != nil || v.String() != "value1" {
		t.Fatalf("failed to get key1: %v, %s", err, v.String())
	}

	if string(l3.data["key1"]) != "value1" {
		t.Fatal("L3 cache not populated")
	}

	// 2. Clear DB, Get key1 again (Hit L3)
	delete(db, "key1")
	// Also clear local cache to force L3 lookup?
	// Group doesn't expose method to clear local cache easily for tests unless we use Remove
	g.RemoveLocal("key1")

	v, err = g.Get("key1")
	if err != nil || v.String() != "value1" {
		t.Fatalf("failed to get key1 from L3: %v, %s", err, v.String())
	}
}
