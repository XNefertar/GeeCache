package geecache

import (
	"fmt"
	pb "geecache/geecachepb"
	"geecache/mq"
	"geecache/singleflight"
	"log"
	"sync"
	"time"
)

type Getter interface {
	Get(key string) ([]byte, error)
}

type GetterFunc func(key string) ([]byte, error)

func (f GetterFunc) Get(key string) ([]byte, error) {
	return f(key)
}

type Setter interface {
	Set(key string, value []byte) error
}

type SetterFunc func(key string, value []byte) error

func (f SetterFunc) Set(key string, value []byte) error {
	return f(key, value)
}

type WriteStrategy int

const (
	StrategyWriteThrough WriteStrategy = iota
	StrategyWriteBack
)

// Group is a cache namespace and associated data loaded spread over
// a group of 1 or more nodes.
type Group struct {
	name         string
	getter       Getter
	setter       Setter
	mainCache    cache
	hotCache     cache
	centralCache CentralCache
	peers        PeerPicker
	loader       *singleflight.Group
	ttl          time.Duration
	mq           mq.MessageQueue
	mqTopic      string
}

var (
	mu     sync.RWMutex
	groups = make(map[string]*Group)
)

// RegisterPeers registers a PeerPicker for choosing remote peer.
func (g *Group) RegisterPeers(peers PeerPicker) error {
	if g.peers != nil {
		return fmt.Errorf("RegisterPeerPicker called more than once")
	}
	g.peers = peers
	return nil
}

func (g *Group) RegisterSetter(setter Setter) {
	g.setter = setter
}

// RegisterMQ 注册消息队列，用于接收缓存失效通知
// topic: 订阅的主题，通常是缓存组的名称
func (g *Group) RegisterMQ(mq mq.MessageQueue, topic string) error {
	ch, err := mq.Subscribe(topic)
	if err != nil {
		return err
	}
	g.mq = mq
	g.mqTopic = topic
	go g.listenMQ(ch)
	return nil
}

func (g *Group) listenMQ(ch <-chan string) {
	for key := range ch {
		// 收到失效广播，仅需删除本地缓存
		// 因为假设所有节点都订阅了 MQ，所以不需要再发起 RPC 广播
		g.RemoveLocal(key)
		// log.Printf("[GeeCache] MQ Invalidate: %s", key)
	}
}

func (g *Group) SetCentralCache(c CentralCache) {
	g.centralCache = c
}

func (g *Group) SetTTL(ttl time.Duration) {
	g.ttl = ttl
}

func (g *Group) RunCleanup(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			g.mainCache.removeExpired()
			g.hotCache.removeExpired()
		}
	}()
}

// NewGroup creates a new instance of Group.
// name: unique name of the group.
// cacheBytes: max bytes of the cache.
// getter: callback to get data from source if cache miss.
func NewGroup(name string, cacheBytes int64, getter Getter) (*Group, error) {
	if getter == nil {
		return nil, fmt.Errorf("nil Getter")
	}
	mu.Lock()
	defer mu.Unlock()

	// Use factory to initialize sharded cache
	c := newCache(cacheBytes)
	// Initialize hotCache with 1/8th of the capacity
	hc := newCache(cacheBytes / 8)

	g := &Group{
		name:      name,
		getter:    getter,
		mainCache: *c,
		hotCache:  *hc,
		loader:    &singleflight.Group{},
	}
	groups[name] = g
	// Start periodic cleanup (Redis style)
	g.RunCleanup(time.Second * 10)
	return g, nil
}

func GetGroup(name string) *Group {
	mu.RLock()
	g := groups[name]
	mu.RUnlock()
	return g
}

// Get value for a key from cache
func (g *Group) Get(key string) (ByteView, error) {
	if key == "" {
		return ByteView{}, fmt.Errorf("key is required")
	}

	// 1. Check Hot Cache (L1) - Client-side caching for hot keys
	if v, ok := g.hotCache.get(key); ok {
		// log.Println("[GeeCache] hotCache hit")
		return v, nil
	}

	// 2. Check Main Cache (L2) - Authoritative cache for this node
	if v, ok := g.mainCache.get(key); ok {
		// log.Println("[GeeCache] hit")
		return v, nil
	}

	return g.load(key)
}

