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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openchoreo/openchoreo/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

// getBuildTemplateForLanguage determines the appropriate build template based on the programming language
func getBuildTemplateForLanguage(language string) BuildTemplateNames {
	for _, buildpack := range utils.Buildpacks {
		if buildpack.Language == language {
			if buildpack.Provider == "Google" {
				return GoogleBuildpackBuildTemplate
			}
			if buildpack.Provider == "AMP-Ballerina" {
				return BallerinaBuildpackBuildTemplate
			}
		}
	}
	return ""
}

func getLanguageVersionEnvVariable(language string) string {
	for _, buildpack := range utils.Buildpacks {
		if buildpack.Language == language {
			return buildpack.VersionEnvVariable
		}
	}
	return ""
}

func createComponentCR(orgName, projName string, req *spec.CreateAgentRequest) *v1alpha1.Component {
	annotations := map[string]string{
		string(AnnotationKeyDisplayName): req.DisplayName,
		string(AnnotationKeyDescription): utils.StrPointerAsStr(req.Description, ""),
	}

	labels := map[string]string{
		string(LabelKeyComponentType): AgentComponentType,
	}

	// Determine build template based on language
	templateRefName := getBuildTemplateForLanguage(req.RuntimeConfigs.Language)
	params := []v1alpha1.Parameter{}
	if templateRefName == GoogleBuildpackBuildTemplate {
		params = []v1alpha1.Parameter{
			{
				Name:  GoogleEntryPoint,
				Value: utils.StrPointerAsStr(req.RuntimeConfigs.RunCommand, ""),
			},
			{
				Name:  LanguageVersion,
				Value: utils.StrPointerAsStr(req.RuntimeConfigs.LanguageVersion, ""),
			},
			{
				Name:  LanguageVersionKey,
				Value: getLanguageVersionEnvVariable(req.RuntimeConfigs.Language),
			},
		}
	}

	templateRef := v1alpha1.TemplateRef{
		Name:       string(templateRefName),
		Parameters: params,
	}

	return &v1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Namespace:   orgName,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: v1alpha1.ComponentSpec{
			Owner: v1alpha1.ComponentOwner{
				ProjectName: projName,
			},
			Type: v1alpha1.ComponentTypeService,
			Build: v1alpha1.BuildSpecInComponent{
				Repository: v1alpha1.BuildRepository{
					URL:     req.Provisioning.Repository.Url,
					AppPath: req.Provisioning.Repository.AppPath,
					Revision: v1alpha1.BuildRevision{
						Branch: req.Provisioning.Repository.Branch,
					},
				},
				TemplateRef: templateRef,
			},
		},
	}
}

func createBuildCR(orgName, projName, componentName, commitId string, component *v1alpha1.Component) *v1alpha1.Build {
	buildUUID := uuid.New().String()
	buildID := strings.ReplaceAll(buildUUID[:8], "-", "")
	buildName := fmt.Sprintf("%s-build-%s", componentName, buildID)

	return &v1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildName,
			Namespace: orgName,
			Labels: map[string]string{
				string(LabelKeyOrganizationName): orgName,
				string(LabelKeyProjectName):      projName,
				string(LabelKeyComponentName):    componentName,
			},
		},
		Spec: v1alpha1.BuildSpec{
			Owner: v1alpha1.BuildOwner{
				ProjectName:   projName,
				ComponentName: componentName,
			},
			Repository: v1alpha1.Repository{
				URL: component.Spec.Build.Repository.URL,
				Revision: v1alpha1.Revision{
					Branch: component.Spec.Build.Repository.Revision.Branch,
					Commit: commitId,
				},
				AppPath: component.Spec.Build.Repository.AppPath,
			},
			TemplateRef: component.Spec.Build.TemplateRef,
		},
	}
}

func createWorkloadCR(orgName, projName, componentName string, envVars []spec.EnvironmentVariable, endpointDetails map[string]spec.EndpointSpec, imageId string) *v1alpha1.Workload {
	var envs []v1alpha1.EnvVar

	workloadName := componentName + "-workload"
	workloadSpec := v1alpha1.WorkloadSpec{
		Owner: v1alpha1.WorkloadOwner{
			ProjectName:   projName,
			ComponentName: componentName,
		},
		WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
			Containers: map[string]v1alpha1.Container{
				"main": {
					Image: imageId,
					Env: func() []v1alpha1.EnvVar {
						for _, env := range envVars {
							envs = append(envs, v1alpha1.EnvVar{
								Key:   env.Key,
								Value: env.Value,
							})
						}
						return envs
					}(),
				},
			},
			Endpoints: func() map[string]v1alpha1.WorkloadEndpoint {
				endpoints := make(map[string]v1alpha1.WorkloadEndpoint)
				for name, endpointSpec := range endpointDetails {
					endpoints[name] = v1alpha1.WorkloadEndpoint{
						Type: v1alpha1.EndpointTypeHTTP,
						Port: endpointSpec.Port,
						Schema: &v1alpha1.Schema{
							Type:    string(v1alpha1.EndpointTypeREST),
							Content: endpointSpec.Schema.Content,
						},
					}
				}
				return endpoints
			}(),
		},
	}

	return &v1alpha1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workloadName,
			Namespace: orgName,
		},
		Spec: workloadSpec,
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
			RepoURL: component.Spec.Build.Repository.URL,
			Branch:  component.Spec.Build.Repository.Revision.Branch,
			AppPath: component.Spec.Build.Repository.AppPath,
		},
		BuildTemplateRef: component.Spec.Build.TemplateRef.Name,
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

