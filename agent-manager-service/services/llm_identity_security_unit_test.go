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

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestLLMIdentitySecurityRejectedOnWrite(t *testing.T) {
	ctx := context.Background()
	enabled := true
	security := &spec.SecurityConfig{Enabled: &enabled, Identity: &spec.IdentitySecurity{Enabled: &enabled}}
	provider := goldenProvider()
	provider.Configuration.Security = utils.ConvertSpecToModelLLMProviderConfigFromRequest(&spec.CreateLLMProviderRequest{
		Security: security,
	}).Security
	proxy := goldenProxy()
	proxy.Configuration.Security = utils.ConvertSpecToModelLLMProxyConfig(spec.LLMProxyConfig{Security: security}).Security

	providerService := NewLLMProviderService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	proxyService := NewLLMProxyService(nil, nil, nil)

	_, err := providerService.Create(ctx, "org", "user", provider)
	require.ErrorIs(t, err, utils.ErrLLMIdentityUnsupported)
	require.ErrorIs(t, err, utils.ErrInvalidInput)

	_, err = providerService.Update(ctx, provider.UUID.String(), "org", provider)
	require.ErrorIs(t, err, utils.ErrLLMIdentityUnsupported)

	_, err = proxyService.Create(ctx, "org", "user", proxy)
	require.ErrorIs(t, err, utils.ErrLLMIdentityUnsupported)

	_, err = proxyService.Update(proxy.UUID.String(), "org", proxy)
	require.ErrorIs(t, err, utils.ErrLLMIdentityUnsupported)
}

func TestLLMIdentitySecurityRejectedDuringRendering(t *testing.T) {
	provider, proxy := goldenProvider(), goldenProxy()
	security := &models.SecurityConfig{Identity: &models.IdentitySecurity{Enabled: boolPtr(true)}}
	provider.Configuration.Security = security
	proxy.Configuration.Security = security

	providerYAML, err := renderLLMProvider(provider)
	require.ErrorIs(t, err, utils.ErrLLMIdentityUnsupported)
	require.Empty(t, providerYAML)

	proxyYAML, err := renderLLMProxy(proxy, provider)
	require.ErrorIs(t, err, utils.ErrLLMIdentityUnsupported)
	require.Empty(t, proxyYAML)
}

func TestValidateLLMSecurityAllowsExplicitlyDisabledIdentity(t *testing.T) {
	enabled, disabled := true, false

	require.NoError(t, validateLLMSecurity(&models.SecurityConfig{
		Enabled:  &disabled,
		Identity: &models.IdentitySecurity{Enabled: &enabled},
	}))
	require.NoError(t, validateLLMSecurity(&models.SecurityConfig{
		Enabled:  &enabled,
		Identity: &models.IdentitySecurity{Enabled: &disabled},
	}))
	require.ErrorIs(t, validateLLMSecurity(&models.SecurityConfig{
		Enabled:  &enabled,
		Identity: &models.IdentitySecurity{},
	}), utils.ErrLLMIdentityUnsupported)
}
