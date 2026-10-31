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

package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// memRepo is an in-memory AgentThunderClientRepository with the compare and
// claim semantics of the real one, for tests of the promotion and sync flows. A
// method a test does not need panics on the nil embed.
type memRepo struct {
	repositories.AgentThunderClientRepository
	mu   sync.Mutex
	rows []*models.AgentThunderClient
	// skew moves the clock the due queries use, so a test can let the debounce
	// pass without waiting for it.
	skew time.Duration

	// noClaim makes every claim lose, so a background attempt does nothing.
	noClaim     bool
	claims      int
	marks       int
	deferrals   []deferral
	updates     []repositories.AgentThunderAttemptUpdate
	failedCalls int
}

type deferral struct {
	id   uuid.UUID
	next time.Time
	note string
}

func newMemRepo() *memRepo { return &memRepo{} }

func (r *memRepo) add(row models.AgentThunderClient) *models.AgentThunderClient {
	r.mu.Lock()
	defer r.mu.Unlock()
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	if row.OUID == "" {
		row.OUID = "org-id"
	}
	if row.ProjectName == "" {
		row.ProjectName = "default"
	}
	if row.ProvisioningType == "" {
		row.ProvisioningType = models.AgentProvisioningTypeInternal
	}
	r.rows = append(r.rows, &row)
	return &row
}

func (r *memRepo) find(agent, env string) *models.AgentThunderClient {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.rows {
		if row.AgentName == agent && row.EnvironmentName == env {
			c := *row
			return &c
		}
	}
	return nil
}

func (r *memRepo) byID(id uuid.UUID) *models.AgentThunderClient {
	for _, row := range r.rows {
		if row.ID == id {
			return row
		}
	}
	return nil
}

func (r *memRepo) now() time.Time { return time.Now().Add(r.skew) }

func (r *memRepo) Upsert(_ context.Context, c *models.AgentThunderClient) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.rows {
		if row.OUID == c.OUID && row.ProjectName == c.ProjectName && row.AgentName == c.AgentName && row.EnvironmentName == c.EnvironmentName {
			return nil
		}
	}
	if c.ID == uuid.Nil {
		c.ID = uuid.New() // the database fills it in, as the real repository's insert does
	}
	row := *c
	r.rows = append(r.rows, &row)
	return nil
}

func (r *memRepo) FindByAgent(_ context.Context, ouID, project, agent string) ([]models.AgentThunderClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []models.AgentThunderClient
	for _, row := range r.rows {
		if row.OUID == ouID && row.ProjectName == project && row.AgentName == agent {
			out = append(out, *row)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EnvironmentName < out[j].EnvironmentName })
	return out, nil
}

func (r *memRepo) FindByOuAndEnvironment(_ context.Context, ouID, env string) ([]models.AgentThunderClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []models.AgentThunderClient{}
	for _, row := range r.rows {
		if row.OUID == ouID && row.EnvironmentName == env {
			out = append(out, *row)
		}
	}
	return out, nil
}

func (r *memRepo) Get(_ context.Context, ouID, project, agent, env string) (*models.AgentThunderClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.rows {
		if row.OUID == ouID && row.ProjectName == project && row.AgentName == agent && row.EnvironmentName == env {
			c := *row
			return &c, nil
		}
	}
	return nil, repositories.ErrAgentThunderClientNotFound
}

func (r *memRepo) ClaimForAttempt(_ context.Context, id uuid.UUID) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.byID(id)
	if r.noClaim || row == nil || row.Status != models.AgentThunderStatusPending {
		return false, nil
	}
	row.Status = models.AgentThunderStatusInProgress
	now := time.Now()
	row.LastAttemptedAt = &now
	r.claims++
	return true, nil
}

func (r *memRepo) UpdateAfterAttempt(_ context.Context, id uuid.UUID, f repositories.AgentThunderAttemptUpdate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates = append(r.updates, f)
	row := r.byID(id)
	row.Status = f.Status
	row.LastError = f.LastError
	row.NextRetryAt = f.NextRetryAt
	row.AttemptCount++
	if f.ThunderAgentID != nil {
		row.ThunderAgentID = *f.ThunderAgentID
	}
	if f.ThunderClientID != nil {
		row.ThunderClientID = *f.ThunderClientID
	}
	if f.SecretRefPath != nil {
		row.SecretRefPath = *f.SecretRefPath
	}
	if f.Status == models.AgentThunderStatusFailed {
		r.failedCalls++
	}
	return nil
}

func (r *memRepo) MarkSyncAfter(_ context.Context, ids []uuid.UUID, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.marks++
	for _, id := range ids {
		row := r.byID(id)
		if row == nil || row.PromotionSource == "" || (row.Status != models.AgentThunderStatusCompleted && row.Status != models.AgentThunderStatusInProgress) {
			continue
		}
		t := at
		row.SyncAfter = &t
	}
	return nil
}

func (r *memRepo) FindSyncDue(_ context.Context, _ time.Time, limit int) ([]models.AgentThunderClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []models.AgentThunderClient
	for _, row := range r.rows {
		if row.Status == models.AgentThunderStatusCompleted && row.SyncAfter != nil && !row.SyncAfter.After(r.now()) {
			out = append(out, *row)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *memRepo) ClaimSync(_ context.Context, id uuid.UUID, observed, leaseUntil time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.byID(id)
	if row == nil || row.Status != models.AgentThunderStatusCompleted || row.SyncAfter == nil || !row.SyncAfter.Equal(observed) {
		return false, nil
	}
	t := leaseUntil
	row.SyncAfter = &t
	return true, nil
}

func (r *memRepo) FinishSync(_ context.Context, id uuid.UUID, claimed time.Time, retryAt *time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.byID(id)
	if row == nil || row.SyncAfter == nil || !row.SyncAfter.Equal(claimed) {
		return false, nil
	}
	row.SyncAfter = retryAt
	return true, nil
}

func (r *memRepo) Defer(_ context.Context, id uuid.UUID, next time.Time, note string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.setNext(id, models.AgentThunderStatusPending, next, note)
}

func (r *memRepo) ReleaseClaim(_ context.Context, id uuid.UUID, next time.Time, note string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.setNext(id, models.AgentThunderStatusInProgress, next, note)
}

func (r *memRepo) setNext(id uuid.UUID, from models.AgentThunderStatus, next time.Time, note string) error {
	row := r.byID(id)
	if row == nil || row.Status != from {
		return repositories.ErrAgentThunderClientNotFound
	}
	row.Status = models.AgentThunderStatusPending
	row.NextRetryAt = &next
	row.LastError = note
	r.deferrals = append(r.deferrals, deferral{id: id, next: next, note: note})
	return nil
}

func (r *memRepo) deferralCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.deferrals)
}

func (r *memRepo) SetLastError(_ context.Context, id uuid.UUID, note string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID(id).LastError = note
	return nil
}

