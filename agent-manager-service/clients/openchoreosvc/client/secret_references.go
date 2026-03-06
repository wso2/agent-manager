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

// -----------------------------------------------------------------------------
// Secret Reference Operations
// -----------------------------------------------------------------------------

// CreateSecretReference creates a new SecretReference CR in the specified namespace
func (c *openChoreoClient) CreateSecretReference(ctx context.Context, namespaceName string, req CreateSecretReferenceRequest) (*SecretReferenceInfo, error) {
	// Build the data sources from the request
	dataSources := make([]gen.SecretDataSource, len(req.SecretKeys))
	for i, key := range req.SecretKeys {
		dataSources[i] = gen.SecretDataSource{
			SecretKey: key,
			RemoteRef: gen.RemoteReference{
				Key:      req.KVPath,
				Property: &key,
			},
		}
	}

	// Build labels for the SecretReference
	labels := map[string]string{
		string(LabelKeyProjectName):   req.ProjectName,
		string(LabelKeyComponentName): req.ComponentName,
	}

	// Build the SecretReference request body
	body := gen.CreateSecretReferenceJSONRequestBody{
		Metadata: gen.ObjectMeta{
			Name:   req.Name,
			Labels: &labels,
		},
		Spec: &gen.SecretReferenceSpec{
			Data: dataSources,
			Template: gen.SecretTemplate{
				Metadata: &struct {
					Annotations *map[string]string `json:"annotations,omitempty"`
					Labels      *map[string]string `json:"labels,omitempty"`
				}{
					Labels: &labels,
				},
			},
		},
	}

	// Set refresh interval if provided
	if req.RefreshInterval != "" {
		body.Spec.RefreshInterval = &req.RefreshInterval
	}

	resp, err := c.ocClient.CreateSecretReferenceWithResponse(ctx, namespaceName, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret reference: %w", err)
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
		return nil, fmt.Errorf("empty response from create secret reference")
	}

	return convertSecretReferenceToInfo(resp.JSON201), nil
}

// GetSecretReference retrieves a SecretReference by name
func (c *openChoreoClient) GetSecretReference(ctx context.Context, namespaceName, secretRefName string) (*SecretReferenceInfo, error) {
	resp, err := c.ocClient.GetSecretReferenceWithResponse(ctx, namespaceName, secretRefName)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret reference: %w", err)
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
		return nil, fmt.Errorf("empty response from get secret reference")
	}

	return convertSecretReferenceToInfo(resp.JSON200), nil
}

// ListSecretReferences lists all SecretReferences in a namespace
func (c *openChoreoClient) ListSecretReferences(ctx context.Context, namespaceName string, componentName string) ([]*SecretReferenceInfo, error) {
	resp, err := c.ocClient.ListSecretReferencesWithResponse(ctx, namespaceName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list secret references: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON200 == nil || len(resp.JSON200.Items) == 0 {
		return []*SecretReferenceInfo{}, nil
	}

	refs := make([]*SecretReferenceInfo, len(resp.JSON200.Items))
	for i := range resp.JSON200.Items {
		labels := resp.JSON200.Items[i].Metadata.Labels
		if labels == nil || (*labels)[string(LabelKeyComponentName)] != componentName {
			continue
		}
		refs[i] = convertSecretReferenceToInfo(&resp.JSON200.Items[i])
	}
	return refs, nil
}

// UpdateSecretReference updates an existing SecretReference
func (c *openChoreoClient) UpdateSecretReference(ctx context.Context, namespaceName, secretRefName string, req CreateSecretReferenceRequest) (*SecretReferenceInfo, error) {
	// Build the data sources from the request
	dataSources := make([]gen.SecretDataSource, len(req.SecretKeys))
	for i, key := range req.SecretKeys {
		dataSources[i] = gen.SecretDataSource{
			SecretKey: key,
			RemoteRef: gen.RemoteReference{
				Key:      req.KVPath,
				Property: &key,
			},
		}
	}

	// Build labels for the SecretReference
	labels := map[string]string{
		string(LabelKeyProjectName):   req.ProjectName,
		string(LabelKeyComponentName): req.ComponentName,
	}

	// Build the SecretReference request body
	body := gen.UpdateSecretReferenceJSONRequestBody{
		Metadata: gen.ObjectMeta{
			Name:   secretRefName,
			Labels: &labels,
		},
		Spec: &gen.SecretReferenceSpec{
			Data: dataSources,
			Template: gen.SecretTemplate{
				Metadata: &struct {
					Annotations *map[string]string `json:"annotations,omitempty"`
					Labels      *map[string]string `json:"labels,omitempty"`
				}{
					Labels: &labels,
				},
			},
		},
	}

	// Set refresh interval if provided
	if req.RefreshInterval != "" {
		body.Spec.RefreshInterval = &req.RefreshInterval
	}

	resp, err := c.ocClient.UpdateSecretReferenceWithResponse(ctx, namespaceName, secretRefName, body)
	if err != nil {
		return nil, fmt.Errorf("failed to update secret reference: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON400: resp.JSON400,
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from update secret reference")
	}

	return convertSecretReferenceToInfo(resp.JSON200), nil
}

// DeleteSecretReference deletes a SecretReference by name
func (c *openChoreoClient) DeleteSecretReference(ctx context.Context, namespaceName, secretRefName string) error {
	resp, err := c.ocClient.DeleteSecretReferenceWithResponse(ctx, namespaceName, secretRefName)
	if err != nil {
		return fmt.Errorf("failed to delete secret reference: %w", err)
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

// GetWorkloadSecretRefNames retrieves the names of all secret references used by a component's workload
func (c *openChoreoClient) GetWorkloadSecretRefNames(ctx context.Context, namespaceName, projectName, componentName string) ([]string, error) {
	// List workloads filtered by component
	resp, err := c.ocClient.ListWorkloadsWithResponse(ctx, namespaceName, &gen.ListWorkloadsParams{
		Component: &componentName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list workloads: %w", err)
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
		return []string{}, nil
	}

	// Collect unique secret reference names from env vars
	secretRefNames := make(map[string]struct{})
	for _, workload := range resp.JSON200.Items {
		if workload.Spec == nil || workload.Spec.Container == nil || workload.Spec.Container.Env == nil {
			continue
		}

		for _, env := range *workload.Spec.Container.Env {
			if env.ValueFrom != nil && env.ValueFrom.SecretRef != nil && env.ValueFrom.SecretRef.Name != nil {
				secretRefNames[*env.ValueFrom.SecretRef.Name] = struct{}{}
			}
		}
	}

	// Convert map to slice
	result := make([]string, 0, len(secretRefNames))
	for name := range secretRefNames {
		result = append(result, name)
	}

	return result, nil
}

// convertSecretReferenceToInfo converts a gen.SecretReference to SecretReferenceInfo
func convertSecretReferenceToInfo(sr *gen.SecretReference) *SecretReferenceInfo {
	if sr == nil {
		return nil
	}

	info := &SecretReferenceInfo{
		Name:      sr.Metadata.Name,
		Namespace: utils.StrPointerAsStr(sr.Metadata.Namespace, ""),
	}

	if sr.Spec != nil {
		info.Data = make([]SecretDataSourceInfo, len(sr.Spec.Data))
		for i, ds := range sr.Spec.Data {
			info.Data[i] = SecretDataSourceInfo{
				SecretKey: ds.SecretKey,
				RemoteRef: RemoteRefInfo{
					Key: ds.RemoteRef.Key,
				},
			}
			if ds.RemoteRef.Property != nil {
				info.Data[i].RemoteRef.Property = *ds.RemoteRef.Property
			}
		}
	}

	return info
}
