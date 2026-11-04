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
	"encoding/json"
	"errors"
	"fmt"
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
	GetKind(ctx context.Context, ouID, kindName string) (*models.AgentKindResponse, error)
	ListKinds(ctx context.Context, ouID string, labelFilter map[string]string, limit, offset int) (*models.AgentKindListResponse, error)
	UpdateKind(ctx context.Context, ouID, kindName string, req *spec.UpdateAgentKindRequest) (*models.AgentKindResponse, error)
	DeleteKind(ctx context.Context, ouID, kindName string) error

	// Version management
	AddVersion(ctx context.Context, ouID, kindName string, req *spec.AddAgentKindVersionRequest) (*models.AgentKindVersionResponse, error)
	GetVersion(ctx context.Context, ouID, kindName, versionTag string) (*models.AgentKindVersionResponse, error)
	ListVersions(ctx context.Context, ouID, kindName string) ([]models.AgentKindVersionResponse, error)
	DeleteVersion(ctx context.Context, ouID, kindName, versionTag string) error

	// Publish shortcut: creates kind if needed + adds version
	PublishKind(ctx context.Context, ouID, projectName, agentName string, req *spec.PublishAgentKindRequest) (*models.AgentKindVersionResponse, error)

	// For use during agent creation from kind
	GetKindVersion(ctx context.Context, ouID, kindName, versionTag string) (*models.AgentKindVersion, error)

	// ListKindAgents returns all agents deployed from a given kind across all projects in the org.
	ListKindAgents(ctx context.Context, ouID, kindName string) ([]*models.AgentResponse, error)

	// HasKindsSourcedFrom reports whether any agent kind is published from the
	// given source agent. See ExistsBySourceAgent in agent_kind_repository.go for
	// why this is a plain (not lock-protected) check, by design.
	HasKindsSourcedFrom(ctx context.Context, ouID, projectName, agentName string) (bool, error)
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
func (s *agentKindService) GetKind(ctx context.Context, ouID, kindName string) (*models.AgentKindResponse, error) {
	kind, err := s.kindRepo.GetKind(ctx, ouID, kindName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentKindNotFound
		}
		return nil, fmt.Errorf("failed to get agent kind: %w", err)
	}
	return toAgentKindResponse(kind), nil
}

// ListKinds returns a paginated list of Agent Kinds in the org, optionally
// filtered by user-defined labels (AND semantics).
func (s *agentKindService) ListKinds(ctx context.Context, ouID string, labelFilter map[string]string, limit, offset int) (*models.AgentKindListResponse, error) {
	kinds, total, err := s.kindRepo.ListKinds(ctx, ouID, labelFilter, limit, offset)
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
func (s *agentKindService) UpdateKind(ctx context.Context, ouID, kindName string, req *spec.UpdateAgentKindRequest) (*models.AgentKindResponse, error) {
	kind, err := s.kindRepo.GetKind(ctx, ouID, kindName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentKindNotFound
		}
		return nil, fmt.Errorf("failed to get agent kind: %w", err)
	}

	kind.DisplayName = req.GetDisplayName()
	kind.Description = req.GetDescription()
	// A nil Labels pointer means "leave unchanged"; an empty map clears them.
	if req.Labels != nil {
		kind.Labels = *req.Labels
	}
	kind.UpdatedAt = time.Now()

	if err := s.kindRepo.UpdateKind(ctx, kind); err != nil {
		return nil, fmt.Errorf("failed to update agent kind: %w", err)
	}

	return toAgentKindResponse(kind), nil
}

// DeleteKind deletes an Agent Kind and cascades to all versions.
// It returns ErrAgentKindHasInstances if any agents are still instantiated from this kind.
func (s *agentKindService) DeleteKind(ctx context.Context, ouID, kindName string) error {
	instances, err := s.ListKindAgents(ctx, ouID, kindName)
	if err != nil {
		return fmt.Errorf("failed to check kind instances before deletion: %w", err)
	}
	if len(instances) > 0 {
		return utils.ErrAgentKindHasInstances
	}

	err = s.kindRepo.DeleteKind(ctx, ouID, kindName)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return utils.ErrAgentKindNotFound
	}
	return err
}

