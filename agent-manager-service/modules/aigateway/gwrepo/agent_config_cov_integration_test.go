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

package gwrepo

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// covConfig creates a configuration of typ for the agent and returns it.
func covConfig(t *testing.T, repo AgentConfigurationRepository, ou string, agent uuid.UUID, name, typ string) *gwmodels.AgentConfiguration {
	t.Helper()
	cfg := &gwmodels.AgentConfiguration{Name: name, OUID: ou, AgentUUID: agent, Type: typ}
	require.NoError(t, repo.Create(context.Background(), db.GetDB(), cfg))
	return cfg
}

func covConfigNames(configs []gwmodels.AgentConfiguration) []string {
	out := make([]string, 0, len(configs))
	for _, c := range configs {
		out = append(out, c.Name)
	}
	return out
}

// An agent's configurations are created once per name, read with their LLM mappings, listed and counted per
// organization, agent and type, updated and deleted.
func TestCovAgentConfigurationRepoLifecycle(t *testing.T) {
	gdb := db.GetDB()
	repo := NewAgentConfigurationRepository(gdb)
	ou := newOrg(t)
	cart := ensureAgent(t, ou, "shop", "cart")
	other := ensureAgent(t, ou, "shop", "billing")
	llm1 := covConfig(t, repo, ou, cart, "llm-one", gwmodels.AgentConfigTypeLLM)
	covConfig(t, repo, ou, cart, "llm-two", gwmodels.AgentConfigTypeLLM)
	covConfig(t, repo, ou, cart, "mcp-one", gwmodels.AgentConfigTypeMCP)
	covConfig(t, repo, ou, other, "llm-billing", gwmodels.AgentConfigTypeLLM)

	err := repo.Create(context.Background(), gdb, &gwmodels.AgentConfiguration{Name: "llm-one", OUID: ou, AgentUUID: cart, Type: gwmodels.AgentConfigTypeLLM})
	require.ErrorIs(t, err, gwerr.ErrAgentConfigAlreadyExists)

	// A mapping's proxy comes back with its handle filled in.
	proxyHandle, proxyID := seedLLMProxy(t, ou)
	require.NoError(t, gdb.Model(&gwmodels.LLMProxy{}).Where("id = ?", proxyID).
		Update("configuration", dbx.JSON(gwmodels.LLMProxyConfig{Name: proxyHandle})).Error)
	env := envOf(t, ou, "dev")
	models := NewEnvAgentModelMappingRepository(gdb, repositories.NewCoreReadAPI(gdb))
	require.NoError(t, models.Create(context.Background(), gdb, &gwmodels.EnvAgentModelMapping{
		ConfigUUID: llm1.UUID, EnvironmentUUID: env, OrgID: ou, LLMProxyUUID: proxyID,
	}))
	got, err := repo.GetByUUID(context.Background(), llm1.UUID, ou)
	require.NoError(t, err)
	require.Len(t, got.EnvMappings, 1)
	require.NotNil(t, got.EnvMappings[0].LLMProxy)
	assert.Equal(t, proxyHandle, got.EnvMappings[0].LLMProxy.Handle)
	_, err = repo.GetByUUID(context.Background(), llm1.UUID, newOrg(t))
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	byAgentName, err := repo.GetByAgentID(context.Background(), "billing", ou)
	require.NoError(t, err)
	assert.Equal(t, "llm-billing", byAgentName.Name)

	all, err := repo.List(context.Background(), ou, 10, 0)
	require.NoError(t, err)
	assert.Len(t, all, 4)
	n, err := repo.Count(context.Background(), ou)
	require.NoError(t, err)
	assert.EqualValues(t, 4, n)
	ofCart, err := repo.ListByAgent(context.Background(), ou, "shop", "cart", 2, 0)
	require.NoError(t, err)
	rest, err := repo.ListByAgent(context.Background(), ou, "shop", "cart", 2, 2)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"llm-one", "llm-two", "mcp-one"}, append(covConfigNames(ofCart), covConfigNames(rest)...))
	n, err = repo.CountByAgent(context.Background(), ou, "shop", "cart")
	require.NoError(t, err)
	assert.EqualValues(t, 3, n)
	llms, err := repo.ListByAgentAndType(context.Background(), ou, "shop", "cart", gwmodels.AgentConfigTypeIDLLM, 10, 0)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"llm-one", "llm-two"}, covConfigNames(llms))
	n, err = repo.CountByAgentAndType(context.Background(), ou, "shop", "cart", gwmodels.AgentConfigTypeIDMCP)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)

	got.Description = "described"
	require.NoError(t, repo.Update(context.Background(), gdb, got))
	got, err = repo.GetByUUID(context.Background(), llm1.UUID, ou)
	require.NoError(t, err)
	assert.Equal(t, "described", got.Description)
	require.Len(t, got.EnvMappings, 1, "an update leaves the mappings alone")

	exists, err := repo.Exists(context.Background(), llm1.UUID, ou)
	require.NoError(t, err)
	assert.True(t, exists)
	require.NoError(t, models.DeleteByConfig(context.Background(), gdb, llm1.UUID))
	require.NoError(t, repo.Delete(context.Background(), gdb, llm1.UUID, ou))
	exists, err = repo.Exists(context.Background(), llm1.UUID, ou)
	require.NoError(t, err)
	assert.False(t, exists)
}

