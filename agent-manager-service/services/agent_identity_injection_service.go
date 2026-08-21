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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// releaseBindingUpdateRetries/releaseBindingUpdateRetryDelay bound the retry
// of a ReleaseBinding/Workload env-var mutation. UpdateReleaseBindingEnvVars
// (and its Remove/RemoveWorkloadEnvVars siblings) do a plain Get-then-Update
// with no built-in retry, so a concurrent writer touching the SAME
// ReleaseBinding at the same moment (e.g. a user calling
// UpdateAgentConfigurations while this service injects/refreshes/removes
// identity vars) can lose an optimistic-concurrency race. Without a retry
// here, that loser's change is silently dropped with no recovery until the
// next unrelated deploy/promote/rotation happens to re-assert it.
const (
	releaseBindingUpdateRetries    = 3
	releaseBindingUpdateRetryDelay = 500 * time.Millisecond
)

// withReleaseBindingRetry retries fn a bounded number of times, but only when
// fn fails with utils.ErrConflict or utils.ErrInternalServerError — a stale
// resourceVersion race that OpenChoreo currently surfaces as a generic 500
// rather than a 409. Any other error (validation, auth, not-found) is
// permanent and returned immediately. Honors ctx cancellation so a doomed or
// already-cancelled request-path call doesn't block a handler goroutine
// sleeping between attempts.
func withReleaseBindingRetry(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt < releaseBindingUpdateRetries; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if !errors.Is(lastErr, utils.ErrConflict) && !errors.Is(lastErr, utils.ErrInternalServerError) {
			return lastErr
		}
		if attempt < releaseBindingUpdateRetries-1 {
			select {
			case <-ctx.Done():
				return lastErr
			case <-time.After(releaseBindingUpdateRetryDelay):
			}
		}
	}
	return lastErr
}

// agentIdentityEnvVarKeySet is the fixed set of env var names owned by
// AgentID credential injection. Never mutated after init — every caller of
// AgentIdentityEnvVarKeys/mergeAgentIdentityEnvVarKeys shares this one map
// instead of each allocating its own copy.
var agentIdentityEnvVarKeySet = map[string]bool{
	client.EnvVarAgentIDClientID:      true,
	client.EnvVarAgentIDClientSecret:  true,
	client.EnvVarAgentIDTokenEndpoint: true,
	client.EnvVarAgentIDScopes:        true,
}

// AgentIdentityEnvVarKeys returns the set of env var names owned by AgentID
// credential injection. Callers that rewrite an agent's env vars from user
// input use this to filter user-echoed copies (the console reads
// configurations back, so these keys can legitimately round-trip in requests)
// before re-appending the canonical values. The returned map is shared and
// must never be mutated by callers — use mergeAgentIdentityEnvVarKeys to
// fold these keys into a caller-owned map instead.
func AgentIdentityEnvVarKeys() map[string]bool {
	return agentIdentityEnvVarKeySet
}

// mergeAgentIdentityEnvVarKeys adds every AgentID-owned env var key into dst,
// allocating it first if nil, and returns it — the repeated "filter
// user-echoed identity keys into a system-managed set" step several callers
// in agent_manager.go each need.
func mergeAgentIdentityEnvVarKeys(dst map[string]bool) map[string]bool {
	if dst == nil {
		dst = make(map[string]bool, len(agentIdentityEnvVarKeySet))
	}
	for k := range agentIdentityEnvVarKeySet {
		dst[k] = true
	}
	return dst
}

