// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	observabilitysvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/observabilitysvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

type AgentManagerService interface {
	ListAgents(ctx context.Context, orgName string, projName string, limit int32, offset int32) ([]*models.AgentResponse, int32, error)
	CreateAgent(ctx context.Context, orgName string, projectName string, req *spec.CreateAgentRequest) error
	UpdateAgentBasicInfo(ctx context.Context, orgName string, projectName string, agentName string, req *spec.UpdateAgentBasicInfoRequest) (*models.AgentResponse, error)
	UpdateAgentBuildParameters(ctx context.Context, orgName string, projectName string, agentName string, req *spec.UpdateAgentBuildParametersRequest) (*models.AgentResponse, error)
	BuildAgent(ctx context.Context, orgName string, projectName string, agentName string, commitId string) (*models.BuildResponse, error)
	DeleteAgent(ctx context.Context, orgName string, projectName string, agentName string) error
	DeployAgent(ctx context.Context, orgName string, projectName string, agentName string, req *spec.DeployAgentRequest) (string, error)
	GetAgent(ctx context.Context, orgName string, projectName string, agentName string) (*models.AgentResponse, error)
	ListAgentBuilds(ctx context.Context, orgName string, projectName string, agentName string, limit int32, offset int32) ([]*models.BuildResponse, int32, error)
	GetBuild(ctx context.Context, orgName string, projectName string, agentName string, buildName string) (*models.BuildDetailsResponse, error)
	GetAgentDeployments(ctx context.Context, orgName string, projectName string, agentName string) ([]*models.DeploymentResponse, error)
	GetAgentEndpoints(ctx context.Context, orgName string, projectName string, agentName string, environmentName string) (map[string]models.EndpointsResponse, error)
	GetAgentConfigurations(ctx context.Context, orgName string, projectName string, agentName string, environment string) ([]models.EnvVars, error)
	GetBuildLogs(ctx context.Context, orgName string, projectName string, agentName string, buildName string) (*models.LogsResponse, error)
	GenerateName(ctx context.Context, orgName string, payload spec.ResourceNameRequest) (string, error)
	GetAgentMetrics(ctx context.Context, orgName string, projectName string, agentName string, payload spec.MetricsFilterRequest) (*spec.MetricsResponse, error)
	GetAgentRuntimeLogs(ctx context.Context, orgName string, projectName string, agentName string, payload spec.LogFilterRequest) (*models.LogsResponse, error)
	GetAgentResourceConfigs(ctx context.Context, orgName string, projectName string, agentName string, environment string) (*spec.AgentResourceConfigsResponse, error)
	UpdateAgentResourceConfigs(ctx context.Context, orgName string, projectName string, agentName string, environment string, req *spec.UpdateAgentResourceConfigsRequest) (*spec.AgentResourceConfigsResponse, error)
}

type agentManagerService struct {
	ocClient               client.OpenChoreoClient
	observabilitySvcClient observabilitysvc.ObservabilitySvcClient
	gitRepositoryService   RepositoryService
	logger                 *slog.Logger
}

func NewAgentManagerService(
	OpenChoreoClient client.OpenChoreoClient,
	observabilitySvcClient observabilitysvc.ObservabilitySvcClient,
	gitRepositoryService RepositoryService,
	logger *slog.Logger,
) AgentManagerService {
	return &agentManagerService{
		ocClient:               OpenChoreoClient,
		observabilitySvcClient: observabilitySvcClient,
		gitRepositoryService:   gitRepositoryService,
		logger:                 logger,
	}
}

func (s *agentManagerService) GetAgent(ctx context.Context, orgName string, projectName string, agentName string) (*models.AgentResponse, error) {
	s.logger.Info("Getting agent", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, err
	}
	agent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to fetch agent from oc: %w", err)
	}
	s.logger.Info("Fetched agent successfully from oc", "agentName", agent.Name, "orgName", orgName, "projectName", projectName, "provisioningType", agent.Provisioning.Type)
	return agent, nil
}

func (s *agentManagerService) ListAgents(ctx context.Context, orgName string, projName string, limit int32, offset int32) ([]*models.AgentResponse, int32, error) {
	s.logger.Info("Listing agents", "orgName", orgName, "projectName", projName, "limit", limit, "offset", offset)
	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, 0, err
	}

	// Fetch all agent components
	agents, err := s.ocClient.ListComponents(ctx, orgName, projName)
	if err != nil {
		s.logger.Error("Failed to list agents from repository", "orgName", orgName, "projectName", projName, "error", err)
		return nil, 0, fmt.Errorf("failed to list agents: %w", err)
	}

	// Calculate total count
	total := int32(len(agents))

	// Apply pagination
	var paginatedAgents []*models.AgentResponse
	if offset >= total {
		// If offset is beyond available data, return empty slice
		paginatedAgents = []*models.AgentResponse{}
	} else {
		endIndex := offset + limit
		if endIndex > total {
			endIndex = total
		}
		paginatedAgents = agents[offset:endIndex]
	}
	s.logger.Info("Listed agents successfully", "orgName", orgName, "projName", projName, "totalAgents", total, "returnedAgents", len(paginatedAgents))
	return paginatedAgents, total, nil
}

