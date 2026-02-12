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

	"github.com/google/uuid"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

func (c *openChoreoClient) CreateComponent(ctx context.Context, namespaceName, projectName string, req CreateComponentRequest) error {
	apiReq, err := buildComponentRequest(namespaceName, projectName, req)
	if err != nil {
		return fmt.Errorf("failed to build component request: %w", err)
	}

	resp, err := c.ocClient.ApplyResourceWithResponse(ctx, apiReq)
	if err != nil {
		return fmt.Errorf("failed to create component: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated && resp.StatusCode() != http.StatusOK {
		return handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{
			NotFoundErr: utils.ErrProjectNotFound,
			ConflictErr: utils.ErrAgentAlreadyExists,
		})
	}
	return nil
}

func buildComponentRequest(orgName, projectName string, req CreateComponentRequest) (gen.ApplyResourceJSONRequestBody, error) {
	if req.ProvisioningType == ProvisioningExternal {
		return createComponentCRForExternalAgents(orgName, projectName, req)
	}
	return createComponentCRForInternalAgents(orgName, projectName, req)
}

func createComponentCRForExternalAgents(orgName, projectName string, req CreateComponentRequest) (gen.ApplyResourceJSONRequestBody, error) {
	annotations := map[string]string{
		string(AnnotationKeyDisplayName): req.DisplayName,
		string(AnnotationKeyDescription): req.Description,
	}
	labels := map[string]string{
		string(LabelKeyProvisioningType): string(req.ProvisioningType),
	}
	componentType, err := getOpenChoreoComponentType(string(req.ProvisioningType), req.AgentType.Type)
	if err != nil {
		return nil, err
	}
	componentCR := gen.ApplyResourceJSONRequestBody{
		"apiVersion": ResourceAPIVersion,
		"kind":       ResourceKindComponent,
		"metadata": map[string]interface{}{
			"name":        req.Name,
			"namespace":   orgName,
			"annotations": annotations,
			"labels":      labels,
		},
		"spec": map[string]interface{}{
			"componentType": componentType,
			"owner": map[string]interface{}{
				"projectName": projectName,
			},
		},
	}
	return componentCR, nil
}

func createComponentCRForInternalAgents(orgName, projectName string, req CreateComponentRequest) (gen.ApplyResourceJSONRequestBody, error) {
	annotations := map[string]string{
		string(AnnotationKeyDisplayName): req.DisplayName,
		string(AnnotationKeyDescription): req.Description,
	}
	labels := map[string]string{
		string(LabelKeyProvisioningType):     string(req.ProvisioningType),
		string(LabelKeyAgentSubType):         req.AgentType.SubType,
		string(LabelKeyAgentLanguage):        req.RuntimeConfigs.Language,
		string(LabelKeyAgentLanguageVersion): req.RuntimeConfigs.LanguageVersion,
	}
	componentType, err := getOpenChoreoComponentType(string(req.ProvisioningType), req.AgentType.Type)
	if err != nil {
		return nil, err
	}
	componentWorkflow := getWorkflowName(req.RuntimeConfigs.Language)
	containerPort, basePath := getInputInterfaceConfig(req)

	// Create parameters as RawExtension
	parameters := map[string]interface{}{
		"exposed":  true,
		"replicas": DefaultReplicaCount,
		"port":     containerPort,
		"resources": map[string]interface{}{
			"requests": map[string]string{
				"cpu":    DefaultCPURequest,
				"memory": DefaultMemoryRequest,
			},
			"limits": map[string]string{
				"cpu":    DefaultCPULimit,
				"memory": DefaultMemoryLimit,
			},
		},
		"basePath": basePath,
		"cors": map[string]interface{}{
			"allowOrigin":  strings.Split(config.GetAgentWorkloadConfig().CORS.AllowOrigin, ","),
			"allowMethods": strings.Split(config.GetAgentWorkloadConfig().CORS.AllowMethods, ","),
			"allowHeaders": strings.Split(config.GetAgentWorkloadConfig().CORS.AllowHeaders, ","),
		},
	}
	componentWorkflowParameters, err := buildWorkflowParameters(req)
	if err != nil {
		return nil, fmt.Errorf("error building workflow parameters: %w", err)
	}
	// Build the ApplyResource request body
	componentCR := gen.ApplyResourceJSONRequestBody{
		"apiVersion": ResourceAPIVersion,
		"kind":       ResourceKindComponent,
		"metadata": map[string]interface{}{
			"name":        req.Name,
			"namespace":   orgName,
			"annotations": annotations,
			"labels":      labels,
		},
		"spec": map[string]interface{}{
			"componentType": componentType,
			"owner": map[string]interface{}{
				"projectName": projectName,
			},
			"autoDeploy": true,
			"parameters": parameters,
			"workflow": map[string]interface{}{
				"name": string(componentWorkflow),
				"systemParameters": map[string]interface{}{
					"repository": map[string]interface{}{
						"url": req.Repository.URL,
						"revision": map[string]interface{}{
							"branch": req.Repository.Branch,
						},
						"appPath": normalizePath(req.Repository.AppPath),
					},
				},
				"parameters": componentWorkflowParameters,
			},
		},
	}
	return componentCR, nil
}

