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
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsecret"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// covMCPProxy creates an MCP proxy, in project when it is not empty, and returns it.
func covMCPProxy(t *testing.T, repo MCPProxyRepository, ou, handle, projectName string) *gwmodels.MCPProxy {
	t.Helper()
	var projectID, name *string
	if projectName != "" {
		id := dbtest.Project(t, db.GetDB(), ou, projectName).String()
		projectID, name = &id, &projectName
	}
	p := &gwmodels.MCPProxy{Configuration: gwmodels.MCPProxyConfig{Name: handle, Version: "v1"}}
	require.NoError(t, db.GetDB().Transaction(func(tx *gorm.DB) error {
		return repo.Create(context.Background(), tx, p, handle, handle, "v1", ou, projectID, name)
	}))
	return p
}

// An MCP proxy is found by handle (locked or not) and in its project by name, moves between projects, updates, and
// is deleted with its endpoints' credentials unless a mapping still uses it.
func TestCovMCPProxyRepoLifecycle(t *testing.T) {
	gdb := db.GetDB()
	secrets := gwtest.NewSecrets()
	repo := NewMCPProxyRepo(gdb, repositories.NewCoreReadAPI(gdb), secrets)
	ou := newOrg(t)
	orgProxy := covMCPProxy(t, repo, ou, "org-"+uuid.NewString()[:6], "")
	projHandle := "proj-" + uuid.NewString()[:6]
	projProxy := covMCPProxy(t, repo, ou, projHandle, "payments")

	byHandle, err := repo.GetByHandle(context.Background(), projHandle, ou)
	require.NoError(t, err)
	assert.Equal(t, projProxy.UUID, byHandle.UUID)
	_, err = repo.GetByHandle(context.Background(), projHandle, newOrg(t))
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		locked, err := repo.GetByHandleForUpdate(context.Background(), tx, projHandle, ou)
		require.NoError(t, err)
		assert.Equal(t, projProxy.UUID, locked.UUID)
		_, err = repo.GetByHandleForUpdate(context.Background(), tx, "missing", ou)
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		return nil
	}))

	inProject, err := repo.GetInProjectByName(context.Background(), projHandle, ou, "payments")
	require.NoError(t, err)
	assert.Equal(t, projProxy.UUID, inProject.UUID)
	_, err = repo.GetInProjectByName(context.Background(), projHandle, ou, "no-such-project")
	require.ErrorIs(t, err, gwerr.ErrMCPProxyNotFound)
	_, err = repo.GetInProjectByName(context.Background(), orgProxy.Configuration.Name, ou, "payments")
	require.ErrorIs(t, err, gwerr.ErrMCPProxyNotFound, "an org proxy is in no project")

	from := *projProxy.ProjectUUID
	to := dbtest.Project(t, gdb, ou, "ledger").String()
	require.NoError(t, repo.ReassignProject(context.Background(), gdb, projProxy.UUID, ou, "ledger", from, to))
	_, err = repo.GetInProjectByName(context.Background(), projHandle, ou, "ledger")
	require.NoError(t, err)
	require.ErrorIs(t, repo.ReassignProject(context.Background(), gdb, projProxy.UUID, newOrg(t), "ledger", from, to),
		gwerr.ErrMCPProxyNotFound, "another org's proxy is not moved")
	require.ErrorIs(t, repo.ReassignProject(context.Background(), gdb, orgProxy.UUID, ou, "ledger", from, to),
		gwerr.ErrMCPProxyNotFound, "an org proxy is in neither project")

	byHandle.Description = "updated"
	byHandle.Configuration.Name, byHandle.Configuration.Version = "Renamed", "v2"
	require.NoError(t, repo.Update(context.Background(), gdb, byHandle, ou))
	updated, err := repo.GetByUUID(context.Background(), projProxy.UUID.String(), ou)
	require.NoError(t, err)
	assert.Equal(t, "updated", updated.Description)
	assert.Equal(t, "v2", updated.Artifact.Version)
	assert.Equal(t, projHandle, updated.Artifact.Name, "a project proxy's artifact is named by its handle")
	orgProxy.Configuration.Name = "Org Display"
	require.NoError(t, repo.Update(context.Background(), gdb, orgProxy, ou))
	updated, err = repo.GetByUUID(context.Background(), orgProxy.UUID.String(), ou)
	require.NoError(t, err)
	assert.Equal(t, "Org Display", updated.Artifact.Name, "an org proxy's artifact carries its display name")
	require.ErrorIs(t, repo.Update(context.Background(), gdb, nil, ou), gorm.ErrRecordNotFound)
	require.ErrorIs(t, repo.Update(context.Background(), gdb, &gwmodels.MCPProxy{UUID: uuid.New()}, ou), gorm.ErrRecordNotFound)

	exists, err := repo.Exists(context.Background(), projHandle, ou)
	require.NoError(t, err)
	assert.True(t, exists)

	// Deleting the proxy deletes its endpoints' credentials.
	endpoints := NewMCPProxyEndpointRepository(gdb)
	require.NoError(t, endpoints.CreateEndpoint(context.Background(), gdb, &gwmodels.MCPProxyEndpoint{
		OrgID: ou, MCPProxyUUID: projProxy.UUID, Handle: "main", Status: gwmodels.StatusCreated,
	}))
	require.NoError(t, secrets.Put(context.Background(), ou, gwsecret.MCPEndpoint(projProxy.UUID, "main"), "token"))
	require.NoError(t, repo.Delete(context.Background(), projHandle, ou))
	assert.Zero(t, secrets.Len())
	exists, err = repo.Exists(context.Background(), projHandle, ou)
	require.NoError(t, err)
	assert.False(t, exists)
	require.ErrorIs(t, repo.Delete(context.Background(), projHandle, ou), gorm.ErrRecordNotFound)
}

