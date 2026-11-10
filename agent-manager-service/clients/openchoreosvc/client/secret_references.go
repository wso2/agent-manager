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
	"errors"
	"fmt"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// -----------------------------------------------------------------------------
// Secret Reference Operations
// -----------------------------------------------------------------------------

// CreateSecretReference creates a new SecretReference CR in the specified namespace
func (c *openChoreoClient) CreateSecretReference(ctx context.Context, ouID string, req CreateSecretReferenceRequest) (*SecretReferenceInfo, error) {
	namespaceName := c.NamespaceFor(ouID)
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
	if len(req.TemplateAnnotations) > 0 {
		annotations := req.TemplateAnnotations
		body.Spec.Template.Metadata.Annotations = &annotations
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
		return nil, terminatingConflict(handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON400: resp.JSON400,
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON409: resp.JSON409,
			JSON500: resp.JSON500,
		}), "secret reference", req.Name, func() (*gen.ObjectMeta, error) {
			getResp, getErr := c.ocClient.GetSecretReferenceWithResponse(ctx, namespaceName, req.Name)
			if getErr != nil || getResp.JSON200 == nil {
				return nil, getErr
			}
			return &getResp.JSON200.Metadata, nil
		})
	}

	if resp.JSON201 == nil {
		return nil, fmt.Errorf("empty response from create secret reference")
	}

	return convertSecretReferenceToInfo(resp.JSON201), nil
}