// AgentIdentityInjectionService delivers an internal agent's AgentID
// credentials (Thunder OAuth2 client) into its running workload — the
// "Gateway Binding" phase of the AgentID feature. It never handles the secret
// VALUE itself: the pod receives the client secret through a SecretKeyRef
// into a Kubernetes Secret that OpenChoreo materializes, in the agent's own
// workload namespace, from a data-plane SecretReference this service owns
// (ensureSecretReference) — pointed at the remote KV key
// agentThunderProvisioningService.storeCredential resolved once, at
// credential creation/rotation, and persisted in binding.SecretRefPath (see
// its doc comment). This service never independently resolves or guesses
// that KV key — it only ever reads binding.SecretRefPath, so it never repeats
// a live OpenChoreo round trip on its own (much more frequent) injection
// path.
//
// This service is wired unconditionally (see wiring.ProvideAgentIdentityInjectionService),
// independent of which AgentThunderProvisioningService implementation a
// deployment plugs in.
//
// External agents are never injected — they run outside the platform and
// generate their own credentials on demand instead. Every method here
// silently no-ops for them.
type AgentIdentityInjectionService interface {
	// EnvVarsForEnvironment returns the identity env vars for one internal
	// agent in one environment, ensuring the backing data-plane
	// SecretReference exists first. Returns (nil, nil) when there is nothing
	// to inject: no binding, provisioning not completed, an external agent,
	// or a revoked credential. Callers treat nil as "skip identity
	// injection" — a normal state, not an error. A non-nil error means the
	// current state could not be determined (or the SecretReference could
	// not be ensured) and the caller must NOT proceed with an env-var
	// rewrite that would silently drop the vars.
	EnvVarsForEnvironment(ctx context.Context, ouID, projectName, agentName, envName string) ([]client.EnvVar, error)

	// InjectForEnvironment pushes the identity env vars into the agent's live
	// workload for one environment (ReleaseBinding merge + pod rollout).
	// No-op when EnvVarsForEnvironment returns nothing, and when the agent is
	// not deployed in that environment yet (the deploy flow injects at deploy
	// time). Used by the live-refresh hooks (MCP config / proxy scope changes)
	// and, via ReconcileForEnvironment, by the injection reconciler.
	InjectForEnvironment(ctx context.Context, ouID, projectName, agentName, envName string) error

	// ReconcileForEnvironment idempotently brings the agent's live workload in
	// line with its desired AgentID env vars, writing ONLY when they are
	// missing or the scope list drifted — so it never causes a needless pod
	// rollout (mirrors the LLM/MCP paths, which also skip an unchanged
	// ReleaseBinding update). Reads the current env vars back via
	// GetComponentConfigurations to decide. Safe to run every reconciler tick:
	// this is what lands credentials on a brand-new git agent whose
	// ReleaseBinding doesn't exist yet when provisioning completes, the moment
	// its first build creates the workload. No-op for external/unprovisioned/
	// revoked bindings and for not-yet-deployed environments.
	ReconcileForEnvironment(ctx context.Context, ouID, projectName, agentName, envName string) error

	// RefreshAfterRotation re-asserts the data-plane SecretReference with a
	// fresh rotated-at annotation and rolls the pod once the secret-store sync
	// has had time to catch up. Rotation never changes the SecretReference's
	// remote KV key (storeCredential's resolved location is stable — a
	// rotation only updates the value in place), so this never touches
	// binding.SecretRefPath itself. No-op for external agents and
	// unprovisioned bindings.
	RefreshAfterRotation(ctx context.Context, ouID, projectName, agentName, envName string) error

	// RemoveForEnvironment removes the identity env vars from the agent's
	// workload for one environment and deletes the backing data-plane
	// SecretReference — used after a revoke, so the pod does not keep
	// serving a credential that can no longer mint tokens.
	// includeWorkloadLevel additionally removes the vars from the shared
	// Workload CR; callers set it only when envName is the pipeline's lowest
	// environment (the deploy flow writes that environment's vars at Workload
	// level, shared by all environments — removing them while revoking a
	// DIFFERENT environment's credential would break the lowest environment's
	// pod).
	RemoveForEnvironment(ctx context.Context, ouID, projectName, agentName, envName string, includeWorkloadLevel bool) error

	// CleanupForEnvironment deletes the AgentID data-plane SecretReference
	// for one (agent, environment) — used on agent deletion, where the
	// workload itself is being deleted so env var removal is pointless but
	// the SecretReference would leak. Best-effort: not-found is success.
	CleanupForEnvironment(ctx context.Context, ouID, agentName, envName string) error
}

