# GeeCache Kubernetes Quick Start

This is a step-by-step guide to get GeeCache running on Kubernetes in 5 minutes.

## Prerequisites

- Kubernetes cluster (Minikube, Kind, or any K8s cluster)
- kubectl installed and configured
- Docker installed

## Option 1: Quick Deploy (No Build Required)

If you just want to test the manifests without building:

```bash
# 1. Create namespace
kubectl create namespace geecache-demo

# 2. Apply manifests (will fail initially without image, but shows structure)
kubectl apply -f configmap.yaml -n geecache-demo
kubectl apply -f statefulset.yaml -n geecache-demo

# Note: Pods will be in ImagePullBackOff state until you build the image
```

## Option 2: Full Deployment with Image Build

### Step 1: Start Local Kubernetes

```bash
# For Minikube
minikube start --cpus=4 --memory=4096

# For Kind
kind create cluster --name geecache-demo
```

### Step 2: Build the Docker Image

```bash
# From repository root
cd /path/to/GeeCache

# Build C++ storage library first (Optional for Docker build as it's multi-stage, but good for local)
# The provided Dockerfile handles the C++ build internally.
# If you are building the image using the provided Dockerfile:
docker build -t geecache:latest -f deploy/k8s/Dockerfile .
```

# Build Docker image
docker build -t geecache:latest -f deploy/k8s/Dockerfile .

# Load into cluster
# For Minikube:
minikube image load geecache:latest

# For Kind:
kind load docker-image geecache:latest --name geecache-demo
```

### Step 3: Deploy to Kubernetes

```bash
cd deploy/k8s

# Create resources
kubectl apply -f configmap.yaml
kubectl apply -f statefulset.yaml

# Wait for pods to be ready
kubectl wait --for=condition=ready pod -l app=geecache --timeout=120s
```

### Step 4: Verify Deployment

```bash
# Check pods
kubectl get pods -l app=geecache

# Expected output:
# NAME         READY   STATUS    RESTARTS   AGE
# geecache-0   1/1     Running   0          2m
# geecache-1   1/1     Running   0          2m
# geecache-2   1/1     Running   0          2m

# Check logs (look for peer discovery)
kubectl logs geecache-0 | grep "K8sPeerPicker"

# Expected output:
# [K8sPeerPicker] Discovered 2 peers via DNS geecache-headless.default.svc.cluster.local
```

### Step 5: Test the Cache

```bash
# Port forward to access the cache
kubectl port-forward svc/geecache 8080:8080 &

# Test cache GET
curl "http://localhost:8080/_geecache/scores/Tom"
# Expected: 630

# Test health endpoint
curl http://localhost:8080/health
# Expected: {"status":"healthy",...}

# Test readiness
curl http://localhost:8080/health/ready
# Expected: {"status":"ready",...}
```

### Step 6: Test Scaling

```bash
# Scale to 5 replicas
kubectl scale statefulset geecache --replicas=5

# Watch pods come up
kubectl get pods -l app=geecache -w

# Check that new pods discovered peers
kubectl logs geecache-4 | grep "Discovered.*peers"
```

### Step 7: Run Load Test

```bash
# Install k6 (if not already installed)
brew install k6  # macOS
# or: sudo apt-get install k6  # Linux

# Run load test
cd ../../tests
k6 run --env BASE_URL=http://localhost:8080 load_test.js
```

## Cleanup

```bash
# Delete all resources
kubectl delete -f deploy/k8s/

# For Minikube
minikube delete

# For Kind
kind delete cluster --name geecache-demo
```

## Next Steps

1. **Run Chaos Tests**: See [tests/chaos/README.md](../../tests/chaos/README.md)
2. **Customize Configuration**: Edit `configmap.yaml` to adjust cache size, TTL, etc.
3. **Add Monitoring**: Set up Prometheus scraping on port 9090
4. **Production Deployment**: Use proper StorageClass, resource limits, and HPA

## Troubleshooting

### Pods not starting

```bash
# Describe pod for events
kubectl describe pod geecache-0

# Check logs
kubectl logs geecache-0 --previous  # If crashed
```

### DNS discovery not working

```bash
# Test DNS resolution from inside pod
kubectl exec geecache-0 -- nslookup geecache-headless.default.svc.cluster.local

# Should return all pod IPs
```

### Image pull errors

```bash
# For local development, ensure imagePullPolicy is IfNotPresent
kubectl patch statefulset geecache -p '{"spec":{"template":{"spec":{"containers":[{"name":"geecache","imagePullPolicy":"IfNotPresent"}]}}}}'
```

### Health checks failing

```bash
# Check health endpoint directly
kubectl exec geecache-0 -- wget -O- http://localhost:8080/health/live
```

## Configuration Examples

### Change cache size

Edit `configmap.yaml`:
```yaml
data:
  CACHE_MEMORY_LIMIT: "268435456"  # 256MB
```

Apply and restart:
```bash
kubectl apply -f configmap.yaml
kubectl rollout restart statefulset geecache
```

### Change TTL

```yaml
data:
  CACHE_TTL_SECONDS: "7200"  # 2 hours
```

### Scale replicas

```bash
# Edit statefulset.yaml
spec:
  replicas: 5

# Or use kubectl scale
kubectl scale statefulset geecache --replicas=5
```

## Demo Script

Complete demo from scratch:

```bash
#!/bin/bash
set -e

echo "=== GeeCache Kubernetes Demo ==="

# Start cluster
echo "1. Starting Minikube..."
minikube start --cpus=4 --memory=4096

# Build image
echo "2. Building Docker image..."
cd ../..
docker build -t geecache:latest -f deploy/k8s/Dockerfile .
minikube image load geecache:latest

# Deploy
echo "3. Deploying to Kubernetes..."
cd deploy/k8s
kubectl apply -f configmap.yaml
kubectl apply -f statefulset.yaml

# Wait
echo "4. Waiting for pods..."
kubectl wait --for=condition=ready pod -l app=geecache --timeout=120s

# Test
echo "5. Testing cache..."
kubectl port-forward svc/geecache 8080:8080 &
PF_PID=$!
sleep 2
curl http://localhost:8080/health
curl "http://localhost:8080/_geecache/scores/Tom"
kill $PF_PID

echo "=== Demo Complete! ==="
echo "Pods running:"
kubectl get pods -l app=geecache
```

Save as `quickstart-demo.sh` and run:
```bash
chmod +x quickstart-demo.sh
./quickstart-demo.sh
```
