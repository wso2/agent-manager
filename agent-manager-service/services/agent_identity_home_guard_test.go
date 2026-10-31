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

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestResolveReportsAPipelineStartingInAPromotedEnvironmentAsAConflictNotUnavailable(t *testing.T) {
	f := newIdentityResourceFixture(t)
	f.svc.resolver = &clientmocks.EnvThunderResolverMock{
		ResolveIdentityFunc: func(context.Context, string, string, string) (thundersvc.EnvIdentityClient, error) {
			return nil, thundersvc.ErrIdentityEnvironmentUnsupported
		},
	}
	_, err := f.svc.ResolveAgentIdentityResources(context.Background(), f.query())
	var typed *IdentityPromotionError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, IdentityPromotionConflict, typed.Kind)
	require.Equal(t, identityHomeMismatchMessage, typed.Message)
}

func TestPromoteIdentityRefusesAHomeEnvironmentThatReceivesItsIdentityByPromotion(t *testing.T) {
	resolver := placementResolver{
		EnvThunderResolverMock: &clientmocks.EnvThunderResolverMock{},
		byPromotion:            map[string]bool{"development": true, "staging": true},
	}
	f := newPromotionFixture(t, resolver)
	f.svc.SetIdentityPromotionManager(noopManager())
	withBooker(f)

	_, err := f.svc.PromoteIdentityResources(auditableCtx(t), externalRequest())
	var typed *IdentityPromotionError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, IdentityPromotionConflict, typed.Kind)
	require.Equal(t, identityHomeMismatchMessage, typed.Message)
	require.Empty(t, f.upserted)
}

type zeroConfigService struct{ AgentConfigurationService }

func (zeroConfigService) List(context.Context, string, string, string, int, int) (*models.AgentModelConfigListResponse, error) {
	return &models.AgentModelConfigListResponse{}, nil
}

func TestCreateAgentIsRefusedWhenTheFirstEnvironmentIsNotWhereIdentityIsManaged(t *testing.T) {
	pf := newPromotionFixture(t, stagingByPromotion())
	pf.svc.envResolver = placementResolver{
		EnvThunderResolverMock: &clientmocks.EnvThunderResolverMock{},
		byPromotion:            map[string]bool{"development": true},
	}
	created := false
	oc := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, orgName string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: orgName}, nil
		},
		GetProjectDeploymentPipelineFunc: func(context.Context, string, string) (*models.DeploymentPipelineResponse, error) {
			return &models.DeploymentPipelineResponse{PromotionPaths: []models.PromotionPath{
				{SourceEnvironmentRef: "development", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
			}}, nil
		},
		CreateComponentFunc: func(context.Context, string, string, client.CreateComponentRequest) error {
			created = true
			return nil
		},
	}
	svc := &agentManagerService{
		ocClient: oc, agentThunderProvisioning: pf.svc, agentConfigurationService: zeroConfigService{}, logger: discardLogger(),
	}

	err := svc.createComponentAgent(context.Background(), "org-id", "default", &spec.CreateAgentRequest{
		Name: "scout", Provisioning: spec.Provisioning{Type: string(utils.InternalAgent)},
	}, "")

	require.ErrorIs(t, err, utils.ErrIdentityHomeMismatch)
	require.False(t, created, "nothing is created before the guard passes")
}
