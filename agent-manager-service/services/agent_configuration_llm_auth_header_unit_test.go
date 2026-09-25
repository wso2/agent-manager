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

package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// llmProviderWith builds the nested LLMProvider literal every test in this file needs,
// varying only in handle and security block. One shape to update when SecurityConfig
// changes, rather than a builder closure re-declared per test.
func llmProviderWith(handle string, security *models.SecurityConfig) *models.LLMProvider {
	return &models.LLMProvider{
		Configuration: models.LLMProviderConfig{Handle: handle, Security: security},
	}
}

// enabledAPIKeySecurity is a security block with api-key auth explicitly turned on —
// the shape the console's security tab persists.
func enabledAPIKeySecurity(key, in string) *models.SecurityConfig {
	enabled := true
	return &models.SecurityConfig{
		Enabled: &enabled,
		APIKey:  &models.APIKeySecurity{Enabled: &enabled, Key: key, In: in},
	}
}

// The name and location an agent must send its credential in are reported from the
// proxy's own stored security config rather than a literal, so a proxy provisioned
// with a non-default header — or configured to read its key from the query string —
// is described accurately in the config response. A proxy that doesn't actually
// require a credential must report neither, or the response fabricates an auth
// requirement that isn't real.
func TestLLMProxyAPIKeySecurity(t *testing.T) {
	trueVal := true
	falseVal := false
	proxyWith := func(key, in string, enabled *bool) *models.LLMProxy {
		return &models.LLMProxy{
			Configuration: models.LLMProxyConfig{
				Security: &models.SecurityConfig{
					Enabled: &trueVal,
					APIKey:  &models.APIKeySecurity{Enabled: enabled, Key: key, In: in},
				},
			},
		}
	}

	tests := []struct {
		name     string
		proxy    *models.LLMProxy
		wantName string
		wantIn   string
	}{
		{
			name:     "reports the stored header",
			proxy:    proxyWith("X-Custom-Key", "header", &trueVal),
			wantName: "X-Custom-Key",
			wantIn:   "header",
		},
		{
			// A proxy's deployed gateway policy enforces whatever header it was
			// stored with, so the stored value has to win over the default even
			// when the default later changes.
			name:     "stored header wins over the default",
			proxy:    proxyWith("X-API-Key", "header", &trueVal),
			wantName: "X-API-Key",
			wantIn:   "header",
		},
		{
			name:     "trims surrounding whitespace",
			proxy:    proxyWith("  x-api-key  ", "header", &trueVal),
			wantName: "x-api-key",
			wantIn:   "header",
		},
		{
			name:     "falls back when the stored header is blank",
			proxy:    proxyWith("   ", "header", &trueVal),
			wantName: models.DefaultLLMProxyAPIKeyHeader,
			wantIn:   "header",
		},
		{
			// A query-based proxy's real parameter name must still be reported —
			// not silently replaced by a header-oriented default (CRIT: this used
			// to be mis-described as "header" with a fabricated name).
			name:     "reports a query-based key with its real name and location",
			proxy:    proxyWith("apikey", "query", &trueVal),
			wantName: "apikey",
			wantIn:   "query",
		},
		{
			name:     "falls back to the header default when the query key is blank",
			proxy:    proxyWith("", "query", &trueVal),
			wantName: models.DefaultLLMProxyAPIKeyHeader,
			wantIn:   "query",
		},
		{
			name:     "treats an unrecognized location as a header",
			proxy:    proxyWith("X-Custom-Key", "cookie", &trueVal),
			wantName: "X-Custom-Key",
			wantIn:   "header",
		},
		{
			// CRIT: this used to still fabricate a default "API-Key"/header pair
			// for a proxy that explicitly turned api-key auth off, incorrectly
			// telling agents a credential was required when the proxy accepts
			// unauthenticated calls.
			name:     "reports nothing when api-key auth is explicitly disabled",
			proxy:    proxyWith("X-Custom-Key", "header", &falseVal),
			wantName: "",
			wantIn:   "",
		},
		{
			name:     "reports nothing when api-key auth is not opted into",
			proxy:    proxyWith("X-Custom-Key", "header", nil),
			wantName: "",
			wantIn:   "",
		},
		{
			name: "reports nothing when the proxy's security block is turned off",
			proxy: &models.LLMProxy{
				Configuration: models.LLMProxyConfig{
					Security: &models.SecurityConfig{
						Enabled: &falseVal,
						APIKey:  &models.APIKeySecurity{Enabled: &trueVal, Key: "X-Custom-Key", In: "header"},
					},
				},
			},
			wantName: "",
			wantIn:   "",
		},
		{
			name:     "reports nothing when no api key security is configured",
			proxy:    &models.LLMProxy{Configuration: models.LLMProxyConfig{Security: &models.SecurityConfig{}}},
			wantName: "",
			wantIn:   "",
		},
		{
			name:     "reports nothing when the proxy has no security block",
			proxy:    &models.LLMProxy{},
			wantName: "",
			wantIn:   "",
		},
		{
			name:     "reports nothing for a nil proxy",
			proxy:    nil,
			wantName: "",
			wantIn:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotIn := llmProxyAPIKeySecurity(tt.proxy)
			require.Equal(t, tt.wantName, gotName)
			require.Equal(t, tt.wantIn, gotIn)
		})
	}
}

