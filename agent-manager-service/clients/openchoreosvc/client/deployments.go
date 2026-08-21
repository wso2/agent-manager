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
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// InternalAgentFromKindWorkloadRequest holds the parameters needed to create a Workload CR for a kind-sourced agent.
type InternalAgentFromKindWorkloadRequest struct {
	ImageID   string
	Endpoints []InputInterfaceEndpoint
	Env       []EnvVar
	Files     []FileVar
}

// InputInterfaceEndpoint describes a single exposed endpoint on a kind-sourced agent workload.
type InputInterfaceEndpoint struct {
	Name       string
	Port       int
	Type       string // e.g. "HTTP"
	BasePath   string
	Visibility []string // e.g. ["external"]
	Schema     *EndpointSchema
}

// EndpointSchema holds OpenAPI spec content for an endpoint.
type EndpointSchema struct {
	Content string
	Type    string // e.g. "OPENAPI"
}

// CreateInternalAgentFromKindWorkload creates a Workload CR directly for a kind-sourced agent,
// bypassing the workflow/build system entirely.
func (c *openChoreoClient) CreateInternalAgentFromKindWorkload(ctx context.Context, ouID, projectName, componentName string, req InternalAgentFromKindWorkloadRequest) error {
	namespaceName := c.NamespaceFor(ouID)
	workloadName := componentName + "-workload"

	// Build endpoint map
	endpointMap := make(map[string]gen.WorkloadEndpoint)
	for i, ep := range req.Endpoints {
		name := ep.Name
		if name == "" {
			name = fmt.Sprintf("%s-endpoint-%d", componentName, i)
		}

		epType := gen.WorkloadEndpointTypeHTTP
		if ep.Type != "" {
			epType = gen.WorkloadEndpointType(ep.Type)
		}

		workloadEp := gen.WorkloadEndpoint{
			Port: ep.Port,
			Type: epType,
		}

		if ep.BasePath != "" {
			workloadEp.BasePath = &ep.BasePath
		}

		if len(ep.Visibility) > 0 {
			vis := make([]gen.WorkloadEndpointVisibility, 0, len(ep.Visibility))
			for _, v := range ep.Visibility {
				vis = append(vis, gen.WorkloadEndpointVisibility(v))
			}
			workloadEp.Visibility = &vis
		}

		if ep.Schema != nil && ep.Schema.Content != "" {
			schemaType := ep.Schema.Type
			workloadEp.Schema = &struct {
				Content *string `json:"content,omitempty"`
				Type    *string `json:"type,omitempty"`
			}{
				Content: &ep.Schema.Content,
				Type:    &schemaType,
			}
		}

		endpointMap[name] = workloadEp
	}

	envVars := toGenEnvVars(req.Env)
	fileVars := toGenFileVars(req.Files)

	workload := gen.CreateWorkloadJSONRequestBody{
		Metadata: gen.ObjectMeta{
			Name:      workloadName,
			Namespace: &namespaceName,
		},
		Spec: &gen.WorkloadSpec{
			Container: &gen.WorkloadContainer{
				Image: req.ImageID,
				Env:   &envVars,
				Files: &fileVars,
			},
			Owner: &struct {
				ComponentName string `json:"componentName"`
				ProjectName   string `json:"projectName"`
			}{ComponentName: componentName, ProjectName: projectName},
			Endpoints: &endpointMap,
		},
	}

	resp, err := c.ocClient.CreateWorkloadWithResponse(ctx, namespaceName, workload)
	if err != nil {
		return fmt.Errorf("failed to create kind-sourced agent workload: %w", err)
	}
	if resp.StatusCode() != http.StatusCreated {
		return handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
			JSON400: resp.JSON400,
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON500: resp.JSON500,
		})
	}
	return nil
}

func (c *openChoreoClient) Deploy(ctx context.Context, ouID, projectName, componentName string, req DeployRequest) error {
	namespaceName := c.NamespaceFor(ouID)
	// List workloads to find the one for this component
	workloadResp, err := c.ocClient.ListWorkloadsWithResponse(ctx, namespaceName, &gen.ListWorkloadsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
	})
	if err != nil {
		return fmt.Errorf("failed to list workloads: %w", err)
	}

	if workloadResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(ctx, workloadResp.StatusCode(), ErrorResponses{
			JSON401: workloadResp.JSON401,
			JSON403: workloadResp.JSON403,
			JSON404: workloadResp.JSON404,
			JSON500: workloadResp.JSON500,
		})
	}

	if workloadResp.JSON200 == nil || len(workloadResp.JSON200.Items) == 0 {
		return fmt.Errorf("no workload found for component %q: %w", componentName, utils.ErrBuildNotComplete)
	}

	workload := workloadResp.JSON200.Items[0]
	workloadName := workload.Metadata.Name

	// Update the container image
	if workload.Spec == nil {
		workload.Spec = &gen.WorkloadSpec{}
	}
	if workload.Spec.Container == nil {
		workload.Spec.Container = &gen.WorkloadContainer{}
	}

	workload.Spec.Container.Image = req.ImageID

	// Update workload
	updateResp, err := c.ocClient.UpdateWorkloadWithResponse(ctx, namespaceName, workloadName, workload)
	if err != nil {
		return fmt.Errorf("failed to update workload: %w", err)
	}

	if updateResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(ctx, updateResp.StatusCode(), ErrorResponses{
			JSON401: updateResp.JSON401,
			JSON403: updateResp.JSON403,
			JSON404: updateResp.JSON404,
			JSON500: updateResp.JSON500,
		})
	}

	// No restartedAt stamp here. The pod rollout is triggered by the write that follows this one —
	// ReplaceReleaseBindingWorkloadOverrides bumps restartedAt in the same update that applies the
	// environment's env vars and file mounts, so the rollout and the config change land together.
	return nil
}

