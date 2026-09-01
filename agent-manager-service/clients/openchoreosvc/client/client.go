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

// Package client provides the OpenChoreo API client.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg clientmocks -out ../../clientmocks/openchoreo_client_fake.go . OpenChoreoClient:OpenChoreoClientMock
package client

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/clients/requests"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
)

// HeaderImpersonateOrg carries the org UUID the call is performed on behalf
// of, resolved from the request context (see middleware.GetResolvedOrg).
const HeaderImpersonateOrg = "X-Impersonate-Org"

// Config contains configuration for the OpenChoreo client
type Config struct {
	BaseURL      string
	AuthProvider AuthProvider
	RetryConfig  requests.RequestRetryConfig
	// DefaultNamespace is the OpenChoreo namespace (organization) all API
	// calls are scoped to. The deployment runs single-namespace, so every
	// method overrides its namespace/org argument with this value.
	DefaultNamespace string
}

// OpenChoreoClient defines the interface for OpenChoreo operations
type OpenChoreoClient interface {
	// NamespaceFor resolves the OpenChoreo namespace an OU's workloads run in.
	// Callers that address that namespace outside this client (e.g. observability
	// log/metric queries) use this so the resolution stays in one place.
	NamespaceFor(ouID string) string

	// Organization Operations (maps to OC namespaces)
	GetOrganization(ctx context.Context, ouID string) (*models.OrganizationResponse, error)
	ListOrganizations(ctx context.Context) ([]*models.OrganizationResponse, error)

	// Project Operations
	CreateProject(ctx context.Context, ouID string, req CreateProjectRequest) error
	GetProject(ctx context.Context, ouID, projectName string) (*models.ProjectResponse, error)
	PatchProject(ctx context.Context, ouID, projectName string, req PatchProjectRequest) error
	DeleteProject(ctx context.Context, ouID, projectName string) error
	ListProjects(ctx context.Context, ouID string) ([]*models.ProjectResponse, error)
	EnsureProjectReleaseBinding(ctx context.Context, ouID, projectName, environmentName string) error

	// Component Operations
	CreateComponent(ctx context.Context, ouID, projectName string, req CreateComponentRequest) error
	GetComponent(ctx context.Context, ouID, projectName, componentName string) (*models.AgentResponse, error)
	// GetComponentReconcileBlock reports why OpenChoreo cannot reconcile the component, or nil when
	// nothing is blocking it. Used to refuse writes the Component controller would silently discard.
	GetComponentReconcileBlock(ctx context.Context, ouID, componentName string) (*ComponentReconcileBlock, error)
	UpdateComponentBasicInfo(ctx context.Context, ouID, projectName, componentName string, req UpdateComponentBasicInfoRequest) error
	GetEnvResourceConfigs(ctx context.Context, ouID, projectName, componentName, environment string) (*ComponentResourceConfigsResponse, error)
	UpdateEnvResourceConfigs(ctx context.Context, ouID, projectName, componentName, environment string, req UpdateComponentResourceConfigsRequest) error
	DeleteComponent(ctx context.Context, ouID, projectName, componentName string) error
	ListComponents(ctx context.Context, ouID, projectName string) ([]*models.AgentResponse, error)
	ListComponentsByKind(ctx context.Context, ouID, projectName, kindName string) ([]*models.AgentResponse, error)
	ComponentExists(ctx context.Context, ouID, projectName, componentName string) (bool, error)
	AttachTraits(ctx context.Context, ouID, projectName, componentName string, traitRequests []TraitRequest) error
	DetachTrait(ctx context.Context, ouID, projectName, componentName string, traitType TraitType) error
	HasTrait(ctx context.Context, ouID, projectName, componentName string, traitType TraitType) (bool, error)
	UpdateComponentDeploymentConfig(ctx context.Context, ouID, projectName, componentName string, req ComponentDeploymentConfigRequest) error
	UpdateComponentEnvVars(ctx context.Context, ouID, projectName, componentName string, envVars []EnvVar) error
	ReplaceComponentEnvVars(ctx context.Context, ouID, projectName, componentName string, envVars []EnvVar) error
	ReplaceComponentFileMounts(ctx context.Context, ouID, projectName, componentName string, files []FileVar) error
	UpdateReleaseBindingEnvVars(ctx context.Context, ouID, projectName, componentName, envName string, envVars []EnvVar) error
	RemoveComponentEnvironmentVariables(ctx context.Context, ouID, projectName, componentName string, envVarKeys []string) error
	RemoveReleaseBindingEnvVars(ctx context.Context, ouID, projectName, componentName, envName string, envVarKeys []string) error
	ReplaceReleaseBindingEnvVars(ctx context.Context, ouID, projectName, componentName, envName string, keysToRemove []string, envVarsToAdd []EnvVar) error
	RemoveWorkloadEnvVars(ctx context.Context, ouID, componentName string, envVarKeys []string) error
	GetComponentEndpoints(ctx context.Context, ouID, projectName, componentName, environment string) (map[string]models.EndpointsResponse, error)
	GetComponentConfigurations(ctx context.Context, ouID, projectName, componentName, environment string) ([]models.EnvVars, error)
	GetComponentFileMounts(ctx context.Context, ouID, projectName, componentName, environment string) ([]models.FileMountEntry, error)

	// Build Operations
	// workflowRunName may be supplied by the service when deployment-specific work must
	// happen before the run is created. An empty value preserves the default behavior and
	// lets the client generate the name.
	TriggerBuild(ctx context.Context, ouID, projectName, componentName, commitID, workflowRunName string) (*models.BuildResponse, error)
	GetBuild(ctx context.Context, ouID, projectName, componentName, buildName string) (*models.BuildDetailsResponse, error)
	ListBuilds(ctx context.Context, ouID, projectName, componentName string) ([]*models.BuildResponse, error)
	UpdateComponentBuildParameters(ctx context.Context, ouID, projectName, componentName string, req UpdateComponentBuildParametersRequest) error

	// Deployment Operations
	Deploy(ctx context.Context, ouID, projectName, componentName string, req DeployRequest) error
	CreateInternalAgentFromKindWorkload(ctx context.Context, ouID, projectName, componentName string, req InternalAgentFromKindWorkloadRequest) error
	// EnsureReleaseAndBinding cuts a ComponentRelease from the component's current state and
	// binds it to the environment, carrying that environment's configuration as workloadOverrides.
	// Components are created with autoDeploy off, so this is the only thing that advances what an
	// environment runs outside the build workflow: every deploy and every kind-sourced agent
	// creation goes through it.
	EnsureReleaseAndBinding(ctx context.Context, ouID, projectName, componentName, environment string, envOverrides []EnvVar, fileOverrides []FileVar) error
	GetDeployments(ctx context.Context, ouID, pipelineName, projectName, componentName string) ([]*models.DeploymentResponse, error)
	UpdateDeploymentState(ctx context.Context, ouID, projectName, componentName, environment string, state gen.ReleaseBindingSpecState) error
	IsDeploymentInProgress(ctx context.Context, ouID, componentName, environment string) (bool, error)

	// Environment Operations
	CreateEnvironment(ctx context.Context, ouID string, req CreateEnvironmentRequest) (*models.EnvironmentResponse, error)
	GetEnvironment(ctx context.Context, ouID, environmentName string) (*models.EnvironmentResponse, error)
	UpdateEnvironment(ctx context.Context, ouID, environmentName string, req UpdateEnvironmentRequest) (*models.EnvironmentResponse, error)
	DeleteEnvironment(ctx context.Context, ouID, environmentName string) error
	ListEnvironments(ctx context.Context, ouID string) ([]*models.EnvironmentResponse, error)

	// Release Binding Operations
	UpdateReleaseBindingTraitConfigs(ctx context.Context, ouID, componentName, environment string, traitConfigs map[string]interface{}, componentTypeConfigs map[string]interface{}) error
	// EnsureReleaseBindingRuntimeClass idempotently reconciles runtimeClassName on a binding created
	// out-of-band by the build workflow. Writes only when the value differs (see impl).
	EnsureReleaseBindingRuntimeClass(ctx context.Context, ouID, componentName, environment, desiredRuntimeClass string) error
	ReplaceReleaseBindingWorkloadOverrides(ctx context.Context, ouID, componentName, environment string, envOverrides []EnvVar, fileOverrides []FileVar) error

	// Promotion Operations
	PromoteComponent(ctx context.Context, ouID, projectName, componentName, sourceEnvironment, targetEnvironment string, envOverrides []EnvVar, fileOverrides []FileVar, traitEnvConfigs map[string]interface{}, componentTypeConfigs map[string]interface{}) error
	// GetSourceEnvWorkloadOverrides fetches the workload overrides (env vars and file mounts)
	// from the source environment's release binding, converted to client types.
	GetSourceEnvWorkloadOverrides(ctx context.Context, ouID, componentName, sourceEnvironment string) ([]EnvVar, []FileVar, error)

	// Infrastructure Operations
	GetProjectDeploymentPipeline(ctx context.Context, ouID, projectName string) (*models.DeploymentPipelineResponse, error)
	CreateDeploymentPipeline(ctx context.Context, ouID, pipelineName string, displayName *string, description *string, promotionPaths []models.PromotionPath) (*models.DeploymentPipelineResponse, error)
	UpdateDeploymentPipeline(ctx context.Context, ouID, pipelineName string, displayName *string, description *string, promotionPaths []models.PromotionPath) (*models.DeploymentPipelineResponse, error)
	DeleteOrgDeploymentPipeline(ctx context.Context, ouID string, pipelineName string) error
	ListDeploymentPipelines(ctx context.Context, ouID string) ([]*models.DeploymentPipelineResponse, error)
	ListDataPlanes(ctx context.Context) ([]*models.DataPlaneResponse, error)

	// WorkflowRun Operations
	CreateWorkflowRun(ctx context.Context, ouID string, req CreateWorkflowRunRequest) (*WorkflowRunResponse, error)
	GetWorkflowRun(ctx context.Context, ouID, runName string) (*WorkflowRunResponse, error)
	ExpireWorkflowRun(ctx context.Context, ouID, runName string) error

	// Secret Reference Operations
	CreateSecretReference(ctx context.Context, ouID string, req CreateSecretReferenceRequest) (*SecretReferenceInfo, error)
	GetSecretReference(ctx context.Context, ouID, secretRefName string) (*SecretReferenceInfo, error)
	ListSecretReferences(ctx context.Context, ouID string, componentName string) ([]*SecretReferenceInfo, error)
	UpdateSecretReference(ctx context.Context, ouID, secretRefName string, req CreateSecretReferenceRequest) (*SecretReferenceInfo, error)
	DeleteSecretReference(ctx context.Context, ouID, secretRefName string) error

	// Workload Operations
	GetWorkloadSecretRefNames(ctx context.Context, ouID, projectName, componentName string) ([]string, error)

	// Secret Operations (OpenChoreo-managed secret storage; the API stores
	// values in the target plane's secret store and manages the underlying
	// SecretReference internally)
	CreateSecret(ctx context.Context, ouID string, req CreateSecretRequest) (*SecretInfo, error)
	GetSecret(ctx context.Context, ouID, secretName string) (*SecretInfo, error)
	UpdateSecret(ctx context.Context, ouID, secretName string, req UpdateSecretRequest) (*SecretInfo, error)
	DeleteSecret(ctx context.Context, ouID, secretName string) error

	// Git Secret Operations
	CreateGitSecret(ctx context.Context, ouID string, req CreateGitSecretRequest) (*GitSecretInfo, error)
	ListGitSecrets(ctx context.Context, ouID string) ([]*GitSecretInfo, error)
	DeleteGitSecret(ctx context.Context, ouID, secretName string) error

	// Authz Operations
	// EnsureClusterRoleBinding creates a ClusterAuthzRoleBinding binding the given clientID (sub claim)
	// to the named ClusterAuthzRole. Idempotent — succeeds silently if the binding already exists.
	EnsureClusterRoleBinding(ctx context.Context, clientID, roleName string) error
}