// A proxy fronting a provider takes the provider's own configured name and location,
// so what an admin sets on the provider is what their agents authenticate with —
// including when the provider reads its own key from the query string, which used to
// be silently collapsed to a header-based default (CRIT: this was the actual cause of
// a proxy still reporting "header"/the default name after its provider was switched
// to query — ProvisionProxy and the resync path both went through this function).
func TestProviderProxyAPIKeySecurity(t *testing.T) {
	providerWithAPIKey := func(apiKey *models.APIKeySecurity) *models.LLMProvider {
		return llmProviderWith("", &models.SecurityConfig{APIKey: apiKey})
	}

	tests := []struct {
		name     string
		provider *models.LLMProvider
		wantName string
		wantIn   string
	}{
		{
			name:     "follows the header configured on the provider",
			provider: providerWithAPIKey(&models.APIKeySecurity{Key: "x-api-key", In: "header"}),
			wantName: "x-api-key",
			wantIn:   "header",
		},
		{
			name:     "treats an unset location as a header",
			provider: providerWithAPIKey(&models.APIKeySecurity{Key: "x-api-key"}),
			wantName: "x-api-key",
			wantIn:   "header",
		},
		{
			name:     "follows the provider's real name and location when it reads its key from the query",
			provider: providerWithAPIKey(&models.APIKeySecurity{Key: "apikey", In: "query"}),
			wantName: "apikey",
			wantIn:   "query",
		},
		{
			name:     "falls back to the header default when the query key is blank",
			provider: providerWithAPIKey(&models.APIKeySecurity{Key: "  ", In: "query"}),
			wantName: models.DefaultLLMProxyAPIKeyHeader,
			wantIn:   "query",
		},
		{
			name:     "falls back when the provider names no header",
			provider: providerWithAPIKey(&models.APIKeySecurity{Key: "  "}),
			wantName: models.DefaultLLMProxyAPIKeyHeader,
			wantIn:   "header",
		},
		{
			name:     "falls back when the provider has no api key security",
			provider: providerWithAPIKey(nil),
			wantName: models.DefaultLLMProxyAPIKeyHeader,
			wantIn:   "header",
		},
		{
			name:     "falls back for a provider with no security block",
			provider: &models.LLMProvider{},
			wantName: models.DefaultLLMProxyAPIKeyHeader,
			wantIn:   "header",
		},
		{
			name:     "falls back for a nil provider",
			provider: nil,
			wantName: models.DefaultLLMProxyAPIKeyHeader,
			wantIn:   "header",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotIn := providerProxyAPIKeySecurity(tt.provider)
			require.Equal(t, tt.wantName, gotName)
			require.Equal(t, tt.wantIn, gotIn)
		})
	}
}

