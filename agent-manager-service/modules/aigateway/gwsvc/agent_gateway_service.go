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

package gwsvc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/constants"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/agentgwv1"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/llmv1"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/mcpv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"gorm.io/gorm"
)

// AgentGatewayService is the gateway's side of an agent: the LLM and MCP
// bindings the agent is created with, its exposure through the gateway, and
// what the gateway holds for it when it goes. It provides the llm.proxy/v1,
// mcp.proxy/v1 and agent.gateway/v1 capabilities to the core's agent service,
// and serves the agent routes that only exist with a gateway (deploy settings,
// deployments).
type AgentGatewayService struct {
	db                        *gorm.DB
	platform                  repositories.PlatformRepository
	agentConfigRepo           repositories.AgentConfigRepository
	agentConfigurationService AgentConfigurationService
	artifactRepo              gwrepo.ArtifactRepository
	aiApplicationService      *AIApplicationService
	gatewayRepo               gwrepo.GatewayRepository
	a2aPublisher              *A2APublisher
	a2aCardRepo               gwrepo.A2AAgentCardRepository
	exposure                  *AgentExposurePublisher
	logger                    *slog.Logger
}

var (
	_ llmv1.Proxy       = llmBinder{}
	_ mcpv1.Proxy       = mcpBinder{}
	_ agentgwv1.Gateway = (*AgentGatewayService)(nil)
)

// NewAgentGatewayService creates the gateway's side of agents.
func NewAgentGatewayService(
	db *gorm.DB,
	platform repositories.PlatformRepository,
	agentConfigRepo repositories.AgentConfigRepository,
	agentConfigurationService AgentConfigurationService,
	artifactRepo gwrepo.ArtifactRepository,
	aiApplicationService *AIApplicationService,
	gatewayRepo gwrepo.GatewayRepository,
	a2aPublisher *A2APublisher,
	a2aCardRepo gwrepo.A2AAgentCardRepository,
	exposure *AgentExposurePublisher,
	logger *slog.Logger,
) *AgentGatewayService {
	return &AgentGatewayService{
		db:                        db,
		platform:                  platform,
		agentConfigRepo:           agentConfigRepo,
		agentConfigurationService: agentConfigurationService,
		artifactRepo:              artifactRepo,
		aiApplicationService:      aiApplicationService,
		gatewayRepo:               gatewayRepo,
		a2aPublisher:              a2aPublisher,
		a2aCardRepo:               a2aCardRepo,
		exposure:                  exposure,
		logger:                    logger,
	}
}

// ValidateProviders refuses LLM providers the organization's catalog does not have.
func (s *AgentGatewayService) ValidateProviders(ctx context.Context, orgID string, providers []string) error {
	return s.agentConfigurationService.ValidateProvidersInCatalog(ctx, orgID, providers)
}

// ValidateProxies refuses MCP proxies the project cannot use.
func (s *AgentGatewayService) ValidateProxies(ctx context.Context, orgID, project string, proxies []string) error {
	return s.agentConfigurationService.ValidateMCPProxiesInCatalog(ctx, orgID, project, proxies)
}

// BindAgent is llm.proxy/v1's: the agent's LLM configurations in the environment.
func (s llmBinder) BindAgent(ctx context.Context, agent contracts.AgentRef, environment string, configs []spec.ModelConfigRequest) error {
	return s.createAgentLLMConfigs(ctx, agent.OrgID, agent.Project, agent.Name, environment, configs)
}

// BindAgent is mcp.proxy/v1's: the agent's MCP configurations in each environment.
func (s mcpBinder) BindAgent(ctx context.Context, agent contracts.AgentRef, environments []string, configs []spec.MCPConfigRequest) error {
	return s.createAgentMCPConfigs(ctx, agent.OrgID, agent.Project, agent.Name, environments, configs)
}

// LLM is the service as llm.proxy/v1.
func (s *AgentGatewayService) LLM() llmv1.Proxy { return llmBinder{s} }

// MCP is the service as mcp.proxy/v1.
func (s *AgentGatewayService) MCP() mcpv1.Proxy { return mcpBinder{s} }

// The two proxies both bind agents, each with its own configuration type, so
// each gets a view of the service with its own BindAgent.
type (
	llmBinder struct{ *AgentGatewayService }
	mcpBinder struct{ *AgentGatewayService }
)

// CheckCreate refuses a name whose deleted agent's configurations were not
// all revoked (see checkNoOrphanedConfigs).
func (s *AgentGatewayService) CheckCreate(ctx context.Context, agent contracts.AgentRef) error {
	return s.checkNoOrphanedConfigs(ctx, agent.OrgID, agent.Project, agent.Name)
}

// Created clears A2A state a deleted agent of the same name left behind.
func (s *AgentGatewayService) Created(ctx context.Context, agent contracts.AgentRef) error {
	return s.clearInheritedA2AState(ctx, agent.OrgID, agent.Project, agent.Name)
}

// Unbind revokes and removes the agent's LLM and MCP configurations.
func (s *AgentGatewayService) Unbind(ctx context.Context, agent contracts.AgentRef) {
	s.deleteAgentLLMConfigurations(ctx, agent.OrgID, agent.Project, agent.Name, s.agentIs(ctx, agent, isExternalProvisioned))
}

// Deleting removes the agent's API artifacts and A2A state.
func (s *AgentGatewayService) Deleting(ctx context.Context, agent contracts.AgentRef) error {
	return s.deleteAgentAPIArtifact(ctx, agent.OrgID, agent.Project, agent.Name, s.agentIs(ctx, agent, a2aCapable))
}

