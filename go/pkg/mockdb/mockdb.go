package mockdb

import (
	"fmt"
	"math/rand"
	"time"
)

// MockDB simulates a database with artificial latency.
type MockDB struct {
	data    map[string][]byte
	latency time.Duration
}

// Config holds configuration for creating a MockDB.
type Config struct {
	Size      int
	ValueSize int
	Latency   time.Duration
}

// New creates a new MockDB with the specified configuration.
func New(cfg Config) *MockDB {
	db := &MockDB{
		data:    make(map[string][]byte, cfg.Size),
		latency: cfg.Latency,
	}

	// Generate a random value template to fail-fast memory allocation
	// and reuse it to speed up initialization.
	val := make([]byte, cfg.ValueSize)
	rand.Read(val)

	for i := 0; i < cfg.Size; i++ {
		key := fmt.Sprintf("key_%d", i)
		db.data[key] = val
	}
	return db
}

// Get retrieves a value by key, simulating latency.
func (db *MockDB) Get(key string) ([]byte, error) {
	if db.latency > 0 {
		time.Sleep(db.latency)
	}
	if v, ok := db.data[key]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("not found")
}
