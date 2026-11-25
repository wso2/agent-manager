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

package gwctl

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// A catalog change keeps the written provider, so the response still carries its model providers.
func TestApplyCatalogKeepsTheWrittenProvider(t *testing.T) {
	gdb := db.GetDB()
	ouID := dbtest.Org(t, gdb)
	secrets := gwtest.NewSecrets()
	providerRepo := gwrepo.NewLLMProviderRepo(gdb, secrets)
	handle := "catalog-" + uuid.NewString()[:8]
	stored := &gwmodels.LLMProvider{
		TemplateHandle: "openai",
		ModelList:      `[{"id":"openai","models":[{"id":"gpt-4o"}]}]`,
		Configuration:  gwmodels.LLMProviderConfig{Name: handle, Handle: handle, Version: "v1.0", Template: "openai"},
		Status:         "CREATED",
	}
	require.NoError(t, providerRepo.Create(gdb, stored, handle, handle, "v1.0", ouID))

	svc := gwsvc.NewLLMProviderService(gdb, providerRepo, nil, nil, nil, gwrepo.NewArtifactRepo(gdb, secrets), nil, nil, nil, nil)
	written, err := svc.Get(stored.UUID.String(), ouID)
	require.NoError(t, err)
	require.NotEmpty(t, written.ModelProviders)
	require.True(t, written.Artifact.InCatalog)

	ctrl := &llmController{providerService: svc}
	inCatalog := false
	got, err := ctrl.applyCatalog(context.Background(), "test", ouID, written, &inCatalog)
	require.NoError(t, err)

	require.Equal(t, written.ModelProviders, got.ModelProviders)
	require.False(t, got.InCatalog)
	require.False(t, got.Artifact.InCatalog)
	reread, err := svc.Get(stored.UUID.String(), ouID)
	require.NoError(t, err)
	require.False(t, reread.Artifact.InCatalog, "the catalog change must be stored")
}

// A failed catalog change is returned, so the handler can fail the request.
func TestApplyCatalogReturnsTheFailure(t *testing.T) {
	gdb := db.GetDB()
	ouID := dbtest.Org(t, gdb)
	secrets := gwtest.NewSecrets()
	svc := gwsvc.NewLLMProviderService(gdb, gwrepo.NewLLMProviderRepo(gdb, secrets), nil, nil, nil, gwrepo.NewArtifactRepo(gdb, secrets), nil, nil, nil, nil)
	missing := uuid.New()
	provider := &gwmodels.LLMProvider{UUID: missing, Artifact: &gwmodels.Artifact{UUID: missing, InCatalog: true}}

	ctrl := &llmController{providerService: svc}
	inCatalog := false
	got, err := ctrl.applyCatalog(context.Background(), "test", ouID, provider, &inCatalog)

	require.Error(t, err)
	require.True(t, got.Artifact.InCatalog, "a failed change must not be reported as applied")
}
