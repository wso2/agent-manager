#!/usr/bin/env bash
# lib-certmanager.sh — render the TLS + gateway resources for the advanced VM install,
# which replaced the old lego + Caddy path. Sourcing only defines functions (no side
# effects); the render_* functions write YAML to stdout, so the caller pipes them to
# `kubectl apply`. No lego container, no systemd timer.
#
# Both TLS modes converge on one seam: a kubernetes.io/tls Secret in GATEWAY_NS that
# the consolidated :443 Gateway references by name, with kgateway terminating TLS.
#   dns01 — cert-manager (a cluster prerequisite) does the ACME DNS-01 challenge,
#           issues a wildcard cert into that Secret, and auto-renews it.
#   byoc  — render_byoc_tls_secret writes the operator's cert/key into that Secret
#           directly; no cert-manager objects are created and nothing auto-renews.
# Everything downstream of the Secret (Gateway, front-proxy routes, ReferenceGrants) is
# identical in both modes.
#
# The caller defines log()/die(); fallbacks are provided so this file is usable standalone.
command -v log >/dev/null 2>&1 || log() { printf '\033[0;34m[certmgr]\033[0m %s\n' "$*"; }
command -v die >/dev/null 2>&1 || die() { printf '\033[0;31m[certmgr] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

# The namespaces the cert-manager resources live in. The wildcard cert + its Secret must
# sit in the gateway's namespace so the consolidated :443 Gateway can reference it directly
# (a Gateway listener's certificateRefs is same-namespace by default).
CERT_MANAGER_NS="${CERT_MANAGER_NS:-cert-manager}"
GATEWAY_NS="${GATEWAY_NS:-openchoreo-control-plane}"

# The four DNS providers we support natively (cert-manager has a built-in dns01 solver for
# each). Kept identical to what the old lego path covered.
SUPPORTED_DNS_PROVIDERS="cloudflare route53 clouddns azuredns"

# dns01_required_vars <provider> — print the env-var names that MUST be set for the given
# provider (one per line). Used by validate_dns01_config. Empty output for an unknown
# provider (validate_dns01_config reports the unknown-provider error separately).
dns01_required_vars() {
  case "$1" in
    cloudflare) printf '%s\n' CLOUDFLARE_API_TOKEN ;;
    route53)    printf '%s\n' AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_REGION ;;
    clouddns)   printf '%s\n' GCP_PROJECT GCP_SERVICE_ACCOUNT_FILE ;;
    azuredns)   printf '%s\n' AZURE_TENANT_ID AZURE_CLIENT_ID AZURE_CLIENT_SECRET AZURE_SUBSCRIPTION_ID AZURE_RESOURCE_GROUP ;;
  esac
}

# validate_dns01_config — confirm DNS_PROVIDER is one we support and every credential it
# needs is set. Appends to CONFIG_ERRORS (does not reset it — the caller owns that array).
validate_dns01_config() {
  local p="${DNS_PROVIDER:-}" v
  case " ${SUPPORTED_DNS_PROVIDERS} " in
    *" ${p} "*) ;;
    *) CONFIG_ERRORS+=("DNS_PROVIDER must be one of: ${SUPPORTED_DNS_PROVIDERS} (got '${p:-<unset>}')"); return ;;
  esac
  while IFS= read -r v; do
    [[ -n "${!v:-}" ]] || CONFIG_ERRORS+=("${v} is required for DNS_PROVIDER=${p}")
  done < <(dns01_required_vars "$p")
  # clouddns needs the service-account JSON file to exist and be readable.
  if [[ "$p" == clouddns && -n "${GCP_SERVICE_ACCOUNT_FILE:-}" && ! -r "${GCP_SERVICE_ACCOUNT_FILE}" ]]; then
    CONFIG_ERRORS+=("GCP_SERVICE_ACCOUNT_FILE not readable: ${GCP_SERVICE_ACCOUNT_FILE}")
  fi
}

