// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package openchoreosvc

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openchoreo/openchoreo/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

func getLanguageVersionEnvVariable(language string) string {
	for _, buildpack := range utils.Buildpacks {
		if buildpack.Language == language {
			return buildpack.VersionEnvVariable
		}
	}
	return ""
}

func getOpenChoreoComponentType(provisioningType string, agentType string) ComponentType {
	if provisioningType == string(utils.ExternalAgent) {
		return ComponentTypeExternalAgentAPI
	}
	if provisioningType == string(utils.InternalAgent) && agentType == string(utils.AgentTypeAPI) {
		return ComponentTypeInternalAgentAPI
	}
	// agent type is already validated in controller layer
	return ""
}

func getOpenChoreoComponentWorkflow(language string) ComponentWorkflow {
	for _, buildpack := range utils.Buildpacks {
		if buildpack.Language == language {
			if buildpack.Provider == string(utils.BuildPackProviderGoogle) {
				return ComponentWorkflowGCB
			}
			if buildpack.Provider == string(utils.BuildPackProviderAMPBallerina) {
				return ComponentWorkflowBallerina
			}
		}
	}
	// language is already validated in controller layer
	return ""
}

func isGoogleBuildpack(language string) bool {
	for _, buildpack := range utils.Buildpacks {
		if buildpack.Language == language && buildpack.Provider == string(utils.BuildPackProviderGoogle) {
			return true
		}
	}
	return false
}

func getInputInterfaceConfig(req *spec.CreateAgentRequest) (int32, string) {
	agentSubType := utils.StrPointerAsStr(req.AgentType.SubType, "")
	if req.AgentType.Type == string(utils.AgentTypeAPI) && agentSubType == string(utils.AgentSubTypeChatAPI) {
		return int32(config.GetConfig().DefaultChatAPI.DefaultHTTPPort), config.GetConfig().DefaultChatAPI.DefaultBasePath
	}
	return req.InputInterface.Port, req.InputInterface.BasePath
}

func getComponentWorkflowParametersForGoogleBuildPack(req *spec.CreateAgentRequest) map[string]interface{} {
	return map[string]interface{}{
		"buildpackConfigs": map[string]interface{}{
			"googleEntryPoint":   req.RuntimeConfigs.RunCommand,
			"languageVersion":    req.RuntimeConfigs.LanguageVersion,
			"languageVersionKey": getLanguageVersionEnvVariable(req.RuntimeConfigs.Language),
		},
		"schemaFilePath": req.InputInterface.Schema.Path,
	}
}

func getComponentWorkflowParametersForBallerinaBuildPack(req *spec.CreateAgentRequest) map[string]interface{} {
	return map[string]interface{}{
		"schemaFilePath": req.InputInterface.Schema.Path,
	}
}

func createComponentCRForExternalAgents(orgName, projectName string, req *spec.CreateAgentRequest) (*v1alpha1.Component, error) {
	annotations := map[string]string{
		string(AnnotationKeyDisplayName): req.DisplayName,
		string(AnnotationKeyDescription): utils.StrPointerAsStr(req.Description, ""),
	}
	labels := map[string]string{
		string(LabelKeyProvisioningType): req.Provisioning.Type,
	}
	componentType := getOpenChoreoComponentType(req.Provisioning.Type, req.AgentType.Type)

	componentCR := &v1alpha1.Component{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Component",
			APIVersion: "openchoreo.dev/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Namespace:   orgName,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: v1alpha1.ComponentSpec{
			Owner: v1alpha1.ComponentOwner{
				ProjectName: projectName,
			},
			ComponentType: string(componentType),
		},
	}
	return componentCR, nil
}