func (r *memRepo) ListHomeAgents(_ context.Context, after *repositories.AgentRef, limit int) ([]repositories.AgentRef, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := map[string]bool{}
	var refs []repositories.AgentRef
	for _, row := range r.rows {
		key := row.OUID + "|" + row.ProjectName + "|" + row.AgentName
		if row.Status != models.AgentThunderStatusCompleted || row.PromotionSource != "" || seen[key] {
			continue
		}
		seen[key] = true
		refs = append(refs, repositories.AgentRef{OUID: row.OUID, ProjectName: row.ProjectName, AgentName: row.AgentName, ProvisioningType: row.ProvisioningType})
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].OUID+"|"+refs[i].ProjectName+"|"+refs[i].AgentName < refs[j].OUID+"|"+refs[j].ProjectName+"|"+refs[j].AgentName
	})
	var out []repositories.AgentRef
	for _, ref := range refs {
		if after != nil && ref.OUID+"|"+ref.ProjectName+"|"+ref.AgentName <= after.OUID+"|"+after.ProjectName+"|"+after.AgentName {
			continue
		}
		out = append(out, ref)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

// identityWorld is the first environment's identity data: roles, groups and who
// holds what. client() is the identity client over it.
type identityWorld struct {
	mu        sync.Mutex
	roleName  map[string]string
	roleScope map[string][]string // role id -> "<proxy>:<action>" scopes
	roleAgent map[string][]string // role id -> agents assigned directly
	roleGroup map[string][]string // role id -> groups assigned
	groups    map[string][]string // group id -> agent members
	failReads error
}

func newIdentityWorld() *identityWorld {
	return &identityWorld{
		roleName: map[string]string{}, roleScope: map[string][]string{}, roleAgent: map[string][]string{},
		roleGroup: map[string][]string{}, groups: map[string][]string{},
	}
}

func (w *identityWorld) role(id, name string, scopes []string, agents []string, groups []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.roleName[id], w.roleScope[id], w.roleAgent[id], w.roleGroup[id] = name, scopes, agents, groups
}

func worldContains(values []string, v string) bool {
	for _, value := range values {
		if value == v {
			return true
		}
	}
	return false
}

// rolesOf lists the roles an agent holds, directly or through a group.
func (w *identityWorld) rolesOf(agent string) []string {
	var out []string
	for id := range w.roleName {
		held := worldContains(w.roleAgent[id], agent)
		for _, g := range w.roleGroup[id] {
			held = held || worldContains(w.groups[g], agent)
		}
		if held {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func (w *identityWorld) client() *clientmocks.EnvIdentityClientMock {
	return &clientmocks.EnvIdentityClientMock{
		GetDefaultOUIDFunc: func(context.Context) (string, error) { return "thunder-ou", nil },
		GetAgentRoleAssignmentsFunc: func(_ context.Context, roleID string) (*thundersvc.AgentRoleAssignments, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			if w.failReads != nil {
				return nil, w.failReads
			}
			out := &thundersvc.AgentRoleAssignments{}
			for _, a := range w.roleAgent[roleID] {
				out.Agents = append(out.Agents, thundersvc.AssignmentEntry{ID: a, Type: "agent"})
			}
			for _, g := range w.roleGroup[roleID] {
				out.Groups = append(out.Groups, thundersvc.ThunderGroup{ID: g})
			}
			return out, nil
		},
		ListGroupMemberEntriesFunc: func(_ context.Context, groupID string, offset, limit int) ([]thundersvc.GroupMember, int, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			if w.failReads != nil {
				return nil, 0, w.failReads
			}
			var members []thundersvc.GroupMember
			for _, a := range w.groups[groupID] {
				members = append(members, thundersvc.GroupMember{ID: a, Type: "agent"})
			}
			members = append(members, thundersvc.GroupMember{ID: "user-1", Type: "user"})
			return members, len(members), nil
		},
		GetAgentRolesFunc: func(_ context.Context, agent string) ([]thundersvc.ThunderRole, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			var out []thundersvc.ThunderRole
			for _, id := range w.rolesOf(agent) {
				held := worldContains(w.roleAgent[id], agent)
				if held {
					out = append(out, thundersvc.ThunderRole{ID: id, Name: w.roleName[id]})
				}
			}
			return out, nil
		},
		GetAgentGroupsFunc: func(_ context.Context, _, agent string) ([]thundersvc.ThunderGroup, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			var out []thundersvc.ThunderGroup
			for id, members := range w.groups {
				if worldContains(members, agent) {
					out = append(out, thundersvc.ThunderGroup{ID: id, Name: id})
				}
			}
			sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
			return out, nil
		},
		GetGroupRolesFunc: func(_ context.Context, group string) ([]thundersvc.ThunderRole, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			var out []thundersvc.ThunderRole
			for id := range w.roleName {
				if worldContains(w.roleGroup[id], group) {
					out = append(out, thundersvc.ThunderRole{ID: id, Name: w.roleName[id]})
				}
			}
			return out, nil
		},
		ListRolesFunc: func(_ context.Context, _ string, _, _ int) ([]thundersvc.ThunderRole, int, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			var out []thundersvc.ThunderRole
			for id, name := range w.roleName {
				out = append(out, thundersvc.ThunderRole{
					ID: id, Name: name,
					Permissions: []thundersvc.RolePermissionRequest{{ResourceServerID: "rs", Permissions: w.roleScope[id]}},
				})
			}
			return out, len(out), nil
		},
	}
}

// resources is what a promotion of the agent would carry right now: every role it
// holds with its current name, and the resource server of every proxy a role
// references. It reads the world when it is called, like the real resolver.
func (w *identityWorld) resources(agent string) AgentIdentityResources {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out AgentIdentityResources
	seen := map[string]bool{}
	for _, id := range w.rolesOf(agent) {
		out.Roles = append(out.Roles, PromotedIdentityResource{ID: id, Name: w.roleName[id]})
		for _, scope := range w.roleScope[id] {
			if handle, ok := scopeProxyHandle(scope); ok && !seen[handle] {
				seen[handle] = true
				out.MCPResourceServers = append(out.MCPResourceServers, MCPResourceServerPromotion{ProxyHandle: handle, DisplayName: handle})
			}
		}
	}
	for id, members := range w.groups {
		if worldContains(members, agent) {
			out.Groups = append(out.Groups, PromotedIdentityResource{ID: id, Name: id})
		}
	}
	return out
}

// lockedRecorder keeps every audit event, safely under concurrent syncs.
type lockedRecorder struct {
	audit.Recorder
	mu     sync.Mutex
	events []audit.Event
}

func (c *lockedRecorder) Record(_ context.Context, e audit.Event) {
	c.mu.Lock()
	c.events = append(c.events, e)
	c.mu.Unlock()
}

func (c *lockedRecorder) RecordSync(_ context.Context, e audit.Event) error {
	c.Record(context.Background(), e)
	return nil
}

func (c *lockedRecorder) all() []audit.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]audit.Event(nil), c.events...)
}

// syncFixture wires a provisioning service for a deployment whose staging and
// production environments receive their identity by promotion from development.
type syncFixture struct {
	svc        *agentThunderProvisioningService
	repo       *memRepo
	world      *identityWorld
	store      *clientmocks.SecretManagementClientMock
	recorder   *lockedRecorder
	logs       *bytes.Buffer
	mu         sync.Mutex
	promotions []AgentIdentityPromotion
	managerErr func(AgentIdentityPromotion) error
	events     []string
}

func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()
	f := &syncFixture{repo: newMemRepo(), world: newIdentityWorld(), recorder: &lockedRecorder{Recorder: audit.NewNoopRecorder()}, logs: &bytes.Buffer{}}
	resolver := placementResolver{
		EnvThunderResolverMock: &clientmocks.EnvThunderResolverMock{
			ResolveIdentityFunc: func(_ context.Context, _, _, env string) (thundersvc.EnvIdentityClient, error) {
				if env != "dev" {
					return nil, thundersvc.ErrIdentityEnvironmentUnsupported
				}
				return f.world.client(), nil
			},
		},
		byPromotion: map[string]bool{"stage": true, "prod": true},
	}
	f.store = &clientmocks.SecretManagementClientMock{
		PatchSecretFunc: patchSecretNotFound,
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			f.record("store")
			return "stored-reference", nil
		},
	}
	f.svc = NewAgentThunderProvisioningService(f.repo, resolver, f.store, nil, slog.New(slog.NewTextHandler(f.logs, nil))).(*agentThunderProvisioningService)
	f.svc.triggerRunner = func(fn func()) { fn() }
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(_ context.Context, p AgentIdentityPromotion) error {
		f.record("manager")
		f.mu.Lock()
		f.promotions = append(f.promotions, p)
		fail := f.managerErr
		f.mu.Unlock()
		if fail != nil {
			return fail(p)
		}
		return nil
	}))
	f.svc.SetAgentIdentityResourceResolver(resourceResolverFunc(func(_ context.Context, q AgentIdentityResourceQuery) (AgentIdentityResources, error) {
		return f.world.resources(q.ThunderAgentID), nil
	}))
	return f
}