type openChoreoClient struct {
	ocClient *gen.ClientWithResponses
	// defaultNamespace is the OpenChoreo namespace all API calls resolve to;
	// see NamespaceFor.
	defaultNamespace string
}

// NamespaceFor resolves the OpenChoreo namespace an OU's workloads run in.
// The deployment currently runs single-namespace, so every OU maps to the one
// configured default namespace (OPEN_CHOREO_DEFAULT_NAMESPACE). Exposed on the
// interface so other components that address the same namespace (e.g. the
// observability service reading a workload's logs) resolve it identically.
//
// Multi-tenancy TODO: when each org gets its own OpenChoreo namespace, replace
// this with a stable OU ID → OC namespace mapping. Namespace and org handle are
// NOT the same thing — key the mapping on the immutable OU ID, never the org
// handle. The handle is user-editable, and the namespace (plus the env-Thunder
// issuer/JWKS/token URLs derived from it) must stay fixed for an org's lifetime;
// deriving it from a value that can change would break addressing and invalidate
// already-issued tokens on every rename.
func (c *openChoreoClient) NamespaceFor(_ string) string {
	return c.defaultNamespace
}

func NewOpenChoreoClient(cfg *Config) (OpenChoreoClient, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	if cfg.AuthProvider == nil {
		return nil, fmt.Errorf("auth provider is required")
	}

	// Configure retry behavior to handle 401 Unauthorized by invalidating the token
	retryConfig := cfg.RetryConfig
	if retryConfig.RetryOnStatus == nil {
		// Custom retry logic that includes 401 handling + default transient errors
		retryConfig.RetryOnStatus = func(statusCode int) bool {
			// Handle 401 by invalidating cached token and retrying
			if statusCode == http.StatusUnauthorized {
				slog.Info("Received 401 Unauthorized, invalidating cached token")
				cfg.AuthProvider.InvalidateToken()
				return true
			}

			return slices.Contains(requests.TransientHTTPErrorCodes, statusCode)
		}
	}

	// Create the retryable HTTP client with 401 handling
	httpClient := requests.NewRetryableHTTPClient(&http.Client{}, retryConfig)

	// Create auth request editor
	authEditor := func(ctx context.Context, req *http.Request) error {
		token, err := cfg.AuthProvider.GetToken(ctx)
		if err != nil {
			return fmt.Errorf("failed to get auth token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		// Use the new OpenAPI handlers instead of legacy handlers
		req.Header.Set("X-Use-OpenAPI", "true")
		return nil
	}

	// Propagate the resolved org so OpenChoreo can attribute the call to the
	// organization being operated on. Absent on contexts that don't originate
	// from an authenticated API request (e.g. background reconcilers).
	orgEditor := func(ctx context.Context, req *http.Request) error {
		if org, ok := orgctx.GetResolvedOrg(ctx); ok && org.OUID != "" {
			req.Header.Set(HeaderImpersonateOrg, org.OUID)
		}
		return nil
	}

	// Create the generated OpenAPI client with retryable HTTP client and auth
	ocClient, err := gen.NewClientWithResponses(
		cfg.BaseURL,
		gen.WithHTTPClient(httpClient),
		gen.WithRequestEditorFn(authEditor),
		gen.WithRequestEditorFn(orgEditor),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenChoreo client: %w", err)
	}

	return &openChoreoClient{
		ocClient:         ocClient,
		defaultNamespace: cfg.DefaultNamespace,
	}, nil
}
