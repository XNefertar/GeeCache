package bridge

import (
	"testing"
)

func TestLSMStore(t *testing.T) {
	store, err := NewLSMStore("/tmp/test_lsm_db")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	key := "hello"
	value := []byte("world")

	// Test Set
	if err := store.Set(key, value); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Test Get
	got, err := store.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(got) != string(value) {
		t.Errorf("Get got %s, want %s", got, value)
	}

	// Test Delete
	if err := store.Delete(key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	got, err = store.Get(key)
	if err != nil {
		t.Fatalf("Get after delete failed: %v", err)
	}
	if got != nil {
		t.Errorf("Get after delete should return nil, got %s", got)
	}
}
