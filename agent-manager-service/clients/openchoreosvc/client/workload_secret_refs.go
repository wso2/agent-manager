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
)

// GetWorkloadSecretRefNames retrieves the names of every SecretReference an agent's
// configuration points at, so DeleteAgent can clean them up along with their KV entries.
//
// Two sources are unioned, because a secret-backed value can be in either:
//   - each ReleaseBinding's spec.workloadOverrides, where per-environment configuration now
//     lives. Every binding for the component is scanned, not just the first environment's: a
//     value set by a deploy reaches only the environment deployed to, so a secret can exist in
//     one binding and not another.
//   - the Workload's container spec, where configuration used to be written before it moved to
//     the bindings (see EnsureReleaseAndBinding for why). Agents created before that move still
//     carry their refs there, and skipping them would orphan the secret and the CR permanently.
//
// Env vars and file mounts both carry valueFrom.secretKeyRef, so both are scanned in each source.
func (c *openChoreoClient) GetWorkloadSecretRefNames(ctx context.Context, ouID, projectName, componentName string) ([]string, error) {
	namespaceName := c.NamespaceFor(ouID)

	// Collect unique secret reference names across both sources.
	secretRefNames := make(map[string]struct{})

	// Source 1: per-environment configuration on the release bindings.
	bindingsResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list release bindings: %w", err)
	}
	if bindingsResp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(bindingsResp.StatusCode(), ErrorResponses{
			JSON401: bindingsResp.JSON401,
			JSON403: bindingsResp.JSON403,
			JSON404: bindingsResp.JSON404,
			JSON500: bindingsResp.JSON500,
		})
	}
	if bindingsResp.JSON200 != nil {
		for _, binding := range bindingsResp.JSON200.Items {
			if binding.Spec == nil || binding.Spec.WorkloadOverrides == nil ||
				binding.Spec.WorkloadOverrides.Container == nil {
				continue
			}
			container := binding.Spec.WorkloadOverrides.Container
			collectSecretRefNames(container.Env, container.Files, secretRefNames)
		}
	}

	// Source 2: the workload itself, for agents predating the move to bindings.
	resp, err := c.ocClient.ListWorkloadsWithResponse(ctx, namespaceName, &gen.ListWorkloadsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
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

	if resp.JSON200 != nil {
		for _, workload := range resp.JSON200.Items {
			if workload.Spec == nil || workload.Spec.Container == nil {
				continue
			}
			collectSecretRefNames(workload.Spec.Container.Env, workload.Spec.Container.Files, secretRefNames)
		}
	}

	// Convert map to slice
	result := make([]string, 0, len(secretRefNames))
	for name := range secretRefNames {
		result = append(result, name)
	}

	return result, nil
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
