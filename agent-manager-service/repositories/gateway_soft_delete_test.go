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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// Delete removes the gateway outright, and with it what only it used: its token stops authenticating, its archived
// deployment history goes, and its identity providers stop being offered for its environment.
func TestGatewayDeleteRemovesTheGatewayAndWhatOnlyItUsed(t *testing.T) {
	gdb := db.GetDB()
	repo := NewGatewayRepo(gdb)
	ouID := newOrg(t)
	gatewayID, artifactUUID := newGateway(t, ouID, models.GatewayRoleIngress), uuid.New()
	envID := dbtest.Environment(t, gdb, ouID)
	var gw models.Gateway
	require.NoError(t, gdb.Where("id = ?", gatewayID).Take(&gw).Error)

	require.NoError(t, gdb.Create(&models.GatewayToken{
		UUID: uuid.New(), OrgID: uuid.MustParse(ouID), GatewayUUID: gatewayID, TokenPrefix: gatewayID.String()[:8],
		TokenHash: "hash", Salt: "salt", Status: "active", CreatedAt: time.Now().UTC(),
	}).Error)
	require.NoError(t, repo.UpsertIdentityProvider(&models.GatewayIdentityProvider{
		UUID: uuid.New(), GatewayUUID: gatewayID, Name: "customer-idp", Issuer: "https://idp.example",
		JWKSUri: "https://idp.example/jwks", Type: models.IdentityProviderTypeCustom,
	}))

	// Archived history only: DeleteGateway refuses a gateway with status rows.
	require.NoError(t, NewArtifactRepo(gdb).Create(gdb, &models.Artifact{
		UUID: artifactUUID, Handle: "proj/agent/" + artifactUUID.String(), Name: "agent-" + artifactUUID.String()[:8],
		Version: "v1.0", Kind: models.KindAgent, OUID: ouID,
	}))
	deployments := NewDeploymentRepo(gdb)
	deployed := models.DeploymentStatusDeployed
	require.NoError(t, deployments.CreateWithLimitEnforcement(&models.Deployment{
		Name: "agent-deployment", ArtifactUUID: artifactUUID, OUID: ouID, GatewayUUID: gatewayID,
		Content: []byte("kind: Agent\n"), Status: &deployed,
	}, 10))
	require.NoError(t, gdb.Where(statusRowWhere, artifactUUID, ouID, gatewayID).
		Delete(&models.DeploymentStatusRecord{}).Error)

	require.NoError(t, repo.Delete(gatewayID.String(), ouID))

	var gateways, tokens int64
	require.NoError(t, gdb.Model(&models.Gateway{}).Where("id = ?", gatewayID).Count(&gateways).Error)
	assert.Zero(t, gateways, "the gateway row is removed")
	require.NoError(t, gdb.Model(&models.GatewayToken{}).Where("gateway_id = ?", gatewayID).Count(&tokens).Error)
	assert.Zero(t, tokens, "a deleted gateway's token must no longer authenticate")

	history, err := deployments.GetDeploymentsWithState(artifactUUID.String(), ouID, nil, nil, 10)
	require.NoError(t, err)
	assert.Empty(t, history, "archived deployments on a deleted gateway must not be listed")

	envProviders, err := repo.ListIdentityProvidersByEnvironment(envID.String())
	require.NoError(t, err)
	assert.Empty(t, envProviders)
	orgProviders, err := repo.ListIdentityProvidersByOrg(ouID)
	require.NoError(t, err)
	assert.Empty(t, orgProviders)

	// The name is free again.
	reused := gw
	reused.UUID = uuid.New()
	require.NoError(t, gdb.Create(&reused).Error)
}