# cert_dns_names — print the SAN hostnames (one per line) the wildcard cert must cover:
# every fixed service host + the three dynamic-tier wildcards. Reads
# AMP_HOST_*/AMP_AGENTS_BASE from the caller's scope (matches the lib-vm.sh cores). CP is
# omitted when AMP_HOST_CP is empty (external gateways off).
# shellcheck disable=SC2154,SC2153  # AMP_HOST_*/AMP_AGENTS_BASE come from the caller's scope by design.
cert_dns_names() {
  printf '%s\n' "$AMP_HOST_CONSOLE" "$AMP_HOST_API" "$AMP_HOST_THUNDER" \
    "$AMP_HOST_OBSERVER" "$AMP_HOST_GATEWAY"
  [[ -n "${AMP_HOST_CP:-}" ]] && printf '%s\n' "$AMP_HOST_CP"
  # and the per-environment api-platform gateways <env>-<org>.<GATEWAY_HOST> that
  # add-environment.sh installs. A wildcard covers each without re-issuing.
  # Every env-Thunder handle sits directly under the base domain (no "thunder."
  # segment — see thunder-naming.sh's thunder_host), covered by the base-domain
  # wildcard below.
  printf '*.%s\n' "$AMP_AGENTS_BASE"
  printf '*.%s\n' "${AMP_HOST_THUNDER#thunder.}"
  printf '*.%s\n' "$AMP_HOST_GATEWAY"
}

# acme_dns_names — cert_dns_names() reduced to the names an ACME CA will accept in a
# SINGLE order. Let's Encrypt (Boulder) rejects an order carrying both a wildcard and a
# name that wildcard already covers: "Domain name <host> is redundant with a wildcard
# domain in the same request". That rejection is fatal at order creation, so the Order
# goes straight to `errored` and NO Challenge is ever created — the install fails looking
# like a DNS-01 problem while `kubectl get challenge -A` shows nothing at all.
#
# Every fixed host sits exactly one label under DOMAIN_BASE, so *.<DOMAIN_BASE> already
# covers all of them and dropping them changes nothing about what the issued cert serves.
# The wildcards are kept verbatim: *.agents.<base> and *.gateway.<base> are one label
# deeper than *.<base> reaches, so none of the three is redundant with another.
#
# byoc deliberately keeps the full cert_dns_names() list — no ACME is involved there, and
# validate_cert's coverage check is wildcard-aware, so a supplied certificate may carry
# the explicit hosts, the wildcards, or both.
acme_dns_names() {
  local -a names=() wildcards=()
  local n w base covered
  while IFS= read -r n; do [[ -n "$n" ]] && names+=("$n"); done < <(cert_dns_names)
  for n in "${names[@]}"; do
    [[ "$n" == \*.* ]] && wildcards+=("$n")
  done
  for n in "${names[@]}"; do
    if [[ "$n" != \*.* ]]; then
      covered=false
      for w in "${wildcards[@]}"; do
        base="${w#\*.}"
        # A wildcard matches exactly one label, so only a single-label child is covered.
        if [[ "$n" == *."$base" && "${n%."$base"}" != *.* ]]; then covered=true; break; fi
      done
      [[ "$covered" == true ]] && continue
    fi
    printf '%s\n' "$n"
  done
}

# _dns01_solver_block — print the cert-manager `dns01:` solver body for DNS_PROVIDER,
# indented 10 spaces to sit under `solvers:\n  - dns01:` in render_acme_clusterissuer.
# Credential values come from the config env vars; the referenced Secret is created by
# render_dns01_credentials_secret. Reads DNS_PROVIDER + provider vars from the environment.
_dns01_solver_block() {
  local secret="$1"
  case "$DNS_PROVIDER" in
    cloudflare)
      cat <<EOF
          cloudflare:
            apiTokenSecretRef:
              name: ${secret}
              key: api-token
