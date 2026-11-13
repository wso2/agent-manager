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

package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/constants"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"gorm.io/gorm"
)

type AgentManagerService interface {
	ListAgents(ctx context.Context, ouID string, projName string, labelFilter map[string]string, limit int32, offset int32) ([]*models.AgentResponse, int32, error)
	ListOrgAgents(ctx context.Context, ouID string) ([]*models.AgentSummary, error)
	CreateAgent(ctx context.Context, ouID string, projectName string, req *spec.CreateAgentRequest) error
	// SetEntitlementChecker installs an optional subscription check for agent
	// creates. It is called once during startup, before the HTTP server begins serving.
	SetEntitlementChecker(checker EntitlementChecker)
	UpdateAgentBasicInfo(ctx context.Context, ouID string, projectName string, agentName string, req *spec.UpdateAgentBasicInfoRequest) (*models.AgentResponse, error)
	DeleteAgent(ctx context.Context, ouID string, projectName string, agentName string) error
	GetAgent(ctx context.Context, ouID string, projectName string, agentName string) (*models.AgentResponse, error)
	GetAgentDeployments(ctx context.Context, ouID string, projectName string, agentName string) ([]*models.DeploymentResponse, error)
	GetAgentEnvConfig(ctx context.Context, ouID string, projectName string, agentName string, environment string) (*models.AgentConfig, error)
	GenerateName(ctx context.Context, ouID string, payload spec.ResourceNameRequest) (string, error)
	UpdateAgentDeploySettings(ctx context.Context, ouID string, projectName string, agentName string, req *spec.UpdateAgentDeploySettingsRequest) error
	GetAgentIdentity(ctx context.Context, ouID string, projectName string, agentName string) ([]models.AgentIdentityEnvironmentView, error)
	RegenerateAgentIdentitySecret(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (*models.AgentRegenerateSecretResponse, error)
	RevokeAgentIdentitySecret(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (models.AgentRevokeSecretResponse, error)
	ProvisionAgentIdentity(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (view models.AgentIdentityEnvironmentView, alreadyExisted bool, err error)
	RetryAgentIdentityProvisioning(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (models.AgentIdentityEnvironmentView, error)
	GetAgentRoles(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) ([]thundersvc.ThunderRole, error)
	GetAgentGroups(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) ([]thundersvc.ThunderGroup, error)
}

type agentManagerService struct {
	db                        *gorm.DB
	platform                  repositories.PlatformRepository
	tokenManagerService       AgentTokenManagerService
	agentConfigRepo           repositories.AgentConfigRepository
	agentConfigurationService AgentConfigurationService
	artifactRepo              repositories.ArtifactRepository
	aiApplicationService      *AIApplicationService
	gatewayRepo               repositories.GatewayRepository
	agentThunderProvisioning  AgentThunderProvisioningService
	monitorManagerService     MonitorManagerService
	identityClient            thundersvc.IdentityClient
	entitlementChecker        EntitlementChecker
	a2aPublisher              *A2APublisher
	a2aCardRepo               repositories.A2AAgentCardRepository
	deploymentRepo            repositories.DeploymentRepository
	gatewayEventsService      *GatewayEventsService
	exposure                  *AgentExposurePublisher
	logger                    *slog.Logger
}

func NewAgentManagerService(
	db *gorm.DB,
	platform repositories.PlatformRepository,
	tokenManagerService AgentTokenManagerService,
	agentConfigRepo repositories.AgentConfigRepository,
	agentConfigurationService AgentConfigurationService,
	artifactRepo repositories.ArtifactRepository,
	aiApplicationService *AIApplicationService,
	gatewayRepo repositories.GatewayRepository,
	agentThunderProvisioning AgentThunderProvisioningService,
	monitorManagerService MonitorManagerService,
	identityClient thundersvc.IdentityClient,
	a2aPublisher *A2APublisher,
	a2aCardRepo repositories.A2AAgentCardRepository,
	deploymentRepo repositories.DeploymentRepository,
	gatewayEventsService *GatewayEventsService,
	exposure *AgentExposurePublisher,
	logger *slog.Logger,
) AgentManagerService {
	return &agentManagerService{
		db:                        db,
		platform:                  platform,
		tokenManagerService:       tokenManagerService,
		agentConfigRepo:           agentConfigRepo,
		agentConfigurationService: agentConfigurationService,
		agentThunderProvisioning:  agentThunderProvisioning,
		monitorManagerService:     monitorManagerService,
		identityClient:            identityClient,
		a2aPublisher:              a2aPublisher,
		a2aCardRepo:               a2aCardRepo,
		deploymentRepo:            deploymentRepo,
		gatewayEventsService:      gatewayEventsService,
		exposure:                  exposure,
		artifactRepo:              artifactRepo,
		aiApplicationService:      aiApplicationService,
		gatewayRepo:               gatewayRepo,
		logger:                    logger,
	}
}

// requireIngressGateway refuses an environment that has no INGRESS or BOTH gateway to expose an API agent through.
func (s *agentManagerService) requireIngressGateway(ouID string, env *models.EnvironmentResponse) error {
	envUUID, err := uuid.Parse(env.UUID)
	if err != nil {
		return fmt.Errorf("environment %s has an invalid UUID %q: %w", env.Name, env.UUID, err)
	}
	gateway, err := findIngressGateway(s.gatewayRepo, ouID, envUUID)
	if err != nil {
		return err
	}
	if gateway == nil {
		return fmt.Errorf("%w: environment %s has no ingress gateway", utils.ErrEnvironmentHasNoGateway, env.Name)
	}
	return nil
}

// Build type constants
const (
	BuildTypeBuildpack = "buildpack"
	BuildTypeDocker    = "docker"
)

// -----------------------------------------------------------------------------
// Mapping Helper Functions
// -----------------------------------------------------------------------------

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

func (s *agentManagerService) GetAgent(ctx context.Context, ouID string, projectName string, agentName string) (*models.AgentResponse, error) {
	agent, err := s.platform.GetAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		return nil, err
	}
	agentConfig, configErr := s.agentConfigRepo.Get(ctx, ouID, projectName, agentName, models.DefaultEnvironmentName)
	switch {
	case errors.Is(configErr, repositories.ErrAgentConfigNotFound):
		defaultEnabled := true
		defaultCORSEnabled := true
		defaultOAuthDisabled := false
		defCORS := config.GetAgentWorkloadConfig().CORS
		defaultResilience := constants.DefaultResilienceTimeoutSeconds
		agent.Configurations = &models.Configurations{
			EnableAutoInstrumentation: &defaultEnabled,
			EnableApiKeySecurity:      &defaultEnabled,
			CorsConfig: &models.CorsConfig{
				Enabled:          &defaultCORSEnabled,
				AllowOrigin:      strings.Split(defCORS.AllowOrigin, ","),
				AllowMethods:     strings.Split(defCORS.AllowMethods, ","),
				AllowHeaders:     strings.Split(defCORS.AllowHeaders, ","),
				AllowCredentials: &defCORS.AllowCredentials,
			},
			EnableOAuthSecurity:      &defaultOAuthDisabled,
			ResilienceTimeoutSeconds: &defaultResilience,
		}
	case configErr != nil:
		return nil, fmt.Errorf("failed to read agent config: %w", configErr)
	default:
		agent.Configurations = &models.Configurations{
			EnableAutoInstrumentation: &agentConfig.EnableAutoInstrumentation,
			InstrumentationVersion:    agentConfig.InstrumentationVersion,
			EnableApiKeySecurity:      &agentConfig.EnableApiKeySecurity,
			CorsConfig: &models.CorsConfig{
				Enabled:          &agentConfig.CORSEnabled,
				AllowOrigin:      agentConfig.CORSAllowOrigins,
				AllowMethods:     agentConfig.CORSAllowMethods,
				AllowHeaders:     agentConfig.CORSAllowHeaders,
				AllowCredentials: &agentConfig.CORSAllowCredentials,
			},
			EnableOAuthSecurity:      &agentConfig.EnableOAuthSecurity,
			OAuthConfig:              oauthConfigFromAgentConfig(agentConfig),
			ResilienceTimeoutSeconds: agentConfig.ResilienceTimeoutSeconds,
		}
	}
	s.populateCreatedBy(ctx, ouID, projectName, agentName, agent)
	return agent, nil
}

// populateCreatedBy best-effort resolves and attaches who created this agent.
func (s *agentManagerService) populateCreatedBy(ctx context.Context, ouID, projectName, agentName string, agent *models.AgentResponse) {
	if s.agentThunderProvisioning == nil || s.identityClient == nil {
		return
	}
	views, err := s.agentThunderProvisioning.GetIdentityViews(ctx, ouID, projectName, agentName)
	if err != nil {
		s.logger.Warn("Failed to fetch agent identity views for createdBy", "agentName", agentName, "ouID", ouID, "error", err)
		return
	}
	// requestedBy is captured once at creation time and copied to every
	// environment's binding, so any non-empty value is equivalent.
	var requestedBy string
	for _, v := range views {
		if v.RequestedBy != "" {
			requestedBy = v.RequestedBy
			break
		}
	}
	if requestedBy == "" {
		return
	}

	createdBy := &models.AgentCreatedBy{ID: requestedBy}
	user, err := s.identityClient.GetUser(ctx, requestedBy)
	// A nil user with no error shouldn't happen, but this path is best-effort
	// decoration of a GetAgent response — never let it panic the request.
	if err != nil || user == nil {
		if err != nil && !thundersvc.IsNotFound(err) {
			s.logger.Warn("Failed to resolve agent creator", "agentName", agentName, "requestedBy", requestedBy, "error", err)
		}
		agent.CreatedBy = createdBy
		return
	}
	if username, ok := user.Attributes["username"].(string); ok && username != "" {
		createdBy.Display = username
	} else {
		createdBy.Display = user.Display
	}
	agent.CreatedBy = createdBy
}

func (s *agentManagerService) ListAgents(ctx context.Context, ouID string, projName string, labelFilter map[string]string, limit int32, offset int32) ([]*models.AgentResponse, int32, error) {
	s.logger.Info("Listing agents", "ouID", ouID, "projectName", projName, "limit", limit, "offset", offset)
	// Validate organization exists
	_, err := s.platform.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to find organization", "ouID", ouID, "error", err)
		return nil, 0, err
	}

	// Fetch all agent components
	agents, err := s.platform.ListAgents(ctx, ouID, projName)
	if err != nil {
		s.logger.Error("Failed to list agents from repository", "ouID", ouID, "projectName", projName, "error", err)
		return nil, 0, fmt.Errorf("failed to list agents: %w", err)
	}

	// Filter before computing total and paginating so both stay correct.
	// agent.Labels already arrives populated from the component's own
	// metadata (see convertComponentFromTyped), no separate fetch needed.
	if len(labelFilter) > 0 {
		filtered := make([]*models.AgentResponse, 0, len(agents))
		for _, agent := range agents {
			if utils.LabelsMatch(agent.Labels, labelFilter) {
				filtered = append(filtered, agent)
			}
		}
		agents = filtered
	}

	total := int32(len(agents))
	paginatedAgents := paginateSlice(agents, offset, limit)
	s.logger.Info("Listed agents successfully", "ouID", ouID, "projName", projName, "totalAgents", total, "returnedAgents", len(paginatedAgents))
	return paginatedAgents, total, nil
}

// ListOrgAgents returns every agent across all projects in the organization, unpaginated,
// with each agent's project name and display name attached.
func (s *agentManagerService) ListOrgAgents(ctx context.Context, ouID string) ([]*models.AgentSummary, error) {
	s.logger.Info("Listing all agents in organization", "ouID", ouID)

	// Validate organization exists
	if _, err := s.platform.GetOrganization(ctx, ouID); err != nil {
		s.logger.Error("Failed to find organization", "ouID", ouID, "error", err)
		return nil, err
	}

	projects, err := listOrgProjects(ctx, s.platform, ouID)
	if err != nil {
		s.logger.Error("Failed to list projects", "ouID", ouID, "error", err)
		return nil, err
	}

	agents, err := fetchAcrossOrgProjects(ctx, projects, ouID, s.platform.ListAgents)
	if err != nil {
		s.logger.Error("Failed to list agents across projects", "ouID", ouID, "error", err)
		return nil, err
	}

	// The project list is already in hand from the fan-out above, so attaching each
	// agent's project display name costs an in-memory lookup, not another round trip.
	projectDisplayNames := make(map[string]string, len(projects))
	for _, p := range projects {
		projectDisplayNames[p.Name] = p.DisplayName
	}
	summaries := make([]*models.AgentSummary, len(agents))
	for i, a := range agents {
		summaries[i] = &models.AgentSummary{
			Name:               a.Name,
			DisplayName:        a.DisplayName,
			ProjectName:        a.ProjectName,
			ProjectDisplayName: projectDisplayNames[a.ProjectName],
		}
	}

	s.logger.Info("Listed org agents successfully", "ouID", ouID, "totalAgents", len(summaries))
	return summaries, nil
}

// listOrgProjects fetches every project in the org, wrapping the error consistently for
// the two org-wide listings that need the project list itself (not just the fan-out
// over it): ListOrgAgents (for project display names) and ListKindAgents (as the
// fan-out target list).
func listOrgProjects(ctx context.Context, platform repositories.PlatformRepository, ouID string) ([]*models.ProjectResponse, error) {
	projects, err := platform.ListProjects(ctx, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	return projects, nil
}

// fetchAcrossOrgProjects concurrently invokes fetch for each of the given projects and
// aggregates the results. Shared by any org-wide listing that fans out per-project (see
// also AgentKindService.ListKindAgents).
func fetchAcrossOrgProjects(
	ctx context.Context,
	projects []*models.ProjectResponse,
	ouID string,
	fetch func(ctx context.Context, ouID, projectName string) ([]*models.AgentResponse, error),
) ([]*models.AgentResponse, error) {
	results := make([][]*models.AgentResponse, len(projects))
	g, gctx := errgroup.WithContext(ctx)
	for i, p := range projects {
		i, projectName := i, p.Name
		g.Go(func() error {
			agents, err := fetch(gctx, ouID, projectName)
			if err != nil {
				return err
			}
			results[i] = agents
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}

	all := make([]*models.AgentResponse, 0, len(projects))
	for _, agents := range results {
		all = append(all, agents...)
	}
	return all, nil
}

// paginateSlice returns items[offset:offset+limit], clamping both bounds defensively so
// negative values or out-of-range offsets never panic the slice expression.
func paginateSlice[T any](items []T, offset, limit int32) []T {
	total := int32(len(items))
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}
	if offset >= total {
		return []T{}
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return items[offset:end]
}

// CreateAgent registers an external agent. Hosted (internal) agents need an
// agent runtime, which this control plane does not provide.
func (s *agentManagerService) CreateAgent(ctx context.Context, ouID string, projectName string, req *spec.CreateAgentRequest) error {
	if req.Provisioning.Type != string(utils.ExternalAgent) {
		return fmt.Errorf("%w: only external agents can be created", utils.ErrNotImplemented)
	}
	if err := s.checkNoOrphanedConfigs(ctx, ouID, projectName, req.Name); err != nil {
		return err
	}
	env, err := s.platform.GetEnvironment(ctx, ouID, models.DefaultEnvironmentName)
	if err != nil {
		return err
	}
	isAPIAgent := req.AgentType != nil && req.AgentType.Type == string(utils.AgentTypeAPI)
	if isAPIAgent {
		if err := s.requireIngressGateway(ouID, env); err != nil {
			return err
		}
	}
	if len(req.ModelConfig) > 0 {
		handles := make([]string, 0, len(req.ModelConfig))
		for _, mc := range req.ModelConfig {
			handles = append(handles, mc.ProviderName)
		}
		if err := s.agentConfigurationService.ValidateProvidersInCatalog(ctx, ouID, handles); err != nil {
			return err
		}
	}
	if len(req.McpConfig) > 0 {
		handles := make([]string, 0, len(req.McpConfig))
		for _, mc := range req.McpConfig {
			handles = append(handles, mc.ProxyName)
		}
		if err := s.agentConfigurationService.ValidateMCPProxiesInCatalog(ctx, ouID, projectName, handles); err != nil {
			return err
		}
	}
	if err := s.checkAgentEntitlement(ctx, ouID, req); err != nil {
		return err
	}

	in := repositories.CreateAgentInput{
		Name:             req.Name,
		DisplayName:      req.DisplayName,
		Description:      utils.StrPointerAsStr(req.Description, ""),
		ProvisioningType: req.Provisioning.Type,
	}
	if req.Labels != nil {
		in.Labels = *req.Labels
	}
	if callerClaims := jwtassertion.GetTokenClaims(ctx); callerClaims != nil {
		in.CreatedBy = callerClaims.Sub
	}
	if _, err := s.platform.CreateAgent(ctx, ouID, projectName, in); err != nil {
		return err
	}

	// After the create, so a duplicate-name request cannot wipe a live agent's rows.
	if err := s.clearInheritedA2AState(ctx, ouID, projectName, req.Name); err != nil {
		if errDeletion := s.platform.DeleteAgent(ctx, ouID, projectName, req.Name); errDeletion != nil {
			s.logger.Error("Failed to roll back agent after card cleanup failure", "agentName", req.Name, "error", errDeletion)
		}
		return err
	}

	var agentAPIArtifact *models.Artifact
	if shape, ok := agentGatewayShapeFor(utils.StrPointerAsStr(req.AgentType.SubType, "")); isAPIAgent && ok {
		agentAPIArtifact, err = ensureAgentEnvAPIArtifact(s.db, s.artifactRepo, ouID, projectName, req.Name, env.UUID, shape.ArtifactKind)
		if err != nil {
			if errDeletion := s.platform.DeleteAgent(ctx, ouID, projectName, req.Name); errDeletion != nil {
				s.logger.Error("Failed to roll back agent after API artifact create failure", "agentName", req.Name, "error", errDeletion)
			}
			return fmt.Errorf("failed to create agent API artifact record: %w", err)
		}
	}

	rollbackAgentCreate := func(reason string) {
		s.deleteAgentLLMConfigurations(context.WithoutCancel(ctx), ouID, projectName, req.Name, true)
		if errDeletion := s.platform.DeleteAgent(context.WithoutCancel(ctx), ouID, projectName, req.Name); errDeletion != nil {
			s.logger.Error("Failed to roll back agent", "agentName", req.Name, "reason", reason, "error", errDeletion)
		}
		if agentAPIArtifact != nil {
			if errDeletion := s.artifactRepo.Delete(s.db, agentAPIArtifact.UUID.String()); errDeletion != nil {
				s.logger.Error("Failed to roll back agent API artifact record", "agentName", req.Name, "reason", reason, "error", errDeletion)
			}
		}
	}
	if len(req.ModelConfig) > 0 {
		if err := s.createAgentLLMConfigs(ctx, ouID, projectName, env.Name, req); err != nil {
			rollbackAgentCreate("LLM config failure")
			return err
		}
	}
	if len(req.McpConfig) > 0 {
		if err := s.createAgentMCPConfigs(ctx, ouID, projectName, []string{env.Name}, req); err != nil {
			rollbackAgentCreate("MCP config failure")
			return err
		}
	}

	s.provisionAgentIdentities(ctx, ouID, projectName, req, "")
	s.logger.Info("Agent created", "agentName", req.Name, "ouID", ouID, "projectName", projectName)
	return nil
}

func (s *agentManagerService) SetEntitlementChecker(checker EntitlementChecker) {
	s.entitlementChecker = checker
}

// checkAgentEntitlement asks the installed EntitlementChecker whether the
// organization may create one more agent of the requested kind. Internal and
// external agents are separate component types, so each is limited on its own.
func (s *agentManagerService) checkAgentEntitlement(ctx context.Context, ouID string, req *spec.CreateAgentRequest) error {
	if s.entitlementChecker == nil {
		return nil
	}
	componentType := constants.ComponentTypeInternalAgentAPI
	if req.Provisioning.Type == string(utils.ExternalAgent) {
		componentType = constants.ComponentTypeExternalAgentAPI
	}
	err := s.entitlementChecker.CheckCreate(ctx, ouID, componentType, 1)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, utils.ErrEntitlementDenied):
		s.logger.Info("Agent create refused by entitlement check", "agentName", req.Name, "ouID", ouID, "componentType", componentType, "error", err)
		return err
	default:
		s.logger.Error("Entitlement check failed", "agentName", req.Name, "ouID", ouID, "componentType", componentType, "error", err)
		return fmt.Errorf("entitlement check for %s: %w", componentType, err)
	}
}

// provisionAgentIdentities starts AgentID provisioning for the agent: one Thunder
// identity per org-level environment (not just this project's deployment
// pipeline — see the AgentID architecture doc, which project-level UIs filter
// down to their own pipeline environments when displaying bindings). A Thunder
// hiccup never fails agent creation; the write-ahead + retry reconciler make it
// durable. awaitEnv, when set, is provisioned inline (see
// AgentThunderProvisioningService.ProvisionForAgent). Returns whether
// provisioning was started, false when this deployment injects no provisioning
// implementation or the environments could not be listed.
func (s *agentManagerService) provisionAgentIdentities(ctx context.Context, ouID, projectName string, req *spec.CreateAgentRequest, awaitEnv string) bool {
	if s.agentThunderProvisioning == nil {
		return false
	}
	envs, envErr := s.platform.ListEnvironments(ctx, ouID)
	if envErr != nil {
		s.logger.Warn("Failed to list org environments for agent thunder provisioning", "agentName", req.Name, "error", envErr)
		return false
	}
	if len(envs) == 0 {
		return false
	}
	envNames := make([]string, 0, len(envs))
	for _, e := range envs {
		envNames = append(envNames, e.Name)
	}
	ownership := models.AgentProvisioningType(req.Provisioning.Type)
	// requestedBy is captured now, synchronously, because the retry reconciler
	// may not attempt some of these bindings until minutes or hours later, long
	// after this request's own caller identity would otherwise be gone. It is
	// AMS's own audit record only — never sent to Thunder (see
	// models.AgentThunderClient.RequestedBy for why Thunder's own "owner" field
	// cannot carry this).
	var requestedBy string
	if callerClaims := jwtassertion.GetTokenClaims(ctx); callerClaims != nil {
		requestedBy = callerClaims.Sub
	}
	s.agentThunderProvisioning.ProvisionForAgent(ctx, ouID, projectName, req.Name, ownership, envNames, requestedBy, awaitEnv)
	return true
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
func (s *agentManagerService) createAgentLLMConfigs(
	ctx context.Context, ouID, projectName, firstEnv string, req *spec.CreateAgentRequest,
) error {
	for i, mc := range req.ModelConfig {
		configName := fmt.Sprintf("%s-llm-config", req.Name)
		if len(req.ModelConfig) > 1 {
			configName = fmt.Sprintf("%s-llm-config-%d", req.Name, i+1)
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
		if _, err := s.agentConfigurationService.Create(ctx, ouID, projectName, req.Name, createReq, "system"); err != nil {
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
func (s *agentManagerService) createAgentMCPConfigs(
	ctx context.Context, ouID, projectName string, pipelineEnvs []string, req *spec.CreateAgentRequest,
) error {
	for i, mc := range req.McpConfig {
		configName := fmt.Sprintf("%s-mcp-config", req.Name)
		if len(req.McpConfig) > 1 {
			configName = fmt.Sprintf("%s-mcp-config-%d", req.Name, i+1)
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
		if _, err := s.agentConfigurationService.Create(ctx, ouID, projectName, req.Name, createReq, "system"); err != nil {
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
		Resilience: utils.ConvertSpecToModelResilience(cfg.Resilience),
	}
}

func convertEnvVars(specVars []spec.EnvironmentVariableConfig) []models.EnvironmentVariableConfig {
	result := make([]models.EnvironmentVariableConfig, 0, len(specVars))
	for _, v := range specVars {
		result = append(result, models.EnvironmentVariableConfig{Name: v.Name, Key: v.Key})
	}
	return result
}

func (s *agentManagerService) UpdateAgentBasicInfo(ctx context.Context, ouID string, projectName string, agentName string, req *spec.UpdateAgentBasicInfoRequest) (*models.AgentResponse, error) {
	if err := s.platform.UpdateAgentBasicInfo(ctx, ouID, projectName, agentName, req.DisplayName, req.Description, req.Labels); err != nil {
		return nil, err
	}
	return s.platform.GetAgent(ctx, ouID, projectName, agentName)
}

func (s *agentManagerService) GenerateName(ctx context.Context, ouID string, payload spec.ResourceNameRequest) (string, error) {
	s.logger.Info("Generating resource name", "resourceType", payload.ResourceType, "displayName", payload.DisplayName, "ouID", ouID)
	// Validate organization exists
	org, err := s.platform.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to find organization", "ouID", ouID, "error", err)
		return "", err
	}

	// Generate candidate name from display name
	candidateName := utils.GenerateCandidateName(payload.DisplayName)
	s.logger.Debug("Generated candidate name", "candidateName", candidateName, "displayName", payload.DisplayName)

	if payload.ResourceType == string(utils.ResourceTypeAgent) {
		projectName := utils.StrPointerAsStr(payload.ProjectName, "")
		// Validates the project name by checking its existence
		project, err := s.platform.GetProject(ctx, ouID, projectName)
		if err != nil {
			s.logger.Error("Failed to find project", "projectName", projectName, "org", ouID, "error", err)
			return "", err
		}

		// Check if candidate name is available
		exists, err := s.platform.AgentExists(ctx, ouID, project.Name, candidateName)
		if err != nil {
			return "", fmt.Errorf("failed to check agent existence: %w", err)
		}
		if !exists {
			return candidateName, nil
		}

		// Name is taken, generate unique name with suffix
		uniqueName, err := s.generateUniqueAgentName(ctx, ouID, project.Name, candidateName)
		if err != nil {
			s.logger.Error("Failed to generate unique agent name", "baseName", candidateName, "ouID", org.Name, "projectName", project.Name, "error", err)
			return "", fmt.Errorf("failed to generate unique agent name: %w", err)
		}
		s.logger.Info("Generated unique agent name", "agentName", uniqueName, "ouID", ouID, "projectName", projectName)
		return uniqueName, nil
	}
	if payload.ResourceType == string(utils.ResourceTypeProject) {
		// Check if candidate name is available
		_, err = s.platform.GetProject(ctx, ouID, candidateName)
		if err != nil && errors.Is(err, utils.ErrProjectNotFound) {
			// Name is available, return it
			s.logger.Info("Generated unique project name", "projectName", candidateName, "ouID", ouID)
			return candidateName, nil
		}
		if err != nil {
			s.logger.Error("Failed to check project name availability", "name", candidateName, "ouID", org.Name, "error", err)
			return "", fmt.Errorf("failed to check project name availability: %w", err)
		}
		// Name is taken, generate unique name with suffix
		uniqueName, err := s.generateUniqueProjectName(ctx, ouID, candidateName)
		if err != nil {
			s.logger.Error("Failed to generate unique project name", "baseName", candidateName, "ouID", org.Name, "error", err)
			return "", fmt.Errorf("failed to generate unique project name: %w", err)
		}
		s.logger.Info("Generated unique project name", "projectName", uniqueName, "ouID", ouID)
		return uniqueName, nil
	}
	if payload.ResourceType == string(utils.ResourceTypeEnvironment) {
		return "", fmt.Errorf("%w: environments cannot be created", utils.ErrNotImplemented)
	}
	return "", errors.New("invalid resource type for name generation")
}

// generateUniqueProjectName creates a unique name by appending a random suffix
func (s *agentManagerService) generateUniqueProjectName(ctx context.Context, ouID string, baseName string) (string, error) {
	// Create a name availability checker function that uses the project repository
	nameChecker := func(name string) (bool, error) {
		_, err := s.platform.GetProject(ctx, ouID, name)
		if err != nil && errors.Is(err, utils.ErrProjectNotFound) {
			// Name is available
			return true, nil
		}
		if err != nil {
			s.logger.Error("Failed to check project name availability", "name", name, "ouID", ouID, "error", err)
			return false, fmt.Errorf("failed to check project name availability: %w", err)
		}
		// Name is taken
		return false, nil
	}

	// Use the common unique name generation logic from utils
	uniqueName, err := utils.GenerateUniqueNameWithSuffix(baseName, nameChecker)
	if err != nil {
		s.logger.Error("Failed to generate unique project name", "baseName", baseName, "ouID", ouID, "error", err)
		return "", fmt.Errorf("failed to generate unique project name: %w", err)
	}

	return uniqueName, nil
}

// generateUniqueAgentName creates a unique name by appending a random suffix
func (s *agentManagerService) generateUniqueAgentName(ctx context.Context, ouID string, projectName string, baseName string) (string, error) {
	// Create a name availability checker function that uses the agent repository
	nameChecker := func(name string) (bool, error) {
		exists, err := s.platform.AgentExists(ctx, ouID, projectName, name)
		if err != nil {
			return false, fmt.Errorf("failed to check agent name availability: %w", err)
		}
		if !exists {
			// Name is available
			return true, nil
		}
		// Name is taken
		return false, nil
	}

	// Use the common unique name generation logic from utils
	uniqueName, err := utils.GenerateUniqueNameWithSuffix(baseName, nameChecker)
	if err != nil {
		return "", fmt.Errorf("failed to generate unique agent name: %w", err)
	}

	return uniqueName, nil
}

func (s *agentManagerService) DeleteAgent(ctx context.Context, ouID string, projectName string, agentName string) error {
	s.logger.Info("Deleting agent", "agentName", agentName, "ouID", ouID, "projectName", projectName)
	if _, err := s.platform.GetProject(ctx, ouID, projectName); err != nil {
		return err
	}

	// Resolve the agent type before the delete, while the agent can still be read.
	isExternalAgent := false
	isA2AAgent := false
	if agent, err := s.platform.GetAgent(ctx, ouID, projectName, agentName); err != nil {
		s.logger.Warn("Failed to determine agent type before deletion", "agentName", agentName, "error", err)
	} else {
		isExternalAgent = isExternalProvisioned(agent)
		isA2AAgent = a2aCapable(agent)
	}

	// Deletion is irreversible and cascades into configs, identities and
	// monitors, so it is refused when it cannot be recorded.
	deleteAttempt, auditErr := audit.Begin(
		ctx, audit.ActionAgentDelete,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceAgent, agentName, agentName),
		audit.Project(projectName),
		audit.Detail("agentName", agentName),
	)
	if auditErr != nil {
		s.logger.Error("Refusing to delete agent: audit record could not be written",
			"agentName", agentName, "error", auditErr)
		return auditErr
	}

	// Teardown runs before the agent row goes: every agent-owned row cascades
	// from it, and the cleanups find what to revoke (proxy credentials,
	// identities, secrets) through those rows.
	cleanupCtx, cancel := detachedCleanupContext(ctx)
	defer cancel()
	s.deleteAgentLLMConfigurations(cleanupCtx, ouID, projectName, agentName, isExternalAgent)
	if s.agentThunderProvisioning != nil {
		s.agentThunderProvisioning.DeleteAllBindings(cleanupCtx, ouID, projectName, agentName)
	}
	if configErr := s.agentConfigRepo.DeleteAllByAgent(ctx, ouID, projectName, agentName); configErr != nil {
		s.logger.Warn("Failed to delete agent configs from database", "agentName", agentName, "error", configErr)
	}
	artifactErr := s.deleteAgentAPIArtifact(ctx, ouID, projectName, agentName, isA2AAgent)
	s.cleanupAgentMonitors(ctx, ouID, projectName, agentName)

	err := s.platform.DeleteAgent(ctx, ouID, projectName, agentName)
	deleteAttempt.Complete(ctx, err)
	if err != nil && !errors.Is(err, utils.ErrAgentNotFound) {
		s.logger.Error("Failed to delete agent", "agentName", agentName, "error", err)
		return err
	}
	return artifactErr
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

// agentCleanupTimeout bounds the detached cleanup goroutine. The retry budget above lets a
// single configuration occupy ~14s of backoff on top of its external calls, so a many-config
// agent could otherwise keep the goroutine alive indefinitely if a gateway stops answering.
// Generous on purpose: the ceiling exists to stop a wedged call from pinning the goroutine for
// the process lifetime, not to cut short revocations that are still making progress.
const agentCleanupTimeout = 10 * time.Minute

// detachedCleanupContext returns a context for post-delete cleanup that outlives the request.
// WithoutCancel drops the request deadline while keeping its values (trace IDs, logger) so the
// work is not cancelled when the HTTP handler returns; the timeout then puts a ceiling back on.
func detachedCleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), agentCleanupTimeout)
}

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
func (s *agentManagerService) clearInheritedA2AState(ctx context.Context, ouID, projectName, agentName string) error {
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

// cleanupAgentMonitors has nothing to remove: Lite runs no monitors.
func (s *agentManagerService) cleanupAgentMonitors(context.Context, string, string, string) {}

// deleteAgentAPIArtifact removes the agent's per-environment API artifacts and
// tells gateways holding an A2A Agent for them to drop it. It returns what it
// could not remove; the component is already gone by then, so deleting the
// agent again retries the rest. isA2AAgent is false for a REST or chat agent, and when the agent could not be
// read; deployment rows then stand in for it.
func (s *agentManagerService) deleteAgentAPIArtifact(ctx context.Context, ouID, projectName, agentName string, isA2AAgent bool) error {
	err := s.cleanUpAgentAPIArtifacts(ctx, ouID, projectName, agentName, isA2AAgent)
	if err != nil {
		return fmt.Errorf("the agent was deleted but its A2A state was not fully removed; delete the agent again to retry: %w", err)
	}
	return nil
}

// cleanUpAgentAPIArtifacts is deleteAgentAPIArtifact's work. Each environment's artifact goes in one transaction
// with its exposure, proxy and card rows and its delete events, so a failure leaves that environment whole for
// the retry. The agent's rows in environments without an artifact go once every artifact is gone.
func (s *agentManagerService) cleanUpAgentAPIArtifacts(ctx context.Context, ouID, projectName, agentName string, isA2AAgent bool) error {
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
func (s *agentManagerService) dropAgentArtifact(
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
func (s *agentManagerService) checkNoOrphanedConfigs(ctx context.Context, ouID, projectName, agentName string) error {
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
func (s *agentManagerService) deleteAgentLLMConfigurations(ctx context.Context, ouID, projectName, agentName string, isExternalAgent bool) {
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
func resolveAPIConfig(existingConfig *models.AgentConfig, enableApiKeySecurity *bool, corsConfig *spec.CORSConfig, enableOAuthSecurity *bool, oauthConfig *spec.OAuthConfig, withDefaults bool) resolvedCORSConfig {
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

// oauthConfigFromAgentConfig builds the response OAuth config from a persisted
// agent config row. Issuers are returned as persisted (no silent default); empty
// header fields fall back to the gateway-compatible defaults.
func oauthConfigFromAgentConfig(cfg *models.AgentConfig) *models.OAuthConfig {
	issuers := cfg.OAuthIssuers
	headerName := cfg.OAuthHeaderName
	if headerName == "" {
		headerName = models.DefaultOAuthHeaderName
	}
	authHeaderPrefix := cfg.OAuthAuthHeaderPrefix
	if authHeaderPrefix == "" {
		authHeaderPrefix = models.DefaultOAuthAuthHeaderPrefix
	}
	return &models.OAuthConfig{
		Issuers:          issuers,
		Audiences:        cfg.OAuthAudiences,
		HeaderName:       headerName,
		AuthHeaderPrefix: authHeaderPrefix,
		ForwardToken:     cfg.OAuthForwardToken,
	}
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
func (s *agentManagerService) validateOAuthIssuersInEnvironment(envUUID string, cfg resolvedCORSConfig) error {
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

// requireEnvTier authorizes the caller to act on the named environment and
// returns it.
//
// The environment tier is an authorization axis of its own, about *where* an
// action lands rather than what it is: agent:env-non-production is the floor,
// and agent:env-production is held in addition to it to reach the environments
// flagged as production. A production environment requires both — the production
// grant is an extra permission stacked on the floor, not a wider substitute for
// it. That is not a free choice: the REST routes and the two MCP tools declare
// the floor statically and deny before this method is reached, so a rule of
// "production grant alone is sufficient" could never fire and would leave the
// two layers describing the same decision differently.
//
// The check lives here rather than in route middleware because the MCP surface
// declares tool permissions statically (mcp/tools/authz.go addTool) and has no
// HTTP request to read a target environment from. This is the one place both
// surfaces pass through, and mcp/tools/authz.go's withEffectiveScopes is what
// guarantees ctx carries the same scopes the tool gate used.
//
// The environment is returned even on a denial, because the caller wants the
// production flag for its audit record and this call has already paid for the
// lookup; it is nil only when the lookup itself failed. The cost is that an
// unresolvable environment fails the operation outright — deliberate, and
// called out in Task 5.
//
// Nothing here allows on failure. There is deliberately no fallback for an
// installation where no environment carries the flag: every environment is then
// non-production, which is what the model says and what the release notes must
// say too.
func (s *agentManagerService) requireEnvTier(
	ctx context.Context, ouID, envName string,
) (*models.EnvironmentResponse, error) {
	return requireEnvironmentTier(ctx, s.platform, s.logger, ouID, envName)
}

// GetAgentIdentity returns the agent's AgentID binding for every environment in
// this project's deployment pipeline. AgentIDs are provisioned across every
// org-level environment (Section 2.1 of the AgentID architecture doc), but a
// project only ever shows the bindings for environments in its own pipeline —
// this filters the org-wide result down to that visibility rule. A safe GET:
// it never returns or destroys a secret. Use RegenerateAgentIdentitySecret to
// obtain a secret for an External agent.
func (s *agentManagerService) GetAgentIdentity(ctx context.Context, ouID string, projectName string, agentName string) ([]models.AgentIdentityEnvironmentView, error) {
	// No provisioning implementation: no identities to report.
	if s.agentThunderProvisioning == nil {
		return []models.AgentIdentityEnvironmentView{}, nil
	}
	views, err := s.agentThunderProvisioning.GetIdentityViews(ctx, ouID, projectName, agentName)
	if err != nil {
		return nil, err
	}

	visible, err := s.visiblePipelineEnvironments(ctx, ouID, projectName)
	if err != nil {
		return nil, err
	}

	filtered := make([]models.AgentIdentityEnvironmentView, 0, len(views))
	for _, v := range views {
		if visible[v.EnvironmentName] {
			filtered = append(filtered, v)
		}
	}
	return filtered, nil
}

// visiblePipelineEnvironments returns the set of environment names visible to
// projectName's deployment pipeline — AgentIDs are provisioned across every
// org-level environment (Section 2.1 of the AgentID architecture doc), but a
// project only ever sees the ones in its own pipeline. Shared by
// GetAgentIdentity (filtering a list) and requireVisibleEnvironment
// (checking a single name).
func (s *agentManagerService) visiblePipelineEnvironments(context.Context, string, string) (map[string]bool, error) {
	return map[string]bool{models.DefaultEnvironmentName: true}, nil
}

// requireVisibleEnvironment validates that environmentName is part of
// projectName's deployment pipeline — the same visibility rule GetAgentIdentity
// applies when filtering its org-wide binding list down to project scope.
func (s *agentManagerService) requireVisibleEnvironment(ctx context.Context, ouID, projectName, environmentName string) error {
	visible, err := s.visiblePipelineEnvironments(ctx, ouID, projectName)
	if err != nil {
		return err
	}
	if !visible[environmentName] {
		return fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, environmentName)
	}
	return nil
}

// GetAgentRoles returns the Thunder roles assigned to the agent's AgentID in
// one environment, if that environment is part of this project's deployment
// pipeline.
func (s *agentManagerService) GetAgentRoles(ctx context.Context, ouID, projectName, agentName, environmentName string) ([]thundersvc.ThunderRole, error) {
	if s.agentThunderProvisioning == nil {
		return nil, fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, environmentName)
	}
	if err := s.requireVisibleEnvironment(ctx, ouID, projectName, environmentName); err != nil {
		return nil, err
	}
	return s.agentThunderProvisioning.GetAgentRoles(ctx, ouID, projectName, agentName, environmentName)
}

// GetAgentGroups returns the Thunder groups the agent's AgentID belongs to in
// one environment, if that environment is part of this project's deployment
// pipeline.
func (s *agentManagerService) GetAgentGroups(ctx context.Context, ouID, projectName, agentName, environmentName string) ([]thundersvc.ThunderGroup, error) {
	if s.agentThunderProvisioning == nil {
		return nil, fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, environmentName)
	}
	if err := s.requireVisibleEnvironment(ctx, ouID, projectName, environmentName); err != nil {
		return nil, err
	}
	return s.agentThunderProvisioning.GetAgentGroups(ctx, ouID, projectName, agentName, environmentName)
}

// RegenerateAgentIdentitySecret rotates the AgentID secret for one environment.
// The new secret is returned in the response for BOTH Internal and External
// agents — the caller already holds agent:update and just explicitly asked
// for a new credential, so withholding the value it produced would only
// force a second call to fetch it separately. ProvisioningType is included
// so the caller can tell which kind of agent this is without a separate lookup.
func (s *agentManagerService) RegenerateAgentIdentitySecret(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (*models.AgentRegenerateSecretResponse, error) {
	if s.agentThunderProvisioning == nil {
		return nil, utils.ErrAgentIdentityNotProvisioned
	}
	ownership, clientID, newSecret, err := s.agentThunderProvisioning.RegenerateSecret(ctx, ouID, projectName, agentName, environmentName)
	if err != nil {
		return nil, err
	}

	return &models.AgentRegenerateSecretResponse{
		EnvironmentName:  environmentName,
		ProvisioningType: ownership,
		ClientID:         clientID,
		ClientSecret:     newSecret,
		Status:           models.AgentRegenerateSecretStatus,
	}, nil
}

// RevokeAgentIdentitySecret invalidates the AgentID secret for one environment.
// It never returns a usable secret — an explicit regenerate is required
// afterward to restore access.
func (s *agentManagerService) RevokeAgentIdentitySecret(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (models.AgentRevokeSecretResponse, error) {
	if s.agentThunderProvisioning == nil {
		return models.AgentRevokeSecretResponse{}, utils.ErrAgentIdentityNotProvisioned
	}
	clientID, err := s.agentThunderProvisioning.RevokeSecret(ctx, ouID, projectName, agentName, environmentName)
	if err != nil {
		return models.AgentRevokeSecretResponse{}, err
	}

	return models.AgentRevokeSecretResponse{
		EnvironmentName: environmentName,
		ClientID:        clientID,
		Status:          models.AgentRevokeSecretStatus,
	}, nil
}

// ProvisionAgentIdentity provisions an AgentID for one environment that doesn't
// have one yet — for example, an agent that existed before this environment
// did (or before it entered the project's pipeline). Internal agents normally
// get their AgentIDs automatically during deployment/promotion, but this
// idempotent repair path also covers older agents whose binding is missing.
//
// alreadyExisted is true when a binding for this environment was already
// present (any status) — the caller uses this to choose between 200 (nothing
// new happened, here's the current state) and 202 (provisioning was just
// kicked off in the background).
func (s *agentManagerService) ProvisionAgentIdentity(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (models.AgentIdentityEnvironmentView, bool, error) {
	if s.agentThunderProvisioning == nil {
		return models.AgentIdentityEnvironmentView{}, false, utils.ErrAgentIdentityNotProvisioned
	}
	agent, err := s.platform.GetAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent for identity provisioning", "agentName", agentName, "error", err)
		return models.AgentIdentityEnvironmentView{}, false, err
	}
	var provisioningType models.AgentProvisioningType
	switch agent.Provisioning.Type {
	case string(utils.InternalAgent):
		provisioningType = models.AgentProvisioningTypeInternal
	case string(utils.ExternalAgent):
		provisioningType = models.AgentProvisioningTypeExternal
	default:
		return models.AgentIdentityEnvironmentView{}, false, fmt.Errorf(
			"%w: agent %q has unsupported provisioning type %q",
			utils.ErrInvalidInput, agentName, agent.Provisioning.Type,
		)
	}

	if _, err := s.platform.GetEnvironment(ctx, ouID, environmentName); err != nil {
		s.logger.Error("Failed to fetch environment for identity provisioning", "environmentName", environmentName, "error", err)
		return models.AgentIdentityEnvironmentView{}, false, err
	}

	var requestedBy string
	if callerClaims := jwtassertion.GetTokenClaims(ctx); callerClaims != nil {
		requestedBy = callerClaims.Sub
	}

	alreadyExisted, err := s.agentThunderProvisioning.ProvisionForEnvironmentIfMissing(
		ctx, ouID, projectName, agentName, environmentName, provisioningType, requestedBy,
	)
	if err != nil {
		return models.AgentIdentityEnvironmentView{}, false, err
	}

	views, err := s.agentThunderProvisioning.GetIdentityViews(ctx, ouID, projectName, agentName)
	if err != nil {
		return models.AgentIdentityEnvironmentView{}, alreadyExisted, err
	}
	for _, v := range views {
		if v.EnvironmentName == environmentName {
			return v, alreadyExisted, nil
		}
	}
	// Upsert inside ProvisionForEnvironmentIfMissing is synchronous, so the row
	// (at minimum PENDING) must already be visible to this read.
	return models.AgentIdentityEnvironmentView{}, alreadyExisted, fmt.Errorf("agent thunder binding for %s/%s vanished immediately after being provisioned", agentName, environmentName)
}

// RetryAgentIdentityProvisioning resets a failed AgentID binding and
// re-attempts provisioning, for both Internal and External agents.
func (s *agentManagerService) RetryAgentIdentityProvisioning(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (models.AgentIdentityEnvironmentView, error) {
	if s.agentThunderProvisioning == nil {
		return models.AgentIdentityEnvironmentView{}, utils.ErrAgentIdentityNotProvisioned
	}
	return s.agentThunderProvisioning.RetryProvisioning(ctx, ouID, projectName, agentName, environmentName)
}

// UpdateAgentDeploySettings updates per-environment deploy settings (CORS, API key security,
// auto instrumentation) on an existing release binding without redeploying or promoting the
// agent. Triggers a pod rollout so policy changes take effect immediately. Any field omitted
// from the request keeps its current DB value.
func (s *agentManagerService) UpdateAgentDeploySettings(ctx context.Context, ouID, projectName, agentName string, req *spec.UpdateAgentDeploySettingsRequest) error {
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
func (s *agentManagerService) GetAgentDeployments(ctx context.Context, ouID string, projectName string, agentName string) ([]*models.DeploymentResponse, error) {
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
func (s *agentManagerService) resolveGatewayDeployments(
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
func (s *agentManagerService) resolveAgentURL(
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

// GetAgentEnvConfig returns the full per-environment agent_configs row for (agent, environment)
// — tracing/instrumentation plus CORS and endpoint-authentication settings — or nil when none is
// persisted yet. Unlike GetAgent (which reads only the lowest environment's config), this is scoped
// to the requested environment so the console seeds the correct per-env values.
func (s *agentManagerService) GetAgentEnvConfig(ctx context.Context, ouID, projectName, agentName, environment string) (*models.AgentConfig, error) {
	cfg, err := s.agentConfigRepo.Get(ctx, ouID, projectName, agentName, environment)
	if errors.Is(err, repositories.ErrAgentConfigNotFound) {
		return nil, nil //nolint:nilnil // "no config yet" is a valid, expected state distinct from an error
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get agent env config: %w", err)
	}
	return cfg, nil
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
func (s *agentManagerService) exposesAgent(agent *models.AgentResponse) bool {
	_, ok := agentGatewayShapeOf(agent)
	return ok && s.exposure != nil && s.a2aPublisher != nil
}

// agentEnvArtifactFunc ensures, in the exposure transaction, the agent's artifact in the environment with the kind
// its gateway shape names, switching a legacy REST agent's Agent row in place; nil for an agent the gateway does
// not serve.
func (s *agentManagerService) agentEnvArtifactFunc(
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
func (s *agentManagerService) saveDeploySettings(
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
func (s *agentManagerService) agentExposureKeyFor(
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

// requireEnvironmentTier is requireEnvTier without the service receiver, shared by other services.
func requireEnvironmentTier(
	ctx context.Context, platform repositories.PlatformRepository, logger *slog.Logger, ouID, envName string,
) (*models.EnvironmentResponse, error) {
	env, err := platform.GetEnvironment(ctx, ouID, envName)
	if err != nil {
		logger.Error("Failed to resolve environment for the tier check",
			"ouID", ouID, "environment", envName, "error", err)
		return nil, err
	}
	// The floor is always required. Production adds to it rather than replacing
	// it, so the missing scope reported is the first one the caller lacks —
	// naming the production grant to someone who is also missing the floor would
	// send them after the wrong permission.
	required := []rbac.Permission{rbac.AgentEnvNonProduction}
	if env.IsProduction {
		required = append(required, rbac.AgentEnvProduction)
	}
	if perm, short := jwtassertion.FirstMissingScope(ctx, required...); short {
		// A middleware denial reaches the trail through recordAuthzDeny, which
		// uses audit.Record plus audit.Skip: the deny event *replaces* the
		// envelope, because the request never reached a handler. This denial is
		// different — the caller did reach the service layer, and on the
		// change-deployment-state path the controller has already opened an
		// attempt. RecordAncillary is therefore correct here (audit/emit.go:63-71:
		// facts about how a request was handled, envelope preserved), and the
		// consequence is deliberate: a tier refusal writes an authz:deny
		// alongside the envelope rather than in place of it, so "what was
		// attempted" survives. Status is left to the envelope for the same
		// reason. grantedScopes is set because every middleware authz:deny
		// carries it and anything alerting on the action reads it.
		audit.RecordAncillary(
			ctx, audit.ActionAuthzDeny,
			audit.Org(ouID),
			audit.Environment(envName),
			audit.OutcomeOpt(audit.OutcomeDeny),
			audit.RequiredPermissions(required...),
			audit.Detail("reason", "missing-environment-tier-scope"),
			audit.Detail("missingScope", perm.Scope()),
			audit.Detail("grantedScopes", jwtassertion.GrantedScopeCount(ctx)),
		)
		return env, fmt.Errorf("%w: %s is required to act on environment %q",
			utils.ErrForbidden, perm.Scope(), envName)
	}
	return env, nil
}

// isLegacySecretRefPath reports whether a stored secret reference is a KV path
// (rows written before reference names were stored) rather than a name.
func isLegacySecretRefPath(secretRefPath string) bool {
	return strings.Contains(secretRefPath, "/")
}
