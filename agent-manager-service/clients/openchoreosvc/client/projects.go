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

	ocapi "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

func (c *openChoreoClient) CreateProject(ctx context.Context, namespaceName string, req CreateProjectRequest) error {
	apiReq := ocapi.CreateProjectJSONRequestBody{
		Name:               req.Name,
		DisplayName:        &req.DisplayName,
		Description:        &req.Description,
		DeploymentPipeline: &req.DeploymentPipeline,
	}

	resp, err := c.ocClient.CreateProjectWithResponse(ctx, namespaceName, apiReq)
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated && resp.StatusCode() != http.StatusOK {
		return handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{
			NotFoundErr: utils.ErrOrganizationNotFound,
			ConflictErr: utils.ErrProjectAlreadyExists,
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
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{
			NotFoundErr: utils.ErrProjectNotFound,
		})
	}

	if resp.JSON200 == nil || resp.JSON200.Data == nil {
		return nil, fmt.Errorf("empty response from get project")
	}

	p := resp.JSON200.Data
	return &models.ProjectResponse{
		UUID:               p.Uid,
		Name:               p.Name,
		OrgName:            p.NamespaceName,
		DisplayName:        utils.StrPointerAsStr(p.DisplayName, ""),
		Description:        utils.StrPointerAsStr(p.Description, ""),
		DeploymentPipeline: utils.StrPointerAsStr(p.DeploymentPipeline, ""),
		CreatedAt:          p.CreatedAt,
	}, nil
}

func (c *openChoreoClient) PatchProject(ctx context.Context, namespaceName, projectName string, req PatchProjectRequest) error {
	body := ocapi.ApplyResourceJSONRequestBody{
		"apiVersion": ResourceAPIVersion,
		"kind":       ResourceKindProject,
		"metadata": map[string]interface{}{
			"name":      projectName,
			"namespace": namespaceName,
			"annotations": map[string]interface{}{
				AnnotationKeyDisplayName: req.DisplayName,
				AnnotationKeyDescription: req.Description,
			},
		},
		"spec": map[string]interface{}{
			"deploymentPipelineRef": req.DeploymentPipeline,
		},
	}

	resp, err := c.ocClient.ApplyResourceWithResponse(ctx, body)
	if err != nil {
		return fmt.Errorf("failed to apply project: %w", err)
	}

	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
		return handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{
			NotFoundErr: utils.ErrProjectNotFound,
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
		return handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{
			NotFoundErr: utils.ErrProjectNotFound,
		})
	}

	return nil
}

func (c *openChoreoClient) ListProjects(ctx context.Context, namespaceName string) ([]*models.ProjectResponse, error) {
	resp, err := c.ocClient.ListProjectsWithResponse(ctx, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{
			NotFoundErr: utils.ErrOrganizationNotFound,
		})
	}

	if resp.JSON200 == nil || resp.JSON200.Data == nil || resp.JSON200.Data.Items == nil {
		return []*models.ProjectResponse{}, nil
	}

	items := *resp.JSON200.Data.Items
	projects := make([]*models.ProjectResponse, len(items))
	for i, p := range items {
		projects[i] = &models.ProjectResponse{
			UUID:               p.Uid,
			Name:               p.Name,
			OrgName:            p.NamespaceName,
			DisplayName:        utils.StrPointerAsStr(p.DisplayName, ""),
			Description:        utils.StrPointerAsStr(p.Description, ""),
			DeploymentPipeline: utils.StrPointerAsStr(p.DeploymentPipeline, ""),
			CreatedAt:          p.CreatedAt,
		}
	}
	return projects, nil
}