func GetLatestBuildStatus(buildConditions []metav1.Condition) string {
	if len(buildConditions) == 0 {
		return statusUnknown
	}

	// Define the order of priority for build conditions (latest to earliest)
	// WorkloadUpdated > BuildCompleted > BuildTriggered > BuildInitiated
	conditionOrder := []string{
		string(ConditionWorkloadUpdated),
		string(ConditionBuildCompleted),
		string(ConditionBuildTriggered),
		string(ConditionBuildInitiated),
	}

	// Find the latest condition based on priority order
	for _, conditionType := range conditionOrder {
		for _, condition := range buildConditions {
			if condition.Type == conditionType {
				if condition.Type == string(ConditionWorkloadUpdated) && condition.Status == metav1.ConditionTrue {
					return statusCompleted
				}
				return condition.Reason
			}
		}
	}

	return statusUnknown
}

// createEndpointDetails creates endpoint specifications based on the input interface type
func createEndpointDetails(agentName string, inputInterface spec.InputInterface) (map[string]spec.EndpointSpec, error) {
	endpointDetails := make(map[string]spec.EndpointSpec)

	if inputInterface.Type == EndpointTypeCustom && inputInterface.CustomOpenAPISpec != nil {
		// Assume a single endpoint
		endpointName := fmt.Sprintf("%s-endpoint", agentName)
		endpointDetails[endpointName] = spec.EndpointSpec{
			Port:   inputInterface.CustomOpenAPISpec.Port,
			Schema: inputInterface.CustomOpenAPISpec.Schema,
		}
		return endpointDetails, nil
	}

	if inputInterface.Type == EndpointTypeDefault {
		// Create a default endpoint with POST /invocations
		endpointName := fmt.Sprintf("%s-endpoint", agentName)
		defaultOpenAPISchema, err := getDefaultOpenAPISchema()
		if err != nil {
			return nil, err
		}
		endpointDetails[endpointName] = spec.EndpointSpec{
			Port: int32(config.GetConfig().DefaultHTTPPort),
			Schema: spec.EndpointSchema{
				Content: utils.StrPointerAsStr(defaultOpenAPISchema, ""),
			},
		}
		return endpointDetails, nil
	}

	return nil, fmt.Errorf("unsupported InputInterface.Type: %q", inputInterface.Type)
}

func toBuildDetailsResponse(build *v1alpha1.Build) (*models.BuildDetailsResponse, error) {
	commitId := build.Spec.Repository.Revision.Commit
	if commitId == "" {
		commitId = "latest"
	}

	buildResp := &models.BuildDetailsResponse{
		BuildResponse: models.BuildResponse{
			UUID:        string(build.UID),
			Name:        build.Name,
			AgentName:   build.Spec.Owner.ComponentName,
			ProjectName: build.Spec.Owner.ProjectName,
			CommitID:    commitId,
			Status:      GetLatestBuildStatus(build.Status.Conditions),
			StartedAt:   build.CreationTimestamp.Time,
			Branch:      build.Spec.Repository.Revision.Branch,
			Image:       build.Status.ImageStatus.Image,
		},
	}

	// Convert conditions to build steps
	buildResp.Steps = extractBuildStepsFromConditions(build.Status.Conditions)

	// Calculate build completion percentage
	if percentage := calculateBuildPercentage(build.Status.Conditions); percentage != nil {
		buildResp.Percent = *percentage
	}

	// Set end time if build is completed
	if endTime := findBuildEndTime(build.Status.Conditions); endTime != nil {
		buildResp.EndedAt = &endTime.Time
		// Calculate duration in seconds
		duration := endTime.Sub(build.CreationTimestamp.Time).Seconds()
		buildResp.DurationSeconds = int32(duration)
	}

	return buildResp, nil
}

