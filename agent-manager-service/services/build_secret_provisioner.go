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

package services

import (
	"context"

	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// BuildSecretProvisioner provisions the per-build git clone secret for a source
// build, keyed by the WorkflowRun name. It is called after the WorkflowRun name is
// chosen but before the WorkflowRun is created, so the secret exists in the workflow
// namespace before the run's checkout-source step clones the repository.
//
// In open-source deployments this is nil and the build behaves exactly as before: the
// clone secret named "{workflowRunName}-git-secret" is materialized (when the component
// has a non-empty repository.secretRef) by the ExternalSecret rendered from the git
// secret the user created with a personal access token, or the clone is anonymous. A
// deployment can inject an implementation (via app.Options) that mints a short-lived
// token from a platform GitHub App and writes that same "{workflowRunName}-git-secret"
// secret directly, so private repos can be cloned without a stored PAT.
//
// Implementations must be idempotent (a client retry after a partial failure re-mints
// the same per-run secret) and must be a no-op — returning nil — when the component is
// not managed by the provisioner (e.g. a public repo, or a PAT/secretRef component),
// leaving the open-source clone paths untouched.
type BuildSecretProvisioner interface {
	// PutSource persists the GitHub App repository binding selected as part of
	// component creation. Implementations must treat this as an idempotent upsert.
	PutSource(ctx context.Context, ouID, projectName, componentName string, source spec.GitHubAppSource) error

	// HasSource reports whether this component has a persisted GitHub App source
	// binding. A false result leaves the public-repository and PAT paths untouched.
	HasSource(ctx context.Context, ouID, projectName, componentName string) (bool, error)

	// DeleteSource removes the persisted GitHub App source binding when its component
	// is deleted. Implementations must treat an already-absent binding as success.
	DeleteSource(ctx context.Context, ouID, projectName, componentName string) error

	// EnsureBuildSecret provisions the "{workflowRunName}-git-secret" clone secret for
	// a component already confirmed to have a GitHub App source binding.
	EnsureBuildSecret(ctx context.Context, ouID, projectName, componentName, workflowRunName string) error
}