// retryReleaseBindingUpdate runs a Get → mutate → Update cycle on a named ReleaseBinding,
// retrying on resource-version conflicts caused by concurrent controller reconciliation.
//
// Both HTTP 409 and HTTP 500 trigger a retry. OpenChoreo currently wraps the
// k8s "object has been modified" conflict as a generic 500 rather than a 409,
// so a strict 409-only policy gives up too early on what is really a stale
// resourceVersion. Retrying 500 here is tactical — fix the real bug upstream in
// OpenChoreo (return 409 for conflicts) and tighten this back to 409-only.
func (c *openChoreoClient) retryReleaseBindingUpdate(
	ctx context.Context,
	namespaceName, bindingName string,
	mutate func(*gen.ReleaseBinding),
) error {
	const maxRetries = 3
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		getResp, err := c.ocClient.GetReleaseBindingWithResponse(ctx, namespaceName, bindingName)
		if err != nil {
			return fmt.Errorf("failed to get release binding %q: %w", bindingName, err)
		}
		if getResp.StatusCode() != http.StatusOK {
			return handleErrorResponse(ctx, getResp.StatusCode(), ErrorResponses{
				JSON401: getResp.JSON401,
				JSON403: getResp.JSON403,
				JSON404: getResp.JSON404,
				JSON500: getResp.JSON500,
			})
		}
		if getResp.JSON200 == nil || getResp.JSON200.Spec == nil {
			return fmt.Errorf("empty response from get release binding %q", bindingName)
		}

		binding := getResp.JSON200
		mutate(binding)

		updateResp, err := c.ocClient.UpdateReleaseBindingWithResponse(ctx, namespaceName, bindingName, *binding)
		if err != nil {
			return fmt.Errorf("failed to update release binding %s: %w", bindingName, err)
		}
		if updateResp.StatusCode() == http.StatusOK {
			return nil
		}
		// 409 Conflict = stale resourceVersion; re-fetch and try again until we hit maxRetries.
		// 500 is included because OpenChoreo currently surfaces conflict errors as a
		// generic Internal Server Error rather than a 409. See the function-level comment.
		if (updateResp.StatusCode() == http.StatusConflict ||
			updateResp.StatusCode() == http.StatusInternalServerError) && attempt < maxRetries {
			logger.GetLogger(ctx).Warn("release binding update failed, retrying with fresh version",
				"binding", bindingName, "status", updateResp.StatusCode(), "attempt", attempt, "max_retries", maxRetries)
			lastErr = fmt.Errorf("status %d on attempt %d", updateResp.StatusCode(), attempt)
			continue
		}
		return handleErrorResponse(ctx, updateResp.StatusCode(), ErrorResponses{
			JSON401: updateResp.JSON401,
			JSON403: updateResp.JSON403,
			JSON404: updateResp.JSON404,
			JSON500: updateResp.JSON500,
		})
	}
	return fmt.Errorf("failed to update release binding %s after %d retries: %w", bindingName, maxRetries, lastErr)
}

// findReleaseBindingForEnv lists release bindings for the named component and returns
// the binding whose Spec.Environment matches env, or (nil, nil) when no such binding exists.
// Returns a wrapped error for RPC failures or non-200 list responses.
//
// Use this helper instead of inlining the same List → loop → match-by-env pattern. Note that
// "no binding" is signalled by a nil return value, not utils.ErrNotFound, because most callers
// want to distinguish "binding does not exist yet" from "the list call failed."
func (c *openChoreoClient) findReleaseBindingForEnv(ctx context.Context, namespaceName, componentName, env string) (*gen.ReleaseBinding, error) {
	listResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list release bindings: %w", err)
	}
	if listResp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(ctx, listResp.StatusCode(), ErrorResponses{
			JSON401: listResp.JSON401,
			JSON403: listResp.JSON403,
			JSON404: listResp.JSON404,
			JSON500: listResp.JSON500,
		})
	}
	if listResp.JSON200 == nil {
		return nil, fmt.Errorf("invalid OpenChoreo response: missing release binding list payload for component %q", componentName)
	}
	for i, b := range listResp.JSON200.Items {
		if b.Spec != nil && b.Spec.Environment == env {
			return &listResp.JSON200.Items[i], nil
		}
	}
	return nil, nil //nolint:nilnil // documented sentinel: callers distinguish "no binding" from "list failed"
}

// bumpRestartedAt stamps a fresh restartedAt on the binding's ComponentTypeEnvironmentConfigs,
// which OpenChoreo watches to trigger a pod rollout. Shared by every mutation that needs pods to
// pick up changed secrets/config, so the mechanism lives in exactly one place.
func bumpRestartedAt(rb *gen.ReleaseBinding) {
	if rb.Spec.ComponentTypeEnvironmentConfigs == nil {
		overrides := make(map[string]interface{})
		rb.Spec.ComponentTypeEnvironmentConfigs = &overrides
	}
	(*rb.Spec.ComponentTypeEnvironmentConfigs)["restartedAt"] = time.Now().Format(time.RFC3339Nano)
}

// UpdateReleaseBindingTraitConfigs updates traitEnvironmentConfigs AND sets restartedAt on a
// release binding in a single Get→mutate→Update cycle. Both changes go together because the
// only reason to update trait configs is so the gateway/pod picks them up, which requires a
// pod rollout. Splitting them produced races (two separate updates contending on the same
// resourceVersion) without giving callers any control they'd actually use.
// Returns ErrNotFound when no binding exists yet for (component, environment).
// mergeAgentAPIKeySecretRef carries the env-injection trait's agentApiKeySecretRef/Property
// forward from existing into incoming when incoming doesn't already set its own value, so a
// wholesale trait-config replacement can't silently drop it. Never mutates existing or incoming
// (or their nested entries): retryReleaseBindingUpdate re-invokes its callback with a freshly
// refetched existing on conflict, and a prior attempt's merge must not leak into that retry.
func mergeAgentAPIKeySecretRef(existing *map[string]interface{}, incoming map[string]interface{}, componentName string) map[string]interface{} {
	if existing == nil {
		return incoming
	}
	envInjKey := componentName + "-" + string(TraitEnvInjection)
	existingCfg, ok := (*existing)[envInjKey].(map[string]interface{})
	if !ok {
		return incoming
	}
	existingRef, ok := existingCfg["agentApiKeySecretRef"].(string)
	if !ok || existingRef == "" {
		return incoming
	}
	incomingCfg, _ := incoming[envInjKey].(map[string]interface{})
	if ref, ok := incomingCfg["agentApiKeySecretRef"].(string); ok && ref != "" {
		return incoming
	}
	merged := make(map[string]interface{}, len(incoming)+1)
	for k, v := range incoming {
		merged[k] = v
	}
	mergedCfg := make(map[string]interface{}, len(incomingCfg)+2)
	for k, v := range incomingCfg {
		mergedCfg[k] = v
	}
	mergedCfg["agentApiKeySecretRef"] = existingRef
	if _, ok := mergedCfg["agentApiKeySecretProperty"]; !ok {
		if existingProperty, ok := existingCfg["agentApiKeySecretProperty"].(string); ok && existingProperty != "" {
			mergedCfg["agentApiKeySecretProperty"] = existingProperty
		}
	}
	merged[envInjKey] = mergedCfg
	return merged
}

func (c *openChoreoClient) UpdateReleaseBindingTraitConfigs(ctx context.Context, ouID, componentName, environment string, traitConfigs map[string]interface{}, componentTypeConfigs map[string]interface{}) error {
	namespaceName := c.NamespaceFor(ouID)
	binding, err := c.findReleaseBindingForEnv(ctx, namespaceName, componentName, environment)
	if err != nil {
		return err
	}
	if binding == nil {
		return fmt.Errorf("no release binding found for component %q in environment %q: %w", componentName, environment, utils.ErrNotFound)
	}

	return c.retryReleaseBindingUpdate(ctx, namespaceName, binding.Metadata.Name, func(rb *gen.ReleaseBinding) {
		merged := mergeAgentAPIKeySecretRef(rb.Spec.TraitEnvironmentConfigs, traitConfigs, componentName)
		rb.Spec.TraitEnvironmentConfigs = &merged
		bumpRestartedAt(rb)
		// Merge component-type configs (e.g. runtimeClassName from the env's isolation tier).
		for k, v := range componentTypeConfigs {
			(*rb.Spec.ComponentTypeEnvironmentConfigs)[k] = v
		}
		// runtimeClassName is derived wholly from the target environment's isolation tier, so
		// the incoming configs are authoritative: when they omit it (the env reverted to the
		// default runc tier) the stale value must be cleared, not left behind.
		if _, ok := componentTypeConfigs["runtimeClassName"]; !ok {
			delete(*rb.Spec.ComponentTypeEnvironmentConfigs, "runtimeClassName")
		}
	})
}