// agentIs reads the agent, which still exists, and tests it; false when it
// cannot be read.
func (s *AgentGatewayService) agentIs(ctx context.Context, ref contracts.AgentRef, test func(*models.AgentResponse) bool) bool {
	agent, err := s.platform.GetAgent(ctx, ref.OrgID, ref.Project, ref.Name)
	if err != nil {
		s.logger.Warn("Failed to determine agent type before deletion", "agentName", ref.Name, "error", err)
		return false
	}
	return test(agent)
}

func (s *AgentGatewayService) requireEnvTier(ctx context.Context, ouID, envName string) (*models.EnvironmentResponse, error) {
	return RequireEnvironmentTier(ctx, s.platform, s.logger, ouID, envName)
}

// resolveResilienceTimeoutSeconds resolves the effective resilience timeout from
// (in precedence order) an explicit request value, the persisted config, and the
// default. An explicitly requested value outside [MinResilienceTimeoutSeconds,
// MaxResilienceTimeoutSeconds] is rejected rather than silently discarded — unlike
// an omitted (nil) request, which falls through to the existing/default value.
func resolveResilienceTimeoutSeconds(existingConfig *models.AgentConfig, requested *int32, withDefaults bool) (int32, error) {
	resolved := int32(0)
	if withDefaults {
		resolved = constants.DefaultResilienceTimeoutSeconds
	}
	if existingConfig != nil && existingConfig.ResilienceTimeoutSeconds != nil {
		resolved = *existingConfig.ResilienceTimeoutSeconds
	}
	if requested != nil {
		if *requested < constants.MinResilienceTimeoutSeconds || *requested > constants.MaxResilienceTimeoutSeconds {
			return 0, fmt.Errorf("%w: resilienceTimeoutSeconds must be between %d and %d seconds, got %d",
				utils.ErrInvalidInput, constants.MinResilienceTimeoutSeconds, constants.MaxResilienceTimeoutSeconds, *requested)
		}
		resolved = *requested
	}
	return resolved, nil
}

// createAgentLLMConfigs configures the agent's LLM providers in the pipeline's entry
// environment only.
//
// Unlike an MCP connection, this deliberately does NOT span the pipeline. createLLMConfig
// resolves the gateway the provider is deployed to per environment and rolls the whole
// configuration back when it cannot — there is no "configure it now, bind it when the
// environment is ready" path for an LLM proxy the way provisionUnconfiguredMCPEnv gives
// one for MCP. Mapping every environment here would therefore make agent creation fail
// outright whenever a higher environment has no gateway hosting the provider yet, turning
// a promotion-time prompt to configure the target into a creation-time blocker.
//
// The consequence is that promoting an agent with an LLM provider still requires the
// target environment to be configured first, and the promotion guard says so.
func (s *AgentGatewayService) createAgentLLMConfigs(
	ctx context.Context, ouID, projectName, agentName, firstEnv string, modelConfig []spec.ModelConfigRequest,
) error {
	for i, mc := range modelConfig {
		configName := fmt.Sprintf("%s-llm-config", agentName)
		if len(modelConfig) > 1 {
			configName = fmt.Sprintf("%s-llm-config-%d", agentName, i+1)
		}
		cfg := models.EnvProviderConfiguration{}
		if mc.Configuration != nil {
			cfg = convertConfiguration(*mc.Configuration)
		}
		createReq := models.CreateAgentModelConfigRequest{
			Name: configName,
			Type: "llm",
			EnvMappings: map[string]models.EnvModelConfigRequest{
				firstEnv: {
					ProviderName:  mc.ProviderName,
					Configuration: cfg,
				},
			},
			EnvironmentVariables: convertEnvVars(mc.EnvironmentVariables),
		}
		if _, err := s.agentConfigurationService.Create(ctx, ouID, projectName, agentName, createReq, "system"); err != nil {
			return fmt.Errorf("failed to create LLM configuration %d: %w", i+1, err)
		}
	}
	return nil
}

// createAgentMCPConfigs binds the agent's MCP connections across EVERY environment in the
// project's deployment pipeline. An MCP connection is environment-agnostic — the same
// proxy backs every environment — so there is no reason for creation to bind only the
// entry point, and doing so left higher environments with no binding to promote into.
//
// Environments where the proxy has no endpoint yet are not skipped: createMCPConfig
// records the connection there with its variables injected empty, which is the state
// ReconcileMCPBindingsForProxy later completes once the proxy becomes deployable.
func (s *AgentGatewayService) createAgentMCPConfigs(
	ctx context.Context, ouID, projectName, agentName string, pipelineEnvs []string, mcpConfig []spec.MCPConfigRequest,
) error {
	for i, mc := range mcpConfig {
		configName := fmt.Sprintf("%s-mcp-config", agentName)
		if len(mcpConfig) > 1 {
			configName = fmt.Sprintf("%s-mcp-config-%d", agentName, i+1)
		}
		envMappings := make(map[string]models.EnvModelConfigRequest, len(pipelineEnvs))
		for _, envName := range pipelineEnvs {
			envMappings[envName] = models.EnvModelConfigRequest{
				// createMCPConfig reads the MCP proxy handle from ProviderName.
				ProviderName: mc.ProxyName,
			}
		}
		createReq := models.CreateAgentModelConfigRequest{
			Name:                 configName,
			Type:                 models.AgentConfigTypeMCP,
			EnvMappings:          envMappings,
			EnvironmentVariables: convertEnvVars(mc.EnvironmentVariables),
			// Every pipeline environment is configured at once, so one misconfigured higher
			// environment must not roll back the whole agent; see createMCPConfig.
			DeferUnbindableMCPEnvs: true,
		}
		if _, err := s.agentConfigurationService.Create(ctx, ouID, projectName, agentName, createReq, "system"); err != nil {
			return fmt.Errorf("failed to create MCP configuration %d: %w", i+1, err)
		}
	}
	return nil
}

