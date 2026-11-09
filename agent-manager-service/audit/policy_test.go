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

package audit

import (
	"net/http"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

func TestShouldAuditByMethod(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   bool
	}{
		{"post is audited", http.MethodPost, "/orgs/{orgName}/projects", true},
		{"put is audited", http.MethodPut, "/orgs/{orgName}/projects/{projName}", true},
		{"patch is audited", http.MethodPatch, "/orgs/{orgName}/x", true},
		{"delete is audited", http.MethodDelete, "/orgs/{orgName}/projects/{projName}", true},
		{"ordinary get is not audited", http.MethodGet, "/orgs/{orgName}/projects", false},
		{"head is not audited", http.MethodHead, "/orgs/{orgName}/projects", false},
		{"options is not audited", http.MethodOptions, "/orgs/{orgName}/projects", false},
		{"sensitive get is audited", http.MethodGet, "/orgs/{orgName}/git-secrets", true},
		{"exempt write is not audited", http.MethodPost, "/orgs/{orgName}/utils/generate-name", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldAudit(tt.method, tt.path); got != tt.want {
				t.Errorf("shouldAudit(%q, %q) = %v, want %v", tt.method, tt.path, got, tt.want)
			}
		})
	}
}

func TestDeriveAction(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		perms  []rbac.Permission
		want   Action
	}{
		{
			name:   "single permission is used verbatim",
			method: http.MethodPost,
			path:   "/orgs/{orgName}/projects/{projName}/agents",
			perms:  []rbac.Permission{rbac.AgentCreate},
			want:   "agent:create",
		},
		{
			name:   "override wins over the permission",
			method: http.MethodPost,
			path:   "/orgs/{orgName}/identities/roles/{roleID}/permissions/add",
			perms:  []rbac.Permission{rbac.RoleUpdate},
			want:   "role:grant-permission",
		},
		{
			name:   "same path under a different method gets its own action",
			method: http.MethodDelete,
			path:   "/orgs/{orgName}/projects/{projName}/agents/{agentName}/identities",
			perms:  []rbac.Permission{rbac.AgentUpdate},
			want:   "agent-identity:revoke-secret",
		},
		{
			name:   "multiple permissions fall back to resource plus method verb",
			method: http.MethodPut,
			path:   "/orgs/{orgName}/unmapped",
			perms:  []rbac.Permission{rbac.EnvironmentRead, rbac.LLMProviderRead},
			want:   "environment:update",
		},
		{
			// A route with no permission has nothing to derive from. Returning
			// empty makes NewRouteMeta panic, which is what forces an explicit
			// actionOverrides entry rather than an invented, unregistered label.
			name:   "no permission yields no action",
			method: http.MethodPost,
			path:   "/orgs/{orgName}/widgets",
			perms:  nil,
			want:   "",
		},
		{
			// The tier says where the operation lands, not what it is, so it is
			// not allowed to name the action. The capability beside it is.
			name:   "the environment tier does not name the action",
			method: http.MethodPost,
			path:   "/orgs/{orgName}/unmapped-tiered",
			perms:  []rbac.Permission{rbac.AgentSuspend, rbac.AgentEnvNonProduction},
			want:   "agent:suspend",
		},
		{
			// A route gated only on the axis is the case this filter exists for:
			// it derives nothing and panics at registration, rather than
			// registering "agent:env-non-production" as an action with no class,
			// no severity and no detail schema.
			name:   "a tier-only route yields no action",
			method: http.MethodPost,
			path:   "/orgs/{orgName}/unmapped-tier-only",
			perms:  []rbac.Permission{rbac.AgentEnvNonProduction},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveAction(tt.method, tt.path, tt.perms); got != tt.want {
				t.Errorf("deriveAction(%q, %q) = %q, want %q", tt.method, tt.path, got, tt.want)
			}
		})
	}
}

// TestNewRouteMetaPanicsWithoutAnAction pins the fail-closed behaviour. A route
// that cannot be labelled must break the build rather than ship as an
// unattributable gap in the trail.
func TestNewRouteMetaPanicsWithoutAnAction(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected a panic for an unlabellable audited route")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "cannot derive an action") {
			t.Errorf("panic message %v does not explain the failure", r)
		}
	}()

	// A path made entirely of parameters yields no resource name and carries no
	// permission, so no action can be derived.
	NewRouteMeta("POST /{a}/{b}", []string{"a", "b"}, nil)
}

