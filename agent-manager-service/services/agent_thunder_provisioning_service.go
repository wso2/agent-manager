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
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// maxProvisionAttempts is the retry budget for one binding: the inline
// fast-path attempt plus reconciler retries, five attempts total before a
// binding is marked FAILED.
const maxProvisionAttempts = 5

// writeAheadUpsertAttempts/writeAheadUpsertRetryDelay cover a momentary DB
// blip on the write-ahead insert itself — there is no row for the reconciler
// to find and retry later if this insert never lands.
const (
	writeAheadUpsertAttempts   = 3
	writeAheadUpsertRetryDelay = 100 * time.Millisecond
)

// provisionRetryDelay is the flat delay before every retry (no escalation to
// longer delays; a real outage is retried just as promptly on attempt 4 as
// attempt 1, since env-Thunder recovering doesn't get less likely over
// time). maxProvisionAttempts-1 retries at this interval means the final
// attempt starts at most (maxProvisionAttempts-1)*provisionRetryDelay after
// the first, so the whole retry budget for one binding resolves — success or
// FAILED — within the 15-minute SLA.
const provisionRetryDelay = 3 * time.Minute

// AgentThunderProvisioningService orchestrates AgentID provisioning: creating,
// regenerating, revoking, and deleting Thunder identities per agent per
// environment. Binding architecture (Section 7 — pushing a credential into the
// gateway) is a later phase and is deliberately not implemented here.
//
// Deployment-pluggable (see app.Options.AgentThunderProvisioning; the
// open-source build injects DBBackedAgentThunderProvisioning). Credential
// storage goes through secretmanagersvc.SecretManagementClient — the same
// deployment-pluggable seam LLM/MCP/publisher secrets already use (see
// app.Options's secretProvider) — instead of talking to OpenBao directly, so
// a deployment can swap secret backends without any AgentID-specific code
// change. AgentThunderClient.SecretRefPath stores the SecretReference name
// CreateSecret returns (empty means "no credential currently stored", e.g.
// revoked) — never a locally-computed KV path. CreateSecret's own call
// already creates a fully-configured SecretReference as a side effect of
// storing the value (see CreateSecretRequest.Labels's doc comment: "Labels
// applied to the underlying SecretReference"), so this stored name is the
// only thing AgentIdentityInjectionService needs to point a pod's
// SecretKeyRef at it — see storeCredential and agentIdentitySecretLocation.
type AgentThunderProvisioningService interface {
	// ProvisionForAgent writes a PENDING binding for every environment in
	// envNames (write-ahead — Section 6.2), then attempts all of them
	// best-effort in the background. Never blocks the caller. requestedBy is
	// the calling user's own subject, captured for audit only (see
	// models.AgentThunderClient.RequestedBy) — it is never sent to Thunder.
	ProvisionForAgent(ctx context.Context, ouID, projectName, agentName string, ownership models.AgentProvisioningType, envNames []string, requestedBy string)

	// AttemptProvision performs one provisioning attempt for a single binding.
	// Exported so the retry reconciler can call it directly for PENDING rows
	// found on its sweep.
	AttemptProvision(ctx context.Context, binding models.AgentThunderClient)

	// ProvisionForEnvironmentIfMissing ensures a binding exists for one agent in
	// one environment — used both by the external-agent identity-provision
	// endpoint and by PromoteAgent's internal-agent hook, so an environment that
	// didn't exist yet when the agent was created (or wasn't part of its
	// pipeline) still gets an AgentID once it's actually needed there. If a
	// binding already exists (any status — pending, completed, or failed), it is
	// left untouched and alreadyExisted is true; the reconciler already owns
	// retrying anything not yet completed. If none exists, behaves exactly like
	// ProvisionForAgent for this one environment: write-ahead PENDING, then a
	// best-effort background attempt.
	ProvisionForEnvironmentIfMissing(ctx context.Context, ouID, projectName, agentName, envName string, ownership models.AgentProvisioningType, requestedBy string) (alreadyExisted bool, err error)

	// RetryProvisioning resets a FAILED binding back to PENDING — clearing its
	// attempt budget and last error — and re-attempts provisioning in the
	// background, for both Internal and External agents. Returns
	// utils.ErrAgentIdentityNotProvisioned if no binding exists for this agent
	// in this environment, or utils.ErrAgentIdentityRetryNotAllowed if the
	// binding exists but its current status is not FAILED — completed,
	// pending, and in-progress bindings already have their own path forward
	// and are rejected rather than silently no-oped.
	RetryProvisioning(ctx context.Context, ouID, projectName, agentName, envName string) (models.AgentIdentityEnvironmentView, error)

	// RegenerateSecret rotates the secret for one binding and returns the
	// binding's ownership type, client ID, and the new secret. The caller (the
	// HTTP layer) decides whether to expose the secret in the response based on
	// ownership — this service always returns the true new secret. For External
	// agents this is the ONLY way to obtain a secret at all (first time or any
	// later rotation) — nothing is ever persisted for them, so there is no
	// separate "claim" step; the secret is minted via Thunder and handed back
	// directly, never written to secretMgmtClient.
	RegenerateSecret(ctx context.Context, ouID, projectName, agentName, envName string) (ownership models.AgentProvisioningType, clientID string, newSecret string, err error)

	// RevokeSecret rotates the secret in Thunder (invalidating the old one) and
	// clears the stored copy, leaving no currently-valid credential until an
	// explicit regenerate. It does not return the new secret value to anyone —
	// only the (unchanged) client ID, so callers can build a response body.
	RevokeSecret(ctx context.Context, ouID, projectName, agentName, envName string) (clientID string, err error)

	// DeleteAllBindings deletes every Thunder identity, stored secret, and
	// binding row for the agent, across all environments. Best-effort: logs
	// failures and never blocks the caller.
	DeleteAllBindings(ctx context.Context, ouID, projectName, agentName string)

	// GetIdentityViews returns the current binding for every environment this
	// agent has been provisioned in. A safe, side-effect-free read: it never
	// returns or destroys a secret — External agents never have one stored to
	// begin with; RegenerateSecret is the only way to obtain one. Callers
	// needing project-level visibility filtering (Section 2.1 of the
	// architecture doc) apply it on top of this org-wide result.
	GetIdentityViews(ctx context.Context, ouID, projectName, agentName string) ([]models.AgentIdentityEnvironmentView, error)

	// GetBindingState returns the raw provisioning state for one (agent, env)
	// binding — status, whether a secret is currently stored, and the last
	// recorded error. Used by callers that need more than "is it ready" (e.g.
	// PromoteAgent's hard block) to tell apart WHY it isn't: still
	// provisioning (retrying helps), permanently failed, or revoked (retrying
	// does not help either). Returns (nil, nil) when no binding exists yet at
	// all — the same "nothing to report" convention used elsewhere in this
	// service, not an error.
	GetBindingState(ctx context.Context, ouID, projectName, agentName, envName string) (*AgentThunderBindingState, error)

	// GetAgentRoles returns the Thunder roles assigned to the agent's AgentID
	// in one environment. Returns utils.ErrAgentIdentityNotProvisioned if the
	// binding doesn't exist or hasn't completed yet.
	GetAgentRoles(ctx context.Context, ouID, projectName, agentName, envName string) ([]thundersvc.ThunderRole, error)

	// GetAgentGroups returns the Thunder groups the agent's AgentID belongs to
	// in one environment. Returns utils.ErrAgentIdentityNotProvisioned if the
	// binding doesn't exist or hasn't completed yet.
	GetAgentGroups(ctx context.Context, ouID, projectName, agentName, envName string) ([]thundersvc.ThunderGroup, error)

	// HealSecretRef corrects binding.SecretRefPath when it doesn't match the
	// credential's actual remote KV key, by re-resolving it via the same
	// GetSecretReference lookup storeCredential itself uses (see its doc
	// comment) — one bounded OpenChoreo read per call, no writes. No-op for
	// external agents, unprovisioned bindings, and bindings already holding
	// the correct value. Called once per completed internal binding from the
	// reconciler's unbounded startup sweep.
	HealSecretRef(ctx context.Context, binding models.AgentThunderClient) error
}

