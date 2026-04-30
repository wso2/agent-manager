package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"	
)

//input structs
type listProjectAgentPairsInput struct {
	OrgName       string `json:"org_name"`
	ProjectSearch string `json:"project_search"`
	AgentSearch   string `json:"agent_search"`
	ProjectLimit  *int   `json:"project_limit"`
	ProjectOffset *int   `json:"project_offset"`
	AgentLimit    *int   `json:"agent_limit"`
	AgentOffset   *int   `json:"agent_offset"`
}

type listAgentsInput struct {
	OrgName     string `json:"org_name"`
	ProjectName string `json:"project_name"`
	Limit       *int   `json:"limit,omitempty"`
	Offset      *int   `json:"offset,omitempty"`
}
type createExternalAgentInput struct {
	OrgName     string  `json:"org_name"`
	ProjectName string  `json:"project_name"`
	DisplayName string  `json:"display_name"`
	Description *string `json:"description"`
	Language    string  `json:"language"`
}

//output structs and helpers
type listAgentItem struct {
	Name         string            `json:"name"`
	Provisioning spec.Provisioning `json:"provisioning"`
}
type listAgentsOutput struct {
	OrgName     string          `json:"org_name"`
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
	Note  string             `json:"note,omitempty"`
}
type envVarInput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (t *Toolsets) registerAgentTools(server *gomcp.Server) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name: "list_agents",
		Description: "List agents in a project. " +
			"An agent is an AI application registered in Agent Manager. Provisioning indicates whether the platform hosts the agent internally or the agent runs externally.",
		InputSchema: createSchema(map[string]any{
			"org_name":     stringProperty("Optional. Organization name."),
			"project_name": stringProperty("Required. Project name to list agents from."),
			"limit":        intProperty(fmt.Sprintf("Optional. Max agents to return (default %d, min %d, max %d).", utils.DefaultLimit, utils.MinLimit, utils.MaxLimit)),
			"offset":       intProperty(fmt.Sprintf("Optional. Pagination offset (default %d, min %d).", utils.DefaultOffset, utils.MinOffset)),
		}, []string{"project_name"}),
	}, withToolLogging("list_agents", listAgents(t.AgentToolset)))

	if t.ProjectToolset != nil {
		gomcp.AddTool(server, &gomcp.Tool{
			Name: "list_project_agent_pairs",
			Description: "List project-agent name pairs within an organization, with optional project and agent name filters. " +
				"Each pair shows the project and the registered agent inside that project.",
			InputSchema: createSchema(map[string]any{
				"org_name":       stringProperty("Optional. Organization name."),
				"project_search": stringProperty("Optional. Filter project names by substring (case-insensitive)."),
				"agent_search":   stringProperty("Optional. Filter agent names by substring (case-insensitive)."),
				"project_limit":  intProperty("Optional. Project pagination limit (1-50)."),
				"project_offset": intProperty("Optional. Project pagination offset (>= 0)."),
				"agent_limit":    intProperty("Optional. Agent pagination limit (1-50)."),
				"agent_offset":   intProperty("Optional. Agent pagination offset (>= 0)."),
			}, nil),
		}, withToolLogging("list_project_agent_pairs", listProjectAgentPairs(t.AgentToolset, t.ProjectToolset)))
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name: "create_external_agent",
		Description: "Register an external agent in a project. " +
			"An external agent runs outside the platform; Agent Manager stores its identity and returns setup steps so you can instrument it and send observability data to the platform.",
		InputSchema: createSchema(map[string]any{
			"org_name":     stringProperty("Optional. Organization name."),
			"project_name": stringProperty("Required. Project name where the agent will be registered."),
			"display_name": stringProperty("Required. Human-readable display name for the agent."),
			"description":  stringProperty("Optional. Short description about what the agent does."),
			"language":     stringProperty("Required. Agent language for setup guide (python or ballerina)."),
		}, []string{"project_name", "display_name", "language"}),
	}, withToolLogging("create_external_agent", createExternalAgent(t.AgentToolset)))
}

func listAgents(handler AgentToolsetHandler) func(context.Context, *gomcp.CallToolRequest, listAgentsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listAgentsInput) (*gomcp.CallToolResult, any, error) {
		if input.ProjectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		orgName := resolveOrgName(input.OrgName)
		if orgName == "" {
			return nil, nil, fmt.Errorf("org_name is required")
		}
		limit := utils.DefaultLimit
		if input.Limit != nil {
			limit = *input.Limit
		}
		if limit < utils.MinLimit || limit > utils.MaxLimit {
			return nil, nil, fmt.Errorf("limit must be between %d and %d", utils.MinLimit, utils.MaxLimit)
		}
		offset := utils.DefaultOffset
		if input.Offset != nil {
			offset = *input.Offset
		}
		if offset < utils.MinOffset {
			return nil, nil, fmt.Errorf("offset must be >= %d", utils.MinOffset)
		}
		// Calls the service-layer interface
		agents, total, err := handler.ListAgents(ctx, orgName, input.ProjectName, int32(limit), int32(offset))
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
			OrgName:     orgName,
			Total:       total,
			ProjectName: input.ProjectName,
			Agents:      formatted,
		}
		return handleToolResult(response, nil)
	}
}

