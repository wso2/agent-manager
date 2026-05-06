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
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// input structs
type listDeploymentsInput struct {
	OrgName     string `json:"org_name"`
	ProjectName string `json:"project_name"`
	AgentName   string `json:"agent_name"`
}
type deployEnvVarInput struct {
	Key         string  `json:"key"`
	Value       *string `json:"value,omitempty"`
	IsSensitive *bool   `json:"is_sensitive,omitempty"`
	SecretRef   *string `json:"secret_ref,omitempty"`
}
type deployAgentInput struct {
	OrgName                   string              `json:"org_name"`
	ProjectName               string              `json:"project_name"`
	AgentName                 string              `json:"agent_name"`
	ImageID                   string              `json:"image_id"`
	Env                       []deployEnvVarInput `json:"env,omitempty"`
	EnableAutoInstrumentation *bool               `json:"enable_auto_instrumentation,omitempty"`
}
type updateDeploymentStateInput struct {
	OrgName     string `json:"org_name"`
	ProjectName string `json:"project_name"`
	AgentName   string `json:"agent_name"`
	Environment string `json:"environment"`
	State       string `json:"state"`
}

// output structs
type listDeploymentsOutput struct {
	OrgName     string                                    `json:"org_name"`
	ProjectName string                                    `json:"project_name"`
	AgentName   string                                    `json:"agent_name"`
	Deployments map[string]spec.DeploymentDetailsResponse `json:"deployments"`
}
type deployAgentOutput struct {
	OrgName     string `json:"org_name"`
	ProjectName string `json:"project_name"`
	AgentName   string `json:"agent_name"`
	Environment string `json:"environment"`
	Note        string `json:"note,omitempty"`
}
type updateDeploymentStateOutput struct {
	Message     string `json:"message"`
	OrgName     string `json:"org_name"`
	ProjectName string `json:"project_name"`
	AgentName   string `json:"agent_name"`
	Environment string `json:"environment"`
	State       string `json:"state"`
}

func (t *Toolsets) registerDeploymentTools(server *gomcp.Server) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name: "list_deployments",
		Description: "List an agent's deployments across environments. " +
			"A deployment is a released agent image running in a specific environment, and each deployment includes its current state such as active, in-progress, failed, not-deployed, or suspended.",
		InputSchema: createSchema(map[string]any{
			"org_name":     stringProperty("Optional. Organization name."),
			"project_name": stringProperty("Required. Project name where the agent exists."),
			"agent_name":   stringProperty("Required. Name of the agent to check deployments for."),
		}, []string{"project_name", "agent_name"}),
	}, withToolLogging("list_deployments", listDeployments(t.DeploymentToolset)))

	gomcp.AddTool(server, &gomcp.Tool{
		Name: "deploy_agent",
		Description: "Deploy an existing agent image. " +
			"A deployment releases a built agent image to the lowest environment in the deployment pipeline. " +
			"This tool accepts a specific image together with optional runtime environment variables and observability settings.",
		InputSchema: createSchema(map[string]any{
			"org_name":                    stringProperty("Optional. Organization name."),
			"project_name":                stringProperty("Required. Project name where the agent exists."),
			"agent_name":                  stringProperty("Required. Name of the agent to be deployed."),
			"image_id":                    stringProperty("Required. Image identifier produced by a build."),
			"enable_auto_instrumentation": boolProperty("Optional. Enable automatic observability instrumentation for the deployed agent."),
			"env": arrayProperty("Optional. Environment variables for deployment.", createSchema(map[string]any{
				"key":          stringProperty("Required. Environment variable key."),
				"value":        stringProperty("Optional. Environment variable value."),
				"is_sensitive": boolProperty("Optional. If true, value is stored as a secret."),
				"secret_ref":   stringProperty("Optional. Reference to existing secret."),
			}, []string{"key"})),
		}, []string{"project_name", "agent_name", "image_id"}),
	}, withToolLogging("deploy_agent", deployAgent(t.DeploymentToolset)))

	gomcp.AddTool(server, &gomcp.Tool{
		Name: "update_deployment_state",
		Description: "Change the state of an agent deployment in a specific environment. " +
			"`redeploy` requests a fresh rollout of the current deployment, and `undeploy` removes the deployment from that environment.",
		InputSchema: createSchema(map[string]any{
			"org_name":     stringProperty("Optional. Organization name."),
			"project_name": stringProperty("Required. Project name where the agent is been registered."),
			"agent_name":   stringProperty("Required. Name of the specific agent."),
			"environment":  stringProperty("Required. Environment name."),
			"state":        enumProperty("Required. Desired deployment action for the selected environment.", []string{"redeploy", "undeploy"}),
		}, []string{"project_name", "agent_name", "environment", "state"}),
	}, withToolLogging("update_deployment_state", updateDeploymentState(t.DeploymentToolset)))
}

