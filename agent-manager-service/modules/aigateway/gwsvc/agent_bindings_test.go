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

package gwsvc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/wso2/agent-manager/agent-manager-service/db/sqlitedialect"
	dbmigrations "github.com/wso2/agent-manager/agent-manager-service/db_migrations"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
)

// bindingConfigs stands in for the configuration service: it remembers the
// configurations it created, and fails creates while fail is set.
type bindingConfigs struct {
	stubAgentConfigurationService
	configs map[string]string // name -> uuid
	created int
	deleted []string
	fail    error
}

func newBindingConfigs() *bindingConfigs {
	b := &bindingConfigs{configs: map[string]string{}}
	b.ListFunc = func(context.Context, string, string, string, int, int) (*gwmodels.AgentModelConfigListResponse, error) {
		resp := &gwmodels.AgentModelConfigListResponse{}
		for name, id := range b.configs {
			resp.Configs = append(resp.Configs, gwmodels.AgentModelConfigListItem{UUID: id, Name: name})
		}
		resp.Pagination.Count = len(resp.Configs)
		return resp, nil
	}
	b.DeleteForAgentDeletionFunc = func(_ context.Context, id uuid.UUID, _, _, _ string, _ bool) error {
		b.forget(id.String())
		return nil
	}
	return b
}

func (b *bindingConfigs) forget(id string) {
	for name, cid := range b.configs {
		if cid == id {
			delete(b.configs, name)
			b.deleted = append(b.deleted, name)
		}
	}
}

func (b *bindingConfigs) Create(_ context.Context, _, _, _ string, req gwmodels.CreateAgentModelConfigRequest, _ string) (*gwmodels.AgentModelConfigResponse, error) {
	if b.fail != nil {
		return nil, b.fail
	}
	b.created++
	id := uuid.NewString()
	b.configs[req.Name] = id
	return &gwmodels.AgentModelConfigResponse{UUID: id}, nil
}

func (b *bindingConfigs) Delete(_ context.Context, id uuid.UUID, _, _, _ string) error {
	b.forget(id.String())
	return nil
}

// bindingsDB is a scratch SQLite database with the core's baseline, the
// module's tables and one agent.
func bindingsDB(t *testing.T) (db *gorm.DB, orgID, agentID string) {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "amp.db") + "?_pragma=foreign_keys(1)&_txlock=immediate&_time_format=sqlite"
	db, err := gorm.Open(sqlitedialect.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, dbmigrations.MigrateBaseline(db, "sqlite"))
	module, err := os.ReadFile(filepath.Join("..", "migrations", "sqlite", "001_init.sql"))
	require.NoError(t, err)
	require.NoError(t, db.Exec(string(module)).Error)

	orgID, projectID, agentID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	require.NoError(t, db.Exec(`INSERT INTO organizations (id, handle, display_name, created_at, updated_at) VALUES (?, 'o', 'O', ?, ?)`, orgID, now, now).Error)
	require.NoError(t, db.Exec(`INSERT INTO projects (id, org_id, name, display_name, created_at, updated_at) VALUES (?, ?, 'p', 'P', ?, ?)`, projectID, orgID, now, now).Error)
	require.NoError(t, db.Exec(`INSERT INTO agents (id, org_id, project_id, name, display_name, provisioning_type, created_at, updated_at)
		VALUES (?, ?, ?, 'a', 'A', 'external', ?, ?)`, agentID, orgID, projectID, now, now).Error)
	return db, orgID, agentID
}

func bindingsService(db *gorm.DB, configs *bindingConfigs) *AgentGatewayService {
	return &AgentGatewayService{
		db:                        db,
		agentConfigurationService: configs,
		aiApplicationService:      newAIAppServiceForCleanup(),
		artifactRepo: &gwrepomocks.ArtifactRepositoryMock{
			ListByHandlePrefixFunc: func(context.Context, []string, string, string) ([]*gwmodels.Artifact, error) { return nil, nil },
		},
		logger: discardLogger(),
	}
}