func listProjectAgentPairs(agentHandler AgentToolsetHandler, projectHandler ProjectToolsetHandler) func(context.Context, *gomcp.CallToolRequest, listProjectAgentPairsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listProjectAgentPairsInput) (*gomcp.CallToolResult, any, error) {
		if input.ProjectLimit != nil && (*input.ProjectLimit < utils.MinLimit || *input.ProjectLimit > utils.MaxLimit) {
			return nil, nil, fmt.Errorf("project_limit must be between %d and %d", utils.MinLimit, utils.MaxLimit)
		}
		if input.ProjectOffset != nil && *input.ProjectOffset < utils.MinOffset {
			return nil, nil, fmt.Errorf("project_offset must be >= %d", utils.MinOffset)
		}
		if input.AgentLimit != nil && (*input.AgentLimit < utils.MinLimit || *input.AgentLimit > utils.MaxLimit) {
			return nil, nil, fmt.Errorf("agent_limit must be between %d and %d", utils.MinLimit, utils.MaxLimit)
		}
		if input.AgentOffset != nil && *input.AgentOffset < utils.MinOffset {
			return nil, nil, fmt.Errorf("agent_offset must be >= %d", utils.MinOffset)
		}

		orgName := resolveOrgName(input.OrgName)
		if orgName == "" {
			return nil, nil, fmt.Errorf("org_name is required")
		}
		projectLimit := utils.DefaultLimit
		if input.ProjectLimit != nil {
			projectLimit = *input.ProjectLimit
		}
		projectOffset := utils.DefaultOffset
		if input.ProjectOffset != nil {
			projectOffset = *input.ProjectOffset
		}
		agentLimit := utils.DefaultLimit
		if input.AgentLimit != nil {
			agentLimit = *input.AgentLimit
		}
		agentOffset := utils.DefaultOffset
		if input.AgentOffset != nil {
			agentOffset = *input.AgentOffset
		}
		projects, _, err := projectHandler.ListProjects(ctx, orgName, projectLimit, projectOffset)
		if err != nil {
			return nil, nil, wrapToolError("list_project_agent_pairs", err)
		}
		pairs := []projectAgentPair{}
		for _, project := range projects {
			if !matchesSearch(project.Name, input.ProjectSearch) {
				continue
			}
			agents, _, err := agentHandler.ListAgents(ctx, orgName, project.Name, int32(agentLimit), int32(agentOffset))
			if err != nil {
				return nil, nil, wrapToolError("list_project_agent_pairs", err)
			}
			for _, agent := range agents {
				if !matchesSearch(agent.Name, input.AgentSearch) {
					continue
				}
				pairs = append(pairs, projectAgentPair{
					ProjectName: project.Name,
					AgentName:   agent.Name,
				})
			}
		}
		note := ""
		if len(pairs) == 0 && (input.ProjectSearch != "" || input.AgentSearch != "") {
			note = "no pairs matched the provided filters; try a broader search"
		}
		return handleToolResult(listProjectAgentPairsOutput{
			Pairs: pairs,
			Count: len(pairs),
			Note:  note,
		}, nil)
	}
}

