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
	"fmt"
	"math"
	"strings"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// input structs
type listProjectAgentPairsInput struct {
	ProjectSearch string `json:"project_search"`
	AgentSearch   string `json:"agent_search"`
	ProjectLimit  *int   `json:"project_limit"`
	ProjectOffset *int   `json:"project_offset"`
	AgentLimit    *int   `json:"agent_limit"`
	AgentOffset   *int   `json:"agent_offset"`
}

type listAgentsInput struct {
	ProjectName string `json:"project_name"`
	Limit       *int   `json:"limit,omitempty"`
	Offset      *int   `json:"offset,omitempty"`
}
type createExternalAgentInput struct {
	ProjectName string  `json:"project_name"`
	AgentName   string  `json:"agent_name"`
	DisplayName string  `json:"display_name"`
	Description *string `json:"description"`
	Language    string  `json:"language"`
	Environment string  `json:"environment"`
}
type envVarInput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
type createInternalAgentPythonInput struct {
	ProjectName string  `json:"project_name"`
	AgentName   string  `json:"agent_name"`
	DisplayName string  `json:"display_name"`
	Description *string `json:"description"`

	RepositoryURL        string `json:"repository_url"`
	Branch               string `json:"branch"`
	AppPath              string `json:"app_path"`
	GithubInstallationID *int64 `json:"github_installation_id"`

	LanguageVersion string `json:"language_version"`
	RunCommand      string `json:"run_command"`

	InterfaceType string `json:"interface_type"`
	Port          *int   `json:"port"`
	BasePath      string `json:"base_path"`
	OpenAPIPath   string `json:"openapi_path"`

	EnableAutoInstrumentation *bool         `json:"enable_auto_instrumentation"`
	InstrumentationVersion    *string       `json:"instrumentation_version,omitempty"`
	Env                       []envVarInput `json:"env"`
}
type internalAgentInput struct {
	RepositoryURL string
	Branch        string
	AppPath       string

	Language        string
	LanguageVersion string
	RunCommand      string
	DockerfilePath  string

	InterfaceType string
	Port          *int
	BasePath      string
	OpenAPIPath   string

	EnableAutoInstrumentation *bool
	InstrumentationVersion    *string
	Env                       []envVarInput
}

// output structs and helpers
type listAgentItem struct {
	Name         string            `json:"name"`
	Provisioning spec.Provisioning `json:"provisioning"`
}
type listAgentsOutput struct {
	Total       int32           `json:"total"`
	ProjectName string          `json:"project_name"`
	Agents      []listAgentItem `json:"agents"`
}
type projectAgentPair struct {
	ProjectName string `json:"project_name"`
	AgentName   string `json:"agent_name"`
}
type listProjectAgentPairsOutput struct {
	Pairs []projectAgentPair `json:"pairs"`
	Count int                `json:"count"`
	// Complete is false when the page left projects or agents out, so a caller
	// never mistakes one page for the whole organization.
	Complete               bool     `json:"complete"`
	MoreProjects           bool     `json:"more_projects,omitempty"`
	ProjectsWithMoreAgents []string `json:"projects_with_more_agents,omitempty"`
	Note                   string   `json:"note,omitempty"`
}
type tokenDetails struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}
type createExternalAgentOutput struct {
	ProjectName                 string       `json:"project_name"`
	AgentName                   string       `json:"agent_name"`
	Language                    string       `json:"language"`
	TokenDetails                tokenDetails `json:"token_details"`
	InstrumentationInstructions string       `json:"instrumentation_instructions"`
	Note                        string       `json:"note"`
}

