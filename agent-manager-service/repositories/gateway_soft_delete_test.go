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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// A soft delete does not fire the FK cascades a hard delete relied on, so Delete has
// to do their work: the gateway's token stops authenticating, its archived deployment
// history goes, and its identity providers stop being offered for its environment.
func TestGatewayDelete_SoftDeleteCleansUpWhatTheCascadeUsedTo(t *testing.T) {
	gdb := db.GetDB()
	repo := NewGatewayRepo(gdb)
	ouID := "ou-" + uuid.New().String()[:8]
	gatewayID, envID, artifactUUID := uuid.New(), uuid.New(), uuid.New()
	name := "soft-gw-" + gatewayID.String()[:8]

	require.NoError(t, gdb.Exec(
		`INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, TRUE)`,
		gatewayID, name, name, "gw.example", models.GatewayRoleIngress, ouID,
	).Error)
	t.Cleanup(func() {
		gdb.Exec(`DELETE FROM artifacts WHERE uuid = ?`, artifactUUID)
		gdb.Exec(`DELETE FROM gateways WHERE ou_id = ?`, ouID)
	})
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateway_tokens (uuid, gateway_uuid, token_prefix, token_hash, salt, status)
		 VALUES (?, ?, ?, 'hash', 'salt', 'active')`,
		uuid.New(), gatewayID, gatewayID.String(),
	).Error)
	require.NoError(t, gdb.Create(&models.GatewayEnvironmentMapping{GatewayUUID: gatewayID, EnvironmentUUID: envID}).Error)
	require.NoError(t, repo.UpsertIdentityProvider(&models.GatewayIdentityProvider{
		UUID: uuid.New(), GatewayUUID: gatewayID, Name: "customer-idp", Issuer: "https://idp.example",
		JWKSUri: "https://idp.example/jwks", Type: models.IdentityProviderTypeCustom,
	}))

	// Archived history only: DeleteGateway refuses a gateway with deployment_status rows.
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
	require.NoError(t, deployments.DeleteStatus(artifactUUID.String(), ouID, gatewayID.String()))

	require.NoError(t, repo.Delete(gatewayID.String(), ouID))

	var deletedAtSet bool
	require.NoError(t, gdb.Raw(`SELECT deleted_at IS NOT NULL FROM gateways WHERE uuid = ?`, gatewayID).
		Scan(&deletedAtSet).Error)
	assert.True(t, deletedAtSet, "the gateway row must be kept and marked deleted, not removed")

	var activeTokens int64
	require.NoError(t, gdb.Model(&models.GatewayToken{}).
		Where("gateway_uuid = ? AND status = ?", gatewayID, "active").Count(&activeTokens).Error)
	assert.Zero(t, activeTokens, "a deleted gateway's token must no longer authenticate")

	history, err := deployments.GetDeploymentsWithState(artifactUUID.String(), ouID, nil, nil, 10)
	require.NoError(t, err)
	assert.Empty(t, history, "archived deployments on a deleted gateway must not be listed")

	envProviders, err := repo.ListIdentityProvidersByEnvironment(envID.String())
	require.NoError(t, err)
	assert.Empty(t, envProviders)
	orgProviders, err := repo.ListIdentityProvidersByOrg(ouID)
	require.NoError(t, err)
	assert.Empty(t, orgProviders)

	// The name is free again: the unique index only covers live gateways.
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, TRUE)`,
		uuid.New(), name, name, "gw.example", models.GatewayRoleIngress, ouID,
	).Error)
}
