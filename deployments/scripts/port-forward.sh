#!/bin/bash
set -e

echo "=== Setting up Port Forwarding for OpenChoreo Services ==="

# Check if kubectl is available
if ! command -v kubectl &> /dev/null; then
    echo "❌ kubectl is not installed"
    exit 1
fi

# Check if cluster is running
if ! kubectl cluster-info --context kind-openchoreo-local &> /dev/null; then
    echo "❌ Kind cluster 'openchoreo-local' is not running"
    exit 1
fi

echo "🔧 Setting kubectl context..."
kubectl config use-context kind-openchoreo-local

echo ""
echo "🌐 Starting port forwarding for OpenChoreo services..."
echo "   Press Ctrl+C to stop all port forwarding"
echo ""

# Function to cleanup background processes on exit
cleanup() {
    echo ""
    echo "🛑 Stopping all port forwarding..."
    jobs -p | xargs kill 2>/dev/null || true
    exit 0
}
trap cleanup EXIT INT TERM

# Port forward OpenSearch
echo "📊 Forwarding OpenSearch (9200)..."
kubectl port-forward -n openchoreo-observability-plane svc/opensearch 9200:9200 &

# Port forward Data Prepper
echo "📊 Forwarding Data Prepper (21893)..."
kubectl port-forward -n openchoreo-observability-plane svc/data-prepper 21893:21893 &

# Port forward OpenSearch Dashboard
echo "📊 Forwarding OpenSearch Dashboard (5601)..."
kubectl port-forward -n openchoreo-observability-plane svc/opensearch-dashboard 5601:5601 &

# Port forward Traces Observer Service
echo "🔍 Forwarding Traces Observer Service (9098)..."
kubectl port-forward -n openchoreo-observability-plane svc/traces-observer-service 9098:9098 &

#Port forward Observer Service API
echo "🔍 Forwarding Observer Service API (8085)..."
kubectl port-forward -n openchoreo-observability-plane svc/observer 8085:8080 &

# Port forward OpenChoreo Control Plane (if available)
echo "🎛️  Forwarding OpenChoreo Control Plane API (8000)..."
kubectl port-forward -n openchoreo-control-plane svc/api-server 8000:8080 &

# Port forward Gateway External
echo "🌐 Forwarding Gateway External (8443)..."
kubectl port-forward -n openchoreo-data-plane svc/gateway-external 8443:443 &

# Port forward Backstage (if installed)
if kubectl get svc backstage-demo -n openchoreo-control-plane &>/dev/null; then
    echo "🎭 Forwarding Backstage Portal (7007)..."
    kubectl port-forward -n openchoreo-control-plane svc/backstage-demo 7007:7007 &
fi

# Port forward Identity Provider (if installed)
if kubectl get svc identity-provider -n openchoreo-identity-system &>/dev/null; then
    echo "🔐 Forwarding Identity Provider (9090)..."
    kubectl port-forward -n openchoreo-identity-system svc/identity-provider 9090:8090 &
fi

echo ""
echo "✅ Port forwarding active:"
echo "   OpenSearch:           http://localhost:9200"
echo "   Data Prepper:        http://localhost:21893"
echo "   Traces Observer Service:      http://localhost:9098"
echo "   OpenSearch Dashboard: http://localhost:5601"
echo "   Control Plane API:    http://localhost:8000"
echo "   Gateway External:     https://localhost:8443"

if kubectl get svc backstage-demo -n openchoreo-control-plane &>/dev/null; then
    echo "   Backstage Portal:     http://localhost:7007"
fi

if kubectl get svc identity-provider -n openchoreo-identity-system &>/dev/null; then
    echo "   Identity Provider:    http://localhost:9090"
fi

echo ""
echo "💡 Keep this terminal open. Press Ctrl+C to stop."

# Wait for all background jobs
wait