func getOpenChoreoComponentType(provisioningType string, agentType string) (ComponentType, error) {
	if provisioningType == string(utils.ExternalAgent) {
		return ComponentTypeExternalAgentAPI, nil
	}
	if provisioningType == string(utils.InternalAgent) && agentType == string(utils.AgentTypeAPI) {
		return ComponentTypeInternalAgentAPI, nil
	}
	// agent type is already validated in controller layer
	return "", fmt.Errorf("invalid provisioning type or agent type")
}

// -----------------------------------------------------------------------------
// Workflow parameter builders
// -----------------------------------------------------------------------------

func getWorkflowName(language string) string {
	for _, bp := range utils.Buildpacks {
		if bp.Language == language {
			if bp.Provider == string(utils.BuildPackProviderGoogle) {
				return WorkflowNameGoogleCloudBuildpacks
			}
			if bp.Provider == string(utils.BuildPackProviderAMPBallerina) {
				return WorkflowNameBallerinaBuilpack
			}
		}
	}
	return ""
}

func buildWorkflowParameters(req CreateComponentRequest) (map[string]any, error) {
	var buildpackConfigs map[string]any
	if isGoogleBuildpack(req.RuntimeConfigs.Language) {
		buildpackConfigs = map[string]any{
			"language":           req.RuntimeConfigs.Language,
			"languageVersion":    req.RuntimeConfigs.LanguageVersion,
			"googleEntryPoint":   req.RuntimeConfigs.RunCommand,
			"languageVersionKey": getLanguageVersionEnvVariable(req.RuntimeConfigs.Language),
		}
	} else {
		buildpackConfigs = map[string]any{
			"language": req.RuntimeConfigs.Language,
		}
	}

	endpoints, err := buildEndpoints(req)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"buildpackConfigs":     buildpackConfigs,
		"endpoints":            endpoints,
		"environmentVariables": buildEnvironmentVariables(req),
	}, nil
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
			"schemaType":    SchemaTypeREST,
			"schemaContent": schemaContent,
		})
	}

	if req.AgentType.Type == string(utils.AgentTypeAPI) && req.AgentType.SubType == string(utils.AgentSubTypeCustomAPI) && req.InputInterface != nil {
		endpoints = append(endpoints, map[string]any{
			"name":           fmt.Sprintf("%s-endpoint", req.Name),
			"port":           req.InputInterface.Port,
			"type":           req.InputInterface.Type,
			"schemaType":     "REST",
			"schemaFilePath": normalizePath(req.InputInterface.SchemaPath),
		})
	}

	return endpoints, nil
}

func buildEnvironmentVariables(req CreateComponentRequest) []map[string]any {
	envVars := make([]map[string]any, 0)
	if req.RuntimeConfigs != nil {
		for _, env := range req.RuntimeConfigs.Env {
			envVars = append(envVars, map[string]any{
				"name":  env.Key,
				"value": env.Value,
			})
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
	resp, err := c.ocClient.GetComponentWithResponse(ctx, namespaceName, projectName, componentName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
		})
	}

	if resp.JSON200 == nil || resp.JSON200.Data == nil {
		return nil, fmt.Errorf("empty response from get component")
	}

	return convertComponent(resp.JSON200.Data), nil
}

