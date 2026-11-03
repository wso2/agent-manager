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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// Exercise the real service through the handler: both layers must map a wrapped
// database collision, and a failed update must stop before any gateway sync.
func TestUpdateLLMProvider_UpdateErrors(t *testing.T) {
	duplicateName := &pgconn.PgError{Code: "23505", ConstraintName: "uq_artifact_name_version_ou_id"}
	tests := []struct {
		name    string
		err     error
		status  int
		code    string
		message string
	}{
		{
			name: "duplicate name and version", err: duplicateName,
			status: http.StatusConflict, code: "CONFLICT",
			message: "Another LLM provider already uses this name and version",
		},
		{
			name:   "wrapped duplicate name and version",
			err:    fmt.Errorf("failed to update artifact name: %w", duplicateName),
			status: http.StatusConflict, code: "CONFLICT",
			message: "Another LLM provider already uses this name and version",
		},
		{
			name:   "unrelated unique constraint",
			err:    &pgconn.PgError{Code: "23505", ConstraintName: "another_constraint"},
			status: http.StatusInternalServerError, code: "INTERNAL_ERROR", message: "Failed to update LLM provider",
		},
		{
			name:   "other database error",
			err:    &pgconn.PgError{Code: "23503", ConstraintName: "uq_artifact_name_version_ou_id"},
			status: http.StatusInternalServerError, code: "INTERNAL_ERROR", message: "Failed to update LLM provider",
		},
		{
			name: "unexpected error", err: errors.New("database unavailable"),
			status: http.StatusInternalServerError, code: "INTERNAL_ERROR", message: "Failed to update LLM provider",
		},
		{
			name: "provider removed concurrently", err: gorm.ErrRecordNotFound,
			status: http.StatusNotFound, code: "NOT_FOUND", message: "LLM provider not found",
		},
	}
	for _, tt := range tests {
		for _, gateways := range []struct {
			name string
			json string
		}{
			{name: "without gateway sync", json: ""},
			{name: "empty gateway sync", json: `,"gateways":[]`},
			{name: "with gateway sync", json: `,"gateways":["00000000-0000-0000-0000-000000000001"]`},
		} {
			t.Run(tt.name+"/"+gateways.name, func(t *testing.T) {
				providerUUID := uuid.New()
				const ouID = "ou-acme"
				existing := &models.LLMProvider{
					UUID: providerUUID,
					Artifact: &models.Artifact{
						UUID: providerUUID, OUID: ouID, Handle: "provider-b", Name: "Provider B", Version: "v1.0",
					},
					Configuration: models.LLMProviderConfig{Name: "Provider B", Version: "v1.0"},
				}
				repo := &repomocks.LLMProviderRepositoryMock{
					GetByUUIDFunc: func(id, org string) (*models.LLMProvider, error) {
						require.Equal(t, providerUUID.String(), id)
						require.Equal(t, ouID, org)
						return existing, nil
					},
					UpdateFunc: func(ctx context.Context, updates *models.LLMProvider, id, org string) error {
						orgContext, ok := middleware.GetResolvedOrg(ctx)
						require.True(t, ok)
						require.Equal(t, ouID, orgContext.OUID)
						require.Equal(t, providerUUID.String(), id)
						require.Equal(t, ouID, org)
						require.Equal(t, "Provider A", updates.Configuration.Name)
						require.Equal(t, "v1.0", updates.Configuration.Version)
						return tt.err
					},
				}
				svc := services.NewLLMProviderService(nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
				// A nil deployment service deliberately makes any attempted gateway I/O fail.
				ctrl := NewLLMController(nil, svc, nil, nil, nil, nil, nil)
				req := httptest.NewRequest(http.MethodPut, "/orgs/acme/llm-providers/"+providerUUID.String(),
					strings.NewReader(`{"name":"Provider A"`+gateways.json+`}`))
				req.SetPathValue(utils.PathParamProviderId, providerUUID.String())
				req = req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: ouID}))
				recorder := httptest.NewRecorder()

				ctrl.UpdateLLMProvider(recorder, req)

				require.Equal(t, tt.status, recorder.Code, recorder.Body.String())
				var response struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				}
				require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
				require.Equal(t, tt.code, response.Code)
				require.Equal(t, tt.message, response.Message)
				require.Len(t, repo.UpdateCalls(), 1)
				require.Len(t, repo.GetByUUIDCalls(), 2, "must not refetch after a failed update")
			})
		}
	}
}
