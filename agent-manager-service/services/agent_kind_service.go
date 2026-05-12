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
	"time"

	"gorm.io/gorm"

	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type AgentKindService interface {
	// Kind CRUD
	GetKind(ctx context.Context, orgName, kindName string) (*models.AgentKindResponse, error)
	ListKinds(ctx context.Context, orgName string, limit, offset int) (*models.AgentKindListResponse, error)
	UpdateKind(ctx context.Context, orgName, kindName string, req *spec.UpdateAgentKindRequest) (*models.AgentKindResponse, error)
	DeleteKind(ctx context.Context, orgName, kindName string) error

	// Version management
	AddVersion(ctx context.Context, orgName, kindName string, req *spec.AddAgentKindVersionRequest) (*models.AgentKindVersionResponse, error)
	GetVersion(ctx context.Context, orgName, kindName, versionTag string) (*models.AgentKindVersionResponse, error)
	ListVersions(ctx context.Context, orgName, kindName string) ([]models.AgentKindVersionResponse, error)
	DeleteVersion(ctx context.Context, orgName, kindName, versionTag string) error

	// Publish shortcut: creates kind if needed + adds version
	PublishKind(ctx context.Context, orgName, projectName, agentName string, req *spec.PublishAgentKindRequest) (*models.AgentKindVersionResponse, error)

	// For use during agent creation from kind
	GetKindVersion(ctx context.Context, orgName, kindName, versionTag string) (*models.AgentKindVersion, error)

	// ListKindAgents returns all agents deployed from a given kind across all projects in the org.
	ListKindAgents(ctx context.Context, orgName, kindName string) ([]*models.AgentResponse, error)
}

type agentKindService struct {
	kindRepo repositories.AgentKindRepository
	ocClient occlient.OpenChoreoClient
}

func NewAgentKindService(kindRepo repositories.AgentKindRepository, ocClient occlient.OpenChoreoClient) AgentKindService {
	return &agentKindService{
		kindRepo: kindRepo,
		ocClient: ocClient,
	}
}

// GetKind returns an Agent Kind with all its versions.
func (s *agentKindService) GetKind(ctx context.Context, orgName, kindName string) (*models.AgentKindResponse, error) {
	kind, err := s.kindRepo.GetKind(ctx, orgName, kindName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentKindNotFound
		}
		return nil, fmt.Errorf("failed to get agent kind: %w", err)
	}
	return toAgentKindResponse(kind), nil
}

// ListKinds returns a paginated list of Agent Kinds in the org.
func (s *agentKindService) ListKinds(ctx context.Context, orgName string, limit, offset int) (*models.AgentKindListResponse, error) {
	kinds, total, err := s.kindRepo.ListKinds(ctx, orgName, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent kinds: %w", err)
	}

	responses := make([]models.AgentKindResponse, len(kinds))
	for i := range kinds {
		r := toAgentKindResponse(&kinds[i])
		responses[i] = *r
	}

	return &models.AgentKindListResponse{
		Kinds:  responses,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

// UpdateKind updates the display name and description of an Agent Kind.
func (s *agentKindService) UpdateKind(ctx context.Context, orgName, kindName string, req *spec.UpdateAgentKindRequest) (*models.AgentKindResponse, error) {
	kind, err := s.kindRepo.GetKind(ctx, orgName, kindName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentKindNotFound
		}
		return nil, fmt.Errorf("failed to get agent kind: %w", err)
	}

	kind.DisplayName = req.GetDisplayName()
	kind.Description = req.GetDescription()
	kind.UpdatedAt = time.Now()

	if err := s.kindRepo.UpdateKind(ctx, kind); err != nil {
		return nil, fmt.Errorf("failed to update agent kind: %w", err)
	}

	return toAgentKindResponse(kind), nil
}

// DeleteKind deletes an Agent Kind and cascades to all versions.
// It returns ErrAgentKindHasInstances if any agents are still instantiated from this kind.
func (s *agentKindService) DeleteKind(ctx context.Context, orgName, kindName string) error {
	instances, err := s.ListKindAgents(ctx, orgName, kindName)
	if err != nil {
		return fmt.Errorf("failed to check kind instances before deletion: %w", err)
	}
	if len(instances) > 0 {
		return utils.ErrAgentKindHasInstances
	}

	err = s.kindRepo.DeleteKind(ctx, orgName, kindName)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return utils.ErrAgentKindNotFound
	}
	return err
}

// AddVersion publishes a new version to an existing Agent Kind.
func (s *agentKindService) AddVersion(ctx context.Context, orgName, kindName string, req *spec.AddAgentKindVersionRequest) (*models.AgentKindVersionResponse, error) {
	kind, err := s.kindRepo.GetKind(ctx, orgName, kindName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentKindNotFound
		}
		return nil, fmt.Errorf("failed to get agent kind: %w", err)
	}

	return s.publishVersion(ctx, orgName, kind, req.GetSourceProjectName(), req.GetSourceAgentName(), req.GetBuildName(), req.GetVersion(), req.GetConfigSchema())
}

