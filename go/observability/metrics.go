package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// CacheRequestsTotal counts the total number of cache requests
	CacheRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "geecache_requests_total",
			Help: "Total number of cache requests",
		},
		[]string{"group", "result"}, // result: hit, miss
	)

	// CacheRequestDuration measures the latency of cache requests
	CacheRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "geecache_request_duration_seconds",
			Help:    "Duration of cache requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"group", "method"}, // method: get, set
	)

	// CacheSize measures the number of items in the cache
	CacheSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "geecache_cache_size",
			Help: "Number of items in the cache",
		},
		[]string{"group", "type"}, // type: main, hot
	)

	// CacheBytes measures the memory usage of the cache
	CacheBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "geecache_cache_bytes",
			Help: "Memory usage of the cache in bytes",
		},
		[]string{"group", "type"}, // type: main, hot
	)

	// CacheEvictionsTotal counts the total number of evicted keys
	CacheEvictionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "geecache_evictions_total",
			Help: "Total number of evicted keys",
		},
		[]string{"group", "type"}, // type: main, hot
	)

	// CacheCapacityBytes measures the capacity of the cache in bytes
	CacheCapacityBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "geecache_cache_capacity_bytes",
			Help: "Capacity of the cache in bytes",
		},
		[]string{"group", "type"}, // type: main, hot
	)
)