// The staleness check decides whether a provider edit redeploys a proxy at all, so it
// has to be exact in both directions: miss a difference and agents keep sending a header
// the gateway no longer accepts; report one spuriously and an unrelated provider edit
// redeploys the whole fleet. Ingress staleness must catch a location-only change too —
// a provider switched from header to query with the same key name still needs its
// dependent proxies resynced (CRIT: this used to compare names only, so a proxy stayed
// on "header" forever after its provider moved to "query").
func TestProxyAuthHeadersStale(t *testing.T) {
	// A provisioned proxy always carries explicit enabled flags (see
	// newProxyIngressSecurity), so the fixtures do too — an absent flag means something
	// different now that the requirement is part of the comparison.
	// A provisioned proxy's upstream block always carries the encrypted credential
	// alongside the header name; a block with a name but nothing to send is what
	// "needs provisioning" looks like, so the fixtures must not conflate the two.
	secretRef := "encrypted-provider-key"
	proxy := func(ingress, ingressIn string, upstream *string) *models.LLMProxy {
		config := models.LLMProxyConfig{}
		if ingress != "" {
			config.Security = enabledAPIKeySecurity(ingress, ingressIn)
		}
		if upstream != nil {
			config.UpstreamAuth = &models.UpstreamAuth{Header: upstream, SecretRef: &secretRef}
		}
		return &models.LLMProxy{Configuration: config}
	}
	header := func(s string) *string { return &s }
	// The ingress target as the sync builds it: what the provider now implies.
	target := func(name, in string) *models.SecurityConfig { return enabledAPIKeySecurity(name, in) }
	noCredential := func() *models.SecurityConfig {
		disabled := false
		return &models.SecurityConfig{
			Enabled: &disabled,
			APIKey:  &models.APIKeySecurity{Enabled: &disabled, Key: models.DefaultLLMProxyAPIKeyHeader, In: "header"},
		}
	}

	tests := []struct {
		name                      string
		proxy                     *models.LLMProxy
		ingress                   *models.SecurityConfig
		upstream                  string
		wantIngress, wantUpstream bool
	}{
		{
			name:    "both already match",
			proxy:   proxy("x-api-key", "header", header("x-api-key")),
			ingress: target("x-api-key", "header"), upstream: "x-api-key",
		},
		{
			name:    "ingress name differs",
			proxy:   proxy("API-Key", "header", header("x-api-key")),
			ingress: target("x-api-key", "header"), upstream: "x-api-key",
			wantIngress: true,
		},
		{
			// Same name, but the provider moved from header to query — the location
			// itself must be recognized as stale, not just the name.
			name:    "ingress location differs",
			proxy:   proxy("x-api-key", "header", header("x-api-key")),
			ingress: target("x-api-key", "query"), upstream: "x-api-key",
			wantIngress: true,
		},
		{
			// CRIT: the provider was switched to None. A disabled config resolves to the
			// same default name as an enabled one, so comparing names alone saw nothing
			// and left every dependent proxy demanding a key the provider no longer
			// issues — the proxy became uncallable rather than open.
			// Both hops have to stand down together: the ingress stops demanding a key,
			// and the proxy stops forwarding one the provider no longer checks.
			name:    "provider turned api-key auth off",
			proxy:   proxy("API-Key", "header", header("API-Key")),
			ingress: noCredential(), upstream: "",
			wantIngress: true, wantUpstream: true,
		},
		{
			// The reverse: auth turned back on must reach proxies provisioned while it
			// was off, or agents are handed a key the gateway never checks.
			name:    "provider turned api-key auth on",
			proxy:   &models.LLMProxy{Configuration: models.LLMProxyConfig{Security: noCredential()}},
			ingress: target("x-api-key", "header"), upstream: "",
			wantIngress: true,
		},
		{
			name:    "upstream differs",
			proxy:   proxy("x-api-key", "header", header("API-Key")),
			ingress: target("x-api-key", "header"), upstream: "x-api-key",
			wantUpstream: true,
		},
		{
			// The pre-existing gap: a proxy provisioned before an edit carries both
			// stale names, and fixing only one leaves the other hop broken.
			name:    "both differ",
			proxy:   proxy("API-Key", "header", header("API-Key")),
			ingress: target("x-api-key", "header"), upstream: "x-api-key",
			wantIngress: true, wantUpstream: true,
		},
		{
			// CRIT: no upstream block at all while the provider requires a credential —
			// the proxy was provisioned before auth was turned on. This used to report
			// "not stale", leaving the proxy calling the provider anonymously forever.
			name:    "provider requires a credential the proxy has no upstream block for",
			proxy:   proxy("x-api-key", "header", nil),
			ingress: target("x-api-key", "header"), upstream: "x-api-key",
			wantUpstream: true,
		},
		{
			// Neither side wants a credential and the proxy forwards none: nothing to
			// converge, so an unrelated provider edit must not redeploy it.
			name:    "unsecured proxy under an unsecured provider is left alone",
			proxy:   proxy("", "", nil),
			ingress: noCredential(), upstream: "",
		},
		{
			// A provider that names nothing to forward means the proxy should stop
			// forwarding too, even while its own ingress still requires a key.
			name:    "provider names no upstream header",
			proxy:   proxy("x-api-key", "header", header("API-Key")),
			ingress: target("x-api-key", "header"), upstream: "",
			wantUpstream: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIngress, gotUpstream := proxyAuthHeadersStale(tt.proxy, tt.ingress, tt.upstream)
			require.Equal(t, tt.wantIngress, gotIngress, "ingress staleness")
			require.Equal(t, tt.wantUpstream, gotUpstream, "upstream staleness")
		})
	}
}

