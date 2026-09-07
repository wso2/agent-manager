#!/usr/bin/env bash
# Pre-flight / config tests for the advanced VM installer: config validation
# (lib-advanced.sh), the cert-manager DNS-01 renderers (lib-certmanager.sh), and the
# advisory DNS check. Run: bash deployments/vm/tests/preflight.sh
# AMP_HOST_*/DOMAIN_BASE/AMP_AGENTS_BASE are consumed by sourced lib functions; the
# source boundary hides that from shellcheck. _resolve_host stubs are invoked indirectly.
# shellcheck disable=SC2034,SC2329
set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../lib-advanced.sh disable=SC1091
source "${SCRIPT_DIR}/../lib-advanced.sh"
# shellcheck source=../lib-vm.sh disable=SC1091
source "${SCRIPT_DIR}/../lib-vm.sh"
# shellcheck source=../lib-certmanager.sh disable=SC1091
source "${SCRIPT_DIR}/../lib-certmanager.sh"

FAILLOG="$(mktemp)"
trap 'rm -f "$FAILLOG"' EXIT
assert_eq() {
  local label="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then printf 'ok   - %s\n' "$label"
  else printf 'FAIL - %s\n      expected: %q\n      actual:   %q\n' "$label" "$expected" "$actual"; echo 1 >>"$FAILLOG"; fi
}

DOMAIN_BASE=amp.mycompany.com
AMP_AGENTS_BASE=agents.amp.mycompany.com
AMP_HOST_CONSOLE=console.amp.mycompany.com
AMP_HOST_API=api.amp.mycompany.com
AMP_HOST_THUNDER=thunder.amp.mycompany.com
AMP_HOST_OBSERVER=observer.amp.mycompany.com
AMP_HOST_GATEWAY=gateway.amp.mycompany.com
AMP_HOST_CP=cp.amp.mycompany.com

# --- cert_dns_names: fixed hosts + the two dynamic wildcards ---
sans="$(cert_dns_names)"
assert_eq "cert SANs include console"          "yes" "$(grep -qxF 'console.amp.mycompany.com' <<<"$sans" && echo yes || echo no)"
assert_eq "cert SANs include cp (external gw)" "yes" "$(grep -qxF 'cp.amp.mycompany.com' <<<"$sans" && echo yes || echo no)"
assert_eq "cert SANs include agents wildcard"  "yes" "$(grep -qxF '*.agents.amp.mycompany.com' <<<"$sans" && echo yes || echo no)"
assert_eq "cert SANs include env-Thunder wild" "yes" "$(grep -qxF '*.amp.mycompany.com' <<<"$sans" && echo yes || echo no)"
# CP omitted when external gateways are off.
AMP_HOST_CP="" sans_nocp="$(AMP_HOST_CP="" cert_dns_names)"
assert_eq "cert SANs omit cp when unset"       "no"  "$(grep -qxF 'cp.amp.mycompany.com' <<<"$sans_nocp" && echo yes || echo no)"

# --- acme_dns_names: the ACME order must carry NO name a wildcard in it already covers ---
# Boulder rejects such an order outright ("redundant with a wildcard domain in the same
# request"), and the Order errors before any Challenge exists — so this is what keeps
# TLS_MODE=dns01 issuable at all, not a cosmetic trim.
# The cp assertion above leaks AMP_HOST_CP="" into the shell (two assignments in one
# command), so restore it — otherwise the cp cases below would pass vacuously.
AMP_HOST_CP=cp.amp.mycompany.com
acme="$(acme_dns_names)"
assert_eq "acme names keep the base wildcard"   "yes" "$(grep -qxF '*.amp.mycompany.com' <<<"$acme" && echo yes || echo no)"
assert_eq "acme names keep agents wildcard"     "yes" "$(grep -qxF '*.agents.amp.mycompany.com' <<<"$acme" && echo yes || echo no)"
assert_eq "acme names keep gateway wildcard"    "yes" "$(grep -qxF '*.gateway.amp.mycompany.com' <<<"$acme" && echo yes || echo no)"
# Each of these is one label under the base domain, so *.amp.mycompany.com covers it.
for h in console api thunder observer gateway cp; do
  assert_eq "acme names drop redundant ${h}"    "no"  "$(grep -qxF "${h}.amp.mycompany.com" <<<"$acme" && echo yes || echo no)"
