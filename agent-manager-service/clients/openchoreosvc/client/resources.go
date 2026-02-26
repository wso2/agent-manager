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

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
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

// CreateSecretReference creates a SecretReference CR to sync secrets from OpenBao to K8s.
// It adds a force-sync annotation with a timestamp to trigger immediate reconciliation
// by the external-secrets operator.
func (c *openChoreoClient) CreateSecretReference(ctx context.Context, req CreateSecretReferenceRequest) error {
	// Build data array with secretKey and remoteRef for each key
	data := make([]map[string]interface{}, 0, len(req.SecretKeys))
	for _, key := range req.SecretKeys {
		data = append(data, map[string]interface{}{
			"secretKey": key,
			"remoteRef": map[string]interface{}{
				"key":      req.KVPath,
				"property": key,
			},
		})
	}

	// Set default refresh interval if not provided
	refreshInterval := req.RefreshInterval
	if refreshInterval == "" {
		refreshInterval = "1h"
	}

	// Build the SecretReference CR with force-sync annotation for immediate reconciliation
	secretRefCR := map[string]interface{}{
		"apiVersion": "openchoreo.dev/v1alpha1",
		"kind":       "SecretReference",
		"metadata": map[string]interface{}{
			"name":      req.Name,
			"namespace": req.Namespace,
		},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"type": "Opaque",
			},
			"data":            data,
			"refreshInterval": refreshInterval,
		},
	}

	return c.ApplyResource(ctx, secretRefCR)
}

// timeNow is a variable to allow mocking in tests
var timeNow = time.Now

// DeleteSecretReference deletes a SecretReference CR
func (c *openChoreoClient) DeleteSecretReference(ctx context.Context, namespace, name string) error {
	secretRefCR := map[string]interface{}{
		"apiVersion": "openchoreo.dev/v1alpha1",
		"kind":       "SecretReference",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}

	return c.DeleteResource(ctx, secretRefCR)
}

// GetSecretReference retrieves a SecretReference CR by name and namespace
func (c *openChoreoClient) GetSecretReference(ctx context.Context, namespace, name string) (*SecretReferenceInfo, error) {
	resp, err := c.ocClient.ListSecretReferencesWithResponse(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to list secret references: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{})
	}

	if resp.JSON200 == nil || resp.JSON200.Data == nil || resp.JSON200.Data.Items == nil {
		return nil, fmt.Errorf("secret reference %s not found in namespace %s", name, namespace)
	}

	// Find the secret reference by name
	for _, sr := range *resp.JSON200.Data.Items {
		if sr.Name == name {
			info := &SecretReferenceInfo{
				Name:      sr.Name,
				Namespace: sr.Namespace,
			}
			if sr.Data != nil {
				for _, d := range *sr.Data {
					info.Data = append(info.Data, SecretDataSourceInfo{
						SecretKey: d.SecretKey,
						RemoteRef: RemoteRefInfo{
							Key:      d.RemoteRef.Key,
							Property: utils.StrPointerAsStr(d.RemoteRef.Property, ""),
						},
					})
				}
			}
			return info, nil
		}
	}

	return nil, fmt.Errorf("secret reference %s not found in namespace %s", name, namespace)
}

// GetWorkloadSecretRefNames extracts secret reference names from the main container's env vars
func (c *openChoreoClient) GetWorkloadSecretRefNames(ctx context.Context, namespaceName, projectName, componentName string) ([]string, error) {
	workloadResp, err := c.ocClient.GetWorkloadsWithResponse(ctx, namespaceName, projectName, componentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workload: %w", err)
	}

	if workloadResp.StatusCode() != http.StatusOK {
		if workloadResp.StatusCode() == http.StatusNotFound {
			// Workload not found - return empty list
			return nil, nil
		}
		return nil, handleErrorResponse(workloadResp.StatusCode(), workloadResp.Body, ErrorContext{
			NotFoundErr: utils.ErrAgentNotFound,
		})
	}

	if workloadResp.JSON200 == nil || workloadResp.JSON200.Data == nil || workloadResp.JSON200.Data.Containers == nil {
		return nil, nil
	}

	// Get the main container and extract secret ref names from env vars
	mainContainer, ok := (*workloadResp.JSON200.Data.Containers)[MainContainerName]
	if !ok || mainContainer.Env == nil {
		return nil, nil
	}

	// Use a map to deduplicate secret reference names
	secretRefNames := make(map[string]struct{})
	for _, env := range *mainContainer.Env {
		if env.ValueFrom != nil && env.ValueFrom.SecretRef != nil {
			secretRefNames[env.ValueFrom.SecretRef.Name] = struct{}{}
		}
	}

	// Convert map to slice
	result := make([]string, 0, len(secretRefNames))
	for name := range secretRefNames {
		result = append(result, name)
	}

	return result, nil
}
