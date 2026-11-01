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
	"log/slog"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// ErrIdentityPromotionNotApplicable is returned by an IdentityPromotionManager
// when the two environments are not both managed through the control plane.
var ErrIdentityPromotionNotApplicable = errors.New("identity promotion does not apply to these environments")

// MCPResourceServerPromotion is one MCP resource server the agent's
// OAuth-secured connections need in the target environment.
type MCPResourceServerPromotion struct {
	ProxyHandle string
	DisplayName string
	// Identifier is the URL the proxy is served at in the TARGET environment. The
	// resource server's identifier is environment specific, so the target must be
	// given this value instead of the source's.
	Identifier string
}

// AgentIdentityPromotion describes one AgentID to carry from a source
// environment into a target environment.
type AgentIdentityPromotion struct {
	OUID              string
	OrgNamespace      string
	SourceEnvironment string
	TargetEnvironment string
	// ThunderAgentID identifies the carried agent identity.
	ThunderAgentID string
	// TargetClientID is the target environment's own client ID for the carried
	// identity. It is generated for the target and never the source's value.
	TargetClientID string
	// ClientSecret is the target-local secret to set. Empty keeps the secret the
	// target already holds, which is what a repeated promotion and a revoked
	// credential both need. It is never the source environment's secret.
	ClientSecret string
	// MCPResourceServers are carried with their target identifiers.
	MCPResourceServers []MCPResourceServerPromotion
	// ProjectName is the OpenChoreo project the agent belongs to. Promotion calls
	// to the identity platform are project scoped.
	ProjectName string
	// Roles and Groups are the agent's roles and groups, read from the default
	// environment and carried by id and name. Role assignments and group members
	// are carried verbatim by id, so they stay valid in the target.
	Roles  []PromotedIdentityResource
	Groups []PromotedIdentityResource
}

const redactedSecret = "[REDACTED]"

// String keeps ClientSecret out of every formatted rendering of the promotion.
// It also leaves out the agent and client identifiers: they identify internal
// resources and have no place in a log line.
func (p AgentIdentityPromotion) String() string {
	return fmt.Sprintf("AgentIdentityPromotion{project=%s source=%s target=%s secret=%s roles=%d groups=%d mcp=%d}",
		p.ProjectName, p.SourceEnvironment, p.TargetEnvironment,
		secretPresence(p.ClientSecret), len(p.Roles), len(p.Groups), len(p.MCPResourceServers))
}

// GoString covers the %#v rendering.
func (p AgentIdentityPromotion) GoString() string { return p.String() }

// LogValue keeps ClientSecret and the identifiers out of structured logs.
func (p AgentIdentityPromotion) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("project", p.ProjectName),
		slog.String("sourceEnvironment", p.SourceEnvironment),
		slog.String("targetEnvironment", p.TargetEnvironment),
		slog.String("clientSecret", secretPresence(p.ClientSecret)),
		slog.Int("roles", len(p.Roles)),
		slog.Int("groups", len(p.Groups)),
		slog.Int("mcpResourceServers", len(p.MCPResourceServers)),
	)
}

func secretPresence(secret string) string {
	if secret == "" {
		return "<empty>"
	}
	return redactedSecret
}

// IdentityPromotionManager carries an AgentID between environments through the
// control plane. Implementations own the deployment's promotion protocol: they
// must carry the agent application, the listed resource servers, roles and
// groups (no users), set the target client ID, the secret (when given) and each
// resource server identifier as the target's own values before applying, and
// refuse the promotion while any value the carried resources reference is
// missing. A refusal is an *IdentityPromotionError so the caller can answer
// precisely. Promoting again must be safe.
//
// Implementations apply with an explicit selection naming only the carried
// agent, resource servers, roles and groups. An omitted selection means the
// target gateway's remembered choice, so it would carry or hold back other
// changes and must never be sent.
type IdentityPromotionManager interface {
	PromoteAgentIdentity(ctx context.Context, promotion AgentIdentityPromotion) error
}

