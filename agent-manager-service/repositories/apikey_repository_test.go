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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// seedArtifactOnGateway adds an artifact of kind to the gateway's org with one key, and returns the key's name.
func seedArtifactOnGateway(t *testing.T, p models.Placement, kind string) (models.Placement, string) {
	t.Helper()
	gdb := db.GetDB()
	artifactUUID := uuid.New()
	handle := "handle-" + artifactUUID.String()[:8]
	require.NoError(t, NewArtifactRepo(gdb).Create(gdb, &models.Artifact{
		UUID: artifactUUID, Handle: handle, Name: handle, Version: "v1.0", Kind: kind, OUID: p.OUID,
	}))
	t.Cleanup(func() { gdb.Exec(`DELETE FROM artifacts WHERE uuid = ?`, artifactUUID) })
	other := p
	other.ArtifactRef = models.ArtifactRef{OUID: p.OUID, ArtifactUUID: artifactUUID, Kind: kind, Handle: handle}
	return other, seedKey(t, other)
}

func seedKey(t *testing.T, p models.Placement) string {
	t.Helper()
	name := "key-" + uuid.NewString()[:8]
	require.NoError(t, NewAPIKeyRepo(db.GetDB()).Upsert(&models.StoredAPIKey{
		UUID: uuid.New(), Name: name, ArtifactUUID: p.ArtifactUUID, OUID: p.OUID, APIKeyHash: "hash",
		Status: "active", Purpose: models.APIKeyPurposeUserManaged, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}))
	return name
}

func sentKeyNames(t *testing.T, p models.Placement, ouID, kind string) []string {
	t.Helper()
	keys, err := NewAPIKeyRepo(db.GetDB()).ListSentToGateway(context.Background(), ouID, p.GatewayUUID.String(), kind)
	require.NoError(t, err)
	names := make([]string, 0, len(keys))
	for _, k := range keys {
		names = append(names, k.Name)
	}
	return names
}

func TestListSentToGatewayListsDeployedAndUndeployedAgents(t *testing.T) {
	deployed := seedPlacement(t, models.KindAgent)
	deployedKey := seedKey(t, deployed)
	startDeploy(t, deployed)
	undeployed, undeployedKey := seedArtifactOnGateway(t, deployed, models.KindAgent)
	startDeploy(t, undeployed)
	_, err := beginUndeploy(t, undeployed)
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{deployedKey, undeployedKey}, sentKeyNames(t, deployed, deployed.OUID, models.KindAgent))
}

func TestListSentToGatewayExcludesPlacedButUnsentAgents(t *testing.T) {
	placed := seedPlacement(t, models.KindAgent)
	seedKey(t, placed)
	insertPendingRow(t, placed)

	assert.Empty(t, sentKeyNames(t, placed, placed.OUID, models.KindAgent))
}

func TestListSentToGatewayExcludesAgentsSentToAnotherGateway(t *testing.T) {
	sent := seedPlacement(t, models.KindAgent)
	seedKey(t, sent)
	startDeploy(t, sent)
	otherGateway := seedPlacement(t, models.KindAgent)
	queried := sent
	queried.GatewayUUID = otherGateway.GatewayUUID

	assert.Empty(t, sentKeyNames(t, queried, sent.OUID, models.KindAgent))
}

func TestListSentToGatewayIsScopedToTheOrg(t *testing.T) {
	sent := seedPlacement(t, models.KindAgent)
	seedKey(t, sent)
	startDeploy(t, sent)

	assert.Empty(t, sentKeyNames(t, sent, "ou-"+uuid.NewString()[:8], models.KindAgent))
}

func TestListSentToGatewayIgnoresOtherKinds(t *testing.T) {
	restAgent := seedPlacement(t, models.KindRestAPI)
	seedKey(t, restAgent)
	startDeploy(t, restAgent)

	assert.Empty(t, sentKeyNames(t, restAgent, restAgent.OUID, models.KindAgent))
	assert.Len(t, sentKeyNames(t, restAgent, restAgent.OUID, models.KindRestAPI), 1)
}