// convertConfiguration maps a generated provider configuration to the model type,
// preserving guardrail policies. Replaces the per-env conversion that convertEnvMappings did.
func convertConfiguration(cfg spec.EnvProviderConfiguration) models.EnvProviderConfiguration {
	policies := make([]models.LLMPolicy, 0, len(cfg.Policies))
	for _, p := range cfg.Policies {
		paths := make([]models.LLMPolicyPath, 0, len(p.Paths))
		for _, pp := range p.Paths {
			paths = append(paths, models.LLMPolicyPath{
				Path:    pp.Path,
				Methods: pp.Methods,
				Params:  pp.Params,
			})
		}
		policies = append(policies, models.LLMPolicy{
			Name:    p.Name,
			Version: p.Version,
			Paths:   paths,
		})
	}
	return models.EnvProviderConfiguration{
		Policies:   policies,
		Resilience: resilienceOf(cfg.Resilience),
	}
}

func convertEnvVars(specVars []spec.EnvironmentVariableConfig) []models.EnvironmentVariableConfig {
	result := make([]models.EnvironmentVariableConfig, 0, len(specVars))
	for _, v := range specVars {
		result = append(result, models.EnvironmentVariableConfig{Name: v.Name, Key: v.Key})
	}
	return result
}

// Agent-deletion cleanup runs detached from the request (see deleteAgentLLMConfigurations),
// so it can afford to wait out the transient gateway/secret-store failures that make
// revocation fail while the cluster is under load — the failure mode this path was
// reported for. Four attempts at a doubling delay spans ~14s per configuration.
// Variables rather than constants so tests can shrink the delay; production never reassigns them.
var (
	agentConfigCleanupAttempts   = 4
	agentConfigCleanupRetryDelay = 2 * time.Second
)

// cleanupRetry bounds the retries of an idempotent cleanup step; delay is the wait after 0-based failed attempt n.
type cleanupRetry struct {
	attempts int
	delay    func(n int) time.Duration
}

// fixedCleanupRetry waits the same backoff between every attempt.
func fixedCleanupRetry(attempts int, backoff time.Duration) cleanupRetry {
	return cleanupRetry{attempts: attempts, delay: func(int) time.Duration { return backoff }}
}

// agentConfigCleanupRetry backs off exponentially: an un-revoked credential stays live, so a few seconds to rule out a transient cause is worth it.
func agentConfigCleanupRetry() cleanupRetry {
	return cleanupRetry{attempts: agentConfigCleanupAttempts, delay: func(n int) time.Duration { return agentConfigCleanupRetryDelay << n }}
}

// retryCleanup runs an idempotent step until it succeeds or r is spent, never waiting after the final attempt.
func retryCleanup(ctx context.Context, logger *slog.Logger, r cleanupRetry, step string, fn func() error, logAttrs ...any) error {
	var lastErr error
	for attempt := range r.attempts {
		if lastErr = fn(); lastErr == nil {
			if attempt > 0 {
				logger.Info(step+" succeeded on retry", append([]any{"attempt", attempt + 1}, logAttrs...)...)
			}
			return nil
		}
		if attempt == r.attempts-1 {
			break
		}
		delay := r.delay(attempt)
		logger.Warn(step+" failed, retrying",
			append([]any{"attempt", attempt + 1, "of", r.attempts, "retryIn", delay, "error", lastErr}, logAttrs...)...)
		select {
		case <-ctx.Done():
			return errors.Join(lastErr, ctx.Err())
		case <-time.After(delay):
		}
	}
	return lastErr
}

// clearInheritedA2AState drops card, publication and proxy rows a deleted same-named agent left behind.
func (s *AgentGatewayService) clearInheritedA2AState(ctx context.Context, ouID, projectName, agentName string) error {
	if s.a2aCardRepo != nil {
		if err := s.a2aCardRepo.DeleteForAgent(ctx, ouID, projectName, agentName); err != nil {
			return fmt.Errorf("failed to clear leftover A2A agent card rows: %w", err)
		}
	}
	if s.a2aPublisher != nil {
		if err := s.a2aPublisher.DeleteForAgent(ctx, ouID, projectName, agentName); err != nil {
			return fmt.Errorf("failed to clear leftover A2A publication and proxy rows: %w", err)
		}
	}
	return nil
}

// deleteAgentAPIArtifact removes the agent's per-environment API artifacts and
// tells gateways holding an A2A Agent for them to drop it. It returns what it
// could not remove; the component is already gone by then, so deleting the
// agent again retries the rest. isA2AAgent is false for a REST or chat agent, and when the agent could not be
// read; deployment rows then stand in for it.
func (s *AgentGatewayService) deleteAgentAPIArtifact(ctx context.Context, ouID, projectName, agentName string, isA2AAgent bool) error {
	err := s.cleanUpAgentAPIArtifacts(ctx, ouID, projectName, agentName, isA2AAgent)
	if err != nil {
		return fmt.Errorf("the agent was deleted but its A2A state was not fully removed; delete the agent again to retry: %w", err)
	}
	return nil
}

// cleanUpAgentAPIArtifacts is deleteAgentAPIArtifact's work. Each environment's artifact goes in one transaction
// with its exposure, proxy and card rows and its delete events, so a failure leaves that environment whole for
// the retry. The agent's rows in environments without an artifact go once every artifact is gone.
func (s *AgentGatewayService) cleanUpAgentAPIArtifacts(ctx context.Context, ouID, projectName, agentName string, isA2AAgent bool) error {
	// Listed by handle, not by pipeline: routing can put an artifact in any org environment.
	prefix := agentAPIArtifactHandlePrefix(projectName, agentName)
	artifacts, err := s.artifactRepo.ListByHandlePrefix(ctx, models.AgentArtifactKinds, prefix, ouID)
	if err != nil {
		return fmt.Errorf("failed to list agent API artifacts for cleanup: %w", err)
	}

	var errs []error
	for _, artifact := range artifacts {
		key := agentExposureKey{OUID: ouID, ProjectName: projectName, AgentName: agentName}
		if err := s.dropAgentArtifact(ctx, key, strings.TrimPrefix(artifact.Handle, prefix), artifact, isA2AAgent); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete agent API artifact %s: %w", artifact.Handle, err))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	if s.a2aPublisher != nil {
		if err := s.a2aPublisher.DeleteForAgent(ctx, ouID, projectName, agentName); err != nil {
			errs = append(errs, err)
		}
	}
	if s.a2aCardRepo != nil {
		if err := s.a2aCardRepo.DeleteForAgent(ctx, ouID, projectName, agentName); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete A2A agent card rows: %w", err))
		}
	}
	return errors.Join(errs...)
}

