#!/bin/bash
set -e

echo "=== Setting up Port Forwarding for OpenChoreo Services ==="

# Check if kubectl is available
if ! command -v kubectl &> /dev/null; then
    echo "❌ kubectl is not installed"
    exit 1
fi

# Check if cluster is running
if ! kubectl cluster-info --context k3d-openchoreo-local-v0.9 &> /dev/null; then
    echo "❌ k3d cluster 'openchoreo-local-v0.9' is not running"
    exit 1
fi

echo "🔧 Setting kubectl context..."
kubectl config use-context k3d-openchoreo-local-v0.9

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

# Port forward OpenTelemetry Collector
echo "📊 Forwarding OpenTelemetry Collector..."
kubectl port-forward -n openchoreo-observability-plane svc/opentelemetry-collector 21893:4318 &

# Port forward Traces Observer Service
echo "🔍 Forwarding Traces Observer Service (9098)..."
kubectl port-forward -n openchoreo-observability-plane svc/amp-traces-observer 9098:9098 &

# Port forward Observer Service API
echo "🔍 Forwarding Observer Service API (8085)..."
kubectl port-forward -n openchoreo-observability-plane svc/observer 8085:8080 &

# Port forward Thunder (IDP)
echo "🔑 Forwarding Thunder IDP Service (8090)..."
kubectl port-forward -n amp-thunder svc/amp-thunder-extension-service 8090:8090 &

# Port forward Observability Gateway
echo "🌐 Forwarding Observability Gateway (22893)..."
kubectl port-forward -n openchoreo-data-plane svc/obs-gateway-gateway-router 22893:8080 &


echo ""
echo "✅ Port forwarding active:"
echo "   Thunder IDP Service:        http://localhost:8090"
echo "   Observer Service API:       http://localhost:8085"
echo "   Traces Observer Service:    http://localhost:9098"
echo "   OpenSearch:                 http://localhost:9200"
echo "   OpenTelemetry Collector:    http://localhost:21893"
echo "   Observability Gateway:      http://localhost:22893"

echo ""
echo "💡 Keep this terminal open. Press Ctrl+C to stop."

# Wait for all background jobs
wait
