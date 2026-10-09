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
	"context"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// TestDomainActionsMatchRouteDerivedActions is the guard that keeps the two
// tiers describing the same operation the same way.
//
// A semantic emit and the coverage tier's fallback for the same route must
// produce an identical action string, or "who deployed agent X" silently
// returns half the answer depending on which tier recorded it.
func TestDomainActionsMatchRouteDerivedActions(t *testing.T) {
	// Each case names the route as the coverage tier sees it — pattern plus the
	// permission that actually gates it — and the action the semantic emit site
	// uses. Both tiers must produce the same string.
	//
	// The permission matters: routes without an actionOverrides entry derive
	// their action from it, so omitting it here would test a fallback that
	// never runs in production.
	pairs := []struct {
		pattern string
		perms   []rbac.Permission
		action  Action
	}{
		{"POST /orgs/{orgName}/git-secrets", []rbac.Permission{rbac.GitSecretCreate}, ActionGitSecretCreate},
		{"DELETE /orgs/{orgName}/git-secrets/{secretName}", []rbac.Permission{rbac.GitSecretDelete}, ActionGitSecretDelete},
		{
			"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/agent-card/source",
			[]rbac.Permission{rbac.AgentUpdate, rbac.AgentEnvNonProduction},
			ActionAgentSetCardSource,
		},
		{
			"DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/agent-card/source",
			[]rbac.Permission{rbac.AgentUpdate, rbac.AgentEnvNonProduction},
			ActionAgentRemoveCardSource,
		},
		{"POST /orgs/{orgName}/llm-providers/{id}/api-keys", nil, ActionAPIKeyCreate},
		{"PUT /orgs/{orgName}/llm-providers/{id}/api-keys/{keyName}", nil, ActionAPIKeyRotate},
		{"DELETE /orgs/{orgName}/llm-providers/{id}/api-keys/{keyName}", nil, ActionAPIKeyRevoke},
		{"POST /orgs/{orgName}/gateways/{gatewayID}/tokens", nil, ActionGatewayTokenRotate},
		{"DELETE /orgs/{orgName}/gateways/{gatewayID}/tokens/{tokenID}", nil, ActionGatewayTokenRevoke},
		{"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/token", nil, ActionAgentTokenMint},
		{
			"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/tracing-token/regenerate",
			nil, ActionAgentTokenRegenerateTracing,
		},
		{"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities", nil, ActionAgentIdentityProvision},
		{
			"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities",
			nil, ActionAgentIdentityRegenerateSecret,
		},
		{
			"DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities",
			nil, ActionAgentIdentityRevokeSecret,
		},
		{
			"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities/retry",
			nil, ActionAgentIdentityRetryProvisioning,
		},
		{"PUT /orgs/{orgName}/environments/{envID}/thunder-system-client", nil, ActionServiceAccountConfigure},
		{"DELETE /orgs/{orgName}/environments/{envID}/thunder-system-client", nil, ActionServiceAccountRemove},
		{"POST /orgs/{orgName}/identities/roles/{roleID}/permissions/add", nil, ActionRoleGrantPermission},
		{"POST /orgs/{orgName}/identities/roles/{roleID}/permissions/remove", nil, ActionRoleRevokePermission},
		{"POST /orgs/{orgName}/identities/roles/{roleID}/assignees/add", nil, ActionRoleAssign},
		{"POST /orgs/{orgName}/identities/roles/{roleID}/assignees/remove", nil, ActionRoleUnassign},
		{"POST /orgs/{orgName}/identities/groups/{groupID}/members/add", nil, ActionGroupAddMember},
		{"POST /orgs/{orgName}/identities/groups/{groupID}/members/remove", nil, ActionGroupRemoveMember},
		{"POST /orgs/{orgName}/identities/users", nil, ActionUserCreate},
		{"POST /orgs/{orgName}/identities/users/invite", nil, ActionUserInvite},
		{"DELETE /orgs/{orgName}/identities/users/{userID}", nil, ActionUserDelete},
		{"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments", nil, ActionAgentDeploy},
		// The promote route declares only the tier floor, so the action can no
		// longer be derived from its permission — actionOverrides supplies it.
		// The audit vocabulary is unchanged: the trail still says agent:promote.
		{
			"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/promote",
			[]rbac.Permission{rbac.AgentEnvNonProduction},
			ActionAgentPromote,
		},
		{
			"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments/state",
			nil, ActionAgentChangeDeploymentState,
		},
		{"DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}", []rbac.Permission{rbac.AgentDelete}, ActionAgentDelete},
		{"DELETE /orgs/{orgName}/projects/{projName}", []rbac.Permission{rbac.ProjectDelete}, ActionProjectDelete},
		{"POST /orgs/{orgName}/gateways/{gatewayID}/environments/{envID}", nil, ActionGatewayAssignEnvironment},
		{"DELETE /orgs/{orgName}/gateways/{gatewayID}/environments/{envID}", nil, ActionGatewayUnassignEnvironment},
		{"PUT /orgs/{orgName}/gateways/{gatewayID}/identity-providers/{name}", nil, ActionGatewaySetIdentityProvider},
		{
			"DELETE /orgs/{orgName}/gateways/{gatewayID}/identity-providers/{name}",
			nil, ActionGatewayRemoveIdentityProvider,
		},
	}

	for _, pair := range pairs {
		t.Run(string(pair.action), func(t *testing.T) {
			method, path := splitPattern(pair.pattern)
			got := deriveAction(method, path, pair.perms)
			if got != pair.action {
				t.Errorf("route %q derives action %q but the semantic emit uses %q; "+
					"the two tiers would split the trail", pair.pattern, got, pair.action)
			}
		})
	}
}

