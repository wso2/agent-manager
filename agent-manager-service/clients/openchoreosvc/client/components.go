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
	componentTypeKind := gen.ComponentSpecComponentTypeKindComponentType
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
	componentTypeKind := gen.ComponentSpecComponentTypeKindComponentType
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
		Exposed:  true,
		Replicas: DefaultReplicaCount,
		Resources: &ResourceConfig{
			Requests: &ResourceRequests{
				CPU:    DefaultCPURequest,
				Memory: DefaultMemoryRequest,
			},
			Limits: &ResourceLimits{
				CPU:    DefaultCPULimit,
				Memory: DefaultMemoryLimit,
			},
		},
		AutoScaling: &AutoScalingConfig{
			Enabled:     DefaultAutoscalingEnabledPtr,
			MinReplicas: DefaultAutoscalingMinReplicasPtr,
			MaxReplicas: DefaultAutoscalingMaxReplicasPtr,
		},
		CORS: &CORSConfig{
			AllowOrigin:  DefaultCORSAllowOrigins,
			AllowMethods: DefaultCORSAllowMethods,
			AllowHeaders: DefaultCORSAllowHeaders,
		},
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
				Name:       &componentWorkflowName,
				Parameters: &componentWorkflowParameters,
				SystemParameters: &struct {
					Repository *struct {
						AppPath  *string `json:"appPath,omitempty"`
						Revision *struct {
							Branch *string `json:"branch,omitempty"`
							Commit *string `json:"commit,omitempty"`
						} `json:"revision,omitempty"`
						Url *string `json:"url,omitempty"`
					} `json:"repository,omitempty"`
				}{
					Repository: &struct {
						AppPath  *string `json:"appPath,omitempty"`
						Revision *struct {
							Branch *string `json:"branch,omitempty"`
							Commit *string `json:"commit,omitempty"`
						} `json:"revision,omitempty"`
						Url *string `json:"url,omitempty"`
					}{
						Url:     &req.Repository.URL,
						AppPath: &req.Repository.AppPath,
						Revision: &struct {
							Branch *string `json:"branch,omitempty"`
							Commit *string `json:"commit,omitempty"`
						}{
							Branch: &req.Repository.Branch,
						},
					},
				},
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

func (c *openChoreoClient) GetComponentResourceConfigs(ctx context.Context, namespaceName, projectName, componentName, environment string) (*ComponentResourceConfigsResponse, error) {
	// If environment is not provided, fetch component-level defaults only
	if environment == "" {
		return c.getComponentLevelResourceConfigs(ctx, namespaceName, projectName, componentName)
	}
	// If environment is provided, fetch both environment-specific and component-level defaults
	return c.getEnvironmentResourceConfigs(ctx, namespaceName, projectName, componentName, environment)
}

func (c *openChoreoClient) UpdateComponentResourceConfigs(ctx context.Context, namespaceName, projectName, componentName, environment string, req UpdateComponentResourceConfigsRequest) error {
	// If environment is provided, update the release binding for that specific environment
	// Otherwise, update the component itself (which updates defaults for all environments)
	if environment != "" {
		return c.updateReleaseBindingResourceConfigs(ctx, namespaceName, projectName, componentName, environment, req)
	}
	return c.updateComponentResourceConfigs(ctx, namespaceName, projectName, componentName, req)
}

