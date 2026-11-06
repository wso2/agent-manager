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
	// "time"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

type Toolsets struct {
	ProjectToolset     ProjectToolsetHandler
	AgentToolset       AgentToolsetHandler
	BuildToolset       BuildToolsetHandler
	DeploymentToolset  DeploymentToolsetHandler
	EnvironmentToolset EnvironmentToolsetHandler
	LLMProviderToolset LLMProviderToolsetHandler
	MCPProxyToolset    MCPProxyToolsetHandler
	AgentConfigToolset AgentConfigToolsetHandler
}

type ProjectToolsetHandler interface {
	ListProjects(ctx context.Context, ouID string, limit int, offset int) ([]*models.ProjectResponse, int32, error)
	CreateProject(ctx context.Context, ouID string, payload spec.CreateProjectRequest) (*models.ProjectResponse, error)
}

type AgentToolsetHandler interface {
	ListAgents(ctx context.Context, ouID string, projName string, limit int32, offset int32) ([]*models.AgentResponse, int32, error)
	GetAgent(ctx context.Context, ouID string, projectName string, agentName string) (*models.AgentResponse, error)
	GenerateToken(ctx context.Context, ouID string, projectName string, agentName string, environment string, expiresIn string) (*spec.TokenResponse, error)
	CreateAgent(ctx context.Context, ouID string, projectName string, req *spec.CreateAgentRequest) error
}

type BuildToolsetHandler interface {
	ListAgentBuilds(ctx context.Context, ouID string, projectName string, agentName string, limit int32, offset int32) ([]*models.BuildResponse, int32, error)
	GetBuild(ctx context.Context, ouID string, projectName string, agentName string, buildName string) (*models.BuildDetailsResponse, error)
	BuildAgent(ctx context.Context, ouID string, projectName string, agentName string, commitId string) (*models.BuildResponse, error)
}

type DeploymentToolsetHandler interface {
	GetAgentDeployments(ctx context.Context, ouID string, projectName string, agentName string) ([]*models.DeploymentResponse, error)
	DeployAgent(ctx context.Context, ouID string, projectName string, agentName string, req *spec.DeployAgentRequest) (string, error)
	UpdateDeploymentState(ctx context.Context, ouID string, projectName string, agentName string, environment string, state string) error
}

type EnvironmentToolsetHandler interface {
	ListEnvironments(ctx context.Context, ouID string, limit int32, offset int32) (*models.EnvironmentListResponse, error)
	GetEnvironment(ctx context.Context, ouID string, envName string) (*models.GatewayEnvironmentResponse, error)
}

type LLMProviderToolsetHandler interface {
	ListLLMProviders(ctx context.Context, ouID string, limit int, offset int) ([]*models.LLMProvider, int, error)
	ListLLMGuardrails(ctx context.Context, ouID string, providerID string) (*models.LLMPolicyAvailabilityResponse, error)
	// GatewayEnvironmentIDs returns the UUIDs of the environments each gateway serves.
	GatewayEnvironmentIDs(ctx context.Context, gatewayIDs []string) (map[string][]string, error)
	GetLLMProvider(ctx context.Context, ouID string, providerID string) (*models.LLMProvider, []*models.Deployment, error)
	// UpdateLLMProviderGuardrails reads the provider once, saves change(its guardrails) and redeploys it. An
	// error from change is returned as is, and nothing is saved.
	UpdateLLMProviderGuardrails(ctx context.Context, ouID string, providerID string, change func([]models.LLMPolicy) ([]models.LLMPolicy, error)) (*models.LLMProvider, []services.DeploymentResult, error)
}

type MCPProxyToolsetHandler interface {
	ListMCPProxies(ctx context.Context, ouID string, limit int, offset int) (*models.MCPProxyListResponse, error)
	GetMCPProxy(ctx context.Context, ouID string, proxyID string) (*models.MCPProxyDTO, error)
}

type AgentConfigToolsetHandler interface {
	CreateAgentConfig(ctx context.Context, ouID string, projectName string, agentName string, req models.CreateAgentModelConfigRequest) (*models.AgentModelConfigResponse, error)
	// ProviderEnvironments lists the environments where the agent can use the LLM provider.
	ProviderEnvironments(ctx context.Context, ouID string, projectName string, agentName string, providerHandle string) ([]string, error)
}