// Renaming the upstream header is not the only way the hop goes wrong. A proxy created
// while its provider was unsecured carries no UpstreamAuth at all, so enabling api-key
// auth later has to mint one — comparing header names alone left that proxy calling the
// provider anonymously, which surfaces as a 401 from the provider's own gateway. The
// reverse has to stop the proxy forwarding a credential nothing asks for any more.
func TestProxyUpstreamAuthAction(t *testing.T) {
	secretRef := "encrypted"
	withUpstream := func(header *string, secret *string) *models.LLMProxy {
		return &models.LLMProxy{Configuration: models.LLMProxyConfig{
			UpstreamAuth: &models.UpstreamAuth{Header: header, SecretRef: secret},
		}}
	}
	name := func(s string) *string { return &s }

	tests := []struct {
		name           string
		proxy          *models.LLMProxy
		upstreamHeader string
		want           upstreamAuthAction
	}{
		{
			name:  "already matches",
			proxy: withUpstream(name("API-Key"), &secretRef), upstreamHeader: "API-Key",
			want: upstreamAuthUnchanged,
		},
		{
			name:  "header renamed",
			proxy: withUpstream(name("API-Key"), &secretRef), upstreamHeader: "x-api-key",
			want: upstreamAuthRename,
		},
		{
			// CRIT: provisioned while the provider was unsecured, so there is no
			// credential to rename — one has to be minted or the hop stays anonymous.
			name:  "provider gained api-key auth after the proxy existed",
			proxy: &models.LLMProxy{}, upstreamHeader: "API-Key",
			want: upstreamAuthProvision,
		},
		{
			name:  "upstream block present but carries no credential",
			proxy: withUpstream(name("API-Key"), nil), upstreamHeader: "API-Key",
			want: upstreamAuthProvision,
		},
		{
			name:  "provider dropped api-key auth",
			proxy: withUpstream(name("API-Key"), &secretRef), upstreamHeader: "",
			want: upstreamAuthClear,
		},
		{
			name:  "unsecured provider, proxy never had upstream auth",
			proxy: &models.LLMProxy{}, upstreamHeader: "",
			want: upstreamAuthUnchanged,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, proxyUpstreamAuthAction(tt.proxy, tt.upstreamHeader))
		})
	}
}