func createComponentCRForInternalAgents(orgName, projectName string, req *spec.CreateAgentRequest) (*v1alpha1.Component, error) {
	annotations := map[string]string{
		string(AnnotationKeyDisplayName): req.DisplayName,
		string(AnnotationKeyDescription): utils.StrPointerAsStr(req.Description, ""),
	}
	labels := map[string]string{
		string(LabelKeyProvisioningType): req.Provisioning.Type,
		string(LabelKeyAgentSubType):     utils.StrPointerAsStr(req.AgentType.SubType, ""),
		string(LabelKeyAgentLanguage):    req.RuntimeConfigs.Language,
		string(LabelKeyAgentLanguageVersion): utils.StrPointerAsStr(req.RuntimeConfigs.LanguageVersion,""),
	}
	componentType := getOpenChoreoComponentType(req.Provisioning.Type, req.AgentType.Type)
	componentWorkflow := getOpenChoreoComponentWorkflow(req.RuntimeConfigs.Language)
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
	}

	var componentWorkflowParameters map[string]interface{}
	if isGoogleBuildpack(req.RuntimeConfigs.Language) {
		componentWorkflowParameters = getComponentWorkflowParametersForGoogleBuildPack(req)
	} else {
		componentWorkflowParameters = getComponentWorkflowParametersForBallerinaBuildPack(req)
	}

	parametersJSON, err := json.Marshal(parameters)
	if err != nil {
		return nil, fmt.Errorf("error marshalling component parameters: %w", err)
	}
	componentWorkflowParametersJSON, err := json.Marshal(componentWorkflowParameters)
	if err != nil {
		return nil, fmt.Errorf("error marshalling component workflow parameters: %w", err)
	}

	componentCR := &v1alpha1.Component{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Component",
			APIVersion: "openchoreo.dev/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Namespace:   orgName,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: v1alpha1.ComponentSpec{
			Owner: v1alpha1.ComponentOwner{
				ProjectName: projectName,
			},
			ComponentType: string(componentType),
			Workflow: &v1alpha1.ComponentWorkflowRunConfig{
				Name: string(componentWorkflow),
				SystemParameters: v1alpha1.SystemParametersValues{
					Repository: v1alpha1.RepositoryValues{
						URL: req.Provisioning.Repository.Url,
						Revision: v1alpha1.RepositoryRevisionValues{
							Branch: req.Provisioning.Repository.Branch,
						},
						AppPath: req.Provisioning.Repository.AppPath,
					},
				},
				Parameters: &runtime.RawExtension{
					Raw: componentWorkflowParametersJSON,
				},
			},
			AutoDeploy: true,
			Parameters: &runtime.RawExtension{
				Raw: parametersJSON,
			},
		},
	}
	return componentCR, nil
}

func createOTELInstrumentationTrait(ocAgentComponent *v1alpha1.Component, envUUID, projectUUID string) (*v1alpha1.ComponentTrait, error) {
	traitParameters := map[string]interface{}{
		"instrumentationImage":  getInstrumentationImage(ocAgentComponent.Labels[string(LabelKeyAgentLanguageVersion)]),
		"sdkVolumeName":         config.GetConfig().OTEL.SDKVolumeName,
		"sdkMountPath":          config.GetConfig().OTEL.SDKMountPath,
		"agentName":             ocAgentComponent.Name,
		"otelEndpoint":          config.GetConfig().OTEL.ExporterEndpoint,
		"isTraceContentEnabled": utils.BoolAsString(config.GetConfig().OTEL.IsTraceContentEnabled),
		"traceAttributes":       fmt.Sprintf("%s=%s,%s=%s,%s=%s", TraceAttributeKeyProject, projectUUID, TraceAttributeKeyEnvironment, envUUID, TraceAttributeKeyComponent, ocAgentComponent.UID),
	}
	traitParametersJSON, err := json.Marshal(traitParameters)
	if err != nil {
		return nil, fmt.Errorf("error marshalling OTEL instrumentation trait parameters: %w", err)
	}

	return &v1alpha1.ComponentTrait{
		Name:         string(TraitTypeOTELInstrumentation),
		InstanceName: fmt.Sprintf("%s-%s", ocAgentComponent.Name, string(TraitTypeOTELInstrumentation)),
		Parameters: &runtime.RawExtension{
			Raw: traitParametersJSON,
		},
	}, nil
}

func getInstrumentationImage(languageVersion string) string {
	// Extract major.minor version (e.g., "3.10.5" -> "3.10")
	parts := strings.Split(languageVersion, ".")
	if len(parts) >= 2 {
		majorMinor := parts[0] + "." + parts[1]
		switch majorMinor {
		case "3.10":
			return config.GetConfig().OTEL.OTELInstrumentationImage.Python310
		case "3.11":
			return config.GetConfig().OTEL.OTELInstrumentationImage.Python311
		case "3.12":
			return config.GetConfig().OTEL.OTELInstrumentationImage.Python312
		case "3.13":
			return config.GetConfig().OTEL.OTELInstrumentationImage.Python313
		}
	}
	return ""
}