// AgentIdentityShutdownContextSetter is an optional capability an
// AgentIdentityInjectionService implementation MAY expose — deliberately not
// part of the interface above, mirroring WorkloadInjectorSetter's pattern
// (see its doc comment) for the same reason: an alternative implementation
// with no deferred background work simply doesn't implement this. app.Run
// type-asserts for it once at startup and provides the app's shutdown
// context, so a RefreshAfterRotation rollout still waiting or in flight when
// the app starts shutting down stops rather than firing an outbound update
// afterward.
type AgentIdentityShutdownContextSetter interface {
	SetShutdownContext(ctx context.Context)
}

type agentIdentityInjectionService struct {
	repo              repositories.AgentThunderClientRepository
	agentConfigRepo   repositories.AgentConfigurationRepository
	mcpProxyScopeRepo repositories.MCPProxyScopeRepository
	ocClient          client.OpenChoreoClient
	refreshInterval   string
	logger            *slog.Logger
	// now is injectable for tests; defaults to time.Now.
	now func() time.Time
	// after is injectable for tests; defaults to time.After. See
	// RefreshAfterRotation's doc comment for why it waits at all.
	after func(time.Duration) <-chan time.Time
	// rolloutTokens/rolloutTokensMu let RefreshAfterRotation tell a stale
	// deferred roll apart from the latest one, per binding — see
	// RefreshAfterRotation's doc comment.
	rolloutTokensMu sync.Mutex
	rolloutTokens   map[string]uint64
	// shutdownCtx/shutdownCtxMu are set via SetShutdownContext; default to
	// context.Background() (never cancelled) so behavior is unchanged
	// wherever that setter is never called, e.g. every existing test.
	shutdownCtxMu sync.Mutex
	shutdownCtx   context.Context
}

// NewAgentIdentityInjectionService creates a new AgentIdentityInjectionService.
// refreshInterval is the data-plane SecretReference's refresh cadence (same
// value secretmanagersvc uses elsewhere, e.g. "1h").
func NewAgentIdentityInjectionService(
	repo repositories.AgentThunderClientRepository,
	agentConfigRepo repositories.AgentConfigurationRepository,
	mcpProxyScopeRepo repositories.MCPProxyScopeRepository,
	ocClient client.OpenChoreoClient,
	refreshInterval string,
	logger *slog.Logger,
) AgentIdentityInjectionService {
	return &agentIdentityInjectionService{
		repo:              repo,
		agentConfigRepo:   agentConfigRepo,
		mcpProxyScopeRepo: mcpProxyScopeRepo,
		ocClient:          ocClient,
		refreshInterval:   refreshInterval,
		logger:            logger,
		now:               time.Now,
		after:             time.After,
		rolloutTokens:     make(map[string]uint64),
		shutdownCtx:       context.Background(),
	}
}

// SetShutdownContext implements AgentIdentityShutdownContextSetter.
func (s *agentIdentityInjectionService) SetShutdownContext(ctx context.Context) {
	s.shutdownCtxMu.Lock()
	s.shutdownCtx = ctx
	s.shutdownCtxMu.Unlock()
}

func (s *agentIdentityInjectionService) currentShutdownContext() context.Context {
	s.shutdownCtxMu.Lock()
	defer s.shutdownCtxMu.Unlock()
	return s.shutdownCtx
}

// defaultSecretSyncWait is the fallback wait when refreshInterval is unset or
// invalid — defensive only, should not normally trigger.
const defaultSecretSyncWait = 30 * time.Second

// secretSyncWaitDuration parses the SecretReference refresh cadence
// (refreshInterval) into a wait duration for RefreshAfterRotation.
func secretSyncWaitDuration(refreshInterval string) time.Duration {
	d, err := time.ParseDuration(refreshInterval)
	if err != nil || d <= 0 {
		return defaultSecretSyncWait
	}
	return d
}

