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
	"path"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func (c *openChoreoClient) CreateComponent(ctx context.Context, ouID, projectName string, req CreateComponentRequest) error {
	namespaceName := c.NamespaceFor(ouID)
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
	if req.AgentKind != nil {
		return buildInternalAgentFromKindComponentRequestBody(namespaceName, projectName, req)
	}
	return buildInternalAgentFromSourceComponentRequestBody(namespaceName, projectName, req)
}

// buildInternalAgentFromKindComponentRequestBody creates a component for an internal agent
// sourced from a published Agent Kind version. Uses a pre-built image — no source build step.
func buildInternalAgentFromKindComponentRequestBody(namespaceName, projectName string, req CreateComponentRequest) (gen.CreateComponentJSONRequestBody, error) {
	annotations := map[string]string{
		string(AnnotationKeyDisplayName): req.DisplayName,
		string(AnnotationKeyDescription): req.Description,
	}
	// Both the kind name and version are recorded so an agent can be traced back to
	// the exact published kind version it was instantiated from. Version tags are
	// free-form, and a tag that isn't a legal label value would be rejected by the
	// API server with an opaque error — so it's checked here and reported as a
	// validation error naming the tag, never dropped to make creation succeed with
	// an incomplete provenance record.
	if err := utils.ValidateLabelValue(req.AgentKind.Version, "agent kind version"); err != nil {
		return gen.CreateComponentJSONRequestBody{}, fmt.Errorf("invalid agent kind version: %w", err)
	}
	labels := map[string]string{
		string(LabelKeyProvisioningType): string(ProvisioningInternal),
		string(LabelKeyAgentSubType):     req.AgentType.SubType,
		string(LabelKeyBuildSource):      BuildSourceKind,
		string(LabelKeyAgentKindName):    req.AgentKind.Name,
		string(LabelKeyAgentKindVersion): req.AgentKind.Version,
	}

	// Mirror the same language label logic as buildInternalAgentFromSourceComponentRequestBody
	if req.Build != nil && req.Build.Buildpack != nil {
		labels[string(LabelKeyAgentLanguage)] = req.Build.Buildpack.Language
		if req.Build.Buildpack.LanguageVersion != "" {
			labels[string(LabelKeyAgentLanguageVersion)] = req.Build.Buildpack.LanguageVersion
		}
	}
	if req.Build != nil && req.Build.Docker != nil {
		labels[string(LabelKeyAgentLanguage)] = "docker"
	}
	addUserLabels(labels, req.Labels)

	componentTypeKind := gen.ComponentSpecComponentTypeKindComponentType
	// The kind's source component build (incl. buildpack language) is resolved
	// upstream and carried on req.Build, so resource parameters are derived the
	// same way as source-based agents.
	resourceParams, err := buildpackResourceParameters(req.Build)
	if err != nil {
		return gen.CreateComponentJSONRequestBody{}, fmt.Errorf("failed to determine resource parameters: %w", err)
	}
	defaultParams := ComponentParameters{
		Exposed:   true,
		Resources: resourceParams,
		RoutePath: req.Name,
	}
	parameters, err := structToMap(defaultParams)
	if err != nil {
		return gen.CreateComponentJSONRequestBody{}, fmt.Errorf("failed to convert parameters to map: %w", err)
	}

	// Kind-sourced agents have no build workflow, so nothing runs amp-generate-workload
	// to cut the ComponentRelease and bind it to the first environment. The backend does
	// that itself instead (EnsureReleaseAndBinding), for the same reason the
	// workflow does it for source-built agents: configuration must land on the
	// per-environment ReleaseBinding rather than the shared Workload, and autoDeploy's
	// controller-created binding carries no overrides. Both agent kinds therefore set
	// this to false and own the release/binding themselves.
	autoDeploy := false
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
				Name: string(ComponentTypeInternalAgentAPI),
			},
			Owner: struct {
				ProjectName string `json:"projectName"`
			}{
				ProjectName: projectName,
			},
			AutoDeploy: &autoDeploy,
			Parameters: &parameters,
			// No Workflow: kind-sourced agents use a directly created Workload CR instead of a build workflow.
		},
	}, nil
}

func buildExternalAgentComponentRequestBody(namespaceName, projectName string, req CreateComponentRequest) (gen.CreateComponentJSONRequestBody, error) {
	annotations := map[string]string{
		string(AnnotationKeyDisplayName): req.DisplayName,
		string(AnnotationKeyDescription): req.Description,
	}
	labels := map[string]string{
		string(LabelKeyProvisioningType): string(req.ProvisioningType),
	}
	addUserLabels(labels, req.Labels)
	componentTypeKind := gen.ComponentSpecComponentTypeKindComponentType
	componentType, err := getOpenChoreoComponentType(string(req.ProvisioningType), req.AgentType.Type)
	if err != nil {
		return gen.CreateComponentJSONRequestBody{}, err
	}

	// An external agent has no Workload and never deploys, so autoDeploy has nothing to act on.
	// Stated rather than left to the field's default, so all three component builders answer the
	// question in the same place.
	autoDeploy := false
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
		},
	}, nil
}

func buildInternalAgentFromSourceComponentRequestBody(namespaceName, projectName string, req CreateComponentRequest) (gen.CreateComponentJSONRequestBody, error) {
	annotations := map[string]string{
		string(AnnotationKeyDisplayName): req.DisplayName,
		string(AnnotationKeyDescription): req.Description,
	}
	buildSource := BuildSourceBuildpack
	if req.Build != nil && req.Build.Docker != nil {
		buildSource = BuildSourceDocker
	}
	labels := map[string]string{
		string(LabelKeyProvisioningType): string(req.ProvisioningType),
		string(LabelKeyAgentSubType):     req.AgentType.SubType,
		string(LabelKeyBuildSource):      buildSource,
	}

	// Add buildpack language labels if applicable
	if req.Build != nil && req.Build.Buildpack != nil {
		labels[string(LabelKeyAgentLanguage)] = req.Build.Buildpack.Language
		if req.Build.Buildpack.LanguageVersion != "" {
			labels[string(LabelKeyAgentLanguageVersion)] = req.Build.Buildpack.LanguageVersion
		}
	}
	// Add docker build specific labels if applicable
	if req.Build != nil && req.Build.Docker != nil {
		labels[string(LabelKeyAgentLanguage)] = "docker"
	}
	addUserLabels(labels, req.Labels)
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
	resourceParams, err := buildpackResourceParameters(req.Build)
	if err != nil {
		return gen.CreateComponentJSONRequestBody{}, fmt.Errorf("failed to determine resource parameters: %w", err)
	}
	defaultParams := ComponentParameters{
		Exposed:   true,
		Resources: resourceParams,
		RoutePath: req.Name,
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

	// The build workflow's generate-workload step cuts the ComponentRelease and binds
	// it to the first environment itself, and every deploy afterwards does the same via
	// EnsureReleaseAndBinding. That is what lets each environment's configuration live on
	// its own ReleaseBinding instead of on the shared Workload. autoDeploy would have
	// OpenChoreo's Component controller create and own that same binding, so the two would
	// fight over spec.releaseName.
	autoDeploy := false
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
				Kind:       &componentWorkflowKind,
				Name:       componentWorkflowName,
				Parameters: &componentWorkflowParameters,
			},
		},
	}, nil
}

