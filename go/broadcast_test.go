package geecache

import (
	pb "geecache/geecachepb"
	"testing"
)

// Mock PeerGetter for Broadcast
type mockBroadcastPeer struct {
	removedKeys []string
}

func (m *mockBroadcastPeer) Get(in *pb.Request, out *pb.Response) error {
	return nil
}

func (m *mockBroadcastPeer) Remove(in *pb.Request) error {
	m.removedKeys = append(m.removedKeys, in.Key)
	return nil
}

// Mock PeerPicker for Broadcast
type mockBroadcastPicker struct {
	peers []*mockBroadcastPeer
}

func (m *mockBroadcastPicker) PickPeer(key string) (PeerGetter, bool) {
	// Return false to simulate local node is the owner, so it populates mainCache
	return nil, false
}

func (m *mockBroadcastPicker) GetAllPeers() []PeerGetter {
	var peers []PeerGetter
	for _, p := range m.peers {
		peers = append(peers, p)
	}
	return peers
}

func TestBroadcastInvalidation(t *testing.T) {
	// 1. Setup
	peer1 := &mockBroadcastPeer{}
	peer2 := &mockBroadcastPeer{}
	picker := &mockBroadcastPicker{peers: []*mockBroadcastPeer{peer1, peer2}}

	g, err := NewGroup("broadcastTest", 2<<20, GetterFunc(func(key string) ([]byte, error) {
		return []byte("value"), nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := g.RegisterPeers(picker); err != nil {
		t.Fatal(err)
	}

	// 2. Populate Cache
	key := "key1"
	_, err = g.Get(key) // Load into cache
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Verify it's in cache (mainCache)
	if _, ok := g.mainCache.get(key); !ok {
		t.Errorf("Key %s should be in mainCache", key)
	}

	// 3. Remove
	err = g.Remove(key)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// 4. Verify Local Removal
	if _, ok := g.mainCache.get(key); ok {
		t.Errorf("Key %s should be removed from mainCache", key)
	}
	if _, ok := g.hotCache.get(key); ok {
		t.Errorf("Key %s should be removed from hotCache", key)
	}

	// 5. Verify Broadcast
	if len(peer1.removedKeys) != 1 || peer1.removedKeys[0] != key {
		t.Errorf("Peer1 should have received remove request for %s", key)
	}
	if len(peer2.removedKeys) != 1 || peer2.removedKeys[0] != key {
		t.Errorf("Peer2 should have received remove request for %s", key)
	}
}
