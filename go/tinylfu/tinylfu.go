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
	OnEvicted func(key K, value V)
}

func NewCache[K comparable, V Value](maxBytes int64, onEvicted func(K, V)) *Cache[K, V] {
	return &Cache[K, V]{
		maxBytes:  maxBytes,
		ll:        list.New(),
		cache:     make(map[K]*list.Element),
		OnEvicted: onEvicted,
	}
}

func (c *Cache[K, V]) Get(key K) (value V, ok bool) {
	if element, ok := c.cache[key]; ok {
		kv := element.Value.(*entry[K, V])
		if !kv.expireAt.IsZero() && kv.expireAt.Before(time.Now()) {
			c.RemoveElement(element)
			var zero V
			return zero, false
		}
		c.ll.MoveToFront(element)
		return kv.value, true
	}
	var zero V
	return zero, false
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

func (c *Cache[K, V]) Contains(key K) bool {
	_, ok := c.cache[key]
	return ok
}

func (c *Cache[K, V]) RemoveTail() (K, V) {
	ent := c.ll.Back()
	if ent != nil {
		kv := ent.Value.(*entry[K, V])
		c.RemoveElement(ent)
		return kv.key, kv.value
	}
	var zeroK K
	var zeroV V
	return zeroK, zeroV
}

func (c *Cache[K, V]) Add(key K, value V, ttl time.Duration) (evictedKey K, evictedValue V, evicted bool) {
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
		// 如果更新导致超出容量，移除旧元素（通常更新不触发 TinyLFU 的 candidate 逻辑，但需维持容量）
		for c.maxBytes != 0 && c.maxBytes < c.nbytes {
			c.RemoveOldest()
		}
		return
	}

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

	// 如果超出容量，移除最旧的元素并返回，以便 WTinyLFU 决定是否将其晋升或丢弃
	if c.maxBytes != 0 && c.maxBytes < c.nbytes {
		evictedKey, evictedValue = c.RemoveTail()
		evicted = true
		// 如果移除一个后仍然超出（例如新元素极大），继续移除直到满足限制
		for c.maxBytes != 0 && c.maxBytes < c.nbytes {
			c.RemoveOldest()
		}
	}
	return
}

func (c *Cache[K, V]) RemoveExpired(maxKeys int) {
	var count int
	for _, element := range c.cache {
		if count >= maxKeys {
			break
		}
		count++
		kv := element.Value.(*entry[K, V])
		if !kv.expireAt.IsZero() && kv.expireAt.Before(time.Now()) {
			c.RemoveElement(element)
		}
	}
}

