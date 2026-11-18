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
	"fmt"
	"math"
	"strings"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
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
	Environment string  `json:"environment"`
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
	ProjectName  string       `json:"project_name"`
	AgentName    string       `json:"agent_name"`
	TokenDetails tokenDetails `json:"token_details"`
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
		}, audit.ActionAgentRead, listProjectAgentPairs(t.AgentToolset, t.ProjectToolset), rbac.AgentRead, rbac.ProjectRead)
	}
	// needs the environment toolset to resolve the token environment
	if t.EnvironmentToolset != nil {
		addTool(reg, server, &gomcp.Tool{
			Name: "create_external_agent",
			Description: "Register an external agent in a project. " +
				"Returns the agent identity and an API token for it.",
			InputSchema: createSchema(map[string]any{
				"project_name": stringProperty("Required. Project name where the agent will be registered."),
				"agent_name":   stringProperty("Required. Unique name for the agent."),
				"display_name": stringProperty("Required. Human-readable display name for the agent."),
				"description":  stringProperty("Optional. Short description about what the agent does."),
				"environment":  stringProperty("Optional. Environment the generated API token is scoped to. If omitted, the organization's only environment is used; required when more than one environment is configured. Use list_environments to discover valid names."),
			}, []string{"project_name", "agent_name", "display_name"}),
		}, audit.ActionAgentCreate, createExternalAgent(t.AgentToolset, t.EnvironmentToolset), rbac.AgentCreate, rbac.AgentTokenManage)
	}
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

		// generate a token for the agent
		expiresIn := "8760h"
		tokenResp, err := handler.GenerateToken(ctx, ouID, input.ProjectName, agentName, environment, expiresIn)
		if err != nil {
			return nil, nil, fmt.Errorf("create_external_agent: agent %q was created but token generation failed: %w", agentName, err)
		}

		response := createExternalAgentOutput{
			ProjectName: input.ProjectName,
			AgentName:   agentName,
			TokenDetails: tokenDetails{
				Token:     tokenResp.Token,
				ExpiresAt: time.Unix(tokenResp.ExpiresAt, 0).UTC().Format(time.RFC3339),
			},
		}
		return handleToolResult(response, nil)
	}
}

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
