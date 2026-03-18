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

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

func (c *openChoreoClient) CreateComponent(ctx context.Context, namespaceName, projectName string, req CreateComponentRequest) error {
	createComponentReqBody, err := buildCreateComponentRequestBody(namespaceName, projectName, req)
	if err != nil {
		return fmt.Errorf("failed to build component request: %w", err)
	}

	resp, err := c.ocClient.CreateComponentWithResponse(ctx, namespaceName, createComponentReqBody)
	if err != nil {
		return fmt.Errorf("failed to create component: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated {
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

func buildCreateComponentRequestBody(namespaceName, projectName string, req CreateComponentRequest) (gen.CreateComponentJSONRequestBody, error) {
	if req.ProvisioningType == ProvisioningExternal {
		return buildExternalAgentComponentRequestBody(namespaceName, projectName, req)
	}
	return buildInternalAgentComponentRequestBody(namespaceName, projectName, req)
}

func buildExternalAgentComponentRequestBody(namespaceName, projectName string, req CreateComponentRequest) (gen.CreateComponentJSONRequestBody, error) {
	annotations := map[string]string{
		string(AnnotationKeyDisplayName): req.DisplayName,
		string(AnnotationKeyDescription): req.Description,
	}
	labels := map[string]string{
		string(LabelKeyProvisioningType): string(req.ProvisioningType),
	}
	componentTypeKind := gen.ComponentSpecComponentTypeKindClusterComponentType
	componentType, err := getOpenChoreoComponentType(string(req.ProvisioningType), req.AgentType.Type)
	if err != nil {
		return gen.CreateComponentJSONRequestBody{}, err
	}

	return gen.CreateComponentJSONRequestBody{
		Metadata: gen.ObjectMeta{
			Name:        req.Name,
			Namespace:   &namespaceName,
			Annotations: &annotations,
			Labels:      &labels,
		},
		Spec: &gen.ComponentSpec{
			ComponentType: struct {
				Kind *gen.ComponentSpecComponentTypeKind `json:"kind,omitempty"`
				Name string                              `json:"name"`
			}{
				Kind: &componentTypeKind,
				Name: componentType,
			},
			Owner: struct {
				ProjectName string `json:"projectName"`
			}{
				ProjectName: projectName,
			},
		},
	}, nil
}

func buildInternalAgentComponentRequestBody(namespaceName, projectName string, req CreateComponentRequest) (gen.CreateComponentJSONRequestBody, error) {
	annotations := map[string]string{
		string(AnnotationKeyDisplayName): req.DisplayName,
		string(AnnotationKeyDescription): req.Description,
	}
	labels := map[string]string{
		string(LabelKeyProvisioningType): string(req.ProvisioningType),
		string(LabelKeyAgentSubType):     req.AgentType.SubType,
	}
	componentTypeKind := gen.ComponentSpecComponentTypeKindClusterComponentType
	componentType, err := getOpenChoreoComponentType(string(req.ProvisioningType), req.AgentType.Type)
	if err != nil {
		return gen.CreateComponentJSONRequestBody{}, err
	}
	componentWorkflowName, err := getWorkflowName(req.Build)
	if err != nil {
		return gen.CreateComponentJSONRequestBody{}, fmt.Errorf("failed to determine workflow name: %w", err)
	}

	// Create default parameters
	defaultParams := ComponentParameters{
		Exposed: true,
	}

	// Convert struct to map for OpenChoreo API
	parameters, err := structToMap(defaultParams)
	if err != nil {
		return gen.CreateComponentJSONRequestBody{}, fmt.Errorf("failed to convert parameters to map: %w", err)
	}

	componentWorkflowParameters, err := buildWorkflowParameters(req)
	if err != nil {
		return gen.CreateComponentJSONRequestBody{}, fmt.Errorf("error building workflow parameters: %w", err)
	}

	autoDeploy := true
	return gen.CreateComponentJSONRequestBody{
		Metadata: gen.ObjectMeta{
			Name:        req.Name,
			Namespace:   &namespaceName,
			Annotations: &annotations,
			Labels:      &labels,
		},
		Spec: &gen.ComponentSpec{
			ComponentType: struct {
				Kind *gen.ComponentSpecComponentTypeKind `json:"kind,omitempty"`
				Name string                              `json:"name"`
			}{
				Kind: &componentTypeKind,
				Name: componentType,
			},
			Owner: struct {
				ProjectName string `json:"projectName"`
			}{
				ProjectName: projectName,
			},
			AutoDeploy: &autoDeploy,
			Parameters: &parameters,
			Workflow: &gen.ComponentWorkflowConfig{
				Name:       componentWorkflowName,
				Parameters: &componentWorkflowParameters,
			},
		},
	}, nil
}

func getOpenChoreoComponentType(provisioningType string, agentType string) (string, error) {
	if provisioningType == string(utils.ExternalAgent) {
		return string(ComponentTypeExternalAgentAPI), nil
	}
	if provisioningType == string(utils.InternalAgent) && agentType == string(utils.AgentTypeAPI) {
		return string(ComponentTypeInternalAgentAPI), nil
	}
	// agent type is already validated in controller layer
	return "", fmt.Errorf("invalid provisioning type or agent type")
}

// -----------------------------------------------------------------------------
// Workflow parameter builders
// -----------------------------------------------------------------------------

func getWorkflowName(build *BuildConfig) (string, error) {
	if build == nil {
		return "", fmt.Errorf("build configuration is required")
	}

	// Check build type first
	if build.Type == BuildTypeDocker && build.Docker != nil {
		return WorkflowNameDocker, nil
	}

	// For buildpack, determine workflow based on language
	if build.Type == BuildTypeBuildpack && build.Buildpack != nil {
		language := build.Buildpack.Language
		for _, bp := range utils.Buildpacks {
			if bp.Language == language {
				if bp.Provider == string(utils.BuildPackProviderGoogle) {
					return WorkflowNameGoogleCloudBuildpacks, nil
				}
				if bp.Provider == string(utils.BuildPackProviderAMPBallerina) {
					return WorkflowNameBallerinaBuilpack, nil
				}
			}
		}
		return "", fmt.Errorf("unsupported buildpack language: %s", language)
	}

	return "", fmt.Errorf("invalid build configuration: unsupported build type '%s'", build.Type)
}

func buildWorkflowParameters(req CreateComponentRequest) (map[string]any, error) {
	params := map[string]any{
		"environmentVariables": buildEnvironmentVariables(req),
	}

	// Add repository details in nested format expected by ClusterWorkflow
	if req.Repository != nil {
		params["repository"] = map[string]any{
			"url":     req.Repository.URL,
			"appPath": req.Repository.AppPath,
			"revision": map[string]any{
				"branch": req.Repository.Branch,
			},
		}
	}

	// Add build-specific configs
	if req.Build != nil {
		if req.Build.Buildpack != nil {
			// Add buildpack configs
			var buildpackConfigs map[string]any
			if isGoogleBuildpack(req.Build.Buildpack.Language) {
				buildpackConfigs = map[string]any{
					"language":           req.Build.Buildpack.Language,
					"languageVersion":    req.Build.Buildpack.LanguageVersion,
					"googleEntryPoint":   req.Build.Buildpack.RunCommand,
					"languageVersionKey": getLanguageVersionEnvVariable(req.Build.Buildpack.Language),
				}
			} else {
				buildpackConfigs = map[string]any{
					"language": req.Build.Buildpack.Language,
				}
			}
			params["buildpackConfigs"] = buildpackConfigs
		} else if req.Build.Docker != nil {
			// Add docker configs
			dockerConfigs := map[string]any{
				"dockerfilePath": normalizePath(req.Build.Docker.DockerfilePath),
			}
			params["dockerConfigs"] = dockerConfigs
		}
	}

	// Add endpoints
	endpoints, err := buildEndpoints(req)
	if err != nil {
		return nil, err
	}
	params["endpoints"] = endpoints

	return params, nil
}

func isGoogleBuildpack(language string) bool {
	for _, bp := range utils.Buildpacks {
		if bp.Language == language && bp.Provider == string(utils.BuildPackProviderGoogle) {
			return true
		}
	}
	return false
}

func getLanguageVersionEnvVariable(language string) string {
	for _, bp := range utils.Buildpacks {
		if bp.Language == language {
			return bp.VersionEnvVariable
		}
	}
	return ""
}

// DefaultEndpointVisibility is the default visibility for endpoints
var DefaultEndpointVisibility = []string{string(gen.WorkloadEndpointVisibilityExternal)}

func buildEndpoints(req CreateComponentRequest) ([]map[string]any, error) {
	endpoints := make([]map[string]any, 0)

	if req.AgentType.Type == string(utils.AgentTypeAPI) && req.AgentType.SubType == string(utils.AgentSubTypeChatAPI) {
		schemaContent, err := getDefaultChatAPISchema()
		if err != nil {
			return nil, fmt.Errorf("failed to read Chat API schema: %w", err)
		}
		endpoints = append(endpoints, map[string]any{
			"name":          fmt.Sprintf("%s-endpoint", req.Name),
			"port":          config.GetConfig().DefaultChatAPI.DefaultHTTPPort,
			"type":          string(utils.InputInterfaceTypeHTTP),
			"basePath":      req.InputInterface.BasePath,
			"visibility":    DefaultEndpointVisibility,
			"schemaType":    SchemaTypeREST,
			"schemaContent": schemaContent,
		})
	}

	if req.AgentType.Type == string(utils.AgentTypeAPI) && req.AgentType.SubType == string(utils.AgentSubTypeCustomAPI) && req.InputInterface != nil {
		endpoints = append(endpoints, map[string]any{
			"name":           fmt.Sprintf("%s-endpoint", req.Name),
			"port":           req.InputInterface.Port,
			"type":           req.InputInterface.Type,
			"basePath":       req.InputInterface.BasePath,
			"visibility":     DefaultEndpointVisibility,
			"schemaType":     "REST",
			"schemaFilePath": normalizePath(req.InputInterface.SchemaPath),
		})
	}

	return endpoints, nil
}

func buildEnvironmentVariables(req CreateComponentRequest) []map[string]any {
	envVars := make([]map[string]any, 0)
	if req.Configurations != nil {
		for _, env := range req.Configurations.Env {
			envVar := map[string]any{
				"name": env.Key,
			}
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
				// Secret reference - use valueFrom pattern
				envVar["valueFrom"] = map[string]any{
					"secretKeyRef": map[string]any{
						"name": env.ValueFrom.SecretKeyRef.Name,
						"key":  env.ValueFrom.SecretKeyRef.Key,
					},
				}
			} else {
				// Plain value
				envVar["value"] = env.Value
			}
			envVars = append(envVars, envVar)
		}
	}
	return envVars
}

func normalizePath(path string) string {
	path = strings.TrimSuffix(path, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func (c *openChoreoClient) GetComponent(ctx context.Context, namespaceName, projectName, componentName string) (*models.AgentResponse, error) {
	resp, err := c.ocClient.GetComponentWithResponse(ctx, namespaceName, componentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get component resource: %w", err)
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
		return nil, fmt.Errorf("empty response from get component")
	}

	return convertComponentFromTyped(resp.JSON200)
}

func (c *openChoreoClient) UpdateComponentBasicInfo(ctx context.Context, namespaceName, projectName, componentName string, req UpdateComponentBasicInfoRequest) error {
	resp, err := c.ocClient.GetComponentWithResponse(ctx, namespaceName, componentName)
	if err != nil {
		return fmt.Errorf("failed to get component: %w", err)
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
		return fmt.Errorf("empty response from get component")
	}

	component := resp.JSON200
	if component.Metadata.Annotations == nil {
		annotations := make(map[string]string)
		component.Metadata.Annotations = &annotations
	}
	(*component.Metadata.Annotations)[string(AnnotationKeyDisplayName)] = req.DisplayName
	(*component.Metadata.Annotations)[string(AnnotationKeyDescription)] = req.Description

	updateResp, err := c.ocClient.UpdateComponentWithResponse(ctx, namespaceName, componentName, *component)
	if err != nil {
		return fmt.Errorf("failed to update component: %w", err)
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

// UpdateEnvResourceConfigs updates environment-specific resource configurations via release binding
func (c *openChoreoClient) UpdateEnvResourceConfigs(ctx context.Context, namespaceName, projectName, componentName, environment string, req UpdateComponentResourceConfigsRequest) error {
	// List release bindings to find the correct binding name for the environment
	componentFilter := componentName
	listResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentFilter,
	})
	if err != nil {
		return fmt.Errorf("failed to list release bindings: %w", err)
	}
	if listResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(listResp.StatusCode(), ErrorResponses{
			JSON401: listResp.JSON401,
			JSON403: listResp.JSON403,
			JSON404: listResp.JSON404,
			JSON500: listResp.JSON500,
		})
	}
	if listResp.JSON200 == nil {
		return fmt.Errorf("empty response from list release bindings")
	}

	// Find the binding for the specified environment
	var bindingName string
	for _, binding := range listResp.JSON200.Items {
		if binding.Spec != nil && binding.Spec.Environment == environment {
			bindingName = binding.Metadata.Name
			break
		}
	}
	if bindingName == "" {
		return fmt.Errorf("release binding not found for environment: %s", environment)
	}

	// Get the release binding
	getResp, err := c.ocClient.GetReleaseBindingWithResponse(ctx, namespaceName, bindingName)
	if err != nil {
		return fmt.Errorf("failed to get release binding: %w", err)
	}
	if getResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(getResp.StatusCode(), ErrorResponses{
			JSON401: getResp.JSON401,
			JSON403: getResp.JSON403,
			JSON404: getResp.JSON404,
			JSON500: getResp.JSON500,
		})
	}
	if getResp.JSON200 == nil {
		return fmt.Errorf("empty response from get release binding")
	}

	releaseBinding := getResp.JSON200
	if releaseBinding.Spec == nil {
		return fmt.Errorf("release binding spec is nil")
	}

	// Get or create componentTypeEnvOverrides
	if releaseBinding.Spec.ComponentTypeEnvironmentConfigs == nil {
		overrides := make(map[string]interface{})
		releaseBinding.Spec.ComponentTypeEnvironmentConfigs = &overrides
	}
	componentTypeEnvOverrides := *releaseBinding.Spec.ComponentTypeEnvironmentConfigs

	// Add replicas if provided
	if req.Replicas != nil {
		componentTypeEnvOverrides["replicas"] = *req.Replicas
	}

	// Add resources if provided
	if req.Resources != nil {
		resourcesMap, err := structToMap(req.Resources)
		if err != nil {
			return fmt.Errorf("failed to convert resources to map: %w", err)
		}
		componentTypeEnvOverrides["resources"] = resourcesMap
	}

	// Add autoscaling to componentTypeEnvOverrides if provided
	if req.AutoScaling != nil {
		// Check if autoscaling already exists, otherwise create a new map
		var autoscaling map[string]interface{}
		if existing, ok := componentTypeEnvOverrides["autoscaling"].(map[string]interface{}); ok {
			autoscaling = existing
		} else {
			autoscaling = make(map[string]interface{})
		}

		if req.AutoScaling.Enabled != nil {
			autoscaling["enabled"] = *req.AutoScaling.Enabled
		}
		if req.AutoScaling.MinReplicas != nil {
			autoscaling["minReplicas"] = *req.AutoScaling.MinReplicas
		}
		if req.AutoScaling.MaxReplicas != nil {
			autoscaling["maxReplicas"] = *req.AutoScaling.MaxReplicas
		}
		if req.AutoScaling.TargetCPUUtilizationPercentage != nil {
			autoscaling["cpuUtilizationPercentage"] = *req.AutoScaling.TargetCPUUtilizationPercentage
		}

		// If autoscaling is enabled and MinReplicas is present, update replicas
		if req.AutoScaling.Enabled != nil && *req.AutoScaling.Enabled && req.AutoScaling.MinReplicas != nil {
			componentTypeEnvOverrides["replicas"] = *req.AutoScaling.MinReplicas
		}

		componentTypeEnvOverrides["autoscaling"] = autoscaling
	}

	// Update the release binding
	updateResp, err := c.ocClient.UpdateReleaseBindingWithResponse(ctx, namespaceName, bindingName, *releaseBinding)
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

// GetEnvResourceConfigs fetches environment-specific resource configurations from release binding
func (c *openChoreoClient) GetEnvResourceConfigs(ctx context.Context, namespaceName, projectName, componentName, environment string) (*ComponentResourceConfigsResponse, error) {
	// Step 1: Get component to find its ComponentType reference
	compResp, err := c.ocClient.GetComponentWithResponse(ctx, namespaceName, componentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}
	if compResp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(compResp.StatusCode(), ErrorResponses{
			JSON401: compResp.JSON401,
			JSON403: compResp.JSON403,
			JSON404: compResp.JSON404,
			JSON500: compResp.JSON500,
		})
	}
	if compResp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from get component")
	}

	// Step 2: Fetch ComponentType schema defaults
	response := c.getEnvConfigDefaultsFromComponentType(ctx, namespaceName, compResp.JSON200)

	// Step 2: Check ReleaseBinding for environment-specific overrides
	componentFilter := componentName
	listResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentFilter,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list release bindings: %w", err)
	}
	if listResp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(listResp.StatusCode(), ErrorResponses{
			JSON401: listResp.JSON401,
			JSON403: listResp.JSON403,
			JSON404: listResp.JSON404,
			JSON500: listResp.JSON500,
		})
	}

	// Find the binding for the specified environment
	var binding *gen.ReleaseBinding
	if listResp.JSON200 != nil {
		for i := range listResp.JSON200.Items {
			b := &listResp.JSON200.Items[i]
			if b.Spec != nil && b.Spec.Environment == environment {
				binding = b
				break
			}
		}
	}

	if binding == nil {
		// No binding found - return ComponentType defaults
		return response, nil
	}

	// Apply overrides from ReleaseBinding's componentTypeEnvOverrides
	if binding.Spec != nil && binding.Spec.ComponentTypeEnvironmentConfigs != nil {
		envOverrides, err := mapToEnvOverrideParameters(*binding.Spec.ComponentTypeEnvironmentConfigs)
		if err != nil {
			return nil, fmt.Errorf("failed to parse env overrides: %w", err)
		}

		// Apply replicas override
		if envOverrides.Replicas != nil {
			replicas := int32(*envOverrides.Replicas)
			response.Replicas = &replicas
		}

		// Apply resources override (merge with defaults)
		if envOverrides.Resources != nil {
			if envOverrides.Resources.Requests != nil {
				if envOverrides.Resources.Requests.CPU != "" {
					response.Resources.Requests.CPU = envOverrides.Resources.Requests.CPU
				}
				if envOverrides.Resources.Requests.Memory != "" {
					response.Resources.Requests.Memory = envOverrides.Resources.Requests.Memory
				}
			}
			if envOverrides.Resources.Limits != nil {
				if envOverrides.Resources.Limits.CPU != "" {
					response.Resources.Limits.CPU = envOverrides.Resources.Limits.CPU
				}
				if envOverrides.Resources.Limits.Memory != "" {
					response.Resources.Limits.Memory = envOverrides.Resources.Limits.Memory
				}
			}
		}

		// Apply autoscaling override from componentTypeEnvOverrides.autoscaling
		if envOverrides.Autoscaling != nil {
			if envOverrides.Autoscaling.Enabled != nil {
				response.AutoScaling.Enabled = envOverrides.Autoscaling.Enabled
			}
			if envOverrides.Autoscaling.MinReplicas != nil {
				response.AutoScaling.MinReplicas = envOverrides.Autoscaling.MinReplicas
			}
			if envOverrides.Autoscaling.MaxReplicas != nil {
				response.AutoScaling.MaxReplicas = envOverrides.Autoscaling.MaxReplicas
			}
			if envOverrides.Autoscaling.TargetCPUUtilizationPercentage != nil {
				response.AutoScaling.TargetCPUUtilizationPercentage = envOverrides.Autoscaling.TargetCPUUtilizationPercentage
			}
		}
	}

	return response, nil
}

