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
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// Default target plane for secrets when the caller does not specify one.
const (
	DefaultSecretTargetPlaneKind = string(gen.TargetPlaneRefKindClusterDataPlane)
	DefaultSecretTargetPlaneName = "default"
)

// CreateSecretRequest contains data for creating a secret via the OpenChoreo
// secret management API. The API stores the values in the plane's secret store
// and manages the underlying SecretReference internally.
type CreateSecretRequest struct {
	Name            string            // Name of the secret
	Data            map[string]string // Map of secret keys to plaintext values
	Labels          map[string]string // Labels applied to the underlying SecretReference
	TargetPlaneKind string            // Kind of the plane hosting the secret data (defaults to DefaultSecretTargetPlaneKind)
	TargetPlaneName string            // Name of the plane hosting the secret data (defaults to DefaultSecretTargetPlaneName)
}

// UpdateSecretRequest contains data for replacing a secret's data via the
// OpenChoreo secret management API. The data map is the final state; keys
// absent from it are pruned. The labels map replaces all user-set labels.
type UpdateSecretRequest struct {
	Data   map[string]string // Map of secret keys to plaintext values
	Labels map[string]string // Labels applied to the underlying SecretReference
}

// SecretInfo contains info about a secret managed via the OpenChoreo API.
type SecretInfo struct {
	Name      string            // Name of the secret
	Namespace string            // Namespace of the secret
	Data      map[string][]byte // Map of secret keys to values
	Labels    map[string]string // Labels on the underlying SecretReference
	CreatedAt string            // Creation timestamp (RFC3339), empty if unknown
}

// -----------------------------------------------------------------------------
// Secret Operations (OpenChoreo-managed secret storage)
// -----------------------------------------------------------------------------

// CreateSecret creates a new secret via the OpenChoreo secret management API
func (c *openChoreoClient) CreateSecret(ctx context.Context, ouID string, req CreateSecretRequest) (*SecretInfo, error) {
	namespaceName := c.NamespaceFor(ouID)

	targetPlaneKind := req.TargetPlaneKind
	if targetPlaneKind == "" {
		targetPlaneKind = DefaultSecretTargetPlaneKind
	}
	targetPlaneName := req.TargetPlaneName
	if targetPlaneName == "" {
		targetPlaneName = DefaultSecretTargetPlaneName
	}

	body := gen.CreateSecretJSONRequestBody{
		SecretName: req.Name,
		SecretType: gen.SecretTypeOpaque,
		Data:       req.Data,
		TargetPlane: gen.TargetPlaneRef{
			Kind: gen.TargetPlaneRefKind(targetPlaneKind),
			Name: targetPlaneName,
		},
	}
	if len(req.Labels) > 0 {
		labels := req.Labels
		body.Labels = &labels
	}

	resp, err := c.ocClient.CreateSecretWithResponse(ctx, namespaceName, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated {
		return nil, handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
			JSON400: resp.JSON400,
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON409: resp.JSON409,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON201 == nil {
		return nil, fmt.Errorf("empty response from create secret")
	}

	return convertSecretToInfo(resp.JSON201), nil
}

// GetSecret retrieves a secret by name via the OpenChoreo secret management API
func (c *openChoreoClient) GetSecret(ctx context.Context, ouID, secretName string) (*SecretInfo, error) {
	namespaceName := c.NamespaceFor(ouID)
	resp, err := c.ocClient.GetSecretWithResponse(ctx, namespaceName, secretName)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from get secret")
	}

	return convertSecretToInfo(resp.JSON200), nil
}

// UpdateSecret replaces a secret's data via the OpenChoreo secret management API
func (c *openChoreoClient) UpdateSecret(ctx context.Context, ouID, secretName string, req UpdateSecretRequest) (*SecretInfo, error) {
	namespaceName := c.NamespaceFor(ouID)

	body := gen.UpdateSecretJSONRequestBody{
		Data: req.Data,
	}
	if len(req.Labels) > 0 {
		labels := req.Labels
		body.Labels = &labels
	}

	resp, err := c.ocClient.UpdateSecretWithResponse(ctx, namespaceName, secretName, body)
	if err != nil {
		return nil, fmt.Errorf("failed to update secret: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
			JSON400: resp.JSON400,
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from update secret")
	}

	return convertSecretToInfo(resp.JSON200), nil
}

// DeleteSecret deletes a secret by name via the OpenChoreo secret management API
func (c *openChoreoClient) DeleteSecret(ctx context.Context, ouID, secretName string) error {
	namespaceName := c.NamespaceFor(ouID)
	resp, err := c.ocClient.DeleteSecretWithResponse(ctx, namespaceName, secretName)
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusNoContent {
		return handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}

	return nil
}

// convertSecretToInfo converts a gen.Secret to SecretInfo
func convertSecretToInfo(s *gen.Secret) *SecretInfo {
	if s == nil {
		return nil
	}

	info := &SecretInfo{
		Name:      s.Metadata.Name,
		Namespace: utils.StrPointerAsStr(s.Metadata.Namespace, ""),
	}

	if s.Data != nil {
		info.Data = *s.Data
	}
	if s.Metadata.Labels != nil {
		info.Labels = *s.Metadata.Labels
	}
	if s.Metadata.CreationTimestamp != nil {
		info.CreatedAt = s.Metadata.CreationTimestamp.Format(time.RFC3339)
	}

	return info
}
