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
	"errors"
	"fmt"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// ProjectAgentIdentityRequest identifies one project-scoped AgentID operation.
// ProjectID and ProjectName are separate because deployments may eventually map
// human-readable names to immutable identity-platform project IDs.
type ProjectAgentIdentityRequest struct {
	OrganizationID   string
	ProjectID        string
	ProjectName      string
	Environment      string
	AgentName        string
	ProvisioningType models.AgentProvisioningType
	ExistingAgentID  string
	ExistingClientID string
	ClientSecret     string
	// NeedsSecret asks Provision to return a client secret. It is set for
	// internal agents, whose secret Agent Manager stores, and also for external
	// agents: in CP_MANAGED mode the control plane needs a secret value on the
	// environment to apply the AgentID, so the provider returns one for them
	// too. For an external agent Agent Manager only hands it to Activate and then
	// discards it; it is never stored or logged. An external agent obtains a
	// usable secret only by regenerating it.
	NeedsSecret bool
}

type ProjectAgentIdentityResult struct {
	AgentID      string
	ClientID     string
	ClientSecret string
}

// ProjectIdentityEnvironment is one environment of a project's promotion
// topology. PromotesTo contains only valid outgoing promotion edges.
type ProjectIdentityEnvironment struct {
	Name       string
	PromotesTo []string
}

// ProjectAgentIdentityPromotionRequest identifies one AgentID promotion. The
// provider owns previewing the change and selecting only ExistingAgentID.
// TargetClientID and TargetClientSecret are chosen by AMS (see Promote).
// SourceEnvironment is always a direct parent of TargetEnvironment in the
// project's current pipeline, so a provider may only promote along that edge.
type ProjectAgentIdentityPromotionRequest struct {
	OrganizationID     string
	ProjectID          string
	ProjectName        string
	AgentName          string
	SourceEnvironment  string
	TargetEnvironment  string
	ProvisioningType   models.AgentProvisioningType
	ExistingAgentID    string
	ExistingClientID   string
	TargetClientID     string
	TargetClientSecret string
	NeedsSecret        bool
}

// ProjectAgentIdentityProvider is implemented by deployments that can create
// project-scoped AgentIDs through an identity control plane. The provider only
// executes operations for the environment named in the request; the project's
// environment order comes from ProjectPipelineReader, never from the provider.
//
// Provision creates the identity in request.Environment, which AMS always sets
// to the pipeline source environment. It must be idempotent for
// ExistingAgentID, must reject non-source environments, and must not persist
// the returned secret. Activate makes the identity effective in that same
// source environment and must be idempotent.
type ProjectAgentIdentityProvider interface {
	Provision(ctx context.Context, request ProjectAgentIdentityRequest) (ProjectAgentIdentityResult, error)
	Activate(ctx context.Context, request ProjectAgentIdentityRequest, agentID string) error
}

// ProjectAgentIdentityPromotionProvider extends source provisioning with
// AgentID-only promotion between environments.
type ProjectAgentIdentityPromotionProvider interface {
	ProjectAgentIdentityProvider

	// Promote carries ONLY the AgentID (request.ExistingAgentID) from
	// request.SourceEnvironment to request.TargetEnvironment. The source is
	// always a direct parent of the target in the project's current pipeline;
	// the provider may promote only along that edge, and returns
	// ErrProjectAgentIdentityPromotionPathUnavailable (wrapped is fine) when its
	// identity service has no such path yet, so AMS waits and asks again instead
	// of failing the binding. It must be idempotent.
	//
	// Contract for the target credential: the provider MUST set
	// request.TargetClientID and request.TargetClientSecret on the target
	// environment unconditionally (overwrite, idempotent), whether or not the
	// environment reports them missing. AMS generates a fresh secret on every
	// attempt and stores exactly the secret Promote returns, so a value the
	// provider keeps because it already existed would diverge from the stored
	// one after any retry. The result must echo the client ID and secret that
	// were set. For external agents the secret is set the same way but AMS never
	// stores it; it can only be obtained by regenerating it.
	Promote(ctx context.Context, request ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error)
}

// ProjectAgentIdentitySecretRequest names one AgentID credential in one environment.
type ProjectAgentIdentitySecretRequest struct {
	OrganizationID string
	ProjectID      string
	ProjectName    string
	Environment    string
	AgentName      string
	AgentID        string
	ClientID       string
	// ClientSecret is chosen by AMS. It is a secret: implementations must never
	// log it or include it in an error.
	ClientSecret string
}

// ProjectAgentIdentitySecretProvider is an optional capability of a
// ProjectAgentIdentityProvider: it replaces the client secret of an existing
// AgentID in one environment. AMS owns the secret value (like Promote), so
// the identity control plane never generates it.
type ProjectAgentIdentitySecretProvider interface {
	// SetSecret makes ClientSecret the only valid secret of the AgentID in request.Environment.
	// MUST set the value unconditionally (overwrite, idempotent) and return only after the
	// environment's identity service accepts it; a retry with the same request must succeed.
	SetSecret(ctx context.Context, request ProjectAgentIdentitySecretRequest) error
}