func toDeploymentDetailsResponse(sb *v1alpha1.ServiceBinding, environmentMap map[string]*models.EnvironmentResponse, promotionPaths []models.PromotionPath) *models.DeploymentResponse {
	if sb == nil {
		return nil
	}

	// Extract deployment status from conditions
	status := mapConditionToBindingStatus(sb.Status.Conditions)

	// Extract last deployed time from conditions
	var lastDeployedTime time.Time
	if len(sb.Status.Conditions) > 0 {
		lastDeployedTime = sb.Status.Conditions[0].LastTransitionTime.Time
	}

	// Extract endpoints from ServiceBinding status
	endpoints := extractEndpointsFromServiceBinding(sb)

	environment := sb.Spec.Environment
	// Get environment display name
	var environmentDisplayName string
	if env, exists := environmentMap[environment]; exists {
		environmentDisplayName = env.DisplayName
	}

	// Find promotion target environment for this environment (linear promotion)
	promotionTargetEnv := findPromotionTargetEnvironment(environment, promotionPaths, environmentMap)

	var imageId string
	if sb.Spec.WorkloadSpec.Containers != nil {
		if mainContainer, exists := sb.Spec.WorkloadSpec.Containers["main"]; exists {
			imageId = mainContainer.Image
		}
	}

	return &models.DeploymentResponse{
		ImageId:                    imageId,
		Status:                     status,
		Environment:                environment,
		EnvironmentDisplayName:     environmentDisplayName,
		PromotionTargetEnvironment: promotionTargetEnv,
		LastDeployedAt:             lastDeployedTime,
		Endpoints:                  endpoints,
	}
}

func mapConditionToBindingStatus(conditions []metav1.Condition) string {
	for _, condition := range conditions {
		if condition.Type == "Ready" {
			if condition.Status == metav1.ConditionTrue {
				return DeploymentStatusActive
			}
			switch condition.Reason {
			case "ResourcesSuspended", "ResourcesUndeployed":
				return DeploymentStatusSuspended
			case "ResourceHealthProgressing":
				return DeploymentStatusInProgress
			case "ResourceHealthDegraded", "ServiceClassNotFound", "APIClassNotFound":
				return DeploymentStatusFailed
			default:
				return DeploymentStatusNotDeployed
			}
		}
	}
	return DeploymentStatusNotDeployed
}

// extractEndpointsFromServiceBinding converts ServiceBinding endpoints to model endpoints
func extractEndpointsFromServiceBinding(sb *v1alpha1.ServiceBinding) []models.Endpoint {
	var endpoints []models.Endpoint
	if sb == nil || sb.Status.Endpoints == nil {
		return endpoints
	}

	for _, endpoint := range sb.Status.Endpoints {
		if endpoint.Public != nil {
			var endpointURL string
			var endpointVisibility string
			if endpoint.Public.URI != "" {
				endpointURL = endpoint.Public.URI
				endpointVisibility = "Public"
			}
			endpoints = append(endpoints, models.Endpoint{
				Name:       endpoint.Name,
				URL:        endpointURL,
				Visibility: endpointVisibility,
			})
		}
	}

	return endpoints
}

// extractBuildStepsFromConditions converts Kubernetes conditions to BuildStep models
func extractBuildStepsFromConditions(conditions []metav1.Condition) []models.BuildStep {
	var steps []models.BuildStep

	// Define the expected order of build conditions
	expectedTypes := []string{
		string(ConditionBuildInitiated),
		string(ConditionBuildTriggered),
		string(ConditionBuildCompleted),
		string(ConditionWorkloadUpdated),
	}

	// Convert each condition to a BuildStep
	for _, expectedType := range expectedTypes {
		for _, condition := range conditions {
			if condition.Type == expectedType {
				steps = append(steps, models.BuildStep{
					Type:    condition.Type,
					Status:  string(condition.Status),
					Message: condition.Message,
					At:      condition.LastTransitionTime.Time,
				})
				break
			}
		}
	}

	return steps
}

// calculateBuildPercentage determines completion percentage based on build conditions
// Each step has specific percentage values: BuildInitiated=10%, BuildTriggered=40%, BuildCompleted=80%, WorkloadUpdated=100%
func calculateBuildPercentage(conditions []metav1.Condition) *float32 {
	percentage := float32(0)

	// Check each condition and assign specific percentage values
	for _, condition := range conditions {
		if condition.Status == metav1.ConditionTrue {
			switch condition.Type {
			case string(ConditionBuildInitiated):
				if percentage < 10 {
					percentage = 10
				}
			case string(ConditionBuildTriggered):
				if percentage < 40 {
					percentage = 40
				}
			case string(ConditionBuildCompleted):
				if percentage < 80 {
					percentage = 80
				}
			case string(ConditionWorkloadUpdated):
				if percentage < 100 {
					percentage = 100
				}
			}
		}
	}

	return &percentage
}

// findBuildEndTime finds the build end time based on conditions:
// - If any stage failed (Status = False), use that stage's LastTransitionTime as end time
// - If no failures, use WorkloadUpdated stage's LastTransitionTime as end time
func findBuildEndTime(conditions []metav1.Condition) *metav1.Time {
	var workloadUpdatedTime *metav1.Time

	// First, check for any failed conditions (Status = False)
	for _, condition := range conditions {
		switch condition.Type {
		case string(ConditionBuildInitiated), string(ConditionBuildTriggered), string(ConditionBuildCompleted), string(ConditionWorkloadUpdated):
			if condition.Status == metav1.ConditionFalse {
				// If any stage failed, return its timestamp immediately
				return &condition.LastTransitionTime
			}
			// Store WorkloadUpdated timestamp for potential use if no failures
			if condition.Type == string(ConditionWorkloadUpdated) {
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