// IdentityPromotionPreviewer is an optional capability of an
// IdentityPromotionManager. PreviewAgentIdentity checks, without writing
// anything, whether the promotion would be refused, and returns an
// *IdentityPromotionError when it would. It is called before any client ID or
// secret is generated, before the audit intent is written and before any write.
// The promotion it receives has no client ID or secret yet.
type IdentityPromotionPreviewer interface {
	PreviewAgentIdentity(ctx context.Context, promotion AgentIdentityPromotion) error
}

// AgentSecretRotation asks for a new client secret on an environment that
// receives its identity by promotion.
type AgentSecretRotation struct {
	OUID           string
	OrgNamespace   string
	Environment    string
	ThunderAgentID string
	// ClientSecret is the new secret the environment's gateway must hold.
	ClientSecret string
	// ProjectName is the OpenChoreo project the agent belongs to.
	ProjectName string
}

// String keeps ClientSecret out of every formatted rendering of the rotation.
func (r AgentSecretRotation) String() string {
	return fmt.Sprintf("AgentSecretRotation{project=%s environment=%s secret=%s}",
		r.ProjectName, r.Environment, secretPresence(r.ClientSecret))
}

// GoString covers the %#v rendering.
func (r AgentSecretRotation) GoString() string { return r.String() }

// LogValue keeps ClientSecret out of structured logs.
func (r AgentSecretRotation) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("project", r.ProjectName),
		slog.String("environment", r.Environment),
		slog.String("clientSecret", secretPresence(r.ClientSecret)),
	)
}

// AgentSecretRotator is an optional capability of an IdentityPromotionManager.
// It sets the secret on the environment's gateway and applies it, so the new
// value takes effect. Rotation, regeneration and revocation all use it.
type AgentSecretRotator interface {
	RotateAgentSecret(ctx context.Context, rotation AgentSecretRotation) error
}

// IdentityPromotionManagerSetter lets app.Run hand the deployment's promotion
// manager to the DB-backed provisioning service.
type IdentityPromotionManagerSetter interface {
	SetIdentityPromotionManager(IdentityPromotionManager)
}

// AgentMCPResourceServerResolver lists the MCP resource servers an agent needs
// in an environment, each with the identifier it serves under there.
type AgentMCPResourceServerResolver interface {
	ResolveAgentMCPResourceServers(ctx context.Context, ouID, projectName, agentName, envName string) ([]MCPResourceServerPromotion, error)
}

// AgentMCPResourceServerResolverSetter backfills the resolver once the MCP
// services exist; the provisioning service is built before them.
type AgentMCPResourceServerResolverSetter interface {
	SetAgentMCPResourceServerResolver(AgentMCPResourceServerResolver)
}

// AgentIdentityPromoter is implemented by a provisioning service that knows which
// environments receive their AgentID by promotion.
type AgentIdentityPromoter interface {
	// IdentityCreatedByPromotion reports whether envName receives its AgentID
	// through promotion instead of at agent creation.
	IdentityCreatedByPromotion(ctx context.Context, ouID, envName string) (bool, error)
}

// AgentIdentityPromotionRequest is one promotion of an agent identity, for
// internal and external agents alike.
type AgentIdentityPromotionRequest struct {
	OUID        string
	ProjectName string
	AgentName   string
	// DefaultEnvironment is the pipeline's lowest environment, where roles and
	// groups are read. Empty means SourceEnvironment.
	DefaultEnvironment string
	SourceEnvironment  string
	TargetEnvironment  string
	RequestedBy        string
	// Ownership decides how the target binding is recorded. An external agent's
	// secret is never stored: the target binding has no secret reference.
	Ownership models.AgentProvisioningType
	// Background marks a promotion run by the reconciler with no request behind
	// it. The audit record then names this service as the actor and RequestedBy as
	// the user it acts for, the way AttemptProvision records its own.
	Background bool
	// Trigger says what started the promotion, for the audit record: the first
	// promotion of a new binding or a re-promotion of changed identity resources.
	Trigger string
}

