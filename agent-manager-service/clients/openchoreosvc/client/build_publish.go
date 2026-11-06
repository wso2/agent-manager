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
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// workflowParamAMPBuild is the build workflow parameter that carries the
// organization's build client. Declared by the AMP build workflows as
// parameters.ampBuild {secretRef}; the workflow resolves that SecretReference
// (in the run's own namespace) into a per-run ExternalSecret and hands the
// client ID and secret to its workload step.
const workflowParamAMPBuild = "ampBuild"

// BuildClientParams names an organization's build client for one build run. It
// is not a secret: SecretRefName names the SecretReference (client-id,
// client-secret) in the organization's own namespace. A name, rather than a
// secret store location, is what keeps a run that a tenant creates directly
// from pointing the ExternalSecret at another organization's secret.
type BuildClientParams struct {
	SecretRefName string
}

// withBuildClient returns params with the run's ampBuild parameter set. The
// component's map is copied rather than modified, so the parameter reaches only
// this WorkflowRun.
func withBuildClient(params map[string]interface{}, buildClient *BuildClientParams) map[string]interface{} {
	out := make(map[string]interface{}, len(params)+1)
	maps.Copy(out, params)
	out[workflowParamAMPBuild] = map[string]interface{}{
		"secretRef": buildClient.SecretRefName,
	}
	return out
}

// attachBuildClient reports whether a build client should ride on a run of the
// given workflow: the workflow is listed in config.BuildClient.Workflows and its
// parameter schema declares ampBuild. A workflow that does not declare it (an
// organization whose namespaced Workflows predate it) would ignore or reject
// the parameter, so the run keeps its previous path instead; so does a run whose
// workflow cannot be looked up, rather than failing the build. Both are logged
// at Error: with the build client enabled, a run that keeps the previous path is
// a rollout gap that must not go unnoticed.
func (c *openChoreoClient) attachBuildClient(ctx context.Context, namespaceName string, kind gen.WorkflowRunConfigKind, workflowName string, buildClient *BuildClientParams) bool {
	if buildClient == nil {
		return false
	}
	if !slices.Contains(config.GetConfig().BuildClient.Workflows, workflowName) {
		slog.DebugContext(ctx, "Build workflow does not take the build client; not attaching it",
			"workflow", workflowName)
		return false
	}
	declared, err := c.workflowDeclaresParameter(ctx, namespaceName, kind, workflowName, workflowParamAMPBuild)
	if err != nil {
		slog.ErrorContext(ctx, "Could not read the build workflow's parameters; not attaching the build client",
			"workflow", workflowName, "kind", string(kind), "error", err)
		return false
	}
	if !declared {
		slog.ErrorContext(ctx, "Build workflow does not declare ampBuild; not attaching the build client",
			"workflow", workflowName, "kind", string(kind))
	}
	return declared
}

// workflowDeclaresParameter reports whether the Workflow or ClusterWorkflow's
// openAPIV3Schema lists param among its top-level properties.
func (c *openChoreoClient) workflowDeclaresParameter(ctx context.Context, namespaceName string, kind gen.WorkflowRunConfigKind, name, param string) (bool, error) {
	var schema *map[string]interface{}
	switch kind {
	case gen.WorkflowRunConfigKindWorkflow:
		resp, err := c.ocClient.GetWorkflowWithResponse(ctx, namespaceName, name)
		if err != nil {
			return false, fmt.Errorf("failed to get workflow %q: %w", name, err)
		}
		if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
			return false, fmt.Errorf("failed to get workflow %q: status %d", name, resp.StatusCode())
		}
		if resp.JSON200.Spec != nil && resp.JSON200.Spec.Parameters != nil {
			schema = resp.JSON200.Spec.Parameters.OpenAPIV3Schema
		}
	default:
		resp, err := c.ocClient.GetClusterWorkflowWithResponse(ctx, name)
		if err != nil {
			return false, fmt.Errorf("failed to get cluster workflow %q: %w", name, err)
		}
		if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
			return false, fmt.Errorf("failed to get cluster workflow %q: status %d", name, resp.StatusCode())
		}
		if resp.JSON200.Spec != nil && resp.JSON200.Spec.Parameters != nil {
			schema = resp.JSON200.Spec.Parameters.OpenAPIV3Schema
		}
	}
	if schema == nil {
		return false, nil
	}
	properties, _ := (*schema)["properties"].(map[string]interface{})
	_, ok := properties[param]
	return ok, nil
}