func (f *syncFixture) record(event string) {
	f.mu.Lock()
	f.events = append(f.events, event)
	f.mu.Unlock()
}

func (f *syncFixture) ctx(t *testing.T) context.Context {
	t.Helper()
	return audit.WithRecorder(context.Background(), f.recorder)
}

// agent adds an agent with a completed identity in development and, for each of
// the given environments, a binding promoted from the one before it.
func (f *syncFixture) agent(name string, promoted map[string]models.AgentThunderStatus) {
	id := "id-" + name
	f.repo.add(models.AgentThunderClient{
		AgentName: name, EnvironmentName: "dev", ThunderAgentID: id, ThunderClientID: "client-dev-" + name,
		SecretRefPath: "ref-dev-" + name, Status: models.AgentThunderStatusCompleted, RequestedBy: "user-1",
	})
	source := "dev"
	for _, env := range []string{"stage", "prod"} {
		status, ok := promoted[env]
		if !ok {
			continue
		}
		row := models.AgentThunderClient{
			AgentName: name, EnvironmentName: env, PromotionSource: source, Status: status, RequestedBy: "user-1",
		}
		if status == models.AgentThunderStatusCompleted {
			row.ThunderAgentID, row.ThunderClientID, row.SecretRefPath = id, "client-"+env+"-"+name, "ref-"+env+"-"+name
		}
		f.repo.add(row)
		source = env
	}
}

func allCompleted() map[string]models.AgentThunderStatus {
	return map[string]models.AgentThunderStatus{"stage": models.AgentThunderStatusCompleted, "prod": models.AgentThunderStatusCompleted}
}

func allPending() map[string]models.AgentThunderStatus {
	return map[string]models.AgentThunderStatus{"stage": models.AgentThunderStatusPending, "prod": models.AgentThunderStatusPending}
}

// promotedTo returns the promotions that went to env, by agent id.
func (f *syncFixture) promotedTo(env string) map[string]AgentIdentityPromotion {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]AgentIdentityPromotion{}
	for _, p := range f.promotions {
		if p.TargetEnvironment == env {
			out[p.ThunderAgentID] = p
		}
	}
	return out
}

func (f *syncFixture) syncAfter(agent, env string) *time.Time {
	return f.repo.find(agent, env).SyncAfter
}

func roleNames(roles []PromotedIdentityResource) []string {
	names := make([]string, 0, len(roles))
	for _, role := range roles {
		names = append(names, role.Name)
	}
	return names
}

// --- triggers: a changed role is promoted again to every environment of every holder ---

func TestRolePermissionChangeRePromotesTheChangedRoleToEveryPipelineEnvironmentOfEveryHolder(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("direct", allCompleted())
	f.agent("grouped", allCompleted())
	f.agent("bystander", allCompleted())
	f.world.groups["travel"] = []string{"id-grouped"}
	f.world.role("role-1", "booker", []string{"booking:read"}, []string{"id-direct"}, []string{"travel"})
	ctx := f.ctx(t)

	// The role changes: a new name and a permission on another MCP server.
	f.world.role("role-1", "booker-v2", []string{"booking:read", "weather:read"}, []string{"id-direct"}, []string{"travel"})
	f.svc.RoleChanged(ctx, "org-id", "dev", "role-1")

	for _, agent := range []string{"direct", "grouped"} {
		for _, env := range []string{"stage", "prod"} {
			require.NotNil(t, f.syncAfter(agent, env), "%s in %s is marked", agent, env)
		}
	}
	require.Nil(t, f.syncAfter("bystander", "stage"), "an agent that does not hold the role is not marked")
	require.Nil(t, f.syncAfter("direct", "dev"), "the first environment is never promoted into")

	require.Zero(t, f.svc.SyncDuePromotions(ctx), "the debounce has not passed")
	require.Empty(t, f.promotions)

	f.repo.skew = identitySyncDebounce + time.Second
	require.Equal(t, 4, f.svc.SyncDuePromotions(ctx), "one promotion per agent and environment")
	f.settleSyncs(ctx) // production waits for stage's sync, then carries the new content

	for _, env := range []string{"stage", "prod"} {
		got := f.promotedTo(env)
		require.Len(t, got, 2)
		for _, agent := range []string{"id-direct", "id-grouped"} {
			promotion := got[agent]
			require.Equal(t, []string{"booker-v2"}, roleNames(promotion.Roles), "the manager received the changed role with its new content")
			require.Equal(t, []string{"booking", "weather"}, proxyHandles(promotion.MCPResourceServers), "and the resource servers it now references")
			require.NotContains(t, got, "id-bystander")
		}
	}
	for _, agent := range []string{"direct", "grouped"} {
		for _, env := range []string{"stage", "prod"} {
			require.Nil(t, f.syncAfter(agent, env), "the marker is cleared once the sync is done")
		}
	}
}

// A later environment is promoted from the one before it, which only runs the new
// content once its own sync is done. Due syncs run in parallel, so without a wait the
// last environment is promoted from a stale version, clears its marker, and never
// receives the change.
func TestALaterEnvironmentIsNotSyncedBeforeItsPredecessorHasFinished(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("direct", allCompleted())
	f.world.role("role-1", "booker", []string{"booking:read"}, []string{"id-direct"}, nil)
	ctx := f.ctx(t)

	var mu sync.Mutex
	var order, violations []string
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(_ context.Context, p AgentIdentityPromotion) error {
		if p.TargetEnvironment == "stage" {
			// Slow enough that production, which a sweep runs alongside it, would overtake it.
			time.Sleep(80 * time.Millisecond)
		}
		predecessor := f.repo.find("direct", "stage")
		mu.Lock()
		defer mu.Unlock()
		if p.TargetEnvironment == "prod" && predecessor.SyncAfter != nil {
			violations = append(violations, "production was promoted while stage still had a pending or running sync")
		}
		order = append(order, p.TargetEnvironment+":"+strings.Join(roleNames(p.Roles), ","))
		return nil
	}))

	f.world.role("role-1", "booker-v2", []string{"booking:read", "weather:read"}, []string{"id-direct"}, nil)
	f.svc.RoleChanged(ctx, "org-id", "dev", "role-1")

	f.repo.skew = identitySyncDebounce + time.Second
	for sweep := 0; sweep < 6; sweep++ {
		f.svc.SyncDuePromotions(ctx)
		f.repo.skew += predecessorRecheckDelay + time.Second
	}

	mu.Lock()
	defer mu.Unlock()
	require.Empty(t, violations)
	require.Equal(t, []string{"stage:booker-v2", "prod:booker-v2"}, order,
		"stage is promoted first, then production, and both carry the changed role")
	require.Nil(t, f.syncAfter("direct", "stage"))
	require.Nil(t, f.syncAfter("direct", "prod"), "the marker clears only after the change reached production")
}

// settleSyncs sweeps until every due sync has run. An environment waits for the one it
// is promoted from, so a pipeline of N environments needs a sweep per level.
func (f *syncFixture) settleSyncs(ctx context.Context) {
	for sweep := 0; sweep < 6; sweep++ {
		f.repo.skew += predecessorRecheckDelay + time.Second
		f.svc.SyncDuePromotions(ctx)
	}
}