// structToMap converts a struct to map[string]interface{} using JSON marshaling
func structToMap(v interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// mapToEnvOverrideParameters converts a map to EnvOverrideParameters using JSON marshaling
func mapToEnvOverrideParameters(m map[string]interface{}) (*EnvOverrideParameters, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var params EnvOverrideParameters
	if err := json.Unmarshal(data, &params); err != nil {
		return nil, err
	}
	return &params, nil
}

// getEnvConfigDefaultsFromComponentType fetches the ClusterComponentType and extracts defaults from its environmentConfigs schema
func (c *openChoreoClient) getEnvConfigDefaultsFromComponentType(ctx context.Context, namespaceName string, component *gen.Component) *ComponentResourceConfigsResponse {
	response := &ComponentResourceConfigsResponse{
		Resources: &ResourceConfig{
			Requests: &ResourceRequests{},
			Limits:   &ResourceLimits{},
		},
		AutoScaling: &AutoScalingConfig{},
	}

	if component == nil || component.Spec == nil {
		return response
	}

	// Get the ClusterComponentType name from component reference
	ctName := component.Spec.ComponentType.Name

	// Fetch ClusterComponentType
	ctResp, err := c.ocClient.GetClusterComponentTypeWithResponse(ctx, ctName)
	if err != nil || ctResp.StatusCode() != http.StatusOK || ctResp.JSON200 == nil {
		return response
	}

	if ctResp.JSON200.Spec == nil || ctResp.JSON200.Spec.EnvironmentConfigs == nil || ctResp.JSON200.Spec.EnvironmentConfigs.OpenAPIV3Schema == nil {
		return response
	}

	// Extract defaults from schema
	applySchemaDefaults(response, *ctResp.JSON200.Spec.EnvironmentConfigs.OpenAPIV3Schema)
	return response
}

// applySchemaDefaults extracts default values from OpenAPI V3 Schema and applies them to the response
func applySchemaDefaults(response *ComponentResourceConfigsResponse, schema map[string]interface{}) {
	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		return
	}

	// Get $defs for resolving references
	defs, _ := schema["$defs"].(map[string]interface{})

	// Extract replicas default
	if replicasProp, ok := properties["replicas"].(map[string]interface{}); ok {
		if defaultVal, ok := replicasProp["default"]; ok {
			if replicas, ok := toInt32(defaultVal); ok {
				response.Replicas = &replicas
			}
		}
	}

	// Extract resources defaults
	if resourcesProp, ok := properties["resources"].(map[string]interface{}); ok {
		applyResourceDefaults(response.Resources, resourcesProp, defs)
	}

	// Extract autoscaling defaults
	if autoscalingProp, ok := properties["autoscaling"].(map[string]interface{}); ok {
		applyAutoscalingDefaults(response.AutoScaling, autoscalingProp, defs)
	}
}

