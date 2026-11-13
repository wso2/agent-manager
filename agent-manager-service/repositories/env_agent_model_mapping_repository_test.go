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

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// TestEnvAgentModelMappingRepo_BackfillsLLMProxyHandle guards against a
// regression where deleting an agent's LLM model config failed with
// "failed to get deployments for proxy \"\": failed to get proxy: record
// not found". LLMProxy.Handle is gorm:"-" (derived, not a DB column), so
// GORM's Preload("LLMProxy") always leaves it blank; the delete flow reads
// mapping.LLMProxy.Handle straight off the ListByConfig/GetByConfigAndEnv
// result to look up proxy deployments to clean up. Without backfilling it
// from Configuration.Name (as agent_configuration_repository.go's
// GetByUUID/GetByAgentID already do via backfillLLMProxyHandles), the
// lookup always resolves an empty handle and the whole delete aborts.
func TestEnvAgentModelMappingRepo_BackfillsLLMProxyHandle(t *testing.T) {
	gdb := db.GetDB()
	ctx := context.Background()

	const proxyName = "backfill-handle-proxy-name"
	ou := newOrg(t)
	_, proxyUUID := seedLLMProxy(t, ou)
	require.NoError(t, gdb.Model(&models.LLMProxy{}).Where("id = ?", proxyUUID).
		Update("configuration", dbx.JSON(map[string]string{"name": proxyName})).Error)
	configUUID, envUUID := mapLLMProxyToAgent(t, ou, proxyUUID)
	config := &models.AgentConfiguration{UUID: configUUID}

	repo := NewEnvAgentModelMappingRepository(gdb)

	t.Run("ListByConfig backfills Handle", func(t *testing.T) {
		mappings, err := repo.ListByConfig(ctx, config.UUID)
		require.NoError(t, err)
		require.Len(t, mappings, 1)
		require.NotNil(t, mappings[0].LLMProxy)
		require.Equal(t, proxyName, mappings[0].LLMProxy.Handle,
			"ListByConfig must backfill LLMProxy.Handle so delete cleanup can resolve the proxy")
	})

	t.Run("GetByConfigAndEnv backfills Handle", func(t *testing.T) {
		got, err := repo.GetByConfigAndEnv(ctx, config.UUID, envUUID)
		require.NoError(t, err)
		require.NotNil(t, got.LLMProxy)
		require.Equal(t, proxyName, got.LLMProxy.Handle,
			"GetByConfigAndEnv must backfill LLMProxy.Handle so delete cleanup can resolve the proxy")
	})
}
