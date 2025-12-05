# Quick Start Guide

Get the Agent Management Platform running with a single command using a dev container!

## Prerequisites

- **Docker** (Docker Desktop or Colima)
- **Docker Compose** (optional, for easier container management)

## 🚀 Installation Using Dev Container

The quick-start includes a dev container with all required tools pre-installed (kubectl, Helm, Kind). This ensures a consistent environment across different systems.

### Step 1: Run the Dev Container

From the `quick-start` directory:

```bash
docker run -it --rm \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v $(pwd):/workspace \
  -p 3000:3000 \
  -p 8080:8080 \
  -p 9098:9098 \
  -p 21893:21893 \
  ghcr.io/wso2/amp-quick-start:0.0.1
```

### Step 2: Run Installation Inside Container

Once inside the container, run the installation script:

```bash
cd /workspace
./install.sh
```

**Time:** ~15-20 minutes

This installs everything you need:
- ✅ Kind cluster (local Kubernetes)
- ✅ OpenChoreo platform
- ✅ Agent Management Platform
- ✅ Full observability stack

## What Happens During Installation

1. **Prerequisites Check**: Verifies Docker, kubectl, Helm, and Kind are available
2. **Kind Cluster Setup**: Creates a local Kubernetes cluster named `openchoreo-local`
3. **OpenChoreo Installation**: Installs OpenChoreo Control Plane, Data Plane, Build Plane, and Observability Plane
4. **Platform Installation**: Installs Agent Management Platform with PostgreSQL, API, and Console
5. **Observability Setup**: Configures DataPrepper and Traces Observer
6. **Port Forwarding**: Automatically sets up port forwarding for all services

## Access Your Platform

After installation completes, your platform is automatically accessible at:

- **Console**: http://localhost:3000
- **API**: http://localhost:8080
- **Traces Observer**: http://localhost:9098
- **Data Prepper**: http://localhost:21893

## Uninstall

**Platform only:**

```bash
./uninstall.sh
```

**Complete cleanup (including Kind cluster):**

```bash
./uninstall.sh --force --delete-namespaces
kind delete cluster --name openchoreo-local
```

## Troubleshooting

### Installation fails?

Run with verbose output:

```bash
./install.sh --verbose
```

### Services not accessible?

Check port forwarding and pod status:

```bash
# Check pods
kubectl get pods -n wso2-amp
kubectl get pods -n openchoreo-observability-plane

# Check port forwarding
ps aux | grep port-forward

# Restart port forwarding
./stop-port-forward.sh
./port-forward.sh
```

### Container can't access Kind cluster?

The container automatically connects to the `kind` network. If issues persist:

1. Verify Kind cluster exists: `kind get clusters`
2. Check Docker network: `docker network inspect kind`
3. Restart the container

### Docker socket permission issues?

On Linux, you may need to add your user to the docker group:

```bash
sudo usermod -aG docker $USER
# Log out and back in
```

## See Also

- [Single Cluster Installation](./single-cluster.md) - Install on existing OpenChoreo cluster
- [Main README](../../README.md) - Project overview and architecture