func proxyHandles(servers []MCPResourceServerPromotion) []string {
	out := make([]string, 0, len(servers))
	for _, s := range servers {
		out = append(out, s.ProxyHandle)
	}
	sort.Strings(out)
	return out
}

func TestASyncNeverMarksAnythingItself(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("direct", allCompleted())
	f.agent("other", allCompleted())
	f.world.role("role-1", "booker", []string{"booking:read"}, []string{"id-direct", "id-other"}, nil)
	ctx := f.ctx(t)

	f.svc.RoleChanged(ctx, "org-id", "dev", "role-1")
	marksAfterTrigger := f.repo.marks
	f.repo.skew = identitySyncDebounce + time.Second
	require.Equal(t, 4, f.svc.SyncDuePromotions(ctx))

	require.Equal(t, marksAfterTrigger, f.repo.marks, "a re-promotion never marks other agents or itself")
	require.Zero(t, f.svc.SyncDuePromotions(ctx), "nothing is left to sync, so the loop ends")
}

func TestABurstOfChangesIsOnePromotionPerAgentAndEnvironment(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("direct", allCompleted())
	f.world.role("role-1", "booker", []string{"booking:read"}, []string{"id-direct"}, nil)
	ctx := f.ctx(t)

	for i := 0; i < 5; i++ {
		f.world.role("role-1", fmt.Sprintf("booker-%d", i), []string{"booking:read"}, []string{"id-direct"}, nil)
		f.svc.RoleChanged(ctx, "org-id", "dev", "role-1")
	}
	f.repo.skew = identitySyncDebounce + time.Second
	require.Equal(t, 2, f.svc.SyncDuePromotions(ctx), "five changes collapse into one promotion for each of the two environments")
	require.Equal(t, []string{"booker-4"}, roleNames(f.promotedTo("stage")["id-direct"].Roles), "and it carries the latest content")
}

func TestANewerRequestDuringASyncIsNeverLost(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("direct", allCompleted())
	f.world.role("role-1", "booker", []string{"booking:read"}, []string{"id-direct"}, nil)
	ctx := f.ctx(t)
	f.svc.RoleChanged(ctx, "org-id", "dev", "role-1")

	// Another change arrives while the first sync is running.
	var arrived sync.Once
	f.managerErr = func(AgentIdentityPromotion) error {
		arrived.Do(func() {
			f.world.role("role-1", "booker-v2", []string{"booking:read"}, []string{"id-direct"}, nil)
			require.NoError(t, f.svc.MarkAgentsForIdentitySync(ctx, "org-id",
				[]AgentIdentityRef{{ProjectName: "default", AgentName: "direct"}}, identitySyncDebounce))
		})
		return nil
	}
	f.repo.skew = identitySyncDebounce + time.Second
	f.svc.SyncDuePromotions(ctx)
	require.NotNil(t, f.syncAfter("direct", "stage"), "the marker moved during the sync, so it is not cleared")

	before := len(f.promotions)
	f.repo.skew = 2 * identitySyncDebounce
	f.svc.SyncDuePromotions(ctx)
	require.Greater(t, len(f.promotions), before, "the newer request is promoted by the next sync")
	require.Equal(t, []string{"booker-v2"}, roleNames(f.promotedTo("stage")["id-direct"].Roles))
	require.Nil(t, f.syncAfter("direct", "stage"))
}

func TestAnAssignmentTriggersThePromotionOfTheRoleAndItsResourceServer(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("assigned", allCompleted())
	f.agent("bystander", allCompleted())
	ctx := f.ctx(t)
	// The role exists and nobody holds it. Then the agent is assigned.
	f.world.role("role-1", "booker", []string{"booking:read"}, nil, nil)
	f.world.role("role-1", "booker", []string{"booking:read"}, []string{"id-assigned"}, nil)

	f.svc.RoleAssigneesChanged(ctx, "org-id", "dev", "role-1", []thundersvc.AssignmentEntry{{ID: "id-assigned", Type: "agent"}})

	require.NotNil(t, f.syncAfter("assigned", "stage"))
	require.Nil(t, f.syncAfter("bystander", "stage"), "only the agents whose assignment changed are marked")
	f.repo.skew = identitySyncDebounce + time.Second
	f.svc.SyncDuePromotions(ctx)
	f.settleSyncs(ctx) // production waits for stage's sync
	for _, env := range []string{"stage", "prod"} {
		promotion := f.promotedTo(env)["id-assigned"]
		require.Equal(t, []string{"booker"}, roleNames(promotion.Roles), "the role is promoted into %s", env)
		require.Equal(t, []string{"booking"}, proxyHandles(promotion.MCPResourceServers), "with its resource server")
	}
}

func TestAnAssignmentThroughAGroupMarksTheGroupsAgentMembers(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("member", allCompleted())
	f.agent("outsider", allCompleted())
	f.world.groups["travel"] = []string{"id-member"}
	ctx := f.ctx(t)

	f.svc.RoleAssigneesChanged(ctx, "org-id", "dev", "role-1", []thundersvc.AssignmentEntry{{ID: "travel", Type: "group"}})

	require.NotNil(t, f.syncAfter("member", "stage"))
	require.Nil(t, f.syncAfter("outsider", "stage"))
}

// Promotion is additive: removing an agent from a role takes nothing away in a
// later environment. The removal still marks the agent so the next promotion
// reads the carried set as it is now.
func TestUnassigningStillTriggersASyncButNothingIsRemovedInLaterEnvironments(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("direct", allCompleted())
	ctx := f.ctx(t)
	f.world.role("role-1", "booker", []string{"booking:read"}, nil, nil) // the agent was just removed

	f.svc.RoleAssigneesChanged(ctx, "org-id", "dev", "role-1", []thundersvc.AssignmentEntry{{ID: "id-direct", Type: "agent"}})
	require.NotNil(t, f.syncAfter("direct", "stage"), "an unassign is a change to the agent's carried set, so it is synced")

	f.repo.skew = identitySyncDebounce + time.Second
	require.Equal(t, 2, f.svc.SyncDuePromotions(ctx))
	require.Empty(t, f.promotedTo("stage")["id-direct"].Roles, "the role is no longer carried; the promotion has no way to remove it from the target")
}

func TestGroupChangesMarkTheAgentsOfTheGroup(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("member", allCompleted())
	f.agent("added", allCompleted())
	f.agent("outsider", allCompleted())
	f.world.groups["travel"] = []string{"id-member"}
	ctx := f.ctx(t)

	f.svc.GroupChanged(ctx, "org-id", "dev", "travel")
	require.NotNil(t, f.syncAfter("member", "stage"), "a group update marks its members")
	require.Nil(t, f.syncAfter("outsider", "stage"))

	f.svc.GroupMembersChanged(ctx, "org-id", "dev", "travel", []thundersvc.GroupMember{{ID: "id-added", Type: "agent"}, {ID: "user-1", Type: "user"}})
	require.NotNil(t, f.syncAfter("added", "stage"), "added members are marked")
}

func TestDeletingARoleOrGroupMarksTheAgentsReadBeforeTheDelete(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("direct", allCompleted())
	f.agent("member", allCompleted())
	f.world.groups["travel"] = []string{"id-member"}
	f.world.role("role-1", "booker", []string{"booking:read"}, []string{"id-direct"}, nil)
	ctx := f.ctx(t)

	afterRole := f.svc.BeforeRoleDelete(ctx, "org-id", "dev", "role-1")
	afterGroup := f.svc.BeforeGroupDelete(ctx, "org-id", "dev", "travel")
	delete(f.world.roleName, "role-1") // they are gone before the marks are written
	delete(f.world.groups, "travel")
	require.Nil(t, f.syncAfter("direct", "stage"), "nothing is marked until the delete has succeeded")

	afterRole()
	afterGroup()
	require.NotNil(t, f.syncAfter("direct", "stage"))
	require.NotNil(t, f.syncAfter("member", "stage"))
}