// AgentThunderBindingState is a minimal, internal snapshot of one binding's
// provisioning state — deliberately not the same type as
// models.AgentIdentityEnvironmentView (the public API shape for GET
// .../identities), since HasSecret exposes internal-agent secret presence
// that view intentionally does not surface for every provisioning type.
type AgentThunderBindingState struct {
	ProvisioningType models.AgentProvisioningType
	Status           models.AgentThunderStatus
	// HasSecret is true when the binding currently has a stored secret
	// reference (SecretRefPath != ""). False for a COMPLETED binding means the
	// credential was revoked, not that provisioning is still in progress.
	HasSecret bool
	LastError string
}

type agentThunderProvisioningService struct {
	repo             repositories.AgentThunderClientRepository
	envResolver      thundersvc.EnvThunderResolver
	secretMgmtClient secretmanagersvc.SecretManagementClient
	// ocClient resolves the REAL remote KV key for a stored credential — the
	// SecretReference secretMgmtClient.CreateSecret manages internally is not
	// itself reachable from the agent's own workload namespace, only whatever
	// the secret provider decides as its physical location; storeCredential
	// reads that back once, at credential creation/rotation, rather than
	// guessing it locally. See storeCredential's doc comment.
	ocClient client.OpenChoreoClient
	// workloadInjector pushes an internal agent's credential into its live
	// workload (Gateway Binding). Optional (nil skips injection). Used by the
	// post-provisioning reconcile hook to cover agents whose workload comes up
	// independently of DeployAgent. Guarded by workloadInjectorMu: it starts
	// nil and is backfilled once via SetWorkloadInjector (see that method's
	// doc comment for why), from a different goroutine than the ones calling
	// AttemptProvision — an RWMutex makes that safe without depending on
	// app.Run's call ordering to establish a happens-before relationship.
	workloadInjectorMu sync.RWMutex
	workloadInjector   AgentIdentityInjectionService
	logger             *slog.Logger
	bindingLocks       keyedMutex
}

// agentIdentitySecretLocation returns the secretmanagersvc.SecretLocation for
// one binding's stored credential. Deterministic from (org, project, env,
// agent) — both the KV path (location.KVPath()) and the SecretReference CR
// name (location.SecretRefName()) are pure functions of these fields, so this
// service and agentIdentityInjectionService always agree on where a
// binding's credential lives without either needing to ask the other or
// round-trip through a stored/returned value. EntityName is agent-scoped
// (not just "agent-identity") so two different agents in the same
// environment never derive the same SecretReference name.
func agentIdentitySecretLocation(ouID, projectName, agentName, envName string) secretmanagersvc.SecretLocation {
	return secretmanagersvc.SecretLocation{
		OrgName:         ouID,
		ProjectName:     projectName,
		EnvironmentName: envName,
		AgentName:       agentName,
		EntityName:      agentName + "-agent-identity",
	}
}

// createSecretConflictRetryMarker is the exact substring secretmanagersvc's
// OpenChoreo provider includes only when a create conflict's own fallback
// update then also fails (PushSecret's "failed to update secret after create
// conflict" wrap in clients/secretmanagersvc/providers/openchoreo/client.go).
// The secret manager doesn't expose a distinct sentinel for this exact shape,
// so this is the only way to tell it apart from other, unrelated not-found
// errors without changing the secret manager itself.
const createSecretConflictRetryMarker = "after create conflict"

// storeCredential writes the client ID/secret pair for one binding via the
// shared secret management client and returns the remote KV key to persist
// in AgentThunderClient.SecretRefPath.
//
// CreateSecret's own return value is only the name of the tracking
// SecretReference it manages internally (see
// SecretManagementClient.CreateSecret's doc comment) — that resource is not
// reachable from the agent's own workload namespace. The actual remote KV
// location is decided by whichever secret provider is plugged in and is only
// ever learned by reading it back via GetSecretReference, once, here — never
// guessed locally, and never re-verified by AgentIdentityInjectionService on
// its own (much more frequent) injection path. Same resolve-don't-guess
// pattern as agentManagerService.storeAgentAPIKey.
func (s *agentThunderProvisioningService) storeCredential(ctx context.Context, ouID, projectName, agentName, envName, clientID, clientSecret string) (string, error) {
	location := agentIdentitySecretLocation(ouID, projectName, agentName, envName)
	data := map[string]string{
		thundersvc.AgentSecretKeyClientID:     clientID,
		thundersvc.AgentSecretKeyClientSecret: clientSecret,
	}
	refName, err := s.secretMgmtClient.CreateSecret(ctx, location, data)
	if err != nil {
		if !errors.Is(err, utils.ErrNotFound) || !strings.Contains(err.Error(), createSecretConflictRetryMarker) {
			return "", err
		}
		// An already-provisioned internal agent already has a data-plane
		// SecretReference for this name, owned by the identity injection
		// service rather than by this code, which is why the create above
		// failed here. Delete it and create again: the retry succeeds
		// because it's now a plain create with nothing in the way, same as
		// a brand-new agent's first credential.
		injector := s.getWorkloadInjector()
		if injector == nil {
			return "", err
		}
		if cleanupErr := injector.CleanupForEnvironment(ctx, ouID, agentName, envName); cleanupErr != nil {
			return "", fmt.Errorf("clear existing SecretReference before retry: %w: %w", cleanupErr, err)
		}
		refName, err = s.secretMgmtClient.CreateSecret(ctx, location, data)
		if err != nil {
			return "", err
		}
	}

	kvPath, err := s.resolveClientSecretKVPath(ctx, ouID, refName)
	if err != nil {
		return "", err
	}
	return kvPath, nil
}