// AgentIdentityPromotionResult reports one promotion.
type AgentIdentityPromotionResult struct {
	// Promoted is true when the identity was carried.
	Promoted bool
	// NotApplicable is true when the target does not receive its identity by
	// promotion; nothing was resolved, previewed or written.
	NotApplicable bool
	// Resources is what the promotion carries.
	Resources AgentIdentityResources
	// HeldBackRoles counts the carried roles left out because another agent that
	// holds them has no completed identity in the target environment yet. They are
	// promoted by a later re-promotion, once those agents have one.
	HeldBackRoles int
}

// AgentIdentityPromotionRunner carries an agent identity, for internal and
// external agents, and returns what was carried.
type AgentIdentityPromotionRunner interface {
	PromoteIdentityResources(ctx context.Context, req AgentIdentityPromotionRequest) (AgentIdentityPromotionResult, error)
}

var (
	_ AgentIdentityPromotionRunner         = (*agentThunderProvisioningService)(nil)
	_ IdentityPromotionManagerSetter       = (*agentThunderProvisioningService)(nil)
	_ AgentMCPResourceServerResolverSetter = (*agentThunderProvisioningService)(nil)
	_ AgentIdentityResourceResolverSetter  = (*agentThunderProvisioningService)(nil)
	_ AgentIdentityPromoter                = (*agentThunderProvisioningService)(nil)
)

func (s *agentThunderProvisioningService) SetIdentityPromotionManager(manager IdentityPromotionManager) {
	s.promotionMu.Lock()
	s.promotionManager = manager
	s.promotionMu.Unlock()
}

func (s *agentThunderProvisioningService) SetAgentMCPResourceServerResolver(resolver AgentMCPResourceServerResolver) {
	s.promotionMu.Lock()
	s.mcpResolver = resolver
	s.promotionMu.Unlock()
}

func (s *agentThunderProvisioningService) SetAgentIdentityResourceResolver(resolver AgentIdentityResourceResolver) {
	s.promotionMu.Lock()
	s.resourceResolver = resolver
	s.promotionMu.Unlock()
}

func (s *agentThunderProvisioningService) promotionDependencies() (IdentityPromotionManager, AgentMCPResourceServerResolver, AgentIdentityResourceResolver) {
	s.promotionMu.RLock()
	defer s.promotionMu.RUnlock()
	return s.promotionManager, s.mcpResolver, s.resourceResolver
}

func (s *agentThunderProvisioningService) IdentityCreatedByPromotion(ctx context.Context, ouID, envName string) (bool, error) {
	placement, ok := s.envResolver.(thundersvc.EnvironmentIdentityPlacement)
	if !ok {
		return false, nil
	}
	return placement.IdentityCreatedByPromotion(ctx, ouID, ThunderOrgNamespace(), envName)
}

