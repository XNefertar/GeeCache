package lru

import (
	"container/list"
	"time"
)

// Cache is a LRU cache. It is not safe for concurrent access.
type Cache struct {
	maxBytes  int64
	nbytes    int64
	ll        *list.List
	cache     map[string]*list.Element
	OnEvicted func(key string, value Value)
}

type entry struct {
	key      string
	value    Value
	expireAt time.Time
	// Redis-style Object Type (0: String, 1: List, etc.)
	dataType uint8
}

type Value interface {
	Len() int
}

// New is the Constructor of Cache
func New(maxBytes int64, onEvicted func(string, Value)) *Cache {
	return &Cache{
		maxBytes:  maxBytes,
		ll:        list.New(),
		cache:     make(map[string]*list.Element),
		OnEvicted: onEvicted,
	}
}

// Get look ups a key's value
func (c *Cache) Get(key string) (value Value, ok bool) {
	if element, ok := c.cache[key]; ok {
		kv := element.Value.(*entry)
		if !kv.expireAt.IsZero() && kv.expireAt.Before(time.Now()) {
			c.RemoveElement(element)
			return nil, false
		}
		c.ll.MoveToFront(element)
		return kv.value, true
	}
	return nil, false
}

// Remove removes the provided key from the cache.
func (c *Cache) Remove(key string) {
	if element, ok := c.cache[key]; ok {
		c.RemoveElement(element)
	}
}

func (c *Cache) RemoveElement(e *list.Element) {
	c.ll.Remove(e)
	kv := e.Value.(*entry)
	delete(c.cache, kv.key)
	c.nbytes -= int64(len(kv.key)) + int64(kv.value.Len())
	if c.OnEvicted != nil {
		c.OnEvicted(kv.key, kv.value)
	}
}

func (c *Cache) RemoveOldest() {
	element := c.ll.Back()
	if element != nil {
		c.RemoveElement(element)
	}
}

// Add adds a value to the cache.
func (c *Cache) Add(key string, value Value, ttl time.Duration) {
	var expireAt time.Time
	if ttl > 0 {
		expireAt = time.Now().Add(ttl)
	}
	if element, ok := c.cache[key]; ok {
		c.ll.MoveToFront(element)
		kv := element.Value.(*entry)
		c.nbytes += int64(value.Len()) - int64(kv.value.Len())
		kv.value = value
		kv.expireAt = expireAt
	} else {
		ele := c.ll.PushFront(&entry{key, value, expireAt, 0})
		c.cache[key] = ele
		c.nbytes += int64(len(key)) + int64(value.Len())
	}
	for c.maxBytes != 0 && c.maxBytes < c.nbytes {
		c.RemoveOldest()
	}
}

func (c *Cache) RemoveExpired(maxKeys int) {
	var count int
	for _, element := range c.cache {
		if count >= maxKeys {
			break
		}
		count++
		kv := element.Value.(*entry)
		if !kv.expireAt.IsZero() && kv.expireAt.Before(time.Now()) {
			c.RemoveElement(element)
		}
	}
}

// Len the number of items in the cache
func (c *Cache) Len() int {
	return c.ll.Len()
}
