#!/usr/bin/env bash
# lib-vm.sh — pure helpers for the VM standalone install.
# Sourcing this file has no side effects; every function writes only to stdout.
#
# The VM installer is Let's Encrypt only and 443-only: certificates issue via the
# TLS-ALPN-01 ACME challenge (inside the :443 handshake), so no inbound port 80 is
# ever required and every public URL is https.

# vm_host <subdomain> <ip> -> "<sub>.amp.<ip>.sslip.io"
vm_host() {
  printf '%s.amp.%s.sslip.io' "$1" "$2"
}

# build_amp_helm_args <ip> <external_gateways:true|false (default true)>
# Prints helm args, one token per line (--set and KEY=VALUE on separate lines).
# Consume with (bash >=4):  mapfile -t ARR < <(build_amp_helm_args ...)
# bash 3.2 (macOS):         while IFS= read -r l; do ARR+=("$l"); done < <(build_amp_helm_args ...)
# amp_helm_args — hostname-driven core. Reads AMP_HOST_API/THUNDER/CONSOLE/OBSERVER/
# GATEWAY/CP (CP empty => external gateways off). Emits one helm token per line.
#
# The service config lives under different top-level keys across chart versions:
# `agentManager` (<=main) was renamed to `agentManagerService` (>=0.15.0). Emit
# both; helm silently ignores whichever key the installed chart doesn't define,
# so the right one always wins regardless of the --version pulled.
#
# config.tlsEnabled (env TLS_ENABLED) selects which advertised endpoint variant
# amp-api hands the console for deployed agents: when true it emits the https URL
# from the release binding instead of the http one. It does NOT change amp-api's
# own serving (that is internalServer.tlsEnabled) — it is purely the endpoint
# scheme. The agent host is only reachable over TLS via Caddy's wildcard site, so
# without this the console emits http:// and the browser blocks it as mixed content.
#
# VERSION SKEW: current charts derive the advertised authorization server from
# the issuer, and append publicURL plus a trailing slash to the audience. This
# installer targets whatever published AMP_VERSION the caller asks for, including
# releases predating that derivation, where the defaults pin the k3d hostnames
# these functions have just moved. So both this function and
# observability_helm_args below restate the derived values explicitly. Commas
# stay escaped or helm's --set splits the value into a list.
#
# "urn:wso2:amp" is the amp resource server's identifier — see
# 60-amp-resource-server.yaml in the Thunder extension chart. If a future
# AMP_VERSION changes that identifier again, this value needs to move with it,
# same as the hostnames above.
#
# thunder.baseURL must be the public Thunder URL (same as keyManager.issuer),
# not the in-cluster service address, because it also derives the RFC 8707
# resource identifier admin API tokens are scoped to. thunder.resolveToHost
# points the actual connection at the in-cluster service instead, since the
# public hostname doesn't resolve from inside the cluster. Leaving either
# unset reproduces AUTH-4030/invalid_target failures on every admin identity
# call (users/roles/groups).
# shellcheck disable=SC2154  # AMP_HOST_* come from the caller's scope by design.
amp_helm_args() {
  local k
  for k in agentManager agentManagerService; do
    printf '%s\n' \
      "--set" "${k}.config.serverPublicURL=https://${AMP_HOST_API}" \
      "--set" "${k}.config.oauthAuthorizationServers=https://${AMP_HOST_THUNDER}" \
      "--set" "${k}.config.keyManager.issuer=https://${AMP_HOST_THUNDER}" \
      "--set" "${k}.config.keyManager.audience=urn:wso2:amp\,amp-console-client\,amp-api-client\,amp-publisher-*\,amctl\,am-mcp\,https://${AMP_HOST_API}/mcp" \
      "--set" "${k}.config.thunder.baseURL=https://${AMP_HOST_THUNDER}" \
      "--set" "${k}.config.thunder.resolveToHost=amp-thunder-extension-service.amp-thunder.svc.cluster.local:8090" \
      "--set" "${k}.config.tlsEnabled=true" \
      "--set" "${k}.config.idpHostBaseDomain=${AMP_HOST_THUNDER#thunder.}" \
      "--set" "${k}.config.agentsBaseDomain=${AMP_AGENTS_BASE}" \
      "--set" "${k}.config.agentsHttpPort=443" \
      "--set" "${k}.config.agentsHttpsPort=443" \
      "--set" "${k}.config.gatewayBaseDomain=${AMP_HOST_GATEWAY}" \
      "--set" "${k}.config.gatewayVhostScheme=https" \
      "--set" "${k}.config.gatewayVhostPort=443"
  done

  printf '%s\n' \
    "--set" "console.config.auth.baseUrl=https://${AMP_HOST_THUNDER}" \
    "--set" "console.config.auth.signInRedirectURL=https://${AMP_HOST_CONSOLE}/login" \
    "--set" "console.config.auth.signOutRedirectURL=https://${AMP_HOST_CONSOLE}/login" \
    "--set" "console.config.apiBaseUrl=https://${AMP_HOST_API}" \
    "--set" "agentManagerService.config.amObserverPublicURL=https://${AMP_HOST_OBSERVER}" \
    "--set" "console.config.instrumentationUrl=https://${AMP_HOST_GATEWAY}/otel" \
    "--set" "console.config.idpHostBaseDomain=${AMP_HOST_THUNDER#thunder.}" \
    "--set" "console.config.tlsEnabled=true"

  # Console and API are ClusterIP behind the OC control-plane kgateway; their
  # HTTPRoutes must match the public hosts Caddy forwards (Host is preserved).
  # Older charts (agentManager key era) predate ocIngress and ignore these.
  printf '%s\n' \
    "--set" "console.ocIngress.hostname=${AMP_HOST_CONSOLE}" \
    "--set" "agentManagerService.ocIngress.hostname=${AMP_HOST_API}"

  if [[ -n "$AMP_HOST_CP" ]]; then
    # Full URL: the console parses it with new URL() to build gateway setup commands.
    printf '%s\n' "--set" "console.config.gatewayControlPlaneUrl=https://${AMP_HOST_CP}"
    # External gateways ride the OC control-plane kgateway like everything
    # else; the gateway-mgmt HTTPRoute must match the public cp host.
    printf '%s\n' "--set" "agentManagerService.ocIngress.gatewayMgmt.hostnames={${AMP_HOST_CP}}"
  fi
}

