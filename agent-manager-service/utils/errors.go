// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package utils

import (
	"errors"
	"fmt"
)

// ValidationError represents a validation error with both user-friendly message and technical details.
// Use this when you need to communicate errors to both end-users (via Message) and developers (via Reason).
type ValidationError struct {
	// Message is a user-friendly error message suitable for display in UI.
	// Should be clear, non-technical, and actionable.
	// Example: "Please provide a valid schema path starting with /"
	Message string

	// Reason contains technical details for debugging.
	// Can include field names, specific validation rules, etc.
	// Example: "inputInterface.schema.path is required and must start with /"
	Reason string

	// sentinel, when set, is the classification error this failure also matches
	// under errors.Is — so a check like errors.Is(err, ErrInvalidInput) still
	// recognises a ValidationError raised in place of a wrapped sentinel.
	// Unexported so a constructor is the only way to set it.
	sentinel error
}

// Error implements the error interface, returning the technical reason for logging.
func (e *ValidationError) Error() string {
	return e.Reason
}

// Unwrap exposes the classification sentinel to errors.Is/errors.As.
func (e *ValidationError) Unwrap() error {
	return e.sentinel
}

// NewValidationError creates a new ValidationError with user-friendly message and technical reason.
func NewValidationError(message, reason string) *ValidationError {
	return &ValidationError{Message: message, Reason: reason, sentinel: nil}
}

// NewInvalidInputError is NewValidationError for a failure that must also keep
// matching ErrInvalidInput under errors.Is, so callers that route on the
// sentinel keep working while the UI gets the short Message instead of the
// whole technical string.
//
// The sentinel is fixed rather than a parameter because every ValidationError
// is rendered as 400 by WriteValidationErrorResponse — pairing one with a
// not-found or conflict sentinel would still answer 400.
func NewInvalidInputError(message, reason string) *ValidationError {
	return &ValidationError{Message: message, Reason: reason, sentinel: ErrInvalidInput}
}

// NewValidationErrorf creates a new ValidationError with formatted reason string.
// The message should be user-friendly, while reasonFmt is for technical details.
func NewValidationErrorf(message, reasonFmt string, args ...interface{}) *ValidationError {
	return NewValidationError(message, fmt.Sprintf(reasonFmt, args...))
}

// IsValidationError checks if an error is a ValidationError and returns it.
// Returns nil if the error is not a ValidationError.
func IsValidationError(err error) *ValidationError {
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve
	}
	return nil
}

