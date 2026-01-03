package geecache

import (
	"geecache/tinylfu"
	"hash/fnv"
	"sync"
	"time"
)

// cacheShard wraps a TinyLFU cache and adds concurrency control.
// Corresponds to a Redis database instance (but sharded).
type cacheShard struct {
	mu         sync.Mutex
	tinylfu    *tinylfu.WTinyLFUCache[string, ByteView]
	cacheBytes int64
	onEvicted  func(key string, value ByteView)
}

func (c *cacheShard) add(key string, value ByteView, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tinylfu == nil {
		// Estimate capacity for sketch. Assuming average item size 1KB.
		capacity := int(c.cacheBytes / 1024)
		if capacity < 100 {
			capacity = 100
		}
		c.tinylfu = tinylfu.NewWTinyLFUCache[string, ByteView](capacity, c.cacheBytes, c.onEvicted)
	}
	c.tinylfu.Put(key, value, ttl)
}

func (c *cacheShard) get(key string) (value ByteView, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tinylfu == nil {
		return
	}
	if v, ok := c.tinylfu.Get(key); ok {
		return v, ok
	}
	return
}

func (c *cacheShard) remove(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tinylfu != nil {
		c.tinylfu.Remove(key)
	}
}

func (c *cacheShard) removeExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tinylfu != nil {
		// Periodic expiration: Randomly sample keys to expire
		c.tinylfu.RemoveExpired(20)
	}
}

// cache implements a sharded cache to reduce lock contention
// This simulates Redis's high performance by parallelizing access across shards
type cache struct {
	shards     []*cacheShard
	shardCount uint64
}

// newCache creates a new sharded cache
func newCache(cacheBytes int64, onEvicted func(key string, value ByteView)) *cache {
	shardCount := uint64(256)
	c := &cache{
		shards:     make([]*cacheShard, shardCount),
		shardCount: shardCount,
	}
	// Distribute capacity across shards
	shardBytes := cacheBytes / int64(shardCount)
	// Ensure at least some bytes per shard if total is small
	if shardBytes == 0 {
		shardBytes = 1024
	}

	for i := uint64(0); i < shardCount; i++ {
		c.shards[i] = &cacheShard{
			cacheBytes: shardBytes,
			onEvicted:  onEvicted,
		}
	}
	return c
}

func (c *cache) getShard(key string) *cacheShard {
	h := fnv.New32a()
	h.Write([]byte(key))
	idx := uint64(h.Sum32()) % c.shardCount
	return c.shards[idx]
}

func (c *cache) add(key string, value ByteView, ttl time.Duration) {
	c.getShard(key).add(key, value, ttl)
}

func (c *cache) get(key string) (ByteView, bool) {
	return c.getShard(key).get(key)
}

func (c *cache) remove(key string) {
	c.getShard(key).remove(key)
}

func (c *cache) removeExpired() {
	for _, shard := range c.shards {
		shard.removeExpired()
	}
}