# build_amp_helm_args <ip> <external_gateways:true|false> — sslip.io-from-IP wrapper.
build_amp_helm_args() {
  local ip="$1" external_gateways="${2:-true}"
  local AMP_HOST_API AMP_HOST_THUNDER AMP_HOST_CONSOLE AMP_HOST_OBSERVER AMP_HOST_GATEWAY AMP_HOST_CP AMP_AGENTS_BASE
  AMP_HOST_API="$(vm_host api "$ip")"
  AMP_HOST_THUNDER="$(vm_host thunder "$ip")"
  AMP_HOST_CONSOLE="$(vm_host console "$ip")"
  AMP_HOST_OBSERVER="$(vm_host observer "$ip")"
  AMP_HOST_GATEWAY="$(vm_host gateway "$ip")"
  AMP_AGENTS_BASE="agents.${ip}.sslip.io"
  AMP_HOST_CP=""; [[ "$external_gateways" == "true" ]] && AMP_HOST_CP="$(vm_host cp "$ip")"
  amp_helm_args
}

# build_gateway_helm_args <ip>
# Prints GATEWAY_HELM_ARGS tokens. Sets the published vhost so deployed-agent
# endpoint URLs are externally reachable (path-routed under this single host),
# and points the gateway runtime's user-token key manager (ThunderKeyManager) at
# the public Thunder issuer. The runtime validates the JWT `iss` claim
# (validateissuer=true); user tokens are minted by the public Thunder, so without
# this invoking a deployed agent 401s.
#
# The whole keymanagers list is supplied via --set-json: a `--set keymanagers[1].issuer`
# does NOT merge into the chart's list, it replaces it with [null, {issuer}], which
# wipes keymanager[0] (agent-manager-service, used for OTel ingest) and the name/jwks
# of [1] -> malformed config.toml -> gateway crash loop. So both entries are restated
# in full; only the ThunderKeyManager issuer differs from the chart default. This is
# chart-version-coupled: re-verify both keymanagers (names + jwks URIs) on chart bumps.
# gateway_helm_args — hostname-driven core. Reads AMP_HOST_GATEWAY, AMP_HOST_THUNDER.
#
# gateway.hostname MUST match the host in gateway.vhost. vhost is the public URL the
# controller mints into LLM-proxy endpoints; hostname is the host the kgateway
# catch-all HTTPRoute matches to forward traffic to the gateway runtime. Left unset,
# hostname defaults to "<env>-<org>.gateway.localhost", which never matches the public
# vhost the LLM judge calls, so the proxy request 404s at kgateway. Keep them aligned.
# shellcheck disable=SC2154  # AMP_HOST_* come from the caller's scope by design.
gateway_helm_args() {
  local thunder keymanagers
  thunder="https://${AMP_HOST_THUNDER}"
  keymanagers=$(printf '[{"name":"agent-manager-service","issuer":"agent-manager-service","jwks":{"remote":{"uri":"http://amp-api.wso2-amp.svc.cluster.local:9000/auth/external/jwks.json","skipTlsVerify":true}}},{"name":"ThunderKeyManager","issuer":"%s","jwks":{"remote":{"uri":"http://amp-thunder-extension-service.amp-thunder:8090/oauth2/jwks","skipTlsVerify":true}}}]' "$thunder")
  printf '%s\n' \
    "--set" "gateway.vhost=https://${AMP_HOST_GATEWAY}" \
    "--set" "gateway.hostname=${AMP_HOST_GATEWAY}" \
    "--set-json" "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers=${keymanagers}"
}

# build_gateway_helm_args <ip> — sslip.io-from-IP wrapper.
build_gateway_helm_args() {
  local ip="$1"
  local AMP_HOST_GATEWAY AMP_HOST_THUNDER
  AMP_HOST_GATEWAY="$(vm_host gateway "$ip")"
  AMP_HOST_THUNDER="$(vm_host thunder "$ip")"
  gateway_helm_args
}