// GetSecretReference retrieves a SecretReference by name
func (c *openChoreoClient) GetSecretReference(ctx context.Context, ouID, secretRefName string) (*SecretReferenceInfo, error) {
	namespaceName := c.NamespaceFor(ouID)
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

// ListSecretReferences lists all SecretReferences in a namespace.
// If componentName is empty, returns all secret references without filtering.
// If componentName is provided, filters by that component label.
func (c *openChoreoClient) ListSecretReferences(ctx context.Context, ouID string, componentName string) ([]*SecretReferenceInfo, error) {
	namespaceName := c.NamespaceFor(ouID)
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

	refs := make([]*SecretReferenceInfo, 0)
	for i := range resp.JSON200.Items {
		if isTerminating(resp.JSON200.Items[i].Metadata) {
			// Deleted but not yet finalized — see isTerminating.
			continue
		}
		// If componentName filter is provided, only include matching refs
		if componentName != "" {
			labels := resp.JSON200.Items[i].Metadata.Labels
			if labels == nil || (*labels)[string(LabelKeyComponentName)] != componentName {
				continue
			}
		}
		refs = append(refs, convertSecretReferenceToInfo(&resp.JSON200.Items[i]))
	}
	return refs, nil
}

// UpdateSecretReference updates an existing SecretReference
func (c *openChoreoClient) UpdateSecretReference(ctx context.Context, ouID, secretRefName string, req CreateSecretReferenceRequest) (*SecretReferenceInfo, error) {
	namespaceName := c.NamespaceFor(ouID)
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
	if len(req.TemplateAnnotations) > 0 {
		annotations := req.TemplateAnnotations
		body.Spec.Template.Metadata.Annotations = &annotations
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
func (c *openChoreoClient) DeleteSecretReference(ctx context.Context, ouID, secretRefName string) error {
	namespaceName := c.NamespaceFor(ouID)
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

// GetWorkloadSecretRefNames retrieves the names of every SecretReference an agent's
// configuration points at, so DeleteAgent can clean them up along with their KV entries.
//
// Three sources are unioned, because a secret-backed value can be in any of them:
//
//   - each ReleaseBinding's spec.workloadOverrides, where per-environment configuration now
//     lives. Every binding for the component is scanned, not just the first environment's: a
//     value set by a deploy reaches only the environment deployed to, so a secret can exist in
//     one binding and not another.
//   - the Workload's container spec, where configuration used to be written before it moved to
//     the bindings (see EnsureReleaseAndBinding for why). Agents created before that move still
//     carry their refs there, and skipping them would orphan the secret and the CR permanently.
//   - the Component's build workflow parameters, where creation seeds the agent's configuration
//     and its agent API key. An agent that never built or deployed has no binding and no
//     Workload, so this is the only place its secret refs are recorded. A Component that is
//     already gone (a retried delete) contributes nothing rather than failing the lookup.
//
// Env vars and file mounts both carry valueFrom.secretKeyRef, so both are scanned in each source.
//
// Each source is read even when another fails, so one failed read does not lose the refs the
// others hold: the refs that could be collected are returned together with the failures,
// joined into one error.
func (c *openChoreoClient) GetWorkloadSecretRefNames(ctx context.Context, ouID, projectName, componentName string) ([]string, error) {
	namespaceName := c.NamespaceFor(ouID)

	// Collect unique secret reference names across all sources.
	secretRefNames := make(map[string]struct{})
	err := errors.Join(
		c.collectComponentWorkflowSecretRefNames(ctx, namespaceName, componentName, secretRefNames),
		c.collectReleaseBindingSecretRefNames(ctx, namespaceName, componentName, secretRefNames),
		c.collectWorkloadSecretRefNames(ctx, namespaceName, componentName, secretRefNames),
	)

	result := make([]string, 0, len(secretRefNames))
	for name := range secretRefNames {
		result = append(result, name)
	}
	return result, err
}

// collectReleaseBindingSecretRefNames adds the SecretReference names in every ReleaseBinding's
// workloadOverrides, where per-environment configuration lives.
func (c *openChoreoClient) collectReleaseBindingSecretRefNames(ctx context.Context, namespaceName, componentName string, out map[string]struct{}) error {
	resp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
	})
	if err != nil {
		return fmt.Errorf("failed to list release bindings: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}
	if resp.JSON200 == nil {
		return nil
	}
	for _, binding := range resp.JSON200.Items {
		if binding.Spec == nil || binding.Spec.WorkloadOverrides == nil ||
			binding.Spec.WorkloadOverrides.Container == nil {
			continue
		}
		container := binding.Spec.WorkloadOverrides.Container
		collectSecretRefNames(container.Env, container.Files, out)
	}
	return nil
}

// collectWorkloadSecretRefNames adds the SecretReference names in the Workload's container
// spec, for agents created before configuration moved to the bindings.
func (c *openChoreoClient) collectWorkloadSecretRefNames(ctx context.Context, namespaceName, componentName string, out map[string]struct{}) error {
	resp, err := c.ocClient.ListWorkloadsWithResponse(ctx, namespaceName, &gen.ListWorkloadsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
	})
	if err != nil {
		return fmt.Errorf("failed to list workloads: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}
	if resp.JSON200 == nil {
		return nil
	}
	for _, workload := range resp.JSON200.Items {
		if workload.Spec == nil || workload.Spec.Container == nil {
			continue
		}
		collectSecretRefNames(workload.Spec.Container.Env, workload.Spec.Container.Files, out)
	}
	return nil
}

// collectComponentWorkflowSecretRefNames adds the SecretReference names behind the
// valueFrom.secretKeyRef entries in the Component's build workflow env vars and file mounts.
func (c *openChoreoClient) collectComponentWorkflowSecretRefNames(ctx context.Context, namespaceName, componentName string, out map[string]struct{}) error {
	resp, err := c.ocClient.GetComponentWithResponse(ctx, namespaceName, componentName)
	if err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode() != http.StatusOK {
		return handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}
	if resp.JSON200 == nil || resp.JSON200.Spec == nil || resp.JSON200.Spec.Workflow == nil ||
		resp.JSON200.Spec.Workflow.Parameters == nil {
		return nil
	}
	params := *resp.JSON200.Spec.Workflow.Parameters
	for _, param := range []string{workflowParamEnvironmentVariables, workflowParamFileMounts} {
		raw, ok := params[param]
		if !ok || raw == nil {
			continue
		}
		data, err := json.Marshal(raw)
		if err != nil {
			return fmt.Errorf("encode Component workflow parameter %q: %w", param, err)
		}
		var entries []struct {
			ValueFrom *gen.EnvVarValueFrom `json:"valueFrom"`
		}
		if err := json.Unmarshal(data, &entries); err != nil {
			return fmt.Errorf("decode Component workflow parameter %q: %w", param, err)
		}
		for _, entry := range entries {
			addSecretRefName(entry.ValueFrom, out)
		}
	}
	return nil
}

// collectSecretRefNames adds the SecretReference names behind any valueFrom.secretKeyRef in the
// given env vars and file mounts to out. Both slices are optional; nil entries are skipped.
func collectSecretRefNames(env *[]gen.EnvVar, files *[]gen.FileVar, out map[string]struct{}) {
	if env != nil {
		for _, e := range *env {
			addSecretRefName(e.ValueFrom, out)
		}
	}
	if files != nil {
		for _, f := range *files {
			addSecretRefName(f.ValueFrom, out)
		}
	}
}

func addSecretRefName(valueFrom *gen.EnvVarValueFrom, out map[string]struct{}) {
	if valueFrom == nil || valueFrom.SecretKeyRef == nil || valueFrom.SecretKeyRef.Name == nil {
		return
	}
	out[*valueFrom.SecretKeyRef.Name] = struct{}{}
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