// applyResourceDefaults extracts resource defaults from schema
func applyResourceDefaults(resources *ResourceConfig, resourcesProp map[string]interface{}, defs map[string]interface{}) {
	resourceProps, ok := resourcesProp["properties"].(map[string]interface{})
	if !ok {
		return
	}

	// Extract requests defaults
	if requestsProp, ok := resourceProps["requests"].(map[string]interface{}); ok {
		requestsProp = resolveRef(requestsProp, defs)
		applyQuantityDefaults(resources.Requests, requestsProp, defs)
	}

	// Extract limits defaults
	if limitsProp, ok := resourceProps["limits"].(map[string]interface{}); ok {
		limitsProp = resolveRef(limitsProp, defs)
		applyQuantityDefaults(resources.Limits, limitsProp, defs)
	}
}

// applyQuantityDefaults extracts CPU/Memory defaults from schema
func applyQuantityDefaults(target interface{}, prop map[string]interface{}, defs map[string]interface{}) {
	props, ok := prop["properties"].(map[string]interface{})
	if !ok {
		return
	}

	switch t := target.(type) {
	case *ResourceRequests:
		if cpuProp, ok := props["cpu"].(map[string]interface{}); ok {
			if defaultVal, ok := cpuProp["default"].(string); ok && defaultVal != "" {
				t.CPU = defaultVal
			}
		}
		if memProp, ok := props["memory"].(map[string]interface{}); ok {
			if defaultVal, ok := memProp["default"].(string); ok && defaultVal != "" {
				t.Memory = defaultVal
			}
		}
	case *ResourceLimits:
		if cpuProp, ok := props["cpu"].(map[string]interface{}); ok {
			if defaultVal, ok := cpuProp["default"].(string); ok && defaultVal != "" {
				t.CPU = defaultVal
			}
		}
		if memProp, ok := props["memory"].(map[string]interface{}); ok {
			if defaultVal, ok := memProp["default"].(string); ok && defaultVal != "" {
				t.Memory = defaultVal
			}
		}
	}
}