func storeBindings(t *testing.T, s *AgentGatewayService, action, orgID, agentID string, ext AgentExtension) bool {
	t.Helper()
	req := modkit.HookRequest{
		Resource: modkit.ResourceAgent, Action: action, OrgID: orgID, ID: agentID,
		Object: map[string]any{"projectName": "p", "name": "a"},
	}
	var changed bool
	require.NoError(t, s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		changed, err = s.StoreAgentExtension(context.Background(), tx, req, ext)
		return err
	}))
	return changed
}

func bindingStates(t *testing.T, s *AgentGatewayService) map[string]AgentBinding {
	t.Helper()
	rows, err := s.AgentBindings(context.Background(), orgOf(t, s), "p", "a")
	require.NoError(t, err)
	out := map[string]AgentBinding{}
	for _, r := range rows {
		out[r.Target] = r
	}
	return out
}

func orgOf(t *testing.T, s *AgentGatewayService) string {
	t.Helper()
	var org string
	require.NoError(t, s.db.Raw(`SELECT id FROM organizations`).Scan(&org).Error)
	return org
}

func llmExt(providers ...string) AgentExtension {
	items := make([]gwspec.ModelConfigRequest, 0, len(providers))
	for _, p := range providers {
		items = append(items, gwspec.ModelConfigRequest{ProviderName: p})
	}
	return AgentExtension{ModelConfig: &items}
}

func TestAgentBindingsBecomeReady(t *testing.T) {
	db, org, agent := bindingsDB(t)
	configs := newBindingConfigs()
	s := bindingsService(db, configs)
	ctx := context.Background()

	storeBindings(t, s, modkit.ActionCreate, org, agent, llmExt("openai"))
	assert.Equal(t, BindingPending, bindingStates(t, s)["openai"].State, "stored with the agent, provisioned later")

	require.NoError(t, s.ReconcileAgentBindings(ctx, org, agent))
	b := bindingStates(t, s)["openai"]
	assert.Equal(t, BindingReady, b.State)
	require.NotNil(t, b.ConfigID)
	assert.Nil(t, b.LastError)

	// A redelivered event changes nothing.
	require.NoError(t, s.ReconcileAgentBindings(ctx, org, agent))
	assert.Equal(t, 1, configs.created)
}

func TestAgentBindingFailsThenRetriesToReady(t *testing.T) {
	db, org, agent := bindingsDB(t)
	configs := newBindingConfigs()
	s := bindingsService(db, configs)
	ctx := context.Background()

	storeBindings(t, s, modkit.ActionCreate, org, agent, llmExt("openai"))
	configs.fail = errors.New("no gateway serves provider openai")
	require.Error(t, s.ReconcileAgentBindings(ctx, org, agent), "the event is retried")
	b := bindingStates(t, s)["openai"]
	assert.Equal(t, BindingFailed, b.State)
	require.NotNil(t, b.LastError)
	assert.Contains(t, *b.LastError, "no gateway serves provider openai")

	configs.fail = nil
	require.NoError(t, s.ReconcileAgentBindings(ctx, org, agent))
	b = bindingStates(t, s)["openai"]
	assert.Equal(t, BindingReady, b.State)
	assert.Nil(t, b.LastError)
}

// A configuration created before its binding could be marked ready is found
// again by name, not created twice.
func TestAgentBindingAdoptsItsConfiguration(t *testing.T) {
	db, org, agent := bindingsDB(t)
	configs := newBindingConfigs()
	s := bindingsService(db, configs)

	storeBindings(t, s, modkit.ActionCreate, org, agent, llmExt("openai"))
	row := bindingStates(t, s)["openai"]
	configs.configs[bindingConfigName("a", row)] = uuid.NewString()

	require.NoError(t, s.ReconcileAgentBindings(context.Background(), org, agent))
	assert.Equal(t, 0, configs.created)
	assert.Equal(t, BindingReady, bindingStates(t, s)["openai"].State)
}

