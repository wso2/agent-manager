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

package gwsvc

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestAgentGatewayShapeTable(t *testing.T) {
	shape, ok := agentGatewayShapeFor(string(utils.AgentSubTypeA2A))
	require.True(t, ok)
	assert.Equal(t, a2aAgentShape, shape)
	byProtocol, ok := agentGatewayShapeForProtocol(shape.Protocol)
	require.True(t, ok)
	assert.Equal(t, shape, byProtocol)
	assert.Equal(t, gwmodels.KindAgent, a2aAgentShape.ArtifactKind)

	for _, subType := range []string{string(utils.AgentSubTypeChatAPI), "unknown"} {
		_, ok = agentGatewayShapeFor(subType)
		assert.False(t, ok, "subtype %s has no shape", subType)
	}
	_, ok = agentGatewayShapeForProtocol(gwmodels.AgentExposureProtocolREST)
	assert.False(t, ok)
}

func TestAgentGatewayShapeOf(t *testing.T) {
	agentOf := func(provisioning utils.AgentProvisioningType, subType utils.AgentSubType) *models.AgentResponse {
		return &models.AgentResponse{
			Provisioning: models.Provisioning{Type: string(provisioning)},
			Type:         models.AgentType{Type: string(utils.AgentTypeAPI), SubType: string(subType)},
		}
	}

	t.Run("an external agent is always A2A", func(t *testing.T) {
		shape, ok := agentGatewayShapeOf(agentOf(utils.ExternalAgent, utils.AgentSubTypeChatAPI))
		require.True(t, ok)
		assert.Equal(t, a2aAgentShape, shape)
	})
	t.Run("a platform agent of a REST subtype has none", func(t *testing.T) {
		_, ok := agentGatewayShapeOf(agentOf(utils.InternalAgent, utils.AgentSubTypeCustomAPI))
		assert.False(t, ok)
	})
}

func TestAgentEnvAPIArtifactKinds(t *testing.T) {
	repo := &gwrepomocks.ArtifactRepositoryMock{
		GetByHandleFunc: func(handle, ouID string) (*gwmodels.Artifact, error) {
			return &gwmodels.Artifact{UUID: uuid.New(), Handle: handle, Kind: gwmodels.KindRestAPI, OUID: ouID}, nil
		},
	}

	found, err := agentEnvAPIArtifact(repo, "ou", "proj", "agent", "env", gwmodels.KindRestAPI)
	require.NoError(t, err)
	assert.Equal(t, gwmodels.KindRestAPI, found.Kind)

	_, err = agentEnvAPIArtifact(repo, "ou", "proj", "agent", "env", gwmodels.KindAgent)
	assert.ErrorIs(t, err, utils.ErrArtifactNotFound)

	_, err = agentEnvAPIArtifact(repo, "ou", "proj", "agent", "env", gwmodels.AgentArtifactKinds...)
	assert.NoError(t, err)
}
