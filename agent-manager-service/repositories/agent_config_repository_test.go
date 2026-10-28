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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// Defaults for a newly proxied environment must never overwrite a saved config.
func TestAgentConfigCreateIfAbsentKeepsAnExistingRow(t *testing.T) {
	repo := NewAgentConfigRepo(db.GetDB())
	ctx := context.Background()
	saved := &models.AgentConfig{
		OUID: "ou-" + uuid.New().String()[:8], ProjectName: "checkout",
		AgentName: "cfg-" + uuid.New().String()[:8], EnvironmentName: "Development",
		EnableApiKeySecurity: false, CORSEnabled: false,
		OAuthHeaderName: "X-Saved", OAuthAuthHeaderPrefix: models.DefaultOAuthAuthHeaderPrefix,
	}
	t.Cleanup(func() { _ = repo.DeleteAllByAgent(ctx, saved.OUID, saved.ProjectName, saved.AgentName) })
	require.NoError(t, repo.Upsert(ctx, saved))

	defaults := *saved
	defaults.ID = uuid.Nil
	defaults.EnableApiKeySecurity, defaults.CORSEnabled = true, true
	defaults.OAuthHeaderName = models.DefaultOAuthHeaderName
	require.NoError(t, repo.CreateIfAbsent(ctx, &defaults))

	got, err := repo.Get(ctx, saved.OUID, saved.ProjectName, saved.AgentName, "Development")
	require.NoError(t, err)
	assert.Equal(t, "X-Saved", got.OAuthHeaderName, "the saved config wins")
}

func TestAgentConfigCreateIfAbsentInsertsDefaults(t *testing.T) {
	repo := NewAgentConfigRepo(db.GetDB())
	ctx := context.Background()
	cfg := &models.AgentConfig{
		OUID: "ou-" + uuid.New().String()[:8], ProjectName: "checkout",
		AgentName: "cfg-new-" + uuid.New().String()[:8], EnvironmentName: "Development",
		EnableApiKeySecurity: true, CORSEnabled: true, CORSAllowOrigins: []string{"*"},
		OAuthHeaderName: models.DefaultOAuthHeaderName, OAuthAuthHeaderPrefix: models.DefaultOAuthAuthHeaderPrefix,
	}
	t.Cleanup(func() { _ = repo.DeleteAllByAgent(ctx, cfg.OUID, cfg.ProjectName, cfg.AgentName) })
	require.NoError(t, repo.CreateIfAbsent(ctx, cfg))

	got, err := repo.Get(ctx, cfg.OUID, cfg.ProjectName, cfg.AgentName, "Development")
	require.NoError(t, err)
	assert.True(t, got.EnableApiKeySecurity)
	assert.Equal(t, []string{"*"}, got.CORSAllowOrigins)
	assert.Nil(t, got.CardCORSEnabled, "the card inherits the agent's CORS")
}

func TestAgentConfigDeleteForEnvironmentDropsEveryAgentsConfig(t *testing.T) {
	repo := NewAgentConfigRepo(db.GetDB())
	ctx := context.Background()
	ouID := "ou-" + uuid.New().String()[:8]
	agentA, agentB := "cfg-a-"+uuid.New().String()[:8], "cfg-b-"+uuid.New().String()[:8]
	newConfig := func(agent, env string) *models.AgentConfig {
		return &models.AgentConfig{
			OUID: ouID, ProjectName: "checkout", AgentName: agent, EnvironmentName: env,
			OAuthHeaderName: models.DefaultOAuthHeaderName, OAuthAuthHeaderPrefix: models.DefaultOAuthAuthHeaderPrefix,
		}
	}
	t.Cleanup(func() {
		_ = repo.DeleteAllByAgent(ctx, ouID, "checkout", agentA)
		_ = repo.DeleteAllByAgent(ctx, ouID, "checkout", agentB)
	})
	require.NoError(t, repo.Upsert(ctx, newConfig(agentA, "dev")))
	require.NoError(t, repo.Upsert(ctx, newConfig(agentB, "dev")))
	require.NoError(t, repo.Upsert(ctx, newConfig(agentA, "prod")))

	require.NoError(t, repo.DeleteForEnvironment(ctx, ouID, "dev"))

	_, err := repo.Get(ctx, ouID, "checkout", agentA, "dev")
	assert.ErrorIs(t, err, ErrAgentConfigNotFound)
	_, err = repo.Get(ctx, ouID, "checkout", agentB, "dev")
	assert.ErrorIs(t, err, ErrAgentConfigNotFound)
	_, err = repo.Get(ctx, ouID, "checkout", agentA, "prod")
	assert.NoError(t, err, "other environments keep their configs")
}
