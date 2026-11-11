#!/usr/bin/env bash
# lib-bootstrap.sh — host preparation shared by install-vm.sh and install-advanced.sh.
# Sourcing only defines functions (no side effects). The caller is expected to define
# log()/die(); fallbacks are provided so this file is usable standalone.

command -v log >/dev/null 2>&1 || log() { printf '\033[0;34m[bootstrap]\033[0m %s\n' "$*"; }
command -v die >/dev/null 2>&1 || die() { printf '\033[0;31m[bootstrap] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

# The VM installer NO LONGER installs host tooling. Piping remote installers
# (get.docker.com, k3d, dl.k8s.io, get-helm-3) added several unauthenticated
# GitHub/network round-trips that get rate-limited (429) behind shared NAT and
# silently mutated host state. Instead we verify the tools are already present and
# fail with actionable hints, checking (command -v) for docker/k3d/kubectl/helm.

# _tool_hint <tool> — print a one-line "how to install" hint for a required tool.
_tool_hint() {
  case "$1" in
    docker)  echo "Docker Engine — https://docs.docker.com/engine/install/" ;;
    k3d)     echo "k3d — https://k3d.io/#installation" ;;
    kubectl) echo "kubectl — https://kubernetes.io/docs/tasks/tools/#kubectl" ;;
    helm)    echo "Helm 3 — https://helm.sh/docs/intro/install/" ;;
    curl)    echo "curl — install via your OS package manager (e.g. apt-get install -y curl)" ;;
    lsof)    echo "lsof — install via your OS package manager (e.g. apt-get install -y lsof)" ;;
    openssl) echo "OpenSSL — install via your OS package manager (e.g. apt-get install -y openssl)" ;;
    *)       echo "$1 — install via your OS package manager" ;;
  esac
}