# build_observability_helm_args <ip>
# Prints OBSERVABILITY_HELM_ARGS tokens. The observer validates the same
# user token (its `iss` must match), so the console's traces page 401s until its
# issuer is the public Thunder URL too. jwksUrl stays on the in-cluster service.
# observability_helm_args — hostname-driven core. Reads AMP_HOST_THUNDER and
# AMP_HOST_OBSERVER.
#
# authorizationServers/audience are restated for the version-skew reason noted
# above amp_helm_args (including the "urn:wso2:amp" identifier value). The last
# audience entry is the observer MCP token's `aud` (its public `/mcp`
# resource); the first three cover console/amctl tokens.
# shellcheck disable=SC2154  # AMP_HOST_* come from the caller's scope by design.
observability_helm_args() {
  printf '%s\n' \
    "--set" "amObserver.auth.issuer=https://${AMP_HOST_THUNDER}" \
    "--set" "amObserver.ocIngress.hostname=${AMP_HOST_OBSERVER}" \
    "--set" "amObserver.publicUrl=https://${AMP_HOST_OBSERVER}" \
    "--set" "amObserver.oauth.authorizationServers=https://${AMP_HOST_THUNDER}" \
    "--set" "amObserver.auth.audience=urn:wso2:amp\,amp-api-client\,am-obs-mcp\,https://${AMP_HOST_OBSERVER}/mcp"
}

# build_observability_helm_args <ip> — sslip.io-from-IP wrapper.
build_observability_helm_args() {
  local ip="$1"
  local AMP_HOST_THUNDER AMP_HOST_OBSERVER
  AMP_HOST_THUNDER="$(vm_host thunder "$ip")"
  AMP_HOST_OBSERVER="$(vm_host observer "$ip")"
  observability_helm_args
}

# build_cp_helm_args <ip>
# Prints CP_HELM_ARGS tokens for the OpenChoreo control-plane install. Thunder's
# issuer is moved to the public sslip.io URL, so the OpenChoreo CP OIDC config
# (which validates the issuer string statically) must accept that same issuer —
# otherwise amp-api -> OpenChoreo calls fail with "INVALID_CLAIMS". jwksUrl /
# wellKnownEndpoint stay on the internal service (they still resolve in-cluster).
# cp_helm_args — hostname-driven core. Reads AMP_HOST_THUNDER.
# shellcheck disable=SC2154  # AMP_HOST_THUNDER comes from the caller's scope by design.
cp_helm_args() {
  printf '%s\n' \
    "--set" "security.oidc.issuer=https://${AMP_HOST_THUNDER}" \
    "--set" "security.oidc.authorizationUrl=https://${AMP_HOST_THUNDER}/oauth2/authorize" \
    "--set" "security.oidc.tokenUrl=https://${AMP_HOST_THUNDER}/oauth2/token"
}

# build_cp_helm_args <ip> — sslip.io-from-IP wrapper.
build_cp_helm_args() {
  local ip="$1"
  local AMP_HOST_THUNDER
  AMP_HOST_THUNDER="$(vm_host thunder "$ip")"
  cp_helm_args
}

# build_platform_resources_helm_args
# Prints PLATFORM_RESOURCES_HELM_ARGS tokens. Two overrides:
#
# 1. OAuth token endpoint. The platform-resources chart's workload-publisher
#    defaults its OAuth token endpoint to the kgateway path
#    (`host.k3d.internal:8080/oauth2/token` + Host `thunder.amp.localhost`). On the
#    VM that route no longer matches: build_cp_helm_args / build_thunder_helm_args
#    move Thunder's vhost to the public sslip.io host, so the localhost Host header
#    404s and `generate-workload-cr` fails with "Failed to get access token". Point
#    it at the Thunder service directly (no gateway, no Host header, no issuer
#    coupling) — the same in-cluster endpoint every other extension already uses.
#
# 2. Default-Environment agent-facing gateway host. The chart seeds the default
#    Environment CR with environment.gateway.http (default am-gateway.localhost:19080).
#    OpenChoreo builds deployed-agent route hostnames as "<env>-<org>.<that host>",
#    and the Environment's external WHOLLY REPLACES the dataplane's, so without this
#    override agent routes land on *.am-gateway.localhost: the console then shows an
#    empty invoke URL and try-out 405s against its own host. Point it at the public
#    agents base (AMP_AGENTS_BASE, e.g. agents.<domain> or agents.<ip>.sslip.io) on
#    :443 so routes resolve to <org>-<project>.<AMP_AGENTS_BASE>, served by Caddy's
#    wildcard *.agents site.
#
#    Set BOTH the http and https variants. Because the override wholly replaces the
#    dataplane's external endpoint, an http-only override drops the https variant
#    dataplane_external_ingress emits. The console reads the https variant when
#    tlsEnabled=true, so an http-only override leaves the binding with only an http
#    externalURL: the invoke URL is empty and try-out falls back to a relative /chat
#    (405) — the very symptom this override exists to fix. Both bind listenerName
#    http (TLS terminates at Caddy) and differ only in advertised scheme.
#
# 2. apiPlatformGatewayVhost.otelEndpointOverride is deliberately NOT set. Deployed
#    agents run inside this cluster, so they reach the gateway runtime directly on
#    the chart's default in-cluster endpoint
#    ("api-platform-<org>-<env>-gateway-gateway-runtime.<org>-<env>:22893/otel").
#    Overriding it with the public gateway host makes every agent egress to
#    <AMP_HOST_GATEWAY>:443 instead, which the sandbox NetworkPolicy refuses on a
#    private-network VM: it allows :443 to 0.0.0.0/0 EXCEPT RFC-1918, and there the
#    public hostname resolves to the VM's own private address. Trace export then
#    fails with "Connection refused". On a public VM the same override happens to
#    work only because the hostname resolves outside those ranges. The in-cluster
#    endpoint is what the sandbox policy explicitly permits and works on both.
# shellcheck disable=SC2154  # AMP_AGENTS_BASE comes from the caller's scope by design.
build_platform_resources_helm_args() {
  printf '%s\n' \
    "--set" "global.oauth.tokenUrl=http://amp-thunder-extension-service.amp-thunder.svc.cluster.local:8090/oauth2/token" \
    "--set" "environment.gateway.http.host=${AMP_AGENTS_BASE}" \
    "--set" "environment.gateway.http.port=443" \
    "--set" "environment.gateway.https.host=${AMP_AGENTS_BASE}" \
    "--set" "environment.gateway.https.port=443"
}