// resolveClientSecretKVPath reads back the real remote KV key for refName's
// AgentSecretKeyClientSecret data source. The one place both storeCredential
// (at creation/rotation) and HealSecretRef (the one-time startup backfill)
// learn where a credential actually lives — never guessed, never computed
// from agentIdentitySecretLocation.
func (s *agentThunderProvisioningService) resolveClientSecretKVPath(ctx context.Context, ouID, refName string) (string, error) {
	if s.ocClient == nil {
		return "", fmt.Errorf("resolve stored KV path for SecretReference %q: OpenChoreo client not configured", refName)
	}
	ref, err := s.ocClient.GetSecretReference(ctx, ouID, refName)
	if err != nil {
		return "", fmt.Errorf("resolve stored KV path for SecretReference %q: %w", refName, err)
	}
	for _, ds := range ref.Data {
		if ds.SecretKey == thundersvc.AgentSecretKeyClientSecret {
			return ds.RemoteRef.Key, nil
		}
	}
	return "", fmt.Errorf("SecretReference %q has no %q data source", refName, thundersvc.AgentSecretKeyClientSecret)
}

// HealSecretRef corrects a binding's SecretRefPath if its stored value
// doesn't match the credential's actual remote KV key. Re-resolves via the
// same GetSecretReference lookup storeCredential itself uses — the binding's
// deterministic SecretReference name is a pure function of (ouID,
// projectName, agentName, envName), so re-deriving it locally and reading it
// back is always safe regardless of how the stored value got stale. Called
// once per completed internal binding from the reconciler's unbounded
// startup sweep; a no-op on every run after the first successful heal for a
// given binding.
//
// The OpenChoreo resolve runs BEFORE the binding lock (never hold a lock
// across I/O), but the write is gated on a fresh re-read taken INSIDE the
// same bindingLocks key every other SecretRefPath writer (RegenerateSecret,
// RevokeSecret, DeleteAllBindings) already holds. The sweep that calls this
// runs concurrently with the HTTP server from process startup (Start()
// backgrounds it and returns immediately) — without the re-read, a revoke or
// delete landing on this binding between the sweep's initial snapshot and
// this call's resolve would have its SecretRefPath="" clear silently
// overwritten with a resolved-but-now-orphaned path, which the injection
// reconciler would then treat as a live credential and re-inject.
func (s *agentThunderProvisioningService) HealSecretRef(ctx context.Context, binding models.AgentThunderClient) error {
	if binding.ProvisioningType != models.AgentProvisioningTypeInternal || binding.SecretRefPath == "" {
		return nil
	}
	refName := agentIdentitySecretLocation(binding.OUID, binding.ProjectName, binding.AgentName, binding.EnvironmentName).SecretRefName()
	kvPath, err := s.resolveClientSecretKVPath(ctx, binding.OUID, refName)
	if err != nil {
		return fmt.Errorf("heal secret ref for binding %s: %w", binding.ID, err)
	}
	if binding.SecretRefPath == kvPath {
		return nil
	}

	release, lockErr := s.bindingLocks.Lock(ctx, bindingLockKey(binding.OUID, binding.ProjectName, binding.AgentName, binding.EnvironmentName))
	if lockErr != nil {
		return fmt.Errorf("heal secret ref for binding %s: %w", binding.ID, lockErr)
	}
	defer release()

	current, err := s.repo.Get(ctx, binding.OUID, binding.ProjectName, binding.AgentName, binding.EnvironmentName)
	if err != nil {
		if errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
			return nil // deleted while this heal was resolving — nothing left to heal
		}
		return fmt.Errorf("heal secret ref for binding %s: %w", binding.ID, err)
	}
	if current.SecretRefPath == "" || current.SecretRefPath == kvPath {
		return nil // revoked/cleared for deletion since the snapshot, or already healed
	}
	return s.repo.UpdateSecretRef(ctx, current.ID, kvPath)
}

// deleteCredential permanently removes the stored credential (and its
// SecretReference CR) for one binding. agentIdentitySecretLocation is a pure
// function of these fields, so any binding's own (ouID, projectName,
// agentName, envName) always reproduces the exact location it was stored at
// — nothing needs to be parsed back out of the stored SecretRefPath itself.
func (s *agentThunderProvisioningService) deleteCredential(ctx context.Context, ouID, projectName, agentName, envName string) error {
	location := agentIdentitySecretLocation(ouID, projectName, agentName, envName)
	return s.secretMgmtClient.DeleteSecret(ctx, location, location.SecretRefName())
}

// WorkloadInjectorSetter is an optional capability a deployment's
// AgentThunderProvisioningService MAY implement — it is deliberately NOT part
// of the AgentThunderProvisioningService interface itself, so an alternative
// implementation that does no workload injection isn't forced to carry a
// meaningless setter. app.Run type-asserts for it instead (see app.Run).
type WorkloadInjectorSetter interface {
	// SetWorkloadInjector backfills the workload injector once the real
	// AgentIdentityInjectionService exists (this service is constructed before
	// the OpenChoreo client — see DBBackedAgentThunderProvisioning). app.Run
	// calls it exactly once, before the reconciler or HTTP server start. No-op
	// when injector is nil. If this backfill is ever skipped (a startup-order
	// regression), the post-provisioning reconcile hook logs a warning rather
	// than silently no-op-ing, so the gap is visible.
	SetWorkloadInjector(injector AgentIdentityInjectionService)
}

func (s *agentThunderProvisioningService) SetWorkloadInjector(injector AgentIdentityInjectionService) {
	if injector == nil {
		return
	}
	s.workloadInjectorMu.Lock()
	s.workloadInjector = injector
	s.workloadInjectorMu.Unlock()
}

func (s *agentThunderProvisioningService) getWorkloadInjector() AgentIdentityInjectionService {
	s.workloadInjectorMu.RLock()
	defer s.workloadInjectorMu.RUnlock()
	return s.workloadInjector
}

// reconcileWorkloadInjection idempotently reconciles binding's live workload
// with its desired AgentID env vars (writing only when missing or stale — see
// AgentIdentityInjectionService.ReconcileForEnvironment). Package-level (not a
// method) since it is shared by two independent callers that each hold their
// own reference to the injector: agentThunderProvisioningService's
// post-AttemptProvision hook, and agentThunderReconcilerService's periodic
// sweep — neither needs this on its exported interface. Together they mean a
// brand-new git agent — whose ReleaseBinding does not exist until its first
// build finishes minutes later — still gets its credentials the moment the
// workload appears. Best-effort: internal agents only, errors logged and
// never returned.
func reconcileWorkloadInjection(ctx context.Context, injector AgentIdentityInjectionService, binding models.AgentThunderClient, logger *slog.Logger) {
	if binding.ProvisioningType != models.AgentProvisioningTypeInternal {
		return
	}
	if injector == nil {
		// Should not happen in production once app.Run's SetWorkloadInjector
		// backfill has run (see that method's doc comment) — logged, not
		// silently skipped, so a future startup-order regression that leaves
		// this nil shows up immediately instead of as a mysteriously
		// uninjected credential days later.
		logger.Warn("Skipping agent identity workload reconcile: no workload injector configured",
			"ou_id", binding.OUID, "binding_id", binding.ID, "agent_name", binding.AgentName, "env_name", binding.EnvironmentName)
		return
	}
	if err := injector.ReconcileForEnvironment(ctx, binding.OUID, binding.ProjectName, binding.AgentName, binding.EnvironmentName); err != nil {
		logger.Warn("Failed to reconcile agent identity credentials into workload",
			"ou_id", binding.OUID, "binding_id", binding.ID, "agent_name", binding.AgentName, "env_name", binding.EnvironmentName, "error", err)
	}
}

