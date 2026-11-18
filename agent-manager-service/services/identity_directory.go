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
	"errors"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/identityv1"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// NewIdentityDirectory is the core's identity.directory/v1: each environment's own Thunder, reached through
// resolver (in Lite, the platform ThunderID for the default environment).
func NewIdentityDirectory(
	resolver thundersvc.EnvThunderResolver, bindings repositories.AgentThunderClientRepository,
) identityv1.Directory {
	return identityDirectory{resolver: resolver, bindings: bindings}
}

type identityDirectory struct {
	resolver thundersvc.EnvThunderResolver
	bindings repositories.AgentThunderClientRepository
}

func (d identityDirectory) ListProvisionedAgents(ctx context.Context, orgID, environment string) ([]identityv1.ProvisionedAgent, error) {
	rows, err := d.bindings.FindByOuAndEnvironment(ctx, orgID, environment)
	if err != nil {
		return nil, err
	}
	out := make([]identityv1.ProvisionedAgent, 0, len(rows))
	for _, row := range rows {
		out = append(out, identityv1.ProvisionedAgent{
			ProjectName: row.ProjectName, AgentName: row.AgentName, Status: string(row.Status), AgentID: row.ThunderAgentID,
		})
	}
	return out, nil
}

func (d identityDirectory) For(ctx context.Context, orgID, environment string) (identityv1.Client, error) {
	c, err := ResolveEnvThunderIdentity(ctx, d.resolver, orgID, environment)
	if err != nil {
		return nil, directoryError(err)
	}
	return directoryClient{c: c}, nil
}

// classified keeps the client's error text and also matches an identityv1 error under errors.Is.
type classified struct {
	kind, err error
}

func (e classified) Error() string   { return e.err.Error() }
func (e classified) Unwrap() []error { return []error{e.kind, e.err} }

func directoryError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, thundersvc.ErrThunderNotProvisioned):
		return classified{kind: identityv1.ErrNotProvisioned, err: err}
	case thundersvc.IsNotFound(err):
		return classified{kind: identityv1.ErrNotFound, err: err}
	}
	return err
}

type directoryClient struct {
	c thundersvc.EnvIdentityClient
}

func toGroup(g thundersvc.ThunderGroup) identityv1.Group {
	return identityv1.Group{
		ID: g.ID, OUID: g.OuID, Name: g.Name, Description: g.Description, CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt,
	}
}

func toGroups(gs []thundersvc.ThunderGroup) []identityv1.Group {
	out := make([]identityv1.Group, 0, len(gs))
	for _, g := range gs {
		out = append(out, toGroup(g))
	}
	return out
}