# build_thunder_helm_args <ip>
# Prints helm args, one token per line.
# thunder_helm_args — hostname-driven core. Reads AMP_HOST_THUNDER, AMP_HOST_CONSOLE,
# AMP_HOST_API, AMP_HOST_OBSERVER.
# shellcheck disable=SC2154  # AMP_HOST_* come from the caller's scope by design.
thunder_helm_args() {
  printf '%s\n' \
    "--set" "thunder.ocIngress.hostname=${AMP_HOST_THUNDER}" \
    "--set" "thunder.configuration.server.publicUrl=https://${AMP_HOST_THUNDER}" \
    "--set" "thunder.configuration.jwt.issuer=https://${AMP_HOST_THUNDER}" \
    "--set" "thunder.configuration.gateClient.hostname=${AMP_HOST_THUNDER}" \
    "--set" "thunder.configuration.gateClient.scheme=https" \
    "--set" "thunder.configuration.gateClient.port=443" \
    "--set" "thunder.configuration.cors.allowedOrigins[0]=https://${AMP_HOST_CONSOLE}"

  # RFC 8707 resource indicators for the two MCP endpoints. Thunder matches the
  # authorize request's `resource` parameter against each resource server's
  # identifier verbatim and answers invalid_target on any mismatch, so the
  # identifiers must be the hosts an MCP client actually dials — not the chart's
  # localhost defaults, which describe a k3d install. Both MCP endpoints are
  # served by services already exposed here: the agent-manager one by amp-api and
  # the observer one by the observer itself.
  #
  # The bootstrap template builds each identifier from a base-URL scalar and
  # appends the trailing slash, so override those two scalars rather than the
  # mcpResourceServers list. A scalar --set merges; --set on a list element
  # replaces the whole element and would drop its name/handle/description/
  # permissionSet, which makes Thunder reject the resource server (empty name)
  # and the bootstrap fail. Pass the base URL without a trailing slash — the
  # template adds it.
  printf '%s\n' \
    "--set" "thunder.bootstrap.agentManagerMcpBaseUrl=https://${AMP_HOST_API}" \
    "--set" "thunder.bootstrap.observerMcpBaseUrl=https://${AMP_HOST_OBSERVER}"

  # The console client's registered redirect URI lives under `setup` (<=main) and
  # was renamed to `bootstrap` (>=0.15.0, which is what the registration template
  # actually reads). Emit both; helm ignores the inert one. Must match the
  # console's signInRedirectURL or Thunder rejects login with "Invalid redirect URI".
  local k
  for k in setup bootstrap; do
    printf '%s\n' "--set" "thunder.${k}.ampConsoleClient.redirectUris[0]=https://${AMP_HOST_CONSOLE}/login"
  done

  # Fixes the console admin login at "admin" for convenience — the chart's
  # own default (unset) would generate a random one instead, which is what a
  # publicly reachable production install wants but is unnecessary friction
  # on a VM only the operator controls.
  printf '%s\n' "--set" "thunder.setup.admin.password=admin"
}

# build_thunder_helm_args <ip> — sslip.io-from-IP wrapper.
build_thunder_helm_args() {
  local ip="$1"
  local AMP_HOST_THUNDER AMP_HOST_CONSOLE AMP_HOST_API AMP_HOST_OBSERVER
  AMP_HOST_THUNDER="$(vm_host thunder "$ip")"
  AMP_HOST_CONSOLE="$(vm_host console "$ip")"
  AMP_HOST_API="$(vm_host api "$ip")"
  AMP_HOST_OBSERVER="$(vm_host observer "$ip")"
  thunder_helm_args
}

