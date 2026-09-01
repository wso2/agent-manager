#!/usr/bin/env bash
# Unit tests for lib-vm.sh + lib-advanced.sh. Run: bash deployments/vm/tests/run.sh
#
# Test groups set AMP_HOST_*/config vars and call the sourced lib functions, which
# read them via bash dynamic scope. shellcheck cannot follow that across the source
# boundary, and the per-group subshells are intentional isolation, so the following
# are expected false positives here:
# shellcheck disable=SC2034  # vars are consumed by sourced lib functions
# shellcheck disable=SC2030,SC2031  # subshell isolation of test-group vars is intentional
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../lib-vm.sh disable=SC1091
source "${SCRIPT_DIR}/../lib-vm.sh"
# shellcheck source=../lib-advanced.sh disable=SC1091
source "${SCRIPT_DIR}/../lib-advanced.sh"
# shellcheck source=../lib-certmanager.sh disable=SC1091
source "${SCRIPT_DIR}/../lib-certmanager.sh"
# shellcheck source=../lib-bootstrap.sh disable=SC1091
source "${SCRIPT_DIR}/../lib-bootstrap.sh"

# Failures are recorded in a marker file (not just a shell var) so that assertions
# inside subshells — used to scope AMP_HOST_* per test group — still fail the suite.
FAILLOG="$(mktemp)"
trap 'rm -f "$FAILLOG"' EXIT
assert_eq() {
  local label="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    printf 'ok   - %s\n' "$label"
  else
    printf 'FAIL - %s\n      expected: %q\n      actual:   %q\n' "$label" "$expected" "$actual"
    echo 1 >>"$FAILLOG"
  fi
}
# has <haystack> <needle> -> "yes" if needle present, else "no"
# (-- so needles starting with '-' aren't parsed as grep options)
has() { grep -qF -- "$2" <<<"$1" && echo yes || echo no; }

# --- vm_host ---
assert_eq "vm_host console" "console.amp.203.0.113.10.sslip.io" "$(vm_host console 203.0.113.10)"
assert_eq "vm_host thunder" "thunder.amp.203.0.113.10.sslip.io" "$(vm_host thunder 203.0.113.10)"

# --- build_amp_helm_args (external gateways on by default) ---
amp="$(build_amp_helm_args 203.0.113.10 true)"
# Service settings are emitted under BOTH chart keys (agentManager + agentManagerService).
assert_eq "amp serverPublicURL (service key)" \
  "agentManagerService.config.serverPublicURL=https://api.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'agentManagerService.config.serverPublicURL' <<<"$amp")"
assert_eq "amp serverPublicURL (legacy key)" \
  "agentManager.config.serverPublicURL=https://api.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'agentManager.config.serverPublicURL' <<<"$amp")"
assert_eq "amp oauthAuthorizationServers (service key)" \
  "agentManagerService.config.oauthAuthorizationServers=https://thunder.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'agentManagerService.config.oauthAuthorizationServers' <<<"$amp")"
assert_eq "amp keyManager.issuer (service key)" \
  "agentManagerService.config.keyManager.issuer=https://thunder.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'agentManagerService.config.keyManager.issuer' <<<"$amp")"
# An MCP token's aud is the public MCP endpoint, so the audience has to follow
# it off the chart's localhost default. Assert only that entry —
# pinning the whole list would break on any unrelated audience change.
assert_eq "amp keyManager.audience carries the public API URL (service key)" "yes" \
  "$(has "$amp" 'agentManagerService.config.keyManager.audience=urn:wso2:amp\,')"
assert_eq "amp keyManager.audience ends with the public API URL (service key)" "yes" \
  "$(has "$amp" 'am-mcp\,https://api.amp.203.0.113.10.sslip.io/mcp')"
assert_eq "amp keyManager.audience carries the public API URL (legacy key)" "yes" \
  "$(has "$amp" 'agentManager.config.keyManager.audience=urn:wso2:amp\,')"
# tlsEnabled=true makes amp-api advertise the https deployed-agent endpoint variant;
# emitted under both keys (old agentManager + new agentManagerService).
assert_eq "amp tlsEnabled (service key)" \
  "agentManagerService.config.tlsEnabled=true" \
  "$(grep -F 'agentManagerService.config.tlsEnabled' <<<"$amp")"
assert_eq "amp tlsEnabled (legacy key)" \
  "agentManager.config.tlsEnabled=true" \
  "$(grep -F 'agentManager.config.tlsEnabled' <<<"$amp")"
assert_eq "amp console apiBaseUrl" \
  "console.config.apiBaseUrl=https://api.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'config.apiBaseUrl' <<<"$amp")"
assert_eq "amp amObserverPublicURL" \
  "agentManagerService.config.amObserverPublicURL=https://observer.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'amObserverPublicURL' <<<"$amp")"
assert_eq "amp console instrumentationUrl" \
  "console.config.instrumentationUrl=https://gateway.amp.203.0.113.10.sslip.io/otel" \
  "$(grep -F 'instrumentationUrl' <<<"$amp")"
assert_eq "amp console signInRedirectURL" \
  "console.config.auth.signInRedirectURL=https://console.amp.203.0.113.10.sslip.io/login" \
  "$(grep -F 'signInRedirectURL' <<<"$amp")"
# Console/API HTTPRoute hostnames must match the public hosts Caddy forwards.
assert_eq "amp console ocIngress hostname" \
  "console.ocIngress.hostname=console.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'console.ocIngress.hostname' <<<"$amp")"
assert_eq "amp api ocIngress hostname" \
  "agentManagerService.ocIngress.hostname=api.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'agentManagerService.ocIngress.hostname' <<<"$amp")"
# external gateways on by default => full-URL gatewayControlPlaneUrl + the
# gateway-management service restored to LoadBalancer (chart default ClusterIP)
assert_eq "amp cp url by default" \
  "console.config.gatewayControlPlaneUrl=https://cp.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'gatewayControlPlaneUrl' <<<"$amp")"
assert_eq "amp gateway-mgmt route hostname when cp on" \
  "agentManagerService.ocIngress.gatewayMgmt.hostnames={cp.amp.203.0.113.10.sslip.io}" \
  "$(grep -F 'gatewayMgmt.hostnames' <<<"$amp")"
# Recorded on the AMS ConfigMap for add-environment.sh to read back. The two base
# domains give an added environment the right hostnames; the scheme/port give it a
# vhost that is actually callable — the runtime is fronted by TLS on :443 here, and
# the chart-default http://<host>:19080 points at a loopback-bound node port.
assert_eq "amp agentsBaseDomain (service key)" \
  "agentManagerService.config.agentsBaseDomain=agents.203.0.113.10.sslip.io" \
  "$(grep -F 'agentManagerService.config.agentsBaseDomain' <<<"$amp")"