done
assert_eq "acme names are exactly the 3 wildcards" "3" "$(grep -c . <<<"$acme")"
# A host NOT covered by any wildcard in the list must survive the filter: an operator
# override can put a service outside DOMAIN_BASE, and dropping it would silently issue a
# cert that does not serve that host.
acme_off="$(AMP_HOST_CONSOLE=console.elsewhere.example acme_dns_names)"
assert_eq "acme names keep an off-base host"    "yes" "$(grep -qxF 'console.elsewhere.example' <<<"$acme_off" && echo yes || echo no)"
# The byoc requirement list is deliberately unfiltered — validate_cert accepts either shape.
assert_eq "cert_dns_names still lists all 9"    "9"   "$(grep -c . <<<"$(cert_dns_names)")"

# --- validate_dns01_config: provider + credential presence (appends to CONFIG_ERRORS) ---
run_validate() { CONFIG_ERRORS=(); validate_dns01_config; echo "${#CONFIG_ERRORS[@]}"; }

n="$(DNS_PROVIDER=cloudflare CLOUDFLARE_API_TOKEN=tok run_validate)"
assert_eq "cloudflare with token: 0 errors" "0" "$n"
n="$(DNS_PROVIDER=cloudflare run_validate)"                # token missing
assert_eq "cloudflare without token: error" "1" "$n"
n="$(DNS_PROVIDER=route53 AWS_ACCESS_KEY_ID=a AWS_SECRET_ACCESS_KEY=s AWS_REGION=us-east-1 run_validate)"
assert_eq "route53 with all creds: 0 errors" "0" "$n"
n="$(DNS_PROVIDER=route53 AWS_ACCESS_KEY_ID=a run_validate)"   # 2 missing
assert_eq "route53 missing creds: 2 errors" "2" "$n"
n="$(DNS_PROVIDER=bogus run_validate)"
assert_eq "unknown provider: error" "1" "$n"

# --- render_acme_clusterissuer emits the right provider solver block ---
issuer_cf="$(DNS_PROVIDER=cloudflare ACME_EMAIL=o@x.com render_acme_clusterissuer amp-acme amp-creds)"
assert_eq "cloudflare issuer has cloudflare solver" "yes" "$(grep -q 'cloudflare:' <<<"$issuer_cf" && echo yes || echo no)"
assert_eq "cloudflare issuer references cred secret" "yes" "$(grep -q 'name: amp-creds' <<<"$issuer_cf" && echo yes || echo no)"
issuer_r53="$(DNS_PROVIDER=route53 ACME_EMAIL=o@x.com AWS_REGION=us-east-1 AWS_ACCESS_KEY_ID=AK render_acme_clusterissuer amp-acme amp-creds)"
assert_eq "route53 issuer has route53 solver" "yes" "$(grep -q 'route53:' <<<"$issuer_r53" && echo yes || echo no)"
assert_eq "route53 issuer sets region"        "yes" "$(grep -q 'region: us-east-1' <<<"$issuer_r53" && echo yes || echo no)"

# --- render_wildcard_certificate covers the SANs and references the issuer ---
cert="$(render_wildcard_certificate amp-tls amp-tls amp-acme)"
assert_eq "cert manifest lists agents wildcard" "yes" "$(grep -q '\*.agents.amp.mycompany.com' <<<"$cert" && echo yes || echo no)"
assert_eq "cert manifest references issuer"     "yes" "$(grep -q 'name: amp-acme' <<<"$cert" && echo yes || echo no)"

