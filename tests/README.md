# GeeCache Testing Guide

This directory contains load testing and chaos engineering tests for GeeCache.

## Directory Structure

```
tests/
├── load_test.js              # K6 load testing script
├── chaos/                    # Chaos Mesh experiments
│   ├── README.md            # Chaos testing guide
│   ├── pod-kill-experiment.yaml
│   ├── network-delay-experiment.yaml
│   ├── network-partition-experiment.yaml
│   └── combined-chaos-experiment.yaml
└── README.md                # This file
```

## Load Testing with K6

### Prerequisites

Install K6:

```bash
# macOS
brew install k6

# Linux
sudo apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
echo "deb https://dl.k6.io/deb stable main" | sudo tee /etc/apt/sources.list.d/k6.list
sudo apt-get update
sudo apt-get install k6

# Windows
choco install k6

# Docker
docker pull grafana/k6
```

### Running Load Tests

#### Basic Load Test

```bash
# Start GeeCache locally or port-forward K8s service
kubectl port-forward svc/geecache 8080:8080 &

# Run the load test
k6 run load_test.js
```

#### Custom Configuration

```bash
# Custom base URL and group name
k6 run --env BASE_URL=http://localhost:8080 --env GROUP_NAME=scores load_test.js

# Run with specific VU count
k6 run --vus 100 --duration 5m load_test.js

# Run with stages (custom ramp-up)
k6 run load_test.js
```

#### Kubernetes Testing

```bash
# Port forward the service
kubectl port-forward svc/geecache 8080:8080 &

# Run extended test
k6 run --duration 10m --vus 200 load_test.js

# Or test against NodePort/LoadBalancer
k6 run --env BASE_URL=http://<external-ip>:8080 load_test.js
```

### Understanding the Load Test

The `load_test.js` script simulates realistic cache usage patterns:

1. **Zipfian Distribution (80-20 rule)**
   - 80% of requests go to hot keys (top 100 keys)
   - 20% of requests go to cold keys (remaining 9,900 keys)
   - This mimics real-world traffic where few items are very popular

2. **Cache Stampede Simulation**
   - 20% of the time, many concurrent requests hit the same key
   - Tests the effectiveness of Singleflight mechanism
   - Key changes every 10 seconds to force cache misses

3. **Performance Thresholds**
   - P95 latency < 500ms
   - P99 latency < 1000ms
   - Error rate < 5%

### Interpreting Results

```
Metrics:
├── http_reqs: Total number of requests
├── http_req_duration: Response time distribution
├── cache_hits: Estimated cache hits (responses < 50ms)
├── cache_misses: Estimated cache misses
└── errors: Failed requests

Key Metrics to Watch:
- P99 latency should be < 1s
- Cache hit rate should be > 70% after warmup
- Error rate should be < 5%
- QPS should scale linearly with VUs
```

### Example Output

```
execution: local
    script: load_test.js
    output: -

  scenarios: (100.00%) 1 scenario, 200 max VUs, 9m30s max duration
           * default: Up to 200 looping VUs for 9m0s over 5 stages

running (9m00.1s), 000/200 VUs, 54321 complete and 0 interrupted iterations
default ✓ [======================================] 000/200 VUs  9m0s

✓ status is 200 or 404
✓ response time < 1000ms

cache_hits................: 43456  80.0%
cache_misses..............: 10865  20.0%
errors....................: 2.1%   rate
http_req_duration.........: avg=85ms  p(95)=250ms p(99)=450ms max=1.2s
http_reqs.................: 54321  100.6/s

=============================================================
GeeCache Load Test Summary
=============================================================

Total Requests: 54321
Cache Hits: 43456
Cache Misses: 10865
Error Rate: 2.10%

Response Time:
  p50: 45ms
  p95: 250ms
  p99: 450ms
  max: 1200ms

=============================================================
```

## Chaos Testing

See [chaos/README.md](chaos/README.md) for detailed chaos testing instructions.

### Quick Start

1. Deploy GeeCache to K8s:
```bash
kubectl apply -f ../deploy/k8s/
```

2. Start load test:
```bash
k6 run --duration 10m --vus 50 load_test.js &
```

3. Inject chaos:
```bash
kubectl apply -f chaos/pod-kill-experiment.yaml
```

4. Monitor:
```bash
kubectl logs -l app=geecache -f
```

## Continuous Testing

### CI/CD Integration

Add to your CI pipeline:

```yaml
# .github/workflows/load-test.yml
name: Load Test
on: [push]
jobs:
  load-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - name: Start GeeCache
        run: |
          docker-compose up -d
          sleep 10
      - name: Run K6
        run: |
          docker run --network=host -v $PWD:/tests grafana/k6 run /tests/tests/load_test.js
      - name: Check results
        run: |
          # Parse summary.json and fail if thresholds exceeded
          cat summary.json
```

### Scheduled Performance Testing

Run nightly performance tests:

```bash
# Cron job example
0 2 * * * cd /path/to/GeeCache && k6 run tests/load_test.js --out influxdb=http://localhost:8086/k6
```

## Benchmarking

Compare performance across versions:

```bash
# Baseline
git checkout v1.0
k6 run load_test.js --summary-export=baseline.json

# New version
git checkout v2.0
k6 run load_test.js --summary-export=v2.json

# Compare
k6 compare baseline.json v2.json
```

## Integration with Monitoring

### Prometheus + Grafana

Export K6 metrics to Prometheus:

```bash
# Run K6 with Prometheus remote write
k6 run --out experimental-prometheus-rw load_test.js

# Or use statsd
k6 run --out statsd load_test.js
```

### Real-time Monitoring

```bash
# Terminal UI
k6 run --out web load_test.js

# Visit http://localhost:5665 for real-time metrics
```

## Best Practices

1. **Warm-up Period**: Include a ramp-up stage to warm the cache
2. **Realistic Data**: Use production-like key distributions
3. **Duration**: Run for at least 5-10 minutes for stable results
4. **Concurrency**: Test with realistic VU counts (10-200)
5. **Iteration**: Run multiple times and average results
6. **Baseline**: Establish baseline metrics before changes
7. **Chaos**: Combine load tests with chaos experiments

## Troubleshooting

### High Error Rate

- Check server logs: `kubectl logs -l app=geecache`
- Verify resource limits: `kubectl top pods`
- Check network connectivity
- Verify DNS resolution

### Low Cache Hit Rate

- Increase cache memory
- Adjust TTL settings
- Check key distribution
- Verify singleflight is working

### High Latency

- Check for network issues
- Verify pod resources (CPU/memory)
- Check for hot keys
- Review database performance

## Further Reading

- [K6 Documentation](https://k6.io/docs/)
- [Chaos Mesh Documentation](https://chaos-mesh.org/)
- [GeeCache K8s Guide](../deploy/k8s/README.md)
- [Performance Tuning](../go/docs/PERFORMANCE.md)