func createComponentWorkflowRunCR(orgName, projName, componentName string, systemParams v1alpha1.SystemParametersValues, component *v1alpha1.Component) *v1alpha1.ComponentWorkflowRun {
	// Generate a unique workflow run name with short UUID
	uuid := uuid.New().String()
	workflowUuid := strings.ReplaceAll(uuid[:8], "-", "")
	workflowRunName := fmt.Sprintf("%s-build-%s", component.Name, workflowUuid)

	return &v1alpha1.ComponentWorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workflowRunName,
			Namespace: orgName,
		},
		Spec: v1alpha1.ComponentWorkflowRunSpec{
			Owner: v1alpha1.ComponentWorkflowOwner{
				ProjectName:   projName,
				ComponentName: componentName,
			},
			Workflow: v1alpha1.ComponentWorkflowRunConfig{
				Name:             component.Spec.Workflow.Name,
				SystemParameters: systemParams,
				Parameters:       component.Spec.Workflow.Parameters,
			},
		},
	}
}

func toComponentResponse(component *v1alpha1.Component) *AgentComponent {
	response := &AgentComponent{
		Name:        component.Name,
		UUID:        string(component.UID),
		DisplayName: component.Annotations[string(AnnotationKeyDisplayName)],
		ProjectName: component.Spec.Owner.ProjectName,
		Provisioning: Provisioning{
			Type: component.Labels[string(LabelKeyProvisioningType)],
		},
		Type: AgentType{
			Type:    strings.Split(string(component.Spec.ComponentType), "/")[1], // e.g., deployment/agent-api -> agent-api
			SubType: component.Labels[string(LabelKeyAgentSubType)],
		},
		Language:    component.Labels[string(LabelKeyAgentLanguage)],
		CreatedAt:   component.CreationTimestamp.Time,
		Status:      "", // Todo: set status
		Description: component.Annotations[string(AnnotationKeyDescription)],
	}

	// Only populate repository info if workflow exists (internal agents)
	if component.Spec.Workflow != nil {
		response.Provisioning.Repository = Repository{
			RepoURL: component.Spec.Workflow.SystemParameters.Repository.URL,
			Branch:  component.Spec.Workflow.SystemParameters.Repository.Revision.Branch,
			AppPath: component.Spec.Workflow.SystemParameters.Repository.AppPath,
		}
	}

	return response
}

func updateWorkloadSpec(existingWorkload *v1alpha1.Workload, req *spec.DeployAgentRequest) {
	var envs []v1alpha1.EnvVar

	// Keep existing endpoints and just update container spec
	existingWorkload.Spec.Containers = map[string]v1alpha1.Container{
		"main": {
			Image: req.ImageId,
			Env: func() []v1alpha1.EnvVar {
				for _, env := range req.Env {
					envs = append(envs, v1alpha1.EnvVar{
						Key:   env.Key,
						Value: env.Value,
					})
				}
				return envs
			}(),
		},
	}
}

func findStatusCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}

	return nil
}

// IsStatusConditionTrue returns true when the conditionType is present and set to `metav1.ConditionTrue`
func isStatusConditionTrue(conditions []metav1.Condition, conditionType string) bool {
	return isStatusConditionPresentAndEqual(conditions, conditionType, metav1.ConditionTrue)
}

// IsStatusConditionPresentAndEqual returns true when conditionType is present and equal to status.
func isStatusConditionPresentAndEqual(conditions []metav1.Condition, conditionType string, status metav1.ConditionStatus) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition.Status == status
		}
	}
	return false
}

func determineBuildStatus(conditions []metav1.Condition) BuildStatus {
	if len(conditions) == 0 {
		return BuildStatusInitiated // Just created
	}

	// Check terminal states first (priority order)
	if isStatusConditionTrue(conditions, string(ConditionWorkloadUpdated)) {
		return WorkloadUpdated // Fully done
	}

	if isStatusConditionTrue(conditions, string(ConditionWorkflowFailed)) {
		return BuildStatusFailed
	}

	if isStatusConditionTrue(conditions, string(ConditionWorkflowSucceeded)) {
		return BuildStatusSucceeded // Workflow succeeded
	}

	// Check active states
	if isStatusConditionTrue(conditions, string(ConditionWorkflowRunning)) {
		return BuildStatusRunning
	}

	// Check if workflow was triggered but not running yet
	workflowCompletedCond := findStatusCondition(conditions, string(ConditionWorkflowCompleted))
	if workflowCompletedCond != nil && workflowCompletedCond.Status == metav1.ConditionFalse {
		if workflowCompletedCond.Reason == string(ConditionWorkflowPending) {
			return BuildStatusTriggered // Argo Workflow created, not started
		}
	}

	return BuildStatusInitiated // Has conditions but unclear state
}

