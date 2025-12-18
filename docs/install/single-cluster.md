# Single Cluster Installation

Install the Agent Management Platform on an existing OpenChoreo cluster.

## Prerequisites

- **OpenChoreo cluster (v0.3.0+)** with the following components installed:
  - OpenChoreo Control Plane
  - OpenChoreo Data Plane 
  - OpenChoreo Build Plane
  - OpenChoreo Observability Plane

  Follow [OpenChoreo Single Cluster Setup](https://openchoreo.dev/docs/v0.3.x/getting-started/single-cluster/) to install open-choreo single cluster.


- Sufficient permissions to create namespaces and deploy resources

## Verify Prerequisites

Before installation, verify your OpenChoreo cluster is ready:

```bash
# Check OpenChoreo namespaces exist
kubectl get namespace openchoreo-control-plane
kubectl get namespace openchoreo-data-plane
kubectl get namespace openchoreo-build-plane
kubectl get namespace openchoreo-observability-plane

# Verify Observability Plane is installed (required)
kubectl get pods -n openchoreo-observability-plane

# Check OpenSearch is available
kubectl get pods -n openchoreo-observability-plane -l app=opensearch
```

## Installation Steps

The Agent Management Platform installation consists of three main components:

1. **Agent Management Platform** - Core platform (PostgreSQL, API, Console)
2. **Observability Stack** - DataPrepper and Traces Observer
3. **Build CI** - Workflow templates for building container images

### Step 1: Install Agent Management Platform

The core platform includes:
- PostgreSQL database
- Agent Manager Service (API)
- Console (Web UI)


**Manual installation:**

```bash
# Set configuration variables
export HELM_CHART_REGISTRY="ghcr.io/wso2"
export AMP_CHART_VERSION="0.0.0-dev"  # Use your desired version
export AMP_NS="wso2-amp"

# Install the platform Helm chart
helm install amp \
  oci://${HELM_CHART_REGISTRY}/wso2-ai-agent-management-platform \
  --version ${AMP_CHART_VERSION} \
  --namespace ${AMP_NS} \
  --create-namespace \
  --timeout 1800s
```

### Step 2: Configure CORS (if needed)

If accessing the console from a different origin, configure CORS:

```bash
# Patch APIClass to allow CORS origin
kubectl patch apiclass default-with-cors \
  -n default \
  --type json \
  -p '[{"op":"add","path":"/spec/restPolicy/defaults/cors/allowOrigins/-","value":"http://localhost:3000"}]'
```

### Step 3: Install Observability Stack

The observability stack includes DataPrepper and Traces Observer:

```bash
# Set configuration variables
export OBSERVABILITY_CHART_VERSION="0.0.0-dev"  # Use your desired version
export OBSERVABILITY_NS="openchoreo-observability-plane"

# Install observability Helm chart
helm install amp-observability-traces \
  oci://${HELM_CHART_REGISTRY}/wso2-amp-observability-extension \
  --version ${OBSERVABILITY_CHART_VERSION} \
  --namespace ${OBSERVABILITY_NS} \
  --create-namespace \
  --timeout 1800s
```

### Step 4: Install Build CI (Optional)

Install workflow templates for building container images:

```bash
# Set configuration variables
export BUILD_CI_CHART_VERSION="0.0.0-dev"  # Use your desired version
export BUILD_CI_NS="openchoreo-build-plane"

# Install Build CI Helm chart
helm install agent-manager-build-ci \
  oci://${HELM_CHART_REGISTRY}/wso2-amp-build-extension \
  --version ${BUILD_CI_CHART_VERSION} \
  --namespace ${BUILD_CI_NS} \
  --create-namespace \
  --timeout 1800s
```

## Verification

Verify all components are installed and running:

```bash
# Check Agent Management Platform pods
kubectl get pods -n wso2-amp

# Check Observability pods
kubectl get pods -n openchoreo-observability-plane | grep -E "data-prepper|amp-traces-observer"

# Check Build CI pods (if installed)
kubectl get pods -n openchoreo-build-plane | grep agent-manager

# Check Helm releases
helm list -n wso2-amp
helm list -n openchoreo-observability-plane
helm list -n openchoreo-build-plane
```

Expected output should show all pods in `Running` or `Completed` state.

## Access the Platform

### Port Forwarding

Set up port forwarding to access the services locally:

```bash
# Console (port 3000)
kubectl port-forward -n wso2-amp svc/amp-console 3000:3000 &

# Agent Manager API (port 8080)
kubectl port-forward -n wso2-amp svc/amp-api 8080:8080 &

# Traces Observer (port 9098)
kubectl port-forward -n openchoreo-observability-plane svc/amp-traces-observer 9098:9098 &

# OTel Collector (port 21893)
kubectl port-forward -n openchoreo-observability-plane svc/opentelemetry-collector 21893:4318 &

# External gateway (port 8443)
kubectl port-forward -n openchoreo-data-plane svc/gateway-external 8443:443 &
```

### Access URLs

After port forwarding is set up:

- **Console**: http://localhost:3000
- **API**: http://localhost:8080
- **Traces Observer**: http://localhost:9098
- **Data Prepper**: http://localhost:21893

## Custom Configuration

### Using Custom Values File

Create a custom values file (e.g., `custom-values.yaml`):

```yaml
agentManagerService:
  replicaCount: 2
  resources:
    requests:
      memory: 512Mi
      cpu: 500m

console:
  replicaCount: 2
  
postgresql:
  auth:
    password: "my-secure-password"
```

Install with custom values:

```bash
helm install amp \
  oci://${HELM_CHART_REGISTRY}/wso2-ai-agent-management-platform \
  --version ${AMP_CHART_VERSION} \
  --namespace ${AMP_NS} \
  --create-namespace \
  -f custom-values.yaml
```

## Troubleshooting

### Installation Fails

1. **Check pod status:**
   ```bash
   kubectl get pods -n wso2-amp
   kubectl describe pod <pod-name> -n wso2-amp
   ```

2. **Check logs:**
   ```bash
   kubectl logs -n wso2-amp deployment/amp-api
   kubectl logs -n wso2-amp deployment/amp-console
   kubectl logs -n wso2-amp deployment/amp-postgresql
   ```

3. **Check events:**
   ```bash
   kubectl get events -n wso2-amp --sort-by='.lastTimestamp'
   ```

4. **Check Helm release status:**
   ```bash
   helm status amp -n wso2-amp
   helm list -n wso2-amp
   ```

### OpenChoreo Observability Plane Not Found

If you see an error about missing Observability Plane:

```bash
# Verify it exists
kubectl get namespace openchoreo-observability-plane

# If missing, install it first:
helm install observability-plane \
  oci://ghcr.io/openchoreo/helm-charts/openchoreo-observability-plane \
  --version 0.3.2 \
  --namespace openchoreo-observability-plane \
  --create-namespace
```

### Services Not Accessible

1. **Verify port forwarding is running:**
   ```bash
   ps aux | grep port-forward
   ```

2. **Check service endpoints:**
   ```bash
   kubectl get endpoints -n wso2-amp
   ```

3. **Restart port forwarding:**
   ```bash
   ./stop-port-forward.sh
   ./port-forward.sh
   ```

### PostgreSQL Not Ready

If PostgreSQL fails to start:

```bash
# Check PostgreSQL pod logs
kubectl logs -n wso2-amp -l app.kubernetes.io/name=postgresql

# Check persistent volume claims
kubectl get pvc -n wso2-amp

# Check storage class
kubectl get storageclass
```

## Uninstallation

### Uninstall Platform Components

```bash
# Uninstall Agent Management Platform
helm uninstall amp -n wso2-amp

# Uninstall Observability Stack
helm uninstall amp-observability-traces -n openchoreo-observability-plane

# Uninstall Build CI (if installed)
helm uninstall agent-manager-build-ci -n openchoreo-build-plane
```

### Complete Cleanup

```bash
# Delete namespace (removes all resources)
kubectl delete namespace wso2-amp

# Note: Do NOT delete openchoreo-observability-plane as it's shared with OpenChoreo
```

## Default Configuration

### Namespaces

- Agent Management Platform: `wso2-amp` (configurable via `AMP_NS`)
- Observability: `openchoreo-observability-plane` (shared with OpenChoreo)
- Build CI: `openchoreo-build-plane` (shared with OpenChoreo)

### Helm Chart Registry

- Registry: `ghcr.io/wso2`
- Charts:
  - `wso2-ai-agent-management-platform`
  - `wso2-amp-observability-extension`
  - `wso2-amp-build-extension`

### Ports

- Console: 3000
- Agent Manager API: 8080
- Traces Observer: 9098
- OTel Collector: 21893

## See Also

- [Quick Start Guide](./quick-start.md) - Complete setup with Kind and OpenChoreo
- [Multi Cluster Installation](./multi-cluster.md) - Multi-cluster setup
- [Main README](../../README.md) - Project overview and architecture