func (t *Toolsets) registerAgentTools(server *gomcp.Server, reg *toolRegistry) {
	addTool(reg, server, &gomcp.Tool{
		Name: "list_agents",
		Description: "List agents in a project. " +
			"An agent is an AI application registered in Agent Manager. Provisioning indicates whether the platform hosts the agent internally or the agent runs externally.",
		InputSchema: createSchema(map[string]any{
			"project_name": stringProperty("Required. Project name to list agents from."),
			"limit":        intProperty(fmt.Sprintf("Optional. Max agents to return (default %d, min %d, max %d).", utils.DefaultLimit, utils.MinLimit, utils.MaxLimit)),
			"offset":       intProperty(fmt.Sprintf("Optional. Pagination offset (default %d, min %d).", utils.DefaultOffset, utils.MinOffset)),
		}, []string{"project_name"}),
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
	}, audit.ActionAgentRead, listAgents(t.AgentToolset), rbac.AgentRead)

	if t.ProjectToolset != nil {
		addTool(reg, server, &gomcp.Tool{
			Name: "list_project_agent_pairs",
			Description: "List project-agent name pairs within your organization (resolved from the caller's token), with optional project and agent name filters. " +
				"Each pair shows the project and the registered agent inside that project. " +
				"Results are paged: when `complete` is false, page on with project_offset or agent_offset before treating the list as the whole organization.",
			InputSchema: createSchema(map[string]any{
				"project_search": stringProperty("Optional. Filter project names by substring (case-insensitive)."),
				"agent_search":   stringProperty("Optional. Filter agent names by substring (case-insensitive)."),
				"project_limit":  intProperty(fmt.Sprintf("Optional. Max projects to return (default %d, min %d, max %d).", utils.DefaultLimit, utils.MinLimit, utils.MaxLimit)),
				"project_offset": intProperty(fmt.Sprintf("Optional. Project pagination offset (default %d, min %d, max %d).", utils.DefaultOffset, utils.MinOffset, math.MaxInt32)),
				"agent_limit":    intProperty(fmt.Sprintf("Optional. Max agents per project to return (default %d, min %d, max %d).", utils.DefaultLimit, utils.MinLimit, utils.MaxLimit)),
				"agent_offset":   intProperty(fmt.Sprintf("Optional. Agent pagination offset within each project (default %d, min %d, max %d).", utils.DefaultOffset, utils.MinOffset, math.MaxInt32)),
			}, nil),
			Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: true},
		}, audit.ActionAgentRead, listProjectAgentPairs(t.AgentToolset, t.ProjectToolset), rbac.AgentRead, rbac.ProjectRead)
	}
	// needs the environment toolset to resolve the token environment
	if t.EnvironmentToolset != nil {
		addTool(reg, server, &gomcp.Tool{
			Name: "create_external_agent",
			Description: "Register an external agent in a project. " +
				"Returns the agent identity, the API token, and step-by-step instrumentation instructions to follow in order to start sending observability data to the platform.",
			InputSchema: createSchema(map[string]any{
				"project_name": stringProperty("Required. Project name where the agent will be registered."),
				"agent_name":   stringProperty("Required. Unique name for the agent."),
				"display_name": stringProperty("Required. Human-readable display name for the agent."),
				"description":  stringProperty("Optional. Short description about what the agent does."),
				"language":     stringProperty("Required. Agent language for setup guide (python or ballerina)."),
				"environment":  stringProperty("Optional. Environment the generated API token is scoped to. If omitted, the organization's only environment is used; required when more than one environment is configured. Use list_environments to discover valid names."),
			}, []string{"project_name", "agent_name", "display_name", "language"}),
			Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false)},
		}, audit.ActionAgentCreate, createExternalAgent(t.AgentToolset, t.EnvironmentToolset), rbac.AgentCreate, rbac.AgentTokenManage)
	}

	addTool(reg, server, &gomcp.Tool{
		Name: "create_internal_agent_python",
		Description: "Create an internal Python agent inside a project. " +
			"An internal agent is hosted by the platform: Agent Manager fetches the source code, builds it, deploys it, and runs it for you. " +
			"Creating the agent automatically starts its initial build.",

		InputSchema: createSchema(map[string]any{
			"project_name": stringProperty("Required. Project name where the agent will be created."),
			"agent_name":   stringProperty("Required. Unique name for the agent."),
			"display_name": stringProperty("Required. Human-readable display name for the agent."),
			"description":  stringProperty("Optional Short description about what the agent does."),

			"repository_url":         stringProperty("Required. GitHub root repository URL. Do not enter .git and the end of repo name(eg: https://github.com/user/repo)"),
			"branch":                 stringProperty("Required. Github repository branch name."),
			"app_path":               stringProperty("Required. Path of the project where agent code lives within the repository (use / for root. specify path if not). It must start with /"),
			"github_installation_id": intProperty("Optional for a public repository; required for a private GitHub repository. The id of the WSO2 Cloud GitHub App installation on the repository owner. The client gets this id when the user connects GitHub."),

			"language_version": stringProperty("Optional. Python version (default: 3.11)."),
			"run_command":      stringProperty("Optional. Start command to run the agent (default: python main.py)."),

			"interface_type": enumProperty("Required. API interface type of the agent. Use DEFAULT for the standard chat interface on /chat at port 8000, or CUSTOM for a user-provided OpenAPI interface.", []string{"DEFAULT", "CUSTOM"}),
			"port":           intProperty("Required when interface_type is CUSTOM. Port number where the agent will be listening."),
			"base_path":      stringProperty("Required when interface_type is CUSTOM. API base path for the custom interface."),
			"openapi_path":   stringProperty("Required when interface_type is CUSTOM. OpenAPI specification file path within the repository (must start with /)."),

			"enable_auto_instrumentation": boolProperty("Automatically enables OTEL tracing instrumentation to your agent for observability."),
			"instrumentation_version":     stringProperty("Optional. AMP instrumentation version to pin for the agent (e.g., '0.3.0'). Selects the matching pre-built init-container image and the bundled traceloop-sdk version. Omit to use the platform default; only versions supported by the deployment are accepted."),
			"env": arrayProperty("Required. Environment variables and other configurations for the agent.(eg: api keys, database URLs, support service URLs). Can be obtained from the .env file in the project repository", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key":   stringProperty("Environment variable key."),
					"value": stringProperty("Environment variable value."),
				},
				"required": []string{"key", "value"},
			}),
		}, []string{"project_name", "agent_name", "display_name", "repository_url", "branch", "app_path", "interface_type", "env"}),
		Annotations: &gomcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false)},
	}, audit.ActionAgentCreate, createInternalAgentPython(t.AgentToolset), rbac.AgentCreate)
}

