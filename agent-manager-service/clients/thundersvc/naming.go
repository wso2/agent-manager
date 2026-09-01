// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package thundersvc

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/utils/ssrf"
)

const (
	thunderInternalPort = 8090
	maxReleaseNameLen   = 53
	truncatePrefixLen   = 46

	// maxJWKSProbeBodyBytes caps how much of a probe response ThunderProbe reads
	// before validating it as JWKS — a real JWKS document is a few KB at most; this
	// just guards against an unrelated server on a fallback address streaming an
	// unbounded body.
	maxJWKSProbeBodyBytes = 64 * 1024

	// probeTimeout bounds a single candidate probe (see probeThunderURL).
	probeTimeout = 2 * time.Second
)

// ssrfProbeClient is the shared SSRF-hardened client for every external probe
// (see probeThunderURL). ssrf.NewClient clones http.DefaultTransport, giving
// it its own connection pool — building one per probe call would leak a pool
// per call, since ThunderProbe runs once per environment on every
// ListThunderInstances (every console Identity page load). http.Client is
// safe for concurrent use, so one shared instance is enough; the per-request
// timeout already comes from probeThunderURL's own reqCtx.
var ssrfProbeClient = ssrf.NewClient(probeTimeout)

// ThunderReleaseName returns the Helm release name (and namespace) for an env-Thunder instance.
// Mirrors thunder_release_name() in deployments/scripts/thunder-naming.sh — must stay in sync.
// That file is the single source of truth for the bash side of this derivation (sourced by
// every script that provisions, removes, or wires the gateway to env-Thunder); this function
// is the one Go-side copy, since Go can't source bash.
//
// Format: amp-thunder-<org>-<env>, capped at 53 characters.
// If the natural name exceeds 53 chars, it is truncated to 46 characters (trailing "-" stripped)
// and a 6-char hex hash of "org/env" is appended for collision safety.
//
// Deliberately lowercases only — does NOT collapse consecutive hyphens like slugify() does.
// The bash scripts that actually provision Thunder use org/env raw, with no hyphen-collapsing.
// Slugifying here would let this function compute a different address than what was actually
// deployed for any org/env containing "--".
func ThunderReleaseName(org, env string) string {
	org = strings.ToLower(org)
	env = strings.ToLower(env)
	if org == "" || env == "" {
		panic("org and env names must be valid alphanumeric slugs and not empty")
	}
	full := fmt.Sprintf("amp-thunder-%s-%s", org, env)
	if len(full) <= maxReleaseNameLen {
		return strings.TrimSuffix(full, "-")
	}
	hash := thunderSHA6(org + "/" + env)
	prefix := strings.TrimSuffix(full[:truncatePrefixLen], "-")
	return fmt.Sprintf("%s-%s", prefix, hash)
}

// ThunderNamespace returns the Kubernetes namespace for an env-Thunder instance.
// The namespace always mirrors the release name.
func ThunderNamespace(org, env string) string {
	return ThunderReleaseName(org, env)
}

// ThunderInternalURL returns the cluster-internal HTTP URL for an env-Thunder's admin API.
// AMS uses this URL to authenticate and create per-agent OAuth clients.
//
// The Thunder Helm chart creates a K8s Service named "{{ .Release.Name }}-service", so the
// cluster-internal DNS is: http://<release>-service.<namespace>.svc.cluster.local:8090
func ThunderInternalURL(org, env string) string {
	release := ThunderReleaseName(org, env)
	return fmt.Sprintf("http://%s-service.%s.svc.cluster.local:%d", release, release, thunderInternalPort)
}

// ThunderJWKSURL returns the cluster-internal URL for fetching the env-Thunder's JWKS.
// Used by the API gateway's ThunderKeyManager to validate agent tokens.
// INTERNAL ONLY — not reachable outside the cluster; use ThunderExternalJWKSURL for developer-facing output.
func ThunderJWKSURL(org, env string) string {
	return ThunderInternalURL(org, env) + "/oauth2/jwks"
}

// ThunderSystemClientSecretName returns the Kubernetes Secret name that holds the
// system-client credentials for the given env-Thunder instance.
// The secret is created by add-environment-thunder.sh and lives in ThunderNamespace.
func ThunderSystemClientSecretName(org, env string) string {
	return ThunderReleaseName(org, env) + "-system-client"
}

