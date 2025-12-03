# Quick Start Guide

Get the Agent Management Platform running with a single command!

### Prerequisites

- **kubectl** configured to access your cluster
- **Helm** v3.8+ installed
- **kind** 

#### Install kind

*macOS* 

```
brew install kind
```

*Linux* 
```
curl -Lo ./kind https://kind.sigs.k8s.io/dl/latest/kind-linux-amd64"
chmod +x ./kind && sudo mv ./kind /usr/local/bin/kind"
```

## 🚀 One-Command Installation

**Complete setup including Kind cluster and OpenChoreo:**

```bash
cd quick-start
./bootstrap.sh
```

**Time:** ~15-20 minutes  
**Prerequisites:** Docker, kubectl, Helm, kind

This installs everything you need:
- ✅ Kind cluster (local Kubernetes)
- ✅ OpenChoreo platform
- ✅ Agent Management Platform
- ✅ Full observability stack

---

### Skip Specific Steps

#### If you already have a kubernetes cluster

```bash
./bootstrap.sh --skip-kind
```

#### Use existing OpenChoreo installation 

( OpenChoreo cluster (v0.3.2+) with Observability Plane installed )

```bash
./bootstrap.sh --skip-openchoreo
```

#### Platform only (assumes Kind + OpenChoreo exist)
```bash
./bootstrap.sh --skip-kind --skip-openchoreo
```

**Time:** ~5-8 minutes

This installs the Agent Management Platform on your existing OpenChoreo cluster.

## Access Your Platform

After installation completes, your platform is automatically accessible at:

- **Console**: http://localhost:3000
- **API**: http://localhost:8080
- **Traces Observer**: http://localhost:9098
- **Data Prepper**: http://localhost:21893

## What's Included

✅ Agent Management Platform  
✅ Full observability stack with distributed tracing  
✅ PostgreSQL database  
✅ Web console  
✅ Automatic port forwarding

## Next Steps

1. **Open the console**: `open http://localhost:3000`
2. **Deploy a sample agent**: See [sample agents](../runtime/sample-agents/)
3. **View traces**: Navigate to the Observability section in the console

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

**Installation fails?** Run with verbose output:
```bash
./install.sh --verbose
```

**Services not accessible?** Check port forwarding:
```bash
kubectl get pods -n agent-management-platform
kubectl get pods -n openchoreo-observability-plane
```

For more help, see [Detailed Installation Guide](./README.md) or [Troubleshooting Guide](./TROUBLESHOOTING.md)

## Advanced Options

For advanced configuration options, custom values, and detailed documentation, see [README.md](./README.md)
