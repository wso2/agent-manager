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

package repositories

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func TestDeploymentHistoryCursorSurvivesArchivedDeploymentDeletion(t *testing.T) {
	ctx := t.Context()
	tx := db.GetDB().WithContext(ctx).Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { tx.Rollback() })

	ouID := "ou-" + uuid.NewString()
	artifactID := uuid.New()
	archiveGateway, activeGateway := uuid.New(), uuid.New()
	archiveGateway[0], activeGateway[0] = 0, 1
	for _, id := range []uuid.UUID{archiveGateway, activeGateway} {
		require.NoError(t, tx.Exec(
			`INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id, is_active)
			 VALUES (?, ?, ?, 'gw.example', ?, ?, TRUE)`,
			id, id.String(), id.String(), models.GatewayRoleIngress, ouID,
		).Error)
	}
	require.NoError(t, NewArtifactRepo(tx).Create(tx, &models.Artifact{
		UUID: artifactID, Handle: artifactID.String(), Name: "test-provider",
		Version: "v1.0", Kind: models.KindLLMProvider, OUID: ouID,
	}))

	firstArchived, lastArchived, active := uuid.New(), uuid.New(), uuid.New()
	firstArchived[0], lastArchived[0] = 1, 0
	createdAt := time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)
	for _, row := range []struct {
		id      uuid.UUID
		gateway uuid.UUID
	}{{firstArchived, archiveGateway}, {lastArchived, archiveGateway}, {active, activeGateway}} {
		require.NoError(t, tx.Exec(
			`INSERT INTO deployments (deployment_id, name, artifact_uuid, ou_id, gateway_uuid, content, created_at)
			 VALUES (?, 'test-deployment', ?, ?, ?, ?, ?)`,
			row.id, artifactID, ouID, row.gateway, []byte("kind: LlmProvider\n"), createdAt,
		).Error)
	}
	require.NoError(t, tx.Exec(
		`INSERT INTO deployment_status (artifact_uuid, ou_id, gateway_uuid, deployment_id, status, performed_at)
		 VALUES (?, ?, ?, ?, 'DEPLOYED', now())`, artifactID, ouID, activeGateway, active,
	).Error)

	repo := NewDeploymentRepo(tx)
	firstPage, err := repo.GetDeploymentsWithStatePage(ctx, artifactID.String(), ouID, nil, nil, 20, nil, 2)
	require.NoError(t, err)
	require.Len(t, firstPage, 2)
	require.Equal(t, firstArchived, firstPage[0].DeploymentID)
	last := firstPage[1]
	require.Equal(t, lastArchived, last.DeploymentID)

	// Remove the exact row represented by the cursor before requesting page two.
	require.NoError(t, repo.Delete(last.DeploymentID.String(), artifactID.String(), ouID))
	cursor := &models.DeploymentPageCursor{
		GatewayUUID: last.GatewayUUID, CreatedAt: last.CreatedAt, DeploymentID: last.DeploymentID,
	}
	secondPage, err := repo.GetDeploymentsWithStatePage(ctx, artifactID.String(), ouID, nil, nil, 20, cursor, 2)
	require.NoError(t, err)
	require.Len(t, secondPage, 1)
	require.Equal(t, active, secondPage[0].DeploymentID)
	require.NotNil(t, secondPage[0].Status)
	require.Equal(t, models.DeploymentStatusDeployed, *secondPage[0].Status)
}
