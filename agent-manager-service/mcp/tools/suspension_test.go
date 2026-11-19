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

package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/workloadpolicy"
)

// toolsThatPushNothingToAGateway are the mutating tools left out of
// gatewayPushTools, and why. A suspended organization's builds, deploys and
// component creates are refused below them, by the OpenChoreo client.
var toolsThatPushNothingToAGateway = map[string]string{
	"add_llm_provider_to_catalog":  "only marks the provider as listed in the catalog",
	"build_agent":                  "builds through OpenChoreo",
	"create_external_agent":        "creates its component through OpenChoreo",
	"create_internal_agent_python": "creates its component and builds through OpenChoreo",
	"create_project":               "creates a project, which runs nothing",
	"deploy_agent":                 "deploys through OpenChoreo",
	"update_deployment_state":      "the service refuses to start a suspended organization's agent; stopping stays open",
}

func TestEveryMutatingToolIsClassifiedForSuspension(t *testing.T) {
	reg := registerMockToolsets()
	for name, action := range reg.actions {
		if action.Class() == audit.ClassRead {
			if gatewayPushTools[name] {
				t.Errorf("read tool %q is in gatewayPushTools", name)
			}
			continue
		}
		_, pushesNothing := toolsThatPushNothingToAGateway[name]
		if gatewayPushTools[name] == pushesNothing {
			t.Errorf("mutating tool %q must be in exactly one of gatewayPushTools and toolsThatPushNothingToAGateway", name)
		}
	}
	for name := range gatewayPushTools {
		if _, ok := reg.actions[name]; !ok {
			t.Errorf("gatewayPushTools names unregistered tool %q", name)
		}
	}
	for name := range toolsThatPushNothingToAGateway {
		if _, ok := reg.actions[name]; !ok {
			t.Errorf("toolsThatPushNothingToAGateway names unregistered tool %q", name)
		}
	}
}

func suspendedGuardCtx(policyErr error, checked *[]string) context.Context {
	ctx := jwtassertion.ContextWithTokenClaimsAndScope(context.Background(), &jwtassertion.TokenClaims{
		OuId:  testOrgName,
		Scope: rbac.LLMProviderUpdate.Scope(),
	})
	return workloadpolicy.WithGuard(ctx, func(_ context.Context, org string) error {
		*checked = append(*checked, org)
		return policyErr
	})
}

func TestAuthzMiddlewareRefusesGatewayPushWhileSuspended(t *testing.T) {
	reg := newToolRegistry()
	reg.permissions["add_llm_provider_guardrails"] = []rbac.Permission{rbac.LLMProviderUpdate}
	var checked []string

	result, nextCalled := callToolViaMiddleware(t, reg, suspendedGuardCtx(workloadpolicy.ErrSuspended, &checked), "add_llm_provider_guardrails")

	if nextCalled {
		t.Fatal("a gateway push ran for a suspended organization")
	}
	if got := denialText(t, result); got != subscriptionSuspendedMessage {
		t.Fatalf("denial text = %q, want %q", got, subscriptionSuspendedMessage)
	}
	if len(checked) != 1 || checked[0] != testOrgName {
		t.Fatalf("policy checked orgs %v, want the token's %q", checked, testOrgName)
	}
}

func TestAuthzMiddlewareAllowsGatewayPushUnlessSuspended(t *testing.T) {
	for name, policyErr := range map[string]error{
		"active organization":       nil,
		"policy that cannot decide": errors.New("lifecycle store unreachable"),
	} {
		t.Run(name, func(t *testing.T) {
			reg := newToolRegistry()
			reg.permissions["attach_llm_provider_to_agent"] = []rbac.Permission{rbac.LLMProviderUpdate}
			var checked []string

			_, nextCalled := callToolViaMiddleware(t, reg, suspendedGuardCtx(policyErr, &checked), "attach_llm_provider_to_agent")

			if !nextCalled {
				t.Fatal("gateway push refused although the organization is not suspended")
			}
		})
	}
}

// Tools that push nothing to a gateway are not asked about here: a suspended
// organization still reads, and its builds and deploys are refused by the
// OpenChoreo client.
func TestAuthzMiddlewareLeavesOtherToolsToTheirServices(t *testing.T) {
	reg := newToolRegistry()
	reg.permissions["list_llm_providers"] = []rbac.Permission{rbac.LLMProviderUpdate}
	var checked []string

	_, nextCalled := callToolViaMiddleware(t, reg, suspendedGuardCtx(workloadpolicy.ErrSuspended, &checked), "list_llm_providers")

	if !nextCalled || len(checked) != 0 {
		t.Fatalf("next ran %t, policy checked %v; want the tool to run unchecked", nextCalled, checked)
	}
}
