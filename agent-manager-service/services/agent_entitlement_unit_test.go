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

// An agent create asks the installed EntitlementChecker before any write,
// naming the component type so internal and external agents are limited
// separately.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func entitlementFixture(checker EntitlementChecker) *agentManagerService {
	svc := &agentManagerService{logger: slog.Default()}
	if checker != nil {
		svc.SetEntitlementChecker(checker)
	}
	return svc
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
	svc := entitlementFixture(checker)

	err := svc.checkAgentEntitlement(context.Background(), "acme", createAgentRequest(utils.ExternalAgent))

	require.ErrorIs(t, err, utils.ErrEntitlementDenied)
	assert.EqualError(t, err, "Quota limit reached for platform_agents.")
}

func TestCreateFailsWithContextWhenEntitlementCheckFails(t *testing.T) {
	checker := &stubEntitlementChecker{err: fmt.Errorf("%w: billing is unreachable", utils.ErrServiceUnavailable)}
	svc := entitlementFixture(checker)

	err := svc.checkAgentEntitlement(context.Background(), "acme", createAgentRequest(utils.ExternalAgent))

	require.ErrorIs(t, err, utils.ErrServiceUnavailable)
	require.NotErrorIs(t, err, utils.ErrEntitlementDenied)
	assert.Contains(t, err.Error(), "entitlement check for proxy/external-agent-api")
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
			svc := entitlementFixture(checker)

			require.NoError(t, svc.checkAgentEntitlement(context.Background(), "acme", createAgentRequest(tc.provisioning)))
			assert.Equal(t, []entitlementCall{{ouID: "acme", resourceType: tc.want, quantity: 1}}, checker.calls)
		})
	}
}

func TestCreateWithoutEntitlementCheckerProceeds(t *testing.T) {
	svc := entitlementFixture(nil)

	require.NoError(t, svc.checkAgentEntitlement(context.Background(), "acme", createAgentRequest(utils.ExternalAgent)))
}
