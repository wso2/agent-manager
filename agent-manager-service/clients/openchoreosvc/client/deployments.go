//
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
//

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
	"encoding/json"
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

func (c *openChoreoClient) Deploy(ctx context.Context, orgName, projectName, componentName string, req DeployRequest) error {
	workloadResp, err := c.ocClient.GetWorkloadsWithResponse(ctx, orgName, projectName, componentName)
	if err != nil {
		return fmt.Errorf("failed to get workload: %w", err)
	}

	if workloadResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(workloadResp.StatusCode(), workloadResp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
		})
	}

	if workloadResp.JSON200 == nil || workloadResp.JSON200.Data == nil {
		return fmt.Errorf("empty workload response")
	}

	// Convert workload response to map to preserve all fields
	workloadBytes, err := json.Marshal(workloadResp.JSON200.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal workload: %w", err)
	}

	var workloadBody gen.CreateWorkloadJSONRequestBody
	if err := json.Unmarshal(workloadBytes, &workloadBody); err != nil {
		return fmt.Errorf("failed to unmarshal workload: %w", err)
	}

	// Build environment variables
	var envVars []gen.EnvVar
	for _, env := range req.Env {
		value := env.Value
		envVars = append(envVars, gen.EnvVar{
			Key:   env.Key,
			Value: &value,
		})
	}

	// Update only the containers with new image and env
	workloadBody["containers"] = map[string]gen.Container{
		MainContainerName: {
			Image: req.ImageID,
			Env:   &envVars,
		},
	}

	// Update workload
	createResp, err := c.ocClient.CreateWorkloadWithResponse(ctx, orgName, projectName, componentName, workloadBody)
	if err != nil {
		return fmt.Errorf("failed to update workload: %w", err)
	}

	if createResp.StatusCode() != http.StatusOK && createResp.StatusCode() != http.StatusCreated {
		return handleErrorResponse(createResp.StatusCode(), createResp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
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
	bindingsResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, orgName, projectName, componentName)
	if err != nil {
		return nil, fmt.Errorf("failed to list release bindings: %w", err)
	}

	if bindingsResp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(bindingsResp.StatusCode(), bindingsResp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
		})
	}

	// Create a map of release bindings by environment for quick lookup
	releaseBindingMap := make(map[string]*gen.ReleaseBindingResponse)
	if bindingsResp.JSON200 != nil && bindingsResp.JSON200.Data != nil && bindingsResp.JSON200.Data.Items != nil {
		for i := range *bindingsResp.JSON200.Data.Items {
			binding := &(*bindingsResp.JSON200.Data.Items)[i]
			releaseBindingMap[binding.Environment] = binding
		}
	}

	// Create environment map for quick lookup
	environmentMap := make(map[string]*models.EnvironmentResponse)
	for _, env := range environments {
		environmentMap[env.Name] = env
	}

	// Construct deployment details in the order defined by the pipeline
	var deploymentDetails []*models.DeploymentResponse
	for _, envName := range environmentOrder {
		// Find promotion target environment for this environment
		promotionTargetEnv := findPromotionTargetEnvironment(envName, pipeline.PromotionPaths, environmentMap)

		if releaseBinding, exists := releaseBindingMap[envName]; exists {
			// Get release for endpoints and image
			releaseResp, err := c.ocClient.GetEnvironmentReleaseWithResponse(ctx, orgName, projectName, componentName, envName)
			if err != nil {
				return nil, fmt.Errorf("failed to get release for environment %s: %w", envName, err)
			}

			var release *gen.ReleaseResponse
			if releaseResp.StatusCode() == http.StatusOK && releaseResp.JSON200 != nil {
				release = releaseResp.JSON200.Data
			}

			deploymentDetail, err := toDeploymentDetailsResponse(releaseBinding, release, environmentMap, promotionTargetEnv)
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

// determineDeploymentStatus determines deployment status from release binding
func determineDeploymentStatus(binding *gen.ReleaseBindingResponse) string {
	if binding == nil || binding.Status == nil {
		return DeploymentStatusNotDeployed
	}

	status := *binding.Status
	switch status {
	case BindingStatusReady, BindingStatusActive:
		return DeploymentStatusActive
	case BindingStatusFailed, BindingStatusError:
		return DeploymentStatusFailed
	case BindingStatusProgressing, BindingStatusPending:
		return DeploymentStatusInProgress
	default:
		return DeploymentStatusInProgress
	}
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

func toDeploymentDetailsResponse(binding *gen.ReleaseBindingResponse, release *gen.ReleaseResponse, environmentMap map[string]*models.EnvironmentResponse, promotionTargetEnv *models.PromotionTargetEnvironment) (*models.DeploymentResponse, error) {
	if binding == nil {
		return nil, fmt.Errorf("release binding is nil")
	}

	status := determineDeploymentStatus(binding)

	endpoints, err := extractEndpointURLFromEnvRelease(release)
	if err != nil {
		return nil, fmt.Errorf("error extracting endpoints: %w", err)
	}

	deployedImage := findDeployedImageFromEnvRelease(release)

	environment := binding.Environment
	var environmentDisplayName string
	if env, exists := environmentMap[environment]; exists {
		environmentDisplayName = env.DisplayName
	}

	return &models.DeploymentResponse{
		ImageId:                    deployedImage,
		Status:                     status,
		Environment:                environment,
		EnvironmentDisplayName:     environmentDisplayName,
		PromotionTargetEnvironment: promotionTargetEnv,
		LastDeployedAt:             binding.CreatedAt,
		Endpoints:                  endpoints,
	}, nil
}

// extractEndpointURLFromEnvRelease extracts endpoint URLs from HTTPRoute resources in the release
func extractEndpointURLFromEnvRelease(release *gen.ReleaseResponse) ([]models.Endpoint, error) {
	var endpoints []models.Endpoint

	if release == nil || release.Spec.Resources == nil {
		return endpoints, nil
	}

	for _, resource := range *release.Spec.Resources {
		obj := resource.Object
		if len(obj) == 0 {
			continue
		}

		kind, _ := obj["kind"].(string)
		if kind != ResourceKindHTTPRoute {
			continue
		}

		hostnames, found, err := unstructured.NestedStringSlice(obj, "spec", "hostnames")
		if err != nil {
			return nil, fmt.Errorf("error extracting hostnames from HTTPRoute: %w", err)
		}
		if !found || len(hostnames) == 0 {
			return nil, fmt.Errorf("HTTPRoute missing hostnames")
		}
		hostname := hostnames[0]

		pathValue, err := extractPathValue(obj)
		if err != nil {
			return nil, fmt.Errorf("error extracting path from HTTPRoute: %w", err)
		}

		port := config.GetConfig().DefaultGatewayPort
		url := fmt.Sprintf("http://%s:%d", hostname, port)
		if pathValue != "" {
			url = fmt.Sprintf("http://%s:%d%s", hostname, port, pathValue)
		}

		endpoints = append(endpoints, models.Endpoint{
			URL:        url,
			Visibility: EndpointVisibilityPublic,
		})
	}

	return endpoints, nil
}

func extractPathValue(obj map[string]interface{}) (string, error) {
	rules, found, err := unstructured.NestedSlice(obj, "spec", "rules")
	if err != nil || !found || len(rules) == 0 {
		return "", fmt.Errorf("HTTPRoute missing rules")
	}

	rule0, ok := rules[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid rule format in HTTPRoute")
	}

	matches, found, err := unstructured.NestedSlice(rule0, "matches")
	if err != nil || !found || len(matches) == 0 {
		return "", fmt.Errorf("HTTPRoute missing matches")
	}

	match0, ok := matches[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid match format in HTTPRoute")
	}

	pathValue, found, err := unstructured.NestedString(match0, "path", "value")
	if err != nil || !found {
		return "", fmt.Errorf("HTTPRoute missing path value")
	}
	return pathValue, nil
}

// findDeployedImageFromEnvRelease extracts the deployed image from the Deployment resource in the release
func findDeployedImageFromEnvRelease(release *gen.ReleaseResponse) string {
	if release == nil || release.Spec.Resources == nil {
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
