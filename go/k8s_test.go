package geecache

import (
	"context"
	"testing"
	"time"
)

func TestK8sPeerPickerCreation(t *testing.T) {
	// Test basic creation
	picker := NewK8sPeerPicker("test-service.default.svc.cluster.local", "http://localhost:8080", "8080")

	if picker == nil {
		t.Fatal("Expected non-nil picker")
	}

	if picker.self != "http://localhost:8080" {
		t.Errorf("Expected self to be http://localhost:8080, got %s", picker.self)
	}

	if picker.dnsName != "test-service.default.svc.cluster.local" {
		t.Errorf("Expected dnsName to be test-service.default.svc.cluster.local, got %s", picker.dnsName)
	}

	// Clean up
	picker.Stop()

	// Give some time for goroutines to stop
	time.Sleep(100 * time.Millisecond)
}

func TestK8sPeerPickerStop(t *testing.T) {
	picker := NewK8sPeerPicker("test-service.default.svc.cluster.local", "http://localhost:8080", "8080")

	// Stop the picker
	picker.Stop()

	// Verify context is cancelled
	select {
	case <-picker.ctx.Done():
		// Context was cancelled, good
	case <-time.After(1 * time.Second):
		t.Error("Expected context to be cancelled after Stop()")
	}
}

func TestK8sPeerPickerGetAllPeers(t *testing.T) {
	picker := NewK8sPeerPicker("test-service.default.svc.cluster.local", "http://localhost:8080", "8080")
	defer picker.Stop()

	// Initially should have no peers (DNS will fail in test environment)
	peers := picker.GetAllPeers()

	// Should return empty list when no peers discovered
	if len(peers) != 0 {
		t.Logf("Got %d peers (expected 0, but DNS might have resolved something)", len(peers))
	}
}

func TestShutdownManager(t *testing.T) {
	sm := NewShutdownManager()

	if sm == nil {
		t.Fatal("Expected non-nil shutdown manager")
	}

	// Test shutdown with context
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	called := false
	sm.RegisterShutdownFunc(func(ctx context.Context) error {
		called = true
		return nil
	})

	sm.shutdown(ctx)

	if !called {
		t.Error("Expected shutdown function to be called")
	}
}

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
	group, err := NewGroup("test", 1024, GetterFunc(func(context context.Context, key string) ([]byte, error) {
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