// EnsureReleaseBindingRuntimeClass idempotently reconciles runtimeClassName on a release
// binding's ComponentTypeEnvironmentConfigs for (component, environment).
//
// Why this exists: OpenChoreo's AutoDeploy creates the release binding when a build completes,
// WITHOUT going through the backend's deploy-time config write — so an agent in an isolation-tier
// environment first comes up on the default (runc) runtime. This is called from the deploy-status
// read path to correct that out-of-band binding.
//
// It is strictly idempotent: it writes (and bumps restartedAt once, to roll the warm pods onto the
// isolation node) ONLY when the binding's current runtimeClassName differs from desired. Once
// correct, every subsequent call is a no-op — no write, no restart loop. Returns nil (no-op) when
// desired is empty or the binding does not exist yet (build/auto-deploy not finished).
func (c *openChoreoClient) EnsureReleaseBindingRuntimeClass(ctx context.Context, ouID, componentName, environment, desiredRuntimeClass string) error {
	namespaceName := c.NamespaceFor(ouID)
	binding, err := c.findReleaseBindingForEnv(ctx, namespaceName, componentName, environment)
	if err != nil {
		return err
	}
	if binding == nil {
		return nil // binding not created yet — nothing to reconcile
	}

	return c.retryReleaseBindingUpdate(ctx, namespaceName, binding.Metadata.Name, func(rb *gen.ReleaseBinding) {
		// Re-read desired-vs-current from the freshly fetched binding INSIDE the mutation
		// closure (not from the stale outer read) so concurrent callers don't each bump
		// restartedAt off the same pre-image and trigger redundant pod rolls.
		if rb.Spec == nil {
			return
		}
		current := ""
		if rb.Spec.ComponentTypeEnvironmentConfigs != nil {
			if v, ok := (*rb.Spec.ComponentTypeEnvironmentConfigs)["runtimeClassName"].(string); ok {
				current = v
			}
		}
		if current == desiredRuntimeClass {
			return // already correct — leave the binding untouched (no restartedAt bump)
		}
		if rb.Spec.ComponentTypeEnvironmentConfigs == nil {
			overrides := make(map[string]interface{})
			rb.Spec.ComponentTypeEnvironmentConfigs = &overrides
		}
		if desiredRuntimeClass == "" {
			// Reverting to the default (runc) tier: remove the key entirely so the
			// SandboxTemplate renders without runtimeClassName, rather than leaving a
			// stale isolation-tier value behind.
			delete(*rb.Spec.ComponentTypeEnvironmentConfigs, "runtimeClassName")
		} else {
			(*rb.Spec.ComponentTypeEnvironmentConfigs)["runtimeClassName"] = desiredRuntimeClass
		}
		// Bump restartedAt so the SandboxTemplate re-renders and the warm pods roll onto the
		// isolation node. Only runs on correction (current != desired), so there is no restart loop.
		(*rb.Spec.ComponentTypeEnvironmentConfigs)["restartedAt"] = time.Now().Format(time.RFC3339Nano)
	})
}

// ReplaceReleaseBindingWorkloadOverrides replaces the container env vars and file mounts on the
// release binding for the given (component, environment), and sets restartedAt to trigger a
// pod rollout — all in a single Get→mutate→Update cycle.
// Passing nil for envOverrides or fileOverrides leaves that aspect untouched; passing an empty
// slice clears it. Returns ErrNotFound when no binding exists yet.
func (c *openChoreoClient) ReplaceReleaseBindingWorkloadOverrides(ctx context.Context, ouID, componentName, environment string, envOverrides []EnvVar, fileOverrides []FileVar) error {
	namespaceName := c.NamespaceFor(ouID)
	binding, err := c.findReleaseBindingForEnv(ctx, namespaceName, componentName, environment)
	if err != nil {
		return err
	}
	if binding == nil {
		return fmt.Errorf("no release binding found for component %q in environment %q: %w", componentName, environment, utils.ErrNotFound)
	}

	return c.retryReleaseBindingUpdate(ctx, namespaceName, binding.Metadata.Name, func(rb *gen.ReleaseBinding) {
		container := &gen.ContainerOverride{}
		if envOverrides != nil {
			envVars := toGenEnvVars(envOverrides)
			container.Env = &envVars
		} else if rb.Spec.WorkloadOverrides != nil && rb.Spec.WorkloadOverrides.Container != nil {
			container.Env = rb.Spec.WorkloadOverrides.Container.Env
		}
		if fileOverrides != nil {
			fileVars := toGenFileVars(fileOverrides)
			container.Files = &fileVars
		} else if rb.Spec.WorkloadOverrides != nil && rb.Spec.WorkloadOverrides.Container != nil {
			container.Files = rb.Spec.WorkloadOverrides.Container.Files
		}
		rb.Spec.WorkloadOverrides = &gen.WorkloadOverrides{Container: container}

		bumpRestartedAt(rb)
	})
}