func listAgents(handler AgentToolsetHandler) func(context.Context, *gomcp.CallToolRequest, listAgentsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listAgentsInput) (*gomcp.CallToolResult, any, error) {
		if input.ProjectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		ouID := resolveOUID(ctx)

		limit, offset, err := resolvePagination(input.Limit, input.Offset)
		if err != nil {
			return nil, nil, err
		}
		// Calls the service-layer interface
		agents, total, err := handler.ListAgents(ctx, ouID, input.ProjectName, int32(limit), int32(offset))
		if err != nil {
			return nil, nil, wrapToolError("list_agents", err)
		}
		formatted := make([]listAgentItem, 0, len(agents))
		for _, agent := range agents {
			if agent == nil {
				continue
			}
			formatted = append(formatted, listAgentItem{
				Name: agent.Name,
				Provisioning: spec.Provisioning{
					Type: agent.Provisioning.Type,
				},
			})
		}
		response := listAgentsOutput{
			Total:       total,
			ProjectName: input.ProjectName,
			Agents:      formatted,
		}
		return handleToolResult(response, nil)
	}
}

func listProjectAgentPairs(agentHandler AgentToolsetHandler, projectHandler ProjectToolsetHandler) func(context.Context, *gomcp.CallToolRequest, listProjectAgentPairsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listProjectAgentPairsInput) (*gomcp.CallToolResult, any, error) {
		projectLimit, projectOffset, err := resolveNamedPagination("project_", input.ProjectLimit, input.ProjectOffset)
		if err != nil {
			return nil, nil, err
		}
		agentLimit, agentOffset, err := resolveNamedPagination("agent_", input.AgentLimit, input.AgentOffset)
		if err != nil {
			return nil, nil, err
		}

		ouID := resolveOUID(ctx)
		projects, projectTotal, err := projectHandler.ListProjects(ctx, ouID, projectLimit, projectOffset)
		if err != nil {
			return nil, nil, wrapToolError("list_project_agent_pairs", err)
		}
		moreProjects := int(projectTotal) > projectOffset+len(projects)
		var projectsWithMoreAgents []string
		pairs := []projectAgentPair{}
		for _, project := range projects {
			if project == nil {
				continue
			}
			if !matchesSearch(project.Name, input.ProjectSearch) {
				continue
			}
			agents, agentTotal, err := agentHandler.ListAgents(ctx, ouID, project.Name, int32(agentLimit), int32(agentOffset))
			if err != nil {
				return nil, nil, wrapToolError("list_project_agent_pairs", err)
			}
			if int(agentTotal) > agentOffset+len(agents) {
				projectsWithMoreAgents = append(projectsWithMoreAgents, project.Name)
			}
			for _, agent := range agents {
				if agent == nil {
					continue
				}
				if !matchesSearch(agent.Name, input.AgentSearch) {
					continue
				}
				pairs = append(pairs, projectAgentPair{
					ProjectName: project.Name,
					AgentName:   agent.Name,
				})
			}
		}
		// A page that skipped earlier projects or agents is not the whole organization either, even when nothing follows.
		skippedEarlier := projectOffset > 0 || agentOffset > 0
		laterPages := moreProjects || len(projectsWithMoreAgents) > 0
		complete := !skippedEarlier && !laterPages
		note := ""
		switch {
		case laterPages:
			note = "This page is partial. Page on with project_offset (more_projects) or agent_offset (projects_with_more_agents) to see the rest"
		case skippedEarlier:
			note = "This is a later page: project_offset or agent_offset skipped earlier projects or agents. Start from offset 0 to see the whole organization"
		case len(pairs) == 0 && (input.ProjectSearch != "" || input.AgentSearch != ""):
			note = "No pairs matched the provided filters. Try a broader search"
		}
		return handleToolResult(listProjectAgentPairsOutput{
			Pairs:                  pairs,
			Count:                  len(pairs),
			Complete:               complete,
			MoreProjects:           moreProjects,
			ProjectsWithMoreAgents: projectsWithMoreAgents,
			Note:                   note,
		}, nil)
	}
}

