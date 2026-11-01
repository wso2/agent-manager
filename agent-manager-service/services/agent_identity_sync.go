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
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
)

// Changes to roles, groups and MCP resource servers reach the environments an
// agent is promoted into asynchronously. Promotion is ADDITIVE: it creates and
// updates what the agent carries, but it never removes anything from a later
// environment. So removing an agent from a role or group, and deleting a role,
// group or agent, does not remove the access it already has in a later
// environment. That needs deletion support in the identity platform's promotion
// and is not attempted here. Those events still mark the agents, so the carried
// set a later promotion reads is current.
const (
	// identitySyncDebounce is how long a change waits before the agent's later
	// environments are re-promoted. Another change within the window moves the
	// time later, so a burst of changes becomes one promotion per agent and
	// environment.
	identitySyncDebounce = 10 * time.Second
	// identitySyncTriggerTimeout bounds the background work one change starts.
	identitySyncTriggerTimeout = 2 * time.Minute
	// maxSyncAgentsPerTrigger and maxSyncRolesPerTrigger bound the fan-out of one
	// change. A change that reaches more is logged and the rest is left to the
	// next change or to the agents' own promotions.
	maxSyncAgentsPerTrigger = 500
	maxSyncRolesPerTrigger  = 200
	// maxRolesScannedForProxy bounds how many roles are read to find the ones that
	// reference one MCP server.
	maxRolesScannedForProxy = 2000
	identityListPageSize    = 100
)

// AgentIdentitySyncMarker is an optional capability of an
// AgentThunderProvisioningService: it marks agents whose carried identity
// resources changed, so their later environments are promoted again with the
// current set. It affects only the agent's completed and in progress bindings in
// environments that receive their identity by promotion: one that is still
// pending reads the current set when it is attempted.
type AgentIdentitySyncMarker interface {
	// MarkAgentsForIdentitySync sets the re-promotion time to now + delay. A later
	// request pushes it out, so a burst collapses into one promotion.
	MarkAgentsForIdentitySync(ctx context.Context, ouID string, refs []AgentIdentityRef, delay time.Duration) error
}

// AgentIdentitySyncTrigger is told about changes to the identity resources agents
// carry. Every method returns at once and does its work in the background: a
// change never fails or waits because of promotion. env is the environment where
// the roles and groups are managed, the first environment of the agents' pipelines.
type AgentIdentitySyncTrigger interface {
	// RoleChanged: a role's name, description or permissions changed.
	RoleChanged(ctx context.Context, ouID, env, roleID string)
	// RoleAssigneesChanged: assignees were added to or removed from a role.
	RoleAssigneesChanged(ctx context.Context, ouID, env, roleID string, assignees []thundersvc.AssignmentEntry)
	// GroupChanged: a group's name or description changed.
	GroupChanged(ctx context.Context, ouID, env, groupID string)
	// GroupMembersChanged: members were added to or removed from a group.
	GroupMembersChanged(ctx context.Context, ouID, env, groupID string, members []thundersvc.GroupMember)
	// BeforeRoleDelete and BeforeGroupDelete read who is affected, since that
	// cannot be read afterwards. Call the returned function once the delete
	// succeeded. A role or group delete is not carried into later environments
	// (see the note above); the agents are marked so their carried set is current.
	BeforeRoleDelete(ctx context.Context, ouID, env, roleID string) (afterDelete func())
	BeforeGroupDelete(ctx context.Context, ouID, env, groupID string) (afterDelete func())
	// MCPProxyChanged: the actions (scopes) of an MCP server changed, so the roles
	// that reference its resource server, and the agents that hold them, did too.
	MCPProxyChanged(ctx context.Context, ouID, env, proxyHandle string)
}

// AgentIdentitySyncTriggerSetter hands the trigger to the services that raise
// changes. They are built before the provisioning service is complete.
type AgentIdentitySyncTriggerSetter interface {
	SetAgentIdentitySyncTrigger(AgentIdentitySyncTrigger)
}

var (
	_ AgentIdentitySyncMarker  = (*agentThunderProvisioningService)(nil)
	_ AgentIdentitySyncTrigger = (*agentThunderProvisioningService)(nil)
)