# Both agent listener variants sit behind Caddy on :443 here — there is no :80 on a
# VM at all — so an added environment must advertise the same pair the installer
# writes for the default environment rather than inferring one port from the other.
assert_eq "amp agentsHttpPort (service key)" \
  "agentManagerService.config.agentsHttpPort=443" \
  "$(grep -F 'agentManagerService.config.agentsHttpPort' <<<"$amp")"
assert_eq "amp agentsHttpsPort (service key)" \
  "agentManagerService.config.agentsHttpsPort=443" \
  "$(grep -F 'agentManagerService.config.agentsHttpsPort' <<<"$amp")"
assert_eq "amp gatewayBaseDomain (service key)" \
  "agentManagerService.config.gatewayBaseDomain=gateway.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'agentManagerService.config.gatewayBaseDomain' <<<"$amp")"
assert_eq "amp gatewayVhostScheme (service key)" \
  "agentManagerService.config.gatewayVhostScheme=https" \
  "$(grep -F 'agentManagerService.config.gatewayVhostScheme' <<<"$amp")"
assert_eq "amp gatewayVhostPort (service key)" \
  "agentManagerService.config.gatewayVhostPort=443" \
  "$(grep -F 'agentManagerService.config.gatewayVhostPort' <<<"$amp")"

# --- build_amp_helm_args (external gateways disabled) ---
amp_nocp="$(build_amp_helm_args 203.0.113.10 false)"
assert_eq "amp no cp when disabled" "" "$(grep -F 'gatewayControlPlaneUrl' <<<"$amp_nocp")"
assert_eq "amp no gateway-mgmt hostname when cp off" "" "$(grep -F 'gatewayMgmt.hostnames' <<<"$amp_nocp")"

# --- build_gateway_helm_args sets the published vhost + user-token keymanager issuer ---
gw="$(build_gateway_helm_args 203.0.113.10)"
assert_eq "gateway vhost" \
  "gateway.vhost=https://gateway.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'gateway.vhost' <<<"$gw")"
assert_eq "gateway hostname matches vhost host" \
  "gateway.hostname=gateway.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'gateway.hostname' <<<"$gw")"
# keymanagers supplied as a full list via --set-json (a list-index --set wipes the
# other entry); ThunderKeyManager gets the public issuer, agent-manager-service kept.
assert_eq "gateway keymanagers via --set-json" "yes" "$(has "$gw" '--set-json')"
km_json="$(grep -F 'keymanagers=' <<<"$gw")"
assert_eq "gateway keymanagers is a full list" "yes" "$(has "$km_json" 'keymanagers=[{')"
assert_eq "gateway keeps agent-manager-service km" "yes" "$(has "$km_json" '"name":"agent-manager-service"')"
assert_eq "gateway ThunderKeyManager public issuer" "yes" \
  "$(has "$km_json" '"name":"ThunderKeyManager","issuer":"https://thunder.amp.203.0.113.10.sslip.io"')"
assert_eq "gateway no sparse/null keymanager" "no" "$(has "$km_json" 'null')"

# --- build_observability_helm_args points the observer at the public issuer ---
obs="$(build_observability_helm_args 203.0.113.10)"
assert_eq "observability issuer -> public thunder" \
  "amObserver.auth.issuer=https://thunder.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'amObserver.auth.issuer' <<<"$obs")"
assert_eq "observability ocIngress hostname" \
  "amObserver.ocIngress.hostname=observer.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'amObserver.ocIngress.hostname' <<<"$obs")"
assert_eq "observability publicUrl -> public observer host" \
  "amObserver.publicUrl=https://observer.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'amObserver.publicUrl' <<<"$obs")"
assert_eq "observability oauth authorizationServers -> public thunder" \
  "amObserver.oauth.authorizationServers=https://thunder.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'amObserver.oauth.authorizationServers' <<<"$obs")"
# The observer mints MCP tokens whose aud is its public `/mcp` resource, so the
# audience list has to follow publicUrl off the chart's localhost
# default. Commas stay escaped or helm splits the value into a list.
assert_eq "observability audience carries the public observer URL" \
  'amObserver.auth.audience=urn:wso2:amp\,amp-api-client\,am-obs-mcp\,https://observer.amp.203.0.113.10.sslip.io/mcp' \
  "$(grep -F 'amObserver.auth.audience' <<<"$obs")"

# --- render_dataplane_external_ingress: public host on :443, both http+https entries
#     bound to the internal http listener (amp-api advertises the https variant) ---
dpe="$(render_dataplane_external_ingress 203.0.113.10)"
assert_eq "dp external public host"    "yes" "$(has "$dpe" 'host: "agents.203.0.113.10.sslip.io"')"
assert_eq "dp external port 443"       "yes" "$(has "$dpe" 'port: 443')"
assert_eq "dp external listener http"  "yes" "$(has "$dpe" 'listenerName: http')"
assert_eq "dp external has http entry"  "yes" "$(printf '%s\n' "$dpe" | grep -qE '^        http:' && echo yes || echo no)"
assert_eq "dp external has https entry" "yes" "$(printf '%s\n' "$dpe" | grep -qE '^        https:' && echo yes || echo no)"
assert_eq "dp external not local default (19080)" "no" "$(has "$dpe" 'port: 1908')"

# --- build_cp_helm_args points OpenChoreo CP OIDC issuer at the public Thunder URL ---
cp_args="$(build_cp_helm_args 203.0.113.10)"
assert_eq "cp oidc issuer" \
  "security.oidc.issuer=https://thunder.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'security.oidc.issuer' <<<"$cp_args")"
assert_eq "cp oidc tokenUrl" \
  "security.oidc.tokenUrl=https://thunder.amp.203.0.113.10.sslip.io/oauth2/token" \
  "$(grep -F 'security.oidc.tokenUrl' <<<"$cp_args")"

# --- build_thunder_helm_args ---
th="$(build_thunder_helm_args 203.0.113.10)"
assert_eq "thunder ocIngress.hostname" \
  "thunder.ocIngress.hostname=thunder.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'ocIngress.hostname' <<<"$th")"
assert_eq "thunder server.publicUrl" \
  "thunder.configuration.server.publicUrl=https://thunder.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'server.publicUrl' <<<"$th")"
assert_eq "thunder jwt.issuer" \
  "thunder.configuration.jwt.issuer=https://thunder.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'jwt.issuer' <<<"$th")"
assert_eq "thunder gateClient.scheme" \
  "thunder.configuration.gateClient.scheme=https" \
  "$(grep -F 'gateClient.scheme' <<<"$th")"
assert_eq "thunder gateClient.port" \
  "thunder.configuration.gateClient.port=443" \
  "$(grep -F 'gateClient.port' <<<"$th")"
assert_eq "thunder cors origin" \
  "thunder.configuration.cors.allowedOrigins[0]=https://console.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'cors.allowedOrigins' <<<"$th")"
