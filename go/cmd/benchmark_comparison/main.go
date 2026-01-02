package main

import (
	"flag"
	"fmt"
	"geecache/bridge"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/syndtr/goleveldb/leveldb"
)

// Configuration flags
var (
	numKeys      = flag.Int("keys", 100000, "Total number of keys")
	valueSize    = flag.Int("valsize", 100, "Value size in bytes")
	concurrency  = flag.Int("c", 1, "Number of concurrent goroutines")
	engine       = flag.String("engine", "all", "Engine to test: all, geecache, geecache_batch, leveldb, badger")
	isRandom     = flag.Bool("random", false, "Use random key access pattern for reads")
	batchSize    = flag.Int("batch", 1000, "Batch size for geecache_batch")
	missingRatio = flag.Float64("missing", 0.0, "Ratio of non-existent keys (0.0 - 1.0)")
	cpuprofile   = flag.String("cpuprofile", "", "write cpu profile to file")
)

// Store interface for unified benchmarking
type Store interface {
	Name() string
	Close()
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
	BatchGet(keys []string) error
}

// --- GeeCache Batch Wrapper ---
type GeeCacheBatchStore struct {
	dir      string
	store    *bridge.LSMStore
	batchBuf []bridge.BatchEntry
	mu       sync.Mutex
}

func NewGeeCacheBatchStore(dir string) (*GeeCacheBatchStore, error) {
	s, err := bridge.NewLSMStore(dir)
	if err != nil {
		return nil, err
	}
	return &GeeCacheBatchStore{store: s, dir: dir, batchBuf: make([]bridge.BatchEntry, 0, *batchSize)}, nil
}

func (s *GeeCacheBatchStore) BatchGet(keys []string) error {
	_, err := s.store.BatchGet(keys)
	return err
}

func (s *GeeCacheBatchStore) Set(k string, v []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	valCopy := make([]byte, len(v))
	copy(valCopy, v)
	s.batchBuf = append(s.batchBuf, bridge.BatchEntry{Key: k, Value: valCopy})

	if len(s.batchBuf) >= *batchSize {
		err := s.store.BatchPut(s.batchBuf)
		s.batchBuf = s.batchBuf[:0] // Reset buffer
		return err
	}
	return nil
}

func (s *GeeCacheBatchStore) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.batchBuf) > 0 {
		err := s.store.BatchPut(s.batchBuf)
		s.batchBuf = s.batchBuf[:0]
		return err
	}
	return nil
}

func (s *GeeCacheBatchStore) Get(k string) ([]byte, error) { return s.store.Get(k) }
func (s *GeeCacheBatchStore) Close() {
	s.Flush()
	s.store.Close()
	os.RemoveAll(s.dir)
}
func (s *GeeCacheBatchStore) Name() string { return fmt.Sprintf("GeeCache Batch(%d)", *batchSize) }

// --- GeeCache Wrapper ---
type GeeCacheStore struct {
	dir   string
	store *bridge.LSMStore
}

func NewGeeCacheStore(dir string) (*GeeCacheStore, error) {
	s, err := bridge.NewLSMStore(dir)
	if err != nil {
		return nil, err
	}
	return &GeeCacheStore{store: s, dir: dir}, nil
}

func (s *GeeCacheStore) Name() string                 { return "GeeCache LSM" }
func (s *GeeCacheStore) Close()                       { s.store.Close(); os.RemoveAll(s.dir) }
func (s *GeeCacheStore) Get(k string) ([]byte, error) { return s.store.Get(k) }
func (s *GeeCacheStore) Set(k string, v []byte) error { return s.store.Set(k, v) }
func (s *GeeCacheStore) BatchGet(keys []string) error {
	for _, k := range keys {
		_, err := s.store.Get(k)
		if err != nil {
			return err
		}
	}
	return nil
}

// --- LevelDB Wrapper ---
type LevelDBStore struct {
	dir string
	db  *leveldb.DB
}

func NewLevelDBStore(dir string) (*LevelDBStore, error) {
	db, err := leveldb.OpenFile(dir, nil)
	if err != nil {
		return nil, err
	}
	return &LevelDBStore{db: db, dir: dir}, nil
}
func (s *LevelDBStore) Set(k string, v []byte) error { return s.db.Put([]byte(k), v, nil) }
func (s *LevelDBStore) Get(k string) ([]byte, error) {
	_, err := s.db.Get([]byte(k), nil)
	return nil, err
}
func (s *LevelDBStore) BatchGet(keys []string) error {
	for _, k := range keys {
		_, err := s.db.Get([]byte(k), nil)
		if err != nil && err != leveldb.ErrNotFound {
			return err
		}
	}
	return nil
}
func (s *LevelDBStore) Close()       { s.db.Close(); os.RemoveAll(s.dir) }
func (s *LevelDBStore) Name() string { return "GoLevelDB" }

// --- BadgerDB Wrapper ---
type BadgerStore struct {
	dir string
	db  *badger.DB
}

func NewBadgerStore(dir string) (*BadgerStore, error) {
	opts := badger.DefaultOptions(dir)
	opts.Logger = nil
	opts.SyncWrites = false // Fair comparison with async LSM
	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}
	return &BadgerStore{db: db, dir: dir}, nil
}
func (s *BadgerStore) Set(k string, v []byte) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(k), v)
	})
}
func (s *BadgerStore) Get(k string) ([]byte, error) {
	var val []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(k))
		if err != nil {
			return err
		}
		return item.Value(func(v []byte) error {
			val = v // Copy value
			return nil
		})
	})
	return val, err
}