// MarkAgentsForIdentitySync implements AgentIdentitySyncMarker.
func (s *agentThunderProvisioningService) MarkAgentsForIdentitySync(ctx context.Context, ouID string,
	refs []AgentIdentityRef, delay time.Duration,
) error {
	if !s.IdentityPromotionEnabled() || len(refs) == 0 {
		return nil
	}
	var ids []uuid.UUID
	var firstErr error
	for _, ref := range refs {
		rows, err := s.repo.FindByAgent(ctx, ouID, ref.ProjectName, ref.AgentName)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("read agent identity bindings: %w", err)
			}
			continue
		}
		for _, row := range rows {
			if row.PromotionSource != "" {
				ids = append(ids, row.ID)
			}
		}
	}
	if len(ids) > 0 {
		if err := s.repo.MarkSyncAfter(ctx, ids, time.Now().Add(delay)); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// runDetached runs fn in the background with a context that outlives the request
// that raised the change and is bounded in time, so no goroutine outlives its
// work. It keeps the request's values (the caller and the audit recorder) but not
// its cancellation.
func (s *agentThunderProvisioningService) runDetached(ctx context.Context, ouID, what string, fn func(ctx context.Context)) {
	detached := context.WithoutCancel(ctx)
	if org, ok := orgctx.GetResolvedOrg(detached); !ok || org.OUID != ouID {
		detached = orgctx.WithResolvedOrg(detached, orgctx.ResolvedOrg{OUID: ouID})
	}
	run := func() {
		runCtx, cancel := context.WithTimeout(detached, identitySyncTriggerTimeout)
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("Recovered from panic while queueing an agent identity sync", "change", what, "panic", r)
			}
		}()
		fn(runCtx)
	}
	if s.triggerRunner != nil {
		s.triggerRunner(run)
		return
	}
	go run()
}

// markThunderAgents turns agent ids into the agents that have a binding in env
// and marks them. It never fails the caller: it logs.
func (s *agentThunderProvisioningService) markThunderAgents(ctx context.Context, ouID, env, what string, agentIDs []string) {
	agentIDs = s.capAgents(what, agentIDs)
	if len(agentIDs) == 0 {
		return
	}
	rows, err := s.repo.FindByOuAndEnvironment(ctx, ouID, env)
	if err != nil {
		s.logger.Warn("Failed to read agents for an identity sync", "change", what, "error", err)
		return
	}
	wanted := make(map[string]bool, len(agentIDs))
	for _, id := range agentIDs {
		wanted[id] = true
	}
	refs := make([]AgentIdentityRef, 0, len(agentIDs))
	for _, row := range rows {
		if row.PromotionSource == "" && row.ThunderAgentID != "" && wanted[row.ThunderAgentID] {
			refs = append(refs, AgentIdentityRef{
				OUID: ouID, ProjectName: row.ProjectName, AgentName: row.AgentName, Ownership: row.ProvisioningType,
			})
		}
	}
	// The mark is the only durable record of the change, so a momentary DB error is
	// retried before it is given up on.
	var markErr error
	for attempt := 0; attempt < writeAheadUpsertAttempts; attempt++ {
		if markErr = s.MarkAgentsForIdentitySync(ctx, ouID, refs, identitySyncDebounce); markErr == nil {
			break
		}
		time.Sleep(writeAheadUpsertRetryDelay)
	}
	if markErr != nil {
		s.logger.Error("Failed to mark agents for an identity sync; their later environments stay as they are until the next change",
			"ouID", ouID, "env", env, "change", what, "agents", len(refs), "error", markErr)
		return
	}
	s.logger.Debug("Marked agents for an identity sync", "change", what, "agents", len(refs))
}

func (s *agentThunderProvisioningService) capAgents(what string, agentIDs []string) []string {
	if len(agentIDs) <= maxSyncAgentsPerTrigger {
		return agentIDs
	}
	s.logger.Warn("Agent identity sync reaches more agents than one change syncs; the rest wait for their next change",
		"change", what, "agents", len(agentIDs), "limit", maxSyncAgentsPerTrigger)
	return agentIDs[:maxSyncAgentsPerTrigger]
}

