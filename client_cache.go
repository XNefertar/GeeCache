package geecache

import (
	"sync"
	"time"
)

// ClientCache is a simple in-memory cache for the client application (L0)
type ClientCache struct {
	mu    sync.RWMutex
	items map[string]clientItem
	ttl   time.Duration
}

type clientItem struct {
	value      []byte
	expiration time.Time
}

func NewClientCache(ttl time.Duration) *ClientCache {
	return &ClientCache{
		items: make(map[string]clientItem),
		ttl:   ttl,
	}
}

func (c *ClientCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	item, ok := c.items[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(item.expiration) {
		return nil, false
	}
	return item.value, true
}

func (c *ClientCache) Set(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = clientItem{
		value:      value,
		expiration: time.Now().Add(c.ttl),
	}
}

// ClientWrapper wraps a Group with an L0 cache
type ClientWrapper struct {
	group *Group
	l0    *ClientCache
}

func NewClientWrapper(group *Group, l0TTL time.Duration) *ClientWrapper {
	return &ClientWrapper{
		group: group,
		l0:    NewClientCache(l0TTL),
	}
}

func (c *ClientWrapper) Get(key string) (ByteView, error) {
	// 1. Check L0
	if v, ok := c.l0.Get(key); ok {
		return ByteView{b: cloneBytes(v)}, nil
	}

	// 2. Check GeeCache (L1 + L2 + L3)
	v, err := c.group.Get(key)
	if err != nil {
		return ByteView{}, err
	}

	// 3. Populate L0
	c.l0.Set(key, v.ByteSlice())
	return v, nil
}