// resolveAgentIdentityScopes returns the full set of OAuth2 scopes the agent
// should request when minting a token: the union of every scope defined on
// EVERY MCP proxy this agent is bound to in this environment (see
// models.MCPProxyScope — a proxy-level action string, not tied to a specific
// environment) — sourced entirely from AMS's own DB, no Thunder role/group
// lookups. Requesting a scope the AgentID isn't actually authorized for is
// safe: Thunder filters requested scopes down to what the agent's role
// assignments actually grant at token-mint time, so this is a "what might I
// need" list, not the enforcement point.
//
// Each MCP proxy an agent is configured with is stored as its OWN
// AgentConfiguration row (see createMCPConfig) — an agent bound to two
// proxies has two rows, both TypeID=AgentConfigTypeIDMCP. This reads ALL of
// them via ListMCPConfigsByAgent and unions their EnvMCPMappings; it must
// NOT use GetByAgentID, which returns a single arbitrary row and would
// silently drop every proxy but one whenever an agent has more than one MCP
// configuration.
//
// Returns an error on a genuine lookup failure (DB/OpenChoreo) instead of
// silently falling back to no scopes: every caller of this service already
// aborts on an error (e.g. a binding-load failure aborts the deploy/promote/
// config-update "to prevent credential loss" — see agent_manager.go), and
// ReconcileForEnvironment's "never causes a needless pod rollout" guarantee
// depends on comparing a TRUSTWORTHY desired scope list against the live
// one — a transient blip silently resolving to an empty list would look like
// a real scope change, causing one rollout to empty scopes and a second
// rollout back once the blip cleared. Since a wrong-but-non-empty scope
// request is no less safe than an empty one (Thunder still filters it at
// mint time), swallowing the error here bought no real safety.
func (s *agentIdentityInjectionService) resolveAgentIdentityScopes(ctx context.Context, binding *models.AgentThunderClient) ([]string, error) {
	configs, err := s.agentConfigRepo.ListMCPConfigsByAgent(ctx, binding.OUID, binding.ProjectName, binding.AgentName)
	if err != nil {
		return nil, fmt.Errorf("resolve agent identity scopes: list mcp configurations: %w", err)
	}
	if len(configs) == 0 {
		return nil, nil // no MCP configurations at all — skip resolving the environment UUID entirely
	}

	env, err := s.ocClient.GetEnvironment(ctx, binding.OUID, binding.EnvironmentName)
	if err != nil {
		return nil, fmt.Errorf("resolve agent identity scopes: resolve environment: %w", err)
	}

	// Proxy handles are needed to build each scope row's token string (see
	// models.MCPProxyScope.ScopeString) — read from each mapping's own
	// preloaded proxy (ListMCPConfigsByAgent preloads EnvMCPMappings.MCPProxy.Artifact)
	// rather than a second lookup. Aggregated across every MCP config row,
	// not just the first one.
	handleByProxy := map[uuid.UUID]string{}
	for _, config := range configs {
		for _, mapping := range config.EnvMCPMappings {
			if mapping.EnvironmentUUID.String() != env.UUID || mapping.MCPProxy == nil {
				continue
			}
			handleByProxy[mapping.MCPProxyUUID] = proxyHandleOf(mapping.MCPProxy)
		}
	}
	if len(handleByProxy) == 0 {
		return nil, nil
	}

	proxyUUIDs := make([]uuid.UUID, 0, len(handleByProxy))
	for id := range handleByProxy {
		proxyUUIDs = append(proxyUUIDs, id)
	}
	scopeRows, err := s.mcpProxyScopeRepo.ListByProxyUUIDs(ctx, proxyUUIDs)
	if err != nil {
		return nil, fmt.Errorf("resolve agent identity scopes: list mcp proxy scopes: %w", err)
	}

	scopeSet := map[string]struct{}{}
	for _, sc := range scopeRows {
		handle := handleByProxy[sc.MCPProxyUUID]
		if handle == "" {
			continue
		}
		scopeSet[sc.ScopeString(handle)] = struct{}{}
	}
	if len(scopeSet) == 0 {
		return nil, nil
	}
	scopes := make([]string, 0, len(scopeSet))
	for scope := range scopeSet {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	return scopes, nil
}

// injectableBinding returns the binding when it is in an injectable state, or
// nil when there is (legitimately) nothing to inject for this agent+env.
func (s *agentIdentityInjectionService) injectableBinding(ctx context.Context, ouID, projectName, agentName, envName string) (*models.AgentThunderClient, error) {
	binding, err := s.repo.Get(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		if errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
			return nil, nil //nolint:nilnil // documented "nothing to inject" state, checked by every caller
		}
		return nil, fmt.Errorf("read agent thunder binding for identity injection: %w", err)
	}
	if binding.ProvisioningType != models.AgentProvisioningTypeInternal {
		return nil, nil //nolint:nilnil // external agents are never injected
	}
	if binding.Status != models.AgentThunderStatusCompleted || binding.ThunderClientID == "" {
		return nil, nil //nolint:nilnil // provisioning not (successfully) finished yet
	}
	if binding.SecretRefPath == "" {
		return nil, nil //nolint:nilnil // credential revoked — nothing valid to inject
	}
	return binding, nil
}

