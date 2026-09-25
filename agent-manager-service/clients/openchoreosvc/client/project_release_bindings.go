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
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
)

// -----------------------------------------------------------------------------
// Project Release Binding Operations
// -----------------------------------------------------------------------------

// projectReleaseBindingName is the (project, environment) binding name convention.
// It matches the one baked into the platform-resources Helm chart
// (templates/project-release-binding.yaml) so the chart-managed binding for the
// default project is recognised here instead of being duplicated.
func projectReleaseBindingName(projectName, environmentName string) string {
	return fmt.Sprintf("%s-%s", projectName, environmentName)
}

// EnsureProjectReleaseBinding creates the ProjectReleaseBinding for
// (project, environment) if it does not already exist.
//
// From OpenChoreo 1.2.0 the cell namespace is no longer implicit: it is a
// resource of the project's (Cluster)ProjectType, and a ProjectReleaseBinding is
// what applies that type to an environment. Without a binding the namespace is
// never created and every component ReleaseBinding in that environment fails
// with `namespaces "dp-..." not found`.
//
// spec.projectRelease is deliberately left unset — the Project controller seeds
// it once with the project's latest ProjectRelease.
//
// The call is idempotent: an existing binding for the same (project,
// environment) is success, and is given the org UUID namespace label if it
// predates it. A name collision with a binding that belongs to a different
// project or environment is an error rather than a silent no-op.
func (c *openChoreoClient) EnsureProjectReleaseBinding(ctx context.Context, ouID, projectName, environmentName string) error {
	namespaceName := c.NamespaceFor(ouID)
	bindingName := projectReleaseBindingName(projectName, environmentName)

	labels := map[string]string{
		string(LabelKeyProjectName):     projectName,
		string(LabelKeyEnvironmentName): environmentName,
	}
	body := gen.CreateProjectReleaseBindingJSONRequestBody{
		Metadata: gen.ObjectMeta{
			Name:      bindingName,
			Namespace: &namespaceName,
			Labels:    &labels,
		},
		Spec: &gen.ProjectReleaseBindingSpec{
			Owner: struct {
				ProjectName string `json:"projectName"`
			}{ProjectName: projectName},
			Environment: environmentName,
			// The project type merges these labels onto the cell namespace it
			// creates, which is where this project's pods run. The organization's
			// UUID goes there because nothing on a pod identifies an
			// organization and nothing downstream can derive it: the namespace
			// name is a hash of the UUID, and the renderer copies only its own
			// fixed label set onto workloads. Without this, compute measured in
			// this namespace is attributable to a project and a component but to
			// no customer.
			EnvironmentConfigs: &map[string]interface{}{
				namespaceLabelsKey: map[string]string{
					string(LabelKeyOrgUUID): ouID,
				},
			},
		},
	}

	resp, err := c.ocClient.CreateProjectReleaseBindingWithResponse(ctx, namespaceName, body)
	if err != nil {
		return fmt.Errorf("failed to create project release binding %q: %w", bindingName, err)
	}

	switch resp.StatusCode() {
	case http.StatusCreated, http.StatusOK:
		return nil
	case http.StatusConflict:
		// Already exists — confirm it is ours before treating it as success.
		return c.reconcileExistingProjectReleaseBinding(ctx, ouID, namespaceName, bindingName, projectName, environmentName)
	default:
		// Named so a caller can tell this apart from the lookup below, which
		// maps the same set of statuses.
		return fmt.Errorf("failed to create project release binding %q: %w", bindingName, handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON400: resp.JSON400,
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON409: resp.JSON409,
			JSON500: resp.JSON500,
		}))
	}
}

// namespaceLabelsKey is the ProjectType environmentConfigs field whose labels
// are merged onto the cell namespace.
const namespaceLabelsKey = "namespaceLabels"

// reconcileExistingProjectReleaseBinding checks that an existing binding really
// is the one for (project, environment), then backfills the org UUID namespace
// label on bindings created before it was set. Binding names are derived from
// both, so a mismatch means two different tuples collapsed onto the same name
// (e.g. project "a-b" + env "c" vs project "a" + env "b-c"); silently accepting
// that would point the project at another project's namespace.
//
// A failed backfill is logged, not returned: the binding is usable without the
// label, and the next deploy or promote retries it.
func (c *openChoreoClient) reconcileExistingProjectReleaseBinding(ctx context.Context, ouID, namespaceName, bindingName, projectName, environmentName string) error {
	resp, err := c.ocClient.GetProjectReleaseBindingWithResponse(ctx, namespaceName, bindingName)
	if err != nil {
		return fmt.Errorf("failed to get existing project release binding %q: %w", bindingName, err)
	}
	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("failed to get existing project release binding %q: %w", bindingName, handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		}))
	}
	if resp.JSON200 == nil || resp.JSON200.Spec == nil {
		return fmt.Errorf("empty response from get project release binding %q", bindingName)
	}

	binding := resp.JSON200
	spec := binding.Spec
	if spec.Owner.ProjectName != projectName || spec.Environment != environmentName {
		return fmt.Errorf(
			"project release binding %q already exists for project %q environment %q, not project %q environment %q",
			bindingName, spec.Owner.ProjectName, spec.Environment, projectName, environmentName,
		)
	}

	if !setNamespaceLabel(spec, string(LabelKeyOrgUUID), ouID) {
		return nil
	}
	updateResp, err := c.ocClient.UpdateProjectReleaseBindingWithResponse(ctx, namespaceName, bindingName, *binding)
	if err == nil && updateResp.StatusCode() != http.StatusOK {
		err = handleErrorResponse(updateResp.StatusCode(), ErrorResponses{
			JSON400: updateResp.JSON400,
			JSON401: updateResp.JSON401,
			JSON403: updateResp.JSON403,
			JSON404: updateResp.JSON404,
			JSON500: updateResp.JSON500,
		})
	}
	if err != nil {
		logger.GetLogger(ctx).Warn("failed to add org UUID namespace label to existing project release binding",
			"binding", bindingName, "namespace", namespaceName, "error", err)
	}
	return nil
}

// setNamespaceLabel sets key=value in spec.environmentConfigs.namespaceLabels,
// keeping any other configs and labels, and reports whether anything changed.
func setNamespaceLabel(spec *gen.ProjectReleaseBindingSpec, key, value string) bool {
	if spec.EnvironmentConfigs == nil {
		spec.EnvironmentConfigs = &map[string]interface{}{}
	}
	configs := *spec.EnvironmentConfigs

	labels := map[string]interface{}{}
	if existing, ok := configs[namespaceLabelsKey].(map[string]interface{}); ok {
		if existing[key] == value {
			return false
		}
		labels = existing
	}
	labels[key] = value
	configs[namespaceLabelsKey] = labels
	return true
}
