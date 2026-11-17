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

package gwsvc

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modules/gateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// mistralaiLikeTemplate mirrors the shape the built-in templates actually serve, as
// observed live: an endpoint URL plus an api-key auth block with a "Bearer " prefix.
func mistralaiLikeTemplate() *models.LLMProviderTemplate {
	return &models.LLMProviderTemplate{
		Handle: "mistralai",
		Metadata: &models.LLMProviderTemplateMetadata{
			EndpointURL: "https://api.mistral.ai",
			Auth: &models.LLMProviderTemplateAuth{
				Type:        "api-key",
				Header:      "Authorization",
				ValuePrefix: "Bearer ",
			},
		},
	}
}

func providerWithUpstream(upstream *models.UpstreamConfig) *models.LLMProvider {
	return &models.LLMProvider{
		Configuration: models.LLMProviderConfig{
			Handle:   "diag",
			Name:     "Diag",
			Version:  "v1.0",
			Template: "mistralai",
			Upstream: upstream,
		},
	}
}

// The core of #1667: every client documents the endpoint URL as inherited from the
// template, but no layer filled it in, so the deployed proxy had no upstream URL and
// the empty value was silently dropped from the gateway config.
func TestApplyTemplateUpstreamDefaults_FillsURLAndAuthWhenAbsent(t *testing.T) {
	provider := providerWithUpstream(nil)

	applyTemplateUpstreamDefaults(provider, mistralaiLikeTemplate())

	main := provider.Configuration.Upstream.Main
	require.NotNil(t, main)
	assert.Equal(t, "https://api.mistral.ai", main.URL)
	require.NotNil(t, main.Auth)
	assert.Equal(t, "api-key", *main.Auth.Type)
	assert.Equal(t, "Authorization", *main.Auth.Header)
}

// Live evidence showed a created provider carrying auth with a type and a value but
// no header at all, so even a provider with a URL could not authenticate upstream.
func TestApplyTemplateUpstreamDefaults_FillsHeaderOnCallerSuppliedAuth(t *testing.T) {
	provider := providerWithUpstream(&models.UpstreamConfig{
		Main: &models.UpstreamEndpoint{
			Auth: &models.UpstreamAuth{
				Type:  strPtr("api-key"),
				Value: strPtr("sk-caller-key"),
			},
		},
	})

	applyTemplateUpstreamDefaults(provider, mistralaiLikeTemplate())

	main := provider.Configuration.Upstream.Main
	assert.Equal(t, "https://api.mistral.ai", main.URL)
	assert.Equal(t, "Authorization", *main.Auth.Header)
	assert.Equal(t, "Bearer sk-caller-key", *main.Auth.Value,
		"the template's value prefix must be applied to the credential")
}

func TestApplyTemplateUpstreamDefaults_DoesNotDoublePrefix(t *testing.T) {
	provider := providerWithUpstream(&models.UpstreamConfig{
		Main: &models.UpstreamEndpoint{
			Auth: &models.UpstreamAuth{Value: strPtr("Bearer sk-already-prefixed")},
		},
	})

	applyTemplateUpstreamDefaults(provider, mistralaiLikeTemplate())

	assert.Equal(t, "Bearer sk-already-prefixed", *provider.Configuration.Upstream.Main.Auth.Value)
}

func TestApplyTemplateUpstreamDefaults_CallerValuesWin(t *testing.T) {
	provider := providerWithUpstream(&models.UpstreamConfig{
		Main: &models.UpstreamEndpoint{
			URL: "https://proxy.internal/mistral",
			Auth: &models.UpstreamAuth{
				Type:   strPtr("none"),
				Header: strPtr("X-Custom-Key"),
			},
		},
	})

	applyTemplateUpstreamDefaults(provider, mistralaiLikeTemplate())

	main := provider.Configuration.Upstream.Main
	assert.Equal(t, "https://proxy.internal/mistral", main.URL)
	assert.Equal(t, "none", *main.Auth.Type)
	assert.Equal(t, "X-Custom-Key", *main.Auth.Header)
}

// A URL with no credential used to leave auth omitted entirely; the template's
// scheme now applies so the provider is at least coherent.
func TestApplyTemplateUpstreamDefaults_URLOnlyStillGetsTheAuthScheme(t *testing.T) {
	provider := providerWithUpstream(&models.UpstreamConfig{
		Main: &models.UpstreamEndpoint{URL: "https://proxy.internal/mistral"},
	})

	applyTemplateUpstreamDefaults(provider, mistralaiLikeTemplate())

	auth := provider.Configuration.Upstream.Main.Auth
	require.NotNil(t, auth)
	assert.Equal(t, "api-key", *auth.Type)
	assert.Equal(t, "Authorization", *auth.Header)
	assert.Nil(t, auth.Value)
}

func TestApplyTemplateUpstreamDefaults_TemplateWithoutMetadataChangesNothing(t *testing.T) {
	provider := providerWithUpstream(nil)

	applyTemplateUpstreamDefaults(provider, &models.LLMProviderTemplate{Handle: "bare"})

	assert.Nil(t, provider.Configuration.Upstream)
}