// PromoteComponent promotes a component from sourceEnvironment to targetEnvironment.
// It finds the release name deployed in the source environment, then creates or updates
// a release binding in the target environment using the naming convention {componentName}-{targetEnv}.
func (c *openChoreoClient) PromoteComponent(ctx context.Context, ouID, projectName, componentName, sourceEnvironment, targetEnvironment string, envOverrides []EnvVar, fileOverrides []FileVar, traitEnvConfigs map[string]interface{}, componentTypeConfigs map[string]interface{}) error {
	namespaceName := c.NamespaceFor(ouID)
	// Step 1: List release bindings for the component to find the source release name
	bindingsResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
	})
	if err != nil {
		return fmt.Errorf("failed to list release bindings: %w", err)
	}
	if bindingsResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(ctx, bindingsResp.StatusCode(), ErrorResponses{
			JSON401: bindingsResp.JSON401,
			JSON403: bindingsResp.JSON403,
			JSON500: bindingsResp.JSON500,
		})
	}

	// Step 2: Find the release name deployed in the source environment
	var sourceReleaseName string
	if bindingsResp.JSON200 != nil {
		for _, b := range bindingsResp.JSON200.Items {
			if b.Spec == nil {
				continue
			}
			if b.Spec.Environment == sourceEnvironment {
				if b.Spec.ReleaseName == nil || *b.Spec.ReleaseName == "" {
					return fmt.Errorf("no release found in source environment %s: %w", sourceEnvironment, utils.ErrNotFound)
				}
				sourceReleaseName = *b.Spec.ReleaseName
				break
			}
		}
	}

	if sourceReleaseName == "" {
		return fmt.Errorf("no release binding found for component %s in source environment %s: %w", componentName, sourceEnvironment, utils.ErrNotFound)
	}

	// Step 3: Check if a release binding already exists in the target environment
	// Release binding names follow the convention: {componentName}-{targetEnvironment}
	targetBindingName := fmt.Sprintf("%s-%s", componentName, targetEnvironment)
	getResp, err := c.ocClient.GetReleaseBindingWithResponse(ctx, namespaceName, targetBindingName)
	if err != nil {
		return fmt.Errorf("failed to check target release binding: %w", err)
	}

	// Build workload overrides if env/file overrides are provided.
	// Nil means "the caller supplied nothing"; an empty slice means "this environment has none"
	// and must still be written, so the target does not silently inherit the component-wide base.
	var workloadOverrides *gen.WorkloadOverrides
	if envOverrides != nil || fileOverrides != nil {
		container := &gen.ContainerOverride{}
		if envOverrides != nil {
			envVars := toGenEnvVars(envOverrides)
			container.Env = &envVars
		}
		if fileOverrides != nil {
			fileVars := toGenFileVars(fileOverrides)
			container.Files = &fileVars
		}
		workloadOverrides = &gen.WorkloadOverrides{Container: container}
	}

	// Build trait environment configs if provided
	var traitConfigs *map[string]interface{}
	if len(traitEnvConfigs) > 0 {
		traitConfigs = &traitEnvConfigs
	}

	// Build component type environment configs (e.g. runtimeClassName from the env's isolation tier).
	var ctConfigs *map[string]interface{}
	if len(componentTypeConfigs) > 0 {
		ctConfigs = &componentTypeConfigs
	}

	// Step 4: Create or update the release binding in the target environment
	if getResp.StatusCode() == http.StatusOK && getResp.JSON200 != nil && getResp.JSON200.Spec != nil {
		activeState := gen.ReleaseBindingSpecStateActive
		incomingTraitConfigs := map[string]interface{}{}
		if traitConfigs != nil {
			incomingTraitConfigs = *traitConfigs
		}
		if err := c.retryReleaseBindingUpdate(ctx, namespaceName, targetBindingName, func(binding *gen.ReleaseBinding) {
			binding.Spec.ReleaseName = &sourceReleaseName
			binding.Spec.State = &activeState
			// Always replace overrides on re-promotion so a clean source environment
			// clears any stale target-specific env vars, file mounts, or trait configs.
			binding.Spec.WorkloadOverrides = workloadOverrides
			merged := mergeAgentAPIKeySecretRef(binding.Spec.TraitEnvironmentConfigs, incomingTraitConfigs, componentName)
			if len(merged) > 0 {
				binding.Spec.TraitEnvironmentConfigs = &merged
			} else {
				binding.Spec.TraitEnvironmentConfigs = traitConfigs
			}
			if ctConfigs != nil {
				binding.Spec.ComponentTypeEnvironmentConfigs = ctConfigs
			}
			// Force a pod rollout, for the same reason Deploy does: a re-promotion
			// whose source release and resolved overrides are unchanged writes back a
			// byte-identical spec, which Kubernetes treats as a no-op — no reconcile,
			// no rollout — while the API and the audit record both report a successful
			// promote. That silence also hides the fresh agent API key minted on every
			// promote: it is written to a fixed secret location, so the reference in
			// the spec never changes and the pod keeps serving the old key.
			// Must come after the ctConfigs assignment above, which replaces the map
			// holding restartedAt wholesale and would otherwise drop the stamp.
			bumpRestartedAt(binding)
		}); err != nil {
			return err
		}
	} else {
		// Create new release binding in target environment
		activeState := gen.ReleaseBindingSpecStateActive
		createBody := gen.CreateReleaseBindingJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:      targetBindingName,
				Namespace: &namespaceName,
			},
			Spec: &gen.ReleaseBindingSpec{
				Environment:                     targetEnvironment,
				ReleaseName:                     &sourceReleaseName,
				State:                           &activeState,
				WorkloadOverrides:               workloadOverrides,
				TraitEnvironmentConfigs:         traitConfigs,
				ComponentTypeEnvironmentConfigs: ctConfigs,
				Owner: struct {
					ComponentName string `json:"componentName"`
					ProjectName   string `json:"projectName"`
				}{
					ComponentName: componentName,
					ProjectName:   projectName,
				},
			},
		}

		createResp, err := c.ocClient.CreateReleaseBindingWithResponse(ctx, namespaceName, createBody)
		if err != nil {
			return fmt.Errorf("failed to create release binding in target environment: %w", err)
		}
		if createResp.StatusCode() != http.StatusCreated {
			return handleErrorResponse(ctx, createResp.StatusCode(), ErrorResponses{
				JSON400: createResp.JSON400,
				JSON401: createResp.JSON401,
				JSON403: createResp.JSON403,
				JSON500: createResp.JSON500,
			})
		}
	}

	return nil
}

// GetSourceEnvWorkloadOverrides returns the effective env vars and file mounts for the source
// environment by merging the Workload CR (base) with the source release binding's WorkloadOverrides
// (per-env overrides). When the same key exists in both, the binding override takes precedence.
func (c *openChoreoClient) GetSourceEnvWorkloadOverrides(ctx context.Context, ouID, componentName, sourceEnvironment string) ([]EnvVar, []FileVar, error) {
	namespaceName := c.NamespaceFor(ouID)
	// Build maps to hold the merged result; overrides win on key conflict.
	envMap := make(map[string]EnvVar)
	fileMap := make(map[string]FileVar)

	// Step 1: Seed with base env vars from the Workload CR (apply to all environments).
	workloadResp, err := c.ocClient.ListWorkloadsWithResponse(ctx, namespaceName, &gen.ListWorkloadsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list workloads: %w", err)
	}
	if workloadResp.StatusCode() != http.StatusOK {
		return nil, nil, handleErrorResponse(ctx, workloadResp.StatusCode(), ErrorResponses{
			JSON401: workloadResp.JSON401,
			JSON403: workloadResp.JSON403,
			JSON500: workloadResp.JSON500,
		})
	}
	if workloadResp.JSON200 != nil && len(workloadResp.JSON200.Items) > 0 {
		wl := workloadResp.JSON200.Items[0]
		if wl.Spec != nil && wl.Spec.Container != nil {
			if wl.Spec.Container.Env != nil {
				for _, e := range *wl.Spec.Container.Env {
					envMap[e.Key] = genEnvVarToClient(e)
				}
			}
			if wl.Spec.Container.Files != nil {
				for _, f := range *wl.Spec.Container.Files {
					fileMap[f.Key] = genFileVarToClient(f)
				}
			}
		}
	}

	// Step 2: Apply per-env overrides from the source release binding (override wins).
	bindingsResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list release bindings: %w", err)
	}
	if bindingsResp.StatusCode() != http.StatusOK {
		return nil, nil, handleErrorResponse(ctx, bindingsResp.StatusCode(), ErrorResponses{
			JSON401: bindingsResp.JSON401,
			JSON403: bindingsResp.JSON403,
			JSON500: bindingsResp.JSON500,
		})
	}
	if bindingsResp.JSON200 != nil {
		for _, b := range bindingsResp.JSON200.Items {
			if b.Spec == nil || b.Spec.Environment != sourceEnvironment {
				continue
			}
			if b.Spec.WorkloadOverrides != nil && b.Spec.WorkloadOverrides.Container != nil {
				container := b.Spec.WorkloadOverrides.Container
				if container.Env != nil {
					for _, e := range *container.Env {
						envMap[e.Key] = genEnvVarToClient(e)
					}
				}
				if container.Files != nil {
					for _, f := range *container.Files {
						fileMap[f.Key] = genFileVarToClient(f)
					}
				}
			}
			break
		}
	}

	// Convert maps to slices.
	envVars := make([]EnvVar, 0, len(envMap))
	for _, ev := range envMap {
		envVars = append(envVars, ev)
	}
	fileVars := make([]FileVar, 0, len(fileMap))
	for _, fv := range fileMap {
		fileVars = append(fileVars, fv)
	}
	return envVars, fileVars, nil
}