// The stored config is written before gateways are redeployed, so after a partial
// rollout it matches the target while some gateways still serve the old policy. The
// pending list is what stops that from reading as converged — without it no later sync
// retries, and a stale gateway keeps accepting requests without a now-required key.
func TestProxyIngressStale_PendingRolloutIsNotConverged(t *testing.T) {
	target := enabledAPIKeySecurity("x-api-key", "header")

	converged := &models.LLMProxy{Configuration: models.LLMProxyConfig{
		Security: enabledAPIKeySecurity("x-api-key", "header"),
	}}
	require.False(t, proxyIngressStale(converged, target),
		"a fully rolled out proxy must not be redeployed again")

	// Same stored config, but a gateway was left behind.
	pending := &models.LLMProxy{Configuration: models.LLMProxyConfig{
		Security:                   enabledAPIKeySecurity("x-api-key", "header"),
		AuthRolloutPendingGateways: []string{"gateway-b"},
	}}
	require.True(t, proxyIngressStale(pending, target),
		"a proxy with a gateway still on the previous auth config has work left")
}

// A provider with authentication set to None must not produce a proxy that demands a
// credential: disabling auth also revokes the provider's keys, so such a proxy cannot be
// called at all. The name and location are still carried so re-enabling restores what
// the admin configured rather than a platform default.
func TestNewProxyIngressSecurityFollowsProvider(t *testing.T) {
	disabled := false

	secured := newProxyIngressSecurity(llmProviderWith("acme", enabledAPIKeySecurity("x-api-key", "header")))
	require.True(t, secured.RequiresAPIKey(), "a secured provider must yield a proxy requiring a credential")
	name, in := secured.APIKeyNameAndLocation(models.DefaultLLMProxyAPIKeyHeader)
	require.Equal(t, "x-api-key", name)
	require.Equal(t, "header", in)

	unsecured := newProxyIngressSecurity(llmProviderWith("acme", &models.SecurityConfig{
		Enabled: &disabled,
		APIKey:  &models.APIKeySecurity{Enabled: &disabled, Key: "x-api-key", In: "header"},
	}))
	require.False(t, unsecured.RequiresAPIKey(), "auth set to None must not yield a proxy demanding a key")
	keptName, keptIn := unsecured.APIKeyNameAndLocation(models.DefaultLLMProxyAPIKeyHeader)
	require.Equal(t, "x-api-key", keptName, "the configured name is kept for when auth is re-enabled")
	require.Equal(t, "header", keptIn)

	noSecurity := newProxyIngressSecurity(llmProviderWith("acme", nil))
	require.False(t, noSecurity.RequiresAPIKey(), "a provider with no security block requires no credential")
}

// Provisioning and reporting must agree on the header name: a freshly built proxy
// config is what a later config response reads back, so the two cannot drift.
func TestLLMProxyProvisionedHeaderMatchesReportedHeader(t *testing.T) {
	enabled := true
	provisioned := &models.LLMProxy{
		Configuration: models.LLMProxyConfig{
			Security: &models.SecurityConfig{
				Enabled: &enabled,
				APIKey: &models.APIKeySecurity{
					Enabled: &enabled,
					Key:     models.DefaultLLMProxyAPIKeyHeader,
					In:      "header",
				},
			},
		},
	}

	gotName, gotIn := llmProxyAPIKeySecurity(provisioned)
	require.Equal(t, models.DefaultLLMProxyAPIKeyHeader, gotName)
	require.Equal(t, "header", gotIn)
}