// ProjectAgentIdentityDeleteRequest names one AgentID to remove from one environment.
type ProjectAgentIdentityDeleteRequest struct {
	OrganizationID string
	ProjectID      string
	ProjectName    string
	Environment    string
	AgentName      string
	AgentID        string
	ClientID       string
	// Source reports that Environment is the project's pipeline source, where the control plane holds the AgentID.
	Source bool
}

// ProjectAgentIdentityDeletionProvider is a capability a ProjectAgentIdentityProvider
// must have in CP_MANAGED mode (SetProjectAgentIdentityProvider rejects one without
// it): it removes an AgentID from one environment.
type ProjectAgentIdentityDeletionProvider interface {
	// DeleteAgentIdentity removes request.AgentID from request.Environment. It is called for the
	// source environment first (the AgentID leaves the control plane configuration), then for each
	// promoted environment. MUST be idempotent: an AgentID that is already gone is success. It
	// returns only after the environment's identity service has accepted the removal.
	DeleteAgentIdentity(ctx context.Context, request ProjectAgentIdentityDeleteRequest) error
}

// errProjectAgentIdentitySecretUnavailable is returned by secret rotation and
// revocation in CP_MANAGED mode when the configured provider cannot set a
// secret. It fails closed: the legacy per-environment Thunder client is never
// used for a CP-managed environment.
var errProjectAgentIdentitySecretUnavailable = errors.New("project AgentID secret provider is not configured")

// ErrProjectAgentIdentityRequiresPromotion tells callers not to create another
// identity directly in a non-source environment.
var ErrProjectAgentIdentityRequiresPromotion = fmt.Errorf(
	"%w: project AgentID must be promoted to this environment", utils.ErrConflict,
)

// ErrProjectAgentIdentitySourceUnavailable marks a promotion target whose source
// environment binding is missing or FAILED, so waiting for it cannot succeed
// until someone fixes the source.
var ErrProjectAgentIdentitySourceUnavailable = errors.New(
	"project AgentID source environment identity is missing or failed",
)

// ErrProjectAgentIdentityEnvironmentNotInPipeline marks a promotion target that
// is no longer an outgoing target of any environment in the project's current
// pipeline, so there is nothing to promote it from.
var ErrProjectAgentIdentityEnvironmentNotInPipeline = errors.New(
	"project AgentID environment is no longer part of the project pipeline",
)

// ErrProjectAgentIdentityPromotionPathUnavailable is returned (wrapped is fine)
// by a provider's Promote when the identity service has no promotion path for
// the requested source to target edge yet. It is a wait, not a failure: AMS
// defers the attempt and tries again later without consuming its retry budget.
var ErrProjectAgentIdentityPromotionPathUnavailable = errors.New(
	"project AgentID promotion path is not available yet",
)

// ProjectAgentIdentityProviderSetter is an optional deployment hook. The
// open-source runtime defaults to LEGACY_DIRECT and needs no provider.
type ProjectAgentIdentityProviderSetter interface {
	SetProjectAgentIdentityProvider(mode thundersvc.IdentityManagementMode, provider ProjectAgentIdentityProvider) error
}

// IdentityManagementModeProvider is an optional capability of an
// AgentThunderProvisioningService: it reports whether AgentIDs are managed by
// the identity control plane. Callers type-assert for it, so an implementation
// without project AgentID support is simply treated as LEGACY_DIRECT.
type IdentityManagementModeProvider interface {
	IdentityManagementMode() thundersvc.IdentityManagementMode
}

// IdentityManagementMode implements IdentityManagementModeProvider. The mode is
// set once at startup by SetProjectAgentIdentityProvider.
func (s *agentThunderProvisioningService) IdentityManagementMode() thundersvc.IdentityManagementMode {
	return s.identityManagementMode
}

func (s *agentThunderProvisioningService) SetProjectAgentIdentityProvider(
	mode thundersvc.IdentityManagementMode, provider ProjectAgentIdentityProvider,
) error {
	switch mode {
	case "", thundersvc.IdentityManagementLegacyDirect:
		s.identityManagementMode = thundersvc.IdentityManagementLegacyDirect
		s.projectIdentityProvider = nil
		s.projectIdentitySecretProvider = nil
		s.projectIdentityDeletionProvider = nil
		return nil
	case thundersvc.IdentityManagementCPManaged:
		if provider == nil {
			return errors.New("CP_MANAGED identity mode requires a project AgentID provider")
		}
		// Required capability: without it deleting an agent would leave its
		// AgentIDs in the identity control plane, so refuse to start half managed.
		deletionProvider, ok := provider.(ProjectAgentIdentityDeletionProvider)
		if !ok {
			return errors.New("CP_MANAGED identity mode requires a project AgentID provider that implements ProjectAgentIdentityDeletionProvider")
		}
		s.identityManagementMode = mode
		s.projectIdentityProvider = provider
		s.projectIdentityDeletionProvider = deletionProvider
		// Optional capability: rotation and revocation fail closed without it.
		secretProvider, _ := provider.(ProjectAgentIdentitySecretProvider)
		s.projectIdentitySecretProvider = secretProvider
		return nil
	default:
		return errors.New("project AgentID provisioning supports only LEGACY_DIRECT or CP_MANAGED mode")
	}
}

var _ ProjectAgentIdentityProviderSetter = (*agentThunderProvisioningService)(nil)
