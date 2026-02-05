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

	if resp.StatusCode() != http.StatusCreated && resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
		})
	}

	if resp.JSON201 == nil || resp.JSON201.Data == nil {
		return nil, fmt.Errorf("empty response from trigger build")
	}

	return toWorkflowRunBuild(resp.JSON201.Data)
}

func (c *openChoreoClient) GetBuild(ctx context.Context, orgName, projectName, componentName, buildName string) (*models.BuildDetailsResponse, error) {
	resp, err := c.ocClient.GetComponentWorkflowRunWithResponse(ctx, orgName, projectName, componentName, buildName)
	if err != nil {
		return nil, fmt.Errorf("failed to get build: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{
			NotFoundErr: utils.ErrBuildNotFound,
		})
	}

	if resp.JSON200 == nil || resp.JSON200.Data == nil {
		return nil, fmt.Errorf("empty response from get build")
	}

	return toBuildDetailsResponse(resp.JSON200.Data)
}

func (c *openChoreoClient) ListBuilds(ctx context.Context, orgName, projectName, componentName string) ([]*models.BuildResponse, error) {
	resp, err := c.ocClient.ListComponentWorkflowRunsWithResponse(ctx, orgName, projectName, componentName)
	if err != nil {
		return nil, fmt.Errorf("failed to list builds: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
		})
	}

	if resp.JSON200 == nil || resp.JSON200.Data == nil || resp.JSON200.Data.Items == nil {
		return []*models.BuildResponse{}, nil
	}

	workflowRuns := *resp.JSON200.Data.Items
	buildResponses := make([]*models.BuildResponse, 0, len(workflowRuns))
	for _, workflowRun := range workflowRuns {
		build, err := toWorkflowRunBuild(&workflowRun)
		if err != nil {
			slog.Error("failed to convert workflow run", "workflowRun", workflowRun.Name, "error", err)
			continue
		}
		buildResponses = append(buildResponses, build)
	}

	// Sort by creation timestamp to ensure consistent ordering for pagination
	sort.Slice(buildResponses, func(i, j int) bool {
		return buildResponses[i].StartedAt.After(buildResponses[j].StartedAt)
	})

	return buildResponses, nil
}

func (c *openChoreoClient) UpdateComponentBuildParameters(ctx context.Context, namespaceName, projectName, componentName string, req UpdateComponentBuildParametersRequest) error {
	// Get existing component to fetch current workflow configuration
	resp, err := c.ocClient.GetComponentWithResponse(ctx, namespaceName, projectName, componentName, nil)
	if err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
		})
	}

	if resp.JSON200 == nil || resp.JSON200.Data == nil {
		return fmt.Errorf("empty response from get component")
	}

	component := resp.JSON200.Data

	// Build updated workflow parameters
	workflowParams, err := buildUpdatedWorkflowParameters(component, req)
	if err != nil {
		return fmt.Errorf("failed to build workflow parameters: %w", err)
	}

	// Build workflow section for the component CR
	workflowSection := map[string]interface{}{
		"parameters": workflowParams,
	}

	// If repository is updated, include systemParameters
	if req.Repository != nil {
		workflowSection["systemParameters"] = map[string]interface{}{
			"repository": map[string]interface{}{
				"url": req.Repository.URL,
				"revision": map[string]interface{}{
					"branch": req.Repository.Branch,
				},
				"appPath":    normalizePath(req.Repository.AppPath),
				"parameters": workflowParams,
			},
		}
	}

	// Build the ApplyResource request body
	body := gen.ApplyResourceJSONRequestBody{
		"apiVersion": ResourceAPIVersion,
		"kind":       ResourceKindComponent,
		"metadata": map[string]interface{}{
			"name":      componentName,
			"namespace": namespaceName,
			
		},
		"spec": map[string]interface{}{
			"workflow": workflowSection,
		},
	}

	// Apply the updated component
	applyResp, err := c.ocClient.ApplyResourceWithResponse(ctx, body)
	if err != nil {
		return fmt.Errorf("failed to update component build parameters: %w", err)
	}

	if applyResp.StatusCode() != http.StatusOK && applyResp.StatusCode() != http.StatusCreated {
		return handleErrorResponse(applyResp.StatusCode(), applyResp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
		})
	}

	return nil
}

// buildUpdatedWorkflowParameters builds workflow parameters based on the update request
func buildUpdatedWorkflowParameters(component *gen.ComponentResponse, req UpdateComponentBuildParametersRequest) (map[string]any, error) {
	// Start with existing workflow parameters if available
	existingParams := make(map[string]any)
	if component.ComponentWorkflow != nil && component.ComponentWorkflow.Parameters != nil {
		existingParams = *component.ComponentWorkflow.Parameters
	}

	// Update buildpack configs if RuntimeConfigs provided
	if req.RuntimeConfigs != nil {
		buildpackConfigs := make(map[string]any)
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
		existingParams["buildpackConfigs"] = buildpackConfigs
	}

	// Update endpoints if InputInterface provided
	if req.InputInterface != nil {
		endpoints, err := buildEndpointsFromInputInterface(component.Name,req.InputInterface)
		if err != nil {
			return nil, fmt.Errorf("failed to build endpoints: %w", err)
		}
		existingParams["endpoints"] = endpoints
	}

	return existingParams, nil
}

// buildEndpointsFromInputInterface builds endpoint configuration from InputInterface
func buildEndpointsFromInputInterface(componentName string,inputInterface *InputInterfaceConfig) ([]map[string]any, error) {
	endpoints := []map[string]any{
		{
			"name": fmt.Sprintf("%s-endpoint", componentName),
			"type": inputInterface.Type,
			"port": inputInterface.Port,
		},
	}

	if inputInterface.SchemaPath != "" {
		endpoints[0]["schemaFilePath"] = inputInterface.SchemaPath
	}

	return endpoints, nil
}

// toWorkflowRunBuild converts a gen.ComponentWorkflowRunResponse to models.BuildResponse
func toWorkflowRunBuild(run *gen.ComponentWorkflowRunResponse) (*models.BuildResponse, error) {
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

	fmt.Printf("build status %s", utils.StrPointerAsStr(run.Image, ""))

	build := &models.BuildResponse{
		UUID:        run.Uuid,
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
		build.BuildParameters.RepoUrl = repo.Url
		build.BuildParameters.AppPath = repo.AppPath
		if repo.Revision != nil {
			build.BuildParameters.Branch = repo.Revision.Branch
		}
	}

	return build, nil
}

// toBuildDetailsResponse converts a gen.ComponentWorkflowRunResponse to models.BuildDetailsResponse
func toBuildDetailsResponse(run *gen.ComponentWorkflowRunResponse) (*models.BuildDetailsResponse, error) {
	build, err := toWorkflowRunBuild(run)
	if err != nil {
		return nil, fmt.Errorf("failed to build response: %w", err)
	}

	status := utils.StrPointerAsStr(run.Status, "")

	// Extract inputInterface from workflow parameters
	_, _, _, inputInterface, _ := extractWorkflowParameters(run.Workflow)

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
func extractWorkflowParameters(workflow *gen.ComponentWorkflowConfigResponse) (string, string, string, *models.InputInterface, error) {
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
			Type: endpoint.Type,
			Port: endpoint.Port,
		}
		if endpoint.SchemaFilePath != "" {
			inputInterface.Schema = &models.InputInterfaceSchema{
				Path: endpoint.SchemaFilePath,
			}
		}
	}

	return language, languageVersion, runCommand, inputInterface, nil
}