// getCleanResourceCR fetches a resource CR and removes server-managed fields
func (c *openChoreoClient) getCleanResourceCR(ctx context.Context, namespaceName, kind, resourceName string, notFoundErr error) (map[string]interface{}, error) {
	resp, err := c.ocClient.GetResourceWithResponse(ctx, namespaceName, kind, resourceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		body := resp.Body
		return nil, handleErrorResponse(resp.StatusCode(), body, ErrorContext{
			NotFoundErr: notFoundErr,
		})
	}

	if resp.JSON200 == nil || resp.JSON200.Data == nil {
		return nil, fmt.Errorf("empty response from get resource")
	}

	// Get the component CR data
	componentCR := *resp.JSON200.Data

	// Remove server-managed fields from metadata
	if metadata, ok := componentCR["metadata"].(map[string]interface{}); ok {
		delete(metadata, "managedFields")
		delete(metadata, "resourceVersion")
		delete(metadata, "generation")
		delete(metadata, "creationTimestamp")
		delete(metadata, "uid")
	}

	// Remove status field (server-managed)
	delete(componentCR, "status")

	return componentCR, nil
}

func (c *openChoreoClient) UpdateComponentBasicInfo(ctx context.Context, namespaceName, projectName, componentName string, req UpdateComponentBasicInfoRequest) error {
	// Fetch the full component CR with server-managed fields removed
	componentCR, err := c.getCleanResourceCR(ctx, namespaceName, ResourceKindComponent, componentName, utils.ErrAgentNotFound)
	if err != nil {
		return fmt.Errorf("failed to get component resource: %w", err)
	}

	// Update annotations in the metadata
	metadata, ok := componentCR["metadata"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid metadata in component CR")
	}

	annotations, ok := metadata["annotations"].(map[string]interface{})
	if !ok {
		annotations = make(map[string]interface{})
		metadata["annotations"] = annotations
	}

	annotations[string(AnnotationKeyDisplayName)] = req.DisplayName
	annotations[string(AnnotationKeyDescription)] = req.Description

	// Apply the updated component CR
	applyResp, err := c.ocClient.ApplyResourceWithResponse(ctx, componentCR)
	if err != nil {
		return fmt.Errorf("failed to update component meta details: %w", err)
	}

	if applyResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(applyResp.StatusCode(), applyResp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
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
	// Fetch the full component CR with server-managed fields removed
	componentCR, err := c.getCleanResourceCR(ctx, namespaceName, ResourceKindComponent, componentName, utils.ErrAgentNotFound)
	if err != nil {
		return fmt.Errorf("failed to get component resource: %w", err)
	}

	// Get or create spec
	spec, ok := componentCR["spec"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid spec in component CR")
	}

	// Get or create parameters
	parameters, ok := spec["parameters"].(map[string]interface{})
	if !ok {
		parameters = make(map[string]interface{})
		spec["parameters"] = parameters
	}

	// Update replicas if provided
	if req.Replicas != nil {
		parameters["replicas"] = *req.Replicas
	}

	// Update resources if provided
	if req.Resources != nil {
		resources := make(map[string]interface{})

		if req.Resources.Requests != nil {
			requests := make(map[string]string)
			if req.Resources.Requests.CPU != "" {
				requests["cpu"] = req.Resources.Requests.CPU
			}
			if req.Resources.Requests.Memory != "" {
				requests["memory"] = req.Resources.Requests.Memory
			}
			if len(requests) > 0 {
				resources["requests"] = requests
			}
		}

		if req.Resources.Limits != nil {
			limits := make(map[string]string)
			if req.Resources.Limits.CPU != "" {
				limits["cpu"] = req.Resources.Limits.CPU
			}
			if req.Resources.Limits.Memory != "" {
				limits["memory"] = req.Resources.Limits.Memory
			}
			if len(limits) > 0 {
				resources["limits"] = limits
			}
		}

		if len(resources) > 0 {
			parameters["resources"] = resources
		}
	}

	// Apply the updated component CR using ApplyResource instead of PatchComponent
	// This avoids the OpenChoreo bug in applyComponentPatch
	applyResp, err := c.ocClient.ApplyResourceWithResponse(ctx, componentCR)
	if err != nil {
		return fmt.Errorf("failed to update component resource configurations: %w", err)
	}

	if applyResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(applyResp.StatusCode(), applyResp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
		})
	}

	return nil
}