// ThunderTokenURL returns the OAuth2 token endpoint for the env-Thunder instance.
// INTERNAL ONLY — uses cluster-internal K8s DNS (svc.cluster.local:8090); not reachable
// outside the cluster. Use ThunderExternalTokenURL for developer-facing output.
func ThunderTokenURL(org, env string) string {
	return ThunderInternalURL(org, env) + "/oauth2/token"
}

// ThunderExternalTokenURL returns the public OAuth2 token endpoint for the
// env-Thunder instance, given its stored EnvThunderURL.ThunderURL origin.
func ThunderExternalTokenURL(thunderURL string) string {
	return thunderURL + "/oauth2/token"
}

// ThunderExternalJWKSURL returns the public JWKS endpoint for the env-Thunder
// instance, given its stored origin.
func ThunderExternalJWKSURL(thunderURL string) string {
	return thunderURL + "/oauth2/jwks"
}

// ThunderOriginFromHandle returns "<scheme>://<handle>.<baseDomain>[:8080]" for
// an on-prem-registered instance. WRITE-TIME ONLY: called from
// EnvironmentService.SetThunderURL to compute the value stored as
// EnvThunderURL.ThunderURL, never by a reader — readers always use the stored
// value directly, since one global base-domain config can't describe every
// environment once a caller can register a URL under a different domain.
//
// Scheme/port must match what env-Thunder itself self-configured as its
// issuer (see thunder-naming.sh's thunder_issuer(), which must stay in sync):
// local dev (TLS disabled) uses plain HTTP on port 8080; TLS_ENABLED=true
// (VM/production, fronted by Caddy) drops the port and uses HTTPS.
func ThunderOriginFromHandle(handle string) string {
	if handle == "" {
		panic("thunder url handle must not be empty — callers must check for \"not provisioned\" before building a URL")
	}
	host := fmt.Sprintf("%s.%s", handle, config.GetConfig().ThunderHostBaseDomain)
	if config.GetConfig().TLSConfig.EnableTLS {
		return fmt.Sprintf("https://%s", host)
	}
	return fmt.Sprintf("http://%s:8080", host)
}

// ThunderIssuerURL returns the public issuer URL for the env-Thunder
// instance — what Thunder stamps into the JWT iss claim. Currently an
// identity function over the stored origin, kept as a distinct name so
// callers stay self-documenting.
func ThunderIssuerURL(thunderURL string) string {
	return thunderURL
}

// isValidJWKS reports whether body is a syntactically valid JWKS document (a JSON
// object with a non-empty "keys" array). An HTTP 200 alone is not sufficient evidence
// of a live Thunder instance — any unrelated server answering on a probed
// address/port (e.g. a stray dev server bound to the same fallback address) would
// otherwise be misreported as a reachable env-Thunder.
func isValidJWKS(body []byte) bool {
	var doc struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return false
	}
	return len(doc.Keys) > 0
}

// thunderURLCandidate pairs a candidate base URL for an env-Thunder instance with
// the actual host:port a caller should connect to in order to reach it. resolveToHost
// is empty when the base URL's own host is directly dialable; when set, callers must
// dial resolveToHost while keeping the base URL's host as the HTTP Host header, so
// Kgateway's host-based routing still selects the right backend.
// external marks the ONE candidate dialed by its own host with no internal
// override — the exact stored EnvThunderURL.ThunderURL — AND whose value is
// caller-supplied (a row registered with no handle). That combination is
// dialed with ssrf.NewClient (re-resolves at dial time, re-validates every
// redirect hop) rather than the plain default client. A handle-derived row
// is never marked external even at this same candidate position: that value
// is server-computed under AMS's own trusted base domain and can legitimately
// be a private address (e.g. a VM install's sslip.io hostname over a LAN IP,
// or an internal corporate DNS name) — SSRF-hardening it would wrongly reject
// a working deployment. Cluster-internal DNS and the local-dev host-override
// candidates are never marked external either, for the same private-target-by
// -design reason.
type thunderURLCandidate struct {
	baseURL       string
	resolveToHost string
	external      bool
}

