#!/bin/bash

container_id="$(cat /etc/hostname)"

# Check if the "kind" network exists and connect the container to kind network
if docker network inspect kind &>/dev/null; then
  # Check if the container is already connected
  if [ "$(docker inspect -f '{{json .NetworkSettings.Networks.kind}}' "${container_id}")" = "null" ]; then
    docker network connect "kind" "${container_id}"
    echo "Connected container ${container_id} to kind network."
  else
    echo "Container ${container_id} is already connected to kind network."
  fi
fi

# Fix kubeconfig to use kind cluster's internal network IP instead of 127.0.0.1
if kind get clusters 2>/dev/null | grep -q "openchoreo-local"; then
  CONTROL_PLANE_IP=$(docker inspect openchoreo-local-control-plane --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' 2>/dev/null | head -1)
  if [ -n "$CONTROL_PLANE_IP" ]; then
    echo "Configuring kubectl to connect to kind cluster at ${CONTROL_PLANE_IP}..."
    mkdir -p /state/kube
    kind get kubeconfig --name openchoreo-local | sed "s|server: https://127.0.0.1:[0-9]*|server: https://${CONTROL_PLANE_IP}:6443|" > /state/kube/config-internal.yaml
    export KUBECONFIG=/state/kube/config-internal.yaml
    echo "✓ kubectl configured successfully"
  fi
fi

exec /bin/bash -l