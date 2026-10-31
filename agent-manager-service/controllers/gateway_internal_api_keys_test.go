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

package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

func TestGetAPIKeysServesBothAgentKinds(t *testing.T) {
	gdb := db.GetDB()
	ouID := "ou-" + uuid.NewString()[:8]
	gatewayID, envID, otherEnvID := uuid.New(), uuid.New(), uuid.New()
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id, is_active)
		 VALUES (?, ?, ?, 'gw.example', ?, ?, TRUE)`,
		gatewayID, "keys-gw-"+gatewayID.String()[:8], "keys gateway", models.GatewayRoleBoth, ouID).Error)
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateway_environment_mappings (gateway_uuid, environment_uuid) VALUES (?, ?)`, gatewayID, envID).Error)
	t.Cleanup(func() {
		gdb.Exec(`DELETE FROM artifacts WHERE ou_id = ?`, ouID)
		gdb.Exec(`DELETE FROM gateways WHERE uuid = ?`, gatewayID)
	})

	seedKey := func(kind string, env uuid.UUID) uuid.UUID {
		artifact := &models.Artifact{
			UUID: uuid.New(), Handle: "proj/" + kind + "/" + env.String(), Name: kind + "-" + uuid.NewString()[:8],
			Version: "v1.0", Kind: kind, OUID: ouID,
		}
		require.NoError(t, repositories.NewArtifactRepo(gdb).Create(gdb, artifact))
		require.NoError(t, gdb.Create(&models.StoredAPIKey{
			UUID: uuid.New(), Name: "key", ArtifactUUID: artifact.UUID, OUID: ouID, APIKeyHash: "hash",
			Status: "active", Purpose: models.APIKeyPurposeUserManaged,
		}).Error)
		return artifact.UUID
	}
	agentArtifact := seedKey(models.KindAgent, envID)
	restArtifact := seedKey(models.KindRestAPI, envID)
	seedKey(models.KindAgent, otherEnvID)
	seedKey(models.KindRestAPI, otherEnvID)

	controller := &gatewayInternalController{
		gatewayService: services.NewPlatformGatewayService(repositories.NewGatewayRepo(gdb), nil),
		apiKeyRepo:     repositories.NewAPIKeyRepo(gdb),
	}
	recorder := httptest.NewRecorder()
	controller.getAgentAPIKeys(recorder, httptest.NewRequest(http.MethodGet, "/", nil), gatewayIdentity{ID: gatewayID.String(), OUID: ouID})

	require.Equal(t, http.StatusOK, recorder.Code)
	var keys []controlPlaneAPIKeyResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &keys))
	served := make([]string, 0, len(keys))
	for _, key := range keys {
		served = append(served, key.ArtifactUUID)
	}
	assert.ElementsMatch(t, []string{agentArtifact.String(), restArtifact.String()}, served)
}
