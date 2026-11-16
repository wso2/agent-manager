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

package shadowagents

import "github.com/wso2/agent-manager/agent-manager-service/models"

// SignalDef describes one kind of evidence: the platform listing that
// returns an agent, and the exact condition, in the platform's official
// field names, under which it does.
type SignalDef struct {
	Code      string
	Provider  string
	Name      string
	Condition string
}

// Evidence codes. Every finding is an agent a platform lists; its evidence
// names the listing (or listings) that returned it.
const (
	SigOpenAIAgentObject = "openai.agent_object"

	SigAnthropicManagedAgent = "anthropic.managed_agent"

	SigBedrockAgentCoreRuntime = "bedrock.agentcore_runtime"
	SigBedrockRegistryAgent    = "bedrock.registry_agent_record"
	SigBedrockClassicAgent     = "bedrock.classic_agent"

	SigGoogleAgentEngine   = "google.agent_runtime_engine"
	SigGoogleRegistryAgent = "google.registry_agent"

	SigFoundryAgentObject   = "foundry.agent_object"
	SigFoundryClassicAgent  = "foundry.classic_agent"
	SigFoundryPossibleDupOf = "foundry.external_possible_duplicate"
)

// defs is the evidence catalog, in display order.
var defs = []SignalDef{
	{
		Code: SigOpenAIAgentObject, Provider: models.ShadowProviderOpenAI, Name: "Agent object",
		Condition: `GET /v1/agents (OpenAI-Beta: agents=v1, project key): an item in data`,
	},
	{
		Code: SigAnthropicManagedAgent, Provider: models.ShadowProviderAnthropic, Name: "Managed Agent",
		Condition: `GET /v1/agents?include_archived=true (managed-agents beta, workspace key): an item in data (archived_at == null means active)`,
	},
	{
		Code: SigBedrockAgentCoreRuntime, Provider: models.ShadowProviderBedrock, Name: "AgentCore runtime",
		Condition: `ListAgentRuntimes: a runtime, with GetAgentRuntime protocolConfiguration.serverProtocol other than "MCP" (an MCP server is not an agent)`,
	},
	{
		Code: SigBedrockRegistryAgent, Provider: models.ShadowProviderBedrock, Name: "Agent Registry agent record",
		Condition: `agent-registry ListRegistryRecords: a record with recordType == "AGENT" (merged into the AgentCore runtime its provenance sourceId names)`,
	},
	{
		Code: SigBedrockClassicAgent, Provider: models.ShadowProviderBedrock, Name: "Classic Bedrock agent",
		Condition: `bedrock-agent ListAgents: an item in agentSummaries`,
	},
	{
		Code: SigGoogleAgentEngine, Provider: models.ShadowProviderGoogle, Name: "Agent Runtime engine",
		Condition: `reasoningEngines.list: an item in reasoningEngines; spec.agentFramework names the framework`,
	},
	{
		Code: SigGoogleRegistryAgent, Provider: models.ShadowProviderGoogle, Name: "Agent Registry agent",
		Condition: `Agent Registry agents.list: an agent entry other than Google's built-in agents (merged into the engine its RuntimeReference names)`,
	},
	{
		Code: SigFoundryAgentObject, Provider: models.ShadowProviderFoundry, Name: "Foundry agent",
		Condition: `GET {projectEndpoint}/agents?api-version=v1: an item in data (state == "enabled" means active)`,
	},
	{
		Code: SigFoundryClassicAgent, Provider: models.ShadowProviderFoundry, Name: "Foundry classic agent",
		Condition: `GET {projectEndpoint}/assistants (classic Agent Service): an item in data`,
	},
	{
		Code: SigFoundryPossibleDupOf, Provider: models.ShadowProviderFoundry, Name: "Possibly registered elsewhere",
		Condition: `definition.kind == "external": Foundry stores no endpoint for external agents, so an agent from another connection with the same name is flagged as a possible duplicate`,
	},
}

var catalog = func() map[string]SignalDef {
	m := make(map[string]SignalDef, len(defs))
	for _, d := range defs {
		m[d.Code] = d
	}
	return m
}()

// Def returns the definition of an evidence code.
func Def(code string) (SignalDef, bool) {
	d, ok := catalog[code]
	return d, ok
}

// Defs returns every evidence definition, in display order.
func Defs() []SignalDef {
	out := make([]SignalDef, len(defs))
	copy(out, defs)
	return out
}
