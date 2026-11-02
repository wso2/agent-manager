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

package repositories

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// seedCatalogProvider writes a catalog-listed provider and an environment its gateway is mapped to, and returns the
// provider's placement on the gateway.
func seedCatalogProvider(t *testing.T) (models.Placement, uuid.UUID) {
	t.Helper()
	gdb := db.GetDB()
	p := seedPlacement(t, models.KindLLMProvider)
	gdb.Exec(`DELETE FROM artifacts WHERE uuid = ?`, p.ArtifactUUID)
	provider := &models.LLMProvider{
		TemplateHandle: "openai",
		Configuration: models.LLMProviderConfig{
			Name: "Catalog " + p.Handle, Handle: p.Handle, Version: "v1.0", Template: "openai",
		},
		Status: "CREATED",
	}
	require.NoError(t, NewLLMProviderRepo(gdb).Create(gdb, provider, p.Handle, provider.Configuration.Name, "v1.0", p.OUID))
	require.NoError(t, NewArtifactRepo(gdb).UpdateCatalogStatus(gdb, provider.UUID.String(), p.OUID, true))
	env := uuid.New()
	require.NoError(t, NewGatewayRepo(gdb).CreateEnvironmentMapping(&models.GatewayEnvironmentMapping{
		GatewayUUID: p.GatewayUUID, EnvironmentUUID: env,
	}))
	t.Cleanup(func() {
		gdb.Exec(`DELETE FROM llm_providers WHERE uuid = ?`, provider.UUID)
		gdb.Exec(`DELETE FROM artifacts WHERE uuid = ?`, provider.UUID)
		gdb.Exec(`DELETE FROM gateway_environment_mappings WHERE gateway_uuid = ?`, p.GatewayUUID)
	})
	p.ArtifactUUID = provider.UUID
	return p, env
}

func listCatalogProviders(t *testing.T, ouID string, env *uuid.UUID) []models.CatalogLLMProviderEntry {
	t.Helper()
	filters := &models.CatalogListFilters{OrganizationName: ouID, Limit: 10}
	if env != nil {
		filters.EnvironmentUUID = env.String()
	}
	entries, _, err := NewCatalogRepo(db.GetDB()).ListLLMProviders(filters)
	require.NoError(t, err)
	return entries
}

// A deploy the gateway hasn't acked isn't listed, in the environment-filtered list or the unfiltered one, until the
// ack makes it live.
func TestCatalogListsADeployOnceItIsLive(t *testing.T) {
	p, env := seedCatalogProvider(t)
	deployment := startDeploy(t, p)

	entries := listCatalogProviders(t, p.OUID, nil)
	require.Len(t, entries, 1)
	assert.Empty(t, entries[0].Deployments)
	assert.Empty(t, listCatalogProviders(t, p.OUID, &env), "not live in the environment yet")

	applyAck(t, p, deployAck(deployment.DeploymentID, models.DeploymentAckStatusSuccess, ""), nil)

	entries = listCatalogProviders(t, p.OUID, nil)
	require.Len(t, entries[0].Deployments, 1)
	assert.Equal(t, models.DeploymentStatusDeployed, entries[0].Deployments[0].Status)
	assert.Equal(t, p.GatewayUUID, entries[0].Deployments[0].GatewayID)
	assert.Len(t, listCatalogProviders(t, p.OUID, &env), 1)
}

// A live deployment with a redeploy queued is still listed, and shows DEPLOYING because the redeploy is owed.
func TestCatalogKeepsALiveDeploymentWhileARedeployIsQueued(t *testing.T) {
	p, _ := seedCatalogProvider(t)
	deployment := startDeploy(t, p)
	applyAck(t, p, deployAck(deployment.DeploymentID, models.DeploymentAckStatusSuccess, ""), nil)
	_, err := NewDeploymentRepo(db.GetDB()).QueueRender(context.Background(), nil, p.OUID, p.ArtifactUUID, nil,
		"inputs changed", QueueScopePlaced)
	require.NoError(t, err)

	entries := listCatalogProviders(t, p.OUID, nil)
	require.Len(t, entries[0].Deployments, 1)
	assert.Equal(t, models.DeploymentStatusDeploying, entries[0].Deployments[0].Status)
}

// A failed deploy and an undeployed provider aren't listed anywhere.
func TestCatalogOmitsFailedAndUndeployed(t *testing.T) {
	p, _ := seedCatalogProvider(t)
	deployment := startDeploy(t, p)
	applyAck(t, p, deployAck(deployment.DeploymentID, models.DeploymentAckStatusFailed, "E"), nil)

	entries := listCatalogProviders(t, p.OUID, nil)
	assert.Empty(t, entries[0].Deployments)

	_, err := beginUndeploy(t, p)
	require.NoError(t, err)
	applyAck(t, p, undeployAck(p.ArtifactUUID.String(), models.DeploymentAckStatusSuccess), nil)

	entries = listCatalogProviders(t, p.OUID, nil)
	assert.Empty(t, entries[0].Deployments)
}