func (s *agentManagerService) CreateAgent(ctx context.Context, orgName string, projectName string, req *spec.CreateAgentRequest) error {
	s.logger.Info("Creating agent", "agentName", req.Name, "orgName", orgName, "projectName", projectName, "provisioningType", req.Provisioning.Type)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return err
	}
	// Create component
	createAgentReq := toCreateAgentRequest(req)
	if err := s.ocClient.CreateComponent(ctx, orgName, projectName, createAgentReq); err != nil {
		s.logger.Error("Failed to create agent component", "agentName", req.Name, "error", err)
		return err
	}

	// For internal agents, attach OTEL instrumentation trait for Python API agents
	if req.Provisioning.Type == string(utils.InternalAgent) {
		s.logger.Debug("Patched component with params successfully", "agentName", req.Name)
		if req.AgentType.Type == string(utils.AgentTypeAPI) && req.RuntimeConfigs.Language == string(utils.LanguagePython) {
			if err := s.ocClient.AttachTrait(ctx, orgName, projectName, req.Name, client.TraitOTELInstrumentation); err != nil {
				s.logger.Error("Failed to attach OTEL instrumentation trait", "agentName", req.Name, "error", err)
				// Rollback - delete the created agent
				errDeletion := s.ocClient.DeleteComponent(ctx, orgName, projectName, req.Name)
				if errDeletion != nil {
					s.logger.Error("Failed to rollback agent creation after OTEL instrumentation trait attachment failure", "agentName", req.Name, "error", errDeletion)
				}
				return fmt.Errorf("error attaching OTEL instrumentation trait: %w", err)
			}
			s.logger.Debug("Attached OTEL instrumentation trait", "agentName", req.Name)
		}
		if err := s.triggerInitialBuild(ctx, orgName, projectName, req); err != nil {
			s.logger.Error("Failed to trigger initial build for agent", "agentName", req.Name, "error", err)
			return err
		}
		s.logger.Debug("Triggered initial build for agent", "agentName", req.Name)
	}

	s.logger.Info("Agent created successfully", "agentName", req.Name, "orgName", orgName, "projectName", projectName, "provisioningType", req.Provisioning.Type)
	return nil
}

func (s *agentManagerService) triggerInitialBuild(ctx context.Context, orgName, projectName string, req *spec.CreateAgentRequest) error {
	// Get the latest commit from the repository
	commitId := ""
	if req.Provisioning.Repository != nil {
		repoURL := req.Provisioning.Repository.Url
		branch := req.Provisioning.Repository.Branch
		owner, repo := utils.ParseGitHubURL(repoURL)
		if owner != "" && repo != "" {
			latestCommit, err := s.gitRepositoryService.GetLatestCommit(ctx, owner, repo, branch)
			if err != nil {
				s.logger.Warn("Failed to get latest commit, will use empty commit", "repoURL", repoURL, "branch", branch, "error", err)
			} else {
				commitId = latestCommit
				s.logger.Debug("Got latest commit for build", "commitId", commitId, "branch", branch)
			}
		}
	}
	// Trigger build in OpenChoreo with the latest commit
	build, err := s.ocClient.TriggerBuild(ctx, orgName, projectName, req.Name, commitId)
	if err != nil {
		return fmt.Errorf("failed to trigger initial build: agentName %s, error: %w", req.Name, err)
	}
	s.logger.Info("Agent component created and build triggered successfully", "agentName", req.Name, "orgName", orgName, "projectName", projectName, "buildName", build.Name, "commitId", commitId)
	return nil
}

func toCreateAgentRequest(req *spec.CreateAgentRequest) client.CreateComponentRequest {
	result := client.CreateComponentRequest{
		Name:             req.Name,
		DisplayName:      req.DisplayName,
		Description:      utils.StrPointerAsStr(req.Description, ""),
		ProvisioningType: client.ProvisioningType(req.Provisioning.Type),
		AgentType: client.AgentTypeConfig{
			Type: req.AgentType.Type,
		},
	}
	if req.Provisioning.Type == string(utils.InternalAgent) {
		result.AgentType.SubType = utils.StrPointerAsStr(req.AgentType.SubType, "")
	}
	if req.Provisioning.Repository != nil {
		result.Repository = &client.RepositoryConfig{
			URL:     req.Provisioning.Repository.Url,
			Branch:  req.Provisioning.Repository.Branch,
			AppPath: req.Provisioning.Repository.AppPath,
		}
	}
	if req.RuntimeConfigs != nil {
		result.RuntimeConfigs = &client.RuntimeConfigs{
			Language:        req.RuntimeConfigs.Language,
			LanguageVersion: utils.StrPointerAsStr(req.RuntimeConfigs.LanguageVersion, ""),
			RunCommand:      utils.StrPointerAsStr(req.RuntimeConfigs.RunCommand, ""),
		}
		if len(req.RuntimeConfigs.Env) > 0 {
			result.RuntimeConfigs.Env = make([]client.EnvVar, len(req.RuntimeConfigs.Env))
			for i, env := range req.RuntimeConfigs.Env {
				result.RuntimeConfigs.Env[i] = client.EnvVar{Key: env.Key, Value: env.Value}
			}
		}
	}
	if req.InputInterface != nil {
		result.InputInterface = &client.InputInterfaceConfig{
			Type: req.InputInterface.Type,
		}
		if req.InputInterface.Port != nil {
			result.InputInterface.Port = *req.InputInterface.Port
		}
		if req.InputInterface.Schema != nil {
			result.InputInterface.SchemaPath = req.InputInterface.Schema.Path
		}
		if req.InputInterface.BasePath != nil {
			result.InputInterface.BasePath = *req.InputInterface.BasePath
		}
	}
	return result
}

