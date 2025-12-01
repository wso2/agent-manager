#!/usr/bin/env bash

# Port forwarding script for Agent Management Platform services
# This script sets up port forwarding for all 4 required ports

set -e

# Default namespaces (can be overridden via environment variables)
AMP_NS="${AMP_NS:-agent-management-platform}"
OBSERVABILITY_NS="${OBSERVABILITY_NS:-openchoreo-observability-plane}"
DATA_PLANE_NS="${DATA_PLANE_NS:-openchoreo-data-plane}"

echo "Starting port forwarding for Agent Management Platform services..."
echo "Namespaces:"
echo "  - AMP: $AMP_NS"
echo "  - Observability: $OBSERVABILITY_NS"
echo "  - Data Plane: $DATA_PLANE_NS"
echo ""

# Port forward Console (3000)
echo "Port forwarding Console (3000)..."
kubectl port-forward -n "$AMP_NS" svc/agent-management-platform-console 3000:3000 &
CONSOLE_PID=$!

# Port forward Agent Manager Service (8080)
echo "Port forwarding Agent Manager Service (8080)..."
kubectl port-forward -n "$AMP_NS" svc/agent-management-platform-agent-manager-service 8080:8080 &
AGENT_MGR_PID=$!

# Port forward Traces Observer Service (9098) - Required
echo "Port forwarding Traces Observer Service (9098)..."
if kubectl get svc traces-observer-service -n "$OBSERVABILITY_NS" >/dev/null 2>&1; then
    kubectl port-forward -n "$OBSERVABILITY_NS" svc/traces-observer-service 9098:9098 &
    TRACES_PID=$!
else
    echo "⚠️  Warning: Traces Observer Service not found in $OBSERVABILITY_NS"
fi

# Port forward Data Prepper (21893) - Required
echo "Port forwarding Data Prepper (21893)..."
if kubectl get svc data-prepper -n "$OBSERVABILITY_NS" >/dev/null 2>&1; then
    kubectl port-forward -n "$OBSERVABILITY_NS" svc/data-prepper 21893:21893 &
    DATAPREPPER_PID=$!
else
    echo "⚠️  Warning: Data Prepper not found in $OBSERVABILITY_NS"
fi

# Port forward External gateway (8443) - Required
echo "Port forwarding External gateway (8443)..."
if kubectl get svc gateway-external -n "$DATA_PLANE_NS" >/dev/null 2>&1; then
    kubectl port-forward -n "$DATA_PLANE_NS" svc/gateway-external 8443:443 &
    EXTERNAL_GATEWAY_PID=$!
else
    echo "⚠️  Warning: External gateway not found in $DATA_PLANE_NS"
fi

echo ""
echo "✓ Port forwarding active!"
echo ""
echo "Services accessible at:"
echo "  - Console:         http://localhost:3000"
echo "  - Agent Manager:   http://localhost:8080"
echo "  - Traces Observer: http://localhost:9098"
echo "  - Data Prepper:    http://localhost:21893"
echo "  - External gateway:  http://localhost:8443"
echo ""
echo "Press Ctrl+C to stop all port forwarding"
echo ""

# Wait for all background processes
wait
