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

import (
	"fmt"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// Field kinds of a connection form.
const (
	FieldText     = "text"
	FieldSecret   = "secret"
	FieldTextarea = "textarea"
	FieldList     = "list"
	FieldCheckbox = "checkbox"
	FieldKeyMap   = "key_map"
)

// EndpointSpec is one provider endpoint a connection calls. Default is the
// official base URL, or the official pattern with placeholders.
type EndpointSpec struct {
	Key          string
	Label        string
	Default      string
	Placeholders []string
	// OfficialHostSuffixes are the hosts the provider officially serves the
	// endpoint from; the UI warns (and nothing more) when a user enters a
	// host outside them.
	OfficialHostSuffixes []string
	// Option, when set, names the checkbox that enables this endpoint.
	Option string
	// An endpoint with one placeholder is used once per value: FillFrom names
	// the list field whose items fill it (e.g. "regions"), less FillExclude;
	// FillValues are fixed values instead. The user can replace the URL of
	// one value; it is stored as "<key>@<value>".
	FillFrom    string
	FillExclude []string
	FillValues  []string
	// Advanced endpoints are kept behind the "Advanced configuration" button.
	Advanced bool
}

// FieldSpec is one field of a provider's connection form.
type FieldSpec struct {
	Key         string
	Label       string
	Kind        string
	Required    bool
	Placeholder string
	Help        string
	// DefaultBool is a checkbox's initial state.
	DefaultBool bool
	// Option, when set, names the checkbox that shows this field; a
	// Required field with an Option is required only when it is ticked.
	// "!key" shows the field while the checkbox is not ticked.
	Option string
	// Recommended marks an optional field the console recommends.
	Recommended bool
	// Section is a heading the console shows the field under.
	Section string
	// Advanced fields are optional settings the console keeps behind an
	// "Advanced configuration" button until it is opened.
	Advanced bool
}

// PermissionSpec explains one credential the customer creates, and the
// least privilege it needs.
type PermissionSpec struct {
	Title string
	// Summary says in one or two plain sentences what Agent Manager does
	// with this credential or source; the console shows it next to the form.
	Summary string
	// Reads lists what Agent Manager reads with it.
	Reads []string
	// Items are how to create it and the permissions it needs.
	Items []string
	// Policy is a ready-to-use role or policy document, when there is one.
	Policy string
	Note   string
}

// ProviderSpec describes a provider to the UI: how to connect, what to
// grant and what the scan can find.
type ProviderSpec struct {
	Provider    string
	DisplayName string
	Description string
	Endpoints   []EndpointSpec
	Fields      []FieldSpec
	Permissions []PermissionSpec
	Docs        []string
	// SourceHost marks a source repository host (GitHub, Azure DevOps): its
	// source code analysis is added separately.
	SourceHost bool
}

// Endpoint keys.
const (
	EPOpenAIAPI = "api"

	EPAnthropicAPI        = "api"
	EPAzureLogin          = "login"
	EPAzureARM            = "arm"
	EPBedrockSTS          = "sts"
	EPBedrockAgentCore    = "agentcoreControl"
	EPBedrockRegistry     = "agentRegistry"
	EPBedrockAgent        = "bedrockAgent"
	EPGoogleToken         = "token"
	EPGoogleAIPlatform    = "aiplatform"
	EPGoogleAIPlatformRep = "aiplatformMultiRegion"
	EPGoogleRegistry      = "agentRegistry"
	EPGitHubAPI           = "api"
	EPAzureDevOps         = "azureDevOps"
)

// Option (checkbox) keys.
const (
	OptAllSubscriptions = "allSubscriptions"
	OptClassicAgents    = "includeClassicAgents"
	OptGlobalMulti      = "includeGlobalAndMultiRegion"
	OptAllRepositories  = "allRepositories"
	OptAllProjects      = "allProjects"
	OptUseEntra         = "useEntra"
	OptReadCIAndEnv     = "readCiAndEnvFiles"
)

// FieldPersonalAccessToken is the GitHub and Azure DevOps token field.
const FieldPersonalAccessToken = "personalAccessToken"

// OptionActive reports whether a field or endpoint with this Option applies,
// given the checkbox values: "" always, "key" when ticked, "!key" when not.
func OptionActive(option string, values map[string]bool) bool {
	if option == "" {
		return true
	}
	if key, negated := strings.CutPrefix(option, "!"); negated {
		return !values[key]
	}
	return values[option]
}

// OptionKey is the checkbox an Option names, without the "!" prefix.
func OptionKey(option string) string {
	return strings.TrimPrefix(option, "!")
}

// Service options: optional services the user can untick. An unticked
// service is not called, and its endpoint is not shown.
const (
	OptBedrockAgentCore     = "scanAgentCore"
	OptBedrockRegistry      = "scanAgentRegistry"
	OptBedrockClassicAgents = "scanClassicAgents"

	OptGoogleRegistry = "scanAgentRegistry"
)

// serviceOptions are the option keys stored as DisabledServices when off
// (OptGoogleRegistry is the same key as OptBedrockRegistry).
var serviceOptions = map[string]bool{
	OptBedrockAgentCore: true, OptBedrockRegistry: true, OptBedrockClassicAgents: true,
}

// IsServiceOption reports whether an option key turns a service on or off.
func IsServiceOption(key string) bool { return serviceOptions[key] }

// ServiceEnabled reports whether the connection scans an optional service.
func ServiceEnabled(cfg models.ShadowConnectionConfig, key string) bool {
	for _, k := range cfg.DisabledServices {
		if k == key {
			return false
		}
	}
	return true
}

// SectionServices is the heading of the service options in the form.
const SectionServices = "Services to scan"

// ReasonServiceOff is recorded for the calls of an unticked service.
const ReasonServiceOff = "not selected for this platform"

// serviceField is the checkbox of an optional service, ticked by default.
func serviceField(key, label, help string) FieldSpec {
	return FieldSpec{Key: key, Label: label, Kind: FieldCheckbox, DefaultBool: true, Help: help, Section: SectionServices}
}

var azureEndpoints = []EndpointSpec{
	{
		Key: EPAzureLogin, Label: "Microsoft Entra login", Default: "https://login.microsoftonline.com",
		OfficialHostSuffixes: []string{"login.microsoftonline.com"}, Advanced: true,
	},
	{
		Key: EPAzureARM, Label: "Azure Resource Manager", Default: "https://management.azure.com",
		OfficialHostSuffixes: []string{"management.azure.com"}, Advanced: true,
	},
}

var azureFields = []FieldSpec{
	{Key: "tenantId", Label: "Directory (tenant) ID", Kind: FieldText, Required: true, Placeholder: "e.g. 72f988bf-86f1-41af-91ab-2d7cd011db47"},
	{Key: "clientId", Label: "Application (client) ID", Kind: FieldText, Required: true, Placeholder: "e.g. 0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b"},
	{Key: "clientSecret", Label: "Client secret", Kind: FieldSecret, Required: true, Placeholder: "Client secret value"},
	{Key: OptAllSubscriptions, Label: "Scan every subscription the app can read", Kind: FieldCheckbox, DefaultBool: true},
	{
		Key: "subscriptionIds", Label: "Subscription IDs", Kind: FieldList, Placeholder: "e.g. 00000000-0000-0000-0000-000000000000",
		Required: true, Option: "!" + OptAllSubscriptions, Help: "The subscriptions to scan.",
	},
}

var providerSpecs = []ProviderSpec{
	{
		Provider:    models.ShadowProviderOpenAI,
		DisplayName: "OpenAI",
		Description: "OpenAI API platform: the agents in each project (GET /v1/agents, with a project key).",
		Endpoints: []EndpointSpec{{
			Key: EPOpenAIAPI, Label: "OpenAI API", Default: "https://api.openai.com/v1",
			OfficialHostSuffixes: []string{"api.openai.com"},
		}},
		Fields: []FieldSpec{
			{
				Key: FieldScopedKeys, Label: "Project keys", Kind: FieldKeyMap, Required: true, Placeholder: "proj_abc123",
				Help: "A read-only project key (scope api.agents.read or api.all.read) per project ID. GET /v1/agents is a " +
					"project endpoint: only a project's own key lists its agents.",
			},
		},
		Permissions: []PermissionSpec{
			{
				Title:   "Project keys",
				Summary: "Lists the agents of each project you add a key for. Every project you want scanned needs its own key.",
				Reads: []string{
					"Agents (GET /v1/agents): ID, name, model, creation time, the type and name of each tool, and the label and URL origin of each MCP server.",
					"Never an agent's instructions or tool definitions.",
				},
				Items: []string{
					"One key per project, ideally a service account key with the scope api.agents.read (or api.all.read).",
					"Add the project ID (proj_...) and its key as one row.",
					"An Admin API key cannot list agents: GET /v1/agents accepts project keys only.",
				},
			},
		},
		Docs: []string{"https://developers.openai.com/api/docs/guides/agents-api/overview"},
	},
	{
		Provider:    models.ShadowProviderAnthropic,
		DisplayName: "Anthropic",
		Description: "Claude Developer Platform: the Managed Agents in each workspace (GET /v1/agents, with a workspace key).",
		Endpoints: []EndpointSpec{
			{Key: EPAnthropicAPI, Label: "Anthropic API", Default: "https://api.anthropic.com", OfficialHostSuffixes: []string{"api.anthropic.com"}},
		},
		Fields: []FieldSpec{
			{
				Key: FieldScopedKeys, Label: "Workspace keys", Kind: FieldKeyMap, Required: true, Placeholder: "wrkspc_01ABC...",
				Help: "An API key per workspace ID. Managed Agents are listed per workspace, with that workspace's key.",
			},
		},
		Permissions: []PermissionSpec{
			{
				Title:   "Workspace keys",
				Summary: "Lists the Managed Agents of each workspace you add a key for, archived ones included.",
				Reads: []string{
					"Managed Agents (GET /v1/agents): ID, name, model, creation and archive time, the type and name of each tool, MCP server names and URL origins, and skill IDs.",
					"Never an agent's system prompt or tool schemas.",
				},
				Items: []string{
					"A key per workspace (or a service account key added to the workspaces).",
					"Add the workspace ID (wrkspc_...) and its key as one row.",
				},
			},
		},
		Docs: []string{"https://platform.claude.com/docs/en/managed-agents/overview"},
	},
	{
		Provider:    models.ShadowProviderBedrock,
		DisplayName: "Amazon Bedrock",
		Description: "AgentCore runtimes, AWS Agent Registry agent records and classic Bedrock agents in each region.",
		Endpoints: []EndpointSpec{
			// STS is used only to assume a role, so it shows only when one is set.
			{Key: EPBedrockSTS, Label: "AWS STS", Default: "https://sts.{region}.amazonaws.com", Placeholders: []string{"region"}, FillFrom: "regions", OfficialHostSuffixes: []string{".amazonaws.com"}, Option: "roleArn"},
			{Key: EPBedrockAgentCore, Label: "AgentCore control plane", Default: "https://bedrock-agentcore-control.{region}.amazonaws.com", Placeholders: []string{"region"}, FillFrom: "regions", OfficialHostSuffixes: []string{".amazonaws.com"}, Option: OptBedrockAgentCore},
			{Key: EPBedrockRegistry, Label: "AWS Agent Registry", Default: "https://agent-registry-control.{region}.api.aws", Placeholders: []string{"region"}, FillFrom: "regions", OfficialHostSuffixes: []string{".api.aws"}, Option: OptBedrockRegistry},
			{Key: EPBedrockAgent, Label: "Bedrock Agents", Default: "https://bedrock-agent.{region}.amazonaws.com", Placeholders: []string{"region"}, FillFrom: "regions", OfficialHostSuffixes: []string{".amazonaws.com"}, Option: OptBedrockClassicAgents},
		},
		Fields: []FieldSpec{
			{Key: "regions", Label: "Regions", Kind: FieldList, Required: true, Placeholder: "e.g. us-east-1"},
			{Key: "accessKeyId", Label: "Access key ID", Kind: FieldText, Required: true, Placeholder: "AKIA..."},
			{Key: "secretAccessKey", Label: "Secret access key", Kind: FieldSecret, Required: true},
			{Key: "sessionToken", Label: "Session token", Kind: FieldSecret, Advanced: true},
			{
				Key: "roleArn", Label: "Role to assume", Kind: FieldText, Placeholder: "arn:aws:iam::123456789012:role/AmpShadowDiscovery",
				Help: "A cross-account read-only role. When set, the access key is used only to assume it.", Advanced: true,
			},
			{Key: "externalId", Label: "External ID", Kind: FieldText, Advanced: true},
			serviceField(OptBedrockAgentCore, "Scan AgentCore runtimes",
				"AgentCore control plane: agent runtimes (a runtime that hosts an MCP server is not an agent and is left out)."),
			serviceField(OptBedrockRegistry, "Scan the AWS Agent Registry", "Agent records in the registries."),
			serviceField(OptBedrockClassicAgents, "Scan classic Bedrock agents", "Bedrock Agents and their action groups."),
		},
		Permissions: []PermissionSpec{
			{
				Title:   "AWS credentials",
				Summary: "An access key that can only read. Agent Manager signs read-only calls with it in each region you list; with a role to assume, the key is used only to get the role's temporary credentials.",
				Items: []string{
					"An access key ID and secret access key; add the session token only for temporary credentials.",
					"Role to assume (Advanced configuration): a cross-account read-only role, called through STS AssumeRole, with the external ID its trust policy requires.",
				},
			},
			{
				Title:   "Read-only IAM policy",
				Summary: "Every action is a Read or List action. Attach it to the role or user Agent Manager uses.",
				Items: []string{
					"There is no AgentCore read-only managed policy; AmazonBedrockReadOnly and AgentRegistryReadOnlyAccess cover parts of it.",
					"Leave out the actions of services you untick: bedrock-agentcore:* (AgentCore runtimes), agent-registry:* (Agent Registry), bedrock:* (classic agents).",
				},
				Policy: `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["bedrock-agentcore:ListAgentRuntimes","bedrock-agentcore:GetAgentRuntime","agent-registry:ListRegistries","agent-registry:ListRegistryRecords","bedrock:ListAgents","bedrock:GetAgent","bedrock:ListAgentActionGroups"],"Resource":"*"}]}`,
			},
			{
				Title:   "What Agent Manager lists",
				Summary: "The agents AWS knows about: AgentCore runtimes, Agent Registry agent records and classic Bedrock agents.",
				Reads: []string{
					"Runtimes: name, status, version, execution role, protocol and artifact type (never environment variables or descriptions).",
					"Registry agent records: name, status, version and the runtime they describe.",
					"Classic agents: name, status, model, orchestration type, service role and action group names.",
				},
			},
		},
		Docs: []string{"https://docs.aws.amazon.com/bedrock-agentcore/latest/APIReference/Welcome.html", "https://docs.aws.amazon.com/bedrock/latest/APIReference/API_agent_ListAgents.html"},
	},
	{
		Provider:    models.ShadowProviderGoogle,
		DisplayName: "Google Agent Platform",
		Description: "Agent Runtime engines (reasoning engines) and Agent Registry agents in each project and location.",
		Endpoints: []EndpointSpec{
			{Key: EPGoogleToken, Label: "OAuth token endpoint", Default: "https://oauth2.googleapis.com/token", OfficialHostSuffixes: []string{"oauth2.googleapis.com"}, Advanced: true},
			// Agent Runtime has no global location, and us/eu use the multi-region endpoint.
			{Key: EPGoogleAIPlatform, Label: "Agent Runtime (regional)", Default: "https://{location}-aiplatform.googleapis.com", Placeholders: []string{"location"}, OfficialHostSuffixes: []string{"aiplatform.googleapis.com"}, FillFrom: "locations", FillExclude: []string{"global", "us", "eu"}},
			{Key: EPGoogleAIPlatformRep, Label: "Agent Runtime (us/eu multi-region)", Default: "https://aiplatform.{location}.rep.googleapis.com", Placeholders: []string{"location"}, OfficialHostSuffixes: []string{".rep.googleapis.com"}, Option: OptGlobalMulti, FillValues: []string{"us", "eu"}},
			{Key: EPGoogleRegistry, Label: "Agent Registry", Default: "https://agentregistry.googleapis.com", OfficialHostSuffixes: []string{"agentregistry.googleapis.com"}, Option: OptGoogleRegistry},
		},
		Fields: []FieldSpec{
			{Key: "projectIds", Label: "Project IDs", Kind: FieldList, Required: true, Placeholder: "e.g. my-agents-project"},
			{Key: "locations", Label: "Locations", Kind: FieldList, Required: true, Placeholder: "e.g. us-central1"},
			{Key: "serviceAccountJson", Label: "Service account key (JSON)", Kind: FieldSecret, Required: true, Placeholder: "{\"type\": \"service_account\", ...}"},
			{Key: OptGlobalMulti, Label: "Include the global Agent Registry location and the us/eu multi-regions", Kind: FieldCheckbox, DefaultBool: true, Advanced: true},
			serviceField(OptGoogleRegistry, "Scan the Agent Registry", "Registered agents."),
		},
		Permissions: []PermissionSpec{
			{
				Title:   "Service account key",
				Summary: "A key for a read-only service account. Agent Manager exchanges it for a short-lived token and calls only list APIs.",
				Items: []string{
					"Organizations created on or after 3 May 2024 block service account key creation by default; an administrator must allow it for this account.",
				},
			},
			{
				Title:   "Read-only roles",
				Summary: "Grant only the roles of what you scan.",
				Items: []string{
					"roles/aiplatform.viewer: list Agent Runtime engines (always needed).",
					"roles/agentregistry.viewer: Agent Registry agents (roles/dialogflow.viewer is not enough).",
				},
			},
			{
				Title:   "What Agent Manager lists",
				Summary: "The agents deployed on Agent Runtime and the agents in the Agent Registry; a registry entry for an engine is merged into that engine.",
				Reads: []string{
					"Engines: resource name, display name, framework, service account, source repository, labels and creation time.",
					"Registry agents: ID, name, version, framework, protocols, skill names and the engine they run on. Google's built-in agents are left out.",
				},
			},
		},
		Docs: []string{"https://cloud.google.com/vertex-ai/generative-ai/docs/reference/rest/v1/projects.locations.reasoningEngines/list", "https://docs.cloud.google.com/agent-registry/roles-permissions"},
	},
	{
		Provider:    models.ShadowProviderFoundry,
		DisplayName: "Microsoft Foundry Agent Service",
		Description: "The agents in every Foundry project (prompt, hosted, workflow, external), and optionally classic agents.",
		Endpoints:   azureEndpoints,
		Fields: append(append([]FieldSpec{}, azureFields...),
			FieldSpec{Key: OptClassicAgents, Label: "Also list classic agents (retires 31 March 2027)", Kind: FieldCheckbox, DefaultBool: true},
		),
		Permissions: []PermissionSpec{
			{
				Title:   "Entra app with a least-privilege custom role",
				Summary: "An app registration with a client secret. Agent Manager signs in with client credentials and reads agent metadata only.",
				Items: []string{
					"Plain Reader cannot read agents. Foundry User is not read-only (it can create, delete and invoke, and list keys).",
					"The agents read permission also allows reading threads and messages; Agent Manager calls only the agent listing endpoints.",
				},
				Policy: `{"Name":"Agent Manager Foundry Discovery Reader","IsCustom":true,"Actions":["Microsoft.Resources/subscriptions/read","Microsoft.CognitiveServices/accounts/read","Microsoft.CognitiveServices/accounts/projects/read"],"NotActions":[],"DataActions":["Microsoft.CognitiveServices/accounts/AIServices/agents/read"],"NotDataActions":[],"AssignableScopes":["/subscriptions/{subscriptionId}"]}`,
			},
			{
				Title:   "What Agent Manager lists",
				Summary: "The Foundry projects of each AI Services account and their agents; optionally classic agents.",
				Reads: []string{
					"Agents: ID, name, state, kind (prompt, hosted, workflow, external), version, model, the type and name of each tool, and MCP server labels and URL origins.",
					"Classic agents: ID, name, model and tools.",
				},
			},
		},
		Docs: []string{"https://learn.microsoft.com/en-us/azure/ai-foundry/agents/overview"},
	},
}

// readCIAndEnvField is the source hosts' option for CI pipeline definitions
// and .env example files (the design leaves them configurable).
func readCIAndEnvField(help string) FieldSpec {
	return FieldSpec{Key: OptReadCIAndEnv, Label: "Also read CI pipeline definitions and .env example files", Kind: FieldCheckbox, DefaultBool: true, Help: help}
}

// CallSourceAnalysis is the call a source host's scan records as skipped
// until the source code analysis exists, with ReasonAnalysisUnavailable.
const (
	CallSourceAnalysis        = "Source code analysis"
	ReasonAnalysisUnavailable = "not available yet: the repositories are analysed once source code analysis is added"
)

// Plural is "1 repository" or "3 repositories": n with the singular or
// plural noun.
func Plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// sourceHostSpecs are the source repository hosts (Method 2). Their
// connections are stored now; the source code analysis that scans them is
// added separately, so they have no connector yet.
var sourceHostSpecs = []ProviderSpec{
	{
		Provider:    models.ShadowProviderGitHub,
		DisplayName: "GitHub",
		SourceHost:  true,
		Description: "Repositories of a GitHub organization or user account: agent framework and model SDK dependencies, MCP and A2A configuration, and agent code.",
		Endpoints: []EndpointSpec{{
			Key: EPGitHubAPI, Label: "GitHub API", Default: "https://api.github.com",
			OfficialHostSuffixes: []string{"api.github.com"},
		}},
		Fields: []FieldSpec{
			{
				Key: "organization", Label: "Organization or user account", Kind: FieldText, Required: true, Placeholder: "e.g. my-org",
				Help: "The resource owner of the token. A fine-grained token reaches the repositories of one owner only.",
			},
			{Key: FieldPersonalAccessToken, Label: "Fine-grained personal access token", Kind: FieldSecret, Required: true, Placeholder: "github_pat_..."},
			{Key: OptAllRepositories, Label: "Scan every repository the token can read", Kind: FieldCheckbox, DefaultBool: true},
			{
				Key: "repositories", Label: "Repositories", Kind: FieldList, Required: true, Option: "!" + OptAllRepositories,
				Placeholder: "e.g. payments-service", Help: "Repository names in the organization, one per line.",
			},
			readCIAndEnvField("GitHub Actions workflows (.github/workflows) and files such as .env.example."),
		},
		Permissions: []PermissionSpec{
			{
				Title:   "Fine-grained personal access token (read-only)",
				Summary: "Read-only access to the repositories of one organization or user account. Today the connection test uses it; source code analysis is not available yet.",
				Reads: []string{
					"During the connection test: the owner, the repository list (name, private, fork, archived, size) and the top-level file list of one repository.",
				},
				Items: []string{
					"In GitHub, open Settings > Developer settings > Personal access tokens > Fine-grained tokens and generate a new token.",
					"Resource owner: the organization or user account to scan.",
					"Repository access: All repositories, or Only select repositories.",
					"Repository permissions: Contents: Read-only. GitHub adds Metadata: Read-only, which is mandatory.",
					"If the organization requires approval of fine-grained tokens, the token can read only public resources until an organization owner approves it.",
					"Do not use a classic token: its repo scope also grants write access.",
				},
			},
			{
				Title:   "GitHub Enterprise Server",
				Summary: "Point the connection at your server instead of github.com.",
				Items:   []string{"Set the GitHub API endpoint to https://HOSTNAME/api/v3."},
			},
		},
		Docs: []string{"https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens"},
	},
	{
		Provider:    models.ShadowProviderAzureDevOps,
		DisplayName: "Azure DevOps",
		SourceHost:  true,
		Description: "Azure Repos repositories of an Azure DevOps organization: agent framework and model SDK dependencies, MCP and A2A configuration, and agent code.",
		Endpoints: []EndpointSpec{
			{
				Key: EPAzureDevOps, Label: "Azure DevOps", Default: "https://dev.azure.com",
				OfficialHostSuffixes: []string{"dev.azure.com"},
			},
			{
				Key: EPAzureLogin, Label: "Microsoft Entra login", Default: "https://login.microsoftonline.com",
				OfficialHostSuffixes: []string{"login.microsoftonline.com"}, Option: OptUseEntra,
			},
		},
		Fields: []FieldSpec{
			{
				Key: "organization", Label: "Organization", Kind: FieldText, Required: true, Placeholder: "e.g. contoso",
				Help: "The organization name in https://dev.azure.com/{organization}.",
			},
			{Key: OptUseEntra, Label: "Sign in with a Microsoft Entra service principal instead of a personal access token", Kind: FieldCheckbox, DefaultBool: false},
			{Key: FieldPersonalAccessToken, Label: "Personal access token", Kind: FieldSecret, Required: true, Option: "!" + OptUseEntra},
			{Key: "tenantId", Label: "Directory (tenant) ID", Kind: FieldText, Required: true, Option: OptUseEntra, Placeholder: "e.g. 72f988bf-86f1-41af-91ab-2d7cd011db47"},
			{Key: "clientId", Label: "Application (client) ID", Kind: FieldText, Required: true, Option: OptUseEntra, Placeholder: "e.g. 0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b"},
			{Key: "clientSecret", Label: "Client secret", Kind: FieldSecret, Required: true, Option: OptUseEntra, Placeholder: "Client secret value"},
			{Key: OptAllProjects, Label: "Scan every project the credential can read", Kind: FieldCheckbox, DefaultBool: true},
			{
				Key: "projects", Label: "Projects", Kind: FieldList, Required: true, Option: "!" + OptAllProjects,
				Placeholder: "e.g. Payments", Help: "Project names in the organization, one per line.",
			},
			readCIAndEnvField("Azure Pipelines YAML definitions and files such as .env.example."),
		},
		Permissions: []PermissionSpec{
			{
				Title:   "Personal access token (read-only)",
				Summary: "Read-only access to one organization's projects and Azure Repos. Today the connection test uses it; source code analysis is not available yet.",
				Reads: []string{
					"During the connection test: the projects, their repositories and the top-level items of one repository.",
				},
				Items: []string{
					"In Azure DevOps, open User settings > Personal access tokens and create a token for this organization only. Global tokens (all accessible organizations) are being retired.",
					"Scopes: Code (read) and Project and team (read).",
					"Organization policies can restrict creating tokens and limit their lifetime.",
				},
			},
			{
				Title:   "Or a Microsoft Entra service principal",
				Summary: "Sign in as an app instead of a person, for organizations connected to Microsoft Entra ID.",
				Items: []string{
					"Register an app in Microsoft Entra ID and create a client secret.",
					"Add the service principal to the organization (Organization settings > Users > Add users), using the Object ID shown under Enterprise applications, not App registrations.",
					"Give it the Basic access level: Stakeholder does not include Azure Repos.",
					"Add it to the Readers group of each project to scan.",
				},
			},
		},
		Docs: []string{
			"https://learn.microsoft.com/en-us/azure/devops/organizations/accounts/use-personal-access-tokens-to-authenticate",
			"https://learn.microsoft.com/en-us/azure/devops/integrate/get-started/authentication/service-principal-managed-identity",
		},
	},
}

// Providers returns the provider catalog.
func Providers() []ProviderSpec {
	return allProviderSpecs
}

var allProviderSpecs = append(append([]ProviderSpec{}, providerSpecs...), sourceHostSpecs...)

// Field returns a provider's field spec.
func (p ProviderSpec) Field(key string) (FieldSpec, bool) {
	for _, f := range p.Fields {
		if f.Key == key {
			return f, true
		}
	}
	return FieldSpec{}, false
}

// Provider returns the spec of one provider.
func Provider(provider string) (ProviderSpec, bool) {
	for _, p := range allProviderSpecs {
		if p.Provider == provider {
			return p, true
		}
	}
	return ProviderSpec{}, false
}

// DefaultEndpoints returns the official endpoints of a provider.
func DefaultEndpoints(provider string) map[string]string {
	p, ok := Provider(provider)
	if !ok {
		return map[string]string{}
	}
	out := make(map[string]string, len(p.Endpoints))
	for _, e := range p.Endpoints {
		out[e.Key] = e.Default
	}
	return out
}
