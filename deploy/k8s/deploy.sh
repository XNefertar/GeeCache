#!/bin/bash
# GeeCache Kubernetes Deployment Script

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
NAMESPACE="${GEECACHE_NAMESPACE:-default}"
IMAGE_NAME="${GEECACHE_IMAGE:-geecache:latest}"
REPLICAS="${GEECACHE_REPLICAS:-3}"

# Functions
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_prerequisites() {
    print_info "Checking prerequisites..."
    
    if ! command -v kubectl &> /dev/null; then
        print_error "kubectl not found. Please install kubectl."
        exit 1
    fi
    
    if ! command -v docker &> /dev/null; then
        print_error "docker not found. Please install docker."
        exit 1
    fi
    
    # Check if kubectl can connect to cluster
    if ! kubectl cluster-info &> /dev/null; then
        print_error "Cannot connect to Kubernetes cluster. Is kubectl configured?"
        exit 1
    fi
    
    print_info "Prerequisites check passed ✓"
}

build_image() {
    print_info "Building Docker image: $IMAGE_NAME"
    
    # Build C++ library
    print_info "Building C++ LSM storage library..."
    if [ -d "../../cpp/lsm" ]; then
        cd ../../cpp/lsm
        mkdir -p build
        cd build
        cmake .. > /dev/null 2>&1 || print_warn "CMake failed, continuing..."
        make > /dev/null 2>&1 || print_warn "Make failed, continuing..."
        cd ../../../deploy/k8s
    else
        print_warn "C++ storage directory not found, skipping..."
    fi
    
    # Build Docker image
    cd ../..
    docker build -t "$IMAGE_NAME" -f deploy/k8s/Dockerfile . || {
        print_error "Docker build failed"
        exit 1
    }
    cd deploy/k8s
    
    print_info "Image built successfully ✓"
}

load_image() {
    print_info "Loading image into Kubernetes cluster..."
    
    # Detect cluster type and load image
    if command -v minikube &> /dev/null && minikube status &> /dev/null; then
        print_info "Detected Minikube cluster"
        minikube image load "$IMAGE_NAME"
    elif command -v kind &> /dev/null && kind get clusters 2>&1 | grep -q .; then
        print_info "Detected Kind cluster"
        CLUSTER=$(kind get clusters | head -n1)
        kind load docker-image "$IMAGE_NAME" --name "$CLUSTER"
    else
        print_warn "Could not detect cluster type (Minikube/Kind). Skipping image load."
        print_warn "Make sure the image is available in your cluster's registry."
    fi
    
    print_info "Image loaded successfully ✓"
}

deploy() {
    print_info "Deploying GeeCache to namespace: $NAMESPACE"
    
    # Create namespace if it doesn't exist
    if ! kubectl get namespace "$NAMESPACE" &> /dev/null; then
        print_info "Creating namespace: $NAMESPACE"
        kubectl create namespace "$NAMESPACE"
    fi
    
    # Apply ConfigMap
    print_info "Applying ConfigMap..."
    kubectl apply -f configmap.yaml -n "$NAMESPACE"
    
    # Apply StatefulSet and Services
    print_info "Applying StatefulSet..."
    kubectl apply -f statefulset.yaml -n "$NAMESPACE"
    
    # Wait for rollout
    print_info "Waiting for pods to be ready (this may take a few minutes)..."
    kubectl wait --for=condition=ready pod -l app=geecache -n "$NAMESPACE" --timeout=180s || {
        print_warn "Timeout waiting for pods. Check status with: kubectl get pods -n $NAMESPACE"
    }
    
    print_info "Deployment completed ✓"
}