// DBBackedAgentThunderProvisioning returns a constructor to be called
// once the DB and secret management client are available (see
// app.Options.AgentThunderProvisioning), rather than building the
// provisioning service immediately. Used by the
// open-source deployment, which provisions AgentIDs against per-environment
// Thunder via env-Thunder. "DB-backed" (not "default") is the load-bearing
// distinction: the env-Thunder system-client credential this service reads
// (via NewEnvThunderSecretReader) is decrypted from AMS's own Postgres rather
// than read back from a key vault — the same reason a SaaS/cloud deployment,
// whose vault may be reveal-once, needs this same DB-backed approach rather
// than an alternative provisioning implementation.
//
// secretMgmtClient is supplied by app.Run, built the same way as the shared
// one wiring.InitializeAppParams builds for every other secret-backed service
// (LLM/MCP/publisher) — this factory can't build it itself: it runs BEFORE
// InitializeAppParams (which constructs the OpenChoreo client the shared
// instance depends on), the same ordering constraint SetWorkloadInjector's
// doc comment describes for workloadInjector below. secretMgmtClient here is
// used only for per-agent credentials; env-Thunder's own system-client secret
// comes from encryptionKey + NewEnvThunderSecretReader (Postgres, not a vault).
//
// workloadInjector starts nil: this factory runs before the OpenChoreo client
// exists (built inside wiring.InitializeAppParams, which this feeds into), so
// there is nothing to build the injector from yet. app.Run backfills it via
// SetWorkloadInjector right after InitializeAppParams returns. Without that, an
// agent whose workload comes up outside AgentManagerService.DeployAgent (a
// git/build-pipeline agent, or a kind-sourced one) would never get its AgentID
// env vars — neither path calls InjectForEnvironment itself.
func DBBackedAgentThunderProvisioning() func(db *gorm.DB, secretMgmtClient secretmanagersvc.SecretManagementClient, ocClient client.OpenChoreoClient, encryptionKey []byte) AgentThunderProvisioningService {
	return func(db *gorm.DB, secretMgmtClient secretmanagersvc.SecretManagementClient, ocClient client.OpenChoreoClient, encryptionKey []byte) AgentThunderProvisioningService {
		envThunderRepo := repositories.NewEnvThunderSystemClientRepo(db)
		envThunderURLRepo := repositories.NewEnvThunderURLRepo(db)
		return NewAgentThunderProvisioningService(
			repositories.NewAgentThunderClientRepo(db),
			thundersvc.NewEnvThunderResolver(
				NewEnvThunderSecretReader(envThunderRepo, encryptionKey),
				NewEnvThunderURLReader(envThunderURLRepo),
			),
			secretMgmtClient,
			ocClient,
			nil, // workload injector — see doc comment above
			slog.Default(),
		)
	}
}

// NewAgentThunderProvisioningService creates a new AgentThunderProvisioningService.
// workloadInjector may be nil (no workload injection on provisioning completion).
func NewAgentThunderProvisioningService(
	repo repositories.AgentThunderClientRepository,
	envResolver thundersvc.EnvThunderResolver,
	secretMgmtClient secretmanagersvc.SecretManagementClient,
	ocClient client.OpenChoreoClient,
	workloadInjector AgentIdentityInjectionService,
	logger *slog.Logger,
) AgentThunderProvisioningService {
	return &agentThunderProvisioningService{
		repo:             repo,
		envResolver:      envResolver,
		secretMgmtClient: secretMgmtClient,
		ocClient:         ocClient,
		workloadInjector: workloadInjector,
		logger:           logger,
	}
}

// keyedMutex serializes RegenerateSecret/RevokeSecret per binding within this
// process, so two concurrent rotations for the same binding can't interleave
// their Thunder call and OpenBao write and leave the stored secret mismatched
// with what Thunder actually considers active. In-process only — it does not
// protect across multiple replicas of this service. Entries are refcounted
// and removed once the last waiter releases, so the map doesn't grow
// unbounded with every binding ever rotated over a long-lived process.
//
// The critical section this guards spans real Thunder/OpenBao/DB I/O (see
// AttemptProvision/RegenerateSecret/RevokeSecret) — that I/O already has its
// own hard timeouts (Thunder's HTTP client: 30s; OpenBao's Vault client:
// 60s), so a holder cannot block a waiter forever. But a waiter should not
// have to sit out another attempt's FULL timeout window just because ITS OWN
// caller (e.g. an HTTP request) gave up sooner — Lock is context-aware so a
// waiter returns as soon as its own ctx is done, instead of only after the
// current holder finishes.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*refCountedMutex
}

// refCountedMutex is a channel-based binary lock (buffered size 1, holding a
// token when free) rather than a sync.Mutex, specifically so acquiring it can
// be select-ed against ctx.Done() — sync.Mutex.Lock has no cancellable variant.
type refCountedMutex struct {
	ch   chan struct{}
	refs int
}

func newRefCountedMutex() *refCountedMutex {
	l := &refCountedMutex{ch: make(chan struct{}, 1)}
	l.ch <- struct{}{}
	return l
}

// Lock blocks until key is free or ctx is done, whichever happens first. On
// success it returns a release func that must be called exactly once; on
// failure it returns a nil func and ctx.Err() — callers must check the error
// before using the func.
func (m *keyedMutex) Lock(ctx context.Context, key string) (func(), error) {
	m.mu.Lock()
	if m.locks == nil {
		m.locks = make(map[string]*refCountedMutex)
	}
	l, ok := m.locks[key]
	if !ok {
		l = newRefCountedMutex()
		m.locks[key] = l
	}
	l.refs++
	m.mu.Unlock()

	release := func() {
		m.mu.Lock()
		l.refs--
		if l.refs == 0 {
			delete(m.locks, key)
		}
		m.mu.Unlock()
	}

	select {
	case <-l.ch:
		return func() {
			l.ch <- struct{}{}
			release()
		}, nil
	case <-ctx.Done():
		// Never acquired — undo the refcount bump so the entry can still be
		// evicted once its actual holder(s) release, same as if this waiter
		// had never shown up.
		release()
		return nil, ctx.Err()
	}
}

func bindingLockKey(ouID, projectName, agentName, envName string) string {
	return ouID + "|" + projectName + "|" + agentName + "|" + envName
}

