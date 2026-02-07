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
	Index       int // 在堆中的索引
}

type MinHeap []*Item

func (h MinHeap) Len() int           { return len(h) }
func (h MinHeap) Less(i, j int) bool { return h[i].Count < h[j].Count }

func (h MinHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].Index = i
	h[j].Index = j
}

func (h *MinHeap) Push(x interface{}) {
	item := x.(*Item)
	item.Index = len(*h)
	*h = append(*h, item)
}

func (h *MinHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	item.Index = -1 // 标记为删除
	*h = old[0 : n-1]
	return item
}

type Bucket struct {
	Fingerprint uint32
	Count       int
}

type HeavyKeeper struct {
	buckets             []Bucket
	k                   int
	b                   float64
	topK                *MinHeap
	fingerprintIndexMap map[uint32]*Item
	mu                  sync.Mutex
}

func NewHeavyKeeper(size int, k int, b float64) *HeavyKeeper {
	h := &MinHeap{}
	heap.Init(h)
	return &HeavyKeeper{
		buckets:             make([]Bucket, size),
		k:                   k,
		b:                   b,
		topK:                h,
		fingerprintIndexMap: make(map[uint32]*Item),
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

	count := hk.insertInternal(item)

	if hk.topK.Len() < hk.k {
		return true
	}
	// Note: (*hk.topK)[0] is the minimum element in the heap (Top K's tail)
	// If current count >= min count in Top K, it's a hot key.
	return count >= (*hk.topK)[0].Count
}

func (hk *HeavyKeeper) Insert(item string) {
	hk.mu.Lock()
	defer hk.mu.Unlock()
	hk.insertInternal(item)
}

func (hk *HeavyKeeper) insertInternal(item string) int {
	fp := hash(item)
	idx := fp % uint32(len(hk.buckets))
	bucket := &hk.buckets[idx]

	if bucket.Fingerprint == fp {
		bucket.Count++
	} else {
		if rand.Float64() < 1.0/math.Pow(hk.b, float64(bucket.Count)) {
			// Evict old fingerprint from the top-K heap if tracked
			if oldItem, ok := hk.fingerprintIndexMap[bucket.Fingerprint]; ok {
				heap.Remove(hk.topK, oldItem.Index)
				delete(hk.fingerprintIndexMap, bucket.Fingerprint)
			}
			bucket.Fingerprint = fp
			bucket.Count = 1
		} else {
			if bucket.Count > 0 {
				bucket.Count--
			}
		}
	}

	if heapItem, exists := hk.fingerprintIndexMap[bucket.Fingerprint]; exists {
		heapItem.Count = bucket.Count
		heap.Fix(hk.topK, heapItem.Index)
	} else {
		if hk.topK.Len() < hk.k {
			newItem := &Item{
				Fingerprint: bucket.Fingerprint,
				Count:       bucket.Count,
			}
			heap.Push(hk.topK, newItem)
			hk.fingerprintIndexMap[bucket.Fingerprint] = newItem
		} else {
			minItem := (*hk.topK)[0]
			if bucket.Count > minItem.Count {
				// 移除堆中最小的元素
				removed := heap.Pop(hk.topK).(*Item)
				delete(hk.fingerprintIndexMap, removed.Fingerprint)

				// 插入新元素
				newItem := &Item{
					Fingerprint: bucket.Fingerprint,
					Count:       bucket.Count,
				}
				heap.Push(hk.topK, newItem)
				hk.fingerprintIndexMap[bucket.Fingerprint] = newItem
			}
		}
	}

	if bucket.Fingerprint == fp {
		return bucket.Count
	}
	return 0
}

func (hk *HeavyKeeper) QueryTopK(ordered bool) []Item {
	hk.mu.Lock()
	defer hk.mu.Unlock()

	result := make([]Item, hk.topK.Len())
	for i, item := range *hk.topK {
		result[i] = *item
	}

	if ordered {
		sort.Slice(result, func(i, j int) bool {
			return result[i].Count > result[j].Count
		})
	}
	return result
}
