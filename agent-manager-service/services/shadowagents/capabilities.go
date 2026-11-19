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

// CapabilitySpec is one thing a scan lists, and what the connection needs
// for it. The console shows it next to the connection form, evaluated
// against the form.
type CapabilitySpec struct {
	Key         string
	Title       string
	Description string
	// Signals are the evidence codes the capability can produce.
	Signals []string
	// Needs are field or option keys that must all be present: a secret or
	// text field with a value (or a stored secret), a key map with at least
	// one key, a list with at least one item, or a ticked checkbox. Empty
	// means the provider's required fields are enough.
	Needs []string
}

// CredentialChoice requires at least one of Fields to be set: a credential
// (a secret or a key map with a key) or a ticked service option.
type CredentialChoice struct {
	Fields  []string
	Message string
}

// Field and option keys used by capabilities and credential choices.
const (
	FieldScopedKeys = "scopedKeys"
)

// providerCapabilities is what each platform's scan lists. It follows the
// connectors: a capability names the evidence its calls produce and the
// fields those calls need.
var providerCapabilities = map[string][]CapabilitySpec{
	models.ShadowProviderOpenAI: {
		{
			Key: "openai.agents", Title: "Agents",
			Description: "Lists the agents in each project you add a project key for, via the OpenAI Agents API (GET /v1/agents).",
			Signals:     []string{SigOpenAIAgentObject},
			Needs:       []string{FieldScopedKeys},
		},
	},
	models.ShadowProviderAnthropic: {
		{
			Key: "anthropic.agents", Title: "Managed Agents",
			Description: "Lists the Managed Agents in each workspace you add a workspace key for, archived ones included, " +
				"via the Claude Managed Agents API (GET /v1/agents).",
			Signals: []string{SigAnthropicManagedAgent},
			Needs:   []string{FieldScopedKeys},
		},
	},
	models.ShadowProviderBedrock: {
		{
			Key: "bedrock.agentcore", Title: "AgentCore runtimes",
			Description: "Lists the agent runtimes in each region via the AgentCore control plane API (ListAgentRuntimes, " +
				"GetAgentRuntime). A runtime that hosts an MCP server is not an agent and is left out.",
			Signals: []string{SigBedrockAgentCoreRuntime},
			Needs:   []string{OptBedrockAgentCore},
		},
		{
			Key: "bedrock.registry", Title: "AWS Agent Registry agent records",
			Description: "Lists the agent records in each registry via the AWS Agent Registry API (ListRegistries, " +
				"ListRegistryRecords). A record for a runtime this scan found is merged into that runtime.",
			Signals: []string{SigBedrockRegistryAgent},
			Needs:   []string{OptBedrockRegistry},
		},
		{
			Key: "bedrock.classic", Title: "Classic Bedrock agents",
			Description: "Lists the classic Bedrock agents in each region via the Bedrock Agents API (ListAgents, GetAgent, " +
				"ListAgentActionGroups).",
			Signals: []string{SigBedrockClassicAgent},
			Needs:   []string{OptBedrockClassicAgents},
		},
	},
	models.ShadowProviderGoogle: {
		{
			Key: "google.engines", Title: "Agent Runtime engines",
			Description: "Lists the reasoning engines in each project and location via the Vertex AI API " +
				"(GET /v1/projects/{project}/locations/{location}/reasoningEngines).",
			Signals: []string{SigGoogleAgentEngine},
		},
		{
			Key: "google.registry", Title: "Agent Registry agents",
			Description: "Lists the agents in each project's Agent Registry via the Agent Registry API " +
				"(GET /v1/projects/{project}/locations/{location}/agents). An agent that runs on an engine this scan " +
				"found is merged into that engine; Google's built-in agents are left out.",
			Signals: []string{SigGoogleRegistryAgent},
			Needs:   []string{OptGoogleRegistry},
		},
		{
			Key: "google.locations", Title: "Every location",
			Description: "Lists the locations where Agent Runtime and the Agent Registry are available to each project via " +
				"their Locations API (GET /v1/projects/{project}/locations), and scans every one. Without it, only the " +
				"locations you enter are scanned.",
			Needs: []string{OptGoogleAllLocations},
		},
	},
	models.ShadowProviderFoundry: {
		{
			Key: "foundry.agents", Title: "Foundry agents",
			Description: "Lists the prompt, hosted, workflow and external agents in every Foundry project via the Foundry " +
				"Agent Service API (GET {project endpoint}/agents). The projects are found via Azure Resource Manager.",
			Signals: []string{SigFoundryAgentObject},
		},
		{
			Key: "foundry.classic", Title: "Classic agents",
			Description: "Lists the agents created with the classic Assistants-based API via the Foundry Agent Service API " +
				"(GET {project endpoint}/assistants). It retires 31 March 2027.",
			Signals: []string{SigFoundryClassicAgent},
			Needs:   []string{OptClassicAgents},
		},
		{
			Key: "foundry.duplicates", Title: "Possible duplicates of external agents",
			Description: "Flags an external agent with the same name as an agent from another platform as a possible duplicate. " +
				"No extra call is made.",
			Signals: []string{SigFoundryPossibleDupOf},
		},
		{
			Key: "foundry.allSubscriptions", Title: "Every subscription the app can read",
			Description: "Lists every subscription the app can read via Azure Resource Manager (GET /subscriptions). Without it, " +
				"only the subscription IDs you enter are scanned.",
			Needs: []string{OptAllSubscriptions},
		},
	},
}

