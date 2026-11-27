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
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/coreext"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func strp(s string) *string { return &s }

// A previously unseen organization gets its default environment on first use, and only once. No organization row
// is written: the organization is the token's.
func TestEnsureOrgCreatesDefaultEnvironmentOnce(t *testing.T) {
	gdb := db.GetDB()
	repo := NewPlatformRepo(gdb)
	ou := uuid.Must(uuid.NewV7()).String()
	t.Cleanup(func() { deleteOrg(t, ou) })
	ctx := orgctx.WithResolvedOrg(context.Background(), orgctx.ResolvedOrg{OUID: ou, OuHandle: "acme-" + ou[len(ou)-8:]})

	id, err := EnsureOrg(ctx, gdb, ou)
	require.NoError(t, err)
	assert.Equal(t, ou, id.String())
	again, err := EnsureOrg(context.Background(), gdb, ou)
	require.NoError(t, err)
	assert.Equal(t, id, again)

	envs, err := repo.ListEnvironments(ctx, ou)
	require.NoError(t, err)
	require.Len(t, envs, 1)
	assert.Equal(t, models.DefaultEnvironmentName, envs[0].Name)
	env, err := repo.GetEnvironment(ctx, ou, "anything")
	require.NoError(t, err)
	assert.Equal(t, envs[0].UUID, env.UUID, "the name is ignored: there is one environment")
}

// The list is the request's organization; a request with none is an error, not an empty success.
func TestListOrganizationsNeedsTheRequestOrg(t *testing.T) {
	repo := NewPlatformRepo(db.GetDB())
	_, err := repo.ListOrganizations(context.Background())
	require.ErrorIs(t, err, utils.ErrOrganizationNotFound)

	ou := uuid.Must(uuid.NewV7()).String()
	t.Cleanup(func() { deleteOrg(t, ou) })
	ctx := orgctx.WithResolvedOrg(context.Background(), orgctx.ResolvedOrg{OUID: ou, OuHandle: "acme"})
	orgs, err := repo.ListOrganizations(ctx)
	require.NoError(t, err)
	require.Len(t, orgs, 1)
	assert.Equal(t, "acme", orgs[0].Handle)
}

func TestEnsureOrgRejectsNonUUID(t *testing.T) {
	for _, bad := range []string{"", "not-a-uuid", "acme"} {
		_, err := EnsureOrg(context.Background(), db.GetDB(), bad)
		require.ErrorIs(t, err, utils.ErrOrganizationNotFound, "%q", bad)
	}
}

// Concurrent first requests for one organization all succeed; exactly one default environment is created.
func TestEnsureOrgConcurrentFirstUse(t *testing.T) {
	gdb := db.GetDB()
	ou := uuid.Must(uuid.NewV7()).String()
	t.Cleanup(func() { deleteOrg(t, ou) })

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = EnsureOrg(context.Background(), gdb, ou)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	var envs int64
	require.NoError(t, gdb.Model(&models.Environment{}).Where("org_id = ?", ou).Count(&envs).Error)
	assert.Equal(t, int64(1), envs)
}

func TestPlatformRepoProjects(t *testing.T) {
	ctx := context.Background()
	repo := NewPlatformRepo(db.GetDB())
	ou := newOrg(t)

	p, err := repo.CreateProject(ctx, ou, CreateProjectInput{Name: "alpha", Description: "first", CreatedBy: "u1"})
	require.NoError(t, err)
	assert.Equal(t, "alpha", p.DisplayName, "display name defaults to the name")
	assert.Equal(t, "first", p.Description)
	_, err = repo.CreateProject(ctx, ou, CreateProjectInput{Name: "beta", DisplayName: "Beta"})
	require.NoError(t, err)

	_, err = repo.CreateProject(ctx, ou, CreateProjectInput{Name: "alpha"})
	require.ErrorIs(t, err, utils.ErrProjectAlreadyExists)

	// Another organization may reuse the name.
	other := newOrg(t)
	_, err = repo.CreateProject(ctx, other, CreateProjectInput{Name: "alpha"})
	require.NoError(t, err)

	list, err := repo.ListProjects(ctx, ou)
	require.NoError(t, err)
	names := []string{}
	for _, p := range list {
		names = append(names, p.Name)
	}
	assert.Equal(t, []string{"alpha", "beta"}, names, "listed in creation order, only this org's")

	require.NoError(t, repo.PatchProject(ctx, ou, "alpha", strp("Alpha"), nil))
	got, err := repo.GetProject(ctx, ou, "alpha")
	require.NoError(t, err)
	assert.Equal(t, "Alpha", got.DisplayName)
	assert.Equal(t, "first", got.Description, "an absent field is left alone")

	require.NoError(t, repo.PatchProject(ctx, ou, "alpha", nil, strp("")))
	got, err = repo.GetProject(ctx, ou, "alpha")
	require.NoError(t, err)
	assert.Empty(t, got.Description, "an empty description clears it")

	_, err = repo.GetProject(ctx, ou, "missing")
	require.ErrorIs(t, err, utils.ErrProjectNotFound)
	require.ErrorIs(t, repo.PatchProject(ctx, ou, "missing", nil, nil), utils.ErrProjectNotFound)
	require.ErrorIs(t, repo.DeleteProject(ctx, ou, "missing"), utils.ErrProjectNotFound)
	_, err = repo.GetProject(ctx, other, "beta")
	require.ErrorIs(t, err, utils.ErrProjectNotFound, "one org cannot read another's project")
}

