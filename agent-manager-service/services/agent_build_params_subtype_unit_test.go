//
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
//

package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// buildParamsServiceForSubType wires a service whose only live collaborator is
// the OpenChoreo client, returning an agent that already carries existingSubType.
func buildParamsServiceForSubType(existingSubType string) (*agentManagerService, *clientmocks.OpenChoreoClientMock) {
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: "org-1"}, nil
		},
		GetProjectFunc: func(_ context.Context, _, _ string) (*models.ProjectResponse, error) {
			return &models.ProjectResponse{Name: "proj-1"}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				Name:         "agent-1",
				Type:         models.AgentType{Type: string(utils.AgentTypeAPI), SubType: existingSubType},
				Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
			}, nil
		},
		UpdateComponentBuildParametersFunc: func(_ context.Context, _, _, _ string, _ client.UpdateComponentBuildParametersRequest) error {
			return nil
		},
	}
	return &agentManagerService{ocClient: ocClient, logger: testLogger()}, ocClient
}

func buildParamsRequestWithSubType(subType string) *spec.UpdateAgentBuildParametersRequest {
	return &spec.UpdateAgentBuildParametersRequest{
		Provisioning:   spec.Provisioning{Type: string(utils.InternalAgent)},
		AgentType:      spec.AgentType{Type: string(utils.AgentTypeAPI), SubType: &subType},
		InputInterface: spec.InputInterface{Type: string(utils.InputInterfaceTypeHTTP)},
	}
}

// An A2A agent provisions differently from a REST one — no api-configuration
// trait, published to the gateway as a kind: Agent — so reconfiguring a build
// must not carry it across that line.
func TestUpdateBuildParametersRejectsLeavingA2A(t *testing.T) {
	svc, ocClient := buildParamsServiceForSubType(string(utils.AgentSubTypeA2A))

	_, err := svc.UpdateAgentBuildParameters(context.Background(), "org-1", "proj-1", "agent-1",
		buildParamsRequestWithSubType(string(utils.AgentSubTypeChatAPI)))

	assert.ErrorIs(t, err, utils.ErrImmutableFieldChange)
	assert.Empty(t, ocClient.UpdateComponentBuildParametersCalls(), "the rejected change never reaches OpenChoreo")
}

func TestUpdateBuildParametersRejectsBecomingA2A(t *testing.T) {
	svc, ocClient := buildParamsServiceForSubType(string(utils.AgentSubTypeChatAPI))

	_, err := svc.UpdateAgentBuildParameters(context.Background(), "org-1", "proj-1", "agent-1",
		buildParamsRequestWithSubType(string(utils.AgentSubTypeA2A)))

	assert.ErrorIs(t, err, utils.ErrImmutableFieldChange)
	assert.Empty(t, ocClient.UpdateComponentBuildParametersCalls(), "the rejected change never reaches OpenChoreo")
}

// An agent created before subtypes were recorded carries no subtype. It is not
// A2A, so it stays reconfigurable as a REST agent — but it cannot be promoted.
func TestUpdateBuildParametersRejectsPromotingAnUntypedAgentToA2A(t *testing.T) {
	svc, _ := buildParamsServiceForSubType("")

	_, err := svc.UpdateAgentBuildParameters(context.Background(), "org-1", "proj-1", "agent-1",
		buildParamsRequestWithSubType(string(utils.AgentSubTypeA2A)))

	assert.ErrorIs(t, err, utils.ErrImmutableFieldChange)
}

// Chat and custom API agents are both REST agents, so switching between them
// stays allowed — the gate is about A2A, not about the subtype in general.
func TestUpdateBuildParametersAllowsChatToCustomAPI(t *testing.T) {
	svc, ocClient := buildParamsServiceForSubType(string(utils.AgentSubTypeChatAPI))

	_, err := svc.UpdateAgentBuildParameters(context.Background(), "org-1", "proj-1", "agent-1",
		buildParamsRequestWithSubType(string(utils.AgentSubTypeCustomAPI)))

	require.NoError(t, err)
	require.Len(t, ocClient.UpdateComponentBuildParametersCalls(), 1)
	assert.Equal(t, string(utils.AgentSubTypeCustomAPI),
		ocClient.UpdateComponentBuildParametersCalls()[0].Req.AgentType.SubType)
}
