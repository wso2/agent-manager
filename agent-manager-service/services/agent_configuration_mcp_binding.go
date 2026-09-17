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
	"sync"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// mcpActivationInputs holds the per-config invariants every environment activation needs.
// Assembling them costs two remote calls and a template rebuild, so they are gathered once
// per config rather than per environment.
type mcpActivationInputs struct {
	envTemplates    []EnvConfigTemplate
	isExternalAgent bool
	firstEnvName    string
}

// soleMCPProxyUUID returns the one proxy every requested environment maps to, or nil when
// the environments disagree (or none were requested). It is what AgentConfiguration.
// MCPProxyUUID stores: the environment-agnostic proxy reference, recorded only when the
// request actually expresses one.
func soleMCPProxyUUID(proxiesByEnv map[string]*models.MCPProxy) *uuid.UUID {
	var sole *uuid.UUID
	for _, proxy := range proxiesByEnv {
		if proxy == nil {
			return nil
		}
		if sole == nil {
			proxyUUID := proxy.UUID
			sole = &proxyUUID
			continue
		}
		if *sole != proxy.UUID {
			return nil
		}
	}
	return sole
}

// setConfigMCPProxy persists a change to the config's environment-agnostic proxy reference,
// and is a no-op when it already holds that value — the steady-state update path must not
// write a row just to store what is already there.
func (s *agentConfigurationService) setConfigMCPProxy(
	ctx context.Context, config *models.AgentConfiguration, proxyUUID *uuid.UUID,
) error {
	if samePtrUUID(config.MCPProxyUUID, proxyUUID) {
		return nil
	}
	config.MCPProxyUUID = proxyUUID
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return s.agentConfigRepo.Update(ctx, tx, config)
	}); err != nil {
		return fmt.Errorf("failed to record MCP proxy reference on configuration %s: %w", config.UUID, err)
	}
	return nil
}

func samePtrUUID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// mcpEnvsNeedingActivation returns the environments where proxy could back a binding for
// this connection but no EnvAgentMCPMapping row exists yet.
//
// An environment qualifies only if it satisfies BOTH halves:
//
//   - The PROXY serves it (an endpoint→environment row). Nothing can be bound in an
//     environment the proxy has no endpoint in.
//   - The CONNECTION is offered there (env var rows exist). Those rows are the
//     connection's own record of its environment scope: configuring it writes them for
//     every requested environment, blank when the proxy is not deployable there yet, and
//     removeMCPMappingEnvironment deletes them when an environment is taken out of scope.
//
// The second half is what keeps this reconcile from overriding an explicit removal.
// `amctl agent mcp unset --env prod` drops prod from the request, which deletes prod's
// mapping AND its var rows — leaving a state indistinguishable from "never configured"
// except by those rows. Without this check the next proxy update would silently re-bind
// prod, re-mint its API key and re-inject its variables.
//
// The cost of the check is that an environment created after the connection was last
// saved has no var rows and so is not auto-bound; re-saving the connection provisions it.
// That case is not a regression: promotion into such an environment is refused by the
// system-managed-keys guard before MCP bindings are ever consulted, since none of the
// agent's configurations have rows there.
//
// Every input is already in hand — mappings and env var rows are preloaded on the
// configuration, endpoint environments on the proxy — so a steady-state reconcile answers
// this with no queries and no remote calls at all.
func mcpEnvsNeedingActivation(
	mappings []models.EnvAgentMCPMapping,
	vars []models.AgentEnvConfigVariable,
	proxy *models.MCPProxy,
) []uuid.UUID {
	mapped := make(map[uuid.UUID]struct{}, len(mappings))
	for i := range mappings {
		mapped[mappings[i].EnvironmentUUID] = struct{}{}
	}
	inScope := make(map[uuid.UUID]struct{}, len(vars))
	for i := range vars {
		inScope[vars[i].EnvironmentUUID] = struct{}{}
	}

	unmapped := make([]uuid.UUID, 0, len(inScope))
	seen := make(map[uuid.UUID]struct{}, len(inScope))
	for i := range proxy.Endpoints {
		for j := range proxy.Endpoints[i].Environments {
			envUUID := proxy.Endpoints[i].Environments[j].EnvironmentUUID
			if _, alreadyMapped := mapped[envUUID]; alreadyMapped {
				continue
			}
			if _, offered := inScope[envUUID]; !offered {
				continue // out of the connection's scope, or removed from it on purpose
			}
			if _, dup := seen[envUUID]; dup {
				continue
			}
			seen[envUUID] = struct{}{}
			unmapped = append(unmapped, envUUID)
		}
	}
	return unmapped
}

