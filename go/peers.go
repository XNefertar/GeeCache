package geecache

import pb "geecache/geecachepb"

type PeerPicker interface {
	PickPeer(key string) (peer PeerGetter, ok bool)
	GetAllPeers() []PeerGetter
}

type PeerGetter interface {
	Get(in *pb.Request, out *pb.Response) error
	Remove(in *pb.Request) error
}