// GetVersion returns a specific version of an Agent Kind.
func (s *agentKindService) GetVersion(ctx context.Context, orgName, kindName, versionTag string) (*models.AgentKindVersionResponse, error) {
	kind, err := s.kindRepo.GetKind(ctx, orgName, kindName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentKindNotFound
		}
		return nil, fmt.Errorf("failed to get agent kind: %w", err)
	}

	v, err := s.kindRepo.GetVersion(ctx, kind.ID, versionTag)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrKindVersionNotFound
		}
		return nil, fmt.Errorf("failed to get agent kind version: %w", err)
	}

	resp := toAgentKindVersionResponse(v)
	return &resp, nil
}

// ListVersions returns all versions of an Agent Kind.
func (s *agentKindService) ListVersions(ctx context.Context, orgName, kindName string) ([]models.AgentKindVersionResponse, error) {
	kind, err := s.kindRepo.GetKind(ctx, orgName, kindName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentKindNotFound
		}
		return nil, fmt.Errorf("failed to get agent kind: %w", err)
	}

	versions, err := s.kindRepo.ListVersions(ctx, kind.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent kind versions: %w", err)
	}

	responses := make([]models.AgentKindVersionResponse, len(versions))
	for i := range versions {
		responses[i] = toAgentKindVersionResponse(&versions[i])
	}
	return responses, nil
}

// DeleteVersion removes a specific version from an Agent Kind.
func (s *agentKindService) DeleteVersion(ctx context.Context, orgName, kindName, versionTag string) error {
	kind, err := s.kindRepo.GetKind(ctx, orgName, kindName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrAgentKindNotFound
		}
		return fmt.Errorf("failed to get agent kind: %w", err)
	}

	err = s.kindRepo.DeleteVersion(ctx, kind.ID, versionTag)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return utils.ErrKindVersionNotFound
	}
	return err
}

// PublishKind is a convenience method: creates the kind if it doesn't exist, then adds a version.
func (s *agentKindService) PublishKind(ctx context.Context, orgName, projectName, agentName string, req *spec.PublishAgentKindRequest) (*models.AgentKindVersionResponse, error) {
	kindName := req.GetKindName()

	kind, err := s.kindRepo.GetKind(ctx, orgName, kindName)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("failed to look up agent kind: %w", err)
		}
		// Kind doesn't exist — create it
		displayName := req.GetKindDisplayName()
		if displayName == "" {
			displayName = kindName
		}
		newKind := &models.AgentKind{
			Name:        kindName,
			DisplayName: displayName,
			Description: req.GetKindDescription(),
			OrgName:     orgName,
			ProjectName: projectName,
			AgentName:   agentName,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Versions:    []models.AgentKindVersion{},
		}
		if createErr := s.kindRepo.CreateKind(ctx, newKind); createErr != nil {
			// Handle concurrent creation race: if another request won the race and created
			// the kind first, retry the lookup rather than surfacing a generic DB error.
			existing, retryErr := s.kindRepo.GetKind(ctx, orgName, kindName)
			if retryErr == nil {
				kind = existing
			} else {
				return nil, fmt.Errorf("failed to create agent kind: %w", createErr)
			}
		} else {
			kind = newKind
		}
	}

	configSchema := req.GetConfigSchema()
	return s.publishVersion(ctx, orgName, kind, projectName, agentName, req.GetBuildName(), req.GetVersion(), configSchema)
}

// GetKindVersion returns the raw DB record for a kind version (used during agent creation from kind).
func (s *agentKindService) GetKindVersion(ctx context.Context, orgName, kindName, versionTag string) (*models.AgentKindVersion, error) {
	kind, err := s.kindRepo.GetKind(ctx, orgName, kindName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentKindNotFound
		}
		return nil, fmt.Errorf("failed to get agent kind: %w", err)
	}

	v, err := s.kindRepo.GetVersion(ctx, kind.ID, versionTag)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrKindVersionNotFound
		}
		return nil, fmt.Errorf("failed to get agent kind version: %w", err)
	}
	return v, nil
}

// ListKindAgents returns all agents across all projects in the org that were deployed from the given kind.
func (s *agentKindService) ListKindAgents(ctx context.Context, orgName, kindName string) ([]*models.AgentResponse, error) {
	// Verify kind exists first
	if _, err := s.kindRepo.GetKind(ctx, orgName, kindName); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentKindNotFound
		}
		return nil, fmt.Errorf("failed to get agent kind: %w", err)
	}

	projects, err := s.ocClient.ListProjects(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	type result struct {
		agents []*models.AgentResponse
		err    error
	}

	results := make([]result, len(projects))
	var wg sync.WaitGroup
	for i, p := range projects {
		wg.Add(1)
		go func(idx int, projectName string) {
			defer wg.Done()
			agents, err := s.ocClient.ListComponentsByKind(ctx, orgName, projectName, kindName)
			results[idx] = result{agents: agents, err: err}
		}(i, p.Name)
	}
	wg.Wait()

	var all []*models.AgentResponse
	for _, r := range results {
		if r.err != nil {
			return nil, r.err
		}
		all = append(all, r.agents...)
	}
	return all, nil
}