// BuildRunInfo is what the build publisher endpoint verifies about a WorkflowRun
// before acting on it.
type BuildRunInfo struct {
	Name          string
	ProjectName   string
	ComponentName string
	// WorkflowName is the Workflow or ClusterWorkflow the run executes.
	WorkflowName string
	// Status is one of the WorkflowStatus* constants (see extractWorkflowRunStatus).
	Status string
	// HealthChecks is the run's spec.workflow.parameters.healthChecks exactly as
	// the run carries it: the agent's health checks, copied into the run when the
	// build started. Nil when the run has none. It is not validated here;
	// PublishBuildWorkload checks it before it writes anything.
	HealthChecks any
}

// GetBuildRun returns the owner labels, workflow and status of a WorkflowRun in
// the organization's namespace. A run that does not exist there is
// utils.ErrNotFound, including a 404 that carries no JSON error body.
func (c *openChoreoClient) GetBuildRun(ctx context.Context, ouID, runName string) (*BuildRunInfo, error) {
	resp, err := c.ocClient.GetWorkflowRunWithResponse(ctx, c.NamespaceFor(ouID), runName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow run %q: %w", runName, err)
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil, fmt.Errorf("%w: workflow run %q", utils.ErrNotFound, runName)
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
		return nil, fmt.Errorf("empty response from get workflow run %q", runName)
	}
	run := resp.JSON200
	info := &BuildRunInfo{
		Name:          run.Metadata.Name,
		ProjectName:   getLabel(run.Metadata.Labels, string(LabelKeyProjectName)),
		ComponentName: getLabel(run.Metadata.Labels, string(LabelKeyComponentName)),
		WorkflowName:  "",
		Status:        extractWorkflowRunStatus(run),
	}
	if run.Spec != nil {
		info.WorkflowName = run.Spec.Workflow.Name
		if run.Spec.Workflow.Parameters != nil {
			info.HealthChecks = (*run.Spec.Workflow.Parameters)[workflowParamHealthChecks]
		}
	}
	return info, nil
}

// buildHealthChecks returns the health checks a build run carries, the way
// amp-generate-workload's Step 3b reads them: unset, null or an empty object
// means the build has none (nil). Anything else must be an object whose
// startup, readiness and liveness are each unset, null or an object; a value
// of any other shape is utils.ErrBadRequest. The value is returned unchanged,
// so the binding gets exactly what the run carries.
func buildHealthChecks(value any) (map[string]any, error) {
	if value == nil {
		return nil, nil //nolint:nilnil // nil means the build has no health checks
	}
	healthChecks, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: the build's healthChecks is not an object", utils.ErrBadRequest)
	}
	if len(healthChecks) == 0 {
		return nil, nil //nolint:nilnil // an empty object means the build has no health checks
	}
	for _, probe := range []string{"startup", "readiness", "liveness"} {
		check, present := healthChecks[probe]
		if !present || check == nil {
			continue
		}
		if _, ok := check.(map[string]any); !ok {
			return nil, fmt.Errorf("%w: the build's healthChecks.%s is not an object", utils.ErrBadRequest, probe)
		}
	}
	return healthChecks, nil
}

// PublishBuildWorkloadRequest is a build's generated Workload, already verified
// by the caller to belong to RunName's project and component.
type PublishBuildWorkloadRequest struct {
	// RunName names the WorkflowRun that built the image. The ComponentRelease is
	// named after it, and the Workload is recorded on it.
	RunName       string
	ProjectName   string
	ComponentName string
	// Image is the image the build published.
	Image string
	// Endpoints is the environment-independent endpoint contract, keyed by
	// endpoint name. Nil or empty leaves spec.endpoints unset.
	Endpoints map[string]gen.WorkloadEndpoint
	// WorkloadOverrides is the build's environment configuration (env vars and
	// file mounts). It is written only when this build creates the first
	// environment's ReleaseBinding; nil leaves spec.workloadOverrides unset.
	WorkloadOverrides *gen.WorkloadOverrides
	// HealthChecks is the run's healthChecks parameter (BuildRunInfo.HealthChecks).
	// Unlike WorkloadOverrides it is written on every build: onto a new binding,
	// and onto an existing one, where a build without health checks removes the
	// previous build's so the environment goes back to the default probes.
	HealthChecks any
}

// buildBindingMaxAttempts bounds the ReleaseBinding read-then-write loop, as the
// amp-generate-workload step does (BINDING_MAX_ATTEMPTS).
const buildBindingMaxAttempts = 3

// buildBindingRetryDelay is the pause between binding attempts (the step's sleep 2).
var buildBindingRetryDelay = 2 * time.Second