# --- render_consolidated_gateway: :443 HTTPS Terminate, from Same, cert ref ---
gw="$(render_consolidated_gateway amp-gw amp-tls 443)"
assert_eq "gateway listens :443"        "yes" "$(grep -q 'port: 443' <<<"$gw" && echo yes || echo no)"
assert_eq "gateway terminates TLS"      "yes" "$(grep -q 'mode: Terminate' <<<"$gw" && echo yes || echo no)"
assert_eq "gateway allows same-ns routes" "yes" "$(grep -q 'from: Same' <<<"$gw" && echo yes || echo no)"
assert_eq "gateway references cert sec" "yes" "$(grep -q 'name: amp-tls' <<<"$gw" && echo yes || echo no)"

# --- render_k3d_advanced_config: publishes :443 (public) + loopback-binds plane ports ---
k3d_out="$(printf 'ports:\n  - port: 8080:8080\n    nodeFilters:\n      - loadbalancer\n' | render_k3d_advanced_config)"
assert_eq "k3d advanced publishes 443"        "yes" "$(grep -q -- '- port: 443:443' <<<"$k3d_out" && echo yes || echo no)"
assert_eq "k3d advanced loopback-binds 8080"  "yes" "$(grep -q -- '- port: 127.0.0.1:8080:8080' <<<"$k3d_out" && echo yes || echo no)"

# --- validate_config: TLS_MODE branching ---
# An absent TLS_MODE must keep the DNS-01 behaviour — every config file written before
# byoc existed omits the key entirely.
run_vc() { CONFIG_ERRORS=(); validate_config >/dev/null; echo "${#CONFIG_ERRORS[@]}"; }

n="$(AMP_VERSION=1 DOMAIN_BASE=d ACME_EMAIL=o@x.com DNS_PROVIDER=cloudflare CLOUDFLARE_API_TOKEN=tok run_vc)"
assert_eq "no TLS_MODE defaults to dns01: 0 errors" "0" "$n"
n="$(AMP_VERSION=1 DOMAIN_BASE=d run_vc)"   # dns01 default still demands the ACME keys
assert_eq "no TLS_MODE still requires ACME keys" "2" "$n"
n="$(TLS_MODE=bogus AMP_VERSION=1 DOMAIN_BASE=d run_vc)"
assert_eq "unknown TLS_MODE: 1 error" "1" "$n"

# byoc must NOT require ACME_EMAIL or DNS_PROVIDER — needing an ACME contact for a
# certificate you already hold is exactly the coupling this mode removes.
tmp_cert="$(mktemp)"; tmp_key="$(mktemp)"
openssl req -x509 -newkey rsa:2048 -nodes -days 90 -keyout "$tmp_key" -out "$tmp_cert" \
  -subj "/CN=amp.mycompany.com" \
  -addext "subjectAltName=DNS:console.amp.mycompany.com,DNS:api.amp.mycompany.com,DNS:thunder.amp.mycompany.com,DNS:observer.amp.mycompany.com,DNS:gateway.amp.mycompany.com,DNS:cp.amp.mycompany.com,DNS:*.agents.amp.mycompany.com,DNS:*.amp.mycompany.com,DNS:*.gateway.amp.mycompany.com" \
  >/dev/null 2>&1
# Fail with the real cause if generation did not work (an openssl without -addext, say).
# Every assertion below reads these files, so without this they all fail at once with
# messages that say nothing about why.
[[ -s "$tmp_cert" && -s "$tmp_key" ]] || {
  printf 'FAIL - could not generate the BYOC test certificate (openssl lacking -addext?)\n'
  echo 1 >>"$FAILLOG"; exit 1
}

n="$(TLS_MODE=byoc AMP_VERSION=1 DOMAIN_BASE=d TLS_CERT_FILE="$tmp_cert" TLS_KEY_FILE="$tmp_key" run_vc)"
assert_eq "byoc without ACME_EMAIL/DNS_PROVIDER: 0 errors" "0" "$n"
n="$(TLS_MODE=byoc AMP_VERSION=1 DOMAIN_BASE=d run_vc)"
assert_eq "byoc without cert/key: 2 errors" "2" "$n"
n="$(TLS_MODE=byoc AMP_VERSION=1 DOMAIN_BASE=d TLS_CERT_FILE=/nope/c.pem TLS_KEY_FILE="$tmp_key" run_vc)"
assert_eq "byoc unreadable cert: 1 error" "1" "$n"
n="$(TLS_MODE=byoc AMP_VERSION=1 DOMAIN_BASE=d TLS_CERT_FILE="$tmp_cert" TLS_KEY_FILE="$tmp_key" TLS_CA_FILE=/nope/ca.pem run_vc)"
assert_eq "byoc unreadable TLS_CA_FILE: 1 error" "1" "$n"