// -----------------------------------------------------------------------------
// Internal helpers
// -----------------------------------------------------------------------------

// publishVersion fetches build+component details from OpenChoreo and persists the version.
func (s *agentKindService) publishVersion(
	ctx context.Context,
	orgName string,
	kind *models.AgentKind,
	sourceProjectName, sourceAgentName, buildName, versionTag string,
	configSchema []spec.AgentKindConfigSchemaItem,
) (*models.AgentKindVersionResponse, error) {
	// Check version doesn't already exist
	existing, err := s.kindRepo.GetVersion(ctx, kind.ID, versionTag)
	if existing != nil {
		return nil, utils.ErrKindVersionAlreadyExists
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check existing version: %w", err)
	}

	build, err := s.ocClient.GetBuild(ctx, orgName, sourceProjectName, sourceAgentName, buildName)
	if err != nil {
		return nil, fmt.Errorf("failed to get build: %w", err)
	}
	if build.ImageId == "" {
		return nil, utils.ErrBuildNotComplete
	}

	version := &models.AgentKindVersion{
		AgentKindID:  kind.ID,
		Version:      versionTag,
		BuildName:    buildName,
		ImageId:      build.ImageId,
		ConfigSchema: toModelConfigSchema(configSchema),
		CreatedAt:    time.Now(),
	}

	if err := s.kindRepo.CreateVersion(ctx, version); err != nil {
		return nil, fmt.Errorf("failed to create agent kind version: %w", err)
	}

	resp := toAgentKindVersionResponse(version)
	return &resp, nil
}

func toModelConfigSchema(items []spec.AgentKindConfigSchemaItem) []models.KindConfigSchemaItem {
	result := make([]models.KindConfigSchemaItem, len(items))
	for i, item := range items {
		result[i] = models.KindConfigSchemaItem{
			Name:        item.GetName(),
			Description: item.GetDescription(),
			IsSecret:    item.GetIsSecret(),
			IsMandatory: item.GetIsMandatory(),
		}
		if v, ok := item.GetDefaultValueOk(); ok && v != nil {
			result[i].DefaultValue = v
		}
	}
	return result
}

func toAgentKindVersionResponse(v *models.AgentKindVersion) models.AgentKindVersionResponse {
	resp := models.AgentKindVersionResponse{
		Version:      v.Version,
		BuildName:    v.BuildName,
		ImageId:      v.ImageId,
		ConfigSchema: v.ConfigSchema,
		CreatedAt:    v.CreatedAt,
	}
	if v.Kind != nil {
		resp.SourceAgentName = v.Kind.AgentName
		resp.SourceProjectName = v.Kind.ProjectName
	}
	return resp
}

func toAgentKindResponse(kind *models.AgentKind) *models.AgentKindResponse {
	versions := make([]models.AgentKindVersionResponse, len(kind.Versions))
	latestVersion := ""
	for i, v := range kind.Versions {
		versions[i] = toAgentKindVersionResponse(&v)
		// The first entry (ordered DESC by created_at) is the latest
		if i == 0 {
			latestVersion = v.Version
		}
	}

	return &models.AgentKindResponse{
		UUID:          kind.ID.String(),
		Name:          kind.Name,
		Kind:          "AgentKind",
		DisplayName:   kind.DisplayName,
		Description:   kind.Description,
		OrgName:       kind.OrgName,
		ProjectName:   kind.ProjectName,
		AgentName:     kind.AgentName,
		LatestVersion: latestVersion,
		Versions:      versions,
		CreatedAt:     kind.CreatedAt,
		UpdatedAt:     kind.UpdatedAt,
	}
}

// ValidateKindConfigValues checks that all mandatory schema items have a value supplied.
func ValidateKindConfigValues(schema []models.KindConfigSchemaItem, envVars []spec.EnvironmentVariable) error {
	provided := make(map[string]string, len(envVars))
	for _, v := range envVars {
		provided[v.GetKey()] = v.GetValue()
	}
	for _, item := range schema {
		if !item.IsMandatory {
			continue
		}
		val, ok := provided[item.Name]
		if !ok || val == "" {
			if item.DefaultValue != nil && *item.DefaultValue != "" {
				continue
			}
			return fmt.Errorf("%w: %q", utils.ErrMissingKindConfigValue, item.Name)
		}
	}
	return nil
}