# redirectUri emitted under both setup (legacy) and bootstrap (>=0.15.0) keys
assert_eq "thunder console redirectUri (bootstrap key)" \
  "thunder.bootstrap.ampConsoleClient.redirectUris[0]=https://console.amp.203.0.113.10.sslip.io/login" \
  "$(grep -F 'thunder.bootstrap.ampConsoleClient.redirectUris' <<<"$th")"
assert_eq "thunder console redirectUri (legacy setup key)" \
  "thunder.setup.ampConsoleClient.redirectUris[0]=https://console.amp.203.0.113.10.sslip.io/login" \
  "$(grep -F 'thunder.setup.ampConsoleClient.redirectUris' <<<"$th")"
# RFC 8707 resource indicators. Thunder compares the authorize request's
# `resource` verbatim, so these must name the hosts an MCP client dials, with the
# trailing slash the client sends. Index 0 is agent-manager, index 1 the observer.
# MCP base URLs are set as mergeable scalars (not list-element overrides, which
# helm --set would replace whole, dropping name/handle/permissionSet). No trailing
# slash — the bootstrap template appends it.
assert_eq "thunder MCP base URL (agent-manager)" \
  "thunder.bootstrap.agentManagerMcpBaseUrl=https://api.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'agentManagerMcpBaseUrl' <<<"$th")"
assert_eq "thunder MCP base URL (observer)" \
  "thunder.bootstrap.observerMcpBaseUrl=https://observer.amp.203.0.113.10.sslip.io" \
  "$(grep -F 'observerMcpBaseUrl' <<<"$th")"
assert_eq "thunder no fragile mcpResourceServers list override" "no" \
  "$(has "$th" 'mcpResourceServers[')"

# --- render_k3d_vm_config ---
k3d_in="$(printf '%s\n' \
  'ports:' \
  '  - port: 3000:3000' \
  '    nodeFilters:' \
  '      - loadbalancer' \
  '  - port: 11082:9200' \
  '    nodeFilters:' \
  '      - loadbalancer')"
k3d_out="$(render_k3d_vm_config <<<"$k3d_in")"
assert_eq "k3d rebinds 3000" \
  "  - port: 127.0.0.1:3000:3000" \
  "$(grep -F '3000' <<<"$k3d_out")"
assert_eq "k3d rebinds mismatched ports" \
  "  - port: 127.0.0.1:11082:9200" \
  "$(grep -F '11082' <<<"$k3d_out")"
assert_eq "k3d leaves nodeFilters intact" \
  "    nodeFilters:" \
  "$(grep -F 'nodeFilters' <<<"$k3d_out" | head -1)"
assert_eq "k3d leaves already-bound entry untouched" \
  "  - port: 127.0.0.1:3000:3000" \
  "$(render_k3d_vm_config <<<'  - port: 127.0.0.1:3000:3000')"
# registry mirror endpoint -> node host (so node containerd can pull); key untouched
reg_in="$(printf '%s\n' \
  '    mirrors:' \
  '      "host.k3d.internal:10082":' \
  '        endpoint:' \
  '          - http://host.k3d.internal:10082')"
reg_out="$(render_k3d_vm_config <<<"$reg_in")"
assert_eq "k3d registry endpoint -> node host" \
  "          - http://k3d-amp-local-server-0:10082" \
  "$(grep -F 'endpoint' -A1 <<<"$reg_out" | grep -F 'http://')"
assert_eq "k3d registry mirror key untouched" \
  '      "host.k3d.internal:10082":' \
  "$(grep -F '"host.k3d.internal:10082":' <<<"$reg_out")"

# --- render_caddyfile (with email, external gateways disabled => no cp) ---
cf="$(render_caddyfile 203.0.113.10 "ops@example.com" false)"
assert_eq "caddy email block" "	email ops@example.com" "$(grep -F 'email ops@example.com' <<<"$cf")"
assert_eq "caddy console site" "console.amp.203.0.113.10.sslip.io {" "$(grep -F 'console.amp' <<<"$cf" | head -1)"
assert_eq "caddy console upstream (via kgateway)" "	reverse_proxy 127.0.0.1:8080" \
  "$(grep -F -A8 'console.amp.203.0.113.10.sslip.io {' <<<"$cf" | grep -F 'reverse_proxy' | head -1)"
# Four sites proxy to the CP kgateway (8080): console, api, the fixed
# platform-Thunder host, and the env-Thunder base-domain wildcard (*.<domain>,
# no fixed "thunder." segment) — kgateway itself discriminates by Host header
# via each backend's HTTPRoute.
# console, api, thunder, the env-Thunder base-domain wildcard, and the on-demand ask
# endpoint — cp is excluded in this render (external gateways off).
assert_eq "caddy cp-kgateway upstream count" "5" "$(grep -cF '127.0.0.1:8080' <<<"$cf")"
assert_eq "caddy env-thunder wildcard site" "*.amp.203.0.113.10.sslip.io {" \
  "$(grep -F '*.amp.203.0.113.10.sslip.io {' <<<"$cf" | head -1)"
# gateway host routes through the kgateway data plane (19080), not the ClusterIP
# runtime (22893) which is not node-published; scope the grep to the gateway block
# since 19080 is shared with the agents site.
assert_eq "caddy gateway upstream (via kgateway)" "	reverse_proxy 127.0.0.1:19080" \
  "$(grep -F -A8 'gateway.amp.203.0.113.10.sslip.io {' <<<"$cf" | grep -F 'reverse_proxy' | head -1)"
assert_eq "caddy env-gateway wildcard site" "*.gateway.amp.203.0.113.10.sslip.io {" \
  "$(grep -F '*.gateway.amp' <<<"$cf" | head -1)"
assert_eq "caddy env-gateway wildcard upstream" "	reverse_proxy 127.0.0.1:19080" \
  "$(grep -F -A8 '*.gateway.amp.203.0.113.10.sslip.io {' <<<"$cf" | grep -F 'reverse_proxy' | head -1)"
assert_eq "caddy no 22893 dead-end" "" "$(grep -F '127.0.0.1:22893' <<<"$cf")"
assert_eq "caddy no cp when disabled" "" "$(grep -F 'cp.amp' <<<"$cf")"
assert_eq "caddy api upstream (via kgateway)" "	reverse_proxy 127.0.0.1:8080" \
  "$(grep -F -A8 'api.amp.203.0.113.10.sslip.io {' <<<"$cf" | grep -F 'reverse_proxy' | head -1)"
assert_eq "caddy observer upstream (via kgateway)" "	reverse_proxy 127.0.0.1:11080" "$(grep -F '127.0.0.1:11080' <<<"$cf")"
# The old dedicated-service upstreams must be gone (ClusterIP, not node-published).
assert_eq "caddy no 3000/9000/9098 dead-ends" "" "$(grep -E '127\.0\.0\.1:(3000|9000|9098)' <<<"$cf")"