// The sync this gates redeploys every dependent proxy, so it must fire only when a
// provider edit actually changed one of the two headers — never on an edit (name,
// description, policy) that left security untouched, and never on a pre-existing
// mismatch that predates this feature but wasn't part of the current edit.
func TestProviderAuthHeadersChanged(t *testing.T) {
	providerWith := func(key string) *models.LLMProvider {
		return &models.LLMProvider{
			Configuration: models.LLMProviderConfig{
				Security: &models.SecurityConfig{APIKey: &models.APIKeySecurity{Key: key, In: "header"}},
			},
		}
	}

	tests := []struct {
		name              string
		existing, updated *models.LLMProvider
		want              bool
	}{
		{
			name:     "unchanged header",
			existing: providerWith("X-API-Key"),
			updated:  providerWith("X-API-Key"),
			want:     false,
		},
		{
			name:     "renamed header",
			existing: providerWith("API-Key"),
			updated:  providerWith("X-API-Key"),
			want:     true,
		},
		{
			// Both provider configs mirror the default when unset, so this is not a
			// change even though neither names a header explicitly.
			name:     "both fall back to the same default",
			existing: &models.LLMProvider{},
			updated:  &models.LLMProvider{},
			want:     false,
		},
		{
			name:     "nil existing provider",
			existing: nil,
			updated:  providerWith("X-API-Key"),
			want:     true,
		},
		{
			name:     "nil updated provider",
			existing: providerWith("X-API-Key"),
			updated:  nil,
			want:     true,
		},
		{
			// CRIT: a pure location change (same key name, header -> query) used to
			// be invisible here, since the comparison went through a name-only lookup
			// that silently collapsed query locations to the same header default —
			// so switching a provider to query-based auth never triggered a resync.
			name: "same name, location changed from header to query",
			existing: &models.LLMProvider{
				Configuration: models.LLMProviderConfig{
					Security: &models.SecurityConfig{APIKey: &models.APIKeySecurity{Key: "apikey", In: "header"}},
				},
			},
			updated: &models.LLMProvider{
				Configuration: models.LLMProviderConfig{
					Security: &models.SecurityConfig{APIKey: &models.APIKeySecurity{Key: "apikey", In: "query"}},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ProviderAuthHeadersChanged(tt.existing, tt.updated))
		})
	}
}

// SyncDependentProxyAuthHeaders runs detached in its own goroutine (see
// controllers/llm_controller.go), so a second edit to the same provider can commit and
// start its own sync before this one runs. It must resolve headers from the provider's
// current row rather than the snapshot it was handed, or whichever sync finishes last
// can leave proxies on a stale header instead of converging on what's actually stored.
func TestSyncDependentProxyAuthHeaders_RefetchesCurrentProvider(t *testing.T) {
	providerUUID := uuid.New()
	staleSnapshot := &models.LLMProvider{
		UUID: providerUUID,
		Configuration: models.LLMProviderConfig{
			Security: &models.SecurityConfig{APIKey: &models.APIKeySecurity{Key: "B-Header", In: "header"}},
		},
	}
	currentInDB := &models.LLMProvider{
		UUID: providerUUID,
		Configuration: models.LLMProviderConfig{
			Security: &models.SecurityConfig{APIKey: &models.APIKeySecurity{Key: "C-Header", In: "header"}},
		},
	}

	var gotProviderID, gotOuID string
	providerRepo := &repomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc: func(providerID, ouID string) (*models.LLMProvider, error) {
			gotProviderID, gotOuID = providerID, ouID
			return currentInDB, nil
		},
	}
	proxyRepo := &repomocks.LLMProxyRepositoryMock{
		ListByProviderFunc: func(ouID, providerUUID string, limit, offset int) ([]*models.LLMProxy, error) {
			return nil, nil
		},
	}
	svc := &LLMProviderService{providerRepo: providerRepo, proxyRepo: proxyRepo}

	err := svc.SyncDependentProxyAuthHeaders(
		context.Background(), staleSnapshot, "ou-acme", &LLMProxyService{}, &LLMProxyDeploymentService{})

	require.NoError(t, err)
	require.Equal(t, providerUUID.String(), gotProviderID, "must refetch the provider named in the (possibly stale) snapshot it was handed")
	require.Equal(t, "ou-acme", gotOuID)
}

