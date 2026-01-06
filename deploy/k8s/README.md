# Running GeeCache on Kubernetes

This guide explains how to deploy and run GeeCache in a Kubernetes cluster with dynamic service discovery, auto-scaling support, and cloud-native features.

## Prerequisites

- Kubernetes cluster (Minikube, Kind, or any K8s cluster)
- kubectl configured
- Docker (for building images)
- Optional: Helm (for Chaos Mesh installation)
- Optional: k6 (for load testing)

## Architecture Overview

GeeCache runs as a StatefulSet with the following components:

- **StatefulSet**: Provides stable pod identities for cache nodes
- **Headless Service**: Enables DNS-based peer discovery
- **Regular Service**: Provides a stable endpoint for external access
- **ConfigMap**: Stores configuration parameters
- **Health Checks**: Liveness and readiness probes for auto-healing

### Dynamic Service Discovery

Unlike the traditional approach where peer addresses are hardcoded via `-peers` flag, the Kubernetes deployment uses **DNS-based service discovery**:

1. Each pod registers with a Headless Service
2. Pods query DNS (e.g., `geecache-headless.default.svc.cluster.local`) to discover all peer IPs
3. DNS queries return all pod IPs in the service
4. The K8sPeerPicker refreshes the peer list every 10 seconds
5. Consistent hashing automatically rebalances when pods are added/removed

This enables:
- **Auto-scaling**: Add/remove pods without manual configuration
- **Self-healing**: Failed pods are automatically replaced and discovered
- **Zero-downtime deployments**: Rolling updates without service interruption

## Quick Start

### 1. Build Docker Image

First, build the GeeCache container image:

```bash
# From the repository root
cd /home/runner/work/GeeCache/GeeCache

# Build the image
docker build -t geecache:latest -f deploy/k8s/Dockerfile .

# For Minikube, load the image
minikube image load geecache:latest

# For Kind
kind load docker-image geecache:latest
```

### 2. Deploy to Kubernetes

Deploy all components:

```bash
kubectl apply -f deploy/k8s/configmap.yaml
kubectl apply -f deploy/k8s/statefulset.yaml
```

### 3. Verify Deployment

Check that pods are running:

```bash
# Watch pods come up
kubectl get pods -l app=geecache -w

# Check pod logs
kubectl logs geecache-0 -f

# Verify service discovery
kubectl logs geecache-0 | grep "K8sPeerPicker"
```

Expected output:
```
[K8sPeerPicker] Discovered 2 peers via DNS geecache-headless.default.svc.cluster.local
```

### 4. Test the Cache

Port-forward to access the cache:

```bash
# Forward cache port
kubectl port-forward svc/geecache 8080:8080

# In another terminal, test the cache
curl "http://localhost:8080/_geecache/scores/Tom"
```

### 5. Check Health

```bash
# Liveness probe
curl http://localhost:8080/health/live

# Readiness probe
curl http://localhost:8080/health/ready

# Full health status
curl http://localhost:8080/health
```

## Configuration

### Environment Variables

Configure GeeCache via the ConfigMap (`deploy/k8s/configmap.yaml`):

| Variable | Description | Default |
|----------|-------------|---------|
| `CACHE_MEMORY_LIMIT` | L1/L2 cache memory limit (bytes) | 134217728 (128MB) |
| `CACHE_HOT_MEMORY_LIMIT` | Hot cache memory limit (bytes) | 16777216 (16MB) |
| `CACHE_TTL_SECONDS` | Main cache TTL | 3600 (1 hour) |
| `CACHE_HOT_TTL_SECONDS` | Hot cache TTL | 300 (5 minutes) |
| `CONSISTENT_HASH_REPLICAS` | Virtual nodes per physical node | 50 |
| `SERVER_PORT` | Cache server port | 8080 |
| `K8S_SERVICE_NAME` | Headless service name for discovery | geecache-headless |
| `K8S_NAMESPACE` | Kubernetes namespace | default |

### Scaling

Scale the cache cluster:

```bash
# Scale to 5 replicas
kubectl scale statefulset geecache --replicas=5

# Watch the scaling process
kubectl get pods -l app=geecache -w

# Verify peer discovery
kubectl logs geecache-4 | grep "Discovered.*peers"
```

The consistent hash ring will automatically rebalance as nodes join.

### Resource Limits

Adjust resource requests/limits in `statefulset.yaml`:

```yaml
resources:
  requests:
    memory: "256Mi"
    cpu: "100m"
  limits:
    memory: "512Mi"
    cpu: "500m"
```

