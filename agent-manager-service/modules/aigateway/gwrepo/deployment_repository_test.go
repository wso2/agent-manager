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

package gwrepo

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// seedAgentDeployment writes a gateway, an agent artifact and one deployment of it.
func seedAgentDeployment(t *testing.T) (artifactUUID uuid.UUID, ouID string) {
	t.Helper()
	gdb := db.GetDB()
	ouID = newOrg(t)
	gatewayID, artifactUUID := newGateway(t, ouID, gwmodels.GatewayRoleIngress), uuid.New()
	require.NoError(t, NewArtifactRepo(gdb).Create(gdb, &gwmodels.Artifact{
		UUID: artifactUUID, Handle: "proj/agent/" + artifactUUID.String(), Name: "agent-" + artifactUUID.String()[:8],
		Version: "v1.0", Kind: gwmodels.KindAgent, OUID: ouID,
	}))
	placement := gwmodels.Placement{
		ArtifactRef: gwmodels.ArtifactRef{OUID: ouID, ArtifactUUID: artifactUUID, Kind: gwmodels.KindAgent},
		GatewayUUID: gatewayID,
	}
	deployment := &gwmodels.Deployment{Name: "agent-deployment", Content: []byte("kind: Agent\n")}
	repo := NewDeploymentRepo(gdb)
	require.NoError(t, repo.StartDeploy(context.Background(), nil, placement, deployment))
	_, err := repo.ApplyAck(context.Background(), gatewayID.String(), gwmodels.DeploymentAck{
		DeploymentID: deployment.DeploymentID.String(), Action: gwmodels.DeploymentAckActionDeploy,
		Status: gwmodels.DeploymentAckStatusSuccess,
	}, nil)
	require.NoError(t, err)
	return artifactUUID, ouID
}

// CreateWithLimitEnforcement's status row must satisfy migration 051's CHECKs.
func TestCreateWithLimitEnforcementRecordsDesiredStateAndPerformedAt(t *testing.T) {
	artifactUUID, ouID := seedAgentDeployment(t)
	gdb := db.GetDB()
	var row gwmodels.DeploymentStatusRecord
	require.NoError(t, gdb.Where("artifact_id = ? AND org_id = ?", artifactUUID, ouID).First(&row).Error)
	assert.Equal(t, gwmodels.DeploymentStatusDeployed, row.StatusDesired)
	assert.Equal(t, gwmodels.NewPerformedAt(row.PerformedAt), row.PerformedAt.UTC(), "performed_at is in milliseconds")
	require.NotNil(t, row.DeploymentID)
}