// The provider's gateway enforces the api-key name its deployment resolved, which
// defaults an unnamed key (see llm_deployment_service). The header a proxy forwards
// that credential under has to resolve identically, or a provider that enabled
// api-key auth without naming a key gets a proxy forwarding it under no name at all
// while its gateway checks for the default — a hop that can never authenticate. A
// provider that never enabled api-key auth still names nothing to forward.
func TestProviderUpstreamAPIKeyHeader(t *testing.T) {
	enabled := true
	disabled := false
	providerWith := func(key string, apiKeyEnabled *bool) *models.LLMProvider {
		sec := enabledAPIKeySecurity(key, "header")
		sec.APIKey.Enabled = apiKeyEnabled
		return llmProviderWith("", sec)
	}

	tests := []struct {
		name     string
		provider *models.LLMProvider
		want     string
	}{
		{
			name:     "named key is forwarded as named",
			provider: providerWith("X-API-Key", &enabled),
			want:     "X-API-Key",
		},
		{
			// The gap this closes: deployment resolves the default for the provider's
			// own gateway, so the forwarded header must be that same default.
			name:     "unnamed key on a secured provider resolves the deployed default",
			provider: providerWith("", &enabled),
			want:     models.DefaultLLMProxyAPIKeyHeader,
		},
		{
			name:     "unnamed key without api-key auth names nothing",
			provider: providerWith("", &disabled),
			want:     "",
		},
		{
			name:     "provider without security names nothing",
			provider: &models.LLMProvider{},
			want:     "",
		},
		{
			name:     "nil provider names nothing",
			provider: nil,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, providerUpstreamAPIKeyHeader(tt.provider))
		})
	}
}

// models.UpstreamAuth can only name a header, so a provider that reads its key from the
// query string cannot be fronted by a proxy that authenticates to it. Provisioning has
// to refuse rather than hand back a proxy whose every upstream call is rejected.
func TestProviderUpstreamAPIKeyAuthRejectsQueryLocation(t *testing.T) {
	providerIn := func(in string) *models.LLMProvider {
		return llmProviderWith("acme-openai", enabledAPIKeySecurity("apikey", in))
	}

	name, err := providerUpstreamAPIKeyAuth(providerIn("header"))
	require.NoError(t, err)
	require.Equal(t, "apikey", name)

	_, err = providerUpstreamAPIKeyAuth(providerIn("query"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "acme-openai")
}

// The gateway's api-key-auth policy declares `in` as enum: ["header"] and reads the
// credential from a header only, so a provider saved with any other location produces
// proxies that reject every request. The edit is refused rather than stored, since the
// resulting 401s say nothing about the cause.
func TestValidateProxyAPIKeyLocation(t *testing.T) {
	providerIn := func(in string) *models.LLMProvider {
		return llmProviderWith("", enabledAPIKeySecurity("API-Key", in))
	}

	tests := []struct {
		name     string
		provider *models.LLMProvider
		wantErr  bool
	}{
		{name: "header is supported", provider: providerIn("header"), wantErr: false},
		{name: "unset location defaults to a header", provider: providerIn(""), wantErr: false},
		{name: "case and padding are ignored", provider: providerIn("  Header "), wantErr: false},
		{name: "query is refused", provider: providerIn("query"), wantErr: true},
		{name: "any other location is refused", provider: providerIn("cookie"), wantErr: true},
		{
			name:     "no api key block is nothing to validate",
			provider: &models.LLMProvider{Configuration: models.LLMProviderConfig{Security: &models.SecurityConfig{}}},
			wantErr:  false,
		},
		{name: "no security block is nothing to validate", provider: &models.LLMProvider{}, wantErr: false},
		{name: "nil provider is nothing to validate", provider: nil, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProxyAPIKeyLocation(tt.provider)
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.ErrorIs(t, err, utils.ErrInvalidInput)
		})
	}
}
