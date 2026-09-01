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

package apitestutils

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// CreateMockOpenChoreoClient creates a mock OpenChoreo client with default behavior for testing
func CreateMockOpenChoreoClient() *clientmocks.OpenChoreoClientMock {
	return &clientmocks.OpenChoreoClientMock{
		// Resolve the namespace the same way the real client does — from the
		// configured default namespace — so callers (e.g. observability) get a
		// realistic value rather than an empty string.
		NamespaceForFunc: func(ouID string) string {
			return config.GetConfig().OpenChoreo.DefaultNamespace
		},
		ListOrganizationsFunc: func(_ context.Context) ([]*models.OrganizationResponse, error) {
			return []*models.OrganizationResponse{
				{Namespace: config.GetConfig().OpenChoreo.DefaultNamespace},
			}, nil
		},
		GetOrganizationFunc: func(ctx context.Context, orgName string) (*models.OrganizationResponse, error) {
			if orgName == "nonexistent-org" {
				return nil, utils.ErrOrganizationNotFound
			}
			return &models.OrganizationResponse{
				Name:        orgName,
				DisplayName: orgName,
				CreatedAt:   time.Now(),
				Status:      "ACTIVE",
			}, nil
		},
		GetProjectFunc: func(ctx context.Context, namespaceName string, projectName string) (*models.ProjectResponse, error) {
			if strings.Contains(projectName, "nonexistent-proj") {
				return nil, utils.ErrProjectNotFound
			}
			return &models.ProjectResponse{
				Name:               projectName,
				DisplayName:        projectName,
				OrgName:            namespaceName,
				DeploymentPipeline: "test-pipeline",
				CreatedAt:          time.Now(),
			}, nil
		},
		ComponentExistsFunc: func(ctx context.Context, namespaceName string, projectName string, componentName string) (bool, error) {
			return false, nil
		},
		CreateComponentFunc: func(ctx context.Context, namespaceName string, projectName string, req client.CreateComponentRequest) error {
			return nil
		},
		AttachTraitsFunc: func(ctx context.Context, namespaceName string, projectName string, componentName string, traitRequests []client.TraitRequest) error {
			return nil
		},
		UpdateComponentDeploymentConfigFunc: func(ctx context.Context, namespaceName string, projectName string, componentName string, req client.ComponentDeploymentConfigRequest) error {
			return nil
		},
		TriggerBuildFunc: func(_ context.Context, namespaceName string, projectName string, componentName string, commitID string, workflowRunName string) (*models.BuildResponse, error) {
			if workflowRunName == "" {
				workflowRunName = fmt.Sprintf("%s-build-1", componentName)
			}
			return &models.BuildResponse{
				UUID:        uuid.New().String(),
				Name:        workflowRunName,
				AgentName:   componentName,
				ProjectName: projectName,
				Status:      "BuildInitiated",
				StartedAt:   time.Now(),
				BuildParameters: models.BuildParameters{
					CommitID: commitID,
				},
			}, nil
		},
		GetProjectDeploymentPipelineFunc: func(ctx context.Context, namespaceName string, projectName string) (*models.DeploymentPipelineResponse, error) {
			return &models.DeploymentPipelineResponse{
				Name:        "test-pipeline",
				DisplayName: "test-pipeline",
				Description: "Test deployment pipeline",
				OrgName:     namespaceName,
				CreatedAt:   time.Now(),
				PromotionPaths: []models.PromotionPath{
					{
						SourceEnvironmentRef: "Development",
					},
				},
			}, nil
		},
		// Project creation, deploy and promote all provision the cell namespace
		// for the environment they touch before anything is released into it.
		EnsureProjectReleaseBindingFunc: func(ctx context.Context, namespaceName, projectName, environmentName string) error {
			return nil
		},
		GetComponentFunc: func(ctx context.Context, namespaceName, projectName, componentName string) (*models.AgentResponse, error) {
			if strings.Contains(componentName, "nonexistent-agent") {
				return nil, utils.ErrAgentNotFound
			}
			return &models.AgentResponse{
				UUID:        "component-uid-123",
				Name:        componentName,
				ProjectName: projectName,
				Provisioning: models.Provisioning{
					Type: "internal",
				},
			}, nil
		},
		GetEnvironmentFunc: func(ctx context.Context, namespaceName, environmentName string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{
				UUID: "environment-uid-123",
				Name: environmentName,
			}, nil
		},
		// Empty by default: CreateAgent's org-wide AgentID provisioning hook calls
		// ListEnvironments and only acts when it returns at least one environment,
		// so tests that don't care about AgentID provisioning stay unaffected by it.
		ListEnvironmentsFunc: func(ctx context.Context, namespaceName string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{}, nil
		},
		DeleteComponentFunc: func(ctx context.Context, namespaceName string, projectName string, componentName string) error {
			return nil
		},
		ListComponentsFunc: func(ctx context.Context, namespaceName string, projectName string) ([]*models.AgentResponse, error) {
			return []*models.AgentResponse{}, nil
		},
		DeleteProjectFunc: func(ctx context.Context, namespaceName string, projectName string) error {
			return nil
		},
		ListDeploymentPipelinesFunc: func(ctx context.Context, namespaceName string) ([]*models.DeploymentPipelineResponse, error) {
			return []*models.DeploymentPipelineResponse{
				{
					Name:        "default",
					DisplayName: "Default Pipeline",
					OrgName:     namespaceName,
				},
			}, nil
		},
		CreateProjectFunc: func(ctx context.Context, namespaceName string, req client.CreateProjectRequest) error {
			return nil
		},
		ListProjectsFunc: func(ctx context.Context, namespaceName string) ([]*models.ProjectResponse, error) {
			return []*models.ProjectResponse{}, nil
		},
		DeployFunc: func(ctx context.Context, namespaceName string, projectName string, componentName string, req client.DeployRequest) error {
			return nil
		},
		IsDeploymentInProgressFunc: func(ctx context.Context, namespaceName string, componentName string, environment string) (bool, error) {
			return false, nil
		},
		DeleteSecretReferenceFunc: func(ctx context.Context, namespace string, name string) error {
			return nil
		},
		GetSecretReferenceFunc: func(ctx context.Context, namespace string, name string) (*client.SecretReferenceInfo, error) {
			// Return a reference exposing an "api-key" data source so flows that resolve a
			// secret reference (e.g. the env-injection trait's agent API key) can succeed.
			return &client.SecretReferenceInfo{
				Name: name,
				Data: []client.SecretDataSourceInfo{
					{
						SecretKey: secretmanagersvc.SecretKeyAPIKey,
						RemoteRef: client.RemoteRefInfo{Key: name, Property: secretmanagersvc.SecretKeyAPIKey},
					},
				},
			}, nil
		},
		GetWorkloadSecretRefNamesFunc: func(ctx context.Context, namespaceName string, projectName string, componentName string) ([]string, error) {
			// Return empty list by default (no secret refs)
			return nil, nil
		},
		UpdateComponentEnvVarsFunc: func(ctx context.Context, namespaceName string, projectName string, componentName string, envVars []client.EnvVar) error {
			return nil
		},
		ReplaceComponentEnvVarsFunc: func(ctx context.Context, namespaceName string, projectName string, componentName string, envVars []client.EnvVar) error {
			return nil
		},
		ReplaceComponentFileMountsFunc: func(ctx context.Context, namespaceName string, projectName string, componentName string, files []client.FileVar) error {
			return nil
		},
		// nil means OpenChoreo can reconcile the component; deploy aborts early otherwise.
		GetComponentReconcileBlockFunc: func(ctx context.Context, namespaceName string, componentName string) (*client.ComponentReconcileBlock, error) {
			// A nil block is the "not blocked" signal this API defines.
			return nil, nil
		},
		RemoveWorkloadEnvVarsFunc: func(ctx context.Context, namespaceName string, componentName string, envVarKeys []string) error {
			return nil
		},
		ReplaceReleaseBindingEnvVarsFunc: func(ctx context.Context, namespaceName string, projectName string, componentName string, envName string, keysToRemove []string, envVarsToAdd []client.EnvVar) error {
			return nil
		},
		// Deploy writes this environment's env vars and file mounts here rather
		// than to the component-wide Workload, so every deploy path reaches it.
		ReplaceReleaseBindingWorkloadOverridesFunc: func(ctx context.Context, ouID string, componentName string, environment string, envOverrides []client.EnvVar, fileOverrides []client.FileVar) error {
			return nil
		},
		EnsureReleaseAndBindingFunc: func(ctx context.Context, ouID string, projectName string, componentName string, environment string, envOverrides []client.EnvVar, fileOverrides []client.FileVar) error {
			return nil
		},
		GetComponentConfigurationsFunc: func(ctx context.Context, namespaceName string, projectName string, componentName string, environment string) ([]models.EnvVars, error) {
			return nil, nil
		},
		GetComponentFileMountsFunc: func(ctx context.Context, namespaceName string, projectName string, componentName string, environment string) ([]models.FileMountEntry, error) {
			return nil, nil
		},
		UpdateReleaseBindingTraitConfigsFunc: func(ctx context.Context, namespaceName, componentName, environment string, traitConfigs map[string]interface{}, componentTypeConfigs map[string]interface{}) error {
			return nil
		},
		EnsureReleaseBindingRuntimeClassFunc: func(ctx context.Context, namespaceName, componentName, environment, desiredRuntimeClass string) error {
			return nil
		},
	}
}

// CreateMockSecretManagementClient creates a mock SecretManagementClient with default behavior for testing.
func CreateMockSecretManagementClient() *clientmocks.SecretManagementClientMock {
	return &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(ctx context.Context, location secretmanagersvc.SecretLocation, data map[string]string) (string, error) {
			return location.KVPath()
		},
		GetSecretFunc: func(ctx context.Context, kvPath string) (*secretmanagersvc.SecretInfo, error) {
			return nil, secretmanagersvc.ErrSecretNotFound
		},
		DeleteSecretFunc: func(ctx context.Context, location secretmanagersvc.SecretLocation, secretRefName string) error {
			return nil
		},
		PatchSecretFunc: func(ctx context.Context, location secretmanagersvc.SecretLocation, data map[string]string, keysToDelete []string) (string, error) {
			return "", nil
		},
	}
}