# --- render_caddyfile: always 443-only TLS-ALPN-01 (disable_redirects + per-site
#     issuer acme/disable_http_challenge); no http mode, no port-80 redirect ---
cf_tls="$(render_caddyfile 203.0.113.10 "ops@example.com" true)"
assert_eq "global disable_redirects"   "yes" "$(has "$cf_tls" 'auto_https disable_redirects')"
assert_eq "issuer acme"                "yes" "$(has "$cf_tls" 'issuer acme')"
assert_eq "disable_http_challenge"     "yes" "$(has "$cf_tls" 'disable_http_challenge')"
assert_eq "keeps email"                "yes" "$(has "$cf_tls" 'email ops@example.com')"
# per-site tls block on each public host incl. cp (6) + the env-Thunder wildcard (1)
# + the env-gateway wildcard (1) + the agent wildcard (1) = 9
assert_eq "tls block per site (9)"     "9"   "$(grep -cF 'issuer acme' <<<"$cf_tls")"
# never serves plain http / disables auto-https
assert_eq "no auto_https off"          "no"  "$(has "$cf_tls" 'auto_https off')"
assert_eq "no http:// public site"     "no"  "$(has "$cf_tls" 'http://console')"

# --- external gateways on by default => cp block present (3rd arg omitted) ---
cf_default="$(render_caddyfile 203.0.113.10 "")"
assert_eq "caddy cp on by default" "cp.amp.203.0.113.10.sslip.io {" "$(grep -F 'cp.amp' <<<"$cf_default" | head -1)"
cf_cp="$(render_caddyfile 203.0.113.10 "" true)"
# cp rides the CP kgateway like console/api (BackendTLSPolicy handles the TLS
# hop in-cluster) — no direct-to-9243 TLS transport anymore.
assert_eq "caddy cp upstream (via kgateway)" "	reverse_proxy 127.0.0.1:8080" \
  "$(grep -F -A8 'cp.amp.203.0.113.10.sslip.io {' <<<"$cf_cp" | grep -F 'reverse_proxy' | head -1)"
assert_eq "caddy cp no direct 9243" "" "$(grep -F '127.0.0.1:9243' <<<"$cf_cp")"

# --- build_platform_resources_helm_args: point the workload publisher at the
#     Thunder service directly (the gateway path 404s once Thunder's vhost moves to
#     the public sslip.io host), AND advertise the public agents host on the default
#     Environment so deployed-agent routes resolve to <org>-<project>.<agents-base>
#     (else OpenChoreo uses am-gateway.localhost and the console invoke URL is empty,
#     with try-out 405ing against its own host). Reads AMP_AGENTS_BASE from scope. ---
(
  AMP_AGENTS_BASE=agents.amp.example.com
  AMP_HOST_GATEWAY=gateway.amp.example.com
  pr="$(build_platform_resources_helm_args)"
  # Agents run in-cluster and must use the chart's default in-cluster runtime
  # endpoint. Overriding it with the public gateway host breaks trace export on a
  # private-network VM, where that hostname resolves into an RFC-1918 range the
  # sandbox NetworkPolicy blocks on :443.
  assert_eq "platform-resources leaves agent OTEL endpoint in-cluster" "no" \
    "$(has "$pr" 'apiPlatformGatewayVhost.otelEndpointOverride')"
  assert_eq "platform-resources oauth tokenUrl (direct svc)" \
    "global.oauth.tokenUrl=http://amp-thunder-extension-service.amp-thunder.svc.cluster.local:8090/oauth2/token" \
    "$(grep -F 'global.oauth.tokenUrl' <<<"$pr")"
  assert_eq "platform-resources oauth not via host.k3d.internal" "no" "$(has "$pr" 'host.k3d.internal')"
  assert_eq "platform-resources env gateway host (public agents base)" \
    "environment.gateway.http.host=agents.amp.example.com" \
    "$(grep -F 'environment.gateway.http.host' <<<"$pr")"
  assert_eq "platform-resources env gateway port 443" \
    "environment.gateway.http.port=443" \
    "$(grep -F 'environment.gateway.http.port' <<<"$pr")"
  # The console reads the https endpoint variant when tlsEnabled=true, so the
  # Environment override MUST carry an https variant too. Without it the binding
  # status has only an http externalURL, the console invoke URL is empty, and
  # try-out falls back to a relative /chat (405). Mirror dataplane_external_ingress.
  assert_eq "platform-resources env gateway https host (public agents base)" \
    "environment.gateway.https.host=agents.amp.example.com" \
    "$(grep -F 'environment.gateway.https.host' <<<"$pr")"
  assert_eq "platform-resources env gateway https port 443" \
    "environment.gateway.https.port=443" \
    "$(grep -F 'environment.gateway.https.port' <<<"$pr")"
)

# --- render_coredns_vm_config rewrites the in-cluster names to the server node ---
cd_cfg="$(render_coredns_vm_config k3d-amp-local-server-0)"
assert_eq "coredns configmap name" "yes" "$(has "$cd_cfg" 'name: coredns-custom')"
assert_eq "coredns openchoreo -> node" "yes" \
  "$(has "$cd_cfg" 'name regex (.+\.)?openchoreo\.localhost k3d-amp-local-server-0')"
assert_eq "coredns amp -> node" "yes" \
  "$(has "$cd_cfg" 'name regex (.+\.)?amp\.localhost k3d-amp-local-server-0')"
assert_eq "coredns host aliases -> node" "yes" \
  "$(has "$cd_cfg" 'name regex (host\.k3d\.internal|host\.docker\.internal) k3d-amp-local-server-0')"
assert_eq "coredns no longer targets host.k3d.internal as dest" "no" \
  "$(has "$cd_cfg" 'localhost host.k3d.internal')"

# --- render_caddyfile: fixed hosts get on-demand certs, approved at the ask endpoint ---
# The env-Thunder wildcard (*.amp.<ip>) covers every fixed host, and Caddy skips eager
# issuance for a name a managed wildcard covers — so left eager these never get a
# certificate at all and every request fails the handshake.
cf_od="$(render_caddyfile 203.0.113.10 "ops@example.com" true)"
# One tls block per site, each carrying on_demand: 6 fixed hosts + 3 wildcard sites.
assert_eq "caddy on_demand on every site" "9" "$(grep -cE '^\s+on_demand$' <<<"$cf_od")"
console_tls="$(awk '/^console\.amp/,/^}/' <<<"$cf_od")"
assert_eq "caddy console site is on-demand" "yes" "$(has "$console_tls" 'on_demand')"
assert_eq "caddy console keeps TLS-ALPN-01 issuer" "yes" "$(has "$console_tls" 'disable_http_challenge')"
ask_block="$(awk '/^http:\/\/127\.0\.0\.1:9753/,/^}/' <<<"$cf_od")"
assert_eq "ask endpoint approves console" "yes" \
  "$(has "$ask_block" '{query.domain} == "console.amp.203.0.113.10.sslip.io"')"
