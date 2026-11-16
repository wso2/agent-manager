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
			Description: "The agents in each project you add a project key for (GET /v1/agents), with their model, " +
				"tools and MCP servers. Only a project key can list a project's agents.",
			Signals: []string{SigOpenAIAgentObject},
			Needs:   []string{FieldScopedKeys},
		},
	},
	models.ShadowProviderAnthropic: {
		{
			Key: "anthropic.agents", Title: "Managed Agents",
			Description: "The Managed Agents in each workspace you add a workspace key for (GET /v1/agents), archived " +
				"ones included, with their model, tools, MCP servers and skills.",
			Signals: []string{SigAnthropicManagedAgent},
			Needs:   []string{FieldScopedKeys},
		},
	},
	models.ShadowProviderBedrock: {
		{
			Key: "bedrock.agentcore", Title: "AgentCore runtimes",
			Description: "Agent runtimes in each region, with status, version, protocol, execution role and artifact. " +
				"A runtime that hosts an MCP server is not an agent and is left out.",
			Signals: []string{SigBedrockAgentCoreRuntime},
			Needs:   []string{OptBedrockAgentCore},
		},
		{
			Key: "bedrock.registry", Title: "AWS Agent Registry agent records",
			Description: "Agent records in each registry; a record for a runtime this scan found is merged into that runtime.",
			Signals:     []string{SigBedrockRegistryAgent},
			Needs:       []string{OptBedrockRegistry},
		},
		{
			Key: "bedrock.classic", Title: "Classic Bedrock agents",
			Description: "Bedrock agents with their model, orchestration, service role and action groups.",
			Signals:     []string{SigBedrockClassicAgent},
			Needs:       []string{OptBedrockClassicAgents},
		},
	},
	models.ShadowProviderGoogle: {
		{
			Key: "google.engines", Title: "Agent Runtime engines",
			Description: "Reasoning engines in each project and location, with their framework, identity and source.",
			Signals:     []string{SigGoogleAgentEngine},
		},
		{
			Key: "google.registry", Title: "Agent Registry agents",
			Description: "Registered agents; an agent that runs on an engine this scan found is merged into that engine.",
			Signals:     []string{SigGoogleRegistryAgent},
			Needs:       []string{OptGoogleRegistry},
		},
		{
			Key: "google.globalMulti", Title: "Global registry and us/eu multi-regions",
			Description: "Also lists the global Agent Registry location and the us and eu multi-region Agent Runtime endpoints.",
			Needs:       []string{OptGlobalMulti},
		},
	},
	models.ShadowProviderFoundry: {
		{
			Key: "foundry.agents", Title: "Foundry agents",
			Description: "Prompt, hosted, workflow and external agents in every Foundry project, with their model, tools and MCP servers.",
			Signals:     []string{SigFoundryAgentObject},
		},
		{
			Key: "foundry.classic", Title: "Classic agents",
			Description: "Agents created with the classic Assistants-based API (retires 31 March 2027).",
			Signals:     []string{SigFoundryClassicAgent},
			Needs:       []string{OptClassicAgents},
		},
		{
			Key: "foundry.duplicates", Title: "Possible duplicates of external agents",
			Description: "An external agent with the same name as an agent from another platform is flagged as a possible duplicate.",
			Signals:     []string{SigFoundryPossibleDupOf},
		},
		{
			Key: "foundry.allSubscriptions", Title: "Every subscription the app can read",
			Description: "Without this, only the subscription IDs you list are scanned.",
			Needs:       []string{OptAllSubscriptions},
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

// sourceCapabilities is what the source code analysis of a repository host
// (GitHub, Azure DevOps) will analyze: the design's detectors D1 to D12. The
// analysis is added separately; the console marks these as not available yet.
func sourceCapabilities(prefix string) []CapabilitySpec {
	return []CapabilitySpec{
		{
			Key: prefix + ".dependencies", Title: "Agent framework and model SDK dependencies",
			Description: "Agent frameworks, MCP and A2A SDKs and model SDKs in manifests and lockfiles (detectors D1, D10).",
		},
		{
			Key: prefix + ".code", Title: "Agent, MCP and tool-calling code",
			Description: "Agent framework imports, MCP client and server code, and tool calling with a model SDK in source files " +
				"(detectors D2, D4, D7, D8).",
		},
		{
			Key: prefix + ".config", Title: "MCP client configuration and A2A agent cards",
			Description: ".mcp.json, mcp.json and Spring application.yml MCP clients, and .well-known/agent-card.json (detectors D3, D5).",
		},
		{
			Key: prefix + ".iac", Title: "Agent platform infrastructure as code",
			Description: "Agent platform resources in Terraform, CloudFormation and Bicep (detector D6).",
		},
		{
			Key: prefix + ".definitions", Title: "Agent and prompt definition files",
			Description: "*.prompty and agent.yaml files (detector D9).",
		},
		{
			Key: prefix + ".ciEnv", Title: "LLM environment variable names in CI and .env examples",
			Description: "Model provider variable names (never values) in pipeline definitions, Helm and Kubernetes files and " +
				".env.example (detector D11).",
			Needs: []string{OptReadCIAndEnv},
		},
		{
			Key: prefix + ".developerTooling", Title: "Developer AI tooling",
			Description: ".vscode/mcp.json, .cursor/mcp.json, AGENTS.md and CLAUDE.md, as context only (detector D12).",
		},
		{
			Key: prefix + ".secrets", Title: "Hard-coded provider keys",
			Description: "The type, file and line of a hard-coded key, never the value or a hash of it.",
		},
	}
}

func init() {
	providerCapabilities[models.ShadowProviderGitHub] = sourceCapabilities("github")
	providerCapabilities[models.ShadowProviderAzureDevOps] = sourceCapabilities("azureDevOps")
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
