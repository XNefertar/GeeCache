# GeeCache

A high-performance, cloud-native distributed cache system combining Go-based distributed cache layer with C++ LSM-tree storage engine.

## Features

### Core Caching
- **Multi-tier Cache Architecture**: L1 (Hot Cache) → L2 (Main Cache) → L3 (LSM Storage)
- **LRU & TinyLFU**: Efficient eviction policies for optimal hit rates
- **TTL Support**: Per-entry and group-level time-to-live
- **Cache Invalidation**: Write-through and write-back strategies with message queue support

### Distributed System
- **Consistent Hashing**: Automatic load balancing with minimal key redistribution
- **Peer-to-Peer Communication**: HTTP and gRPC transport protocols
- **Singleflight**: Prevents cache stampede during concurrent requests
- **Hot Key Detection**: Automatic hot cache replication for frequently accessed keys

### Cloud-Native (NEW) ☁️
- **Kubernetes Support**: Native StatefulSet deployment with DNS-based service discovery
- **Dynamic Peer Discovery**: Automatic detection of pod additions/removals
- **Graceful Shutdown**: SIGTERM handling with connection draining
- **Health Checks**: Liveness and readiness probes for Kubernetes
- **Environment Configuration**: 12-factor app compatible

### Observability
- **Metrics**: Prometheus-compatible metrics endpoint
- **Health API**: JSON health status with group information
- **Structured Logging**: Detailed logs for debugging and monitoring

### Storage
- **LSM-Tree Engine**: C++ implementation for persistent L3 storage
- **Write-Ahead Log**: Durability guarantees
- **SSTable Compaction**: Efficient disk usage
- **Skip List**: Fast in-memory indexing

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Client Request                          │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  L1: Hot Cache (TinyLFU)  │  Short TTL, Hot Key Replication│
└──────────────────────┬──────────────────────────────────────┘
                       │ Miss
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  L2: Main Cache (LRU)     │  Per-Node Authoritative Cache  │
└──────────────────────┬──────────────────────────────────────┘
                       │ Miss
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  Consistent Hash Routing  │  Pick peer or handle locally   │
└──────────────────────┬──────────────────────────────────────┘
                       │ Local
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  L3: LSM Storage (C++)    │  Persistent Disk-backed Cache  │
└──────────────────────┬──────────────────────────────────────┘
                       │ Miss
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  Database / Source       │  Original Data Source            │
└─────────────────────────────────────────────────────────────┘
```

## Quick Start

### Local Development

1. **Clone the repository**
```bash
git clone https://github.com/XNefertar/GeeCache.git
cd GeeCache
```

2. **Build C++ storage engine**
```bash
cd cpp/lsm
mkdir -p build && cd build
cmake ..
make
```

3. **Run Go tests**
```bash
cd ../../../go
go test -v -race ./...
```

4. **Start a local cache node**
```bash
cd go
go run cmd/k8s_server/main.go -port 8080 -k8s=false
```

5. **Test the cache**
```bash
curl "http://localhost:8080/_geecache/scores/Tom"
```

### Kubernetes Deployment

**Full guide:** [deploy/k8s/README.md](deploy/k8s/README.md)

```bash
# Build and load image
docker build -t geecache:latest -f deploy/k8s/Dockerfile .
minikube image load geecache:latest  # or: kind load docker-image geecache:latest

# Deploy to K8s
kubectl apply -f deploy/k8s/configmap.yaml
kubectl apply -f deploy/k8s/statefulset.yaml

# Verify deployment
kubectl get pods -l app=geecache -w
kubectl logs geecache-0 -f

# Test the cache
kubectl port-forward svc/geecache 8080:8080
curl "http://localhost:8080/_geecache/scores/Tom"
```

## Cloud-Native Features

### Dynamic Service Discovery

Traditional approach (static):
```bash
# ❌ Old way: hardcoded peer addresses
./geecache -peers="http://10.0.0.1:8080,http://10.0.0.2:8080"
```

Kubernetes approach (dynamic):
```yaml
# ✅ New way: DNS-based discovery
env:
  - name: K8S_SERVICE_NAME
    value: "geecache-headless"
  - name: K8S_NAMESPACE
    value: "default"
```

The cache automatically discovers peers via DNS queries to the Headless Service, enabling:
- Auto-scaling without manual reconfiguration
- Self-healing when pods fail
- Zero-downtime rolling updates

### Graceful Shutdown

```go
// Automatically handles SIGTERM/SIGINT
shutdownMgr := geecache.NewShutdownManager()
shutdownMgr.RegisterServer(httpServer)
shutdownMgr.RegisterPeerPicker(k8sPicker)
shutdownMgr.WaitForShutdown()  // Blocks until signal received
```

### Health Checks

```bash
# Liveness: Is the app running?
curl http://localhost:8080/health/live