func TestNewRouteMetaSkipsPolicyForUnauditedRoutes(t *testing.T) {
	meta := NewRouteMeta("GET /orgs/{orgName}/projects", []string{"orgName"}, []rbac.Permission{rbac.ProjectRead})
	if meta.Audited {
		t.Error("ordinary read should not be audited")
	}
	if meta.Action != "" {
		t.Errorf("unaudited route should carry no action, got %q", meta.Action)
	}
}

func TestNewRouteMetaSplitsPatternAndParams(t *testing.T) {
	pattern := "PUT /orgs/{orgName}/projects/{projName}"
	meta := NewRouteMeta(pattern, ExtractPathParams(pattern), []rbac.Permission{rbac.ProjectUpdate})

	if meta.Method != http.MethodPut {
		t.Errorf("Method = %q, want PUT", meta.Method)
	}
	if meta.Path != "/orgs/{orgName}/projects/{projName}" {
		t.Errorf("Path = %q", meta.Path)
	}
	if len(meta.Params) != 2 || meta.Params[0] != "orgName" || meta.Params[1] != "projName" {
		t.Errorf("Params = %v, want [orgName projName]", meta.Params)
	}
}

// TestProjectMCPBindingAPIKeyRoutesHaveTheirOwnActions keeps the binding key operations apart in the audit trail:
// they share one permission, so without overrides they would all record mcp-server:api-key-manage.
func TestProjectMCPBindingAPIKeyRoutesHaveTheirOwnActions(t *testing.T) {
	const keys = "/orgs/{orgName}/projects/{projName}/mcp-proxies/{proxyId}/environments/{envName}/api-keys"
	for pattern, want := range map[string]Action{
		"POST " + keys:                  ActionAPIKeyCreate,
		"POST " + keys + "/test":        ActionAPIKeyIssueTest,
		"PUT " + keys + "/{keyName}":    ActionAPIKeyRotate,
		"DELETE " + keys + "/{keyName}": ActionAPIKeyRevoke,
	} {
		t.Run(pattern, func(t *testing.T) {
			meta := NewRouteMeta(pattern, ExtractPathParams(pattern), []rbac.Permission{rbac.MCPServerAPIKeyManage})
			if !meta.Audited {
				t.Fatal("route should be audited")
			}
			if meta.Action != want {
				t.Errorf("Action = %q, want %q", meta.Action, want)
			}
		})
	}
}

// TestCredentialActionsAreCritical checks the classification that drives
// alerting. Credential and privilege changes must not be filed as routine.
func TestCredentialActionsAreCritical(t *testing.T) {
	critical := []Action{
		"api-key:create", "api-key:rotate", "api-key:revoke",
		"git-secret:create", "git-secret:delete",
		"gateway-token:rotate", "agent-token:mint",
		"agent-identity:provision", "agent-identity:revoke-secret",
		"role:grant-permission", "role:assign",
		"user:create", "user:delete", "group:add-member",
		"service-account:configure",
	}

	for _, action := range critical {
		t.Run(string(action), func(t *testing.T) {
			if got := action.Severity(); got != SeverityCritical {
				t.Errorf("Severity() = %d, want %d (critical)", got, SeverityCritical)
			}
			switch class := action.Class(); class {
			case ClassCredential, ClassIdentity:
			default:
				t.Errorf("Class() = %q, want credential or identity", class)
			}
		})
	}
}

func TestDeploymentActionsAreClassified(t *testing.T) {
	for _, action := range []Action{"agent:deploy", "agent:promote", "agent:build", "llm-provider:undeploy"} {
		t.Run(string(action), func(t *testing.T) {
			if got := action.Class(); got != ClassDeployment {
				t.Errorf("Class() = %q, want %q", got, ClassDeployment)
			}
		})
	}
}

// TestIdentityReadsAreNotCritical separates reading identity data from changing
// it, so that listing roles does not trigger the same alert as granting one.
func TestIdentityReadsAreNotCritical(t *testing.T) {
	for _, action := range []Action{"role:read", "user:list", "group:view"} {
		t.Run(string(action), func(t *testing.T) {
			if got := action.Severity(); got == SeverityCritical {
				t.Errorf("Severity() = critical for a read action")
			}
		})
	}
}
