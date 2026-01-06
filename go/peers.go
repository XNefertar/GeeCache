package geecache

import (
	"context"
	pb "geecache/geecachepb"
)

type PeerPicker interface {
	PickPeer(key string) (peer PeerGetter, ok bool)
	GetAllPeers() []PeerGetter
}

type PeerGetter interface {
	Get(ctx context.Context, in *pb.Request, out *pb.Response) error
	Remove(ctx context.Context, in *pb.Request) error
}
