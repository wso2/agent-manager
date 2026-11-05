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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func TestPlacementPolicy_ZeroValueIsPlatformAgent(t *testing.T) {
	assert.Equal(t, platformAgentPolicy, placementPolicy{})
}

func TestPlacementPolicy_ProviderHostingAndAPIKeyPurpose(t *testing.T) {
	cases := []struct {
		name       string
		policy     placementPolicy
		hosting    anchorHosting
		purpose    int
		isExternal bool
		allowsSelf bool
	}{
		{"platform agent", platformAgentPolicy, managedOnly, models.APIKeyPurposeConsoleManaged, false, false},
		{"external agent", externalAgentPolicy, managedFirst, models.APIKeyPurposeUserManaged, true, true},
		{"monitor", monitorPolicy, managedOnly, models.APIKeyPurposeConsoleManaged, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.hosting, tc.policy.providerHosting())
			assert.Equal(t, tc.purpose, tc.policy.apiKeyPurpose())
			assert.Equal(t, tc.isExternal, tc.policy.external())
			assert.Equal(t, tc.allowsSelf, tc.policy.allowsSelfHosted())
		})
	}
}

func TestPlacementPolicy_RefusedBy(t *testing.T) {
	enabled := true
	identityOn := &models.SecurityConfig{Enabled: &enabled, Identity: &models.IdentitySecurity{Enabled: &enabled}}
	managed := &models.Gateway{Hosting: models.GatewayHostingManaged}
	selfHosted := &models.Gateway{Hosting: models.GatewayHostingSelfHosted}

	cases := []struct {
		name     string
		policy   placementPolicy
		gateway  *models.Gateway
		security *models.SecurityConfig
		want     hostingRule
	}{
		{"platform agent, managed, no identity", platformAgentPolicy, managed, nil, 0},
		{"platform agent, managed, identity", platformAgentPolicy, managed, identityOn, 0},
		{"platform agent, self-hosted, no identity", platformAgentPolicy, selfHosted, nil, ruleManagedConsumer},
		{"platform agent, self-hosted, identity", platformAgentPolicy, selfHosted, identityOn, ruleManagedConsumer | ruleAgentIdentity},
		{"external agent, managed, no identity", externalAgentPolicy, managed, nil, 0},
		{"external agent, managed, identity", externalAgentPolicy, managed, identityOn, 0},
		{"external agent, self-hosted, no identity", externalAgentPolicy, selfHosted, nil, 0},
		{"external agent, self-hosted, identity", externalAgentPolicy, selfHosted, identityOn, ruleAgentIdentity},
		{"monitor, managed, no identity", monitorPolicy, managed, nil, 0},
		{"monitor, managed, identity", monitorPolicy, managed, identityOn, 0},
		{"monitor, self-hosted, no identity", monitorPolicy, selfHosted, nil, ruleManagedConsumer},
		{"monitor, self-hosted, identity", monitorPolicy, selfHosted, identityOn, ruleManagedConsumer | ruleAgentIdentity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.policy.refusedBy(tc.gateway, tc.security)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.want.has(ruleManagedConsumer), tc.policy.consumerRefuses(tc.gateway))
			assert.Equal(t, tc.want.has(ruleAgentIdentity), identityRefuses(tc.gateway, tc.security))
		})
	}
}

func TestPlacementPolicy_NilGatewayIsNeverRefused(t *testing.T) {
	enabled := true
	identityOn := &models.SecurityConfig{Enabled: &enabled, Identity: &models.IdentitySecurity{Enabled: &enabled}}
	for _, p := range []placementPolicy{platformAgentPolicy, externalAgentPolicy, monitorPolicy} {
		assert.Zero(t, p.refusedBy(nil, identityOn))
	}
}
