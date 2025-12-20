package geecache

import (
	"fmt"
	pb "geecache/geecachepb"
	"testing"
	"time"
)

// Mock PeerGetter
type mockPeerGetter struct {
	key   string
	value []byte
	calls int
}

func (m *mockPeerGetter) Get(in *pb.Request, out *pb.Response) error {
	m.calls++
	if in.Key != m.key {
		return fmt.Errorf("key mismatch")
	}
	out.Value = m.value
	return nil
}

// Mock PeerPicker
type mockPeerPicker struct {
	peer *mockPeerGetter
}

func (m *mockPeerPicker) PickPeer(key string) (PeerGetter, bool) {
	// Always return the same peer for testing
	return m.peer, true
}

func TestHotCacheProtection(t *testing.T) {
	// 1. Setup
	key := "hotKey"
	val := []byte("hotValue")

	// Mock Peer
	peer := &mockPeerGetter{
		key:   key,
		value: val,
	}
	picker := &mockPeerPicker{peer: peer}

	// Create Group
	// Use a getter that fails to ensure we are hitting the peer
	getter := GetterFunc(func(key string) ([]byte, error) {
		return nil, fmt.Errorf("should not be called")
	})

	// Use a large enough cache size to ensure shards have capacity
	// 2<<20 = 2MB. hotCache = 128KB. 256 shards -> 512 bytes per shard.
	// Key+Value is small, so this should be enough.
	g := NewGroup("hotCacheTest", 2<<20, getter)
	g.RegisterPeers(picker)

	// 2. First Request - Should hit the peer
	v, err := g.Get(key)
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}
	if string(v.ByteSlice()) != string(val) {
		t.Errorf("expected %s, got %s", val, v.ByteSlice())
	}

	if peer.calls != 1 {
		t.Errorf("expected 1 peer call, got %d", peer.calls)
	}

	// 3. Second Request - Should hit the hotCache (no peer call)
	v, err = g.Get(key)
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}
	if string(v.ByteSlice()) != string(val) {
		t.Errorf("expected %s, got %s", val, v.ByteSlice())
	}

	if peer.calls != 1 {
		t.Errorf("expected 1 peer call (cached), got %d", peer.calls)
	}

	// 4. Wait for TTL (5s) + buffer
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	// Wait slightly more than 5s to ensure expiration
	time.Sleep(6 * time.Second)

	// 5. Third Request - Should hit the peer again
	v, err = g.Get(key)
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}

	if peer.calls != 2 {
		t.Errorf("expected 2 peer calls (expired), got %d", peer.calls)
	}
}
