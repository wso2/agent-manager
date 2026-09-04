#!/usr/bin/env bash
# lib-advanced.sh — config loading, host derivation, and pre-flight validation for
# install-advanced.sh. Sourcing only defines functions (no side effects).

# derive_hosts — from DOMAIN_BASE (+ optional HOST_* overrides, AGENTS_BASE,
# EXTERNAL_GATEWAYS), set the AMP_HOST_*/AMP_AGENTS_BASE variables the lib-vm.sh
# cores read. Caller should declare these in its scope (or accept globals).
# shellcheck disable=SC2034  # AMP_HOST_*/AMP_AGENTS_BASE are consumed by the lib-vm.sh cores.
derive_hosts() {
  : "${DOMAIN_BASE:?derive_hosts requires DOMAIN_BASE}"
  AMP_HOST_CONSOLE="${HOST_CONSOLE:-console.${DOMAIN_BASE}}"
  AMP_HOST_API="${HOST_API:-api.${DOMAIN_BASE}}"
  AMP_HOST_THUNDER="${HOST_THUNDER:-thunder.${DOMAIN_BASE}}"
  AMP_HOST_OBSERVER="${HOST_OBSERVER:-observer.${DOMAIN_BASE}}"
  AMP_HOST_GATEWAY="${HOST_GATEWAY:-gateway.${DOMAIN_BASE}}"
  AMP_AGENTS_BASE="${AGENTS_BASE:-agents.${DOMAIN_BASE}}"
  if [[ "${EXTERNAL_GATEWAYS:-true}" == "true" ]]; then
    AMP_HOST_CP="${HOST_CP:-cp.${DOMAIN_BASE}}"
  else
    AMP_HOST_CP=""
  fi
}

# The TLS modes the advanced installer supports. Both end at the same seam — a
# kubernetes.io/tls Secret in the gateway's namespace that the consolidated :443
# Gateway references by name — they only differ in who produces it:
#   dns01 — cert-manager issues (and auto-renews) it via the ACME DNS-01 challenge.
#   byoc  — the operator supplies the cert/key; the installer creates the Secret
#           verbatim. No ACME, no DNS-provider credential, no auto-renewal.
SUPPORTED_TLS_MODES="dns01 byoc"

# tls_mode — print the configured mode, defaulting to dns01. Every existing config
# file predates TLS_MODE, so an unset value must keep the DNS-01 behaviour.
tls_mode() { printf '%s' "${TLS_MODE:-dns01}"; }

# load_config <file> — source an env-style config file in the caller's scope.
load_config() {
  local file="${1:?load_config requires a config file path}"
  [[ -f "$file" ]] || { printf 'config file not found: %s\n' "$file" >&2; return 1; }
  # Export every assignment so config values survive into the base installer, which
  # runs in a subshell (`source install.sh`), and into the helper scripts it invokes.
  # A plain `source` leaves them unexported and they would silently not propagate.
  # shellcheck disable=SC1090
  set -a; source "$file"; set +a
}