func TestAnMCPScopeChangeFansOutToTheHoldersOfTheRolesThatReferenceTheServer(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("booker", allCompleted())
	f.agent("grouped", allCompleted())
	f.agent("weather", allCompleted())
	f.world.groups["travel"] = []string{"id-grouped"}
	f.world.role("role-1", "booker", []string{"booking:read"}, []string{"id-booker"}, nil)
	f.world.role("role-2", "travel", []string{"booking:write"}, nil, []string{"travel"})
	f.world.role("role-3", "weather", []string{"weather:read"}, []string{"id-weather"}, nil)
	ctx := f.ctx(t)

	f.svc.MCPProxyChanged(ctx, "org-id", "dev", "booking")

	require.NotNil(t, f.syncAfter("booker", "stage"))
	require.NotNil(t, f.syncAfter("grouped", "stage"), "a holder through a group is reached too")
	require.Nil(t, f.syncAfter("weather", "stage"), "a role of another server is not")
}

func TestTriggersNeverFailOrWaitWhenTheIdentityProviderCannotBeRead(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("direct", allCompleted())
	f.world.role("role-1", "booker", nil, []string{"id-direct"}, nil)
	f.world.failReads = errors.New("identity provider down")
	ctx := f.ctx(t)

	// None of these returns an error or panics: a change never fails because of promotion.
	f.svc.RoleChanged(ctx, "org-id", "dev", "role-1")
	f.svc.GroupChanged(ctx, "org-id", "dev", "travel")
	f.svc.MCPProxyChanged(ctx, "org-id", "dev", "booking")
	f.svc.BeforeRoleDelete(ctx, "org-id", "dev", "role-1")()

	require.Nil(t, f.syncAfter("direct", "stage"))
}

func TestTriggersDoNothingWhereIdentityIsNotPromoted(t *testing.T) {
	f := newSyncFixture(t)
	f.svc.SetIdentityPromotionManager(nil)
	f.agent("direct", allCompleted())
	f.world.role("role-1", "booker", nil, []string{"id-direct"}, nil)
	ran := false
	f.svc.triggerRunner = func(func()) { ran = true }

	f.svc.RoleChanged(f.ctx(t), "org-id", "dev", "role-1")
	f.svc.RoleAssigneesChanged(f.ctx(t), "org-id", "dev", "role-1", []thundersvc.AssignmentEntry{{ID: "id-direct", Type: "agent"}})
	f.svc.MCPProxyChanged(f.ctx(t), "org-id", "dev", "booking")

	require.False(t, ran, "a deployment with direct identity management does no sync work at all")
	require.Zero(t, f.repo.marks)
}

func TestTheFanOutOfOneChangeIsBounded(t *testing.T) {
	f := newSyncFixture(t)
	var ids []string
	for i := 0; i < maxSyncAgentsPerTrigger+25; i++ {
		name := fmt.Sprintf("agent-%d", i)
		f.agent(name, allCompleted())
		ids = append(ids, "id-"+name)
	}
	f.world.role("role-1", "booker", nil, ids, nil)

	f.svc.RoleChanged(f.ctx(t), "org-id", "dev", "role-1")

	marked := 0
	for _, row := range f.repo.rows {
		if row.SyncAfter != nil && row.EnvironmentName == "stage" {
			marked++
		}
	}
	require.Equal(t, maxSyncAgentsPerTrigger, marked, "one change syncs at most the cap, and the rest is logged")
	require.Contains(t, f.logs.String(), "more agents than one change syncs")
}

// --- promotion of a new binding ---

func TestAPromotionWaitsForTheEnvironmentItIsPromotedFromWithoutUsingAnAttempt(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("scout", allPending())
	// Production is promoted from staging, which is still pending.
	prod := f.repo.find("scout", "prod")

	f.svc.AttemptProvision(f.ctx(t), *prod)

	require.Empty(t, f.promotions, "nothing is promoted before the environment it comes from is completed")
	require.Zero(t, f.repo.claims, "waiting does not claim the binding")
	require.Empty(t, f.repo.updates, "and does not consume an attempt")
	require.Len(t, f.repo.deferrals, 1)
	require.Contains(t, f.repo.deferrals[0].note, "stage")
	require.True(t, f.repo.deferrals[0].next.After(time.Now()))
	got := f.repo.find("scout", "prod")
	require.Equal(t, models.AgentThunderStatusPending, got.Status)
	require.Zero(t, got.AttemptCount)
}

func TestPromotionsRunInPipelineOrderFromEachBindingsOwnSource(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("scout", allPending())
	ctx := f.ctx(t)

	f.svc.AttemptProvision(ctx, *f.repo.find("scout", "stage"))
	f.svc.AttemptProvision(ctx, *f.repo.find("scout", "prod"))

	require.Len(t, f.promotions, 2)
	require.Equal(t, "stage", f.promotions[0].TargetEnvironment)
	require.Equal(t, "dev", f.promotions[0].SourceEnvironment)
	require.Equal(t, "prod", f.promotions[1].TargetEnvironment)
	require.Equal(t, "stage", f.promotions[1].SourceEnvironment, "production is promoted from staging, not from development")
	for _, env := range []string{"stage", "prod"} {
		require.Equal(t, models.AgentThunderStatusCompleted, f.repo.find("scout", env).Status)
	}
}

func TestTheFirstPromotionCreatesTheTargetCredentialsAndARepeatKeepsThem(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("scout", map[string]models.AgentThunderStatus{"stage": models.AgentThunderStatusPending})
	ctx := f.ctx(t)

	f.svc.AttemptProvision(ctx, *f.repo.find("scout", "stage"))

	require.Len(t, f.promotions, 1)
	first := f.promotions[0]
	require.NotEmpty(t, first.TargetClientID)
	require.NotEqual(t, "client-dev-scout", first.TargetClientID, "the target gets its own client id")
	require.NotEmpty(t, first.ClientSecret)
	row := f.repo.find("scout", "stage")
	require.Equal(t, first.TargetClientID, row.ThunderClientID)
	require.Equal(t, "stored-reference", row.SecretRefPath, "an internal agent's secret is stored")
	require.Len(t, f.store.CreateSecretCalls(), 1)

	// A later change re-promotes with the credentials the target already has.
	f.repo.skew = identitySyncDebounce + time.Second
	require.NoError(t, f.svc.MarkAgentsForIdentitySync(ctx, "org-id", []AgentIdentityRef{{ProjectName: "default", AgentName: "scout"}}, 0))
	require.Equal(t, 1, f.svc.SyncDuePromotions(ctx))
	require.Len(t, f.promotions, 2)
	second := f.promotions[1]
	require.Equal(t, first.TargetClientID, second.TargetClientID, "a repeat keeps the target's client id")
	require.Empty(t, second.ClientSecret, "and sets no new secret")
	require.Len(t, f.store.CreateSecretCalls(), 1, "the stored secret of the running agent is untouched")
}