// An endpoint is created, updated, bound to environments, resolved per proxy and environment, and deleted.
func TestCovMCPProxyEndpointRepo(t *testing.T) {
	gdb := db.GetDB()
	repo := NewMCPProxyEndpointRepository(gdb)
	ou := newOrg(t)
	proxy := covMCPProxy(t, NewMCPProxyRepo(gdb, repositories.NewCoreReadAPI(gdb), gwtest.NewSecrets()), ou, "ep-"+uuid.NewString()[:6], "")
	dev, prod := envOf(t, ou, "dev"), envOf(t, ou, "prod")

	require.Error(t, repo.CreateEndpoint(context.Background(), gdb, nil))
	ep := &gwmodels.MCPProxyEndpoint{OrgID: ou, MCPProxyUUID: proxy.UUID, Handle: "main", Status: gwmodels.StatusCreated}
	require.NoError(t, repo.CreateEndpoint(context.Background(), gdb, ep))
	assert.NotEqual(t, uuid.Nil, ep.UUID)
	require.Error(t, repo.CreateEndpoint(context.Background(), gdb, &gwmodels.MCPProxyEndpoint{
		OrgID: ou, MCPProxyUUID: uuid.New(), Handle: "orphan",
	}), "an endpoint needs its proxy")

	ep.Name, ep.Status = "Main endpoint", "deployed"
	ep.Configuration.Upstream = &gwmodels.UpstreamEndpoint{URL: "https://mcp.example.com"}
	require.NoError(t, repo.UpdateEndpoint(context.Background(), gdb, ep))
	require.ErrorIs(t, repo.UpdateEndpoint(context.Background(), gdb, &gwmodels.MCPProxyEndpoint{UUID: uuid.New()}), gorm.ErrRecordNotFound)
	require.ErrorIs(t, repo.UpdateEndpoint(context.Background(), gdb, nil), gorm.ErrRecordNotFound)

	require.Error(t, repo.AddEndpointEnvironment(context.Background(), gdb, nil))
	for _, env := range []uuid.UUID{dev, prod} {
		require.NoError(t, repo.AddEndpointEnvironment(context.Background(), gdb, &gwmodels.MCPProxyEndpointEnvironment{
			EndpointUUID: ep.UUID, EnvironmentUUID: env, OrgID: ou, MCPProxyUUID: proxy.UUID,
			ArtifactUUID: newArtifact(t, ou, gwmodels.KindMCPMapping, "binding-"+uuid.NewString()[:8]),
			Status:       gwmodels.StatusCreated,
		}))
	}

	got, err := repo.GetEndpoint(context.Background(), ep.UUID)
	require.NoError(t, err)
	assert.Equal(t, "Main endpoint", got.Name)
	assert.Equal(t, "https://mcp.example.com", got.Configuration.Upstream.URL)
	assert.Len(t, got.Environments, 2)
	_, err = repo.GetEndpoint(context.Background(), uuid.New())
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	listed, err := repo.ListEndpointsByProxy(context.Background(), proxy.UUID)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	envs, err := repo.ListEndpointEnvironments(context.Background(), ep.UUID)
	require.NoError(t, err)
	assert.Len(t, envs, 2)
	inProd, err := repo.GetEndpointEnvByProxyAndEnv(context.Background(), proxy.UUID, prod)
	require.NoError(t, err)
	assert.Equal(t, ep.UUID, inProd.EndpointUUID)
	_, err = repo.GetEndpointEnvByProxyAndEnv(context.Background(), proxy.UUID, uuid.New())
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	require.NoError(t, repo.RemoveEndpointEnvironment(context.Background(), gdb, ep.UUID, prod))
	require.ErrorIs(t, repo.RemoveEndpointEnvironment(context.Background(), gdb, ep.UUID, prod), gorm.ErrRecordNotFound)
	byProxy, err := repo.ListEndpointEnvironmentsByProxy(context.Background(), proxy.UUID)
	require.NoError(t, err)
	require.Len(t, byProxy, 1)
	assert.Equal(t, dev, byProxy[0].EnvironmentUUID)

	require.NoError(t, repo.RemoveEndpointEnvironment(context.Background(), gdb, ep.UUID, dev))
	require.NoError(t, repo.DeleteEndpoint(context.Background(), gdb, ep.UUID))
	require.ErrorIs(t, repo.DeleteEndpoint(context.Background(), gdb, ep.UUID), gorm.ErrRecordNotFound)
}

