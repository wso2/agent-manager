#!/bin/bash
set -e

# Get the absolute directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Change to script directory to ensure consistent working directory
cd "$SCRIPT_DIR"
source "$SCRIPT_DIR/env.sh"


# Ensure the k3d cluster holds its network addresses before compose attaches.
# Running this before `docker compose up` means the k3d containers have already
# claimed their addresses, so compose services allocate above them.
# Allows k3d getting intended ips when a cluster restart happens . 

NETWORK="k3d-${CLUSTER_NAME}"
SERVER="k3d-${CLUSTER_NAME}-server-0"
SERVERLB="k3d-${CLUSTER_NAME}-serverlb"


# Util: does this container exist (running or not)?
container_exists() {
    docker container inspect "$1" &>/dev/null
}

# Util: is this container running?
container_running() {
    [ "$(docker container inspect -f '{{.State.Running}}' "$1" 2>/dev/null)" = "true" ]
}

# Util: the address a container asks for on the k3d network ("" if dynamic)
desired_ip() {
    docker container inspect "$1" \
        --format "{{with (index .NetworkSettings.Networks \"${NETWORK}\")}}{{with .IPAMConfig}}{{.IPv4Address}}{{end}}{{end}}" 2>/dev/null
}

# Util: name of the container currently holding an address ("" if free)
ip_holder() {
    docker network inspect "${NETWORK}" \
        --format "{{range .Containers}}{{if eq .IPv4Address \"$1/16\"}}{{.Name}}{{end}}{{end}}" 2>/dev/null
}

# Util: start a container and wait for it to report Running
start_and_wait() {
    local name="$1"
    if container_running "$name"; then
        return 0
    fi
    echo "   Starting ${name}..."
    if ! docker start "$name" >/dev/null 2>&1; then
        return 1
    fi
    for _ in {1..30}; do
        if container_running "$name"; then
            return 0
        fi
        sleep 1
    done
    return 1
}

echo "🔍 Checking k3d cluster '${CLUSTER_NAME}' before starting compose services..."

# No network means no cluster.
if ! docker network inspect "${NETWORK}" &>/dev/null; then
    echo "❌ Docker network '${NETWORK}' not found — the k3d cluster does not exist."
    echo "   Create it first:  make setup-k3d"
    exit 1
fi

if ! container_exists "$SERVER"; then
    echo "❌ Network '${NETWORK}' exists but container '${SERVER}' does not."
    echo "   The cluster is in a partial state. Recreate it:  make setup-k3d"
    exit 1
fi

if container_running "$SERVER" && container_running "$SERVERLB"; then
    echo "✅ k3d cluster is running — its addresses are already claimed"
    exit 0
fi

echo "⚠️  k3d cluster is not fully running. Starting it before compose attaches..."

# Server first: it is the node whose IP k3s persists in its node record, so it
# must claim its address before anything allocating dynamically can take it.
if ! start_and_wait "$SERVER"; then
    want="$(desired_ip "$SERVER")"
    echo "❌ Failed to start ${SERVER}."
    if [ -n "$want" ]; then
        holder="$(ip_holder "$want")"
        if [ -n "$holder" ] && [ "$holder" != "$SERVER" ]; then
            echo "   Its address ${want} is held by '${holder}'."
            echo "   Free it, then retry:  docker stop ${holder} && make dev-up"
            exit 1
        fi
    fi
    echo "   Logs:  docker logs ${SERVER}"
    exit 1
fi

if container_exists "$SERVERLB" && ! start_and_wait "$SERVERLB"; then
    want="$(desired_ip "$SERVERLB")"
    echo "❌ Failed to start ${SERVERLB}."
    if [ -n "$want" ]; then
        holder="$(ip_holder "$want")"
        if [ -n "$holder" ] && [ "$holder" != "$SERVERLB" ]; then
            echo "   Its address ${want} is held by '${holder}'."
            echo "   Free it, then retry:  docker stop ${holder} && make dev-up"
            exit 1
        fi
    fi
    echo "   Logs:  docker logs ${SERVERLB}"
    exit 1
fi

echo "✅ k3d cluster is running — its addresses are now claimed"