func (s *agentThunderProvisioningService) ProvisionForAgent(
	ctx context.Context, ouID, projectName, agentName string, ownership models.AgentProvisioningType, envNames []string, requestedBy string,
) {
	bindings := make([]models.AgentThunderClient, 0, len(envNames))
	for _, env := range envNames {
		b := models.AgentThunderClient{
			OUID:             ouID,
			ProjectName:      projectName,
			AgentName:        agentName,
			EnvironmentName:  env,
			ProvisioningType: ownership,
			Status:           models.AgentThunderStatusPending,
			RequestedBy:      requestedBy,
		}
		// Retry a few times before giving up: this row is the ONLY thing the
		// reconciler can later find and heal, so a momentary DB blip here must
		// not permanently drop the environment with no path to provisioning.
		var err error
		for attempt := 0; attempt < writeAheadUpsertAttempts; attempt++ {
			if err = s.repo.Upsert(ctx, &b); err == nil {
				break
			}
			time.Sleep(writeAheadUpsertRetryDelay)
		}
		if err != nil {
			s.logger.Error("Failed to write-ahead agent thunder binding after retries", "agent_name", agentName, "env", env, "error", err)
			continue
		}
		bindings = append(bindings, b)
	}

	// Detach from the request context so the background attempt survives the
	// HTTP handler returning, mirroring the existing deleteAgentLLMConfigurations
	// pattern in agent_manager.go.
	go s.attemptAll(context.WithoutCancel(ctx), bindings)
}

func (s *agentThunderProvisioningService) attemptAll(ctx context.Context, bindings []models.AgentThunderClient) {
	for _, b := range bindings {
		s.AttemptProvision(ctx, b)
	}
}

func (s *agentThunderProvisioningService) ProvisionForEnvironmentIfMissing(
	ctx context.Context, ouID, projectName, agentName, envName string, ownership models.AgentProvisioningType, requestedBy string,
) (bool, error) {
	_, err := s.repo.Get(ctx, ouID, projectName, agentName, envName)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
		return false, fmt.Errorf("check existing agent thunder binding: %w", err)
	}

	s.ProvisionForAgent(ctx, ouID, projectName, agentName, ownership, []string{envName}, requestedBy)
	return false, nil
}

func (s *agentThunderProvisioningService) RetryProvisioning(ctx context.Context, ouID, projectName, agentName, envName string) (models.AgentIdentityEnvironmentView, error) {
	binding, err := s.repo.Get(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		if errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
			return models.AgentIdentityEnvironmentView{}, fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
		}
		return models.AgentIdentityEnvironmentView{}, err
	}
	if binding.Status != models.AgentThunderStatusFailed {
		return models.AgentIdentityEnvironmentView{}, fmt.Errorf("%w: %s in %s is %s", utils.ErrAgentIdentityRetryNotAllowed, agentName, envName, binding.Status)
	}

	reset, err := s.repo.ResetFailedForRetry(ctx, binding.ID)
	if err != nil {
		return models.AgentIdentityEnvironmentView{}, fmt.Errorf("reset agent thunder binding for retry: %w", err)
	}
	if !reset {
		// Lost a race with something else that moved the binding out of FAILED
		// between the read above and this conditional update (another retry,
		// or the reconciler) — same outward signal as "not currently failed."
		return models.AgentIdentityEnvironmentView{}, fmt.Errorf("%w: %s in %s changed state concurrently", utils.ErrAgentIdentityRetryNotAllowed, agentName, envName)
	}

	binding.Status = models.AgentThunderStatusPending
	binding.AttemptCount = 0
	binding.LastError = ""
	binding.LastAttemptedAt = nil
	binding.NextRetryAt = nil

	// Detach from the request context so the background attempt survives the
	// HTTP handler returning, mirroring ProvisionForAgent's own attemptAll call.
	go s.AttemptProvision(context.WithoutCancel(ctx), *binding)

	return models.AgentIdentityEnvironmentView{
		EnvironmentName:  binding.EnvironmentName,
		ProvisioningType: binding.ProvisioningType,
		Status:           models.AgentThunderStatusPending,
		AgentID:          binding.ThunderAgentID,
		ClientID:         binding.ThunderClientID,
		RequestedBy:      binding.RequestedBy,
	}, nil
}