// genEnvVarToClient converts a gen.EnvVar to the client EnvVar type.
func genEnvVarToClient(e gen.EnvVar) EnvVar {
	ev := EnvVar{Key: e.Key}
	if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil &&
		e.ValueFrom.SecretKeyRef.Name != nil && e.ValueFrom.SecretKeyRef.Key != nil {
		ev.ValueFrom = &EnvVarValueFrom{
			SecretKeyRef: &SecretKeyRef{
				Name: *e.ValueFrom.SecretKeyRef.Name,
				Key:  *e.ValueFrom.SecretKeyRef.Key,
			},
		}
	} else if e.Value != nil {
		ev.Value = *e.Value
	}
	return ev
}

// genFileVarToClient converts a gen.FileVar to the client FileVar type.
func genFileVarToClient(f gen.FileVar) FileVar {
	fv := FileVar{Key: f.Key, MountPath: f.MountPath}
	if f.ValueFrom != nil && f.ValueFrom.SecretKeyRef != nil &&
		f.ValueFrom.SecretKeyRef.Name != nil && f.ValueFrom.SecretKeyRef.Key != nil {
		fv.ValueFrom = &EnvVarValueFrom{
			SecretKeyRef: &SecretKeyRef{
				Name: *f.ValueFrom.SecretKeyRef.Name,
				Key:  *f.ValueFrom.SecretKeyRef.Key,
			},
		}
	} else if f.Value != nil {
		fv.Value = *f.Value
	}
	return fv
}

// toGenEnvVars converts client EnvVar slice to gen EnvVar slice
func toGenEnvVars(envVars []EnvVar) []gen.EnvVar {
	result := make([]gen.EnvVar, len(envVars))
	for i, env := range envVars {
		genEnv := gen.EnvVar{Key: env.Key}
		if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
			secretName := env.ValueFrom.SecretKeyRef.Name
			secretKey := env.ValueFrom.SecretKeyRef.Key
			genEnv.ValueFrom = &gen.EnvVarValueFrom{
				SecretKeyRef: &struct {
					Key  *string `json:"key,omitempty"`
					Name *string `json:"name,omitempty"`
				}{Name: &secretName, Key: &secretKey},
			}
		} else {
			v := env.Value
			genEnv.Value = &v
		}
		result[i] = genEnv
	}
	return result
}

// toGenFileVars converts client FileVar slice to gen FileVar slice
func toGenFileVars(fileVars []FileVar) []gen.FileVar {
	result := make([]gen.FileVar, len(fileVars))
	for i, f := range fileVars {
		genFile := gen.FileVar{Key: f.Key, MountPath: f.MountPath}
		if f.ValueFrom != nil && f.ValueFrom.SecretKeyRef != nil {
			secretName := f.ValueFrom.SecretKeyRef.Name
			secretKey := f.ValueFrom.SecretKeyRef.Key
			genFile.ValueFrom = &gen.EnvVarValueFrom{
				SecretKeyRef: &struct {
					Key  *string `json:"key,omitempty"`
					Name *string `json:"name,omitempty"`
				}{Name: &secretName, Key: &secretKey},
			}
		} else {
			v := f.Value
			genFile.Value = &v
		}
		result[i] = genFile
	}
	return result
}