func (s *BadgerStore) BatchGet(keys []string) error {
	return s.db.View(func(txn *badger.Txn) error {
		for _, k := range keys {
			_, err := txn.Get([]byte(k))
			if err != nil && err != badger.ErrKeyNotFound {
				return err
			}
		}
		return nil
	})
}

func (s *BadgerStore) Close()       { s.db.Close(); os.RemoveAll(s.dir) }
func (s *BadgerStore) Name() string { return "BadgerDB" }

// --- Benchmark Logic ---

func main() {
	flag.Parse()
	rand.Seed(time.Now().UnixNano())

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	fmt.Printf("Benchmark Config: Keys=%d, ValSize=%dB, Concurrency=%d, RandomRead=%v, MissingRatio=%.2f\n",
		*numKeys, *valueSize, *concurrency, *isRandom, *missingRatio)
	fmt.Println("-----------------------------------------------------------------------")
	fmt.Printf("%-20s | %-10s | %-10s | %-10s | %-10s | %-10s\n",
		"Engine", "Phase", "OPS/sec", "P50(us)", "P99(us)", "P99.9(us)")
	fmt.Println("-----------------------------------------------------------------------")

	var engines []string
	if *engine == "all" {
		engines = []string{"geecache_batch", "leveldb", "badger"}
	} else {
		engines = strings.Split(*engine, ",")
	}

	for _, e := range engines {
		runBenchmarkForEngine(e)
	}
}

func runBenchmarkForEngine(engineName string) {
	var store Store
	var err error
	dir := filepath.Join(os.TempDir(), "bench_"+engineName+"_"+fmt.Sprint(time.Now().UnixNano()))
	os.RemoveAll(dir)

	switch engineName {
	case "geecache":
		store, err = NewGeeCacheStore(dir)
	case "geecache_batch":
		store, err = NewGeeCacheBatchStore(dir)
	case "leveldb":
		store, err = NewLevelDBStore(dir)
	case "badger":
		store, err = NewBadgerStore(dir)
	default:
		log.Fatalf("Unknown engine: %s", engineName)
	}

	if err != nil {
		log.Fatalf("Failed to init %s: %v", engineName, err)
	}
	defer store.Close()

	// 1. Write Phase
	runPhase(store, "Write", func(i int) error {
		key := fmt.Sprintf("key_%09d", i)
		val := make([]byte, *valueSize)
		return store.Set(key, val)
	}, nil)

	// 2. Read Phase
	keyGen := func(i int) string {
		var kIdx int
		if *isRandom {
			kIdx = rand.Intn(*numKeys)
		} else {
			kIdx = i
		}

		// Determine if we should simulate a missing key
		isMissing := false
		if *missingRatio > 0 {
			if *missingRatio >= 1.0 {
				isMissing = true
			} else {
				isMissing = rand.Float64() < *missingRatio
			}
		}

		if isMissing {
			// Generate a key that definitely doesn't exist
			kIdx += *numKeys
		}

		return fmt.Sprintf("key_%09d", kIdx)
	}

	runPhase(store, "Read", func(i int) error {
		key := keyGen(i)
		_, err := store.Get(key)
		return err
	}, keyGen)
}

func runPhase(store Store, phase string, op func(int) error, keyGen func(int) string) {
	var wg sync.WaitGroup
	latencies := make([]int64, *numKeys) // Store latency in microseconds
	start := time.Now()
	opsPerThread := *numKeys / *concurrency

	// Progress counter
	var completedOps int64

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(threadID int) {
			defer wg.Done()
			base := threadID * opsPerThread

			if phase == "Read" && *batchSize > 1 && keyGen != nil {
				keys := make([]string, 0, *batchSize)
				for j := 0; j < opsPerThread; j++ {
					idx := base + j
					keys = append(keys, keyGen(idx))

					if len(keys) >= *batchSize {
						t0 := time.Now()
						err := store.BatchGet(keys)
						if err != nil {
							log.Printf("Error in %s: %v", phase, err)
						}
						lat := time.Since(t0).Microseconds()

						// Record latency for all keys in batch
						for k := 0; k < len(keys); k++ {
							slot := idx - len(keys) + 1 + k
							if slot < len(latencies) {
								latencies[slot] = lat
							}
						}
						atomic.AddInt64(&completedOps, int64(len(keys)))
						keys = keys[:0]
					}
				}
				if len(keys) > 0 {
					store.BatchGet(keys)
					atomic.AddInt64(&completedOps, int64(len(keys)))
				}
			} else {
				for j := 0; j < opsPerThread; j++ {
					idx := base + j
					t0 := time.Now()
					if err := op(idx); err != nil {
						// Ignore not found errors in random read
						if phase == "Write" {
							log.Printf("Error in %s: %v", phase, err)
						}
					}
					lat := time.Since(t0).Microseconds()
					if idx < len(latencies) {
						latencies[idx] = lat
					}
					atomic.AddInt64(&completedOps, 1)
				}
			}
		}(i)
	}
	wg.Wait()
	duration := time.Since(start)
	ops := float64(*numKeys) / duration.Seconds()

	// Calculate Percentiles
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[int(float64(len(latencies))*0.50)]
	p99 := latencies[int(float64(len(latencies))*0.99)]
	p999 := latencies[int(float64(len(latencies))*0.999)]

	fmt.Printf("%-20s | %-10s | %-10.0f | %-10d | %-10d | %-10d\n",
		store.Name(), phase, ops, p50, p99, p999)
}
