package geecache

// MessageQueue 定义了消息队列的抽象接口
// 任何实现了该接口的组件（如 Redis Pub/Sub, Kafka Consumer）都可以集成到 GeeCache 中
type MessageQueue interface {
	// Subscribe 订阅一个主题，返回一个只读通道，用于接收消息（Key）
	Subscribe(topic string) (<-chan string, error)
	// Publish 发布一条消息（Key）到指定主题
	Publish(topic string, key string) error
}

// MemoryMQ 是一个基于 Go Channel 的简单内存消息队列实现，用于测试或单机演示
type MemoryMQ struct {
	queues map[string][]chan string
}

func NewMemoryMQ() *MemoryMQ {
	return &MemoryMQ{
		queues: make(map[string][]chan string),
	}
}

func (m *MemoryMQ) Subscribe(topic string) (<-chan string, error) {
	ch := make(chan string, 100)
	m.queues[topic] = append(m.queues[topic], ch)
	return ch, nil
}

func (m *MemoryMQ) Publish(topic string, key string) error {
	if subscribers, ok := m.queues[topic]; ok {
		for _, ch := range subscribers {
			go func(c chan string) {
				c <- key
			}(ch)
		}
	}
	return nil
}