// applyAutoscalingDefaults extracts autoscaling defaults from schema
func applyAutoscalingDefaults(autoscaling *AutoScalingConfig, prop map[string]interface{}, defs map[string]interface{}) {
	prop = resolveRef(prop, defs)
	props, ok := prop["properties"].(map[string]interface{})
	if !ok {
		return
	}

	if enabledProp, ok := props["enabled"].(map[string]interface{}); ok {
		if defaultVal, ok := enabledProp["default"].(bool); ok {
			autoscaling.Enabled = &defaultVal
		}
	}
	if minProp, ok := props["minReplicas"].(map[string]interface{}); ok {
		if defaultVal, ok := toInt32(minProp["default"]); ok {
			autoscaling.MinReplicas = &defaultVal
		}
	}
	if maxProp, ok := props["maxReplicas"].(map[string]interface{}); ok {
		if defaultVal, ok := toInt32(maxProp["default"]); ok {
			autoscaling.MaxReplicas = &defaultVal
		}
	}
	if targetCPUProp, ok := props["targetCPUUtilizationPercentage"].(map[string]interface{}); ok {
		if defaultVal, ok := toInt32(targetCPUProp["default"]); ok {
			autoscaling.TargetCPUUtilizationPercentage = &defaultVal
		}
	}
}

// resolveRef resolves $ref references in OpenAPI V3 Schema
func resolveRef(prop map[string]interface{}, defs map[string]interface{}) map[string]interface{} {
	ref, ok := prop["$ref"].(string)
	if !ok || defs == nil {
		return prop
	}

	// Parse reference like "#/$defs/ResourceQuantity"
	const prefix = "#/$defs/"
	if !strings.HasPrefix(ref, prefix) {
		return prop
	}

	defName := strings.TrimPrefix(ref, prefix)
	if resolved, ok := defs[defName].(map[string]interface{}); ok {
		return resolved
	}
	return prop
}

// toInt32 converts various numeric types to int32
func toInt32(v interface{}) (int32, bool) {
	switch val := v.(type) {
	case int:
		return int32(val), true
	case int32:
		return val, true
	case int64:
		return int32(val), true
	case float64:
		return int32(val), true
	case float32:
		return int32(val), true
	}
	return 0, false
}