// componentWorkflowKind is the kind recorded on a Component's workflow reference.
//
// The namespaced Workflow is used rather than the cluster-scoped ClusterWorkflow:
// a ClusterWorkflow pins workflowPlaneRef to the single cluster-wide
// ClusterWorkflowPlane, so its builds always run on that one plane. A namespaced
// Workflow resolves the WorkflowPlane inside the organization's own namespace,
// which is what lets an org build on the data centre it actually belongs to.
//
// Recorded on the Component so each agent keeps the kind it was created with:
// builds.go reads this and only falls back to ClusterWorkflow when it is unset,
// which is the case for agents created before this change.
var componentWorkflowKind = gen.ComponentWorkflowConfigKindWorkflow

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

// defaultResourceParameters returns the platform default resource requests/limits.
func defaultResourceParameters() *ResourceConfig {
	return &ResourceConfig{
		Requests: &ResourceRequests{
			CPU:    DefaultCPURequest,
			Memory: DefaultMemoryRequest,
		},
		Limits: &ResourceLimits{
			CPU:    DefaultCPULimit,
			Memory: DefaultMemoryLimit,
		},
	}
}

// buildpackResourceParameters returns the component-level resource parameters to
// pin at creation time for the given build. Ballerina agents need a higher
// baseline than the platform defaults, so their requests/limits are set
// explicitly. Docker builds and all other buildpack languages get the platform
// defaults. Returns an error if the build has neither buildpack nor docker
// configuration.
func buildpackResourceParameters(build *BuildConfig) (*ResourceConfig, error) {
	if build == nil || (build.Buildpack == nil && build.Docker == nil) {
		return nil, fmt.Errorf("build configuration is required to determine resource parameters")
	}
	if build.Buildpack != nil && build.Buildpack.Language == string(utils.LanguageBallerina) {
		return &ResourceConfig{
			Requests: &ResourceRequests{
				CPU:    BallerinaCPURequest,
				Memory: BallerinaMemoryRequest,
			},
			Limits: &ResourceLimits{
				CPU:    BallerinaCPULimit,
				Memory: BallerinaMemoryLimit,
			},
		}, nil
	}
	return defaultResourceParameters(), nil
}

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

// Build workflow parameter names carrying the agent's runtime env vars and file mounts. They are
// seeded once at agent creation and then left alone; deploy writes runtime config to each
// environment's ReleaseBinding workloadOverrides instead.
const (
	workflowParamEnvironmentVariables = "environmentVariables"
	workflowParamFileMounts           = "fileMounts"
)

