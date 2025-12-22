# Quick Start Guide

Get the Agent Management Platform running with a single command using a dev container!

## Prerequisites

Ensure the following before you begin:

- **Docker** (Engine 26.0+ recommended)
    Allocate at least 8 GB RAM and 4 CPUs.

- **Mac users**: Use Colima for best compatibility

  ```sh
  colima start --vm-type=vz --vz-rosetta --cpu 4 --memory 8
  ```

## 🚀 Installation Using Dev Container

The quick-start includes a dev container with all required tools pre-installed (kubectl, Helm, K3d). This ensures a consistent environment across different systems.

### Step 1: Run the Dev Container

```bash
docker run --rm -it --name amp-quick-start \
  -v /var/run/docker.sock:/var/run/docker.sock \
  --network=host \
  ghcr.io/wso2/amp-quick-start:v0.0.0-dev
```

### Step 2: Run Installation Inside Container

Once inside the container, run the installation script:

```bash
./install.sh
```

**Time:** ~15-20 minutes

This installs everything you need:
- ✅ K3d cluster
- ✅ OpenChoreo platform
- ✅ Agent Management Platform
- ✅ Full observability stack

## What Happens During Installation

1. **Prerequisites Check**: Verifies Docker, kubectl, Helm, and K3d are available
2. **Kind Cluster Setup**: Creates a local Kubernetes cluster named `amp-local`
3. **OpenChoreo Installation**: Installs OpenChoreo Control Plane, Data Plane, Build Plane, and Observability Plane
4. **Platform Installation**: Installs Agent Management Platform with PostgreSQL, API, and Console along with platform k8s resources
5. **Observability Setup**: Configures Traces Observer

## Access Your Platform

After installation completes, use the following endpoints to access the platform.

- **Console**: [`http://localhost:3000`](http://localhost:3000)
- **OpenTelemetry Collector**: [`http://localhost:21893`](http://localhost:21893)

## Uninstall

**Platform only:**

```bash
./uninstall.sh
```

## See Also

- [Single Cluster Installation](./install/single-cluster.md) - Install on existing OpenChoreo cluster
- [README](../README.md) - Project overview and architecture