# render_k3d_vm_config [node_host]  (reads k3d config on stdin, writes VM config on stdout)
# Two rewrites:
#  1. '- port: <host>:<container>' -> '- port: 127.0.0.1:<host>:<container>' so the
#     k3d host ports bind to loopback only. Already-bound entries are left untouched.
#  2. The containerd registry mirror *endpoint* host.k3d.internal:10082 -> <node_host>:10082.
#     The mirror key stays host.k3d.internal:10082 (it must match the image tag the
#     publish step writes), but the node's containerd resolves host.k3d.internal via
#     its own /etc/hosts to the Docker bridge gateway — which has nothing listening
#     once ports are loopback-bound, so agent image pulls fail with ImagePullBackOff.
#     The node *can* reach the registry LoadBalancer at its own node hostname, which
#     k3d puts in the node's /etc/hosts (IP-independent). Pod-side DNS is handled
#     separately by render_coredns_vm_config; this covers the node containerd path.
render_k3d_vm_config() {
  local node_host="${1:-k3d-amp-local-server-0}"
  sed -E \
    -e 's/^([[:space:]]*- port: )([0-9]+:[0-9]+)/\1127.0.0.1:\2/' \
    -e "s#^([[:space:]]*- )http://host\\.k3d\\.internal:10082#\\1http://${node_host}:10082#"
}

# render_k3d_advanced_config [node_host]  (reads k3d config on stdin, writes on stdout)
# Like render_k3d_vm_config (loopback-binds every plane port + rewrites the registry
# mirror endpoint), but ALSO publishes :443 to the host so the consolidated kgateway
# Gateway is reachable from the network. The advanced install has no Caddy: kgateway
# terminates TLS on :443 directly, and only :443 faces the network (every other plane
# port stays loopback-bound). Injected right after the `ports:` key so k3d's serverlb
# forwards host :443 to the node, where klipper binds it for the gateway's LoadBalancer.
render_k3d_advanced_config() {
  render_k3d_vm_config "$@" | awk '
    { print }
    /^ports:/ { print "  - port: 443:443"; print "    nodeFilters:"; print "      - loadbalancer" }
  '
}

# render_coredns_vm_config <node_host>
# Prints a `coredns-custom` ConfigMap that rewrites the in-cluster *.localhost /
# host.k3d.internal names to the k3d server node (<node_host>, e.g.
# k3d-amp-local-server-0), instead of the base config's `host.k3d.internal`.
#
# Why the VM needs this: the stock config points these names at host.k3d.internal,
# which ensure_coredns_host_aliases maps to the Docker bridge gateway (the host),
# relying on a host hairpin to the published service ports. But the VM installer
# binds every k3d host port to 127.0.0.1 (render_k3d_vm_config), so the gateway IP
# has nothing listening — observer->authz (build logs) and the registry push/pull
# both fail with "connection refused". The server node is where klipper exposes
# all the LoadBalancer service ports, so rewriting straight to its hostname is
# reachable and, unlike a NodeHosts alias, survives k3s NodeHosts reconciliation
# (the node entry is always present). Applied via install.sh's COREDNS_FILE hook.
render_coredns_vm_config() {
  local node_host="$1"
  cat <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: coredns-custom
  namespace: kube-system
data:
  amp.override: |
    rewrite stop {
      name regex (.+\\.)?amp\\.localhost ${node_host}
      answer auto
    }
  openchoreo.override: |
    rewrite stop {
      name regex (.+\\.)?openchoreo\\.localhost ${node_host}
      answer auto
    }
  hostalias.override: |
    rewrite stop {
      name regex (host\\.k3d\\.internal|host\\.docker\\.internal) ${node_host}
      answer auto
    }
EOF
}

# render_dataplane_external_ingress <ip>
# Prints the `external:` http/https entries for install.sh's ClusterDataPlane
# (DP_EXTERNAL_INGRESS hook), advertising deployed-agent endpoints under the
# public host <org>-<project>.agents.<ip>.sslip.io instead of the local default
# openchoreoapis.localhost:19080.
#
# Emits BOTH entries on :443, bound to the internal http listener (TLS is
# terminated at Caddy's wildcard *.agents site). Both variants resolve to the same
# host:port/path and differ only in scheme; amp-api advertises the https one to the
# console (build_amp_helm_args sets config.tlsEnabled=true), so the browser calls
# https://...:443 directly and the wildcard site serves it. The http entry is kept
# too: a release binding missing a variant makes the console fall back to a relative
# /chat (405 from its own nginx), so emitting both keeps the binding complete.
# dataplane_external_ingress — hostname-driven core; reads AMP_AGENTS_BASE.
# shellcheck disable=SC2154  # AMP_AGENTS_BASE comes from the caller's scope by design.
dataplane_external_ingress() {
  local host="$AMP_AGENTS_BASE"
  printf '        http:\n          host: "%s"\n          listenerName: http\n          port: 443\n' "$host"
  printf '        https:\n          host: "%s"\n          listenerName: http\n          port: 443\n' "$host"
}

# render_dataplane_external_ingress <ip> — sslip.io-from-IP wrapper.
render_dataplane_external_ingress() {
  local AMP_AGENTS_BASE="agents.${1}.sslip.io"
  dataplane_external_ingress
}