func (s *agentManagerService) UpdateAgentBasicInfo(ctx context.Context, orgName string, projectName string, agentName string, req *spec.UpdateAgentBasicInfoRequest) (*models.AgentResponse, error) {
	s.logger.Info("Updating agent basic info", "agentName", agentName, "orgName", orgName, "projectName", projectName)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, err
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", orgName, "error", err)
		return nil, err
	}

	// Fetch existing agent to validate it exists
	_, err = s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch existing agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
	}
	// Update agent basic info in OpenChoreo
	updateReq := client.UpdateComponentBasicInfoRequest{
		DisplayName: req.DisplayName,
		Description: req.Description,
	}
	if err := s.ocClient.UpdateComponentBasicInfo(ctx, orgName, projectName, agentName, updateReq); err != nil {
		s.logger.Error("Failed to update agent meta data in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to update agent basic info: %w", err)
	}

	// Fetch agent to return current state
	updatedAgent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
	}

	s.logger.Info("Agent basic info update called", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	return updatedAgent, nil
}

func (s *agentManagerService) UpdateAgentBuildParameters(ctx context.Context, orgName string, projectName string, agentName string, req *spec.UpdateAgentBuildParametersRequest) (*models.AgentResponse, error) {
	s.logger.Info("Updating agent build parameters", "agentName", agentName, "orgName", orgName, "projectName", projectName)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, err
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", orgName, "error", err)
		return nil, err
	}

	// Fetch existing agent to validate immutable fields
	existingAgent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch existing agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
	}

	// Check immutable fields - agentType cannot be changed if provided
	if req.AgentType.Type != existingAgent.Type.Type {
		s.logger.Error("Cannot change agent type", "existingType", existingAgent.Type.Type, "requestedType", req.AgentType.Type)
		return nil, fmt.Errorf("%w: agent type cannot be changed", utils.ErrImmutableFieldChange)
	}

	// Check immutable fields - provisioning type cannot be changed if provided
	if req.Provisioning.Type != existingAgent.Provisioning.Type {
		s.logger.Error("Cannot change provisioning type", "existingType", existingAgent.Provisioning.Type, "requestedType", req.Provisioning.Type)
		return nil, fmt.Errorf("%w: provisioning type cannot be changed", utils.ErrImmutableFieldChange)
	}

	// Update agent build parameters in OpenChoreo
	updateReq := buildUpdateBuildParametersRequest(req)
	if err := s.ocClient.UpdateComponentBuildParameters(ctx, orgName, projectName, agentName, updateReq); err != nil {
		s.logger.Error("Failed to update agent build parameters in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to update agent build parameters: %w", err)
	}

	// Fetch agent to return current state
	updatedAgent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
	}

	s.logger.Info("Agent build parameters updated successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	return updatedAgent, nil
}

func (s *agentManagerService) GetAgentResourceConfigs(ctx context.Context, orgName string, projectName string, agentName string, environment string) (*spec.AgentResourceConfigsResponse, error) {
	s.logger.Info("Getting agent resource configurations", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, err
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", orgName, "error", err)
		return nil, err
	}

	// Validate agent exists
	_, err = s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
	}

	// Validate environment if provided
	if environment != "" {
		_, err = s.ocClient.GetEnvironment(ctx, orgName, environment)
		if err != nil {
			s.logger.Error("Failed to validate environment", "environment", environment, "orgName", orgName, "error", err)
			return nil, fmt.Errorf("failed to get environments for organization %s: %w", orgName, err)
		}
	}

	// Fetch resource configurations from OpenChoreo
	configs, err := s.ocClient.GetComponentResourceConfigs(ctx, orgName, projectName, agentName, environment)
	if err != nil {
		s.logger.Error("Failed to fetch agent resource configurations", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment, "error", err)
		return nil, fmt.Errorf("failed to get agent resource configurations: %w", err)
	}

	// Convert client response to spec response
	response := buildResourceConfigsResponse(configs)

	s.logger.Info("Fetched agent resource configurations successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment)
	return response, nil
}