var (
	// Resource not found errors
	ErrProjectNotFound             = errors.New("project not found")
	ErrAgentAlreadyExists          = errors.New("agent already exists")
	ErrAgentNotFound               = errors.New("agent not found")
	ErrTraitNotFound               = errors.New("trait not found")
	ErrOrganizationNotFound        = errors.New("organization not found")
	ErrBuildNotFound               = errors.New("build not found")
	ErrEnvironmentNotFound         = errors.New("environment not found")
	ErrAgentIdentityNotProvisioned = errors.New("agent identity not yet provisioned for this environment")
	// ErrAgentIdentityRetryNotAllowed is returned when a retry is requested for
	// a binding whose current status is not failed. Maps to 409.
	ErrAgentIdentityRetryNotAllowed = errors.New("agent identity binding is not in a failed state and cannot be retried")
	ErrOrganizationAlreadyExists    = errors.New("organization already exists")
	ErrProjectAlreadyExists         = errors.New("project already exists")
	ErrDeploymentPipelineNotFound   = errors.New("deployment pipeline not found")
	ErrDeploymentPipelineInUse      = errors.New("deployment pipeline is referenced by one or more projects")
	ErrDeploymentInProgress         = errors.New("a deployment is already in progress")
	// ErrBuildInProgress is returned when agent configuration is created/updated
	// while a build is running. Argo Workflows resolves workflow.parameters once
	// at WorkflowRun submission, so a config change written to the Component CR
	// after that point is never picked up by the in-flight build. Maps to 409.
	ErrBuildInProgress            = errors.New("a build is already in progress for this agent")
	ErrProjectHasAssociatedAgents = errors.New("project has associated agents")
	// ErrComponentNotReconcilable reports that OpenChoreo cannot reconcile the agent's Component,
	// so a deploy or promote would be silently discarded: the writes land on the Component and
	// Workload, but no new ComponentRelease is cut, so the pods restart on the previous snapshot
	// with the old image and env. Both operations are refused up front rather than reporting a
	// success that did nothing. The wording says "deployed" for both, since a promote is the
	// deploy of an existing release into the next environment.
	ErrComponentNotReconcilable       = errors.New("agent cannot be deployed in its current state")
	ErrMonitorNotFound                = errors.New("monitor not found")
	ErrMonitorAlreadyExists           = errors.New("monitor already exists")
	ErrMonitorRunNotFound             = errors.New("monitor run not found")
	ErrMonitorAlreadyStopped          = errors.New("monitor already stopped")
	ErrMonitorAlreadyActive           = errors.New("monitor already active")
	ErrEvaluatorNotFound              = errors.New("evaluator not found")
	ErrCustomEvaluatorNotFound        = errors.New("custom evaluator not found")
	ErrCustomEvaluatorAlreadyExists   = errors.New("custom evaluator already exists")
	ErrCustomEvaluatorIdentifierTaken = errors.New("evaluator identifier conflicts with a built-in evaluator")
	ErrCustomEvaluatorInUse           = errors.New("custom evaluator is referenced by one or more active monitors")
	ErrInvalidInput                   = errors.New("invalid input")
	ErrImmutableFieldChange           = errors.New("cannot change immutable field")
	ErrScopeNotFound                  = errors.New("scope not found")

	// HTTP errors
	ErrBadRequest   = errors.New("bad request")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")

	// Server errors
	ErrInternalServerError = errors.New("internal server error")
	ErrServiceUnavailable  = errors.New("service unavailable")

	// Gateway-related errors
	ErrGatewayNotFound          = errors.New("gateway not found")
	ErrGatewayAlreadyExists     = errors.New("gateway already exists")
	ErrInvalidAdapterType       = errors.New("invalid adapter type")
	ErrGatewayUnreachable       = errors.New("gateway unreachable")
	ErrInvalidGatewayConfig     = errors.New("invalid gateway configuration")
	ErrEnvironmentAlreadyExists = errors.New("environment already exists")
	ErrEnvironmentHasGateways   = errors.New("environment has associated gateways")
	ErrEnvironmentInUse         = errors.New("environment is referenced by one or more deployment pipelines")
	// ErrThunderHandleTaken is returned when a user-supplied env-Thunder URL handle
	// is already registered to a different (org, env) pair. Maps to 409.
	ErrThunderHandleTaken = errors.New("thunder url handle is already in use")
	// ErrInvalidThunderHandle is returned when a user-supplied env-Thunder URL
	// handle fails format validation. Maps to 400.
	ErrInvalidThunderHandle = errors.New("invalid thunder url handle")
	// ErrThunderHandleNotFound is returned when no env-Thunder URL handle has been
	// registered for an environment. Maps to 404.
	ErrThunderHandleNotFound = errors.New("thunder url handle not found")
	// ErrInvalidThunderURL is returned when a caller-supplied full env-Thunder
	// URL (the url registration path) fails shape or SSRF validation. Maps to 400.
	ErrInvalidThunderURL = errors.New("invalid thunder url")
	// ErrThunderURLTaken is returned when a caller-supplied full env-Thunder URL
	// is already registered to a different (org, env) pair. Maps to 409.
	ErrThunderURLTaken = errors.New("thunder url is already in use")
	// ErrThunderHandleAndURLBothSet is returned when a SetThunderURL request
	// supplies both handle and url — the two registration paths are mutually
	// exclusive. Maps to 400.
	ErrThunderHandleAndURLBothSet = errors.New("thunder url request must not set both handle and url")
	// ErrEnvThunderURLAlreadyClaimed is returned by EnvThunderURLRepository.Insert
	// when a DIFFERENT concurrent request already claimed the SAME (ouID, envName)
	// first — i.e. the (ou_id, env_name) unique constraint was violated, not the
	// thunder_handle one. Internal-only signal between the repository and
	// EnvironmentService.SetThunderURL: the service reacts by reading back the
	// row that won and adopting/rejecting it, exactly as it would for an
	// already-existing row seen up front — this never reaches a controller.
	ErrEnvThunderURLAlreadyClaimed = errors.New("env-thunder url already claimed by a concurrent request")

	// ErrGatewayIngressCapExceeded is returned when assigning an ingress-capable gateway to
	// an environment that already has one. Maps to 409.
	ErrGatewayIngressCapExceeded = errors.New("environment already has an ingress gateway")

	// LLM Provider-related errors
	ErrProviderNotFound        = errors.New("provider not found")
	ErrProviderAlreadyExists   = errors.New("provider already exists")
	ErrProviderHasDeployments  = errors.New("provider has active deployments")
	ErrDeploymentNotFound      = errors.New("deployment not found")
	ErrDeploymentFailed        = errors.New("deployment failed")
	ErrPolicyNotSupported      = errors.New("policy not supported by gateway")
	ErrInvalidProviderConfig   = errors.New("invalid provider configuration")
	ErrSystemTemplateImmutable = errors.New("system templates cannot be modified or deleted")
	ErrSystemTemplateOverride  = errors.New("cannot create user template with same handle as system template")

	// API Platform integration errors
	ErrHandleExists                = errors.New("handle already exists")
	ErrGatewayHasAssociatedAPIs    = errors.New("gateway has associated APIs")
	ErrGatewayHasDeployments       = errors.New("cannot delete gateway: it has active API deployments. Please undeploy all APIs before deleting the gateway")
	ErrAPINotFound                 = errors.New("API not found")
	ErrDeploymentNotActive         = errors.New("deployment not active")
	ErrLLMProviderTemplateNotFound = errors.New("LLM provider template not found")
	ErrLLMProviderTemplateExists   = errors.New("LLM provider template already exists")
	ErrLLMProviderNotFound         = errors.New("LLM provider not found")
	ErrLLMProviderExists           = errors.New("LLM provider already exists")
	ErrLLMProviderHasProxies       = errors.New("cannot delete LLM provider: it has associated LLM proxies. Please delete all proxies before deleting the provider")
	ErrLLMProviderUndeployFailed   = errors.New("cannot delete LLM provider: undeploying it from its gateways failed. Retry, or undeploy manually before deleting")
	ErrLLMProviderDeleteInProgress = errors.New("cannot delete LLM provider: a delete is already in progress for it")
	ErrLLMProviderBeingDeleted     = errors.New("cannot create LLM proxy: the LLM provider is being deleted")
	ErrLLMProxyNotFound            = errors.New("LLM proxy not found")
	ErrLLMProxyExists              = errors.New("LLM proxy already exists")
	ErrMCPProxyNotFound            = errors.New("MCP proxy not found")
	ErrMCPProxyExists              = errors.New("MCP proxy already exists")
	ErrMCPProxyHasMappings         = errors.New("cannot delete MCP proxy: it has associated MCP proxy mappings. Please delete all mappings before deleting the proxy")
	ErrMCPEnvAlreadyBound          = errors.New("environment is already assigned to another endpoint in this MCP proxy")
	ErrInvalidURL                  = errors.New("invalid URL")
	ErrURLUnreachable              = errors.New("URL unreachable")
	ErrMCPServerUnauthorized       = errors.New("MCP server unauthorized")
	ErrMCPResponseTooLarge         = errors.New("MCP response body too large")
	ErrBaseDeploymentNotFound      = errors.New("base deployment not found")
	ErrDeploymentIsDeployed        = errors.New("deployment is currently deployed")
	ErrDeploymentAlreadyDeployed   = errors.New("deployment already deployed")
	ErrGatewayIDMismatch           = errors.New("gateway ID mismatch")
	ErrDeploymentNameRequired      = errors.New("deployment name required")
	ErrDeploymentBaseRequired      = errors.New("deployment base required")
	ErrDeploymentGatewayIDRequired = errors.New("deployment gateway ID required")
	ErrInvalidDeploymentStatus     = errors.New("invalid deployment status")
	ErrArtifactNotFound            = errors.New("artifact not found")
	ErrArtifactExists              = errors.New("artifact already exists")
	ErrDevPortalNotFound           = errors.New("devportal not found")
	ErrAPIAlreadyPublished         = errors.New("api is already published to devportal")
	ErrAPIPublicationNotFound      = errors.New("api publication not found")

	// Implementation status errors
	ErrNotImplemented = errors.New("not implemented")

	// Agent Configuration errors
	ErrAgentConfigNotFound      = errors.New("agent configuration not found")
	ErrAgentConfigAlreadyExists = errors.New("agent configuration already exists for this agent")
	// ErrOrphanedAgentConfigsExist is returned when an agent is created with the name of a
	// previously deleted agent whose configuration rows survived, because revoking their LLM
	// proxy credentials failed. Configurations are keyed by agent name, so letting the create
	// through would silently hand the new agent the old agent's un-rotated credential. Maps to 409.
	ErrOrphanedAgentConfigsExist = errors.New("configurations from a previously deleted agent with this name have not been fully revoked")
	// ErrAgentConfigNotExternal is returned when a user-managed API key action
	// (create/rotate/revoke) is attempted against a configuration whose agent is
	// managed/internal. Only external agents own their proxy API keys; managed
	// agents have the platform inject them, so user-managed actions are rejected.
	ErrAgentConfigNotExternal = errors.New("API key management is only available for external agents")

	// Secret management errors
	ErrSecretPathConflict = errors.New("secret path is owned by another system")

	// Git secret errors
	ErrGitSecretNotFound      = errors.New("git secret not found")
	ErrGitSecretAlreadyExists = errors.New("git secret already exists")
	ErrGitSecretInvalidType   = errors.New("invalid git secret type")

	// Agent Kind errors
	ErrAgentKindNotFound         = errors.New("agent kind not found")
	ErrAgentKindAlreadyExists    = errors.New("agent kind already exists")
	ErrAgentKindHasInstances     = errors.New("agent kind cannot be deleted while agents are instantiated from it")
	ErrAgentIsKindSource         = errors.New("agent cannot be deleted while it is the source of an agent kind")
	ErrKindVersionNotFound       = errors.New("agent kind version not found")
	ErrKindVersionAlreadyExists  = errors.New("agent kind version already exists")
	ErrBuildNotComplete          = errors.New("build must be completed before publishing as a kind")
	ErrMissingKindConfigValue    = errors.New("missing required configuration value for agent kind")
	ErrKindImageAlreadyPublished = errors.New("this build image is already published as a kind version")
	ErrSourceAgentNotFound       = errors.New("source agent not found; cannot publish a kind from a deleted agent")
)