// holderReader reads who holds roles, through one identity client. It keeps the
// agent members of the groups it has read, so a group shared by several roles is
// read once.
type holderReader struct {
	client       thundersvc.EnvIdentityClient
	groupMembers map[string][]string
	// truncated marks the groups whose agent members were cut at the limit. A
	// marking caller does not mind; the hold back rule must, because it cannot say
	// every holder is ready when it has not read them all.
	truncated map[string]bool
	// sawTruncated is set by the last roleHolders call when a group it read was cut.
	sawTruncated bool
}

func newHolderReader(client thundersvc.EnvIdentityClient) *holderReader {
	return &holderReader{client: client, groupMembers: map[string][]string{}, truncated: map[string]bool{}}
}

// groupAgents lists the agent members of a group.
func (r *holderReader) groupAgents(ctx context.Context, groupID string) ([]string, error) {
	if cached, ok := r.groupMembers[groupID]; ok {
		return cached, nil
	}
	agents := []string{}
	for offset := 0; ; {
		if len(agents) >= maxSyncAgentsPerTrigger {
			slog.Warn("A group has more agent members than one change syncs; the rest wait for their next change",
				"limit", maxSyncAgentsPerTrigger)
			r.truncated[groupID] = true
			break
		}
		members, total, err := r.client.ListGroupMemberEntries(ctx, groupID, offset, identityListPageSize)
		if err != nil {
			return nil, err
		}
		for _, member := range members {
			if member.Type == "agent" {
				agents = append(agents, member.ID)
			}
		}
		offset += len(members)
		if offset >= total || len(members) == 0 {
			break
		}
	}
	r.groupMembers[groupID] = agents
	return agents, nil
}

