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

package utils

import (
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
)

func ConvertToAgentListResponse(components []*models.AgentResponse) []spec.AgentResponse {
	if len(components) == 0 {
		return []spec.AgentResponse{}
	}
	responses := make([]spec.AgentResponse, len(components))
	for i, component := range components {
		responses[i] = ConvertToAgentResponse(component)
	}
	return responses
}

func ConvertToAgentResponse(component *models.AgentResponse) spec.AgentResponse {
	if component == nil {
		return spec.AgentResponse{}
	}

	if component.Provisioning.Type == string(InternalAgent) {
		return convertToInternalAgentResponse(component)
	}
	return convertToExternalAgentResponse(component)
}

func convertToInternalAgentResponse(component *models.AgentResponse) spec.AgentResponse {
	return spec.AgentResponse{
		Uuid: component.UUID,
		Name:        component.Name,
		DisplayName: component.DisplayName,
		Description: component.Description,
		ProjectName: component.ProjectName,
		CreatedAt:   component.CreatedAt,
		Status:      &component.Status,
		Provisioning: spec.Provisioning{
			Type: component.Provisioning.Type,
			Repository: &spec.RepositoryConfig{
				Url:     component.Provisioning.Repository.Url,
				Branch:  component.Provisioning.Repository.Branch,
				AppPath: component.Provisioning.Repository.AppPath,
			},
		},
		RuntimeConfigs: &spec.RuntimeConfiguration{
			Language: component.Language,
		},
		AgentType: spec.AgentType{
			Type:    component.Type.Type,
			SubType: &component.Type.SubType,
		},
	}
}

func convertToExternalAgentResponse(component *models.AgentResponse) spec.AgentResponse {
	return spec.AgentResponse{
		Uuid:  component.UUID,
		Name:        component.Name,
		DisplayName: component.DisplayName,
		Description: component.Description,
		ProjectName: component.ProjectName,
		CreatedAt:   component.CreatedAt,
		Status:      &component.Status,
		Provisioning: spec.Provisioning{
			Type: component.Provisioning.Type,
		},
		AgentType: spec.AgentType{
			Type: component.Type.Type,
		},
	}
}

func ConvertToBuildResponse(build *models.BuildResponse) spec.BuildResponse {
	if build == nil {
		return spec.BuildResponse{}
	}
	return spec.BuildResponse{
		BuildId:     &build.UUID,
		AgentName:   build.AgentName,
		ProjectName: build.ProjectName,
		CommitId:    build.CommitID,
		Status:      &build.Status,
		StartedAt:   build.StartedAt,
		ImageId:     &build.Image,
		BuildName:   build.Name,
		Branch:      build.Branch,
		EndedAt:     build.EndedAt,
	}
}

func ConvertToBuildListResponse(builds []*models.BuildResponse) []spec.BuildResponse {
	if len(builds) == 0 {
		return []spec.BuildResponse{}
	}
	responses := make([]spec.BuildResponse, len(builds))
	for i, build := range builds {
		responses[i] = ConvertToBuildResponse(build)
	}
	return responses
}

func ConvertToBuildDetailsResponse(buildDetails *models.BuildDetailsResponse) spec.BuildDetailsResponse {
	if buildDetails == nil {
		return spec.BuildDetailsResponse{}
	}

	steps := make([]spec.BuildStep, len(buildDetails.Steps))
	for i, step := range buildDetails.Steps {
		steps[i] = spec.BuildStep{
			Type:       step.Type,
			Status:     step.Status,
			Message:    step.Message,
			StartedAt:  step.StartedAt,
			FinishedAt: step.FinishedAt,
		}
	}
	return spec.BuildDetailsResponse{
		BuildId:         &buildDetails.UUID,
		AgentName:       buildDetails.AgentName,
		ProjectName:     buildDetails.ProjectName,
		CommitId:        buildDetails.CommitID,
		Status:          &buildDetails.Status,
		StartedAt:       buildDetails.StartedAt,
		ImageId:         &buildDetails.Image,
		BuildName:       buildDetails.Name,
		Branch:          buildDetails.Branch,
		Percent:         &buildDetails.Percent,
		Steps:           steps,
		DurationSeconds: &buildDetails.DurationSeconds,
		EndedAt:         buildDetails.EndedAt,
	}
}