func toRole(r thundersvc.ThunderRole) identityv1.Role {
	perms := make([]identityv1.RolePermission, 0, len(r.Permissions))
	for _, p := range r.Permissions {
		perms = append(perms, identityv1.RolePermission{ResourceServerID: p.ResourceServerID, Permissions: p.Permissions})
	}
	return identityv1.Role{
		ID: r.ID, OUID: r.OuID, Name: r.Name, Description: r.Description, Permissions: perms,
		IsReadOnly: r.IsReadOnly, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func toRoles(rs []thundersvc.ThunderRole) []identityv1.Role {
	out := make([]identityv1.Role, 0, len(rs))
	for _, r := range rs {
		out = append(out, toRole(r))
	}
	return out
}

func fromPermission(p identityv1.RolePermission) thundersvc.RolePermissionRequest {
	return thundersvc.RolePermissionRequest{ResourceServerID: p.ResourceServerID, Permissions: p.Permissions}
}

func fromPermissions(ps []identityv1.RolePermission) []thundersvc.RolePermissionRequest {
	if ps == nil {
		return nil
	}
	out := make([]thundersvc.RolePermissionRequest, 0, len(ps))
	for _, p := range ps {
		out = append(out, fromPermission(p))
	}
	return out
}

func fromMembers(ms []identityv1.Member) []thundersvc.GroupMember {
	out := make([]thundersvc.GroupMember, 0, len(ms))
	for _, m := range ms {
		out = append(out, thundersvc.GroupMember{ID: m.ID, Type: m.Type})
	}
	return out
}

func assignments(ms []identityv1.Member) thundersvc.RoleAssignmentsRequest {
	entries := make([]thundersvc.AssignmentEntry, 0, len(ms))
	for _, m := range ms {
		entries = append(entries, thundersvc.AssignmentEntry{ID: m.ID, Type: m.Type})
	}
	return thundersvc.RoleAssignmentsRequest{Assignments: entries}
}

func (d directoryClient) DefaultOUID(ctx context.Context) (string, error) {
	id, err := d.c.GetDefaultOUID(ctx)
	return id, directoryError(err)
}

func (d directoryClient) ListGroups(ctx context.Context, ouID string, offset, limit int) ([]identityv1.Group, int, error) {
	gs, total, err := d.c.ListGroupsByOUId(ctx, ouID, offset, limit)
	if err != nil {
		return nil, 0, directoryError(err)
	}
	return toGroups(gs), total, nil
}

func (d directoryClient) GetGroup(ctx context.Context, groupID string) (identityv1.Group, error) {
	g, err := d.c.GetGroup(ctx, groupID)
	if err != nil {
		return identityv1.Group{}, directoryError(err)
	}
	return toGroup(*g), nil
}

func (d directoryClient) CreateGroup(ctx context.Context, in identityv1.GroupInput) (identityv1.Group, error) {
	g, err := d.c.CreateGroup(ctx, thundersvc.CreateGroupRequest{Name: in.Name, OuID: in.OUID, Description: in.Description})
	if err != nil {
		return identityv1.Group{}, directoryError(err)
	}
	return toGroup(*g), nil
}

func (d directoryClient) UpdateGroup(ctx context.Context, groupID string, in identityv1.GroupInput) (identityv1.Group, error) {
	g, err := d.c.UpdateGroup(ctx, groupID, thundersvc.UpdateGroupRequest{OuID: in.OUID, Name: in.Name, Description: in.Description})
	if err != nil {
		return identityv1.Group{}, directoryError(err)
	}
	return toGroup(*g), nil
}

func (d directoryClient) DeleteGroup(ctx context.Context, groupID string) error {
	return directoryError(d.c.DeleteGroup(ctx, groupID))
}

func (d directoryClient) ListGroupMembers(ctx context.Context, groupID string, offset, limit int) ([]identityv1.Member, int, error) {
	ms, total, err := d.c.ListGroupMemberEntries(ctx, groupID, offset, limit)
	if err != nil {
		return nil, 0, directoryError(err)
	}
	out := make([]identityv1.Member, 0, len(ms))
	for _, m := range ms {
		out = append(out, identityv1.Member{ID: m.ID, Type: m.Type})
	}
	return out, total, nil
}

func (d directoryClient) AddGroupMembers(ctx context.Context, groupID string, members []identityv1.Member) error {
	return directoryError(d.c.AddGroupMemberEntries(ctx, groupID, fromMembers(members)))
}

func (d directoryClient) RemoveGroupMembers(ctx context.Context, groupID string, members []identityv1.Member) error {
	return directoryError(d.c.RemoveGroupMemberEntries(ctx, groupID, fromMembers(members)))
}

func (d directoryClient) GetGroupRoles(ctx context.Context, groupID string) ([]identityv1.Role, error) {
	rs, err := d.c.GetGroupRoles(ctx, groupID)
	if err != nil {
		return nil, directoryError(err)
	}
	return toRoles(rs), nil
}

func (d directoryClient) ListRoles(ctx context.Context, ouID string, offset, limit int) ([]identityv1.Role, int, error) {
	rs, total, err := d.c.ListRoles(ctx, ouID, offset, limit)
	if err != nil {
		return nil, 0, directoryError(err)
	}
	return toRoles(rs), total, nil
}

func (d directoryClient) GetRole(ctx context.Context, roleID string) (identityv1.Role, error) {
	r, err := d.c.GetRole(ctx, roleID)
	if err != nil {
		return identityv1.Role{}, directoryError(err)
	}
	return toRole(*r), nil
}

func (d directoryClient) CreateRole(ctx context.Context, in identityv1.RoleInput) (identityv1.Role, error) {
	r, err := d.c.CreateRole(ctx, thundersvc.CreateRoleRequest{Name: in.Name, OuID: in.OUID, Description: in.Description})
	if err != nil {
		return identityv1.Role{}, directoryError(err)
	}
	return toRole(*r), nil
}

func (d directoryClient) UpdateRole(ctx context.Context, roleID string, in identityv1.RoleInput) (identityv1.Role, error) {
	r, err := d.c.UpdateRole(ctx, roleID, thundersvc.UpdateRoleRequest{
		OuID: in.OUID, Name: in.Name, Description: in.Description, Permissions: fromPermissions(in.Permissions),
	})
	if err != nil {
		return identityv1.Role{}, directoryError(err)
	}
	return toRole(*r), nil
}

func (d directoryClient) DeleteRole(ctx context.Context, roleID string) error {
	return directoryError(d.c.DeleteRole(ctx, roleID))
}

func (d directoryClient) AddRolePermissions(ctx context.Context, roleID string, p identityv1.RolePermission) error {
	return directoryError(d.c.AddRolePermissions(ctx, roleID, fromPermission(p)))
}

func (d directoryClient) RemoveRolePermissions(ctx context.Context, roleID string, p identityv1.RolePermission) error {
	return directoryError(d.c.RemoveRolePermissions(ctx, roleID, fromPermission(p)))
}

func (d directoryClient) GetRoleAssignments(ctx context.Context, roleID string) (identityv1.RoleAssignments, error) {
	a, err := d.c.GetAgentRoleAssignments(ctx, roleID)
	if err != nil {
		return identityv1.RoleAssignments{}, directoryError(err)
	}
	out := identityv1.RoleAssignments{Groups: toGroups(a.Groups)}
	for _, e := range a.Agents {
		out.Agents = append(out.Agents, identityv1.Member{ID: e.ID, Type: e.Type})
	}
	return out, nil
}

func (d directoryClient) AddRoleAssignees(ctx context.Context, roleID string, assignees []identityv1.Member) error {
	return directoryError(d.c.AddRoleAssignees(ctx, roleID, assignments(assignees)))
}

func (d directoryClient) RemoveRoleAssignees(ctx context.Context, roleID string, assignees []identityv1.Member) error {
	return directoryError(d.c.RemoveRoleAssignees(ctx, roleID, assignments(assignees)))
}

func (d directoryClient) EnsureResourceServer(ctx context.Context, handle, displayName, identifier string, actions []string) (string, error) {
	id, err := d.c.EnsureProxyResourceServer(ctx, handle, displayName, identifier, actions)
	return id, directoryError(err)
}

func (d directoryClient) RemoveResourceServerAction(ctx context.Context, handle, action string) (string, error) {
	id, err := d.c.DeleteProxyResourceServerAction(ctx, handle, action)
	return id, directoryError(err)
}

func (d directoryClient) DeleteResourceServer(ctx context.Context, handle string) error {
	return directoryError(d.c.DeleteProxyResourceServer(ctx, handle))
}