func listDeployments(handler DeploymentToolsetHandler) func(context.Context, *gomcp.CallToolRequest, listDeploymentsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listDeploymentsInput) (*gomcp.CallToolResult, any, error) {
		if input.ProjectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if input.AgentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}

		orgName := resolveOrgName(input.OrgName)

		deployments, err := handler.GetAgentDeployments(ctx, orgName, input.ProjectName, input.AgentName)
		if err != nil {
			return nil, nil, wrapToolError("list_deployments", err)
		}

		response := listDeploymentsOutput{
			OrgName:     orgName,
			ProjectName: input.ProjectName,
			AgentName:   input.AgentName,
			Deployments: utils.ConvertToDeploymentDetailsResponse(deployments),
		}

		return handleToolResult(response, nil)
	}
}

func deployAgent(handler DeploymentToolsetHandler) func(context.Context, *gomcp.CallToolRequest, deployAgentInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input deployAgentInput) (*gomcp.CallToolResult, any, error) {
		if input.ProjectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if input.AgentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}
		if input.ImageID == "" {
			return nil, nil, fmt.Errorf("image_id is required")
		}

		orgName := resolveOrgName(input.OrgName)

		env := make([]spec.EnvironmentVariable, 0, len(input.Env))
		for _, item := range input.Env {
			env = append(env, spec.EnvironmentVariable{
				Key:         item.Key,
				Value:       item.Value,
				IsSensitive: item.IsSensitive,
				SecretRef:   item.SecretRef,
			})
		}

		req := &spec.DeployAgentRequest{
			ImageId:                   input.ImageID,
			Env:                       env,
			EnableAutoInstrumentation: input.EnableAutoInstrumentation,
		}

		environment, err := handler.DeployAgent(ctx, orgName, input.ProjectName, input.AgentName, req)
		if err != nil {
			return nil, nil, wrapToolError("deploy_agent", err)
		}

		response := deployAgentOutput{
			OrgName:     orgName,
			ProjectName: input.ProjectName,
			AgentName:   input.AgentName,
			Environment: environment,
			Note:        fmt.Sprintf("Deployment started in environment '%s'. Use list_deployments to track status.", environment),
		}

		return handleToolResult(response, nil)
	}
}

func updateDeploymentState(handler DeploymentToolsetHandler) func(context.Context, *gomcp.CallToolRequest, updateDeploymentStateInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input updateDeploymentStateInput) (*gomcp.CallToolResult, any, error) {
		if input.ProjectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if input.AgentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}
		if input.Environment == "" {
			return nil, nil, fmt.Errorf("environment is required")
		}
		state := strings.ToLower(strings.TrimSpace(input.State))
		var actionMessage string
		switch state {
		case "redeploy":
			state = utils.DeploymentStateActive
			actionMessage = "Now redeploying"
		case "undeploy":
			state = utils.DeploymentStateUndeploy
			actionMessage = "Now undeploying"
		default:
			return nil, nil, fmt.Errorf("state must be redeploy or undeploy")
		}

		orgName := resolveOrgName(input.OrgName)

		if err := handler.UpdateDeploymentState(ctx, orgName, input.ProjectName, input.AgentName, input.Environment, state); err != nil {
			return nil, nil, wrapToolError("update_deployment_state", err)
		}

		response := updateDeploymentStateOutput{
			Message:     fmt.Sprintf("Deployment state transition request accepted. %s'.", actionMessage),
			OrgName:     orgName,
			ProjectName: input.ProjectName,
			AgentName:   input.AgentName,
			Environment: input.Environment,
			State:       input.State,
		}

		return handleToolResult(response, nil)
	}
}
