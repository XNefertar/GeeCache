package lifecycle

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// ShutdownManager handles graceful shutdown of GeeCache servers
type ShutdownManager struct {
	servers       []*http.Server
	peerPickers   []interface{ Stop() }
	mu            sync.Mutex
	shutdownFuncs []func(context.Context) error
	timeout       time.Duration
}

// NewShutdownManager creates a new shutdown manager
func NewShutdownManager() *ShutdownManager {
	// Default timeout, can be overridden via environment variable
	timeout := 30 * time.Second
	if timeoutStr := os.Getenv("SHUTDOWN_TIMEOUT_SECONDS"); timeoutStr != "" {
		if t, err := strconv.Atoi(timeoutStr); err == nil {
			timeout = time.Duration(t) * time.Second
		}
	}

	return &ShutdownManager{
		servers:       make([]*http.Server, 0),
		peerPickers:   make([]interface{ Stop() }, 0),
		shutdownFuncs: make([]func(context.Context) error, 0),
		timeout:       timeout,
	}
}

// RegisterServer registers an HTTP server for graceful shutdown
func (sm *ShutdownManager) RegisterServer(server *http.Server) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.servers = append(sm.servers, server)
}

// RegisterPeerPicker registers a peer picker (like K8sPeerPicker) for shutdown
func (sm *ShutdownManager) RegisterPeerPicker(picker interface{ Stop() }) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.peerPickers = append(sm.peerPickers, picker)
}

// RegisterShutdownFunc registers a custom shutdown function
func (sm *ShutdownManager) RegisterShutdownFunc(fn func(context.Context) error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.shutdownFuncs = append(sm.shutdownFuncs, fn)
}

// WaitForShutdown blocks until shutdown signal is received
func (sm *ShutdownManager) WaitForShutdown() {
	// Create channel to listen for interrupt or terminate signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// Wait for signal
	sig := <-sigChan
	log.Printf("[ShutdownManager] Received signal: %v, starting graceful shutdown...", sig)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown in order: custom functions, peer pickers, then servers
	sm.shutdown(ctx)

	log.Println("[ShutdownManager] Graceful shutdown completed")
}

// shutdown performs the actual shutdown
func (sm *ShutdownManager) shutdown(ctx context.Context) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Use configured timeout
	shutdownCtx, cancel := context.WithTimeout(ctx, sm.timeout)
	defer cancel()

	// 1. Execute custom shutdown functions
	for i, fn := range sm.shutdownFuncs {
		if err := fn(shutdownCtx); err != nil {
			log.Printf("[ShutdownManager] Custom shutdown function %d failed: %v", i, err)
		}
	}

	// 2. Stop peer pickers (stop discovering new peers)
	for _, picker := range sm.peerPickers {
		picker.Stop()
	}

	// 3. Shutdown HTTP servers (drain connections)
	var wg sync.WaitGroup
	for _, server := range sm.servers {
		wg.Add(1)
		go func(srv *http.Server) {
			defer wg.Done()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				log.Printf("[ShutdownManager] Server shutdown error: %v", err)
			}
		}(server)
	}

	// Wait for all servers to finish with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("[ShutdownManager] All servers shutdown successfully")
	case <-shutdownCtx.Done():
		log.Println("[ShutdownManager] Shutdown timeout exceeded, forcing exit")
	}
}