// PromoteIdentityResources carries the agent application, its MCP resource
// servers, roles and groups into the target environment. Roles and groups are
// read from the default environment, the only environment where they are
// created; the target gets only its own client ID, client secret and resource
// server identifiers. The order is: preview, resolve resources, issue
// credentials, audit intent, carry, store. Nothing is written before the preview
// and the resource lookups pass, and a dry run stops right after them.
//
// An external agent's secret is generated only because the carried client
// needs a value; it is handed to the manager and discarded, never stored.
func (s *agentThunderProvisioningService) PromoteIdentityResources(ctx context.Context, req AgentIdentityPromotionRequest,
) (AgentIdentityPromotionResult, error) {
	ouID, projectName, agentName := req.OUID, req.ProjectName, req.AgentName
	defaultEnv, sourceEnv, targetEnv := req.DefaultEnvironment, req.SourceEnvironment, req.TargetEnvironment
	notApplicable := AgentIdentityPromotionResult{NotApplicable: true}

	byPromotion, err := s.IdentityCreatedByPromotion(ctx, ouID, targetEnv)
	if err != nil {
		return AgentIdentityPromotionResult{}, fmt.Errorf("resolve identity placement for %s: %w", targetEnv, err)
	}
	if !byPromotion {
		return notApplicable, nil
	}
	manager, mcpResolver, resourceResolver := s.promotionDependencies()
	if manager == nil {
		return AgentIdentityPromotionResult{}, fmt.Errorf("%w: no identity promotion manager is configured for environment %s",
			thundersvc.ErrIdentityCapabilityUnavailable, targetEnv)
	}
	if defaultEnv == "" {
		defaultEnv = sourceEnv
	}
	// Roles and groups are read from the default environment, so it must be
	// where identity is managed. Otherwise nothing here can be carried.
	if homePromoted, err := s.IdentityCreatedByPromotion(ctx, ouID, defaultEnv); err != nil {
		return AgentIdentityPromotionResult{}, fmt.Errorf("resolve identity placement for %s: %w", defaultEnv, err)
	} else if homePromoted {
		return AgentIdentityPromotionResult{}, identityHomeMismatchError(thundersvc.ErrIdentityEnvironmentUnsupported)
	}

	// Held while the identity is carried, so a promotion, a secret rotation and a
	// revoke of the same binding never interleave.
	release, err := s.bindingLocks.Lock(ctx, bindingLockKey(ouID, projectName, agentName, targetEnv))
	if err != nil {
		return AgentIdentityPromotionResult{}, err
	}
	defer release()

	source, err := s.repo.Get(ctx, ouID, projectName, agentName, sourceEnv)
	if err != nil {
		if errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
			return AgentIdentityPromotionResult{}, fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, sourceEnv)
		}
		return AgentIdentityPromotionResult{}, fmt.Errorf("read source agent identity: %w", err)
	}
	if source.Status != models.AgentThunderStatusCompleted || source.ThunderAgentID == "" {
		return AgentIdentityPromotionResult{}, fmt.Errorf("%w: %s in %s is %s", utils.ErrAgentIdentityNotProvisioned, agentName, sourceEnv, source.Status)
	}
	existing, err := s.existingTargetBinding(ctx, ouID, projectName, agentName, targetEnv)
	if err != nil {
		return AgentIdentityPromotionResult{}, err
	}
	// A target that already holds a completed identity keeps its client ID and
	// secret: a repeated promotion must not rotate the credential of an agent
	// that may be running, and a revoked credential must stay revoked.
	firstTime := existing == nil || existing.Status != models.AgentThunderStatusCompleted || existing.ThunderClientID == ""

	var servers []MCPResourceServerPromotion
	if mcpResolver != nil {
		if servers, err = mcpResolver.ResolveAgentMCPResourceServers(ctx, ouID, projectName, agentName, targetEnv); err != nil {
			return AgentIdentityPromotionResult{}, fmt.Errorf("resolve MCP resource servers for %s: %w", targetEnv, err)
		}
	}
	var resources AgentIdentityResources
	if resourceResolver != nil {
		resources, err = resourceResolver.ResolveAgentIdentityResources(ctx, AgentIdentityResourceQuery{
			OUID: ouID, ProjectName: projectName, AgentName: agentName, ThunderAgentID: source.ThunderAgentID,
			DefaultEnvironment: defaultEnv, TargetEnvironment: targetEnv,
		})
		if err != nil {
			return AgentIdentityPromotionResult{}, fmt.Errorf("resolve agent identity resources for %s: %w", targetEnv, err)
		}
	}
	// A role is carried only when every agent that holds it already has an
	// identity in the target. The others wait for a later re-promotion.
	var heldBack int
	if resources.Roles, heldBack, err = s.holdBackRoles(ctx, ouID, defaultEnv, targetEnv, source.ThunderAgentID, resources.Roles); err != nil {
		return AgentIdentityPromotionResult{}, err
	}
	promotion := AgentIdentityPromotion{
		OUID: ouID, OrgNamespace: ThunderOrgNamespace(), SourceEnvironment: sourceEnv, TargetEnvironment: targetEnv,
		ThunderAgentID: source.ThunderAgentID, MCPResourceServers: mergeMCPResourceServers(servers, resources.MCPResourceServers),
		ProjectName: projectName, Roles: resources.Roles, Groups: resources.Groups,
	}
	carried := AgentIdentityResources{
		Roles: promotion.Roles, Groups: promotion.Groups, MCPResourceServers: promotion.MCPResourceServers,
	}

	// Every MCP resource server the agent carries must exist in the home
	// environment before anything is audited or issued.
	if err := s.preflightMCPResourceServers(ctx, mcpResolver, resourceResolver, promotion, defaultEnv); err != nil {
		s.logPromotionRefusal(ctx, "preflight", promotion, err)
		return AgentIdentityPromotionResult{}, err
	}

	// The preview runs before any credential exists, so a refused promotion
	// issues, records and stores nothing.
	if previewer, ok := manager.(IdentityPromotionPreviewer); ok {
		if err := previewer.PreviewAgentIdentity(ctx, promotion); err != nil {
			if errors.Is(err, ErrIdentityPromotionNotApplicable) {
				return notApplicable, nil
			}
			s.logPromotionRefusal(ctx, "preview", promotion, err)
			return AgentIdentityPromotionResult{}, fmt.Errorf("check promotion of the agent identity from %s to %s: %w", sourceEnv, targetEnv, err)
		}
	}
	if firstTime {
		if promotion.TargetClientID, err = thundersvc.GenerateClientID(); err != nil {
			return AgentIdentityPromotionResult{}, fmt.Errorf("generate target client ID: %w", err)
		}
		if promotion.ClientSecret, err = thundersvc.GenerateClientSecret(); err != nil {
			return AgentIdentityPromotionResult{}, fmt.Errorf("generate target client secret: %w", err)
		}
	} else {
		promotion.TargetClientID = existing.ThunderClientID
	}

	// Issuing the target credential is recorded before it exists, and refused if
	// it cannot be recorded.
	auditOpts := []audit.Option{
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceAgentIdentity, agentName, agentName),
		audit.Project(projectName),
		audit.Environment(targetEnv),
		audit.Detail("agentName", agentName),
		audit.Detail("environment", targetEnv),
		audit.Detail("sourceEnvironment", sourceEnv),
		audit.Detail("provisioningType", string(req.Ownership)),
		audit.Detail("clientId", promotion.TargetClientID),
		audit.Detail("roleCount", len(promotion.Roles)),
		audit.Detail("groupCount", len(promotion.Groups)),
		audit.Detail("resourceServerCount", len(promotion.MCPResourceServers)),
	}
	if req.Trigger != "" {
		auditOpts = append(auditOpts, audit.Detail("trigger", req.Trigger))
	}
	if req.Background {
		// No request is behind this credential: the actor is this service and the
		// user who asked for the agent is carried as OnBehalfOf.
		auditOpts = append(
			auditOpts,
			audit.Actor(audit.ActorSystem, systemActorAgentThunderReconciler, ""),
			audit.SurfaceOpt(audit.SurfaceSystem),
			audit.OnBehalfOf(req.RequestedBy),
		)
	}
	attempt, err := audit.Begin(ctx, audit.ActionAgentIdentityPromote, auditOpts...)
	if err != nil {
		return AgentIdentityPromotionResult{}, err
	}
	promoted, err := s.carryIdentity(ctx, manager, promotion, existing, firstTime, req)
	// Success is recorded only when the identity was carried. A manager that
	// reports the environments as not applicable carried nothing and issued
	// nothing, so that is recorded as a failure with its reason, not as success.
	attempt.Complete(ctx, err, audit.Detail("carried", promoted))
	if errors.Is(err, ErrIdentityPromotionNotApplicable) {
		return notApplicable, nil
	}
	if err != nil {
		s.logPromotionRefusal(ctx, "promote", promotion, err)
		return AgentIdentityPromotionResult{}, err
	}
	return AgentIdentityPromotionResult{Promoted: promoted, Resources: carried, HeldBackRoles: heldBack}, nil
}