func createExternalAgent(handler AgentToolsetHandler, envHandler EnvironmentToolsetHandler) func(context.Context, *gomcp.CallToolRequest, createExternalAgentInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input createExternalAgentInput) (*gomcp.CallToolResult, any, error) {
		if input.ProjectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		agentName := strings.TrimSpace(input.AgentName)
		if agentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}
		if err := utils.ValidateResourceName(agentName, "agent"); err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(input.DisplayName) == "" {
			return nil, nil, fmt.Errorf("display_name is required")
		}
		if strings.TrimSpace(input.Language) == "" {
			return nil, nil, fmt.Errorf("language is required")
		}

		// Validate language of the agent before creation
		language := strings.ToLower(strings.TrimSpace(input.Language))
		if language != "python" && language != "ballerina" {
			return nil, nil, fmt.Errorf("create_external_agent: unsupported language %q (use python or ballerina)", language)
		}

		ouID := resolveOUID(ctx)

		// resolve the token environment before creating the agent so a bad
		// environment never leaves an agent behind without a token
		environment, err := resolveTokenEnvironment(ctx, "create_external_agent", envHandler, ouID, input.Environment)
		if err != nil {
			return nil, nil, err
		}

		// external agent creation
		req := buildExternalAgentRequest(agentName, input.DisplayName, normalizeOptionalString(input.Description))
		if err := utils.ValidateAgentCreatePayload(req); err != nil {
			return nil, nil, err
		}

		if err := handler.CreateAgent(ctx, ouID, input.ProjectName, &req); err != nil {
			return nil, nil, wrapToolError("create_external_agent", err)
		}

		// generate a token for the agent that allows instrumentation
		expiresIn := "8760h"
		tokenResp, err := handler.GenerateToken(ctx, ouID, input.ProjectName, agentName, environment, expiresIn)
		if err != nil {
			return nil, nil, fmt.Errorf("create_external_agent: agent %q was created but token generation failed: %w", agentName, err)
		}

		cfg := config.GetConfig()
		otelEndpoint := resolveConsoleOtelEndpoint(cfg.InstrumentationURL)

		// outputs the  setup instructions to enable instrumentation
		var instructions string
		switch language {
		case "python":
			instructions = buildPythonInstructions(otelEndpoint, tokenResp.Token)
		case "ballerina":
			instructions = buildBallerinaInstructions(otelEndpoint, tokenResp.Token)
		}
		if strings.TrimSpace(instructions) == "" {
			return nil, nil, fmt.Errorf("create_external_agent: agent %q was created but instructions builder returned empty output for language %q", agentName, language)
		}

		response := createExternalAgentOutput{
			ProjectName: input.ProjectName,
			AgentName:   agentName,
			Language:    language,
			TokenDetails: tokenDetails{
				Token:     tokenResp.Token,
				ExpiresAt: time.Unix(tokenResp.ExpiresAt, 0).UTC().Format(time.RFC3339),
			},
			InstrumentationInstructions: instructions,
			Note:                        verifyInstrumentationNote,
		}
		return handleToolResult(response, nil)
	}
}