# caddyfile <tls_mode> <email> <cert_file> <key_file> <listen_port>
# Hostname-driven core. Reads AMP_HOST_CONSOLE/API/THUNDER/OBSERVER/GATEWAY/CP
# (CP empty => no cp site) and AMP_AGENTS_BASE. tls_mode is one of:
#   letsencrypt  — terminate TLS on :443, auto-ACME via TLS-ALPN-01 (no port 80),
#                  on-demand certs for the per-agent wildcard.
#   byoc         — terminate TLS on :443 with the operator-supplied cert/key (no ACME).
#   upstream     — a load balancer / proxy in front terminates TLS; Caddy listens on
#                  plain HTTP at <listen_port> and only routes by Host.
# In every mode the published URLs are https:// (what the browser sees); only Caddy's
# listener differs.
# shellcheck disable=SC2154  # AMP_HOST_*/AMP_AGENTS_BASE come from the caller's scope.
caddyfile() {
  local tls_mode="$1" email="$2" cert_file="$3" key_file="$4" listen_port="${5:-80}" trusted_proxies="${6:-0.0.0.0/0}" ask_secret="${7:-}"
  local console_origin="https://${AMP_HOST_CONSOLE}"

  # Per-mode building blocks computed once.
  local tls_block="" agent_tls="" gopts="" scheme="https" addr_suffix=""
  case "$tls_mode" in
    letsencrypt)
      # Every public site forces the TLS-ALPN-01 ACME challenge (it runs inside the
      # :443 TLS handshake) so certificate issuance never depends on inbound port 80.
      #
      # The fixed hosts (console/api/thunder/observer/gateway/cp) get on_demand as
      # well, even though their names are known here and could be issued eagerly.
      # They have to: the env-Thunder site below is a wildcard sitting directly under
      # the base domain (*.amp.<ip>.sslip.io), so it COVERS every fixed host, and
      # Caddy skips eager issuance for any name a managed wildcard covers. That
      # wildcard is itself on-demand, because TLS-ALPN-01 cannot issue a wildcard —
      # so with the fixed hosts left eager, nothing ever obtains their certificates:
      # Caddy logs "enabling automatic TLS certificate management" for all of them,
      # never attempts a single obtain (no ACME account, empty certificate store),
      # and every request dies in the handshake with an internal error, on a cluster
      # that is otherwise entirely healthy. on_demand moves issuance to the first
      # handshake per host, and prewarm_fixed_host_certs walks them at install time
      # so no user's request is the one that races it.
      agent_tls=$'\ttls {\n\t\ton_demand\n\t\tissuer acme {\n\t\t\tdisable_http_challenge\n\t\t}\n\t}\n'
      tls_block="$agent_tls"
      [[ -n "$email" ]] && gopts+=$'\temail '"$email"$'\n'
      gopts+=$'\tauto_https disable_redirects\n'
      gopts+=$'\ton_demand_tls {\n\t\task http://127.0.0.1:9753/internal/thunder-ask\n\t}\n'
      ;;
    byoc)
      # Serve the operator-supplied cert/key on every site (incl. the agent wildcard,
      # whose cert must carry the *.<AGENTS_BASE> SAN — enforced by validate_cert).
      tls_block=$'\ttls '"$cert_file"' '"$key_file"$'\n'
      agent_tls="$tls_block"
      gopts+=$'\tauto_https disable_redirects\n'
      ;;
    upstream)
      # The LB owns the public cert; Caddy routes plain HTTP and trusts the LB's
      # X-Forwarded-* headers (so backends still see the https scheme). Only the
      # configured proxy CIDRs are trusted — scope this to the LB's source ranges
      # (and firewall :<listen_port> to the LB) so a direct caller can't spoof them.
      scheme="http"; addr_suffix=":${listen_port}"
      gopts+=$'\thttp_port '"$listen_port"$'\n'
      gopts+=$'\tservers {\n\t\ttrusted_proxies static '"$trusted_proxies"$'\n\t}\n'
      ;;
    *) printf 'caddyfile: unknown tls_mode %q\n' "$tls_mode" >&2; return 1 ;;
  esac

  printf '{\n%s}\n\n' "$gopts"

  _site() {   # _site <host> <upstream_port>
    printf '%s%s {\n%s\treverse_proxy 127.0.0.1:%s\n}\n\n' \
      "$([[ "$scheme" == http ]] && printf 'http://')" "$1$addr_suffix" "$tls_block" "$2"
  }

  # Console and API are ClusterIP behind the OC control-plane kgateway (8080),
  # which discriminates by Host header via their HTTPRoutes (amp_helm_args sets
  # the route hostnames to these public hosts; Caddy preserves Host).
  _site "$AMP_HOST_CONSOLE"  8080   # console UI (OC kgateway, host-routed)
  _site "$AMP_HOST_API"      8080   # agent-manager REST API (OC kgateway, host-routed)
  _site "$AMP_HOST_THUNDER"  8080   # Thunder OAuth (OC kgateway, host-routed)

  # Env-Thunder instances: one per org/environment, created dynamically after
  # initial install (not just at install time) — <handle>.<base-domain> (no fixed
  # "thunder." segment — see thunder-naming.sh's thunder_host), wildcard-matched
  # and proxied to the SAME kgateway listener as platform Thunder above (port
  # 8080; kgateway itself discriminates by Host header via each env-Thunder's own
  # HTTPRoute — see add-environment-thunder.sh's apply_httproute). This wildcard
  # is broader than just env-Thunder hosts, but Caddy always matches the most
  # specific site first, so the exact _site blocks above (console/api/thunder/
  # observer/gateway/cp) still win for their own hostnames; only a handle that
  # doesn't match any of those falls through to here — which
  # reservedThunderHandles (environment_service.go) prevents a handle from ever
  # equalling one of those reserved site names in the first place. A real wildcard cert can't be issued via
  # TLS-ALPN-01, so — like the agents site below — this needs on-demand TLS (one
  # concrete cert per hostname, issued the first time it's actually requested).
  printf '%s*.%s%s {\n%s\treverse_proxy 127.0.0.1:8080\n}\n\n' \
    "$([[ "$scheme" == http ]] && printf 'http://')" "${AMP_HOST_THUNDER#thunder.}" "$addr_suffix" "$agent_tls"

  # Observer is ClusterIP behind the OC observability-plane kgateway
  # (11080), host-routed the same way (observability_helm_args sets the route
  # hostname).
  _site "$AMP_HOST_OBSERVER" 11080  # observer (OC kgateway, host-routed)
  # The api-platform gateway runtime is a ClusterIP service (ports 22893/22894 are
  # not node-published), so it is reached through the kgateway data plane on 19080 —
  # which has a catch-all HTTPRoute for AMP_HOST_GATEWAY that forwards to the runtime
  # (see gateway_helm_args setting gateway.hostname). Pointing here at 22893 dead-ends
  # (Caddy gets an empty reply -> 502), breaking the publicly-minted LLM-judge proxy
  # URL. (Agent OTel ingestion is unaffected: it goes in-cluster to the runtime
  # ClusterIP, not through this host.)
  _site "$AMP_HOST_GATEWAY"  19080  # api-platform gateway via kgateway (LLM proxy)

  # Per-environment gateways (<env>-<org>.$AMP_HOST_GATEWAY, added post-install) share
  # the listener above; on-demand TLS since TLS-ALPN-01 cannot issue a wildcard cert.
  printf '%s*.%s%s {\n%s\treverse_proxy 127.0.0.1:19080\n}\n\n' \
    "$([[ "$scheme" == http ]] && printf 'http://')" "$AMP_HOST_GATEWAY" "$addr_suffix" "$agent_tls"

  if [[ -n "$AMP_HOST_CP" ]]; then
    # Gateway control plane rides the OC control-plane kgateway (host-routed;
    # the kgateway re-encrypts to the TLS backend via BackendTLSPolicy).
    # reverse_proxy upgrades the gateway control WebSocket transparently.
    _site "$AMP_HOST_CP" 8080
  fi

  # On-demand TLS ask endpoint exists only in letsencrypt mode. Caddy triggers
  # on-demand for SNI matching any wildcard site — the *.agents and per-env
  # gateway wildcards below, AND the env-Thunder base-domain wildcard above —
  # so this one shared endpoint has to answer for all three tiers.
  #
  # The gateway/agents wildcards are matched and allowed HERE, in Caddy, before
  # ever calling out to AMS: AMP_HOST_GATEWAY ("gateway.<thunder base>") is
  # itself a subdomain of the env-Thunder base domain, so a naive "does this
  # request's domain end in the base domain" check on the AMS side would treat
  # every per-env gateway host as an env-Thunder handle lookup too (and 403 it,
  # since "myenv-myorg.gateway" isn't a bare single-label handle) — matching
  # them here keeps their cert issuance independent of AMS/kgateway being up,
  # exactly like before this endpoint existed, and keeps the only genuinely
  # ambiguous case (a bare label directly under the base domain) as the one
  # that actually needs AMS's registered-handle check.
  #
  # Everything else falls through to AMS's own /internal/thunder-ask
  # (agent-manager-service/api/thunder_ask_routes.go) via the SAME kgateway
  # listener every other site already uses, with the Host header rewritten to
  # AMP_HOST_API so kgateway's Host-based routing lands on AMS rather than
  # whatever Caddy's on-demand-tls client itself sent (no new container/process
  # — this is just another Caddy site, like every _site above).
  if [[ "$tls_mode" == letsencrypt ]]; then
    # ask_secret, when non-empty, is amp-api's own auto-generated
    # thunder-ask-secret (agent-manager-service/config: THUNDER_ASK_SECRET) —
    # install-vm.sh reads it back out of the already-deployed amp Secret after
    # the "amp" Helm release exists (start_caddy runs after run_install's base
    # installer, so it's always there by this point) and passes it in here.
    # Empty when unset/unreadable (e.g. an existingSecret override with no
    # matching key) — the ask call still works either way, it just draws from
    # AMS's shared public rate-limit budget instead of its own, same as before
    # this header existed.
    local ask_secret_header=""
    [[ -n "$ask_secret" ]] && ask_secret_header=$'\t\theader_up X-Thunder-Ask-Secret '"${ask_secret}"$'\n'
    # The fixed hosts are on-demand too (see the letsencrypt tls_block above), so
    # their issuance also has to be approved here. They cannot fall through to AMS:
    # it answers for registered env-Thunder handles, which is a bare single label
    # under the base domain, and would 403 "console"/"api"/... — leaving the whole
    # platform without certificates. Matched exactly, not by suffix, so only these
    # names are covered and a look-alike still has to satisfy AMS.
    local fixed_hosts=("$AMP_HOST_CONSOLE" "$AMP_HOST_API" "$AMP_HOST_THUNDER" "$AMP_HOST_OBSERVER" "$AMP_HOST_GATEWAY")
    [[ -n "$AMP_HOST_CP" ]] && fixed_hosts+=("$AMP_HOST_CP")
    local fixed_expr="" fixed_host
    for fixed_host in "${fixed_hosts[@]}"; do
      [[ -n "$fixed_expr" ]] && fixed_expr+=" || "
      fixed_expr+="{query.domain} == \"${fixed_host}\""
    done
    printf 'http://127.0.0.1:9753 {\n\t@gateway_or_agents expression `{query.domain}.endsWith(".%s") || {query.domain}.endsWith(".%s")`\n\trespond @gateway_or_agents 200\n\t@fixed_hosts expression `%s`\n\trespond @fixed_hosts 200\n\treverse_proxy 127.0.0.1:8080 {\n\t\theader_up Host %s\n%s\t}\n}\n\n' \
      "$AMP_HOST_GATEWAY" "$AMP_AGENTS_BASE" "$fixed_expr" "$AMP_HOST_API" "$ask_secret_header"
  fi

  # Deployed-agent endpoints: <org>-<project>.<AGENTS_BASE> (one host per org/project,
  # dynamic), proxied to the data-plane gateway + CORS (the gateway adds none);
  # X-API-Key is the header the console sends the token in.
  local cors_block
  cors_block=$(printf '\theader {\n\t\tAccess-Control-Allow-Origin "%s"\n\t\tAccess-Control-Allow-Methods "GET, POST, PUT, DELETE, PATCH, OPTIONS"\n\t\tAccess-Control-Allow-Headers "Authorization, Content-Type, X-API-Key"\n\t\tAccess-Control-Allow-Credentials "true"\n\t\tAccess-Control-Max-Age "3600"\n\t\tVary Origin\n\t\tdefer\n\t}\n\t@cors_preflight method OPTIONS\n\trespond @cors_preflight 204\n' "$console_origin")
  printf '%s*.%s%s {\n%s%s\n\treverse_proxy 127.0.0.1:19080\n}\n\n' \
    "$([[ "$scheme" == http ]] && printf 'http://')" "$AMP_AGENTS_BASE" "$addr_suffix" "$agent_tls" "$cors_block"

  unset -f _site
}