## Load Testing

Run load tests against the Kubernetes deployment:

```bash
# Port forward the service
kubectl port-forward svc/geecache 8080:8080 &

# Run k6 load test
cd tests
k6 run --env BASE_URL=http://localhost:8080 --env GROUP_NAME=scores load_test.js
```

### Expected Results

On a 3-node cluster with moderate load (100 VUs):
- **QPS**: 5,000-10,000 req/s per node
- **P99 Latency**: < 100ms (cache hit), < 500ms (cache miss)
- **Cache Hit Rate**: > 80% after warmup
- **Error Rate**: < 1%

## Chaos Testing

Test resilience with Chaos Mesh. See [tests/chaos/README.md](../../tests/chaos/README.md) for details.

### Quick Chaos Test

1. Install Chaos Mesh:
```bash
kubectl create ns chaos-mesh
helm repo add chaos-mesh https://charts.chaos-mesh.org
helm install chaos-mesh chaos-mesh/chaos-mesh -n chaos-mesh
```

2. Start load test:
```bash
k6 run --duration 10m --vus 50 tests/load_test.js &
```

3. Inject chaos:
```bash
kubectl apply -f tests/chaos/pod-kill-experiment.yaml
```

4. Monitor behavior:
```bash
kubectl logs -l app=geecache -f | grep -E "Discovered|error|SlowDB"
```

**Expected behavior during pod kill:**
- Cache hit rate drops briefly (~5-10s)
- Other pods continue serving normally
- DNS discovery detects change within 10s
- No 5xx errors (requests route to surviving nodes)
- Hit rate recovers to normal within 30s

## Monitoring

### Metrics Endpoint

Access Prometheus metrics:

```bash
kubectl port-forward svc/geecache 9090:9090
curl http://localhost:9090/metrics
```

### Useful Metrics

- `geecache_gets_total`: Total cache get operations
- `geecache_hits_total`: Cache hits
- `geecache_loads_total`: Cache loads from source
- `geecache_peer_requests_total`: Peer-to-peer requests
- `geecache_peer_errors_total`: Peer request errors

### Logs

Stream logs from all pods:

```bash
# All pods
kubectl logs -l app=geecache -f

# Specific pod
kubectl logs geecache-0 -f

# Filter for important events
kubectl logs -l app=geecache -f | grep -E "K8sPeerPicker|SlowDB|error|panic"
```

## Troubleshooting

### Pods not discovering each other

Check DNS resolution:

```bash
kubectl exec geecache-0 -- nslookup geecache-headless.default.svc.cluster.local
```

Should return all pod IPs.

### High error rate

Check pod health:

```bash
kubectl get pods -l app=geecache
kubectl describe pod geecache-0
kubectl logs geecache-0 --tail=100
```

### Memory issues

Check resource usage:

```bash
kubectl top pods -l app=geecache
```

Adjust memory limits in `statefulset.yaml` if needed.

### Readiness probe failing

Check health endpoint:

```bash
kubectl port-forward geecache-0 8080:8080
curl -v http://localhost:8080/health/ready
```

## Cleanup

Remove all GeeCache resources:

```bash
kubectl delete -f deploy/k8s/
```

## Advanced Topics

### Using with Horizontal Pod Autoscaler (HPA)

```bash
kubectl autoscale statefulset geecache --cpu-percent=70 --min=3 --max=10
```

Note: HPA with StatefulSet requires Kubernetes 1.23+

### Persistent Storage

GeeCache uses ephemeral storage by default. For persistent L3 cache:

1. Configure a StorageClass
2. Update `volumeClaimTemplates` in `statefulset.yaml`
3. Mount volume to `/data`

### Multi-Namespace Deployment

To deploy in a different namespace:

```bash
kubectl create namespace geecache-prod
kubectl apply -f deploy/k8s/ -n geecache-prod
```

Update `K8S_NAMESPACE` in ConfigMap accordingly.

## Best Practices

1. **Start with 3 replicas** for basic redundancy
2. **Set appropriate TTLs** based on your data freshness requirements
3. **Monitor cache hit rate** - aim for > 70%
4. **Test chaos scenarios** before production
5. **Use resource limits** to prevent memory issues
6. **Enable Prometheus scraping** for observability
7. **Implement circuit breakers** for downstream services

## Next Steps

- [Run Chaos Tests](../../tests/chaos/README.md)
- [Load Testing Guide](../../tests/README.md)
- [Performance Tuning](../go/docs/PERFORMANCE.md)
- [Observability Setup](../go/docs/OBSERVABILITY.md)
