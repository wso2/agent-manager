---
sidebar_position: 2
---
# On Self-Hosted Kubernetes

Install the Agent Manager on a self-hosted Kubernetes cluster with OpenChoreo.

## Overview

This guide walks through deploying the Agent Manager on a self-hosted Kubernetes cluster. The installation consists of two main phases:

1. **OpenChoreo Platform Setup** - Install the base OpenChoreo platform (Control Plane, Data Plane, Build Plane, Observability Plane)
2. **Agent Manager Installation** - Install the Agent Manager components on top of OpenChoreo

**Important:** This setup is designed for development and exploration. For production deployments, additional security hardening, TLS configuration, and identity provider integration are required.

## Prerequisites

### Hardware Requirements

- **Minimum Resources:**
  - 8 GB RAM
  - 4 CPU cores
  - ~10 GB free disk space

### Required Tools

Before installation, ensure you have the following tools installed:

- **Docker** (v26.0+) - Container runtime
- **kubectl** (v1.32+) - Kubernetes command-line tool
- **helm** (v3.12+) - Package manager for Kubernetes
- **k3d** (v5.8+) - Lightweight Kubernetes for local development (optional, for local clusters)

**Platform-Specific Notes:**
- **macOS users:** Use Colima with VZ and Rosetta support
- **Rancher Desktop users:** Must use containerd and configure HTTP registry access for the Build Plane

Verify tools are installed:

```bash
docker --version
kubectl version --client
helm version
k3d version  # If using k3d for local development
```

### For Existing Kubernetes Clusters

If you have an existing Kubernetes cluster, ensure:

- Kubernetes 1.32+ is running
- cert-manager is pre-installed
- An ingress controller is configured
- Cluster has minimum 8 GB RAM and 4 CPU cores

### Permissions

Ensure you have sufficient permissions to:

- Create namespaces
- Deploy Helm charts
- Create and manage Kubernetes resources (Deployments, Services, ConfigMaps, Secrets)
- Access cluster resources via kubectl

## Phase 1: OpenChoreo Platform Setup

The Agent Manager requires a complete OpenChoreo platform installation consisting of four planes:

1. **Control Plane** - Core orchestration and management
2. **Data Plane** - Runtime environment for agents
3. **Build Plane** - Build and CI/CD capabilities
4. **Observability Plane** - Monitoring and observability stack

### Step 1: Install OpenChoreo Control Plane

The Control Plane provides the core orchestration layer. It will be accessible at `http://openchoreo.localhost:8080`.

```bash
# Install Control Plane
helm install openchoreo-control-plane \
  oci://ghcr.io/openchoreo/helm-charts/openchoreo-control-plane \
  --version 0.9.0 \
  --namespace openchoreo-control-plane \
  --create-namespace \
  --values https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.0.0-dev/deployments/single-cluster/values-cp.yaml

# Wait for Control Plane to be ready
kubectl wait --for=condition=Available \
  deployment -l app.kubernetes.io/name=openchoreo-control-plane \
  -n openchoreo-control-plane --timeout=600s
```

### Step 2: Install OpenChoreo Data Plane

The Data Plane provides the runtime environment for deployed agents.

```bash
# Install Data Plane
helm install openchoreo-data-plane \
  oci://ghcr.io/openchoreo/helm-charts/openchoreo-data-plane \
  --version 0.9.0 \
  --namespace openchoreo-data-plane \
  --create-namespace \
  --values https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.0.0-dev/deployments/single-cluster/values-dp.yaml

# Wait for Data Plane to be ready
kubectl wait --for=condition=Available \
  deployment -l app.kubernetes.io/name=openchoreo-data-plane \
  -n openchoreo-data-plane --timeout=600s
```