func TestPlatformRepoDeleteProjectRefusedWhileItHasAgents(t *testing.T) {
	ctx := context.Background()
	repo := NewPlatformRepo(db.GetDB())
	ou := newOrg(t)
	_, err := repo.CreateProject(ctx, ou, CreateProjectInput{Name: "busy"})
	require.NoError(t, err)
	_, err = repo.CreateAgent(ctx, ou, "busy", CreateAgentInput{Name: "a1", ProvisioningType: "internal"})
	require.NoError(t, err)

	err = repo.DeleteProject(ctx, ou, "busy")
	require.ErrorIs(t, err, httpx.ErrConflict)
	assert.Contains(t, err.Error(), "still has 1 agent(s)")
	_, err = repo.GetProject(ctx, ou, "busy")
	require.NoError(t, err, "the project is still there")

	require.NoError(t, repo.DeleteAgent(ctx, ou, "busy", "a1"))
	require.NoError(t, repo.DeleteProject(ctx, ou, "busy"))
	_, err = repo.GetProject(ctx, ou, "busy")
	require.ErrorIs(t, err, utils.ErrProjectNotFound)
}

func TestPlatformRepoAgents(t *testing.T) {
	ctx := context.Background()
	gdb := db.GetDB()
	repo := NewPlatformRepo(gdb)
	ou := newOrg(t)
	for _, p := range []string{"p1", "p2"} {
		_, err := repo.CreateProject(ctx, ou, CreateProjectInput{Name: p})
		require.NoError(t, err)
	}

	a, err := repo.CreateAgent(ctx, ou, "p1", CreateAgentInput{
		Name: "bot", ProvisioningType: "external", Labels: map[string]string{"team": "core", "tier": ""},
	})
	require.NoError(t, err)
	assert.Equal(t, "bot", a.DisplayName)
	assert.Equal(t, map[string]string{"team": "core", "tier": ""}, a.Labels)
	assert.Equal(t, "p1", a.ProjectName)

	_, err = repo.CreateAgent(ctx, ou, "p1", CreateAgentInput{Name: "bot2", DisplayName: "Bot 2", ProvisioningType: "internal"})
	require.NoError(t, err)

	// Agent names are unique across the organization, not per project.
	_, err = repo.CreateAgent(ctx, ou, "p2", CreateAgentInput{Name: "bot", ProvisioningType: "internal"})
	require.ErrorIs(t, err, utils.ErrAgentAlreadyExists)
	_, err = repo.CreateAgent(ctx, ou, "nope", CreateAgentInput{Name: "x", ProvisioningType: "internal"})
	require.ErrorIs(t, err, utils.ErrProjectNotFound)

	// The schema refuses an unknown provisioning type.
	_, err = repo.CreateAgent(ctx, ou, "p1", CreateAgentInput{Name: "weird", ProvisioningType: "hosted"})
	require.Error(t, err)
	assert.False(t, errors.Is(err, utils.ErrAgentAlreadyExists))

	list, err := repo.ListAgents(ctx, ou, "p1")
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "bot2", list[0].Name, "newest first")
	n, err := repo.CountProjectAgents(ctx, ou, "p1")
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	n, err = repo.CountProjectAgents(ctx, ou, "p2")
	require.NoError(t, err)
	assert.Zero(t, n)
	_, err = repo.CountProjectAgents(ctx, ou, "nope")
	require.ErrorIs(t, err, utils.ErrProjectNotFound)
	_, err = repo.ListAgents(ctx, ou, "nope")
	require.ErrorIs(t, err, utils.ErrProjectNotFound)

	exists, err := repo.AgentExists(ctx, ou, "p1", "bot")
	require.NoError(t, err)
	assert.True(t, exists)
	exists, err = repo.AgentExists(ctx, ou, "p2", "bot")
	require.NoError(t, err)
	assert.False(t, exists, "the agent lives in p1, not p2")
	_, err = repo.AgentExists(ctx, ou, "nope", "bot")
	require.ErrorIs(t, err, utils.ErrProjectNotFound, "a missing project is an error, not false")

	// Labels: nil keeps them, a map replaces them, an empty map clears them.
	require.NoError(t, repo.UpdateAgentBasicInfo(ctx, ou, "p1", "bot", "Bot", "desc", nil, nil))
	got, err := repo.GetAgent(ctx, ou, "p1", "bot")
	require.NoError(t, err)
	assert.Equal(t, "Bot", got.DisplayName)
	assert.Equal(t, "desc", got.Description)
	assert.Equal(t, map[string]string{"team": "core", "tier": ""}, got.Labels)

	require.NoError(t, repo.UpdateAgentBasicInfo(ctx, ou, "p1", "bot", "Bot", "", &map[string]string{"env": "prod"}, nil))
	got, err = repo.GetAgent(ctx, ou, "p1", "bot")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"env": "prod"}, got.Labels)
	assert.Empty(t, got.Description)

	require.NoError(t, repo.UpdateAgentBasicInfo(ctx, ou, "p1", "bot", "Bot", "", &map[string]string{}, nil))
	got, err = repo.GetAgent(ctx, ou, "p1", "bot")
	require.NoError(t, err)
	assert.Empty(t, got.Labels)

	require.ErrorIs(t, repo.UpdateAgentBasicInfo(ctx, ou, "p1", "ghost", "", "", nil, nil), utils.ErrAgentNotFound)
	require.ErrorIs(t, repo.CheckDeleteAgent(ctx, ou, "p1", "ghost"), utils.ErrAgentNotFound)
	require.NoError(t, repo.CheckDeleteAgent(ctx, ou, "p1", "bot"))

	// Deleting an agent removes its labels with it (ON DELETE CASCADE).
	require.NoError(t, repo.UpdateAgentBasicInfo(ctx, ou, "p1", "bot", "Bot", "", &map[string]string{"k": "v"}, nil))
	require.NoError(t, repo.DeleteAgent(ctx, ou, "p1", "bot"))
	_, err = repo.GetAgent(ctx, ou, "p1", "bot")
	require.ErrorIs(t, err, utils.ErrAgentNotFound)
	var labels int64
	require.NoError(t, gdb.Model(&models.AgentLabel{}).Where("org_id = ?", ou).Count(&labels).Error)
	assert.Zero(t, labels)
	require.ErrorIs(t, repo.DeleteAgent(ctx, ou, "p1", "bot"), utils.ErrAgentNotFound)
}

