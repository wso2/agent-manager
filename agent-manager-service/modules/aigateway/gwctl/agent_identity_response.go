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

package gwctl

import "github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/identityv1"

// The identity.directory/v1 types carry no JSON names, so the agent-identity
// routes map them to these response bodies, which keep the field names the
// console reads.

type agentIdentityGroupResponse struct {
	ID          string `json:"id"`
	OUID        string `json:"ouId,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

type agentIdentityRolePermissionResponse struct {
	ResourceServerID string   `json:"resourceServerId"`
	Permissions      []string `json:"permissions"`
}

type agentIdentityRoleResponse struct {
	ID          string                                `json:"id"`
	OUID        string                                `json:"ouId,omitempty"`
	Name        string                                `json:"name"`
	Description string                                `json:"description,omitempty"`
	Permissions []agentIdentityRolePermissionResponse `json:"permissions,omitempty"`
	IsReadOnly  bool                                  `json:"isReadOnly"`
	CreatedAt   string                                `json:"createdAt,omitempty"`
	UpdatedAt   string                                `json:"updatedAt,omitempty"`
}

type agentIdentityMemberResponse struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type agentIdentityRoleAssignmentsResponse struct {
	Agents []agentIdentityMemberResponse `json:"agents,omitempty"`
	Groups []agentIdentityGroupResponse  `json:"groups,omitempty"`
}

func toGroupResponse(g identityv1.Group) agentIdentityGroupResponse {
	return agentIdentityGroupResponse{
		ID:          g.ID,
		OUID:        g.OUID,
		Name:        g.Name,
		Description: g.Description,
		CreatedAt:   g.CreatedAt,
		UpdatedAt:   g.UpdatedAt,
	}
}

func toGroupResponses(groups []identityv1.Group) []agentIdentityGroupResponse {
	out := make([]agentIdentityGroupResponse, 0, len(groups))
	for _, g := range groups {
		out = append(out, toGroupResponse(g))
	}
	return out
}

func toRoleResponse(r identityv1.Role) agentIdentityRoleResponse {
	var perms []agentIdentityRolePermissionResponse
	for _, p := range r.Permissions {
		perms = append(perms, agentIdentityRolePermissionResponse{
			ResourceServerID: p.ResourceServerID,
			Permissions:      p.Permissions,
		})
	}
	return agentIdentityRoleResponse{
		ID:          r.ID,
		OUID:        r.OUID,
		Name:        r.Name,
		Description: r.Description,
		Permissions: perms,
		IsReadOnly:  r.IsReadOnly,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func toRoleResponses(roles []identityv1.Role) []agentIdentityRoleResponse {
	out := make([]agentIdentityRoleResponse, 0, len(roles))
	for _, r := range roles {
		out = append(out, toRoleResponse(r))
	}
	return out
}

func toMemberResponses(members []identityv1.Member) []agentIdentityMemberResponse {
	out := make([]agentIdentityMemberResponse, 0, len(members))
	for _, m := range members {
		out = append(out, agentIdentityMemberResponse{ID: m.ID, Type: m.Type})
	}
	return out
}

func toRoleAssignmentsResponse(a identityv1.RoleAssignments) agentIdentityRoleAssignmentsResponse {
	var resp agentIdentityRoleAssignmentsResponse
	if len(a.Agents) > 0 {
		resp.Agents = toMemberResponses(a.Agents)
	}
	if len(a.Groups) > 0 {
		resp.Groups = toGroupResponses(a.Groups)
	}
	return resp
}