// PublishBuildWorkload is the backend's equivalent of amp-generate-workload's
// Steps 4-7, so a build no longer needs an OpenChoreo credential of its own:
//
//  1. Create the Workload <component>-workload, or replace it when it exists
//     (the create is answered 409).
//  2. Record the Workload on the WorkflowRun as the openchoreo.dev/workload
//     annotation, which GetBuild/ListBuilds read the built image and the
//     resolved endpoint schemas back from.
//  3. Cut a ComponentRelease named after the run. The run name is unique per
//     build, so this create cannot collide with an earlier release.
//  4. Bind the project's first environment (<component>-<env>) to the release.
//     A binding that does not exist yet is created carrying the build's
//     WorkloadOverrides. One that exists has only its release pin advanced:
//     once it exists it owns the environment's configuration, which the backend
//     edits directly, and a rebuild must not roll that back. Either way the
//     binding gets this build's health checks (componentTypeEnvironmentConfigs.
//     healthChecks), removed when the build has none.
//
// The build's health checks are checked before anything is written, so a
// malformed value (utils.ErrBadRequest) changes nothing.
//
// The binding write is retried, re-reading each time, on 409 and 5xx up to
// buildBindingMaxAttempts, because the backend writes the same binding
// concurrently; see the step's comment for why 500 is retryable.
func (c *openChoreoClient) PublishBuildWorkload(ctx context.Context, ouID string, req PublishBuildWorkloadRequest) error {
	healthChecks, err := buildHealthChecks(req.HealthChecks)
	if err != nil {
		return err
	}

	namespaceName := c.NamespaceFor(ouID)
	workloadName := req.ComponentName + "-workload"

	workload := gen.Workload{
		Metadata: gen.ObjectMeta{
			Name:      workloadName,
			Namespace: &namespaceName,
		},
		Spec: &gen.WorkloadSpec{
			Owner: &struct {
				ComponentName string `json:"componentName"`
				ProjectName   string `json:"projectName"`
			}{ComponentName: req.ComponentName, ProjectName: req.ProjectName},
			Container: &gen.WorkloadContainer{Image: req.Image},
		},
	}
	if len(req.Endpoints) > 0 {
		endpoints := req.Endpoints
		workload.Spec.Endpoints = &endpoints
	}

	if err := c.createOrReplaceWorkload(ctx, namespaceName, workload); err != nil {
		return err
	}
	if err := c.annotateRunWithWorkload(ctx, namespaceName, req.RunName, workload); err != nil {
		return err
	}
	if err := c.generateBuildRelease(ctx, namespaceName, req.ComponentName, req.RunName); err != nil {
		return err
	}

	pipeline, err := c.GetProjectDeploymentPipeline(ctx, ouID, req.ProjectName)
	if err != nil {
		return fmt.Errorf("failed to resolve the deployment pipeline of project %q: %w", req.ProjectName, err)
	}
	firstEnv := pipelineRootEnvironment(pipeline.PromotionPaths)
	if firstEnv == "" {
		return fmt.Errorf("deployment pipeline of project %q has no root environment", req.ProjectName)
	}
	return c.bindBuildRelease(ctx, namespaceName, req, firstEnv, healthChecks)
}

// setBindingHealthChecks puts the build's health checks on the binding's
// componentTypeEnvironmentConfigs, or removes any it has when the build has
// none, leaving the binding's other component-type configs as they are.
func setBindingHealthChecks(spec *gen.ReleaseBindingSpec, healthChecks map[string]any) {
	// A nil map must reach the helper as an untyped nil, which means "remove".
	var value any
	if healthChecks != nil {
		value = healthChecks
	}
	spec.ComponentTypeEnvironmentConfigs = replaceHealthChecksInComponentTypeConfigs(spec.ComponentTypeEnvironmentConfigs, value)
}

// createOrReplaceWorkload is Step 4: POST the Workload, and PUT the same body when
// the create is answered 409 because it already exists.
func (c *openChoreoClient) createOrReplaceWorkload(ctx context.Context, namespaceName string, workload gen.Workload) error {
	createResp, err := c.ocClient.CreateWorkloadWithResponse(ctx, namespaceName, workload)
	if err != nil {
		return fmt.Errorf("failed to create workload %q: %w", workload.Metadata.Name, err)
	}
	if is2xx(createResp.StatusCode()) {
		return nil
	}
	if createResp.StatusCode() != http.StatusConflict {
		return handleErrorResponse(createResp.StatusCode(), ErrorResponses{
			JSON400: createResp.JSON400,
			JSON401: createResp.JSON401,
			JSON403: createResp.JSON403,
			JSON500: createResp.JSON500,
		})
	}
	updateResp, err := c.ocClient.UpdateWorkloadWithResponse(ctx, namespaceName, workload.Metadata.Name, workload)
	if err != nil {
		return fmt.Errorf("failed to update workload %q: %w", workload.Metadata.Name, err)
	}
	if is2xx(updateResp.StatusCode()) {
		return nil
	}
	return handleErrorResponse(updateResp.StatusCode(), ErrorResponses{
		JSON400: updateResp.JSON400,
		JSON401: updateResp.JSON401,
		JSON403: updateResp.JSON403,
		JSON404: updateResp.JSON404,
		JSON500: updateResp.JSON500,
	})
}

