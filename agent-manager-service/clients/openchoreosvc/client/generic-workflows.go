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
	"fmt"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// CreateWorkflowRunRequest contains parameters for creating a workflow run
type CreateWorkflowRunRequest struct {
	Name         string // Name for the WorkflowRun resource (required)
	WorkflowName string
	Parameters   map[string]interface{}
}

// WorkflowRunResponse represents a workflow run response
type WorkflowRunResponse struct {
	Name         string
	WorkflowName string
	Status       string
	Phase        string
	OrgName      string
	Parameters   map[string]interface{}
}

// CreateWorkflowRun creates a new workflow run via OpenChoreo
func (c *openChoreoClient) CreateWorkflowRun(ctx context.Context, namespaceName string, req CreateWorkflowRunRequest) (*WorkflowRunResponse, error) {
	workflowKind := gen.WorkflowRunConfigKindWorkflow
	apiReq := gen.CreateWorkflowRunJSONRequestBody{
		Metadata: gen.ObjectMeta{
			Name:      req.Name,
			Namespace: &namespaceName,
		},
		Spec: &gen.WorkflowRunSpec{
			Workflow: gen.WorkflowRunConfig{
				Kind:       &workflowKind,
				Name:       req.WorkflowName,
				Parameters: &req.Parameters,
			},
		},
	}

	resp, err := c.ocClient.CreateWorkflowRunWithResponse(ctx, namespaceName, apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create workflow run: %w", err)
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
		return nil, fmt.Errorf("empty response from create workflow run")
	}

	return convertWorkflowRunToResponse(resp.JSON201), nil
}

// GetWorkflowRun retrieves a workflow run by namespace and run name from OpenChoreo
func (c *openChoreoClient) GetWorkflowRun(ctx context.Context, namespaceName, runName string) (*WorkflowRunResponse, error) {
	resp, err := c.ocClient.GetWorkflowRunWithResponse(ctx, namespaceName, runName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow run: %w", err)
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
		return nil, fmt.Errorf("empty response from get workflow run")
	}

	return convertWorkflowRunToResponse(resp.JSON200), nil
}

// ExpireWorkflowRun sets a short TTL on a workflow run to trigger cleanup
func (c *openChoreoClient) ExpireWorkflowRun(ctx context.Context, namespaceName, runName string) error {
	// Get the current WorkflowRun
	getResp, err := c.ocClient.GetWorkflowRunWithResponse(ctx, namespaceName, runName)
	if err != nil {
		return fmt.Errorf("failed to get workflow run: %w", err)
	}

	if getResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(getResp.StatusCode(), ErrorResponses{
			JSON401: getResp.JSON401,
			JSON403: getResp.JSON403,
			JSON404: getResp.JSON404,
			JSON500: getResp.JSON500,
		})
	}

	if getResp.JSON200 == nil || getResp.JSON200.Spec == nil {
		return fmt.Errorf("empty response from get workflow run")
	}
	// Set a short TTL to trigger cleanup
	ttl := "5s"
	getResp.JSON200.Spec.TtlAfterCompletion = &ttl

	// Update the WorkflowRun
	updateResp, err := c.ocClient.UpdateWorkflowRunWithResponse(ctx, namespaceName, runName, *getResp.JSON200)
	if err != nil {
		return fmt.Errorf("failed to update workflow run: %w", err)
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

// convertWorkflowRunToResponse converts gen.WorkflowRun to WorkflowRunResponse
func convertWorkflowRunToResponse(run *gen.WorkflowRun) *WorkflowRunResponse {
	if run == nil {
		return nil
	}

	resp := &WorkflowRunResponse{
		Name:    run.Metadata.Name,
		OrgName: utils.StrPointerAsStr(run.Metadata.Namespace, ""),
	}

	if run.Spec != nil {
		resp.WorkflowName = run.Spec.Workflow.Name
		if run.Spec.Workflow.Parameters != nil {
			resp.Parameters = *run.Spec.Workflow.Parameters
		}
	}

	// Extract status from conditions
	if run.Status != nil && run.Status.Conditions != nil {
		for _, cond := range *run.Status.Conditions {
			switch cond.Type {
			case WorkflowConditionSucceeded:
				if cond.Status == "True" {
					resp.Status = "Succeeded"
					return resp // Succeeded is terminal, return immediately
				}
			case WorkflowConditionFailed:
				if cond.Status == "True" {
					resp.Status = "Failed"
					return resp // Failed is terminal, return immediately
				}
			case WorkflowConditionCompleted:
				// WorkflowCompleted indicates completion - check reason for success/failure
				if cond.Status == "True" && resp.Status == "" {
					switch cond.Reason {
					case WorkflowReasonSucceeded:
						resp.Status = "Succeeded"
					case WorkflowConditionFailed:
						resp.Status = "Failed"
					}
				}
			case WorkflowConditionRunning:
				if cond.Status == "True" && resp.Status == "" {
					resp.Status = "Running"
				}
			}
		}
	}

	return resp
}
