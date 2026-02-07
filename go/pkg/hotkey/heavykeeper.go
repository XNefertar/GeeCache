package hotkey

import (
	"container/heap"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"sort"
	"sync"
)

type Item struct {
	Fingerprint uint32
	Count       int
}

type MinHeap []Item

func (h MinHeap) Len() int           { return len(h) }
func (h MinHeap) Less(i, j int) bool { return h[i].Count < h[j].Count }
func (h MinHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *MinHeap) Push(x interface{}) {
	*h = append(*h, x.(Item))
}

func (h *MinHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

type Bucket struct {
	Fingerprint uint32
	Count       int
}

type HeavyKeeper struct {
	buckets []Bucket
	k       int
	b       float64
	topK    *MinHeap
	mu      sync.Mutex
}

func NewHeavyKeeper(size int, k int, b float64) *HeavyKeeper {
	h := &MinHeap{}
	heap.Init(h)
	return &HeavyKeeper{
		buckets: make([]Bucket, size),
		k:       k,
		b:       b,
		topK:    h,
	}
}

func hash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

// CheckAndAdd adds the item and returns true if it is considered a hot key (in Top K)
func (hk *HeavyKeeper) CheckAndAdd(item string) bool {
	hk.mu.Lock()
	defer hk.mu.Unlock()

	hk.insertInternal(item)

	// Check if cached items count fits in Top K
	fp := hash(item)
	idx := fp % uint32(len(hk.buckets))
	bucket := &hk.buckets[idx]

	if hk.topK.Len() < hk.k {
		return true
	}
	// Note: (*hk.topK)[0] is the minimum element in the heap (Top K's tail)
	// If current count >= min count in Top K, it's a hot key.
	return bucket.Count >= (*hk.topK)[0].Count
}

func (hk *HeavyKeeper) Insert(item string) {
	hk.mu.Lock()
	defer hk.mu.Unlock()
	hk.insertInternal(item)
}

func (hk *HeavyKeeper) insertInternal(item string) {
	fp := hash(item)
	idx := fp % uint32(len(hk.buckets))
	bucket := &hk.buckets[idx]

	if bucket.Fingerprint == fp {
		bucket.Count++
	} else {
		if rand.Float64() < 1.0/math.Pow(hk.b, float64(bucket.Count)) {
			bucket.Fingerprint = fp
			bucket.Count = 1
		} else {
			if bucket.Count > 0 {
				bucket.Count--
			}
		}
	}

	if hk.topK.Len() < hk.k {
		heap.Push(hk.topK, Item{
			Fingerprint: bucket.Fingerprint,
			Count:       bucket.Count,
		})
	} else {
		if bucket.Count > (*hk.topK)[0].Count {
			heap.Pop(hk.topK)
			heap.Push(hk.topK, Item{
				Fingerprint: bucket.Fingerprint,
				Count:       bucket.Count,
			})
		}
	}
}

func (hk *HeavyKeeper) QueryTopK(ordered bool) []Item {
	result := make([]Item, hk.topK.Len())
	copy(result, *hk.topK)

	if ordered {
		sort.Slice(result, func(i, j int) bool {
			return result[i].Count > result[j].Count
		})
	}
	return result
}