// annotateRunWithWorkload is Step 5: set the openchoreo.dev/workload annotation on
// the WorkflowRun to the compact JSON of the Workload that was written.
func (c *openChoreoClient) annotateRunWithWorkload(ctx context.Context, namespaceName, runName string, workload gen.Workload) error {
	workloadJSON, err := json.Marshal(workload)
	if err != nil {
		return fmt.Errorf("failed to encode workload for workflow run %q: %w", runName, err)
	}

	getResp, err := c.ocClient.GetWorkflowRunWithResponse(ctx, namespaceName, runName)
	if err != nil {
		return fmt.Errorf("failed to get workflow run %q: %w", runName, err)
	}
	if getResp.StatusCode() != http.StatusOK {
		return handleErrorResponse(getResp.StatusCode(), ErrorResponses{
			JSON401: getResp.JSON401,
			JSON403: getResp.JSON403,
			JSON404: getResp.JSON404,
			JSON500: getResp.JSON500,
		})
	}
	if getResp.JSON200 == nil {
		return fmt.Errorf("empty response from get workflow run %q", runName)
	}
	run := *getResp.JSON200
	annotations := map[string]string{}
	if run.Metadata.Annotations != nil {
		maps.Copy(annotations, *run.Metadata.Annotations)
	}
	annotations[workflowRunWorkloadAnnotationKey] = string(workloadJSON)
	run.Metadata.Annotations = &annotations

	updateResp, err := c.ocClient.UpdateWorkflowRunWithResponse(ctx, namespaceName, runName, run)
	if err != nil {
		return fmt.Errorf("failed to update workflow run %q: %w", runName, err)
	}
	if is2xx(updateResp.StatusCode()) {
		return nil
	}
	return handleErrorResponse(updateResp.StatusCode(), ErrorResponses{
		JSON400: updateResp.JSON400,
		JSON401: updateResp.JSON401,
		JSON403: updateResp.JSON403,
		JSON404: updateResp.JSON404,
		JSON500: updateResp.JSON500,
	})
}

// generateBuildRelease is Step 6: cut the ComponentRelease, named after the run.
//
// A 409 means a release of that name already exists. Run names are unique per
// build, so that release can only be this build's own, cut by an earlier attempt
// of the same request that failed later on; it is accepted, which keeps the
// whole publish safe to repeat with the same body (the build step resends the
// identical document on retry).
func (c *openChoreoClient) generateBuildRelease(ctx context.Context, namespaceName, componentName, releaseName string) error {
	resp, err := c.ocClient.GenerateReleaseWithResponse(ctx, namespaceName, componentName,
		gen.GenerateReleaseJSONRequestBody{ReleaseName: &releaseName})
	if err != nil {
		return fmt.Errorf("failed to generate component release %q: %w", releaseName, err)
	}
	if is2xx(resp.StatusCode()) || resp.StatusCode() == http.StatusConflict {
		return nil
	}
	return handleErrorResponse(resp.StatusCode(), ErrorResponses{
		JSON400: resp.JSON400,
		JSON401: resp.JSON401,
		JSON403: resp.JSON403,
		JSON404: resp.JSON404,
		JSON500: resp.JSON500,
	})
}