func buildWorkflowParameters(req CreateComponentRequest) (map[string]any, error) {
	params := map[string]any{
		workflowParamEnvironmentVariables: buildEnvironmentVariables(req),
		workflowParamFileMounts:           buildFileMounts(req),
	}

	// Add repository details in nested format expected by ClusterWorkflow
	if req.Repository != nil {
		repoParams := map[string]any{
			"url":     req.Repository.URL,
			"appPath": req.Repository.AppPath,
			"revision": map[string]any{
				"branch": req.Repository.Branch,
			},
		}
		if req.Repository.SecretRef != "" {
			repoParams["secretRef"] = req.Repository.SecretRef
		}
		params["repository"] = repoParams
	}

	// Add build-specific configs
	if req.Build != nil {
		if req.Build.Buildpack != nil {
			// Add buildEnv for buildpack configuration
			buildEnv := buildBuildpackEnv(req.Build.Buildpack)
			params["buildEnv"] = buildEnv
		} else if req.Build.Docker != nil {
			// Add docker configs in nested format expected by ClusterWorkflow
			appPath := normalizePath(req.Repository.AppPath)
			dockerfilePath := resolveDockerfilePath(appPath, req.Build.Docker.DockerfilePath)
			dockerParams := map[string]any{
				"context":  appPath,
				"filePath": dockerfilePath,
			}
			params["docker"] = dockerParams
			// Initialize empty buildEnv and buildArgs for docker builds
			params["buildEnv"] = []map[string]any{}
			params["buildArgs"] = []map[string]any{}
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

// buildBuildpackEnv creates the buildEnv array for buildpack configurations
// Reference: https://cloud.google.com/docs/buildpacks/set-environment-variables
func buildBuildpackEnv(bp *BuildpackConfig) []map[string]any {
	buildEnv := make([]map[string]any, 0)

	// Add language version if specified
	if bp.LanguageVersion != "" {
		versionEnvVar := getLanguageVersionEnvVariable(bp.Language)
		if versionEnvVar != "" {
			buildEnv = append(buildEnv, map[string]any{
				"name":  versionEnvVar,
				"value": bp.LanguageVersion,
			})
		}
	}

	// Add entrypoint/run command if specified (Google buildpack specific)
	if bp.RunCommand != "" && isGoogleBuildpack(bp.Language) {
		buildEnv = append(buildEnv, map[string]any{
			"name":  BuildEnvGoogleEntrypoint,
			"value": bp.RunCommand,
		})
	}

	return buildEnv
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
			"schemaType":    SchemaTypeOpenAPI,
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
			"schemaType":     SchemaTypeOpenAPI,
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

func buildFileMounts(req CreateComponentRequest) []map[string]any {
	fileMounts := make([]map[string]any, 0)
	if req.Configurations != nil {
		for _, f := range req.Configurations.Files {
			fileVar := map[string]any{
				"key":       f.Key,
				"mountPath": f.MountPath,
			}
			if f.ValueFrom != nil && f.ValueFrom.SecretKeyRef != nil {
				fileVar["valueFrom"] = map[string]any{
					"secretKeyRef": map[string]any{
						"name": f.ValueFrom.SecretKeyRef.Name,
						"key":  f.ValueFrom.SecretKeyRef.Key,
					},
				}
			} else {
				fileVar["value"] = f.Value
			}
			fileMounts = append(fileMounts, fileVar)
		}
	}
	return fileMounts
}

func normalizePath(path string) string {
	return strings.TrimSuffix(path, "/")
}

// resolveDockerfilePath prepends appPath to dockerfilePath so the result
// is relative to the repository root (as expected by the workflow).
func resolveDockerfilePath(appPath, dockerfilePath string) string {
	return normalizePath(appPath) + "/" + strings.TrimPrefix(normalizePath(dockerfilePath), "/")
}

// ComponentReconcileBlock describes a Ready=False condition that stops OpenChoreo from cutting
// new ComponentReleases for a component.
type ComponentReconcileBlock struct {
	Reason  string
	Message string
}

// GetComponentReconcileBlock returns the blocking Ready condition for a component, or nil when it
// reconciles normally. An absent Ready condition counts as not blocked — the component simply has
// not been reconciled yet, which is not evidence of a problem.
func (c *openChoreoClient) GetComponentReconcileBlock(ctx context.Context, ouID, componentName string) (*ComponentReconcileBlock, error) {
	namespaceName := c.NamespaceFor(ouID)
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
	if resp.JSON200 == nil || resp.JSON200.Status == nil {
		return nil, nil //nolint:nilnil // documented contract: a nil block means "not blocked", not an error
	}
	return reconcileBlockFromConditions(resp.JSON200.Status.Conditions), nil
}

// reconcileBlockFromConditions returns the blocking Ready condition, or nil when none applies.
func reconcileBlockFromConditions(conditions *[]gen.Condition) *ComponentReconcileBlock {
	if conditions == nil {
		return nil
	}
	for _, cond := range *conditions {
		if cond.Type != BindingStatusReady || cond.Status != "False" {
			continue
		}
		if _, blocking := componentBlockingReasons[cond.Reason]; !blocking {
			continue
		}
		block := &ComponentReconcileBlock{Reason: cond.Reason}
		if cond.Message != nil {
			block.Message = *cond.Message
		}
		return block
	}
	return nil
}

func (c *openChoreoClient) GetComponent(ctx context.Context, ouID, projectName, componentName string) (*models.AgentResponse, error) {
	namespaceName := c.NamespaceFor(ouID)
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

func (c *openChoreoClient) UpdateComponentBasicInfo(ctx context.Context, ouID, projectName, componentName string, req UpdateComponentBasicInfoRequest) error {
	namespaceName := c.NamespaceFor(ouID)
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

	if req.Labels != nil {
		merged := mergeUserLabels(component.Metadata.Labels, *req.Labels)
		component.Metadata.Labels = &merged
	}

	component.Status = nil
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
func (c *openChoreoClient) UpdateEnvResourceConfigs(ctx context.Context, ouID, projectName, componentName, environment string, req UpdateComponentResourceConfigsRequest) error {
	namespaceName := c.NamespaceFor(ouID)
	// List release bindings to find the correct binding name for the environment
	componentFilter := componentName
	listResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentFilter,
		Limit:     &defaultListLimit,
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

	// Add resources if provided — merge onto the existing override instead of
	// replacing it wholesale, so a partial update (e.g. cpu only) doesn't drop a
	// previously-set override for the untouched field (e.g. an existing memory limit).
	if req.Resources != nil {
		resourcesMap, err := structToMap(req.Resources)
		if err != nil {
			return fmt.Errorf("failed to convert resources to map: %w", err)
		}
		existingResources, _ := componentTypeEnvOverrides["resources"].(map[string]interface{})
		mergedResources := mergeResourceOverrides(existingResources, resourcesMap)
		// Validate the exact override being persisted, not an earlier caller-side
		// snapshot — the binding may have changed in between. Kubernetes rejects a
		// requests>limits pod spec at admission, after the old pod is already gone
		// (warm pools recreate), so an invalid pair must never be written.
		if err := c.validateMergedResourceOverrides(ctx, namespaceName, componentName, mergedResources); err != nil {
			return err
		}
		componentTypeEnvOverrides["resources"] = mergedResources
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
func (c *openChoreoClient) GetEnvResourceConfigs(ctx context.Context, ouID, projectName, componentName, environment string) (*ComponentResourceConfigsResponse, error) {
	namespaceName := c.NamespaceFor(ouID)
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
	response, err := c.getEnvConfigDefaultsFromComponentType(ctx, namespaceName, compResp.JSON200)
	if err != nil {
		return nil, fmt.Errorf("failed to get component type defaults: %w", err)
	}

	// Step 3: Check ReleaseBinding for environment-specific overrides
	binding, err := c.findReleaseBindingForEnv(ctx, namespaceName, componentName, environment)
	if err != nil {
		return nil, err
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

// mergeResourceOverrides merges a new resources override (requests/limits) onto an
// existing one field-by-field, so e.g. updating only cpu.requests doesn't wipe an
// existing memory.limits override that the update didn't touch.
func mergeResourceOverrides(existing, incoming map[string]interface{}) map[string]interface{} {
	merged := map[string]interface{}{}
	for _, key := range []string{"requests", "limits"} {
		existingSub, _ := existing[key].(map[string]interface{})
		incomingSub, hasIncoming := incoming[key].(map[string]interface{})

		switch {
		case hasIncoming && existingSub != nil:
			sub := make(map[string]interface{}, len(existingSub)+len(incomingSub))
			for k, v := range existingSub {
				sub[k] = v
			}
			for k, v := range incomingSub {
				sub[k] = v
			}
			merged[key] = sub
		case hasIncoming:
			merged[key] = incomingSub
		case existingSub != nil:
			merged[key] = existingSub
		}
	}
	return merged
}

// validateMergedResourceOverrides checks that the merged resources override about to
// be written to a release binding keeps requests within limits, resolving any side
// absent from the override against the component's baseline — the same fallback the
// ComponentType template applies at render time.
func (c *openChoreoClient) validateMergedResourceOverrides(ctx context.Context, namespaceName, componentName string, merged map[string]interface{}) error {
	compResp, err := c.ocClient.GetComponentWithResponse(ctx, namespaceName, componentName)
	if err != nil {
		return fmt.Errorf("failed to get component for resource validation: %w", err)
	}
	if compResp.StatusCode() != http.StatusOK || compResp.JSON200 == nil {
		return fmt.Errorf("failed to get component for resource validation: status %d", compResp.StatusCode())
	}
	defaults, err := c.getEnvConfigDefaultsFromComponentType(ctx, namespaceName, compResp.JSON200)
	if err != nil {
		return fmt.Errorf("failed to get component type defaults for resource validation: %w", err)
	}

	pick := func(section, field, fallback string) string {
		if sub, ok := merged[section].(map[string]interface{}); ok {
			if v, ok := sub[field].(string); ok && v != "" {
				return v
			}
		}
		return fallback
	}
	cpuRequest := pick("requests", "cpu", defaults.Resources.Requests.CPU)
	memRequest := pick("requests", "memory", defaults.Resources.Requests.Memory)
	cpuLimit := pick("limits", "cpu", defaults.Resources.Limits.CPU)
	memLimit := pick("limits", "memory", defaults.Resources.Limits.Memory)

	if err := utils.ValidateRequestWithinLimit(cpuRequest, cpuLimit, "CPU"); err != nil {
		return err
	}
	return utils.ValidateRequestWithinLimit(memRequest, memLimit, "memory")
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
func (c *openChoreoClient) getEnvConfigDefaultsFromComponentType(ctx context.Context, namespaceName string, component *gen.Component) (*ComponentResourceConfigsResponse, error) {
	response := &ComponentResourceConfigsResponse{
		Resources: &ResourceConfig{
			Requests: &ResourceRequests{},
			Limits:   &ResourceLimits{},
		},
		AutoScaling: &AutoScalingConfig{},
	}

	if component == nil || component.Spec == nil {
		return response, nil
	}

	// Get the ClusterComponentType name from component reference
	// The name may be prefixed with "deployment/" or similar, extract just the name part
	ctName := component.Spec.ComponentType.Name
	if parts := strings.Split(ctName, "/"); len(parts) > 1 {
		ctName = parts[len(parts)-1]
	}

	// Fetch ClusterComponentType
	ctResp, err := c.ocClient.GetComponentTypeWithResponse(ctx, namespaceName, ctName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster component type: %w", err)
	}
	if ctResp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(ctResp.StatusCode(), ErrorResponses{
			JSON401: ctResp.JSON401,
			JSON403: ctResp.JSON403,
			JSON404: ctResp.JSON404,
			JSON500: ctResp.JSON500,
		})
	}
	if ctResp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from get cluster component type")
	}

	if ctResp.JSON200.Spec != nil && ctResp.JSON200.Spec.EnvironmentConfigs != nil && ctResp.JSON200.Spec.EnvironmentConfigs.OpenAPIV3Schema != nil {
		// Extract defaults from schema (covers replicas/autoscaling, which the
		// environmentConfigs schema declares defaults for).
		applySchemaDefaults(response, *ctResp.JSON200.Spec.EnvironmentConfigs.OpenAPIV3Schema)
	}

	// The ComponentType template's resources fields fall back to the component's own
	// baseline parameters (`parameters.resources.*`) at render time — the
	// environmentConfigs schema declares no cpu/memory defaults, so relying on it alone
	// (as above) always reports blank resources when no per-environment override
	// exists. Overlay the component's actual baseline so this reports what the agent
	// is really running with.
	applyComponentResourceParameterDefaults(response.Resources, component.Spec.Parameters)
	return response, nil
}

// applyComponentResourceParameterDefaults overlays the component's own baseline
// resources (component.spec.parameters.resources, set at creation time from the
// ComponentType's parameters schema) onto the response. This mirrors the
// ComponentType template's `parameters.resources.*` render-time fallback.
func applyComponentResourceParameterDefaults(resources *ResourceConfig, parameters *map[string]interface{}) {
	if parameters == nil {
		return
	}
	resourcesRaw, ok := (*parameters)["resources"]
	if !ok {
		return
	}
	data, err := json.Marshal(resourcesRaw)
	if err != nil {
		return
	}
	var baseline ResourceConfig
	if err := json.Unmarshal(data, &baseline); err != nil {
		return
	}
	if baseline.Requests != nil {
		if baseline.Requests.CPU != "" {
			resources.Requests.CPU = baseline.Requests.CPU
		}
		if baseline.Requests.Memory != "" {
			resources.Requests.Memory = baseline.Requests.Memory
		}
	}
	if baseline.Limits != nil {
		if baseline.Limits.CPU != "" {
			resources.Limits.CPU = baseline.Limits.CPU
		}
		if baseline.Limits.Memory != "" {
			resources.Limits.Memory = baseline.Limits.Memory
		}
	}
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
	resourcesProp = resolveRef(resourcesProp, defs)
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

func (c *openChoreoClient) DeleteComponent(ctx context.Context, ouID, projectName, componentName string) error {
	namespaceName := c.NamespaceFor(ouID)
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

func (c *openChoreoClient) ListComponents(ctx context.Context, ouID, projectName string) ([]*models.AgentResponse, error) {
	namespaceName := c.NamespaceFor(ouID)
	resp, err := c.ocClient.ListComponentsWithResponse(ctx, namespaceName, &gen.ListComponentsParams{
		Project: &projectName,
		Limit:   &defaultListLimit,
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

func (c *openChoreoClient) ListComponentsByKind(ctx context.Context, ouID, projectName, kindName string) ([]*models.AgentResponse, error) {
	namespaceName := c.NamespaceFor(ouID)
	labelSelector := string(LabelKeyAgentKindName) + "=" + kindName
	resp, err := c.ocClient.ListComponentsWithResponse(ctx, namespaceName, &gen.ListComponentsParams{
		Project:       &projectName,
		Limit:         &defaultListLimit,
		LabelSelector: &labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list components by kind: %w", err)
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

func (c *openChoreoClient) ComponentExists(ctx context.Context, ouID, projectName, componentName string) (bool, error) {
	namespaceName := c.NamespaceFor(ouID)
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
	TraitKind TraitKind
	TraitType TraitType
	Opts      []TraitOption
}

// AttachTraits attaches one or more traits to a component in a single GET-UPDATE cycle.
func (c *openChoreoClient) AttachTraits(ctx context.Context, ouID, projectName, componentName string, traitRequests []TraitRequest) error {
	namespaceName := c.NamespaceFor(ouID)
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

	// Build an index of existing traits so we can update them in-place rather than
	// skipping them. This ensures re-deploys always apply the latest parameters
	// (artifactId, policies, port, basePath) even when the trait already exists.
	existingTraitIdx := make(map[string]int, len(traits))
	for i, trait := range traits {
		existingTraitIdx[trait.Name] = i
	}

	for _, req := range traitRequests {
		newTrait, err := c.buildTrait(ctx, namespaceName, projectName, componentName, req)
		if err != nil {
			return fmt.Errorf("failed to build trait %s: %w", req.TraitType, err)
		}
		if idx, exists := existingTraitIdx[string(req.TraitType)]; exists {
			traits[idx] = newTrait
		} else {
			traits = append(traits, newTrait)
		}
	}

	component.Spec.Traits = &traits

	component.Status = nil
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
func (c *openChoreoClient) DetachTrait(ctx context.Context, ouID, projectName, componentName string, traitType TraitType) error {
	namespaceName := c.NamespaceFor(ouID)
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
	component.Status = nil
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
func (c *openChoreoClient) HasTrait(ctx context.Context, ouID, projectName, componentName string, traitType TraitType) (bool, error) {
	namespaceName := c.NamespaceFor(ouID)
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

// UpdateComponentDeploymentConfig applies deploy-time Component CR changes in one GET-UPDATE cycle.
func (c *openChoreoClient) UpdateComponentDeploymentConfig(ctx context.Context, ouID, projectName, componentName string, req ComponentDeploymentConfigRequest) error {
	namespaceName := c.NamespaceFor(ouID)
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

	if len(req.TraitsToDetach) > 0 || len(req.TraitsToAttach) > 0 {
		detachSet := make(map[string]bool, len(req.TraitsToDetach))
		for _, traitType := range req.TraitsToDetach {
			detachSet[string(traitType)] = true
		}

		traits := make([]gen.ComponentTrait, 0)
		if component.Spec.Traits != nil {
			for _, trait := range *component.Spec.Traits {
				if !detachSet[trait.Name] {
					traits = append(traits, trait)
				}
			}
		}

		existingTraitIdx := make(map[string]int, len(traits))
		for i, trait := range traits {
			existingTraitIdx[trait.Name] = i
		}

		for _, traitReq := range req.TraitsToAttach {
			newTrait, err := c.buildTrait(ctx, namespaceName, projectName, componentName, traitReq)
			if err != nil {
				return fmt.Errorf("failed to build trait %s: %w", traitReq.TraitType, err)
			}
			if idx, exists := existingTraitIdx[string(traitReq.TraitType)]; exists {
				traits[idx] = newTrait
			} else {
				existingTraitIdx[string(traitReq.TraitType)] = len(traits)
				traits = append(traits, newTrait)
			}
		}

		component.Spec.Traits = &traits
	}

	if req.Env != nil {
		replaceComponentWorkflowEnvVars(component, req.Env)
	}

	component.Status = nil
	updateResp, err := c.ocClient.UpdateComponentWithResponse(ctx, namespaceName, componentName, *component)
	if err != nil {
		return fmt.Errorf("failed to update component deployment config: %w", err)
	}
	if updateResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(updateResp.StatusCode(), ErrorResponses{
			JSON400: updateResp.JSON400,
			JSON401: updateResp.JSON401,
			JSON403: updateResp.JSON403,
			JSON404: updateResp.JSON404,
			JSON500: updateResp.JSON500,
		})
	}

	return nil
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
	component.Status = nil
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
func (c *openChoreoClient) UpdateComponentEnvVars(ctx context.Context, ouID, projectName, componentName string, envVars []EnvVar) error {
	namespaceName := c.NamespaceFor(ouID)
	return c.mergeComponentEnvVars(ctx, namespaceName, componentName, envVars)
}

// ReplaceComponentEnvVars replaces all environment variables in the component's workflow parameters.
// Unlike mergeComponentEnvVars which merges with existing vars, this completely replaces them.
func (c *openChoreoClient) ReplaceComponentEnvVars(ctx context.Context, ouID, projectName, componentName string, envVars []EnvVar) error {
	namespaceName := c.NamespaceFor(ouID)
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

	replaceComponentWorkflowEnvVars(component, envVars)

	component.Status = nil
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

func replaceComponentWorkflowEnvVars(component *gen.Component, envVars []EnvVar) {
	if component.Spec.Workflow == nil {
		// No workflow exists (e.g. kind-sourced agents) — skip setting workflow env vars.
		// Env vars for these agents are applied directly to the Workload CR during deploy.
		return
	}
	if component.Spec.Workflow.Parameters == nil {
		params := make(map[string]interface{})
		component.Spec.Workflow.Parameters = &params
	}

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

	(*component.Spec.Workflow.Parameters)["environmentVariables"] = newEnvVars
}

// ReplaceComponentFileMounts replaces all file mount configurations in the component's workflow parameters.
func (c *openChoreoClient) ReplaceComponentFileMounts(ctx context.Context, ouID, projectName, componentName string, files []FileVar) error {
	namespaceName := c.NamespaceFor(ouID)
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

	if component.Spec.Workflow == nil {
		component.Spec.Workflow = &gen.ComponentWorkflowConfig{}
	}
	if component.Spec.Workflow.Parameters == nil {
		params := make(map[string]interface{})
		component.Spec.Workflow.Parameters = &params
	}
	workflowParams := *component.Spec.Workflow.Parameters

	newFileMounts := make([]map[string]any, 0, len(files))
	for _, f := range files {
		fileVar := map[string]any{
			"key":       f.Key,
			"mountPath": f.MountPath,
		}
		if f.ValueFrom != nil && f.ValueFrom.SecretKeyRef != nil {
			fileVar["valueFrom"] = map[string]any{
				"secretKeyRef": map[string]any{
					"name": f.ValueFrom.SecretKeyRef.Name,
					"key":  f.ValueFrom.SecretKeyRef.Key,
				},
			}
		} else {
			fileVar["value"] = f.Value
		}
		newFileMounts = append(newFileMounts, fileVar)
	}

	workflowParams["fileMounts"] = newFileMounts

	updateResp, err := c.ocClient.UpdateComponentWithResponse(ctx, namespaceName, componentName, *component)
	if err != nil {
		return fmt.Errorf("failed to replace component file mounts: %w", err)
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
func (c *openChoreoClient) UpdateReleaseBindingEnvVars(ctx context.Context, ouID, projectName, componentName, envName string, envVars []EnvVar) error {
	namespaceName := c.NamespaceFor(ouID)
	binding, err := c.findReleaseBindingForEnv(ctx, namespaceName, componentName, envName)
	if err != nil {
		return err
	}
	if binding == nil {
		// No binding for this environment yet — agent not deployed there; skip silently.
		return nil
	}
	bindingName := binding.Metadata.Name

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
	for _, genEnv := range toGenEnvVars(envVars) {
		existing[genEnv.Key] = genEnv
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
func (c *openChoreoClient) RemoveComponentEnvironmentVariables(ctx context.Context, ouID, projectName, componentName string, envVarKeys []string) error {
	namespaceName := c.NamespaceFor(ouID)
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

	component.Status = nil
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
func (c *openChoreoClient) RemoveReleaseBindingEnvVars(ctx context.Context, ouID, projectName, componentName, envName string, envVarKeys []string) error {
	namespaceName := c.NamespaceFor(ouID)
	if len(envVarKeys) == 0 {
		return nil
	}

	componentFilter := componentName
	listResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentFilter,
		Limit:     &defaultListLimit,
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

// ReplaceReleaseBindingEnvVars atomically removes specified keys and merges new env vars into
// the ReleaseBinding in a single Get/Update cycle. This avoids resource version conflicts that
// occur when RemoveReleaseBindingEnvVars and UpdateReleaseBindingEnvVars are called back-to-back.
func (c *openChoreoClient) ReplaceReleaseBindingEnvVars(ctx context.Context, ouID, projectName, componentName, envName string, keysToRemove []string, envVarsToAdd []EnvVar) error {
	namespaceName := c.NamespaceFor(ouID)
	componentFilter := componentName
	listResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentFilter,
		Limit:     &defaultListLimit,
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
		return nil
	}

	var bindingName string
	for _, b := range listResp.JSON200.Items {
		if b.Spec != nil && b.Spec.Environment == envName {
			bindingName = b.Metadata.Name
			break
		}
	}
	if bindingName == "" {
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

	if releaseBinding.Spec.WorkloadOverrides == nil {
		releaseBinding.Spec.WorkloadOverrides = &gen.WorkloadOverrides{}
	}
	if releaseBinding.Spec.WorkloadOverrides.Container == nil {
		releaseBinding.Spec.WorkloadOverrides.Container = &gen.ContainerOverride{}
	}

	// Step 1: Build map from existing env vars, removing specified keys.
	removeSet := make(map[string]bool, len(keysToRemove))
	for _, k := range keysToRemove {
		removeSet[k] = true
	}
	existing := make(map[string]gen.EnvVar)
	if releaseBinding.Spec.WorkloadOverrides.Container.Env != nil {
		for _, ev := range *releaseBinding.Spec.WorkloadOverrides.Container.Env {
			if !removeSet[ev.Key] {
				existing[ev.Key] = ev
			}
		}
	}

	// Step 2: Merge new env vars on top.
	for _, genEnv := range toGenEnvVars(envVarsToAdd) {
		existing[genEnv.Key] = genEnv
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

// RemoveWorkloadEnvVars removes env var keys from the Workload for the specified component.
// The Workload is a live runtime resource; removing env vars here ensures that stale entries
// (e.g., from a deleted LLM config) do not persist after the configuration is cleaned up.
func (c *openChoreoClient) RemoveWorkloadEnvVars(ctx context.Context, ouID, componentName string, envVarKeys []string) error {
	namespaceName := c.NamespaceFor(ouID)
	if len(envVarKeys) == 0 {
		return nil
	}

	workloadResp, err := c.ocClient.ListWorkloadsWithResponse(ctx, namespaceName, &gen.ListWorkloadsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
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
		return nil // No workload — nothing to remove
	}

	workload := workloadResp.JSON200.Items[0]
	workloadName := workload.Metadata.Name

	if workload.Spec == nil || workload.Spec.Container == nil || workload.Spec.Container.Env == nil {
		return nil
	}

	removeSet := make(map[string]bool, len(envVarKeys))
	for _, k := range envVarKeys {
		removeSet[k] = true
	}

	existing := *workload.Spec.Container.Env
	filtered := make([]gen.EnvVar, 0, len(existing))
	for _, ev := range existing {
		if !removeSet[ev.Key] {
			filtered = append(filtered, ev)
		}
	}
	workload.Spec.Container.Env = &filtered

	updateResp, err := c.ocClient.UpdateWorkloadWithResponse(ctx, namespaceName, workloadName, workload)
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

// TraitOption allows passing optional parameters when building traits.
type TraitOption func(map[string]interface{})

// WithInstrumentationEnabled sets the instrumentationEnabled flag for the OTEL instrumentation trait.
func WithInstrumentationEnabled(enabled bool) TraitOption {
	return func(params map[string]interface{}) {
		params["instrumentationEnabled"] = enabled
	}
}

// WithEnvInjectionEnabled sets the envInjectionEnabled flag for the env-injection trait.
func WithEnvInjectionEnabled(enabled bool) TraitOption {
	return func(params map[string]interface{}) {
		params["envInjectionEnabled"] = enabled
	}
}

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

// WithAgentApiKeySecretRef sets the remote reference (key/path) of the agent API key for
// the env-injection trait. The raw key value is stored in the secret store by the caller;
// only this reference is passed to the trait so the secret never lands in the control plane.
func WithAgentApiKeySecretRef(secretRef string) TraitOption {
	return func(params map[string]interface{}) {
		params["agentApiKeySecretRef"] = secretRef
	}
}

// WithAgentApiKeySecretProperty sets the optional property within the remote secret that
// holds the agent API key. Empty values are ignored so the trait omits the property.
func WithAgentApiKeySecretProperty(property string) TraitOption {
	return func(params map[string]interface{}) {
		if property != "" {
			params["agentApiKeySecretProperty"] = property
		}
	}
}

// WithOtelEndpointEnvName overrides the env var name the env-injection trait uses
// for the OTEL endpoint (default AMP_OTEL_ENDPOINT). Empty values are ignored so
// the trait keeps its default. Used by Ballerina to inject the Ballerina config-var
// name (BAL_CONFIG_VAR_BALLERINAX_AMP_OTELENDPOINT).
func WithOtelEndpointEnvName(name string) TraitOption {
	return func(params map[string]interface{}) {
		if name != "" {
			params["otelEndpointEnvName"] = name
		}
	}
}

// WithApiKeyEnvName overrides the env var name the env-injection trait uses for the
// agent API key (default AMP_AGENT_API_KEY). Empty values are ignored. Used by
// Ballerina to inject BAL_CONFIG_VAR_BALLERINAX_AMP_APIKEY.
func WithApiKeyEnvName(name string) TraitOption {
	return func(params map[string]interface{}) {
		if name != "" {
			params["apiKeyEnvName"] = name
		}
	}
}

// WithLanguageVersion sets the language version for the OTEL instrumentation trait,
// so it does not need to re-fetch the component to determine the instrumentation image.
func WithLanguageVersion(lv string) TraitOption {
	return func(params map[string]interface{}) {
		params["languageVersion"] = lv
	}
}

// WithPolicies sets the policies array for the api-configuration trait.
func WithPolicies(policies []map[string]interface{}) TraitOption {
	return func(params map[string]interface{}) {
		params["policies"] = policies
	}
}

// WithArtifactID sets the artifact UUID annotation for the api-configuration trait.
func WithArtifactID(artifactID string) TraitOption {
	return func(params map[string]interface{}) {
		params["artifactId"] = artifactID
	}
}

const (
	DefaultResilienceTimeoutSeconds int32 = 30
	MinResilienceTimeoutSeconds     int32 = 1
	MaxResilienceTimeoutSeconds     int32 = 3600
)

func FormatResilienceTimeout(seconds int32) string {
	return fmt.Sprintf("%ds", seconds)
}

// transforming to gateway policy format
func WithResilienceTimeout(seconds int32) TraitOption {
	return func(params map[string]interface{}) {
		params["resilienceTimeout"] = FormatResilienceTimeout(seconds)
	}
}

// APIKeyAuthPolicy returns the policy map for API key authentication.
func APIKeyAuthPolicy() map[string]interface{} {
	return map[string]interface{}{
		"name":    "api-key-auth",
		"version": "v1",
		"params": map[string]interface{}{
			"key": "X-API-Key",
			"in":  "header",
		},
	}
}

// OAuth (jwt-auth) policy contract. These literals are the gateway policy's
// contract (configured under policy_configurations.jwtauth_v1) and are the only
// place "jwt" wording survives — user-facing surfaces say "OAuth".
const (
	OAuthPolicyName    = "jwt-auth"
	OAuthPolicyVersion = "v1"
)

// OAuthPolicyParams holds the jwt-auth policy parameters resolved for an agent.
// The policy is authentication-only: it validates the token issuer, signature,
// expiry and (optionally) the audience (aud) claim — audience restriction is token
// validation (confused-deputy defense), not authorization. Authorization params
// (scopes, required claims) are not supported and are deferred to a future policy.
type OAuthPolicyParams struct {
	Issuers          []string
	Audiences        []string
	HeaderName       string
	AuthHeaderPrefix string
	// ForwardToken controls whether the validated token header is forwarded to
	// the upstream service (true) or stripped before proxying (false).
	ForwardToken bool
}

// OAuthPolicy returns the policy map for OAuth (JWT) authentication. Empty
// issuers/claims are omitted; header name and prefix are passed through as
// resolved by the caller, which guarantees non-empty gateway-compatible values
// (see models.DefaultOAuthHeaderName / DefaultOAuthAuthHeaderPrefix).
func OAuthPolicy(p OAuthPolicyParams) map[string]interface{} {
	params := map[string]interface{}{}
	if len(p.Issuers) > 0 {
		params["issuers"] = p.Issuers
	}
	if len(p.Audiences) > 0 {
		params["audiences"] = p.Audiences
	}
	params["headerName"] = p.HeaderName
	params["authHeaderPrefix"] = p.AuthHeaderPrefix
	params["forwardToken"] = p.ForwardToken
	return map[string]interface{}{
		"name":    OAuthPolicyName,
		"version": OAuthPolicyVersion,
		"params":  params,
	}
}

// CORSPolicy returns a CORS policy map with the given allowed origins, methods, headers, and credentials flag.
func CORSPolicy(allowedOrigins, allowedMethods, allowedHeaders []string, allowCredentials bool) map[string]interface{} {
	return map[string]interface{}{
		"name":    "cors",
		"version": "v1",
		"params": map[string]interface{}{
			"allowedOrigins":   allowedOrigins,
			"allowedMethods":   allowedMethods,
			"allowedHeaders":   allowedHeaders,
			"allowCredentials": allowCredentials,
		},
	}
}

// WithInstrumentationVersion pins the AMP instrumentation version for the OTEL
// instrumentation trait — the init-container image resolves to
// `amp-python-instrumentation-provider:<instrumentation_version>-python<X.Y>`.
// Nil falls back to the platform default (cfg.OTEL.DefaultInstrumentationVersion).
func WithInstrumentationVersion(version *string) TraitOption {
	return func(params map[string]interface{}) {
		if version != nil && *version != "" {
			params["instrumentationVersion"] = *version
		}
	}
}

func (c *openChoreoClient) buildTrait(ctx context.Context, namespaceName, projectName, componentName string, req TraitRequest) (gen.ComponentTrait, error) {
	if req.TraitKind == "" {
		return gen.ComponentTrait{}, fmt.Errorf("trait kind is required")
	}
	kind := gen.ComponentTraitKind(req.TraitKind)
	trait := gen.ComponentTrait{
		Kind:         &kind,
		Name:         string(req.TraitType),
		InstanceName: fmt.Sprintf("%s-%s", componentName, string(req.TraitType)),
	}
	switch req.TraitType {
	case TraitOTELInstrumentation:
		params, err := c.buildOTELTraitParameters(ctx, namespaceName, projectName, componentName, req.Opts...)
		if err != nil {
			return gen.ComponentTrait{}, err
		}
		trait.Parameters = &params
	case TraitEnvInjection:
		params, err := c.buildEnvInjectionTraitParameters(req.Opts...)
		if err != nil {
			return gen.ComponentTrait{}, err
		}
		trait.Parameters = &params
	case TraitBallerinaOTELInstrumentation:
		params := c.buildBallerinaConfigFileTraitParameters(req.Opts...)
		trait.Parameters = &params
	case TraitAPIManagement:
		params, err := c.buildAPIConfigurationTraitParameters(componentName, req.Opts...)
		if err != nil {
			return gen.ComponentTrait{}, err
		}
		trait.Parameters = &params
	default:
		return gen.ComponentTrait{}, fmt.Errorf("unsupported trait type: %s", req.TraitType)
	}
	return trait, nil
}

func (c *openChoreoClient) buildAPIConfigurationTraitParameters(componentName string, opts ...TraitOption) (map[string]interface{}, error) {
	params := map[string]interface{}{
		"apiName":           componentName,
		"apiVersion":        "v1.0",
		"context":           fmt.Sprintf("/%s", componentName),
		"upstreamPort":      config.GetConfig().DefaultChatAPI.DefaultHTTPPort,
		"upstreamBasePath":  config.GetConfig().DefaultChatAPI.DefaultBasePath,
		"resilienceTimeout": FormatResilienceTimeout(DefaultResilienceTimeoutSeconds),
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

	// Use the language version passed via WithLanguageVersion if available;
	// otherwise fall back to fetching the component (legacy path for direct callers).
	languageVersion, _ := params["languageVersion"].(string)
	if languageVersion == "" {
		component, err := c.GetComponent(ctx, namespaceName, projectName, componentName)
		if err != nil {
			return nil, fmt.Errorf("failed to get component for trait attachment: %w", err)
		}
		if component.Build != nil && component.Build.Buildpack != nil {
			languageVersion = component.Build.Buildpack.LanguageVersion
		}
	}

	// Get the project to validate it has a deployment pipeline configured.
	project, err := c.GetProject(ctx, namespaceName, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get project for trait attachment: %w", err)
	}
	if project.DeploymentPipeline == "" {
		return nil, fmt.Errorf("failed to attach trait: project %s does not have a deployment pipeline configured", projectName)
	}

	cfg := config.GetConfig()

	// Per-agent instrumentation version (from WithInstrumentationVersion) overrides
	// the platform default; an empty/unset value falls back to the default.
	instrumentationVersion, _ := params["instrumentationVersion"].(string)
	if instrumentationVersion == "" {
		instrumentationVersion = cfg.OTEL.DefaultInstrumentationVersion
	}
	instrumentationImage, err := getInstrumentationImage(languageVersion, instrumentationVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to build instrumentation image: %w", err)
	}

	result := map[string]interface{}{
		"instrumentationImage":  instrumentationImage,
		"sdkVolumeName":         cfg.OTEL.SDKVolumeName,
		"sdkMountPath":          cfg.OTEL.SDKMountPath,
		"isTraceContentEnabled": utils.BoolAsString(cfg.OTEL.IsTraceContentEnabled),
	}
	if v, ok := params["instrumentationEnabled"]; ok {
		result["instrumentationEnabled"] = v
	}
	return result, nil
}

// buildEnvInjectionTraitParameters builds parameters for the env injection trait
// which injects AMP_OTEL_ENDPOINT and AMP_AGENT_API_KEY environment variables
func (c *openChoreoClient) buildEnvInjectionTraitParameters(opts ...TraitOption) (map[string]interface{}, error) {
	params := make(map[string]interface{})
	for _, opt := range opts {
		opt(params)
	}
	agentApiKeySecretRef, _ := params["agentApiKeySecretRef"].(string)
	if agentApiKeySecretRef == "" {
		return nil, fmt.Errorf("agent API key secret reference is required for env injection trait")
	}

	// otelEndpoint is no longer set here — the env-injection trait derives it from the
	// apiGatewayName convention ("api-platform-<org>-<env>") + the gateway runtime
	// in-cluster Service. The trait still honours an explicit override via
	// environmentConfigs.otelEndpoint / parameters.otelEndpoint when set.
	result := map[string]interface{}{
		"agentApiKeySecretRef": agentApiKeySecretRef,
	}
	if v, ok := params["agentApiKeySecretProperty"].(string); ok && v != "" {
		result["agentApiKeySecretProperty"] = v
	}
	if v, ok := params["envInjectionEnabled"]; ok {
		result["envInjectionEnabled"] = v
	}
	// Language-specific env var names; the trait defaults to the AMP_* names.
	if v, ok := params["otelEndpointEnvName"].(string); ok && v != "" {
		result["otelEndpointEnvName"] = v
	}
	if v, ok := params["apiKeyEnvName"].(string); ok && v != "" {
		result["apiKeyEnvName"] = v
	}
	return result, nil
}

// buildBallerinaConfigFileTraitParameters builds parameters for the Ballerina
// config-file trait, which mounts am-instrumentation.toml via an init container and
// sets BAL_CONFIG_FILES. The OTEL endpoint + API key env vars are injected by the
// shared env-injection trait (with Ballerina-specific names), not here. The image is
// always built with observability included; whether tracing is emitted is a runtime
// decision gated by the instrumentationEnabled parameter (set via
// WithInstrumentationEnabled) and overridable per-environment via traitEnvironmentConfigs.
func (c *openChoreoClient) buildBallerinaConfigFileTraitParameters(opts ...TraitOption) map[string]interface{} {
	params := make(map[string]interface{})
	for _, opt := range opts {
		opt(params)
	}
	return params
}

// BuildInstrumentationImage is the exported wrapper around getInstrumentationImage
// so callers outside this package (e.g. the service layer) can compute the
// init-container image reference for a per-environment instrumentationImage
// override without re-deriving the registry/name/tag convention.
func BuildInstrumentationImage(languageVersion, instrumentationVersion string) (string, error) {
	return getInstrumentationImage(languageVersion, instrumentationVersion)
}

// getInstrumentationImage builds the pre-built init-container image reference for
// the given AMP instrumentation version and the agent's Python runtime version,
// e.g. ghcr.io/wso2/amp-python-instrumentation-provider:0.3.0-python3.11.
func getInstrumentationImage(languageVersion, instrumentationVersion string) (string, error) {
	// Trim before splitting so the built tag matches the trimmed major.minor the
	// service validates against the catalog (normalizePythonMinor also trims);
	// otherwise a value like "  3.11  " would validate yet produce a malformed
	// image tag with embedded whitespace.
	parts := strings.Split(strings.TrimSpace(languageVersion), ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid languageVersion format: expected 'major.minor' but got '%s'", languageVersion)
	}
	pythonMajorMinor := strings.TrimSpace(parts[0]) + "." + strings.TrimSpace(parts[1])
	return fmt.Sprintf("%s/%s:%s-python%s", InstrumentationImageRegistry, InstrumentationImageName, instrumentationVersion, pythonMajorMinor), nil
}

func (c *openChoreoClient) GetComponentEndpoints(ctx context.Context, ouID, projectName, componentName, environment string) (map[string]models.EndpointsResponse, error) {
	namespaceName := c.NamespaceFor(ouID)
	// List release bindings filtering by component to get endpoint URLs
	releaseBindingResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
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
					// Use ExternalURLs based on TLSConfig.EnableTLS
					if ep.ExternalURLs != nil {
						var endpointURL *gen.EndpointURL
						if config.GetConfig().TLSConfig.EnableTLS {
							endpointURL = ep.ExternalURLs.Https
						} else {
							endpointURL = ep.ExternalURLs.Http
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
		Limit:     &defaultListLimit,
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

func (c *openChoreoClient) GetComponentConfigurations(ctx context.Context, ouID, projectName, componentName, environment string) ([]models.EnvVars, error) {
	namespaceName := c.NamespaceFor(ouID)
	// Create a map to store environment variables (for easy merging)
	type envVarEntry struct {
		Value       string
		IsSensitive bool
		SecretRef   string
		SecretKey   string // The key within the secret (e.g., "api-key")
	}
	envVarMap := make(map[string]envVarEntry)

	// List workloads to extract base environment variables
	workloadResp, err := c.ocClient.ListWorkloadsWithResponse(ctx, namespaceName, &gen.ListWorkloadsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
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
				secretKey := ""
				if isSensitive && env.ValueFrom.SecretKeyRef.Name != nil {
					secretRef = *env.ValueFrom.SecretKeyRef.Name
				}
				if isSensitive && env.ValueFrom.SecretKeyRef.Key != nil {
					secretKey = *env.ValueFrom.SecretKeyRef.Key
				}
				envVarMap[env.Key] = envVarEntry{
					Value:       utils.StrPointerAsStr(env.Value, ""),
					IsSensitive: isSensitive,
					SecretRef:   secretRef,
					SecretKey:   secretKey,
				}
			}
		}
	}

	// List release bindings filtering by component to get overrides
	releaseBindingResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
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
						secretKey := ""
						if isSensitive && env.ValueFrom.SecretKeyRef.Name != nil {
							secretRef = *env.ValueFrom.SecretKeyRef.Name
						}
						if isSensitive && env.ValueFrom.SecretKeyRef.Key != nil {
							secretKey = *env.ValueFrom.SecretKeyRef.Key
						}
						envVarMap[env.Key] = envVarEntry{
							Value:       utils.StrPointerAsStr(env.Value, ""),
							IsSensitive: isSensitive,
							SecretRef:   secretRef,
							SecretKey:   secretKey,
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
			SecretKey:   entry.SecretKey,
		})
	}

	return envVars, nil
}

func (c *openChoreoClient) GetComponentFileMounts(ctx context.Context, ouID, projectName, componentName, environment string) ([]models.FileMountEntry, error) {
	namespaceName := c.NamespaceFor(ouID)
	// List workloads to extract file mounts
	workloadResp, err := c.ocClient.ListWorkloadsWithResponse(ctx, namespaceName, &gen.ListWorkloadsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
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

	// Base file mounts from the Workload, keyed so the environment's overrides can replace them.
	// Order is tracked separately: a map alone would return the mounts in a different order on
	// every call, which reshuffles the list the console renders.
	fileMountMap := make(map[string]models.FileMountEntry)
	var order []string
	appendMount := func(f gen.FileVar) {
		isSensitive := f.ValueFrom != nil && f.ValueFrom.SecretKeyRef != nil
		secretRef := ""
		if isSensitive && f.ValueFrom.SecretKeyRef.Name != nil {
			secretRef = *f.ValueFrom.SecretKeyRef.Name
		}
		if _, seen := fileMountMap[f.Key]; !seen {
			order = append(order, f.Key)
		}
		fileMountMap[f.Key] = models.FileMountEntry{
			Key:         f.Key,
			MountPath:   f.MountPath,
			Value:       utils.StrPointerAsStr(f.Value, ""),
			IsSensitive: isSensitive,
			SecretRef:   secretRef,
		}
	}

	if workloadResp.JSON200 != nil && len(workloadResp.JSON200.Items) > 0 {
		workload := workloadResp.JSON200.Items[0]
		if workload.Spec != nil && workload.Spec.Container != nil && workload.Spec.Container.Files != nil {
			for _, f := range *workload.Spec.Container.Files {
				appendMount(f)
			}
		}
	}

	// Overlay this environment's release binding overrides. Deploy writes the effective set here,
	// so without this a mount added to one environment is invisible to the console and to
	// GET .../file-mounts, which would report the shared base for every environment instead.
	// Union semantics match the render: mergeFileConfigs falls back to the base for keys an
	// override omits, and the base is retained, so base-only mounts still read correctly.
	// Mirrors GetComponentConfigurations.
	releaseBindingResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
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

	if releaseBindingResp.JSON200 != nil {
		for _, binding := range releaseBindingResp.JSON200.Items {
			if binding.Spec == nil || binding.Spec.Environment != environment {
				continue
			}
			if binding.Spec.WorkloadOverrides != nil && binding.Spec.WorkloadOverrides.Container != nil &&
				binding.Spec.WorkloadOverrides.Container.Files != nil {
				for _, f := range *binding.Spec.WorkloadOverrides.Container.Files {
					appendMount(f)
				}
			}
			break
		}
	}

	var fileMounts []models.FileMountEntry
	for _, key := range order {
		fileMounts = append(fileMounts, fileMountMap[key])
	}

	return fileMounts, nil
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
		Type:     componentTypeName,
		Language: getLabel(comp.Metadata.Labels, string(LabelKeyAgentLanguage)),
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
		Type:   agentType,
		Labels: extractUserLabels(comp.Metadata.Labels),
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
			language := getLabel(comp.Metadata.Labels, string(LabelKeyAgentLanguage))
			agent.Build = extractBuildParams(params, language, agent.Provisioning.Repository.AppPath)
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

	if getLabel(comp.Metadata.Labels, string(LabelKeyBuildSource)) == BuildSourceKind {
		agent.KindName = getLabel(comp.Metadata.Labels, string(LabelKeyAgentKindName))
		// Empty for agents created before the version label existed.
		agent.KindVersion = getLabel(comp.Metadata.Labels, string(LabelKeyAgentKindVersion))
		// Enrich agent.Build from labels — kind agents have no workflow, so extractBuildParams
		// is never called above, but the build source info is stored in labels at creation time.
		language := getLabel(comp.Metadata.Labels, string(LabelKeyAgentLanguage))
		languageVersion := getLabel(comp.Metadata.Labels, string(LabelKeyAgentLanguageVersion))
		if language == "docker" {
			agent.Build = &models.Build{Type: BuildTypeDocker, Docker: &models.DockerConfig{}}
		} else if language != "" {
			agent.Build = &models.Build{
				Type: BuildTypeBuildpack,
				Buildpack: &models.BuildpackConfig{
					Language:        language,
					LanguageVersion: languageVersion,
				},
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
		Url:       getMapString(repo, "url"),
		Branch:    branch,
		AppPath:   getMapString(repo, "appPath"),
		SecretRef: getMapString(repo, "secretRef"),
	}
}

// extractBuildParams extracts build configuration (buildpack or docker) from parameters.
// appPath is used to convert the stored absolute filePath back to a path relative to appPath.
func extractBuildParams(params map[string]interface{}, language, appPath string) *models.Build {
	// Check for docker build (has docker object with filePath)
	if dc, ok := params["docker"].(map[string]interface{}); ok {
		filePath := path.Clean(getMapString(dc, "filePath"))
		if appPath != "" {
			filePath = strings.TrimPrefix(filePath, path.Clean(appPath))
		}
		return &models.Build{
			Type:   BuildTypeDocker,
			Docker: &models.DockerConfig{DockerfilePath: filePath},
		}
	}

	// Check for buildpack build (has buildEnv array or language label)
	if buildEnv, ok := params["buildEnv"].([]interface{}); ok || language != "" {
		buildpackConfig := &models.BuildpackConfig{
			Language: language,
		}

		// Extract language version and entrypoint from buildEnv
		if ok && len(buildEnv) > 0 {
			versionEnvVar := getLanguageVersionEnvVariable(language)
			for _, item := range buildEnv {
				if env, ok := item.(map[string]interface{}); ok {
					name := getMapString(env, "name")
					value := getMapString(env, "value")

					// Check if this is the version env var for this language
					if name != "" && name == versionEnvVar {
						buildpackConfig.LanguageVersion = value
					}
					// Check for Google entrypoint
					if name == BuildEnvGoogleEntrypoint {
						buildpackConfig.RunCommand = value
					}
				}
			}
		}

		return &models.Build{
			Type:      BuildTypeBuildpack,
			Buildpack: buildpackConfig,
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
