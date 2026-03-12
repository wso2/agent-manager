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
	"log/slog"
	"net/http"
	"sort"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

func (c *openChoreoClient) TriggerBuild(ctx context.Context, orgName, projectName, componentName, commitID string) (*models.BuildResponse, error) {
	params := &gen.CreateComponentWorkflowRunParams{}
	if commitID != "" {
		params.Commit = &commitID
	}

	resp, err := c.ocClient.CreateComponentWorkflowRunWithResponse(ctx, orgName, projectName, componentName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to trigger build: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated {
		return nil, handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON400: resp.JSON400,
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON201 == nil {
		return nil, fmt.Errorf("empty response from trigger build")
	}

	return toWorkflowRunBuild(resp.JSON201)
}

func (c *openChoreoClient) GetBuild(ctx context.Context, orgName, projectName, componentName, buildName string) (*models.BuildDetailsResponse, error) {
	resp, err := c.ocClient.GetComponentWorkflowRunWithResponse(ctx, orgName, projectName, componentName, buildName)
	if err != nil {
		return nil, fmt.Errorf("failed to get build: %w", err)
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
		return nil, fmt.Errorf("empty response from get build")
	}

	return toBuildDetailsResponse(resp.JSON200)
}

func (c *openChoreoClient) ListBuilds(ctx context.Context, orgName, projectName, componentName string) ([]*models.BuildResponse, error) {
	resp, err := c.ocClient.ListComponentWorkflowRunsWithResponse(ctx, orgName, projectName, componentName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list builds: %w", err)
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
		return []*models.BuildResponse{}, nil
	}

	workflowRuns := resp.JSON200.Items
	buildResponses := make([]*models.BuildResponse, 0, len(workflowRuns))
	for _, workflowRun := range workflowRuns {
		build, err := toWorkflowRunBuild(&workflowRun)
		if err != nil {
			slog.Error("failed to convert workflow run", "workflowRun", workflowRun.Name, "error", err)
			continue
		}
		buildResponses = append(buildResponses, build)
	}
	// Temporarily enrich build responses with input interface details by fetching the component.
	// fetch component
	component, err := c.GetComponent(ctx, orgName, projectName, componentName)
	if err != nil {
		slog.Error("failed to fetch component for build listing", "componentName", componentName, "error", err)
	} else {
		// Enrich builds with input interface details from component workflow parameters
		if component.Provisioning.Repository.Branch != "" {
			for _, build := range buildResponses {
				build.BuildParameters.Branch = component.Provisioning.Repository.Branch
			}
		}
	}

	// Sort by creation timestamp to ensure consistent ordering for pagination
	sort.Slice(buildResponses, func(i, j int) bool {
		return buildResponses[i].StartedAt.After(buildResponses[j].StartedAt)
	})

	return buildResponses, nil
}

func (c *openChoreoClient) UpdateComponentBuildParameters(ctx context.Context, namespaceName, projectName, componentName string, req UpdateComponentBuildParametersRequest) error {
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

	// Build updated workflow parameters
	updatedParams, err := buildUpdatedWorkflowParameters(componentName, workflowParams, req)
	if err != nil {
		return fmt.Errorf("failed to build workflow parameters: %w", err)
	}
	component.Spec.Workflow.Parameters = &updatedParams

	// If repository is updated, update systemParameters
	if req.Repository != nil {
		appPath := normalizePath(req.Repository.AppPath)
		component.Spec.Workflow.SystemParameters = &struct {
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
				AppPath: &appPath,
				Revision: &struct {
					Branch *string `json:"branch,omitempty"`
					Commit *string `json:"commit,omitempty"`
				}{
					Branch: &req.Repository.Branch,
				},
			},
		}
	}

	// Update spec.parameters.basePath and port if InputInterface is provided
	if req.InputInterface != nil {
		if component.Spec.Parameters == nil {
			params := make(map[string]interface{})
			component.Spec.Parameters = &params
		}
		parameters := *component.Spec.Parameters

		if req.InputInterface.BasePath != "" {
			parameters["basePath"] = req.InputInterface.BasePath
		}
		if req.InputInterface.Port > 0 {
			parameters["port"] = req.InputInterface.Port
		}
	}

	// Update the component
	updateResp, err := c.ocClient.UpdateComponentWithResponse(ctx, namespaceName, componentName, *component)
	if err != nil {
		return fmt.Errorf("failed to update component build parameters: %w", err)
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

// buildUpdatedWorkflowParameters builds workflow parameters from existing params
func buildUpdatedWorkflowParameters(componentName string, existingParams map[string]any, req UpdateComponentBuildParametersRequest) (map[string]any, error) {
	// Update build configs based on build type
	if req.Build != nil {
		if req.Build.Buildpack != nil {
			// Update buildpack configs
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
			existingParams["buildpackConfigs"] = buildpackConfigs
			delete(existingParams, "dockerConfigs") // Clean up docker configs when build type is Buildpack
		} else if req.Build.Docker != nil {
			// Update docker configs
			dockerConfigs := map[string]any{
				"dockerfilePath": normalizePath(req.Build.Docker.DockerfilePath),
			}
			existingParams["dockerConfigs"] = dockerConfigs
			delete(existingParams, "buildpackConfigs") // Clean up buildpack configs when build type is Docker
		}
	}

	// Update endpoints if InputInterface provided
	if req.InputInterface != nil {
		endpoints, err := buildEndpointsFromInputInterface(componentName, req.InputInterface, req.AgentType)
		if err != nil {
			return nil, fmt.Errorf("failed to build endpoints: %w", err)
		}
		existingParams["endpoints"] = endpoints
	}

	return existingParams, nil
}

// buildEndpointsFromInputInterface builds endpoint configuration from InputInterface
// For chat-api agents, uses default port from config; for custom-api, uses the provided port
func buildEndpointsFromInputInterface(componentName string, inputInterface *InputInterfaceConfig, agentType AgentTypeConfig) ([]map[string]any, error) {
	var port int32
	var basePath string

	// Use default port and basePath for chat-api agents, similar to buildEndpoints in components.go
	if agentType.Type == string(utils.AgentTypeAPI) && agentType.SubType == string(utils.AgentSubTypeChatAPI) {
		port = int32(config.GetConfig().DefaultChatAPI.DefaultHTTPPort)
		basePath = config.GetConfig().DefaultChatAPI.DefaultBasePath
	} else {
		port = inputInterface.Port
		basePath = inputInterface.BasePath
	}

	endpoints := []map[string]any{
		{
			"name":       fmt.Sprintf("%s-endpoint", componentName),
			"type":       inputInterface.Type,
			"port":       port,
			"basePath":   basePath,
			"visibility": DefaultEndpointVisibility,
		},
	}

	if inputInterface.SchemaPath != "" {
		endpoints[0]["schemaFilePath"] = inputInterface.SchemaPath
		endpoints[0]["schemaType"] = "REST"
	}
	return endpoints, nil
}

// toWorkflowRunBuild converts a gen.ComponentWorkflowRun to models.BuildResponse
func toWorkflowRunBuild(run *gen.ComponentWorkflowRun) (*models.BuildResponse, error) {
	commit := utils.StrPointerAsStr(run.Commit, "")
	if commit == "" {
		commit = "latest"
	} else {
		// Convert to short SHA (8 characters) to match workflow template behavior
		commit = utils.ToShortSHA(commit)
	}

	language, languageVersion, runCommand, _, err := extractWorkflowParameters(run.Workflow)
	if err != nil {
		return nil, fmt.Errorf("failed to extract build parameters: %w", err)
	}

	build := &models.BuildResponse{
		UUID:        utils.StrPointerAsStr(run.Uuid, ""),
		Name:        run.Name,
		AgentName:   run.ComponentName,
		ProjectName: run.ProjectName,
		Status:      utils.StrPointerAsStr(run.Status, ""),
		StartedAt:   run.CreatedAt,
		ImageId:     utils.StrPointerAsStr(run.Image, ""),
		BuildParameters: models.BuildParameters{
			CommitID:        commit,
			Language:        language,
			LanguageVersion: languageVersion,
			RunCommand:      runCommand,
		},
	}

	// Extract repo details from workflow system parameters
	if run.Workflow != nil && run.Workflow.SystemParameters != nil && run.Workflow.SystemParameters.Repository != nil {
		repo := run.Workflow.SystemParameters.Repository
		build.BuildParameters.RepoUrl = utils.StrPointerAsStr(repo.Url, "")
		build.BuildParameters.AppPath = utils.StrPointerAsStr(repo.AppPath, "")
		if repo.Revision != nil {
			build.BuildParameters.Branch = utils.StrPointerAsStr(repo.Revision.Branch, "")
		}
	}

	return build, nil
}

// toBuildDetailsResponse converts a gen.ComponentWorkflowRun to models.BuildDetailsResponse
func toBuildDetailsResponse(run *gen.ComponentWorkflowRun) (*models.BuildDetailsResponse, error) {
	build, err := toWorkflowRunBuild(run)
	if err != nil {
		return nil, fmt.Errorf("failed to build response: %w", err)
	}

	status := utils.StrPointerAsStr(run.Status, "")

	// Extract inputInterface from workflow parameters
	_, _, _, inputInterface, err := extractWorkflowParameters(run.Workflow)
	if err != nil {
		return nil, fmt.Errorf("failed to extract workflow parameters: %w", err)
	}

	details := &models.BuildDetailsResponse{
		BuildResponse:  *build,
		InputInterface: inputInterface,
	}

	// Map status to build steps
	details.Steps = mapStatusToBuildSteps(status)

	// Calculate build completion percentage
	if percentage := calculateBuildPercentage(details.Steps); percentage != nil {
		details.Percent = *percentage
	}

	return details, nil
}

// Initiated → Triggered → Running → Succeeded → Completed
func mapStatusToBuildSteps(apiStatus string) []models.BuildStep {
	steps := []models.BuildStep{
		{Type: string(BuildStatusInitiated), Status: string(BuildStepStatusSucceeded), Message: "Build initiated"},
		{Type: string(BuildStatusTriggered), Status: string(BuildStepStatusPending)},
		{Type: string(BuildStatusRunning), Status: string(BuildStepStatusPending)},
		{Type: string(BuildStatusSucceeded), Status: string(BuildStepStatusPending)},
		{Type: string(BuildStatusCompleted), Status: string(BuildStepStatusPending)},
	}

	switch apiStatus {
	// workflow succeeded AND the workload CR was successfully created/updated
	case WorkflowStatusCompleted:
		steps[StepIndexTriggered] = models.BuildStep{Type: string(BuildStatusTriggered), Status: string(BuildStepStatusSucceeded), Message: "Build triggered"}
		steps[StepIndexRunning] = models.BuildStep{Type: string(BuildStatusRunning), Status: string(BuildStepStatusSucceeded), Message: "Build execution finished"}
		steps[StepIndexCompleted] = models.BuildStep{Type: string(BuildStatusSucceeded), Status: string(BuildStepStatusSucceeded), Message: "Build workflow completed successfully"}
		steps[StepIndexWorkloadUpdated] = models.BuildStep{Type: string(BuildStatusCompleted), Status: string(BuildStepStatusSucceeded), Message: "Workload updated successfully"}
	// The workflow itself has completed, but the workload CR may not have been updated yet
	case WorkflowStatusSucceeded:
		steps[StepIndexTriggered] = models.BuildStep{Type: string(BuildStatusTriggered), Status: string(BuildStepStatusSucceeded), Message: "Build triggered"}
		steps[StepIndexRunning] = models.BuildStep{Type: string(BuildStatusRunning), Status: string(BuildStepStatusSucceeded), Message: "Build execution finished"}
		steps[StepIndexCompleted] = models.BuildStep{Type: string(BuildStatusSucceeded), Status: string(BuildStepStatusSucceeded), Message: "Build workflow completed successfully"}
		steps[StepIndexWorkloadUpdated] = models.BuildStep{Type: string(BuildStatusCompleted), Status: string(BuildStepStatusRunning), Message: "Updating workload"}
	case WorkflowStatusRunning:
		steps[StepIndexTriggered] = models.BuildStep{Type: string(BuildStatusTriggered), Status: string(BuildStepStatusSucceeded), Message: "Build triggered"}
		steps[StepIndexRunning] = models.BuildStep{Type: string(BuildStatusRunning), Status: string(BuildStepStatusRunning), Message: "Build running"}
	case WorkflowStatusPending:
		steps[StepIndexTriggered] = models.BuildStep{Type: string(BuildStatusTriggered), Status: string(BuildStepStatusSucceeded), Message: "Build triggered"}
	case WorkflowStatusFailed:
		steps[StepIndexTriggered] = models.BuildStep{Type: string(BuildStatusTriggered), Status: string(BuildStepStatusSucceeded), Message: "Build triggered"}
		steps[StepIndexRunning] = models.BuildStep{Type: string(BuildStatusRunning), Status: string(BuildStepStatusSucceeded), Message: "Build execution finished"}
		steps[StepIndexCompleted] = models.BuildStep{Type: string(BuildStatusSucceeded), Status: string(BuildStepStatusFailed), Message: "Build workflow failed"}
		steps[StepIndexWorkloadUpdated] = models.BuildStep{Type: string(BuildStatusCompleted), Status: string(BuildStepStatusPending), Message: "Workload update skipped"}
	}

	return steps
}

// calculateBuildPercentage determines completion percentage based on build steps.
// Each completed step advances the percentage; a running step counts as half.
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

// extractWorkflowParameters extracts language, languageVersion, runCommand and inputInterface
// from the workflow configuration parameters map.
func extractWorkflowParameters(workflow *gen.ComponentWorkflowConfig) (string, string, string, *models.InputInterface, error) {
	if workflow == nil || workflow.Parameters == nil {
		return "", "", "", nil, nil
	}

	// Marshal the parameters map to JSON, then unmarshal to our struct
	paramsJSON, err := json.Marshal(*workflow.Parameters)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("failed to marshal workflow parameters: %w", err)
	}

	var params workflowParameters
	if err := json.Unmarshal(paramsJSON, &params); err != nil {
		return "", "", "", nil, fmt.Errorf("failed to unmarshal workflow parameters: %w", err)
	}

	language := params.BuildpackConfigs.Language
	languageVersion := params.BuildpackConfigs.LanguageVersion
	runCommand := params.BuildpackConfigs.GoogleEntryPoint

	// Extract inputInterface from endpoints
	var inputInterface *models.InputInterface
	if len(params.Endpoints) > 0 {
		endpoint := params.Endpoints[0]
		inputInterface = &models.InputInterface{
			Type:       endpoint.Type,
			Port:       endpoint.Port,
			BasePath:   endpoint.BasePath,
			Visibility: endpoint.Visibility,
		}
		if endpoint.SchemaFilePath != "" {
			inputInterface.Schema = &models.InputInterfaceSchema{
				Path: endpoint.SchemaFilePath,
			}
		}
	}

	return language, languageVersion, runCommand, inputInterface, nil
}