// mcpConfigTargetsProxy reports whether this connection's environment-agnostic intent is
// proxy, which is what authorises binding it in an environment it has no mapping row for.
//
// MCPProxyUUID answers that directly and is the only thing that can answer it for a
// connection with no mapping anywhere. A row left NULL by migration044 — its environments
// named different proxies, so there is no single intent — falls back to the mapping rows
// and is claimed only when they unanimously name this proxy. Guessing for a genuinely
// divergent connection would bind the wrong proxy.
// Unanimous mapping rows outrank the column. They are the connection's actual resource
// state, whereas the column caches an intent that is only authoritative when there are no
// rows to speak for themselves. The two disagree when updateMCPConfig re-points every
// environment and then fails to persist the column: mappings all name the new proxy while
// the column still names the old one. Letting the column win there left the configuration
// unreachable from either side — the new proxy's reconcile disowned it on the column, and
// the old proxy's found none of its own environments unmapped — with nothing but a manual
// re-save to break the tie.
func mcpConfigTargetsProxy(config *models.AgentConfiguration, proxyUUID uuid.UUID) bool {
	if mapped, unanimous := soleMappingProxyUUID(config.EnvMCPMappings); unanimous {
		return mapped == proxyUUID
	}
	// Either no mappings at all — the connection a proxy deployable nowhere leaves behind,
	// which only the column can describe — or environments naming different proxies, where
	// the column is the sole record of a single intent and NULL correctly means "none".
	if config.MCPProxyUUID != nil {
		return *config.MCPProxyUUID == proxyUUID
	}
	return false
}

// mcpEnvIndex is the organization's environment name↔UUID index. Both directions are
// needed: candidates are UUIDs, deployment pipelines name environments.
type mcpEnvIndex struct {
	nameByUUID map[uuid.UUID]string
	uuidByName map[string]uuid.UUID
}

// mcpPipelineEnvs is the environment set a project's deployment pipeline covers.
// restrict distinguishes "the pipeline covers no environment" from "there is no pipeline
// to restrict by" — an external agent has none, and its bindings must not be filtered
// away on that basis.
type mcpPipelineEnvs struct {
	envs     map[uuid.UUID]struct{}
	restrict bool
}

// mcpReconcileScope holds the lookups one reconcile run shares across the configurations
// it visits. Every lookup is lazy and memoized: a reconcile that finds nothing to backfill
// never performs one, and a reconcile that does pays for each environment listing once and
// each project's pipeline once, however many configurations reference the proxy.
type mcpReconcileScope struct {
	svc       *agentConfigurationService
	ouID      string
	envs      func() (mcpEnvIndex, error)
	pipelines map[string]mcpPipelineEnvs
}

func (s *agentConfigurationService) newMCPReconcileScope(ctx context.Context, ouID string) *mcpReconcileScope {
	return &mcpReconcileScope{
		svc:       s,
		ouID:      ouID,
		envs:      sync.OnceValues(func() (mcpEnvIndex, error) { return s.mcpEnvironmentIndex(ctx, ouID) }),
		pipelines: map[string]mcpPipelineEnvs{},
	}
}