// HasKindsSourcedFrom reports whether any agent kind is published from the
// given source agent. This is a plain existence check, not lock-protected —
// see the NOTE on AgentKindRepository.ExistsBySourceAgent for why a small
// check-then-act window here is an accepted, intentional trade-off rather than
// an oversight.
func (s *agentKindService) HasKindsSourcedFrom(ctx context.Context, ouID, projectName, agentName string) (bool, error) {
	exists, err := s.kindRepo.ExistsBySourceAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		return false, fmt.Errorf("failed to check agent kinds sourced from agent: %w", err)
	}
	return exists, nil
}

// AddVersion publishes a new version to an existing Agent Kind.
func (s *agentKindService) AddVersion(ctx context.Context, ouID, kindName string, req *spec.AddAgentKindVersionRequest) (*models.AgentKindVersionResponse, error) {
	kind, err := s.kindRepo.GetKind(ctx, ouID, kindName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentKindNotFound
		}
		return nil, fmt.Errorf("failed to get agent kind: %w", err)
	}

	metadata, err := marshalMetadata(req.GetMetadata())
	if err != nil {
		return nil, err
	}
	return s.publishVersion(ctx, ouID, kind, req.GetSourceProjectName(), req.GetSourceAgentName(), req.GetBuildName(), req.GetVersion(), req.GetConfigSchema(), metadata)
}

// GetVersion returns a specific version of an Agent Kind.
func (s *agentKindService) GetVersion(ctx context.Context, ouID, kindName, versionTag string) (*models.AgentKindVersionResponse, error) {
	kind, err := s.kindRepo.GetKind(ctx, ouID, kindName)
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
func (s *agentKindService) ListVersions(ctx context.Context, ouID, kindName string) ([]models.AgentKindVersionResponse, error) {
	kind, err := s.kindRepo.GetKind(ctx, ouID, kindName)
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
		responses[i] = toAgentKindVersionSummary(&versions[i])
	}
	return responses, nil
}

// DeleteVersion removes a specific version from an Agent Kind.
func (s *agentKindService) DeleteVersion(ctx context.Context, ouID, kindName, versionTag string) error {
	kind, err := s.kindRepo.GetKind(ctx, ouID, kindName)
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
func (s *agentKindService) PublishKind(ctx context.Context, ouID, projectName, agentName string, req *spec.PublishAgentKindRequest) (*models.AgentKindVersionResponse, error) {
	kindName := req.GetKindName()

	kind, err := s.kindRepo.GetKind(ctx, ouID, kindName)
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
			OUID:        ouID,
			ProjectName: projectName,
			AgentName:   agentName,
			Labels:      req.GetKindLabels(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Versions:    []models.AgentKindVersion{},
		}
		if newKind.Labels == nil {
			newKind.Labels = map[string]string{}
		}
		if createErr := s.kindRepo.CreateKind(ctx, newKind); createErr != nil {
			// Handle concurrent creation race
			existing, retryErr := s.kindRepo.GetKind(ctx, ouID, kindName)
			if retryErr == nil {
				kind = existing
			} else {
				return nil, fmt.Errorf("failed to create agent kind: %w", createErr)
			}
		} else {
			kind = newKind
		}
	}

	metadata, err := marshalMetadata(req.GetMetadata())
	if err != nil {
		return nil, err
	}
	return s.publishVersion(ctx, ouID, kind, projectName, agentName, req.GetBuildName(), req.GetVersion(), req.GetConfigSchema(), metadata)
}

// GetKindVersion returns the raw DB record for a kind version (used during agent creation from kind).
func (s *agentKindService) GetKindVersion(ctx context.Context, ouID, kindName, versionTag string) (*models.AgentKindVersion, error) {
	kind, err := s.kindRepo.GetKind(ctx, ouID, kindName)
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
func (s *agentKindService) ListKindAgents(ctx context.Context, ouID, kindName string) ([]*models.AgentResponse, error) {
	// Verify kind exists first
	if _, err := s.kindRepo.GetKind(ctx, ouID, kindName); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentKindNotFound
		}
		return nil, fmt.Errorf("failed to get agent kind: %w", err)
	}

	projects, err := listOrgProjects(ctx, s.ocClient, ouID)
	if err != nil {
		return nil, err
	}

	return fetchAcrossOrgProjects(ctx, projects, ouID, func(ctx context.Context, ouID, projectName string) ([]*models.AgentResponse, error) {
		return s.ocClient.ListComponentsByKind(ctx, ouID, projectName, kindName)
	})
}

