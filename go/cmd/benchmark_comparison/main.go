package main

import (
	"fmt"
	"geecache/bridge"
	"log"
	"os"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	numOps    = 100000
	valueSize = 100
	keyPrefix = "key_"
)

func main() {
	fmt.Printf("Starting Benchmark Comparison (Ops: %d, ValueSize: %dB)\n", numOps, valueSize)
	fmt.Println("------------------------------------------------")

	// 1. GeeCache LSM (C++ Bridge)
	runGeeCacheLSM()

	// 2. GoLevelDB
	runGoLevelDB()

	// 3. BadgerDB
	runBadgerDB()
}

func runGeeCacheLSM() {
	dir := "/tmp/bench_geecache_lsm"
	os.RemoveAll(dir)
	defer os.RemoveAll(dir)

	store, err := bridge.NewLSMStore(dir)
	if err != nil {
		log.Fatalf("Failed to open GeeCache LSM: %v", err)
	}
	defer store.Close()

	fmt.Println("Running GeeCache LSM Benchmark...")
	start := time.Now()
	val := make([]byte, valueSize)
	for i := 0; i < numOps; i++ {
		key := fmt.Sprintf("%s%d", keyPrefix, i)
		if err := store.Set(key, val); err != nil {
			log.Fatalf("GeeCache Set failed: %v", err)
		}
	}
	duration := time.Since(start)
	fmt.Printf("  Write: %v, %.2f ops/sec\n", duration, float64(numOps)/duration.Seconds())

	start = time.Now()
	for i := 0; i < numOps; i++ {
		key := fmt.Sprintf("%s%d", keyPrefix, i)
		if _, err := store.Get(key); err != nil {
			log.Fatalf("GeeCache Get failed: %v", err)
		}
	}
	duration = time.Since(start)
	fmt.Printf("  Read:  %v, %.2f ops/sec\n", duration, float64(numOps)/duration.Seconds())
	fmt.Println("------------------------------------------------")
}

func runGoLevelDB() {
	dir := "/tmp/bench_goleveldb"
	os.RemoveAll(dir)
	defer os.RemoveAll(dir)

	db, err := leveldb.OpenFile(dir, nil)
	if err != nil {
		log.Fatalf("Failed to open GoLevelDB: %v", err)
	}
	defer db.Close()

	fmt.Println("Running GoLevelDB Benchmark...")
	start := time.Now()
	val := make([]byte, valueSize)
	for i := 0; i < numOps; i++ {
		key := fmt.Sprintf("%s%d", keyPrefix, i)
		if err := db.Put([]byte(key), val, nil); err != nil {
			log.Fatalf("GoLevelDB Put failed: %v", err)
		}
	}
	duration := time.Since(start)
	fmt.Printf("  Write: %v, %.2f ops/sec\n", duration, float64(numOps)/duration.Seconds())

	start = time.Now()
	for i := 0; i < numOps; i++ {
		key := fmt.Sprintf("%s%d", keyPrefix, i)
		if _, err := db.Get([]byte(key), nil); err != nil {
			log.Fatalf("GoLevelDB Get failed: %v", err)
		}
	}
	duration = time.Since(start)
	fmt.Printf("  Read:  %v, %.2f ops/sec\n", duration, float64(numOps)/duration.Seconds())
	fmt.Println("------------------------------------------------")
}

func runBadgerDB() {
	dir := "/tmp/bench_badger"
	os.RemoveAll(dir)
	defer os.RemoveAll(dir)

	opts := badger.DefaultOptions(dir)
	opts.Logger = nil       // Disable logging
	opts.SyncWrites = false // Disable sync for fair comparison
	db, err := badger.Open(opts)
	if err != nil {
		log.Fatalf("Failed to open BadgerDB: %v", err)
	}
	defer db.Close()

	fmt.Println("Running BadgerDB Benchmark...")
	start := time.Now()
	val := make([]byte, valueSize)

	for i := 0; i < numOps; i++ {
		key := fmt.Sprintf("%s%d", keyPrefix, i)
		err := db.Update(func(txn *badger.Txn) error {
			return txn.Set([]byte(key), val)
		})
		if err != nil {
			log.Fatalf("BadgerDB Set failed: %v", err)
		}
	}
	duration := time.Since(start)
	fmt.Printf("  Write: %v, %.2f ops/sec\n", duration, float64(numOps)/duration.Seconds())

	start = time.Now()
	for i := 0; i < numOps; i++ {
		key := fmt.Sprintf("%s%d", keyPrefix, i)
		err := db.View(func(txn *badger.Txn) error {
			_, err := txn.Get([]byte(key))
			return err
		})
		if err != nil {
			log.Fatalf("BadgerDB Get failed: %v", err)
		}
	}
	duration = time.Since(start)
	fmt.Printf("  Read:  %v, %.2f ops/sec\n", duration, float64(numOps)/duration.Seconds())
	fmt.Println("------------------------------------------------")
}
