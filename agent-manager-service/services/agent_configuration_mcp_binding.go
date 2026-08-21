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

// mcpEnvsNeedingActivation returns the environments where an agent's MCP connection was
// configured but never bound: env var rows exist (the environment was in the connection's
// requested set, so its URL/API-key variables are injected) while no EnvAgentMCPMapping
// does. That is the state provisionUnconfiguredMCPEnv leaves behind when the proxy has no
// endpoint in the environment yet — the variables are injected empty, and nothing has ever
// filled them in afterwards.
//
// The proxy to bind is inferred from the config's already-mapped environments, so the
// inference is only made when every existing mapping names proxyUUID. A config bound to
// different proxies per environment records no intent for the unmapped ones, and guessing
// there would bind the wrong proxy.
func mcpEnvsNeedingActivation(
	mappings []models.EnvAgentMCPMapping,
	vars []models.AgentEnvConfigVariable,
	proxyUUID uuid.UUID,
) []uuid.UUID {
	if len(mappings) == 0 {
		return nil // nothing links this config to this proxy
	}
	handledEnvs := make(map[uuid.UUID]struct{}, len(mappings)+len(vars))
	for i := range mappings {
		if mappings[i].MCPProxyUUID != proxyUUID {
			return nil // ambiguous intent, see above
		}
		handledEnvs[mappings[i].EnvironmentUUID] = struct{}{}
	}

	unmapped := make([]uuid.UUID, 0, len(vars))
	for i := range vars {
		envUUID := vars[i].EnvironmentUUID
		if _, handled := handledEnvs[envUUID]; handled {
			continue
		}
		handledEnvs[envUUID] = struct{}{}
		unmapped = append(unmapped, envUUID)
	}
	return unmapped
}

// ReconcileMCPBindingsForProxy binds agents to proxy in environments that have become
// deployable since the agent's MCP connection was configured. Best-effort per
// (config, environment): failures are collected and returned but never abort the proxy
// update that triggered the reconcile.
//
// A connection with no mapping in ANY environment is out of reach here: nothing links it to
// this proxy, since the link is the mapping row itself. Re-saving the connection binds it.
// Two flows leave a connection in that state — the proxy had no endpoint anywhere when the
// connection was configured, and teardownMCPMappingKeepEnvVars dropping the last mapping
// when an environment stopped being deployable.
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
		return fmt.Errorf("failed to load MCP proxy %q for binding reconcile: %w", proxyHandle, err)
	}
	if proxy == nil {
		return nil
	}

	proxyMappings, err := s.envMCPMappingRepo.ListByMCPProxy(ctx, proxy.UUID)
	if err != nil {
		return fmt.Errorf("failed to list agent bindings for MCP proxy %s: %w", proxy.UUID, err)
	}
	if len(proxyMappings) == 0 {
		return nil
	}

	// Listing environments is a remote call, and the steady-state reconcile finds nothing to
	// backfill and never needs a name: resolve on first use, once for all configs.
	envNames := sync.OnceValues(func() (map[uuid.UUID]string, error) {
		return s.mcpEnvironmentNames(ctx, ouID)
	})

	var errs []error
	for _, configUUID := range distinctConfigUUIDs(proxyMappings) {
		if err := s.reconcileConfigMCPBindings(ctx, ouID, proxy, configUUID, envNames); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *agentConfigurationService) mcpEnvironmentNames(ctx context.Context, ouID string) (map[uuid.UUID]string, error) {
	envs, err := s.infraResourceManager.ListOrgEnvironments(ctx, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments for MCP binding reconcile: %w", err)
	}
	names := make(map[uuid.UUID]string, len(envs))
	for _, env := range envs {
		envUUID, parseErr := uuid.Parse(env.UUID)
		if parseErr != nil {
			continue
		}
		names[envUUID] = env.Name
	}
	return names, nil
}

func distinctConfigUUIDs(mappings []models.EnvAgentMCPMapping) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(mappings))
	configUUIDs := make([]uuid.UUID, 0, len(mappings))
	for i := range mappings {
		if _, ok := seen[mappings[i].ConfigUUID]; ok {
			continue
		}
		seen[mappings[i].ConfigUUID] = struct{}{}
		configUUIDs = append(configUUIDs, mappings[i].ConfigUUID)
	}
	return configUUIDs
}

// reconcileConfigMCPBindings works cheapest-filter-first: the two candidate gates read only
// rows already in hand or indexed by environment, and the remote calls behind envNames and
// the activation inputs are reached only once an environment is genuinely bindable.
func (s *agentConfigurationService) reconcileConfigMCPBindings(
	ctx context.Context, ouID string, proxy *models.MCPProxy, configUUID uuid.UUID,
	envNames func() (map[uuid.UUID]string, error),
) error {
	// Preloads the config's MCP mappings and env var rows, which is all the candidate scan
	// below reads.
	config, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // config deleted since its mappings were listed
		}
		return fmt.Errorf("failed to load agent configuration %s: %w", configUUID, err)
	}

	candidates := mcpEnvsNeedingActivation(config.EnvMCPMappings, config.EnvVariables, proxy.UUID)
	if len(candidates) == 0 {
		return nil
	}
	bindable := s.deployableMCPEnvs(ctx, proxy, ouID, candidates)
	if len(bindable) == 0 {
		return nil
	}

	names, err := envNames()
	if err != nil {
		return err
	}
	inputs, err := s.mcpActivationInputsForConfig(ctx, ouID, config)
	if err != nil {
		return err
	}

	var errs []error
	boundEnvNames := make(map[string]struct{}, len(bindable))
	for _, envUUID := range bindable {
		envName := names[envUUID]
		if envName == "" {
			continue // environment since deleted
		}
		if err := s.activateMCPMappingForEnv(ctx, config, proxy, envUUID, envName, ouID, inputs); err != nil {
			errs = append(errs, fmt.Errorf("failed to bind agent %q to MCP proxy in environment %s: %w", config.AgentID, envName, err))
			continue
		}
		s.logger.Info("Backfilled MCP binding for environment that became deployable",
			"agent_name", config.AgentID, "config_name", config.Name, "environment", envName, "mcp_proxy_uuid", proxy.UUID)
		boundEnvNames[envName] = struct{}{}
	}
	// The agent's AgentID token scopes are derived from its MCP mappings, so the bindings
	// just created change them too.
	s.refreshTouchedMCPEnvironments(ctx, ouID, config.ProjectName, config.AgentID, boundEnvNames)
	return errors.Join(errs...)
}

// deployableMCPEnvs narrows candidates to the environments proxy can actually back a binding
// in. A lookup failure is logged and the environment skipped: the next proxy update retries.
func (s *agentConfigurationService) deployableMCPEnvs(
	ctx context.Context, proxy *models.MCPProxy, ouID string, candidates []uuid.UUID,
) []uuid.UUID {
	bindable := make([]uuid.UUID, 0, len(candidates))
	for _, envUUID := range candidates {
		if _, err := s.resolveDeployableMCPGateway(ctx, proxy, ouID, envUUID); err != nil {
			if !errors.Is(err, errMCPEnvNotDeployable) {
				s.logger.Warn("Skipping MCP binding backfill; gateway lookup failed",
					"environment_uuid", envUUID, "mcp_proxy_uuid", proxy.UUID, "error", err)
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
		s.logger.Warn("failed to inject MCP mapping env vars", "environment", envName, "error", injectErr)
	}
	return nil
}