// -----------------------------------------------------------------------------
// Internal helpers
// -----------------------------------------------------------------------------

// publishVersion fetches build details from OpenChoreo and persists the version.
func (s *agentKindService) publishVersion(
	ctx context.Context,
	ouID string,
	kind *models.AgentKind,
	sourceProjectName, sourceAgentName, buildName, versionTag string,
	configSchema []spec.AgentKindConfigSchemaItem,
	metadata json.RawMessage,
) (*models.AgentKindVersionResponse, error) {
	if err := RejectPlaceholderSecretDefaults(configSchema); err != nil {
		return nil, err
	}

	// Check version doesn't already exist
	existing, err := s.kindRepo.GetVersion(ctx, kind.ID, versionTag)
	if existing != nil {
		return nil, utils.ErrKindVersionAlreadyExists
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check existing version: %w", err)
	}

	// Explicitly verify the source agent component still exists, rather than
	// relying on GetBuild to fail if it doesn't. Build CRs aren't guaranteed to be
	// cascade-deleted with their owning component, so without this check a kind
	// could be published from (or have a version added from) an agent that was
	// already deleted — with no concurrency/timing involved at all. This is what
	// actually closes the "kind published from a deleted agent" gap; the
	// recheck in DeleteAgent only narrows the much rarer concurrent-race variant.
	if _, err := s.ocClient.GetComponent(ctx, ouID, sourceProjectName, sourceAgentName); err != nil {
		return nil, fmt.Errorf("%w: %s/%s", utils.ErrSourceAgentNotFound, sourceProjectName, sourceAgentName)
	}

	build, err := s.ocClient.GetBuild(ctx, ouID, sourceProjectName, sourceAgentName, buildName)
	if err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			return nil, utils.ErrBuildNotFound
		}
		return nil, fmt.Errorf("failed to get build: %w", err)
	}
	if build.ImageId == "" {
		return nil, utils.ErrBuildNotComplete
	}

	// Block re-publishing the same image under a different version of this kind
	if dup, dupErr := s.kindRepo.GetVersionByImageID(ctx, kind.ID, build.ImageId); dupErr == nil && dup != nil {
		return nil, fmt.Errorf("%w: already published as version %q of kind %q", utils.ErrKindImageAlreadyPublished, dup.Version, kind.Name)
	} else if dupErr != nil && !errors.Is(dupErr, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check for duplicate image: %w", dupErr)
	}

	// Block re-publishing the same image under any other kind in the org
	if orgDup, orgDupErr := s.kindRepo.FindVersionByImageIDInOrg(ctx, ouID, build.ImageId); orgDupErr == nil && orgDup != nil {
		return nil, fmt.Errorf("%w: image already published as version %q of kind %q", utils.ErrKindImageAlreadyPublished, orgDup.Version, orgDup.Kind.Name)
	} else if orgDupErr != nil && !errors.Is(orgDupErr, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check for duplicate image across org: %w", orgDupErr)
	}

	version := &models.AgentKindVersion{
		AgentKindID:  kind.ID,
		Version:      versionTag,
		BuildName:    buildName,
		ImageId:      build.ImageId,
		ConfigSchema: toModelConfigSchema(configSchema),
		Metadata:     metadata,
		CreatedAt:    time.Now(),
	}

	if err := s.kindRepo.CreateVersion(ctx, version); err != nil {
		return nil, fmt.Errorf("failed to create agent kind version: %w", err)
	}

	resp := toAgentKindVersionResponse(version)
	return &resp, nil
}

// marshalMetadata converts a map to json.RawMessage; returns nil for nil/empty maps.
func marshalMetadata(m map[string]interface{}) (json.RawMessage, error) {
	if len(m) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}
	return b, nil
}

