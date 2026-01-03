package tinylfu

import (
	"reflect"
	"testing"
	"time"
)

type String string

func (d String) Len() int {
	return len(d)
}

func TestGet(t *testing.T) {
	lfu := NewWTinyLFUCache[string, String](100, 100, nil)
	lfu.Put("key1", String("1234"), 0)
	if v, ok := lfu.Get("key1"); !ok || string(v) != "1234" {
		t.Fatalf("cache hit key1=1234 failed")
	}
	if _, ok := lfu.Get("key2"); ok {
		t.Fatalf("cache miss key2 failed")
	}
}

func TestRemoveOldest(t *testing.T) {
	// TinyLFU admission policy:
	// If cache is full, compare freq of new item vs victim.
	// If new item freq <= victim freq, reject new item.

	k1, k2, k3 := "key1", "key2", "k3"
	v1, v2, v3 := "value1", "value2", "v3"
	// Cap allows 2 items in Probation.
	// Window: 1, Protected: 79, Probation: 20.
	// Items go to Probation first (via Window eviction).
	cap := int64(100)

	lfu := NewWTinyLFUCache[string, String](100, cap, nil)

	// 1. Add k1. Freq(k1)=1. Probation: [k1]
	lfu.Put(k1, String(v1), 0)

	// 2. Add k2. Freq(k2)=1. Probation: [k2, k1]
	lfu.Put(k2, String(v2), 0)

	// 3. Add k3. Freq(k3)=1. Probation full.
	// Victim is k1 (LRU of Probation). Freq(k1)=1.
	// Candidate k3. Freq(k3)=1.
	// Candidate <= Victim (1 <= 1) -> Reject k3.
	lfu.Put(k3, String(v3), 0)

	if lfu.Len() != 2 {
		t.Fatalf("cache length should be 2, got %d", lfu.Len())
	}

	// k3 should be rejected
	if _, ok := lfu.Get(k3); ok {
		t.Fatalf("k3 should be rejected due to low frequency")
	}

	// k1 should still be there
	if _, ok := lfu.Get(k1); !ok {
		t.Fatalf("k1 should be kept")
	}
}

func TestEvictionWithFrequency(t *testing.T) {
	k1, k2, k3 := "key1", "key2", "k3"
	v1, v2, v3 := "value1", "value2", "v3"
	cap := int64(100)

	lfu := NewWTinyLFUCache[string, String](100, cap, nil)

	lfu.Put(k1, String(v1), 0) // k1 freq=1
	lfu.Put(k2, String(v2), 0) // k2 freq=1

	// Boost k3 frequency
	lfu.sketch.Increment(k3)
	lfu.sketch.Increment(k3)

	lfu.Put(k3, String(v3), 0) // Put increments k3 again.

	// Now k3 should replace k1
	if _, ok := lfu.Get(k3); !ok {
		t.Fatalf("k3 should be accepted")
	}
	if _, ok := lfu.Get(k1); ok {
		t.Fatalf("k1 should be evicted")
	}
}

func TestOnEvicted(t *testing.T) {
	keys := make([]string, 0)
	callback := func(key string, value String) {
		keys = append(keys, key)
	}
	// Capacity 100 -> Probation 20.
	lfu := NewWTinyLFUCache[string, String](100, int64(100), callback)

	// Add k1 (size 4)
	lfu.Put("k1", String("v1"), 0)

	// Add k2 (size 4)
	lfu.Put("k2", String("v2"), 0)

	// Probation: [k2, k1]. Used: 8.

	// Boost k3 frequency
	lfu.sketch.Increment("k3")
	lfu.sketch.Increment("k3")

	// Add k3 (size 14). Total needed 8+14=22 > 20.
	// k3 (freq 2) vs k1 (freq 1). k3 wins.
	// k1 evicted.
	// Probation: [k3, k2]. Used: 18.
	lfu.Put("k3", String("123456789012"), 0)

	// Expect k1 evicted.
	expect := []string{"k1"}

	if !reflect.DeepEqual(expect, keys) {
		t.Fatalf("Call OnEvicted failed, expect keys equals to %s, got %s", expect, keys)
	}
}

func TestExpiration(t *testing.T) {
	lfu := NewWTinyLFUCache[string, String](100, 100, nil)
	lfu.Put("key1", String("1234"), time.Second)
	if v, ok := lfu.Get("key1"); !ok || string(v) != "1234" {
		t.Fatalf("cache hit key1=1234 failed")
	}
}

func TestRemoveExpired(t *testing.T) {
	lfu := NewWTinyLFUCache[string, String](100, 100, nil)
	lfu.Put("key1", String("1234"), time.Millisecond*100)
	lfu.Put("key2", String("5678"), time.Hour)

	time.Sleep(time.Millisecond * 200)

	if lfu.Len() != 2 {
		t.Fatalf("should have 2 items before cleanup")
	}

	lfu.RemoveExpired(10)

	if lfu.Len() != 1 {
		t.Fatalf("should have 1 item after cleanup, got %d", lfu.Len())
	}
}
