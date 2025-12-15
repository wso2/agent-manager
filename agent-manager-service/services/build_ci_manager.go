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
	"fmt"
	"log/slog"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	clients "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
)

type BuildCIManagerService interface {
	HandleBuildCallback(ctx context.Context, orgName string, projectName string, agentName string) (string, error)
}

type buildCIManagerService struct {
	OpenChoreoSvcClient clients.OpenChoreoSvcClient
	OrganizationRepo    repositories.OrganizationRepository
	ProjectRepo         repositories.ProjectRepository
	AgentRepo           repositories.AgentRepository
	logger              *slog.Logger
}

func NewBuildCIManager(
	openChoreoSvcClient clients.OpenChoreoSvcClient,
	logger *slog.Logger,
	orgRepo repositories.OrganizationRepository,
	projectRepo repositories.ProjectRepository,
	agentRepo repositories.AgentRepository,
) BuildCIManagerService {
	return &buildCIManagerService{
		OpenChoreoSvcClient: openChoreoSvcClient,
		OrganizationRepo:    orgRepo,
		ProjectRepo:         projectRepo,
		AgentRepo:           agentRepo,
		logger:              logger,
	}
}

func (b *buildCIManagerService) HandleBuildCallback(ctx context.Context, orgName string, projectName string, agentName string) (string, error) {
	// Get organization
	org, err := b.OrganizationRepo.GetOrganizationByOcName(ctx, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			b.logger.Error("Organization not found", "organization", orgName)
			return "", fmt.Errorf("organization not found: %s", orgName)
		}
		return "", fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}

	// Get project
	project, err := b.ProjectRepo.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			b.logger.Error("Project not found", "project", projectName, "organization", orgName)
			return "", fmt.Errorf("project not found: %s", projectName)
		}
		return "", fmt.Errorf("failed to find project %s: %w", projectName, err)
	}

	// Get agent from database
	agent, err := b.AgentRepo.GetAgentByName(ctx, org.ID, project.ID, agentName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			b.logger.Error("Agent not found", "agentName", agentName, "project", projectName, "organization", orgName)
			return "", fmt.Errorf("agent not found: %s", agentName)
		}
		return "", fmt.Errorf("failed to fetch agent: %w", err)
	}
	if agent.AgentDetails == nil || agent.AgentDetails.WorkloadSpec == nil {
		return "", fmt.Errorf("agent workload specification is missing for agent: %s", agentName)
	}
	// Build Workload CR template with placeholders
	workloadCR := buildWorkloadCRTemplate(agent.AgentDetails.WorkloadSpec, org.OpenChoreoOrgName, projectName, agentName)

	b.logger.Info("Successfully generated workload CR template",
		"agentName", agentName,
		"project", projectName,
		"organization", org.OrgName)

	return workloadCR, nil
}

// buildWorkloadCRTemplate constructs a Workload CR object with placeholders and converts to YAML string
// IMAGE_TAG - placeholder for the actual container image
// SCHEMA_CONTENT - placeholder for the OpenAPI schema content (if applicable)
func buildWorkloadCRTemplate(workloadSpec map[string]interface{}, orgName, projectName, componentName string) string {
	// Create Workload object
	workload := &v1alpha1.Workload{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "openchoreo.dev/v1alpha1",
			Kind:       "Workload",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-workload", componentName),
			Namespace: orgName,
		},
		Spec: v1alpha1.WorkloadSpec{
			Owner: v1alpha1.WorkloadOwner{
				ProjectName:   projectName,
				ComponentName: componentName,
			},
			WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
				Containers: map[string]v1alpha1.Container{
					"main": {
						Image: "IMAGE_TAG", // Placeholder for actual image
						Env:   buildEnvVars(workloadSpec),
					},
				},
				Endpoints: buildEndpoints(workloadSpec, componentName),
			},
		},
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(workload)
	if err != nil {
		// Fallback to empty string if marshaling fails
		return ""
	}

	return string(yamlBytes)
}

// buildEnvVars converts environment variables from workload spec to v1alpha1.EnvVar slice
func buildEnvVars(workloadSpec map[string]interface{}) []v1alpha1.EnvVar {
	var envVars []v1alpha1.EnvVar

	if envVarsList, ok := workloadSpec["envVars"].([]interface{}); ok {
		for _, envVarItem := range envVarsList {
			if envVar, ok := envVarItem.(map[string]interface{}); ok {
				key, _ := envVar["key"].(string)
				value, _ := envVar["value"].(string)
				envVars = append(envVars, v1alpha1.EnvVar{
					Key:   key,
					Value: value,
				})
			}
		}
	}

	return envVars
}

// buildEndpoints converts endpoints from workload spec to v1alpha1.WorkloadEndpoint map
func buildEndpoints(workloadSpec map[string]interface{}, componentName string) map[string]v1alpha1.WorkloadEndpoint {
	endpoints := make(map[string]v1alpha1.WorkloadEndpoint)

	if endpointsList, ok := workloadSpec["endpoints"].([]interface{}); ok {
		for _, endpointItem := range endpointsList {
			if endpoint, ok := endpointItem.(map[string]interface{}); ok {
				endpointName, _ := endpoint["name"].(string)
				endpointType, _ := endpoint["type"].(string)

				// Handle port - JSON unmarshaling converts numbers to float64
				var port int32
				if portFloat, ok := endpoint["port"].(float64); ok {
					port = int32(portFloat)
				}

				workloadEndpoint := v1alpha1.WorkloadEndpoint{
					Type: v1alpha1.EndpointType(endpointType),
					Port: port,
				}

				// Check if schema content or schema path is provided
				schemaContent, hasSchemaContent := endpoint["schemaContent"].(string)
				schemaPath, hasSchemaPath := endpoint["schemaPath"].(string)

				// If schema content exists or schema path exists, use placeholder
				if hasSchemaContent && schemaContent != "" {
					workloadEndpoint.Schema = &v1alpha1.Schema{
						Type:    string(v1alpha1.EndpointTypeREST),
						Content: schemaContent,
					}
				} else if hasSchemaPath && schemaPath != "" {
					workloadEndpoint.Schema = &v1alpha1.Schema{
						Type:    string(v1alpha1.EndpointTypeREST),
						Content: "SCHEMA_CONTENT", // Placeholder for actual schema
					}
				}

				endpoints[endpointName] = workloadEndpoint
			}
		}
	}

	return endpoints
}
