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
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// seedLLMProxyArtifact writes an LLM proxy artifact and its llm_proxies row in ouID, and returns its handle and UUID.
// The proxy row's provider and the mapping rows' agent configuration and monitor aren't seeded, so the inserts run
// with FK triggers off.
func seedLLMProxyArtifact(t *testing.T, ouID string) (string, uuid.UUID) {
	t.Helper()
	gdb := db.GetDB()
	proxyUUID := uuid.New()
	handle := "proxy-" + proxyUUID.String()[:8]
	require.NoError(t, NewArtifactRepo(gdb).Create(gdb, &models.Artifact{
		UUID: proxyUUID, Handle: handle, Name: handle, Version: "v1.0", Kind: models.KindLLMProxy, OUID: ouID,
	}))
	execWithoutFKs(t, `INSERT INTO llm_proxies (uuid, project_uuid, provider_uuid, status, configuration)
		VALUES (?, ?, ?, 'deployed', '{}')`, proxyUUID, uuid.New(), uuid.New())
	t.Cleanup(func() {
		gdb.Exec(`DELETE FROM env_agent_model_mapping WHERE llm_proxy_uuid = ?`, proxyUUID)
		gdb.Exec(`DELETE FROM monitor_llm_mapping WHERE llm_proxy_uuid = ?`, proxyUUID)
		gdb.Exec(`DELETE FROM llm_proxies WHERE uuid = ?`, proxyUUID)
		gdb.Exec(`DELETE FROM artifacts WHERE uuid = ?`, proxyUUID)
	})
	return handle, proxyUUID
}

// execWithoutFKs runs one statement with FK triggers off, restoring them before the pooled connection is reused.
func execWithoutFKs(t *testing.T, sql string, args ...any) {
	t.Helper()
	require.NoError(t, db.GetDB().Connection(func(tx *gorm.DB) error {
		if err := tx.Exec("SET session_replication_role = 'replica'").Error; err != nil {
			return err
		}
		defer func() { _ = tx.Exec("SET session_replication_role = 'origin'").Error }()
		return tx.Exec(sql, args...).Error
	}))
}

func TestIsAgentOrMonitorProxy(t *testing.T) {
	repo := NewLLMProxyRepo(db.GetDB())
	ctx := context.Background()
	ouID := "ou-" + uuid.New().String()[:8]

	agentProxy, agentProxyUUID := seedLLMProxyArtifact(t, ouID)
	execWithoutFKs(t, `INSERT INTO env_agent_model_mapping (config_uuid, environment_uuid, llm_proxy_uuid)
		VALUES (?, ?, ?)`, uuid.New(), uuid.New(), agentProxyUUID)
	monitorProxy, monitorProxyUUID := seedLLMProxyArtifact(t, ouID)
	execWithoutFKs(t, `INSERT INTO monitor_llm_mapping (monitor_id, llm_proxy_uuid) VALUES (?, ?)`,
		uuid.New(), monitorProxyUUID)
	userProxy, _ := seedLLMProxyArtifact(t, ouID)

	tests := []struct {
		name   string
		handle string
		ouID   string
		want   bool
	}{
		{name: "agent LLM config proxy", handle: agentProxy, ouID: ouID, want: true},
		{name: "monitor proxy", handle: monitorProxy, ouID: ouID, want: true},
		{name: "user proxy", handle: userProxy, ouID: ouID, want: false},
		{name: "unknown handle", handle: "no-such-proxy", ouID: ouID, want: false},
		{name: "agent proxy handle in another org", handle: agentProxy, ouID: "ou-other", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.IsAgentOrMonitorProxy(ctx, tt.handle, tt.ouID)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