func TestAnExternalAgentsPromotedSecretIsNeverStored(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("scout", map[string]models.AgentThunderStatus{"stage": models.AgentThunderStatusPending})
	row := f.repo.find("scout", "stage")
	f.repo.byID(row.ID).ProvisioningType = models.AgentProvisioningTypeExternal
	row.ProvisioningType = models.AgentProvisioningTypeExternal

	f.svc.AttemptProvision(f.ctx(t), *row)

	require.Len(t, f.promotions, 1)
	require.NotEmpty(t, f.promotions[0].ClientSecret, "the carried client needs a value")
	require.Empty(t, f.store.CreateSecretCalls())
	require.Empty(t, f.store.PatchSecretCalls())
	got := f.repo.find("scout", "stage")
	require.Equal(t, models.AgentThunderStatusCompleted, got.Status)
	require.Empty(t, got.SecretRefPath, "no secret reference is recorded for an external agent")
	require.Equal(t, models.AgentProvisioningTypeExternal, got.ProvisioningType)
}

func TestABackgroundPromotionIsAuditedBeforeTheCredentialIsIssued(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("scout", map[string]models.AgentThunderStatus{"stage": models.AgentThunderStatusPending})
	var auditedBeforeManager bool
	f.managerErr = func(AgentIdentityPromotion) error {
		for _, e := range f.recorder.all() {
			if e.Action == audit.ActionAgentIdentityPromote && e.Outcome == audit.OutcomeUnknown {
				auditedBeforeManager = true
			}
		}
		return nil
	}

	f.svc.AttemptProvision(f.ctx(t), *f.repo.find("scout", "stage"))

	require.True(t, auditedBeforeManager, "the intent is recorded before the target is given its credential")
	var intent, outcome *audit.Event
	events := f.recorder.all()
	for i := range events {
		e := events[i]
		if e.Action != audit.ActionAgentIdentityPromote {
			continue
		}
		if e.Outcome == audit.OutcomeUnknown {
			intent = &events[i]
		} else {
			outcome = &events[i]
		}
	}
	require.NotNil(t, intent)
	require.NotNil(t, outcome)
	require.Equal(t, audit.ActorSystem, intent.ActorType, "no request is behind a background promotion")
	require.Equal(t, systemActorAgentThunderReconciler, intent.ActorID)
	require.Equal(t, "user-1", intent.OnBehalfOf, "the user who asked for the agent is carried")
	require.Equal(t, "stage", intent.Environment)
	require.Equal(t, promotionTriggerCreation, intent.Details["trigger"])
	require.Equal(t, audit.OutcomeSuccess, outcome.Outcome)
}

func TestAPromotionIsRefusedAndNothingIsIssuedWhenTheAuditTrailIsUnavailable(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("scout", map[string]models.AgentThunderStatus{"stage": models.AgentThunderStatusPending})

	f.svc.AttemptProvision(context.Background(), *f.repo.find("scout", "stage")) // no recorder installed

	require.Empty(t, f.promotions, "no credential is issued without an audit record")
	require.Empty(t, f.store.CreateSecretCalls())
	require.NotEqual(t, models.AgentThunderStatusCompleted, f.repo.find("scout", "stage").Status)
}

// --- error classes ---

func TestUnavailableAndForbiddenFollowTheNormalRetryBudgetThenFail(t *testing.T) {
	for _, kind := range []IdentityPromotionErrorKind{IdentityPromotionUnavailable, IdentityPromotionForbidden} {
		t.Run(string(kind), func(t *testing.T) {
			f := newSyncFixture(t)
			f.agent("scout", map[string]models.AgentThunderStatus{"stage": models.AgentThunderStatusPending})
			f.managerErr = func(AgentIdentityPromotion) error {
				return &IdentityPromotionError{Kind: kind, Message: "the control plane is not reachable"}
			}
			ctx := f.ctx(t)

			for attempt := 1; attempt < maxProvisionAttempts; attempt++ {
				f.svc.AttemptProvision(ctx, *f.repo.find("scout", "stage"))
				got := f.repo.find("scout", "stage")
				require.Equal(t, models.AgentThunderStatusPending, got.Status, "attempt %d is retried", attempt)
				require.Equal(t, attempt, got.AttemptCount)
				require.NotNil(t, got.NextRetryAt)
				require.WithinDuration(t, time.Now().Add(provisionRetryDelay), *got.NextRetryAt, 5*time.Second)
				require.Contains(t, got.LastError, "control plane is not reachable")
			}
			f.svc.AttemptProvision(ctx, *f.repo.find("scout", "stage"))
			require.Equal(t, models.AgentThunderStatusFailed, f.repo.find("scout", "stage").Status, "the budget is exhausted")
		})
	}
}

func TestUserFixableRefusalsKeepRetryingOnALongerBackoffAndNeverExhaust(t *testing.T) {
	for _, kind := range []IdentityPromotionErrorKind{IdentityPromotionBlocked, IdentityPromotionConflict, IdentityPromotionMissingValues} {
		t.Run(string(kind), func(t *testing.T) {
			f := newSyncFixture(t)
			f.agent("scout", map[string]models.AgentThunderStatus{"stage": models.AgentThunderStatusPending})
			f.managerErr = func(AgentIdentityPromotion) error {
				return &IdentityPromotionError{
					Kind: kind, Message: `role "booker" references MCP server "booking", which is not deployed to "stage"`,
					MissingValues: []string{"SOME_URL"},
				}
			}
			ctx := f.ctx(t)

			// Far more attempts than the retry budget: it never becomes failed.
			for i := 0; i < 3*maxProvisionAttempts; i++ {
				row := f.repo.find("scout", "stage")
				f.svc.AttemptProvision(ctx, *row)
				got := f.repo.find("scout", "stage")
				require.Equal(t, models.AgentThunderStatusPending, got.Status, "round %d", i)
				require.NotNil(t, got.NextRetryAt)
				require.GreaterOrEqual(t, time.Until(*got.NextRetryAt), userFixableRetryFloor-5*time.Second)
				require.LessOrEqual(t, time.Until(*got.NextRetryAt), userFixableRetryCap+5*time.Second)
				require.Contains(t, got.LastError, `Role "booker" references MCP server "booking"`, "the reason is on the binding")
				require.Contains(t, got.LastError, "1 required value(s) are missing")
				require.NotContains(t, got.LastError, "SOME_URL", "value names are never shown")
			}
			require.Zero(t, f.repo.failedCalls, "a refusal the user has to fix never exhausts into a failure")
			require.Zero(t, f.repo.find("scout", "stage").AttemptCount, "and never consumes the retry budget")
		})
	}
}

func TestUserFixableRetryDelayDoublesFromTheFloorToTheCap(t *testing.T) {
	now := time.Now()
	at := func(previous time.Duration) models.AgentThunderClient {
		next := now.Add(previous)
		return models.AgentThunderClient{LastAttemptedAt: &now, NextRetryAt: &next}
	}
	require.Equal(t, userFixableRetryFloor, userFixableRetryDelay(models.AgentThunderClient{}))
	require.Equal(t, userFixableRetryFloor, userFixableRetryDelay(at(time.Minute)), "never below the floor")
	require.Equal(t, 10*time.Minute, userFixableRetryDelay(at(5*time.Minute)))
	require.Equal(t, 20*time.Minute, userFixableRetryDelay(at(10*time.Minute)))
	require.Equal(t, userFixableRetryCap, userFixableRetryDelay(at(20*time.Minute)), "capped at about thirty minutes")
	require.Equal(t, userFixableRetryCap, userFixableRetryDelay(at(24*time.Hour)))
}

