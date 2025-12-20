package geecache

import (
	"geecache/lru"
	"sync"
	"time"
)

type cache struct {
	mu         sync.Mutex
	lru        *lru.Cache
	cacheBytes int64
}

func (c *cache) add(key string, value ByteView, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru == nil {
		c.lru = lru.New(c.cacheBytes, nil)
	}
	c.lru.Add(key, value, ttl)
}

func (c *cache) get(key string) (value ByteView, ok bool) {
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

func (c *cache) removeExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru != nil {
		// 优化：每次随机检查 20 个 Key (参考 Redis 策略)
		// 避免全量遍历导致的长时间锁阻塞 (Stop-The-World)
		c.lru.RemoveExpired(20)
	}
}