func (c *openChoreoClient) GetDeployments(ctx context.Context, ouID, pipelineName, projectName, componentName string) ([]*models.DeploymentResponse, error) {
	namespaceName := c.NamespaceFor(ouID)
	// Get the deployment pipeline for environment ordering
	pipeline, err := c.GetProjectDeploymentPipeline(ctx, namespaceName, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment pipeline: %w", err)
	}

	// Get all environments for display names
	environments, err := c.ListEnvironments(ctx, namespaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	// Create environment order based on the deployment pipeline
	environmentOrder := buildEnvironmentOrder(pipeline.PromotionPaths)

	// Get release bindings for the component
	bindingsResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list release bindings: %w", err)
	}

	if bindingsResp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(ctx, bindingsResp.StatusCode(), ErrorResponses{
			JSON401: bindingsResp.JSON401,
			JSON403: bindingsResp.JSON403,
			JSON404: bindingsResp.JSON404,
			JSON500: bindingsResp.JSON500,
		})
	}

	// Create a map of release bindings by environment for quick lookup
	releaseBindingMap := make(map[string]*gen.ReleaseBinding)
	if bindingsResp.JSON200 != nil {
		for i := range bindingsResp.JSON200.Items {
			binding := &bindingsResp.JSON200.Items[i]
			if binding.Spec != nil {
				releaseBindingMap[binding.Spec.Environment] = binding
			}
		}
	}

	// Create environment map for quick lookup
	environmentMap := make(map[string]*models.EnvironmentResponse)
	for _, env := range environments {
		environmentMap[env.Name] = env
	}

	// Fetch workload to get endpoint visibility and schema info
	workloadEndpoints := make(map[string]*gen.WorkloadEndpoint)
	var liveWorkloadContainerImage string
	workloadResp, err := c.ocClient.ListWorkloadsWithResponse(ctx, namespaceName, &gen.ListWorkloadsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
	})
	if err == nil && workloadResp.StatusCode() == http.StatusOK && workloadResp.JSON200 != nil && len(workloadResp.JSON200.Items) > 0 {
		workload := workloadResp.JSON200.Items[0]
		if workload.Spec != nil && workload.Spec.Container != nil && workload.Spec.Container.Image != "" {
			liveWorkloadContainerImage = workload.Spec.Container.Image
		}
		if workload.Spec != nil && workload.Spec.Endpoints != nil {
			for name, ep := range *workload.Spec.Endpoints {
				epCopy := ep
				workloadEndpoints[name] = &epCopy
			}
		}
	}

	// List all ComponentReleases for the component and create a map by release name
	componentReleaseMap := make(map[string]*gen.ComponentRelease)
	releasesResp, err := c.ocClient.ListComponentReleasesWithResponse(ctx, namespaceName, &gen.ListComponentReleasesParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
	})
	if err == nil && releasesResp.StatusCode() == http.StatusOK && releasesResp.JSON200 != nil {
		for i := range releasesResp.JSON200.Items {
			release := &releasesResp.JSON200.Items[i]
			componentReleaseMap[release.Metadata.Name] = release
		}
	}

	// Construct deployment details in the order defined by the pipeline
	var deploymentDetails []*models.DeploymentResponse
	for _, envName := range environmentOrder {
		// Find promotion target environment for this environment
		promotionTargetEnv := findPromotionTargetEnvironment(envName, pipeline.PromotionPaths, environmentMap)

		if releaseBinding, exists := releaseBindingMap[envName]; exists {
			// Look up the ComponentRelease from the map using the release name from the binding
			var componentRelease *gen.ComponentRelease
			if releaseBinding.Spec.ReleaseName != nil && *releaseBinding.Spec.ReleaseName != "" {
				componentRelease = componentReleaseMap[*releaseBinding.Spec.ReleaseName]
			}

			// Only bindings that would otherwise report "active" need the extra
			// resource-tree lookup; every other status is already accurate without it.
			var runtime runtimeReplicaState
			if determineDeploymentStatus(releaseBinding, runtimeReplicaState{}) == DeploymentStatusActive {
				runtime = c.fetchRuntimeReplicaState(ctx, namespaceName, releaseBinding.Metadata.Name)
			}

			deploymentDetail, err := toDeploymentDetailsResponse(releaseBinding, componentRelease, environmentMap, promotionTargetEnv, workloadEndpoints, liveWorkloadContainerImage, runtime)
			if err != nil {
				return nil, fmt.Errorf("failed to build deployment details for environment %s: %w", envName, err)
			}
			deploymentDetails = append(deploymentDetails, deploymentDetail)
		} else {
			var displayName string
			if env, envExists := environmentMap[envName]; envExists {
				displayName = env.DisplayName
			}

			deploymentDetails = append(deploymentDetails, &models.DeploymentResponse{
				Environment:                envName,
				EnvironmentDisplayName:     displayName,
				PromotionTargetEnvironment: promotionTargetEnv,
				Status:                     DeploymentStatusNotDeployed,
				Endpoints:                  []models.Endpoint{},
			})
		}
	}

	// For kind-sourced agents (no release bindings — they use the workload model directly),
	// synthesize a deployment entry from the live workload.
	if len(releaseBindingMap) == 0 && liveWorkloadContainerImage != "" {
		if len(deploymentDetails) > 0 {
			deploymentDetails[0].Status = DeploymentStatusActive
			deploymentDetails[0].ImageId = liveWorkloadContainerImage
		} else {
			deploymentDetails = []*models.DeploymentResponse{{
				Status:    DeploymentStatusActive,
				ImageId:   liveWorkloadContainerImage,
				Endpoints: []models.Endpoint{},
			}}
		}
	}

	return deploymentDetails, nil
}

// FindFirstEnvironment returns the name of the first (source/dev) environment
// from the deployment pipeline promotion paths, or "" if none.
func FindFirstEnvironment(promotionPaths []models.PromotionPath) string {
	order := buildEnvironmentOrder(promotionPaths)
	if len(order) == 0 {
		return ""
	}
	return order[0]
}

// buildEnvironmentOrder creates an ordered list of environments based on promotion paths
func buildEnvironmentOrder(promotionPaths []models.PromotionPath) []string {
	if len(promotionPaths) == 0 {
		return []string{}
	}

	var order []string
	visited := make(map[string]bool)

	// Start with source environments
	for _, path := range promotionPaths {
		if !visited[path.SourceEnvironmentRef] {
			order = append(order, path.SourceEnvironmentRef)
			visited[path.SourceEnvironmentRef] = true
		}

		// Add target environments
		for _, target := range path.TargetEnvironmentRefs {
			if !visited[target.Name] {
				order = append(order, target.Name)
				visited[target.Name] = true
			}
		}
	}

	return order
}

// IsDeploymentInProgress checks whether the release binding for the given component and environment
// has a deployment currently in progress (ResourcesReady condition with ResourcesProgressing reason).
func (c *openChoreoClient) IsDeploymentInProgress(ctx context.Context, ouID, componentName, environment string) (bool, error) {
	namespaceName := c.NamespaceFor(ouID)
	resp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
	})
	if err != nil {
		return false, fmt.Errorf("failed to list release bindings: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return false, handleErrorResponse(ctx, resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON500: resp.JSON500,
		})
	}

	if resp.JSON200 == nil {
		return false, nil
	}

	// Find the release binding for the target environment
	for i := range resp.JSON200.Items {
		binding := &resp.JSON200.Items[i]
		if binding.Spec != nil && binding.Spec.Environment == environment {
			// Deliberately binding-only: this guards against concurrent *rollouts*, and
			// callers abort the deploy when it reports true. Counting a booting pod as
			// in-progress here would block redeploys for the whole startup window — and
			// forever for an agent whose pods never become ready.
			status := determineDeploymentStatus(binding, runtimeReplicaState{})
			return status == DeploymentStatusInProgress, nil
		}
	}

	return false, nil
}

// runtimeReplicaState is the agent's live pod readiness, read from the SandboxWarmPool
// in the release binding's Kubernetes resource tree.
//
// A ReleaseBinding reports Ready=True (and ResourcesReady=True, "All N resources ready")
// as soon as the dataplane resources are applied — it never observes whether a pod passed
// its readiness probe. An agent whose container is still booting therefore shows as Ready
// on the binding while its Service has zero ready endpoints, which is what made the console
// report "active" while invocations still failed with 503. The warm pool's replica counts
// are the only readiness signal available, so they are consulted separately.
//
// found is false when the state could not be determined (no warm pool node in the tree, or
// the tree fetch failed); callers then fall back to the binding conditions alone rather than
// reporting a worse status than they can prove.
type runtimeReplicaState struct {
	found   bool
	desired int32
	ready   int32

	// notReadyResource describes a resource the deployment depends on that reports itself not
	// ready — in practice an ExternalSecret that will not sync, which leaves the container
	// in CreateContainerConfigError ("secret ... not found") indefinitely. It separates
	// "cannot start" from "still starting" so a broken deployment is not reported as
	// in-progress forever. Empty when nothing reported itself unready.
	notReadyResource string
}

// isBooting reports whether the agent is expected to be serving but no replica is ready
// yet, and nothing is known to be preventing it from starting.
func (r runtimeReplicaState) isBooting() bool {
	return r.found && r.desired > 0 && r.ready == 0 && r.notReadyResource == ""
}