// updateComponentResourceConfigs updates component-level parameters (defaults for all environments)
func (c *openChoreoClient) updateComponentResourceConfigs(ctx context.Context, namespaceName, projectName, componentName string, req UpdateComponentResourceConfigsRequest) error {
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
	if component.Spec == nil {
		return fmt.Errorf("component spec is nil")
	}

	// Get or create parameters
	if component.Spec.Parameters == nil {
		params := make(map[string]interface{})
		component.Spec.Parameters = &params
	}
	parameters := *component.Spec.Parameters

	// Update replicas if provided
	if req.Replicas != nil {
		parameters["replicas"] = *req.Replicas
	}

	// Update resources if provided
	if req.Resources != nil {
		resourcesMap, err := structToMap(req.Resources)
		if err != nil {
			return fmt.Errorf("failed to convert resources to map: %w", err)
		}
		parameters["resources"] = resourcesMap
	}

	// Update autoscaling if provided
	if req.AutoScaling != nil {
		autoscalingMap, err := structToMap(req.AutoScaling)
		if err != nil {
			return fmt.Errorf("failed to convert autoscaling to map: %w", err)
		}
		parameters["autoscaling"] = autoscalingMap
	}

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

// updateReleaseBindingResourceConfigs updates environment-specific parameters via release binding
func (c *openChoreoClient) updateReleaseBindingResourceConfigs(ctx context.Context, namespaceName, projectName, componentName, environment string, req UpdateComponentResourceConfigsRequest) error {
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
	if releaseBinding.Spec.ComponentTypeEnvOverrides == nil {
		overrides := make(map[string]interface{})
		releaseBinding.Spec.ComponentTypeEnvOverrides = &overrides
	}
	componentTypeEnvOverrides := *releaseBinding.Spec.ComponentTypeEnvOverrides

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

	// Add autoscaling if provided
	if req.AutoScaling != nil {
		autoscalingMap, err := structToMap(req.AutoScaling)
		if err != nil {
			return fmt.Errorf("failed to convert autoscaling to map: %w", err)
		}
		componentTypeEnvOverrides["autoscaling"] = autoscalingMap
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

// getComponentLevelResourceConfigs fetches component-level default resource configurations
func (c *openChoreoClient) getComponentLevelResourceConfigs(ctx context.Context, namespaceName, projectName, componentName string) (*ComponentResourceConfigsResponse, error) {
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
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from get component")
	}

	response := &ComponentResourceConfigsResponse{}
	component := resp.JSON200

	if component.Spec != nil && component.Spec.Parameters != nil {
		params, err := mapToComponentParameters(*component.Spec.Parameters)
		if err != nil {
			return nil, fmt.Errorf("failed to parse component parameters: %w", err)
		}

		// Extract replicas (>= 0 to support scale-to-zero)
		if params.Replicas >= 0 {
			replicas := int32(params.Replicas)
			response.Replicas = &replicas
		}

		// Extract resources
		response.Resources = params.Resources

		// Extract autoscaling
		response.AutoScaling = params.AutoScaling
	}

	return response, nil
}

// getEnvironmentResourceConfigs fetches environment-specific resource configurations along with component defaults
func (c *openChoreoClient) getEnvironmentResourceConfigs(ctx context.Context, namespaceName, projectName, componentName, environment string) (*ComponentResourceConfigsResponse, error) {
	// Fetch the component to get its parameters
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

	// Extract component parameters
	var componentParams *ComponentParameters
	component := compResp.JSON200
	if component.Spec != nil && component.Spec.Parameters != nil {
		componentParams, err = mapToComponentParameters(*component.Spec.Parameters)
		if err != nil {
			return nil, fmt.Errorf("failed to parse component parameters: %w", err)
		}
	}

	// List release bindings to find the one for this environment
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

	// Initialize response from component parameters
	response := &ComponentResourceConfigsResponse{}
	if componentParams != nil {
		// Support scale-to-zero (>= 0)
		if componentParams.Replicas >= 0 {
			replicas := int32(componentParams.Replicas)
			response.Replicas = &replicas
		}
		response.Resources = componentParams.Resources
		response.AutoScaling = componentParams.AutoScaling
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
		// No binding found - return component parameters
		return response, nil
	}

	// Check if there are overrides in componentTypeEnvOverrides
	if binding.Spec != nil && binding.Spec.ComponentTypeEnvOverrides != nil {
		envOverrides, err := mapToEnvOverrideParameters(*binding.Spec.ComponentTypeEnvOverrides)
		if err != nil {
			return nil, fmt.Errorf("failed to parse env overrides: %w", err)
		}

		// Apply replicas override
		if envOverrides.Replicas != nil {
			replicas := int32(*envOverrides.Replicas)
			response.Replicas = &replicas
		}

		// Apply resources override (merge with component parameters)
		if envOverrides.Resources != nil {
			if response.Resources == nil {
				response.Resources = &ResourceConfig{}
			}
			if envOverrides.Resources.Requests != nil {
				if response.Resources.Requests == nil {
					response.Resources.Requests = &ResourceRequests{}
				}
				if envOverrides.Resources.Requests.CPU != "" {
					response.Resources.Requests.CPU = envOverrides.Resources.Requests.CPU
				}
				if envOverrides.Resources.Requests.Memory != "" {
					response.Resources.Requests.Memory = envOverrides.Resources.Requests.Memory
				}
			}
			if envOverrides.Resources.Limits != nil {
				if response.Resources.Limits == nil {
					response.Resources.Limits = &ResourceLimits{}
				}
				if envOverrides.Resources.Limits.CPU != "" {
					response.Resources.Limits.CPU = envOverrides.Resources.Limits.CPU
				}
				if envOverrides.Resources.Limits.Memory != "" {
					response.Resources.Limits.Memory = envOverrides.Resources.Limits.Memory
				}
			}
		}

		// Apply autoscaling override
		if envOverrides.AutoScaling != nil {
			response.AutoScaling = envOverrides.AutoScaling
		}
	}

	return response, nil
}

// extractResourceConfig extracts ResourceConfig from a map
func extractResourceConfig(resources map[string]interface{}) *ResourceConfig {
	config := &ResourceConfig{}

	// Extract requests
	if requests, ok := resources["requests"].(map[string]interface{}); ok {
		requestsConfig := &ResourceRequests{}
		if cpu, ok := requests["cpu"].(string); ok {
			requestsConfig.CPU = cpu
		}
		if memory, ok := requests["memory"].(string); ok {
			requestsConfig.Memory = memory
		}
		if requestsConfig.CPU != "" || requestsConfig.Memory != "" {
			config.Requests = requestsConfig
		}
	}

	// Extract limits
	if limits, ok := resources["limits"].(map[string]interface{}); ok {
		limitsConfig := &ResourceLimits{}
		if cpu, ok := limits["cpu"].(string); ok {
			limitsConfig.CPU = cpu
		}
		if memory, ok := limits["memory"].(string); ok {
			limitsConfig.Memory = memory
		}
		if limitsConfig.CPU != "" || limitsConfig.Memory != "" {
			config.Limits = limitsConfig
		}
	}

	if config.Requests != nil || config.Limits != nil {
		return config
	}
	return nil
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

// mapToComponentParameters converts a map to ComponentParameters using JSON marshaling
func mapToComponentParameters(m map[string]interface{}) (*ComponentParameters, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var params ComponentParameters
	if err := json.Unmarshal(data, &params); err != nil {
		return nil, err
	}
	return &params, nil
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

func getInputInterfaceConfig(req CreateComponentRequest) (int32, string) {
	agentSubType := req.AgentType.SubType
	if req.AgentType.Type == string(utils.AgentTypeAPI) && agentSubType == string(utils.AgentSubTypeChatAPI) {
		return int32(config.GetConfig().DefaultChatAPI.DefaultHTTPPort), config.GetConfig().DefaultChatAPI.DefaultBasePath
	}
	// agentSubType is validated in controller layer
	return req.InputInterface.Port, req.InputInterface.BasePath
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

func (c *openChoreoClient) AttachTrait(ctx context.Context, namespaceName, projectName, componentName string, traitType TraitType, agentApiKey ...string) error {
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
	var traits []gen.ComponentTrait
	if component.Spec.Traits != nil {
		traits = *component.Spec.Traits
	}

	// Check if trait already exists
	for _, trait := range traits {
		if trait.Name == string(traitType) {
			return nil
		}
	}

	// Add the new trait with type-specific parameters
	newTrait, err := c.buildTrait(ctx, namespaceName, projectName, componentName, traitType, agentApiKey...)
	if err != nil {
		return fmt.Errorf("failed to build trait: %w", err)
	}
	traits = append(traits, newTrait)
	component.Spec.Traits = &traits

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

// InjectTracingEnvVars updates the tracing related environment variables for a component
func (c *openChoreoClient) InjectTracingEnvVars(ctx context.Context, namespaceName, projectName, componentName string, envVars []EnvVar) error {
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

func (c *openChoreoClient) buildTrait(ctx context.Context, namespaceName, projectName, componentName string, traitType TraitType, agentApiKey ...string) (gen.ComponentTrait, error) {
	trait := gen.ComponentTrait{
		Name:         string(traitType),
		InstanceName: fmt.Sprintf("%s-%s", componentName, string(traitType)),
	}
	apiKey := ""
	if len(agentApiKey) > 0 {
		apiKey = agentApiKey[0]
	}
	switch traitType {
	case TraitOTELInstrumentation:
		params, err := c.buildOTELTraitParameters(ctx, namespaceName, projectName, componentName, apiKey)
		if err != nil {
			return gen.ComponentTrait{}, err
		}
		trait.Parameters = &params
	case TraitEnvInjection:
		params, err := c.buildEnvInjectionTraitParameters(apiKey)
		if err != nil {
			return gen.ComponentTrait{}, err
		}
		trait.Parameters = &params
	}
	return trait, nil
}

func (c *openChoreoClient) buildOTELTraitParameters(ctx context.Context, namespaceName, projectName, componentName, agentApiKey string) (map[string]interface{}, error) {
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
func (c *openChoreoClient) buildEnvInjectionTraitParameters(agentApiKey string) (map[string]interface{}, error) {
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
					endpointURLs[ep.Name] = ep.InvokeURL
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
				basePath := ""
				if endpoint.BasePath != nil {
					basePath = *endpoint.BasePath
				}
				details := models.EndpointsResponse{
					Endpoint: models.Endpoint{
						Name: endpointName,
						URL:  fmt.Sprintf("%s%s", endpointURLs[endpointName], basePath),
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
	// Value is stored as a struct to track sensitivity
	type envVarEntry struct {
		Value       string
		IsSensitive bool
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
				envVarMap[env.Key] = envVarEntry{Value: utils.StrPointerAsStr(env.Value, "")}
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
						envVarMap[env.Key] = envVarEntry{Value: utils.StrPointerAsStr(env.Value, "")}
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

// extractRepositoryFromTyped extracts repository details from ComponentWorkflowConfig
func extractRepositoryFromTyped(workflow *gen.ComponentWorkflowConfig) models.Repository {
	if workflow.SystemParameters == nil || workflow.SystemParameters.Repository == nil {
		return models.Repository{}
	}
	repo := workflow.SystemParameters.Repository
	result := models.Repository{
		Url:     utils.StrPointerAsStr(repo.Url, ""),
		AppPath: utils.StrPointerAsStr(repo.AppPath, ""),
	}
	if repo.Revision != nil {
		result.Branch = utils.StrPointerAsStr(repo.Revision.Branch, "")
	}
	return result
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
