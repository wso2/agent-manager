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

package gwapi

import (
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
)

// TestSemanticActionsMatchRouteActions keeps the record a service writes and
// the record its REST route writes under one action, so "who rotated this
// key" is one query whichever surface the call came from.
func TestSemanticActionsMatchRouteActions(t *testing.T) {
	pairs := map[string]audit.Action{
		"POST /orgs/{orgName}/llm-providers/{id}/api-keys":                                                     gwaudit.APIKeyCreate,
		"PUT /orgs/{orgName}/llm-providers/{id}/api-keys/{keyName}":                                            gwaudit.APIKeyRotate,
		"DELETE /orgs/{orgName}/llm-providers/{id}/api-keys/{keyName}":                                         gwaudit.APIKeyRevoke,
		"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys/test":       gwaudit.APIKeyIssueTest,
		"POST /orgs/{orgName}/gateways/{gatewayID}/tokens":                                                     gwaudit.GatewayTokenRotate,
		"DELETE /orgs/{orgName}/gateways/{gatewayID}/tokens/{tokenID}":                                         gwaudit.GatewayTokenRevoke,
		"POST /orgs/{orgName}/gateways":                                                                        gwaudit.GatewayCreate,
		"PUT /orgs/{orgName}/gateways/{gatewayID}":                                                             gwaudit.GatewayUpdate,
		"DELETE /orgs/{orgName}/gateways/{gatewayID}":                                                          gwaudit.GatewayDelete,
		"POST /orgs/{orgName}/gateways/{gatewayID}/environments/{envID}":                                       gwaudit.GatewayAssignEnvironment,
		"DELETE /orgs/{orgName}/gateways/{gatewayID}/environments/{envID}":                                     gwaudit.GatewayUnassignEnvironment,
		"PUT /orgs/{orgName}/gateways/{gatewayID}/identity-providers/{name}":                                   gwaudit.GatewaySetIdentityProvider,
		"DELETE /orgs/{orgName}/gateways/{gatewayID}/identity-providers/{name}":                                gwaudit.GatewayRemoveIdentityProvider,
		"PUT /orgs/{orgName}/llm-providers/{providerId}":                                                       gwaudit.LLMProviderUpdate,
		"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}":                  gwaudit.AgentConfigUpdate,
		"DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}":               gwaudit.AgentConfigDelete,
		"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/agent-card/source":    gwaudit.AgentSetCardSource,
		"DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/agent-card/source": gwaudit.AgentRemoveCardSource,
		"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/a2a-proxy":            gwaudit.AgentSetA2AProxy,
		"DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/a2a-proxy":         gwaudit.AgentRemoveA2AProxy,
	}
	for pattern, want := range pairs {
		opts, ok := routeOptions[pattern]
		if !ok {
			t.Errorf("%s declares no route options", pattern)
			continue
		}
		if got := audit.Action(gwperm.Module + ":" + modkit.Options(opts).AuditAction); got != want {
			t.Errorf("%s records %q but its service records %q", pattern, got, want)
		}
	}
	internal := map[string]audit.Action{
		"GET /llm-providers/api-keys":         gwaudit.APIKeySync,
		"POST /deployments/fetch-batch":       gwaudit.DeploymentSync,
		"POST /artifacts/exists":              gwaudit.ArtifactCheckExists,
		"POST /gateways/{gatewayId}/manifest": gwaudit.GatewayPushManifest,
	}
	for pattern, want := range internal {
		if got := audit.Action(gwperm.Module + ":" + modkit.Options(internalRouteOptions[pattern]).AuditAction); got != want {
			t.Errorf("%s records %q but its handler records %q", pattern, got, want)
		}
	}
}

// TestRouteOptionsNameDeclaredActionsAndEvents: every option names one of
// the manifest's audit actions and webhook events.
func TestRouteOptionsNameDeclaredActionsAndEvents(t *testing.T) {
	actions := map[string]bool{}
	for _, a := range gwaudit.Actions {
		actions[a.Name] = true
	}
	events := map[string]bool{}
	for _, e := range gwaudit.Events {
		events[e.Name] = true
	}
	for _, table := range []map[string][]modkit.RouteOption{routeOptions, internalRouteOptions} {
		for pattern, opts := range table {
			o := modkit.Options(opts)
			if o.AuditAction != "" && !actions[o.AuditAction] {
				t.Errorf("%s records %q, which gwaudit.Actions does not declare", pattern, o.AuditAction)
			}
			if o.Event != "" && !events[o.Event] {
				t.Errorf("%s emits %q, which gwaudit.Events does not declare", pattern, o.Event)
			}
		}
	}
}
