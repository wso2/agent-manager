# Single Cluster Installation

Install the Agent Management Platform on an existing OpenChoreo cluster.

## Prerequisites

### Required Tools

Before installation, ensure you have the following tools installed:

- **kubectl** - Kubernetes command-line tool
- **helm** (v3.x) - Package manager for Kubernetes
- **curl** - Command-line tool for transferring data
- **Docker** - Container runtime (if using k3d for local development)

Verify tools are installed:

```bash
kubectl version --client
helm version
curl --version
docker --version  # If using k3d
```

### OpenChoreo Cluster Requirements

The Agent Management Platform requires an **OpenChoreo cluster (v0.9.0)** with the following components installed:

- **OpenChoreo Control Plane** - Core orchestration and management
- **OpenChoreo Data Plane** - Runtime environment for agents
- **OpenChoreo Build Plane** - Build and CI/CD capabilities
- **OpenChoreo Observability Plane** - Observability and monitoring stack

### Installing OpenChoreo with Custom Values

If you need to install OpenChoreo components, this repository provides custom values files optimized for single-cluster setups:

- **Control Plane**: `deployments/single-cluster/values-cp.yaml`
- **Build Plane**: `deployments/single-cluster/values-bp.yaml`
- **Data Plane**: `deployments/single-cluster/values-dp.yaml`
- **Observability Plane**: `deployments/single-cluster/values-op.yaml`

These values files configure:

- Development mode settings for local development
- Single-cluster installation mode (non-HA)
- Standalone OpenSearch (instead of operator-managed cluster)
- Traefik ingress configuration for k3d
- Cluster gateway configuration
- Enable API Platform

#### Install OpenChoreo Control Plane

```bash

# Install Control Plane
helm install openchoreo-control-plane \
  oci://ghcr.io/openchoreo/helm-charts/openchoreo-control-plane \
  --version 0.9.0 \
  --namespace openchoreo-control-plane \
  --create-namespace \
  --values https://raw.githubusercontent.com/wso2/ai-agent-management-platform/amp/v0.0.0-dev/deployments/single-cluster/values-cp.yaml
```

#### Install OpenChoreo Build Plane

```bash

# Install Build Plane
helm install openchoreo-build-plane \
  oci://ghcr.io/openchoreo/helm-charts/openchoreo-build-plane \
  --version 0.9.0 \
  --namespace openchoreo-build-plane \
  --create-namespace \
  --values https://raw.githubusercontent.com/wso2/ai-agent-management-platform/amp/v0.0.0-dev/deployments/single-cluster/values-bp.yaml
```

#### Install OpenChoreo Data Plane

```bash

# Install Data Plane
helm install openchoreo-data-plane \
  oci://ghcr.io/openchoreo/helm-charts/openchoreo-data-plane \
  --version 0.9.0 \
  --namespace openchoreo-data-plane \
  --create-namespace \
  --values https://raw.githubusercontent.com/wso2/ai-agent-management-platform/amp/v0.0.0-dev/deployments/single-cluster/values-dp.yaml
```

#### Install OpenChoreo Observability Plane

Create namespace _openchoreo-observability-plane_

```bash
kubectl create namespace openchoreo-observability-plane
```

Create the opentelemetry collector config map

```bash
kubectl apply -f https://raw.githubusercontent.com/wso2/ai-agent-management-platform/amp/v0.0.0-dev/deployments/values/oc-collector-configmap.yaml
```

Install the Openchoreo observability plane to the same namespace.

```bash
helm install openchoreo-observability-plane \
  oci://ghcr.io/openchoreo/helm-charts/openchoreo-observability-plane \
  --version 0.9.0 \
  --namespace openchoreo-observability-plane \
  --create-namespace \
  --values https://raw.githubusercontent.com/wso2/ai-agent-management-platform/amp/v0.0.0-dev/deployments/single-cluster/values-op.yaml
```

