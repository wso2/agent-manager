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

// refusedWhileSuspended reports whether the workload policy on ctx refuses tool
// for the organization ouID. Only a suspension refuses; a policy that cannot
// decide lets the call through, as it does for the equivalent API request.
func refusedWhileSuspended(ctx context.Context, tool, ouID string) bool {
	if !gatewayPushTools[tool] {
		return false
	}
	return errors.Is(workloadpolicy.Check(ctx, ouID), workloadpolicy.ErrSuspended)
}
