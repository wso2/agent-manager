---
sidebar_position: 3
---
# On Managed Kubernetes

Install the Agent Manager on managed Kubernetes services (AWS EKS, Google GKE, Azure AKS, etc.).

## Overview

This guide walks through deploying the Agent Manager on managed Kubernetes clusters provided by cloud platforms. The installation consists of two main phases:

1. **OpenChoreo Platform Setup** - Install the base OpenChoreo platform (Control Plane, Data Plane, Build Plane, Observability Plane)
2. **Agent Manager Installation** - Install the Agent Manager components on top of OpenChoreo

**Important:** This setup is designed for development and exploration. For production deployments, additional security hardening, proper domain configuration, identity provider integration, and persistent storage are required.

## Prerequisites

### Kubernetes Cluster Requirements

You need a managed Kubernetes cluster with the following specifications:

- **Kubernetes version:** 1.32 or higher
- **Cluster size:** At least 3 nodes
- **Node resources:** Each node should have minimum 4 CPU cores and 8 GB RAM
- **LoadBalancer support:** Cloud provider LoadBalancer service type support (or MetalLB)
- **Public IP accessibility:** LoadBalancer must be publicly accessible for Let's Encrypt HTTP-01 validation

### Supported Cloud Providers

This guide has been tested with:

- **Amazon Web Services (EKS)**
- **Google Cloud Platform (GKE)**
- **Microsoft Azure (AKS)**
- Other managed Kubernetes services with LoadBalancer support

### Required Tools

Before installation, ensure you have the following tools installed:

- **kubectl** (v1.32+) - Kubernetes command-line tool
- **helm** (v3.12+) - Package manager for Kubernetes
- **curl** - Command-line tool for transferring data

Verify tools are installed:

```bash
kubectl version --client
helm version
curl --version
```

### Pre-installed Components

Ensure your cluster has the following components installed:

- **cert-manager** (v1.18.4+) - Required for TLS certificate management

Install cert-manager if not already installed:

```bash
# Install cert-manager with CRDs enabled
helm install cert-manager \
  oci://quay.io/jetstack/charts/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --version v1.18.4 \
  --set crds.enabled=true

# Wait for cert-manager to be ready
kubectl wait --for=condition=Available \
  deployment -l app.kubernetes.io/instance=cert-manager \
  -n cert-manager --timeout=300s
```

### Permissions

Ensure you have sufficient permissions to:

- Create namespaces
- Deploy Helm charts
- Create and manage Kubernetes resources (Deployments, Services, ConfigMaps, Secrets)
- Create LoadBalancer services
- Manage cert-manager Issuers and Certificates
- Access cluster resources via kubectl

## Phase 1: OpenChoreo Platform Setup

The Agent Manager requires a complete OpenChoreo platform installation.