func (s *agentManagerService) UpdateAgentResourceConfigs(ctx context.Context, orgName string, projectName string, agentName string, environment string, req *spec.UpdateAgentResourceConfigsRequest) (*spec.AgentResourceConfigsResponse, error) {
	s.logger.Info("Updating agent resource configurations", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, err
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", orgName, "error", err)
		return nil, err
	}

	// Fetch existing agent to validate it exists
	_, err = s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch existing agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
	}

	// Validate environment if provided (for environment-specific updates)
	if environment != "" {
		_, err = s.ocClient.GetEnvironment(ctx, orgName, environment)
		if err != nil {
			s.logger.Error("Failed to validate environment", "environment", environment, "orgName", orgName, "error", err)
			return nil, fmt.Errorf("failed to get environments for organization %s: %w", orgName, err)
		}
	}

	// Update agent resource configurations in OpenChoreo
	updateReq := buildUpdateResourceConfigsRequest(req)
	if err := s.ocClient.UpdateComponentResourceConfigs(ctx, orgName, projectName, agentName, environment, updateReq); err != nil {
		s.logger.Error("Failed to update agent resource configurations in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment, "error", err)
		return nil, fmt.Errorf("failed to update agent resource configurations: %w", err)
	}

	// Fetch updated resource configurations to return
	updatedConfigs, err := s.GetAgentResourceConfigs(ctx, orgName, projectName, agentName, environment)
	if err != nil {
		s.logger.Error("Failed to fetch updated resource configurations", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment, "error", err)
		return nil, fmt.Errorf("failed to get agent resource configurations: %w", err)
	}

	s.logger.Info("Agent resource configurations updated successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment)
	return updatedConfigs, nil
}

// buildUpdateResourceConfigsRequest converts spec request to client request
func buildUpdateResourceConfigsRequest(req *spec.UpdateAgentResourceConfigsRequest) client.UpdateComponentResourceConfigsRequest {
	updateReq := client.UpdateComponentResourceConfigsRequest{}

	if req.Replicas != nil {
		updateReq.Replicas = req.Replicas
	}

	if req.Resources != nil {
		updateReq.Resources = &client.ResourceConfig{}

		if req.Resources.Requests != nil {
			updateReq.Resources.Requests = &client.ResourceRequests{
				CPU:    utils.StrPointerAsStr(req.Resources.Requests.Cpu, ""),
				Memory: utils.StrPointerAsStr(req.Resources.Requests.Memory, ""),
			}
		}

		if req.Resources.Limits != nil {
			updateReq.Resources.Limits = &client.ResourceLimits{
				CPU:    utils.StrPointerAsStr(req.Resources.Limits.Cpu, ""),
				Memory: utils.StrPointerAsStr(req.Resources.Limits.Memory, ""),
			}
		}
	}

	return updateReq
}

// buildResourceConfigsResponse converts client response to spec response
func buildResourceConfigsResponse(clientResp *client.ComponentResourceConfigsResponse) *spec.AgentResourceConfigsResponse {
	response := &spec.AgentResourceConfigsResponse{}

	if clientResp.Replicas != nil {
		response.Replicas = clientResp.Replicas
	}

	if clientResp.Resources != nil {
		response.Resources = convertClientResourceConfigToSpec(clientResp.Resources)
	}

	if clientResp.DefaultReplicas != nil {
		response.DefaultReplicas = clientResp.DefaultReplicas
	}

	if clientResp.DefaultResources != nil {
		response.DefaultResources = convertClientResourceConfigToSpec(clientResp.DefaultResources)
	}

	if clientResp.IsDefaultsOverridden != nil {
		response.IsDefaultsOverridden = clientResp.IsDefaultsOverridden
	}

	return response
}

// convertClientResourceConfigToSpec converts client ResourceConfig to spec ResourceConfig
func convertClientResourceConfigToSpec(clientConfig *client.ResourceConfig) *spec.ResourceConfig {
	if clientConfig == nil {
		return nil
	}

	specConfig := &spec.ResourceConfig{}

	if clientConfig.Requests != nil {
		requests := &spec.ResourceRequests{}
		if clientConfig.Requests.CPU != "" {
			cpu := clientConfig.Requests.CPU
			requests.Cpu = &cpu
		}
		if clientConfig.Requests.Memory != "" {
			memory := clientConfig.Requests.Memory
			requests.Memory = &memory
		}
		specConfig.Requests = requests
	}

	if clientConfig.Limits != nil {
		limits := &spec.ResourceLimits{}
		if clientConfig.Limits.CPU != "" {
			cpu := clientConfig.Limits.CPU
			limits.Cpu = &cpu
		}
		if clientConfig.Limits.Memory != "" {
			memory := clientConfig.Limits.Memory
			limits.Memory = &memory
		}
		specConfig.Limits = limits
	}

	return specConfig
}

// buildUpdateBuildParametersRequest converts spec request to client request
func buildUpdateBuildParametersRequest(req *spec.UpdateAgentBuildParametersRequest) client.UpdateComponentBuildParametersRequest {
	updateReq := client.UpdateComponentBuildParametersRequest{}

	// Map Repository
	if req.Provisioning.Repository != nil {
		updateReq.Repository = &client.RepositoryConfig{
			URL:     req.Provisioning.Repository.Url,
			Branch:  req.Provisioning.Repository.Branch,
			AppPath: req.Provisioning.Repository.AppPath,
		}
	}

	// Map RuntimeConfigs
	updateReq.RuntimeConfigs = &client.RuntimeConfigs{
		Language:        req.RuntimeConfigs.Language,
		LanguageVersion: utils.StrPointerAsStr(req.RuntimeConfigs.LanguageVersion, ""),
		RunCommand:      utils.StrPointerAsStr(req.RuntimeConfigs.RunCommand, ""),
	}

	// Map InputInterface
	port := int32(0)
	if req.InputInterface.Port != nil {
		port = *req.InputInterface.Port
	}

	updateReq.InputInterface = &client.InputInterfaceConfig{
		Type: req.InputInterface.Type,
		Port: port,
	}

	if req.InputInterface.BasePath != nil {
		updateReq.InputInterface.BasePath = *req.InputInterface.BasePath
	}

	if req.InputInterface.Schema != nil && req.InputInterface.Schema.Path != "" {
		updateReq.InputInterface.SchemaPath = req.InputInterface.Schema.Path
	}

	return updateReq
}

func (s *agentManagerService) GenerateName(ctx context.Context, orgName string, payload spec.ResourceNameRequest) (string, error) {
	s.logger.Info("Generating resource name", "resourceType", payload.ResourceType, "displayName", payload.DisplayName, "orgName", orgName)
	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return "", err
	}

	// Generate candidate name from display name
	candidateName := utils.GenerateCandidateName(payload.DisplayName)
	s.logger.Debug("Generated candidate name", "candidateName", candidateName, "displayName", payload.DisplayName)

	if payload.ResourceType == string(utils.ResourceTypeAgent) {
		projectName := utils.StrPointerAsStr(payload.ProjectName, "")
		// Validates the project name by checking its existence
		project, err := s.ocClient.GetProject(ctx, orgName, projectName)
		if err != nil {
			s.logger.Error("Failed to find project", "projectName", projectName, "org", orgName, "error", err)
			return "", err
		}

		// Check if candidate name is available
		exists, err := s.ocClient.ComponentExists(ctx, org.Name, project.Name, candidateName, false)
		if err != nil {
			return "", fmt.Errorf("failed to check agent existence: %w", err)
		}
		if !exists {
			return candidateName, nil
		}

		// Name is taken, generate unique name with suffix
		uniqueName, err := s.generateUniqueAgentName(ctx, org.Name, project.Name, candidateName)
		if err != nil {
			s.logger.Error("Failed to generate unique agent name", "baseName", candidateName, "orgName", org.Name, "projectName", project.Name, "error", err)
			return "", fmt.Errorf("failed to generate unique agent name: %w", err)
		}
		s.logger.Info("Generated unique agent name", "agentName", uniqueName, "orgName", orgName, "projectName", projectName)
		return uniqueName, nil
	}
	if payload.ResourceType == string(utils.ResourceTypeProject) {
		// Check if candidate name is available
		_, err = s.ocClient.GetProject(ctx, org.Name, candidateName)
		if err != nil && errors.Is(err, utils.ErrProjectNotFound) {
			// Name is available, return it
			s.logger.Info("Generated unique project name", "projectName", candidateName, "orgName", orgName)
			return candidateName, nil
		}
		if err != nil {
			s.logger.Error("Failed to check project name availability", "name", candidateName, "orgName", org.Name, "error", err)
			return "", fmt.Errorf("failed to check project name availability: %w", err)
		}
		// Name is taken, generate unique name with suffix
		uniqueName, err := s.generateUniqueProjectName(ctx, org.Name, candidateName)
		if err != nil {
			s.logger.Error("Failed to generate unique project name", "baseName", candidateName, "orgName", org.Name, "error", err)
			return "", fmt.Errorf("failed to generate unique project name: %w", err)
		}
		s.logger.Info("Generated unique project name", "projectName", uniqueName, "orgName", orgName)
		return uniqueName, nil
	}
	return "", errors.New("invalid resource type for name generation")
}

