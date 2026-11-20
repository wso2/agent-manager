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

	SigCustomAgent = "custom.agent"
)

// defs is the evidence catalog, in display order: the platform listings,
// then the source evidence of each repository host.
var defs = append([]SignalDef{
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
	{
		Code: SigCustomAgent, Provider: models.ShadowProviderCustom, Name: "Agent in your inventory",
		Condition: `GET {Agents URL}?limit=100 (Authorization: Bearer <API key>): an item in agents with an id and a name`,
	},
}, sourceDefs()...)

// sourceEvidence is what the source code scan finds in a repository: the
// design's detectors D1 to D12, at each repository host, so their
// codes cannot drift.
var sourceEvidence = []struct{ slug, name, condition string }{
	{
		"d1_agent_dependency", "Agent framework dependency",
		"A manifest or lockfile declares an agent framework (LangChain or LangGraph, CrewAI, AutoGen, Semantic Kernel, " +
			"OpenAI Agents SDK, Google ADK, Spring AI)",
	},
	{"d2_agent_import", "Agent framework import", "A source file imports an agent framework"},
	{"d3_mcp_client_config", "MCP client config", "An .mcp.json or mcp.json file, or an MCP client section in a Spring application.yml"},
	{"d4_mcp_client_code", "MCP client code", "A source file creates an MCP client"},
	{"d5_a2a_artifact", "A2A artifact", "A .well-known/agent-card.json file, or a dependency on or import of the A2A SDK"},
	{
		"d6_agent_platform_iac", "Agent platform IaC",
		"Terraform, CloudFormation or Bicep declares an agent platform resource (Bedrock agent or AgentCore runtime, " +
			"Vertex AI reasoning engine, Azure AI Foundry agent)",
	},
	{"d7_tool_calling", "Tool calling with a model SDK", "A source file passes tool definitions to a model SDK call"},
	{"d8_mcp_server_code", "MCP server code", "A source file creates an MCP server"},
	{"d9_agent_definition", "Agent or prompt definition file", "A *.prompty or agent.yaml file"},
	{"d10_model_sdk", "Model SDK only", "A manifest or lockfile declares a model SDK and no agent framework"},
	{
		"d11_llm_env_var", "LLM environment variable name",
		"A model provider variable name (never its value) in a CI pipeline definition",
	},
	{
		"d12_developer_tooling", "Developer AI tooling",
		"A .vscode/mcp.json, .cursor/mcp.json, AGENTS.md or CLAUDE.md file; context only, not an agent on its own",
	},
}

// sourceProviders are the repository hosts the source code scan reads, in
// display order.
var sourceProviders = []string{models.ShadowProviderGitHub}

// sourceSignalCode is the evidence code of a source detector at a
// repository host, e.g. github.d1_agent_dependency.
func sourceSignalCode(provider, slug string) string {
	return provider + "." + slug
}

// sourceDefs returns the source evidence of every repository host.
func sourceDefs() []SignalDef {
	out := make([]SignalDef, 0, len(sourceProviders)*len(sourceEvidence))
	for _, p := range sourceProviders {
		for _, e := range sourceEvidence {
			out = append(out, SignalDef{Code: sourceSignalCode(p, e.slug), Provider: p, Name: e.name, Condition: e.condition})
		}
	}
	return out
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