func TestAgentUpdateReplacesBindings(t *testing.T) {
	db, org, agent := bindingsDB(t)
	configs := newBindingConfigs()
	s := bindingsService(db, configs)
	ctx := context.Background()

	storeBindings(t, s, modkit.ActionCreate, org, agent, llmExt("openai"))
	require.NoError(t, s.ReconcileAgentBindings(ctx, org, agent))
	oldName := bindingConfigName("a", bindingStates(t, s)["openai"])

	assert.False(t, storeBindings(t, s, modkit.ActionUpdate, org, agent, llmExt("openai")), "the same list changes nothing")
	assert.False(t, storeBindings(t, s, modkit.ActionUpdate, org, agent, AgentExtension{}), "an absent list leaves the bindings alone")
	require.True(t, storeBindings(t, s, modkit.ActionUpdate, org, agent, llmExt("anthropic")))
	states := bindingStates(t, s)
	assert.Equal(t, BindingRemoving, states["openai"].State)
	assert.Equal(t, BindingPending, states["anthropic"].State)

	require.NoError(t, s.ReconcileAgentBindings(ctx, org, agent))
	states = bindingStates(t, s)
	assert.NotContains(t, states, "openai")
	assert.Equal(t, BindingReady, states["anthropic"].State)
	assert.Equal(t, []string{oldName}, configs.deleted)
}

func TestAgentDeletedWhileBindingsPending(t *testing.T) {
	db, org, agent := bindingsDB(t)
	configs := newBindingConfigs()
	s := bindingsService(db, configs)
	ctx := context.Background()

	storeBindings(t, s, modkit.ActionCreate, org, agent, llmExt("openai", "anthropic"))
	// The first binding provisioned before the agent went; the second did not.
	rows := bindingStates(t, s)
	configs.configs[bindingConfigName("a", rows["openai"])] = uuid.NewString()

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := RecordAgentTeardown(tx, modkit.HookRequest{OrgID: org, ID: agent, Object: map[string]any{"projectName": "p", "name": "a"}}); err != nil {
			return err
		}
		return tx.Exec(`DELETE FROM agents WHERE id = ?`, agent).Error
	}))

	// agent.created arrives after the delete: nothing more is provisioned.
	require.NoError(t, s.ReconcileAgentBindings(ctx, org, agent))
	assert.Equal(t, 0, configs.created)

	// agent.deleted revokes what the bindings provisioned and clears the name.
	require.NoError(t, s.TeardownAgent(ctx, agent))
	assert.Empty(t, configs.configs)
	var left int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM mod_ai_gateway_agent_teardowns`).Scan(&left).Error)
	assert.Zero(t, left)
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM mod_ai_gateway_agent_bindings`).Scan(&left).Error)
	assert.Zero(t, left)
	// A redelivered agent.deleted finds nothing to do.
	require.NoError(t, s.TeardownAgent(ctx, agent))
}

func TestAgentTeardownRetriesUntilRevoked(t *testing.T) {
	db, org, agent := bindingsDB(t)
	configs := newBindingConfigs()
	s := bindingsService(db, configs)
	ctx := context.Background()
	configs.configs["a-llm-config-x"] = uuid.NewString()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return RecordAgentTeardown(tx, modkit.HookRequest{OrgID: org, ID: agent, Object: map[string]any{"projectName": "p", "name": "a"}})
	}))

	revoke := configs.DeleteForAgentDeletionFunc
	configs.DeleteForAgentDeletionFunc = func(context.Context, uuid.UUID, string, string, string, bool) error {
		return errors.New("gateway unreachable")
	}
	require.Error(t, s.TeardownAgent(ctx, agent))
	var td AgentTeardown
	require.NoError(t, db.Take(&td, "agent_id = ?", agent).Error)
	require.NotNil(t, td.LastError)

	configs.DeleteForAgentDeletionFunc = revoke
	require.NoError(t, s.TeardownAgent(ctx, agent))
	assert.Empty(t, configs.configs)
}
