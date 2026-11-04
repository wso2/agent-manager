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

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

func recordingAckRepo() *repomocks.DeploymentRepositoryMock {
	return &repomocks.DeploymentRepositoryMock{
		ApplyAckFunc: func(context.Context, string, models.DeploymentAck, repositories.AfterLiveFunc) (bool, error) {
			return true, nil
		},
	}
}

func TestDeploymentAckHandlerRecordsAgentAcksUnderEitherResourceType(t *testing.T) {
	for _, resourceType := range []string{"agentproxy", "Agent"} {
		t.Run(resourceType, func(t *testing.T) {
			repo := recordingAckRepo()
			msg := `{"type":"deployment.ack","payload":{"deploymentId":"d-1","artifactId":"a-1","resourceType":"` +
				resourceType + `","action":"deploy","status":"success"}}`

			assert.True(t, NewDeploymentAckHandler(repo, nil).HandleMessage("gw-1", []byte(msg)))

			calls := repo.ApplyAckCalls()
			require.Len(t, calls, 1)
			assert.Equal(t, "gw-1", calls[0].GatewayID)
			assert.Equal(t, models.DeploymentAck{
				DeploymentID: "d-1", ArtifactID: "a-1", ResourceType: resourceType,
				Action: models.DeploymentAckActionDeploy, Status: models.DeploymentAckStatusSuccess,
			}, calls[0].Ack)
		})
	}
}

// Undeploy events carry an empty deployment id, which the gateway echoes; the ack is applied on artifact and gateway.
func TestDeploymentAckHandlerAppliesAnUndeployAckWithNoDeploymentID(t *testing.T) {
	for _, resourceType := range []string{"llmprovider", "llmproxy", "mcpproxy", "Agent"} {
		t.Run(resourceType, func(t *testing.T) {
			repo := recordingAckRepo()
			msg := `{"type":"deployment.ack","payload":{"deploymentId":"","artifactId":"a-1","resourceType":"` +
				resourceType + `","action":"undeploy","status":"success"}}`

			assert.True(t, NewDeploymentAckHandler(repo, nil).HandleMessage("gw-1", []byte(msg)))

			calls := repo.ApplyAckCalls()
			require.Len(t, calls, 1)
			assert.Empty(t, calls[0].Ack.DeploymentID)
			assert.Equal(t, "a-1", calls[0].Ack.ArtifactID)
			assert.Equal(t, models.DeploymentAckActionUndeploy, calls[0].Ack.Action)
		})
	}
}

func TestDeploymentAckHandlerSkipsUnsupportedResourceTypes(t *testing.T) {
	repo := recordingAckRepo()
	msg := `{"type":"deployment.ack","payload":{"deploymentId":"d-1","artifactId":"a-1","resourceType":"websubapi",` +
		`"action":"deploy","status":"success"}}`

	assert.True(t, NewDeploymentAckHandler(repo, nil).HandleMessage("gw-1", []byte(msg)))
	assert.Empty(t, repo.ApplyAckCalls())
}

func TestAckAcceptsAPIResourceType(t *testing.T) {
	repo := recordingAckRepo()
	msg := `{"type":"deployment.ack","payload":{"deploymentId":"d-1","artifactId":"a-1","resourceType":"api",` +
		`"action":"deploy","status":"success"}}`

	assert.True(t, NewDeploymentAckHandler(repo, nil).HandleMessage("gw-1", []byte(msg)))
	assert.Len(t, repo.ApplyAckCalls(), 1)
}

func TestDeploymentAckHandlerPassesItsAfterLiveHook(t *testing.T) {
	repo := recordingAckRepo()
	called := false
	afterLive := func(context.Context, *gorm.DB, models.Placement, uuid.UUID) error {
		called = true
		return nil
	}
	msg := `{"type":"deployment.ack","payload":{"deploymentId":"d-1","artifactId":"a-1","resourceType":"Agent",` +
		`"action":"deploy","status":"success"}}`

	assert.True(t, NewDeploymentAckHandler(repo, afterLive).HandleMessage("gw-1", []byte(msg)))

	calls := repo.ApplyAckCalls()
	require.Len(t, calls, 1)
	require.NotNil(t, calls[0].AfterLive)
	require.NoError(t, calls[0].AfterLive(context.Background(), nil, models.Placement{}, uuid.Nil))
	assert.True(t, called)
}