// A scope is unique per proxy and action, keeps its tools deduplicated and sorted, and is updated and deleted.
func TestCovMCPProxyScopeRepo(t *testing.T) {
	gdb := db.GetDB()
	repo := NewMCPProxyScopeRepository(gdb)
	ou := newOrg(t)
	proxies := NewMCPProxyRepo(gdb, repositories.NewCoreReadAPI(gdb), gwtest.NewSecrets())
	p1 := covMCPProxy(t, proxies, ou, "s1-"+uuid.NewString()[:6], "")
	p2 := covMCPProxy(t, proxies, ou, "s2-"+uuid.NewString()[:6], "")

	read := &gwmodels.MCPProxyScope{MCPProxyUUID: p1.UUID, Action: "read", Description: "r", Tools: []string{"search", "fetch", "search"}}
	require.NoError(t, repo.Create(context.Background(), read))
	assert.Equal(t, ou, read.OrgID, "the scope takes its proxy's organization")
	require.NoError(t, repo.Create(context.Background(), &gwmodels.MCPProxyScope{MCPProxyUUID: p1.UUID, Action: "admin"}))
	require.NoError(t, repo.Create(context.Background(), &gwmodels.MCPProxyScope{MCPProxyUUID: p2.UUID, Action: "read", Tools: []string{"x"}}))
	require.Error(t, repo.Create(context.Background(), &gwmodels.MCPProxyScope{MCPProxyUUID: p1.UUID, Action: "read"}),
		"one scope per proxy and action")
	require.Error(t, repo.Create(context.Background(), &gwmodels.MCPProxyScope{MCPProxyUUID: uuid.New(), Action: "read"}))

	got, err := repo.Get(context.Background(), p1.UUID, "read")
	require.NoError(t, err)
	assert.Equal(t, []string{"fetch", "search"}, got.Tools)
	_, err = repo.Get(context.Background(), p1.UUID, "write")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	ofP1, err := repo.ListByProxy(context.Background(), p1.UUID)
	require.NoError(t, err)
	require.Len(t, ofP1, 2)
	assert.Equal(t, "admin", ofP1[0].Action, "ordered by action")
	assert.Equal(t, []string{}, ofP1[0].Tools, "a scope without tools lists none")
	both, err := repo.ListByProxyUUIDs(context.Background(), []uuid.UUID{p1.UUID, p2.UUID})
	require.NoError(t, err)
	assert.Len(t, both, 3)
	none, err := repo.ListByProxyUUIDs(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, none)

	require.NoError(t, repo.Update(context.Background(), &gwmodels.MCPProxyScope{
		MCPProxyUUID: p1.UUID, Action: "read", Description: "reads", Tools: []string{"list"},
	}))
	got, err = repo.Get(context.Background(), p1.UUID, "read")
	require.NoError(t, err)
	assert.Equal(t, "reads", got.Description)
	assert.Equal(t, []string{"list"}, got.Tools)
	require.ErrorIs(t, repo.Update(context.Background(), &gwmodels.MCPProxyScope{MCPProxyUUID: p1.UUID, Action: "write"}),
		gorm.ErrRecordNotFound)

	require.NoError(t, repo.Delete(context.Background(), p1.UUID, "read"))
	require.ErrorIs(t, repo.Delete(context.Background(), p1.UUID, "read"), gorm.ErrRecordNotFound)
}