// generateUniqueProjectName creates a unique name by appending a random suffix
func (s *agentManagerService) generateUniqueProjectName(ctx context.Context, orgName string, baseName string) (string, error) {
	// Create a name availability checker function that uses the project repository
	nameChecker := func(name string) (bool, error) {
		_, err := s.ocClient.GetProject(ctx, orgName, name)
		if err != nil && errors.Is(err, utils.ErrProjectNotFound) {
			// Name is available
			return true, nil
		}
		if err != nil {
			s.logger.Error("Failed to check project name availability", "name", name, "orgName", orgName, "error", err)
			return false, fmt.Errorf("failed to check project name availability: %w", err)
		}
		// Name is taken
		return false, nil
	}

	// Use the common unique name generation logic from utils
	uniqueName, err := utils.GenerateUniqueNameWithSuffix(baseName, nameChecker)
	if err != nil {
		s.logger.Error("Failed to generate unique project name", "baseName", baseName, "orgName", orgName, "error", err)
		return "", fmt.Errorf("failed to generate unique project name: %w", err)
	}

	return uniqueName, nil
}

// generateUniqueAgentName creates a unique name by appending a random suffix
func (s *agentManagerService) generateUniqueAgentName(ctx context.Context, orgName string, projectName string, baseName string) (string, error) {
	// Create a name availability checker function that uses the agent repository
	nameChecker := func(name string) (bool, error) {
		exists, err := s.ocClient.ComponentExists(ctx, orgName, projectName, name, false)
		if err != nil {
			return false, fmt.Errorf("failed to check agent name availability: %w", err)
		}
		if !exists {
			// Name is available
			return true, nil
		}
		// Name is taken
		return false, nil
	}

	// Use the common unique name generation logic from utils
	uniqueName, err := utils.GenerateUniqueNameWithSuffix(baseName, nameChecker)
	if err != nil {
		return "", fmt.Errorf("failed to generate unique agent name: %w", err)
	}

	return uniqueName, nil
}