// updateReleaseBindingResourceConfigs updates environment-specific parameters via release binding
func (c *openChoreoClient) updateReleaseBindingResourceConfigs(ctx context.Context, namespaceName, projectName, componentName, environment string, req UpdateComponentResourceConfigsRequest) error {
	// List release bindings to find the correct binding name for the environment
	listResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, projectName, componentName)
	if err != nil {
		return fmt.Errorf("failed to list release bindings: %w", err)
	}

	if listResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(listResp.StatusCode(), listResp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
		})
	}

	// Find the binding for the specified environment
	var bindingName string
	if listResp.JSON200 != nil && listResp.JSON200.Data != nil && listResp.JSON200.Data.Items != nil {
		for _, binding := range *listResp.JSON200.Data.Items {
			if binding.Environment == environment {
				bindingName = binding.Name
				break
			}
		}
	}

	if bindingName == "" {
		return fmt.Errorf("release binding not found for environment: %s", environment)
	}

	// Build componentTypeEnvOverrides with resources and replicas
	componentTypeEnvOverrides := make(map[string]interface{})

	// Add replicas if provided
	if req.Replicas != nil {
		componentTypeEnvOverrides["replicas"] = *req.Replicas
	}

	if req.Resources != nil {
		resources := make(map[string]interface{})
		if req.Resources.Requests != nil {
			requests := make(map[string]string)
			if req.Resources.Requests.CPU != "" {
				requests["cpu"] = req.Resources.Requests.CPU
			}
			if req.Resources.Requests.Memory != "" {
				requests["memory"] = req.Resources.Requests.Memory
			}
			if len(requests) > 0 {
				resources["requests"] = requests
			}
		}

		if req.Resources.Limits != nil {
			limits := make(map[string]string)
			if req.Resources.Limits.CPU != "" {
				limits["cpu"] = req.Resources.Limits.CPU
			}
			if req.Resources.Limits.Memory != "" {
				limits["memory"] = req.Resources.Limits.Memory
			}
			if len(limits) > 0 {
				resources["limits"] = limits
			}
		}

		if len(resources) > 0 {
			componentTypeEnvOverrides["resources"] = resources
		}
	}

	// Build the patch request body
	patchBody := gen.PatchReleaseBindingJSONRequestBody{
		ComponentTypeEnvOverrides: &componentTypeEnvOverrides,
	}

	// Use PatchReleaseBinding
	resp, err := c.ocClient.PatchReleaseBindingWithResponse(ctx, namespaceName, projectName, componentName, bindingName, patchBody)
	if err != nil {
		return fmt.Errorf("failed to patch release binding resource configurations: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
		})
	}

	return nil
}

// getComponentLevelResourceConfigs fetches component-level default resource configurations
func (c *openChoreoClient) getComponentLevelResourceConfigs(ctx context.Context, namespaceName, projectName, componentName string) (*ComponentResourceConfigsResponse, error) {
	// Get the component CR to extract parameters
	componentCR, err := c.getCleanResourceCR(ctx, namespaceName, ResourceKindComponent, componentName, utils.ErrAgentNotFound)
	if err != nil {
		return nil, fmt.Errorf("failed to get component resource: %w", err)
	}

	response := &ComponentResourceConfigsResponse{}

	// Extract parameters from component spec
	if spec, ok := componentCR["spec"].(map[string]interface{}); ok {
		if parameters, ok := spec["parameters"].(map[string]interface{}); ok {
			// Extract replicas
			if replicas, ok := parameters["replicas"].(float64); ok {
				replicasInt := int32(replicas)
				response.Replicas = &replicasInt
			}

			// Extract resources
			if resources, ok := parameters["resources"].(map[string]interface{}); ok {
				response.Resources = extractResourceConfig(resources)
			}
		}
	}

	return response, nil
}