func createInternalAgentPython(handler AgentToolsetHandler) func(context.Context, *gomcp.CallToolRequest, createInternalAgentPythonInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input createInternalAgentPythonInput) (*gomcp.CallToolResult, any, error) {
		if input.ProjectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		agentName := strings.TrimSpace(input.AgentName)
		if agentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}
		if err := utils.ValidateResourceName(agentName, "agent"); err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(input.DisplayName) == "" {
			return nil, nil, fmt.Errorf("display_name is required")
		}
		if strings.TrimSpace(input.RepositoryURL) == "" {
			return nil, nil, fmt.Errorf("repository_url is required")
		}
		if strings.TrimSpace(input.Branch) == "" {
			return nil, nil, fmt.Errorf("branch is required")
		}
		if strings.TrimSpace(input.AppPath) == "" {
			return nil, nil, fmt.Errorf("app_path is required")
		}
		if strings.TrimSpace(input.LanguageVersion) == "" {
			input.LanguageVersion = "3.11"
		}
		if strings.TrimSpace(input.RunCommand) == "" {
			input.RunCommand = "python main.py"
		}
		if strings.TrimSpace(input.InterfaceType) == "" {
			return nil, nil, fmt.Errorf("interface_type is required")
		}
		if input.Env == nil {
			return nil, nil, fmt.Errorf("env is required")
		}

		ouID := resolveOUID(ctx)

		req, err := buildInternalAgentRequest(agentName, input.DisplayName, normalizeOptionalString(input.Description), internalAgentInput{
			RepositoryURL:             input.RepositoryURL,
			Branch:                    input.Branch,
			AppPath:                   input.AppPath,
			Language:                  "python",
			LanguageVersion:           input.LanguageVersion,
			RunCommand:                input.RunCommand,
			InterfaceType:             input.InterfaceType,
			Port:                      input.Port,
			BasePath:                  input.BasePath,
			OpenAPIPath:               input.OpenAPIPath,
			EnableAutoInstrumentation: input.EnableAutoInstrumentation,
			InstrumentationVersion:    input.InstrumentationVersion,
			Env:                       input.Env,
		})
		if err != nil {
			return nil, nil, err
		}
		if input.GithubInstallationID != nil {
			githubApp, err := buildGitHubAppSource(*input.GithubInstallationID, req.Provisioning.Repository)
			if err != nil {
				return nil, nil, err
			}
			req.GithubApp = githubApp
		}

		if err := utils.ValidateAgentCreatePayload(*req); err != nil {
			return nil, nil, err
		}

		if err := handler.CreateAgent(ctx, ouID, input.ProjectName, req); err != nil {
			if errors.Is(err, services.ErrGitHubAuthRequired) {
				return nil, nil, gitHubReconnectError{cause: err}
			}
			return nil, nil, wrapToolError("create_internal_agent_python", err)
		}

		response := map[string]any{
			"project_name": input.ProjectName,
			"agent_name":   agentName,
			"display_name": input.DisplayName,
			"note":         "Agent created and initial build triggered. Check the build details and build logs to track and verify progress.",
		}
		return handleToolResult(response, nil)
	}
}

// helper functions needed

func matchesSearch(value, search string) bool {
	needle := strings.ToLower(strings.TrimSpace(search))
	if needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(value), needle)
}

func buildExternalAgentRequest(name, displayName string, description *string) spec.CreateAgentRequest {
	return spec.CreateAgentRequest{
		Name:        name,
		DisplayName: displayName,
		Description: description,
		Provisioning: spec.Provisioning{
			Type: "external",
		},
		AgentType: &spec.AgentType{
			Type: "external-agent-api",
		},
	}
}

