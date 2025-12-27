package tests

import (
	"fmt"
	"geecache"
	"geecache/geecachegrpc"
	pb "geecache/geecachepb"
	"testing"
	"time"
)

func TestGRPCGetter(t *testing.T) {
	// 1. Setup a Group and a GRPCPool server
	groupName := "grpc_test_group"
	addr := "localhost:9001"

	// Mock DB
	db := map[string]string{
		"key1": "value1",
	}

	g, _ := geecache.NewGroup(groupName, 2<<10, geecache.GetterFunc(
		func(key string) ([]byte, error) {
			if v, ok := db[key]; ok {
				return []byte(v), nil
			}
			return nil, fmt.Errorf("%s not exist", key)
		}))

	pool := geecachegrpc.NewGRPCPool(addr)
	pool.Set(addr)
	g.RegisterPeers(pool)

	// Start server in goroutine
	go pool.Run()
	time.Sleep(time.Second) // Wait for start

	// 2. Create a grpcGetter (client)
	clientAddr := "localhost:9002"
	clientPool := geecachegrpc.NewGRPCPool(clientAddr)
	clientPool.Set(addr) // The client knows about the server

	// Force pick the peer (since we only have one peer in the list that is not self)
	// consistenthash might map key to the peer.
	// Since we only added `addr` (9001) to `clientPool`, and `clientPool.self` is 9002.
	// `PickPeer` should return 9001.

	peer, ok := clientPool.PickPeer("any_key")
	if !ok {
		t.Fatal("failed to pick peer")
	}

	// 3. Call Get
	req := &pb.Request{
		Group: groupName,
		Key:   "key1",
	}
	res := &pb.Response{}
	err := peer.Get(req, res)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(res.Value) != "value1" {
		t.Errorf("expected value1, got %s", string(res.Value))
	}

	// 4. Test Remove (should not fail)
	err = peer.Remove(req)
	if err != nil {
		t.Errorf("Remove failed: %v", err)
	}
}