// pipelineEnvironments returns the environments projectName's deployment pipeline covers.
// A project with no pipeline yields restrict=false, meaning candidates are not filtered by
// it at all.
func (r *mcpReconcileScope) pipelineEnvironments(ctx context.Context, projectName string) (mcpPipelineEnvs, error) {
	if cached, ok := r.pipelines[projectName]; ok {
		return cached, nil
	}
	unrestricted := mcpPipelineEnvs{envs: nil, restrict: false}

	pipeline, err := r.svc.ocClient.GetProjectDeploymentPipeline(ctx, r.ouID, projectName)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) || errors.Is(err, utils.ErrDeploymentPipelineNotFound) {
			r.pipelines[projectName] = unrestricted
			return unrestricted, nil
		}
		return mcpPipelineEnvs{envs: nil, restrict: false},
			fmt.Errorf("failed to get deployment pipeline for project %s: %w", projectName, err)
	}
	if pipeline == nil {
		r.pipelines[projectName] = unrestricted
		return unrestricted, nil
	}
	// A pipeline that exists and covers no environment is a definite answer — "nothing to
	// deploy into" — not the absence of one, so it restricts to the empty set rather than
	// waiving the filter. Treating it as unrestricted would let the scan bind, key and
	// inject environments the project cannot deploy to at all.
	names := client.PipelineEnvironments(pipeline.PromotionPaths)
	if len(names) == 0 {
		empty := mcpPipelineEnvs{envs: map[uuid.UUID]struct{}{}, restrict: true}
		r.pipelines[projectName] = empty
		return empty, nil
	}

	index, err := r.envs()
	if err != nil {
		return mcpPipelineEnvs{envs: nil, restrict: false}, err
	}
	set := make(map[uuid.UUID]struct{}, len(names))
	for _, name := range names {
		if envUUID, ok := index.uuidByName[name]; ok {
			set[envUUID] = struct{}{}
		}
	}
	resolved := mcpPipelineEnvs{envs: set, restrict: true}
	r.pipelines[projectName] = resolved
	return resolved, nil
}

// ReconcileMCPBindingsForProxy binds agents to proxy in environments that have become
// deployable since the agent's MCP connection was configured. Best-effort per
// (config, environment): failures are collected and returned but never abort the caller
// that triggered the reconcile.
func (s *agentConfigurationService) ReconcileMCPBindingsForProxy(ctx context.Context, ouID, proxyHandle string) error {
	// Every collaborator the reconcile dereferences, guarded together: a partially wired
	// service must skip the backfill, not panic partway through it.
	if s.envMCPMappingRepo == nil || s.mcpProxyRepo == nil || s.agentConfigRepo == nil || s.infraResourceManager == nil {
		return nil
	}
	// Reloaded rather than taken from the caller so the endpoint→environment rows this
	// reconcile reads are the ones the proxy update just committed.
	proxy, err := s.mcpProxyRepo.GetByHandle(ctx, proxyHandle, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("failed to load MCP proxy %q for binding reconcile: %w", proxyHandle, err)
	}
	if proxy == nil {
		return nil
	}
	return s.reconcileProxyBindings(ctx, s.newMCPReconcileScope(ctx, ouID), proxy, nil)
}