// dropAgentArtifact deletes one per-environment agent artifact, with the agent's A2A rows in its environment, writing
// its delete events in the same transaction. An artifact whose handle names no environment UUID goes alone.
func (s *AgentGatewayService) dropAgentArtifact(
	ctx context.Context, key agentExposureKey, envID string, artifact *models.Artifact, isA2AAgent bool,
) error {
	if s.a2aPublisher == nil {
		return s.artifactRepo.Delete(s.db, artifact.UUID.String())
	}
	envUUID, envErr := uuid.Parse(envID)
	key.EnvironmentUUID = envUUID
	return s.a2aPublisher.tx(ctx, func(tx *gorm.DB) error {
		if envErr != nil {
			return s.a2aPublisher.exposure.DropArtifact(ctx, tx, agentArtifactRef(artifact))
		}
		return s.a2aPublisher.DropAgentArtifact(ctx, tx, key, agentArtifactRef(artifact), isA2AAgent)
	})
}

// checkNoOrphanedConfigs refuses to create an agent whose name still has agent_configurations
// rows attached to it.
//
// Configurations are keyed by (ou_id, project_name, agent_id) where agent_id is the agent's
// *name*, not a per-instance identity, and DeleteForAgentDeletion deliberately keeps a row
// whose external teardown failed so the proxy, key and deployment it names can still be found
// and revoked. Those two facts together mean a new agent created with a deleted agent's name
// would adopt the surviving rows — and with them a live, un-rotated LLM proxy credential — with
// nothing in the UI to distinguish it from a fresh binding. Refusing the create is what keeps
// that from happening silently; deleting the agent again re-runs the revocation and clears the
// name once it succeeds.
//
// A list failure is fatal here rather than best-effort: without the list we cannot tell a clean
// name from an orphaned one, and guessing wrong leaks a credential.
func (s *AgentGatewayService) checkNoOrphanedConfigs(ctx context.Context, ouID, projectName, agentName string) error {
	listResp, err := s.agentConfigurationService.List(ctx, ouID, projectName, agentName, 1, 0)
	if err != nil {
		s.logger.Error("Failed to check for orphaned agent configurations before create",
			"agentName", agentName, "ouID", ouID, "projectName", projectName, "error", err)
		return fmt.Errorf("failed to check for leftover configurations for agent %s: %w", agentName, err)
	}
	if listResp.Pagination.Count == 0 {
		return nil
	}

	s.logger.Error("Refusing to create agent: configurations from a previously deleted agent with this name were not fully revoked",
		"agentName", agentName, "ouID", ouID, "projectName", projectName, "orphanedConfigs", listResp.Pagination.Count)
	return fmt.Errorf("%w: %d configuration(s) for agent %q in project %q remain; deletion cleanup may still be running, so retry shortly, then delete the agent again to re-attempt revoking them, or use a different name",
		utils.ErrOrphanedAgentConfigsExist, listResp.Pagination.Count, agentName, projectName)
}

// deleteAgentLLMConfigurations removes all LLM and MCP configurations for an agent during agent deletion.
// isExternalAgent must be resolved by the caller before the component is deleted so this function
// requires no OC calls. Calls DeleteForAgentDeletion which skips Component/Workload/ReleaseBinding
// patching and SecretReference CR deletion (handled by component teardown).
// After all configs are deleted, the shared AI application records (one per agent+env) are removed.
// Best-effort: individual failures are logged but do not abort the agent deletion.
func (s *AgentGatewayService) deleteAgentLLMConfigurations(ctx context.Context, ouID, projectName, agentName string, isExternalAgent bool) {
	listResp, err := s.agentConfigurationService.List(ctx, ouID, projectName, agentName, 1000, 0)
	if err != nil {
		s.logger.Warn("Failed to list agent configurations for cleanup", "agentName", agentName, "error", err)
		return
	}
	for _, cfg := range listResp.Configs {
		configUUID, parseErr := uuid.Parse(cfg.UUID)
		if parseErr != nil {
			s.logger.Warn("Failed to parse config UUID during agent deletion", "uuid", cfg.UUID, "type", cfg.Type, "error", parseErr)
			continue
		}
		delErr := retryCleanup(ctx, s.logger, agentConfigCleanupRetry(), "Configuration teardown", func() error {
			return s.agentConfigurationService.DeleteForAgentDeletion(ctx, configUUID, ouID, projectName, agentName, isExternalAgent)
		}, "configUUID", cfg.UUID)
		if delErr != nil {
			// DeleteForAgentDeletion keeps the agent_configurations row when any external
			// step failed, so the record of what still needs revoking survives. Creating an
			// agent with this name is refused until it is gone (see checkNoOrphanedConfigs).
			s.logger.Error("Gave up revoking configuration during agent deletion; its LLM proxy credential may still be live",
				"configUUID", cfg.UUID, "type", cfg.Type, "agentName", agentName, "ouID", ouID,
				"attempts", agentConfigCleanupAttempts, "error", delErr)
		}
	}

	// Delete all AI application records for this agent (one per environment) now that all configs are gone.
	if delErr := s.aiApplicationService.DeleteAllByAgent(ctx, ouID, projectName, agentName); delErr != nil {
		s.logger.Warn("Failed to delete AI applications during agent deletion", "agentName", agentName, "error", delErr)
	}
}