func TestNoSecretOrInternalIdentifierReachesLogsOrTheBindingsNote(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("scout", map[string]models.AgentThunderStatus{"stage": models.AgentThunderStatusPending})
	var secret, clientID string
	f.managerErr = func(p AgentIdentityPromotion) error {
		secret, clientID = p.ClientSecret, p.TargetClientID
		return &IdentityPromotionError{
			Kind: IdentityPromotionUnavailable, Message: "the control plane is not reachable",
			Cause: errors.New("dial tcp 10.0.0.7:8090: connection refused"),
		}
	}

	f.svc.AttemptProvision(f.ctx(t), *f.repo.find("scout", "stage"))

	require.NotEmpty(t, secret)
	logged := f.logs.String()
	note := f.repo.find("scout", "stage").LastError
	for _, forbidden := range []string{secret, clientID, "id-scout", "client-dev-scout"} {
		require.NotContains(t, logged, forbidden, "logs carry no secret, client id or internal id")
		require.NotContains(t, note, forbidden, "neither does the note the user sees")
	}
	require.NotContains(t, note, "10.0.0.7", "the cause stays in the log")
}

// --- the cross pipeline rule ---

func TestARoleWaitsWhileAnotherAgentThatHoldsItHasNoIdentityInTheEnvironment(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("first", map[string]models.AgentThunderStatus{"stage": models.AgentThunderStatusPending})
	f.agent("second", map[string]models.AgentThunderStatus{"stage": models.AgentThunderStatusPending})
	f.world.role("role-1", "booker", []string{"booking:read"}, []string{"id-first", "id-second"}, nil)
	f.world.role("role-2", "solo", nil, []string{"id-first"}, nil)
	ctx := f.ctx(t)

	f.svc.AttemptProvision(ctx, *f.repo.find("first", "stage"))

	require.Len(t, f.promotions, 1)
	require.Equal(t, []string{"solo"}, roleNames(f.promotions[0].Roles),
		"the role another agent still waits on is held back and the rest is promoted")
	require.Equal(t, models.AgentThunderStatusCompleted, f.repo.find("first", "stage").Status)
	require.Equal(t, "1 role waits for other agents' identity in stage", f.repo.find("first", "stage").LastError)
}

func TestACompletedIdentityMarksTheAgentsThatShareARoleSoHeldRolesConverge(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("first", map[string]models.AgentThunderStatus{"stage": models.AgentThunderStatusPending})
	f.agent("second", map[string]models.AgentThunderStatus{"stage": models.AgentThunderStatusPending})
	f.world.role("role-1", "booker", []string{"booking:read"}, []string{"id-first", "id-second"}, nil)
	ctx := f.ctx(t)

	// The first agent completes with the role held back for the second.
	f.svc.AttemptProvision(ctx, *f.repo.find("first", "stage"))
	require.Empty(t, f.promotions[0].Roles)
	require.Equal(t, "1 role waits for other agents' identity in stage", f.repo.find("first", "stage").LastError)

	// The second completes. It carries the role, because the first is ready, and the
	// first is told to re-promote.
	f.svc.AttemptProvision(ctx, *f.repo.find("second", "stage"))
	require.Equal(t, []string{"booker"}, roleNames(f.promotions[1].Roles))
	require.NotNil(t, f.syncAfter("first", "stage"), "the agent that shares the role is marked")
	require.Nil(t, f.syncAfter("second", "stage"), "the agent that completed does not mark itself")

	f.repo.skew = identitySyncDebounce + time.Second
	require.Equal(t, 1, f.svc.SyncDuePromotions(ctx))
	require.Equal(t, []string{"booker"}, roleNames(f.promotions[2].Roles), "the held role is carried by the re-promotion")
	require.Empty(t, f.repo.find("first", "stage").LastError, "and the note is cleared")
	require.Zero(t, f.svc.SyncDuePromotions(ctx), "converged, with nothing left to do")
}

func TestARoleHeldBackThroughAGroupMemberWaitsForTheMember(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("first", allCompleted())
	f.agent("grouped", map[string]models.AgentThunderStatus{"stage": models.AgentThunderStatusPending})
	f.world.groups["travel"] = []string{"id-grouped"}
	f.world.role("role-1", "booker", nil, []string{"id-first"}, []string{"travel"})
	ctx := f.ctx(t)
	require.NoError(t, f.svc.MarkAgentsForIdentitySync(ctx, "org-id", []AgentIdentityRef{{ProjectName: "default", AgentName: "first"}}, 0))

	f.repo.skew = time.Second
	f.svc.SyncDuePromotions(ctx)
	require.Empty(t, f.promotedTo("stage")["id-first"].Roles, "the group's member has no identity in staging yet")
	require.Equal(t, "1 role waits for other agents' identity in stage", f.repo.find("first", "stage").LastError)
}

// --- creation and backfill ---

func pipelineOf(names ...string) []PipelineIdentityEnvironment {
	out := make([]PipelineIdentityEnvironment, 0, len(names))
	for i, name := range names {
		step := PipelineIdentityEnvironment{Name: name}
		if i > 0 {
			step.Source = names[i-1]
		}
		out = append(out, step)
	}
	return out
}

func TestTheOrderedPipelineFlattensADAGBreadthFirstAndRecordsTheSource(t *testing.T) {
	paths := func(edges ...[2]string) []models.PromotionPath {
		bySource := map[string][]models.TargetEnvironmentRef{}
		var order []string
		for _, e := range edges {
			if _, ok := bySource[e[0]]; !ok {
				order = append(order, e[0])
			}
			bySource[e[0]] = append(bySource[e[0]], models.TargetEnvironmentRef{Name: e[1]})
		}
		var out []models.PromotionPath
		for _, source := range order {
			out = append(out, models.PromotionPath{SourceEnvironmentRef: source, TargetEnvironmentRefs: bySource[source]})
		}
		return out
	}
	require.Equal(t, pipelineOf("dev", "stage", "prod"), orderedPipelineIdentityEnvironments(paths([2]string{"dev", "stage"}, [2]string{"stage", "prod"})))
	require.Equal(t, []PipelineIdentityEnvironment{
		{Name: "dev"}, {Name: "stage", Source: "dev"}, {Name: "qa", Source: "dev"}, {Name: "prod", Source: "stage"},
	}, orderedPipelineIdentityEnvironments(paths([2]string{"dev", "stage"}, [2]string{"dev", "qa"}, [2]string{"stage", "prod"})),
		"a DAG is flattened breadth first with the first predecessor recorded")
	require.Nil(t, orderedPipelineIdentityEnvironments(nil))
	require.Nil(t, orderedPipelineIdentityEnvironments(paths([2]string{"a", "b"}, [2]string{"b", "a"})), "a cycle has no first environment")
}

func TestAnAgentGetsABindingForEveryEnvironmentOfItsPipelineInOrder(t *testing.T) {
	f := newSyncFixture(t)
	f.repo.add(models.AgentThunderClient{AgentName: "other", EnvironmentName: "elsewhere"}) // unrelated
	ref := AgentIdentityRef{OUID: "org-id", ProjectName: "default", AgentName: "scout", Ownership: models.AgentProvisioningTypeExternal}
	// The background attempts that follow lose every claim and stop; the promoted
	// ones wait for the environment before them.
	f.repo.noClaim = true

	handled := f.svc.ProvisionForAgentInPipeline(f.ctx(t), ref, pipelineOf("dev", "stage", "prod"), "user-1", "")

	require.True(t, handled)
	require.Eventually(t, func() bool { return f.repo.deferralCount() == 2 }, 5*time.Second, 5*time.Millisecond,
		"the promoted bindings are attempted in the background and wait for their source")
	dev, stage, prod := f.repo.find("scout", "dev"), f.repo.find("scout", "stage"), f.repo.find("scout", "prod")
	require.NotNil(t, dev)
	require.NotNil(t, stage)
	require.NotNil(t, prod)
	require.Empty(t, dev.PromotionSource, "the first environment is provisioned directly")
	require.Equal(t, "dev", stage.PromotionSource)
	require.Equal(t, "stage", prod.PromotionSource)
	for _, row := range []*models.AgentThunderClient{dev, stage, prod} {
		require.Equal(t, models.AgentProvisioningTypeExternal, row.ProvisioningType)
		require.Equal(t, "user-1", row.RequestedBy)
	}
}

