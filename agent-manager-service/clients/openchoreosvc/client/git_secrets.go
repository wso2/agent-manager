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
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// Default workflow plane configuration for git secrets
const (
	DefaultWorkflowPlaneKind = gen.CreateGitSecretRequestWorkflowPlaneKindClusterWorkflowPlane
	DefaultWorkflowPlaneName = "default"
)

// -----------------------------------------------------------------------------
// Git Secret Operations
// -----------------------------------------------------------------------------

// CreateGitSecret creates a new git secret via OpenChoreo
func (c *openChoreoClient) CreateGitSecret(ctx context.Context, namespaceName string, req CreateGitSecretRequest) (*GitSecretInfo, error) {
	// Build the request body
	body := gen.CreateGitSecretJSONRequestBody{
		SecretName:        req.Name,
		SecretType:        gen.CreateGitSecretRequestSecretType(req.SecretType),
		WorkflowPlaneKind: DefaultWorkflowPlaneKind,
		WorkflowPlaneName: DefaultWorkflowPlaneName,
	}

	// Set credentials for basic auth
	if req.Token != "" {
		body.Token = &req.Token
	}
	if req.Username != "" {
		body.Username = &req.Username
	}

	resp, err := c.ocClient.CreateGitSecretWithResponse(ctx, namespaceName, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create git secret: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated {
		return nil, handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON400: resp.JSON400,
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON409: resp.JSON409,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON201 == nil {
		return nil, fmt.Errorf("empty response from create git secret")
	}

	return convertGitSecretToInfo(resp.JSON201), nil
}

// ListGitSecrets lists all git secrets in a namespace
func (c *openChoreoClient) ListGitSecrets(ctx context.Context, namespaceName string) ([]*GitSecretInfo, error) {
	resp, err := c.ocClient.ListGitSecretsWithResponse(ctx, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to list git secrets: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON200 == nil || len(resp.JSON200.Items) == 0 {
		return []*GitSecretInfo{}, nil
	}

	secrets := make([]*GitSecretInfo, len(resp.JSON200.Items))
	for i := range resp.JSON200.Items {
		secrets[i] = convertGitSecretToInfo(&resp.JSON200.Items[i])
	}
	return secrets, nil
}

// DeleteGitSecret deletes a git secret by name
func (c *openChoreoClient) DeleteGitSecret(ctx context.Context, namespaceName, secretName string) error {
	resp, err := c.ocClient.DeleteGitSecretWithResponse(ctx, namespaceName, secretName)
	if err != nil {
		return fmt.Errorf("failed to delete git secret: %w", err)
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

// convertGitSecretToInfo converts a gen.GitSecretResponse to GitSecretInfo
func convertGitSecretToInfo(gs *gen.GitSecretResponse) *GitSecretInfo {
	if gs == nil {
		return nil
	}

	info := &GitSecretInfo{
		Name:      utils.StrPointerAsStr(gs.Name, ""),
		Namespace: utils.StrPointerAsStr(gs.Namespace, ""),
	}

	if gs.WorkflowPlaneKind != nil {
		info.WorkflowPlaneKind = *gs.WorkflowPlaneKind
	}
	if gs.WorkflowPlaneName != nil {
		info.WorkflowPlaneName = *gs.WorkflowPlaneName
	}

	return info
}
