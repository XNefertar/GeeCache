package tinylfu

import (
	"container/list"
	"fmt"
	"hash/fnv"
	"math/bits"
	"time"
)

type Value interface {
	Len() int
}

type entry[K comparable, V Value] struct {
	key      K
	value    V
	expireAt time.Time
	dataType uint8
}

type Cache[K comparable, V Value] struct {
	maxBytes  int64
	nbytes    int64
	ll        *list.List
	cache     map[K]*list.Element
	OnEvicted func(key K, value Value)
}

func NewCache[K comparable, V Value](maxBytes int64, onEvicted func(K, Value)) *Cache[K, V] {
	return &Cache[K, V]{
		maxBytes:  maxBytes,
		ll:        list.New(),
		cache:     make(map[K]*list.Element),
		OnEvicted: onEvicted,
	}
}

func (c *Cache[K, V]) Get(key K) (value Value, ok bool) {
	if element, ok := c.cache[key]; ok {
		kv := element.Value.(*entry[K, V])
		if !kv.expireAt.IsZero() && kv.expireAt.Before(time.Now()) {
			c.RemoveElement(element)
			return nil, false
		}
		c.ll.MoveToFront(element)
		return kv.value, true
	}
	return nil, false
}

func (c *Cache[K, V]) Remove(key K) {
	if element, ok := c.cache[key]; ok {
		c.RemoveElement(element)
	}
}

func (c *Cache[K, V]) RemoveElement(e *list.Element) {
	c.ll.Remove(e)
	kv := e.Value.(*entry[K, V])
	delete(c.cache, kv.key)
	kSize := 0
	if k, ok := any(kv.key).(string); ok {
		kSize = len(k)
	}
	c.nbytes -= int64(kSize) + int64(kv.value.Len())
	if c.OnEvicted != nil {
		c.OnEvicted(kv.key, kv.value)
	}
}

func (c *Cache[K, V]) RemoveOldest() {
	element := c.ll.Back()
	if element != nil {
		c.RemoveElement(element)
	}
}

func (c *Cache[K, V]) Add(key K, value V, ttl time.Duration) {
	var expiredAt time.Time
	if ttl > 0 {
		expiredAt = time.Now().Add(ttl)
	}
	if element, ok := c.cache[key]; ok {
		c.ll.MoveToFront(element)
		kv := element.Value.(*entry[K, V])
		c.nbytes += int64(value.Len()) - int64(kv.value.Len())
		kv.value = value
		kv.expireAt = expiredAt
	} else {
		ele := c.ll.PushFront(&entry[K, V]{
			key:      key,
			value:    value,
			expireAt: expiredAt,
			dataType: 0,
		})
		c.cache[key] = ele
		kSize := 0
		if k, ok := any(key).(string); ok {
			kSize = len(k)
		}
		c.nbytes += int64(kSize) + int64(value.Len())
	}
	for c.maxBytes != 0 && c.maxBytes < c.nbytes {
		c.RemoveOldest()
	}
}

func (c *Cache[K, V]) RemoveExpired(maxKeys int) {
	var count int
	for _, element := range c.cache {
		if count >= maxKeys {
			break
		}
		count++
		kv := element.Value.(*entry[K, Value])
		if !kv.expireAt.IsZero() && kv.expireAt.Before(time.Now()) {
			c.RemoveElement(element)
		}
	}
}

func (c *Cache[K, V]) PeekOldest() *entry[K, Value] {
	element := c.ll.Back()
	if element != nil {
		return element.Value.(*entry[K, Value])
	}
	return nil
}

func (c *Cache[K, V]) Len() int {
	return c.ll.Len()
}

type cmSketch struct {
	rows    [4][]byte
	mask    uint32
	counter int
	resetAt int
}

func newCMSketch(capacity int) *cmSketch {
	width := nextPowerOfTwo(capacity)
	rows := [4][]byte{}
	for i := range 4 {
		rows[i] = make([]byte, width)
	}
	return &cmSketch{
		rows:    rows,
		mask:    uint32(width - 1),
		resetAt: capacity * 10,
	}
}

func nextPowerOfTwo(v int) int {
	if v <= 0 {
		return 1
	}
	return 1 << bits.Len(uint(v-1))
}

func (s *cmSketch) Increment(keyString string) {
	s.counter++
	h := fnv.New32a()
	h.Write([]byte(keyString))
	bashHash := h.Sum32()

	for i := range 4 {
		idx := (bashHash + uint32(i*1337)) & s.mask
		if s.rows[i][idx] < 255 {
			s.rows[i][idx]++
		}
	}

	if s.counter >= s.resetAt {
		s.reset()
	}
}

func (s *cmSketch) reset() {
	for i := range 4 {
		for j := range s.rows[i] {
			s.rows[i][j] >>= 1
		}
	}
	s.counter = 0
}

func (s *cmSketch) Estimate(keyString string) uint8 {
	h := fnv.New32a()
	h.Write([]byte(keyString))
	bashHash := h.Sum32()

	minCount := uint8(255)
	for i := range 4 {
		idx := (bashHash + uint32(i*1337)) & s.mask
		minCount = min(minCount, s.rows[i][idx])
	}
	return minCount
}

type TinyLFUCache[K comparable, V Value] struct {
	cache  *Cache[K, Value]
	sketch *cmSketch
}

func NewTinyLFU[K comparable, V Value](capacity int, maxBytes int64, onEvicted func(K, Value)) *TinyLFUCache[K, V] {
	return &TinyLFUCache[K, V]{
		cache:  NewCache[K, Value](maxBytes, onEvicted),
		sketch: newCMSketch(capacity),
	}
}

func (c *TinyLFUCache[K, V]) getKeyStr(key K) string {
	if k, ok := any(key).(string); ok {
		return k
	}
	return fmt.Sprintf("%v", key)
}

func (c *TinyLFUCache[K, V]) Get(key K) (Value, bool) {
	keyStr := c.getKeyStr(key)
	c.sketch.Increment(keyStr)
	return c.cache.Get(key)
}

func (c *TinyLFUCache[K, V]) Put(key K, value V, ttl time.Duration) {
	keyStr := c.getKeyStr(key)
	c.sketch.Increment(keyStr)

	if _, ok := c.cache.cache[key]; ok {
		c.cache.Add(key, value, ttl)
		return
	}

	// Calculate size of new item
	kSize := 0
	if k, ok := any(key).(string); ok {
		kSize = len(k)
	}
	newSize := int64(kSize) + int64(value.Len())

	if c.cache.maxBytes > 0 && c.cache.nbytes+newSize > c.cache.maxBytes {
		victimKey := c.cache.PeekOldest()
		if victimKey != nil {
			victimKeyStr := c.getKeyStr(victimKey.key)
			candidateFreq := c.sketch.Estimate(keyStr)
			victimFreq := c.sketch.Estimate(victimKeyStr)

			if candidateFreq <= victimFreq {
				return // Reject
			}
		}
	}
	c.cache.Add(key, value, ttl)
}

func (c *TinyLFUCache[K, V]) Remove(key K) {
	c.cache.Remove(key)
}

func (c *TinyLFUCache[K, V]) RemoveExpired(maxKeys int) {
	c.cache.RemoveExpired(maxKeys)
}