// isFailed reports whether the agent cannot serve and a dependency explains why.
// Requires ready == 0: a pool still serving on a previously synced Secret is working,
// whatever that Secret's ExternalSecret currently reports.
func (r runtimeReplicaState) isFailed() bool {
	return r.found && r.desired > 0 && r.ready == 0 && r.notReadyResource != ""
}

// runtimeReplicaStateFromTree extracts warm pool replica counts and any blocking
// dependency failure from a resource tree.
//
// Node objects are inspected rather than the nodes' health field, which cannot be used
// for either signal: OpenChoreo reports the SandboxWarmPool as Healthy regardless of how
// many replicas are ready, and reports an ExternalSecret as Healthy while its own status
// says Ready=False/SecretSyncedError. Pods are not in the tree at all (and the OpenChoreo
// API exposes no pod endpoint), so container-level failures such as ImagePullBackOff or
// CrashLoopBackOff cannot be detected here.
func runtimeReplicaStateFromTree(tree *gen.K8sResourceTreeResponse) runtimeReplicaState {
	if tree == nil {
		return runtimeReplicaState{}
	}

	state := runtimeReplicaState{}
	for _, rendered := range tree.RenderedReleases {
		for i := range rendered.Nodes {
			node := &rendered.Nodes[i]

			if node.Kind == resourceKindSandboxWarmPool {
				state.found = true
				// A pool with no status yet has nothing ready; the zero counts already say so.
				if status, ok := node.Object["status"].(map[string]interface{}); ok {
					state.desired = replicaCount(status, "replicas")
					state.ready = replicaCount(status, "readyReplicas")
				}
			}

			// Any resource that reports itself not ready is a candidate explanation. No kind
			// is singled out: ExternalSecret happens to be the only kind in the tree today
			// that publishes a Ready condition (Backend and RestApi use Accepted/Programmed,
			// the rest publish none), so naming it would add a rule without changing what is
			// matched, and would need editing whenever another kind starts reporting Ready.
			if state.notReadyResource == "" {
				state.notReadyResource = notReadyCondition(node)
			}
		}
	}

	return state
}

// notReadyCondition returns a description of a node's Ready=False condition, or "" when
// the node is ready or reports no Ready condition.
func notReadyCondition(node *gen.ResourceNode) string {
	status, ok := node.Object["status"].(map[string]interface{})
	if !ok {
		return ""
	}
	conditions, ok := status["conditions"].([]interface{})
	if !ok {
		return ""
	}

	for _, raw := range conditions {
		condition, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if condition["type"] != "Ready" || condition["status"] != "False" {
			continue
		}

		reason, _ := condition["reason"].(string)
		message, _ := condition["message"].(string)
		return strings.TrimSpace(fmt.Sprintf("%s %s: %s %s", node.Kind, node.Name, reason, message))
	}

	return ""
}

// replicaCount reads an integral replica field out of an unstructured status map. JSON
// numbers decode as float64, so the concrete numeric types are handled explicitly.
func replicaCount(status map[string]interface{}, key string) int32 {
	switch v := status[key].(type) {
	case float64:
		return int32(v)
	case int64:
		return int32(v)
	case int:
		return int32(v)
	default:
		return 0
	}
}

// fetchRuntimeReplicaState looks up the live warm pool replica counts for a release binding.
// It is best effort: any failure yields an unknown state so deployment listings degrade to
// binding-only status instead of erroring.
func (c *openChoreoClient) fetchRuntimeReplicaState(ctx context.Context, namespaceName, bindingName string) runtimeReplicaState {
	if bindingName == "" {
		return runtimeReplicaState{}
	}

	resp, err := c.ocClient.GetReleaseBindingK8sResourceTreeWithResponse(ctx, namespaceName, bindingName)
	if err != nil {
		logger.GetLogger(ctx).Warn("failed to fetch resource tree for deployment readiness",
			"binding", bindingName, "namespace", namespaceName, "error", err)
		return runtimeReplicaState{}
	}
	if resp.StatusCode() != http.StatusOK {
		logger.GetLogger(ctx).Warn("resource tree request returned non-OK for deployment readiness",
			"binding", bindingName, "namespace", namespaceName, "status", resp.StatusCode())
		return runtimeReplicaState{}
	}

	state := runtimeReplicaStateFromTree(resp.JSON200)
	logger.GetLogger(ctx).Debug("resolved agent runtime state",
		"binding", bindingName, "namespace", namespaceName,
		"desired", state.desired, "ready", state.ready, "cause", state.notReadyResource)

	return state
}

// determineDeploymentStatus determines deployment status from release binding conditions,
// downgrading "active" to "in-progress" while the agent's pods are still starting.
// Pass an unknown runtimeReplicaState to derive the status from the binding alone.
func determineDeploymentStatus(binding *gen.ReleaseBinding, runtime runtimeReplicaState) string {
	if binding == nil {
		return DeploymentStatusNotDeployed
	}

	// Check if the binding state is set to Undeploy (suspended)
	if binding.Spec != nil && binding.Spec.State != nil && *binding.Spec.State == gen.ReleaseBindingSpecStateUndeploy {
		return DeploymentStatusSuspended
	}

	if binding.Status == nil || binding.Status.Conditions == nil {
		return DeploymentStatusNotDeployed
	}

	// Check conditions for status
	for _, condition := range *binding.Status.Conditions {
		// Look for "Ready" condition
		if condition.Type == "Ready" {
			switch condition.Status {
			case "True":
				// The binding is applied, but the agent cannot serve traffic until a pod
				// passes its readiness probe. Reporting "active" here is what let callers
				// invoke an agent whose container was still booting and get a 503.
				if runtime.isFailed() {
					return DeploymentStatusFailed
				}
				if runtime.isBooting() {
					return DeploymentStatusInProgress
				}
				return DeploymentStatusActive
			case "False":
				// Check reason for more specific status
				switch condition.Reason {
				case "Progressing", "Pending", "ResourcesProgressing":
					return DeploymentStatusInProgress
				case "Failed", "Error":
					return DeploymentStatusFailed
				}
				return DeploymentStatusFailed
			}
		}
	}

	return DeploymentStatusInProgress
}

func findPromotionTargetEnvironment(sourceEnvName string, promotionPaths []models.PromotionPath, environmentMap map[string]*models.EnvironmentResponse) *models.PromotionTargetEnvironment {
	for _, path := range promotionPaths {
		if path.SourceEnvironmentRef != sourceEnvName {
			continue
		}

		// Since promotion is linear, take the first (and only) target
		if len(path.TargetEnvironmentRefs) == 0 {
			return nil
		}

		targetEnvName := path.TargetEnvironmentRefs[0].Name
		var targetDisplayName string
		if env, exists := environmentMap[targetEnvName]; exists {
			targetDisplayName = env.DisplayName
		}
		return &models.PromotionTargetEnvironment{
			Name:        targetEnvName,
			DisplayName: targetDisplayName,
		}
	}
	return nil
}