const verifyInstrumentationNote = "Test instrumentation by querying the agent once, then fetch traces to verify instrumentation."

func buildPythonInstructions(otelEndpoint, token string) string {
	return fmt.Sprintf(`Follow these steps to enable instrumentation:

	1. Install the AMP instrumentation package:
	pip install amp-instrumentation

	2. Export the following environment variables in the agent's runtime environment:
	export AMP_OTEL_ENDPOINT=%q
	export AMP_AGENT_API_KEY=%q

	3. Run the agent with instrumentation enabled:
	amp-instrument <your_existing_start_command>`,

		otelEndpoint, token)
}

func buildBallerinaInstructions(otelEndpoint, token string) string {
	return fmt.Sprintf(`Follow these steps to enable instrumentation:

	1. Add the import to your main .bal file:
	import ballerinax/amp as _;

	2. Append to Ballerina.toml at the project root:
	[build-options]
	observabilityIncluded = true

	3. Append to Config.toml at the project root:
	[ballerina.observe]
	tracingEnabled = true
	tracingProvider = "amp"

	4. Export environment variables:
	export BAL_CONFIG_VAR_BALLERINAX_AMP_OTELENDPOINT=%q
	export BAL_CONFIG_VAR_BALLERINAX_AMP_APIKEY=%q

	5. Build with `+"`bal build`"+` and run normally — observability is included automatically.`,

		otelEndpoint, token)
}

func resolveConsoleOtelEndpoint(value string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return "http://default-default.gateway.localhost:19080/otel"
}

func buildInternalAgentRequest(name, displayName string, description *string, input internalAgentInput) (*spec.CreateAgentRequest, error) {
	repoURL := strings.TrimSpace(input.RepositoryURL)
	if repoURL == "" {
		return nil, fmt.Errorf("repository_url is required")
	}
	branch := strings.TrimSpace(input.Branch)
	if branch == "" {
		branch = "main"
	}
	appPath := strings.TrimSpace(input.AppPath)
	if appPath == "" {
		appPath = "/"
	}

	interfaceType := strings.ToUpper(strings.TrimSpace(input.InterfaceType))
	if interfaceType == "" {
		interfaceType = "DEFAULT"
	}
	if interfaceType != "DEFAULT" && interfaceType != "CUSTOM" {
		return nil, fmt.Errorf("interface_type must be DEFAULT or CUSTOM")
	}

	provisioning := spec.Provisioning{
		Type: "internal",
		Repository: &spec.RepositoryConfig{
			Url:     repoURL,
			Branch:  branch,
			AppPath: appPath,
		},
	}

	subType := "chat-api"
	if interfaceType == "CUSTOM" {
		subType = "custom-api"
	}

	agentType := &spec.AgentType{
		Type:    "agent-api",
		SubType: &subType,
	}

	build, err := buildCreateAgentBuild(input)
	if err != nil {
		return nil, err
	}

	configurations := buildConfigurations(input)

	inputInterface, err := buildInputInterface(interfaceType, input)
	if err != nil {
		return nil, err
	}

	return &spec.CreateAgentRequest{
		Name:           name,
		DisplayName:    displayName,
		Description:    description,
		Provisioning:   provisioning,
		AgentType:      agentType,
		Build:          build,
		Configurations: configurations,
		InputInterface: inputInterface,
	}, nil
}

func buildGitHubAppSource(installationID int64, repository *spec.RepositoryConfig) (*spec.GitHubAppSource, error) {
	if installationID <= 0 {
		return nil, fmt.Errorf("github_installation_id must be a positive integer")
	}
	owner, repo := utils.ParseGitHubURL(repository.Url)
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("github_installation_id needs a github.com repository_url (eg: https://github.com/user/repo)")
	}
	branch, appPath, repositoryURL := repository.Branch, repository.AppPath, repository.Url
	return &spec.GitHubAppSource{
		InstallationId: installationID,
		Owner:          owner,
		Repo:           repo,
		Branch:         &branch,
		AppPath:        &appPath,
		RepositoryUrl:  &repositoryURL,
	}, nil
}

// The console agent matches this exact text, so it carries no tool-name prefix.
type gitHubReconnectError struct{ cause error }

