#!/bin/bash

set -e

# Setup docker socket permissions for wso2-amp user
# This allows k3d and docker commands to work without sudo
if [ -S /var/run/docker.sock ]; then
  DOCKER_SOCK_GID=$(stat -c '%g' /var/run/docker.sock 2>/dev/null || stat -f '%g' /var/run/docker.sock 2>/dev/null || echo "0")

  if [ "$DOCKER_SOCK_GID" != "0" ]; then
    # Create docker group with the same GID as the socket
    if ! getent group "$DOCKER_SOCK_GID" >/dev/null 2>&1; then
      addgroup -g "$DOCKER_SOCK_GID" docker >/dev/null 2>&1 || true
    fi

    # Add wso2-amp user to the docker group
    addgroup wso2-amp docker >/dev/null 2>&1 || true
  fi
fi

# Preserve environment variables by writing them to a file that .bashrc will source
# This ensures DEV_MODE, OPENCHOREO_VERSION, and DEBUG are available after su -
cat > /home/wso2-amp/.env_from_docker <<EOF
export DEV_MODE='${DEV_MODE}'
export OPENCHOREO_VERSION='${OPENCHOREO_VERSION}'
export DEBUG='${DEBUG}'
EOF
chown wso2-amp:wso2-amp /home/wso2-amp/.env_from_docker

# Switch to wso2-amp user and start interactive bash
# The '-' flag starts a login shell, which sources ~/.bash_profile
# which in turn sources ~/.bashrc.
# Note: kubeconfig setup happens in .bashrc automatically
exec su - wso2-amp