assert_eq "ask endpoint approves cp when enabled" "yes" \
  "$(has "$ask_block" '{query.domain} == "cp.amp.203.0.113.10.sslip.io"')"
# Exact match only — a look-alike must still satisfy AMS's registered-handle check.
assert_eq "ask endpoint does not suffix-match the base domain" "no" \
  "$(has "$ask_block" '{query.domain}.endsWith(".amp.203.0.113.10.sslip.io")')"
ask_block_nocp="$(awk '/^http:\/\/127\.0\.0\.1:9753/,/^}/' <<<"$cf")"
assert_eq "ask endpoint omits cp when disabled" "no" "$(has "$ask_block_nocp" 'cp.amp')"

# --- render_caddyfile: deployed-agent invocation (wildcard site, on-demand TLS,
#     CORS, ask endpoint) ---
cf_ai="$(render_caddyfile 203.0.113.10 "ops@example.com" true)"
# No CSP: amp-api advertises the https agent endpoint (config.tlsEnabled=true), so the
# console emits https directly and no upgrade-insecure-requests workaround is needed.
assert_eq "console has no CSP workaround" "no" \
  "$(has "$cf_ai" 'Content-Security-Policy')"
assert_eq "global on_demand_tls ask" "yes" "$(has "$cf_ai" 'ask http://127.0.0.1:9753')"
assert_eq "on-demand ask endpoint site" "yes" "$(has "$cf_ai" 'http://127.0.0.1:9753 {')"
assert_eq "wildcard agent site" "yes" "$(has "$cf_ai" '*.agents.203.0.113.10.sslip.io {')"
assert_eq "agent site on_demand tls" "yes" "$(has "$cf_ai" 'on_demand')"
assert_eq "agent site proxies data-plane gw" "yes" "$(has "$cf_ai" 'reverse_proxy 127.0.0.1:19080')"
assert_eq "agent CORS allow-origin = console" "yes" \
  "$(has "$cf_ai" 'Access-Control-Allow-Origin "https://console.amp.203.0.113.10.sslip.io"')"
assert_eq "agent CORS allows X-API-Key" "yes" "$(has "$cf_ai" 'Authorization, Content-Type, X-API-Key')"
assert_eq "agent CORS preflight short-circuit" "yes" "$(has "$cf_ai" 'respond @cors_preflight 204')"
# agent site forces TLS-ALPN-01 (disable_http_challenge) alongside on_demand
assert_eq "agent site on_demand + disable_http_challenge" "yes" \
  "$(printf '%s' "$cf_ai" | awk '/\*\.agents\./{f=1} f' | grep -qF 'disable_http_challenge' && echo yes || echo no)"

# --- hostname-driven cores: set AMP_HOST_* and call the core directly ---
(
  AMP_HOST_CONSOLE=console.amp.example.com
  AMP_HOST_API=api.amp.example.com
  AMP_HOST_THUNDER=thunder.amp.example.com
  AMP_HOST_OBSERVER=observer.amp.example.com
  AMP_HOST_GATEWAY=gateway.amp.example.com
  AMP_HOST_CP=cp.amp.example.com
  AMP_AGENTS_BASE=agents.amp.example.com

  core_amp="$(amp_helm_args)"
  assert_eq "core amp serverPublicURL (service key)" \
    "agentManagerService.config.serverPublicURL=https://api.amp.example.com" \
    "$(grep -F 'agentManagerService.config.serverPublicURL' <<<"$core_amp")"
  assert_eq "core amp cp url present" \
    "console.config.gatewayControlPlaneUrl=https://cp.amp.example.com" \
    "$(grep -F 'gatewayControlPlaneUrl' <<<"$core_amp")"
  # The audience must land on the same host as serverPublicURL above and as
  # thunder.bootstrap.agentManagerMcpBaseUrl below.
  assert_eq "core amp keyManager.audience carries the public API URL" "yes" \
    "$(has "$core_amp" 'am-mcp\,https://api.amp.example.com/mcp')"

  core_th="$(thunder_helm_args)"
  assert_eq "core thunder jwt.issuer" \
    "thunder.configuration.jwt.issuer=https://thunder.amp.example.com" \
    "$(grep -F 'jwt.issuer' <<<"$core_th")"
  # The advanced path reaches the same core, so the MCP resource indicators must
  # follow the configured domain here too.
  assert_eq "core thunder MCP base URL (agent-manager)" \
    "thunder.bootstrap.agentManagerMcpBaseUrl=https://api.amp.example.com" \
    "$(grep -F 'agentManagerMcpBaseUrl' <<<"$core_th")"
  assert_eq "core thunder MCP base URL (observer)" \
    "thunder.bootstrap.observerMcpBaseUrl=https://observer.amp.example.com" \
    "$(grep -F 'observerMcpBaseUrl' <<<"$core_th")"

  core_obs="$(observability_helm_args)"
  assert_eq "core observability audience carries the public observer URL" \
    'amObserver.auth.audience=urn:wso2:amp\,amp-api-client\,am-obs-mcp\,https://observer.amp.example.com/mcp' \
    "$(grep -F 'amObserver.auth.audience' <<<"$core_obs")"

  core_gw="$(gateway_helm_args)"
  assert_eq "core gateway vhost" \
    "gateway.vhost=https://gateway.amp.example.com" \
    "$(grep -F 'gateway.vhost' <<<"$core_gw")"
  assert_eq "core gateway hostname matches vhost host" \
    "gateway.hostname=gateway.amp.example.com" \
    "$(grep -F 'gateway.hostname' <<<"$core_gw")"

  core_cp="$(cp_helm_args)"
  assert_eq "core cp oidc issuer" \
    "security.oidc.issuer=https://thunder.amp.example.com" \
    "$(grep -F 'security.oidc.issuer' <<<"$core_cp")"

  # The advanced install has no Caddy: one DNS-01 wildcard cert covers the dynamic
  # tiers instead of on-demand issuance. Per-environment gateways are one of those
  # tiers, so *.<gateway host> must be a SAN — without it an added environment gets a
  # hostname that resolves and routes but fails the TLS handshake.
  dns_names="$(cert_dns_names)"
  assert_eq "cert covers the agents wildcard" "*.agents.amp.example.com" \
    "$(grep -Fx '*.agents.amp.example.com' <<<"$dns_names")"
  assert_eq "cert covers the env-Thunder wildcard" "*.amp.example.com" \
    "$(grep -Fx '*.amp.example.com' <<<"$dns_names")"
  assert_eq "cert covers the env-gateway wildcard" "*.gateway.amp.example.com" \
    "$(grep -Fx '*.gateway.amp.example.com' <<<"$dns_names")"
  # The exact gateway host must stay a SAN alongside its wildcard: *.gateway makes
  # gateway.<base> an empty non-terminal, so nothing broader covers it (RFC 4592).
  assert_eq "cert keeps the exact gateway host" "gateway.amp.example.com" \
    "$(grep -Fx 'gateway.amp.example.com' <<<"$dns_names")"
)

