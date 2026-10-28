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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

func TestDeploymentAckHandlerRecordsAgentAcksUnderEitherResourceType(t *testing.T) {
	for _, resourceType := range []string{"agentproxy", "Agent"} {
		t.Run(resourceType, func(t *testing.T) {
			repo := &repomocks.DeploymentRepositoryMock{
				UpdateStatusByDeploymentIDFunc: func(string, string, models.DeploymentStatus) (time.Time, error) {
					return time.Now(), nil
				},
			}
			msg := `{"type":"deployment.ack","payload":{"deploymentId":"d-1","artifactId":"a-1","resourceType":"` +
				resourceType + `","action":"deploy","status":"success"}}`

			assert.True(t, NewDeploymentAckHandler(repo).HandleMessage("gw-1", []byte(msg)))

			calls := repo.UpdateStatusByDeploymentIDCalls()
			require.Len(t, calls, 1)
			assert.Equal(t, models.DeploymentStatusDeployed, calls[0].Status)
		})
	}
}