func TestAnAgentOutsideAPromotingPipelineIsProvisionedTheLegacyWay(t *testing.T) {
	f := newSyncFixture(t)
	ref := AgentIdentityRef{OUID: "org-id", ProjectName: "default", AgentName: "scout", Ownership: models.AgentProvisioningTypeInternal}

	require.False(t, f.svc.ProvisionForAgentInPipeline(f.ctx(t), ref, pipelineOf("dev", "qa"), "user-1", ""),
		"no environment of this pipeline receives its identity by promotion")
	require.Empty(t, f.repo.rows, "nothing is written, so the caller provisions as it always did")

	f.svc.SetIdentityPromotionManager(nil)
	require.False(t, f.svc.ProvisionForAgentInPipeline(f.ctx(t), ref, pipelineOf("dev", "stage"), "user-1", ""),
		"without a promotion manager the pipeline is not promoted")
	require.Empty(t, f.repo.rows)
}

func TestProvisioningFallsBackToTheLegacyWayWhenAPlacementCannotBeRead(t *testing.T) {
	f := newSyncFixture(t)
	f.svc.envResolver = placementResolver{EnvThunderResolverMock: &clientmocks.EnvThunderResolverMock{}, err: errors.New("target resolver down")}
	ref := AgentIdentityRef{OUID: "org-id", ProjectName: "default", AgentName: "scout"}

	require.False(t, f.svc.ProvisionForAgentInPipeline(f.ctx(t), ref, pipelineOf("dev", "stage"), "user-1", ""))
	require.Empty(t, f.repo.rows)
}

type fakePipelines map[string][]PipelineIdentityEnvironment

func (p fakePipelines) ProjectIdentityPipeline(_ context.Context, _, project string) ([]PipelineIdentityEnvironment, error) {
	pipeline, ok := p[project]
	if !ok {
		return nil, errors.New("project not found")
	}
	return pipeline, nil
}

func TestTheBackfillCreatesTheMissingPipelineBindingsAndIsIdempotent(t *testing.T) {
	f := newSyncFixture(t)
	// An agent created before identity followed the pipeline has its first binding only.
	f.agent("legacy", nil)
	// One whose pipeline gained a third environment has the first two.
	f.agent("grown", map[string]models.AgentThunderStatus{"stage": models.AgentThunderStatusCompleted})
	// One that already has everything, and one of a project that cannot be read.
	f.agent("complete", allCompleted())
	f.repo.add(models.AgentThunderClient{ProjectName: "gone", AgentName: "orphan", EnvironmentName: "dev", Status: models.AgentThunderStatusCompleted})
	reconciler := NewAgentThunderReconcilerService(f.svc, nil, f.repo, f.svc.logger).(*agentThunderReconcilerService)
	reconciler.SetAgentIdentityPipelineReader(fakePipelines{"default": pipelineOf("dev", "stage", "prod")})
	rowsBefore := len(f.repo.rows)

	reconciler.ensurePipelineBindings(f.ctx(t))

	require.NotNil(t, f.repo.find("legacy", "stage"))
	require.NotNil(t, f.repo.find("legacy", "prod"))
	require.Equal(t, "dev", f.repo.find("legacy", "stage").PromotionSource)
	require.Equal(t, "stage", f.repo.find("legacy", "prod").PromotionSource)
	require.Equal(t, models.AgentThunderStatusPending, f.repo.find("legacy", "prod").Status)
	require.NotNil(t, f.repo.find("grown", "prod"), "a pipeline that gained an environment gets its binding")
	require.Equal(t, models.AgentThunderStatusCompleted, f.repo.find("grown", "stage").Status, "an existing binding is left alone")
	require.Len(t, f.repo.rows, rowsBefore+3)
	require.Equal(t, "user-1", f.repo.find("legacy", "stage").RequestedBy, "the binding keeps the user the agent was created for")

	reconciler.ensurePipelineBindings(f.ctx(t))
	require.Len(t, f.repo.rows, rowsBefore+3, "a second pass creates nothing")
}

func TestTheBackfillDoesNothingWithoutAPipelineReaderOrPromotion(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("legacy", nil)
	reconciler := NewAgentThunderReconcilerService(f.svc, nil, f.repo, f.svc.logger).(*agentThunderReconcilerService)
	before := len(f.repo.rows)

	reconciler.ensurePipelineBindings(f.ctx(t))
	require.Len(t, f.repo.rows, before, "no reader")

	reconciler.SetAgentIdentityPipelineReader(fakePipelines{"default": pipelineOf("dev", "stage")})
	f.svc.SetIdentityPromotionManager(nil)
	require.False(t, reconciler.promotionEnabled(), "direct identity management starts neither the sync loop nor the backfill")
	reconciler.ensurePipelineBindings(f.ctx(t))
	require.Len(t, f.repo.rows, before, "no promotion")
}

func TestTheSyncRunnerDoesNothingWithoutPromotion(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("direct", allCompleted())
	f.svc.SetIdentityPromotionManager(nil)
	f.repo.skew = time.Hour
	require.Zero(t, f.svc.SyncDuePromotions(f.ctx(t)))
}

func TestAPromotionSaysSoWhenTheEnvironmentItIsPromotedFromFailed(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("scout", map[string]models.AgentThunderStatus{"stage": models.AgentThunderStatusFailed, "prod": models.AgentThunderStatusPending})

	f.svc.AttemptProvision(f.ctx(t), *f.repo.find("scout", "prod"))

	require.Empty(t, f.promotions)
	require.Contains(t, f.repo.find("scout", "prod").LastError, "failed to set up", "the user is told why it waits")
	require.Zero(t, f.repo.claims)
}

func TestAPromotionIntoAnEnvironmentThatIsNoLongerPromotedWaitsOnTheLongBackoffWithItsOwnNote(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("scout", map[string]models.AgentThunderStatus{"stage": models.AgentThunderStatusPending})
	f.svc.envResolver = placementResolver{
		EnvThunderResolverMock: &clientmocks.EnvThunderResolverMock{}, byPromotion: map[string]bool{},
	}

	f.svc.AttemptProvision(f.ctx(t), *f.repo.find("scout", "stage"))

	got := f.repo.find("scout", "stage")
	require.Equal(t, models.AgentThunderStatusPending, got.Status, "a retry that cannot help does not burn the budget")
	require.Contains(t, got.LastError, "no longer receives its identity by promotion")
	require.Zero(t, got.AttemptCount)
}

func TestARoleWhoseGroupHoldersWereNotAllReadIsHeldBack(t *testing.T) {
	f := newSyncFixture(t)
	f.agent("first", allCompleted())
	var members []string
	for i := 0; i < maxSyncAgentsPerTrigger+10; i++ {
		members = append(members, fmt.Sprintf("id-member-%d", i))
	}
	f.world.groups["big"] = members
	f.world.role("role-1", "booker", nil, []string{"id-first"}, []string{"big"})
	ctx := f.ctx(t)
	require.NoError(t, f.svc.MarkAgentsForIdentitySync(ctx, "org-id", []AgentIdentityRef{{ProjectName: "default", AgentName: "first"}}, 0))

	f.repo.skew = time.Second
	f.svc.SyncDuePromotions(ctx)

	require.Empty(t, f.promotedTo("stage")["id-first"].Roles, "holders that were not read cannot be shown to be ready")
	require.Equal(t, "1 role waits for other agents' identity in stage", f.repo.find("first", "stage").LastError)
}
