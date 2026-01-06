package main

import (
	"context"
	"flag"
	"fmt"
	"geecache"
	"geecache/geecachehttp"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

var (
	port        = flag.String("port", getEnv("SERVER_PORT", "8080"), "Server port")
	apiMode     = flag.Bool("api", false, "Start API server")
	healthCheck = flag.Bool("health-check", false, "Run health check and exit")

	// Kubernetes service discovery
	k8sMode     = flag.Bool("k8s", true, "Enable Kubernetes service discovery")
	serviceName = flag.String("service", getEnv("K8S_SERVICE_NAME", "geecache-headless"), "K8s headless service name")
	namespace   = flag.String("namespace", getEnv("K8S_NAMESPACE", "default"), "K8s namespace")

	// Cache configuration
	cacheBytes  = flag.Int64("cache", getEnvInt64("CACHE_MEMORY_LIMIT", 128<<20), "Cache memory limit in bytes")
	cacheTTL    = flag.Int("ttl", getEnvInt("CACHE_TTL_SECONDS", 3600), "Cache TTL in seconds")
	hotCacheTTL = flag.Int("hot-ttl", getEnvInt("CACHE_HOT_TTL_SECONDS", 300), "Hot cache TTL in seconds")
)

// Mock database
var db = map[string]string{
	"Tom":   "630",
	"Jack":  "589",
	"Sam":   "567",
	"Alice": "720",
	"Bob":   "680",
}

func main() {
	flag.Parse()

	// Health check mode
	if *healthCheck {
		os.Exit(runHealthCheck())
	}

	// Create cache group with TTL
	group, err := geecache.NewGroup(
		"scores",
		*cacheBytes,
		geecache.GetterFunc(func(ctx context.Context, key string) ([]byte, error) {
			log.Printf("[SlowDB] search key %s", key)
			if v, ok := db[key]; ok {
				return []byte(v), nil
			}
			return nil, fmt.Errorf("%s not exist", key)
		}),
		geecache.WithMainCacheTTL(time.Duration(*cacheTTL)*time.Second),
		geecache.WithHotCacheTTL(time.Duration(*hotCacheTTL)*time.Second),
	)
	if err != nil {
		log.Fatalf("Failed to create group: %v", err)
	}

	// Create health check
	health := geecache.NewHealthCheck()
	health.RegisterGroup(group)

	// Setup shutdown manager
	shutdownMgr := geecache.NewShutdownManager()

	// Get pod info
	podName := getEnv("POD_NAME", "")
	podIP := getEnv("POD_IP", "127.0.0.1")

	// Server address
	addr := fmt.Sprintf("http://%s:%s", podIP, *port)

	log.Printf("Starting GeeCache server on %s", addr)
	log.Printf("Pod: %s, Namespace: %s", podName, *namespace)

	// Setup peer picker based on mode
	if *k8sMode {
		// Kubernetes mode with DNS-based discovery
		dnsName := fmt.Sprintf("%s.%s.svc.cluster.local", *serviceName, *namespace)
		log.Printf("Using Kubernetes service discovery: %s", dnsName)

		k8sPicker := geecache.NewK8sPeerPicker(dnsName, addr, *port)
		if err := group.RegisterPeers(k8sPicker); err != nil {
			log.Fatalf("Failed to register K8s peers: %v", err)
		}
		shutdownMgr.RegisterPeerPicker(k8sPicker)
	} else {
		// Traditional HTTP pool mode (for backward compatibility)
		pool := geecachehttp.NewHTTPPool(addr)
		if err := group.RegisterPeers(pool); err != nil {
			log.Fatalf("Failed to register HTTP pool: %v", err)
		}

		// Start HTTP server for peer communication
		go func() {
			srv := &http.Server{
				Addr:    ":" + *port,
				Handler: pool,
			}
			shutdownMgr.RegisterServer(srv)
			log.Printf("Cache peer server listening on :%s", *port)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("HTTP server error: %v", err)
			}
		}()
	}

	// Mark as ready after peer setup
	health.SetReady(true)

	// API server mode
	if *apiMode {
		apiPort := getEnv("API_PORT", "9999")
		startAPIServer(apiPort, group, health)
	} else {
		// Standard mode: serve cache requests and health checks
		mux := http.NewServeMux()

		// Register health endpoints
		health.RegisterHandlers(mux)

		// Register cache endpoints (if not using HTTP pool)
		if *k8sMode {
			// Create HTTP pool for serving requests
			pool := geecachehttp.NewHTTPPool(addr)
			mux.Handle("/_geecache/", pool)
		}

		srv := &http.Server{
			Addr:    ":" + *port,
			Handler: mux,
		}
		shutdownMgr.RegisterServer(srv)

		log.Printf("GeeCache is running at %s", addr)
		log.Printf("Health check available at http://localhost:%s/health", *port)

		// Start server in goroutine
		go func() {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Server error: %v", err)
			}
		}()
	}

	// Wait for shutdown signal
	shutdownMgr.WaitForShutdown()
}

func startAPIServer(apiPort string, group *geecache.Group, health *geecache.HealthCheck) {
	mux := http.NewServeMux()

	// API endpoint
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "key is required", http.StatusBadRequest)
			return
		}

		view, err := group.Get(r.Context(), key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Write(view.ByteSlice())
	})

	// Register health endpoints
	health.RegisterHandlers(mux)

	srv := &http.Server{
		Addr:    ":" + apiPort,
		Handler: mux,
	}

	log.Printf("Frontend server is running at http://localhost:%s", apiPort)
	log.Fatal(srv.ListenAndServe())
}

func runHealthCheck() int {
	// Simple health check for container HEALTHCHECK
	resp, err := http.Get("http://localhost:" + *port + "/health/live")
	if err != nil {
		log.Printf("Health check failed: %v", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return 0
	}
	return 1
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultValue
}