// thunderBaseURLCandidates returns, in preference order, every base URL an env-Thunder
// instance might be reachable at: cluster-internal DNS (the real address in any
// Kubernetes deployment) and the public ingress hostname are always tried. Two more,
// host-header-resolved fallbacks targeting fixed local addresses (Docker Desktop's
// host.docker.internal, then plain Linux host networking) are appended ONLY when
// config.IsLocalDevEnv is set: they exist purely to compensate for
// agent-manager-service running as a plain Docker container (docker-compose) rather
// than a Kubernetes pod in local dev, where neither of the two real candidates can be
// resolved. They must never be tried outside that context — in-cluster, the
// cluster-internal candidate always succeeds when Thunder is actually reachable, and
// probing 127.0.0.1 there would hit agent-manager-service's own pod rather than
// Thunder, which is both pointless and a latent false-positive risk if anything ever
// answers on that port. This is the single source of truth for the candidate cascade —
// both ThunderProbe and ResolveThunderBaseURL (used by EnvThunderResolver to reach
// env-Thunder's admin API for per-agent client provisioning) build their attempts from
// this same list, so the two can never drift out of sync with each other.
//
// thunderURL is the stored EnvThunderURL.ThunderURL origin and builds the
// external candidate directly; it must be non-empty (callers check for "not
// provisioned" before ever reaching here). callerSupplied reports whether
// thunderURL was supplied directly by the caller (no handle) rather than
// computed by AMS from a handle — see thunderURLCandidate's doc comment for
// why this, not candidate position alone, decides which candidate gets
// marked external.
func thunderBaseURLCandidates(org, env, thunderURL string, callerSupplied bool) []thunderURLCandidate {
	if thunderURL == "" {
		panic("thunder url must not be empty — callers must check for \"not provisioned\" before building candidates")
	}
	externalBaseURL := thunderURL
	candidates := []thunderURLCandidate{
		{baseURL: ThunderInternalURL(org, env)},
		{baseURL: externalBaseURL, external: callerSupplied},
	}
	if config.GetConfig().IsLocalDevEnv {
		candidates = append(
			candidates,
			thunderURLCandidate{baseURL: externalBaseURL, resolveToHost: "host.docker.internal:8080"},
			thunderURLCandidate{baseURL: externalBaseURL, resolveToHost: "127.0.0.1:8080"},
		)
	}
	return candidates
}

// probeThunderURL reports whether a GET to url succeeds AND its body is a valid JWKS
// document (see isValidJWKS) — a bare HTTP 200 is not sufficient evidence of a live
// Thunder instance, since an unrelated server answering on a probed fallback address
// would otherwise be misreported as reachable. Optionally dials resolveToHost instead
// of the URL's own host (see thunderURLCandidate).
//
// external selects an SSRF-hardened client (re-resolves at dial time, re-validates
// every redirect hop) instead of the plain default one — see thunderURLCandidate's
// doc comment for exactly which candidate this applies to and why.
func probeThunderURL(ctx context.Context, url, resolveToHost string, external bool) bool {
	reqCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	if resolveToHost != "" {
		req.Host = req.URL.Host
		req.URL.Host = resolveToHost
	}
	httpClient := http.DefaultClient
	if external {
		httpClient = ssrfProbeClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJWKSProbeBodyBytes))
	if err != nil {
		return false
	}
	return isValidJWKS(body)
}

// thunderURLProber checks whether a candidate base URL is reachable — its JWKS
// endpoint is used as the health check since it requires no authentication.
// Injectable so callers can test the candidate-selection cascade without real
// network access.
type thunderURLProber func(ctx context.Context, candidate thunderURLCandidate) bool

// defaultThunderURLProber is the real, network-probing implementation used outside tests.
func defaultThunderURLProber(ctx context.Context, candidate thunderURLCandidate) bool {
	return probeThunderURL(ctx, candidate.baseURL+"/oauth2/jwks", candidate.resolveToHost, candidate.external)
}

// resolveThunderBaseURL returns the first candidate base URL that prober reports as
// reachable, trying them in thunderBaseURLCandidates' preference order. ok is false
// if none respond.
func resolveThunderBaseURL(ctx context.Context, org, env, thunderURL string, callerSupplied bool, prober thunderURLProber) (candidate thunderURLCandidate, ok bool) {
	for _, c := range thunderBaseURLCandidates(org, env, thunderURL, callerSupplied) {
		if prober(ctx, c) {
			return c, true
		}
	}
	return thunderURLCandidate{}, false
}