// secretRotatedAtAnnotation/secretRotatedAtFormat stamp a fresh value on the
// SecretReference's Secret template on every rotation, marking its spec as
// changed. This alone does not force the secret-store sync ahead of its own
// refresh cadence — see RefreshAfterRotation for how the wait is handled.
const (
	secretRotatedAtAnnotation = "amp.wso2.com/secret-rotated-at"
	secretRotatedAtFormat     = time.RFC3339Nano
)

// ensureSecretReference creates or updates the data-plane SecretReference CR
// that lets OpenChoreo materialize the stored credential as a Kubernetes
// Secret in the agent's own workload namespace. templateAnnotations may be
// nil; see secretRotatedAtAnnotation for when it is not.
//
// KVPath comes directly from binding.SecretRefPath — the remote key
// agentThunderProvisioningService.storeCredential resolved once, at
// credential creation/rotation, by reading the SecretReference CreateSecret
// itself manages back from OpenChoreo (see storeCredential's doc comment).
// This method never independently computes or re-resolves that value; it
// only ever asserts the CR's spec from whatever's already in the DB, so it
// carries no live-read dependency of its own — safe to call on every
// deploy/promote/config-update, and from the reconciler on every tick.
func (s *agentIdentityInjectionService) ensureSecretReference(ctx context.Context, binding *models.AgentThunderClient, templateAnnotations map[string]string) (string, error) {
	location := agentIdentitySecretLocation(binding.OUID, binding.ProjectName, binding.AgentName, binding.EnvironmentName)
	refName := location.SecretRefName()
	req := client.CreateSecretReferenceRequest{
		Namespace:           binding.OUID,
		Name:                refName,
		ProjectName:         binding.ProjectName,
		ComponentName:       binding.AgentName,
		KVPath:              binding.SecretRefPath,
		SecretKeys:          []string{thundersvc.AgentSecretKeyClientSecret},
		RefreshInterval:     s.refreshInterval,
		TemplateAnnotations: templateAnnotations,
	}
	if _, err := s.ocClient.CreateSecretReference(ctx, binding.OUID, req); err != nil {
		// A concurrent caller (another request for this same binding, or the
		// reconciler) may have already created it — fall back to asserting
		// the same spec via update rather than treating this as fatal.
		if !errors.Is(err, utils.ErrConflict) {
			return "", fmt.Errorf("create agent identity SecretReference %q: %w", refName, err)
		}
		if _, err := s.ocClient.UpdateSecretReference(ctx, binding.OUID, refName, req); err != nil {
			return "", fmt.Errorf("update agent identity SecretReference %q after create conflict: %w", refName, err)
		}
	}
	return refName, nil
}

