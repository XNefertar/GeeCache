package tests

import (
	"geecache"
	"geecache/bridge"
	"os"
	"testing"
)

func TestLSMIntegration(t *testing.T) {
	path := "/tmp/geecache_lsm_integration"
	os.RemoveAll(path)

	lsmStore, err := bridge.NewLSMStore(path)
	if err != nil {
		t.Fatalf("Failed to create LSM store: %v", err)
	}
	defer lsmStore.Close()

	db := map[string]string{
		"key1": "value1",
	}
	getter := geecache.GetterFunc(func(key string) ([]byte, error) {
		if v, ok := db[key]; ok {
			return []byte(v), nil
		}
		return nil, nil
	})

	g := geecache.NewGroup("lsm_integration", 2<<10, getter)
	g.SetCentralCache(lsmStore)

	// 1. Get key1 (Miss L3, Hit DB, Populate L3)
	v, err := g.Get("key1")
	if err != nil || v.String() != "value1" {
		t.Fatalf("failed to get key1: %v, %s", err, v.String())
	}

	// Verify it's in LSM
	val, err := lsmStore.Get("key1")
	if err != nil {
		t.Fatalf("LSM Get failed: %v", err)
	}
	if string(val) != "value1" {
		t.Errorf("LSM should have key1, got %s", val)
	}

	// 2. Create a new group that shares the same LSM store, but has no DB source
	// This simulates a restart where L1/L2 are empty, but L3 has data.
	g2 := geecache.NewGroup("lsm_integration_2", 2<<10, geecache.GetterFunc(func(key string) ([]byte, error) {
		return nil, nil // DB miss
	}))
	g2.SetCentralCache(lsmStore)

	v2, err := g2.Get("key1")
	if err != nil || v2.String() != "value1" {
		t.Fatalf("failed to get key1 from L3: %v, %s", err, v2.String())
	}

	// 3. Test Write (Set) with Write-Through
	g.RegisterSetter(geecache.SetterFunc(func(key string, value []byte) error {
		// Mock DB set
		return nil
	}))

	err = g.Set("key2", []byte("value2"), geecache.StrategyWriteThrough)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Verify L3 has it
	val2, err := lsmStore.Get("key2")
	if err != nil {
		t.Fatalf("LSM Get key2 failed: %v", err)
	}
	if string(val2) != "value2" {
		t.Errorf("LSM should have key2, got %s", val2)
	}
}