# prewarm_fixed_host_certs <ip> <external_gateways:true|false (default true)>
# Walk every fixed host once so its certificate exists before anyone browses to it.
#
# Those hosts are on-demand in letsencrypt mode (see caddyfile()), so the first
# handshake to each triggers issuance — and that first request is consumed by the
# ACME challenge, so the client sees the challenge certificate rather than the real
# one, the same one-time error on-a-vm.mdx documents for the dynamic agent hosts.
# Doing the walk here means the operator's first console visit is never that request.
#
# Best-effort by design: a failure here only restores the previous behaviour
# (issued on the first real request), so it must never fail the install.
prewarm_fixed_host_certs() {
  local ip="$1" external_gateways="${2:-true}" host fqdn
  local hosts=(console api thunder observer gateway)
  [[ "$external_gateways" == "true" ]] && hosts+=(cp)
  log "Pre-warming TLS certificates for the fixed hosts"
  for host in "${hosts[@]}"; do
    fqdn="$(vm_host "$host" "$ip")"
    # --resolve pins the connection to the loopback listener, since a cloud VM
    # often cannot reach its own public IP; SNI still selects the right site, which
    # is all Caddy needs to start issuance.
    if curl -sS -o /dev/null -m 90 --retry 2 --retry-all-errors \
      --resolve "${fqdn}:443:127.0.0.1" "https://${fqdn}/" >/dev/null 2>&1; then
      log "  ${fqdn}: certificate ready"
    else
      log "  ${fqdn}: not issued yet — it will be obtained on the first request"
    fi
  done
}

