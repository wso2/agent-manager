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
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/growthanalytics"
)

type toolUsage struct {
	code           string
	action         string
	creationMethod string
}

var trackedTools = map[string]toolUsage{
	"create_external_agent":         {code: "amp.agent-development.create-agent", action: "created-agent", creationMethod: "external"},
	"create_internal_agent_python":  {code: "amp.agent-development.create-agent", action: "created-agent", creationMethod: "platform-hosted"},
	"create_project":                {code: "amp.deployment-ops.create-project", action: "created-project", creationMethod: ""},
	"build_agent":                   {code: "amp.agent-development.build-agent", action: "built-agent", creationMethod: ""},
	"deploy_agent":                  {code: "amp.deployment-ops.deploy-agent", action: "deployed-agent", creationMethod: ""},
	"update_deployment_state":       {code: "amp.deployment-ops.deploy-agent.suspend", action: "invalid-deployment-state", creationMethod: ""},
	"add_llm_provider_guardrails":   {code: "amp.connections.create-llm-provider", action: "updated-llm-provider", creationMethod: ""},
	"remove_llm_provider_guardrail": {code: "amp.connections.create-llm-provider", action: "updated-llm-provider", creationMethod: ""},
	"add_llm_provider_to_catalog":   {code: "amp.connections.create-llm-provider", action: "updated-llm-provider-catalog-status", creationMethod: ""},
	"attach_llm_provider_to_agent":  {code: "amp.agent-development.bind-model-config", action: "created-model-config-binding", creationMethod: ""},
	"attach_mcp_proxy_to_agent":     {code: "amp.agent-development.bind-mcp-config", action: "created-mcp-config-binding", creationMethod: ""},
}

func reportToolUsage(ctx context.Context, req *gomcp.CallToolRequest, name string, input any, succeeded bool) {
	if req == nil || req.Extra == nil || req.Extra.TokenInfo == nil {
		return
	}
	report, ok := req.Extra.TokenInfo.Extra[growthanalytics.MCPReporterKey].(growthanalytics.MCPReporter)
	if !ok {
		return
	}
	usage, ok := trackedTools[name]
	if !ok {
		return
	}

	dimensions := map[string]interface{}{"channel": "mcp", "action": usage.action}
	if usage.creationMethod != "" {
		dimensions["creation_method"] = usage.creationMethod
	}
	if update, ok := input.(updateDeploymentStateInput); ok {
		switch strings.ToLower(strings.TrimSpace(update.State)) {
		case "redeploy":
			dimensions["action"] = "resumed-agent-deployment"
		case "undeploy":
			dimensions["action"] = "suspended-agent-deployment"
		}
	}
	if name == "build_agent" {
		dimensions["outcome"] = growthanalytics.DynamicOutcome
	}
	report(ctx, usage.code, dimensions, succeeded)
}
