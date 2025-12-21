package geecache

// CentralCache defines the interface for L3 cache (e.g. Redis, Memcached)
type CentralCache interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
	Delete(key string) error
}

// NoOpCentralCache is a default implementation that does nothing
type NoOpCentralCache struct{}

func (c *NoOpCentralCache) Get(key string) ([]byte, error) {
	return nil, nil
}

func (c *NoOpCentralCache) Set(key string, value []byte) error {
	return nil
}

func (c *NoOpCentralCache) Delete(key string) error {
	return nil
}