func createExternalAgent(handler AgentToolsetHandler) func(context.Context, *gomcp.CallToolRequest, createExternalAgentInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input createExternalAgentInput) (*gomcp.CallToolResult, any, error) {
		if input.ProjectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if strings.TrimSpace(input.DisplayName) == "" {
			return nil, nil, fmt.Errorf("display_name is required")
		}
		if strings.TrimSpace(input.Language) == "" {
			return nil, nil, fmt.Errorf("language is required")
		}

		orgName := resolveOrgName(input.OrgName)
		if orgName == "" {
			return nil, nil, fmt.Errorf("org_name is required")
		}

		resourceReq := spec.ResourceNameRequest{
			DisplayName:  strings.TrimSpace(input.DisplayName),
			ResourceType: "agent",
			ProjectName:  &input.ProjectName,
		}

		// generate the name(unique identifier) for an agent using a display name
		agentName, err := handler.GenerateName(ctx, orgName, resourceReq)
		if err != nil {
			return nil, nil, wrapToolError("create_external_agent", err)
		}

		req := buildExternalAgentRequest(agentName, input.DisplayName, normalizeOptionalString(input.Description))
		if err := utils.ValidateAgentCreatePayload(req); err != nil {
			return nil, nil, err
		}

		if err := handler.CreateAgent(ctx, orgName, input.ProjectName, &req); err != nil {
			return nil, nil, wrapToolError("create_external_agent", err)
		}

		// generate a token for the agent that allows instrumentation
		expiresIn := "8760h"
		tokenResp, err := handler.GenerateToken(ctx, orgName, input.ProjectName, agentName, "", expiresIn)
		if err != nil {
			return nil, nil, wrapToolError("create_external_agent", err)
		}

		cfg := config.GetConfig()
		otelEndpoint := resolveConsoleOtelEndpoint(cfg.OTEL.ExporterEndpoint)

		// outputs the  setup instructions to enable instrumentation
		instructions := buildSetupInstructions(otelEndpoint, tokenResp.Token, expiresIn)

		language := strings.ToLower(strings.TrimSpace(input.Language))
		
		selected, ok := instructions.Guides[language]
		if !ok {
			return nil, nil, fmt.Errorf("create_external_agent: unsupported language %q (use python or ballerina)", language)
		}

		response := map[string]any{
			"org_name":         orgName,
			"project_name":     input.ProjectName,
			"agent_name":       agentName,
			"token":            tokenResp.Token,
			"token_expires_at": tokenResp.ExpiresAt,
			"token_duration":   expiresIn,
			"otel_endpoint":    instructions.OtelEndpoint,
			"language":         selected.Language,
			"steps":            selected.Steps,
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
	subType := "custom-api"
	return spec.CreateAgentRequest{
		Name:        name,
		DisplayName: displayName,
		Description: description,
		Provisioning: spec.Provisioning{
			Type: "external",
		},
		AgentType: spec.AgentType{
			Type:    "external-agent-api",
			SubType: &subType,
		},
	}
}

func resolveConsoleOtelEndpoint(defaultEndpoint string) string {
	if env := strings.TrimSpace(os.Getenv("INSTRUMENTATION_URL")); env != "" {
		return env
	}
	return "http://localhost:22893/otel"
}

// setupInstructions contains UI-aligned setup steps for external agents.
type setupInstructions struct {
	TokenDuration string
	OtelEndpoint  string
	Guides        map[string]setupGuide
}

type setupGuide struct {
	Language string
	Steps    []setupStep
}

type setupStep struct {
	StepNumber  int    `json:"step_number"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Code        string `json:"code"`
}

func buildSetupInstructions(otelEndpoint, apiKey, tokenDuration string) setupInstructions {
	pythonSteps := []setupStep{
		{
			StepNumber:  1,
			Title:       "Install AMP Instrumentation Package",
			Description: "Provides the ability to instrument your agent and export traces.",
			Code:        "pip install amp-instrumentation",
		},
		{
			StepNumber:  2,
			Title:       "Generate API Key",
			Description: "Token generated successfully. Copy it now as you won't be able to see it again.",
			Code:        fmt.Sprintf("AMP_AGENT_API_KEY=\"%s\"", apiKey),
		},
		{
			StepNumber:  3,
			Title:       "Set environment variables",
			Description: "Sets the agent endpoint and API key so traces can be exported securely.",
			Code:        fmt.Sprintf("export AMP_OTEL_ENDPOINT=\"%s\"\nexport AMP_AGENT_API_KEY=\"%s\"", otelEndpoint, apiKey),
		},
		{
			StepNumber:  4,
			Title:       "Run Agent with Instrumentation Enabled",
			Description: "Replace <run_command> with your agent's start command.",
			Code:        "amp-instrument <run_command>",
		},
	}

	ballerinaSteps := []setupStep{
		{
			StepNumber:  1,
			Title:       "Import Amp Module",
			Description: "Add the import to your Ballerina program.",
			Code:        "import ballerinax/amp as _;",
		},
		{
			StepNumber:  2,
			Title:       "Set environment variables",
			Description: "Sets the agent endpoint and API key so traces can be exported securely.",
			Code:        fmt.Sprintf("export AMP_OTEL_ENDPOINT=\"%s\"\nexport AMP_AGENT_API_KEY=\"%s\"", otelEndpoint, apiKey),
		},
		{
			StepNumber:  3,
			Title:       "Run Agent",
			Description: "Run your Ballerina agent with instrumentation enabled.",
			Code:        "bal run",
		},
	}
	return setupInstructions{
		TokenDuration: tokenDuration,
		OtelEndpoint:  otelEndpoint,
		Guides: map[string]setupGuide{
			"python":    {Language: "python", Steps: pythonSteps},
			"ballerina": {Language: "ballerina", Steps: ballerinaSteps},
		},
	}
}