func (c *openChoreoClient) DeleteComponent(ctx context.Context, namespaceName, projectName, componentName string) error {
	resp, err := c.ocClient.DeleteComponentWithResponse(ctx, namespaceName, componentName)
	if err != nil {
		return fmt.Errorf("failed to delete component: %w", err)
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

func (c *openChoreoClient) ListComponents(ctx context.Context, namespaceName, projectName string) ([]*models.AgentResponse, error) {
	resp, err := c.ocClient.ListComponentsWithResponse(ctx, namespaceName, &gen.ListComponentsParams{
		Project: &projectName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list components: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}
	if resp.JSON200 == nil || len(resp.JSON200.Items) == 0 {
		return []*models.AgentResponse{}, nil
	}

	components := make([]*models.AgentResponse, 0, len(resp.JSON200.Items))
	for i := range resp.JSON200.Items {
		comp, err := convertComponentFromTyped(&resp.JSON200.Items[i])
		if err != nil {
			slog.Error("failed to convert component", "component", resp.JSON200.Items[i].Metadata.Name, "error", err)
			continue
		}
		components = append(components, comp)
	}
	return components, nil
}

func (c *openChoreoClient) ComponentExists(ctx context.Context, namespaceName, projectName, componentName string, verifyProject bool) (bool, error) {
	_, err := c.GetComponent(ctx, namespaceName, projectName, componentName)
	if err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// listComponentTraits retrieves the current traits attached to a component
func (c *openChoreoClient) listComponentTraits(ctx context.Context, namespaceName, projectName, componentName string) ([]gen.ComponentTrait, error) {
	resp, err := c.ocClient.GetComponentWithResponse(ctx, namespaceName, componentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}
	if resp.JSON200 == nil || resp.JSON200.Spec == nil || resp.JSON200.Spec.Traits == nil {
		return []gen.ComponentTrait{}, nil
	}
	return *resp.JSON200.Spec.Traits, nil
}

// TraitRequest holds the parameters for a single trait to attach.
type TraitRequest struct {
	TraitType TraitType
	Opts      []TraitOption
}

// AttachTraits attaches one or more traits to a component in a single GET-UPDATE cycle.
func (c *openChoreoClient) AttachTraits(ctx context.Context, namespaceName, projectName, componentName string, traitRequests []TraitRequest) error {
	if len(traitRequests) == 0 {
		return nil
	}

	resp, err := c.ocClient.GetComponentWithResponse(ctx, namespaceName, componentName)
	if err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}
	if resp.JSON200 == nil || resp.JSON200.Spec == nil {
		return fmt.Errorf("invalid component response")
	}

	component := resp.JSON200
	var traits []gen.ComponentTrait
	if component.Spec.Traits != nil {
		traits = *component.Spec.Traits
	}

	existingTraits := make(map[string]bool, len(traits))
	for _, trait := range traits {
		existingTraits[trait.Name] = true
	}

	added := false
	for _, req := range traitRequests {
		if existingTraits[string(req.TraitType)] {
			continue
		}
		newTrait, err := c.buildTrait(ctx, namespaceName, projectName, componentName, req.TraitType, req.Opts...)
		if err != nil {
			return fmt.Errorf("failed to build trait %s: %w", req.TraitType, err)
		}
		traits = append(traits, newTrait)
		existingTraits[string(req.TraitType)] = true
		added = true
	}

	if !added {
		return nil
	}

	component.Spec.Traits = &traits

	updateResp, err := c.ocClient.UpdateComponentWithResponse(ctx, namespaceName, componentName, *component)
	if err != nil {
		return fmt.Errorf("failed to update component: %w", err)
	}
	if updateResp.StatusCode() != http.StatusOK {
		slog.Error("AttachTraits: UpdateComponent failed", "statusCode", updateResp.StatusCode())
		return handleErrorResponse(updateResp.StatusCode(), ErrorResponses{
			JSON401: updateResp.JSON401,
			JSON403: updateResp.JSON403,
			JSON404: updateResp.JSON404,
			JSON500: updateResp.JSON500,
		})
	}

	return nil
}

// DetachTrait removes a trait from a component
func (c *openChoreoClient) DetachTrait(ctx context.Context, namespaceName, projectName, componentName string, traitType TraitType) error {
	// Get the component
	resp, err := c.ocClient.GetComponentWithResponse(ctx, namespaceName, componentName)
	if err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}
	if resp.JSON200 == nil || resp.JSON200.Spec == nil {
		return fmt.Errorf("invalid component response")
	}

	component := resp.JSON200
	if component.Spec.Traits == nil {
		return nil // No traits to remove
	}

	// Build new traits list excluding the trait to detach
	var updatedTraits []gen.ComponentTrait
	traitFound := false
	for _, trait := range *component.Spec.Traits {
		if trait.Name == string(traitType) {
			traitFound = true
			continue
		}
		updatedTraits = append(updatedTraits, trait)
	}

	if !traitFound {
		return nil
	}

	component.Spec.Traits = &updatedTraits

	// Update component
	updateResp, err := c.ocClient.UpdateComponentWithResponse(ctx, namespaceName, componentName, *component)
	if err != nil {
		return fmt.Errorf("failed to update component: %w", err)
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

// HasTrait checks if a component has a specific trait attached
func (c *openChoreoClient) HasTrait(ctx context.Context, namespaceName, projectName, componentName string, traitType TraitType) (bool, error) {
	traits, err := c.listComponentTraits(ctx, namespaceName, projectName, componentName)
	if err != nil {
		return false, err
	}

	for _, trait := range traits {
		if trait.Name == string(traitType) {
			return true, nil
		}
	}

	return false, nil
}

// mergeComponentEnvVars merges the provided env vars into the component's workflow parameters
// and updates the Component CR. Shared by UpdateComponentEnvVars and UpdateComponentEnvironmentVariables.
func (c *openChoreoClient) mergeComponentEnvVars(ctx context.Context, namespaceName, componentName string, envVars []EnvVar) error {
	// Get the component
	resp, err := c.ocClient.GetComponentWithResponse(ctx, namespaceName, componentName)
	if err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}
	if resp.JSON200 == nil || resp.JSON200.Spec == nil {
		return fmt.Errorf("invalid component response")
	}

	component := resp.JSON200

	// Ensure workflow exists
	if component.Spec.Workflow == nil {
		component.Spec.Workflow = &gen.ComponentWorkflowConfig{}
	}

	// Get or create workflow parameters
	if component.Spec.Workflow.Parameters == nil {
		params := make(map[string]interface{})
		component.Spec.Workflow.Parameters = &params
	}
	workflowParams := *component.Spec.Workflow.Parameters

	// Get existing environment variables
	existingEnvVars := make([]map[string]any, 0)
	if envVarsInterface, ok := workflowParams["environmentVariables"].([]interface{}); ok {
		for _, env := range envVarsInterface {
			if envMap, ok := env.(map[string]interface{}); ok {
				existingEnvVars = append(existingEnvVars, envMap)
			}
		}
	}

	// Build merged environment variables map
	envMap := make(map[string]map[string]any)
	for _, env := range existingEnvVars {
		if name, ok := env["name"].(string); ok {
			envMap[name] = env
		}
	}
	for _, newEnv := range envVars {
		envVar := map[string]any{
			"name": newEnv.Key,
		}
		if newEnv.ValueFrom != nil && newEnv.ValueFrom.SecretKeyRef != nil {
			// Secret reference - use valueFrom pattern
			envVar["valueFrom"] = map[string]any{
				"secretKeyRef": map[string]any{
					"name": newEnv.ValueFrom.SecretKeyRef.Name,
					"key":  newEnv.ValueFrom.SecretKeyRef.Key,
				},
			}
		} else {
			// Plain value
			envVar["value"] = newEnv.Value
		}
		envMap[newEnv.Key] = envVar
	}

	// Convert map to slice
	mergedEnvVars := make([]map[string]any, 0, len(envMap))
	for _, env := range envMap {
		mergedEnvVars = append(mergedEnvVars, env)
	}

	// Update workflow parameters
	workflowParams["environmentVariables"] = mergedEnvVars

	// Update the component
	updateResp, err := c.ocClient.UpdateComponentWithResponse(ctx, namespaceName, componentName, *component)
	if err != nil {
		return fmt.Errorf("failed to update component environment variables: %w", err)
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

// UpdateComponentEnvVars updates the environment variables in the component's workflow parameters.
func (c *openChoreoClient) UpdateComponentEnvVars(ctx context.Context, namespaceName, projectName, componentName string, envVars []EnvVar) error {
	return c.mergeComponentEnvVars(ctx, namespaceName, componentName, envVars)
}

// ReplaceComponentEnvVars replaces all environment variables in the component's workflow parameters.
// Unlike mergeComponentEnvVars which merges with existing vars, this completely replaces them.
func (c *openChoreoClient) ReplaceComponentEnvVars(ctx context.Context, namespaceName, projectName, componentName string, envVars []EnvVar) error {
	resp, err := c.ocClient.GetComponentWithResponse(ctx, namespaceName, componentName)
	if err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}
	if resp.JSON200 == nil || resp.JSON200.Spec == nil {
		return fmt.Errorf("invalid component response")
	}

	component := resp.JSON200

	// Ensure workflow exists
	if component.Spec.Workflow == nil {
		component.Spec.Workflow = &gen.ComponentWorkflowConfig{}
	}

	// Get or create workflow parameters
	if component.Spec.Workflow.Parameters == nil {
		params := make(map[string]interface{})
		component.Spec.Workflow.Parameters = &params
	}
	workflowParams := *component.Spec.Workflow.Parameters

	// Build new environment variables slice (replacing all existing)
	newEnvVars := make([]map[string]any, 0, len(envVars))
	for _, newEnv := range envVars {
		envVar := map[string]any{
			"name": newEnv.Key,
		}
		if newEnv.ValueFrom != nil && newEnv.ValueFrom.SecretKeyRef != nil {
			envVar["valueFrom"] = map[string]any{
				"secretKeyRef": map[string]any{
					"name": newEnv.ValueFrom.SecretKeyRef.Name,
					"key":  newEnv.ValueFrom.SecretKeyRef.Key,
				},
			}
		} else {
			envVar["value"] = newEnv.Value
		}
		newEnvVars = append(newEnvVars, envVar)
	}

	workflowParams["environmentVariables"] = newEnvVars

	updateResp, err := c.ocClient.UpdateComponentWithResponse(ctx, namespaceName, componentName, *component)
	if err != nil {
		return fmt.Errorf("failed to replace component environment variables: %w", err)
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

// UpdateReleaseBindingEnvVars merges env vars into the ReleaseBinding for the specified environment,
// then sets restartedAt to trigger a pod rollout. If no binding exists for the component+environment yet
// (agent not deployed), returns nil — the Component CR vars will be picked up on first deploy.
func (c *openChoreoClient) UpdateReleaseBindingEnvVars(ctx context.Context, namespaceName, projectName, componentName, envName string, envVars []EnvVar) error {
	componentFilter := componentName
	listResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentFilter,
	})
	if err != nil {
		return fmt.Errorf("failed to list release bindings: %w", err)
	}
	if listResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(listResp.StatusCode(), ErrorResponses{
			JSON401: listResp.JSON401,
			JSON403: listResp.JSON403,
			JSON404: listResp.JSON404,
			JSON500: listResp.JSON500,
		})
	}
	if listResp.JSON200 == nil || len(listResp.JSON200.Items) == 0 {
		// No bindings yet — agent not deployed; skip silently.
		return nil
	}

	// Find the binding for the specified environment (client-side filter since the API has no env param).
	var bindingName string
	for _, b := range listResp.JSON200.Items {
		if b.Spec != nil && b.Spec.Environment == envName {
			bindingName = b.Metadata.Name
			break
		}
	}
	if bindingName == "" {
		// No binding for this environment yet — agent not deployed there; skip silently.
		return nil
	}

	getResp, err := c.ocClient.GetReleaseBindingWithResponse(ctx, namespaceName, bindingName)
	if err != nil {
		return fmt.Errorf("failed to get release binding %q: %w", bindingName, err)
	}
	if getResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(getResp.StatusCode(), ErrorResponses{
			JSON401: getResp.JSON401,
			JSON403: getResp.JSON403,
			JSON404: getResp.JSON404,
			JSON500: getResp.JSON500,
		})
	}
	if getResp.JSON200 == nil {
		return fmt.Errorf("empty response from get release binding")
	}

	releaseBinding := getResp.JSON200
	if releaseBinding.Spec == nil {
		return fmt.Errorf("release binding spec is nil")
	}

	// Ensure WorkloadOverrides and Container exist.
	if releaseBinding.Spec.WorkloadOverrides == nil {
		releaseBinding.Spec.WorkloadOverrides = &gen.WorkloadOverrides{}
	}
	if releaseBinding.Spec.WorkloadOverrides.Container == nil {
		releaseBinding.Spec.WorkloadOverrides.Container = &gen.ContainerOverride{}
	}

	// Build merged env var map (existing + new, keyed by name).
	existing := make(map[string]gen.EnvVar)
	if releaseBinding.Spec.WorkloadOverrides.Container.Env != nil {
		for _, ev := range *releaseBinding.Spec.WorkloadOverrides.Container.Env {
			existing[ev.Key] = ev
		}
	}
	for _, newEnv := range envVars {
		genEnv := gen.EnvVar{Key: newEnv.Key}
		if newEnv.ValueFrom != nil && newEnv.ValueFrom.SecretKeyRef != nil {
			name := newEnv.ValueFrom.SecretKeyRef.Name
			key := newEnv.ValueFrom.SecretKeyRef.Key
			genEnv.ValueFrom = &gen.EnvVarValueFrom{
				SecretKeyRef: &struct {
					Key  *string `json:"key,omitempty"`
					Name *string `json:"name,omitempty"`
				}{
					Name: &name,
					Key:  &key,
				},
			}
		} else {
			v := newEnv.Value
			genEnv.Value = &v
		}
		existing[newEnv.Key] = genEnv
	}

	merged := make([]gen.EnvVar, 0, len(existing))
	for _, ev := range existing {
		merged = append(merged, ev)
	}
	releaseBinding.Spec.WorkloadOverrides.Container.Env = &merged

	// Set restartedAt to trigger pod rollout.
	if releaseBinding.Spec.ComponentTypeEnvironmentConfigs == nil {
		overrides := make(map[string]interface{})
		releaseBinding.Spec.ComponentTypeEnvironmentConfigs = &overrides
	}
	// restartedAt triggers a pod rollout via ComponentTypeEnvironmentConfigs.
	// NOTE: This assumes OpenChoreo interprets this key as a rollout signal.
	// If pods are not restarted after env var updates, revisit the OpenChoreo API spec.
	(*releaseBinding.Spec.ComponentTypeEnvironmentConfigs)["restartedAt"] = time.Now().Format(time.RFC3339)

	updateResp, err := c.ocClient.UpdateReleaseBindingWithResponse(ctx, namespaceName, bindingName, *releaseBinding)
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

// RemoveComponentEnvironmentVariables removes the specified env var keys from the component's
// workflow parameters and updates the component CR.
func (c *openChoreoClient) RemoveComponentEnvironmentVariables(ctx context.Context, namespaceName, projectName, componentName string, envVarKeys []string) error {
	resp, err := c.ocClient.GetComponentWithResponse(ctx, namespaceName, componentName)
	if err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}
	if resp.JSON200 == nil || resp.JSON200.Spec == nil {
		return fmt.Errorf("invalid component response")
	}

	component := resp.JSON200

	if component.Spec.Workflow == nil || component.Spec.Workflow.Parameters == nil {
		// Nothing to remove.
		return nil
	}
	workflowParams := *component.Spec.Workflow.Parameters

	existingEnvVars := make([]map[string]any, 0)
	if envVarsInterface, ok := workflowParams["environmentVariables"].([]interface{}); ok {
		for _, env := range envVarsInterface {
			if envMap, ok := env.(map[string]interface{}); ok {
				existingEnvVars = append(existingEnvVars, envMap)
			}
		}
	}

	removeSet := make(map[string]bool, len(envVarKeys))
	for _, k := range envVarKeys {
		removeSet[k] = true
	}

	filtered := make([]map[string]any, 0, len(existingEnvVars))
	for _, ev := range existingEnvVars {
		if name, ok := ev["name"].(string); ok && removeSet[name] {
			continue
		}
		filtered = append(filtered, ev)
	}

	workflowParams["environmentVariables"] = filtered

	updateResp, err := c.ocClient.UpdateComponentWithResponse(ctx, namespaceName, componentName, *component)
	if err != nil {
		return fmt.Errorf("failed to update component environment variables: %w", err)
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

// RemoveReleaseBindingEnvVars removes env var keys from the ReleaseBinding for the specified environment,
// then sets restartedAt to trigger a pod rollout. If no binding exists for the component+environment yet,
// returns nil (idempotent — nothing to remove).
func (c *openChoreoClient) RemoveReleaseBindingEnvVars(ctx context.Context, namespaceName, projectName, componentName, envName string, envVarKeys []string) error {
	if len(envVarKeys) == 0 {
		return nil
	}

	componentFilter := componentName
	listResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentFilter,
	})
	if err != nil {
		return fmt.Errorf("failed to list release bindings: %w", err)
	}
	if listResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(listResp.StatusCode(), ErrorResponses{
			JSON401: listResp.JSON401,
			JSON403: listResp.JSON403,
			JSON404: listResp.JSON404,
			JSON500: listResp.JSON500,
		})
	}
	if listResp.JSON200 == nil || len(listResp.JSON200.Items) == 0 {
		// No bindings yet — nothing to remove.
		return nil
	}

	// Find the binding for the specified environment.
	var bindingName string
	for _, b := range listResp.JSON200.Items {
		if b.Spec != nil && b.Spec.Environment == envName {
			bindingName = b.Metadata.Name
			break
		}
	}
	if bindingName == "" {
		// No binding for this environment — nothing to remove.
		return nil
	}

	getResp, err := c.ocClient.GetReleaseBindingWithResponse(ctx, namespaceName, bindingName)
	if err != nil {
		return fmt.Errorf("failed to get release binding %q: %w", bindingName, err)
	}
	if getResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(getResp.StatusCode(), ErrorResponses{
			JSON401: getResp.JSON401,
			JSON403: getResp.JSON403,
			JSON404: getResp.JSON404,
			JSON500: getResp.JSON500,
		})
	}
	if getResp.JSON200 == nil {
		return fmt.Errorf("empty response from get release binding")
	}

	releaseBinding := getResp.JSON200
	if releaseBinding.Spec == nil {
		return fmt.Errorf("release binding spec is nil")
	}

	// If there are no workload overrides or no env vars set, nothing to remove.
	if releaseBinding.Spec.WorkloadOverrides == nil ||
		releaseBinding.Spec.WorkloadOverrides.Container == nil ||
		releaseBinding.Spec.WorkloadOverrides.Container.Env == nil {
		return nil
	}

	// Build remove set and filter out matching keys.
	removeSet := make(map[string]bool, len(envVarKeys))
	for _, k := range envVarKeys {
		removeSet[k] = true
	}

	existing := *releaseBinding.Spec.WorkloadOverrides.Container.Env
	filtered := make([]gen.EnvVar, 0, len(existing))
	for _, ev := range existing {
		if !removeSet[ev.Key] {
			filtered = append(filtered, ev)
		}
	}
	releaseBinding.Spec.WorkloadOverrides.Container.Env = &filtered

	// Set restartedAt to trigger pod rollout.
	if releaseBinding.Spec.ComponentTypeEnvironmentConfigs == nil {
		overrides := make(map[string]interface{})
		releaseBinding.Spec.ComponentTypeEnvironmentConfigs = &overrides
	}
	(*releaseBinding.Spec.ComponentTypeEnvironmentConfigs)["restartedAt"] = time.Now().Format(time.RFC3339)

	updateResp, err := c.ocClient.UpdateReleaseBindingWithResponse(ctx, namespaceName, bindingName, *releaseBinding)
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

// TraitOption allows passing optional parameters when building traits.
type TraitOption func(map[string]interface{})

// WithUpstreamPort sets the upstream port for the api-configuration trait.
func WithUpstreamPort(port int32) TraitOption {
	return func(params map[string]interface{}) {
		params["upstreamPort"] = port
	}
}

// WithUpstreamBasePath sets the upstream base path for the api-configuration trait.
func WithUpstreamBasePath(basePath string) TraitOption {
	return func(params map[string]interface{}) {
		params["upstreamBasePath"] = basePath
	}
}

// WithAgentApiKey sets the agent API key for OTEL and env-injection traits.
func WithAgentApiKey(apiKey string) TraitOption {
	return func(params map[string]interface{}) {
		params["agentApiKey"] = apiKey
	}
}

func (c *openChoreoClient) buildTrait(ctx context.Context, namespaceName, projectName, componentName string, traitType TraitType, opts ...TraitOption) (gen.ComponentTrait, error) {
	traitKind := gen.ComponentTraitKindClusterTrait
	trait := gen.ComponentTrait{
		Kind:         &traitKind,
		Name:         string(traitType),
		InstanceName: fmt.Sprintf("%s-%s", componentName, string(traitType)),
	}
	switch traitType {
	case TraitOTELInstrumentation:
		params, err := c.buildOTELTraitParameters(ctx, namespaceName, projectName, componentName, opts...)
		if err != nil {
			return gen.ComponentTrait{}, err
		}
		trait.Parameters = &params
	case TraitEnvInjection:
		params, err := c.buildEnvInjectionTraitParameters(opts...)
		if err != nil {
			return gen.ComponentTrait{}, err
		}
		trait.Parameters = &params
	case TraitAPIManagement:
		params, err := c.buildAPIConfigurationTraitParameters(componentName, opts...)
		if err != nil {
			return gen.ComponentTrait{}, err
		}
		trait.Parameters = &params
	default:
		return gen.ComponentTrait{}, fmt.Errorf("unsupported trait type: %s", traitType)
	}
	return trait, nil
}

func (c *openChoreoClient) buildAPIConfigurationTraitParameters(componentName string, opts ...TraitOption) (map[string]interface{}, error) {
	params := map[string]interface{}{
		"apiName":          componentName,
		"apiVersion":       "v1.0",
		"context":          fmt.Sprintf("/%s", componentName),
		"upstreamPort":     config.GetConfig().DefaultChatAPI.DefaultHTTPPort,
		"upstreamBasePath": config.GetConfig().DefaultChatAPI.DefaultBasePath,
	}
	for _, opt := range opts {
		opt(params)
	}
	return params, nil
}

func (c *openChoreoClient) buildOTELTraitParameters(ctx context.Context, namespaceName, projectName, componentName string, opts ...TraitOption) (map[string]interface{}, error) {
	params := make(map[string]interface{})
	for _, opt := range opts {
		opt(params)
	}
	agentApiKey, _ := params["agentApiKey"].(string)
	if agentApiKey == "" {
		return nil, fmt.Errorf("agent API key is required for OTEL instrumentation trait")
	}
	// Get the component to retrieve UUID and language version
	component, err := c.GetComponent(ctx, namespaceName, projectName, componentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get component for trait attachment: %w", err)
	}
	languageVersion := ""
	if component.Build != nil && component.Build.Buildpack != nil {
		languageVersion = component.Build.Buildpack.LanguageVersion
	}

	// Get the project to find the deployment pipeline
	project, err := c.GetProject(ctx, namespaceName, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get project for trait attachment: %w", err)
	}
	if project.DeploymentPipeline == "" {
		return nil, fmt.Errorf("failed to attach trait: project %s does not have a deployment pipeline configured", projectName)
	}

	cfg := config.GetConfig()
	instrumentationImage, err := getInstrumentationImage(languageVersion, cfg.PackageVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to build instrumentation image: %w", err)
	}

	return map[string]interface{}{
		"instrumentationImage":  instrumentationImage,
		"sdkVolumeName":         cfg.OTEL.SDKVolumeName,
		"sdkMountPath":          cfg.OTEL.SDKMountPath,
		"otelEndpoint":          cfg.OTEL.ExporterEndpoint,
		"isTraceContentEnabled": utils.BoolAsString(cfg.OTEL.IsTraceContentEnabled),
		"agentApiKey":           agentApiKey,
	}, nil
}

// buildEnvInjectionTraitParameters builds parameters for the env injection trait
// which injects AMP_OTEL_ENDPOINT and AMP_AGENT_API_KEY environment variables
func (c *openChoreoClient) buildEnvInjectionTraitParameters(opts ...TraitOption) (map[string]interface{}, error) {
	params := make(map[string]interface{})
	for _, opt := range opts {
		opt(params)
	}
	agentApiKey, _ := params["agentApiKey"].(string)
	if agentApiKey == "" {
		return nil, fmt.Errorf("agent API key is required for env injection trait")
	}

	cfg := config.GetConfig()
	return map[string]interface{}{
		"otelEndpoint": cfg.OTEL.ExporterEndpoint,
		"agentApiKey":  agentApiKey,
	}, nil
}

func getInstrumentationImage(languageVersion, packageVersion string) (string, error) {
	parts := strings.Split(languageVersion, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid languageVersion format: expected 'major.minor' but got '%s'", languageVersion)
	}
	pythonMajorMinor := parts[0] + "." + parts[1]
	return fmt.Sprintf("%s/%s:%s-python%s", InstrumentationImageRegistry, InstrumentationImageName, packageVersion, pythonMajorMinor), nil
}

func (c *openChoreoClient) GetComponentEndpoints(ctx context.Context, namespaceName, projectName, componentName, environment string) (map[string]models.EndpointsResponse, error) {
	// List release bindings filtering by component to get endpoint URLs
	releaseBindingResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list release bindings: %w", err)
	}
	if releaseBindingResp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(releaseBindingResp.StatusCode(), ErrorResponses{
			JSON401: releaseBindingResp.JSON401,
			JSON403: releaseBindingResp.JSON403,
			JSON404: releaseBindingResp.JSON404,
			JSON500: releaseBindingResp.JSON500,
		})
	}

	// Extract endpoint URLs from release binding for the specified environment
	endpointURLs := make(map[string]string)
	if releaseBindingResp.JSON200 != nil {
		for _, binding := range releaseBindingResp.JSON200.Items {
			if binding.Spec != nil && binding.Spec.Environment == environment && binding.Status != nil && binding.Status.Endpoints != nil {
				for _, ep := range *binding.Status.Endpoints {
					// Use ExternalURLs based on IsLocalDevEnv config
					if ep.ExternalURLs != nil {
						var endpointURL *gen.EndpointURL
						if config.GetConfig().IsLocalDevEnv {
							endpointURL = ep.ExternalURLs.Http
						} else {
							endpointURL = ep.ExternalURLs.Https
						}
						if endpointURL != nil {
							endpointURLs[ep.Name] = buildEndpointURLString(endpointURL)
						}
					}
				}
				break
			}
		}
	}

	// List workloads to extract endpoint schema
	workloadResp, err := c.ocClient.ListWorkloadsWithResponse(ctx, namespaceName, &gen.ListWorkloadsParams{
		Component: &componentName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list workloads: %w", err)
	}
	if workloadResp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(workloadResp.StatusCode(), ErrorResponses{
			JSON401: workloadResp.JSON401,
			JSON403: workloadResp.JSON403,
			JSON404: workloadResp.JSON404,
			JSON500: workloadResp.JSON500,
		})
	}

	endpointDetails := make(map[string]models.EndpointsResponse)

	// Extract endpoint details from workload spec
	if workloadResp.JSON200 != nil && len(workloadResp.JSON200.Items) > 0 {
		workload := workloadResp.JSON200.Items[0]
		if workload.Spec != nil && workload.Spec.Endpoints != nil {
			for endpointName, endpoint := range *workload.Spec.Endpoints {
				visibility := ""
				if endpoint.Visibility != nil && len(*endpoint.Visibility) > 0 {
					visibility = string((*endpoint.Visibility)[0])
				}
				details := models.EndpointsResponse{
					Endpoint: models.Endpoint{
						Name:       endpointName,
						URL:        endpointURLs[endpointName],
						Visibility: visibility,
					},
				}
				if endpoint.Schema != nil && endpoint.Schema.Content != nil {
					details.Schema = models.EndpointSchema{Content: *endpoint.Schema.Content}
				}
				endpointDetails[endpointName] = details
			}
		}
	}

	return endpointDetails, nil
}

