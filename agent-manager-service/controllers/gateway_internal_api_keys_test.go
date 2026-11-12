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
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

func TestGetAPIKeysServesBothAgentKinds(t *testing.T) {
	gdb := db.GetDB()
	ouID := dbtest.Org(t, db.GetDB())
	envID, otherEnvID := uuid.New(), uuid.New()
	gatewayID := dbtest.GatewayIn(t, gdb, ouID, models.GatewayRoleBoth, envID).UUID
	dbtest.EnvironmentWithID(t, gdb, ouID, otherEnvID)

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

// pullFixture is an authenticated gateway holding one deployed RestApi artifact.
type pullFixture struct {
	controller *gatewayInternalController
	token      string
	artifact   uuid.UUID
}

const restAPIYAML = "kind: RestApi\nmetadata:\n  name: pulled\n"

func newPullFixture(t *testing.T) pullFixture {
	t.Helper()
	gdb := db.GetDB()
	ouID := dbtest.Org(t, db.GetDB())
	gatewayID, artifactID := dbtest.GatewayIn(t, gdb, ouID, models.GatewayRoleBoth).UUID, uuid.New()
	handle := "handle-" + artifactID.String()[:8]
	require.NoError(t, repositories.NewArtifactRepo(gdb).Create(gdb, &models.Artifact{
		UUID: artifactID, Handle: handle, Name: handle, Version: "v1.0", Kind: models.KindRestAPI, OUID: ouID,
	}))
	placement := models.Placement{
		ArtifactRef: models.ArtifactRef{OUID: ouID, ArtifactUUID: artifactID, Kind: models.KindRestAPI, Handle: handle},
		GatewayUUID: gatewayID,
	}
	deploymentRepo := repositories.NewDeploymentRepo(gdb)
	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		return deploymentRepo.StartDeploy(context.Background(), tx, placement, &models.Deployment{
			Name: "deployment", ArtifactUUID: artifactID, OUID: ouID, GatewayUUID: gatewayID, Content: []byte(restAPIYAML),
		})
	}))

	gatewayRepo := repositories.NewGatewayRepo(gdb)
	gatewayService := services.NewPlatformGatewayService(gatewayRepo, nil)
	token, err := gatewayService.RotateToken(gatewayID.String(), ouID)
	require.NoError(t, err)
	return pullFixture{
		controller: &gatewayInternalController{
			gatewayService: gatewayService,
			gatewayInternalService: services.NewGatewayInternalAPIService(
				nil, nil, deploymentRepo, nil, gatewayRepo, nil, nil, nil,
			),
		},
		token:    token.Token,
		artifact: artifactID,
	}
}

func (f pullFixture) getAPI(id string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/apis/"+id, nil)
	request.SetPathValue("apiId", id)
	request.Header.Set("api-key", f.token)
	recorder := httptest.NewRecorder()
	f.controller.GetAPI(recorder, request)
	return recorder
}

func TestGetAPIZip(t *testing.T) {
	f := newPullFixture(t)

	recorder := f.getAPI(f.artifact.String())

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "application/zip", recorder.Header().Get("Content-Type"))
	body := recorder.Body.Bytes()
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	require.NoError(t, err)
	require.Len(t, archive.File, 1)
	assert.Equal(t, "api-"+f.artifact.String()+".yaml", archive.File[0].Name)
	entry, err := archive.File[0].Open()
	require.NoError(t, err)
	defer entry.Close()
	content, err := io.ReadAll(entry)
	require.NoError(t, err)
	assert.Equal(t, restAPIYAML, string(content))
}

func TestGetAPINotFound(t *testing.T) {
	f := newPullFixture(t)

	assert.Equal(t, http.StatusNotFound, f.getAPI(uuid.NewString()).Code)
}