# --- cert_sans: DNS entries only (no header line, no non-DNS SAN types) ---
assert_eq "cert_sans drops the header line" "no" \
  "$(cert_sans "$tmp_cert" | grep -q 'Alternative' && echo yes || echo no)"
assert_eq "cert_sans count matches" "9" "$(cert_sans "$tmp_cert" | grep -c .)"

# --- validate_cert: pairing, expiry, and SAN coverage driven by cert_dns_names ---
validate_cert "$tmp_cert" "$tmp_key" 2>/dev/null
assert_eq "full-SAN cert passes" "0" "${#CERT_ERRORS[@]}"

# A *.<DOMAIN_BASE> wildcard covers the service hosts AND per-environment Thunder
# (handles sit directly under the base domain, no fixed subdomain segment), but NOT
# the *.agents or *.gateway tiers, which sit one label deeper — the single most
# common BYOC mistake.
tmp_c2="$(mktemp)"; tmp_k2="$(mktemp)"
openssl req -x509 -newkey rsa:2048 -nodes -days 90 -keyout "$tmp_k2" -out "$tmp_c2" \
  -subj "/CN=amp.mycompany.com" -addext "subjectAltName=DNS:*.amp.mycompany.com" >/dev/null 2>&1
validate_cert "$tmp_c2" "$tmp_k2" 2>/dev/null
assert_eq "*.<base> alone misses 2 wildcards" "2" "${#CERT_ERRORS[@]}"
assert_eq "missing *.gateway is named" "yes" \
  "$(printf '%s\n' "${CERT_ERRORS[@]}" | grep -qF 'missing the wildcard SAN: *.gateway.amp.mycompany.com' && echo yes || echo no)"

# Mismatched key: cert from one pair, key from the other.
validate_cert "$tmp_cert" "$tmp_k2" 2>/dev/null
assert_eq "mismatched key is rejected" "yes" \
  "$(printf '%s\n' "${CERT_ERRORS[@]}" | grep -qF 'cert and key do not match' && echo yes || echo no)"

# Unparseable inputs must be named as such. Digesting openssl's (empty) output on failure
# would make two unreadable files compare equal, so a garbage cert+key pair would pass the
# pairing check and then be reported as an expired certificate.
tmp_junk="$(mktemp)"; printf 'not a certificate\n' > "$tmp_junk"
validate_cert "$tmp_junk" "$tmp_junk" 2>/dev/null
assert_eq "garbage cert+key is not reported as matching" "no" \
  "$(printf '%s\n' "${CERT_ERRORS[@]}" | grep -qF 'cert and key do not match' && echo yes || echo no)"
assert_eq "garbage cert is named unreadable" "yes" \
  "$(printf '%s\n' "${CERT_ERRORS[@]}" | grep -qF 'not a readable X.509 certificate' && echo yes || echo no)"
assert_eq "garbage cert is not called expired" "no" \
  "$(printf '%s\n' "${CERT_ERRORS[@]}" | grep -qF 'cert is expired' && echo yes || echo no)"
# An encrypted key is the realistic case: it must not surface as a pairing mismatch.
tmp_enc="$(mktemp)"
openssl genrsa -aes256 -passout pass:secret 2048 > "$tmp_enc" 2>/dev/null
if [[ -s "$tmp_enc" ]] && grep -q "ENCRYPTED" "$tmp_enc"; then
  validate_cert "$tmp_cert" "$tmp_enc" 2>/dev/null
  assert_eq "encrypted key is named, not a mismatch" "yes" \
    "$(printf '%s\n' "${CERT_ERRORS[@]}" | grep -qF 'not a readable unencrypted private key' && echo yes || echo no)"
