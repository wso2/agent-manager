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

// -----------------------------------------------------------------------------
// Organization Operations (maps to OC namespaces)
// -----------------------------------------------------------------------------

func (c *openChoreoClient) GetOrganization(ctx context.Context, orgName string) (*models.OrganizationResponse, error) {
	resp, err := c.ocClient.GetNamespaceWithResponse(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
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
		return nil, fmt.Errorf("empty response from get organization")
	}

	ns := resp.JSON200
	displayName := getAnnotation(ns.Metadata.Annotations, AnnotationKeyDisplayName)
	description := getAnnotation(ns.Metadata.Annotations, AnnotationKeyDescription)

	var createdAt time.Time
	if ns.Metadata.CreationTimestamp != nil {
		createdAt = *ns.Metadata.CreationTimestamp
	}

	return &models.OrganizationResponse{
		Name:        ns.Metadata.Name,
		DisplayName: displayName,
		Description: description,
		Namespace:   ns.Metadata.Name,
		CreatedAt:   createdAt,
	}, nil
}

func (c *openChoreoClient) ListOrganizations(ctx context.Context) ([]*models.OrganizationResponse, error) {
	resp, err := c.ocClient.ListNamespacesWithResponse(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list organizations: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON200 == nil || len(resp.JSON200.Items) == 0 {
		return []*models.OrganizationResponse{}, nil
	}

	orgs := make([]*models.OrganizationResponse, len(resp.JSON200.Items))
	for i, ns := range resp.JSON200.Items {
		displayName := getAnnotation(ns.Metadata.Annotations, AnnotationKeyDisplayName)
		description := getAnnotation(ns.Metadata.Annotations, AnnotationKeyDescription)

		var createdAt time.Time
		if ns.Metadata.CreationTimestamp != nil {
			createdAt = *ns.Metadata.CreationTimestamp
		}

		orgs[i] = &models.OrganizationResponse{
			Name:        ns.Metadata.Name,
			DisplayName: displayName,
			Description: description,
			Namespace:   ns.Metadata.Name,
			CreatedAt:   createdAt,
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
		return nil, handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from get environment")
	}

	return convertEnvironmentToResponse(resp.JSON200), nil
}

func (c *openChoreoClient) ListEnvironments(ctx context.Context, namespaceName string) ([]*models.EnvironmentResponse, error) {
	resp, err := c.ocClient.ListEnvironmentsWithResponse(ctx, namespaceName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON200 == nil || len(resp.JSON200.Items) == 0 {
		return []*models.EnvironmentResponse{}, nil
	}

	envs := make([]*models.EnvironmentResponse, len(resp.JSON200.Items))
	for i := range resp.JSON200.Items {
		envs[i] = convertEnvironmentToResponse(&resp.JSON200.Items[i])
	}
	return envs, nil
}

// -----------------------------------------------------------------------------
// Deployment Pipeline Operations
// -----------------------------------------------------------------------------

func (c *openChoreoClient) GetProjectDeploymentPipeline(ctx context.Context, namespaceName, projectName string) (*models.DeploymentPipelineResponse, error) {
	// First get the project to find the deployment pipeline reference
	projectResp, err := c.ocClient.GetProjectWithResponse(ctx, namespaceName, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	if projectResp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(projectResp.StatusCode(), ErrorResponses{
			JSON401: projectResp.JSON401,
			JSON403: projectResp.JSON403,
			JSON404: projectResp.JSON404,
			JSON500: projectResp.JSON500,
		})
	}
	if projectResp.JSON200 == nil || projectResp.JSON200.Spec == nil || projectResp.JSON200.Spec.DeploymentPipelineRef == nil {
		return nil, fmt.Errorf("project does not have a deployment pipeline reference")
	}

	pipelineName := *projectResp.JSON200.Spec.DeploymentPipelineRef

	// Get the deployment pipeline by name
	resp, err := c.ocClient.GetDeploymentPipelineWithResponse(ctx, namespaceName, pipelineName)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment pipeline: %w", err)
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
		return nil, fmt.Errorf("empty response from get deployment pipeline")
	}

	return convertDeploymentPipeline(resp.JSON200, namespaceName), nil
}

func convertDeploymentPipeline(p *ocapi.DeploymentPipeline, orgName string) *models.DeploymentPipelineResponse {
	if p == nil {
		return nil
	}

	displayName := getAnnotation(p.Metadata.Annotations, AnnotationKeyDisplayName)
	description := getAnnotation(p.Metadata.Annotations, AnnotationKeyDescription)

	var createdAt time.Time
	if p.Metadata.CreationTimestamp != nil {
		createdAt = *p.Metadata.CreationTimestamp
	}

	var promotionPaths []models.PromotionPath
	if p.Spec != nil && p.Spec.PromotionPaths != nil {
		promotionPaths = make([]models.PromotionPath, len(*p.Spec.PromotionPaths))
		for i, pp := range *p.Spec.PromotionPaths {
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
		Name:           p.Metadata.Name,
		DisplayName:    displayName,
		Description:    description,
		OrgName:        orgName,
		CreatedAt:      createdAt,
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
	resp, err := c.ocClient.ListDataPlanesWithResponse(ctx, namespaceName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list data planes: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON200 == nil || len(resp.JSON200.Items) == 0 {
		return []*models.DataPlaneResponse{}, nil
	}

	dataPlanes := make([]*models.DataPlaneResponse, len(resp.JSON200.Items))
	for i, dp := range resp.JSON200.Items {
		displayName := getAnnotation(dp.Metadata.Annotations, AnnotationKeyDisplayName)
		description := getAnnotation(dp.Metadata.Annotations, AnnotationKeyDescription)

		var createdAt time.Time
		if dp.Metadata.CreationTimestamp != nil {
			createdAt = *dp.Metadata.CreationTimestamp
		}

		dataPlanes[i] = &models.DataPlaneResponse{
			Name:        dp.Metadata.Name,
			OrgName:     namespaceName,
			DisplayName: displayName,
			Description: description,
			CreatedAt:   createdAt,
		}
	}
	return dataPlanes, nil
}

func convertEnvironmentToResponse(env *ocapi.Environment) *models.EnvironmentResponse {
	if env == nil {
		return nil
	}

	displayName := getAnnotation(env.Metadata.Annotations, AnnotationKeyDisplayName)

	var createdAt time.Time
	if env.Metadata.CreationTimestamp != nil {
		createdAt = *env.Metadata.CreationTimestamp
	}

	var dataplaneRef string
	var isProduction bool
	if env.Spec != nil {
		if env.Spec.DataPlaneRef != nil {
			dataplaneRef = env.Spec.DataPlaneRef.Name
		}
		if env.Spec.IsProduction != nil {
			isProduction = *env.Spec.IsProduction
		}
	}

	// DNS prefix is derived from gateway config if available
	var dnsPrefix string
	if env.Spec != nil && env.Spec.Gateway != nil && env.Spec.Gateway.PublicVirtualHost != nil {
		dnsPrefix = *env.Spec.Gateway.PublicVirtualHost
	}

	return &models.EnvironmentResponse{
		UUID:         utils.StrPointerAsStr(env.Metadata.Uid, ""),
		Name:         env.Metadata.Name,
		DisplayName:  displayName,
		DataplaneRef: dataplaneRef,
		IsProduction: isProduction,
		DNSPrefix:    dnsPrefix,
		CreatedAt:    createdAt,
	}
}