EOF
      ;;
    route53)
      cat <<EOF
          route53:
            region: ${AWS_REGION}
            accessKeyID: ${AWS_ACCESS_KEY_ID}
            secretAccessKeySecretRef:
              name: ${secret}
              key: secret-access-key
EOF
      [[ -n "${AWS_HOSTED_ZONE_ID:-}" ]] && printf '            hostedZoneID: %s\n' "$AWS_HOSTED_ZONE_ID"
      ;;
    clouddns)
      cat <<EOF
          cloudDNS:
            project: ${GCP_PROJECT}
            serviceAccountSecretRef:
              name: ${secret}
              key: service-account.json
EOF
      ;;
    azuredns)
      cat <<EOF
          azureDNS:
            clientID: ${AZURE_CLIENT_ID}
            clientSecretSecretRef:
              name: ${secret}
              key: client-secret
            subscriptionID: ${AZURE_SUBSCRIPTION_ID}
            tenantID: ${AZURE_TENANT_ID}
            resourceGroupName: ${AZURE_RESOURCE_GROUP}
            hostedZoneName: ${AZURE_HOSTED_ZONE_NAME:-${DOMAIN_BASE}}
EOF
      ;;
    *) die "_dns01_solver_block: unsupported DNS_PROVIDER '${DNS_PROVIDER}'" ;;
  esac
}

# render_dns01_credentials_secret <secret_name> — print an Opaque Secret in CERT_MANAGER_NS
# holding the provider credential the dns01 solver reads. Only the value that must stay
# secret goes here (tokens/keys); non-secret fields like region/project are set inline in
# the ClusterIssuer. Reads DNS_PROVIDER + provider vars from the environment.
render_dns01_credentials_secret() {
  local name="$1"
  printf 'apiVersion: v1\nkind: Secret\nmetadata:\n  name: %s\n  namespace: %s\ntype: Opaque\nstringData:\n' \
    "$name" "$CERT_MANAGER_NS"
  case "$DNS_PROVIDER" in
    cloudflare) printf '  api-token: %s\n' "$(_yaml_quote "$CLOUDFLARE_API_TOKEN")" ;;
    route53)    printf '  secret-access-key: %s\n' "$(_yaml_quote "$AWS_SECRET_ACCESS_KEY")" ;;
    azuredns)   printf '  client-secret: %s\n' "$(_yaml_quote "$AZURE_CLIENT_SECRET")" ;;
    clouddns)
      # The whole service-account JSON is the secret; embed it as a block scalar.
      # Trailing newline guarantees the block ends cleanly even when the SA JSON file
      # lacks one, so the caller's "---" document separator stays on its own line.
      printf '  service-account.json: |\n'
      sed 's/^/    /' "$GCP_SERVICE_ACCOUNT_FILE"
      printf '\n'
      ;;
    *) die "render_dns01_credentials_secret: unsupported DNS_PROVIDER '${DNS_PROVIDER}'" ;;
  esac
}

# _yaml_quote <value> — single-quote a scalar for YAML (doubling any embedded single
# quotes), so tokens containing YAML-significant characters are passed through verbatim.
_yaml_quote() { printf "'%s'" "${1//\'/\'\'}"; }

# render_acme_clusterissuer <issuer_name> <cred_secret_name> — print the ACME DNS-01
# ClusterIssuer. cert-manager registers the ACME account (email) and stores its key in
# <issuer>-account-key. ACME_SERVER overrides the CA (e.g. LE staging for testing).
# Reads ACME_EMAIL, ACME_SERVER (optional), DNS_PROVIDER + provider vars.
render_acme_clusterissuer() {
  local issuer="$1" secret="$2" server="${ACME_SERVER:-https://acme-v02.api.letsencrypt.org/directory}"
  cat <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: ${issuer}
spec:
  acme:
    email: ${ACME_EMAIL}
    server: ${server}
    privateKeySecretRef:
      name: ${issuer}-account-key
    solvers:
      - dns01:
$(_dns01_solver_block "$secret")
EOF
}