func (s *agentManagerService) DeleteAgent(ctx context.Context, orgName string, projectName string, agentName string) error {
	s.logger.Info("Deleting agent", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return err
	}
	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "orgName", orgName, "error", err)
		return err
	}
	// Delete agent component in OpenChoreo
	s.logger.Debug("Deleting oc agent", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	err = s.ocClient.DeleteComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		if errors.Is(err, utils.ErrAgentNotFound) {
			s.logger.Warn("Agent not found during deletion, delete is idempotent", "agentName", agentName, "orgName", orgName, "projectName", projectName)
			return nil
		}
		s.logger.Error("Failed to delete oc agent", "agentName", agentName, "error", err)
		return err
	}
	s.logger.Debug("Agent deleted from OpenChoreo successfully", "orgName", orgName, "agentName", agentName)
	return nil
}

// BuildAgent triggers a build for an agent.
func (s *agentManagerService) BuildAgent(ctx context.Context, orgName string, projectName string, agentName string, commitId string) (*models.BuildResponse, error) {
	s.logger.Info("Building agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "commitId", commitId)
	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, err
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, err
	}

	agent, err := s.ocClient.GetComponent(ctx, org.Name, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from OpenChoreo", "agentName", agentName, "error", err)
		return nil, err
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return nil, fmt.Errorf("build operation is not supported for agent type: '%s'", agent.Provisioning.Type)
	}
	// Trigger build in OpenChoreo
	s.logger.Debug("Triggering build in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "commitId", commitId)
	build, err := s.ocClient.TriggerBuild(ctx, orgName, projectName, agentName, commitId)
	if err != nil {
		s.logger.Error("Failed to trigger build in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
	}
	s.logger.Info("Build triggered successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "buildName", build.Name)
	return build, nil
}

// DeployAgent deploys an agent.
func (s *agentManagerService) DeployAgent(ctx context.Context, orgName string, projectName string, agentName string, req *spec.DeployAgentRequest) (string, error) {
	s.logger.Info("Deploying agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "imageId", req.ImageId)
	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return "", err
	}
	agent, err := s.ocClient.GetComponent(ctx, org.Name, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from OpenChoreo", "agentName", agentName, "error", err)
		return "", err
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return "", fmt.Errorf("deploy operation is not supported for agent type: '%s'", agent.Provisioning.Type)
	}

	// Convert to deploy request
	deployReq := client.DeployRequest{
		ImageID: req.ImageId,
	}
	if len(req.Env) > 0 {
		deployReq.Env = make([]client.EnvVar, len(req.Env))
		for i, env := range req.Env {
			deployReq.Env[i] = client.EnvVar{
				Key:   env.Key,
				Value: env.Value,
			}
		}
	}

	// Deploy agent component in OpenChoreo
	s.logger.Debug("Deploying agent component in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "imageId", req.ImageId)
	if err := s.ocClient.Deploy(ctx, orgName, projectName, agentName, deployReq); err != nil {
		s.logger.Error("Failed to deploy agent component in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return "", err
	}

	// Get deployment pipeline from project
	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to fetch deployment pipeline", "orgName", orgName, "projectName", projectName, "error", err)
		return "", fmt.Errorf("failed to fetch deployment pipeline: %w", err)
	}
	lowestEnv := findLowestEnvironment(pipeline.PromotionPaths)
	s.logger.Info("Agent deployed successfully to "+lowestEnv, "agentName", agentName, "orgName", org.Name, "projectName", projectName, "environment", lowestEnv)
	return lowestEnv, nil
}

func findLowestEnvironment(promotionPaths []models.PromotionPath) string {
	if len(promotionPaths) == 0 {
		return ""
	}

	// Collect all target environments
	targets := make(map[string]bool)
	for _, path := range promotionPaths {
		for _, target := range path.TargetEnvironmentRefs {
			targets[target.Name] = true
		}
	}

	// Find a source environment that is not a target
	for _, path := range promotionPaths {
		if !targets[path.SourceEnvironmentRef] {
			return path.SourceEnvironmentRef
		}
	}
	return ""
}

func (s *agentManagerService) ListAgentBuilds(ctx context.Context, orgName string, projectName string, agentName string, limit int32, offset int32) ([]*models.BuildResponse, int32, error) {
	s.logger.Info("Listing agent builds", "agentName", agentName, "orgName", orgName, "projectName", projectName, "limit", limit, "offset", offset)
	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to validate organization", "orgName", orgName, "error", err)
		return nil, 0, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}

	// Check if component already exists
	agent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch component", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, 0, err
	}

	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return nil, 0, fmt.Errorf("build operation is not supported for agent type: '%s'", agent.Provisioning.Type)
	}

	// Fetch all builds from OpenChoreo first
	allBuilds, err := s.ocClient.ListBuilds(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to list builds from OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, 0, err
	}

	// Calculate total count
	total := int32(len(allBuilds))

	// Apply pagination
	var paginatedBuilds []*models.BuildResponse
	if offset >= total {
		// If offset is beyond available data, return empty slice
		paginatedBuilds = []*models.BuildResponse{}
	} else {
		endIndex := offset + limit
		if endIndex > total {
			endIndex = total
		}
		paginatedBuilds = allBuilds[offset:endIndex]
	}

	s.logger.Info("Listed builds successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "totalBuilds", total, "returnedBuilds", len(paginatedBuilds))
	return paginatedBuilds, total, nil
}

func (s *agentManagerService) GetBuild(ctx context.Context, orgName string, projectName string, agentName string, buildName string) (*models.BuildDetailsResponse, error) {
	s.logger.Info("Getting build details", "agentName", agentName, "buildName", buildName, "orgName", orgName, "projectName", projectName)
	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, err
	}
	agent, err := s.ocClient.GetComponent(ctx, org.Name, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from OpenChoreo", "agentName", agentName, "error", err)
		return nil, err
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return nil, fmt.Errorf("build operation is not supported for agent type: '%s'", agent.Provisioning.Type)
	}
	// Fetch the build from OpenChoreo
	build, err := s.ocClient.GetBuild(ctx, orgName, projectName, agentName, buildName)
	if err != nil {
		s.logger.Error("Failed to get build from OpenChoreo", "buildName", buildName, "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
	}

	s.logger.Info("Fetched build successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "buildName", build.Name)
	return build, nil
}

func (s *agentManagerService) GetAgentDeployments(ctx context.Context, orgName string, projectName string, agentName string) ([]*models.DeploymentResponse, error) {
	s.logger.Info("Getting agent deployments", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, err
	}
	project, err := s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", orgName, "error", err)
		return nil, err
	}
	agent, err := s.ocClient.GetComponent(ctx, org.Name, project.Name, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from OpenChoreo", "agentName", agentName, "error", err)
		return nil, err
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return nil, fmt.Errorf("deployment operation is not supported for agent type: '%s'", agent.Provisioning.Type)
	}

	// Get deployment pipeline name from project
	pipelineName := project.DeploymentPipeline
	deployments, err := s.ocClient.GetDeployments(ctx, orgName, pipelineName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to get deployments from OpenChoreo", "agentName", agentName, "pipelineName", pipelineName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to get deployments for agent %s: %w", agentName, err)
	}

	s.logger.Info("Fetched deployments successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "deploymentCount", len(deployments))
	return deployments, nil
}

func (s *agentManagerService) GetAgentEndpoints(ctx context.Context, orgName string, projectName string, agentName string, environmentName string) (map[string]models.EndpointsResponse, error) {
	s.logger.Info("Getting agent endpoints", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environmentName)
	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, err
	}
	project, err := s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, err
	}
	agent, err := s.ocClient.GetComponent(ctx, org.Name, project.Name, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "projectName", projectName, "orgName", orgName, "error", err)
		return nil, err
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return nil, fmt.Errorf("endpoints are not supported for agent type: '%s'", agent.Provisioning.Type)
	}
	// Check if environment exists
	_, err = s.ocClient.GetEnvironment(ctx, orgName, environmentName)
	if err != nil {
		s.logger.Error("Failed to validate environment", "environment", environmentName, "orgName", orgName, "error", err)
		return nil, fmt.Errorf("failed to get environments for organization %s: %w", orgName, err)
	}
	s.logger.Debug("Fetching agent endpoints from OpenChoreo", "agentName", agentName, "environment", environmentName, "orgName", orgName, "projectName", projectName)
	endpoints, err := s.ocClient.GetComponentEndpoints(ctx, orgName, projectName, agentName, environmentName)
	if err != nil {
		s.logger.Error("Failed to fetch endpoints", "agentName", agentName, "environment", environmentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to get endpoints for agent %s: %w", agentName, err)
	}

	s.logger.Info("Fetched endpoints successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environmentName, "endpointCount", len(endpoints))
	return endpoints, nil
}

func (s *agentManagerService) GetAgentConfigurations(ctx context.Context, orgName string, projectName string, agentName string, environment string) ([]models.EnvVars, error) {
	s.logger.Info("Getting agent configurations", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment)
	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, err
	}
	agent, err := s.ocClient.GetComponent(ctx, org.Name, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "projectName", projectName, "orgName", orgName, "error", err)
		return nil, err
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		s.logger.Warn("Configuration operation not supported for agent type", "agentName", agentName, "provisioningType", agent.Provisioning.Type, "orgName", orgName, "projectName", projectName)
		return nil, fmt.Errorf("configuration operation is not supported for agent type: '%s'", agent.Provisioning.Type)
	}
	// Check if environment exists
	_, err = s.ocClient.GetEnvironment(ctx, orgName, environment)
	if err != nil {
		s.logger.Error("Failed to validate environment", "environment", environment, "orgName", orgName, "error", err)
		return nil, fmt.Errorf("failed to get environments for organization %s: %w", orgName, err)
	}

	s.logger.Debug("Fetching agent configurations from OpenChoreo", "agentName", agentName, "environment", environment, "orgName", orgName, "projectName", projectName)
	configurations, err := s.ocClient.GetComponentConfigurations(ctx, orgName, projectName, agentName, environment)
	if err != nil {
		s.logger.Error("Failed to fetch configurations", "agentName", agentName, "environment", environment, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to get configurations for agent %s: %w", agentName, err)
	}

	s.logger.Info("Fetched configurations successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment, "configCount", len(configurations))
	return configurations, nil
}

func (s *agentManagerService) GetBuildLogs(ctx context.Context, orgName string, projectName string, agentName string, buildName string) (*models.LogsResponse, error) {
	s.logger.Info("Getting build logs", "agentName", agentName, "buildName", buildName, "orgName", orgName, "projectName", projectName)
	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to validate organization", "orgName", orgName, "error", err)
		return nil, err
	}
	// Validates the project name by checking its existence
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to get OpenChoreo project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, err
	}

	// Check if component already exists
	_, err = s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to check component existence", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
	}

	// Check if build exists
	build, err := s.ocClient.GetBuild(ctx, orgName, projectName, agentName, buildName)
	if err != nil {
		s.logger.Error("Failed to get build", "buildName", buildName, "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
	}

	// Fetch the build logs from Observability service
	buildLogsParams := observabilitysvc.BuildLogsParams{
		NamespaceName:      orgName,
		ProjectName:        projectName,
		AgentComponentName: agentName,
		BuildName:          build.Name,
	}
	buildLogs, err := s.observabilitySvcClient.GetBuildLogs(ctx, buildLogsParams)
	if err != nil {
		s.logger.Error("Failed to fetch build logs from observability service", "buildName", build.Name, "error", err)
		return nil, fmt.Errorf("failed to fetch build logs: %w", err)
	}
	s.logger.Info("Fetched build logs successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "buildName", buildName, "logCount", len(buildLogs.Logs))
	return buildLogs, nil
}

func (s *agentManagerService) GetAgentRuntimeLogs(ctx context.Context, orgName string, projectName string, agentName string, payload spec.LogFilterRequest) (*models.LogsResponse, error) {
	s.logger.Info("Getting application logs", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to validate organization", "orgName", orgName, "error", err)
		return nil, err
	}
	// Validates the project name by checking its existence
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to get OpenChoreo project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, err
	}

	// Check if component already exists
	agent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to check component existence", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return nil, fmt.Errorf("runtime logs are not supported for agent type: '%s'", agent.Provisioning.Type)
	}
	// Fetch environment from open choreo
	environment, err := s.ocClient.GetEnvironment(ctx, orgName, payload.EnvironmentName)
	if err != nil {
		s.logger.Error("Failed to fetch environment from OpenChoreo", "environmentName", payload.EnvironmentName, "orgName", orgName, "error", err)
		return nil, err
	}

	// Fetch the run time logs from Observability service
	componentLogsParams := observabilitysvc.ComponentLogsParams{
		AgentComponentId: agent.UUID,
		EnvId:            environment.UUID,
		NamespaceName:    orgName,
		ComponentName:    agentName,
		ProjectName:      projectName,
		EnvironmentName:  payload.EnvironmentName,
	}
	applicationLogs, err := s.observabilitySvcClient.GetComponentLogs(ctx, componentLogsParams, payload)
	if err != nil {
		s.logger.Error("Failed to fetch application logs from observability service", "agent", agentName, "error", err)
		return nil, fmt.Errorf("failed to fetch application logs: %w", err)
	}
	s.logger.Info("Fetched application logs successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "logCount", len(applicationLogs.Logs))
	return applicationLogs, nil
}