// resolvedTracingConfig holds resolved instrumentation config values.
type resolvedTracingConfig struct {
	EnableAutoInstrumentation bool
}

// resolvedCORSConfig holds resolved CORS and API key security config values.
type resolvedCORSConfig struct {
	EnableApiKeySecurity bool
	CORSEnabled          bool
	CORSAllowOrigins     []string
	CORSAllowMethods     []string
	CORSAllowHeaders     []string
	CORSAllowCredentials bool
	// OAuth security (mutually exclusive with EnableApiKeySecurity).
	EnableOAuthSecurity   bool
	OAuthIssuers          []string
	OAuthAudiences        []string
	OAuthHeaderName       string
	OAuthAuthHeaderPrefix string
	OAuthForwardToken     bool
}

// agentConfigFromAPIConfig is the environment's config row carrying api; callers set the remaining fields.
func agentConfigFromAPIConfig(ouID, projectName, agentName, envName string, api resolvedCORSConfig) *models.AgentConfig {
	return &models.AgentConfig{
		OUID:                  ouID,
		ProjectName:           projectName,
		AgentName:             agentName,
		EnvironmentName:       envName,
		EnableApiKeySecurity:  api.EnableApiKeySecurity,
		CORSEnabled:           api.CORSEnabled,
		CORSAllowOrigins:      api.CORSAllowOrigins,
		CORSAllowMethods:      api.CORSAllowMethods,
		CORSAllowHeaders:      api.CORSAllowHeaders,
		CORSAllowCredentials:  api.CORSAllowCredentials,
		EnableOAuthSecurity:   api.EnableOAuthSecurity,
		OAuthIssuers:          api.OAuthIssuers,
		OAuthAudiences:        api.OAuthAudiences,
		OAuthHeaderName:       api.OAuthHeaderName,
		OAuthAuthHeaderPrefix: api.OAuthAuthHeaderPrefix,
		OAuthForwardToken:     api.OAuthForwardToken,
	}
}

// resolveTracingConfig resolves instrumentation config: request > DB > default (true if withDefaults, else false).
func resolveTracingConfig(existingConfig *models.AgentConfig, enableAutoInstrumentation *bool, withDefaults bool) resolvedTracingConfig {
	resolved := resolvedTracingConfig{EnableAutoInstrumentation: withDefaults}
	if existingConfig != nil {
		resolved.EnableAutoInstrumentation = existingConfig.EnableAutoInstrumentation
	}
	if enableAutoInstrumentation != nil {
		resolved.EnableAutoInstrumentation = *enableAutoInstrumentation
	}
	return resolved
}

// resolveAPIConfig resolves CORS, API key, and OAuth security config: request > DB > env-var defaults (only if withDefaults).
func resolveAPIConfig(existingConfig *models.AgentConfig, enableApiKeySecurity *bool, corsConfig *gwspec.CORSConfig, enableOAuthSecurity *bool, oauthConfig *gwspec.OAuthConfig, withDefaults bool) resolvedCORSConfig {
	var resolved resolvedCORSConfig
	if withDefaults {
		defaultCORS := config.GetAgentWorkloadConfig().CORS
		resolved = resolvedCORSConfig{
			EnableApiKeySecurity: true,
			CORSEnabled:          true,
			CORSAllowOrigins:     strings.Split(defaultCORS.AllowOrigin, ","),
			CORSAllowMethods:     strings.Split(defaultCORS.AllowMethods, ","),
			CORSAllowHeaders:     strings.Split(defaultCORS.AllowHeaders, ","),
			CORSAllowCredentials: defaultCORS.AllowCredentials,
			EnableOAuthSecurity:  false,
			OAuthForwardToken:    models.DefaultOAuthForwardToken,
		}
	}

	if existingConfig != nil {
		resolved.EnableApiKeySecurity = existingConfig.EnableApiKeySecurity
		resolved.CORSEnabled = existingConfig.CORSEnabled
		if len(existingConfig.CORSAllowOrigins) > 0 {
			resolved.CORSAllowOrigins = existingConfig.CORSAllowOrigins
		}
		if len(existingConfig.CORSAllowMethods) > 0 {
			resolved.CORSAllowMethods = existingConfig.CORSAllowMethods
		}
		if len(existingConfig.CORSAllowHeaders) > 0 {
			resolved.CORSAllowHeaders = existingConfig.CORSAllowHeaders
		}
		resolved.CORSAllowCredentials = existingConfig.CORSAllowCredentials
		resolved.EnableOAuthSecurity = existingConfig.EnableOAuthSecurity
		resolved.OAuthIssuers = existingConfig.OAuthIssuers
		resolved.OAuthAudiences = existingConfig.OAuthAudiences
		resolved.OAuthForwardToken = existingConfig.OAuthForwardToken
		if existingConfig.OAuthHeaderName != "" {
			resolved.OAuthHeaderName = existingConfig.OAuthHeaderName
		}
		if existingConfig.OAuthAuthHeaderPrefix != "" {
			resolved.OAuthAuthHeaderPrefix = existingConfig.OAuthAuthHeaderPrefix
		}
	}

	if enableApiKeySecurity != nil {
		resolved.EnableApiKeySecurity = *enableApiKeySecurity
	}
	if corsConfig != nil {
		if corsConfig.Enabled != nil {
			resolved.CORSEnabled = *corsConfig.Enabled
		}
		if len(corsConfig.AllowOrigin) > 0 {
			resolved.CORSAllowOrigins = corsConfig.AllowOrigin
		}
		if len(corsConfig.AllowMethods) > 0 {
			resolved.CORSAllowMethods = corsConfig.AllowMethods
		}
		if len(corsConfig.AllowHeaders) > 0 {
			resolved.CORSAllowHeaders = corsConfig.AllowHeaders
		}
		if corsConfig.AllowCredentials != nil {
			resolved.CORSAllowCredentials = *corsConfig.AllowCredentials
		}
	}

	if enableOAuthSecurity != nil {
		resolved.EnableOAuthSecurity = *enableOAuthSecurity
	}
	if oauthConfig != nil {
		if oauthConfig.Issuers != nil {
			resolved.OAuthIssuers = oauthConfig.Issuers
		}
		if oauthConfig.Audiences != nil {
			resolved.OAuthAudiences = oauthConfig.Audiences
		}
		if oauthConfig.HeaderName != nil && *oauthConfig.HeaderName != "" {
			resolved.OAuthHeaderName = *oauthConfig.HeaderName
		}
		if oauthConfig.AuthHeaderPrefix != nil && *oauthConfig.AuthHeaderPrefix != "" {
			resolved.OAuthAuthHeaderPrefix = *oauthConfig.AuthHeaderPrefix
		}
		if oauthConfig.ForwardToken != nil {
			resolved.OAuthForwardToken = *oauthConfig.ForwardToken
		}
	}

	// Header name/prefix are NOT NULL columns — guarantee non-empty values so the
	// Upsert never writes an empty string over the gateway-compatible defaults.
	if resolved.OAuthHeaderName == "" {
		resolved.OAuthHeaderName = models.DefaultOAuthHeaderName
	}
	if resolved.OAuthAuthHeaderPrefix == "" {
		resolved.OAuthAuthHeaderPrefix = models.DefaultOAuthAuthHeaderPrefix
	}

	return resolved
}

