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

// -----------------------------------------------------------------------------
// Organization Operations (maps to OC namespaces)
// -----------------------------------------------------------------------------

func (c *openChoreoClient) GetOrganization(ctx context.Context, orgName string) (*models.OrganizationResponse, error) {
	resp, err := c.ocClient.GetNamespaceWithResponse(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{
			NotFoundErr: utils.ErrOrganizationNotFound,
		})
	}

	if resp.JSON200 == nil || resp.JSON200.Data == nil {
		return nil, fmt.Errorf("empty response from get organization")
	}

	ns := resp.JSON200.Data
	return &models.OrganizationResponse{
		Name:        ns.Name,
		DisplayName: utils.StrPointerAsStr(ns.DisplayName, ""),
		Description: utils.StrPointerAsStr(ns.Description, ""),
		Namespace:   ns.Name,
		CreatedAt:   ns.CreatedAt,
	}, nil
}

func (c *openChoreoClient) ListOrganizations(ctx context.Context) ([]*models.OrganizationResponse, error) {
	resp, err := c.ocClient.ListNamespacesWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list organizations: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{})
	}
	fmt.Print("list orgs\n")
	fmt.Println(string(resp.Body))
	fmt.Print("list orgs\n")

	if resp.JSON200 == nil || resp.JSON200.Data == nil || resp.JSON200.Data.Items == nil {
		return []*models.OrganizationResponse{}, nil
	}

	items := *resp.JSON200.Data.Items
	orgs := make([]*models.OrganizationResponse, len(items))
	for i, ns := range items {
		orgs[i] = &models.OrganizationResponse{
			Name:        ns.Name,
			DisplayName: utils.StrPointerAsStr(ns.DisplayName, ""),
			Description: utils.StrPointerAsStr(ns.Description, ""),
			Namespace:   ns.Name, // Namespace name is the same as org name
			CreatedAt:   ns.CreatedAt,
		}
	}
	return orgs, nil
}

// -----------------------------------------------------------------------------
// Environment Operations
// -----------------------------------------------------------------------------

func (c *openChoreoClient) GetEnvironment(ctx context.Context, namespaceName, environmentName string) (*models.EnvironmentResponse, error) {
	resp, err := c.ocClient.GetEnvironmentWithResponse(ctx, namespaceName, environmentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{
			NotFoundErr: utils.ErrEnvironmentNotFound,
		})
	}

	if resp.JSON200 == nil || resp.JSON200.Data == nil {
		return nil, fmt.Errorf("empty response from get environment")
	}

	env := resp.JSON200.Data
	return &models.EnvironmentResponse{
		UUID:         env.Uid,
		Name:         env.Name,
		DisplayName:  utils.StrPointerAsStr(env.DisplayName, ""),
		DataplaneRef: utils.StrPointerAsStr(env.DataPlaneRef, ""),
		IsProduction: env.IsProduction,
		DNSPrefix:    utils.StrPointerAsStr(env.DnsPrefix, ""),
		CreatedAt:    env.CreatedAt,
	}, nil
}

func (c *openChoreoClient) ListEnvironments(ctx context.Context, namespaceName string) ([]*models.EnvironmentResponse, error) {
	resp, err := c.ocClient.ListEnvironmentsWithResponse(ctx, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{})
	}

	if resp.JSON200 == nil || resp.JSON200.Data == nil || resp.JSON200.Data.Items == nil {
		return []*models.EnvironmentResponse{}, nil
	}

	items := *resp.JSON200.Data.Items
	envs := make([]*models.EnvironmentResponse, len(items))
	for i, env := range items {
		envs[i] = &models.EnvironmentResponse{
			UUID:         env.Uid,
			Name:         env.Name,
			DisplayName:  utils.StrPointerAsStr(env.DisplayName, ""),
			DataplaneRef: utils.StrPointerAsStr(env.DataPlaneRef, ""),
			IsProduction: env.IsProduction,
			DNSPrefix:    utils.StrPointerAsStr(env.DnsPrefix, ""),
			CreatedAt:    env.CreatedAt,
		}
	}
	return envs, nil
}

// -----------------------------------------------------------------------------
// Deployment Pipeline Operations
// -----------------------------------------------------------------------------

func (c *openChoreoClient) GetProjectDeploymentPipeline(ctx context.Context, namespaceName, projectName string) (*models.DeploymentPipelineResponse, error) {
	resp, err := c.ocClient.GetProjectDeploymentPipelineWithResponse(ctx, namespaceName, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment pipeline: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{
			NotFoundErr: utils.ErrDeploymentPipelineNotFound,
		})
	}

	if resp.JSON200 == nil || resp.JSON200.Data == nil {
		return nil, fmt.Errorf("empty response from get deployment pipeline")
	}

	return convertDeploymentPipeline(resp.JSON200.Data, namespaceName), nil
}

func convertDeploymentPipeline(p *ocapi.DeploymentPipelineResponse, orgName string) *models.DeploymentPipelineResponse {
	if p == nil {
		return nil
	}

	var promotionPaths []models.PromotionPath
	if p.PromotionPaths != nil {
		promotionPaths = make([]models.PromotionPath, len(*p.PromotionPaths))
		for i, pp := range *p.PromotionPaths {
			targetRefs := make([]models.TargetEnvironmentRef, len(pp.TargetEnvironmentRefs))
			for j, tr := range pp.TargetEnvironmentRefs {
				targetRefs[j] = models.TargetEnvironmentRef{
					Name:             tr.Name,
					RequiresApproval: utils.BoolPointerAsBool(tr.RequiresApproval, false),
				}
			}
			promotionPaths[i] = models.PromotionPath{
				SourceEnvironmentRef:  pp.SourceEnvironmentRef,
				TargetEnvironmentRefs: targetRefs,
			}
		}
	}

	return &models.DeploymentPipelineResponse{
		Name:           p.Name,
		DisplayName:    utils.StrPointerAsStr(p.DisplayName, ""),
		Description:    utils.StrPointerAsStr(p.Description, ""),
		OrgName:        orgName,
		CreatedAt:      p.CreatedAt,
		PromotionPaths: promotionPaths,
	}
}

func (c *openChoreoClient) ListDeploymentPipelines(ctx context.Context, namespaceName string) ([]*models.DeploymentPipelineResponse, error) {
	// API does not support listing deployment pipelines directly
	return nil, fmt.Errorf("not implemented: API does not support listing deployment pipelines")
}

// -----------------------------------------------------------------------------
// Data Plane Operations
// -----------------------------------------------------------------------------

func (c *openChoreoClient) ListDataPlanes(ctx context.Context, namespaceName string) ([]*models.DataPlaneResponse, error) {
	resp, err := c.ocClient.ListDataPlanesWithResponse(ctx, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to list data planes: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{})
	}

	if resp.JSON200 == nil || resp.JSON200.Data == nil || resp.JSON200.Data.Items == nil {
		return []*models.DataPlaneResponse{}, nil
	}

	items := *resp.JSON200.Data.Items
	dataPlanes := make([]*models.DataPlaneResponse, len(items))
	for i, dp := range items {
		dataPlanes[i] = &models.DataPlaneResponse{
			Name:        dp.Name,
			OrgName:     namespaceName,
			DisplayName: utils.StrPointerAsStr(dp.DisplayName, ""),
			Description: utils.StrPointerAsStr(dp.Description, ""),
			CreatedAt:   dp.CreatedAt,
		}
	}
	return dataPlanes, nil
}