func (g *Group) Set(key string, value []byte, strategy WriteStrategy) error {
	if g.setter == nil {
		return fmt.Errorf("setter not registered")
	}

	// Helper to broadcast invalidation
	broadcast := func() {
		if g.mq != nil {
			g.mq.Publish(g.mqTopic, key)
		}
	}

	switch strategy {
	case StrategyWriteThrough:
		// 1. Update DB first (Synchronous)
		if err := g.setter.Set(key, value); err != nil {
			return err
		}
		// 2. Update L3 (Central Cache)
		if g.centralCache != nil {
			g.centralCache.Set(key, value)
		}
		// 3. Update Local Cache (L2)
		g.populateCache(key, ByteView{b: cloneBytes(value)})
		// 4. Broadcast Invalidation
		broadcast()
		return nil
	case StrategyWriteBack:
		// 1. Update Local Cache (L2)
		g.populateCache(key, ByteView{b: cloneBytes(value)})
		// 2. Async update DB & L3
		go func() {
			if err := g.setter.Set(key, value); err != nil {
				log.Printf("[GeeCache] Write-Back failed for key %s: %v", key, err)
			} else {
				// Only update L3 if DB write succeeded
				if g.centralCache != nil {
					g.centralCache.Set(key, value)
				}
				// Broadcast Invalidation after DB update to avoid stale reads from DB?
				// Or broadcast immediately?
				// Immediate broadcast might cause peers to read stale data from DB.
				// Delayed broadcast ensures DB is ready.
				broadcast()
			}
		}()
		return nil
	default:
		return fmt.Errorf("unknown strategy")
	}
}

func (g *Group) RemoveLocal(key string) {
	g.mainCache.remove(key)
	g.hotCache.remove(key)
}

func (g *Group) Remove(key string) error {
	g.RemoveLocal(key)

	if g.centralCache != nil {
		g.centralCache.Delete(key)
	}

	// Prefer MQ for cluster-wide invalidation
	if g.mq != nil {
		return g.mq.Publish(g.mqTopic, key)
	}

	if g.peers != nil {
		peers := g.peers.GetAllPeers()
		var wg sync.WaitGroup
		for _, peer := range peers {
			wg.Add(1)
			go func(p PeerGetter) {
				defer wg.Done()
				req := &pb.Request{
					Group: g.name,
					Key:   key,
				}
				p.Remove(req)
			}(peer)
		}
		wg.Wait()
	}
	return nil
}

func (g *Group) load(key string) (value ByteView, err error) {
	viewi, err := g.loader.Do(key, func() (interface{}, error) {
		if g.peers != nil {
			if peer, ok := g.peers.PickPeer(key); ok {
				if value, err := g.getFromPeer(peer, key); err == nil {
					// Hot Key Protection: Cache remote value locally for a short time
					g.populateHotCache(key, value)
					return value, nil
				}
				log.Println("[GeeCache] Failed to get from peer", err)
			}
		}

		return g.getLocally(key)
	})

	if err == nil {
		return viewi.(ByteView), nil
	}
	return
}

func (g *Group) getFromPeer(peer PeerGetter, key string) (ByteView, error) {
	req := &pb.Request{
		Group: g.name,
		Key:   key,
	}
	res := &pb.Response{}
	err := peer.Get(req, res)
	if err != nil {
		return ByteView{}, err
	}
	return ByteView{b: res.Value}, nil
}

func (g *Group) getLocally(key string) (ByteView, error) {
	// 1. Try L3 (Central Cache)
	if g.centralCache != nil {
		if v, err := g.centralCache.Get(key); err == nil && v != nil {
			value := ByteView{b: cloneBytes(v)}
			g.populateCache(key, value)
			return value, nil
		}
	}

	// 2. Fallback to Source (DB)
	bytes, err := g.getter.Get(key)
	if err != nil {
		return ByteView{}, err
	}
	value := ByteView{
		b: cloneBytes(bytes),
	}

	// 3. Populate L3
	if g.centralCache != nil {
		g.centralCache.Set(key, value.ByteSlice())
	}

	g.populateCache(key, value)
	return value, nil
}

func (g *Group) populateCache(key string, value ByteView) {
	g.mainCache.add(key, value, g.ttl)
}

func (g *Group) populateHotCache(key string, value ByteView) {
	// Hot Cache TTL: Short (e.g. 10% of main TTL or fixed 5s) to prevent stale data
	// while protecting against hot key spikes.
	hotCacheTTL := 5 * time.Second
	g.hotCache.add(key, value, hotCacheTTL)
}