// recordingHost is a module host whose hooks record what they see and can refuse one phase.
type recordingHost struct {
	mu       sync.Mutex
	refuse   modkit.Phase
	phases   []string
	events   []modkit.Event
	objects  []map[string]any
	withTx   []bool
	checkErr error
}

func (h *recordingHost) Capability(string) (any, bool)                   { return nil, false }
func (h *recordingHost) Begin(context.Context, string) coreext.Operation { return recordingOp{h} }

type recordingOp struct{ h *recordingHost }

func (o recordingOp) Check(map[string]json.RawMessage) error { return o.h.checkErr }

func (o recordingOp) Run(_ context.Context, tx *gorm.DB, phase modkit.Phase, req modkit.HookRequest, _ map[string]json.RawMessage) error {
	o.h.mu.Lock()
	defer o.h.mu.Unlock()
	o.h.phases = append(o.h.phases, req.Action+":"+string(phase))
	o.h.objects = append(o.h.objects, req.Object)
	o.h.withTx = append(o.h.withTx, tx != nil)
	if phase == o.h.refuse {
		return modkit.Reject("refused by module")
	}
	return nil
}

func (o recordingOp) Emit(_ context.Context, _ *gorm.DB, e modkit.Event) error {
	o.h.mu.Lock()
	defer o.h.mu.Unlock()
	o.h.events = append(o.h.events, e)
	return nil
}