func toBuildDetailsResponse(componentWorkflow *v1alpha1.ComponentWorkflowRun) (*models.BuildDetailsResponse, error) {
	commitId := componentWorkflow.Spec.Workflow.SystemParameters.Repository.Revision.Commit
	if commitId == "" {
		commitId = "latest"
	}

	buildResp := &models.BuildDetailsResponse{
		BuildResponse: models.BuildResponse{
			UUID:        string(componentWorkflow.UID),
			Name:        componentWorkflow.Name,
			AgentName:   componentWorkflow.Spec.Owner.ComponentName,
			ProjectName: componentWorkflow.Spec.Owner.ProjectName,
			CommitID:    commitId,
			Status:      string(determineBuildStatus(componentWorkflow.Status.Conditions)),
			StartedAt:   componentWorkflow.CreationTimestamp.Time,
			Branch:      componentWorkflow.Spec.Workflow.SystemParameters.Repository.Revision.Branch,
			Image:       componentWorkflow.Status.ImageStatus.Image,
		},
	}

	// Convert conditions to build steps
	buildResp.Steps = MapConditionsToBuildSteps(componentWorkflow.Status.Conditions)

	// Calculate build completion percentage
	if percentage := calculateBuildPercentage(buildResp.Steps); percentage != nil {
		buildResp.Percent = *percentage
	}

	// Set end time if build is completed
	if endTime := findBuildEndTime(componentWorkflow.Status.Conditions); endTime != nil {
		buildResp.EndedAt = &endTime.Time
		// Calculate duration in seconds
		duration := endTime.Sub(componentWorkflow.CreationTimestamp.Time).Seconds()
		buildResp.DurationSeconds = int32(duration)
	}

	return buildResp, nil
}

func toDeploymentDetailsResponse(binding *v1alpha1.ReleaseBinding, envRelease *v1alpha1.Release, environmentMap map[string]*models.EnvironmentResponse, promotionTargetEnv *models.PromotionTargetEnvironment) (*models.DeploymentResponse, error) {
	if binding == nil {
		return nil, nil
	}

	// Extract deployment status from Release Binding
	status := determineReleaseBindingStatus(binding)

	// Extract last deployed time from conditions
	var lastDeployedTime time.Time
	if len(binding.Status.Conditions) > 0 {
		lastDeployedTime = binding.Status.Conditions[0].LastTransitionTime.Time
	}

	// Extract endpoints from EnvRelease status
	endpoints, err := extractEndpointURLFromEnvRelease(envRelease)
	if err != nil {
		return nil, fmt.Errorf("error extracting endpoints: %w", err)
	}
	// Extract deployed image from EnvRelease status
	deployedImage := findDeployedImageFromEnvRelease(envRelease)

	environment := binding.Spec.Environment
	// Get environment display name
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
		LastDeployedAt:             lastDeployedTime,
		Endpoints:                  endpoints,
	}, nil
}

func determineReleaseBindingStatus(binding *v1alpha1.ReleaseBinding) string {
	if len(binding.Status.Conditions) == 0 {
		return DeploymentStatusNotDeployed
	}

	generation := binding.ObjectMeta.Generation

	// Collect all conditions for the current generation
	var conditionsForGeneration []metav1.Condition
	for i := range binding.Status.Conditions {
		if binding.Status.Conditions[i].ObservedGeneration == generation {
			conditionsForGeneration = append(conditionsForGeneration, binding.Status.Conditions[i])
		}
	}

	// Expected conditions: ReleaseSynced, ResourcesReady, Ready
	// If there are less than 3 conditions for the current generation, it's still in progress
	if len(conditionsForGeneration) < 3 {
		return DeploymentStatusInProgress
	}

	// Check if any condition has Status == False with ResourcesDegraded reason
	for i := range conditionsForGeneration {
		if conditionsForGeneration[i].Status == metav1.ConditionFalse && conditionsForGeneration[i].Reason == "ResourcesDegraded" {
			return DeploymentStatusFailed
		}
	}

	// Check if any condition has Status == False with ResourcesProgressing reason
	for i := range conditionsForGeneration {
		if conditionsForGeneration[i].Status == metav1.ConditionFalse && conditionsForGeneration[i].Reason == "ResourcesProgressing" {
			return DeploymentStatusInProgress
		}
	}

	// If all three conditions are present and none are degraded, it's ready
	return DeploymentStatusActive
}