// logPromotionRefusal logs a failed promotion step with its full cause. The
// promotion is logged through its redacting LogValue.
func (s *agentThunderProvisioningService) logPromotionRefusal(ctx context.Context, step string, promotion AgentIdentityPromotion, err error) {
	var typed *IdentityPromotionError
	kind := ""
	if errors.As(err, &typed) {
		kind = string(typed.Kind)
	}
	s.logger.ErrorContext(ctx, "Agent identity promotion failed", "step", step, "kind", kind,
		"promotion", promotion, "error", err)
}

func (s *agentThunderProvisioningService) existingTargetBinding(ctx context.Context, ouID, projectName, agentName,
	targetEnv string,
) (*models.AgentThunderClient, error) {
	existing, err := s.repo.Get(ctx, ouID, projectName, agentName, targetEnv)
	switch {
	case errors.Is(err, repositories.ErrAgentThunderClientNotFound):
		return nil, nil //nolint:nilnil // documented: nil means no binding yet
	case err != nil:
		return nil, fmt.Errorf("read target agent identity: %w", err)
	}
	return existing, nil
}

// carryIdentity has the manager set the target's values while carrying the
// identity, and only then stores the new secret: a failed promotion leaves
// nothing stored that the target does not hold, and a failed store is repaired
// by the next promotion (the binding is not yet completed). An external agent's
// secret is not stored at all; the user regenerates one when they need it.
func (s *agentThunderProvisioningService) carryIdentity(ctx context.Context, manager IdentityPromotionManager,
	promotion AgentIdentityPromotion, existing *models.AgentThunderClient, firstTime bool, req AgentIdentityPromotionRequest,
) (bool, error) {
	err := manager.PromoteAgentIdentity(ctx, promotion)
	if errors.Is(err, ErrIdentityPromotionNotApplicable) {
		return false, err
	}
	if err != nil {
		return false, fmt.Errorf("promote agent identity from %s to %s: %w",
			promotion.SourceEnvironment, promotion.TargetEnvironment, err)
	}
	if !firstTime {
		return true, nil
	}
	var secretRef string
	if req.Ownership != models.AgentProvisioningTypeExternal {
		secretRef, err = s.upsertCredential(ctx, promotion.OUID, req.ProjectName, req.AgentName, promotion.TargetEnvironment,
			promotion.TargetClientID, promotion.ClientSecret)
		if err != nil {
			return false, fmt.Errorf("store promoted agent secret: %w", err)
		}
	}
	if err := s.recordPromotedBinding(ctx, promotion, existing, req, secretRef); err != nil {
		return false, err
	}
	return true, nil
}