func (s *agentThunderProvisioningService) AttemptProvision(ctx context.Context, binding models.AgentThunderClient) {
	// Held for the entire attempt so this can't interleave its Thunder
	// RegenerateAgentSecret/OpenBao Store calls with a concurrent
	// RegenerateSecret/RevokeSecret on the same binding. ctx here is normally
	// either detached (ProvisionForAgent's background goroutine) or the
	// reconciler's own long-lived context, so in practice this only returns
	// early on process shutdown — see keyedMutex.Lock's doc comment.
	release, err := s.bindingLocks.Lock(ctx, bindingLockKey(binding.OUID, binding.ProjectName, binding.AgentName, binding.EnvironmentName))
	if err != nil {
		s.logger.Warn("Agent thunder provisioning attempt cancelled while waiting for binding lock",
			"binding_id", binding.ID, "agent_name", binding.AgentName, "env_name", binding.EnvironmentName, "error", err)
		return
	}
	defer release()

	// A panic here (e.g. AgentThunderAppName's on an invalid slug) would
	// otherwise crash the whole process — this runs on a detached goroutine
	// or the reconciler's per-binding goroutine, never the request path.
	// Recovering isolates it to just this one binding.
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("Recovered from panic during agent thunder provisioning attempt",
				"binding_id", binding.ID, "agent_name", binding.AgentName, "env_name", binding.EnvironmentName, "panic", r)
			s.recordFailure(ctx, binding, "", "", fmt.Errorf("panic during provisioning attempt: %v", r))
		}
	}()

	// Atomically claim the binding before doing anything else: the inline
	// fast-path goroutine (ProvisionForAgent) and the reconciler's sweep can
	// both land on the same freshly-written binding within the same ~60s
	// window, since a write-ahead row has next_retry_at=nil, which FindDue
	// matches immediately. Without this claim, both could concurrently call
	// Thunder's CreateAgentIdentity/RegenerateAgentSecret and UpdateAfterAttempt
	// on the same row. claimed=false means someone else already holds this
	// binding (or it raced past being pending/stale) — skip silently rather
	// than duplicate the work.
	claimed, claimErr := s.repo.ClaimForAttempt(ctx, binding.ID)
	if claimErr != nil {
		s.logger.Error("Failed to claim agent thunder binding for attempt", "binding_id", binding.ID, "error", claimErr)
		return
	}
	if !claimed {
		s.logger.Debug("Agent thunder binding already claimed by another attempt, skipping", "binding_id", binding.ID)
		return
	}

	thunderClient, err := ResolveEnvThunderClient(ctx, s.envResolver, binding.OUID, binding.EnvironmentName)
	if err != nil {
		s.recordFailure(ctx, binding, "", "", err)
		return
	}

	// External agents only need a client ID at provisioning time.
	// Secrets are minted on demand via RegenerateSecret and never persisted.
	isExternal := binding.ProvisioningType == models.AgentProvisioningTypeExternal

	thunderAgentID := binding.ThunderAgentID
	clientID := binding.ThunderClientID
	var clientSecret string

	if thunderAgentID == "" {
		ouID, err := thunderClient.GetDefaultOUID(ctx)
		if err != nil {
			s.recordFailure(ctx, binding, "", "", fmt.Errorf("get default OU: %w", err))
			return
		}

		appName := thundersvc.AgentThunderAppName(binding.OUID, binding.EnvironmentName, binding.ProjectName, binding.AgentName)
		var created bool
		thunderAgentID, clientID, clientSecret, created, err = thunderClient.CreateAgentIdentity(ctx, ouID, appName, "")
		if err != nil {
			s.recordFailure(ctx, binding, "", "", fmt.Errorf("create agent identity: %w", err))
			return
		}

		if isExternal {
			clientSecret = ""
		} else if !created && clientSecret == "" {
			// Partial-failure recovery (Section 6.8): if Thunder already had this
			// agent (created=false, found via the 409 fallback), it never returns a
			// secret. That only happens if a prior attempt got as far as creating
			// the identity in Thunder but failed before we could store a secret —
			// there is no way to retrieve the original one, so generate a fresh,
			// storable secret now instead of leaving the binding stuck forever.
			clientSecret, err = thunderClient.RegenerateAgentSecret(ctx, thunderAgentID)
			if err != nil {
				// thunderAgentID/clientID are already resolved at this point —
				// pass them through so a failure here doesn't lose them (see
				// the identical reasoning on the storeCredential failure below).
				s.recordFailure(ctx, binding, thunderAgentID, clientID, fmt.Errorf("recover secret for existing agent identity: %w", err))
				return
			}
		}
	} else if !isExternal && binding.SecretRefPath == "" {
		// Retry of an attempt that created the identity but failed before storing a secret.
		clientSecret, err = thunderClient.RegenerateAgentSecret(ctx, thunderAgentID)
		if err != nil {
			s.recordFailure(ctx, binding, thunderAgentID, clientID, fmt.Errorf("recover secret for existing agent identity: %w", err))
			return
		}
	}

	secretRefPath := binding.SecretRefPath
	if !isExternal && clientSecret != "" {
		secretRefPath, err = s.storeCredential(ctx, binding.OUID, binding.ProjectName, binding.AgentName, binding.EnvironmentName, clientID, clientSecret)
		if err != nil {
			// The Thunder identity was already created successfully above —
			// pass thunderAgentID/clientID through so recordFailure persists
			// them despite this failure. Without this, the next retry would
			// see ThunderAgentID=="" and call CreateAgentIdentity again for a
			// name that already exists, hitting the 409 fallback and forcing
			// an unnecessary secret rotation.
			s.recordFailure(ctx, binding, thunderAgentID, clientID, fmt.Errorf("store agent secret: %w", err))
			return
		}
	}

	if err := s.repo.UpdateAfterAttempt(ctx, binding.ID, repositories.AgentThunderAttemptUpdate{
		Status:          models.AgentThunderStatusCompleted,
		ThunderAgentID:  &thunderAgentID,
		ThunderClientID: &clientID,
		SecretRefPath:   &secretRefPath,
	}); err != nil {
		s.logger.Error("Failed to record successful agent thunder provisioning", "binding_id", binding.ID, "error", err)
		return
	}

	// A credential issued with no request behind it: the actor is this service,
	// and the user who originally asked for the agent is carried as OnBehalfOf.
	// That attribution is exactly what requested_by was captured for.
	audit.Record(
		ctx, audit.ActionSystemAgentIdentityProvisioned,
		audit.Org(binding.OUID),
		audit.ResourceNamed(audit.ResourceAgentIdentity, binding.AgentName, binding.AgentName),
		audit.Project(binding.ProjectName),
		audit.Environment(binding.EnvironmentName),
		audit.Actor(audit.ActorSystem, systemActorAgentThunderReconciler, ""),
		audit.SurfaceOpt(audit.SurfaceSystem),
		audit.OnBehalfOf(binding.RequestedBy),
		audit.Detail("agentName", binding.AgentName),
		audit.Detail("environment", binding.EnvironmentName),
		audit.Detail("clientId", clientID),
	)

	// Gateway Binding: push the credential into the live workload for internal
	// agents that were already deployed before this attempt completed. Purely
	// best-effort — the binding is COMPLETED either way, and the deploy flow
	// re-derives these env vars on every (re)deploy regardless. For a
	// brand-new git agent whose first build has not yet created a
	// ReleaseBinding (minutes away), this reconcile no-ops now and the
	// reconciler's periodic sweep lands the credentials once the workload
	// appears — see reconcileWorkloadInjection.
	reconcileWorkloadInjection(ctx, s.getWorkloadInjector(), binding, s.logger)
}

// recordFailure retries cause up to maxProvisionAttempts times before marking
// the binding FAILED. ErrThunderNotProvisioned is treated the same as any
// other failure, not as an immediate permanent one — the environment may just
// be mid-bootstrap (add-environment-thunder.sh still running), a window the
// existing retry budget already covers. resolvedAgentID/resolvedClientID
// carry through a Thunder identity that was already created before a LATER
// step in this same attempt failed (e.g. secret storage) — passing "" for
// either means nothing was resolved yet, so the update leaves the
// corresponding field nil (unchanged) rather than clobbering an
// already-stored value with a blank one.
func (s *agentThunderProvisioningService) recordFailure(ctx context.Context, binding models.AgentThunderClient, resolvedAgentID, resolvedClientID string, cause error) {
	attemptsSoFar := binding.AttemptCount + 1
	exhausted := attemptsSoFar >= maxProvisionAttempts

	update := repositories.AgentThunderAttemptUpdate{
		LastError: cause.Error(),
	}
	if resolvedAgentID != "" {
		update.ThunderAgentID = &resolvedAgentID
	}
	if resolvedClientID != "" {
		update.ThunderClientID = &resolvedClientID
	}
	if exhausted {
		update.Status = models.AgentThunderStatusFailed
		// The agent will not get an identity without operator action, so this
		// is recorded rather than left as one more retry log line.
		audit.Record(
			ctx, audit.ActionSystemAgentIdentityExhausted,
			audit.Org(binding.OUID),
			audit.ResourceNamed(audit.ResourceAgentIdentity, binding.AgentName, binding.AgentName),
			audit.Project(binding.ProjectName),
			audit.Environment(binding.EnvironmentName),
			audit.Actor(audit.ActorSystem, systemActorAgentThunderReconciler, ""),
			audit.SurfaceOpt(audit.SurfaceSystem),
			audit.OnBehalfOf(binding.RequestedBy),
			audit.Detail("agentName", binding.AgentName),
			audit.Detail("environment", binding.EnvironmentName),
			audit.Detail("reason", "retries-exhausted"),
		)
	} else {
		update.Status = models.AgentThunderStatusPending
		next := time.Now().Add(provisionRetryDelay)
		update.NextRetryAt = &next
	}

	if err := s.repo.UpdateAfterAttempt(ctx, binding.ID, update); err != nil {
		s.logger.Error("Failed to record agent thunder provisioning failure", "binding_id", binding.ID, "error", err)
	}
}