// TestCredentialAndIdentityActionsAreCritical pins the severities that drive
// alerting, so a later edit cannot quietly downgrade a credential or privilege
// change to routine.
func TestCredentialAndIdentityActionsAreCritical(t *testing.T) {
	critical := []Action{
		ActionAPIKeyCreate, ActionAPIKeyRotate, ActionAPIKeyRevoke,
		ActionGitSecretCreate, ActionGitSecretDelete,
		ActionGatewayTokenRotate, ActionGatewayTokenRevoke,
		ActionAgentTokenMint, ActionAgentTokenRegenerateTracing,
		ActionAgentIdentityProvision, ActionAgentIdentityRegenerateSecret, ActionAgentIdentityRevokeSecret,
		ActionAgentIdentityRetryProvisioning,
		ActionServiceAccountConfigure, ActionServiceAccountRemove,
		ActionRoleGrantPermission, ActionRoleRevokePermission,
		ActionRoleAssign, ActionRoleUnassign,
		ActionGroupAddMember, ActionGroupRemoveMember,
		ActionUserInvite, ActionUserCreate, ActionUserUpdate, ActionUserDelete,
		// Changing a gateway's trusted issuers decides whose tokens it accepts.
		ActionGatewaySetIdentityProvider, ActionGatewayRemoveIdentityProvider,
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

// TestPrivilegeChangeRecordsWhatChanged is the point of the semantic tier for
// the escalation path: "a role was updated" is not an audit trail, "alice
// granted SRE deploy-production" is.
func TestPrivilegeChangeRecordsWhatChanged(t *testing.T) {
	sink := NewMemorySink()
	rec := NewRecorder(sink, quietLogger(), Config{BatchSize: 1})
	ctx := WithRecorder(context.Background(), rec)

	granted := []string{"amp:agent:deploy-production", "amp:audit-event:read"}
	Record(
		ctx, ActionRoleGrantPermission,
		ResourceNamed(ResourceRole, "role-77", "SRE"),
		Detail("roleName", "SRE"),
		Detail("permissions", granted),
		Detail("permissionCount", len(granted)),
	)
	if err := rec.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}

	events := sink.Events()
	if len(events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(events))
	}
	e := events[0]

	if e.ResourceName != "SRE" {
		t.Errorf("ResourceName = %q, want the role that was changed", e.ResourceName)
	}
	perms, ok := e.Details["permissions"].([]string)
	if !ok {
		t.Fatalf("permissions detail is %T, want []string", e.Details["permissions"])
	}
	if len(perms) != 2 || perms[0] != "amp:agent:deploy-production" {
		t.Errorf("permissions = %v, want the granted scopes recorded in full", perms)
	}
}

// TestUserCreateRecordsAttributeKeysNotValues covers the free-form attribute map
// that the user-creation route accepts, which carries a password.
func TestUserCreateRecordsAttributeKeysNotValues(t *testing.T) {
	sink := NewMemorySink()
	rec := NewRecorder(sink, quietLogger(), Config{BatchSize: 1})
	ctx := WithRecorder(context.Background(), rec)

	keys, count, sensitive := AttributeKeySummary(map[string]string{
		"username": "carol", "password": "hunter2", "apiToken": "sk-live-xyz",
	})
	Record(
		ctx, ActionUserCreate,
		ResourceNamed(ResourceUser, "carol", "carol"),
		Detail("username", "carol"),
		Detail("attributeKeys", keys),
		Detail("attributeCount", count),
		Detail("containsSensitiveKey", sensitive),
	)
	if err := rec.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}

	events := sink.Events()
	if len(events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(events))
	}

	rendered := renderDetails(events[0].Details)
	for _, secret := range []string{"hunter2", "sk-live-xyz"} {
		if contains(rendered, secret) {
			t.Errorf("attribute value %q reached the record: %s", secret, rendered)
		}
	}
	if !contains(rendered, "password") {
		t.Error("the password key name should be recorded even though its value is not")
	}
	if got, _ := events[0].Details["containsSensitiveKey"].(bool); !got {
		t.Error("containsSensitiveKey should flag the password-shaped attribute")
	}
}

// TestDeployRecordsTargetEnvironment covers the mismatch this event exists to
// resolve: the route is gated by agent:deploy-non-production regardless of
// where the agent lands, so only the record establishes the real target.
func TestDeployRecordsTargetEnvironment(t *testing.T) {
	sink := NewMemorySink()
	rec := NewRecorder(sink, quietLogger(), Config{BatchSize: 1})
	ctx := WithRecorder(context.Background(), rec)

	Record(
		ctx, ActionAgentDeploy,
		ResourceNamed(ResourceAgent, "agent-1", "checkout-agent"),
		Environment("production"),
		Detail("agentName", "checkout-agent"),
		Detail("environment", "production"),
		Detail("isProduction", true),
	)
	if err := rec.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}

	events := sink.Events()
	if len(events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(events))
	}
	if events[0].Environment != "production" {
		t.Errorf("Environment = %q, want the real deploy target", events[0].Environment)
	}
	if isProd, _ := events[0].Details["isProduction"].(bool); !isProd {
		t.Error("isProduction should record that this reached production")
	}
}

func renderDetails(details map[string]any) string {
	out := ""
	for k, v := range details {
		out += k + "="
		switch val := v.(type) {
		case string:
			out += val
		case []string:
			for _, s := range val {
				out += s + ","
			}
		}
		out += " "
	}
	return out
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		}()
}
