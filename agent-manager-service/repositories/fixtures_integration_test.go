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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// Rows hang off an organization, project, agent and environment, which the tables reference by id. These helpers
// create them on demand: each top-level test gets its own organization, so tests stay apart, and every helper call in
// one test shares it.

var (
	testOrgsMu sync.Mutex
	testOrgs   = map[string]string{}
)

// testOrg is the calling test's organization.
func testOrg(t *testing.T) string {
	t.Helper()
	name := strings.Split(t.Name(), "/")[0]
	testOrgsMu.Lock()
	defer testOrgsMu.Unlock()
	if ou, ok := testOrgs[name]; ok {
		return ou
	}
	ou := dbtest.Org(t, db.GetDB())
	testOrgs[name] = ou
	return ou
}

// newOrg is a further organization, for tests that check one org cannot see another's rows.
func newOrg(t *testing.T) string {
	t.Helper()
	return dbtest.Org(t, db.GetDB())
}

// envOf is the id of the organization's environment named name, created on first use.
func envOf(t *testing.T, ou, name string) uuid.UUID {
	t.Helper()
	gormDB := db.GetDB()
	var env models.Environment
	err := gormDB.Where("org_id = ? AND name = ?", ou, name).Limit(1).Find(&env).Error
	require.NoError(t, err)
	if env.ID != uuid.Nil {
		return env.ID
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	env = models.Environment{
		ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(ou+"/"+name)), OrgID: uuid.MustParse(ou), Name: name,
		DisplayName: name, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, gormDB.Create(&env).Error)
	return env.ID
}

// ensureAgent is the id of the agent named agent in project, creating both on first use.
func ensureAgent(t *testing.T, ou, project, agent string) uuid.UUID {
	t.Helper()
	gormDB := db.GetDB()
	var p models.Project
	require.NoError(t, gormDB.Where("org_id = ? AND name = ?", ou, project).Limit(1).Find(&p).Error)
	if p.ID == uuid.Nil {
		p.ID = dbtest.Project(t, gormDB, ou, project)
	}
	var a models.Agent
	require.NoError(t, gormDB.Where("org_id = ? AND name = ?", ou, agent).Limit(1).Find(&a).Error)
	if a.ID != uuid.Nil {
		return a.ID
	}
	return dbtest.Agent(t, gormDB, ou, p.ID, agent)
}

// newArtifact creates an artifact of kind in the organization and returns its id.
func newArtifact(t *testing.T, ou, kind, handle string) uuid.UUID {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	a := models.Artifact{
		UUID: uuid.Must(uuid.NewV7()), Handle: handle, Name: handle, Version: "v1.0", Kind: kind, OUID: ou,
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.GetDB().Create(&a).Error)
	return a.UUID
}

// seedAgent makes sure the agent and each named environment exist in the organization.
func seedAgent(t *testing.T, ou, project, agent string, envs ...string) {
	t.Helper()
	ensureAgent(t, ou, project, agent)
	for _, env := range envs {
		envOf(t, ou, env)
	}
}

// newGateway creates an active gateway with role in the organization, mapped to its default environment.
func newGateway(t *testing.T, ou, role string) uuid.UUID {
	t.Helper()
	return dbtest.Gateway(t, db.GetDB(), ou, role).UUID
}

// statusRowWhere selects one placement's status row.
const statusRowWhere = "artifact_id = ? AND org_id = ? AND gateway_id = ?"

// envWithID makes sure the organization has an environment with id, named name unless the name is taken.
func envWithID(t *testing.T, ou string, id uuid.UUID, name string) {
	t.Helper()
	gormDB := db.GetDB()
	var n int64
	require.NoError(t, gormDB.Model(&models.Environment{}).Where("id = ?", id).Count(&n).Error)
	if n > 0 {
		return
	}
	require.NoError(t, gormDB.Model(&models.Environment{}).Where("org_id = ? AND name = ?", ou, name).Count(&n).Error)
	if n > 0 {
		name += "-" + id.String()[:8]
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, gormDB.Create(&models.Environment{
		ID: id, OrgID: uuid.MustParse(ou), Name: name, DisplayName: name, CreatedAt: now, UpdatedAt: now,
	}).Error)
}

// seedLLMProxy writes a provider and an LLM proxy of it in the organization's project "checkout", and returns the
// proxy's handle and id.
func seedLLMProxy(t *testing.T, ou string) (string, uuid.UUID) {
	t.Helper()
	gdb := db.GetDB()
	providerID := newArtifact(t, ou, models.KindLLMProvider, "provider-"+uuid.NewString()[:8])
	require.NoError(t, gdb.Create(&models.LLMProvider{
		UUID: providerID, OrgID: ou, TemplateHandle: "openai", Status: "CREATED",
	}).Error)
	ensureAgent(t, ou, "checkout", "proxy-owner")
	var project models.Project
	require.NoError(t, gdb.Where("org_id = ? AND name = ?", ou, "checkout").Take(&project).Error)
	handle := "proxy-" + uuid.NewString()[:8]
	proxyID := newArtifact(t, ou, models.KindLLMProxy, handle)
	require.NoError(t, gdb.Create(&models.LLMProxy{
		UUID: proxyID, OrgID: ou, ProjectUUID: project.ID, ProviderUUID: providerID, Status: "deployed",
	}).Error)
	return handle, proxyID
}

// mapLLMProxyToAgent makes the proxy an agent's LLM configuration in the default environment, and returns the
// configuration's and the environment's ids.
func mapLLMProxyToAgent(t *testing.T, ou string, proxyID uuid.UUID) (uuid.UUID, uuid.UUID) {
	t.Helper()
	gdb := db.GetDB()
	agentID := ensureAgent(t, ou, "checkout", "proxy-owner")
	now := time.Now().UTC().Truncate(time.Microsecond)
	cfg := models.AgentConfiguration{
		UUID: uuid.Must(uuid.NewV7()), Name: "cfg-" + uuid.NewString()[:8], OUID: ou, AgentUUID: agentID, Type: "llm",
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, gdb.Omit(clause.Associations).Create(&cfg).Error)
	envID := dbtest.Environment(t, gdb, ou)
	require.NoError(t, gdb.Omit(clause.Associations).Create(&models.EnvAgentModelMapping{
		ConfigUUID: cfg.UUID, EnvironmentUUID: envID, OrgID: ou, LLMProxyUUID: proxyID, CreatedAt: now,
	}).Error)
	return cfg.UUID, envID
}
