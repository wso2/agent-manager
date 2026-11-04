#!/usr/bin/env bash
#
# Checks that the runner can host the k3d cluster before the install starts.

set -uo pipefail

PROBE_PORT="${PROBE_PORT:-6551}"
# Small image used to test port publishing.
PROBE_IMAGE="${PROBE_IMAGE:-public.ecr.aws/docker/library/busybox:1.37}"
REQUIRED_PORTS="${REQUIRED_PORTS:-6550 8080 8443 10082 11080 11082 11085 19080 19443}"
MIN_CPUS="${MIN_CPUS:-4}"
MIN_MEM_GB="${MIN_MEM_GB:-16}"
MIN_DISK_GB="${MIN_DISK_GB:-60}"
# "true" turns the capacity checks below from warnings into a hard failure.
ENFORCE_CAPACITY="${ENFORCE_CAPACITY:-false}"

log()  { echo "  $*"; }
ok()   { echo "✅ $*"; }
warn() { echo "::warning::$*"; }
die()  { echo "::error::$*"; exit 1; }

failed=0

# ---------------------------------------------------------------------------
# Host capacity
#
# Warns when below the minimums, or fails with ENFORCE_CAPACITY=true.
# ---------------------------------------------------------------------------

echo "::group::Host capacity"
# Treat a non-numeric value as 0.
numeric_or_zero() {
  case "$1" in
    ''|*[!0-9]*) echo 0 ;;
    *) echo "$1" ;;
  esac
}
cpus="$(numeric_or_zero "$(nproc 2>/dev/null)")"
mem_gb="$(numeric_or_zero "$(awk '/MemTotal/ {printf "%d", $2/1024/1024}' /proc/meminfo 2>/dev/null)")"
disk_gb="$(numeric_or_zero "$(df -BG --output=avail / 2>/dev/null | tail -1 | tr -dc '0-9')")"
log "vCPUs: ${cpus} (want >= ${MIN_CPUS})"
log "Memory: ${mem_gb} GiB (want >= ${MIN_MEM_GB})"
log "Free disk on /: ${disk_gb} GiB (want >= ${MIN_DISK_GB})"
capacity_short=0
[ "${cpus:-0}" -lt "${MIN_CPUS}" ] && { capacity_short=1; warn "Runner has ${cpus} vCPUs; the full AMP stack needs about ${MIN_CPUS}. Expect pods Pending with 'Insufficient cpu'."; }
[ "${mem_gb:-0}" -lt "${MIN_MEM_GB}" ] && { capacity_short=1; warn "Runner has ${mem_gb} GiB RAM; the full AMP stack (OpenSearch, per-environment Thunder and gateway stacks) needs about ${MIN_MEM_GB}."; }
[ "${disk_gb:-0}" -lt "${MIN_DISK_GB}" ] && { capacity_short=1; warn "Only ${disk_gb} GiB free on /; image pulls plus k3d state need about ${MIN_DISK_GB}. k3s will start evicting pods once imagefs drops below 10%."; }
if [ "${capacity_short}" -eq 1 ] && [ "${ENFORCE_CAPACITY}" = "true" ]; then
  failed=1
  echo "::error::Host is below the minimum capacity for the nightly e2e job (${cpus} vCPU / ${mem_gb} GiB / ${disk_gb} GiB free; need >= ${MIN_CPUS} / ${MIN_MEM_GB} / ${MIN_DISK_GB}). Resize the VM or run on a GitHub-hosted runner."
fi
echo "::endgroup::"

# ---------------------------------------------------------------------------
# Docker reachability
# ---------------------------------------------------------------------------

echo "::group::Docker daemon"
if ! docker info >/dev/null 2>&1; then
  die "Docker is not reachable from this runner. Check 'systemctl status docker' and that $(id -un) can use the socket."
fi
log "DOCKER_HOST=${DOCKER_HOST:-<unset>}"
log "context: $(docker context show 2>/dev/null || echo '<none>')"
log "daemon:  $(docker info --format '{{.Name}} / {{.ServerVersion}} / {{.OperatingSystem}}' 2>/dev/null)"
echo "::endgroup::"

# ---------------------------------------------------------------------------
# Published ports are reachable from the runner
# ---------------------------------------------------------------------------

echo "::group::Docker published-port reachability"
docker rm -f amp-portprobe >/dev/null 2>&1 || true
if ! docker run -d --rm --name amp-portprobe \
      -p "127.0.0.1:${PROBE_PORT}:80" "${PROBE_IMAGE}" \
      httpd -f -p 80 -h /etc >/dev/null 2>&1; then
  docker rm -f amp-portprobe >/dev/null 2>&1 || true
  die "Could not start the port probe container from ${PROBE_IMAGE}. Either the daemon cannot pull images, or host port ${PROBE_PORT} is already bound."
fi

reachable=0
for _ in $(seq 1 15); do
  if curl -fsS -m 2 "http://127.0.0.1:${PROBE_PORT}/hostname" >/dev/null 2>&1; then
    reachable=1
    break
  fi
  sleep 1
done
docker rm -f amp-portprobe >/dev/null 2>&1 || true

if [ "${reachable}" -eq 1 ]; then
  ok "Container ports published by this daemon are reachable on the runner's loopback"
else
  failed=1
  echo "::error::A container port published to 127.0.0.1:${PROBE_PORT} is NOT reachable from this runner."
  cat <<'EOF'
  This is the failure mode from PR #1233. The Docker daemon publishes ports
  into its own network namespace, not this runner's, so k3d will create the
  cluster successfully and then every kubectl call against the published API
  port (kubeAPI 6550) will hang until quick-start/install.sh times out.

  It means the runner is not its own Docker host — typically Docker-in-Docker,
  a containerised runner sharing the host daemon, or a remote DOCKER_HOST.

  Fixes, in order of preference:
    1. Run the runner agent directly on the VM (as a systemd service) against
       a local Docker Engine, so published ports land on the runner itself.
    2. If the runner must stay containerised, give it the daemon's network
       namespace (--network host against a local daemon), so published ports
       and the runner share one namespace.
EOF
fi
echo "::endgroup::"

# ---------------------------------------------------------------------------
# Host ports the k3d config publishes
#
# Fails if a port is still in use, for example by a previous run.
# ---------------------------------------------------------------------------

echo "::group::Required host ports"
busy=""
for p in ${REQUIRED_PORTS}; do
  if holder="$(lsof -nP -iTCP:"${p}" -sTCP:LISTEN 2>/dev/null | tail -n +2)" && [ -n "${holder}" ]; then
    busy="${busy} ${p}"
    echo "  port ${p} IN USE:"
    echo "${holder}" | sed 's/^/    /'
  fi
done
if [ -n "${busy}" ]; then
  failed=1
  echo "::error::Host port(s)${busy} are already bound; the k3d cluster cannot publish them. A previous run's teardown did not complete — remove the leftovers (k3d cluster delete --all; docker ps -a) and re-run."
else
  ok "All k3d host ports are free: ${REQUIRED_PORTS}"
fi
echo "::endgroup::"

[ "${failed}" -eq 0 ] || die "Preflight failed; not attempting the install."
ok "Preflight passed"
