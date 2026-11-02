//go:build integration

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// unmapCase is one environment-addressed kind's record in env, its artifact deployed on gateway, which is mapped to
// env.
type unmapCase struct {
	sync     *syncFixture
	core     *reconcile.Core
	env      uuid.UUID
	gateway  uuid.UUID
	artifact models.ArtifactRef
}

func a2aUnmapCase(t *testing.T) unmapCase {
	f := newA2AFixture(t)
	artifact, key := f.platformAgent(t)
	require.NoError(t, f.publisher.exposure.Expose(context.Background(), nil, key, artifact.ArtifactUUID, models.AgentExposureProtocolA2A))
	return unmapCase{sync: f.sync, core: f.core, env: f.env, gateway: f.gateway, artifact: artifact}
}

func mcpUnmapCase(t *testing.T) unmapCase {
	f := newMCPFixture(t)
	return unmapCase{
		sync: f.sync, core: f.svc.core, env: f.env, gateway: f.mapGateway(t, f.env),
		artifact: models.ArtifactRef{OUID: f.sync.ouID, ArtifactUUID: f.artifact, Kind: models.KindMCPMapping},
	}
}

func agentLLMProxyUnmapCase(t *testing.T) unmapCase {
	f := newProxyFixture(t)
	env := uuid.New()
	proxy := f.newProxy(t)
	f.mapToAgent(t, proxy, env)
	return unmapCase{
		sync: f.sync, core: f.proxies.core, env: env, gateway: f.mapGateway(t, env),
		artifact: models.ArtifactRef{OUID: f.sync.ouID, ArtifactUUID: proxy.UUID, Kind: models.KindLLMProxy, Handle: proxy.Handle},
	}
}

func (c unmapCase) unmap(t *testing.T, env uuid.UUID) error {
	t.Helper()
	svc := NewPlatformGatewayService(repositories.NewGatewayRepo(db.GetDB()), nil).WithEnvironmentGuard(c.core)
	return svc.RemoveGatewayFromEnvironment(context.Background(), c.gateway.String(), env.String())
}

func (c unmapCase) setRow(t *testing.T, status, desired models.DeploymentStatus) {
	t.Helper()
	require.NoError(t, db.GetDB().Exec(`UPDATE deployment_status SET status = ?, status_desired = ?, next_attempt_at = NULL
		WHERE artifact_uuid = ? AND gateway_uuid = ?`, status, desired, c.artifact.ArtifactUUID, c.gateway).Error)
}

func (c unmapCase) queueRedeploy(t *testing.T) {
	t.Helper()
	require.NoError(t, db.GetDB().Exec(`UPDATE deployment_status SET next_attempt_at = NOW()
		WHERE artifact_uuid = ? AND gateway_uuid = ?`, c.artifact.ArtifactUUID, c.gateway).Error)
}

func (c unmapCase) mapTo(t *testing.T, env uuid.UUID) {
	t.Helper()
	require.NoError(t, db.GetDB().Exec(`INSERT INTO gateway_environment_mappings (gateway_uuid, environment_uuid) VALUES (?, ?)`,
		c.gateway, env).Error)
}

func (c unmapCase) mapped(t *testing.T, env uuid.UUID) bool {
	t.Helper()
	exists, err := repositories.NewGatewayRepo(db.GetDB()).EnvironmentMappingExists(c.gateway.String(), env.String())
	require.NoError(t, err)
	return exists
}

// A gateway serving an environment's records can't be unmapped from it, whatever state their rows are in, until
// they are undeployed. Its mapping to another environment comes off.
func TestUnmappingAGatewayThatServesTheEnvironment(t *testing.T) {
	kinds := []struct {
		name  string
		setup func(t *testing.T) unmapCase
	}{
		{name: "A2A agent", setup: a2aUnmapCase},
		{name: "MCP proxy", setup: mcpUnmapCase},
		{name: "agent LLM proxy", setup: agentLLMProxyUnmapCase},
	}
	for _, kind := range kinds {
		t.Run(kind.name, func(t *testing.T) {
			c := kind.setup(t)
			c.sync.deploy(t, c.artifact, c.gateway)

			for _, status := range []models.DeploymentStatus{
				models.DeploymentStatusPending, models.DeploymentStatusDeploying,
				models.DeploymentStatusDeployed, models.DeploymentStatusFailed,
			} {
				c.setRow(t, status, models.DeploymentStatusDeployed)
				require.ErrorIs(t, c.unmap(t, c.env), utils.ErrGatewayServesEnvironment, "a %s row blocks", status)
				assert.True(t, c.mapped(t, c.env), "the mapping stays after a refused unmap")
			}

			c.setRow(t, models.DeploymentStatusUndeployed, models.DeploymentStatusUndeployed)
			c.queueRedeploy(t)
			require.ErrorIs(t, c.unmap(t, c.env), utils.ErrGatewayServesEnvironment, "a redeploy owed on an undeployed row blocks")
			c.setRow(t, models.DeploymentStatusFailed, models.DeploymentStatusUndeployed)
			c.queueRedeploy(t)
			require.ErrorIs(t, c.unmap(t, c.env), utils.ErrGatewayServesEnvironment, "a redeploy owed on a failed undeploy blocks")

			other := uuid.New()
			c.mapTo(t, other)
			require.NoError(t, c.unmap(t, other), "a record in another environment doesn't block")
			assert.False(t, c.mapped(t, other))

			c.setRow(t, models.DeploymentStatusFailed, models.DeploymentStatusUndeployed)
			require.NoError(t, c.unmap(t, c.env), "a row being undeployed doesn't block")
			c.mapTo(t, c.env)
			c.setRow(t, models.DeploymentStatusUndeployed, models.DeploymentStatusUndeployed)
			require.NoError(t, c.unmap(t, c.env), "an undeployed row doesn't block")
			assert.False(t, c.mapped(t, c.env))
		})
	}
}

// Gateway-addressed deployments, an LLM provider's and a proxy's deployed by the public route, aren't the
// environment's: they don't hold its mapping.
func TestUnmappingAGatewayWithOnlyGatewayAddressedDeployments(t *testing.T) {
	f := newProxyFixture(t)
	env := uuid.New()
	gateway := f.mapGateway(t, env)
	proxy := f.newProxy(t)
	f.sync.deploy(t, f.sync.seedArtifact(t, f.sync.ouID, models.KindLLMProvider), gateway)
	f.sync.deploy(t, models.ArtifactRef{OUID: f.sync.ouID, ArtifactUUID: proxy.UUID, Kind: models.KindLLMProxy}, gateway)
	c := unmapCase{sync: f.sync, core: f.proxies.core, env: env, gateway: gateway}

	require.NoError(t, c.unmap(t, env))

	assert.False(t, c.mapped(t, env))
}
