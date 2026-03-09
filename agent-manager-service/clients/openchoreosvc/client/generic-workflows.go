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

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/gen"
)

// CreateWorkflowRunRequest contains parameters for creating a workflow run
type CreateWorkflowRunRequest struct {
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
	apiReq := gen.CreateWorkflowRunJSONRequestBody{
		WorkflowName: req.WorkflowName,
		Parameters:   req.Parameters,
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

// convertWorkflowRunToResponse converts gen.WorkflowRun to WorkflowRunResponse
func convertWorkflowRunToResponse(run *gen.WorkflowRun) *WorkflowRunResponse {
	if run == nil {
		return nil
	}

	resp := &WorkflowRunResponse{
		Name:         run.Name,
		WorkflowName: run.WorkflowName,
		Status:       string(run.Status),
		OrgName:      run.OrgName,
	}

	if run.Phase != nil {
		resp.Phase = *run.Phase
	}

	if run.Parameters != nil {
		resp.Parameters = *run.Parameters
	}

	return resp
}