func ConvertToDeploymentDetailsResponse(deploymentDetails []*models.DeploymentResponse) map[string]spec.DeploymentDetailsResponse {
	result := make(map[string]spec.DeploymentDetailsResponse)

	if len(deploymentDetails) == 0 {
		return result
	}

	for _, deployment := range deploymentDetails {
		// Convert model endpoints to spec endpoints
		endpoints := make([]spec.DeploymentEndpoint, len(deployment.Endpoints))
		for i, endpoint := range deployment.Endpoints {
			endpoints[i] = spec.DeploymentEndpoint{
				Name:       endpoint.Name,
				Visibility: endpoint.Visibility,
				Url:        endpoint.URL,
			}
		}

		// Create the deployment details response
		var envDisplayName *string
		if deployment.EnvironmentDisplayName != "" {
			envDisplayName = &deployment.EnvironmentDisplayName
		}

		deploymentResponse := spec.DeploymentDetailsResponse{
			ImageId:                deployment.ImageId,
			Status:                 deployment.Status,
			LastDeployed:           deployment.LastDeployedAt,
			Endpoints:              endpoints,
			EnvironmentDisplayName: envDisplayName,
		}

		// Add to result map with environment name as key
		result[deployment.Environment] = deploymentResponse
	}

	return result
}

func ConvertToAgentEndpointResponse(endpointDetails map[string]models.EndpointsResponse) map[string]spec.EndpointConfiguration {
	result := make(map[string]spec.EndpointConfiguration)

	if len(endpointDetails) == 0 {
		return result
	}
	for endpointName, details := range endpointDetails {
		result[endpointName] = spec.EndpointConfiguration{
			Url:          details.URL,
			EndpointName: endpointName,
			Visibility:   details.Visibility,
			Schema: spec.EndpointSchema{
				Content: details.Schema.Content,
			},
		}
	}

	return result
}

func ConvertToEnvironmentListResponse(environments []*models.EnvironmentResponse) []spec.Environment {
	if len(environments) == 0 {
		return []spec.Environment{}
	}

	responses := make([]spec.Environment, len(environments))
	for i, env := range environments {
		responses[i] = ConvertToEnvironmentResponse(env)
	}

	return responses
}

func ConvertToEnvironmentResponse(env *models.EnvironmentResponse) spec.Environment {
	if env == nil {
		return spec.Environment{}
	}

	return spec.Environment{
		Name:         env.Name,
		DataplaneRef: env.DataplaneRef,
		IsProduction: env.IsProduction,
		CreatedAt:    env.CreatedAt,
		DisplayName:  &env.DisplayName,
		DnsPrefix:    &env.DNSPrefix,
	}
}

func ConvertToDeploymentPipelinesListResponse(pipelines []*models.DeploymentPipelineResponse, total int32, limit int32, offset int32) spec.DeploymentPipelineListResponse {
	responses := make([]spec.DeploymentPipelineResponse, len(pipelines))
	for i, pipeline := range pipelines {
		responses[i] = ConvertToDeploymentPipelineResponse(pipeline)
	}

	return spec.DeploymentPipelineListResponse{
		DeploymentPipelines: responses,
		Total:               total,
		Limit:               limit,
		Offset:              offset,
	}
}

func ConvertToDeploymentPipelineResponse(pipeline *models.DeploymentPipelineResponse) spec.DeploymentPipelineResponse {
	if pipeline == nil {
		return spec.DeploymentPipelineResponse{}
	}

	promotionPaths := make([]spec.PromotionPath, len(pipeline.PromotionPaths))
	for i, path := range pipeline.PromotionPaths {
		targetRefs := make([]spec.TargetEnvironmentRef, len(path.TargetEnvironmentRefs))
		for j, target := range path.TargetEnvironmentRefs {
			targetRefs[j] = spec.TargetEnvironmentRef{
				Name: target.Name,
			}
		}
		promotionPaths[i] = spec.PromotionPath{
			SourceEnvironmentRef:  path.SourceEnvironmentRef,
			TargetEnvironmentRefs: targetRefs,
		}
	}

	return spec.DeploymentPipelineResponse{
		Name:           pipeline.Name,
		DisplayName:    pipeline.DisplayName,
		Description:    pipeline.Description,
		OrgName:        pipeline.OrgName,
		CreatedAt:      pipeline.CreatedAt,
		PromotionPaths: promotionPaths,
	}
}