// deleteSecretReference deletes the data-plane SecretReference for one
// binding. Best-effort by convention of every caller: not-found is success.
func (s *agentIdentityInjectionService) deleteSecretReference(ctx context.Context, ouID, projectName, agentName, envName string) error {
	refName := agentIdentitySecretLocation(ouID, projectName, agentName, envName).SecretRefName()
	if err := s.ocClient.DeleteSecretReference(ctx, ouID, refName); err != nil && !errors.Is(err, utils.ErrNotFound) {
		return fmt.Errorf("delete agent identity SecretReference %q: %w", refName, err)
	}
	return nil
}

// buildEnvVars assembles the four identity env vars for one binding. The
// token endpoint uses the cluster-internal env-Thunder URL: env-Thunder is not
// published on the gateway vhost, so pods reach it by in-cluster address.
//
// The URL is built from ThunderOrgNamespace(), NOT binding.OUID: env-Thunder
// is addressed by the org's namespace/handle (e.g. "default"), and binding.OUID
// is the OU UUID from the JWT — passing it here would target a K8s Service
// that doesn't exist. See ThunderOrgNamespace's doc comment for why this is a
// config-pinned value shared with agentThunderProvisioningService, not an
// independent lookup.
func (s *agentIdentityInjectionService) buildEnvVars(ctx context.Context, binding *models.AgentThunderClient, secretRefName string) ([]client.EnvVar, error) {
	scopes, err := s.resolveAgentIdentityScopes(ctx, binding)
	if err != nil {
		return nil, err
	}
	return []client.EnvVar{
		{Key: client.EnvVarAgentIDClientID, Value: binding.ThunderClientID},
		{
			Key: client.EnvVarAgentIDClientSecret,
			ValueFrom: &client.EnvVarValueFrom{
				SecretKeyRef: &client.SecretKeyRef{
					Name: secretRefName,
					Key:  thundersvc.AgentSecretKeyClientSecret,
				},
			},
		},
		{Key: client.EnvVarAgentIDTokenEndpoint, Value: thundersvc.ThunderTokenURL(ThunderOrgNamespace(), binding.EnvironmentName)},
		{Key: client.EnvVarAgentIDScopes, Value: strings.Join(scopes, " ")},
	}, nil
}

func (s *agentIdentityInjectionService) envVarsForEnvironment(ctx context.Context, ouID, projectName, agentName, envName string, templateAnnotations map[string]string) ([]client.EnvVar, error) {
	binding, err := s.injectableBinding(ctx, ouID, projectName, agentName, envName)
	if err != nil || binding == nil {
		return nil, err
	}
	refName, err := s.ensureSecretReference(ctx, binding, templateAnnotations)
	if err != nil {
		return nil, err
	}
	return s.buildEnvVars(ctx, binding, refName)
}

func (s *agentIdentityInjectionService) EnvVarsForEnvironment(ctx context.Context, ouID, projectName, agentName, envName string) ([]client.EnvVar, error) {
	return s.envVarsForEnvironment(ctx, ouID, projectName, agentName, envName, nil)
}

func (s *agentIdentityInjectionService) InjectForEnvironment(ctx context.Context, ouID, projectName, agentName, envName string) error {
	envVars, err := s.EnvVarsForEnvironment(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		return err
	}
	if len(envVars) == 0 {
		s.logger.Debug("No agent identity env vars to inject", "agent_name", agentName, "env_name", envName)
		return nil
	}
	// Merges into the ReleaseBinding's workload overrides and stamps
	// restartedAt for a pod rollout; silently no-ops when the agent is not
	// deployed in this environment yet (kind-sourced agents, which have no
	// ReleaseBinding, pick the vars up on their next deploy instead).
	if err := withReleaseBindingRetry(ctx, func() error {
		return s.ocClient.UpdateReleaseBindingEnvVars(ctx, ouID, projectName, agentName, envName, envVars)
	}); err != nil {
		return fmt.Errorf("inject agent identity env vars into release binding: %w", err)
	}
	s.logger.Info("Injected agent identity env vars", "agent_name", agentName, "env_name", envName)
	return nil
}

