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
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// creationProvisioningStub records how agent creation asked for identity.
type creationProvisioningStub struct {
	*stubAgentThunderProvisioning
	handlePipeline bool
	pipelineCalls  []PipelineIdentityEnvironment
	pipelineAwait  string
	legacyEnvs     []string
	legacyAwait    string
}

func (s *creationProvisioningStub) ProvisionForAgentInPipeline(_ context.Context, _ AgentIdentityRef,
	pipeline []PipelineIdentityEnvironment, _, awaitEnv string,
) bool {
	s.pipelineCalls, s.pipelineAwait = pipeline, awaitEnv
	return s.handlePipeline
}

func (s *creationProvisioningStub) EnsurePipelineBindings(context.Context, AgentIdentityRef, []PipelineIdentityEnvironment) (int, bool) {
	return 0, false
}

func (s *creationProvisioningStub) IdentityPromotionEnabled() bool { return true }

func (s *creationProvisioningStub) ProvisionForAgent(_ context.Context, _, _, _ string, _ models.AgentProvisioningType,
	envNames []string, _, awaitEnv string,
) {
	s.legacyEnvs, s.legacyAwait = envNames, awaitEnv
}

func creationService(stub AgentThunderProvisioningService) *agentManagerService {
	return &agentManagerService{
		ocClient: &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
				return []*models.EnvironmentResponse{{Name: "dev"}, {Name: "stage"}, {Name: "prod"}, {Name: "elsewhere"}}, nil
			},
		},
		agentThunderProvisioning: stub,
		logger:                   discardLogger(),
	}
}

var devStageProd = []models.PromotionPath{
	{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "stage"}}},
	{SourceEnvironmentRef: "stage", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "prod"}}},
}

func TestAgentCreationProvisionsThePipelineInOrderWhenIdentityIsPromoted(t *testing.T) {
	stub := &creationProvisioningStub{stubAgentThunderProvisioning: &stubAgentThunderProvisioning{}, handlePipeline: true}
	s := creationService(stub)

	started := s.provisionAgentIdentities(context.Background(), "org-id", "default",
		&spec.CreateAgentRequest{Name: "scout", Provisioning: spec.Provisioning{Type: "internal"}}, "dev", devStageProd)

	require.True(t, started)
	require.Equal(t, pipelineOf("dev", "stage", "prod"), stub.pipelineCalls, "only the project's pipeline, in promotion order")
	require.Equal(t, "dev", stub.pipelineAwait, "the first environment is still provisioned inline")
	require.Nil(t, stub.legacyEnvs, "the per environment path is not used")
}

func TestAgentCreationKeepsTheLegacyPathWhenThePipelineIsNotPromoted(t *testing.T) {
	stub := &creationProvisioningStub{stubAgentThunderProvisioning: &stubAgentThunderProvisioning{}, handlePipeline: false}
	s := creationService(stub)

	started := s.provisionAgentIdentities(context.Background(), "org-id", "default",
		&spec.CreateAgentRequest{Name: "scout", Provisioning: spec.Provisioning{Type: "external"}}, "dev", devStageProd)

	require.True(t, started)
	require.Equal(t, []string{"dev", "stage", "prod", "elsewhere"}, stub.legacyEnvs, "every org environment, as it always was")
	require.Equal(t, "dev", stub.legacyAwait)
}

func TestAgentCreationWithoutThePipelineCapabilityIsUnchanged(t *testing.T) {
	stub := &legacyOnlyProvisioning{stubAgentThunderProvisioning: &stubAgentThunderProvisioning{}}
	s := creationService(stub)

	require.True(t, s.provisionAgentIdentities(context.Background(), "org-id", "default",
		&spec.CreateAgentRequest{Name: "scout", Provisioning: spec.Provisioning{Type: "internal"}}, "", devStageProd))
	require.Equal(t, []string{"dev", "stage", "prod", "elsewhere"}, stub.envs)
}

type legacyOnlyProvisioning struct {
	*stubAgentThunderProvisioning
	envs []string
}

func (s *legacyOnlyProvisioning) ProvisionForAgent(_ context.Context, _, _, _ string, _ models.AgentProvisioningType,
	envNames []string, _, _ string,
) {
	s.envs = envNames
}