// extractEndpointsFromEnvReleaseBinding converts EnvRelease endpoints to model endpoints
func extractEndpointURLFromEnvRelease(envRelease *v1alpha1.Release) ([]models.Endpoint, error) {
	var endpoints []models.Endpoint

	if envRelease == nil || envRelease.Spec.Resources == nil {
		return endpoints, nil
	}

	// Check spec.resources for HTTPRoute definitions
	specResources := envRelease.Spec.Resources

	// Find all HTTPRoute objects in spec resources
	for i := range specResources {
		rawResource := specResources[i].Object.Raw
		if len(rawResource) == 0 {
			continue
		}

		var obj unstructured.Unstructured
		if err := json.Unmarshal(rawResource, &obj); err != nil {
			return nil, fmt.Errorf("error unmarshalling resource: %w", err)
		}

		// Check if this is an HTTPRoute
		if obj.GetKind() != "HTTPRoute" {
			continue
		}

		// Get hostname
		hostnames, found, err := unstructured.NestedStringSlice(obj.Object, "spec", "hostnames")
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
		// Construct the invoke URL
		port := config.GetConfig().DefaultGatewayPort
		url := fmt.Sprintf("http://%s:%d", hostname, port)
		if pathValue != "" {
			url = fmt.Sprintf("http://%s:%d%s", hostname, port, pathValue)
		}

		endpoints = append(endpoints, models.Endpoint{
			URL:        url,
			Visibility: "Public",
		})

	}

	return endpoints, nil
}

func extractPathValue(obj unstructured.Unstructured) (string, error) {
	// Get path from rules[0].matches[0].path.value
	rules, found, err := unstructured.NestedSlice(obj.Object, "spec", "rules")
	if err != nil || !found || len(rules) == 0 {
		return "", fmt.Errorf("HTTPRoute missing rules")
	}

	rule0, ok := rules[0].(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid rule format in HTTPRoute")
	}

	matches, found, err := unstructured.NestedSlice(rule0, "matches")
	if err != nil || !found || len(matches) == 0 {
		return "", fmt.Errorf("HTTPRoute missing matches")
	}

	match0, ok := matches[0].(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid match format in HTTPRoute")
	}
	pathValue, found, err := unstructured.NestedString(match0, "path", "value")
	if err != nil || !found {
		return "", fmt.Errorf("HTTPRoute missing path value")
	}
	return pathValue, nil
}

// findDeployedImageFromEnvRelease extracts the deployed image from the Deployment resource in the Release
func findDeployedImageFromEnvRelease(envRelease *v1alpha1.Release) string {
	if envRelease == nil || envRelease.Spec.Resources == nil {
		return ""
	}

	// Iterate through resources to find the Deployment
	for i := range envRelease.Spec.Resources {
		rawResource := envRelease.Spec.Resources[i].Object.Raw
		if len(rawResource) == 0 {
			continue
		}

		// Unmarshal the RawExtension to extract the object
		var obj unstructured.Unstructured
		if err := json.Unmarshal(rawResource, &obj); err != nil {
			continue
		}

		// Check if this is a Deployment
		if obj.GetKind() != "Deployment" {
			continue
		}

		// Extract image from spec.template.spec.containers[].image
		containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
		if err != nil || !found {
			continue
		}

		for _, container := range containers {
			containerMap, ok := container.(map[string]interface{})
			if !ok {
				continue
			}
			// Look for the "main" container
			if name, ok := containerMap["name"].(string); ok && name == "main" {
				if image, ok := containerMap["image"].(string); ok {
					return image
				}
			}
		}

	}

	return ""
}