else
  printf 'ok   - encrypted key is named, not a mismatch (skipped: could not build one)\n'
fi

# SAN matching is case-insensitive (RFC 4343): DNS names differing only in case must match.
tmp_c4="$(mktemp)"; tmp_k4="$(mktemp)"
openssl req -x509 -newkey rsa:2048 -nodes -days 90 -keyout "$tmp_k4" -out "$tmp_c4" \
  -subj "/CN=amp.mycompany.com" \
  -addext "subjectAltName=DNS:CONSOLE.amp.mycompany.com,DNS:API.amp.mycompany.com,DNS:Thunder.amp.mycompany.com,DNS:observer.amp.mycompany.com,DNS:gateway.amp.mycompany.com,DNS:cp.amp.mycompany.com,DNS:*.Agents.amp.mycompany.com,DNS:*.AMP.mycompany.com,DNS:*.gateway.amp.mycompany.com" \
  >/dev/null 2>&1
validate_cert "$tmp_c4" "$tmp_k4" 2>/dev/null
assert_eq "mixed-case SANs are accepted" "0" "${#CERT_ERRORS[@]}"
rm -f "$tmp_junk" "$tmp_enc" "$tmp_c4" "$tmp_k4"

# Expired cert. -days 1 with a backdated start puts notAfter in the past.
tmp_c3="$(mktemp)"; tmp_k3="$(mktemp)"
openssl req -x509 -newkey rsa:2048 -nodes -not_before 20200101000000Z -not_after 20200102000000Z \
  -keyout "$tmp_k3" -out "$tmp_c3" -subj "/CN=amp.mycompany.com" \
  -addext "subjectAltName=DNS:*.amp.mycompany.com" >/dev/null 2>&1
if [[ -s "$tmp_c3" ]]; then
  validate_cert "$tmp_c3" "$tmp_k3" 2>/dev/null
  assert_eq "expired cert is rejected" "yes" \
    "$(printf '%s\n' "${CERT_ERRORS[@]}" | grep -qF 'cert is expired' && echo yes || echo no)"
else
  # -not_before/-not_after need OpenSSL 3.2+; skip rather than fail on older toolchains.
  printf 'ok   - expired cert is rejected (skipped: openssl lacks -not_after)\n'
fi

# --- render_byoc_tls_secret: kubernetes.io/tls in the gateway namespace ---
byoc_secret="$(render_byoc_tls_secret amp-wildcard-tls "$tmp_cert" "$tmp_key")"
assert_eq "byoc secret type"      "yes" "$(grep -q 'type: kubernetes.io/tls' <<<"$byoc_secret" && echo yes || echo no)"
assert_eq "byoc secret namespace" "yes" "$(grep -q 'namespace: openchoreo-control-plane' <<<"$byoc_secret" && echo yes || echo no)"
assert_eq "byoc secret has tls.crt" "yes" "$(grep -q 'tls.crt:' <<<"$byoc_secret" && echo yes || echo no)"
assert_eq "byoc secret has tls.key" "yes" "$(grep -q 'tls.key:' <<<"$byoc_secret" && echo yes || echo no)"
# The base64 must round-trip: a truncated or line-wrapped encoding yields a Secret that
# applies cleanly but serves a corrupt key, which only shows up as a TLS handshake error.
decoded="$(grep 'tls.crt:' <<<"$byoc_secret" | awk '{print $2}' | openssl base64 -d -A)"
assert_eq "byoc secret tls.crt round-trips" "yes" \
  "$(grep -q 'BEGIN CERTIFICATE' <<<"$decoded" && echo yes || echo no)"

