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

// Package llmv1 is the "llm.proxy/v1" capability: giving agents access to the
// LLM providers of their organization's catalog through a gateway.
package llmv1

import (
	"context"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// ID is the capability id.
const ID = "llm.proxy/v1"

// Proxy binds agents to LLM providers.
type Proxy interface {
	// ValidateProviders refuses providers the organization's catalog does
	// not have.
	ValidateProviders(ctx context.Context, orgID string, providers []string) error
	// BindAgent gives an agent the LLM providers it asked for at creation,
	// in one environment.
	BindAgent(ctx context.Context, agent contracts.AgentRef, environment string, configs []spec.ModelConfigRequest) error
}
