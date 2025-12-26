package geecachegrpc

import (
	"context"
	"fmt"
	"geecache"
	"geecache/consistenthash"
	pb "geecache/geecachepb"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// GRPCPool implements PeerPicker for a pool of GRPC peers.
type GRPCPool struct {
	self        string
	mu          sync.RWMutex
	peers       *consistenthash.Map
	grpcGetters map[string]*grpcGetter
	pb.UnimplementedGroupCacheServer
}

// NewGRPCPool initializes an GRPC pool of peers.
func NewGRPCPool(self string) *GRPCPool {
	return &GRPCPool{
		self:        self,
		grpcGetters: make(map[string]*grpcGetter),
	}
}

// Log info with server name
func (p *GRPCPool) Log(format string, v ...interface{}) {
	log.Printf("[Server %s] %s", p.self, fmt.Sprintf(format, v...))
}

// Set updates the pool's list of peers.
func (p *GRPCPool) Set(peers ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.peers = consistenthash.New(50, nil)
	p.peers.Add(peers...)
	for _, getter := range p.grpcGetters {
		getter.Close()
	}
	p.grpcGetters = make(map[string]*grpcGetter, len(peers))
	for _, peer := range peers {
		p.grpcGetters[peer] = &grpcGetter{addr: peer}
	}
}

// PickPeer picks a peer according to key
func (p *GRPCPool) PickPeer(key string) (peer geecache.PeerGetter, ok bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.peers.Get(key) != "" && p.peers.Get(key) != p.self {
		// p.Log("Pick peer %s", p.peers.Get(key))
		return p.grpcGetters[p.peers.Get(key)], true
	}
	return nil, false
}

// GetAllPeers returns all the peers in the pool
func (p *GRPCPool) GetAllPeers() []geecache.PeerGetter {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var peers []geecache.PeerGetter
	for _, peer := range p.grpcGetters {
		peers = append(peers, peer)
	}
	return peers
}

// grpcGetter implements PeerGetter
type grpcGetter struct {
	addr string
	conn *grpc.ClientConn
	once sync.Once
}

func (g *grpcGetter) getConn() (*grpc.ClientConn, error) {
	var err error
	g.once.Do(func() {
		g.conn, err = grpc.NewClient(g.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	})
	if err != nil {
		return nil, err
	}
	return g.conn, nil
}

func (g *grpcGetter) Close() {
	if g.conn != nil {
		g.conn.Close()
	}
}

func (g *grpcGetter) Get(in *pb.Request, out *pb.Response) error {
	conn, err := g.getConn()
	if err != nil {
		return err
	}

	client := pb.NewGroupCacheClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	resp, err := client.Get(ctx, in)
	if err != nil {
		return err
	}
	out.Value = resp.Value
	return nil
}

func (g *grpcGetter) Remove(in *pb.Request) error {
	conn, err := g.getConn()
	if err != nil {
		return err
	}

	client := pb.NewGroupCacheClient(conn)
	_, err = client.Remove(context.Background(), in)
	return err
}

// Run starts the GRPC server
func (p *GRPCPool) Run() error {
	lis, err := net.Listen("tcp", p.self)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}
	server := grpc.NewServer()
	pb.RegisterGroupCacheServer(server, p)
	// p.Log("rpc server is running at %s", p.self)
	return server.Serve(lis)
}

// Get implements GroupCacheServer
func (p *GRPCPool) Get(ctx context.Context, req *pb.Request) (*pb.Response, error) {
	// p.Log("Get %s %s", req.Group, req.Key)
	group := geecache.GetGroup(req.Group)
	if group == nil {
		return nil, fmt.Errorf("no such group: %s", req.Group)
	}
	view, err := group.Get(req.Key)
	if err != nil {
		return nil, err
	}
	return &pb.Response{Value: view.ByteSlice()}, nil
}

// Remove implements GroupCacheServer
func (p *GRPCPool) Remove(ctx context.Context, req *pb.Request) (*pb.Response, error) {
	// p.Log("Remove %s %s", req.Group, req.Key)
	group := geecache.GetGroup(req.Group)
	if group == nil {
		return nil, fmt.Errorf("no such group: %s", req.Group)
	}
	group.RemoveLocal(req.Key)
	return &pb.Response{}, nil
}