**📚 Base Installation Guide: [OpenChoreo Managed Kubernetes Installation](https://openchoreo.dev/docs/getting-started/try-it-out/on-managed-kubernetes/)**

### Important: Agent Manager-Specific Configuration

While following the OpenChoreo installation guide, apply these Agent Manager-specific configurations:

| Component | Required Values File |
|-----------|---------------------|
| **Build Plane** | [values-bp.yaml](https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.5.0/deployments/single-cluster/values-bp.yaml) |
| **Observability Plane** | [values-op.yaml](https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.5.0/deployments/single-cluster/values-op.yaml) |
| **Control Plane** | [values-cp.yaml](https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.5.0/deployments/single-cluster/values-cp.yaml) |
| **Data Plane** | [values-dp.yaml](https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.5.0/deployments/single-cluster/values-dp.yaml) |

### Installation Steps

Follow the **[OpenChoreo Managed Kubernetes Installation Guide](https://openchoreo.dev/docs/getting-started/try-it-out/on-managed-kubernetes/)** with the following Agent Manager-specific configurations.

#### Step 1: Install OpenChoreo Control Plane

```bash
# Install Control Plane
helm install openchoreo-control-plane \
  oci://ghcr.io/openchoreo/helm-charts/openchoreo-control-plane \
  --version 0.13.0 \
  --namespace openchoreo-control-plane \
  --create-namespace \
  --timeout 600s

# Wait for Control Plane to be ready
kubectl wait --for=condition=Available \
  deployment --all -n openchoreo-control-plane --timeout=600s
```

**Note:** Follow the OpenChoreo guide to configure the domain and TLS after obtaining the LoadBalancer IP.

#### Step 2: Install OpenChoreo Data Plane

```bash
# Set Control Plane domain (obtained from LoadBalancer IP in Step 1)
export CP_DOMAIN="openchoreo.192-168-1-1.nip.io"  # Replace with your domain

# Install Data Plane
helm install openchoreo-data-plane \
  oci://ghcr.io/openchoreo/helm-charts/openchoreo-data-plane \
  --version 0.13.0 \
  --namespace openchoreo-data-plane \
  --create-namespace \
  --timeout 600s \
  --set controlPlane.url=https://${CP_DOMAIN}

# Wait for Data Plane to be ready
kubectl wait --for=condition=Available \
  deployment --all -n openchoreo-data-plane --timeout=600s
```

**Note:** Follow the OpenChoreo guide to register the Data Plane with the Control Plane (creating DataPlane CR with CA certificates).

#### Step 3: Install OpenChoreo Build Plane

```bash
# Install Build Plane (choose registry option based on your needs)
helm install openchoreo-build-plane \
  oci://ghcr.io/openchoreo/helm-charts/openchoreo-build-plane \
  --version 0.13.0 \
  --namespace openchoreo-build-plane \
  --create-namespace \
  --timeout 600s \
  --set controlPlane.url=https://${CP_DOMAIN}

# Wait for Build Plane to be ready
kubectl wait --for=condition=Available \
  deployment --all -n openchoreo-build-plane --timeout=600s

# Save the registry endpoint for later use
export REGISTRY_ENDPOINT="registry.${CP_DOMAIN}"  # Adjust based on your registry configuration
echo "Registry endpoint: ${REGISTRY_ENDPOINT}"
```

**Note:** Follow the OpenChoreo guide to:
- Configure registry endpoint (built-in or cloud provider)
- Register the Build Plane with the Control Plane (creating BuildPlane CR with CA certificates)

#### Step 4: Install OpenChoreo Observability Plane

**Agent Manager requires custom OpenTelemetry Collector configuration:**

```bash
# Create namespace
kubectl create namespace openchoreo-observability-plane

# Apply custom OpenTelemetry Collector ConfigMap (required for Agent Manager)
kubectl apply -f https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.5.0/deployments/values/oc-collector-configmap.yaml \
  -n openchoreo-observability-plane

# Install Observability Plane
helm install openchoreo-observability-plane \
  oci://ghcr.io/openchoreo/helm-charts/openchoreo-observability-plane \
  --version 0.13.0 \
  --namespace openchoreo-observability-plane \
  --timeout 900s \
  --set controlPlane.url=https://${CP_DOMAIN}

# Wait for deployments to be ready
kubectl wait --for=condition=Available \
  deployment --all -n openchoreo-observability-plane --timeout=900s

# Wait for StatefulSets (OpenSearch) to be ready
for sts in $(kubectl get statefulset -n openchoreo-observability-plane -o name 2>/dev/null); do
  kubectl rollout status "${sts}" -n openchoreo-observability-plane --timeout=900s
done
```

**Note:** Follow the OpenChoreo guide to register the Observability Plane with the Control Plane (creating ObservabilityPlane CR with CA certificates).

#### Step 5: Configure Observability Integration

Link the Data Plane and Build Plane to the Observability Plane:

```bash
# Configure DataPlane to use observability plane
kubectl patch dataplane default -n default --type merge \
  -p '{"spec":{"observabilityPlaneRef":"default"}}'

# Configure BuildPlane to use observability plane
kubectl patch buildplane default -n default --type merge \
  -p '{"spec":{"observabilityPlaneRef":"default"}}'
```

**For detailed plane registration steps (extracting CA certificates, creating plane CRs, and configuring domains/TLS), refer to the [OpenChoreo Managed Kubernetes Guide](https://openchoreo.dev/docs/getting-started/try-it-out/on-managed-kubernetes/).**

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

# Verify plane registrations
kubectl get dataplane default -n default
kubectl get buildplane default -n default
kubectl get observabilityplane default -n default
```

All pods should be in `Running` or `Completed` state before proceeding.

### Access OpenChoreo Console

The OpenChoreo console is available at:
- Console: `https://${CP_DOMAIN}`
- API: `https://api.${CP_DOMAIN}`

You can access it directly using the domain configured above.

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
# Version (default: 0.5.0)
export VERSION="0.5.0"

# Helm chart registry
export HELM_CHART_REGISTRY="ghcr.io/wso2"

# Namespaces
export AMP_NS="wso2-amp"
export BUILD_CI_NS="openchoreo-build-plane"
export OBSERVABILITY_NS="openchoreo-observability-plane"
export DEFAULT_NS="default"
export DATA_PLANE_NS="openchoreo-data-plane"
```

### Step 1: Install Gateway Operator

The Gateway Operator manages API Gateway resources and enables secure trace ingestion to the Observability Plane.

```bash
# Install Gateway Operator
helm install gateway-operator \
  oci://ghcr.io/wso2/api-platform/helm-charts/gateway-operator \
  --version 0.2.0 \
  --namespace ${DATA_PLANE_NS} \
  --set logging.level=debug \
  --set gateway.helm.chartVersion=0.3.0 \
  --timeout 600s

# Wait for Gateway Operator deployment
kubectl wait --for=condition=Available \
  deployment -l app.kubernetes.io/name=gateway-operator \
  -n ${DATA_PLANE_NS} --timeout=300s
```

**Apply Gateway Operator Configuration:**

This configuration sets up API authentication (using JWT/JWKS) and rate limiting policies:

```bash
# Apply Gateway Operator configuration
kubectl apply -f https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.5.0/deployments/values/api-platform-operator-full-config.yaml
```

### Step 2: Create Gateway and API Resources

Deploy the observability gateway and trace API endpoint for secure trace ingestion:

```bash
# Apply Observability Gateway
kubectl apply -f https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.5.0/deployments/values/obs-gateway.yaml

# Wait for Gateway to be programmed
kubectl wait --for=condition=Programmed \
  gateway/obs-gateway -n ${DATA_PLANE_NS} --timeout=180s

# Apply OTEL Collector RestApi
kubectl apply -f https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.5.0/deployments/values/otel-collector-rest-api.yaml

# Wait for RestApi to be programmed
kubectl wait --for=condition=Programmed \
  restapi/traces-api-secure -n ${DATA_PLANE_NS} --timeout=120s
```

### Step 3: Install AMP Thunder Extension

The AMP Thunder Extension provides additional identity and authentication features for the Agent Management Platform.

```bash
# Set Thunder namespace
export THUNDER_NS="amp-thunder"

# Install AMP Thunder Extension
helm install amp-thunder-extension \
  oci://${HELM_CHART_REGISTRY}/wso2-amp-thunder-extension \
  --version 0.5.0 \
  --namespace ${THUNDER_NS} \
  --create-namespace \
  --timeout 1800s
```

**Note:** This extension is optional. The platform will function without it, but Thunder extension features may not be available.

### Step 4: Install Agent Manager

The core platform includes:

- PostgreSQL database
- Agent Manager Service (API)
- Console (Web UI)

**Installation:**

Get the observability gateway endpoint for console instrumentation:

```bash
# Get the observability gateway LoadBalancer IP/hostname
OBS_GATEWAY_IP=$(kubectl get svc obs-gateway-gateway-router -n ${DATA_PLANE_NS} \
  -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || \
  kubectl get svc obs-gateway-gateway-router -n ${DATA_PLANE_NS} \
  -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null)

echo "Observability Gateway: ${OBS_GATEWAY_IP}"

# Install the platform Helm chart with instrumentation URL configured
helm install amp \
  oci://${HELM_CHART_REGISTRY}/wso2-agent-manager \
  --version 0.5.0 \
  --namespace ${AMP_NS} \
  --create-namespace \
  --set console.config.instrumentationUrl="http://${OBS_GATEWAY_IP}:22893/otel" \
  --timeout 1800s
```

**Note:** If you're using port-forwarding or exposing the gateway differently, update the `console.config.instrumentationUrl` accordingly.

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

### Step 5: Install Platform Resources Extension

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
  --version 0.5.0 \
  --namespace ${DEFAULT_NS} \
  --timeout 1800s
```

**Note:** This extension is non-fatal if installation fails. The platform will function, but default resources may not be available.

### Step 6: Install Observability Extension

The observability extension includes the Traces Observer service for querying traces from OpenSearch.

**Note:** The OpenTelemetry Collector ConfigMap should have been applied in Phase 1, Step 5. If you skipped it, apply it now:

```bash
# Verify or apply the OpenTelemetry collector config map
kubectl apply -f https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.5.0/deployments/values/oc-collector-configmap.yaml \
  -n ${OBSERVABILITY_NS}
```

**Installation:**

```bash
# Install observability Helm chart
helm install amp-observability-traces \
  oci://${HELM_CHART_REGISTRY}/wso2-amp-observability-extension \
  --version 0.5.0 \
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

### Step 7: Install Build Extension

Install workflow templates for building container images. The Build Extension must be configured to use the **same container registry** that was configured when installing OpenChoreo Build Plane.

**Registry Configuration:**

In Phase 1, Step 4, you installed OpenChoreo Build Plane and configured a registry endpoint. The Agent Manager Build Extension must use the **same registry endpoint**.

**If you saved the REGISTRY_ENDPOINT variable from Step 4:**

```bash
# Verify the registry endpoint (should be set from Phase 1, Step 4)
echo "Using registry endpoint: ${REGISTRY_ENDPOINT}"

# Install Build Extension with the same registry endpoint
helm install build-workflow-extensions \
  oci://${HELM_CHART_REGISTRY}/wso2-amp-build-extension \
  --version 0.5.0 \
  --namespace ${BUILD_CI_NS} \
  --set global.registry.endpoint=${REGISTRY_ENDPOINT} \
  --timeout 1800s
```

**If you need to set the REGISTRY_ENDPOINT again:**

Set it to match what you configured in Phase 1, Step 4:

```bash
# Option 1: If you used OpenChoreo's built-in registry
export REGISTRY_ENDPOINT="registry.${CP_DOMAIN}"

# Option 2: If you used AWS ECR
# export REGISTRY_ENDPOINT="<account-id>.dkr.ecr.<region>.amazonaws.com"

# Option 3: If you used Google GCR/Artifact Registry
# export REGISTRY_ENDPOINT="gcr.io/<project-id>"
# export REGISTRY_ENDPOINT="<region>-docker.pkg.dev/<project-id>/<repository>"

# Option 4: If you used Azure ACR
# export REGISTRY_ENDPOINT="<registry-name>.azurecr.io"

# Install Build Extension
helm install build-workflow-extensions \
  oci://${HELM_CHART_REGISTRY}/wso2-amp-build-extension \
  --version 0.5.0 \
  --namespace ${BUILD_CI_NS} \
  --set global.registry.endpoint=${REGISTRY_ENDPOINT} \
  --timeout 1800s
```

**Verification:**

You can verify the registry endpoint configured in your OpenChoreo Build Plane by checking the workflow templates:

```bash
kubectl get clusterworkflowtemplate ballerina-buildpack-ci -o yaml | grep REGISTRY_ENDPOINT
```

This should show the registry endpoint that OpenChoreo Build Plane is using. Ensure the Agent Manager Build Extension uses the same value.

**Important:** The registry endpoint must be:
- Accessible from both build pods (for pushing images) and kubelet on all nodes (for pulling images)
- The same value configured in Phase 1, Step 4 during OpenChoreo Build Plane installation
- Properly authenticated if using external cloud registries (ECR/GCR/ACR)

**Note:** This extension is optional. The platform will function without it, but build CI features may not work.

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

# 9. Check LoadBalancer IPs
echo "=== LoadBalancer IPs ==="
kubectl get svc -n openchoreo-control-plane -l app.kubernetes.io/name=openchoreo-control-plane
kubectl get svc -n openchoreo-data-plane -l app.kubernetes.io/name=openchoreo-data-plane
```

Expected output should show all pods in `Running` or `Completed` state.

## Access the Platform

### Access via LoadBalancer

The OpenChoreo platform and Agent Manager are accessible through their respective LoadBalancer IPs:

**OpenChoreo Platform:**
- Console: `https://${CP_DOMAIN}`
- API: `https://api.${CP_DOMAIN}`

**Agent Manager:**
- Console: Accessible through OpenChoreo console or via port forwarding (see below)
- API: Accessible via port forwarding (see below)

### Expose Agent Manager via LoadBalancer (Optional)

If you want to expose the Agent Manager console and API through LoadBalancers:

```bash
# Create LoadBalancer service for Agent Manager Console
kubectl patch svc amp-console -n wso2-amp \
  -p '{"spec": {"type": "LoadBalancer"}}'

# Create LoadBalancer service for Agent Manager API
kubectl patch svc amp-api -n wso2-amp \
  -p '{"spec": {"type": "LoadBalancer"}}'

# Get the LoadBalancer IPs
kubectl get svc amp-console -n wso2-amp
kubectl get svc amp-api -n wso2-amp
```

### Port Forwarding (Alternative)

For direct access without exposing through LoadBalancers:

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

**Access URLs (Port Forwarding):**

- **Agent Manager Console**: `http://localhost:3000`
- **Agent Manager API**: `http://localhost:9000`
- **Observability Gateway (HTTP)**: `http://localhost:22893/otel`
- **Observability Gateway (HTTPS)**: `https://localhost:22894/otel`

## Cloud Provider Specific Notes

### AWS EKS

- LoadBalancers are private by default and return a hostname instead of an IP
- You must patch services to be internet-facing before obtaining IPs
- Use `dig` to resolve hostnames to IPs for nip.io domain generation
- Consider using AWS Route53 for proper DNS management in production

### Google Cloud Platform (GKE)

- LoadBalancers return IPs directly, no special handling needed
- Consider using Cloud DNS for proper domain management in production
- Ensure firewall rules allow HTTP/HTTPS traffic to LoadBalancers

### Microsoft Azure (AKS)

- LoadBalancers return IPs directly, no special handling needed
- Consider using Azure DNS for proper domain management in production
- Ensure Network Security Groups allow HTTP/HTTPS traffic to LoadBalancers

## Production Considerations

**Important:** This installation is designed for development and exploration only. For production deployments, you must:

1. **Use proper domains** - Replace nip.io with real, registered domain names
2. **Configure DNS** - Set up proper DNS records pointing to your LoadBalancer IPs
3. **Replace default credentials** - Integrate with a proper identity provider (OAuth, SAML, etc.)
4. **Configure wildcard TLS certificates** - Use DNS-01 validation instead of HTTP-01 to issue wildcard certificates
5. **Implement multi-cluster connectivity** - Configure proper networking between planes if deployed across clusters
6. **Set up persistent observability storage** - Configure persistent volumes and backup strategies for OpenSearch
7. **Resource sizing** - Adjust resource requests/limits based on workload requirements
8. **High availability** - Deploy multiple replicas of critical components across availability zones
9. **Monitoring and alerting** - Set up proper monitoring for production workloads
10. **Security hardening** - Apply security best practices (network policies, RBAC, pod security policies)
11. **Cost optimization** - Use node selectors, taints/tolerations, and autoscaling for cost efficiency
12. **Backup and disaster recovery** - Implement backup strategies for databases and persistent data

## Troubleshooting

### Common Issues

**Pods stuck in Pending state:**
```bash
# Check resource availability
kubectl describe pod <pod-name> -n <namespace>

# Check node resources
kubectl top nodes

# Check persistent volume claims
kubectl get pvc -A
```

**LoadBalancer not getting external IP:**
```bash
# Check service events
kubectl describe svc <service-name> -n <namespace>

# For AWS EKS, ensure the service is internet-facing
kubectl get svc <service-name> -n <namespace> -o yaml | grep aws-load-balancer-scheme
```

**Let's Encrypt certificate not being issued:**
```bash
# Check certificate status
kubectl describe certificate -n <namespace>

# Check cert-manager logs
kubectl logs -n cert-manager -l app=cert-manager

# Ensure LoadBalancer is publicly accessible
curl -v http://<loadbalancer-ip>/.well-known/acme-challenge/test
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
# Verify planeID matches between plane CR and Helm values
kubectl get dataplane default -n default -o yaml
kubectl get buildplane default -n default -o yaml

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

**AWS EKS specific issues:**
```bash
# Check LoadBalancer is internet-facing
kubectl get svc -n <namespace> -o jsonpath='{.metadata.annotations}'

# Verify security groups allow traffic
aws ec2 describe-security-groups --filters "Name=tag:kubernetes.io/cluster/<cluster-name>,Values=owned"
```

## Reference Configuration Files

All configuration values files used in this guide are available in the repository:

- **Control Plane Values**: [deployments/single-cluster/values-cp.yaml](https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.5.0/deployments/single-cluster/values-cp.yaml)
- **Data Plane Values**: [deployments/single-cluster/values-dp.yaml](https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.5.0/deployments/single-cluster/values-dp.yaml)
- **Build Plane Values**: [deployments/single-cluster/values-bp.yaml](https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.5.0/deployments/single-cluster/values-bp.yaml)
- **Observability Plane Values**: [deployments/single-cluster/values-op.yaml](https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.5.0/deployments/single-cluster/values-op.yaml)
- **Gateway Operator Config**: [deployments/values/api-platform-operator-full-config.yaml](https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.5.0/deployments/values/api-platform-operator-full-config.yaml)
- **Observability Gateway**: [deployments/values/obs-gateway.yaml](https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.5.0/deployments/values/obs-gateway.yaml)
- **OTEL Collector ConfigMap**: [deployments/values/oc-collector-configmap.yaml](https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.5.0/deployments/values/oc-collector-configmap.yaml)
- **OTEL Collector RestApi**: [deployments/values/otel-collector-rest-api.yaml](https://raw.githubusercontent.com/wso2/agent-manager/amp/v0.5.0/deployments/values/otel-collector-rest-api.yaml)

You can customize these files for your specific deployment needs.