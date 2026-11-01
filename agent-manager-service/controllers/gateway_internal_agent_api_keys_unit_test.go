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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

func TestListSentA2AAgentAPIKeysServesTheGatewaysAgentKeys(t *testing.T) {
	artifactID := uuid.New()
	var gotOU, gotGateway, gotKind string
	repo := &repomocks.APIKeyRepositoryMock{
		ListSentToGatewayFunc: func(_ context.Context, ouID, gatewayUUID, kind string) ([]models.StoredAPIKey, error) {
			gotOU, gotGateway, gotKind = ouID, gatewayUUID, kind
			return []models.StoredAPIKey{{UUID: uuid.New(), Name: "k1", ArtifactUUID: artifactID, APIKeyHash: "abc", Status: "active"}}, nil
		},
	}
	controller := &gatewayInternalController{apiKeyRepo: repo}
	recorder := httptest.NewRecorder()

	controller.listSentA2AAgentAPIKeys(recorder, httptest.NewRequest(http.MethodGet, "/", nil), gatewayIdentity{ID: "gw-1", OUID: "ou-1"})

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "ou-1", gotOU)
	assert.Equal(t, "gw-1", gotGateway)
	assert.Equal(t, models.KindAgent, gotKind, "REST agents are RestApi artifacts and must stay out")
	var keys []controlPlaneAPIKeyResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &keys))
	require.Len(t, keys, 1)
	assert.Equal(t, "k1", keys[0].Name)
	assert.Equal(t, artifactID.String(), keys[0].ArtifactUUID)
	assert.Equal(t, map[string]string{"sha256": "abc"}, keys[0].APIKeyHashes)
}

func TestListSentA2AAgentAPIKeysServesAnEmptyListAsAnArray(t *testing.T) {
	repo := &repomocks.APIKeyRepositoryMock{
		ListSentToGatewayFunc: func(context.Context, string, string, string) ([]models.StoredAPIKey, error) {
			return nil, nil
		},
	}
	recorder := httptest.NewRecorder()

	(&gatewayInternalController{apiKeyRepo: repo}).listSentA2AAgentAPIKeys(
		recorder, httptest.NewRequest(http.MethodGet, "/", nil), gatewayIdentity{ID: "gw-1", OUID: "ou-1"})

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `[]`, recorder.Body.String())
}

func TestListSentA2AAgentAPIKeysFailsRatherThanServeAnEmptyList(t *testing.T) {
	repo := &repomocks.APIKeyRepositoryMock{
		ListSentToGatewayFunc: func(context.Context, string, string, string) ([]models.StoredAPIKey, error) {
			return nil, errors.New("db down")
		},
	}
	recorder := httptest.NewRecorder()

	(&gatewayInternalController{apiKeyRepo: repo}).listSentA2AAgentAPIKeys(
		recorder, httptest.NewRequest(http.MethodGet, "/", nil), gatewayIdentity{ID: "gw-1", OUID: "ou-1"})

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
}