// ReconcileMCPBindingsForEnvironment reconciles every MCP proxy that has an endpoint bound
// to envUUID.
//
// Deployability flips on gateway topology as well as on the proxy: mapping the first
// egress gateway to an environment makes every proxy already bound there deployable, and
// with no trigger of its own that left every affected agent permanently unbound until
// someone re-saved the proxy.
func (s *agentConfigurationService) ReconcileMCPBindingsForEnvironment(ctx context.Context, ouID string, envUUID uuid.UUID) error {
	if s.envMCPMappingRepo == nil || s.mcpProxyRepo == nil || s.agentConfigRepo == nil || s.infraResourceManager == nil {
		return nil
	}
	proxies, err := s.mcpProxiesBoundToEnvironment(ctx, ouID, envUUID)
	if err != nil {
		return err
	}
	only := map[uuid.UUID]struct{}{envUUID: {}}

	// One scope across every proxy: an organization can have many proxies bound to the
	// environment whose gateway just arrived, and they share one environment listing and
	// one pipeline lookup per project. A scope per proxy would repeat both N times.
	scope := s.newMCPReconcileScope(ctx, ouID)

	var errs []error
	for _, proxy := range proxies {
		if err := s.reconcileProxyBindings(ctx, scope, proxy, only); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ReconcileMCPBindingsForAgentEnvironment binds every MCP connection of one agent in one
// environment. Callers that know the agent and environment but not the proxy — promotion,
// which must not refuse a target whose only problem is a binding nothing has written yet —
// use this.
func (s *agentConfigurationService) ReconcileMCPBindingsForAgentEnvironment(
	ctx context.Context, agentID, ouID, projectName, environmentName string,
) error {
	if s.envMCPMappingRepo == nil || s.mcpProxyRepo == nil || s.agentConfigRepo == nil || s.infraResourceManager == nil {
		return nil
	}
	envUUID, err := s.resolveEnvironmentUUID(ctx, ouID, environmentName)
	if err != nil {
		return err
	}
	configs, err := s.agentConfigRepo.ListMCPConfigsByAgent(ctx, ouID, projectName, agentID)
	if err != nil {
		return fmt.Errorf("failed to list MCP configurations for agent %s: %w", agentID, err)
	}

	scope := s.newMCPReconcileScope(ctx, ouID)
	only := map[uuid.UUID]struct{}{envUUID: {}}

	var errs []error
	for i := range configs {
		proxy, found, err := s.resolveConfigMCPProxy(ctx, &configs[i], ouID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !found {
			continue // no single proxy intent recorded; nothing to bind on its behalf
		}
		if err := s.reconcileConfigMCPBindings(ctx, scope, proxy, configs[i].UUID, only); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// resolveConfigMCPProxy loads the proxy a connection references, preloaded with the
// endpoint graph a reconcile reads. found is false — without an error — when the
// connection records no single environment-agnostic proxy, or when the proxy it named has
// since been deleted. Neither is a failure: such a connection is only ever reconciled from
// the proxy side, where the caller names the proxy.
func (s *agentConfigurationService) resolveConfigMCPProxy(
	ctx context.Context, config *models.AgentConfiguration, ouID string,
) (proxy *models.MCPProxy, found bool, err error) {
	proxyUUID, recorded := configMCPProxyUUID(config)
	if !recorded {
		return nil, false, nil
	}
	proxy, err = s.mcpProxyRepo.GetByUUID(ctx, proxyUUID.String(), ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil // proxy deleted; its mappings cascade away with it
		}
		return nil, false, fmt.Errorf("failed to load MCP proxy %s for configuration %s: %w", proxyUUID, config.UUID, err)
	}
	return proxy, true, nil
}

// configMCPProxyUUID returns the proxy this connection references and whether it records
// one at all: its own environment-agnostic column first, falling back to the mapping rows
// when they unanimously agree (see mcpConfigTargetsProxy for why unanimity is required).
func configMCPProxyUUID(config *models.AgentConfiguration) (uuid.UUID, bool) {
	if config.MCPProxyUUID != nil {
		return *config.MCPProxyUUID, true
	}
	return soleMappingProxyUUID(config.EnvMCPMappings)
}

// soleMappingProxyUUID returns the proxy every existing mapping names, and false when they
// disagree or there are none.
func soleMappingProxyUUID(mappings []models.EnvAgentMCPMapping) (uuid.UUID, bool) {
	if len(mappings) == 0 {
		return uuid.Nil, false
	}
	sole := mappings[0].MCPProxyUUID
	for i := range mappings {
		if mappings[i].MCPProxyUUID != sole {
			return uuid.Nil, false
		}
	}
	return sole, true
}

// mcpProxiesBoundToEnvironment returns every MCP proxy in the organization with an
// endpoint bound to envUUID.
func (s *agentConfigurationService) mcpProxiesBoundToEnvironment(
	ctx context.Context, ouID string, envUUID uuid.UUID,
) ([]*models.MCPProxy, error) {
	var bound []*models.MCPProxy
	for offset := 0; ; offset += mcpProxyScanPageSize {
		page, err := s.mcpProxyRepo.List(ctx, ouID, mcpProxyScanPageSize, offset)
		if err != nil {
			return nil, fmt.Errorf("failed to list MCP proxies for environment %s reconcile: %w", envUUID, err)
		}
		for _, proxy := range page {
			if endpoint, _ := resolveMCPEndpointForEnv(proxy, envUUID.String()); endpoint != nil {
				bound = append(bound, proxy)
			}
		}
		if len(page) < mcpProxyScanPageSize {
			return bound, nil
		}
	}
}

// mcpProxyScanPageSize pages mcpProxiesBoundToEnvironment. The scan runs once per gateway
// assignment, so the page size only bounds peak memory, not latency.
const mcpProxyScanPageSize = 100

// reconcileProxyBindings backfills every configuration referencing proxy. When only is
// non-nil, candidates are restricted to those environments.
func (s *agentConfigurationService) reconcileProxyBindings(
	ctx context.Context, scope *mcpReconcileScope, proxy *models.MCPProxy, only map[uuid.UUID]struct{},
) error {
	configUUIDs, err := s.mcpConfigUUIDsForProxy(ctx, scope.ouID, proxy)
	if err != nil {
		return err
	}
	if len(configUUIDs) == 0 {
		return nil
	}

	var errs []error
	for _, configUUID := range configUUIDs {
		if err := s.reconcileConfigMCPBindings(ctx, scope, proxy, configUUID, only); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// mcpConfigUUIDsForProxy returns every configuration that references proxy, from both
// directions: the configuration's own environment-agnostic proxy column, and — for rows
// migration044 left NULL — the mapping rows. The column is what reaches a connection with
// no mapping in ANY environment, which the mapping-row query by construction cannot see.
func (s *agentConfigurationService) mcpConfigUUIDsForProxy(
	ctx context.Context, ouID string, proxy *models.MCPProxy,
) ([]uuid.UUID, error) {
	seen := make(map[uuid.UUID]struct{})
	var configUUIDs []uuid.UUID

	configs, err := s.agentConfigRepo.ListMCPConfigsByProxy(ctx, ouID, proxy.UUID)
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP configurations for proxy %s: %w", proxy.UUID, err)
	}
	for i := range configs {
		if _, dup := seen[configs[i].UUID]; dup {
			continue
		}
		seen[configs[i].UUID] = struct{}{}
		configUUIDs = append(configUUIDs, configs[i].UUID)
	}

	proxyMappings, err := s.envMCPMappingRepo.ListByMCPProxy(ctx, proxy.UUID)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent bindings for MCP proxy %s: %w", proxy.UUID, err)
	}
	for i := range proxyMappings {
		configUUID := proxyMappings[i].ConfigUUID
		if _, dup := seen[configUUID]; dup {
			continue
		}
		seen[configUUID] = struct{}{}
		configUUIDs = append(configUUIDs, configUUID)
	}
	return configUUIDs, nil
}

func (s *agentConfigurationService) mcpEnvironmentIndex(ctx context.Context, ouID string) (mcpEnvIndex, error) {
	envs, err := s.infraResourceManager.ListOrgEnvironments(ctx, ouID)
	if err != nil {
		return mcpEnvIndex{}, fmt.Errorf("failed to list environments for MCP binding reconcile: %w", err)
	}
	index := mcpEnvIndex{
		nameByUUID: make(map[uuid.UUID]string, len(envs)),
		uuidByName: make(map[string]uuid.UUID, len(envs)),
	}
	for _, env := range envs {
		envUUID, parseErr := uuid.Parse(env.UUID)
		if parseErr != nil {
			continue
		}
		index.nameByUUID[envUUID] = env.Name
		index.uuidByName[env.Name] = envUUID
	}
	return index, nil
}

// reconcileConfigMCPBindings works cheapest-filter-first: the intent check and the
// candidate scan read only rows already in hand, so the environment listing, the pipeline
// lookup and the per-environment gateway resolution are all reached only once there is an
// unmapped environment to act on.
func (s *agentConfigurationService) reconcileConfigMCPBindings(
	ctx context.Context, scope *mcpReconcileScope, proxy *models.MCPProxy, configUUID uuid.UUID,
	only map[uuid.UUID]struct{},
) error {
	// Preloads the config's MCP mappings and env var rows, which is all the candidate scan
	// and the activation inputs below read.
	config, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, scope.ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // config deleted since it was listed
		}
		return fmt.Errorf("failed to load agent configuration %s: %w", configUUID, err)
	}
	if config.TypeID != models.AgentConfigTypeIDMCP || !mcpConfigTargetsProxy(config, proxy.UUID) {
		return nil
	}

	// Converge a column that its own mapping rows have overtaken — the residue of an
	// updateMCPConfig that re-pointed every environment and then failed to persist the
	// reference. Reaching here means the rows unanimously name this proxy, so the column
	// is simply stale; leaving it would keep every later reader deciding on out-of-date
	// intent. A no-op when they already agree, and best-effort: failing to tidy the cache
	// must not stop the bindings this reconcile came to write.
	if config.MCPProxyUUID == nil || *config.MCPProxyUUID != proxy.UUID {
		if _, unanimous := soleMappingProxyUUID(config.EnvMCPMappings); unanimous {
			proxyUUID := proxy.UUID
			if err := s.setConfigMCPProxy(ctx, config, &proxyUUID); err != nil {
				s.logger.Warn("Failed to converge stale MCP proxy reference on configuration",
					"configUUID", config.UUID, "mcpProxyUUID", proxy.UUID, "error", err)
			}
		}
	}

	candidates := mcpEnvsNeedingActivation(config.EnvMCPMappings, config.EnvVariables, proxy)
	if only != nil {
		candidates = retainEnvs(candidates, only)
	}
	if len(candidates) == 0 {
		return nil
	}

	// An environment the proxy serves but the agent's project never deploys to would get a
	// mapping, an API key and env var rows it can never use. Applied only now, so the
	// steady-state reconcile above never pays for the pipeline lookup.
	pipelineEnvs, err := scope.pipelineEnvironments(ctx, config.ProjectName)
	if err != nil {
		return err
	}
	if pipelineEnvs.restrict {
		candidates = retainEnvs(candidates, pipelineEnvs.envs)
		if len(candidates) == 0 {
			return nil
		}
	}

	bindable := s.deployableMCPEnvs(ctx, proxy, scope.ouID, candidates)
	if len(bindable) == 0 {
		return nil
	}

	index, err := scope.envs()
	if err != nil {
		return err
	}
	inputs, err := s.mcpActivationInputsForConfig(ctx, scope.ouID, config)
	if err != nil {
		return err
	}

	var errs []error
	boundEnvNames := make(map[string]struct{}, len(bindable))
	for _, envUUID := range bindable {
		envName := index.nameByUUID[envUUID]
		if envName == "" {
			continue // environment since deleted
		}
		if err := s.activateMCPMappingForEnv(ctx, config, proxy, envUUID, envName, scope.ouID, inputs); err != nil {
			errs = append(errs, fmt.Errorf("failed to bind agent %q to MCP proxy in environment %s: %w", config.AgentID, envName, err))
			continue
		}
		s.logger.Info("Backfilled MCP binding for environment that became deployable",
			"agentName", config.AgentID, "configName", config.Name, "environment", envName, "mcpProxyUUID", proxy.UUID)
		boundEnvNames[envName] = struct{}{}
	}
	// The agent's AgentID token scopes are derived from its MCP mappings, so the bindings
	// just created change them too.
	s.refreshTouchedMCPEnvironments(ctx, scope.ouID, config.ProjectName, config.AgentID, boundEnvNames)
	return errors.Join(errs...)
}

func retainEnvs(envUUIDs []uuid.UUID, keep map[uuid.UUID]struct{}) []uuid.UUID {
	retained := make([]uuid.UUID, 0, len(envUUIDs))
	for _, envUUID := range envUUIDs {
		if _, ok := keep[envUUID]; ok {
			retained = append(retained, envUUID)
		}
	}
	return retained
}

// deployableMCPEnvs narrows candidates to the environments proxy can actually back a
// binding in.
//
// Every skip is logged. A silently dropped environment is indistinguishable from "nothing
// to do", which is how this class of breakage reached users: the binding never appeared,
// the logs said nothing, and the agent failed at promotion instead.
func (s *agentConfigurationService) deployableMCPEnvs(
	ctx context.Context, proxy *models.MCPProxy, ouID string, candidates []uuid.UUID,
) []uuid.UUID {
	bindable := make([]uuid.UUID, 0, len(candidates))
	for _, envUUID := range candidates {
		if _, err := s.resolveDeployableMCPGateway(ctx, proxy, ouID, envUUID); err != nil {
			if errors.Is(err, errMCPEnvNotDeployable) {
				s.logger.Info("Skipping MCP binding backfill; environment is not deployable yet",
					"ouID", ouID, "environmentUUID", envUUID, "mcpProxyUUID", proxy.UUID)
			} else {
				s.logger.Warn("Skipping MCP binding backfill; gateway lookup failed",
					"ouID", ouID, "environmentUUID", envUUID, "mcpProxyUUID", proxy.UUID, "error", err)
			}
			continue
		}
		bindable = append(bindable, envUUID)
	}
	return bindable
}

func (s *agentConfigurationService) mcpActivationInputsForConfig(
	ctx context.Context, ouID string, config *models.AgentConfiguration,
) (mcpActivationInputs, error) {
	// Rebuilt from the names already persisted for the config, so a backfill reuses the exact
	// variable names the agent was promoted with (including user overrides) rather than
	// re-deriving defaults from the config name.
	envTemplates, err := s.mcpEnvTemplatesFromVars(config, config.EnvVariables)
	if err != nil {
		return mcpActivationInputs{}, err
	}
	isExternalAgent, firstEnvName, err := s.agentDeploymentShape(ctx, ouID, config.ProjectName, config.AgentID)
	if err != nil {
		return mcpActivationInputs{}, err
	}
	return mcpActivationInputs{
		envTemplates:    envTemplates,
		isExternalAgent: isExternalAgent,
		firstEnvName:    firstEnvName,
	}, nil
}

func (s *agentConfigurationService) agentDeploymentShape(ctx context.Context, ouID, projectName, agentName string) (isExternal bool, firstEnvName string, err error) {
	agentComp, err := s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
	if err != nil {
		return false, "", fmt.Errorf("failed to determine agent type for %s: %w", agentName, err)
	}
	isExternal = agentComp.Provisioning.Type == string(utils.ExternalAgent)
	if isExternal {
		return true, "", nil
	}
	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, ouID, projectName)
	if err != nil {
		// A project with no pipeline simply has no first environment; anything else is a
		// real lookup failure and must not masquerade as one.
		if errors.Is(err, utils.ErrProjectNotFound) || errors.Is(err, utils.ErrDeploymentPipelineNotFound) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("failed to get deployment pipeline for project %s: %w", projectName, err)
	}
	if pipeline == nil {
		return false, "", nil
	}
	return false, client.FindFirstEnvironment(pipeline.PromotionPaths), nil
}

// ListUnresolvedMCPBindings returns the names of the agent's MCP connections that are
// configured for environmentName — so their URL and API-key variables are injected into the
// workload there — but resolve to no proxy URL, leaving those variables injected empty. An
// agent in this state starts and runs, but every call it makes through the connection fails.
func (s *agentConfigurationService) ListUnresolvedMCPBindings(
	ctx context.Context, agentID, ouID, projectName, environmentName string,
) (map[string]struct{}, error) {
	envUUID, err := s.resolveEnvironmentUUID(ctx, ouID, environmentName)
	if err != nil {
		return nil, err
	}
	configs, err := s.agentConfigRepo.ListByAgent(ctx, ouID, projectName, agentID, agentConfigListAll, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent configurations: %w", err)
	}

	unresolved := make(map[string]struct{}, len(configs))
	for i := range configs {
		config := &configs[i]
		if config.TypeID != models.AgentConfigTypeIDMCP {
			continue
		}
		vars, err := s.envVariableRepo.ListByConfigAndEnv(ctx, config.UUID, envUUID)
		if err != nil {
			return nil, fmt.Errorf("failed to list env config variables for config %s: %w", config.UUID, err)
		}
		if len(vars) == 0 {
			continue // not configured for this environment at all
		}
		urlValue, err := s.systemManagedMCPURL(ctx, config, ouID, environmentName, envUUID)
		if err != nil {
			return nil, err
		}
		if urlValue == "" {
			unresolved[config.Name] = struct{}{}
		}
	}
	return unresolved, nil
}

// activateMCPMappingForEnv binds config to sourceProxy in a deployable environment: it
// creates the mapping row, mints the per-agent inbound API key against the proxy's shared
// gateway artifact when the proxy has api-key security enabled, and injects the resulting
// URL / API-key env vars. Nothing is deployed — the proxy already owns the environment's
// single gateway artifact.
//
// The env var rows are ensured rather than inserted outright: an environment that was
// previously unconfigured already has them, persisted blank by provisionUnconfiguredMCPEnv.
// Insert-only would silently no-op on the unique constraint and leave the API-key row
// pointing at no secret, so the secret reference is written explicitly afterwards.
func (s *agentConfigurationService) activateMCPMappingForEnv(
	ctx context.Context,
	config *models.AgentConfiguration,
	sourceProxy *models.MCPProxy,
	envUUID uuid.UUID,
	envName, ouID string,
	inputs mcpActivationInputs,
) (err error) {
	// A backfill binds an environment the agent was already promoted into, so its env var
	// rows are already there. Recorded before anything is written so rollback knows not to
	// delete rows this call did not create.
	existingVars, err := s.envVariableRepo.ListByConfigAndEnv(ctx, config.UUID, envUUID)
	if err != nil {
		return fmt.Errorf("failed to read existing MCP environment variables: %w", err)
	}
	envVarsCreatedHere := len(existingVars) == 0

	handle := mcpMappingProxyName(config.ProjectName, config.AgentID, config.Name, envName)
	mapping := &models.EnvAgentMCPMapping{
		ConfigUUID:      config.UUID,
		EnvironmentUUID: envUUID,
		MCPProxyUUID:    sourceProxy.UUID,
		ArtifactUUID:    uuid.New(),
	}
	deployedProxy := buildAgentMCPConfigProxy(config, mapping, sourceProxy, envName, ouID, handle)
	proxyMapping := buildMCPProxyMapping(sourceProxy.UUID, deployedProxy)
	if txErr := s.db.Transaction(func(tx *gorm.DB) error {
		return s.envMCPMappingRepo.Create(ctx, tx, mapping, proxyMapping, handle, handle, mcpProxyArtifactVersion(sourceProxy), ouID)
	}); txErr != nil {
		return fmt.Errorf("failed to create MCP mapping: %w", txErr)
	}
	// The mapping row opens the rollback window: every failure past this point tears the
	// half-built binding back down so a retry starts clean.
	defer func() {
		if err != nil {
			s.cleanupNewMCPMapping(ctx, config, mapping, envName, ouID, envVarsCreatedHere)
		}
	}()

	// Must precede credential provisioning: ensureMCPMappingCredentials points the API-key
	// row at the secret it mints, and fails when that row does not exist yet.
	if err = s.ensureMCPEnvVarRows(ctx, config.UUID, envUUID, inputs.envTemplates); err != nil {
		return fmt.Errorf("failed to create MCP environment variables: %w", err)
	}

	if mcpProxyAPIKeySecurityEnabled(sourceProxy, envUUID.String()) {
		if _, err = s.ensureMCPMappingCredentials(ctx, config, mapping, envName, ouID); err != nil {
			return err
		}
	} else if err = s.updateMCPMappingSecretReference(ctx, config.UUID, envUUID, ""); err != nil {
		return fmt.Errorf("failed to clear MCP API key secret reference: %w", err)
	}

	if inputs.isExternalAgent {
		return nil
	}
	// Warn-only, and deliberately not assigned to err: a failed injection leaves a valid
	// binding the agent picks up on its next deploy, so it must not trigger the rollback.
	if injectErr := s.injectMCPMappingEnvVars(ctx, config, mapping, sourceProxy, envName, ouID,
		inputs.envTemplates, inputs.firstEnvName); injectErr != nil {
		s.logger.Warn("failed to inject MCP mapping env vars", "environment", envName, "err", injectErr)
	}
	return nil
}
