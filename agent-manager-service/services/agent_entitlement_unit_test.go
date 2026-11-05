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

// An agent create asks the installed EntitlementChecker before any OpenChoreo
// write, naming the component type so internal and external agents are limited
// separately.
package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type entitlementCall struct {
	ouID         string
	resourceType string
	quantity     int
}

type stubEntitlementChecker struct {
	err   error
	calls []entitlementCall
}

func (c *stubEntitlementChecker) CheckCreate(_ context.Context, ouID, resourceType string, quantity int) error {
	c.calls = append(c.calls, entitlementCall{ouID: ouID, resourceType: resourceType, quantity: quantity})
	return c.err
}

func entitlementFixture(checker EntitlementChecker) (*agentManagerService, *writeRecorder) {
	svc, rec := gatewayGateFixture(apiAgentType(""), ingressGateway)
	svc.agentConfigurationService = &stubAgentConfigurationService{
		ListFunc: func(context.Context, string, string, string, int, int) (*models.AgentModelConfigListResponse, error) {
			return &models.AgentModelConfigListResponse{}, nil
		},
	}
	if checker != nil {
		svc.SetEntitlementChecker(checker)
	}
	return svc, rec
}

func createAgentRequest(provisioningType utils.AgentProvisioningType) *spec.CreateAgentRequest {
	return &spec.CreateAgentRequest{
		Name:         "my-agent",
		Provisioning: spec.Provisioning{Type: string(provisioningType)},
		AgentType:    &spec.AgentType{Type: string(utils.AgentTypeAPI)},
	}
}

func TestCreateRefusedWhenEntitlementDenied(t *testing.T) {
	checker := &stubEntitlementChecker{err: &utils.EntitlementDeniedError{Message: "Quota limit reached for platform_agents."}}
	svc, rec := entitlementFixture(checker)

	err := svc.createComponentAgent(tierGrantedCtx(t), "acme", "proj1", createAgentRequest(utils.InternalAgent), "")

	require.ErrorIs(t, err, utils.ErrEntitlementDenied)
	assert.EqualError(t, err, "Quota limit reached for platform_agents.")
	rec.assertNoWrites(t)
}

func TestCreateChecksTheAgentsComponentType(t *testing.T) {
	cases := map[string]struct {
		provisioning utils.AgentProvisioningType
		want         string
	}{
		"internal agent": {provisioning: utils.InternalAgent, want: "proxy/agent-api"},
		"external agent": {provisioning: utils.ExternalAgent, want: "proxy/external-agent-api"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			checker := &stubEntitlementChecker{}
			svc, rec := entitlementFixture(checker)

			err := svc.createComponentAgent(tierGrantedCtx(t), "acme", "proj1", createAgentRequest(tc.provisioning), "")

			require.ErrorIs(t, err, errStopAfterTier, "an allowed create goes on to create the component")
			assert.Equal(t, []entitlementCall{{ouID: "acme", resourceType: tc.want, quantity: 1}}, checker.calls)
			assert.Contains(t, rec.calls, "CreateComponent")
		})
	}
}

func TestCreateWithoutEntitlementCheckerProceeds(t *testing.T) {
	svc, rec := entitlementFixture(nil)

	err := svc.createComponentAgent(tierGrantedCtx(t), "acme", "proj1", createAgentRequest(utils.InternalAgent), "")

	require.ErrorIs(t, err, errStopAfterTier)
	assert.Contains(t, rec.calls, "CreateComponent")
}
