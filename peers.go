package geecache

import pb "geecache/geecachepb"

type PeerGetter interface {
	Get(in *pb.Request, out *pb.Response) error
	Remove(in *pb.Request) error
}
