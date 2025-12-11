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
	"os"
	"path/filepath"
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

// getDefaultOpenAPISchema reads and returns the default OpenAPI schema from file system
func getDefaultOpenAPISchema() (*string, error) {
	schemaPath := filepath.Join("clients", "openchoreosvc", "default-openapi-schema.yaml")

	content, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, err
	}
	schema := string(content)
	return &schema, nil
}

func getLanguageVersionEnvVariable(language string) string {
	for _, buildpack := range utils.Buildpacks {
		if buildpack.Language == language {
			return buildpack.VersionEnvVariable
		}
	}
	return ""
}

func getOpenChoreoComponentType(agentType string, agentSubType string) ComponentType {
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

func getContainerPort(req *spec.CreateAgentRequest) int32 {
	if req.AgentType.Type == string(utils.AgentTypeAPI) && req.AgentType.SubType == string(utils.AgentSubTypeChatAPI) {
		return int32(config.GetConfig().DefaultHTTPPort)
	}
	return req.InputInterface.Port
}

func createComponentCR(orgName, projectName string, req *spec.CreateAgentRequest) *v1alpha1.Component {
	annotations := map[string]string{
		string(AnnotationKeyDisplayName): req.DisplayName,
		string(AnnotationKeyDescription): utils.StrPointerAsStr(req.Description, ""),
	}
	componentType := getOpenChoreoComponentType(req.AgentType.Type, req.AgentType.SubType)
	componentWorkflow := getOpenChoreoComponentWorkflow(req.RuntimeConfigs.Language)
	containerPort := getContainerPort(req)

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
	}
	parametersJSON, _ := json.Marshal(parameters)

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
			},
			AutoDeploy: true,
			Parameters: &runtime.RawExtension{
				Raw: parametersJSON,
			},
		},
	}

	return componentCR
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