func TestValidateProviderVersion(t *testing.T) {
	for _, valid := range []string{"v1.0", "v2.11", "v10.0"} {
		assert.NoError(t, validateProviderVersion(valid), "version %q should be accepted", valid)
	}
	// "v1" is what the CLI sent by default and the server accepted without complaint,
	// despite the spec pattern.
	for _, invalid := range []string{"v1", "1.0", "v1.0.0", "", "latest"} {
		err := validateProviderVersion(invalid)
		assert.ErrorIs(t, err, utils.ErrInvalidInput, "version %q should be rejected", invalid)
	}
}

// serviceWithTemplateRepo builds a service whose built-in store is empty, so
// resolveTemplate always falls through to the org-template repository.
func serviceWithTemplateRepo(
	getByHandle func(handle, ouID string) (*models.LLMProviderTemplate, error),
) *LLMProviderService {
	return &LLMProviderService{
		templateStore: NewLLMTemplateStore(),
		templateRepo: &gwrepomocks.LLMProviderTemplateRepositoryMock{
			GetByHandleFunc: getByHandle,
		},
	}
}

func TestResolveTemplate_PrefersBuiltIn(t *testing.T) {
	store := NewLLMTemplateStore()
	store.Load([]*models.LLMProviderTemplate{mistralaiLikeTemplate()})
	// templateRepo left nil: a built-in hit must not reach the database.
	svc := &LLMProviderService{templateStore: store}

	resolved, err := svc.resolveTemplate("mistralai", "ou-acme")

	require.NoError(t, err)
	assert.Equal(t, "https://api.mistral.ai", resolved.Metadata.EndpointURL)
}

func TestResolveTemplate_FallsBackToOrgTemplate(t *testing.T) {
	custom := &models.LLMProviderTemplate{
		Handle:   "inhouse",
		Metadata: &models.LLMProviderTemplateMetadata{EndpointURL: "https://llm.internal"},
	}
	svc := serviceWithTemplateRepo(func(handle, ouID string) (*models.LLMProviderTemplate, error) {
		assert.Equal(t, "inhouse", handle)
		assert.Equal(t, "ou-acme", ouID)
		return custom, nil
	})

	resolved, err := svc.resolveTemplate("inhouse", "ou-acme")

	require.NoError(t, err)
	assert.Equal(t, "https://llm.internal", resolved.Metadata.EndpointURL)
}

func TestResolveTemplate_UnknownHandleIsTemplateNotFound(t *testing.T) {
	svc := serviceWithTemplateRepo(func(_, _ string) (*models.LLMProviderTemplate, error) {
		return nil, gorm.ErrRecordNotFound
	})

	_, err := svc.resolveTemplate("ghost", "ou-acme")

	assert.ErrorIs(t, err, utils.ErrLLMProviderTemplateNotFound)
}

func TestResolveTemplate_RepositoryErrorIsNotMaskedAsNotFound(t *testing.T) {
	boom := errors.New("connection refused")
	svc := serviceWithTemplateRepo(func(_, _ string) (*models.LLMProviderTemplate, error) {
		return nil, boom
	})

	_, err := svc.resolveTemplate("inhouse", "ou-acme")

	assert.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, utils.ErrLLMProviderTemplateNotFound)
}

// The built-in templates live in one process-wide store whose Get copies only one
// level deep: Metadata is cloned, but Metadata.Auth is a pointer the clone shares with
// the store. Handing those fields to a provider by reference would let one
// organization's create rewrite the template every other organization then resolves,
// so the defaults must be copied in.
func TestApplyTemplateUpstreamDefaults_LeavesTheSharedStoreTemplateIntact(t *testing.T) {
	store := NewLLMTemplateStore()
	store.Load([]*models.LLMProviderTemplate{mistralaiLikeTemplate()})
	svc := &LLMProviderService{templateStore: store}

	resolved, err := svc.resolveTemplate("mistralai", "ou-acme")
	require.NoError(t, err)

	provider := providerWithUpstream(&models.UpstreamConfig{
		Main: &models.UpstreamEndpoint{
			Auth: &models.UpstreamAuth{Value: strPtr("sk-caller-key")},
		},
	})
	applyTemplateUpstreamDefaults(provider, resolved)

	// Write through every pointer the provider now holds. Aliased defaults would
	// carry these writes back into the store.
	auth := provider.Configuration.Upstream.Main.Auth
	*auth.Type = "tampered-type"
	*auth.Header = "X-Tampered"
	*auth.Value = "tampered-value"

	stored := store.Get("mistralai")
	require.NotNil(t, stored.Metadata.Auth)
	assert.Equal(t, "api-key", stored.Metadata.Auth.Type)
	assert.Equal(t, "Authorization", stored.Metadata.Auth.Header)
	assert.Equal(t, "Bearer ", stored.Metadata.Auth.ValuePrefix)
	assert.Equal(t, "https://api.mistral.ai", stored.Metadata.EndpointURL)
}

func TestSummarizeDeploymentFailures_NamesEachGateway(t *testing.T) {
	summary := summarizeDeploymentFailures([]DeploymentResult{
		{GatewayID: "gw-1", Success: false, Error: "upstream url is required"},
		{GatewayID: "gw-2", Success: true},
		{GatewayID: "gw-3", Success: false, Error: "gateway offline"},
	})

	assert.Contains(t, summary, "gw-1: upstream url is required")
	assert.Contains(t, summary, "gw-3: gateway offline")
	assert.NotContains(t, summary, "gw-2")
}
