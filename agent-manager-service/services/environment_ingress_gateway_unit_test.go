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

// An API agent is exposed through its environment's ingress gateway, so create, build, deploy and promote
// refuse an environment without one before any OpenChoreo write.
package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type gatewayMapping int

const (
	noGateway gatewayMapping = iota
	egressOnlyGateway
	ingressGateway
)

// gatewayRepoMapping serves the one gateway a mapping describes, honouring the role filter findIngressGateway sends.
func gatewayRepoMapping(mapping gatewayMapping) *repomocks.GatewayRepositoryMock {
	return &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(f repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			role := models.GatewayRoleEgress
			if mapping == ingressGateway {
				role = models.GatewayRoleIngress
			}
			if mapping == noGateway {
				return nil, nil
			}
			for _, wanted := range f.FunctionalityTypeIn {
				if wanted == role {
					return []*models.Gateway{{UUID: uuid.New(), GatewayFunctionalityType: role}}, nil
				}
			}
			return nil, nil
		},
	}
}

func envWithUUID() func(context.Context, string, string) (*models.EnvironmentResponse, error) {
	envUUID := uuid.NewString()
	return func(_ context.Context, _, envName string) (*models.EnvironmentResponse, error) {
		return &models.EnvironmentResponse{Name: envName, UUID: envUUID}, nil
	}
}

// writeRecorder counts the OpenChoreo writes and the reconcile-block read a refused operation must not reach.
type writeRecorder struct{ calls []string }

func (r *writeRecorder) record(name string) { r.calls = append(r.calls, name) }

func gatewayGateFixture(agentType models.AgentType, mapping gatewayMapping) (*agentManagerService, *writeRecorder) {
	rec := &writeRecorder{}
	oc := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
				Type:         agentType,
			}, nil
		},
		GetProjectDeploymentPipelineFunc: func(context.Context, string, string) (*models.DeploymentPipelineResponse, error) {
			return threeEnvPipeline(), nil
		},
		GetEnvironmentFunc: envWithUUID(),
		ListBuildsFunc:     buildsProducing("img:1"),
		GetComponentReconcileBlockFunc: func(context.Context, string, string) (*client.ComponentReconcileBlock, error) {
			rec.record("GetComponentReconcileBlock")
			return nil, nil //nolint:nilnil // documented contract: a nil block means "not blocked"
		},
		UpdateComponentDeploymentConfigFunc: func(context.Context, string, string, string, client.ComponentDeploymentConfigRequest) error {
			rec.record("UpdateComponentDeploymentConfig")
			return nil
		},
		DeployFunc: func(context.Context, string, string, string, client.DeployRequest) error {
			rec.record("Deploy")
			return nil
		},
		EnsureProjectReleaseBindingFunc: func(context.Context, string, string, string) error {
			rec.record("EnsureProjectReleaseBinding")
			return errStopAfterTier
		},
		IsDeploymentInProgressFunc: func(context.Context, string, string, string) (bool, error) { return false, nil },
		PromoteComponentFunc: func(context.Context, string, string, string, string, string, []client.EnvVar, []client.FileVar, map[string]interface{}, map[string]interface{}) error {
			rec.record("PromoteComponent")
			return nil
		},
		CreateComponentFunc: func(context.Context, string, string, client.CreateComponentRequest) error {
			rec.record("CreateComponent")
			return errStopAfterTier
		},
		TriggerBuildFunc: func(context.Context, string, string, string, string, string, []client.EnvVar, client.BuildClientSource) (*models.BuildResponse, error) {
			rec.record("TriggerBuild")
			return &models.BuildResponse{}, nil
		},
	}
	svc := &agentManagerService{ocClient: oc, gatewayRepo: gatewayRepoMapping(mapping), logger: discardLogger()}
	return svc, rec
}

func (f *writeRecorder) assertNoWrites(t *testing.T) {
	t.Helper()
	assert.Empty(t, f.calls, "a refused operation must not reach OpenChoreo")
}