# render_wildcard_certificate <cert_name> <secret_name> <issuer_name> — print the
# Certificate whose issued Secret the consolidated :443 Gateway references. dnsNames come
# from acme_dns_names — cert_dns_names minus any host already covered by a wildcard in
# the same list, which an ACME CA refuses to issue in one order. Lives in GATEWAY_NS.
render_wildcard_certificate() {
  local name="$1" secret="$2" issuer="$3" d
  cat <<EOF
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: ${name}
  namespace: ${GATEWAY_NS}
spec:
  secretName: ${secret}
  duration: 2160h
  renewBefore: 720h
  privateKey:
    algorithm: RSA
    size: 2048
  dnsNames:
EOF
  while IFS= read -r d; do
    [[ -n "$d" ]] && printf '    - %s\n' "$(_yaml_quote "$d")"
  done < <(acme_dns_names)
  cat <<EOF
  issuerRef:
    name: ${issuer}
    kind: ClusterIssuer
    group: cert-manager.io
EOF
}

# render_byoc_tls_secret <secret_name> <cert_file> <key_file> — print the
# kubernetes.io/tls Secret holding an operator-supplied certificate chain and private
# key, in GATEWAY_NS so the consolidated :443 Gateway's certificateRefs resolves
# same-namespace. This is the byoc counterpart to render_wildcard_certificate: it
# produces the very same Secret the DNS-01 path has cert-manager issue, so nothing
# downstream (Gateway, routes, grants) can tell the two modes apart.
#
# Uses `data:` with base64 rather than `stringData:` with a block scalar: PEM is
# multi-line and any indentation slip silently corrupts the key. `openssl base64 -A`
# (not `base64 -w0`, which is GNU-only) keeps this portable across macOS and Linux.
render_byoc_tls_secret() {
  local name="$1" cert="$2" key="$3" crt_b64 key_b64
  # Encode before the heredoc, not inside it. Command substitution in a heredoc swallows
  # the exit status, so a file that moved or lost its permissions after the pre-flight
  # would expand to an empty value here and still return 0 — the caller would then see
  # only kubectl's decoding complaint, which never names the file at fault.
  crt_b64="$(openssl base64 -A -in "$cert")" && [[ -n "$crt_b64" ]] \
    || die "render_byoc_tls_secret: could not read/encode the certificate: ${cert}"
  key_b64="$(openssl base64 -A -in "$key")" && [[ -n "$key_b64" ]] \
    || die "render_byoc_tls_secret: could not read/encode the private key: ${key}"
  cat <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: ${name}
  namespace: ${GATEWAY_NS}
type: kubernetes.io/tls
data:
  tls.crt: ${crt_b64}
  tls.key: ${key_b64}
EOF
}

# render_platform_ca_configmap <ca_file> — print the ConfigMap holding the operator's
# CA certificate, so components created AFTER the install can find it.
#
# Environments are added long after install-advanced.sh exits, and each one provisions
# its own env-Thunder that must validate platform Thunder's HTTPS JWKS URL. On a byoc
# install that URL is served with the operator's certificate, so the in-cluster
# self-signed root add-environment-thunder.sh otherwise falls back to is the wrong CA —
# it mounts a bundle that cannot verify the chain, exits 0, and the failure only shows
# up later as a broken login into that environment. Persisting the CA here is what makes
# `TLS_CA_FILE` apply to every future environment rather than only the default one.
#
# A CA certificate is public by definition (it is sent in the TLS handshake), so a
# ConfigMap is the right kind — no Secret needed.
#
# YAML infers a block scalar's indentation from its first non-empty line, so the sed
# below strips any leading whitespace before applying exactly four spaces. Otherwise a CA
# file opening with an indented line — the human-readable preamble `openssl x509 -text`
# emits, for instance — would set a deeper inferred indent, and every subsequent line
# would either gain leading spaces inside the value or end the block early. The ConfigMap
# still applies either way; the damage only appears later as a trust failure.
#
# An explicit indentation indicator (`|4`) looks like the tidier fix but is not: the
# indicator counts from the parent node's indentation, not from column zero, so here it
# would demand six spaces and fail to parse at all.
render_platform_ca_configmap() {
  local ca="$1"
  [[ -s "$ca" ]] || die "render_platform_ca_configmap: CA file is missing or empty: ${ca}"
  cat <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: amp-platform-ca
  namespace: ${GATEWAY_NS}
data:
  ca.crt: |
$(sed -e 's/[[:space:]]*$//' -e 's/^[[:space:]]*//' -e 's/^/    /' "$ca")
EOF
}