// validateAuthExclusivity rejects configs that enable both API key and OAuth
// security — only one auth policy may be attached to an agent endpoint.
func validateAuthExclusivity(cfg resolvedCORSConfig) error {
	if cfg.EnableApiKeySecurity && cfg.EnableOAuthSecurity {
		return fmt.Errorf("%w: API key and OAuth security are mutually exclusive — enable only one", utils.ErrInvalidInput)
	}
	return nil
}

// validateOAuthSecurityConfig rejects an OAuth-enabled config with no issuers.
// Issuers reference the environment's identity providers; there is no platform
// default, so an empty list is a configuration error. (Membership of each issuer
// in the environment's identity providers is validated separately where the
// environment context is available.)
func validateOAuthSecurityConfig(cfg resolvedCORSConfig) error {
	if cfg.EnableOAuthSecurity && len(cfg.OAuthIssuers) == 0 {
		return fmt.Errorf("%w: OAuth security requires at least one identity provider issuer", utils.ErrInvalidInput)
	}
	return nil
}

// validateOAuthIssuersInEnvironment rejects any OAuth issuer that is not one of
// the environment's configured identity providers. Issuers are gateway-owned;
// the agent can only reference providers that exist in its target environment.
func (s *AgentGatewayService) validateOAuthIssuersInEnvironment(envUUID string, cfg resolvedCORSConfig) error {
	if !cfg.EnableOAuthSecurity || envUUID == "" {
		return nil
	}
	providers, err := s.gatewayRepo.ListIdentityProvidersByEnvironment(envUUID)
	if err != nil {
		return err
	}
	valid := make(map[string]bool, len(providers))
	for _, p := range providers {
		valid[p.Name] = true
	}
	for _, issuer := range cfg.OAuthIssuers {
		if !valid[issuer] {
			return fmt.Errorf("%w: identity provider %q is not configured for this environment", utils.ErrInvalidInput, issuer)
		}
	}
	return nil
}

// buildPolicies builds the api-configuration trait policies from resolved config.
// Returns a non-nil slice so the "no authentication" mode (no CORS, no auth)
// marshals to an empty JSON array — the api-configuration trait rejects a null
// policies field ("policies must be of type array").
func buildPolicies(cfg resolvedCORSConfig) []map[string]interface{} {
	policies := make([]map[string]interface{}, 0)
	// CORS must be first so preflight OPTIONS requests are handled before
	// any auth policy runs. Exactly one auth policy (api-key-auth or jwt-auth)
	// is appended after — mutual exclusivity is enforced upstream.
	if cfg.CORSEnabled {
		policies = append(policies, corsPolicy(cfg.CORSAllowOrigins, cfg.CORSAllowMethods, cfg.CORSAllowHeaders, cfg.CORSAllowCredentials))
	}
	if cfg.EnableApiKeySecurity {
		policies = append(policies, apiKeyAuthPolicy())
	}
	if cfg.EnableOAuthSecurity {
		// Issuers are passed through as persisted — empty issuers are rejected by
		// validateOAuthSecurityConfig before reaching here, so no default is invented.
		policies = append(policies, oauthPolicy(oauthPolicyParams{
			Issuers:          cfg.OAuthIssuers,
			Audiences:        cfg.OAuthAudiences,
			HeaderName:       cfg.OAuthHeaderName,
			AuthHeaderPrefix: cfg.OAuthAuthHeaderPrefix,
			ForwardToken:     cfg.OAuthForwardToken,
		}))
	}
	return policies
}

