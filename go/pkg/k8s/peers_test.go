package k8s

import (
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
