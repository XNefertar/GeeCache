package geecache

import (
	"geecache/lru"
	"hash/fnv"
	"sync"
	"time"
)

// cacheShard wraps an LRU cache and adds concurrency control.
// Corresponds to a Redis database instance (but sharded).
type cacheShard struct {
	mu         sync.Mutex
	lru        *lru.Cache
	cacheBytes int64
}

func (c *cacheShard) add(key string, value ByteView, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru == nil {
		c.lru = lru.New(c.cacheBytes, nil)
	}
	c.lru.Add(key, value, ttl)
}

func (c *cacheShard) get(key string) (value ByteView, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru == nil {
		return
	}
	if v, ok := c.lru.Get(key); ok {
		return v.(ByteView), ok
	}
	return
}

func (c *cacheShard) removeExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru != nil {
		// Periodic expiration: Randomly sample keys to expire
		c.lru.RemoveExpired(20)
	}
}

// cache implements a sharded cache to reduce lock contention
// This simulates Redis's high performance by parallelizing access across shards
type cache struct {
	shards     []*cacheShard
	shardCount uint64
}

// newCache creates a new sharded cache
func newCache(cacheBytes int64) *cache {
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

func (c *cache) removeExpired() {
	for _, shard := range c.shards {
		shard.removeExpired()
	}
}