// bindBuildRelease is Step 7: create <component>-<env> pinned to the release, or
// advance an existing binding's release pin, re-reading and retrying on 409/5xx.
// Both writes carry the build's health checks (see setBindingHealthChecks).
func (c *openChoreoClient) bindBuildRelease(ctx context.Context, namespaceName string, req PublishBuildWorkloadRequest, environment string, healthChecks map[string]any) error {
	bindingName := req.ComponentName + "-" + environment
	releaseName := req.RunName

	var lastStatus int
	for attempt := 1; attempt <= buildBindingMaxAttempts; attempt++ {
		if attempt > 1 {
			if err := sleepContext(ctx, buildBindingRetryDelay); err != nil {
				return fmt.Errorf("binding release %q to %q interrupted: %w", releaseName, bindingName, err)
			}
		}

		getResp, err := c.ocClient.GetReleaseBindingWithResponse(ctx, namespaceName, bindingName)
		if err != nil {
			return fmt.Errorf("failed to look up release binding %q: %w", bindingName, err)
		}

		var writeStatus int
		var writeErrs ErrorResponses
		switch {
		case getResp.StatusCode() == http.StatusNotFound:
			spec := &gen.ReleaseBindingSpec{
				Environment:       environment,
				ReleaseName:       &releaseName,
				WorkloadOverrides: req.WorkloadOverrides,
				Owner: struct {
					ComponentName string `json:"componentName"`
					ProjectName   string `json:"projectName"`
				}{ComponentName: req.ComponentName, ProjectName: req.ProjectName},
			}
			setBindingHealthChecks(spec, healthChecks)
			createResp, err := c.ocClient.CreateReleaseBindingWithResponse(ctx, namespaceName, gen.CreateReleaseBindingJSONRequestBody{
				Metadata: gen.ObjectMeta{
					Name:      bindingName,
					Namespace: &namespaceName,
					Labels:    c.withResourceLabels(nil),
				},
				Spec: spec,
			})
			if err != nil {
				return fmt.Errorf("failed to create release binding %q: %w", bindingName, err)
			}
			writeStatus = createResp.StatusCode()
			writeErrs = ErrorResponses{
				JSON400: createResp.JSON400,
				JSON401: createResp.JSON401,
				JSON403: createResp.JSON403,
				JSON500: createResp.JSON500,
			}
		case is2xx(getResp.StatusCode()):
			if getResp.JSON200 == nil || getResp.JSON200.Spec == nil {
				return fmt.Errorf("empty response from get release binding %q", bindingName)
			}
			binding := *getResp.JSON200
			binding.Spec.ReleaseName = &releaseName
			setBindingHealthChecks(binding.Spec, healthChecks)
			binding.Metadata.Labels = c.withResourceLabels(binding.Metadata.Labels)
			updateResp, err := c.ocClient.UpdateReleaseBindingWithResponse(ctx, namespaceName, bindingName, binding)
			if err != nil {
				return fmt.Errorf("failed to update release binding %q: %w", bindingName, err)
			}
			writeStatus = updateResp.StatusCode()
			writeErrs = ErrorResponses{
				JSON400: updateResp.JSON400,
				JSON401: updateResp.JSON401,
				JSON403: updateResp.JSON403,
				JSON404: updateResp.JSON404,
				JSON500: updateResp.JSON500,
			}
		case getResp.StatusCode() >= http.StatusInternalServerError:
			// The read races the same writers as the write below, so spend the
			// same retry budget on it rather than failing the build.
			lastStatus = getResp.StatusCode()
			slog.WarnContext(ctx, "Release binding lookup failed, retrying",
				"binding", bindingName, "status", lastStatus, "attempt", attempt, "maxAttempts", buildBindingMaxAttempts)
			continue
		default:
			return handleErrorResponse(getResp.StatusCode(), ErrorResponses{
				JSON401: getResp.JSON401,
				JSON403: getResp.JSON403,
				JSON500: getResp.JSON500,
			})
		}

		if is2xx(writeStatus) {
			return nil
		}
		if writeStatus == http.StatusConflict || writeStatus >= http.StatusInternalServerError {
			lastStatus = writeStatus
			slog.WarnContext(ctx, "Release binding write failed, re-reading and retrying",
				"binding", bindingName, "status", writeStatus, "attempt", attempt, "maxAttempts", buildBindingMaxAttempts)
			continue
		}
		return handleErrorResponse(writeStatus, writeErrs)
	}
	return fmt.Errorf("%w: release binding %q was not written after %d attempts (last status %d)",
		utils.ErrInternalServerError, bindingName, buildBindingMaxAttempts, lastStatus)
}

// pipelineRootEnvironment returns the project's first environment the way
// OpenChoreo's Component controller (and amp-generate-workload) resolves it: the
// first source environment, in promotion-path order, that is never itself a
// promotion target. Empty source names are skipped. "" when there is none.
func pipelineRootEnvironment(paths []models.PromotionPath) string {
	targets := make(map[string]bool)
	for _, path := range paths {
		for _, target := range path.TargetEnvironmentRefs {
			targets[target.Name] = true
		}
	}
	for _, path := range paths {
		if path.SourceEnvironmentRef != "" && !targets[path.SourceEnvironmentRef] {
			return path.SourceEnvironmentRef
		}
	}
	return ""
}

func is2xx(status int) bool {
	return status >= http.StatusOK && status < http.StatusMultipleChoices
}

// sleepContext waits for d, returning early with the context's error when it is
// cancelled first.
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-timer.C:
		return nil
	}
}