# --- core respects AMP_HOST_CP="" as external-gateways-off ---
(
  AMP_HOST_CONSOLE=console.amp.example.com
  AMP_HOST_API=api.amp.example.com
  AMP_HOST_THUNDER=thunder.amp.example.com
  AMP_HOST_OBSERVER=observer.amp.example.com
  AMP_HOST_GATEWAY=gateway.amp.example.com
  AMP_HOST_CP=""
  AMP_AGENTS_BASE=agents.amp.example.com
  assert_eq "core amp no cp when AMP_HOST_CP empty" "" \
    "$(grep -F 'gatewayControlPlaneUrl' <<<"$(amp_helm_args)")"
)

# --- dataplane_external_ingress core reads AMP_AGENTS_BASE ---
(
  AMP_AGENTS_BASE=agents.amp.example.com
  dpe_core="$(dataplane_external_ingress)"
  assert_eq "core dp external host" "yes" "$(has "$dpe_core" 'host: "agents.amp.example.com"')"
  assert_eq "core dp external port 443" "yes" "$(has "$dpe_core" 'port: 443')"
)

# --- caddyfile core, letsencrypt mode ---
(
  AMP_HOST_CONSOLE=console.amp.example.com
  AMP_HOST_API=api.amp.example.com
  AMP_HOST_THUNDER=thunder.amp.example.com
  AMP_HOST_OBSERVER=observer.amp.example.com
  AMP_HOST_GATEWAY=gateway.amp.example.com
  AMP_HOST_CP=cp.amp.example.com
  AMP_AGENTS_BASE=agents.amp.example.com
  cf_le="$(caddyfile letsencrypt "ops@example.com" "" "" "")"
  assert_eq "core caddy LE console site" "yes" "$(has "$cf_le" 'console.amp.example.com {')"
  assert_eq "core caddy LE console upstream" "yes" "$(has "$cf_le" 'reverse_proxy 127.0.0.1:8080')"
  assert_eq "core caddy LE issuer acme" "yes" "$(has "$cf_le" 'issuer acme')"
  assert_eq "core caddy LE disable_http_challenge" "yes" "$(has "$cf_le" 'disable_http_challenge')"
  assert_eq "core caddy LE email" "yes" "$(has "$cf_le" 'email ops@example.com')"
  assert_eq "core caddy LE cp site" "yes" "$(has "$cf_le" 'cp.amp.example.com {')"
  assert_eq "core caddy LE agent wildcard" "yes" "$(has "$cf_le" '*.agents.amp.example.com {')"
  assert_eq "core caddy LE on_demand ask" "yes" "$(has "$cf_le" 'ask http://127.0.0.1:9753')"
)

# --- caddyfile core, byoc mode ---
(
  AMP_HOST_CONSOLE=console.amp.example.com
  AMP_HOST_API=api.amp.example.com
  AMP_HOST_THUNDER=thunder.amp.example.com
  AMP_HOST_OBSERVER=observer.amp.example.com
  AMP_HOST_GATEWAY=gateway.amp.example.com
  AMP_HOST_CP=cp.amp.example.com
  AMP_AGENTS_BASE=agents.amp.example.com
  cf_byoc="$(caddyfile byoc "" /opt/amp/certs/fullchain.pem /opt/amp/certs/privkey.pem "")"
  # byoc serves one operator-supplied cert on every site, so nothing is ever issued
  # on demand — the wildcard-coverage problem that forces on_demand under
  # letsencrypt does not apply here.
  assert_eq "core caddy byoc has no on_demand" "no" "$(has "$cf_byoc" 'on_demand')"
  assert_eq "byoc serves provided cert/key" "yes" \
    "$(has "$cf_byoc" 'tls /opt/amp/certs/fullchain.pem /opt/amp/certs/privkey.pem')"
  assert_eq "byoc no acme issuer" "no" "$(has "$cf_byoc" 'issuer acme')"
  assert_eq "byoc no on_demand ask endpoint" "no" "$(has "$cf_byoc" 'ask http://127.0.0.1:9753')"
  assert_eq "byoc agent site uses provided cert (not on_demand)" "no" "$(has "$cf_byoc" 'on_demand')"
  assert_eq "byoc still has agent wildcard" "yes" "$(has "$cf_byoc" '*.agents.amp.example.com {')"
  assert_eq "byoc still serves https (no http:// site)" "no" "$(has "$cf_byoc" 'http://console')"
  assert_eq "byoc keeps CORS for agents" "yes" "$(has "$cf_byoc" 'X-API-Key')"
)

# --- caddyfile core, upstream mode (LB terminates TLS; Caddy routes plain HTTP) ---
(
  AMP_HOST_CONSOLE=console.amp.example.com
  AMP_HOST_API=api.amp.example.com
  AMP_HOST_THUNDER=thunder.amp.example.com
  AMP_HOST_OBSERVER=observer.amp.example.com
  AMP_HOST_GATEWAY=gateway.amp.example.com
  AMP_HOST_CP=cp.amp.example.com
  AMP_AGENTS_BASE=agents.amp.example.com
  cf_up="$(caddyfile upstream "" "" "" 8080)"
  assert_eq "upstream sets http_port" "yes" "$(has "$cf_up" 'http_port 8080')"
  assert_eq "upstream trusts proxy headers" "yes" "$(has "$cf_up" 'trusted_proxies static 0.0.0.0/0')"
  assert_eq "upstream console site is plain http" "yes" "$(has "$cf_up" 'http://console.amp.example.com:8080 {')"
  assert_eq "upstream console upstream port" "yes" "$(has "$cf_up" 'reverse_proxy 127.0.0.1:8080')"
  assert_eq "upstream no acme" "no" "$(has "$cf_up" 'issuer acme')"
  assert_eq "upstream no tls cert directive" "no" "$(has "$cf_up" 'tls /')"
  assert_eq "upstream agent wildcard plain http" "yes" "$(has "$cf_up" 'http://*.agents.amp.example.com:8080 {')"
  assert_eq "upstream keeps CORS" "yes" "$(has "$cf_up" 'X-API-Key')"
)

# --- upstream default listen port is 80 ---
(
  AMP_HOST_CONSOLE=console.amp.example.com
  AMP_HOST_API=api.amp.example.com
  AMP_HOST_THUNDER=thunder.amp.example.com
  AMP_HOST_OBSERVER=observer.amp.example.com
  AMP_HOST_GATEWAY=gateway.amp.example.com
  AMP_HOST_CP=""
  AMP_AGENTS_BASE=agents.amp.example.com
  cf_up80="$(caddyfile upstream "" "" "" "")"
  assert_eq "upstream default port 80" "yes" "$(has "$cf_up80" 'http_port 80')"
  assert_eq "upstream console site port 80" "yes" "$(has "$cf_up80" 'http://console.amp.example.com:80 {')"
  assert_eq "upstream no cp site when AMP_HOST_CP empty" "no" "$(has "$cf_up80" 'cp.amp.example.com')"
)

