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

// UNIT tests that each tiered operation checks the tier against the RIGHT
// environment.
//
// requireEnvTier's own contract is covered in agent_env_tier_unit_test.go. What
// these add is the wiring: DeployAgent derives its target from the pipeline
// rather than being handed one, so a wrong-environment bug there is invisible
// from the request. Each test drives the real method and asserts the tier check
// both ran and ran against the expected environment name.
package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	wiringProject = "checkout"
	wiringAgent   = "greeter"
)

// wiringPipeline is a dev → staging → production promotion path. Its lowest
// environment is dev, which is what DeployAgent must tier against.
func wiringPipeline() *models.DeploymentPipelineResponse {
	return &models.DeploymentPipelineResponse{
		Name:    "default",
		OrgName: tierOUID,
		PromotionPaths: []models.PromotionPath{
			{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
			{SourceEnvironmentRef: "staging", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: tierProdEnv}}},
		},
	}
}

// errStopAfterTier is returned by the first OpenChoreo write past the tier
// check. A test that expects the check to PASS needs the method to stop
// somewhere deterministic straight afterwards, and this is the marker that it
// got that far.
var errStopAfterTier = errors.New("stop: the tier check passed")

// wiringService returns a service wired far enough to reach the tier check on
// each of the three methods, plus a pointer to the environment name the check
// was asked about.
//
// Nothing past the check is wired for real: the release-binding call returns
// errStopAfterTier, so a passing check surfaces as that sentinel rather than as
// a panic on the next nil mock, and a failing one surfaces as ErrForbidden.
// Which of the two comes back is the assertion.
func wiringService(envs map[string]bool) (*agentManagerService, *string) {
	var checked string
	return &agentManagerService{
		ocClient: &clientmocks.OpenChoreoClientMock{
			GetComponentReconcileBlockFunc: func(context.Context, string, string) (*client.ComponentReconcileBlock, error) {
				return nil, nil //nolint:nilnil // a nil block means the component can reconcile
			},
			IsDeploymentInProgressFunc: func(_ context.Context, _, _, _ string) (bool, error) {
				return false, nil
			},
			EnsureProjectReleaseBindingFunc: func(_ context.Context, _, _, _ string) error {
				return errStopAfterTier
			},
			GetOrganizationFunc: func(_ context.Context, ouID string) (*models.OrganizationResponse, error) {
				return &models.OrganizationResponse{Name: ouID}, nil
			},
			GetComponentFunc: func(_ context.Context, _, _, name string) (*models.AgentResponse, error) {
				return &models.AgentResponse{
					UUID:         "agent-uuid",
					Name:         name,
					Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
				}, nil
			},
			GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
				return wiringPipeline(), nil
			},
			GetEnvironmentFunc: func(_ context.Context, _, envName string) (*models.EnvironmentResponse, error) {
				checked = envName
				isProduction, known := envs[envName]
				if !known {
					return nil, utils.ErrNotFound
				}
				return &models.EnvironmentResponse{Name: envName, IsProduction: isProduction}, nil
			},
		},
		logger: discardLogger(),
	}, &checked
}

// TestDeployAgent_TiersAgainstThePipelineLowestEnvironment is the reason
// agent:deploy-production was dead code: the target is derived, never supplied,
// so it is the pipeline's lowest environment that decides the tier.
func TestDeployAgent_TiersAgainstThePipelineLowestEnvironment(t *testing.T) {
	svc, checked := wiringService(map[string]bool{"dev": true})
	ctx := tierCtx(t, rbac.AgentEnvNonProduction)

	_, err := svc.DeployAgent(ctx, tierOUID, wiringProject, wiringAgent,
		&spec.DeployAgentRequest{ImageId: "img:1"})
	require.ErrorIs(t, err, utils.ErrForbidden)
	require.Equal(t, "dev", *checked, "the tier was checked against the wrong environment")
}

// TestPromoteAgent_TiersAgainstTheTargetEnvironment pins the case the old
// agent:promote scope conflated: promotion into staging and promotion into
// production were one permission.
func TestPromoteAgent_TiersAgainstTheTargetEnvironment(t *testing.T) {
	svc, checked := wiringService(map[string]bool{tierProdEnv: true})
	ctx := tierCtx(t, rbac.AgentEnvNonProduction)

	err := svc.PromoteAgent(ctx, tierOUID, wiringProject, wiringAgent, &spec.PromoteAgentRequest{
		SourceEnvironment: "staging",
		TargetEnvironment: tierProdEnv,
	})
	require.ErrorIs(t, err, utils.ErrForbidden)
	require.Equal(t, tierProdEnv, *checked)
}

// TestPromoteAgent_AllowedIntoNonProductionTarget is the grant this change hands
// Developer and AI Lead, who could not promote at all before.
func TestPromoteAgent_AllowedIntoNonProductionTarget(t *testing.T) {
	svc, checked := wiringService(map[string]bool{"staging": false})
	ctx := tierCtx(t, rbac.AgentEnvNonProduction)

	err := svc.PromoteAgent(ctx, tierOUID, wiringProject, wiringAgent, &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})
	// Reaching the release-binding call is the proof the tier check let it
	// through; a refusal would have returned ErrForbidden before it.
	require.ErrorIs(t, err, errStopAfterTier,
		"a non-production target must not be refused to a caller holding the floor")
	require.Equal(t, "staging", *checked)
}

// TestUpdateAgentDeploymentState_TiersAgainstTheRequestedEnvironment covers the
// third gate. Suspend needs the capability AND the tier, so the floor alone must
// not admit a production environment.
func TestUpdateAgentDeploymentState_TiersAgainstTheRequestedEnvironment(t *testing.T) {
	svc, checked := wiringService(map[string]bool{tierProdEnv: true})
	ctx := tierCtx(t, rbac.AgentSuspend, rbac.AgentEnvNonProduction)

	err := svc.UpdateAgentDeploymentState(ctx, tierOUID, wiringProject, wiringAgent,
		tierProdEnv, utils.DeploymentStateUndeploy)
	require.ErrorIs(t, err, utils.ErrForbidden)
	require.Equal(t, tierProdEnv, *checked)
}