func (s *agentIdentityInjectionService) ReconcileForEnvironment(ctx context.Context, ouID, projectName, agentName, envName string) error {
	desired, err := s.EnvVarsForEnvironment(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		return err
	}
	if len(desired) == 0 {
		// External/unprovisioned/revoked — nothing this service should inject.
		return nil
	}

	current, err := s.ocClient.GetComponentConfigurations(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		return fmt.Errorf("read current agent env vars for identity reconcile: %w", err)
	}
	if identityEnvVarsInSync(desired, current) {
		// Live workload already carries the identity vars with the current
		// scope list — writing would only stamp a needless pod rollout.
		return nil
	}

	// Something is missing or stale (most commonly: the workload just came up
	// from a first build that finished after provisioning, so it has none of
	// the identity vars yet). InjectForEnvironment writes them and rolls the
	// pod; it still safely no-ops if the ReleaseBinding does not exist yet.
	return s.InjectForEnvironment(ctx, ouID, projectName, agentName, envName)
}

// identityEnvVarsInSync reports whether the live env vars already carry every
// AgentID key AND the scope list already matches desired. Client ID / secret
// ref / token endpoint are stable for a given (agent, environment) once set
// (rotation re-asserts them through RefreshAfterRotation, a separate path), so
// their mere presence is enough; the scope list is the value that legitimately
// changes over time, so it is compared exactly. Absent the workload entirely
// (nothing read back), this returns false so the caller attempts an inject —
// which itself no-ops when there is no ReleaseBinding to write to.
func identityEnvVarsInSync(desired []client.EnvVar, current []models.EnvVars) bool {
	currentByKey := make(map[string]models.EnvVars, len(current))
	for _, ev := range current {
		currentByKey[ev.Key] = ev
	}
	for _, want := range desired {
		got, ok := currentByKey[want.Key]
		if !ok {
			return false
		}
		// The scope list is the only identity var whose value changes in place;
		// compare it exactly so scope edits re-inject.
		if want.Key == client.EnvVarAgentIDScopes && got.Value != want.Value {
			return false
		}
	}
	return true
}

// RefreshAfterRotation re-asserts a rotated internal agent's credential and
// rolls its pod so it picks up the new value.
//
// The SecretReference update runs synchronously and its error is returned.
// The pod roll is deferred: it waits out refreshInterval first, because the
// secret-store sync only re-fetches a rotated value on its own cadence, not
// immediately when the SecretReference's spec changes. Rolling immediately
// used to lose that race — the new pod started before the sync caught up and
// kept the stale, already-invalidated secret for its whole lifetime
// (secretKeyRef env vars resolve once at container start, never hot-reload).
// The roll itself runs detached so a manual regenerate call never blocks on
// the wait; a roll failure is only logged, not returned. The wait, and the
// roll itself once started, both stop early if the app starts shutting down
// (see AgentIdentityShutdownContextSetter) instead of firing an outbound
// update after the app has begun tearing down.
//
// Rotations for the same binding in quick succession each get their own
// deferred roll, but only the latest one is allowed to actually run —
// otherwise every rotation restarts the pod, even ones already superseded by
// a later rotation before their own wait finished.
func (s *agentIdentityInjectionService) RefreshAfterRotation(ctx context.Context, ouID, projectName, agentName, envName string) error {
	annotations := map[string]string{
		secretRotatedAtAnnotation: s.now().UTC().Format(secretRotatedAtFormat),
	}
	envVars, err := s.envVarsForEnvironment(ctx, ouID, projectName, agentName, envName, annotations)
	if err != nil {
		return err
	}
	if len(envVars) == 0 {
		return nil
	}

	key := bindingLockKey(ouID, projectName, agentName, envName)
	s.rolloutTokensMu.Lock()
	s.rolloutTokens[key]++
	myToken := s.rolloutTokens[key]
	s.rolloutTokensMu.Unlock()

	wait := secretSyncWaitDuration(s.refreshInterval)
	shutdownCtx := s.currentShutdownContext()
	go func() {
		select {
		case <-s.after(wait):
		case <-shutdownCtx.Done():
			return // app is shutting down; abandon this deferred rollout
		}

		s.rolloutTokensMu.Lock()
		isLatest := s.rolloutTokens[key] == myToken
		s.rolloutTokensMu.Unlock()
		if !isLatest {
			return // a later rotation for this binding superseded this roll
		}

		// context.WithoutCancel keeps ctx's values (e.g. the request's
		// correlation ID, for this call's own logging) without inheriting
		// its cancellation — the roll must outlive the request that
		// triggered it. shutdownCtx below is the only thing allowed to cut
		// it short: a small bridge goroutine cancels refreshCtx if shutdown
		// fires before the roll (including its retries) finishes.
		refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseBindingUpdateRetries*releaseBindingUpdateRetryDelay+30*time.Second)
		defer cancel()
		bridgeDone := make(chan struct{})
		defer close(bridgeDone)
		go func() {
			select {
			case <-shutdownCtx.Done():
				cancel()
			case <-bridgeDone:
			}
		}()

		// Re-merging identical env vars is a no-op content-wise, but the call
		// also stamps restartedAt on the ReleaseBinding — the pod rollout that
		// makes the workload actually pick up the refreshed Secret.
		if err := withReleaseBindingRetry(refreshCtx, func() error {
			return s.ocClient.UpdateReleaseBindingEnvVars(refreshCtx, ouID, projectName, agentName, envName, envVars)
		}); err != nil {
			s.logger.Warn("Failed to roll out pod after agent identity secret rotation", "agent_name", agentName, "env_name", envName, "error", err)
			return
		}
		s.logger.Info("Refreshed agent identity credentials in workload after rotation", "agent_name", agentName, "env_name", envName)
	}()
	return nil
}