func MapConditionsToBuildSteps(conditions []metav1.Condition) []models.BuildStep {
	steps := []models.BuildStep{
		{Type: string(BuildStatusInitiated), Status: string(BuildStepStatusPending)},
		{Type: string(BuildStatusTriggered), Status: string(BuildStepStatusPending)},
		{Type: string(BuildStatusRunning), Status: string(BuildStepStatusPending)},
		{Type: string(BuildStatusCompleted), Status: string(BuildStepStatusPending)},
		{Type: string(WorkloadUpdated), Status: string(BuildStepStatusPending)},
	}

	// Helper to find condition
	findCondition := func(condType string) *metav1.Condition {
		for i := range conditions {
			if conditions[i].Type == condType {
				return &conditions[i]
			}
		}
		return nil
	}

	workflowCompleted := findCondition(string(ConditionWorkflowCompleted))
	workflowRunning := findCondition(string(ConditionWorkflowRunning))
	workflowSucceeded := findCondition(string(ConditionWorkflowSucceeded))
	workflowFailed := findCondition(string(ConditionWorkflowFailed))
	workloadUpdated := findCondition(string(ConditionWorkloadUpdated))

	// Step 1: BuildInitiated (always succeeded if ComponentWorkflowRun exists)
	steps[StepIndexInitiated].Status = string(BuildStepStatusSucceeded)
	steps[StepIndexInitiated].Message = "Build initiated"

	// Step 2: BuildTriggered (workflow created)
	if workflowCompleted != nil || workflowRunning != nil {
		steps[StepIndexTriggered].Status = string(BuildStepStatusSucceeded)
		steps[StepIndexTriggered].Message = "Build triggered"
		if workflowCompleted != nil {
			steps[StepIndexTriggered].StartedAt = &workflowCompleted.LastTransitionTime.Time
			steps[StepIndexTriggered].FinishedAt = &workflowCompleted.LastTransitionTime.Time
		}
	}

	// Step 3: BuildRunning
	if workflowRunning != nil && workflowRunning.Status == metav1.ConditionTrue {
		steps[StepIndexRunning].Status = string(BuildStepStatusRunning)
		steps[StepIndexRunning].Message = "Build running"
		steps[StepIndexRunning].StartedAt = &workflowRunning.LastTransitionTime.Time
	} else if workflowCompleted != nil && workflowCompleted.Status == metav1.ConditionTrue {
		steps[StepIndexRunning].Status = string(BuildStepStatusSucceeded)
		steps[StepIndexRunning].Message = "Build execution finished"
		if workflowRunning != nil {
			steps[StepIndexRunning].StartedAt = &workflowRunning.LastTransitionTime.Time
		}
		steps[StepIndexRunning].FinishedAt = &workflowCompleted.LastTransitionTime.Time
	}

	// Step 4: BuildCompleted (succeeded or failed)
	if workflowFailed != nil && workflowFailed.Status == metav1.ConditionTrue {
		steps[StepIndexCompleted].Status = string(BuildStepStatusFailed)
		steps[StepIndexCompleted].Message = workflowFailed.Message
		steps[StepIndexCompleted].StartedAt = &workflowFailed.LastTransitionTime.Time
		steps[StepIndexCompleted].FinishedAt = &workflowFailed.LastTransitionTime.Time
	} else if workflowSucceeded != nil && workflowSucceeded.Status == metav1.ConditionTrue {
		steps[StepIndexCompleted].Status = string(BuildStepStatusSucceeded)
		steps[StepIndexCompleted].Message = "Build completed successfully"
		steps[StepIndexCompleted].StartedAt = &workflowSucceeded.LastTransitionTime.Time
		steps[StepIndexCompleted].FinishedAt = &workflowSucceeded.LastTransitionTime.Time
	}

	// Step 5: WorkloadUpdated (final deployment step)
	if workloadUpdated != nil && workloadUpdated.Status == metav1.ConditionTrue {
		steps[StepIndexWorkloadUpdated].Status = string(BuildStepStatusSucceeded)
		steps[StepIndexWorkloadUpdated].Message = "Workload updated successfully"
		steps[StepIndexWorkloadUpdated].StartedAt = &workloadUpdated.LastTransitionTime.Time
		steps[StepIndexWorkloadUpdated].FinishedAt = &workloadUpdated.LastTransitionTime.Time
	} else if workflowSucceeded != nil && workflowSucceeded.Status == metav1.ConditionTrue {
		steps[StepIndexWorkloadUpdated].Status = string(BuildStepStatusRunning)
		steps[StepIndexWorkloadUpdated].Message = "Updating workload"
	} else if workflowFailed != nil && workflowFailed.Status == metav1.ConditionTrue {
		steps[StepIndexWorkloadUpdated].Status = string(BuildStepStatusPending)
		steps[StepIndexWorkloadUpdated].Message = "Workload update skipped"
	}
	return steps
}