// UpdateAgentDeploySettings updates per-environment deploy settings (CORS, API key security,
// auto instrumentation) on an existing release binding without redeploying or promoting the
// agent. Triggers a pod rollout so policy changes take effect immediately. Any field omitted
// from the request keeps its current DB value.
func (s *AgentGatewayService) UpdateAgentDeploySettings(ctx context.Context, ouID, projectName, agentName string, req *gwspec.UpdateAgentDeploySettingsRequest) error {
	s.logger.Info("Updating agent deploy settings", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", req.EnvironmentName)

	if req.EnvironmentName == "" {
		return fmt.Errorf("%w: environmentName is required", utils.ErrInvalidInput)
	}

	agent, err := s.platform.GetAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		return err
	}
	// Hosted agents take their settings through a runtime that is not available.
	if !isExternalProvisioned(agent) {
		return fmt.Errorf("%w: deploy settings of hosted agents", utils.ErrNotImplemented)
	}
	// Both validates the environment and authorizes the caller against its tier
	// — one lookup, because requireEnvTier resolves the same environment and
	// translates the same not-found error. The route's agent:update is the
	// capability; this is the environment axis, and holding one does not imply
	// the other.
	targetEnv, err := s.requireEnvTier(ctx, ouID, req.EnvironmentName)
	if err != nil {
		return err
	}
	req.EnvironmentName = targetEnv.Name
	pubKey, publishes, err := s.agentExposureKeyFor(ctx, agent, ouID, projectName, agentName, req.EnvironmentName, targetEnv.UUID)
	if err != nil {
		return err
	}
	// A direct external environment has nothing that applies deploy settings.
	if !publishes {
		return fmt.Errorf("%w: deploy settings apply only to environments that route this agent through the gateway",
			utils.ErrInvalidInput)
	}

	// Resolve final settings: precedence is request → existing DB
	var existingConfig *models.AgentConfig
	switch cfg, getErr := s.agentConfigRepo.Get(ctx, ouID, projectName, agentName, req.EnvironmentName); {
	case errors.Is(getErr, repositories.ErrAgentConfigNotFound):
		// No existing config for this env — request values / defaults apply.
	case getErr != nil:
		return fmt.Errorf("failed to read agent config for environment %q: %w", req.EnvironmentName, getErr)
	default:
		existingConfig = cfg
	}
	tracingCfg := resolveTracingConfig(existingConfig, req.EnableAutoInstrumentation, false)
	apiCfg := resolveAPIConfig(existingConfig, req.EnableApiKeySecurity, req.CorsConfig, req.EnableOAuthSecurity, req.OauthConfig, false)
	isA2AAgent := a2aCapable(agent)
	if isA2AAgent {
		apiCfg = withA2AVersionHeader(apiCfg)
	}
	cardCORSOverride := resolveCardCORSOverride(existingConfig, req.AgentCardCorsConfig)
	if err := validateCardCORS(isA2AAgent, req.AgentCardCorsConfig, cardCORSOverride); err != nil {
		return err
	}
	resilienceTimeoutSeconds, err := resolveResilienceTimeoutSeconds(existingConfig, req.ResilienceTimeoutSeconds, false)
	if err != nil {
		return err
	}
	if err := validateAuthExclusivity(apiCfg); err != nil {
		return err
	}
	if err := validateOAuthSecurityConfig(apiCfg); err != nil {
		return err
	}
	if err := s.validateOAuthIssuersInEnvironment(targetEnv.UUID, apiCfg); err != nil {
		return err
	}
	var existingInstrumentationVersion *string
	if existingConfig != nil {
		existingInstrumentationVersion = existingConfig.InstrumentationVersion
	}
	resolvedInstrumentationVersion := existingInstrumentationVersion

	var settingsResilienceTimeoutSeconds *int32
	if resilienceTimeoutSeconds > 0 {
		settingsResilienceTimeoutSeconds = &resilienceTimeoutSeconds
	}
	// Persist resolved config so subsequent deploy/promote calls see the current values.
	agentConfig := agentConfigFromAPIConfig(ouID, projectName, agentName, req.EnvironmentName, apiCfg)
	agentConfig.EnableAutoInstrumentation = tracingCfg.EnableAutoInstrumentation
	agentConfig.InstrumentationVersion = resolvedInstrumentationVersion
	agentConfig.ResilienceTimeoutSeconds = settingsResilienceTimeoutSeconds
	agentConfig.SetCardCORSOverride(cardCORSOverride)
	artifact := s.agentEnvArtifactFunc(agent, ouID, projectName, agentName, targetEnv.UUID)
	if err := s.saveDeploySettings(ctx, agent, pubKey, artifact, agentConfig); err != nil {
		s.logger.Error("Failed to persist agent deploy settings", "agentName", agentName, "environment", req.EnvironmentName, "error", err)
		return fmt.Errorf("failed to persist agent deploy settings: %w", err)
	}

	s.logger.Info("Agent deploy settings updated successfully", "agentName", agentName, "environment", req.EnvironmentName)
	return nil
}

// GetAgentDeployments reports where an external agent is reached: one entry
// for the organization's default environment, carrying its gateway URL and
// deployment status when the gateway serves the agent.
func (s *AgentGatewayService) GetAgentDeployments(ctx context.Context, ouID string, projectName string, agentName string) ([]*models.DeploymentResponse, error) {
	agent, err := s.platform.GetAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		return nil, err
	}
	env, err := s.platform.GetEnvironment(ctx, ouID, models.DefaultEnvironmentName)
	if err != nil {
		return nil, err
	}
	deployment := &models.DeploymentResponse{
		AgentName:              agentName,
		ProjectName:            projectName,
		Status:                 deploymentStatusActive,
		Environment:            env.Name,
		EnvironmentDisplayName: env.DisplayName,
		EnvironmentUUID:        env.UUID,
		Endpoints:              []models.Endpoint{},
	}
	deployments := []*models.DeploymentResponse{deployment}
	if s.exposesAgent(agent) {
		s.resolveGatewayDeployments(ctx, agent, ouID, projectName, agentName, deployments)
	}
	return deployments, nil
}