# --- derive_hosts: defaults from DOMAIN_BASE ---
(
  DOMAIN_BASE=amp.mycompany.com
  EXTERNAL_GATEWAYS=true
  derive_hosts
  assert_eq "derive console default" "console.amp.mycompany.com" "$AMP_HOST_CONSOLE"
  assert_eq "derive api default"     "api.amp.mycompany.com"     "$AMP_HOST_API"
  assert_eq "derive thunder default" "thunder.amp.mycompany.com" "$AMP_HOST_THUNDER"
  assert_eq "derive cp default"      "cp.amp.mycompany.com"      "$AMP_HOST_CP"
  assert_eq "derive agents default"  "agents.amp.mycompany.com"  "$AMP_AGENTS_BASE"
)

# --- derive_hosts: per-service override + custom AGENTS_BASE ---
(
  DOMAIN_BASE=amp.mycompany.com
  EXTERNAL_GATEWAYS=true
  HOST_CONSOLE=ui.mycompany.com
  AGENTS_BASE=run.mycompany.com
  derive_hosts
  assert_eq "derive console override" "ui.mycompany.com" "$AMP_HOST_CONSOLE"
  assert_eq "derive api still default" "api.amp.mycompany.com" "$AMP_HOST_API"
  assert_eq "derive agents override" "run.mycompany.com" "$AMP_AGENTS_BASE"
)

# --- derive_hosts: external gateways off => empty cp host ---
(
  DOMAIN_BASE=amp.mycompany.com
  EXTERNAL_GATEWAYS=false
  derive_hosts
  assert_eq "derive cp empty when external gateways off" "" "$AMP_HOST_CP"
)

# --- validate_config: complete DNS-01 config passes ---
(
  AMP_VERSION=0.15.0; DOMAIN_BASE=amp.mycompany.com; ACME_EMAIL=ops@mycompany.com
  DNS_PROVIDER=cloudflare; CLOUDFLARE_API_TOKEN=tok
  validate_config; rc=$?
  assert_eq "validate complete DNS-01 config rc=0" "0" "$rc"
)

# --- validate_config: missing ACME_EMAIL fails (required for the ACME account) ---
(
  AMP_VERSION=0.16.0; DOMAIN_BASE=amp.mycompany.com
  DNS_PROVIDER=cloudflare; CLOUDFLARE_API_TOKEN=tok
  validate_config; rc=$?
  assert_eq "validate missing ACME_EMAIL rc=1" "1" "$rc"
)

# --- validate_config: missing DOMAIN_BASE fails ---
(
  AMP_VERSION=0.15.0; ACME_EMAIL=ops@mycompany.com
  DNS_PROVIDER=cloudflare; CLOUDFLARE_API_TOKEN=tok
  validate_config; rc=$?
  assert_eq "validate missing DOMAIN_BASE rc=1" "1" "$rc"
  assert_eq "validate names DOMAIN_BASE" "yes" \
    "$(printf '%s\n' "${CONFIG_ERRORS[@]}" | grep -qF 'DOMAIN_BASE' && echo yes || echo no)"
)

# --- validate_config: unknown DNS_PROVIDER fails ---
(
  AMP_VERSION=0.15.0; DOMAIN_BASE=amp.mycompany.com; ACME_EMAIL=ops@mycompany.com
  DNS_PROVIDER=banana
  validate_config; rc=$?
  assert_eq "validate unknown DNS_PROVIDER rc=1" "1" "$rc"
)

# --- validate_config: missing provider credentials fail and are named ---
(
  AMP_VERSION=0.15.0; DOMAIN_BASE=amp.mycompany.com; ACME_EMAIL=ops@mycompany.com
  DNS_PROVIDER=cloudflare
  validate_config; rc=$?
  assert_eq "validate cloudflare without token rc=1" "1" "$rc"
  assert_eq "validate names CLOUDFLARE_API_TOKEN" "yes" \
    "$(printf '%s\n' "${CONFIG_ERRORS[@]}" | grep -qF 'CLOUDFLARE_API_TOKEN' && echo yes || echo no)"
)

# --- install-advanced.sh --init emits a sourceable, complete template ---
ADV="${SCRIPT_DIR}/../install-advanced.sh"
init_out="$(bash "$ADV" --init)"
assert_eq "init has AMP_VERSION"   "yes" "$(has "$init_out" 'AMP_VERSION=')"
assert_eq "init has DOMAIN_BASE"   "yes" "$(has "$init_out" 'DOMAIN_BASE=')"
assert_eq "init has ACME_EMAIL"    "yes" "$(has "$init_out" 'ACME_EMAIL=')"
assert_eq "init has DNS_PROVIDER"  "yes" "$(has "$init_out" 'DNS_PROVIDER=')"
assert_eq "init mentions ACME_SERVER" "yes" "$(has "$init_out" 'ACME_SERVER=')"
assert_eq "init has TLS_MODE"      "yes" "$(has "$init_out" 'TLS_MODE=dns01')"
assert_eq "init mentions TLS_CERT_FILE" "yes" "$(has "$init_out" 'TLS_CERT_FILE=')"
assert_eq "init mentions TLS_KEY_FILE"  "yes" "$(has "$init_out" 'TLS_KEY_FILE=')"
assert_eq "init mentions TLS_CA_FILE"   "yes" "$(has "$init_out" 'TLS_CA_FILE=')"
# The emitted template must be valid shell (sourceable without error).
tmp_init="$(mktemp)"; printf '%s\n' "$init_out" > "$tmp_init"
if bash -n "$tmp_init"; then assert_eq "init template is valid shell" "0" "0"; else assert_eq "init template is valid shell" "0" "1"; fi
rm -f "$tmp_init"

# --- --dry-run renders the cert-manager resources + helm args, no cluster work ---
tmp_cfg="$(mktemp)"
cat > "$tmp_cfg" <<'CFG'
AMP_VERSION=0.15.0
DOMAIN_BASE=amp.mycompany.com
ACME_EMAIL=ops@mycompany.com
DNS_PROVIDER=cloudflare
CLOUDFLARE_API_TOKEN=dummy-token
EXTERNAL_GATEWAYS=true
CFG
dry_out="$(bash "$ADV" --config "$tmp_cfg" --dry-run 2>&1)"
assert_eq "dry-run renders ACME ClusterIssuer" "yes" "$(has "$dry_out" 'kind: ClusterIssuer')"
assert_eq "dry-run renders consolidated gateway" "yes" "$(has "$dry_out" 'kind: Gateway')"
assert_eq "dry-run renders amp helm arg" "yes" "$(has "$dry_out" 'serverPublicURL=https://api.amp.mycompany.com')"
assert_eq "dry-run does NOT start install" "no" "$(has "$dry_out" 'Running base installer')"
rm -f "$tmp_cfg"