func toDeploymentDetailsResponse(binding *gen.ReleaseBinding, componentRelease *gen.ComponentRelease, environmentMap map[string]*models.EnvironmentResponse, promotionTargetEnv *models.PromotionTargetEnvironment, workloadEndpoints map[string]*gen.WorkloadEndpoint, liveWorkloadContainerImage string, runtime runtimeReplicaState) (*models.DeploymentResponse, error) {
	if binding == nil || binding.Spec == nil {
		return nil, fmt.Errorf("release binding is nil or has no spec")
	}

	status := determineDeploymentStatus(binding, runtime)

	// Extract endpoints from release binding status, enriched with workload endpoint info
	endpoints := extractEndpointsFromBinding(binding, workloadEndpoints)

	deployedImage := findDeployedImageFromComponentRelease(componentRelease)
	if deployedImage == "" && liveWorkloadContainerImage != "" {
		deployedImage = liveWorkloadContainerImage
	}

	environment := binding.Spec.Environment
	var environmentDisplayName string
	if env, exists := environmentMap[environment]; exists {
		environmentDisplayName = env.DisplayName
	}

	t := getLastDeployedTime(binding)
	var lastDeployedAt *time.Time
	if !t.IsZero() {
		lastDeployedAt = &t
	}

	return &models.DeploymentResponse{
		ImageId:                    deployedImage,
		Status:                     status,
		Environment:                environment,
		EnvironmentDisplayName:     environmentDisplayName,
		PromotionTargetEnvironment: promotionTargetEnv,
		LastDeployedAt:             lastDeployedAt,
		Endpoints:                  endpoints,
	}, nil
}

// getLastDeployedTime extracts the most accurate last deployed time from a ReleaseBinding.
// Sandbox agents stay Ready=True across redeploys — only LastSpecUpdateTime changes on each
// deploy — so we take the max of both to return the true last-deployed time.
func getLastDeployedTime(binding *gen.ReleaseBinding) time.Time {
	var readyTime, specUpdateTime time.Time

	if binding.Status != nil {
		if binding.Status.Conditions != nil {
			for _, c := range *binding.Status.Conditions {
				if c.Type == "Ready" {
					readyTime = c.LastTransitionTime
				}
			}
		}
		if binding.Status.LastSpecUpdateTime != nil {
			specUpdateTime = *binding.Status.LastSpecUpdateTime
		}
	}

	t := readyTime
	if specUpdateTime.After(t) {
		t = specUpdateTime
	}
	if !t.IsZero() {
		return t
	}
	if binding.Metadata.CreationTimestamp != nil {
		return *binding.Metadata.CreationTimestamp
	}

	return time.Time{}
}

// extractEndpointsFromBinding extracts endpoint URLs from the release binding status
// and enriches them with visibility and schema info from workload endpoints
func extractEndpointsFromBinding(binding *gen.ReleaseBinding, workloadEndpoints map[string]*gen.WorkloadEndpoint) []models.Endpoint {
	if binding == nil || binding.Status == nil || binding.Status.Endpoints == nil {
		return []models.Endpoint{}
	}

	endpoints := make([]models.Endpoint, 0, len(*binding.Status.Endpoints))
	for _, ep := range *binding.Status.Endpoints {
		var urlStr string
		// Use ExternalURLs based on IsLocalDevEnv config
		if ep.ExternalURLs != nil {
			var endpointURL *gen.EndpointURL
			if config.GetConfig().TLSConfig.EnableTLS {
				endpointURL = ep.ExternalURLs.Https
			} else {
				endpointURL = ep.ExternalURLs.Http
			}
			if endpointURL != nil {
				urlStr = buildEndpointURLString(endpointURL)
			}
		}

		endpoint := models.Endpoint{
			Name: ep.Name,
			URL:  urlStr,
		}

		// Enrich with visibility from workload endpoint
		if workloadEp, exists := workloadEndpoints[ep.Name]; exists {
			if workloadEp.Visibility != nil && len(*workloadEp.Visibility) > 0 {
				endpoint.Visibility = string((*workloadEp.Visibility)[0])
			}
		}

		endpoints = append(endpoints, endpoint)
	}
	return endpoints
}

// UpdateDeploymentState updates the state of a deployment (Active or Undeploy)
func (c *openChoreoClient) UpdateDeploymentState(ctx context.Context, ouID, projectName, componentName, environment string, state gen.ReleaseBindingSpecState) error {
	namespaceName := c.NamespaceFor(ouID)
	// List release bindings for the component
	bindingsResp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
	})
	if err != nil {
		return fmt.Errorf("failed to list release bindings: %w", err)
	}

	if bindingsResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(ctx, bindingsResp.StatusCode(), ErrorResponses{
			JSON401: bindingsResp.JSON401,
			JSON403: bindingsResp.JSON403,
			JSON404: bindingsResp.JSON404,
			JSON500: bindingsResp.JSON500,
		})
	}

	// Find the binding for the specified environment
	var targetBinding *gen.ReleaseBinding
	if bindingsResp.JSON200 != nil {
		for i := range bindingsResp.JSON200.Items {
			binding := &bindingsResp.JSON200.Items[i]
			if binding.Spec != nil && binding.Spec.Environment == environment {
				targetBinding = binding
				break
			}
		}
	}

	if targetBinding == nil {
		return fmt.Errorf("no release binding found for environment %s: %w", environment, utils.ErrNotFound)
	}

	// Update the state
	targetBinding.Spec.State = &state

	// Update the release binding
	bindingName := targetBinding.Metadata.Name
	updateResp, err := c.ocClient.UpdateReleaseBindingWithResponse(ctx, namespaceName, bindingName, *targetBinding)
	if err != nil {
		return fmt.Errorf("failed to update release binding: %w", err)
	}

	if updateResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(ctx, updateResp.StatusCode(), ErrorResponses{
			JSON401: updateResp.JSON401,
			JSON403: updateResp.JSON403,
			JSON404: updateResp.JSON404,
			JSON500: updateResp.JSON500,
		})
	}

	return nil
}

// extractImageFromWorkloadMap reads container image from OpenChoreo workload JSON shapes.
func extractImageFromWorkloadMap(workload map[string]interface{}) string {
	if len(workload) == 0 {
		return ""
	}
	if img, ok, err := unstructured.NestedString(workload, "spec", "container", "image"); err == nil && ok && img != "" {
		return img
	}
	if img, ok, err := unstructured.NestedString(workload, "container", "image"); err == nil && ok && img != "" {
		return img
	}
	containers, found, err := unstructured.NestedSlice(workload, "spec", "containers")
	if err == nil && found {
		for _, c := range containers {
			cm, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			if img, ok := cm["image"].(string); ok && img != "" {
				return img
			}
		}
	}
	return ""
}

// findDeployedImageFromComponentRelease extracts the deployed image from the ComponentRelease workload spec
// The image is located at spec.container.image (or equivalent) within the frozen workload object.
func findDeployedImageFromComponentRelease(release *gen.ComponentRelease) string {
	if release == nil || release.Spec == nil {
		return ""
	}

	workload := release.Spec.Workload
	if len(workload) == 0 {
		return ""
	}

	return extractImageFromWorkloadMap(workload)
}