func (c *openChoreoClient) GetComponentConfigurations(ctx context.Context, namespaceName, projectName, componentName, environment string) ([]models.EnvVars, error) {
	// Create a map to store environment variables (for easy merging)
	type envVarEntry struct {
		Value       string
		IsSensitive bool
		SecretRef   string
	}
	envVarMap := make(map[string]envVarEntry)

	// List workloads to extract base environment variables
	workloadResp, err := c.ocClient.ListWorkloadsWithResponse(ctx, namespaceName, &gen.ListWorkloadsParams{
		Component: &componentName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list workloads: %w", err)
	}
	if workloadResp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(workloadResp.StatusCode(), ErrorResponses{
			JSON401: workloadResp.JSON401,
			JSON403: workloadResp.JSON403,
			JSON404: workloadResp.JSON404,
			JSON500: workloadResp.JSON500,
		})
	}

	// Extract base environment variables from workload
	if workloadResp.JSON200 != nil && len(workloadResp.JSON200.Items) > 0 {
		workload := workloadResp.JSON200.Items[0]
		if workload.Spec != nil && workload.Spec.Container != nil && workload.Spec.Container.Env != nil {
			for _, env := range *workload.Spec.Container.Env {
				// Check if this is a secret reference (sensitive value)
				isSensitive := env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil
				secretRef := ""
				if isSensitive && env.ValueFrom.SecretKeyRef.Name != nil {
					secretRef = *env.ValueFrom.SecretKeyRef.Name
				}
				envVarMap[env.Key] = envVarEntry{
					Value:       utils.StrPointerAsStr(env.Value, ""),
					IsSensitive: isSensitive,
					SecretRef:   secretRef,
				}
			}
		}
	}

	// List release bindings filtering by component to get overrides
	releaseBindingResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list release bindings: %w", err)
	}

	if releaseBindingResp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(releaseBindingResp.StatusCode(), ErrorResponses{
			JSON401: releaseBindingResp.JSON401,
			JSON403: releaseBindingResp.JSON403,
			JSON404: releaseBindingResp.JSON404,
			JSON500: releaseBindingResp.JSON500,
		})
	}

	if releaseBindingResp.JSON200 != nil && len(releaseBindingResp.JSON200.Items) > 0 {
		// Find the binding for the specified environment
		for _, binding := range releaseBindingResp.JSON200.Items {
			if binding.Spec != nil && binding.Spec.Environment == environment {
				// Extract workload overrides from binding
				if binding.Spec.WorkloadOverrides != nil && binding.Spec.WorkloadOverrides.Container != nil && binding.Spec.WorkloadOverrides.Container.Env != nil {
					for _, env := range *binding.Spec.WorkloadOverrides.Container.Env {
						// Check if this is a secret reference (sensitive value)
						isSensitive := env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil
						secretRef := ""
						if isSensitive && env.ValueFrom.SecretKeyRef.Name != nil {
							secretRef = *env.ValueFrom.SecretKeyRef.Name
						}
						envVarMap[env.Key] = envVarEntry{
							Value:       utils.StrPointerAsStr(env.Value, ""),
							IsSensitive: isSensitive,
							SecretRef:   secretRef,
						}
					}
				}
				break
			}
		}
	}

	// Convert map back to slice
	var envVars []models.EnvVars
	for key, entry := range envVarMap {
		envVars = append(envVars, models.EnvVars{
			Key:         key,
			Value:       entry.Value,
			IsSensitive: entry.IsSensitive,
			SecretRef:   entry.SecretRef,
		})
	}

	return envVars, nil
}