**Important:** Create a TLS certificate for the Data Plane gateway and register the DataPlane custom resource with the Control Plane. The `planeID` in the DataPlane CR must match the `clusterAgent.planeId` Helm value. Refer to the [OpenChoreo documentation](https://openchoreo.dev/docs/v0.9.x/getting-started/try-it-out/on-self-hosted-kubernetes/) for detailed steps.

### Step 3: Install OpenChoreo Build Plane

The Build Plane enables CI/CD capabilities for building container images and deploys a container registry.

**Install Build Plane with Registry Configuration:**

The Build Plane installation includes a container registry. The registry endpoint must be configured to be accessible from both build pods (for pushing images) and kubelet (for pulling images).

For single-cluster k3d setups, use the provided values file which configures the registry endpoint as `host.k3d.internal:10082`:

```bash
# Install Build Plane with k3d registry configuration
helm install openchoreo-build-plane \
  oci://ghcr.io/openchoreo/helm-charts/openchoreo-build-plane \
  --version 0.9.0 \
  --namespace openchoreo-build-plane \
  --create-namespace \
  --values https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.0.0-dev/deployments/single-cluster/values-bp.yaml

# Wait for Build Plane to be ready
kubectl wait --for=condition=Available \
  deployment -l app.kubernetes.io/name=openchoreo-build-plane \
  -n openchoreo-build-plane --timeout=600s

# Wait for registry to be ready
kubectl wait --for=condition=Available \
  deployment/registry -n openchoreo-build-plane --timeout=300s
```

**Important - Registry Endpoint Configuration:**

The `values-bp.yaml` file configures the registry endpoint as:
```yaml
global:
  defaultResources:
    registry:
      endpoint: host.k3d.internal:10082
```

This endpoint (`host.k3d.internal:10082`) will be used later when installing the Agent Manager Build Extension in Phase 2, Step 6.

**HTTP Registry Access:**

The OpenChoreo Build Plane uses an HTTP registry (not HTTPS). If you encounter image pull failures with errors like `"http: server gave HTTP response to HTTPS client"`, configure your container runtime to allow HTTP access:

- **Rancher Desktop**: Configure insecure registries following [Rancher Desktop - Configuring Private Registries](https://docs.rancherdesktop.io/how-to-guides/mirror-private-registry/)
- **Docker Desktop/Colima**: Add the registry to Docker daemon's insecure registries list
- **k3d**: Automatically handles HTTP registry configuration

For detailed Build Plane configuration and alternative registry endpoints, refer to the [OpenChoreo Build Plane documentation](https://openchoreo.dev/docs/v0.9.x/getting-started/try-it-out/on-self-hosted-kubernetes/).

### Step 4: Install OpenChoreo Observability Plane

The Observability Plane integrates with OpenSearch for monitoring and observability.

```bash
# Create namespace
kubectl create namespace openchoreo-observability-plane

# Create the OpenTelemetry collector ConfigMap
kubectl apply -f https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.0.0-dev/deployments/values/oc-collector-configmap.yaml

# Install Observability Plane
helm install openchoreo-observability-plane \
  oci://ghcr.io/openchoreo/helm-charts/openchoreo-observability-plane \
  --version 0.9.0 \
  --namespace openchoreo-observability-plane \
  --values https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.0.0-dev/deployments/single-cluster/values-op.yaml

# Wait for Observability Plane to be ready
kubectl wait --for=condition=Available \
  deployment -l app.kubernetes.io/name=openchoreo-observability-plane \
  -n openchoreo-observability-plane --timeout=600s
```

### Step 5: Complete OpenChoreo Configuration

Follow the [OpenChoreo Single Cluster Setup Guide](https://openchoreo.dev/docs/v0.9.x/getting-started/try-it-out/on-self-hosted-kubernetes/) to complete:

1. Install cert-manager (if not already installed)
2. Create Gateway TLS certificates
3. Register BuildPlane, DataPlane, and ObservabilityPlane with the Control Plane

### Verify OpenChoreo Installation

Before proceeding to Agent Manager installation, verify all OpenChoreo components are running:

```bash
# Check all OpenChoreo namespaces exist
kubectl get namespace openchoreo-control-plane
kubectl get namespace openchoreo-data-plane
kubectl get namespace openchoreo-build-plane
kubectl get namespace openchoreo-observability-plane

# Verify pods are running
kubectl get pods -n openchoreo-control-plane
kubectl get pods -n openchoreo-data-plane
kubectl get pods -n openchoreo-build-plane
kubectl get pods -n openchoreo-observability-plane

# Check OpenSearch is available (required for Agent Manager)
kubectl get pods -n openchoreo-observability-plane -l app=opensearch
```

All pods should be in `Running` or `Completed` state before proceeding.

### Access OpenChoreo Console

The OpenChoreo console is available at:
- Console: `http://openchoreo.localhost:8080`
- API: `http://api.openchoreo.localhost:8080`
- Default credentials: `admin@openchoreo.dev` / `Admin@123`

**Security Note:** For production use, replace default credentials with a proper identity provider.

## Phase 2: Agent Manager Installation

Now that OpenChoreo is installed, you can install the Agent Manager components.

The Agent Manager installation consists of four main components:

1. **Agent Manager** - Core platform (PostgreSQL, API, Console)
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

### Step 1: Install Agent Manager

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
  oci://${HELM_CHART_REGISTRY}/wso2-agent-manager \
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

### Step 4: Install and Configure Gateway Operator

The Gateway Operator manages API Gateway resources and enables secure trace ingestion.

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

# Apply the Gateway Operator configuration for API authentication and rate limiting
kubectl apply -f https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.0.0-dev/deployments/values/api-platform-operator-full-config.yaml
```

**Note:** For local development, you may need to update the JWKS URI in the configuration to use `http://host.docker.internal:9000/auth/external/jwks.json` instead of the cluster-internal service URL.

**Create Gateway and API Resources:**

```bash
# Apply Observability Gateway
kubectl apply -f https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.0.0-dev/deployments/values/obs-gateway.yaml

# Wait for Gateway to be programmed
kubectl wait --for=condition=Programmed \
  gateway/obs-gateway -n ${DATA_PLANE_NS} --timeout=180s

# Apply OTEL Collector RestApi
kubectl apply -f https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.0.0-dev/deployments/values/otel-collector-rest-api.yaml

# Wait for RestApi to be programmed
kubectl wait --for=condition=Programmed \
  restapi/traces-api-secure -n ${DATA_PLANE_NS} --timeout=120s
```

### Step 5: Configure Observability Integration

Configure the DataPlane and BuildPlane to use the observability plane:

```bash
# Configure DataPlane observer
kubectl patch dataplane default -n default --type merge \
  -p '{"spec":{"observabilityPlaneRef":"default"}}'

# Configure BuildPlane observer
kubectl patch buildplane default -n default --type merge \
  -p '{"spec":{"observabilityPlaneRef":"default"}}'
```

### Step 6: Install Build Extension

Install workflow templates for building container images. The Build Extension must be configured to use the **same container registry** that was installed by OpenChoreo Build Plane.

**Registry Configuration:**

In Phase 1, Step 3, you installed OpenChoreo Build Plane which deployed a container registry with the endpoint `host.k3d.internal:10082` (configured in `deployments/single-cluster/values-bp.yaml`).

The Agent Manager Build Extension must point to this same registry endpoint:

```bash
# Set the registry endpoint (must match OpenChoreo Build Plane configuration)
# This value comes from the values-bp.yaml used in Phase 1, Step 3
export REGISTRY_ENDPOINT="host.k3d.internal:10082"

# Install Build Extension with the same registry endpoint
helm install build-workflow-extensions \
  oci://${HELM_CHART_REGISTRY}/wso2-amp-build-extension \
  --version ${VERSION} \
  --namespace ${BUILD_CI_NS} \
  --set global.registry.endpoint=${REGISTRY_ENDPOINT} \
  --timeout 1800s
```

**If You Used a Different Registry Endpoint:**

If you installed OpenChoreo Build Plane with a different registry endpoint configuration (not using the provided `values-bp.yaml`), make sure to use the **same endpoint** you configured in the Build Plane installation.

Common alternative endpoints:
- **Docker Desktop / Colima:** `host.docker.internal:10082`
- **Node IP based:** `<node-ip>:10082`
- **In-cluster only:** `registry.openchoreo-build-plane.svc.cluster.local:5000`

**Verification:**

You can verify the registry endpoint configured in your OpenChoreo Build Plane by checking the workflow templates:

```bash
kubectl get clusterworkflowtemplate ballerina-buildpack-ci -o yaml | grep REGISTRY_ENDPOINT
```

**Note:** This extension is non-fatal if installation fails. The platform will function, but build CI features may not work.

## Verification

Verify all components are installed and running:

```bash
# 1. Check OpenChoreo Platform Components
echo "=== OpenChoreo Platform Status ==="
kubectl get pods -n openchoreo-control-plane
kubectl get pods -n openchoreo-data-plane
kubectl get pods -n openchoreo-build-plane
kubectl get pods -n openchoreo-observability-plane

# 2. Check Agent Manager Components
echo "=== Agent Manager Status ==="
kubectl get pods -n wso2-amp

# 3. Check Observability Extension
echo "=== Observability Extension Status ==="
kubectl get pods -n openchoreo-observability-plane | grep -E "amp-traces-observer"

# 4. Check Build Extension
echo "=== Build Extension Status ==="
kubectl get pods -n openchoreo-build-plane | grep build-workflow

# 5. Check Gateway Operator
echo "=== Gateway Operator Status ==="
kubectl get pods -n openchoreo-data-plane -l app.kubernetes.io/name=gateway-operator

# 6. Check Gateway and API Resources
echo "=== Gateway and API Resources ==="
kubectl get gateway obs-gateway -n openchoreo-data-plane
kubectl get restapi traces-api-secure -n openchoreo-data-plane

# 7. Check Helm Releases
echo "=== Helm Releases ==="
helm list -n openchoreo-control-plane
helm list -n openchoreo-data-plane
helm list -n openchoreo-build-plane
helm list -n openchoreo-observability-plane
helm list -n wso2-amp
helm list -n default

# 8. Verify Plane Registrations
echo "=== Plane Registrations ==="
kubectl get dataplane default -n default -o jsonpath='{.spec.observabilityPlaneRef}'
kubectl get buildplane default -n default -o jsonpath='{.spec.observabilityPlaneRef}'
```

Expected output should show all pods in `Running` or `Completed` state.

## Access the Platform

### Access via Ingress (Recommended)

If you're using the provided k3d/Traefik ingress configuration, the services are accessible directly:

**OpenChoreo Platform:**
- Console: `http://openchoreo.localhost:8080`
- API: `http://api.openchoreo.localhost:8080`
- Default credentials: `admin@openchoreo.dev` / `Admin@123`

**Agent Manager:**
- Console: Access through OpenChoreo console or via port forwarding (see below)
- API: Access via port forwarding (see below)

### Port Forwarding (Alternative)

For direct access or non-ingress setups, use port forwarding:

```bash
# Agent Manager Console (port 3000)
kubectl port-forward -n wso2-amp svc/amp-console 3000:3000 &

# Agent Manager API (port 9000)
kubectl port-forward -n wso2-amp svc/amp-api 9000:9000 &

# Observability Gateway HTTP (port 22893)
kubectl port-forward -n openchoreo-data-plane svc/obs-gateway-gateway-router 22893:22893 &

# Observability Gateway HTTPS (port 22894)
kubectl port-forward -n openchoreo-data-plane svc/obs-gateway-gateway-router 22894:22894 &
```

### Access URLs (Port Forwarding)

After port forwarding is set up:

- **Agent Manager Console**: `http://localhost:3000`
- **Agent Manager API**: `http://localhost:9000`
- **Observability Gateway (HTTP)**: `http://localhost:22893/otel`
- **Observability Gateway (HTTPS)**: `https://localhost:22894/otel`

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
  oci://${HELM_CHART_REGISTRY}/wso2-agent-manager \
  --version ${VERSION} \
  --namespace ${AMP_NS} \
  --create-namespace \
  --timeout 1800s \
  -f custom-values.yaml
```

## Production Considerations

**Important:** This installation is designed for development and exploration only. For production deployments, you must:

1. **Replace default credentials** with a proper identity provider (OAuth, SAML, etc.)
2. **Configure TLS certificates** - Replace self-signed certificates with proper CA-signed certificates
3. **Implement multi-cluster connectivity** - Configure proper networking between planes
4. **Set up persistent observability storage** - Configure persistent volumes and backup strategies for OpenSearch
5. **Resource sizing** - Adjust resource requests/limits based on workload requirements
6. **High availability** - Deploy multiple replicas of critical components
7. **Monitoring and alerting** - Set up proper monitoring for production workloads
8. **Security hardening** - Apply security best practices (network policies, RBAC, pod security policies)

## Troubleshooting

### Common Issues

**Pods stuck in Pending state:**
```bash
# Check resource availability
kubectl describe pod <pod-name> -n <namespace>

# Check node resources
kubectl top nodes
```

**Gateway not becoming Programmed:**
```bash
# Check Gateway Operator logs
kubectl logs -n openchoreo-data-plane -l app.kubernetes.io/name=gateway-operator

# Check Gateway status
kubectl describe gateway obs-gateway -n openchoreo-data-plane
```

**Plane registration issues:**
```bash
# Verify planeID matches between DataPlane CR and Helm values
kubectl get dataplane default -n default -o yaml

# Check Control Plane logs
kubectl logs -n openchoreo-control-plane -l app.kubernetes.io/name=openchoreo-control-plane
```

**OpenSearch connectivity issues:**
```bash
# Check OpenSearch pods
kubectl get pods -n openchoreo-observability-plane -l app=opensearch

# Test OpenSearch connectivity
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
  curl -v http://opensearch.openchoreo-observability-plane.svc.cluster.local:9200
```

## See Also

- [Quick Start Guide](./quick-start.md) - Complete automated setup with k3d and OpenChoreo
- [What is Agent Manager](../overview/what-is-amp.md) - Project overview and architecture
- [OpenChoreo Documentation](https://openchoreo.dev/docs/v0.9.x/) - Official OpenChoreo setup and configuration
- [OpenChoreo Self-Hosted Kubernetes Guide](https://openchoreo.dev/docs/v0.9.x/getting-started/try-it-out/on-self-hosted-kubernetes/) - Detailed OpenChoreo deployment guide