// recordPromotedBinding marks the target binding completed with the carried
// identity and the target's own client ID, creating the binding when promotion
// is the first thing to touch the target.
func (s *agentThunderProvisioningService) recordPromotedBinding(ctx context.Context, promotion AgentIdentityPromotion,
	existing *models.AgentThunderClient, req AgentIdentityPromotionRequest, secretRef string,
) error {
	if existing == nil {
		ownership := req.Ownership
		if ownership == "" {
			ownership = models.AgentProvisioningTypeInternal
		}
		binding := models.AgentThunderClient{
			OUID: promotion.OUID, ProjectName: req.ProjectName, AgentName: req.AgentName, EnvironmentName: promotion.TargetEnvironment,
			ProvisioningType: ownership, ThunderAgentID: promotion.ThunderAgentID,
			ThunderClientID: promotion.TargetClientID, SecretRefPath: secretRef,
			Status: models.AgentThunderStatusCompleted, RequestedBy: req.RequestedBy,
			// A binding created by a promotion is a promoted one, never mistaken for a
			// binding created directly in its environment.
			PromotionSource: promotion.SourceEnvironment,
		}
		if err := s.repo.Upsert(ctx, &binding); err != nil {
			return fmt.Errorf("record promoted agent identity: %w", err)
		}
		return nil
	}
	if secretRef == "" {
		secretRef = existing.SecretRefPath
	}
	if err := s.repo.UpdateAfterAttempt(ctx, existing.ID, repositories.AgentThunderAttemptUpdate{
		Status: models.AgentThunderStatusCompleted, ThunderAgentID: &promotion.ThunderAgentID,
		ThunderClientID: &promotion.TargetClientID, SecretRefPath: &secretRef,
	}); err != nil {
		return fmt.Errorf("update promoted agent identity: %w", err)
	}
	return nil
}