// calculateBuildPercentage determines completion percentage based on build steps
// Each step has specific percentage values: BuildInitiated=10%, BuildTriggered=30%, BuildRunning=50%, BuildCompleted=80%, BuildSucceeded=100%
func calculateBuildPercentage(steps []models.BuildStep) *float32 {
	percentage := float32(0)
	totalSteps := float32(len(steps))

	if totalSteps == 0 {
		return &percentage
	}

	completedSteps := float32(0)

	for _, step := range steps {
		if step.Status == string(BuildStepStatusSucceeded) {
			completedSteps++
		} else if step.Status == string(BuildStepStatusRunning) {
			// Running step counts as 0.5 completed
			completedSteps += 0.5
			break // Don't count subsequent steps
		} else if step.Status == string(BuildStepStatusFailed) {
			// If failed, stop counting and return current percentage
			break
		} else {
			// Pending steps, stop counting
			break
		}
	}

	percentage = (completedSteps / totalSteps) * 100
	return &percentage
}

func findBuildEndTime(conditions []metav1.Condition) *metav1.Time {
	var workloadUpdatedTime *metav1.Time

	for _, condition := range conditions {
		switch condition.Type {
		case string(ConditionWorkflowFailed):
			// If workflow failed, return its timestamp
			if condition.Status == metav1.ConditionTrue {
				return &condition.LastTransitionTime
			}
		case string(ConditionWorkloadUpdated):
			// If workload was updated (completed), store this timestamp
			if condition.Status == metav1.ConditionTrue {
				workloadUpdatedTime = &condition.LastTransitionTime
			}
		case string(ConditionWorkflowSucceeded):
			// If workflow succeeded but workload not yet updated, store this timestamp
			if condition.Status == metav1.ConditionTrue && workloadUpdatedTime == nil {
				workloadUpdatedTime = &condition.LastTransitionTime
			}
		}
	}
	return workloadUpdatedTime
}

// buildEnvironmentOrder creates an ordered list of environments based on the promotion paths
func buildEnvironmentOrder(promotionPaths []models.PromotionPath) []string {
	if len(promotionPaths) == 0 {
		return []string{}
	}

	var environmentOrder []string
	visited := make(map[string]bool)

	// Start with the first source environment
	for _, path := range promotionPaths {
		if !visited[path.SourceEnvironmentRef] {
			environmentOrder = append(environmentOrder, path.SourceEnvironmentRef)
			visited[path.SourceEnvironmentRef] = true
		}

		// Add target environments in order
		for _, target := range path.TargetEnvironmentRefs {
			if !visited[target.Name] {
				environmentOrder = append(environmentOrder, target.Name)
				visited[target.Name] = true
			}
		}
	}

	return environmentOrder
}

// buildPromotionPaths converts v1alpha1 PromotionPaths to models.PromotionPath
func buildPromotionPaths(promoPaths []v1alpha1.PromotionPath) []models.PromotionPath {
	promotionPaths := make([]models.PromotionPath, 0, len(promoPaths))
	for _, path := range promoPaths {
		targetRefs := make([]models.TargetEnvironmentRef, 0, len(path.TargetEnvironmentRefs))
		for _, target := range path.TargetEnvironmentRefs {
			targetRefs = append(targetRefs, models.TargetEnvironmentRef{
				Name: target.Name,
			})
		}
		promotionPaths = append(promotionPaths, models.PromotionPath{
			SourceEnvironmentRef:  path.SourceEnvironmentRef,
			TargetEnvironmentRefs: targetRefs,
		})
	}
	return promotionPaths
}

func findLowestEnvironment(promotionPaths []models.PromotionPath) string {
	if len(promotionPaths) == 0 {
		return ""
	}

	// Collect all target environments
	targets := make(map[string]bool)
	for _, path := range promotionPaths {
		for _, target := range path.TargetEnvironmentRefs {
			targets[target.Name] = true
		}
	}

	// Find a source environment that is not a target
	for _, path := range promotionPaths {
		if !targets[path.SourceEnvironmentRef] {
			return path.SourceEnvironmentRef
		}
	}
	return ""
}