// providerCredentialChoices are the platforms whose required credential is
// one of several fields.
var providerCredentialChoices = map[string]CredentialChoice{
	models.ShadowProviderBedrock: {
		Fields:  []string{OptBedrockAgentCore, OptBedrockRegistry, OptBedrockClassicAgents},
		Message: "Select at least one Bedrock service to scan.",
	},
}

// sourceCapabilities is what the source code scan of a repository host
// (GitHub, Azure DevOps) analyzes: the design's detectors D1 to D12. Signals
// are the evidence codes the scan command publishes.
func sourceCapabilities(provider, prefix string) []CapabilitySpec {
	codes := func(slugs ...string) []string {
		out := make([]string, 0, len(slugs))
		for _, s := range slugs {
			out = append(out, sourceSignalCode(provider, s))
		}
		return out
	}
	return []CapabilitySpec{
		{
			Key: prefix + ".dependencies", Title: "Agent framework and model SDK dependencies",
			Description: "Agent frameworks and model SDKs in manifests and lockfiles (detectors D1, D10).",
			Signals:     codes("d1_agent_dependency", "d10_model_sdk"),
		},
		{
			Key: prefix + ".code", Title: "Agent, MCP and tool-calling code",
			Description: "Agent framework imports, MCP client and server code, and tool calling with a model SDK in source files " +
				"(detectors D2, D4, D7, D8).",
			Signals: codes("d2_agent_import", "d4_mcp_client_code", "d7_tool_calling", "d8_mcp_server_code"),
		},
		{
			Key: prefix + ".config", Title: "MCP client configuration and A2A agent cards",
			Description: ".mcp.json, mcp.json and Spring application.yml MCP clients, and .well-known/agent-card.json (detectors D3, D5).",
			Signals:     codes("d3_mcp_client_config", "d5_a2a_artifact"),
		},
		{
			Key: prefix + ".iac", Title: "Agent platform infrastructure as code",
			Description: "Agent platform resources in Terraform, CloudFormation and Bicep (detector D6).",
			Signals:     codes("d6_agent_platform_iac"),
		},
		{
			Key: prefix + ".definitions", Title: "Agent and prompt definition files",
			Description: "*.prompty and agent.yaml files (detector D9).",
			Signals:     codes("d9_agent_definition"),
		},
		{
			Key: prefix + ".ci", Title: "LLM environment variable names in CI pipelines",
			Description: "Model provider variable names (never values) in CI pipeline definitions (detector D11).",
			Signals:     codes("d11_llm_env_var"),
			Needs:       []string{OptReadCI},
		},
		{
			Key: prefix + ".developerTooling", Title: "Developer AI tooling",
			Description: ".vscode/mcp.json, .cursor/mcp.json, AGENTS.md and CLAUDE.md, as context only (detector D12).",
			Signals:     codes("d12_developer_tooling"),
		},
	}
}

func init() {
	providerCapabilities[models.ShadowProviderGitHub] = sourceCapabilities(models.ShadowProviderGitHub, "github")
	providerCapabilities[models.ShadowProviderAzureDevOps] = sourceCapabilities(models.ShadowProviderAzureDevOps, "azureDevOps")
}

// Capabilities returns what a platform's scan lists.
func Capabilities(provider string) []CapabilitySpec {
	return providerCapabilities[provider]
}

// CredentialChoiceOf returns the platform's credential choice, if it has one.
func CredentialChoiceOf(provider string) (CredentialChoice, bool) {
	c, ok := providerCredentialChoices[provider]
	return c, ok
}
