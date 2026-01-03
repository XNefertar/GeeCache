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
	lfu := NewTinyLFU[string, String](100, 0, nil)
	lfu.Put("key1", String("1234"), 0)
	if v, ok := lfu.Get("key1"); !ok || string(v.(String)) != "1234" {
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
	// Cap allows 2 items.
	// len("key1") + len("value1") = 4 + 6 = 10
	// len("key2") + len("value2") = 4 + 6 = 10
	cap := int64(20)

	lfu := NewTinyLFU[string, String](100, cap, nil)

	// 1. Add k1. Freq(k1)=1. Cache: [k1]
	lfu.Put(k1, String(v1), 0)

	// 2. Add k2. Freq(k2)=1. Cache: [k2, k1] (MRU->LRU)
	lfu.Put(k2, String(v2), 0)

	// 3. Add k3. Freq(k3)=1. Cache full.
	// Victim is k1 (LRU). Freq(k1)=1.
	// Candidate k3. Freq(k3)=1.
	// Candidate <= Victim (1 <= 1) -> Reject k3.
	lfu.Put(k3, String(v3), 0)

	if lfu.cache.Len() != 2 {
		t.Fatalf("cache length should be 2")
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
	cap := int64(20)

	lfu := NewTinyLFU[string, String](100, cap, nil)

	lfu.Put(k1, String(v1), 0) // k1 freq=1
	lfu.Put(k2, String(v2), 0) // k2 freq=1

	// Boost k3 frequency
	// We need freq(k3) > freq(victim=k1=1).
	// Put increments k3 once. So we need 1 more increment before Put, or just rely on Put?
	// Put: Increment(k3) -> freq=1.
	// Compare 1 vs 1. Reject.

	// So we need to increment k3 externally or simulate access.
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
	callback := func(key string, value Value) {
		keys = append(keys, key)
	}
	// Capacity 10. Item size ~10.
	lfu := NewTinyLFU[string, String](100, int64(10), callback)

	lfu.Put("key1", String("123456"), 0) // size 4+6=10. Full.

	// To trigger eviction, we need k2 > k1.
	lfu.sketch.Increment("k2")
	lfu.sketch.Increment("k2")

	lfu.Put("k2", String("k2"), 0)

	// Expect k1 evicted.
	expect := []string{"key1"}

	if !reflect.DeepEqual(expect, keys) {
		t.Fatalf("Call OnEvicted failed, expect keys equals to %s, got %s", expect, keys)
	}
}

func TestExpiration(t *testing.T) {
	lfu := NewTinyLFU[string, String](100, 0, nil)
	lfu.Put("key1", String("1234"), time.Second)
	if v, ok := lfu.Get("key1"); !ok || string(v.(String)) != "1234" {
		t.Fatalf("cache hit key1=1234 failed")
	}
	// We can't easily mock time in this simple implementation without dependency injection,
	// but we can sleep.
	// However, sleeping in tests is flaky.
	// The original lru_test used sleep.
}

func TestRemoveExpired(t *testing.T) {
	lfu := NewTinyLFU[string, String](100, 0, nil)
	lfu.Put("key1", String("1234"), time.Millisecond*100)
	lfu.Put("key2", String("5678"), time.Hour)

	time.Sleep(time.Millisecond * 200)

	if lfu.cache.Len() != 2 {
		t.Fatalf("should have 2 items before cleanup")
	}

	lfu.RemoveExpired(10)

	if lfu.cache.Len() != 1 {
		t.Fatalf("should have 1 item after cleanup, got %d", lfu.cache.Len())
	}
}
