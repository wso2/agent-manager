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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
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

func TestCreateFailsWithContextWhenEntitlementCheckFails(t *testing.T) {
	checker := &stubEntitlementChecker{err: fmt.Errorf("%w: billing is unreachable", utils.ErrServiceUnavailable)}
	svc, rec := entitlementFixture(checker)

	err := svc.createComponentAgent(tierGrantedCtx(t), "acme", "proj1", createAgentRequest(utils.InternalAgent), "")

	require.ErrorIs(t, err, utils.ErrServiceUnavailable)
	require.NotErrorIs(t, err, utils.ErrEntitlementDenied)
	assert.Contains(t, err.Error(), "entitlement check for proxy/agent-api")
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

// stubBuildEntitlementChecker also checks builds, as a deployment that limits
// build minutes installs.
type stubBuildEntitlementChecker struct {
	stubEntitlementChecker
	buildErr    error
	buildChecks []string
}

func (c *stubBuildEntitlementChecker) CheckBuild(_ context.Context, ouID string) error {
	c.buildChecks = append(c.buildChecks, ouID)
	return c.buildErr
}

var buildMinutesUsedUp = &utils.EntitlementDeniedError{Message: "Your subscription's 750 build minutes for this month are used up."}

func buildFixture(checker EntitlementChecker) (*agentManagerService, *writeRecorder) {
	svc, rec := entitlementFixture(checker)
	svc.ocClient.(*clientmocks.OpenChoreoClientMock).GetProjectFunc = func(_ context.Context, _, name string) (*models.ProjectResponse, error) {
		return &models.ProjectResponse{Name: name}, nil
	}
	return svc, rec
}

func TestBuildRefusedWhenBuildEntitlementDenied(t *testing.T) {
	checker := &stubBuildEntitlementChecker{buildErr: buildMinutesUsedUp}
	svc, rec := buildFixture(checker)

	_, err := svc.BuildAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", "")

	require.ErrorIs(t, err, utils.ErrEntitlementDenied)
	assert.EqualError(t, err, buildMinutesUsedUp.Message)
	assert.Equal(t, []string{"acme"}, checker.buildChecks)
	assert.NotContains(t, rec.calls, "TriggerBuild")
}

func TestBuildFailsWithContextWhenBuildEntitlementCheckFails(t *testing.T) {
	checker := &stubBuildEntitlementChecker{buildErr: fmt.Errorf("%w: billing is unreachable", utils.ErrServiceUnavailable)}
	svc, rec := buildFixture(checker)

	_, err := svc.BuildAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", "")

	require.ErrorIs(t, err, utils.ErrServiceUnavailable)
	require.NotErrorIs(t, err, utils.ErrEntitlementDenied)
	assert.Contains(t, err.Error(), "build entitlement check")
	assert.NotContains(t, rec.calls, "TriggerBuild")
}

func TestBuildProceedsWhenBuildEntitlementAllows(t *testing.T) {
	checker := &stubBuildEntitlementChecker{}
	svc, rec := buildFixture(checker)

	_, err := svc.BuildAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", "")

	require.NoError(t, err)
	assert.Equal(t, []string{"acme"}, checker.buildChecks)
	assert.Contains(t, rec.calls, "TriggerBuild")
}

// A checker that only checks creates (the long-standing interface) is never
// asked about builds, and builds go ahead.
func TestBuildNotCheckedByCreateOnlyChecker(t *testing.T) {
	svc, rec := buildFixture(&stubEntitlementChecker{err: buildMinutesUsedUp})

	_, err := svc.BuildAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", "")

	require.NoError(t, err)
	assert.Contains(t, rec.calls, "TriggerBuild")
}

func TestCreateOfSourceBuiltAgentRefusedWhenBuildEntitlementDenied(t *testing.T) {
	checker := &stubBuildEntitlementChecker{buildErr: buildMinutesUsedUp}
	svc, rec := entitlementFixture(checker)

	err := svc.createComponentAgent(tierGrantedCtx(t), "acme", "proj1", createAgentRequest(utils.InternalAgent), "")

	require.ErrorIs(t, err, utils.ErrEntitlementDenied)
	assert.EqualError(t, err, buildMinutesUsedUp.Message)
	assert.Len(t, checker.calls, 1, "the agent count is still checked first")
	rec.assertNoWrites(t)
}

// Only a create that builds asks about builds: an agent from a published image
// (a Kind) and an external agent build nothing.
func TestCreateThatBuildsNothingSkipsBuildEntitlement(t *testing.T) {
	cases := map[string]struct {
		provisioning utils.AgentProvisioningType
		imageID      string
	}{
		"internal agent from an image": {provisioning: utils.InternalAgent, imageID: "img:1"},
		"external agent":               {provisioning: utils.ExternalAgent},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			checker := &stubBuildEntitlementChecker{buildErr: buildMinutesUsedUp}
			svc, rec := entitlementFixture(checker)

			err := svc.createComponentAgent(tierGrantedCtx(t), "acme", "proj1", createAgentRequest(tc.provisioning), tc.imageID)

			require.NotErrorIs(t, err, utils.ErrEntitlementDenied)
			assert.Empty(t, checker.buildChecks)
			assert.Contains(t, rec.calls, "CreateComponent")
		})
	}
}
