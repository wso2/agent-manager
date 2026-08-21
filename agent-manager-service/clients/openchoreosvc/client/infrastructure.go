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
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	ocapi "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// -----------------------------------------------------------------------------
// Organization Operations (maps to OC namespaces)
// -----------------------------------------------------------------------------

func (c *openChoreoClient) GetOrganization(ctx context.Context, ouID string) (*models.OrganizationResponse, error) {
	namespaceName := c.NamespaceFor(ouID)
	resp, err := c.ocClient.GetNamespaceWithResponse(ctx, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
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
		return nil, handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
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

func (c *openChoreoClient) CreateEnvironment(ctx context.Context, ouID string, req CreateEnvironmentRequest) (*models.EnvironmentResponse, error) {
	namespaceName := c.NamespaceFor(ouID)
	annotations := map[string]string{
		AnnotationKeyDisplayName: req.DisplayName,
	}
	if req.Description != "" {
		annotations[AnnotationKeyDescription] = req.Description
	}
	if req.IsolationTier != "" {
		annotations[AnnotationKeyIsolationTier] = req.IsolationTier
	}

	isProduction := req.IsProduction
	dataplaneRefKind := ocapi.EnvironmentSpecDataPlaneRefKindClusterDataPlane

	gateway, err := c.resolveEnvGatewaySpec(ctx, req.DataplaneRef, req.Gateway)
	if err != nil && !errors.Is(err, errInheritGateway) {
		return nil, fmt.Errorf("failed to resolve gateway spec: %w", err)
	}

	body := ocapi.CreateEnvironmentJSONRequestBody{
		Metadata: ocapi.ObjectMeta{
			Name:        req.Name,
			Namespace:   &namespaceName,
			Annotations: &annotations,
		},
		Spec: &ocapi.EnvironmentSpec{
			DataPlaneRef: &struct {
				Kind ocapi.EnvironmentSpecDataPlaneRefKind `json:"kind"`
				Name string                                `json:"name"`
			}{
				Kind: dataplaneRefKind,
				Name: req.DataplaneRef,
			},
			IsProduction: &isProduction,
			Gateway:      gateway,
		},
	}

	resp, err := c.ocClient.CreateEnvironmentWithResponse(ctx, namespaceName, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create environment: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated {
		return nil, handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
			JSON400: resp.JSON400,
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON409: resp.JSON409,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON201 == nil {
		return nil, fmt.Errorf("empty response from create environment")
	}

	return convertEnvironmentToResponse(resp.JSON201), nil
}

func (c *openChoreoClient) UpdateEnvironment(ctx context.Context, ouID, environmentName string, req UpdateEnvironmentRequest) (*models.EnvironmentResponse, error) {
	namespaceName := c.NamespaceFor(ouID)
	// First get the existing environment to preserve fields
	getResp, err := c.ocClient.GetEnvironmentWithResponse(ctx, namespaceName, environmentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get environment for update: %w", err)
	}
	if getResp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(ctx, getResp.StatusCode(), ErrorResponses{
			JSON401: getResp.JSON401,
			JSON403: getResp.JSON403,
			JSON404: getResp.JSON404,
			JSON500: getResp.JSON500,
		})
	}
	if getResp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from get environment")
	}

	env := getResp.JSON200

	// Update annotations
	if env.Metadata.Annotations == nil {
		annotations := make(map[string]string)
		env.Metadata.Annotations = &annotations
	}
	if req.DisplayName != nil {
		(*env.Metadata.Annotations)[AnnotationKeyDisplayName] = *req.DisplayName
	}
	if req.Description != nil {
		(*env.Metadata.Annotations)[AnnotationKeyDescription] = *req.Description
	}

	// Update spec fields
	if req.IsProduction != nil {
		if env.Spec == nil {
			env.Spec = &ocapi.EnvironmentSpec{}
		}
		env.Spec.IsProduction = req.IsProduction
	}
	if req.Gateway != nil {
		if env.Spec == nil {
			env.Spec = &ocapi.EnvironmentSpec{}
		}
		dataPlaneRef := ""
		if env.Spec.DataPlaneRef != nil {
			dataPlaneRef = env.Spec.DataPlaneRef.Name
		}
		gateway, gwErr := c.resolveEnvGatewaySpec(ctx, dataPlaneRef, req.Gateway)
		if gwErr != nil && !errors.Is(gwErr, errInheritGateway) {
			return nil, fmt.Errorf("failed to resolve gateway spec: %w", gwErr)
		}
		env.Spec.Gateway = gateway
	}

	resp, err := c.ocClient.UpdateEnvironmentWithResponse(ctx, namespaceName, environmentName, *env)
	if err != nil {
		return nil, fmt.Errorf("failed to update environment: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
			JSON400: resp.JSON400,
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON409: resp.JSON409,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from update environment")
	}

	return convertEnvironmentToResponse(resp.JSON200), nil
}

func (c *openChoreoClient) GetEnvironment(ctx context.Context, ouID, environmentName string) (*models.EnvironmentResponse, error) {
	namespaceName := c.NamespaceFor(ouID)
	resp, err := c.ocClient.GetEnvironmentWithResponse(ctx, namespaceName, environmentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
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

// DeleteEnvironment removes the Environment CR from OpenChoreo. Returns utils.ErrNotFound when
// the named environment does not exist. OpenChoreo refuses deletion while bindings/deployments
// still reference the environment, so callers should ensure local-side cleanup of agents
// (configs, monitors, applications) happens before this call.
func (c *openChoreoClient) DeleteEnvironment(ctx context.Context, ouID, environmentName string) error {
	namespaceName := c.NamespaceFor(ouID)
	resp, err := c.ocClient.DeleteEnvironmentWithResponse(ctx, namespaceName, environmentName)
	if err != nil {
		return fmt.Errorf("failed to delete environment: %w", err)
	}

	// OpenChoreo returns 204 No Content on success; tolerate 200 as well for forward compatibility.
	switch resp.StatusCode() {
	case http.StatusNoContent, http.StatusOK:
		return nil
	}

	return handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
		JSON401: resp.JSON401,
		JSON403: resp.JSON403,
		JSON404: resp.JSON404,
		JSON500: resp.JSON500,
	})
}

func (c *openChoreoClient) ListEnvironments(ctx context.Context, ouID string) ([]*models.EnvironmentResponse, error) {
	namespaceName := c.NamespaceFor(ouID)
	resp, err := c.ocClient.ListEnvironmentsWithResponse(ctx, namespaceName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
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

func (c *openChoreoClient) GetProjectDeploymentPipeline(ctx context.Context, ouID, projectName string) (*models.DeploymentPipelineResponse, error) {
	namespaceName := c.NamespaceFor(ouID)
	// First get the project to find the deployment pipeline reference.
	// A 404 here means the *project* is missing, not the pipeline — surface that
	// distinctly so callers don't mislabel a missing project as a missing pipeline.
	projectResp, err := c.ocClient.GetProjectWithResponse(ctx, namespaceName, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	if projectResp.StatusCode() != http.StatusOK {
		if projectResp.StatusCode() == http.StatusNotFound {
			return nil, fmt.Errorf("%w: %s", utils.ErrProjectNotFound, projectName)
		}
		return nil, handleErrorResponse(ctx, projectResp.StatusCode(), ErrorResponses{
			JSON401: projectResp.JSON401,
			JSON403: projectResp.JSON403,
			JSON404: projectResp.JSON404,
			JSON500: projectResp.JSON500,
		})
	}
	if projectResp.JSON200 == nil || projectResp.JSON200.Spec == nil || projectResp.JSON200.Spec.DeploymentPipelineRef == nil {
		return nil, fmt.Errorf("%w: project %s has no deployment pipeline reference", utils.ErrDeploymentPipelineNotFound, projectName)
	}

	pipelineName := projectResp.JSON200.Spec.DeploymentPipelineRef.Name

	// Get the deployment pipeline by name
	resp, err := c.ocClient.GetDeploymentPipelineWithResponse(ctx, namespaceName, pipelineName)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment pipeline: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
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

func convertDeploymentPipeline(p *ocapi.DeploymentPipeline, namespaceName string) *models.DeploymentPipelineResponse {
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
		promotionPaths = make([]models.PromotionPath, 0, len(*p.Spec.PromotionPaths))
		for _, pp := range *p.Spec.PromotionPaths {
			// Strip dummy " " targets injected as an OC REST API workaround
			// (OC requires at least one targetEnvironmentRef via the API).
			targetRefs := make([]models.TargetEnvironmentRef, 0, len(pp.TargetEnvironmentRefs))
			for _, tr := range pp.TargetEnvironmentRefs {
				if name := strings.TrimSpace(tr.Name); name != "" {
					targetRefs = append(targetRefs, models.TargetEnvironmentRef{Name: name})
				}
			}
			promotionPaths = append(promotionPaths, models.PromotionPath{
				SourceEnvironmentRef:  pp.SourceEnvironmentRef.Name,
				TargetEnvironmentRefs: targetRefs,
			})
		}
	}

	return &models.DeploymentPipelineResponse{
		Name:           p.Metadata.Name,
		DisplayName:    displayName,
		Description:    description,
		OrgName:        namespaceName,
		CreatedAt:      createdAt,
		PromotionPaths: promotionPaths,
	}
}

// ocDummyTarget is a single-element slice injected when a path has no real targets.
// OC's REST API requires at least one targetEnvironmentRef per path; the space character
// satisfies the 1-char minimum and is stripped by convertDeploymentPipeline on read.
var ocDummyTarget = []ocapi.TargetEnvironmentRef{{Name: " "}}

// toOCPromotionPaths converts model promotion paths to the OC API format.
func toOCPromotionPaths(promotionPaths []models.PromotionPath) []ocapi.PromotionPath {
	ocPaths := make([]ocapi.PromotionPath, len(promotionPaths))
	for i, p := range promotionPaths {
		var targets []ocapi.TargetEnvironmentRef
		if len(p.TargetEnvironmentRefs) == 0 {
			targets = ocDummyTarget
		} else {
			targets = make([]ocapi.TargetEnvironmentRef, len(p.TargetEnvironmentRefs))
			for j, t := range p.TargetEnvironmentRefs {
				targets[j] = ocapi.TargetEnvironmentRef{Name: t.Name}
			}
		}
		ocPaths[i] = ocapi.PromotionPath{
			SourceEnvironmentRef: struct {
				Kind *ocapi.PromotionPathSourceEnvironmentRefKind `json:"kind,omitempty"`
				Name string                                       `json:"name"`
			}{Name: p.SourceEnvironmentRef},
			TargetEnvironmentRefs: targets,
		}
	}
	return ocPaths
}

func (c *openChoreoClient) CreateDeploymentPipeline(ctx context.Context, ouID, pipelineName string, displayName *string, description *string, promotionPaths []models.PromotionPath) (*models.DeploymentPipelineResponse, error) {
	namespaceName := c.NamespaceFor(ouID)
	annotations := make(map[string]string)
	if displayName != nil {
		annotations[AnnotationKeyDisplayName] = *displayName
	}
	if description != nil {
		annotations[AnnotationKeyDescription] = *description
	}

	ocPaths := toOCPromotionPaths(promotionPaths)

	pipeline := ocapi.DeploymentPipeline{
		Metadata: ocapi.ObjectMeta{
			Name:        pipelineName,
			Namespace:   &namespaceName,
			Annotations: &annotations,
		},
		Spec: &ocapi.DeploymentPipelineSpec{
			PromotionPaths: &ocPaths,
		},
	}

	resp, err := c.ocClient.CreateDeploymentPipelineWithResponse(ctx, namespaceName, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment pipeline: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated {
		return nil, handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
			JSON400: resp.JSON400,
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON409: resp.JSON409,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON201 == nil {
		return nil, fmt.Errorf("empty response from create deployment pipeline")
	}

	return convertDeploymentPipeline(resp.JSON201, namespaceName), nil
}

func (c *openChoreoClient) UpdateDeploymentPipeline(ctx context.Context, ouID, pipelineName string, displayName *string, description *string, promotionPaths []models.PromotionPath) (*models.DeploymentPipelineResponse, error) {
	namespaceName := c.NamespaceFor(ouID)
	// Get existing pipeline to preserve metadata
	getResp, err := c.ocClient.GetDeploymentPipelineWithResponse(ctx, namespaceName, pipelineName)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment pipeline for update: %w", err)
	}
	if getResp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(ctx, getResp.StatusCode(), ErrorResponses{
			JSON401: getResp.JSON401,
			JSON403: getResp.JSON403,
			JSON404: getResp.JSON404,
			JSON500: getResp.JSON500,
		})
	}
	if getResp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from get deployment pipeline")
	}

	pipeline := getResp.JSON200

	// Update annotations for display name and description
	if displayName != nil || description != nil {
		if pipeline.Metadata.Annotations == nil {
			annotations := make(map[string]string)
			pipeline.Metadata.Annotations = &annotations
		}
		if displayName != nil {
			(*pipeline.Metadata.Annotations)[AnnotationKeyDisplayName] = *displayName
		}
		if description != nil {
			(*pipeline.Metadata.Annotations)[AnnotationKeyDescription] = *description
		}
	}

	// Convert model promotion paths to OC API format
	ocPaths := toOCPromotionPaths(promotionPaths)

	if pipeline.Spec == nil {
		pipeline.Spec = &ocapi.DeploymentPipelineSpec{}
	}
	pipeline.Spec.PromotionPaths = &ocPaths

	resp, err := c.ocClient.UpdateDeploymentPipelineWithResponse(ctx, namespaceName, pipelineName, *pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to update deployment pipeline: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
			JSON400: resp.JSON400,
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from update deployment pipeline")
	}

	return convertDeploymentPipeline(resp.JSON200, namespaceName), nil
}

func (c *openChoreoClient) DeleteOrgDeploymentPipeline(ctx context.Context, ouID string, pipelineName string) error {
	namespaceName := c.NamespaceFor(ouID)
	resp, err := c.ocClient.DeleteDeploymentPipelineWithResponse(ctx, namespaceName, pipelineName)
	if err != nil {
		return fmt.Errorf("failed to delete deployment pipeline: %w", err)
	}
	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusNoContent {
		return handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}
	return nil
}

func (c *openChoreoClient) ListDeploymentPipelines(ctx context.Context, ouID string) ([]*models.DeploymentPipelineResponse, error) {
	namespaceName := c.NamespaceFor(ouID)
	resp, err := c.ocClient.ListDeploymentPipelinesWithResponse(ctx, namespaceName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployment pipelines: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON200 == nil || len(resp.JSON200.Items) == 0 {
		return []*models.DeploymentPipelineResponse{}, nil
	}

	pipelines := make([]*models.DeploymentPipelineResponse, len(resp.JSON200.Items))
	for i, p := range resp.JSON200.Items {
		pipelines[i] = convertDeploymentPipeline(&p, namespaceName)
	}
	return pipelines, nil
}

// -----------------------------------------------------------------------------
// Data Plane Operations
// -----------------------------------------------------------------------------

func (c *openChoreoClient) ListDataPlanes(ctx context.Context) ([]*models.DataPlaneResponse, error) {
	resp, err := c.ocClient.ListClusterDataPlanesWithResponse(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list data planes: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
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
	description := getAnnotation(env.Metadata.Annotations, AnnotationKeyDescription)
	isolationTier := getAnnotation(env.Metadata.Annotations, AnnotationKeyIsolationTier)

	var createdAt time.Time
	if env.Metadata.CreationTimestamp != nil {
		createdAt = *env.Metadata.CreationTimestamp
	}

	var dataplaneRef string
	var isProduction bool
	var gateway *models.GatewaySpec
	if env.Spec != nil {
		if env.Spec.DataPlaneRef != nil {
			dataplaneRef = env.Spec.DataPlaneRef.Name
		}
		if env.Spec.IsProduction != nil {
			isProduction = *env.Spec.IsProduction
		}
		gateway = fromOCGatewaySpec(env.Spec.Gateway)
	}

	return &models.EnvironmentResponse{
		UUID:          utils.StrPointerAsStr(env.Metadata.Uid, ""),
		Name:          env.Metadata.Name,
		DisplayName:   displayName,
		DataplaneRef:  dataplaneRef,
		IsProduction:  isProduction,
		Description:   description,
		IsolationTier: isolationTier,
		Gateway:       gateway,
		CreatedAt:     createdAt,
	}
}

// -----------------------------------------------------------------------------
// Gateway spec resolution
//
// OpenChoreo resolves an Environment's effective gateway by merging at the
// endpoint-object level (see openchoreo internal/pipeline/component/context:
// mergeGatewayNetworkData): if Environment.spec.gateway.ingress.external is set
// it WHOLLY REPLACES the dataplane's external endpoint — there is no per-field
// fallback, and Name/Namespace are required with no defaulting webhook. The
// agent HTTPRoute's parentRef is built straight from these (name+namespace), and
// its hostname from "<env>-<org>.<listener.host>".
//

// errInheritGateway signals that no gateway spec should be emitted and the
// environment should inherit the dataplane gateway. It is an expected outcome,
// not a failure — callers check it with errors.Is and treat the spec as absent.
var errInheritGateway = errors.New("no gateway spec; inherit dataplane gateway")

// resolveEnvGatewaySpec turns the caller-supplied (trimmed) gateway override into
// a complete OC GatewaySpec by seeding from the dataplane's gateway and applying
// only the host/port the caller set. It returns errInheritGateway when there is
// no override or no dataplane base to anchor Name/Namespace, meaning the
// environment should inherit the dataplane gateway.
func (c *openChoreoClient) resolveEnvGatewaySpec(ctx context.Context, dataPlaneRef string, override *GatewaySpec) (*ocapi.GatewaySpec, error) {
	if override == nil {
		return nil, errInheritGateway
	}

	// Propagates errInheritGateway when the dataplane has no gateway to seed
	// Name/Namespace from — we can't form a valid override, so inherit instead.
	base, err := c.getClusterDataPlaneGateway(ctx, dataPlaneRef)
	if err != nil {
		return nil, err
	}

	return &ocapi.GatewaySpec{
		Ingress: applyNetworkOverride(base.Ingress, override.Ingress),
		Egress:  applyNetworkOverride(base.Egress, override.Egress),
	}, nil
}

// getClusterDataPlaneGateway fetches the ClusterDataPlane's gateway config (the
// source OC falls back to). Environments here always reference a ClusterDataPlane
// (see CreateEnvironment). It returns errInheritGateway when the dataplane has no
// gateway set, so there is no base to seed an override from.
func (c *openChoreoClient) getClusterDataPlaneGateway(ctx context.Context, dataPlaneRef string) (*ocapi.GatewaySpec, error) {
	resp, err := c.ocClient.GetClusterDataPlaneWithResponse(ctx, dataPlaneRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster dataplane %q: %w", dataPlaneRef, err)
	}
	if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		return nil, handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}
	if resp.JSON200.Spec == nil || resp.JSON200.Spec.Gateway == nil {
		return nil, errInheritGateway
	}
	return resp.JSON200.Spec.Gateway, nil
}

// applyNetworkOverride returns the dataplane network endpoint with any caller
// host/port overrides applied. The base carries the real Name/Namespace.
func applyNetworkOverride(base *ocapi.GatewayNetworkSpec, ov *GatewayNetworkSpec) *ocapi.GatewayNetworkSpec {
	if base == nil {
		return nil
	}
	out := *base
	if ov != nil {
		out.External = applyEndpointOverride(base.External, ov.External)
		out.Internal = applyEndpointOverride(base.Internal, ov.Internal)
	}
	return &out
}

func applyEndpointOverride(base *ocapi.GatewayEndpointSpec, ov *GatewayEndpointSpec) *ocapi.GatewayEndpointSpec {
	if base == nil {
		return nil
	}
	out := *base // preserves required Name/Namespace
	if ov != nil {
		out.Http = applyListenerOverride(base.Http, ov.HTTP)
		out.Https = applyListenerOverride(base.Https, ov.HTTPS)
		out.Tls = applyListenerOverride(base.Tls, ov.TLS)
	}
	return &out
}

// applyListenerOverride patches host/port from the caller onto the dataplane
// listener, preserving ListenerName. If the dataplane has no such listener there
// is nothing to anchor to, so the override slot is dropped (inherits nothing).
func applyListenerOverride(base *ocapi.GatewayListenerSpec, ov *GatewayListenerSpec) *ocapi.GatewayListenerSpec {
	if base == nil || ov == nil {
		return base
	}
	out := *base
	if ov.Host != nil {
		out.Host = ov.Host
	}
	if ov.Port != nil {
		out.Port = ov.Port
	}
	return &out
}

func fromOCGatewaySpec(g *ocapi.GatewaySpec) *models.GatewaySpec {
	if g == nil {
		return nil
	}
	ingress := fromOCGatewayNetworkSpec(g.Ingress)
	egress := fromOCGatewayNetworkSpec(g.Egress)
	if ingress == nil && egress == nil {
		return nil
	}
	return &models.GatewaySpec{Ingress: ingress, Egress: egress}
}

func fromOCGatewayNetworkSpec(n *ocapi.GatewayNetworkSpec) *models.GatewayNetworkSpec {
	if n == nil {
		return nil
	}
	external := fromOCGatewayEndpointSpec(n.External)
	internal := fromOCGatewayEndpointSpec(n.Internal)
	if external == nil && internal == nil {
		return nil
	}
	return &models.GatewayNetworkSpec{External: external, Internal: internal}
}

func fromOCGatewayEndpointSpec(e *ocapi.GatewayEndpointSpec) *models.GatewayEndpointSpec {
	if e == nil {
		return nil
	}
	http := fromOCGatewayListenerSpec(e.Http)
	https := fromOCGatewayListenerSpec(e.Https)
	tls := fromOCGatewayListenerSpec(e.Tls)
	if http == nil && https == nil && tls == nil {
		return nil
	}
	return &models.GatewayEndpointSpec{HTTP: http, HTTPS: https, TLS: tls}
}

func fromOCGatewayListenerSpec(l *ocapi.GatewayListenerSpec) *models.GatewayListenerSpec {
	if l == nil {
		return nil
	}
	return &models.GatewayListenerSpec{Port: l.Port, Host: l.Host}
}