// An LLM mapping updates in place, is deleted per environment or per configuration, and the agents consuming a
// proxy are listed by name.
func TestCovEnvAgentModelMappingRepo(t *testing.T) {
	gdb := db.GetDB()
	repo := NewEnvAgentModelMappingRepository(gdb, repositories.NewCoreReadAPI(gdb))
	configs := NewAgentConfigurationRepository(gdb)
	ou := newOrg(t)
	proxyHandle, proxyID := seedLLMProxy(t, ou)
	cart := ensureAgent(t, ou, "checkout", "cart")
	cfg := covConfig(t, configs, ou, cart, "llm", gwmodels.AgentConfigTypeLLM)
	dev, prod := envOf(t, ou, "dev"), envOf(t, ou, "prod")
	for _, env := range []uuid.UUID{dev, prod} {
		require.NoError(t, repo.Create(context.Background(), gdb, &gwmodels.EnvAgentModelMapping{
			ConfigUUID: cfg.UUID, EnvironmentUUID: env, OrgID: ou, LLMProxyUUID: proxyID,
		}))
	}
	require.Error(t, repo.Create(context.Background(), gdb, &gwmodels.EnvAgentModelMapping{
		ConfigUUID: cfg.UUID, EnvironmentUUID: dev, OrgID: ou, LLMProxyUUID: proxyID,
	}), "one mapping per configuration and environment")

	consumers, err := repo.ListAgentConsumersByProxyUUIDs(context.Background(), []uuid.UUID{proxyID})
	require.NoError(t, err)
	require.Len(t, consumers, 1, "one row per agent, however many environments")
	assert.Equal(t, AgentConsumer{ProxyHandle: proxyHandle, ProxyName: proxyHandle, ProjectName: "checkout", AgentID: "cart"}, consumers[0])
	none, err := repo.ListAgentConsumersByProxyUUIDs(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, none)

	mapping, err := repo.GetByConfigAndEnv(context.Background(), cfg.UUID, dev)
	require.NoError(t, err)
	mapping.LLMProxy = nil
	mapping.PolicyConfiguration = gwmodels.LLMPolicies{{Name: "rate-limit", Version: "v1"}}
	require.NoError(t, repo.Update(context.Background(), gdb, mapping))
	mapping, err = repo.GetByConfigAndEnv(context.Background(), cfg.UUID, dev)
	require.NoError(t, err)
	require.Len(t, mapping.PolicyConfiguration, 1)
	assert.Equal(t, "rate-limit", mapping.PolicyConfiguration[0].Name)

	require.NoError(t, repo.Delete(context.Background(), gdb, cfg.UUID, dev))
	_, err = repo.GetByConfigAndEnv(context.Background(), cfg.UUID, dev)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.NoError(t, repo.DeleteByConfig(context.Background(), gdb, cfg.UUID))
	left, err := repo.ListByConfig(context.Background(), cfg.UUID)
	require.NoError(t, err)
	assert.Empty(t, left)
}

