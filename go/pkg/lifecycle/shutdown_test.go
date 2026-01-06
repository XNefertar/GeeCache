package lifecycle

import (
	"context"
	"testing"
	"time"
)

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
