package geecache

import (
	"fmt"
	pb "geecache/geecachepb"
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

type Group struct {
	name      string
	getter    Getter
	mainCache cache
	hotCache  cache
	peers     PeerPicker
	loader    *singleflight.Group
	ttl       time.Duration
}

var (
	mu     sync.RWMutex
	groups = make(map[string]*Group)
)

func (g *Group) RegisterPeers(peers PeerPicker) {
	if g.peers != nil {
		panic("RegisterPeerPicker called more than once")
	}
	g.peers = peers
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

func NewGroup(name string, cacheBytes int64, getter Getter) *Group {
	if getter == nil {
		panic("nil Getter")
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
	return g
}

func GetGroup(name string) *Group {
	mu.RLock()
	g := groups[name]
	mu.RUnlock()
	return g
}

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
	bytes, err := g.getter.Get(key)
	if err != nil {
		return ByteView{}, err
	}
	value := ByteView{
		b: cloneBytes(bytes),
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