# validate_config — check the required keys for the configured TLS mode. Populates the
# CONFIG_ERRORS array (reset on each call) and returns 1 if any error was recorded.
# Note this validates config *keys* only. The BYOC certificate itself is checked by
# validate_cert, which needs the derived hostnames and so runs after derive_hosts.
validate_config() {
  CONFIG_ERRORS=()
  [[ -n "${AMP_VERSION:-}" ]] || CONFIG_ERRORS+=("AMP_VERSION is required (an amp/v* release tag, e.g. 1.0.0)")
  [[ -n "${DOMAIN_BASE:-}" ]] || CONFIG_ERRORS+=("DOMAIN_BASE is required (e.g. amp.mycompany.com)")
  local mode; mode="$(tls_mode)"
  case " ${SUPPORTED_TLS_MODES} " in
    *" ${mode} "*) ;;
    *) CONFIG_ERRORS+=("TLS_MODE must be one of: ${SUPPORTED_TLS_MODES} (got '${TLS_MODE:-<unset>}')")
       return 1 ;;
  esac
  case "$mode" in
    dns01)
      [[ -n "${ACME_EMAIL:-}" ]] || CONFIG_ERRORS+=("ACME_EMAIL is required for TLS_MODE=dns01 (cert-manager registers an ACME account with it)")
      validate_dns01_config   # DNS_PROVIDER + provider-specific credentials (lib-certmanager.sh)
      ;;
    byoc)
      validate_byoc_config
      ;;
  esac
  (( ${#CONFIG_ERRORS[@]} == 0 ))
}

# validate_byoc_config — confirm the operator-supplied cert/key (and the optional CA
# bundle) are configured and readable. Appends to CONFIG_ERRORS (does not reset it —
# the caller owns that array). ACME_EMAIL/DNS_PROVIDER are deliberately NOT required:
# byoc never contacts an ACME CA or a DNS provider.
validate_byoc_config() {
  local v
  for v in TLS_CERT_FILE TLS_KEY_FILE; do
    if [[ -z "${!v:-}" ]]; then
      CONFIG_ERRORS+=("${v} is required for TLS_MODE=byoc")
    elif [[ ! -r "${!v}" ]]; then
      CONFIG_ERRORS+=("${v} not readable: ${!v}")
    fi
  done
  # TLS_CA_FILE is optional (only needed when the cert chains to a private CA), but if
  # given it must be usable — a typo'd path would otherwise silently skip CA trust.
  if [[ -n "${TLS_CA_FILE:-}" && ! -r "${TLS_CA_FILE}" ]]; then
    CONFIG_ERRORS+=("TLS_CA_FILE not readable: ${TLS_CA_FILE}")
  fi
}

# cert_sans <cert_file> — print the DNS SANs in a certificate, one per line, lowercased.
# Parses only the `DNS:` entries, so the "X509v3 Subject Alternative Name:" header line
# and any non-DNS SAN types (IP:, email:) are dropped rather than treated as hostnames.
#
# DNS names are case-insensitive (RFC 4343), and both this list and the required names it
# is matched against are compared literally, so both sides are lowercased: a cert carrying
# "DNS:Console.example.com", or a config written as DOMAIN_BASE=Example.com, would
# otherwise be reported as a missing SAN. (The `DNS:` prefix itself is openssl's own
# output, not user data, so matching it literally is safe — and keeps this portable to
# BSD sed, which has no case-insensitive substitution flag.)
cert_sans() {
  openssl x509 -noout -ext subjectAltName -in "$1" 2>/dev/null \
    | tr ',' '\n' | sed -n 's/^[[:space:]]*DNS:[[:space:]]*//p' | sed 's/[[:space:]]//g' \
    | tr '[:upper:]' '[:lower:]'
}

# validate_cert <cert_file> <key_file> — verify a BYOC cert is usable and covers every
# hostname this install serves. Populates CERT_ERRORS; returns 1 on any error. Prints a
# soft note (does not fail) when expiry is under 30 days.
#
# The required SAN list comes from cert_dns_names() (lib-certmanager.sh) — the same
# function that builds the DNS-01 Certificate's dnsNames — so the two can never drift.
# That matters: the per-environment API Platform Gateway tier (*.<gateway host>) was
# added after the original BYOC implementation, and a hardcoded list silently misses it.
validate_cert() {
  local cert="$1" key="$2"
  CERT_ERRORS=()
  [[ -r "$cert" ]] || CERT_ERRORS+=("cert file not readable: $cert")
  [[ -r "$key" ]]  || CERT_ERRORS+=("key file not readable: $key")
  if (( ${#CERT_ERRORS[@]} )); then return 1; fi

  # 1. Both files actually parse, THEN that they are a pair. Compare the public keys
  #    themselves rather than digests of them: piping a failed openssl into `openssl md5`
  #    yields the digest of empty input, which is non-empty and identical on both sides —
  #    so an unparseable cert AND key would silently "match". Comparing public keys (not
  #    moduli) also keeps this working for EC keys, which `openssl rsa -modulus` rejects.
  local cpub kpub
  cpub="$(openssl x509 -noout -pubkey -in "$cert" 2>/dev/null)"
  kpub="$(openssl pkey -pubout -in "$key" 2>/dev/null)"
  [[ -n "$cpub" ]] || CERT_ERRORS+=("cert is not a readable X.509 certificate: $cert")
  # An encrypted private key lands here too: openssl cannot read it unattended, which is
  # worth naming, because "cert and key do not match" would send the operator hunting for
  # the wrong problem entirely.
  [[ -n "$kpub" ]] || CERT_ERRORS+=("key is not a readable unencrypted private key (decrypt it first): $key")
  if [[ -n "$cpub" && -n "$kpub" && "$cpub" != "$kpub" ]]; then
    CERT_ERRORS+=("cert and key do not match (public key mismatch)")
  fi

  # Everything below reads the certificate, so it can only mislead if the cert did not
  # parse — "cert is expired" for a file that is not a certificate, say.
  if [[ -z "$cpub" ]]; then return 1; fi

  # 2. Expiry: hard-fail if already expired; soft-note if < 30 days remain. There is no
  #    auto-renewal in byoc mode, so a short-dated cert is worth flagging up front.
  if ! openssl x509 -checkend 0 -noout -in "$cert" >/dev/null 2>&1; then
    CERT_ERRORS+=("cert is expired")
  elif ! openssl x509 -checkend $((30*24*3600)) -noout -in "$cert" >/dev/null 2>&1; then
    printf '[preflight] NOTE: cert expires in under 30 days, and byoc certs are NOT auto-renewed\n' >&2
  fi

  # 3. SAN coverage for every name in cert_dns_names(). Both sides are lowercased, since
  #    DNS names are case-insensitive but these comparisons are literal.
  local sans want
  sans="$(cert_sans "$cert")"
  _san_covers() {  # _san_covers <hostname>
    local host="$1" s
    while IFS= read -r s; do
      [[ -z "$s" ]] && continue
      [[ "$s" == "$host" ]] && return 0
      # wildcard match: *.foo.com covers bar.foo.com (exactly one extra label)
      if [[ "$s" == \*.* ]]; then
        local base="${s#\*.}"
        [[ "$host" == *."$base" && "${host%."$base"}" != *.* ]] && return 0
      fi
    done <<<"$sans"
    return 1
  }
  while IFS= read -r want; do
    [[ -z "$want" ]] && continue
    want="$(printf '%s' "$want" | tr '[:upper:]' '[:lower:]')"
    if [[ "$want" == \*.* ]]; then
      # A wildcard requirement needs a literal wildcard SAN. A broader wildcard one
      # level up does NOT satisfy it: *.<base> matches only a single extra label, so
      # it never covers <org>-<project>.agents.<base>. This is the certificate-side
      # analogue of the RFC 4592 trap the DNS records section documents.
      grep -qxF "$want" <<<"$sans" \
        || CERT_ERRORS+=("cert is missing the wildcard SAN: ${want}")
    else
      _san_covers "$want" || CERT_ERRORS+=("cert SANs do not cover ${want}")
    fi
  done < <(cert_dns_names)
  unset -f _san_covers

  (( ${#CERT_ERRORS[@]} == 0 ))
}

# _resolve_host <hostname> — print ALL A records (one per line). Overridable in tests.
# Uses dig if present, else getent. Prints nothing if unresolved.
_resolve_host() {
  if command -v dig >/dev/null 2>&1; then
    dig +short A "$1" | grep -E '^[0-9.]+$'
  else
    getent ahostsv4 "$1" 2>/dev/null | awk '{print $1}' | sort -u
  fi
}

# _local_ips — print this host's IPv4 addresses (one per line), excluding loopback.
_local_ips() {
  if command -v hostname >/dev/null 2>&1 && hostname -I >/dev/null 2>&1; then
    hostname -I | tr ' ' '\n' | grep -E '^[0-9.]+$' | grep -vE '^127\.'
  else
    ip -4 -o addr show scope global 2>/dev/null | awk '{print $4}' | cut -d/ -f1
  fi
}

# _public_ip — best-effort public egress IP (cloud VMs are NAT'd, so a host's own
# interfaces show only the private IP while DNS must point at the public one). Empty
# if it can't be determined. Overridable in tests.
_public_ip() {
  local ip
  for url in https://api.ipify.org https://ifconfig.me https://icanhazip.com; do
    if command -v curl >/dev/null 2>&1; then
      ip="$(curl -fsS --max-time 4 "$url" 2>/dev/null)"
    elif command -v wget >/dev/null 2>&1; then
      ip="$(wget -qO- --timeout=4 "$url" 2>/dev/null)"
    fi
    ip="$(echo "$ip" | tr -d '[:space:]')"
    [[ "$ip" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] && { echo "$ip"; return; }
  done
  # Must succeed even when every endpoint is unreachable (egress-restricted VMs):
  # the caller assigns the output under `set -e`, so a non-zero status here would
  # abort the whole install instead of just skipping the public-IP DNS candidate.
  return 0
}

# validate_dns <ip> [more_ips...] — confirm derived hosts resolve to one of the given
# candidate IPs (this VM's local addresses + its public egress IP). Hard-fail in
# letsencrypt mode (ACME needs correct DNS); advisory otherwise. Populates DNS_ERRORS.
# shellcheck disable=SC2154  # AMP_HOST_*/AMP_AGENTS_BASE come from the caller's scope.
validate_dns() {
  local -a candidates=("$@")
  DNS_ERRORS=()
  DNS_NOTES=()
  local host got ip ok e
  # The two probe.* names stand in for the *.agents and *.<gateway> wildcards: they
  # resolve only if the wildcard record exists. Checking $AMP_HOST_GATEWAY on its own
  # is not enough — adding the deeper *.<gateway> wildcard turns that name into an
  # empty non-terminal, so a broader *.<base> wildcard silently stops covering it
  # (RFC 4592), exactly as it does for thunder.<base>.
  for host in "$AMP_HOST_CONSOLE" "$AMP_HOST_API" "$AMP_HOST_THUNDER" \
              "$AMP_HOST_OBSERVER" "$AMP_HOST_GATEWAY" "${AMP_HOST_CP:-}" \
              "probe.${AMP_AGENTS_BASE}" "probe.${AMP_HOST_GATEWAY}"; do
    [[ -z "$host" ]] && continue
    got="$(_resolve_host "$host")"
    if [[ -z "$got" ]]; then
      DNS_ERRORS+=("$host does not resolve to any A record")
      continue
    fi
    # Every A record must point at this VM (a resolver may return several / rotate
    # them), so a host that partly points elsewhere is caught regardless of order.
    while IFS= read -r ip; do
      [[ -z "$ip" ]] && continue
      # A loopback answer is this installer's own doing, not a DNS fault. ensure_loopback_alias
      # points the API and Thunder hosts at 127.0.0.1 in /etc/hosts so in-install API calls
      # reach the gateway, and nothing removes those entries afterwards. On a re-install the
      # local stub resolver answers from /etc/hosts, so those names come back as 127.0.0.1
      # here and would otherwise be reported as pointing "not at this VM" — sending the
      # operator to debug DNS that is in fact correct. It only affects resolution ON this
      # VM; clients elsewhere follow the public records.
      if [[ "$ip" == 127.* ]]; then
        DNS_NOTES+=("$host resolves to ${ip} through a local /etc/hosts alias this installer added; clients off this VM are unaffected")
        continue
      fi
      ok=no
      for e in "${candidates[@]}"; do [[ -n "$e" && "$ip" == "$e" ]] && { ok=yes; break; }; done
      [[ "$ok" == yes ]] || DNS_ERRORS+=("$host resolves to '${ip}', not this VM (${candidates[*]})")
    done <<<"$got"
  done
  if (( ${#DNS_NOTES[@]} )); then
    printf '[preflight] note: %s\n' "${DNS_NOTES[@]}" >&2
  fi
  if (( ${#DNS_ERRORS[@]} )); then
    printf '[preflight] DNS issue: %s\n' "${DNS_ERRORS[@]}" >&2
    printf '[preflight] (advisory: certificate issuance uses DNS-01 and needs no inbound; point your DNS — or client /etc/hosts entries — at this VM so clients can reach the services)\n' >&2
  fi
  return 0
}