func (s *agentThunderProvisioningService) RegenerateSecret(ctx context.Context, ouID, projectName, agentName, envName string) (models.AgentProvisioningType, string, string, error) {
	release, err := s.bindingLocks.Lock(ctx, bindingLockKey(ouID, projectName, agentName, envName))
	if err != nil {
		return "", "", "", fmt.Errorf("wait for agent thunder binding lock: %w", err)
	}
	defer release()

	binding, err := s.repo.Get(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		if errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
			return "", "", "", fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
		}
		return "", "", "", err
	}
	if binding.ThunderAgentID == "" {
		return "", "", "", fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
	}

	thunderClient, err := ResolveEnvThunderClient(ctx, s.envResolver, ouID, envName)
	if err != nil {
		return "", "", "", err
	}

	newSecret, err := thunderClient.RegenerateAgentSecret(ctx, binding.ThunderAgentID)
	if err != nil {
		return "", "", "", err
	}

	// External agent secrets are only returned to the caller and never persisted.
	// Internal agent secrets must be stored to be injected into the workload.
	if binding.ProvisioningType != models.AgentProvisioningTypeExternal {
		secretPath, err := s.storeCredential(ctx, ouID, projectName, agentName, envName, binding.ThunderClientID, newSecret)
		if err != nil {
			return "", "", "", fmt.Errorf("store regenerated secret: %w", err)
		}
		if err := s.repo.UpdateSecretRef(ctx, binding.ID, secretPath); err != nil {
			return "", "", "", fmt.Errorf("record regenerated secret location: %w", err)
		}
	}

	return binding.ProvisioningType, binding.ThunderClientID, newSecret, nil
}

// GetAgentRoles returns the Thunder roles assigned to the agent's AgentID in
// one environment.
func (s *agentThunderProvisioningService) GetAgentRoles(ctx context.Context, ouID, projectName, agentName, envName string) ([]thundersvc.ThunderRole, error) {
	binding, err := s.repo.Get(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		if errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
			return nil, fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
		}
		return nil, err
	}
	if binding.ThunderAgentID == "" {
		return nil, fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
	}

	client, err := ResolveEnvThunderIdentity(ctx, s.envResolver, ouID, envName)
	if err != nil {
		return nil, err
	}

	roles, err := client.GetAgentRoles(ctx, binding.ThunderAgentID)
	if err != nil {
		return nil, fmt.Errorf("get agent roles: %w", err)
	}
	return roles, nil
}

// GetAgentGroups returns the Thunder groups the agent's AgentID belongs to in
// one environment.
func (s *agentThunderProvisioningService) GetAgentGroups(ctx context.Context, ouID, projectName, agentName, envName string) ([]thundersvc.ThunderGroup, error) {
	binding, err := s.repo.Get(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		if errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
			return nil, fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
		}
		return nil, err
	}
	if binding.ThunderAgentID == "" {
		return nil, fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
	}

	client, err := ResolveEnvThunderIdentity(ctx, s.envResolver, ouID, envName)
	if err != nil {
		return nil, err
	}

	envOUID, err := client.GetDefaultOUID(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve environment identity provider OU: %w", err)
	}

	groups, err := client.GetAgentGroups(ctx, envOUID, binding.ThunderAgentID)
	if err != nil {
		return nil, fmt.Errorf("get agent groups: %w", err)
	}
	return groups, nil
}

func (s *agentThunderProvisioningService) RevokeSecret(ctx context.Context, ouID, projectName, agentName, envName string) (string, error) {
	release, err := s.bindingLocks.Lock(ctx, bindingLockKey(ouID, projectName, agentName, envName))
	if err != nil {
		return "", fmt.Errorf("wait for agent thunder binding lock: %w", err)
	}
	defer release()

	binding, err := s.repo.Get(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		if errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
			return "", fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
		}
		return "", err
	}
	if binding.ThunderAgentID == "" {
		return "", fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
	}

	thunderClient, err := ResolveEnvThunderClient(ctx, s.envResolver, ouID, envName)
	if err != nil {
		return "", err
	}

	// Rotating without storing the result is deliberate: revoke is a kill
	// switch, not "give me a fresh usable one" — that is what regenerate is for.
	if _, err := thunderClient.RegenerateAgentSecret(ctx, binding.ThunderAgentID); err != nil {
		return "", fmt.Errorf("revoke (rotate) secret: %w", err)
	}

	if binding.SecretRefPath != "" {
		if err := s.deleteCredential(ctx, ouID, projectName, agentName, envName); err != nil {
			// Non-fatal: the stored copy is now stale either way. Leave
			// secret_ref_path set (rather than clearing it below) so a
			// second revoke call reaches this same branch and retries the
			// delete, instead of orphaning the stored secret and its
			// SecretReference CR with no path left pointing at them.
			s.logger.Warn("Failed to delete stored secret during revoke; leaving secret_ref_path set so a re-revoke retries", "binding_id", binding.ID, "error", err)
			return binding.ThunderClientID, nil
		}
		if err := s.repo.UpdateSecretRef(ctx, binding.ID, ""); err != nil {
			return "", err
		}
	}
	return binding.ThunderClientID, nil
}

func (s *agentThunderProvisioningService) GetIdentityViews(ctx context.Context, ouID, projectName, agentName string) ([]models.AgentIdentityEnvironmentView, error) {
	bindings, err := s.repo.FindByAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		return nil, err
	}

	views := make([]models.AgentIdentityEnvironmentView, 0, len(bindings))
	for _, b := range bindings {
		views = append(views, models.AgentIdentityEnvironmentView{
			EnvironmentName:  b.EnvironmentName,
			ProvisioningType: b.ProvisioningType,
			Status:           b.Status,
			AgentID:          b.ThunderAgentID,
			ClientID:         b.ThunderClientID,
			LastError:        b.LastError,
			RequestedBy:      b.RequestedBy,
		})
	}

	return views, nil
}

func (s *agentThunderProvisioningService) GetBindingState(ctx context.Context, ouID, projectName, agentName, envName string) (*AgentThunderBindingState, error) {
	binding, err := s.repo.Get(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		if errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
			return nil, nil //nolint:nilnil // documented "no binding yet" state, see doc comment
		}
		return nil, fmt.Errorf("read agent thunder binding state: %w", err)
	}
	return &AgentThunderBindingState{
		ProvisioningType: binding.ProvisioningType,
		Status:           binding.Status,
		HasSecret:        binding.SecretRefPath != "",
		LastError:        binding.LastError,
	}, nil
}

