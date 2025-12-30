package bridge

import (
	"os"
	"testing"
)

func TestLSMStore(t *testing.T) {
	path := "/tmp/test_lsm_db"
	os.RemoveAll(path)
	store, err := NewLSMStore(path)
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

func TestLSMPersistence(t *testing.T) {
	path := "/tmp/test_lsm_persistence"
	os.RemoveAll(path)

	store, err := NewLSMStore(path)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	key := "persist_key"
	value := []byte("persist_value")
	if err := store.Set(key, value); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	store.Close()

	// Reopen
	store2, err := NewLSMStore(path)
	if err != nil {
		t.Fatalf("Failed to reopen store: %v", err)
	}
	defer store2.Close()

	got, err := store2.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(got) != string(value) {
		t.Errorf("Get got %s, want %s", got, value)
	}
}

func TestBatchPut(t *testing.T) {
	path := "/tmp/test_lsm_batch"
	os.RemoveAll(path)
	store, err := NewLSMStore(path)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	entries := []BatchEntry{
		{"k1", []byte("v1")},
		{"k2", []byte("v2")},
		{"k3", []byte("v3")},
	}

	if err := store.BatchPut(entries); err != nil {
		t.Fatalf("BatchPut failed: %v", err)
	}

	for _, e := range entries {
		got, err := store.Get(e.Key)
		if err != nil {
			t.Errorf("Get %s failed: %v", e.Key, err)
		}
		if string(got) != string(e.Value) {
			t.Errorf("Get %s got %s, want %s", e.Key, got, e.Value)
		}
	}
}
