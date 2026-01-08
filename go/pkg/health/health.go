package health

import (
	"encoding/json"
	"geecache"
	"net/http"
	"sync"
	"time"
)

// HealthCheck provides health checking endpoints for Kubernetes probes
type HealthCheck struct {
	ready     bool
	mu        sync.RWMutex
	groups    map[string]*geecache.Group
	startTime time.Time
}

// HealthStatus represents the health status response
type HealthStatus struct {
	Status    string            `json:"status"`
	Timestamp string            `json:"timestamp"`
	Uptime    string            `json:"uptime"`
	Groups    []string          `json:"groups,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
}

// NewHealthCheck creates a new health check handler
func NewHealthCheck() *HealthCheck {
	return &HealthCheck{
		ready:     false,
		groups:    make(map[string]*geecache.Group),
		startTime: time.Now(),
	}
}

// SetReady marks the service as ready to serve traffic
func (h *HealthCheck) SetReady(ready bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ready = ready
}

// RegisterGroup registers a cache group for health monitoring
func (h *HealthCheck) RegisterGroup(g *geecache.Group) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.groups[g.Name()] = g
}

// LivenessHandler handles Kubernetes liveness probe (is the app running?)
func (h *HealthCheck) LivenessHandler(w http.ResponseWriter, r *http.Request) {
	status := HealthStatus{
		Status:    "alive",
		Timestamp: time.Now().Format(time.RFC3339),
		Uptime:    time.Since(h.startTime).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}

// ReadinessHandler handles Kubernetes readiness probe (ready to serve traffic?)
func (h *HealthCheck) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	ready := h.ready
	groups := make([]string, 0, len(h.groups))
	for name := range h.groups {
		groups = append(groups, name)
	}
	h.mu.RUnlock()

	status := HealthStatus{
		Timestamp: time.Now().Format(time.RFC3339),
		Uptime:    time.Since(h.startTime).String(),
		Groups:    groups,
	}

	if ready {
		status.Status = "ready"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	} else {
		status.Status = "not_ready"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(status)
}

// HealthHandler provides a combined health check endpoint
func (h *HealthCheck) HealthHandler(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	ready := h.ready
	groups := make([]string, 0, len(h.groups))
	details := make(map[string]string)

	for name, g := range h.groups {
		groups = append(groups, name)
		// Add basic group info
		if g.HasPeers() {
			details[name+"_peers"] = "configured"
		}
	}
	h.mu.RUnlock()

	status := HealthStatus{
		Timestamp: time.Now().Format(time.RFC3339),
		Uptime:    time.Since(h.startTime).String(),
		Groups:    groups,
		Details:   details,
	}

	if ready {
		status.Status = "healthy"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	} else {
		status.Status = "unhealthy"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(status)
}

// RegisterHandlers registers all health check endpoints on a mux
func (h *HealthCheck) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.HealthHandler)
	mux.HandleFunc("/health/live", h.LivenessHandler)
	mux.HandleFunc("/health/ready", h.ReadinessHandler)
}
