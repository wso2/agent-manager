#!/usr/bin/env bash
#
# Cleans up a self-hosted runner after a run so the next run starts clean.
# Every command is best-effort and the script always exits 0.

set -uo pipefail

PRUNE_IMAGES="${PRUNE_IMAGES:-true}"
REGISTRY="${REGISTRY:-}"
KEEP_CONTAINERS="${KEEP_CONTAINERS:-}"

log()  { echo "  $*"; }
ok()   { echo "✅ $*"; }
warn() { echo "::warning::$*"; }

have() { command -v "$1" >/dev/null 2>&1; }

echo "::group::Teardown — compose stack"

# --- compose --------------------------------------------------------------
# In case a job started the docker compose stack (deployments/standalone).
if have docker && [ -f deployments/standalone/docker-compose.yml ]; then
  ( cd deployments/standalone \
    && docker compose --profile console-image down -v --remove-orphans >/dev/null 2>&1 ) \
    && log "Compose stack removed" || true
fi

echo "::endgroup::"

if ! have docker; then
  ok "Teardown complete (no Docker on this runner)"
  exit 0
fi

echo "::group::Teardown — containers, volumes, networks"

# --- containers ------------------------------------------------------------
# Remove leftover containers, except the runner's own container if it runs
# in one. Skipped if that container cannot be identified.
containerised=false
[ -f /.dockerenv ] && containerised=true
self_cid="$(grep -oE '(/docker/|docker-)[0-9a-f]{64}' /proc/self/cgroup 2>/dev/null \
  | grep -oE '[0-9a-f]{64}' | head -1)"
[ -n "${self_cid}" ] && containerised=true
if [ -n "${self_cid}" ] && ! docker ps -aq --no-trunc 2>/dev/null | grep -qx "${self_cid}"; then
  self_cid=""
fi

skip_containers=false
if [ "${containerised}" = "true" ] && [ -z "${self_cid}" ]; then
  skip_containers=true
  warn "Runner looks containerised but its container id could not be confirmed; skipping the container sweep."
elif [ -n "${self_cid}" ]; then
  log "Runner is containerised (${self_cid:0:12}); it is excluded from the sweep"
fi

leftover=""
[ "${skip_containers}" = "false" ] && leftover="$(docker ps -aq --no-trunc 2>/dev/null)"
if [ -n "${leftover}" ]; then
  for cid in ${leftover}; do
    [ -n "${self_cid}" ] && [ "${cid}" = "${self_cid}" ] && continue
    if [ -n "${KEEP_CONTAINERS}" ]; then
      name="$(docker inspect -f '{{.Name}}' "${cid}" 2>/dev/null | sed 's|^/||')"
      if echo "${name}" | grep -qE "${KEEP_CONTAINERS}"; then
        log "Keeping container ${name} (matches keep-containers)"
        continue
      fi
    fi
    docker rm -f "${cid}" >/dev/null 2>&1 || true
  done
  ok "Removed leftover containers"
else
  log "No containers to remove"
fi

# --- buildx ----------------------------------------------------------------
# Remove buildx builders left by cancelled jobs.
if docker buildx version >/dev/null 2>&1; then
  # Parse the plain table rather than --format, which older buildx lacks.
  docker buildx ls 2>/dev/null | awk '{gsub(/\*$/, "", $1); print $1}' | grep -E '^builder-' | while read -r b; do
    docker buildx rm -f "${b}" >/dev/null 2>&1 && log "Removed buildx builder ${b}" || true
  done
  docker buildx prune -af >/dev/null 2>&1 && log "Pruned buildx cache" || true
fi

# --- images, volumes, networks --------------------------------------------
if [ "${PRUNE_IMAGES}" = "true" ]; then
  # `docker system prune` also removes containers, so use it only when no
  # container has to be kept.
  if [ -z "${KEEP_CONTAINERS}" ] && [ "${skip_containers}" = "false" ]; then
    docker system prune -af --volumes >/dev/null 2>&1 || true
    ok "Pruned all unused images, volumes and networks"
  else
    docker image prune -af >/dev/null 2>&1 || true
    docker volume prune -f >/dev/null 2>&1 || true
    docker network prune -f >/dev/null 2>&1 || true
    ok "Pruned unused images, volumes and networks (containers kept)"
  fi
else
  if [ -z "${KEEP_CONTAINERS}" ] && [ "${skip_containers}" = "false" ]; then
    docker container prune -f >/dev/null 2>&1 || true
  fi
  docker volume prune -f >/dev/null 2>&1 || true
  docker network prune -f >/dev/null 2>&1 || true
  ok "Pruned unused volumes and networks (image cache kept)"
fi

echo "::endgroup::"

echo "::group::Teardown — credentials and workspace"

# --- registry logout -------------------------------------------------------
if [ -n "${REGISTRY}" ]; then
  docker logout "${REGISTRY}" >/dev/null 2>&1 && log "docker logout ${REGISTRY}" || true
fi

# --- workspace leftovers ---------------------------------------------------
rm -rf /tmp/pod-logs >/dev/null 2>&1 || true
rm -f "${HOME}/.kube/config.lock" >/dev/null 2>&1 || true

echo "::endgroup::"

echo "::group::Teardown — remaining host state"
df -h / 2>/dev/null
docker system df 2>/dev/null || true
docker ps -a 2>/dev/null || true
echo "::endgroup::"

ok "Teardown complete"
exit 0