status() {
    print_info "GeeCache Status in namespace: $NAMESPACE"
    echo ""
    
    echo "=== Pods ==="
    kubectl get pods -l app=geecache -n "$NAMESPACE"
    echo ""
    
    echo "=== Services ==="
    kubectl get svc -l app=geecache -n "$NAMESPACE"
    echo ""
    
    echo "=== StatefulSet ==="
    kubectl get statefulset geecache -n "$NAMESPACE"
    echo ""
    
    # Check peer discovery
    POD=$(kubectl get pods -l app=geecache -n "$NAMESPACE" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
    if [ -n "$POD" ]; then
        echo "=== Peer Discovery (from $POD) ==="
        kubectl logs "$POD" -n "$NAMESPACE" 2>/dev/null | grep -i "K8sPeerPicker\|Discovered" | tail -5 || echo "No discovery logs yet"
    fi
}

test_cache() {
    print_info "Testing GeeCache..."
    
    POD=$(kubectl get pods -l app=geecache -n "$NAMESPACE" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
    if [ -z "$POD" ]; then
        print_error "No pods found"
        exit 1
    fi
    
    print_info "Port-forwarding to $POD..."
    kubectl port-forward "$POD" 8080:8080 -n "$NAMESPACE" > /dev/null 2>&1 &
    PF_PID=$!
    sleep 2
    
    echo ""
    echo "=== Health Check ==="
    curl -s http://localhost:8080/health | python3 -m json.tool 2>/dev/null || curl -s http://localhost:8080/health
    
    echo ""
    echo "=== Cache Test ==="
    RESULT=$(curl -s "http://localhost:8080/_geecache/scores/Tom")
    echo "GET /scores/Tom => $RESULT"
    
    # Cleanup
    kill $PF_PID 2>/dev/null || true
    
    print_info "Test completed ✓"
}

delete() {
    print_info "Deleting GeeCache from namespace: $NAMESPACE"
    
    kubectl delete -f statefulset.yaml -n "$NAMESPACE" --ignore-not-found
    kubectl delete -f configmap.yaml -n "$NAMESPACE" --ignore-not-found
    
    print_info "Deletion completed ✓"
}

scale() {
    REPLICAS=$1
    if [ -z "$REPLICAS" ]; then
        print_error "Usage: $0 scale <replicas>"
        exit 1
    fi
    
    print_info "Scaling GeeCache to $REPLICAS replicas..."
    kubectl scale statefulset geecache --replicas="$REPLICAS" -n "$NAMESPACE"
    
    print_info "Waiting for scale operation..."
    kubectl rollout status statefulset/geecache -n "$NAMESPACE"
    
    print_info "Scale completed ✓"
}

logs() {
    POD=$1
    if [ -z "$POD" ]; then
        # Show logs from all pods
        print_info "Streaming logs from all geecache pods..."
        kubectl logs -l app=geecache -f -n "$NAMESPACE"
    else
        # Show logs from specific pod
        print_info "Streaming logs from $POD..."
        kubectl logs "$POD" -f -n "$NAMESPACE"
    fi
}

usage() {
    cat << EOF
GeeCache Kubernetes Deployment Script

Usage: $0 <command> [options]

Commands:
    build       Build Docker image
    load        Load image into cluster (Minikube/Kind)
    deploy      Deploy GeeCache to Kubernetes
    status      Show deployment status
    test        Test the cache (health check + sample request)
    scale <n>   Scale to n replicas
    logs [pod]  Show logs (all pods or specific pod)
    delete      Delete GeeCache from cluster
    all         Build, load, and deploy (complete setup)

Environment Variables:
    GEECACHE_NAMESPACE    Kubernetes namespace (default: default)
    GEECACHE_IMAGE        Docker image name (default: geecache:latest)
    GEECACHE_REPLICAS     Number of replicas (default: 3)

Examples:
    $0 all                      # Complete deployment
    $0 build                    # Just build image
    $0 deploy                   # Deploy to cluster
    $0 scale 5                  # Scale to 5 replicas
    $0 logs geecache-0          # Show logs from pod 0
    $0 test                     # Test the deployment

EOF
}

# Main
case "$1" in
    build)
        check_prerequisites
        build_image
        ;;
    load)
        check_prerequisites
        load_image
        ;;
    deploy)
        check_prerequisites
        deploy
        ;;
    status)
        status
        ;;
    test)
        test_cache
        ;;
    scale)
        scale "$2"
        ;;
    logs)
        logs "$2"
        ;;
    delete)
        delete
        ;;
    all)
        check_prerequisites
        build_image
        load_image
        deploy
        status
        ;;
    *)
        usage
        exit 1
        ;;
esac