// -----------------------------------------------------------------------------
// Helper functions
// -----------------------------------------------------------------------------

// convertComponentFromTyped converts a gen.Component to models.AgentResponse
func convertComponentFromTyped(comp *gen.Component) (*models.AgentResponse, error) {
	if comp == nil {
		return nil, fmt.Errorf("component is nil")
	}
	if comp.Spec == nil {
		return nil, fmt.Errorf("component spec is nil")
	}

	provisioningType := getLabel(comp.Metadata.Labels, string(LabelKeyProvisioningType))
	componentTypeName := comp.Spec.ComponentType.Name
	if parts := strings.Split(componentTypeName, "/"); len(parts) > 1 {
		componentTypeName = parts[len(parts)-1]
	}
	agentType := models.AgentType{
		Type: componentTypeName,
	}
	if provisioningType == string(utils.InternalAgent) {
		agentType.SubType = getLabel(comp.Metadata.Labels, string(LabelKeyAgentSubType))
	}

	agent := &models.AgentResponse{
		Name:        comp.Metadata.Name,
		UUID:        utils.StrPointerAsStr(comp.Metadata.Uid, ""),
		DisplayName: getAnnotation(comp.Metadata.Annotations, AnnotationKeyDisplayName),
		Description: getAnnotation(comp.Metadata.Annotations, AnnotationKeyDescription),
		ProjectName: comp.Spec.Owner.ProjectName,
		Provisioning: models.Provisioning{
			Type: provisioningType,
		},
		Type: agentType,
	}

	if comp.Metadata.CreationTimestamp != nil {
		agent.CreatedAt = *comp.Metadata.CreationTimestamp
	}

	if comp.Spec.Parameters != nil {
		if basePath, ok := (*comp.Spec.Parameters)["basePath"].(string); ok {
			agent.InputInterface = &models.InputInterface{BasePath: basePath}
		}
	}

	if comp.Spec.Workflow != nil {
		agent.Provisioning.Repository = extractRepositoryFromTyped(comp.Spec.Workflow)
		if comp.Spec.Workflow.Parameters != nil {
			params := *comp.Spec.Workflow.Parameters
			agent.Build = extractBuildParams(params)
			if inputInterface := extractInputInterface(params); inputInterface != nil {
				if agent.InputInterface == nil {
					agent.InputInterface = inputInterface
				} else {
					agent.InputInterface.Port = inputInterface.Port
					agent.InputInterface.Type = inputInterface.Type
					agent.InputInterface.Schema = inputInterface.Schema
					agent.InputInterface.BasePath = inputInterface.BasePath
					agent.InputInterface.Visibility = inputInterface.Visibility
				}
			}
		}
	}

	return agent, nil
}

