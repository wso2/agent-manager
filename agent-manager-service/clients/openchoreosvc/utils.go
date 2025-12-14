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

func getOpenChoreoComponentType(agentType string) ComponentType {
	if agentType == string(utils.AgentTypeAPI) {
		return ComponentTypeAgentAPI
	}
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
	if req.AgentType.Type == string(utils.AgentTypeAPI) && req.AgentType.SubType == string(utils.AgentSubTypeChatAPI) {
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

func createComponentCR(orgName, projectName string, req *spec.CreateAgentRequest) *v1alpha1.Component {
	annotations := map[string]string{
		string(AnnotationKeyDisplayName): req.DisplayName,
		string(AnnotationKeyDescription): utils.StrPointerAsStr(req.Description, ""),
	}
	componentType := getOpenChoreoComponentType(req.AgentType.Type)
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

	parametersJSON, _ := json.Marshal(parameters)
	componentWorkflowParametersJSON, _ := json.Marshal(componentWorkflowParameters)

	componentCR := &v1alpha1.Component{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Component",
			APIVersion: "openchoreo.dev/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Namespace:   orgName,
			Annotations: annotations,
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

	// Add OpenTelemetry instrumentation trait for Python agents
	if req.AgentType.Type == string(utils.AgentTypeAPI) && req.RuntimeConfigs.Language == string(utils.LanguagePython) {
		componentCR.Spec.Traits = []v1alpha1.ComponentTrait{
			createOTELInstrumentationTrait(req),
		}
	}

	return componentCR
}

func createOTELInstrumentationTrait(req *spec.CreateAgentRequest) v1alpha1.ComponentTrait {
	traitParameters := map[string]interface{}{
		"instrumentationImage":  getInstrumentationImage(utils.StrPointerAsStr(req.RuntimeConfigs.LanguageVersion, "")),
		"sdkVolumeName":         config.GetConfig().OTEL.SDKVolumeName,
		"sdkMountPath":          config.GetConfig().OTEL.SDKMountPath,
		"agentName":             req.Name,
		"otelEndpoint":          config.GetConfig().OTEL.ExporterEndpoint,
		"isTraceContentEnabled": utils.BoolAsString(config.GetConfig().OTEL.IsTraceContentEnabled),
	}
	traitParametersJSON, _ := json.Marshal(traitParameters)

	return v1alpha1.ComponentTrait{
		Name:         string(TraitTypeOTELInstrumentation),
		InstanceName: fmt.Sprintf("%s-%s", req.Name, string(TraitTypeOTELInstrumentation)),
		Parameters: &runtime.RawExtension{
			Raw: traitParametersJSON,
		},
	}
}

func getInstrumentationImage(languageVersion string) string {
	// Extract major.minor version (e.g., "3.10.5" -> "3.10")
	parts := strings.Split(languageVersion, ".")
	if len(parts) >= 2 {
		majorMinor := parts[0] + "." + parts[1]
		switch majorMinor {
		case "3.10":
			return ""
		case "3.11":
			return "ghcr.io/agent-mgt-platform/otel-tracing-instrumentation:python3.11@sha256:d06e28a12e4a83edfcb8e4f6cb98faf5950266b984156f3192433cf0f903e529"
		case "3.12":
			return ""
		case "3.13":
			return ""
		}
	}
	return ""
}

func createComponentWorkflowRunCR(orgName, projName, componentName string, systemParams v1alpha1.SystemParametersValues, component *v1alpha1.Component) *v1alpha1.ComponentWorkflowRun {
	// Generate a unique workflow run name with short UUID
	uuid := uuid.New().String()
	workflowUuid := strings.ReplaceAll(uuid[:8], "-", "")
	workflowRunName := fmt.Sprintf("%s-workflow-%s", component.Name, workflowUuid)

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
	return &AgentComponent{
		Name:        component.Name,
		DisplayName: component.Annotations[string(AnnotationKeyDisplayName)],
		ProjectName: component.Spec.Owner.ProjectName,
		Repository: Repository{
			RepoURL: component.Spec.Workflow.SystemParameters.Repository.URL,
			Branch:  component.Spec.Workflow.SystemParameters.Repository.Revision.Branch,
			AppPath: component.Spec.Workflow.SystemParameters.Repository.AppPath,
		},
		CreatedAt:   component.CreationTimestamp.Time,
		Status:      "", // Todo: set status
		Description: component.Annotations[string(AnnotationKeyDescription)],
	}
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
	// Check if workflow was initiated (WorkflowCompleted condition exists)
	workflowCompletedCond := findStatusCondition(conditions, string(ConditionWorkflowCompleted))
	if workflowCompletedCond == nil {
		return BuildStatusInitiated // ComponentWorkflowRun just created
	}

	// Check if workload was updated (final state)
	if isStatusConditionTrue(conditions, string(ConditionWorkloadUpdated)) {
		return BuildStatusCompleted // Everything done
	}

	// Check if workflow is completed
	if workflowCompletedCond.Status == metav1.ConditionTrue {
		// Check success vs failure
		if isStatusConditionTrue(conditions, string(ConditionWorkflowSucceeded)) {
			return BuildStatusSucceeded
		}
		if isStatusConditionTrue(conditions, string(ConditionWorkflowFailed)) {
			return BuildStatusFailed
		}
		return BuildStatusCompleted // Completed but state unclear
	}

	// Check if workflow is running
	if isStatusConditionTrue(conditions, string(ConditionWorkflowRunning)) {
		return BuildStatusRunning
	}

	// Workflow pending/triggered but not yet running
	if workflowCompletedCond.Reason == string(ConditionWorkflowPending) {
		return BuildStatusTriggered // Argo Workflow created, pending execution
	}

	return BuildStatusInitiated // Default fallback
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

func toDeploymentDetailsResponse(binding *v1alpha1.ReleaseBinding, envRelease *v1alpha1.Release, environmentMap map[string]*models.EnvironmentResponse, promotionTargetEnv *models.PromotionTargetEnvironment) *models.DeploymentResponse {
	if binding == nil {
		return nil
	}

	// Extract deployment status from Release Binding
	status := determineReleaseBindingStatus(binding)

	// Extract last deployed time from conditions
	var lastDeployedTime time.Time
	if len(binding.Status.Conditions) > 0 {
		lastDeployedTime = binding.Status.Conditions[0].LastTransitionTime.Time
	}

	// Extract endpoints from EnvRelease status
	endpoints := extractEndpointURLFromEnvRelease(envRelease)
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
	}
}

func determineReleaseBindingStatus(binding *v1alpha1.ReleaseBinding) string {
	if len(binding.Status.Conditions) == 0 {
		return DeploymentStatusNotReady
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
		return DeploymentStatusNotReady
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
			return DeploymentStatusNotReady
		}
	}

	// If all three conditions are present and none are degraded, it's ready
	return DeploymentStatusActive
}

// extractEndpointsFromEnvReleaseBinding converts EnvRelease endpoints to model endpoints
func extractEndpointURLFromEnvRelease(envRelease *v1alpha1.Release) []models.Endpoint {
	var endpoints []models.Endpoint

	if envRelease == nil || envRelease.Spec.Resources == nil {
		return endpoints
	}

	// Check spec.resources for HTTPRoute definitions
	specResources := envRelease.Spec.Resources

	// Find all HTTPRoute objects in spec resources
	for i := range specResources {
		resource := &specResources[i]

		// Unmarshal the RawExtension to extract the object
		if len(resource.Object.Raw) > 0 {
			var objMap map[string]interface{}
			if err := json.Unmarshal(resource.Object.Raw, &objMap); err != nil {
				continue
			}

			// Check if this is an HTTPRoute
			if kind, ok := objMap["kind"].(string); !ok || kind != "HTTPRoute" {
				continue
			}

			// Extract hostname and path from the HTTPRoute spec
			var hostname string
			var pathValue string

			if spec, ok := objMap["spec"].(map[string]interface{}); ok {
				// Get hostname
				if hostnames, ok := spec["hostnames"].([]interface{}); ok && len(hostnames) > 0 {
					hostname, _ = hostnames[0].(string)
				}

				// Get path from rules
				if rules, ok := spec["rules"].([]interface{}); ok && len(rules) > 0 {
					if rule, ok := rules[0].(map[string]interface{}); ok {
						// Get path from matches
						if matches, ok := rule["matches"].([]interface{}); ok && len(matches) > 0 {
							if match, ok := matches[0].(map[string]interface{}); ok {
								if path, ok := match["path"].(map[string]interface{}); ok {
									pathValue, _ = path["value"].(string)
								}
							}
						}
					}
				}
			}

			// Construct the invoke URL if hostname is available
			if hostname != "" {
				url := fmt.Sprintf("http://%s:9080", hostname)
				if pathValue != "" {
					url = fmt.Sprintf("http://%s:9080%s", hostname, pathValue)
				}

				endpoints = append(endpoints, models.Endpoint{
					URL:        url,
					Visibility: "Public",
				})
			}
		}
	}

	return endpoints
}

// findDeployedImageFromEnvRelease extracts the deployed image from the Deployment resource in the Release
func findDeployedImageFromEnvRelease(envRelease *v1alpha1.Release) string {
	if envRelease == nil || envRelease.Spec.Resources == nil {
		return ""
	}

	// Iterate through resources to find the Deployment
	for i := range envRelease.Spec.Resources {
		resource := &envRelease.Spec.Resources[i]

		// Unmarshal the RawExtension to extract the object
		if len(resource.Object.Raw) > 0 {
			var objMap map[string]interface{}
			if err := json.Unmarshal(resource.Object.Raw, &objMap); err != nil {
				continue
			}

			// Check if this is a Deployment
			if kind, ok := objMap["kind"].(string); !ok || kind != "Deployment" {
				continue
			}

			// Extract image from spec.template.spec.containers[].image
			if spec, ok := objMap["spec"].(map[string]interface{}); ok {
				if template, ok := spec["template"].(map[string]interface{}); ok {
					if podSpec, ok := template["spec"].(map[string]interface{}); ok {
						if containers, ok := podSpec["containers"].([]interface{}); ok {
							for _, container := range containers {
								if containerMap, ok := container.(map[string]interface{}); ok {
									// Look for the "main" container
									if name, ok := containerMap["name"].(string); ok && name == "main" {
										if image, ok := containerMap["image"].(string); ok {
											return image
										}
									}
								}
							}
						}
					}
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
		{Type: string(BuildStatusSucceeded), Status: string(BuildStepStatusPending)},
		{Type: string(BuildStatusCompleted), Status: string(BuildStepStatusPending)},
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
	// Step 1: Build Initiated (always true if ComponentWorkflowRun exists)
	steps[0].Status = string(BuildStepStatusSucceeded)
	steps[0].Message = "ComponentWorkflowRun created"

	// Step 2: Build Triggered (WorkflowCompleted condition exists)
	if workflowCompleted != nil {
		steps[1].Status = string(BuildStepStatusSucceeded)
		steps[1].Message = "Argo Workflow resource created"
		steps[1].StartedAt = &workflowCompleted.LastTransitionTime.Time
		steps[1].FinishedAt = &workflowCompleted.LastTransitionTime.Time
	}

	// Step 3: Build Running (WorkflowRunning = True)
	if workflowRunning != nil && workflowRunning.Status == metav1.ConditionTrue {
		steps[2].Status = string(BuildStepStatusRunning)
		steps[2].Message = workflowRunning.Message
		steps[2].StartedAt = &workflowRunning.LastTransitionTime.Time
	} else if workflowCompleted != nil && workflowCompleted.Status == metav1.ConditionTrue {
		// Workflow completed, so running step is done
		steps[2].Status = string(BuildStepStatusSucceeded)
		steps[2].Message = "Build execution completed"
		if workflowRunning != nil {
			steps[2].StartedAt = &workflowRunning.LastTransitionTime.Time
			steps[2].FinishedAt = &workflowCompleted.LastTransitionTime.Time
		}
	} else if workflowCompleted != nil {
		// Workflow initiated but not running yet
		steps[2].Status = string(BuildStepStatusPending)
		steps[2].Message = "Waiting for workflow to start"
	}

	// Step 4: Build Completed
	if workflowCompleted != nil && workflowCompleted.Status == metav1.ConditionTrue {
		if workflowSucceeded != nil && workflowSucceeded.Status == metav1.ConditionTrue {
			steps[3].Status = string(BuildStepStatusSucceeded)
			steps[3].Message = "Build completed successfully"
			steps[3].FinishedAt = &workflowSucceeded.LastTransitionTime.Time
		} else if workflowFailed != nil && workflowFailed.Status == metav1.ConditionTrue {
			steps[3].Status = string(BuildStepStatusFailed)
			steps[3].Message = workflowFailed.Message
			steps[3].FinishedAt = &workflowFailed.LastTransitionTime.Time
		} else {
			steps[3].Status = string(BuildStepStatusSucceeded)
			steps[3].Message = "Build workflow completed"
			steps[3].FinishedAt = &workflowCompleted.LastTransitionTime.Time
		}
		steps[3].StartedAt = &workflowCompleted.LastTransitionTime.Time
	}

	// Step 5: Workload Updated
	if workloadUpdated != nil && workloadUpdated.Status == metav1.ConditionTrue {
		steps[4].Status = string(BuildStepStatusSucceeded)
		steps[4].Message = workloadUpdated.Message
		steps[4].StartedAt = &workloadUpdated.LastTransitionTime.Time
		steps[4].FinishedAt = &workloadUpdated.LastTransitionTime.Time
	} else if workflowSucceeded != nil && workflowSucceeded.Status == metav1.ConditionTrue {
		// Build succeeded but workload not yet updated
		steps[4].Status = string(BuildStepStatusPending)
		steps[4].Message = "Updating workload CR"
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