# render_caddyfile <ip> <acme_email> <external_gateways:true|false (default true)>
# sslip.io-from-IP wrapper, letsencrypt only. Preserves the original output
# byte-for-byte. Prints a complete Caddyfile to stdout: 443-only, every site forces
# the TLS-ALPN-01 challenge so issuance never needs inbound port 80.
render_caddyfile() {
  local ip="$1" email="$2" external_gateways="${3:-true}" ask_secret="${4:-}"
  local AMP_HOST_CONSOLE AMP_HOST_API AMP_HOST_THUNDER AMP_HOST_OBSERVER AMP_HOST_GATEWAY AMP_HOST_CP AMP_AGENTS_BASE
  AMP_HOST_CONSOLE="$(vm_host console "$ip")"
  AMP_HOST_API="$(vm_host api "$ip")"
  AMP_HOST_THUNDER="$(vm_host thunder "$ip")"
  AMP_HOST_OBSERVER="$(vm_host observer "$ip")"
  AMP_HOST_GATEWAY="$(vm_host gateway "$ip")"
  AMP_HOST_CP=""; [[ "$external_gateways" == "true" ]] && AMP_HOST_CP="$(vm_host cp "$ip")"
  AMP_AGENTS_BASE="agents.${ip}.sslip.io"
  caddyfile letsencrypt "$email" "" "" "" "" "$ask_secret"
}
