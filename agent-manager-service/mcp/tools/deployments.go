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

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// input structs
type listDeploymentsInput struct {
	ProjectName string `json:"project_name"`
	AgentName   string `json:"agent_name"`
}

// output structs
type listDeploymentsOutput struct {
	ProjectName string                                    `json:"project_name"`
	AgentName   string                                    `json:"agent_name"`
	Deployments map[string]spec.DeploymentDetailsResponse `json:"deployments"`
}

func (t *Toolsets) registerDeploymentTools(server *gomcp.Server, reg *toolRegistry) {
	addTool(reg, server, &gomcp.Tool{
		Name:        "list_deployments",
		Description: "List where an agent is reached and its gateway deployment status.",
		InputSchema: createSchema(map[string]any{
			"project_name": stringProperty("Required. Project name where the agent exists."),
			"agent_name":   stringProperty("Required. Name of the agent to check deployments for."),
		}, []string{"project_name", "agent_name"}),
	}, audit.ActionAgentRead, listDeployments(t.DeploymentToolset), rbac.AgentRead)
}

func listDeployments(handler DeploymentToolsetHandler) func(context.Context, *gomcp.CallToolRequest, listDeploymentsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listDeploymentsInput) (*gomcp.CallToolResult, any, error) {
		if input.ProjectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if input.AgentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}

		ouID := resolveOUID(ctx)

		deployments, err := handler.GetAgentDeployments(ctx, ouID, input.ProjectName, input.AgentName)
		if err != nil {
			return nil, nil, wrapToolError("list_deployments", err)
		}

		response := listDeploymentsOutput{
			ProjectName: input.ProjectName,
			AgentName:   input.AgentName,
			Deployments: utils.ConvertToDeploymentDetailsResponse(deployments),
		}

		return handleToolResult(response, nil)
	}
}