// ResolveThunderBaseURL returns the first reachable base URL for an env-Thunder
// instance, trying cluster-internal DNS, the external ingress hostname, then (in local
// dev only) Docker Desktop/Linux host-networking fallbacks. Callers that build an HTTP
// client against the result must dial resolveToHost (when non-empty) instead of the
// base URL's own host, while still sending the base URL's host as the Host header.
// thunderURL is the stored EnvThunderURL.ThunderURL origin; callerSupplied reports
// whether it was supplied directly by the caller (no handle) rather than computed
// by AMS from a handle.
func ResolveThunderBaseURL(ctx context.Context, org, env, thunderURL string, callerSupplied bool) (baseURL, resolveToHost string, ok bool) {
	c, ok := resolveThunderBaseURL(ctx, org, env, thunderURL, callerSupplied, defaultThunderURLProber)
	return c.baseURL, c.resolveToHost, ok
}

// ThunderProbe checks whether an env-Thunder instance is reachable by trying the same
// candidate cascade as ResolveThunderBaseURL (each candidate's JWKS endpoint, validated
// as an actual JWKS document via isValidJWKS — not just an HTTP 200). All candidates
// are probed CONCURRENTLY, not one after another, so latency is bounded by a single
// probe's 2-second timeout rather than the sum of however many are tried. Callers treat
// a negative probe as "not provisioned" and skip the env. thunderURL is the stored
// EnvThunderURL.ThunderURL origin; callerSupplied reports whether it was supplied
// directly by the caller (no handle) rather than computed by AMS from a handle.
func ThunderProbe(ctx context.Context, org, env, thunderURL string, callerSupplied bool) bool {
	candidates := thunderBaseURLCandidates(org, env, thunderURL, callerSupplied)

	probeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan bool, len(candidates))
	for _, c := range candidates {
		go func(c thunderURLCandidate) {
			results <- probeThunderURL(probeCtx, c.baseURL+"/oauth2/jwks", c.resolveToHost, c.external)
		}(c)
	}

	for range candidates {
		if <-results {
			return true
		}
	}
	return false
}

const (
	maxAgentAppNameLen = 100
	// agentAppNameTruncatePrefixLen leaves room for "-" + a 6-char thunderSHA6
	// suffix (7 chars) within maxAgentAppNameLen, mirroring ThunderReleaseName's
	// truncation scheme above.
	agentAppNameTruncatePrefixLen = maxAgentAppNameLen - 7
)

// AgentThunderAppName returns the OAuth app name to use in Thunder for a per-agent client.
// Format: amp-agent-<org>-<env>-<project>-<agent>, truncated to 100 chars with a
// collision-safe hash suffix (see ThunderReleaseName). The name mirrors
// amp-publisher-<org> but is fully scoped to env + project + agent to avoid
// collisions.
//
// env is included even though each env already has its own physically separate
// Thunder instance (so it isn't needed for uniqueness there): without it, every
// env-Thunder's agent list looks identical — e.g. "amp-agent-default-default-x" in
// both the "stage" and "testing" instances — with nothing in the name itself
// showing which environment you're looking at from inside Thunder's own console.
func AgentThunderAppName(org, env, project, agent string) string {
	org = slugify(org)
	env = slugify(env)
	project = slugify(project)
	agent = slugify(agent)
	if org == "" || env == "" || project == "" || agent == "" {
		panic("org, env, project, and agent names must be valid alphanumeric slugs and not empty")
	}
	name := fmt.Sprintf("amp-agent-%s-%s-%s-%s", org, env, project, agent)
	if len(name) <= maxAgentAppNameLen {
		return strings.TrimSuffix(name, "-")
	}
	hash := thunderSHA6(org + "/" + env + "/" + project + "/" + agent)
	prefix := strings.TrimRight(name[:agentAppNameTruncatePrefixLen], "-")
	return fmt.Sprintf("%s-%s", prefix, hash)
}

// thunderSHA6 returns the first 6 hex characters of the SHA-256 hash of s.
// Produces the same output as _sha6() in deployments/scripts/thunder-naming.sh.
func thunderSHA6(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:])[:6]
}

// slugify converts string to lowercase, replaces invalid characters (spaces, underscores)
// with hyphens, merges consecutive hyphens, and trims leading/trailing hyphens.
func slugify(s string) string {
	s = strings.ToLower(s)
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		} else if r == '-' || r == '_' || r == ' ' {
			sb.WriteRune('-')
		}
	}
	res := sb.String()
	for strings.Contains(res, "--") {
		res = strings.ReplaceAll(res, "--", "-")
	}
	return strings.Trim(res, "-")
}