# render_consolidated_gateway <name> <cert_secret> [port] — print the single HTTPS
# Gateway that fronts all three planes on :443. It terminates TLS with the cert-manager
# wildcard Secret, then forwards by host to each plane's own gateway via the front-proxy
# HTTPRoutes (see render_frontproxy_route). Those routes live alongside the Gateway in
# GATEWAY_NS, so allowedRoutes is `Same`: the plane gateways keep their own routes and
# plane separation is preserved — nothing reparents onto this Gateway. Lives in GATEWAY_NS
# (same as the cert Secret, so the listener's certificateRefs resolves same-namespace).
render_consolidated_gateway() {
  local name="$1" secret="$2" port="${3:-443}"
  cat <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: ${name}
  namespace: ${GATEWAY_NS}
spec:
  gatewayClassName: kgateway
  listeners:
    - name: https
      port: ${port}
      protocol: HTTPS
      tls:
        mode: Terminate
        certificateRefs:
          - name: ${secret}
      allowedRoutes:
        namespaces:
          from: Same
EOF
}

# render_frontproxy_route <name> <gateway> <backend_ns> <backend_svc> <backend_port> <host>...
# Print an HTTPRoute in GATEWAY_NS, parented to the consolidated :443 Gateway, that
# forwards the given hostnames to one plane's gateway Service. This is the front-proxy
# model: the :443 Gateway terminates TLS and forwards by host to each plane's own gateway
# (kgateway->kgateway, validated on a live VM), which then routes to the workload with the
# Host header preserved. Plane separation stays intact (each plane keeps its gateway and
# its routes), and wildcard hostnames (*.agents, *.thunder) cover the dynamic tiers —
# deployed agents and per-env Thunder — with no per-object reparenting.
render_frontproxy_route() {
  local name="$1" gateway="$2" bns="$3" bsvc="$4" bport="$5"; shift 5
  cat <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: ${name}
  namespace: ${GATEWAY_NS}
spec:
  parentRefs:
    - group: gateway.networking.k8s.io
      kind: Gateway
      name: ${gateway}
  hostnames:
EOF
  local h
  for h in "$@"; do
    [[ -n "$h" ]] && printf '    - %s\n' "$(_yaml_quote "$h")"
  done
  cat <<EOF
  rules:
    - backendRefs:
        - group: ""
          kind: Service
          name: ${bsvc}
          namespace: ${bns}
          port: ${bport}
EOF
}

# render_backend_referencegrant <target_ns> — print a ReferenceGrant in <target_ns> that
# lets the front-proxy HTTPRoutes (in GATEWAY_NS) backendRef Services there. Required for
# the observability and data plane gateways, which are cross-namespace from GATEWAY_NS; the
# control-plane gateway is same-namespace as GATEWAY_NS and needs no grant.
render_backend_referencegrant() {
  local tns="$1"
  cat <<EOF
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: amp-frontproxy-to-services
  namespace: ${tns}
spec:
  from:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      namespace: ${GATEWAY_NS}
  to:
    - group: ""
      kind: Service
EOF
}