// A configuration's variables are inserted once per key, listed per environment, renamed and deleted.
func TestCovAgentEnvConfigVariableRepo(t *testing.T) {
	gdb := db.GetDB()
	repo := NewAgentEnvConfigVariableRepository(gdb, repositories.NewCoreReadAPI(gdb))
	ou := newOrg(t)
	cfg := covConfig(t, NewAgentConfigurationRepository(gdb), ou, ensureAgent(t, ou, "shop", "cart"), "llm", gwmodels.AgentConfigTypeLLM)
	dev, prod := envOf(t, ou, "dev"), envOf(t, ou, "prod")
	vars := func(env uuid.UUID, names ...string) []gwmodels.AgentEnvConfigVariable {
		out := make([]gwmodels.AgentEnvConfigVariable, 0, len(names))
		for i, n := range names {
			out = append(out, gwmodels.AgentEnvConfigVariable{
				ConfigUUID: cfg.UUID, EnvironmentUUID: env, VariableKey: []string{"url", "key"}[i], OrgID: ou, VariableName: n,
			})
		}
		return out
	}
	require.NoError(t, repo.CreateBatch(context.Background(), gdb, nil))
	require.NoError(t, repo.CreateBatch(context.Background(), gdb, vars(dev, "DEV_URL", "DEV_KEY")))
	require.NoError(t, repo.CreateBatch(context.Background(), gdb, vars(prod, "PROD_URL")))
	require.NoError(t, repo.CreateBatch(context.Background(), gdb, vars(dev, "IGNORED")), "an existing key is kept")

	inDev, err := repo.ListByConfigAndEnv(context.Background(), cfg.UUID, dev)
	require.NoError(t, err)
	names := map[string]string{}
	for _, v := range inDev {
		names[v.VariableKey] = v.VariableName
	}
	assert.Equal(t, map[string]string{"url": "DEV_URL", "key": "DEV_KEY"}, names)
	all, err := repo.ListByConfig(context.Background(), cfg.UUID)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		locked, err := repo.ListByConfigForUpdate(context.Background(), tx, cfg.UUID)
		require.NoError(t, err)
		assert.Len(t, locked, 3)
		require.NoError(t, repo.UpdateVariableNames(context.Background(), tx, cfg.UUID, map[string]string{"url": "SERVICE_URL"}))
		// Renaming to the current name changes no row but is not an error.
		require.NoError(t, repo.UpdateVariableNames(context.Background(), tx, cfg.UUID, map[string]string{"url": "SERVICE_URL"}))
		require.ErrorContains(t, repo.UpdateVariableNames(context.Background(), tx, cfg.UUID, map[string]string{"nope": "X"}),
			`unknown environment variable key "nope"`)
		return nil
	}))
	all, err = repo.ListByConfig(context.Background(), cfg.UUID)
	require.NoError(t, err)
	for _, v := range all {
		if v.VariableKey == "url" {
			assert.Equal(t, "SERVICE_URL", v.VariableName, "renamed in every environment")
		}
	}

	require.NoError(t, repo.DeleteByConfigAndEnv(context.Background(), gdb, cfg.UUID, prod))
	all, err = repo.ListByConfig(context.Background(), cfg.UUID)
	require.NoError(t, err)
	assert.Len(t, all, 2)
	require.NoError(t, repo.DeleteByConfig(context.Background(), gdb, cfg.UUID))
	all, err = repo.ListByConfig(context.Background(), cfg.UUID)
	require.NoError(t, err)
	assert.Empty(t, all)
}
