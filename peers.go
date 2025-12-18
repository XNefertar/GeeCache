package geecache

type PeerGetter interface {
	Get(group string, key string) ([]byte, error)
}
