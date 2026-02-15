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

// ApplyResource creates or updates a generic resource via OpenChoreo
func (c *openChoreoClient) ApplyResource(ctx context.Context, body map[string]interface{}) error {
	resp, err := c.ocClient.ApplyResourceWithResponse(ctx, gen.ApplyResourceJSONRequestBody(body))
	if err != nil {
		return fmt.Errorf("failed to apply resource: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated && resp.StatusCode() != http.StatusOK {
		return handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{})
	}

	return nil
}

// GetResource retrieves a resource by namespace, kind, and name from OpenChoreo.
// The returned map includes the full resource including .status.
func (c *openChoreoClient) GetResource(ctx context.Context, namespaceName, kind, name string) (map[string]interface{}, error) {
	resp, err := c.ocClient.GetResourceWithResponse(ctx, namespaceName, kind, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{})
	}

	if resp.JSON200 == nil || resp.JSON200.Data == nil {
		return nil, fmt.Errorf("empty response from get resource")
	}

	return *resp.JSON200.Data, nil
}

// DeleteResource deletes a generic resource via OpenChoreo
func (c *openChoreoClient) DeleteResource(ctx context.Context, body map[string]interface{}) error {
	resp, err := c.ocClient.DeleteResourceWithResponse(ctx, gen.DeleteResourceJSONRequestBody(body))
	if err != nil {
		return fmt.Errorf("failed to delete resource: %w", err)
	}

	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusNoContent {
		return handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{})
	}

	return nil
}