// getEnvironmentResourceConfigs fetches environment-specific resource configurations along with component defaults
func (c *openChoreoClient) getEnvironmentResourceConfigs(ctx context.Context, namespaceName, projectName, componentName, environment string) (*ComponentResourceConfigsResponse, error) {
	// First, get component-level defaults
	componentDefaults, err := c.getComponentLevelResourceConfigs(ctx, namespaceName, projectName, componentName)
	if err != nil {
		return nil, err
	}

	// List release bindings to find the one for this environment
	listResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, projectName, componentName)
	if err != nil {
		return nil, fmt.Errorf("failed to list release bindings: %w", err)
	}

	if listResp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(listResp.StatusCode(), listResp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
		})
	}

	response := &ComponentResourceConfigsResponse{
		DefaultReplicas:  componentDefaults.Replicas,
		DefaultResources: componentDefaults.Resources,
	}

	// Find the binding for the specified environment
	var binding *gen.ReleaseBindingResponse
	if listResp.JSON200 != nil && listResp.JSON200.Data != nil && listResp.JSON200.Data.Items != nil {
		for _, b := range *listResp.JSON200.Data.Items {
			if b.Environment == environment {
				binding = &b
				break
			}
		}
	}

	if binding == nil {
		// No binding found - return component defaults
		isOverridden := false
		response.Replicas = componentDefaults.Replicas
		response.Resources = componentDefaults.Resources
		response.IsDefaultsOverridden = &isOverridden
		return response, nil
	}

	// Check if there are overrides in componentTypeEnvOverrides
	hasOverrides := false
	if binding.ComponentTypeEnvOverrides != nil {
		overrides := *binding.ComponentTypeEnvOverrides

		// Check for replicas override
		if replicas, ok := overrides["replicas"].(float64); ok {
			replicasInt := int32(replicas)
			response.Replicas = &replicasInt
			hasOverrides = true
		} else {
			// Use component default
			response.Replicas = componentDefaults.Replicas
		}

		// Check for resources override
		if resources, ok := overrides["resources"].(map[string]interface{}); ok {
			response.Resources = extractResourceConfig(resources)
			hasOverrides = true
		} else {
			// Use component default
			response.Resources = componentDefaults.Resources
		}
	} else {
		// No overrides - use component defaults
		response.Replicas = componentDefaults.Replicas
		response.Resources = componentDefaults.Resources
	}

	response.IsDefaultsOverridden = &hasOverrides
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

func (c *openChoreoClient) DeleteComponent(ctx context.Context, namespaceName, projectName, componentName string) error {
	resp, err := c.ocClient.DeleteComponentWithResponse(ctx, namespaceName, projectName, componentName)
	if err != nil {
		return fmt.Errorf("failed to delete component: %w", err)
	}

	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusNoContent {
		return handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
		})
	}

	return nil
}

func (c *openChoreoClient) ListComponents(ctx context.Context, namespaceName, projectName string) ([]*models.AgentResponse, error) {
	resp, err := c.ocClient.ListComponentsWithResponse(ctx, namespaceName, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to list components: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{
			NotFoundErr: utils.ErrProjectNotFound,
		})
	}

	if resp.JSON200 == nil || resp.JSON200.Data == nil || resp.JSON200.Data.Items == nil {
		return []*models.AgentResponse{}, nil
	}

	items := *resp.JSON200.Data.Items
	components := make([]*models.AgentResponse, len(items))
	for i, comp := range items {
		components[i] = convertComponent(&comp)
	}
	return components, nil
}

