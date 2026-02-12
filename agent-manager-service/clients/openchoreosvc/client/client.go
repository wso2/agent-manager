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

// Package client provides the OpenChoreo API client.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg clientmocks -out ../../clientmocks/openchoreo_client_fake.go . OpenChoreoClient:OpenChoreoClientMock
package client

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/requests"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// Config contains configuration for the OpenChoreo client
type Config struct {
	BaseURL      string
	AuthProvider AuthProvider
	RetryConfig  requests.RequestRetryConfig
}

// OpenChoreoClient defines the interface for OpenChoreo operations
type OpenChoreoClient interface {
	// Organization Operations (maps to OC namespaces)
	GetOrganization(ctx context.Context, orgName string) (*models.OrganizationResponse, error)
	ListOrganizations(ctx context.Context) ([]*models.OrganizationResponse, error)

	// Project Operations
	CreateProject(ctx context.Context, namespaceName string, req CreateProjectRequest) error
	GetProject(ctx context.Context, namespaceName, projectName string) (*models.ProjectResponse, error)
	PatchProject(ctx context.Context, namespaceName, projectName string, req PatchProjectRequest) error
	DeleteProject(ctx context.Context, namespaceName, projectName string) error
	ListProjects(ctx context.Context, namespaceName string) ([]*models.ProjectResponse, error)

	// Component Operations
	CreateComponent(ctx context.Context, namespaceName, projectName string, req CreateComponentRequest) error
	GetComponent(ctx context.Context, namespaceName, projectName, componentName string) (*models.AgentResponse, error)
	UpdateComponentBasicInfo(ctx context.Context, namespaceName, projectName, componentName string, req UpdateComponentBasicInfoRequest) error
	GetComponentResourceConfigs(ctx context.Context, namespaceName, projectName, componentName, environment string) (*ComponentResourceConfigsResponse, error)
	UpdateComponentResourceConfigs(ctx context.Context, namespaceName, projectName, componentName, environment string, req UpdateComponentResourceConfigsRequest) error
	DeleteComponent(ctx context.Context, namespaceName, projectName, componentName string) error
	ListComponents(ctx context.Context, namespaceName, projectName string) ([]*models.AgentResponse, error)
	ComponentExists(ctx context.Context, namespaceName, projectName, componentName string, verifyProject bool) (bool, error)
	AttachTrait(ctx context.Context, namespaceName, projectName, componentName string, traitType TraitType) error
	GetComponentEndpoints(ctx context.Context, namespaceName, projectName, componentName, environment string) (map[string]models.EndpointsResponse, error)
	GetComponentConfigurations(ctx context.Context, namespaceName, projectName, componentName, environment string) ([]models.EnvVars, error)

	// Build Operations
	TriggerBuild(ctx context.Context, namespaceName, projectName, componentName, commitID string) (*models.BuildResponse, error)
	GetBuild(ctx context.Context, namespaceName, projectName, componentName, buildName string) (*models.BuildDetailsResponse, error)
	ListBuilds(ctx context.Context, namespaceName, projectName, componentName string) ([]*models.BuildResponse, error)
	UpdateComponentBuildParameters(ctx context.Context, namespaceName, projectName, componentName string, req UpdateComponentBuildParametersRequest) error

	// Deployment Operations
	Deploy(ctx context.Context, namespaceName, projectName, componentName string, req DeployRequest) error
	GetDeployments(ctx context.Context, namespaceName, pipelineName, projectName, componentName string) ([]*models.DeploymentResponse, error)

	// Environment Operations
	GetEnvironment(ctx context.Context, namespaceName, environmentName string) (*models.EnvironmentResponse, error)
	ListEnvironments(ctx context.Context, namespaceName string) ([]*models.EnvironmentResponse, error)

	// Infrastructure Operations
	GetProjectDeploymentPipeline(ctx context.Context, namespaceName, projectName string) (*models.DeploymentPipelineResponse, error)
	ListDeploymentPipelines(ctx context.Context, namespaceName string) ([]*models.DeploymentPipelineResponse, error)
	ListDataPlanes(ctx context.Context, namespaceName string) ([]*models.DataPlaneResponse, error)
}

type openChoreoClient struct {
	baseURL  string
	ocClient *gen.ClientWithResponses
}

func NewOpenChoreoClient(cfg *Config) (OpenChoreoClient, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	if cfg.AuthProvider == nil {
		return nil, fmt.Errorf("auth provider is required")
	}

	// Configure retry behavior to handle 401 Unauthorized by invalidating the token
	retryConfig := cfg.RetryConfig
	if retryConfig.RetryOnStatus == nil {
		// Custom retry logic that includes 401 handling + default transient errors
		retryConfig.RetryOnStatus = func(statusCode int) bool {
			// Handle 401 by invalidating cached token and retrying
			if statusCode == http.StatusUnauthorized {
				slog.Info("Received 401 Unauthorized, invalidating cached token")
				cfg.AuthProvider.InvalidateToken()
				return true
			}

			return slices.Contains(requests.TransientHTTPErrorCodes, statusCode)
		}
	}

	// Create the retryable HTTP client with 401 handling
	httpClient := requests.NewRetryableHTTPClient(&http.Client{}, retryConfig)

	// Create auth request editor
	authEditor := func(ctx context.Context, req *http.Request) error {
		slog.Debug("Adding auth token to request")
		token, err := cfg.AuthProvider.GetToken(ctx)
		if err != nil {
			return fmt.Errorf("failed to get auth token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}

	// Create the generated OpenAPI client with retryable HTTP client and auth
	ocClient, err := gen.NewClientWithResponses(
		cfg.BaseURL,
		gen.WithHTTPClient(httpClient),
		gen.WithRequestEditorFn(authEditor),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenChoreo client: %w", err)
	}

	return &openChoreoClient{
		baseURL:  cfg.BaseURL,
		ocClient: ocClient,
	}, nil
}
