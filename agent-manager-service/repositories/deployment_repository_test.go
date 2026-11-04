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

// seedAgentDeployment writes a gateway, an agent artifact and one deployment of it.
func seedAgentDeployment(t *testing.T) (artifactUUID uuid.UUID, ouID string) {
	t.Helper()
	gdb := db.GetDB()
	ouID = "ou-" + uuid.New().String()[:8]
	gatewayID, artifactUUID := uuid.New(), uuid.New()
	name := "dep-gw-" + gatewayID.String()[:8]
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, TRUE)`,
		gatewayID, name, name, "gw.example", models.GatewayRoleIngress, ouID).Error)
	require.NoError(t, NewArtifactRepo(gdb).Create(gdb, &models.Artifact{
		UUID: artifactUUID, Handle: "proj/agent/" + artifactUUID.String(), Name: "agent-" + artifactUUID.String()[:8],
		Version: "v1.0", Kind: models.KindAgent, OUID: ouID,
	}))
	placement := models.Placement{
		ArtifactRef: models.ArtifactRef{OUID: ouID, ArtifactUUID: artifactUUID, Kind: models.KindAgent},
		GatewayUUID: gatewayID,
	}
	deployment := &models.Deployment{Name: "agent-deployment", Content: []byte("kind: Agent\n")}
	repo := NewDeploymentRepo(gdb)
	require.NoError(t, repo.StartDeploy(context.Background(), nil, placement, deployment))
	_, err := repo.ApplyAck(context.Background(), gatewayID.String(), models.DeploymentAck{
		DeploymentID: deployment.DeploymentID.String(), Action: models.DeploymentAckActionDeploy,
		Status: models.DeploymentAckStatusSuccess,
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		gdb.Exec(`DELETE FROM artifacts WHERE uuid = ?`, artifactUUID)
		gdb.Exec(`DELETE FROM gateways WHERE uuid = ?`, gatewayID)
	})
	return artifactUUID, ouID
}

// CreateWithLimitEnforcement's status row must satisfy migration 051's CHECKs.
func TestCreateWithLimitEnforcementRecordsDesiredStateAndPerformedAt(t *testing.T) {
	artifactUUID, ouID := seedAgentDeployment(t)
	gdb := db.GetDB()
	var row models.DeploymentStatusRecord
	require.NoError(t, gdb.Where("artifact_uuid = ? AND ou_id = ?", artifactUUID, ouID).First(&row).Error)
	assert.Equal(t, models.DeploymentStatusDeployed, row.StatusDesired)
	assert.Equal(t, models.NewPerformedAt(row.PerformedAt), row.PerformedAt.UTC(), "performed_at is in milliseconds")
	require.NotNil(t, row.DeploymentID)
}
