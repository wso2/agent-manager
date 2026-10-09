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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// undeployService answers the OC state update with stateErr and records card deletes.
func undeployService(stateErr error, cards *repomocks.A2AAgentCardRepositoryMock) *agentManagerService {
	svc, _ := wiringService(map[string]bool{"dev": false})
	svc.ocClient.(*clientmocks.OpenChoreoClientMock).UpdateDeploymentStateFunc = func(
		context.Context, string, string, string, string, gen.ReleaseBindingSpecState,
	) error {
		return stateErr
	}
	svc.a2aCardRepo = cards
	return svc
}

func deletingPlatformCards(deleteErr error) *repomocks.A2AAgentCardRepositoryMock {
	return &repomocks.A2AAgentCardRepositoryMock{
		DeletePlatformForAgentEnvFunc: func(context.Context, string, string, string, string) error { return deleteErr },
	}
}

// An undeployed agent's platform card must not keep showing as fetched.
func TestUndeployDropsThePlatformCard(t *testing.T) {
	cards := deletingPlatformCards(nil)
	svc := undeployService(nil, cards)

	err := svc.UpdateAgentDeploymentState(tierCtx(t, rbac.AgentSuspend, rbac.AgentEnvNonProduction),
		tierOUID, wiringProject, wiringAgent, "dev", utils.DeploymentStateUndeploy)

	require.NoError(t, err)
	require.Len(t, cards.DeletePlatformForAgentEnvCalls(), 1)
	call := cards.DeletePlatformForAgentEnvCalls()[0]
	assert.Equal(t, []string{tierOUID, wiringProject, wiringAgent, "dev"},
		[]string{call.OuID, call.ProjectName, call.AgentName, call.EnvironmentName})
}

func TestUndeployKeepsTheCardWhenNotUndeploying(t *testing.T) {
	// DeletePlatformForAgentEnvFunc is nil: reaching it panics.
	cards := &repomocks.A2AAgentCardRepositoryMock{}
	ctx := tierCtx(t, rbac.AgentSuspend, rbac.AgentEnvNonProduction)

	t.Run("redeploy", func(t *testing.T) {
		err := undeployService(nil, cards).UpdateAgentDeploymentState(ctx,
			tierOUID, wiringProject, wiringAgent, "dev", utils.DeploymentStateActive)
		require.NoError(t, err)
	})
	t.Run("OC refuses the undeploy", func(t *testing.T) {
		err := undeployService(errors.New("binding locked"), cards).UpdateAgentDeploymentState(ctx,
			tierOUID, wiringProject, wiringAgent, "dev", utils.DeploymentStateUndeploy)
		require.Error(t, err)
	})
}

func TestUndeploySucceedsWhenTheCardCleanupFails(t *testing.T) {
	svc := undeployService(nil, deletingPlatformCards(errors.New("db down")))

	err := svc.UpdateAgentDeploymentState(tierCtx(t, rbac.AgentSuspend, rbac.AgentEnvNonProduction),
		tierOUID, wiringProject, wiringAgent, "dev", utils.DeploymentStateUndeploy)

	require.NoError(t, err)
}