// An agent's MCP configuration records its proxy and environment scope, maps per environment through a mapping
// artifact, and is listed by agent and by proxy; a proxy still mapped cannot be deleted.
func TestCovMCPConfigurationMappings(t *testing.T) {
	gdb := db.GetDB()
	ou := newOrg(t)
	proxies := NewMCPProxyRepo(gdb, repositories.NewCoreReadAPI(gdb), gwtest.NewSecrets())
	configs := NewAgentConfigurationRepository(gdb)
	mappings := NewEnvAgentMCPMappingRepository(gdb)
	require.NotPanics(t, func() {
		require.Error(t, mappings.Create(context.Background(), gdb, &gwmodels.EnvAgentMCPMapping{}, nil, "h", "n", "v1", ou),
			"a missing proxy mapping is refused, not dereferenced")
		require.Error(t, mappings.Create(context.Background(), gdb, nil, &gwmodels.MCPProxyMapping{}, "h", "n", "v1", ou))
	})
	proxyHandle := "m-" + uuid.NewString()[:6]
	proxy := covMCPProxy(t, proxies, ou, proxyHandle, "")
	other := covMCPProxy(t, proxies, ou, "m2-"+uuid.NewString()[:6], "")
	cart := ensureAgent(t, ou, "shop", "cart")
	cfg := covConfig(t, configs, ou, cart, "tools", gwmodels.AgentConfigTypeMCP)
	dev, prod, stage := envOf(t, ou, "dev"), envOf(t, ou, "prod"), envOf(t, ou, "stage")

	require.NoError(t, configs.SetMCPProxyRef(context.Background(), gdb, cfg.UUID, other.UUID))
	require.NoError(t, configs.SetMCPProxyRef(context.Background(), gdb, cfg.UUID, proxy.UUID), "re-pointing is idempotent")
	require.NoError(t, configs.SetMCPEnvScope(context.Background(), gdb, cfg.UUID, []uuid.UUID{prod, stage}, nil))
	require.NoError(t, configs.SetMCPEnvScope(context.Background(), gdb, cfg.UUID, []uuid.UUID{prod, stage}, []uuid.UUID{stage}),
		"an environment both excluded and included stays included")

	for _, env := range []uuid.UUID{dev} {
		handle := "map-" + uuid.NewString()[:6]
		require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
			return mappings.Create(context.Background(), tx,
				&gwmodels.EnvAgentMCPMapping{ConfigUUID: cfg.UUID, EnvironmentUUID: env, OrgID: ou, MCPProxyUUID: proxy.UUID},
				&gwmodels.MCPProxyMapping{Configuration: gwmodels.MCPProxyConfig{Name: handle}}, handle, handle, "v1", ou)
		}))
	}
	err := mappings.Create(context.Background(), gdb,
		&gwmodels.EnvAgentMCPMapping{ConfigUUID: cfg.UUID, EnvironmentUUID: prod, OrgID: ou, MCPProxyUUID: proxy.UUID, ArtifactUUID: uuid.New()},
		&gwmodels.MCPProxyMapping{UUID: uuid.New()}, "mismatch", "mismatch", "v1", ou)
	require.ErrorContains(t, err, "must match")

	got, err := configs.GetByUUID(context.Background(), cfg.UUID, ou)
	require.NoError(t, err)
	require.NotNil(t, got.MCPProxyRef)
	assert.Equal(t, proxy.UUID, got.MCPProxyRef.MCPProxyUUID)
	require.NotNil(t, got.MCPScope, "the scope is recorded")
	require.Len(t, got.MCPEnvExclusions, 1)
	assert.Equal(t, prod, got.MCPEnvExclusions[0].EnvironmentUUID)
	require.Len(t, got.EnvMCPMappings, 1, "exclusions are not mappings")
	assert.Equal(t, dev, got.EnvMCPMappings[0].EnvironmentUUID)
	require.NotNil(t, got.EnvMCPMappings[0].MCPProxy)
	assert.Equal(t, proxy.UUID, got.EnvMCPMappings[0].MCPProxy.UUID)

	byAgent, err := configs.ListMCPConfigsByAgent(context.Background(), ou, "shop", "cart")
	require.NoError(t, err)
	assert.Equal(t, []string{"tools"}, covConfigNames(byAgent))
	byProxy, err := configs.ListMCPConfigsByProxy(context.Background(), ou, proxy.UUID)
	require.NoError(t, err)
	assert.Equal(t, []string{"tools"}, covConfigNames(byProxy))
	byOther, err := configs.ListMCPConfigsByProxy(context.Background(), ou, other.UUID)
	require.NoError(t, err)
	assert.Empty(t, byOther)

	for name, list := range map[string]func() ([]gwmodels.EnvAgentMCPMapping, error){
		"config": func() ([]gwmodels.EnvAgentMCPMapping, error) {
			return mappings.ListByConfig(context.Background(), cfg.UUID)
		},
		"proxy": func() ([]gwmodels.EnvAgentMCPMapping, error) {
			return mappings.ListByMCPProxy(context.Background(), proxy.UUID)
		},
		"environment": func() ([]gwmodels.EnvAgentMCPMapping, error) {
			return mappings.ListByEnvironment(context.Background(), dev)
		},
	} {
		listed, err := list()
		require.NoError(t, err, name)
		require.Len(t, listed, 1, name)
		require.NotNil(t, listed[0].MCPProxyMapping, name)
		assert.Equal(t, proxy.UUID, listed[0].MCPProxyMapping.SourceMCPProxyUUID, name)
		assert.Equal(t, gwmodels.KindMCPMapping, listed[0].Artifact.Kind, name)
	}
	inProd, err := mappings.ListByEnvironment(context.Background(), prod)
	require.NoError(t, err)
	assert.Empty(t, inProd, "an exclusion is not listed as a mapping")

	require.ErrorIs(t, proxies.Delete(context.Background(), proxyHandle, ou), gwerr.ErrMCPProxyHasMappings)

	listed, err := mappings.ListByConfig(context.Background(), cfg.UUID)
	require.NoError(t, err)
	m := listed[0]
	m.MCPProxy, m.MCPProxyMapping, m.Artifact = nil, nil, nil
	m.MCPProxyUUID = other.UUID
	require.NoError(t, mappings.Update(context.Background(), gdb, &m))
	moved, err := mappings.ListByMCPProxy(context.Background(), other.UUID)
	require.NoError(t, err)
	assert.Len(t, moved, 1)

	require.NoError(t, mappings.Delete(context.Background(), gdb, cfg.UUID, dev))
	require.NoError(t, mappings.DeleteByConfig(context.Background(), gdb, cfg.UUID))
	listed, err = mappings.ListByConfig(context.Background(), cfg.UUID)
	require.NoError(t, err)
	assert.Empty(t, listed)

	require.NoError(t, configs.ClearMCPProxyRef(context.Background(), gdb, cfg.UUID))
	got, err = configs.GetByUUID(context.Background(), cfg.UUID, ou)
	require.NoError(t, err)
	assert.Nil(t, got.MCPProxyRef)
}