// rotatePromotedSecret gives the environment's gateway a new secret for the
// agent and returns it. The caller stores it for an internal agent and only
// returns it to the user for an external one.
func (s *agentThunderProvisioningService) rotatePromotedSecret(ctx context.Context, projectName string,
	binding *models.AgentThunderClient,
) (string, error) {
	manager, _, _ := s.promotionDependencies()
	rotator, ok := manager.(AgentSecretRotator)
	if !ok {
		return "", fmt.Errorf("%w: secret rotation is not available for environment %s",
			thundersvc.ErrIdentityCapabilityUnavailable, binding.EnvironmentName)
	}
	secret, err := thundersvc.GenerateClientSecret()
	if err != nil {
		return "", fmt.Errorf("generate client secret: %w", err)
	}
	if err := rotator.RotateAgentSecret(ctx, AgentSecretRotation{
		OUID: binding.OUID, OrgNamespace: ThunderOrgNamespace(), Environment: binding.EnvironmentName,
		ThunderAgentID: binding.ThunderAgentID, ClientSecret: secret, ProjectName: projectName,
	}); err != nil {
		return "", fmt.Errorf("rotate agent secret in %s: %w", binding.EnvironmentName, err)
	}
	return secret, nil
}

// upsertCredential stores the credential, replacing one that is already stored.
// It patches first: the SaaS secret API creates a new secret on every push, so
// pushing again would leave the previous one behind, still holding the old
// secret. A secret that does not exist yet is created.
func (s *agentThunderProvisioningService) upsertCredential(ctx context.Context, ouID, projectName, agentName, envName,
	clientID, clientSecret string,
) (string, error) {
	location := agentIdentitySecretLocation(ouID, projectName, agentName, envName)
	ref, err := s.secretMgmtClient.PatchSecret(ctx, location, map[string]string{
		thundersvc.AgentSecretKeyClientID:     clientID,
		thundersvc.AgentSecretKeyClientSecret: clientSecret,
	}, nil)
	if err == nil {
		return ref, nil
	}
	if !errors.Is(err, secretmanagersvc.ErrSecretNotFound) {
		return "", err
	}
	return s.storeCredential(ctx, ouID, projectName, agentName, envName, clientID, clientSecret)
}

// preflightMCPResourceServers makes sure every MCP resource server the promotion
// carries exists in the home environment (see AgentMCPResourceServerEnsurer).
// Without the capability nothing is checked. The promotion is refused with a typed
// Blocked error when an MCP server is not deployed in the home environment.
func (s *agentThunderProvisioningService) preflightMCPResourceServers(ctx context.Context, mcpResolver AgentMCPResourceServerResolver,
	resourceResolver AgentIdentityResourceResolver, promotion AgentIdentityPromotion, homeEnv string,
) error {
	ensurer, ok := mcpResolver.(AgentMCPResourceServerEnsurer)
	if !ok {
		ensurer, ok = resourceResolver.(AgentMCPResourceServerEnsurer)
	}
	if !ok || len(promotion.MCPResourceServers) == 0 {
		return nil
	}
	handles := make([]string, 0, len(promotion.MCPResourceServers))
	for _, server := range promotion.MCPResourceServers {
		handles = append(handles, server.ProxyHandle)
	}
	result, err := ensurer.EnsureResourceServersForAgent(ctx, MCPResourceServerPreflightQuery{
		OUID: promotion.OUID, HomeEnvironment: homeEnv, ProxyHandles: handles,
	})
	if err != nil {
		return fmt.Errorf("check MCP resource servers in %s: %w", homeEnv, err)
	}
	if len(result.Blockers) > 0 {
		return &IdentityPromotionError{Kind: IdentityPromotionBlocked, Message: result.Blockers[0].Message}
	}
	return nil
}
