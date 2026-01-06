package health

import (
	"context"
	"geecache"
	"testing"
)

func TestHealthCheck(t *testing.T) {
	hc := NewHealthCheck()

	if hc == nil {
		t.Fatal("Expected non-nil health check")
	}

	// Test initial state
	hc.mu.RLock()
	ready := hc.ready
	hc.mu.RUnlock()

	if ready {
		t.Error("Expected initial ready state to be false")
	}

	// Test SetReady
	hc.SetReady(true)

	hc.mu.RLock()
	ready = hc.ready
	hc.mu.RUnlock()

	if !ready {
		t.Error("Expected ready state to be true after SetReady(true)")
	}

	// Test RegisterGroup
	group, err := geecache.NewGroup("test", 1024, geecache.GetterFunc(func(ctx context.Context, key string) ([]byte, error) {
		return []byte("value"), nil
	}))
	if err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}

	hc.RegisterGroup(group)

	hc.mu.RLock()
	_, exists := hc.groups["test"]
	hc.mu.RUnlock()

	if !exists {
		t.Error("Expected group to be registered")
	}
}