func (s *agentIdentityInjectionService) RemoveForEnvironment(ctx context.Context, ouID, projectName, agentName, envName string, includeWorkloadLevel bool) error {
	binding, err := s.repo.Get(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		if errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
			return nil
		}
		return fmt.Errorf("read agent thunder binding for identity removal: %w", err)
	}
	if binding.ProvisioningType != models.AgentProvisioningTypeInternal {
		return nil
	}

	identityKeys := AgentIdentityEnvVarKeys()
	keys := make([]string, 0, len(identityKeys))
	for k := range identityKeys {
		keys = append(keys, k)
	}

	// Remove from the per-environment ReleaseBinding overrides (idempotent —
	// nil when not deployed) and roll the pod.
	if err := withReleaseBindingRetry(ctx, func() error {
		return s.ocClient.RemoveReleaseBindingEnvVars(ctx, ouID, projectName, agentName, envName, keys)
	}); err != nil {
		return fmt.Errorf("remove agent identity env vars from release binding: %w", err)
	}
	// The deploy flow writes the lowest environment's vars at Workload level,
	// shared across environments — only strip those when we're actually
	// revoking that environment's credential.
	if includeWorkloadLevel {
		if err := withReleaseBindingRetry(ctx, func() error {
			return s.ocClient.RemoveWorkloadEnvVars(ctx, ouID, agentName, keys)
		}); err != nil {
			return fmt.Errorf("remove agent identity env vars from workload: %w", err)
		}
	}

	if err := s.deleteSecretReference(ctx, ouID, projectName, agentName, envName); err != nil {
		return fmt.Errorf("delete agent identity SecretReference during removal: %w", err)
	}

	s.logger.Info("Removed agent identity env vars from workload", "agent_name", agentName, "env_name", envName, "include_workload_level", includeWorkloadLevel)
	return nil
}

func (s *agentIdentityInjectionService) CleanupForEnvironment(ctx context.Context, ouID, agentName, envName string) error {
	// projectName is not part of SecretRefName's computation (see
	// SecretLocation.SecretRefName) — this interface method doesn't carry
	// one, matching agentThunderProvisioningService's call sites.
	return s.deleteSecretReference(ctx, ouID, "", agentName, envName)
}