func ConvertToOrganizationResponse(org *models.OrganizationResponse) spec.OrganizationResponse {
	if org == nil {
		return spec.OrganizationResponse{}
	}

	return spec.OrganizationResponse{
		Name:        org.Name,
		CreatedAt:   org.CreatedAt,
		DisplayName: org.DisplayName,
		Description: org.Description,
		Namespace:   org.Namespace,
	}
}

func ConvertToOrganizationListItems(org *models.OrganizationResponse) spec.OrganizationListItem {
	if org == nil {
		return spec.OrganizationListItem{}
	}

	return spec.OrganizationListItem{
		Name:      org.Name,
		CreatedAt: org.CreatedAt,
	}
}

func ConvertToOrganizationListResponse(orgs []*models.OrganizationResponse) []spec.OrganizationListItem {
	if len(orgs) == 0 {
		return []spec.OrganizationListItem{}
	}

	responses := make([]spec.OrganizationListItem, len(orgs))
	for i, org := range orgs {
		responses[i] = ConvertToOrganizationListItems(org)
	}

	return responses
}

func ConvertToProjectResponse(project *models.ProjectResponse) spec.ProjectResponse {
	if project == nil {
		return spec.ProjectResponse{}
	}

	return spec.ProjectResponse{
		Uuid:                project.UUID,
		Name:               project.Name,
		DisplayName:        project.DisplayName,
		Description:        project.Description,
		CreatedAt:          project.CreatedAt,
		DeploymentPipeline: project.DeploymentPipeline,
		OrgName:            project.OrgName,
	}
}

func ConvertToProjectListItem(project *models.ProjectResponse) spec.ProjectListItem {
	if project == nil {
		return spec.ProjectListItem{}
	}

	return spec.ProjectListItem{
		Uuid:        project.UUID,
		Name:        project.Name,
		DisplayName: project.DisplayName,
		CreatedAt:   project.CreatedAt,
		OrgName:     project.OrgName,
	}
}

func ConvertToProjectListResponse(projects []*models.ProjectResponse) []spec.ProjectListItem {
	if len(projects) == 0 {
		return []spec.ProjectListItem{}
	}

	responses := make([]spec.ProjectListItem, len(projects))
	for i, project := range projects {
		responses[i] = ConvertToProjectListItem(project)
	}

	return responses
}

func ConvertToBuildLogsResponse(buildLogs models.BuildLogsResponse) spec.BuildLogsResponse {
	logEntries := make([]spec.LogEntry, len(buildLogs.Logs))
	for i, logEntry := range buildLogs.Logs {
		logEntries[i] = spec.LogEntry{
			Timestamp: logEntry.Timestamp,
			Log:       logEntry.Log,
			LogLevel:  logEntry.LogLevel,
		}
	}
	responses := spec.BuildLogsResponse{
		Logs:       logEntries,
		TotalCount: buildLogs.TotalCount,
		TookMs:     buildLogs.TookMs,
	}

	return responses
}

func ConvertToDataPlaneListResponse(dataPlanes []*models.DataPlaneResponse) []spec.DataPlane {
	if len(dataPlanes) == 0 {
		return []spec.DataPlane{}
	}

	responses := make([]spec.DataPlane, len(dataPlanes))
	for i, dp := range dataPlanes {
		responses[i] = spec.DataPlane{
			Name:        dp.Name,
			OrgName:     dp.OrgName,
			DisplayName: dp.DisplayName,
			Description: dp.Description,
			CreatedAt:   dp.CreatedAt,
		}
	}

	return responses
}
