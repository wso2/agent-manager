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

// Package agentgwv1 is the "agent.gateway/v1" capability: the gateway's part
// in an agent's lifecycle. The core calls it around creating and deleting an
// agent, outside its own transaction, because the gateway's work (revoking
// credentials, withdrawing what gateways serve) talks to other systems and
// retries.
package agentgwv1

import (
	"context"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts"
)

// ID is the capability id.
const ID = "agent.gateway/v1"

// Gateway is called by the core as agents come and go.
type Gateway interface {
	// CheckCreate runs before a new agent is written. It refuses a name that
	// still has gateway state from a deleted agent of that name.
	CheckCreate(ctx context.Context, agent contracts.AgentRef) error
	// Created runs right after the agent is written and clears state a
	// deleted agent of the same name left behind. An error makes the core
	// delete the agent again.
	Created(ctx context.Context, agent contracts.AgentRef) error
	// Unbind removes the agent's LLM and MCP bindings and revokes their
	// credentials, retrying for a while. It runs before an agent is deleted
	// and when a create fails after binding. It does not fail: what it
	// cannot revoke it keeps, so CheckCreate refuses the name until a later
	// delete clears it.
	Unbind(ctx context.Context, agent contracts.AgentRef)
	// Deleting withdraws the agent from the gateways before the agent is
	// deleted. The delete goes ahead either way; an error is reported to
	// the caller afterwards, and deleting the agent again retries.
	Deleting(ctx context.Context, agent contracts.AgentRef) error
}