func getAnnotation(annotations *map[string]string, key string) string {
	if annotations == nil {
		return ""
	}
	return (*annotations)[string(key)]
}

func getLabel(labels *map[string]string, key string) string {
	if labels == nil {
		return ""
	}
	return (*labels)[string(key)]
}

// extractRepositoryFromTyped extracts repository details from ComponentWorkflowConfig parameters
func extractRepositoryFromTyped(workflow *gen.ComponentWorkflowConfig) models.Repository {
	if workflow == nil || workflow.Parameters == nil {
		return models.Repository{}
	}
	params := *workflow.Parameters

	repo, ok := params["repository"].(map[string]interface{})
	if !ok {
		return models.Repository{}
	}

	branch := ""
	if revision, ok := repo["revision"].(map[string]interface{}); ok {
		branch = getMapString(revision, "branch")
	}
	return models.Repository{
		Url:     getMapString(repo, "url"),
		Branch:  branch,
		AppPath: getMapString(repo, "appPath"),
	}
}

// extractBuildParams extracts build configuration (buildpack or docker) from parameters
func extractBuildParams(params map[string]interface{}) *models.Build {
	if bp, ok := params["buildpackConfigs"].(map[string]interface{}); ok {
		return &models.Build{
			Type: BuildTypeBuildpack,
			Buildpack: &models.BuildpackConfig{
				Language:        getMapString(bp, "language"),
				LanguageVersion: getMapString(bp, "languageVersion"),
				RunCommand:      getMapString(bp, "googleEntryPoint"),
			},
		}
	}
	if dc, ok := params["dockerConfigs"].(map[string]interface{}); ok {
		return &models.Build{
			Type:   BuildTypeDocker,
			Docker: &models.DockerConfig{DockerfilePath: getMapString(dc, "dockerfilePath")},
		}
	}
	return nil
}

// extractInputInterface extracts endpoint/input interface info from parameters
func extractInputInterface(params map[string]interface{}) *models.InputInterface {
	endpoints, ok := params["endpoints"].([]interface{})
	if !ok || len(endpoints) == 0 {
		return nil
	}
	ep, ok := endpoints[0].(map[string]interface{})
	if !ok {
		return nil
	}
	inputInterface := &models.InputInterface{
		Type:     getMapString(ep, "type"),
		BasePath: getMapString(ep, "basePath"),
	}
	if port, ok := ep["port"].(float64); ok {
		inputInterface.Port = int32(port)
	}
	if schemaPath := getMapString(ep, "schemaFilePath"); schemaPath != "" {
		inputInterface.Schema = &models.InputInterfaceSchema{Path: schemaPath}
	}
	if visibility, ok := ep["visibility"].([]interface{}); ok {
		inputInterface.Visibility = make([]string, 0, len(visibility))
		for _, v := range visibility {
			if s, ok := v.(string); ok {
				inputInterface.Visibility = append(inputInterface.Visibility, s)
			}
		}
	}
	return inputInterface
}

func getMapString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// buildEndpointURLString constructs a URL string from EndpointURL struct
func buildEndpointURLString(ep *gen.EndpointURL) string {
	if ep == nil {
		return ""
	}
	scheme := ""
	if ep.Scheme != nil {
		scheme = *ep.Scheme
	}
	host := ep.Host
	if ep.Port != nil {
		host = fmt.Sprintf("%s:%d", host, *ep.Port)
	}
	path := ""
	if ep.Path != nil {
		path = *ep.Path
	}
	return fmt.Sprintf("%s://%s%s", scheme, host, path)
}