func (c *openChoreoClient) ComponentExists(ctx context.Context, namespaceName, projectName, componentName string, verifyProject bool) (bool, error) {
	_, err := c.GetComponent(ctx, namespaceName, projectName, componentName)
	if err != nil {
		if errors.Is(err, utils.ErrAgentNotFound) {
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

func (c *openChoreoClient) AttachTrait(ctx context.Context, namespaceName, projectName, componentName string, traitType TraitType) error {
	// Get the current traits for the component
	listResp, err := c.ocClient.ListComponentTraitsWithResponse(ctx, namespaceName, projectName, componentName)
	if err != nil {
		return fmt.Errorf("failed to list component traits: %w", err)
	}

	if listResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(listResp.StatusCode(), listResp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
		})
	}

	// Build the new traits list including the new trait
	var traits []gen.ComponentTraitRequest

	// Parse existing traits from the generic response
	if listResp.JSON200 != nil && listResp.JSON200.Data != nil && listResp.JSON200.Data.Items != nil {
		for _, item := range *listResp.JSON200.Data.Items {
			name, _ := item["name"].(string)
			instanceName, _ := item["instanceName"].(string)
			if name == string(traitType) {
				// Trait already exists, no need to add
				return nil
			}
			trait := gen.ComponentTraitRequest{
				Name:         name,
				InstanceName: instanceName,
			}
			if params, ok := item["parameters"].(map[string]interface{}); ok {
				trait.Parameters = &params
			}
			traits = append(traits, trait)
		}
	}

	// Add the new trait with type-specific parameters
	newTrait, err := c.buildTraitRequest(ctx, namespaceName, projectName, componentName, traitType)
	if err != nil {
		return fmt.Errorf("failed to build trait request: %w", err)
	}
	traits = append(traits, newTrait)

	// Update traits
	updateReq := gen.UpdateComponentTraitsJSONRequestBody{
		Traits: traits,
	}

	updateResp, err := c.ocClient.UpdateComponentTraitsWithResponse(ctx, namespaceName, projectName, componentName, updateReq)
	if err != nil {
		return fmt.Errorf("failed to update component traits: %w", err)
	}

	if updateResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(updateResp.StatusCode(), updateResp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
		})
	}

	return nil
}

func (c *openChoreoClient) buildTraitRequest(ctx context.Context, namespaceName, projectName, componentName string, traitType TraitType) (gen.ComponentTraitRequest, error) {
	trait := gen.ComponentTraitRequest{
		Name:         string(traitType),
		InstanceName: fmt.Sprintf("%s-%s", componentName, string(traitType)),
	}
	if traitType == TraitOTELInstrumentation {
		params, err := c.buildOTELTraitParameters(ctx, namespaceName, projectName, componentName)
		if err != nil {
			return gen.ComponentTraitRequest{}, err
		}
		trait.Parameters = &params
	}
	return trait, nil
}

func (c *openChoreoClient) buildOTELTraitParameters(ctx context.Context, namespaceName, projectName, componentName string) (map[string]interface{}, error) {
	// Get the component to retrieve UUID and language version
	component, err := c.GetComponent(ctx, namespaceName, projectName, componentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get component for trait attachment: %w", err)
	}
	languageVersion := ""
	if component.RuntimeConfigs != nil {
		languageVersion = component.RuntimeConfigs.LanguageVersion
	}

	// Get the project to find the deployment pipeline
	project, err := c.GetProject(ctx, namespaceName, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get project for trait attachment: %w", err)
	}
	if project.DeploymentPipeline == "" {
		return nil, fmt.Errorf("failed to attach trait: project %s does not have a deployment pipeline configured", projectName)
	}

	// Get the deployment pipeline to find the lowest environment
	pipeline, err := c.GetProjectDeploymentPipeline(ctx, namespaceName, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment pipeline for trait attachment: %w", err)
	}
	lowestEnvName := findLowestEnvironment(pipeline.PromotionPaths)

	// Get the environment UUID
	env, err := c.GetEnvironment(ctx, namespaceName, lowestEnvName)
	if err != nil {
		return nil, fmt.Errorf("failed to get environment for trait attachment: %w", err)
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
		"traceAttributes":       fmt.Sprintf("%s=%s,%s=%s", TraceAttributeKeyEnvironment, env.UUID, TraceAttributeKeyComponent, component.UUID),
		"agentApiKey":           uuid.New().String(),
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

func findLowestEnvironment(promotionPaths []models.PromotionPath) string {
	if len(promotionPaths) == 0 {
		return ""
	}
	targets := make(map[string]bool)
	for _, path := range promotionPaths {
		for _, target := range path.TargetEnvironmentRefs {
			targets[target.Name] = true
		}
	}
	for _, path := range promotionPaths {
		if !targets[path.SourceEnvironmentRef] {
			return path.SourceEnvironmentRef
		}
	}
	return ""
}

func (c *openChoreoClient) GetComponentEndpoints(ctx context.Context, namespaceName, projectName, componentName, environment string) (map[string]models.EndpointsResponse, error) {
	// Get the workload to extract endpoint schema
	workloadResp, err := c.ocClient.GetWorkloadsWithResponse(ctx, namespaceName, projectName, componentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload: %w", err)
	}

	if workloadResp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(workloadResp.StatusCode(), workloadResp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
		})
	}

	// Get the environment release to extract endpoint URLs
	releaseResp, err := c.ocClient.GetEnvironmentReleaseWithResponse(ctx, namespaceName, projectName, componentName, environment)
	if err != nil {
		return nil, fmt.Errorf("failed to get environment release: %w", err)
	}

	if releaseResp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(releaseResp.StatusCode(), releaseResp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
		})
	}

	// Extract endpoint URLs from the release
	var endpoints []models.Endpoint
	if releaseResp.JSON200 != nil && releaseResp.JSON200.Data != nil {
		endpoints, err = extractEndpointURLsFromRelease(releaseResp.JSON200.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to extract endpoint URLs from release: %w", err)
		}
	}

	// Extract endpoint details from workload spec
	endpointDetails := make(map[string]models.EndpointsResponse)

	// Get endpoints from workload spec
	if workloadResp.JSON200 != nil && workloadResp.JSON200.Data != nil && workloadResp.JSON200.Data.Endpoints != nil {
		for endpointName, endpoint := range *workloadResp.JSON200.Data.Endpoints {
			details := models.EndpointsResponse{
				Endpoint: models.Endpoint{
					Name: endpointName,
				},
			}

			// Set URL from release if available
			if len(endpoints) > 0 {
				details.URL = endpoints[0].URL
				details.Visibility = endpoints[0].Visibility
			}

			// Get schema content from workload endpoint
			if endpoint.Schema != nil && endpoint.Schema.Content != nil {
				details.Schema = models.EndpointSchema{Content: *endpoint.Schema.Content}
			}

			endpointDetails[endpointName] = details
		}
	}

	return endpointDetails, nil
}