func (gitHubReconnectError) Error() string {
	return "GitHub authorization expired; reconnect GitHub and retry."
}

func (e gitHubReconnectError) Unwrap() error { return e.cause }

func buildCreateAgentBuild(input internalAgentInput) (*spec.Build, error) {
	switch strings.ToLower(strings.TrimSpace(input.Language)) {
	case "python":
		runCommand := strings.TrimSpace(input.RunCommand)
		if runCommand == "" {
			return nil, fmt.Errorf("run_command is required for python buildpack")
		}
		languageVersion := strings.TrimSpace(input.LanguageVersion)
		if languageVersion == "" {
			return nil, fmt.Errorf("language_version is required for python buildpack")
		}
		return &spec.Build{
			BuildpackBuild: &spec.BuildpackBuild{
				Type: "buildpack",
				Buildpack: spec.BuildpackConfig{
					Language:        "python",
					LanguageVersion: &languageVersion,
					RunCommand:      &runCommand,
				},
			},
		}, nil
	case "docker":
		dockerfilePath := strings.TrimSpace(input.DockerfilePath)
		if dockerfilePath == "" {
			return nil, fmt.Errorf("dockerfile_path is required for docker builds")
		}
		if !strings.HasPrefix(dockerfilePath, "/") {
			return nil, fmt.Errorf("dockerfile_path must start with '/' for docker builds")
		}
		return &spec.Build{
			DockerBuild: &spec.DockerBuild{
				Type: "docker",
				Docker: spec.DockerConfig{
					DockerfilePath: dockerfilePath,
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported language: %s", input.Language)
	}
}

func buildConfigurations(input internalAgentInput) *spec.Configurations {
	envVars := sanitizeEnvVars(input.Env)
	if len(envVars) == 0 && input.EnableAutoInstrumentation == nil && input.InstrumentationVersion == nil {
		return nil
	}
	config := &spec.Configurations{Env: envVars}
	if input.EnableAutoInstrumentation != nil {
		config.EnableAutoInstrumentation = input.EnableAutoInstrumentation
	}
	if input.InstrumentationVersion != nil {
		config.SetInstrumentationVersion(*input.InstrumentationVersion)
	}
	return config
}

func buildInputInterface(interfaceType string, input internalAgentInput) (*spec.InputInterface, error) {
	inputInterface := &spec.InputInterface{Type: "HTTP"}
	if interfaceType != "CUSTOM" {
		return inputInterface, nil
	}

	if input.Port == nil || *input.Port < 1 || *input.Port > 65535 {
		return nil, fmt.Errorf("port is required for CUSTOM interface and must be 1-65535")
	}
	basePath := strings.TrimSpace(input.BasePath)
	// if basePath == "" {
	// 	return nil, fmt.Errorf("base_path is required for CUSTOM interface")
	// }
	openAPIPath := strings.TrimSpace(input.OpenAPIPath)
	if openAPIPath == "" {
		return nil, fmt.Errorf("openapi_path is required for CUSTOM interface")
	}
	if !strings.HasPrefix(openAPIPath, "/") {
		return nil, fmt.Errorf("openapi_path must start with '/'")
	}

	port := int32(*input.Port)
	inputInterface.Port = &port
	inputInterface.BasePath = &basePath
	inputInterface.Schema = &spec.InputInterfaceSchema{Path: &openAPIPath}
	return inputInterface, nil
}

func sanitizeEnvVars(env []envVarInput) []spec.EnvironmentVariable {
	if len(env) == 0 {
		return nil
	}

	sanitized := make([]spec.EnvironmentVariable, 0, len(env))
	for _, item := range env {
		key := strings.TrimSpace(item.Key)
		key = strings.ReplaceAll(key, "\\n", "")
		key = strings.ReplaceAll(key, "\\r", "")
		key = strings.ReplaceAll(key, "\n", "")
		key = strings.ReplaceAll(key, "\r", "")
		value := strings.TrimSpace(item.Value)
		if key == "" || value == "" {
			continue
		}
		key = strings.Join(strings.Fields(key), "_")
		valueCopy := value
		sanitized = append(sanitized, spec.EnvironmentVariable{Key: key, Value: &valueCopy})
	}
	return sanitized
}