// roleHolders lists the agents that hold a role: the ones assigned to it and the
// agent members of the groups assigned to it.
func (r *holderReader) roleHolders(ctx context.Context, roleID string) ([]string, error) {
	assignments, err := r.client.GetAgentRoleAssignments(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if assignments == nil {
		return nil, nil
	}
	r.sawTruncated = false
	holders := make([]string, 0, len(assignments.Agents))
	for _, agent := range assignments.Agents {
		holders = append(holders, agent.ID)
	}
	for _, group := range assignments.Groups {
		members, err := r.groupAgents(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		holders = append(holders, members...)
		r.sawTruncated = r.sawTruncated || r.truncated[group.ID]
	}
	return uniqueStrings(holders), nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// assignedAgents lists the agents an assignment list reaches: the agents in it
// and the agent members of its groups.
func (r *holderReader) assignedAgents(ctx context.Context, assignees []thundersvc.AssignmentEntry) ([]string, error) {
	agents := []string{}
	for _, assignee := range assignees {
		switch assignee.Type {
		case "agent":
			agents = append(agents, assignee.ID)
		case "group":
			members, err := r.groupAgents(ctx, assignee.ID)
			if err != nil {
				return nil, err
			}
			agents = append(agents, members...)
		}
	}
	return uniqueStrings(agents), nil
}

func (s *agentThunderProvisioningService) homeIdentityClient(ctx context.Context, ouID, env string) (thundersvc.EnvIdentityClient, error) {
	client, err := ResolveEnvThunderIdentity(ctx, s.envResolver, ouID, env)
	if err != nil {
		return nil, fmt.Errorf("resolve the identity client of %s: %w", env, err)
	}
	return client, nil
}

// RoleChanged implements AgentIdentitySyncTrigger.
func (s *agentThunderProvisioningService) RoleChanged(ctx context.Context, ouID, env, roleID string) {
	if !s.IdentityPromotionEnabled() {
		return
	}
	s.runDetached(ctx, ouID, "role-changed", func(ctx context.Context) {
		client, err := s.homeIdentityClient(ctx, ouID, env)
		if err != nil {
			s.logger.Warn("Cannot sync agents after a role change", "ouID", ouID, "env", env, "error", err)
			return
		}
		holders, err := newHolderReader(client).roleHolders(ctx, roleID)
		if err != nil {
			s.logger.Warn("Cannot read who holds a changed role", "ouID", ouID, "env", env, "error", err)
			return
		}
		s.markThunderAgents(ctx, ouID, env, "role-changed", holders)
	})
}

// RoleAssigneesChanged implements AgentIdentitySyncTrigger.
func (s *agentThunderProvisioningService) RoleAssigneesChanged(ctx context.Context, ouID, env, _ string,
	assignees []thundersvc.AssignmentEntry,
) {
	if !s.IdentityPromotionEnabled() || len(assignees) == 0 {
		return
	}
	// The slice belongs to the request; the background work reads its own copy.
	assignees = append([]thundersvc.AssignmentEntry(nil), assignees...)
	s.runDetached(ctx, ouID, "role-assignees-changed", func(ctx context.Context) {
		client, err := s.homeIdentityClient(ctx, ouID, env)
		if err != nil {
			s.logger.Warn("Cannot sync agents after a role assignment change", "ouID", ouID, "env", env, "error", err)
			return
		}
		agents, err := newHolderReader(client).assignedAgents(ctx, assignees)
		if err != nil {
			s.logger.Warn("Cannot read the agents of a role assignment change", "ouID", ouID, "env", env, "error", err)
			return
		}
		s.markThunderAgents(ctx, ouID, env, "role-assignees-changed", agents)
	})
}

// GroupChanged implements AgentIdentitySyncTrigger.
func (s *agentThunderProvisioningService) GroupChanged(ctx context.Context, ouID, env, groupID string) {
	if !s.IdentityPromotionEnabled() {
		return
	}
	s.runDetached(ctx, ouID, "group-changed", func(ctx context.Context) {
		client, err := s.homeIdentityClient(ctx, ouID, env)
		if err != nil {
			s.logger.Warn("Cannot sync agents after a group change", "ouID", ouID, "env", env, "error", err)
			return
		}
		agents, err := newHolderReader(client).groupAgents(ctx, groupID)
		if err != nil {
			s.logger.Warn("Cannot read the members of a changed group", "ouID", ouID, "env", env, "error", err)
			return
		}
		s.markThunderAgents(ctx, ouID, env, "group-changed", agents)
	})
}

// GroupMembersChanged implements AgentIdentitySyncTrigger.
func (s *agentThunderProvisioningService) GroupMembersChanged(ctx context.Context, ouID, env, _ string,
	members []thundersvc.GroupMember,
) {
	if !s.IdentityPromotionEnabled() {
		return
	}
	agents := make([]string, 0, len(members))
	for _, member := range members {
		if member.Type == "agent" {
			agents = append(agents, member.ID)
		}
	}
	if len(agents) == 0 {
		return
	}
	s.runDetached(ctx, ouID, "group-members-changed", func(ctx context.Context) {
		s.markThunderAgents(ctx, ouID, env, "group-members-changed", agents)
	})
}

// BeforeRoleDelete implements AgentIdentitySyncTrigger.
func (s *agentThunderProvisioningService) BeforeRoleDelete(ctx context.Context, ouID, env, roleID string) func() {
	return s.beforeDelete(ctx, ouID, env, "role-deleted", func(ctx context.Context, reader *holderReader) ([]string, error) {
		return reader.roleHolders(ctx, roleID)
	})
}

// BeforeGroupDelete implements AgentIdentitySyncTrigger.
func (s *agentThunderProvisioningService) BeforeGroupDelete(ctx context.Context, ouID, env, groupID string) func() {
	return s.beforeDelete(ctx, ouID, env, "group-deleted", func(ctx context.Context, reader *holderReader) ([]string, error) {
		return reader.groupAgents(ctx, groupID)
	})
}

// beforeDelete reads the affected agents while the resource still exists, within a
// short bound, and returns the function that marks them. A failed read is logged
// and the delete goes on: it never fails or waits for promotion.
func (s *agentThunderProvisioningService) beforeDelete(ctx context.Context, ouID, env, what string,
	read func(ctx context.Context, reader *holderReader) ([]string, error),
) func() {
	noop := func() {}
	if !s.IdentityPromotionEnabled() {
		return noop
	}
	readCtx, cancel := context.WithTimeout(ctx, identityDeleteReadTimeout)
	defer cancel()
	client, err := s.homeIdentityClient(readCtx, ouID, env)
	if err != nil {
		s.logger.Warn("Cannot read the agents a delete affects", "change", what, "ouID", ouID, "env", env, "error", err)
		return noop
	}
	agents, err := read(readCtx, newHolderReader(client))
	if err != nil {
		s.logger.Warn("Cannot read the agents a delete affects", "change", what, "ouID", ouID, "env", env, "error", err)
		return noop
	}
	if len(agents) == 0 {
		return noop
	}
	return func() {
		s.runDetached(ctx, ouID, what, func(ctx context.Context) {
			s.markThunderAgents(ctx, ouID, env, what, agents)
		})
	}
}

// identityDeleteReadTimeout bounds the read a delete makes before it proceeds.
const identityDeleteReadTimeout = 5 * time.Second

// MCPProxyChanged implements AgentIdentitySyncTrigger.
func (s *agentThunderProvisioningService) MCPProxyChanged(ctx context.Context, ouID, env, proxyHandle string) {
	if !s.IdentityPromotionEnabled() || proxyHandle == "" {
		return
	}
	s.runDetached(ctx, ouID, "mcp-scopes-changed", func(ctx context.Context) {
		client, err := s.homeIdentityClient(ctx, ouID, env)
		if err != nil {
			s.logger.Warn("Cannot sync agents after an MCP server change", "ouID", ouID, "env", env, "error", err)
			return
		}
		roleIDs, err := rolesReferencingProxy(ctx, client, proxyHandle)
		if err != nil {
			s.logger.Warn("Cannot read the roles of a changed MCP server", "ouID", ouID, "env", env, "error", err)
			return
		}
		reader := newHolderReader(client)
		var agents []string
		for _, roleID := range roleIDs {
			holders, err := reader.roleHolders(ctx, roleID)
			if err != nil {
				s.logger.Warn("Cannot read who holds a role of a changed MCP server", "ouID", ouID, "env", env, "error", err)
				return
			}
			agents = append(agents, holders...)
		}
		s.markThunderAgents(ctx, ouID, env, "mcp-scopes-changed", uniqueStrings(agents))
	})
}

// rolesReferencingProxy lists the roles with a permission of the MCP server, a
// scope written "<proxy handle>:<action>". The listing carries the permissions.
func rolesReferencingProxy(ctx context.Context, client thundersvc.EnvIdentityClient, proxyHandle string) ([]string, error) {
	prefix := proxyHandle + ":"
	var roleIDs []string
	scanned := 0
	for offset := 0; scanned < maxRolesScannedForProxy; {
		roles, total, err := client.ListRoles(ctx, "", offset, identityListPageSize)
		if err != nil {
			return nil, err
		}
		for _, role := range roles {
			scanned++
			if isHiddenIdentityRole(role) || !roleHasScopePrefix(role, prefix) {
				continue
			}
			roleIDs = append(roleIDs, role.ID)
		}
		offset += len(roles)
		if offset >= total || len(roles) == 0 {
			break
		}
	}
	if scanned >= maxRolesScannedForProxy {
		slog.Warn("Stopped scanning roles for an MCP server change at the limit", "limit", maxRolesScannedForProxy)
	}
	if len(roleIDs) > maxSyncRolesPerTrigger {
		slog.Warn("More roles reference an MCP server than one change syncs; the rest wait for their next change",
			"roles", len(roleIDs), "limit", maxSyncRolesPerTrigger)
		roleIDs = roleIDs[:maxSyncRolesPerTrigger]
	}
	return roleIDs, nil
}

func roleHasScopePrefix(role thundersvc.ThunderRole, prefix string) bool {
	for _, grant := range role.Permissions {
		for _, scope := range grant.Permissions {
			if strings.HasPrefix(scope, prefix) {
				return true
			}
		}
	}
	return false
}

// holdBackRoles applies the cross pipeline rule: a role is carried into the
// target environment only when every agent that holds it (directly or through a
// group) already has a completed identity there. The agent being promoted counts
// as having one. The rest are held back and promoted by a later re-promotion; held
// is how many. Nothing is read when no role is carried.
func (s *agentThunderProvisioningService) holdBackRoles(ctx context.Context, ouID, homeEnv, targetEnv, agentID string,
	roles []PromotedIdentityResource,
) (kept []PromotedIdentityResource, held int, err error) {
	if len(roles) == 0 {
		return roles, 0, nil
	}
	client, err := s.homeIdentityClient(ctx, ouID, homeEnv)
	if err != nil {
		return nil, 0, unavailableIdentity("the identity provider of the default environment is not reachable", err)
	}
	// The other agents' identities in the target are read only when a role has a
	// holder besides the agent itself, so the common case reads nothing.
	var ready map[string]bool
	readReady := func() error {
		if ready != nil {
			return nil
		}
		rows, err := s.repo.FindByOuAndEnvironment(ctx, ouID, targetEnv)
		if err != nil {
			return unavailableIdentity("the identity of the other agents in "+targetEnv+" could not be read", err)
		}
		ready = map[string]bool{agentID: true}
		for _, row := range rows {
			if row.Status == models.AgentThunderStatusCompleted && row.ThunderAgentID != "" {
				ready[row.ThunderAgentID] = true
			}
		}
		return nil
	}
	reader := newHolderReader(client)
	kept = make([]PromotedIdentityResource, 0, len(roles))
	for _, role := range roles {
		holders, err := reader.roleHolders(ctx, role.ID)
		if err != nil {
			return nil, 0, unavailableIdentity(fmt.Sprintf("the assignees of role %q could not be read", role.Name), err)
		}
		// A role with more holders than were read cannot be shown to have every one
		// of them ready, so it waits.
		waiting := reader.sawTruncated
		for _, holder := range holders {
			if waiting {
				break
			}
			if holder == agentID {
				continue
			}
			if err := readReady(); err != nil {
				return nil, 0, err
			}
			if !ready[holder] {
				waiting = true
				break
			}
		}
		if waiting {
			held++
			continue
		}
		kept = append(kept, role)
	}
	return kept, held, nil
}

// heldRolesNote is the short note left on a binding while roles wait for other
// agents' identity in its environment. It is empty when none wait.
func heldRolesNote(held int, env string) string {
	switch {
	case held <= 0:
		return ""
	case held == 1:
		return "1 role waits for other agents' identity in " + env
	default:
		return fmt.Sprintf("%d roles wait for other agents' identity in %s", held, env)
	}
}

// markCoAssignees marks the agents that share a role or a group with the agent
// whose identity just completed in env, so a role that was held back for it is
// carried. It runs for a first promotion only, never for a re-promotion, so a
// sync cannot mark itself again. It logs and never fails the caller.
func (s *agentThunderProvisioningService) markCoAssignees(ctx context.Context, b models.AgentThunderClient, homeEnv, agentID string) {
	client, err := s.homeIdentityClient(ctx, b.OUID, homeEnv)
	if err != nil {
		s.logger.Warn("Cannot find the agents that share roles with a promoted agent", "agentName", b.AgentName, "error", err)
		return
	}
	reader := newHolderReader(client)
	others, err := s.coAssigneeAgentIDs(ctx, client, reader, agentID)
	if err != nil {
		s.logger.Warn("Cannot find the agents that share roles with a promoted agent", "agentName", b.AgentName, "error", err)
		return
	}
	s.markThunderAgents(ctx, b.OUID, homeEnv, "co-assignee-completed", others)
}

// coAssigneeAgentIDs lists the agents that hold a role, or belong to a group,
// the agent holds or belongs to, directly or through its groups.
func (s *agentThunderProvisioningService) coAssigneeAgentIDs(ctx context.Context, client thundersvc.EnvIdentityClient,
	reader *holderReader, agentID string,
) ([]string, error) {
	thunderOU, err := client.GetDefaultOUID(ctx)
	if err != nil {
		return nil, err
	}
	roles, err := client.GetAgentRoles(ctx, agentID)
	if err != nil {
		return nil, err
	}
	groups, err := client.GetAgentGroups(ctx, thunderOU, agentID)
	if err != nil {
		return nil, err
	}
	roleIDs := map[string]bool{}
	for _, role := range roles {
		roleIDs[role.ID] = true
	}
	var others []string
	for _, group := range groups {
		groupRoles, err := client.GetGroupRoles(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		for _, role := range groupRoles {
			roleIDs[role.ID] = true
		}
		members, err := reader.groupAgents(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		others = append(others, members...)
	}
	for roleID := range roleIDs {
		holders, err := reader.roleHolders(ctx, roleID)
		if err != nil {
			return nil, err
		}
		others = append(others, holders...)
	}
	out := make([]string, 0, len(others))
	for _, id := range uniqueStrings(others) {
		if id != agentID {
			out = append(out, id)
		}
	}
	return out, nil
}