# verify_prerequisites [extra-tool ...] — confirm the required host tools are present
# (and the Docker daemon is reachable) BEFORE any cluster work. Core set:
# docker/k3d/kubectl/helm/curl; callers may append extras (e.g. openssl). All missing
# tools are reported together with install hints, then the installer exits non-zero.
verify_prerequisites() {
  # lsof is required by the wrapped quick-start installer (install.sh port checks),
  # so include it here to fail fast in this friendly preflight rather than deep inside.
  local -a required=(docker k3d kubectl helm curl lsof) missing=()
  required+=("$@")
  local t
  for t in "${required[@]}"; do
    command -v "$t" >/dev/null 2>&1 || missing+=("$t")
  done

  if (( ${#missing[@]} )); then
    log "Missing required tools: ${missing[*]}"
    log "This installer does not install host tooling — install these and re-run:"
    for t in "${missing[@]}"; do
      printf '    - %s\n' "$(_tool_hint "$t")" >&2
    done
    die "prerequisites not met (${#missing[@]} missing) — install the tools above first"
  fi

  # Docker present but the daemon may be down or not permitted for this user.
  if ! docker info >/dev/null 2>&1; then
    die "Docker is installed but its daemon is not reachable — start it (e.g. 'sudo systemctl enable --now docker') and ensure this user can run it, then re-run"
  fi

  log "Prerequisites present: ${required[*]}"
}

# ensure_firewall <port> — open the given inbound TCP port on the OS firewall.
ensure_firewall() {
  local port="${1:?ensure_firewall requires a port}"
  if command -v ufw >/dev/null 2>&1; then
    ufw allow "${port}/tcp" || true
    log "ufw: opened ${port}"
  elif command -v firewall-cmd >/dev/null 2>&1; then
    firewall-cmd --permanent --add-port="${port}/tcp" || true
    firewall-cmd --reload || true
    log "firewalld: opened ${port}"
  else
    log "No ufw/firewalld found; assuming the host firewall is open for ${port}"
  fi
  log "Ensure inbound ${port}/tcp is open in your cloud security group (raw TCP, no TLS-terminating proxy in front) — the TLS listener needs it."
}

# ensure_loopback_alias HOST... — points each hostname at 127.0.0.1 in /etc/hosts.
#
# The installer has to call the platform through its own routes before the public
# TLS listener exists (Caddy and the consolidated gateway are both brought up only
# after the base installer returns). The plane ports are loopback-bound, so the
# request has to go to 127.0.0.1 while still carrying the route's hostname in the
# Host header — which is exactly what resolving the hostname to loopback gives us.
# Entries stay after the install: the host resolving its own service names to
# itself is what a caller on this VM wants anyway.
ensure_loopback_alias() {
  local host entry
  for host in "$@"; do
    [[ -n "$host" ]] || continue
    entry="127.0.0.1 ${host}"
    # Exact-line match: only ever matches an entry this helper wrote, so a
    # hostname already mapped elsewhere is left alone rather than duplicated.
    grep -qxF "$entry" /etc/hosts 2>/dev/null && continue
    printf '%s\n' "$entry" >>/etc/hosts
    log "Mapped ${host} to 127.0.0.1 for in-install API calls"
  done
}

ensure_disk() {
  local avail_kb min_kb=$((40 * 1024 * 1024))
  avail_kb="$(df -Pk / | awk 'NR==2 {print $4}')"
  if [[ -n "$avail_kb" && "$avail_kb" -lt "$min_kb" ]]; then
    log "WARNING: only $((avail_kb / 1024 / 1024)) GB free on / — agent builds may"
    log "         hit DiskPressure. A 50 GB+ disk is recommended (see the VM docs)."
  fi
}

# inotify_bump_target <current> <floor> — echo <floor> when the current sysctl value is
# below it (or empty/non-numeric, i.e. unreadable), else echo nothing. Pure: lets the
# caller decide whether a bump is needed without touching sysctl, and keeps the
# never-lower-an-existing-higher-value rule unit-testable.
inotify_bump_target() {
  local current="$1" floor="$2"
  if [[ "$current" =~ ^[0-9]+$ ]] && (( current >= floor )); then return 0; fi
  printf '%s' "$floor"
}

# ensure_inotify_limits — raise fs.inotify.max_user_{instances,watches} to the floors k3d
# needs (kubelet + many containers each open watches) and persist them. A fresh
# single-user install can otherwise exhaust the default instance ceiling: systemd logs
# "Failed to allocate directory watch: Too many open files" and k3d configmap/secret
# watches silently go stale. Only keys actually below their floor are touched/persisted,
# so a host already tuned higher is never lowered.
ensure_inotify_limits() {
  local inst_floor=512 watch_floor=524288 cur tgt conf=/etc/sysctl.d/99-amp-inotify.conf
  local -a lines=()

  cur="$(sysctl -n fs.inotify.max_user_instances 2>/dev/null || true)"
  tgt="$(inotify_bump_target "$cur" "$inst_floor")"
  if [[ -n "$tgt" ]]; then
    sysctl -w "fs.inotify.max_user_instances=$tgt" >/dev/null 2>&1 || true
    lines+=("fs.inotify.max_user_instances=$tgt")
  fi

  cur="$(sysctl -n fs.inotify.max_user_watches 2>/dev/null || true)"
  tgt="$(inotify_bump_target "$cur" "$watch_floor")"
  if [[ -n "$tgt" ]]; then
    sysctl -w "fs.inotify.max_user_watches=$tgt" >/dev/null 2>&1 || true
    lines+=("fs.inotify.max_user_watches=$tgt")
  fi

  if (( ${#lines[@]} )); then
    printf '%s\n' "${lines[@]}" > "$conf" 2>/dev/null || true
    log "Raised inotify limits for k3d (${lines[*]})"
  else
    log "inotify limits already sufficient for k3d"
  fi
}

# verify_caddy_up — fail loudly if the amp-caddy container isn't healthy shortly after
# start. Caddy runs on the host network, so a port collision (e.g. an UPSTREAM_LISTEN_PORT
# already bound by k3d) makes it crash-loop; without this check the installer would
# report success while every request 404s through a dead proxy.
verify_caddy_up() {
  sleep 5
  local status restarts
  status="$(docker inspect -f '{{.State.Status}}' amp-caddy 2>/dev/null || true)"
  restarts="$(docker inspect -f '{{.RestartCount}}' amp-caddy 2>/dev/null || echo 0)"
  if [[ "$status" != "running" || "${restarts:-0}" -gt 0 ]]; then
    log "amp-caddy is not healthy (status=${status:-missing}, restarts=${restarts:-?}). Recent logs:"
    docker logs --tail 25 amp-caddy 2>&1 | sed 's/^/    /' >&2 || true
    die "Caddy failed to start — see logs above (in upstream mode, a common cause is UPSTREAM_LISTEN_PORT colliding with a loopback-bound cluster port)"
  fi
  log "Caddy healthy (status=${status})"
}
