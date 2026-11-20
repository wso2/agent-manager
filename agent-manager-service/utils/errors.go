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

import "errors"

var (
	// Resource not found errors
	ErrProjectNotFound             = errors.New("project not found")
	ErrAgentAlreadyExists          = errors.New("agent already exists")
	ErrAgentNotFound               = errors.New("agent not found")
	ErrOrganizationNotFound        = errors.New("organization not found")
	ErrEnvironmentNotFound         = errors.New("environment not found")
	ErrAgentIdentityNotProvisioned = errors.New("agent identity not yet provisioned for this environment")
	// ErrAgentIdentityRetryNotAllowed is returned when a retry is requested for
	// a binding whose current status is not failed. Maps to 409.
	ErrAgentIdentityRetryNotAllowed = errors.New("agent identity binding is not in a failed state and cannot be retried")
	ErrOrganizationAlreadyExists    = errors.New("organization already exists")
	ErrProjectAlreadyExists         = errors.New("project already exists")
	ErrProjectHasAssociatedAgents   = errors.New("project has associated agents")
	ErrEventDestinationNotFound     = errors.New("event destination not found")
	ErrEventDestinationInUse        = errors.New("event destination is used by one or more subscriptions")
	ErrEventDestinationNameTaken    = errors.New("an event destination with this name already exists")
	ErrEventSubscriptionNotFound    = errors.New("event subscription not found")
	ErrImmutableFieldChange         = errors.New("cannot change immutable field")

	// ErrResourceBeingDeleted is returned when a create collides with a same-named
	// resource that has been deleted but is still waiting on its cleanup finalizer.
	// It deliberately does not wrap ErrConflict: callers that recover from a conflict
	// by updating the existing object must not do so against one that is going away.
	ErrResourceBeingDeleted = errors.New("a resource with this name is still being deleted")

	ErrInvalidAdapterType   = errors.New("invalid adapter type")
	ErrGatewayUnreachable   = errors.New("gateway unreachable")
	ErrInvalidGatewayConfig = errors.New("invalid gateway configuration")

	// ErrEnvironmentHasNoGateway is returned when an API agent would be created, built, deployed or
	// promoted into an environment with no ingress gateway. Maps to 409.
	ErrEnvironmentHasNoGateway = errors.New("environment has no ingress gateway")

	// LLM Provider-related errors
	ErrProviderNotFound       = errors.New("provider not found")
	ErrProviderAlreadyExists  = errors.New("provider already exists")
	ErrProviderHasDeployments = errors.New("provider has active deployments")
	ErrDeploymentFailed       = errors.New("deployment failed")
	ErrPolicyNotSupported     = errors.New("policy not supported by gateway")
	ErrInvalidProviderConfig  = errors.New("invalid provider configuration")

	// API Platform integration errors
	ErrHandleExists             = errors.New("handle already exists")
	ErrGatewayHasAssociatedAPIs = errors.New("gateway has associated APIs")
	ErrDevPortalNotFound        = errors.New("devportal not found")
	ErrAPIAlreadyPublished      = errors.New("api is already published to devportal")
	ErrAPIPublicationNotFound   = errors.New("api publication not found")

	// ErrOrphanedAgentConfigsExist is returned when an agent is created with the name of a
	// previously deleted agent whose configuration rows survived, because revoking their LLM
	// proxy credentials failed. Configurations are keyed by agent name, so letting the create
	// through would silently hand the new agent the old agent's un-rotated credential. Maps to 409.
	ErrOrphanedAgentConfigsExist = errors.New("configurations from a previously deleted agent with this name have not been fully revoked")

	ErrSecretPathConflict = errors.New("secret path is owned by another system")
)

// Project-scoped MCP proxy errors.
var (
	// ErrMCPProxyCreateNotPermitted means the caller may update project proxies but not
	// create one, and the upsert would have created it (403 on the project routes).
	ErrMCPProxyCreateNotPermitted = errors.New("not permitted to create MCP proxies")
	// ErrMCPUpstreamNotReady means the proxy's source component has no reachable endpoint in the
	// environment yet (409 UPSTREAM_NOT_READY); nothing was stored.
	ErrMCPUpstreamNotReady = errors.New("the source component is not deployed in this environment yet")
	// ErrMCPProxyEnvironmentNotBound means the proxy has no binding in the environment (404).
	ErrMCPProxyEnvironmentNotBound = errors.New("MCP proxy is not bound to this environment")
	// ErrMCPJWTSecurityNotAvailable is the phase-1 refusal of jwt security (400).
	ErrMCPJWTSecurityNotAvailable = errors.New("jwt security is not available yet")
	// ErrMCPAmbiguousGateway means the binding's environment has more than one egress gateway and the
	// request named none (400 AMBIGUOUS_GATEWAY on the project binding routes).
	ErrMCPAmbiguousGateway = errors.New("the environment has more than one egress gateway; specify gatewayId")
)