# Readiness: Ready to serve traffic?
curl http://localhost:8080/health/ready

# Full status
curl http://localhost:8080/health
```

Kubernetes probes:
```yaml
livenessProbe:
  httpGet:
    path: /health/live
    port: 8080
readinessProbe:
  httpGet:
    path: /health/ready
    port: 8080
```

## Testing

### Unit Tests

```bash
cd go
go test -v -race ./...
```

### Load Testing

**Full guide:** [tests/README.md](tests/README.md)

```bash
# Install k6
brew install k6  # macOS

# Run load test
cd tests
k6 run load_test.js
```

Features:
- Zipfian distribution (80-20 rule)
- Cache stampede simulation
- Hotspot traffic patterns
- Performance thresholds

### Chaos Testing

**Full guide:** [tests/chaos/README.md](tests/chaos/README.md)

```bash
# Install Chaos Mesh
kubectl create ns chaos-mesh
helm install chaos-mesh chaos-mesh/chaos-mesh -n chaos-mesh

# Run chaos experiment
kubectl apply -f tests/chaos/pod-kill-experiment.yaml

# Monitor
kubectl logs -l app=geecache -f
```

Scenarios:
- Pod kill (node failure)
- Network delay (latency injection)
- Network partition (split-brain)
- Combined chaos

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SERVER_PORT` | HTTP server port | 8080 |
| `CACHE_MEMORY_LIMIT` | L1/L2 memory limit (bytes) | 134217728 (128MB) |
| `CACHE_TTL_SECONDS` | Main cache TTL | 3600 |
| `CACHE_HOT_TTL_SECONDS` | Hot cache TTL | 300 |
| `K8S_SERVICE_NAME` | Headless service name | geecache-headless |
| `K8S_NAMESPACE` | Kubernetes namespace | default |

### Example ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: geecache-config
data:
  CACHE_MEMORY_LIMIT: "268435456"  # 256MB
  CACHE_TTL_SECONDS: "7200"        # 2 hours
  K8S_SERVICE_NAME: "geecache-headless"
```

## Performance

### Benchmarks

On a 3-node K8s cluster with 100 concurrent users:

| Metric | Value |
|--------|-------|
| QPS | 8,000-12,000 req/s per node |
| P50 Latency | < 10ms (cache hit) |
| P95 Latency | < 100ms (cache hit) |
| P99 Latency | < 500ms (cache miss) |
| Cache Hit Rate | > 80% (after warmup) |
| Memory per Node | 128-512MB |

### Scaling

- **Horizontal**: Add more pods with `kubectl scale`
- **Vertical**: Adjust resource limits in StatefulSet
- **Consistent Hash**: Minimal key redistribution during scaling

## Monitoring

### Prometheus Metrics

```bash
kubectl port-forward svc/geecache 9090:9090
curl http://localhost:9090/metrics
```

Key metrics:
- `geecache_gets_total`: Total GET operations
- `geecache_hits_total`: Cache hits
- `geecache_loads_total`: Loads from source
- `geecache_peer_requests_total`: P2P requests

### Grafana Dashboard

Import the dashboard from `deploy/monitoring/grafana-dashboard.json` (TODO)

## Documentation

- [Kubernetes Deployment Guide](deploy/k8s/README.md)
- [Load Testing Guide](tests/README.md)
- [Chaos Testing Guide](tests/chaos/README.md)
- [Test Guide (Traditional)](go/docs/TEST_GUIDE.md)
- [Consistency Model](go/docs/consistency.md)

## Architecture Decisions

- **Why LSM?**: Optimized for write-heavy workloads, efficient disk usage
- **Why Consistent Hashing?**: Minimal key redistribution during scaling
- **Why Singleflight?**: Prevents cache stampede (thundering herd)
- **Why Multi-tier?**: Balances hit rate, latency, and durability
- **Why StatefulSet?**: Stable pod identities for consistent hashing

## Roadmap

- [x] Core distributed cache
- [x] LSM storage engine
- [x] Kubernetes support
- [x] Dynamic service discovery
- [x] Graceful shutdown
- [x] Health checks
- [x] Load testing
- [x] Chaos testing
- [ ] Horizontal Pod Autoscaler integration
- [ ] Prometheus Operator monitoring
- [ ] Grafana dashboards
- [ ] Multi-region deployment
- [ ] Advanced eviction policies

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License.

## Acknowledgments

- Inspired by [groupcache](https://github.com/golang/groupcache)
- LSM design based on LevelDB/RocksDB
- Consistent hashing algorithm from Dynamo paper
- Cloud-native patterns from Kubernetes community

## Contact

- GitHub Issues: [XNefertar/GeeCache/issues](https://github.com/XNefertar/GeeCache/issues)
- Discussions: [XNefertar/GeeCache/discussions](https://github.com/XNefertar/GeeCache/discussions)
