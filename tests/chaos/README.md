# Chaos Mesh Test Scenarios for GeeCache

This directory contains Chaos Mesh experiment definitions for testing GeeCache resilience.

## Prerequisites

1. Install Chaos Mesh in your Kubernetes cluster:
```bash
kubectl create ns chaos-mesh
helm repo add chaos-mesh https://charts.chaos-mesh.org
helm install chaos-mesh chaos-mesh/chaos-mesh -n chaos-mesh --version 2.6.0
```

2. Deploy GeeCache:
```bash
kubectl apply -f ../../deploy/k8s/
```

## Test Scenarios

### 1. Pod Kill (Node Failure Simulation)

Randomly kills one GeeCache pod to test failover and consistency hash rebalancing.

```bash
kubectl apply -f pod-kill-experiment.yaml
```

**Expected Behavior:**
- Other pods continue serving requests
- Consistent hash ring rebalances automatically
- DNS discovery detects the missing pod within 10 seconds
- Cache hit rate may temporarily drop but recovers
- No data loss (keys redistributed to other nodes)

**Validation:**
- Monitor logs: `kubectl logs -l app=geecache -f`
- Check metrics for cache hit rate recovery
- Verify no 5xx errors in K6 load test

### 2. Network Delay (Latency Injection)

Adds 100ms delay to network traffic between pods.

```bash
kubectl apply -f network-delay-experiment.yaml
```

**Expected Behavior:**
- Request latency increases by ~100ms
- Singleflight prevents request amplification
- No timeouts or errors (unless timeout is < 100ms)
- System remains functional, just slower

**Validation:**
- Check p99 latency in K6 results (should increase by ~100ms)
- Verify singleflight is coalescing requests (check logs)

### 3. Network Partition (Split Brain Simulation)

Isolates one pod from the rest of the cluster.

```bash
kubectl apply -f network-partition-experiment.yaml
```

**Expected Behavior:**
- Partitioned pod cannot communicate with peers
- Other pods continue serving normally
- Requests to partitioned pod fallback to local getter
- May see increased database queries on partitioned pod

**Validation:**
- Check database query count (should increase on isolated pod)
- Verify other pods maintain normal hit rate

### 4. Combined Chaos (Realistic Scenario)

Applies multiple chaos scenarios simultaneously.

```bash
kubectl apply -f combined-chaos-experiment.yaml
```

## Running Chaos Tests with Load

1. Start continuous load test:
```bash
k6 run --duration 10m --vus 50 load_test.js
```

2. In another terminal, apply chaos experiment:
```bash
kubectl apply -f pod-kill-experiment.yaml
```

3. Monitor the results:
```bash
# Watch pod status
kubectl get pods -l app=geecache -w

# Stream logs
kubectl logs -l app=geecache -f | grep -E "K8sPeerPicker|SlowDB|error"

# Check metrics
kubectl port-forward svc/geecache 9090:9090
# Visit http://localhost:9090/metrics
```

## Cleanup

Remove all chaos experiments:
```bash
kubectl delete -f .
```

## Metrics to Monitor

- **Cache Hit Rate**: Should recover within 30 seconds after chaos
- **Error Rate**: Should remain < 5% during chaos
- **P99 Latency**: May spike during chaos but should recover
- **DNS Refresh Rate**: Should detect changes within 10 seconds
- **Singleflight Effectiveness**: Prevents stampede during recovery