// RejectPlaceholderSecretDefaults errors if a secret item's submitted default value is
// models.RedactedSecretDefaultPlaceholder. That string is the server's own stand-in for
// a hidden secret default (see models.RedactSecretDefaults) — a client can only have
// gotten it by reading a previous response, never by knowing a real secret value.
// Storing it here would let it later be applied to an agent as its actual secret,
// which is never a legitimate default.
func RejectPlaceholderSecretDefaults(items []spec.AgentKindConfigSchemaItem) error {
	for _, item := range items {
		if !item.GetIsSecret() {
			continue
		}
		if v, ok := item.GetDefaultValueOk(); ok && v != nil && *v == models.RedactedSecretDefaultPlaceholder {
			return fmt.Errorf("%w: default value for secret item %q cannot be the reserved placeholder", utils.ErrInvalidInput, item.GetName())
		}
	}
	return nil
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

// toAgentKindVersionResponse builds the full response including metadata (used for single-version fetch).
func toAgentKindVersionResponse(v *models.AgentKindVersion) models.AgentKindVersionResponse {
	resp := models.AgentKindVersionResponse{
		Version:      v.Version,
		BuildName:    v.BuildName,
		ImageId:      v.ImageId,
		ConfigSchema: models.RedactSecretDefaults(v.ConfigSchema),
		Metadata:     v.Metadata,
		CreatedAt:    v.CreatedAt,
	}
	if v.Kind != nil {
		resp.SourceAgentName = v.Kind.AgentName
		resp.SourceProjectName = v.Kind.ProjectName
	}
	return resp
}

// toAgentKindVersionSummary builds a response without metadata (used for list views).
func toAgentKindVersionSummary(v *models.AgentKindVersion) models.AgentKindVersionResponse {
	resp := models.AgentKindVersionResponse{
		Version:      v.Version,
		BuildName:    v.BuildName,
		ImageId:      v.ImageId,
		ConfigSchema: models.RedactSecretDefaults(v.ConfigSchema),
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
		versions[i] = toAgentKindVersionSummary(&v)
		// The first entry (ordered DESC by created_at) is the latest
		if i == 0 {
			latestVersion = v.Version
		}
	}

	labels := kind.Labels
	if labels == nil {
		labels = map[string]string{}
	}

	return &models.AgentKindResponse{
		UUID:          kind.ID.String(),
		Name:          kind.Name,
		Kind:          "AgentKind",
		DisplayName:   kind.DisplayName,
		Description:   kind.Description,
		ProjectName:   kind.ProjectName,
		AgentName:     kind.AgentName,
		LatestVersion: latestVersion,
		Labels:        labels,
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

// RejectDuplicateEnvKeys errors if envVars names the same key more than once. There
// is exactly one process environment per name, so a duplicate is always invalid
// input, not a matter of "last one wins" — silently picking one would let the wrong
// entry govern which value (and whether it's treated as sensitive) actually applies.
func RejectDuplicateEnvKeys(envVars []spec.EnvironmentVariable) error {
	seen := make(map[string]bool, len(envVars))
	for _, v := range envVars {
		key := v.GetKey()
		if seen[key] {
			return fmt.Errorf("%w: duplicate environment variable key %q", utils.ErrInvalidInput, key)
		}
		seen[key] = true
	}
	return nil
}

// ApplySecretConfigDefaults fills in each IsSecret schema item's stored default value
// for any item the caller didn't submit (or submitted empty), and forces IsSensitive
// on every env var that matches a secret schema item regardless of what the caller
// sent. The server is the only party that still holds a secret's real default (see
// models.RedactSecretDefaults), so applying it here is what lets someone accept a
// kind's default without ever having it round-tripped through their browser.
// Precondition: envVars has no duplicate keys — call RejectDuplicateEnvKeys first.
func ApplySecretConfigDefaults(schema []models.KindConfigSchemaItem, envVars []spec.EnvironmentVariable) []spec.EnvironmentVariable {
	indexByKey := make(map[string]int, len(envVars))
	for i, v := range envVars {
		indexByKey[v.GetKey()] = i
	}

	result := envVars
	for _, item := range schema {
		if !item.IsSecret {
			continue
		}
		if i, ok := indexByKey[item.Name]; ok {
			result[i].SetIsSensitive(true)
			if result[i].GetValue() == "" && item.DefaultValue != nil && *item.DefaultValue != "" {
				result[i].SetValue(*item.DefaultValue)
			}
			continue
		}
		if item.DefaultValue != nil && *item.DefaultValue != "" {
			ev := spec.EnvironmentVariable{Key: item.Name}
			ev.SetValue(*item.DefaultValue)
			ev.SetIsSensitive(true)
			result = append(result, ev)
		}
	}
	return result
}
