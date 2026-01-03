package tests

import (
	"fmt"
	"geecache/lru"
	"geecache/tinylfu"
	"math/rand"
	"testing"
	"time"
)

// String implements Value interface
type String string

func (s String) Len() int {
	return len(s)
}

// CacheWrapper unifies the interface for testing
type CacheWrapper interface {
	Get(key string) bool
	Add(key string, val String)
	Name() string
}

// LRU Wrapper
type LRUWrapper struct {
	c *lru.Cache
}

func NewLRUWrapper(maxBytes int64) *LRUWrapper {
	return &LRUWrapper{c: lru.New(maxBytes, nil)}
}

func (w *LRUWrapper) Get(key string) bool {
	_, ok := w.c.Get(key)
	return ok
}

func (w *LRUWrapper) Add(key string, val String) {
	w.c.Add(key, val, 0)
}

func (w *LRUWrapper) Name() string { return "LRU" }

// TinyLFU Wrapper
type TinyLFUWrapper struct {
	c *tinylfu.TinyLFUCache[string, String]
}

func NewTinyLFUWrapper(capacity int, maxBytes int64) *TinyLFUWrapper {
	return &TinyLFUWrapper{c: tinylfu.NewTinyLFU[string, String](capacity, maxBytes, nil)}
}

func (w *TinyLFUWrapper) Get(key string) bool {
	_, ok := w.c.Get(key)
	return ok
}

func (w *TinyLFUWrapper) Add(key string, val String) {
	w.c.Put(key, val, 0)
}

func (w *TinyLFUWrapper) Name() string { return "TinyLFU" }

// Zipfian generator
type ZipfGenerator struct {
	zipf *rand.Zipf
}

func NewZipfGenerator(s float64, v float64, n uint64) *ZipfGenerator {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	z := rand.NewZipf(r, s, v, n)
	return &ZipfGenerator{zipf: z}
}

func (z *ZipfGenerator) Next() uint64 {
	return z.zipf.Uint64()
}

func runTrace(t *testing.T, cache CacheWrapper, keys []string) float64 {
	hits := 0
	for _, key := range keys {
		if cache.Get(key) {
			hits++
		} else {
			cache.Add(key, String(".")) // Value size 1
		}
	}
	return float64(hits) / float64(len(keys))
}

// TestHitRatioComparison compares LRU and TinyLFU hit ratios
func TestHitRatioComparison(t *testing.T) {
	// Configuration
	// Item size: Key(6 bytes) + Value(1 byte) = 7 bytes (approx, depends on implementation details)
	// In lru.go: nbytes += int64(len(key)) + int64(value.Len())
	// Key "000000" is 6 bytes. Value "." is 1 byte. Total 7.
	itemSize := int64(7)
	cacheCapacityItems := 100
	maxBytes := int64(cacheCapacityItems) * itemSize

	// 1. Zipfian Distribution (Standard Workload)
	t.Run("Zipfian Workload", func(t *testing.T) {
		universeSize := uint64(1000)
		requestCount := 10000
		// s=1.01 (close to 1) is standard Zipf.
		zipf := NewZipfGenerator(1.01, 1.0, universeSize)

		keys := make([]string, requestCount)
		for i := 0; i < requestCount; i++ {
			keys[i] = fmt.Sprintf("%06d", zipf.Next())
		}

		lruCache := NewLRUWrapper(maxBytes)
		lfuCache := NewTinyLFUWrapper(cacheCapacityItems*10, maxBytes) // Sketch larger than cache

		lruHit := runTrace(t, lruCache, keys)
		lfuHit := runTrace(t, lfuCache, keys)

		t.Logf("Zipfian - LRU Hit Ratio: %.2f%%", lruHit*100)
		t.Logf("Zipfian - TinyLFU Hit Ratio: %.2f%%", lfuHit*100)

		if lfuHit < lruHit {
			t.Log("Warning: TinyLFU performed worse than LRU in Zipfian workload")
		} else {
			diff := (lfuHit - lruHit) * 100
			t.Logf("TinyLFU Improvement: +%.2f%%", diff)
		}
	})

	// 2. Scan Resistance (Pollution)
	t.Run("Scan Resistance", func(t *testing.T) {
		// Scenario:
		// 1. Warm up with 50 hot keys (repeated 10 times each)
		// 2. Scan 200 random keys (once each) - This would flush a 100-item LRU
		// 3. Access the 50 hot keys again

		hotKeys := make([]string, 50)
		for i := 0; i < 50; i++ {
			hotKeys[i] = fmt.Sprintf("hot-%03d", i)
		}

		scanKeys := make([]string, 200)
		for i := 0; i < 200; i++ {
			scanKeys[i] = fmt.Sprintf("scan-%03d", i)
		}

		// Build trace
		var trace []string

		// Phase 1: Warmup
		for r := 0; r < 10; r++ {
			for _, k := range hotKeys {
				trace = append(trace, k)
			}
		}

		// Phase 2: Scan
		trace = append(trace, scanKeys...)

		// Phase 3: Check Hot Keys
		// We only care about hits in this phase for the metric, but runTrace calculates global.
		// Let's just run the whole trace.
		// Actually, to see if they survived, we should look at the end.
		// But runTrace is fine, we expect higher overall hit ratio if hot keys survived.

		// Let's add Phase 3 explicitly
		for _, k := range hotKeys {
			trace = append(trace, k)
		}

		lruCache := NewLRUWrapper(maxBytes)
		lfuCache := NewTinyLFUWrapper(cacheCapacityItems*10, maxBytes)

		lruHit := runTrace(t, lruCache, trace)
		lfuHit := runTrace(t, lfuCache, trace)

		t.Logf("Scan - LRU Hit Ratio: %.2f%%", lruHit*100)
		t.Logf("Scan - TinyLFU Hit Ratio: %.2f%%", lfuHit*100)

		diff := (lfuHit - lruHit) * 100
		t.Logf("TinyLFU Improvement: +%.2f%%", diff)

		if diff < 2.0 {
			t.Log("TinyLFU did not show significant scan resistance improvement")
		}
	})
}
