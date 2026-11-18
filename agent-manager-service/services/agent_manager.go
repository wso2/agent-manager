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

	"golang.org/x/sync/errgroup"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/coreext"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"gorm.io/gorm"
)

type AgentManagerService interface {
	ListAgents(ctx context.Context, ouID string, projName string, labelFilter map[string]string, limit int32, offset int32) ([]*models.AgentResponse, int32, error)
	ListOrgAgents(ctx context.Context, ouID string) ([]*models.AgentSummary, error)
	CreateAgent(ctx context.Context, ouID string, projectName string, req *spec.CreateAgentRequest) error
	UpdateAgentBasicInfo(ctx context.Context, ouID string, projectName string, agentName string, req *spec.UpdateAgentBasicInfoRequest) (*models.AgentResponse, error)
	DeleteAgent(ctx context.Context, ouID string, projectName string, agentName string) error
	GetAgent(ctx context.Context, ouID string, projectName string, agentName string) (*models.AgentResponse, error)
	GenerateName(ctx context.Context, ouID string, payload spec.ResourceNameRequest) (string, error)
	GetAgentIdentity(ctx context.Context, ouID string, projectName string, agentName string) ([]models.AgentIdentityEnvironmentView, error)
	RegenerateAgentIdentitySecret(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (*models.AgentRegenerateSecretResponse, error)
	RevokeAgentIdentitySecret(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (models.AgentRevokeSecretResponse, error)
	ProvisionAgentIdentity(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (view models.AgentIdentityEnvironmentView, alreadyExisted bool, err error)
	RetryAgentIdentityProvisioning(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (models.AgentIdentityEnvironmentView, error)
	GetAgentRoles(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) ([]thundersvc.ThunderRole, error)
	GetAgentGroups(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) ([]thundersvc.ThunderGroup, error)
}

type agentManagerService struct {
	db                       *gorm.DB
	platform                 repositories.PlatformRepository
	tokenManagerService      AgentTokenManagerService
	agentThunderProvisioning AgentThunderProvisioningService
	identityClient           thundersvc.IdentityClient
	logger                   *slog.Logger
}

// NewAgentManagerService creates the core's agent service. Modules add to an
// agent's lifecycle through their hooks and event handlers, with the
// extensions block of the request.
func NewAgentManagerService(
	db *gorm.DB,
	platform repositories.PlatformRepository,
	tokenManagerService AgentTokenManagerService,
	agentThunderProvisioning AgentThunderProvisioningService,
	identityClient thundersvc.IdentityClient,
	logger *slog.Logger,
) AgentManagerService {
	return &agentManagerService{
		db:                       db,
		platform:                 platform,
		tokenManagerService:      tokenManagerService,
		agentThunderProvisioning: agentThunderProvisioning,
		identityClient:           identityClient,
		logger:                   logger,
	}
}

// Build type constants
const (
	BuildTypeBuildpack = "buildpack"
	BuildTypeDocker    = "docker"
)

// -----------------------------------------------------------------------------
// Mapping Helper Functions
// -----------------------------------------------------------------------------

func (s *agentManagerService) GetAgent(ctx context.Context, ouID string, projectName string, agentName string) (*models.AgentResponse, error) {
	agent, err := s.platform.GetAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		return nil, err
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

	in := repositories.CreateAgentInput{
		Name:             req.Name,
		DisplayName:      req.DisplayName,
		Description:      utils.StrPointerAsStr(req.Description, ""),
		ProvisioningType: req.Provisioning.Type,
	}
	if req.Labels != nil {
		in.Labels = *req.Labels
	}
	ext, err := coreext.Blocks(req.Extensions)
	if err != nil {
		return err
	}
	in.Extensions = ext
	if callerClaims := jwtassertion.GetTokenClaims(ctx); callerClaims != nil {
		in.CreatedBy = callerClaims.Sub
	}
	if _, err := s.platform.CreateAgent(ctx, ouID, projectName, in); err != nil {
		return err
	}

	s.provisionAgentIdentities(ctx, ouID, projectName, req, "")
	s.logger.Info("Agent created", "agentName", req.Name, "ouID", ouID, "projectName", projectName)
	return nil
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

func (s *agentManagerService) UpdateAgentBasicInfo(ctx context.Context, ouID string, projectName string, agentName string, req *spec.UpdateAgentBasicInfoRequest) (*models.AgentResponse, error) {
	ext, err := coreext.Blocks(req.Extensions)
	if err != nil {
		return nil, err
	}
	if err := s.platform.UpdateAgentBasicInfo(ctx, ouID, projectName, agentName, req.DisplayName, req.Description, req.Labels, ext); err != nil {
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

	// Modules may refuse the delete; ask before anything is torn down, so a
	// refusal leaves the agent and everything it owns in place. The check and
	// the delete share one operation, so they see the same modules.
	ctx = coreext.Start(ctx, modkit.ResourceAgent)
	if err := s.platform.CheckDeleteAgent(ctx, ouID, projectName, agentName); err != nil && !errors.Is(err, utils.ErrAgentNotFound) {
		return err
	}

	// Deletion is irreversible and cascades into configs and identities, so
	// it is refused when it cannot be recorded.
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

	// Identities are revoked before the agent row goes, while their rows still
	// name what to revoke. Modules revoke theirs in agent.deleted handlers.
	cleanupCtx, cancel := detachedCleanupContext(ctx)
	defer cancel()
	if s.agentThunderProvisioning != nil {
		s.agentThunderProvisioning.DeleteAllBindings(cleanupCtx, ouID, projectName, agentName)
	}

	err := s.platform.DeleteAgent(ctx, ouID, projectName, agentName)
	deleteAttempt.Complete(ctx, err)
	if err != nil && !errors.Is(err, utils.ErrAgentNotFound) {
		s.logger.Error("Failed to delete agent", "agentName", agentName, "error", err)
		return err
	}
	return nil
}

// agentCleanupTimeout bounds the detached cleanup of the agent's identities.
// Generous on purpose: the ceiling exists to stop a wedged call from pinning the goroutine for
// the process lifetime, not to cut short revocations that are still making progress.
const agentCleanupTimeout = 10 * time.Minute

// detachedCleanupContext returns a context for post-delete cleanup that outlives the request.
// WithoutCancel drops the request deadline while keeping its values (trace IDs, logger) so the
// work is not cancelled when the HTTP handler returns; the timeout then puts a ceiling back on.
func detachedCleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), agentCleanupTimeout)
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

// IsLegacySecretRefPath reports whether a stored secret reference is a KV path
// (rows written before reference names were stored) rather than a name.
func IsLegacySecretRefPath(secretRefPath string) bool {
	return strings.Contains(secretRefPath, "/")
}

// GetProjectAgent reads the agent, reporting a miss as utils.ErrAgentNotFound.
// The lookup is by name across the organization, so an agent in another project is a miss too.
func GetProjectAgent(ctx context.Context, oc repositories.PlatformRepository, ouID, projectName, agentName string) (*models.AgentResponse, error) {
	agent, err := oc.GetAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		return nil, err
	}
	if agent.ProjectName != projectName {
		return nil, utils.ErrAgentNotFound
	}
	return agent, nil
}
