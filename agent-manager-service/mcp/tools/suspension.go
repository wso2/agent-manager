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

	"github.com/wso2/agent-manager/agent-manager-service/workloadpolicy"
)

// gatewayPushTools are the tools that deploy a new or changed artifact to a
// gateway. Gateway deploys do not go through OpenChoreo, whose client refuses a
// suspended organization's builds and deploys, so these tools are refused here
// instead, before they run. Every mutating tool must be in this set or in the
// tests' list of tools that push nothing to a gateway
// (TestEveryMutatingToolIsClassifiedForSuspension).
var gatewayPushTools = map[string]bool{
	// Redeploy the provider, with its new guardrails, to its gateways.
	"add_llm_provider_guardrails":   true,
	"remove_llm_provider_guardrail": true,
	// Create an agent configuration, which deploys the agent's LLM or MCP
	// proxy to the gateways of its environments.
	"attach_llm_provider_to_agent": true,
	"attach_mcp_proxy_to_agent":    true,
}

const subscriptionSuspendedMessage = "The Agent Manager subscription is suspended. Renew the subscription to resume."

// checkSuspension applies the workload policy on ctx to tool for the
// organization ouID. It returns nil for a tool that pushes nothing to a gateway
// and when the policy allows the call, workloadpolicy.ErrSuspended when the
// organization is suspended, and the policy's error when it cannot decide, so
// the call stops as the equivalent API request does.
func checkSuspension(ctx context.Context, tool, ouID string) error {
	if !gatewayPushTools[tool] {
		return nil
	}
	return workloadpolicy.Check(ctx, ouID)
}
