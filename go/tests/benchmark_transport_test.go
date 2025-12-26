package tests

import (
	"fmt"
	"geecache"
	"geecache/geecachegrpc"
	"geecache/geecachehttp"
	pb "geecache/geecachepb"
	"net/http"
	"sync"
	"testing"
	"time"
)

var (
	httpServerOnce sync.Once
	grpcServerOnce sync.Once
)

// setupBenchGroup creates a group that returns constant value
func setupBenchGroup() *geecache.Group {
	// Check if group already exists to avoid panic
	if g := geecache.GetGroup("bench_transport"); g != nil {
		return g
	}
	g, _ := geecache.NewGroup("bench_transport", 2<<10, geecache.GetterFunc(
		func(key string) ([]byte, error) {
			return []byte("value-1234567890"), nil
		}))
	return g
}

func BenchmarkTransport_HTTP(b *testing.B) {
	setupBenchGroup()
	port := 8002
	addr := fmt.Sprintf("localhost:%d", port)
	fullAddr := "http://" + addr

	// Start Server Once
	httpServerOnce.Do(func() {
		go func() {
			peers := geecachehttp.NewHTTPPool(fullAddr)
			http.ListenAndServe(addr, peers)
		}()
		// Give server time to start
		time.Sleep(500 * time.Millisecond)
	})

	// Setup Client
	clientPool := geecachehttp.NewHTTPPool("http://localhost:8003")
	clientPool.Set(fullAddr)

	peer, ok := clientPool.PickPeer("key")
	if !ok {
		b.Fatal("failed to pick peer")
	}

	req := &pb.Request{
		Group: "bench_transport",
		Key:   "key",
	}
	res := &pb.Response{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := peer.Get(req, res); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransport_GRPC(b *testing.B) {
	setupBenchGroup()
	port := 9002
	addr := fmt.Sprintf("localhost:%d", port)

	// Start Server Once
	grpcServerOnce.Do(func() {
		go func() {
			peers := geecachegrpc.NewGRPCPool(addr)
			peers.Run()
		}()
		// Give server time to start
		time.Sleep(500 * time.Millisecond)
	})

	// Setup Client
	clientPool := geecachegrpc.NewGRPCPool("localhost:9003")
	clientPool.Set(addr)

	peer, ok := clientPool.PickPeer("key")
	if !ok {
		b.Fatal("failed to pick peer")
	}

	req := &pb.Request{
		Group: "bench_transport",
		Key:   "key",
	}
	res := &pb.Response{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := peer.Get(req, res); err != nil {
			b.Fatal(err)
		}
	}
}
