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
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/syndtr/goleveldb/leveldb"
)

// Configuration flags
var (
	numKeys     = flag.Int("keys", 100000, "Total number of keys")
	valueSize   = flag.Int("valsize", 100, "Value size in bytes")
	concurrency = flag.Int("c", 1, "Number of concurrent goroutines")
	engine      = flag.String("engine", "all", "Engine to test: all, geecache, geecache_batch, leveldb, badger")
	isRandom    = flag.Bool("random", false, "Use random key access pattern for reads")
	batchSize   = flag.Int("batch", 1000, "Batch size for geecache_batch")
)

// Store interface for unified benchmarking
type Store interface {
	Set(key string, value []byte) error
	Get(key string) ([]byte, error)
	Close()
	Name() string
}

// --- GeeCache Batch Wrapper ---
type GeeCacheBatchStore struct {
	store    *bridge.LSMStore
	dir      string
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

func (s *GeeCacheBatchStore) Set(k string, v []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Copy value to avoid race conditions if caller reuses buffer
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
	store *bridge.LSMStore
	dir   string
}

func NewGeeCacheStore(dir string) (*GeeCacheStore, error) {
	s, err := bridge.NewLSMStore(dir)
	if err != nil {
		return nil, err
	}
	return &GeeCacheStore{store: s, dir: dir}, nil
}
func (s *GeeCacheStore) Set(k string, v []byte) error { return s.store.Set(k, v) }
func (s *GeeCacheStore) Get(k string) ([]byte, error) { return s.store.Get(k) }
func (s *GeeCacheStore) Close()                       { s.store.Close(); os.RemoveAll(s.dir) }
func (s *GeeCacheStore) Name() string                 { return "GeeCache LSM" }

// --- LevelDB Wrapper ---
type LevelDBStore struct {
	db  *leveldb.DB
	dir string
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
func (s *LevelDBStore) Close()       { s.db.Close(); os.RemoveAll(s.dir) }
func (s *LevelDBStore) Name() string { return "GoLevelDB" }

// --- BadgerDB Wrapper ---
type BadgerStore struct {
	db  *badger.DB
	dir string
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
func (s *BadgerStore) Close()       { s.db.Close(); os.RemoveAll(s.dir) }
func (s *BadgerStore) Name() string { return "BadgerDB" }

// --- Benchmark Logic ---

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")

func main() {
	flag.Parse()

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	fmt.Printf("Benchmark Config: Keys=%d, ValSize=%dB, Concurrency=%d, RandomRead=%v\n",
		*numKeys, *valueSize, *concurrency, *isRandom)
	fmt.Println("-----------------------------------------------------------------------")
	fmt.Printf("%-15s | %-10s | %-10s | %-10s | %-10s | %-10s\n",
		"Engine", "Phase", "OPS/sec", "P50(us)", "P99(us)", "P99.9(us)")
	fmt.Println("-----------------------------------------------------------------------")

	engines := []string{}
	if *engine == "all" {
		engines = []string{"geecache", "geecache_batch", "leveldb", "badger"}
	} else {
		engines = []string{*engine}
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
	})

	// 2. Read Phase
	runPhase(store, "Read", func(i int) error {
		kIdx := i
		if *isRandom {
			kIdx = rand.Intn(*numKeys)
		}
		key := fmt.Sprintf("key_%09d", kIdx)
		_, err := store.Get(key)
		return err
	})
}

func runPhase(store Store, phase string, op func(int) error) {
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

	fmt.Printf("%-15s | %-10s | %-10.0f | %-10d | %-10d | %-10d\n",
		store.Name(), phase, ops, p50, p99, p999)
}