# --- --dry-run in byoc mode: TLS Secret instead of the cert-manager objects ---
# A config with no ACME_EMAIL and no DNS_PROVIDER must be accepted, and the private key
# must never reach stdout (dry-run output lands in terminal scrollback and CI logs).
byoc_dir="$(mktemp -d)"
byoc_d=amp.mycompany.com
openssl req -x509 -newkey rsa:2048 -nodes -days 90 \
  -keyout "${byoc_dir}/key.pem" -out "${byoc_dir}/cert.pem" -subj "/CN=${byoc_d}" \
  -addext "subjectAltName=DNS:console.${byoc_d},DNS:api.${byoc_d},DNS:thunder.${byoc_d},DNS:observer.${byoc_d},DNS:gateway.${byoc_d},DNS:cp.${byoc_d},DNS:*.agents.${byoc_d},DNS:*.${byoc_d},DNS:*.gateway.${byoc_d}" \
  >/dev/null 2>&1
cat > "${byoc_dir}/config.env" <<CFG
AMP_VERSION=0.15.0
DOMAIN_BASE=${byoc_d}
TLS_MODE=byoc
TLS_CERT_FILE=${byoc_dir}/cert.pem
TLS_KEY_FILE=${byoc_dir}/key.pem
CFG
byoc_out="$(bash "$ADV" --config "${byoc_dir}/config.env" --dry-run 2>&1)"
assert_eq "byoc dry-run renders a TLS Secret"      "yes" "$(has "$byoc_out" 'type: kubernetes.io/tls')"
assert_eq "byoc dry-run renders the gateway"       "yes" "$(has "$byoc_out" 'kind: Gateway')"
assert_eq "byoc dry-run renders frontproxy routes" "yes" "$(has "$byoc_out" 'amp-frontproxy-dataplane')"
assert_eq "byoc dry-run has no ClusterIssuer"      "no"  "$(has "$byoc_out" 'kind: ClusterIssuer')"
assert_eq "byoc dry-run has no Certificate"        "no"  "$(has "$byoc_out" 'kind: Certificate')"
assert_eq "byoc dry-run never prints the key"      "no"  "$(has "$byoc_out" 'PRIVATE KEY')"
assert_eq "byoc dry-run never prints the cert"     "no"  "$(has "$byoc_out" 'BEGIN CERTIFICATE')"
# No TLS_CA_FILE in this config, so no CA ConfigMap should be rendered.
assert_eq "byoc without TLS_CA_FILE: no CA cm"     "no"  "$(has "$byoc_out" 'name: amp-platform-ca')"

# With TLS_CA_FILE the installer must persist the CA, so environments created later
# can trust the listener instead of falling back to the wrong in-cluster root.
cp "${byoc_dir}/cert.pem" "${byoc_dir}/ca.pem"
printf 'TLS_CA_FILE=%s/ca.pem\n' "$byoc_dir" >> "${byoc_dir}/config.env"
ca_out="$(bash "$ADV" --config "${byoc_dir}/config.env" --dry-run 2>&1)"
assert_eq "byoc with TLS_CA_FILE renders CA cm"    "yes" "$(has "$ca_out" 'name: amp-platform-ca')"
# Scope the namespace check to the ConfigMap: the Gateway and the front-proxy routes in
# the same dry-run output also sit in openchoreo-control-plane, so an unscoped grep would
# pass even if the CA landed somewhere else entirely.
assert_eq "CA cm lands in the gateway namespace"   "yes" \
  "$(grep -A1 'name: amp-platform-ca' <<<"$ca_out" | grep -q 'namespace: openchoreo-control-plane' && echo yes || echo no)"
assert_eq "byoc with TLS_CA_FILE still hides key"  "no"  "$(has "$ca_out" 'PRIVATE KEY')"

# A cert missing one dynamic-tier wildcard must be rejected before any cluster work,
# naming the SAN. *.<DOMAIN_BASE> covers the service hosts but NOT the deeper tiers.
openssl req -x509 -newkey rsa:2048 -nodes -days 90 \
  -keyout "${byoc_dir}/bad-key.pem" -out "${byoc_dir}/bad-cert.pem" -subj "/CN=${byoc_d}" \
  -addext "subjectAltName=DNS:*.${byoc_d},DNS:*.agents.${byoc_d}" \
  >/dev/null 2>&1
sed -e "s#${byoc_dir}/cert.pem#${byoc_dir}/bad-cert.pem#" -e "s#${byoc_dir}/key.pem#${byoc_dir}/bad-key.pem#" \
  "${byoc_dir}/config.env" > "${byoc_dir}/bad.env"
bad_out="$(bash "$ADV" --config "${byoc_dir}/bad.env" --dry-run 2>&1)"; bad_rc=$?
assert_eq "byoc bad-SAN cert exits non-zero"  "1"   "$bad_rc"
assert_eq "byoc bad-SAN names the missing SAN" "yes" "$(has "$bad_out" "missing the wildcard SAN: *.gateway.${byoc_d}")"
assert_eq "byoc bad-SAN stops before install" "no"  "$(has "$bad_out" 'DRY RUN — derived hosts')"
rm -rf "$byoc_dir"

# --- inotify_bump_target: bump to floor only when below it (or unreadable) ---
assert_eq "inotify below floor bumps"         "512"    "$(inotify_bump_target 128 512)"
assert_eq "inotify at floor no bump"          ""       "$(inotify_bump_target 512 512)"
assert_eq "inotify above floor never lowers"  ""       "$(inotify_bump_target 1024 512)"
assert_eq "inotify empty current bumps"       "512"    "$(inotify_bump_target "" 512)"
assert_eq "inotify non-numeric current bumps" "512"    "$(inotify_bump_target foo 512)"
assert_eq "inotify watches floor"             "524288" "$(inotify_bump_target 8192 524288)"

# --- _public_ip: succeeds with empty output when every endpoint is unreachable
# (egress-restricted VM). install-advanced.sh assigns its output under set -e, so a
# non-zero status would abort the install instead of skipping the public-IP candidate. ---
(
  # shellcheck disable=SC2329  # invoked indirectly by _public_ip
  curl() { return 6; }   # shadow the binaries: every probe fails
  # shellcheck disable=SC2329
  wget() { return 4; }
  out="$(_public_ip)"; rc=$?
  assert_eq "_public_ip offline exit status"  "0" "$rc"
  assert_eq "_public_ip offline output empty" ""  "$out"
)

if [[ -s "$FAILLOG" ]]; then echo "TESTS FAILED"; exit 1; fi
echo "ALL TESTS PASSED"