Follow the [OpenChoreo Single Cluster Setup](https://openchoreo.dev/docs/v0.9.x/getting-started/try-it-out/on-self-hosted-kubernetes/) guide to install cert-manager, create the Gateway TLS certificate, and register the BuildPlane, DataPlane, and Observability Plane with the Control Plane.

### Permissions

Ensure you have sufficient permissions to:

- Create namespaces
- Deploy Helm charts
- Create and manage Kubernetes resources
- Access cluster resources via kubectl

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

The Agent Management Platform installation consists of four main components:

1. **Agent Management Platform** - Core platform (PostgreSQL, API, Console)
2. **Platform Resources Extension** - Default Organization, Project, Environment, DeploymentPipeline
3. **Observability Extension** - Traces Observer service
4. **Build Extension** - Workflow templates for building container images

### Configuration Variables

Set the following environment variables before installation:

```bash
# Version (default: 0.0.0-dev)
export VERSION="0.0.0-dev"

# Helm chart registry
export HELM_CHART_REGISTRY="ghcr.io/wso2"

# Namespaces
export AMP_NS="wso2-amp"
export BUILD_CI_NS="openchoreo-build-plane"
export OBSERVABILITY_NS="openchoreo-observability-plane"
export DEFAULT_NS="default"
export DATA_PLANE_NS="openchoreo-data-plane"
```

### Step 1: Install Agent Management Platform

The core platform includes:

- PostgreSQL database
- Agent Manager Service (API)
- Console (Web UI)

**Installation:**

```bash
# Set configuration variables
export HELM_CHART_REGISTRY="ghcr.io/wso2"
export VERSION="0.0.0-dev"  # Use your desired version
export AMP_NS="wso2-amp"

# Install the platform Helm chart
helm install amp \
  oci://${HELM_CHART_REGISTRY}/wso2-ai-agent-management-platform \
  --version ${VERSION} \
  --namespace ${AMP_NS} \
  --create-namespace \
  --timeout 1800s
```

**Wait for components to be ready:**

```bash
# Wait for PostgreSQL StatefulSet
kubectl wait --for=jsonpath='{.status.readyReplicas}'=1 \
  statefulset/amp-postgresql -n ${AMP_NS} --timeout=600s

# Wait for Agent Manager Service
kubectl wait --for=condition=Available \
  deployment/amp-api -n ${AMP_NS} --timeout=600s

# Wait for Console
kubectl wait --for=condition=Available \
  deployment/amp-console -n ${AMP_NS} --timeout=600s
```

### Step 2: Install Platform Resources Extension

The Platform Resources Extension creates default resources:

- Default Organization
- Default Project
- Environment
- DeploymentPipeline

**Installation:**

```bash
# Install Platform Resources Extension
helm install amp-platform-resources \
  oci://${HELM_CHART_REGISTRY}/wso2-amp-platform-resources-extension \
  --version ${VERSION} \
  --namespace ${DEFAULT_NS} \
  --timeout 1800s
```

**Note:** This extension is non-fatal if installation fails. The platform will function, but default resources may not be available.

### Step 3: Install Observability Extension

The observability extension includes the Traces Observer service for querying traces from OpenSearch.

**Installation:**

```bash
# Set configuration variables
export OBSERVABILITY_NS="openchoreo-observability-plane"

# Install observability Helm chart
helm install amp-observability-traces \
  oci://${HELM_CHART_REGISTRY}/wso2-amp-observability-extension \
  --version ${VERSION} \
  --namespace ${OBSERVABILITY_NS} \
  --timeout 1800s
```

**Wait for Traces Observer to be ready:**

```bash
# Wait for Traces Observer deployment
kubectl wait --for=condition=Available \
  deployment/amp-traces-observer -n ${OBSERVABILITY_NS} --timeout=600s
```

**Note:** This extension is non-fatal if installation fails. The platform will function, but observability features may not work.

### Step 4: Configure Observability Integration

Configure the DataPlane and BuildPlane to use the observability observer:

```bash
# Configure DataPlane observer
kubectl patch dataplane default -n default --type merge \
  -p '{"spec":{"observabilityPlaneRef":"default"}}'

# Configure BuildPlane observer
kubectl patch buildplane default -n default --type merge \
  -p '{"spec":{"observabilityPlaneRef":"default"}}'
```

### Step 5: Install Build Extension

Install workflow templates for building container images:

```bash
# Set configuration variables
export BUILD_CI_NS="openchoreo-build-plane"

# Install Build CI Helm chart
helm install build-workflow-extensions \
  oci://${HELM_CHART_REGISTRY}/wso2-amp-build-extension \
  --version ${VERSION} \
  --namespace ${BUILD_CI_NS} \
  --timeout 1800s
```

**Note:** This extension is non-fatal if installation fails. The platform will function, but build CI features may not work.

### Step 6: Install and Configure Gateway Operator

The Gateway Operator manages API Gateway resources and enables secure trace ingestion.

**Install the Gateway Operator:**


```bash
# Install Gateway Operator
helm install gateway-operator \
  oci://ghcr.io/wso2/api-platform/helm-charts/gateway-operator \
  --version 0.2.0 \
  --namespace ${DATA_PLANE_NS} \
  --create-namespace \
  --set logging.level=debug \
  --set gateway.helm.chartVersion=0.3.0

# Wait for Gateway Operator deployment
kubectl wait --for=condition=Available \
  deployment -l app.kubernetes.io/name=gateway-operator \
  -n ${DATA_PLANE_NS} --timeout=300s

# Configure the Gateway Operator

# Apply the Gateway Operator configuration for API authentication and rate limiting
kubectl apply -f https://raw.githubusercontent.com/wso2/ai-agent-management-platform/amp/v0.3.0/deployments/values/api-platform-operator-full-config.yaml
```

**Note:** For local development, you may need to update the JWKS URI in the configuration to use `http://host.docker.internal:9000/auth/external/jwks.json` instead of the cluster-internal service URL.

**Create Gateway and API Resources:**

```bash
# Apply Observability Gateway
kubectl apply -f https://raw.githubusercontent.com/wso2/ai-agent-management-platform/amp/v0.3.0/deployments/values/obs-gateway.yaml

# Wait for Gateway to be programmed
kubectl wait --for=condition=Programmed \
  gateway/obs-gateway -n ${DATA_PLANE_NS} --timeout=180s

# Apply OTEL Collector RestApi
kubectl apply -f https://raw.githubusercontent.com/wso2/ai-agent-management-platform/amp/v0.3.0/deployments/values/otel-collector-rest-api.yaml

# Wait for RestApi to be programmed
kubectl wait --for=condition=Programmed \
  restapi/traces-api-secure -n ${DATA_PLANE_NS} --timeout=120s
```

## Verification

Verify all components are installed and running:

```bash
# Check Agent Management Platform pods
kubectl get pods -n wso2-amp

# Check Observability pods
kubectl get pods -n openchoreo-observability-plane | grep -E "amp-traces-observer"

# Check Build CI pods (if installed)
kubectl get pods -n openchoreo-build-plane | grep build-workflow

# Check Gateway Operator
kubectl get pods -n openchoreo-data-plane -l app.kubernetes.io/name=gateway-operator

# Check Gateway and API resources
kubectl get gateway obs-gateway -n openchoreo-data-plane
kubectl get restapi traces-api-secure -n openchoreo-data-plane

# Check Helm releases
helm list -n wso2-amp
helm list -n openchoreo-observability-plane
helm list -n openchoreo-build-plane
helm list -n default

# Verify DataPlane and BuildPlane observer configuration
kubectl get dataplane default -n default -o jsonpath='{.spec.observer}' | jq
kubectl get buildplane default -n default -o jsonpath='{.spec.observer}' | jq
```

Expected output should show all pods in `Running` or `Completed` state.

## Access the Platform

### Port Forwarding

Set up port forwarding to access the services locally:

```bash
# Console (port 3000)
kubectl port-forward -n wso2-amp svc/amp-console 3000:3000 &

# Agent Manager API (port 8080)
kubectl port-forward -n wso2-amp svc/amp-api 9000:9000 &

# Port forward Observability Gateway
echo "🌐 Forwarding Observability Gateway HTTP (22893)..."
kubectl port-forward -n openchoreo-data-plane svc/obs-gateway-gateway-router 22893:22893 &

# Port forward Observability Gateway
echo "🌐 Forwarding Observability Gateway HTTPS (22894)..."
kubectl port-forward -n openchoreo-data-plane svc/obs-gateway-gateway-router 22894:22894 &


```

### Access URLs

After port forwarding is set up:

- **Console**: http://localhost:3000
- **API**: http://localhost:9000
- **Observabliity Gateway**: http://localhost:22893/otel
- **Observability Gateway (HTTPS)** https://localhost:22894/otel

### Handling Self-Signed Certificate Issues (HTTPS)

If you need to use the HTTPS endpoint for OTEL exporters and encounter self-signed certificate issues, you can extract and use the certificate authority (CA) certificate from the cluster:

```bash
# Extract the CA certificate from the Kubernetes secret
kubectl get secret obs-gateway-gateway-controller-tls \
  -n openchoreo-data-plane \
  -o jsonpath='{.data.ca\.crt}' | base64 --decode > ca.crt

# Export the certificate path for OTEL exporters (use absolute path to the ca.crt file)
export OTEL_EXPORTER_OTLP_CERTIFICATE=$(pwd)/ca.crt
```


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
  --version ${VERSION} \
  --namespace ${AMP_NS} \
  --create-namespace \
  --timeout 1800s \
  -f custom-values.yaml
```

## See Also

- [Quick Start Guide](../quick-start.md) - Complete setup with k3d and OpenChoreo
- [Main README](../../README.md) - Project overview and architecture
- [OpenChoreo Documentation](https://openchoreo.dev/docs/v0.9.x/) - OpenChoreo setup and configuration