// resolveGatewayDeployments shows each bound environment's deployment as the gateway serves it: every endpoint at
// the agent's gateway URL, and the gateway deployment's status. Best-effort: an environment whose gateway deployment
// can't be read keeps its direct URLs and shows no gateway status.
func (s *AgentGatewayService) resolveGatewayDeployments(
	ctx context.Context, agent *models.AgentResponse, ouID, projectName, agentName string, deployments []*models.DeploymentResponse,
) {
	for _, deployment := range deployments {
		if deployment == nil || deployment.EnvironmentUUID == "" || deployment.Status == deploymentStatusNotDeployed {
			continue
		}
		url, err := s.resolveAgentURL(ctx, agent, ouID, projectName, agentName, deployment.Environment, deployment.EnvironmentUUID)
		if err != nil {
			s.logger.Warn("Failed to resolve the agent's gateway deployment; showing its direct URL",
				"agentName", agentName, "environment", deployment.Environment, "error", err)
			continue
		}
		for i := range deployment.Endpoints {
			deployment.Endpoints[i].URL = url.For(deployment.Endpoints[i].URL)
		}
		if url.Status != nil {
			deployment.GatewayStatus = string(url.Status.Status)
			deployment.GatewayStatusReason = url.Status.Reason
		}
	}
}

// resolveAgentURL says where the agent is reached in the environment and what its gateway deployment shows.
func (s *AgentGatewayService) resolveAgentURL(
	ctx context.Context, agent *models.AgentResponse, ouID, projectName, agentName, environmentName, environmentUUID string,
) (AgentURL, error) {
	key, err := agentExposureKeyOf(agent, ouID, projectName, agentName, environmentName, environmentUUID)
	if err != nil {
		return AgentURL{}, err
	}
	url, err := s.exposure.ResolveURL(ctx, key)
	if err != nil {
		return AgentURL{}, fmt.Errorf("failed to resolve agent %s's gateway URL in %s: %w", agentName, environmentName, err)
	}
	return url, nil
}

// parseAgentUUID returns nil for a missing or malformed UUID; the reconciler then checks by name.
func parseAgentUUID(raw string) *uuid.UUID {
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil
	}
	return &id
}

// exposesAgent is true for an agent its environment's ingress gateway serves, on a service wired to expose it.
func (s *AgentGatewayService) exposesAgent(agent *models.AgentResponse) bool {
	_, ok := agentGatewayShapeOf(agent)
	return ok && s.exposure != nil && s.a2aPublisher != nil
}

// agentEnvArtifactFunc ensures, in the exposure transaction, the agent's artifact in the environment with the kind
// its gateway shape names, switching a legacy REST agent's Agent row in place; nil for an agent the gateway does
// not serve.
func (s *AgentGatewayService) agentEnvArtifactFunc(
	agent *models.AgentResponse, ouID, projectName, agentName, envUUID string,
) func(tx *gorm.DB) (*models.Artifact, error) {
	shape, ok := agentGatewayShapeOf(agent)
	if !ok {
		return nil
	}
	return func(tx *gorm.DB) (*models.Artifact, error) {
		artifact, err := ensureExposedAgentEnvAPIArtifact(tx, s.artifactRepo, ouID, projectName, agentName, envUUID, shape)
		if err != nil {
			return nil, fmt.Errorf("failed to ensure the agent's API artifact for environment %s: %w", envUUID, err)
		}
		return artifact, nil
	}
}

// saveDeploySettings saves the environment's config in one transaction with the agent's gateway exposure, so
// neither lands without the other; the agent republishes as SaveAndRepublish describes.
func (s *AgentGatewayService) saveDeploySettings(
	ctx context.Context, agent *models.AgentResponse, key agentExposureKey,
	artifact func(tx *gorm.DB) (*models.Artifact, error), cfg *models.AgentConfig,
) error {
	save := func(tx *gorm.DB) error { return s.agentConfigRepo.WithTx(tx).Upsert(ctx, cfg) }
	return s.a2aPublisher.SaveAndRepublish(ctx, agent, key, artifact, save)
}

// agentExposureKeyOf names the agent's exposure in the environment.
func agentExposureKeyOf(
	agent *models.AgentResponse, ouID, projectName, agentName, environmentName, environmentUUID string,
) (agentExposureKey, error) {
	envUUID, err := uuid.Parse(environmentUUID)
	if err != nil {
		return agentExposureKey{}, fmt.Errorf("environment %q has an unparseable UUID %q: %w", environmentName, environmentUUID, err)
	}
	return agentExposureKey{
		OUID: ouID, ProjectName: projectName, AgentName: agentName, EnvironmentName: environmentName,
		EnvironmentUUID: envUUID, AgentUUID: parseAgentUUID(agent.UUID),
	}, nil
}

// agentExposureKeyFor is the agent's exposure in the environment and whether a settings save there re-renders it on
// the gateway: always for a platform agent the gateway serves, and for an external agent only while the
// environment is proxied.
func (s *AgentGatewayService) agentExposureKeyFor(
	ctx context.Context,
	agent *models.AgentResponse,
	ouID, projectName, agentName, environmentName, environmentUUID string,
) (agentExposureKey, bool, error) {
	if !s.exposesAgent(agent) {
		return agentExposureKey{}, false, nil
	}
	key, err := agentExposureKeyOf(agent, ouID, projectName, agentName, environmentName, environmentUUID)
	if err != nil {
		return agentExposureKey{}, false, err
	}
	if !isExternalProvisioned(agent) {
		return key, true, nil
	}
	proxied, err := s.a2aPublisher.ShouldPublish(ctx, nil, agent, ouID, projectName, key.EnvironmentUUID)
	if err != nil || !proxied {
		return agentExposureKey{}, false, err
	}
	return key, true, nil
}

// resilienceOf converts the resilience of an agent-create model config.
func resilienceOf(r *spec.Resilience) *models.Resilience {
	if r == nil {
		return nil
	}
	return &models.Resilience{Timeout: r.Timeout, IdleTimeout: r.IdleTimeout}
}