# --- render_platform_ca_configmap: persists the operator CA for LATER environments ---
# Without this, an environment created after install falls back to the in-cluster
# self-signed root — the wrong CA on a byoc install — and fails to validate the JWKS
# URL while still reporting success.
ca_cm="$(render_platform_ca_configmap "$tmp_cert")"
assert_eq "platform CA cm is a ConfigMap"  "yes" "$(grep -q '^kind: ConfigMap' <<<"$ca_cm" && echo yes || echo no)"
assert_eq "platform CA cm name"            "yes" "$(grep -q 'name: amp-platform-ca' <<<"$ca_cm" && echo yes || echo no)"
assert_eq "platform CA cm namespace"       "yes" "$(grep -q 'namespace: openchoreo-control-plane' <<<"$ca_cm" && echo yes || echo no)"
assert_eq "platform CA cm has ca.crt key"  "yes" "$(grep -q 'ca.crt: |' <<<"$ca_cm" && echo yes || echo no)"
# The PEM must survive the block-scalar indent: a mis-indented body yields a ConfigMap
# that applies cleanly but hands env-Thunder an unparseable trust bundle.
ca_body="$(sed -n '/ca.crt: |/,$p' <<<"$ca_cm" | tail -n +2 | sed 's/^    //')"
assert_eq "platform CA cm PEM round-trips" "yes" \
  "$(printf '%s\n' "$ca_body" | openssl x509 -noout -subject >/dev/null 2>&1 && echo yes || echo no)"

# Parse the rendered YAML for real when kubectl is available. The manual de-indent above
# cannot tell a valid block scalar from an invalid header (an explicit indentation
# indicator, say), so on its own it would pass on YAML that no parser accepts.
if command -v kubectl >/dev/null 2>&1; then
  ca_parsed="$(printf '%s\n' "$ca_cm" | kubectl create --validate=false --dry-run=client -o jsonpath='{.data.ca\.crt}' -f - 2>/dev/null || true)"
  assert_eq "platform CA cm is valid YAML" "yes" \
    "$(printf '%s' "$ca_parsed" | grep -q 'BEGIN CERTIFICATE' && echo yes || echo no)"
  assert_eq "platform CA cm PEM is not indented" "no" \
    "$(printf '%s' "$ca_parsed" | grep -qE '^[[:space:]]+-----BEGIN' && echo yes || echo no)"
  # A CA file whose first line is indented (openssl x509 -text preamble) must not shift
  # the inferred block indentation.
  tmp_messy="$(mktemp)"
  { printf '\n  Certificate:\n    Data:\n'; cat "$tmp_cert"; } > "$tmp_messy"
  messy_parsed="$(render_platform_ca_configmap "$tmp_messy" | kubectl create --validate=false --dry-run=client -o jsonpath='{.data.ca\.crt}' -f - 2>/dev/null || true)"
  assert_eq "indented-preamble CA still renders valid YAML" "yes" \
    "$(printf '%s' "$messy_parsed" | openssl x509 -noout -subject >/dev/null 2>&1 && echo yes || echo no)"
  rm -f "$tmp_messy"
  # The byoc Secret must parse too.
  sec_type="$(render_byoc_tls_secret amp-wildcard-tls "$tmp_cert" "$tmp_key" | kubectl create --validate=false --dry-run=client -o jsonpath='{.type}' -f - 2>/dev/null || true)"
  assert_eq "byoc secret is valid YAML" "kubernetes.io/tls" "$sec_type"
else
  printf 'ok   - rendered YAML parses (skipped: kubectl not installed)\n'
fi

rm -f "$tmp_cert" "$tmp_key" "$tmp_c2" "$tmp_k2" "$tmp_c3" "$tmp_k3"

# --- validate_dns is advisory: records errors but never fails (no more hard-fail mode) ---
_resolve_host() { echo "198.51.100.5"; }        # resolves to a non-candidate IP
validate_dns 203.0.113.10; rc=$?
assert_eq "validate_dns advisory rc=0"       "0"  "$rc"
assert_eq "validate_dns records the mismatch" "yes" "$([[ ${#DNS_ERRORS[@]} -gt 0 ]] && echo yes || echo no)"
unset -f _resolve_host

if [[ -s "$FAILLOG" ]]; then echo "PREFLIGHT TESTS FAILED"; exit 1; fi
echo "ALL PREFLIGHT TESTS PASSED"