func (c *openChoreoClient) GetComponentConfigurations(ctx context.Context, namespaceName, projectName, componentName, environment string) ([]models.EnvVars, error) {
	// Get the workload to extract base environment variables
	workloadResp, err := c.ocClient.GetWorkloadsWithResponse(ctx, namespaceName, projectName, componentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload: %w", err)
	}

	if workloadResp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(workloadResp.StatusCode(), workloadResp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
		})
	}

	// Create a map to store environment variables (for easy merging)
	envVarMap := make(map[string]string)

	// Extract base environment variables from workload
	if workloadResp.JSON200 != nil && workloadResp.JSON200.Data != nil && workloadResp.JSON200.Data.Containers != nil {
		if mainContainer, ok := (*workloadResp.JSON200.Data.Containers)[MainContainerName]; ok {
			if mainContainer.Env != nil {
				for _, env := range *mainContainer.Env {
					envVarMap[env.Key] = utils.StrPointerAsStr(env.Value, "")
				}
			}
		}
	}

	// Get the ReleaseBinding for the specified environment to get overrides
	releaseBindingResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, projectName, componentName)
	if err != nil {
		return nil, fmt.Errorf("failed to list release bindings: %w", err)
	}

	if releaseBindingResp.StatusCode() == http.StatusOK && releaseBindingResp.JSON200 != nil &&
		releaseBindingResp.JSON200.Data != nil && releaseBindingResp.JSON200.Data.Items != nil {
		// Find the binding for the specified environment
		for _, binding := range *releaseBindingResp.JSON200.Data.Items {
			if binding.Environment == environment {
				// Extract workload overrides from binding
				if binding.WorkloadOverrides != nil && binding.WorkloadOverrides.Containers != nil {
					if mainContainer, ok := (*binding.WorkloadOverrides.Containers)[MainContainerName]; ok {
						if mainContainer.Env != nil {
							for _, env := range *mainContainer.Env {
								envVarMap[env.Key] = utils.StrPointerAsStr(env.Value, "")
							}
						}
					}
				}
				break
			}
		}
	}

	// Convert map back to slice
	var envVars []models.EnvVars
	for key, value := range envVarMap {
		envVars = append(envVars, models.EnvVars{
			Key:   key,
			Value: value,
		})
	}

	return envVars, nil
}