func (s *agentManagerService) GetAgentMetrics(ctx context.Context, orgName string, projectName string, agentName string, payload spec.MetricsFilterRequest) (*spec.MetricsResponse, error) {
	s.logger.Info("Getting agent metrics", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to validate organization", "orgName", orgName, "error", err)
		return nil, err
	}
	// Validates the project name by checking its existence
	project, err := s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to get OpenChoreo project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, err
	}
	// Fetch environment from open choreo
	environment, err := s.ocClient.GetEnvironment(ctx, orgName, payload.EnvironmentName)
	if err != nil {
		s.logger.Error("Failed to fetch environment from OpenChoreo", "environmentName", payload.EnvironmentName, "orgName", orgName, "error", err)
		return nil, err
	}
	// Check if component already exists
	agent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to check component existence", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
	}

	// Fetch the metrics from Observability service
	componentMetricsParams := observabilitysvc.ComponentMetricsParams{
		AgentComponentId: agent.UUID,
		EnvId:            environment.UUID,
		ProjectId:        project.UUID,
		NamespaceName:    orgName,
		ProjectName:      projectName,
		ComponentName:    agentName,
		EnvironmentName:  payload.EnvironmentName,
	}
	metrics, err := s.observabilitySvcClient.GetComponentMetrics(ctx, componentMetricsParams, payload)
	if err != nil {
		s.logger.Error("Failed to fetch agent metrics from observability service", "agent", agentName, "error", err)
		return nil, fmt.Errorf("failed to fetch agent metrics: %w", err)
	}
	s.logger.Info("Fetched agent metrics successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	return utils.ConvertToMetricsResponse(metrics), nil
}
