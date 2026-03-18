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

package client

import (
	"context"
	"fmt"
	"net/http"
	"time"

	ocapi "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

func (c *openChoreoClient) CreateProject(ctx context.Context, namespaceName string, req CreateProjectRequest) error {
	annotations := map[string]string{
		string(AnnotationKeyDisplayName): req.DisplayName,
		string(AnnotationKeyDescription): req.Description,
	}
	apiReq := ocapi.CreateProjectJSONRequestBody{
		Metadata: ocapi.ObjectMeta{
			Name:        req.Name,
			Namespace:   &namespaceName,
			Annotations: &annotations,
		},
		Spec: &ocapi.ProjectSpec{
			DeploymentPipelineRef: &req.DeploymentPipeline,
		},
	}

	resp, err := c.ocClient.CreateProjectWithResponse(ctx, namespaceName, apiReq)
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated && resp.StatusCode() != http.StatusOK {
		return handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON400: resp.JSON400,
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON409: resp.JSON409,
			JSON500: resp.JSON500,
		})
	}

	return nil
}

func (c *openChoreoClient) GetProject(ctx context.Context, namespaceName, projectName string) (*models.ProjectResponse, error) {
	resp, err := c.ocClient.GetProjectWithResponse(ctx, namespaceName, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from get project")
	}

	return convertProjectToResponse(resp.JSON200), nil
}

func (c *openChoreoClient) PatchProject(ctx context.Context, namespaceName, projectName string, req PatchProjectRequest) error {
	// Get the project
	resp, err := c.ocClient.GetProjectWithResponse(ctx, namespaceName, projectName)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}
	if resp.JSON200 == nil {
		return fmt.Errorf("invalid project response")
	}

	project := resp.JSON200

	// Update annotations
	if project.Metadata.Annotations == nil {
		annotations := make(map[string]string)
		project.Metadata.Annotations = &annotations
	}
	(*project.Metadata.Annotations)[string(AnnotationKeyDisplayName)] = req.DisplayName
	(*project.Metadata.Annotations)[string(AnnotationKeyDescription)] = req.Description

	// Update spec
	if project.Spec == nil {
		project.Spec = &ocapi.ProjectSpec{}
	}
	project.Spec.DeploymentPipelineRef = &req.DeploymentPipeline

	// Update the project
	updateResp, err := c.ocClient.UpdateProjectWithResponse(ctx, namespaceName, projectName, *project)
	if err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}
	if updateResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(updateResp.StatusCode(), ErrorResponses{
			JSON401: updateResp.JSON401,
			JSON403: updateResp.JSON403,
			JSON404: updateResp.JSON404,
			JSON500: updateResp.JSON500,
		})
	}

	return nil
}

func (c *openChoreoClient) DeleteProject(ctx context.Context, namespaceName, projectName string) error {
	resp, err := c.ocClient.DeleteProjectWithResponse(ctx, namespaceName, projectName)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusNoContent {
		return handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}

	return nil
}

func (c *openChoreoClient) ListProjects(ctx context.Context, namespaceName string) ([]*models.ProjectResponse, error) {
	resp, err := c.ocClient.ListProjectsWithResponse(ctx, namespaceName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON200 == nil {
		return []*models.ProjectResponse{}, nil
	}

	items := resp.JSON200.Items
	projects := make([]*models.ProjectResponse, len(items))
	for i := range items {
		projects[i] = convertProjectToResponse(&items[i])
	}
	return projects, nil
}

func convertProjectToResponse(p *ocapi.Project) *models.ProjectResponse {
	displayName := ""
	description := ""
	if p.Metadata.Annotations != nil {
		if v, ok := (*p.Metadata.Annotations)[string(AnnotationKeyDisplayName)]; ok {
			displayName = v
		}
		if v, ok := (*p.Metadata.Annotations)[string(AnnotationKeyDescription)]; ok {
			description = v
		}
	}

	deploymentPipeline := ""
	if p.Spec != nil && p.Spec.DeploymentPipelineRef != nil {
		deploymentPipeline = *p.Spec.DeploymentPipelineRef
	}

	var createdAt time.Time
	if p.Metadata.CreationTimestamp != nil {
		createdAt = *p.Metadata.CreationTimestamp
	}

	return &models.ProjectResponse{
		UUID:               utils.StrPointerAsStr(p.Metadata.Uid, ""),
		Name:               p.Metadata.Name,
		OrgName:            utils.StrPointerAsStr(p.Metadata.Namespace, ""),
		DisplayName:        displayName,
		Description:        description,
		DeploymentPipeline: deploymentPipeline,
		CreatedAt:          createdAt,
	}
}