func createServiceCR(orgName, projName, componentName string, workloadName string, serviceClassName string, apis map[string]*v1alpha1.ServiceAPI) *v1alpha1.Service {
	serviceName := componentName + "-service"

	return &v1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: orgName,
		},
		Spec: v1alpha1.ServiceSpec{
			Owner: v1alpha1.ServiceOwner{
				ProjectName:   projName,
				ComponentName: componentName,
			},
			WorkloadName: workloadName,
			ClassName:    serviceClassName,
			APIs:         apis,
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
		CreatedAt:        component.CreationTimestamp.Time,
		Status:           "", // Todo: set status
		Description:      component.Annotations[string(AnnotationKeyDescription)],
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

func getComponentWorkflowStatus(buildConditions []metav1.Condition) string {
	if len(buildConditions) == 0 {
		return statusPending
	}

	// Check conditions in priority order
	// Similar to build workflow status logic
	for _, condition := range buildConditions {
		//  workflow has fully finished - not only did the build succeed, but the Workload CR was successfully created
		if condition.Type == string(ConditionWorkloadUpdated) && condition.Status == metav1.ConditionTrue {
			return statusCompleted
		}
	}

	for _, condition := range buildConditions {
		if condition.Type == string(ConditionWorkflowFailed) && condition.Status == metav1.ConditionTrue {
			return statusFailed
		}
	}

	for _, condition := range buildConditions {
		// workflow itself completed successfully, but the Workload CR may not have been create
		if condition.Type == string(ConditionWorkflowSucceeded) && condition.Status == metav1.ConditionTrue {
			return statusSucceeded
		}
	}

	for _, condition := range buildConditions {
		if condition.Type == string(ConditionWorkflowRunning) && condition.Status == metav1.ConditionTrue {
			return statusRunning
		}
	}

	return statusPending
}

// createEndpointDetails creates endpoint specifications based on the input interface type
func createEndpointDetails(agentName string, agentType spec.AgentType, inputInterface spec.InputInterface) (map[string]spec.InputInterface, error) {
	endpointDetails := make(map[string]spec.InputInterface)

	if agentType.SubType == string(utils.AgentSubTypeCustomAPI) {
		// Assume a single endpoint
		endpointName := fmt.Sprintf("%s-endpoint", agentName)
		endpointDetails[endpointName] = spec.InputInterface{
			Port:   inputInterface.Port,
			Schema: inputInterface.Schema,
		}
		return endpointDetails, nil
	}

	if agentType.SubType == string(utils.AgentSubTypeChatAPI) {
		// Create a default endpoint with POST /chat
		endpointName := fmt.Sprintf("%s-endpoint", agentName)
		defaultOpenAPISchema, err := getDefaultOpenAPISchema()
		if err != nil {
			return nil, err
		}
		endpointDetails[endpointName] = spec.InputInterface{
			Port: int32(config.GetConfig().DefaultHTTPPort),
			Schema: spec.InputInterfaceSchema{
				Path: utils.StrPointerAsStr(defaultOpenAPISchema, ""),
			},
		}
		return endpointDetails, nil
	}

	return nil, fmt.Errorf("unsupported InputInterface.Type: %q", inputInterface.Type)
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
			Status:      getComponentWorkflowStatus(componentWorkflow.Status.Conditions),
			StartedAt:   componentWorkflow.CreationTimestamp.Time,
			Branch:      componentWorkflow.Spec.Workflow.SystemParameters.Repository.Revision.Branch,
			Image:       componentWorkflow.Status.ImageStatus.Image,
		},
	}

	// Convert conditions to build steps
	buildResp.Steps = extractBuildStepsFromConditions(componentWorkflow.Status.Conditions)

	// Calculate build completion percentage
	if percentage := calculateBuildPercentage(componentWorkflow.Status.Conditions); percentage != nil {
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

func  determineReleaseBindingStatus(binding *v1alpha1.ReleaseBinding) string {
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
					URL: url,
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
									// Look for the "main" container or return the first one
									if name, ok := containerMap["name"].(string); ok && name == "main" {
										if image, ok := containerMap["image"].(string); ok {
											return image
										}
									}
								}
							}
							// If no "main" container found, return the first container's image
							if len(containers) > 0 {
								if containerMap, ok := containers[0].(map[string]interface{}); ok {
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

	return ""
}

// extractBuildStepsFromConditions converts Kubernetes conditions to BuildStep models
func extractBuildStepsFromConditions(conditions []metav1.Condition) []models.BuildStep {
	var steps []models.BuildStep

	// Process conditions in chronological order to build step sequence
	for _, condition := range conditions {
		var stepType string
		var stepStatus string
		var stepMessage string

		switch condition.Type {
		case string(ConditionWorkflowRunning):
			if condition.Status == metav1.ConditionTrue {
				stepType = "WorkflowRunning"
				stepStatus =  string(condition.Status)
				stepMessage = condition.Message
			}
		case string(ConditionWorkflowSucceeded):
			if condition.Status == metav1.ConditionTrue {
				stepType = "WorkflowSucceeded"
				stepStatus = string(condition.Status)
				stepMessage = condition.Message
			}
		case string(ConditionWorkflowFailed):
			if condition.Status == metav1.ConditionTrue {
				stepType = "WorkflowFailed"
				stepStatus =  string(condition.Status)
				stepMessage = condition.Message
			}
		case string(ConditionWorkloadUpdated):
			if condition.Status == metav1.ConditionTrue {
				stepType = "WorkloadUpdated"
				stepStatus =  string(condition.Status)
				stepMessage = condition.Message
			}
		default:
			// Skip unknown condition types
			continue
		}

		// Only add step if it has a valid type
		if stepType != "" {
			steps = append(steps, models.BuildStep{
				Type:    stepType,
				Status:  stepStatus,
				Message: stepMessage,
				At:      condition.LastTransitionTime.Time,
			})
		}
	}
	
	return steps
}

// calculateBuildPercentage determines completion percentage based on build conditions
// Each step has specific percentage values: BuildInitiated=10%, BuildTriggered=40%, BuildCompleted=80%, WorkloadUpdated=100%
func calculateBuildPercentage(conditions []metav1.Condition) *float32 {
	percentage := float32(0)

	// Check conditions in priority order
	for _, condition := range conditions {
		if condition.Type == string(ConditionWorkloadUpdated) && condition.Status == metav1.ConditionTrue {
			percentage = 100
			return &percentage
		}
	}

	for _, condition := range conditions {
		if condition.Type == string(ConditionWorkflowFailed) && condition.Status == metav1.ConditionTrue {
			// Keep current percentage if failed
			return &percentage
		}
	}

	for _, condition := range conditions {
		if condition.Type == string(ConditionWorkflowSucceeded) && condition.Status == metav1.ConditionTrue {
			percentage = 80
			return &percentage
		}
	}

	for _, condition := range conditions {
		if condition.Type == string(ConditionWorkflowRunning) && condition.Status == metav1.ConditionTrue {
			percentage = 40
			return &percentage
		}
	}

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