func (c *Cache[K, V]) PeekOldest() *entry[K, V] {
	element := c.ll.Back()
	if element != nil {
		return element.Value.(*entry[K, V])
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

type WTinyLFUCache[K comparable, V Value] struct {
	window *Cache[K, V]

	protected *Cache[K, V]
	probation *Cache[K, V]

	sketch *cmSketch
}

func NewWTinyLFUCache[K comparable, V Value](capacity int, maxBytes int64, onEvicted func(K, V)) *WTinyLFUCache[K, V] {
	if maxBytes < 1 {
		return nil
	}

	// Window Cache: 1% of total bytes
	windowBytes := int64(float64(maxBytes) * 0.01)
	if windowBytes < 1 {
		windowBytes = 1
	}

	// Main Cache: 99% of total bytes
	mainBytes := maxBytes - windowBytes

	// Protected: 80% of Main
	protectedBytes := int64(float64(mainBytes) * 0.8)

	// Probation: 20% of Main
	probationBytes := mainBytes - protectedBytes

	if probationBytes < 1 {
		probationBytes = 1
	}
	// Adjust protected if needed to ensure total <= maxBytes (integer math might leave gaps, which is fine)

	return &WTinyLFUCache[K, V]{
		window:    NewCache[K, V](windowBytes, nil),
		protected: NewCache[K, V](protectedBytes, nil),
		probation: NewCache[K, V](probationBytes, onEvicted), // Eviction from probation is real eviction
		sketch:    newCMSketch(capacity),
	}
}

func (c *WTinyLFUCache[K, V]) getKeyStr(key K) string {
	if k, ok := any(key).(string); ok {
		return k
	}
	return fmt.Sprintf("%v", key)
}

func (c *WTinyLFUCache[K, V]) Get(key K) (V, bool) {
	keyStr := c.getKeyStr(key)
	c.sketch.Increment(keyStr)

	if val, ok := c.window.Get(key); ok {
		return val, ok
	}

	if val, ok := c.protected.Get(key); ok {
		return val, ok
	}

	if val, ok := c.probation.Get(key); ok {
		// Promote to protected
		// Retrieve expiration from probation entry
		elem := c.probation.cache[key]
		ent := elem.Value.(*entry[K, V])
		expireAt := ent.expireAt

		c.probation.Remove(key)

		// Calculate TTL for protected
		var ttl time.Duration
		if !expireAt.IsZero() {
			ttl = time.Until(expireAt)
			if ttl <= 0 {
				ttl = time.Nanosecond
			}
		}

		// Check if protected will evict to capture victim's expiration
		var victimExpireAt time.Time
		kSize := 0
		if k, ok := any(key).(string); ok {
			kSize = len(k)
		}
		newSize := int64(kSize) + int64(val.Len())

		if c.protected.maxBytes > 0 && c.protected.nbytes+newSize > c.protected.maxBytes {
			if tail := c.protected.ll.Back(); tail != nil {
				victimExpireAt = tail.Value.(*entry[K, V]).expireAt
			} else {
				victimExpireAt = expireAt
			}
		}

		k, v, evicted := c.protected.Add(key, val, ttl)
		if evicted {
			// Demote evicted from protected to probation
			var victimTTL time.Duration
			if !victimExpireAt.IsZero() {
				victimTTL = time.Until(victimExpireAt)
				if victimTTL <= 0 {
					victimTTL = time.Nanosecond
				}
			}
			c.probation.Add(k, v, victimTTL)
		}
		return val, true
	}
	var zeroV V
	return zeroV, false
}

func (c *WTinyLFUCache[K, V]) Put(key K, value V, ttl time.Duration) {
	keyStr := c.getKeyStr(key)
	c.sketch.Increment(keyStr)

	if c.window.Contains(key) {
		c.window.Add(key, value, ttl)
		return
	}

	if c.protected.Contains(key) {
		c.protected.Add(key, value, ttl)
		return
	}

	if c.probation.Contains(key) {
		// Update in probation
		c.probation.Add(key, value, ttl)
		return
	}

	candidateKey, candidateValue, evicted := c.window.Add(key, value, ttl)
	if !evicted {
		return
	}

	c.admit(candidateKey, candidateValue, ttl)
}

func (c *WTinyLFUCache[K, V]) admit(candidateKey K, candidateValue V, candidateTTL time.Duration) {
	kSize := 0
	if k, ok := any(candidateKey).(string); ok {
		kSize = len(k)
	}
	newSize := int64(kSize) + int64(candidateValue.Len())

	if c.probation.maxBytes == 0 || c.probation.nbytes+newSize <= c.probation.maxBytes {
		c.probation.Add(candidateKey, candidateValue, candidateTTL)
		return
	}

	victimEntry := c.probation.PeekOldest()
	if victimEntry == nil {
		c.probation.Add(candidateKey, candidateValue, candidateTTL)
		return
	}

	candidateKeyStr := c.getKeyStr(candidateKey)
	candidateFreq := c.sketch.Estimate(candidateKeyStr)
	victimFreq := c.sketch.Estimate(c.getKeyStr(victimEntry.key))

	if candidateFreq > victimFreq {
		c.probation.Add(candidateKey, candidateValue, candidateTTL)
	} else {
		// Reject candidate
		if c.probation.OnEvicted != nil {
			c.probation.OnEvicted(candidateKey, candidateValue)
		}
	}
}
func (c *WTinyLFUCache[K, V]) Remove(key K) {
	c.window.Remove(key)
	c.protected.Remove(key)
	c.probation.Remove(key)
}

func (c *WTinyLFUCache[K, V]) RemoveExpired(maxKeys int) {
	c.window.RemoveExpired(maxKeys)
	c.protected.RemoveExpired(maxKeys)
	c.probation.RemoveExpired(maxKeys)
}

func (c *WTinyLFUCache[K, V]) Len() int {
	return c.window.Len() + c.protected.Len() + c.probation.Len()
}
