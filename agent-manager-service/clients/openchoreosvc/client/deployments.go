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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

func (c *openChoreoClient) Deploy(ctx context.Context, orgName, projectName, componentName string, req DeployRequest) error {
	// List workloads to find the one for this component
	workloadResp, err := c.ocClient.ListWorkloadsWithResponse(ctx, orgName, &gen.ListWorkloadsParams{
		Component: &componentName,
	})
	if err != nil {
		return fmt.Errorf("failed to list workloads: %w", err)
	}

	if workloadResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(workloadResp.StatusCode(), ErrorResponses{
			JSON401: workloadResp.JSON401,
			JSON403: workloadResp.JSON403,
			JSON404: workloadResp.JSON404,
			JSON500: workloadResp.JSON500,
		})
	}

	if workloadResp.JSON200 == nil || len(workloadResp.JSON200.Items) == 0 {
		return fmt.Errorf("no workload found for component")
	}

	workload := workloadResp.JSON200.Items[0]
	workloadName := workload.Metadata.Name

	// Update the container image and environment variables
	if workload.Spec == nil {
		workload.Spec = &gen.WorkloadSpec{}
	}
	if workload.Spec.Container == nil {
		workload.Spec.Container = &gen.WorkloadContainer{}
	}

	// Update image
	workload.Spec.Container.Image = req.ImageID

	// Update environment variables if provided (nil means no change, empty slice means clear all)
	if req.Env != nil {
		var envVars []gen.EnvVar
		for _, env := range req.Env {
			genEnvVar := gen.EnvVar{
				Key: env.Key,
			}
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
				// Secret env var - use ValueFrom with SecretRef
				secretName := env.ValueFrom.SecretKeyRef.Name
				secretKey := env.ValueFrom.SecretKeyRef.Key
				genEnvVar.ValueFrom = &gen.EnvVarValueFrom{
					SecretRef: &struct {
						Key  *string `json:"key,omitempty"`
						Name *string `json:"name,omitempty"`
					}{
						Name: &secretName,
						Key:  &secretKey,
					},
				}
			} else {
				// Plain env var - use Value directly
				value := env.Value
				genEnvVar.Value = &value
			}
			envVars = append(envVars, genEnvVar)
		}
		workload.Spec.Container.Env = &envVars
	}

	// Update workload
	updateResp, err := c.ocClient.UpdateWorkloadWithResponse(ctx, orgName, workloadName, workload)
	if err != nil {
		return fmt.Errorf("failed to update workload: %w", err)
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

func (c *openChoreoClient) GetDeployments(ctx context.Context, orgName, pipelineName, projectName, componentName string) ([]*models.DeploymentResponse, error) {
	// Get the deployment pipeline for environment ordering
	pipeline, err := c.GetProjectDeploymentPipeline(ctx, orgName, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment pipeline: %w", err)
	}

	// Get all environments for display names
	environments, err := c.ListEnvironments(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	// Create environment order based on the deployment pipeline
	environmentOrder := buildEnvironmentOrder(pipeline.PromotionPaths)

	// Get release bindings for the component
	bindingsResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, orgName, &gen.ListReleaseBindingsParams{
		Component: &componentName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list release bindings: %w", err)
	}

	if bindingsResp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(bindingsResp.StatusCode(), ErrorResponses{
			JSON401: bindingsResp.JSON401,
			JSON403: bindingsResp.JSON403,
			JSON404: bindingsResp.JSON404,
			JSON500: bindingsResp.JSON500,
		})
	}

	// Create a map of release bindings by environment for quick lookup
	releaseBindingMap := make(map[string]*gen.ReleaseBinding)
	if bindingsResp.JSON200 != nil {
		for i := range bindingsResp.JSON200.Items {
			binding := &bindingsResp.JSON200.Items[i]
			if binding.Spec != nil {
				releaseBindingMap[binding.Spec.Environment] = binding
			}
		}
	}

	// Create environment map for quick lookup
	environmentMap := make(map[string]*models.EnvironmentResponse)
	for _, env := range environments {
		environmentMap[env.Name] = env
	}

	// Fetch workload to get endpoint visibility and schema info
	workloadEndpoints := make(map[string]*gen.WorkloadEndpoint)
	workloadResp, err := c.ocClient.ListWorkloadsWithResponse(ctx, orgName, &gen.ListWorkloadsParams{
		Component: &componentName,
	})
	if err == nil && workloadResp.StatusCode() == http.StatusOK && workloadResp.JSON200 != nil && len(workloadResp.JSON200.Items) > 0 {
		workload := workloadResp.JSON200.Items[0]
		if workload.Spec != nil && workload.Spec.Endpoints != nil {
			for name, ep := range *workload.Spec.Endpoints {
				epCopy := ep
				workloadEndpoints[name] = &epCopy
			}
		}
	}

	// Construct deployment details in the order defined by the pipeline
	var deploymentDetails []*models.DeploymentResponse
	for _, envName := range environmentOrder {
		// Find promotion target environment for this environment
		promotionTargetEnv := findPromotionTargetEnvironment(envName, pipeline.PromotionPaths, environmentMap)

		if releaseBinding, exists := releaseBindingMap[envName]; exists {
			// Get release for image - use ListReleasesWithResponse with environment and component filters
			releaseResp, err := c.ocClient.ListReleasesWithResponse(ctx, orgName, &gen.ListReleasesParams{
				Component:   &componentName,
				Environment: &envName,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to get release for environment %s: %w", envName, err)
			}
			if releaseResp.StatusCode() != http.StatusOK {
				return nil, handleErrorResponse(releaseResp.StatusCode(), ErrorResponses{
					JSON401: releaseResp.JSON401,
					JSON403: releaseResp.JSON403,
					JSON404: releaseResp.JSON404,
					JSON500: releaseResp.JSON500,
				})
			}

			var release *gen.Release
			if releaseResp.JSON200 != nil && len(releaseResp.JSON200.Items) > 0 {
				release = &releaseResp.JSON200.Items[0]
			}

			deploymentDetail, err := toDeploymentDetailsResponse(releaseBinding, release, environmentMap, promotionTargetEnv, workloadEndpoints)
			if err != nil {
				return nil, fmt.Errorf("failed to build deployment details for environment %s: %w", envName, err)
			}
			deploymentDetails = append(deploymentDetails, deploymentDetail)
		} else {
			var displayName string
			if env, envExists := environmentMap[envName]; envExists {
				displayName = env.DisplayName
			}

			deploymentDetails = append(deploymentDetails, &models.DeploymentResponse{
				Environment:                envName,
				EnvironmentDisplayName:     displayName,
				PromotionTargetEnvironment: promotionTargetEnv,
				Status:                     DeploymentStatusNotDeployed,
				Endpoints:                  []models.Endpoint{},
			})
		}
	}

	return deploymentDetails, nil
}

// buildEnvironmentOrder creates an ordered list of environments based on promotion paths
func buildEnvironmentOrder(promotionPaths []models.PromotionPath) []string {
	if len(promotionPaths) == 0 {
		return []string{}
	}

	var order []string
	visited := make(map[string]bool)

	// Start with source environments
	for _, path := range promotionPaths {
		if !visited[path.SourceEnvironmentRef] {
			order = append(order, path.SourceEnvironmentRef)
			visited[path.SourceEnvironmentRef] = true
		}

		// Add target environments
		for _, target := range path.TargetEnvironmentRefs {
			if !visited[target.Name] {
				order = append(order, target.Name)
				visited[target.Name] = true
			}
		}
	}

	return order
}

// determineDeploymentStatus determines deployment status from release binding conditions
func determineDeploymentStatus(binding *gen.ReleaseBinding) string {
	if binding == nil {
		return DeploymentStatusNotDeployed
	}

	// Check if the binding state is set to Undeploy (suspended)
	if binding.Spec != nil && binding.Spec.State != nil && *binding.Spec.State == gen.ReleaseBindingSpecStateUndeploy {
		return DeploymentStatusSuspended
	}

	if binding.Status == nil || binding.Status.Conditions == nil {
		return DeploymentStatusNotDeployed
	}

	// Check conditions for status
	for _, condition := range *binding.Status.Conditions {
		// Look for "Ready" condition
		if condition.Type == "Ready" {
			switch condition.Status {
			case "True":
				return DeploymentStatusActive
			case "False":
				// Check reason for more specific status
				switch condition.Reason {
				case "Progressing", "Pending", "ResourcesProgressing":
					return DeploymentStatusInProgress
				case "Failed", "Error":
					return DeploymentStatusFailed
				}
				return DeploymentStatusFailed
			}
		}
	}

	return DeploymentStatusInProgress
}

func findPromotionTargetEnvironment(sourceEnvName string, promotionPaths []models.PromotionPath, environmentMap map[string]*models.EnvironmentResponse) *models.PromotionTargetEnvironment {
	for _, path := range promotionPaths {
		if path.SourceEnvironmentRef != sourceEnvName {
			continue
		}

		// Since promotion is linear, take the first (and only) target
		if len(path.TargetEnvironmentRefs) == 0 {
			return nil
		}

		targetEnvName := path.TargetEnvironmentRefs[0].Name
		var targetDisplayName string
		if env, exists := environmentMap[targetEnvName]; exists {
			targetDisplayName = env.DisplayName
		}
		return &models.PromotionTargetEnvironment{
			Name:        targetEnvName,
			DisplayName: targetDisplayName,
		}
	}
	return nil
}

func toDeploymentDetailsResponse(binding *gen.ReleaseBinding, release *gen.Release, environmentMap map[string]*models.EnvironmentResponse, promotionTargetEnv *models.PromotionTargetEnvironment, workloadEndpoints map[string]*gen.WorkloadEndpoint) (*models.DeploymentResponse, error) {
	if binding == nil || binding.Spec == nil {
		return nil, fmt.Errorf("release binding is nil or has no spec")
	}

	status := determineDeploymentStatus(binding)

	// Extract endpoints from release binding status, enriched with workload endpoint info
	endpoints := extractEndpointsFromBinding(binding, workloadEndpoints)

	deployedImage := findDeployedImageFromEnvRelease(release)

	environment := binding.Spec.Environment
	var environmentDisplayName string
	if env, exists := environmentMap[environment]; exists {
		environmentDisplayName = env.DisplayName
	}

	// Use the Ready condition's LastTransitionTime for accurate last deployed time,
	// falling back to CreationTimestamp if no Ready condition is found
	lastDeployedAt := getLastDeployedTime(binding)

	return &models.DeploymentResponse{
		ImageId:                    deployedImage,
		Status:                     status,
		Environment:                environment,
		EnvironmentDisplayName:     environmentDisplayName,
		PromotionTargetEnvironment: promotionTargetEnv,
		LastDeployedAt:             lastDeployedAt,
		Endpoints:                  endpoints,
	}, nil
}

// getLastDeployedTime extracts the most accurate last deployed time from a ReleaseBinding.
// It looks for the Ready condition's LastTransitionTime, falling back to CreationTimestamp.
func getLastDeployedTime(binding *gen.ReleaseBinding) time.Time {
	// Try to get LastTransitionTime from the Ready condition
	if binding.Status != nil && binding.Status.Conditions != nil {
		for _, condition := range *binding.Status.Conditions {
			if condition.Type == "Ready" {
				return condition.LastTransitionTime
			}
		}
	}

	// Fall back to CreationTimestamp if no Ready condition found
	if binding.Metadata.CreationTimestamp != nil {
		return *binding.Metadata.CreationTimestamp
	}

	return time.Time{}
}

// extractEndpointsFromBinding extracts endpoint URLs from the release binding status
// and enriches them with visibility and schema info from workload endpoints
func extractEndpointsFromBinding(binding *gen.ReleaseBinding, workloadEndpoints map[string]*gen.WorkloadEndpoint) []models.Endpoint {
	if binding == nil || binding.Status == nil || binding.Status.Endpoints == nil {
		return []models.Endpoint{}
	}

	endpoints := make([]models.Endpoint, 0, len(*binding.Status.Endpoints))
	for _, ep := range *binding.Status.Endpoints {
		endpoint := models.Endpoint{
			Name: ep.Name,
			URL:  ep.InvokeURL,
		}

		// Enrich with visibility from workload endpoint
		if workloadEp, exists := workloadEndpoints[ep.Name]; exists {
			if workloadEp.Visibility != nil && len(*workloadEp.Visibility) > 0 {
				endpoint.Visibility = string((*workloadEp.Visibility)[0])
			}
		}

		endpoints = append(endpoints, endpoint)
	}
	return endpoints
}

// UpdateDeploymentState updates the state of a deployment (Active or Undeploy)
func (c *openChoreoClient) UpdateDeploymentState(ctx context.Context, namespaceName, projectName, componentName, environment string, state gen.ReleaseBindingSpecState) error {
	// List release bindings for the component
	bindingsResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentName,
	})
	if err != nil {
		return fmt.Errorf("failed to list release bindings: %w", err)
	}

	if bindingsResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(bindingsResp.StatusCode(), ErrorResponses{
			JSON401: bindingsResp.JSON401,
			JSON403: bindingsResp.JSON403,
			JSON404: bindingsResp.JSON404,
			JSON500: bindingsResp.JSON500,
		})
	}

	// Find the binding for the specified environment
	var targetBinding *gen.ReleaseBinding
	if bindingsResp.JSON200 != nil {
		for i := range bindingsResp.JSON200.Items {
			binding := &bindingsResp.JSON200.Items[i]
			if binding.Spec != nil && binding.Spec.Environment == environment {
				targetBinding = binding
				break
			}
		}
	}

	if targetBinding == nil {
		return fmt.Errorf("no release binding found for environment %s: %w", environment, utils.ErrNotFound)
	}

	// Update the state
	targetBinding.Spec.State = &state

	// Update the release binding
	bindingName := targetBinding.Metadata.Name
	updateResp, err := c.ocClient.UpdateReleaseBindingWithResponse(ctx, namespaceName, bindingName, *targetBinding)
	if err != nil {
		return fmt.Errorf("failed to update release binding: %w", err)
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

// findDeployedImageFromEnvRelease extracts the deployed image from the Deployment resource in the release
func findDeployedImageFromEnvRelease(release *gen.Release) string {
	if release == nil || release.Spec == nil || release.Spec.Resources == nil {
		return ""
	}

	for _, resource := range *release.Spec.Resources {
		obj := resource.Object
		if len(obj) == 0 {
			continue
		}

		kind, _ := obj["kind"].(string)
		if kind != ResourceKindDeployment {
			continue
		}

		containers, found, err := unstructured.NestedSlice(obj, "spec", "template", "spec", "containers")
		if err != nil || !found {
			continue
		}

		for _, container := range containers {
			containerMap, ok := container.(map[string]interface{})
			if !ok {
				continue
			}
			if name, ok := containerMap["name"].(string); ok && name == MainContainerName {
				if image, ok := containerMap["image"].(string); ok {
					return image
				}
			}
		}
	}

	return ""
}
