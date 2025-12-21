package geecache

import (
	"fmt"
	"testing"
	"time"
)

func TestWriteStrategies(t *testing.T) {
	var db = make(map[string]string)
	var getter = GetterFunc(func(key string) ([]byte, error) {
		if v, ok := db[key]; ok {
			return []byte(v), nil
		}
		return nil, fmt.Errorf("key not found")
	})
	var setter = SetterFunc(func(key string, value []byte) error {
		db[key] = string(value)
		return nil
	})

	g := NewGroup("write_test", 2<<20, getter)
	g.RegisterSetter(setter)

	// Test Write-Through
	key1 := "key1"
	val1 := "val1"
	err := g.Set(key1, []byte(val1), StrategyWriteThrough)
	if err != nil {
		t.Fatal(err)
	}

	// Check DB
	if db[key1] != val1 {
		t.Fatalf("Write-Through failed to update DB. got %s, want %s", db[key1], val1)
	}
	// Check Cache
	if v, err := g.Get(key1); err != nil || string(v.ByteSlice()) != val1 {
		t.Fatalf("Write-Through failed to update Cache. got %v, want %s", v, val1)
	}

	// Test Write-Back
	key2 := "key2"
	val2 := "val2"
	err = g.Set(key2, []byte(val2), StrategyWriteBack)
	if err != nil {
		t.Fatal(err)
	}

	// Check Cache immediately
	if v, err := g.Get(key2); err != nil || string(v.ByteSlice()) != val2 {
		t.Fatalf("Write-Back failed to update Cache immediately. got %v, want %s", v, val2)
	}

	// Check DB (might not be updated yet, but likely is fast enough in test)
	// We wait a bit to be sure
	time.Sleep(100 * time.Millisecond)
	if db[key2] != val2 {
		t.Fatalf("Write-Back failed to update DB eventually. got %s, want %s", db[key2], val2)
	}
}