func TestPlatformRepoAgentHooksAndEvents(t *testing.T) {
	ctx := context.Background()
	repo := NewPlatformRepo(db.GetDB())
	ou := newOrg(t)
	_, err := repo.CreateProject(ctx, ou, CreateProjectInput{Name: "hooks"})
	require.NoError(t, err)

	h := &recordingHost{}
	coreext.Set(h)
	t.Cleanup(func() { coreext.Set(nil) })

	_, err = repo.CreateAgent(ctx, ou, "hooks", CreateAgentInput{Name: "a", ProvisioningType: "internal", Labels: map[string]string{"x": "1"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"create:validate", "create:apply"}, h.phases)
	_, hasID := h.objects[0]["id"]
	assert.False(t, hasID, "validate sees the submitted agent, before it has an id")
	assert.NotEmpty(t, h.objects[1]["id"], "apply sees the stored agent")
	assert.Equal(t, map[string]string{"x": "1"}, h.objects[1]["labels"])
	require.Len(t, h.events, 1)
	assert.Equal(t, modkit.EventAgentCreated, h.events[0].Type)
	assert.Equal(t, "a", h.events[0].Subject)
	assert.Equal(t, ou, h.events[0].OrgID)

	// CheckDeleteAgent runs validate outside any transaction; DeleteAgent finalizes and emits.
	h.phases, h.withTx = nil, nil
	require.NoError(t, repo.CheckDeleteAgent(ctx, ou, "hooks", "a"))
	assert.Equal(t, []string{"delete:validate"}, h.phases)
	assert.Equal(t, []bool{false}, h.withTx)
	require.NoError(t, repo.DeleteAgent(ctx, ou, "hooks", "a"))
	assert.Equal(t, modkit.EventAgentDeleted, h.events[len(h.events)-1].Type)

	// A finalize failure cannot stop a delete.
	_, err = repo.CreateAgent(ctx, ou, "hooks", CreateAgentInput{Name: "b", ProvisioningType: "internal"})
	require.NoError(t, err)
	h.refuse = modkit.Finalize
	require.NoError(t, repo.DeleteAgent(ctx, ou, "hooks", "b"))
	_, err = repo.GetAgent(ctx, ou, "hooks", "b")
	require.ErrorIs(t, err, utils.ErrAgentNotFound)
}

func TestPlatformRepoModuleRefusalRollsBack(t *testing.T) {
	ctx := context.Background()
	gdb := db.GetDB()
	repo := NewPlatformRepo(gdb)
	ou := newOrg(t)
	_, err := repo.CreateProject(ctx, ou, CreateProjectInput{Name: "guarded"})
	require.NoError(t, err)

	h := &recordingHost{}
	coreext.Set(h)
	t.Cleanup(func() { coreext.Set(nil) })

	// Refused at apply: the agent row and its labels were written in the transaction and must be gone.
	h.refuse = modkit.Apply
	_, err = repo.CreateAgent(ctx, ou, "guarded", CreateAgentInput{Name: "nope", ProvisioningType: "internal", Labels: map[string]string{"k": "v"}})
	var mErr *modkit.Error
	require.ErrorAs(t, err, &mErr)
	assert.Equal(t, "refused by module", mErr.Message)
	exists, err := repo.AgentExists(ctx, ou, "guarded", "nope")
	require.NoError(t, err)
	assert.False(t, exists)
	var labels int64
	require.NoError(t, gdb.Model(&models.AgentLabel{}).Where("org_id = ?", ou).Count(&labels).Error)
	assert.Zero(t, labels)
	assert.Empty(t, h.events, "no event for a rolled-back create")

	// A rejected extension block stops the create before any hook runs.
	h.refuse, h.phases = "", nil
	h.checkErr = modkit.Reject("extensions.x: no module named \"x\" is active")
	_, err = repo.CreateAgent(ctx, ou, "guarded", CreateAgentInput{Name: "ext", ProvisioningType: "internal"})
	require.ErrorAs(t, err, &mErr)
	assert.Empty(t, h.phases)
	h.checkErr = nil

	// Refused update at apply leaves the old display name and labels.
	_, err = repo.CreateAgent(ctx, ou, "guarded", CreateAgentInput{Name: "kept", ProvisioningType: "internal", Labels: map[string]string{"old": "1"}})
	require.NoError(t, err)
	h.refuse = modkit.Apply
	err = repo.UpdateAgentBasicInfo(ctx, ou, "guarded", "kept", "Changed", "", &map[string]string{"new": "2"}, nil)
	require.ErrorAs(t, err, &mErr)
	got, err := repo.GetAgent(ctx, ou, "guarded", "kept")
	require.NoError(t, err)
	assert.Equal(t, "kept", got.DisplayName)
	assert.Equal(t, map[string]string{"old": "1"}, got.Labels)

	// A module holding rows of the project refuses its delete at finalize; the project stays.
	h.refuse = modkit.Finalize
	_, err = repo.CreateProject(ctx, ou, CreateProjectInput{Name: "held"})
	require.NoError(t, err)
	require.ErrorAs(t, repo.DeleteProject(ctx, ou, "held"), &mErr)
	_, err = repo.GetProject(ctx, ou, "held")
	require.NoError(t, err)
}

func TestIsDefaultEnvironment(t *testing.T) {
	assert.True(t, IsDefaultEnvironment(""))
	assert.True(t, IsDefaultEnvironment(models.DefaultEnvironmentName))
	assert.False(t, IsDefaultEnvironment("staging"))
}

// deleteOrg removes what an organization created outside dbtest.Org got: its default environment.
func deleteOrg(t *testing.T, ou string) {
	t.Helper()
	gdb := db.GetDB()
	require.NoError(t, gdb.Where("org_id = ?", ou).Delete(&models.Environment{}).Error)
}