func deployInto(t *testing.T, svc *agentManagerService) error {
	_, err := svc.DeployAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.DeployAgentRequest{ImageId: "img:1"})
	return err
}

func apiAgentType(subType utils.AgentSubType) models.AgentType {
	return models.AgentType{Type: string(utils.AgentTypeAPI), SubType: string(subType)}
}

func TestDeployRefusedWithoutIngressGateway(t *testing.T) {
	for _, subType := range []utils.AgentSubType{utils.AgentSubTypeA2A, utils.AgentSubTypeCustomAPI} {
		t.Run(string(subType), func(t *testing.T) {
			svc, rec := gatewayGateFixture(apiAgentType(subType), noGateway)

			err := deployInto(t, svc)

			require.ErrorIs(t, err, utils.ErrEnvironmentHasNoGateway)
			rec.assertNoWrites(t)
		})
	}
}

func TestDeployRefusedWithOnlyEgressGateway(t *testing.T) {
	svc, rec := gatewayGateFixture(apiAgentType(""), egressOnlyGateway)

	err := deployInto(t, svc)

	require.ErrorIs(t, err, utils.ErrEnvironmentHasNoGateway)
	rec.assertNoWrites(t)
}

func TestDeployNonAPIAgentNotRefused(t *testing.T) {
	svc, rec := gatewayGateFixture(models.AgentType{Type: "agent-chat"}, noGateway)

	err := deployInto(t, svc)

	assert.NotErrorIs(t, err, utils.ErrEnvironmentHasNoGateway)
	assert.Contains(t, rec.calls, "GetComponentReconcileBlock")
}

func TestDeployAPIAgentWithIngressGatewayProceeds(t *testing.T) {
	svc, rec := gatewayGateFixture(apiAgentType(""), ingressGateway)

	err := deployInto(t, svc)

	assert.NotErrorIs(t, err, utils.ErrEnvironmentHasNoGateway)
	assert.Contains(t, rec.calls, "GetComponentReconcileBlock")
}

func TestPromoteRefusedWithoutIngressGateway(t *testing.T) {
	svc, rec := gatewayGateFixture(apiAgentType(""), noGateway)

	err := svc.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent",
		&spec.PromoteAgentRequest{SourceEnvironment: "dev", TargetEnvironment: "staging"})

	require.ErrorIs(t, err, utils.ErrEnvironmentHasNoGateway)
	rec.assertNoWrites(t)
}

func TestCreateRefusedWithoutIngressGateway(t *testing.T) {
	svc, rec := gatewayGateFixture(apiAgentType(""), noGateway)
	svc.agentConfigurationService = &stubAgentConfigurationService{
		ListFunc: func(context.Context, string, string, string, int, int) (*models.AgentModelConfigListResponse, error) {
			return &models.AgentModelConfigListResponse{}, nil
		},
	}
	req := &spec.CreateAgentRequest{
		Name:         "my-agent",
		Provisioning: spec.Provisioning{Type: string(utils.InternalAgent)},
		AgentType:    &spec.AgentType{Type: string(utils.AgentTypeAPI)},
	}

	err := svc.createComponentAgent(tierGrantedCtx(t), "acme", "proj1", req, "")

	require.ErrorIs(t, err, utils.ErrEnvironmentHasNoGateway)
	rec.assertNoWrites(t)
}

func TestBuildRefusedWithoutIngressGateway(t *testing.T) {
	svc, rec := gatewayGateFixture(apiAgentType(""), noGateway)
	svc.ocClient.(*clientmocks.OpenChoreoClientMock).GetProjectFunc = func(_ context.Context, _, name string) (*models.ProjectResponse, error) {
		return &models.ProjectResponse{Name: name}, nil
	}

	_, err := svc.BuildAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", "")

	require.ErrorIs(t, err, utils.ErrEnvironmentHasNoGateway)
	rec.assertNoWrites(t)
}