// -----------------------------------------------------------------------------
// Helper functions
// -----------------------------------------------------------------------------

// convertComponent converts an gen.ComponentResponse to models.AgentResponse
func convertComponent(comp *gen.ComponentResponse) *models.AgentResponse {
	if comp == nil {
		return nil
	}

	provisioningType := string(ProvisioningInternal)
	if comp.Type == string(ComponentTypeExternalAgentAPI) {
		provisioningType = string(ProvisioningExternal)
	}

	agent := &models.AgentResponse{
		UUID:        comp.Uid,
		Name:        comp.Name,
		DisplayName: utils.StrPointerAsStr(comp.DisplayName, ""),
		Description: utils.StrPointerAsStr(comp.Description, ""),
		ProjectName: comp.ProjectName,
		Status:      utils.StrPointerAsStr(comp.Status, ""),
		CreatedAt:   comp.CreatedAt,
		Provisioning: models.Provisioning{
			Type: provisioningType,
		},
	}

	// Extract details from componentWorkflow if present
	if comp.ComponentWorkflow != nil {
		extractComponentWorkflowDetails(agent, comp.ComponentWorkflow)
	}

	// Temporary workaround: Extract agent type from component type (until OC API supports labels)
	// Component type format: "deployment/agent-api" or "deployment/external-agent-api"
	if comp.Type != "" {
		parts := strings.Split(comp.Type, "/")
		if len(parts) >= 2 {
			agent.Type.Type = parts[1]
		}
	}

	// Temporary workaround: Determine subtype based on schema presence
	// If schema path exists, it's a custom-api, otherwise chat-api
	if provisioningType == string(ProvisioningInternal) {
		if agent.InputInterface != nil && agent.InputInterface.Schema != nil && agent.InputInterface.Schema.Path != "" {
			agent.Type.SubType = string(utils.AgentSubTypeCustomAPI)
		} else {
			agent.Type.SubType = string(utils.AgentSubTypeChatAPI)
		}
	}

	return agent
}

func extractComponentWorkflowDetails(agent *models.AgentResponse, workflow *gen.ComponentWorkflow) {
	if workflow.Parameters == nil {
		return
	}

	params := *workflow.Parameters

	// Extract buildpackConfigs
	if buildpackConfigs, ok := params["buildpackConfigs"].(map[string]interface{}); ok {
		if agent.RuntimeConfigs == nil {
			agent.RuntimeConfigs = &models.RuntimeConfigs{}
		}
		if language, ok := buildpackConfigs["language"].(string); ok {
			agent.RuntimeConfigs.Language = language
		}
		if langVersion, ok := buildpackConfigs["languageVersion"].(string); ok {
			agent.RuntimeConfigs.LanguageVersion = langVersion
		}
		// googleEntryPoint is the run command for Google buildpacks
		if runCmd, ok := buildpackConfigs["googleEntryPoint"].(string); ok {
			agent.RuntimeConfigs.RunCommand = runCmd
		}
	}

	// Extract endpoint/input interface info
	if endpoints, ok := params["endpoints"].([]interface{}); ok && len(endpoints) > 0 {
		if endpoint, ok := endpoints[0].(map[string]interface{}); ok {
			agent.InputInterface = &models.InputInterface{}
			if port, ok := endpoint["port"].(float64); ok {
				agent.InputInterface.Port = int32(port)
			}
			if interfaceType, ok := endpoint["type"].(string); ok {
				agent.InputInterface.Type = interfaceType
			}
			// Extract schema file path only
			if schemaPath, ok := endpoint["schemaFilePath"].(string); ok {
				if agent.InputInterface.Schema == nil {
					agent.InputInterface.Schema = &models.InputInterfaceSchema{}
				}
				agent.InputInterface.Schema.Path = schemaPath
			}
		}
	}

	// Extract git repository info from systemParameters
	repo := workflow.SystemParameters.Repository
	agent.Provisioning.Repository = models.Repository{
		Url:     repo.Url,
		AppPath: utils.StrPointerAsStr(repo.AppPath, ""),
		Branch:  repo.Revision.Branch,
	}
}