func (s *agentThunderProvisioningService) DeleteAllBindings(ctx context.Context, ouID, projectName, agentName string) {
	snapshot, err := s.repo.FindByAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent thunder bindings for deletion", "ou_id", ouID, "agent_name", agentName, "error", err)
		return
	}

	// Acquire each binding's lock — the same one AttemptProvision/RegenerateSecret/
	// RevokeSecret hold for their duration — before touching it, then re-fetch its
	// current row rather than trusting the snapshot above. This guarantees any
	// AttemptProvision in flight for a binding has finished, and its Thunder
	// identity and secret are visible, before this method deletes it.
	//
	// Each binding's release is paired 1:1 with its entry in bindings (not
	// collected into one shared defer) so its lock can be released right after
	// THAT binding's own cleanup finishes, further down — not held through
	// every other binding's external cleanup too. The lock still guards this
	// binding's own full lifecycle end-to-end, including its own external
	// cleanup below: releasing it any earlier (e.g. right after DeleteByIDs)
	// would let a concurrent re-provision for the same binding key store a
	// fresh secret at the same deterministic path before this method's own
	// delayed OpenBao delete call runs, deleting that fresh secret instead of
	// the one this method actually snapshotted.
	bindings := make([]models.AgentThunderClient, 0, len(snapshot))
	releases := make([]func(), 0, len(snapshot))
	for _, b := range snapshot {
		release, lockErr := s.bindingLocks.Lock(ctx, bindingLockKey(ouID, projectName, agentName, b.EnvironmentName))
		if lockErr != nil {
			s.logger.Warn("Agent thunder binding deletion cancelled while waiting for binding lock",
				"ou_id", ouID, "binding_id", b.ID, "agent_name", agentName, "env", b.EnvironmentName, "error", lockErr)
			continue
		}

		current, getErr := s.repo.Get(ctx, ouID, projectName, agentName, b.EnvironmentName)
		if getErr != nil {
			release()
			if errors.Is(getErr, repositories.ErrAgentThunderClientNotFound) {
				continue // already gone — e.g. deleted by a concurrent call
			}
			s.logger.Error("Failed to re-fetch agent thunder binding before deletion", "ou_id", ouID, "binding_id", b.ID, "agent_name", agentName, "env", b.EnvironmentName, "error", getErr)
			continue
		}
		bindings = append(bindings, *current)
		releases = append(releases, release)
	}

	// Clear secret_ref_path on every row FIRST — before the row-delete attempt
	// below (which can itself fail) and before any external cleanup. A
	// completed row with a non-empty secret_ref_path is exactly what
	// FindRecentlyCompletedInternal's periodic reconcile sweep looks for; if
	// DeleteByIDs below fails and such a row survives, the reconciler would
	// treat it as a healthy binding and recreate a SecretReference CR pointing
	// at the very OpenBao path this method deletes further down — a phantom
	// resource pointing at nothing. Clearing this field makes injectableBinding
	// treat the row as "nothing valid to inject" (same as a revoked credential)
	// immediately, regardless of what fails afterward. Best-effort: a failure
	// here only re-opens the (already time-bounded, see
	// identityInjectionReconcileWindow) stale-row window, it never blocks the
	// rest of this cleanup.
	for _, b := range bindings {
		if err := s.repo.UpdateSecretRef(ctx, b.ID, ""); err != nil {
			s.logger.Warn("Failed to clear agent thunder binding secret ref before deletion", "ou_id", ouID, "binding_id", b.ID, "agent_name", agentName, "env", b.EnvironmentName, "error", err)
		}
	}

	// Delete the rows right after snapshotting them, by ID — before the slow
	// per-environment Thunder cleanup below, not after. Upsert's OnConflict
	// DoNothing means a same-named agent recreated while an old row is still
	// present silently gets no fresh row at all, not just a wiped one — doing
	// this late (or by agent name instead of ID) leaves that window open for
	// as long as the Thunder calls below take. A crash between here and the
	// Thunder cleanup can orphan a Thunder identity, an accepted, harmless
	// tradeoff already made elsewhere in this file.
	ids := make([]uuid.UUID, 0, len(bindings))
	for _, b := range bindings {
		ids = append(ids, b.ID)
	}
	if err := s.repo.DeleteByIDs(ctx, ids); err != nil {
		// Do NOT return here: by the time this method runs, the agent's own
		// primary record (the OpenChoreo Component) is already gone from the
		// caller's perspective — this is best-effort cleanup of everything
		// ELSE the agent left behind. A failed DB-row delete leaves a few
		// stale local rows behind (now with secret_ref_path already cleared
		// above, so they can't be mistaken for a healthy binding); bailing out
		// here would ALSO skip deleting the live Thunder identities,
		// SecretReference CRs, and OpenBao secrets below — external,
		// still-active resources whose leak is a materially worse outcome (an
		// orphaned Thunder identity can still mint valid tokens indefinitely)
		// than a few dangling database rows.
		s.logger.Error("Failed to delete agent thunder client rows; continuing to clean up external resources anyway", "ou_id", ouID, "agent_name", agentName, "error", err)
	}

	// Each binding's lock releases as soon as ITS OWN external cleanup below
	// finishes (via the inline defer), not after every other binding's —
	// otherwise a binding with a fast cleanup would still sit blocked behind
	// however long its siblings' Thunder/OpenBao calls take, even though
	// nothing about it depends on them.
	injector := s.getWorkloadInjector()
	for i, b := range bindings {
		func() {
			defer releases[i]()

			// The AgentID SecretReference CR is independent of whether the identity
			// ever landed in Thunder — clean it up for every internal binding, even
			// ones that never completed (a crash between storeCredential creating it
			// and UpdateAfterAttempt persisting ThunderAgentID/SecretRefPath would
			// otherwise leave it permanently orphaned, since it precedes the
			// early-return below).
			if b.ProvisioningType == models.AgentProvisioningTypeInternal {
				if injector == nil {
					s.logger.Warn("Skipping agent identity SecretReference cleanup: no workload injector configured",
						"ou_id", ouID, "binding_id", b.ID, "agent_name", agentName, "env", b.EnvironmentName)
				} else if err := injector.CleanupForEnvironment(ctx, ouID, agentName, b.EnvironmentName); err != nil {
					s.logger.Warn("Failed to clean up agent identity data-plane SecretReference", "ou_id", ouID, "binding_id", b.ID, "agent_name", agentName, "env", b.EnvironmentName, "error", err)
				}
			}
			if b.ThunderAgentID == "" {
				return // never made it to Thunder — nothing to delete there
			}
			thunderClient, err := ResolveEnvThunderClient(ctx, s.envResolver, ouID, b.EnvironmentName)
			if err != nil {
				s.logger.Warn("Env-thunder resolver error during agent binding cleanup", "ou_id", ouID, "binding_id", b.ID, "agent_name", agentName, "env", b.EnvironmentName, "error", err)
				return
			}
			if _, err := thunderClient.DeleteAgentIdentity(ctx, b.ThunderAgentID); err != nil {
				s.logger.Warn("Failed to delete Thunder agent identity", "ou_id", ouID, "binding_id", b.ID, "agent_name", agentName, "env", b.EnvironmentName, "error", err)
			}
			if b.SecretRefPath != "" {
				if err := s.deleteCredential(ctx, ouID, projectName, agentName, b.EnvironmentName); err != nil {
					s.logger.Warn("Failed to delete stored agent secret", "ou_id", ouID, "binding_id", b.ID, "agent_name", agentName, "env", b.EnvironmentName, "error", err)
				}
			}
		}()
	}
}

// systemActorAgentThunderReconciler identifies the AgentID provisioning
// reconciler in audit records it produces on its own initiative.
const systemActorAgentThunderReconciler = "system:agent-thunder-reconciler"
