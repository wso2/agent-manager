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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// DeploymentResult captures the outcome of deploying to a single gateway
type DeploymentResult struct {
	GatewayID string `json:"gateway_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

// CreateAndDeployResponse contains the created provider and deployment results
type CreateAndDeployResponse struct {
	Provider    *models.LLMProvider `json:"provider"`
	Deployments []DeploymentResult  `json:"deployments"`
}

// UpdateAndSyncResponse contains the updated provider and sync results
type UpdateAndSyncResponse struct {
	Provider      *models.LLMProvider `json:"provider"`
	Deployments   []DeploymentResult  `json:"deployments"`   // Results for new gateway deployments
	Undeployments []DeploymentResult  `json:"undeployments"` // Results for removed gateway undeployments
}

// LLMProviderService handles LLM provider business logic
type LLMProviderService struct {
	db                 *gorm.DB
	providerRepo       repositories.LLMProviderRepository
	templateRepo       repositories.LLMProviderTemplateRepository
	templateStore      *LLMTemplateStore
	proxyRepo          repositories.LLMProxyRepository
	artifactRepo       repositories.ArtifactRepository
	encryptionKey      []byte
	gatewayRepo        repositories.GatewayRepository
	deploymentRepo     repositories.DeploymentRepository
	agentMappingRepo   repositories.EnvAgentModelMappingRepository
	monitorMappingRepo repositories.MonitorLLMMappingRepository
	apiKeyService      *LLMProviderAPIKeyService
}

// NewLLMProviderService creates a new LLM provider service
func NewLLMProviderService(
	db *gorm.DB,
	providerRepo repositories.LLMProviderRepository,
	templateRepo repositories.LLMProviderTemplateRepository,
	templateStore *LLMTemplateStore,
	proxyRepo repositories.LLMProxyRepository,
	artifactRepo repositories.ArtifactRepository,
	encryptionKey []byte,
	gatewayRepo repositories.GatewayRepository,
	deploymentRepo repositories.DeploymentRepository,
	agentMappingRepo repositories.EnvAgentModelMappingRepository,
	monitorMappingRepo repositories.MonitorLLMMappingRepository,
	apiKeyService *LLMProviderAPIKeyService,
) *LLMProviderService {
	return &LLMProviderService{
		db:                 db,
		providerRepo:       providerRepo,
		templateRepo:       templateRepo,
		templateStore:      templateStore,
		proxyRepo:          proxyRepo,
		artifactRepo:       artifactRepo,
		encryptionKey:      encryptionKey,
		gatewayRepo:        gatewayRepo,
		deploymentRepo:     deploymentRepo,
		agentMappingRepo:   agentMappingRepo,
		monitorMappingRepo: monitorMappingRepo,
		apiKeyService:      apiKeyService,
	}
}

// providerVersionPattern mirrors the spec's `version` pattern. The endpoint used to
// accept anything, so a client sending "v1" persisted a version its own generated
// types declare as invalid.
var providerVersionPattern = regexp.MustCompile(`^v\d+\.\d+$`)

func validateProviderVersion(version string) error {
	if !providerVersionPattern.MatchString(version) {
		return fmt.Errorf("%w: version must match %s, e.g. v1.0", utils.ErrInvalidInput, providerVersionPattern)
	}
	return nil
}

// rollbackCreatedProvider removes a provider whose every gateway deployment failed,
// so a failed CreateAndDeploy leaves nothing behind. The rollback error is returned
// as well as logged: it is not the error the caller reports — the deployment failure
// explains what went wrong — but a rollback that failed means the provider survived,
// and a caller told only "deployments failed" would retry the same handle and be
// rejected with ErrLLMProviderExists by a provider it does not know exists.
func (s *LLMProviderService) rollbackCreatedProvider(
	ctx context.Context, created *models.LLMProvider, ouID string,
	deploymentService *LLMProviderDeploymentService,
) error {
	if err := s.Delete(ctx, created.UUID.String(), ouID, deploymentService); err != nil {
		slog.Error("LLMProviderService.CreateAndDeploy: failed to roll back provider after deployment failure",
			"ouID", ouID, "providerUUID", created.UUID, "error", err)
		return fmt.Errorf("failed to roll back provider %s: %w", created.UUID, err)
	}
	slog.Info("LLMProviderService.CreateAndDeploy: rolled back provider after deployment failure",
		"ouID", ouID, "providerUUID", created.UUID)
	return nil
}

// summarizeDeploymentFailures joins the per-gateway errors so the caller learns why
// the deployments failed instead of only that they did.
func summarizeDeploymentFailures(results []DeploymentResult) string {
	reasons := make([]string, 0, len(results))
	for _, result := range results {
		if !result.Success {
			reasons = append(reasons, fmt.Sprintf("%s: %s", result.GatewayID, result.Error))
		}
	}
	return strings.Join(reasons, "; ")
}

// resolveTemplate returns the built-in or org-owned template behind handle.
func (s *LLMProviderService) resolveTemplate(handle, ouID string) (*models.LLMProviderTemplate, error) {
	if builtin := s.templateStore.Get(handle); builtin != nil {
		return builtin, nil
	}

	userTemplate, err := s.templateRepo.GetByHandle(handle, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrLLMProviderTemplateNotFound
		}
		return nil, fmt.Errorf("failed to validate template: %w", err)
	}
	if userTemplate == nil {
		return nil, utils.ErrLLMProviderTemplateNotFound
	}
	return userTemplate, nil
}

// applyTemplateUpstreamDefaults fills the upstream URL and auth scheme from the
// template's metadata wherever the caller left them unset. Every client documents
// these as inherited from the template, but no layer applied them: a provider
// created without an explicit --upstream-url deployed a proxy whose upstream URL was
// empty and was silently dropped from the gateway config.
//
// Caller-supplied values always win, and this must run before the credential is
// encrypted so the template's value prefix lands inside the ciphertext.
func applyTemplateUpstreamDefaults(provider *models.LLMProvider, template *models.LLMProviderTemplate) {
	meta := template.Metadata
	hasInheritableDefaults := meta != nil && (meta.EndpointURL != "" || meta.Auth != nil)
	if !hasInheritableDefaults {
		return
	}

	if provider.Configuration.Upstream == nil {
		provider.Configuration.Upstream = &models.UpstreamConfig{}
	}
	if provider.Configuration.Upstream.Main == nil {
		provider.Configuration.Upstream.Main = &models.UpstreamEndpoint{}
	}
	main := provider.Configuration.Upstream.Main

	if main.URL == "" {
		main.URL = meta.EndpointURL
	}
	if meta.Auth != nil {
		main.Auth = applyTemplateAuthDefaults(main.Auth, meta.Auth)
	}
}

// applyTemplateAuthDefaults fills in the auth type, header and value prefix the
// template declares. An endpoint that has a URL but no auth block still gets the
// template's scheme, so `--upstream-url` without a credential no longer produces a
// provider that cannot authenticate.
func applyTemplateAuthDefaults(auth *models.UpstreamAuth, templateAuth *models.LLMProviderTemplateAuth) *models.UpstreamAuth {
	if auth == nil {
		auth = &models.UpstreamAuth{}
	}
	// Copied rather than aliased: for a built-in handle templateAuth points into the
	// process-wide template store, so handing out &templateAuth.Type would let a write
	// through the provider corrupt the template for every organization.
	if utils.StrPointerAsStr(auth.Type, "") == "" && templateAuth.Type != "" {
		authType := templateAuth.Type
		auth.Type = &authType
	}
	if utils.StrPointerAsStr(auth.Header, "") == "" && templateAuth.Header != "" {
		header := templateAuth.Header
		auth.Header = &header
	}
	if templateAuth.ValuePrefix != "" && auth.Value != nil &&
		!strings.HasPrefix(*auth.Value, templateAuth.ValuePrefix) {
		prefixed := templateAuth.ValuePrefix + *auth.Value
		auth.Value = &prefixed
	}
	return auth
}

// resolveProvider looks up a provider by UUID or handle.
func (s *LLMProviderService) resolveProvider(identifier, ouID string) (*models.LLMProvider, error) {
	if _, err := uuid.Parse(identifier); err == nil {
		return s.providerRepo.GetByUUID(identifier, ouID)
	}
	return s.providerRepo.GetByHandle(identifier, ouID)
}

// Create creates a new LLM provider
func (s *LLMProviderService) Create(ctx context.Context, ouID, createdBy string, provider *models.LLMProvider) (*models.LLMProvider, error) {
	logger.GetLogger(ctx).Info("LLMProviderService.Create: starting", "ou_id", ouID, "created_by", createdBy)

	if provider == nil {
		logger.GetLogger(ctx).Error("LLMProviderService.Create: provider is nil", "ou_id", ouID)
		return nil, utils.ErrInvalidInput
	}

	// Extract handle, name, and version from configuration
	// Note: handle is not in Configuration, so we use name as handle
	name := provider.Configuration.Name
	version := provider.Configuration.Version

	// Use name as handle (artifact identifier)
	handle := provider.Configuration.Handle

	logger.GetLogger(ctx).Info("LLMProviderService.Create: extracted configuration", "ou_id", ouID, "handle", handle, "name", name, "version", version)

	if handle == "" || name == "" || version == "" {
		logger.GetLogger(ctx).Error("LLMProviderService.Create: missing required fields", "ou_id", ouID, "handle", handle, "name", name, "version", version)
		return nil, utils.ErrInvalidInput
	}

	// Fail fast if a provider with this handle already exists, before touching KV.
	if _, err := s.providerRepo.GetByHandle(handle, ouID); err == nil {
		logger.GetLogger(ctx).Warn("LLMProviderService.Create: provider already exists", "ou_id", ouID, "handle", handle)
		return nil, utils.ErrLLMProviderExists
	}

	// Validate template exists
	template := provider.Configuration.Template
	if template == "" {
		logger.GetLogger(ctx).Error("LLMProviderService.Create: template not specified", "ou_id", ouID, "handle", handle)
		return nil, utils.ErrInvalidInput
	}

	// Set default values
	provider.CreatedBy = createdBy
	if provider.Configuration.Context == nil {
		defaultContext := "/"
		provider.Configuration.Context = &defaultContext
	}

	logger.GetLogger(ctx).Info("LLMProviderService.Create: set default values", "ou_id", ouID, "handle", handle, "context", *provider.Configuration.Context)

	// Serialize model providers to ModelList
	if len(provider.ModelProviders) > 0 {
		logger.GetLogger(ctx).Info("LLMProviderService.Create: serializing model providers", "ou_id", ouID, "handle", handle, "count", len(provider.ModelProviders))
		modelListBytes, err := json.Marshal(provider.ModelProviders)
		if err != nil {
			logger.GetLogger(ctx).Warn("LLMProviderService.Create: failed to serialize model providers", "ou_id", ouID, "handle", handle, "error", err)
			return nil, fmt.Errorf("failed to serialize model providers: %w", err)
		}
		provider.ModelList = string(modelListBytes)
	}

	// Resolve the template (built-in or org-owned) rather than merely checking that
	// it exists: the upstream endpoint and auth scheme every client documents as
	// "inherited from the template" live in its metadata.
	slog.Info("LLMProviderService.Create: resolving template", "ouID", ouID, "handle", handle, "template", template)
	resolvedTemplate, err := s.resolveTemplate(template, ouID)
	if err != nil {
		slog.Warn("LLMProviderService.Create: template unusable", "ouID", ouID, "handle", handle, "template", template, "error", err)
		return nil, err
	}

	// Set template handle in provider
	provider.TemplateHandle = template

	applyTemplateUpstreamDefaults(provider, resolvedTemplate)

	if err := validateProviderVersion(version); err != nil {
		slog.Warn("LLMProviderService.Create: invalid version", "ouID", ouID, "handle", handle, "version", version)
		return nil, err
	}

	// Validate mutual exclusivity of Auth.Value and Auth.SecretRef
	if provider.Configuration.Upstream != nil &&
		provider.Configuration.Upstream.Main != nil &&
		provider.Configuration.Upstream.Main.Auth != nil {
		if err := provider.Configuration.Upstream.Main.Auth.Validate(); err != nil {
			return nil, err
		}
	}

	if err := provider.Configuration.Resilience.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", utils.ErrInvalidInput, err)
	}

	// Encrypt upstream API key if provided
	if provider.Configuration.Upstream != nil &&
		provider.Configuration.Upstream.Main != nil &&
		provider.Configuration.Upstream.Main.Auth != nil &&
		provider.Configuration.Upstream.Main.Auth.Value != nil {

		encrypted, err := utils.EncryptBytes([]byte(*provider.Configuration.Upstream.Main.Auth.Value), s.encryptionKey)
		if err != nil {
			logger.GetLogger(ctx).Warn("LLMProviderService.Create: failed to encrypt upstream key",
				"ou_id", ouID, "handle", handle, "error", err)
			return nil, fmt.Errorf("failed to encrypt upstream API key: %w", err)
		}
		encoded := base64.StdEncoding.EncodeToString(encrypted)

		// Replace plaintext with encrypted reference
		provider.Configuration.Upstream.Main.Auth.SecretRef = &encoded
		provider.Configuration.Upstream.Main.Auth.Value = nil

		logger.GetLogger(ctx).Info("LLMProviderService.Create: encrypted upstream key",
			"ou_id", ouID, "handle", handle)
	}

	// Create provider in transaction with validation
	slog.Info("LLMProviderService.Create: creating provider in database", "ouID", ouID, "handle", handle, "name", name, "version", version)
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Create provider - uniqueness enforced by DB constraint
		return s.providerRepo.Create(tx, provider, handle, name, version, ouID)
	})
	if err != nil {
		// Check for unique constraint violation
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			logger.GetLogger(ctx).Warn("LLMProviderService.Create: provider already exists (unique constraint)", "ou_id", ouID, "handle", handle)
			return nil, utils.ErrLLMProviderExists
		}
		// Return template not found error directly
		if errors.Is(err, utils.ErrLLMProviderTemplateNotFound) {
			return nil, err
		}
		logger.GetLogger(ctx).Warn("LLMProviderService.Create: failed to create provider", "ou_id", ouID, "handle", handle, "error", err)
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	logger.GetLogger(ctx).Info("LLMProviderService.Create: provider created, fetching details", "ou_id", ouID, "handle", handle, "uuid", provider.UUID)

	// Fetch created provider by UUID
	created, err := s.providerRepo.GetByUUID(provider.UUID.String(), ouID)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProviderService.Create: failed to fetch created provider", "ou_id", ouID, "uuid", provider.UUID, "error", err)
		return nil, fmt.Errorf("failed to fetch created provider: %w", err)
	}

	// Parse model providers from ModelList
	if created.ModelList != "" {
		logger.GetLogger(ctx).Info("LLMProviderService.Create: parsing model providers from ModelList", "ou_id", ouID, "handle", handle)
		if err := json.Unmarshal([]byte(created.ModelList), &created.ModelProviders); err != nil {
			logger.GetLogger(ctx).Warn("LLMProviderService.Create: failed to parse model providers", "ou_id", ouID, "handle", handle, "error", err)
			return nil, fmt.Errorf("failed to parse model providers: %w", err)
		}
	}

	logger.GetLogger(ctx).Info("LLMProviderService.Create: completed successfully", "ou_id", ouID, "handle", handle, "provider_uuid", created.UUID)
	return created, nil
}

// List lists all LLM providers for an organization
func (s *LLMProviderService) List(ctx context.Context, ouID string, limit, offset int) ([]*models.LLMProvider, int, error) {
	logger.GetLogger(ctx).Info("LLMProviderService.List: starting", "ou_id", ouID, "limit", limit, "offset", offset)

	providers, err := s.providerRepo.List(ouID, limit, offset)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProviderService.List: failed to list providers", "ou_id", ouID, "error", err)
		return nil, 0, fmt.Errorf("failed to list providers: %w", err)
	}

	logger.GetLogger(ctx).Info("LLMProviderService.List: providers retrieved from repository", "ou_id", ouID, "count", len(providers))

	// Parse model providers for each provider
	for i, p := range providers {
		if p.ModelList != "" {
			if err := json.Unmarshal([]byte(p.ModelList), &p.ModelProviders); err != nil {
				logger.GetLogger(ctx).Warn("LLMProviderService.List: failed to parse model providers", "ou_id", ouID, "provider_index", i, "provider_uuid", p.UUID, "error", err)
				return nil, 0, fmt.Errorf("failed to parse model providers: %w", err)
			}
		}
	}

	totalCount, err := s.providerRepo.Count(ouID)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProviderService.List: failed to count providers", "ou_id", ouID, "error", err)
		return nil, 0, fmt.Errorf("failed to count providers: %w", err)
	}

	logger.GetLogger(ctx).Info("LLMProviderService.List: completed successfully", "ou_id", ouID, "count", len(providers), "total", totalCount)
	return providers, totalCount, nil
}

// ListAvailableLLMPolicies returns full guardrail policy definitions reported by active
// gateways in the organization, so the console can list and configure them directly
// without depending on the external policy hub. When providerID is non-empty, the result
// is scoped to the gateways that provider is currently deployed to, instead of every
// active gateway in the org.
func (s *LLMProviderService) ListAvailableLLMPolicies(ctx context.Context, ouID, providerID string) (*models.LLMPolicyAvailabilityResponse, error) {
	_ = ctx

	var available map[string]llmPolicyManifestItem
	var err error
	if providerID != "" {
		provider, resolveErr := s.resolveProvider(providerID, ouID)
		if resolveErr != nil {
			if errors.Is(resolveErr, gorm.ErrRecordNotFound) {
				return nil, utils.ErrLLMProviderNotFound
			}
			return nil, fmt.Errorf("failed to resolve provider: %w", resolveErr)
		}
		if provider == nil {
			return nil, utils.ErrLLMProviderNotFound
		}
		available, err = intersectDeployedGatewayLLMPolicies(s.gatewayRepo, s.deploymentRepo, provider.UUID, ouID)
	} else {
		available, err = intersectActiveGatewayLLMPolicies(s.gatewayRepo, ouID)
	}
	if err != nil {
		return nil, err
	}

	sorted := sortedLLMPolicyManifestItems(available)
	items := make([]models.LLMPolicyDefinition, 0, len(sorted))
	for _, item := range sorted {
		items = append(items, models.LLMPolicyDefinition{
			Name:             item.Name,
			Version:          item.Version,
			DisplayName:      item.DisplayName,
			Description:      item.Description,
			Parameters:       item.Parameters,
			SystemParameters: item.SystemParameters,
		})
	}

	return &models.LLMPolicyAvailabilityResponse{
		Count: int32(len(items)),
		List:  items,
	}, nil
}

// Get retrieves an LLM provider by ID
func (s *LLMProviderService) Get(ctx context.Context, providerID, ouID string) (*models.LLMProvider, error) {
	logger.GetLogger(ctx).Info("LLMProviderService.Get: starting", "ou_id", ouID, "provider_id", providerID)

	if providerID == "" {
		logger.GetLogger(ctx).Error("LLMProviderService.Get: providerID is empty", "ou_id", ouID)
		return nil, utils.ErrInvalidInput
	}

	provider, err := s.resolveProvider(providerID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logger.GetLogger(ctx).Warn("LLMProviderService.Get: provider not found", "ou_id", ouID, "provider_id", providerID)
			return nil, utils.ErrLLMProviderNotFound
		}
		logger.GetLogger(ctx).Warn("LLMProviderService.Get: failed to get provider", "ou_id", ouID, "provider_id", providerID, "error", err)
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		logger.GetLogger(ctx).Warn("LLMProviderService.Get: provider not found", "ou_id", ouID, "provider_id", providerID)
		return nil, utils.ErrLLMProviderNotFound
	}

	// Parse model providers from ModelList
	if provider.ModelList != "" {
		logger.GetLogger(ctx).Info("LLMProviderService.Get: parsing model providers", "ou_id", ouID, "provider_id", providerID, "provider_uuid", provider.UUID)
		if err := json.Unmarshal([]byte(provider.ModelList), &provider.ModelProviders); err != nil {
			logger.GetLogger(ctx).Warn("LLMProviderService.Get: failed to parse model providers", "ou_id", ouID, "provider_id", providerID, "error", err)
			return nil, fmt.Errorf("failed to parse model providers: %w", err)
		}
	}

	logger.GetLogger(ctx).Info("LLMProviderService.Get: completed successfully", "ou_id", ouID, "provider_id", providerID, "provider_uuid", provider.UUID)
	return provider, nil
}

// Update updates an existing LLM provider
func (s *LLMProviderService) Update(ctx context.Context, providerID, ouID string, updates *models.LLMProvider) (*models.LLMProvider, error) {
	logger.GetLogger(ctx).Info("LLMProviderService.Update: starting", "ou_id", ouID, "provider_id", providerID)

	if providerID == "" || updates == nil {
		logger.GetLogger(ctx).Error("LLMProviderService.Update: invalid input", "ou_id", ouID, "provider_id", providerID, "updates_is_nil", updates == nil)
		return nil, utils.ErrInvalidInput
	}

	// Validate template exists (check both built-in and user templates)
	template := updates.Configuration.Template
	if template != "" {
		logger.GetLogger(ctx).Info("LLMProviderService.Update: validating template", "ou_id", ouID, "provider_id", providerID, "template", template)
		templateExists := s.templateStore.Exists(template)
		if !templateExists {
			// Check user templates in database
			userTemplateExists, err := s.templateRepo.Exists(template, ouID)
			if err != nil {
				logger.GetLogger(ctx).Warn("LLMProviderService.Update: failed to validate user template", "ou_id", ouID, "provider_id", providerID, "template", template, "error", err)
				return nil, fmt.Errorf("failed to validate template: %w", err)
			}
			if !userTemplateExists {
				logger.GetLogger(ctx).Warn("LLMProviderService.Update: template not found", "ou_id", ouID, "provider_id", providerID, "template", template)
				return nil, utils.ErrLLMProviderTemplateNotFound
			}
		}
		// Set template handle in updates
		updates.TemplateHandle = template
	}

	// Serialize model providers to ModelList
	if len(updates.ModelProviders) > 0 {
		logger.GetLogger(ctx).Info("LLMProviderService.Update: serializing model providers", "ou_id", ouID, "provider_id", providerID, "count", len(updates.ModelProviders))
		modelListBytes, err := json.Marshal(updates.ModelProviders)
		if err != nil {
			logger.GetLogger(ctx).Warn("LLMProviderService.Update: failed to serialize model providers", "ou_id", ouID, "provider_id", providerID, "error", err)
			return nil, fmt.Errorf("failed to serialize model providers: %w", err)
		}
		updates.ModelList = string(modelListBytes)
	}

	// Validate mutual exclusivity of Auth.Value and Auth.SecretRef
	if updates.Configuration.Upstream != nil &&
		updates.Configuration.Upstream.Main != nil &&
		updates.Configuration.Upstream.Main.Auth != nil {
		if err := updates.Configuration.Upstream.Main.Auth.Validate(); err != nil {
			return nil, err
		}
	}

	if err := updates.Configuration.Resilience.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", utils.ErrInvalidInput, err)
	}

	// Encrypt upstream API key if a new value is provided
	if updates.Configuration.Upstream != nil &&
		updates.Configuration.Upstream.Main != nil &&
		updates.Configuration.Upstream.Main.Auth != nil &&
		updates.Configuration.Upstream.Main.Auth.Value != nil {

		encrypted, err := utils.EncryptBytes([]byte(*updates.Configuration.Upstream.Main.Auth.Value), s.encryptionKey)
		if err != nil {
			logger.GetLogger(ctx).Warn("LLMProviderService.Update: failed to encrypt upstream key",
				"ou_id", ouID, "provider_id", providerID, "error", err)
			return nil, fmt.Errorf("failed to encrypt upstream API key: %w", err)
		}
		encoded := base64.StdEncoding.EncodeToString(encrypted)
		// Replace plaintext with encrypted reference
		updates.Configuration.Upstream.Main.Auth.SecretRef = &encoded
		updates.Configuration.Upstream.Main.Auth.Value = nil
	}

	// Snapshot whether API key auth was enabled before this update, so we can revoke
	// user-managed keys below if the user is turning it off. The read must succeed: a
	// swallowed error here would leave the snapshot at false and silently skip revocation,
	// so surface it to the caller instead of proceeding with a stale snapshot.
	existing, err := s.resolveProvider(providerID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logger.GetLogger(ctx).Warn("LLMProviderService.Update: provider not found", "ou_id", ouID, "provider_id", providerID)
			return nil, utils.ErrLLMProviderNotFound
		}
		logger.GetLogger(ctx).Warn("LLMProviderService.Update: failed to read provider before update", "ou_id", ouID, "provider_id", providerID, "error", err)
		return nil, fmt.Errorf("failed to read provider before update: %w", err)
	}
	if existing == nil {
		logger.GetLogger(ctx).Warn("LLMProviderService.Update: provider not found", "ou_id", ouID, "provider_id", providerID)
		return nil, utils.ErrLLMProviderNotFound
	}
	apiKeyAuthWasEnabled := isAPIKeyAuthEnabled(existing.Configuration.Security)

	// Update provider
	logger.GetLogger(ctx).Info("LLMProviderService.Update: updating provider in database", "ou_id", ouID, "provider_id", providerID)
	if err := s.providerRepo.Update(updates, providerID, ouID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logger.GetLogger(ctx).Warn("LLMProviderService.Update: provider not found", "ou_id", ouID, "provider_id", providerID)
			return nil, utils.ErrLLMProviderNotFound
		}
		logger.GetLogger(ctx).Warn("LLMProviderService.Update: failed to update provider", "ou_id", ouID, "provider_id", providerID, "error", err)
		return nil, fmt.Errorf("failed to update provider: %w", err)
	}

	// Fetch updated provider
	logger.GetLogger(ctx).Info("LLMProviderService.Update: fetching updated provider", "ou_id", ouID, "provider_id", providerID)
	updated, err := s.resolveProvider(providerID, ouID)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProviderService.Update: failed to fetch updated provider", "ou_id", ouID, "provider_id", providerID, "error", err)
		return nil, fmt.Errorf("failed to fetch updated provider: %w", err)
	}
	if updated == nil {
		logger.GetLogger(ctx).Warn("LLMProviderService.Update: updated provider not found", "ou_id", ouID, "provider_id", providerID)
		return nil, utils.ErrLLMProviderNotFound
	}

	// Parse model providers from ModelList
	if updated.ModelList != "" {
		logger.GetLogger(ctx).Info("LLMProviderService.Update: parsing model providers", "ou_id", ouID, "provider_id", providerID)
		if err := json.Unmarshal([]byte(updated.ModelList), &updated.ModelProviders); err != nil {
			logger.GetLogger(ctx).Warn("LLMProviderService.Update: failed to parse model providers", "ou_id", ouID, "provider_id", providerID, "error", err)
			return nil, fmt.Errorf("failed to parse model providers: %w", err)
		}
	}

	// If API key authentication was just turned off, revoke all user-managed API keys for
	// this provider. Best-effort: log and continue so a revoke failure doesn't fail the update.
	if apiKeyAuthWasEnabled && !isAPIKeyAuthEnabled(updated.Configuration.Security) && s.apiKeyService != nil {
		if err := s.apiKeyService.RevokeAllUserManagedKeys(ctx, ouID, providerID); err != nil {
			logger.GetLogger(ctx).Warn("LLMProviderService.Update: failed to revoke user-managed API keys after disabling API key security",
				"ou_id", ouID, "provider_id", providerID, "error", err)
		}
	}

	logger.GetLogger(ctx).Info("LLMProviderService.Update: completed successfully", "ou_id", ouID, "provider_id", providerID, "provider_uuid", updated.UUID)
	return updated, nil
}

// Delete deletes an LLM provider after undeploying from all gateways
func (s *LLMProviderService) Delete(ctx context.Context, providerID, ouID string, deploymentService *LLMProviderDeploymentService) error {
	logger.GetLogger(ctx).Info("LLMProviderService.Delete: starting", "ou_id", ouID, "provider_id", providerID)

	if providerID == "" {
		logger.GetLogger(ctx).Error("LLMProviderService.Delete: providerID is empty", "ou_id", ouID)
		return utils.ErrInvalidInput
	}

	// Verify provider exists
	provider, err := s.resolveProvider(providerID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logger.GetLogger(ctx).Warn("LLMProviderService.Delete: provider not found", "ou_id", ouID, "provider_id", providerID)
			return utils.ErrLLMProviderNotFound
		}
		logger.GetLogger(ctx).Warn("LLMProviderService.Delete: failed to get provider", "ou_id", ouID, "provider_id", providerID, "error", err)
		return fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		logger.GetLogger(ctx).Warn("LLMProviderService.Delete: provider not found", "ou_id", ouID, "provider_id", providerID)
		return utils.ErrLLMProviderNotFound
	}

	// Get all deployed gateways for this provider. providerID may be a handle, so
	// use the UUID resolved above rather than re-parsing the raw identifier.
	providerUUID := provider.UUID

	gatewayIDs, err := deploymentService.deploymentRepo.GetDeployedGatewaysByProvider(providerUUID, ouID)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProviderService.Delete: failed to get deployed gateways", "ou_id", ouID, "provider_id", providerID, "error", err)
		return fmt.Errorf("failed to get deployed gateways: %w", err)
	}

	logger.GetLogger(ctx).Info("LLMProviderService.Delete: found deployed gateways", "ou_id", ouID, "provider_id", providerID, "gateway_count", len(gatewayIDs))

	// Undeploy from all gateways before deleting
	if len(gatewayIDs) > 0 {
		undeploymentErrors := []string{}
		successfulUndeployments := 0

		for _, gatewayID := range gatewayIDs {
			logger.GetLogger(ctx).Info("LLMProviderService.Delete: undeploying from gateway", "ou_id", ouID, "provider_id", providerID, "gateway_id", gatewayID)

			// Get current deployment for this gateway
			deployments, err := deploymentService.GetLLMProviderDeployments(providerID, ouID, &gatewayID, nil)
			if err != nil {
				logger.GetLogger(ctx).Error("LLMProviderService.Delete: failed to get deployments for gateway", "ou_id", ouID, "provider_id", providerID, "gateway_id", gatewayID, "error", err)
				undeploymentErrors = append(undeploymentErrors, fmt.Sprintf("gateway %s: failed to fetch deployments: %v", gatewayID, err))
				continue
			}

			// Find the deployed deployment and undeploy it
			found := false
			for _, deployment := range deployments {
				if deployment.Status != nil && *deployment.Status == models.DeploymentStatusDeployed {
					found = true
					if _, err := deploymentService.UndeployLLMProviderDeployment(ctx, providerID, deployment.DeploymentID.String(), gatewayID, ouID); err != nil {
						logger.GetLogger(ctx).Error("LLMProviderService.Delete: failed to undeploy from gateway", "ou_id", ouID, "provider_id", providerID, "gateway_id", gatewayID, "deployment_id", deployment.DeploymentID, "error", err)
						undeploymentErrors = append(undeploymentErrors, fmt.Sprintf("gateway %s: %v", gatewayID, err))
					} else {
						logger.GetLogger(ctx).Info("LLMProviderService.Delete: undeployed from gateway successfully", "ou_id", ouID, "provider_id", providerID, "gateway_id", gatewayID)
						successfulUndeployments++
					}
					break
				}
			}
			if !found {
				logger.GetLogger(ctx).Warn("LLMProviderService.Delete: no deployed deployment found for gateway", "ou_id", ouID, "provider_id", providerID, "gateway_id", gatewayID)
			}
		}

		logger.GetLogger(ctx).Info("LLMProviderService.Delete: undeployment results", "ou_id", ouID, "provider_id", providerID, "successful_undeployments", successfulUndeployments, "total_gateways", len(gatewayIDs), "error_count", len(undeploymentErrors))

		// If all undeployments failed, return error. Wrapped in a sentinel so the
		// controller can report an actionable 409 instead of flattening a
		// caller-fixable condition into an opaque 500.
		if len(undeploymentErrors) > 0 && successfulUndeployments == 0 {
			slog.Error("LLMProviderService.Delete: all undeployments failed", "ouID", ouID, "providerID", providerID, "errors", undeploymentErrors)
			return fmt.Errorf("%w: %d of %d gateways: %s", utils.ErrLLMProviderUndeployFailed,
				len(undeploymentErrors), len(gatewayIDs), strings.Join(undeploymentErrors, "; "))
		}

		// If some undeployments failed, log warning but continue with deletion
		if len(undeploymentErrors) > 0 {
			logger.GetLogger(ctx).Warn("LLMProviderService.Delete: some undeployments failed, continuing with deletion", "ou_id", ouID, "provider_id", providerID, "errors", undeploymentErrors)
		}
	}

	// Now delete the provider from database (cascade deletes mappings)
	logger.GetLogger(ctx).Info("LLMProviderService.Delete: deleting provider from database", "ou_id", ouID, "provider_id", providerID)
	if err := s.providerRepo.Delete(provider.UUID.String(), ouID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logger.GetLogger(ctx).Warn("LLMProviderService.Delete: provider not found", "ou_id", ouID, "provider_id", providerID)
			return utils.ErrLLMProviderNotFound
		}
		logger.GetLogger(ctx).Warn("LLMProviderService.Delete: failed to delete provider", "ou_id", ouID, "provider_id", providerID, "error", err)
		return fmt.Errorf("failed to delete provider: %w", err)
	}

	// No KV cleanup needed — encrypted value is stored in the DB and deleted with the provider record

	logger.GetLogger(ctx).Info("LLMProviderService.Delete: completed successfully", "ou_id", ouID, "provider_id", providerID)
	return nil
}

// UpdateAndSync updates an LLM provider and syncs its gateway deployments
func (s *LLMProviderService) UpdateAndSync(ctx context.Context, providerID, ouID string, updates *models.LLMProvider, gatewayIDs []string, deploymentService *LLMProviderDeploymentService) (*UpdateAndSyncResponse, error) {
	logger.GetLogger(ctx).Info("LLMProviderService.UpdateAndSync: starting", "provider_id", providerID, "ou_id", ouID, "gateway_count", len(gatewayIDs))

	// First, update the provider using the existing Update method
	updated, err := s.Update(ctx, providerID, ouID, updates)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProviderService.UpdateAndSync: failed to update provider", "provider_id", providerID, "ou_id", ouID, "error", err)
		return nil, err
	}

	logger.GetLogger(ctx).Info("LLMProviderService.UpdateAndSync: provider updated successfully", "provider_id", providerID, "provider_uuid", updated.UUID)

	// Parse UUIDs
	providerUUID, err := uuid.Parse(providerID)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProviderService.UpdateAndSync: invalid provider UUID", "provider_id", providerID, "error", err)
		return nil, fmt.Errorf("invalid provider UUID: %w", err)
	}

	// Convert gateway IDs to UUIDs and track invalid ones
	gatewayUUIDs := make([]uuid.UUID, 0, len(gatewayIDs))
	invalidGatewayResults := []DeploymentResult{}
	for _, gatewayID := range gatewayIDs {
		gatewayUUID, err := uuid.Parse(gatewayID)
		if err != nil {
			logger.GetLogger(ctx).Error("LLMProviderService.UpdateAndSync: invalid gateway UUID", "ou_id", ouID, "gateway_id", gatewayID, "error", err)
			invalidGatewayResults = append(invalidGatewayResults, DeploymentResult{
				GatewayID: gatewayID,
				Success:   false,
				Error:     fmt.Sprintf("invalid gateway UUID: %v", err),
			})
			continue
		}
		gatewayUUIDs = append(gatewayUUIDs, gatewayUUID)
	}

	// Return error if ALL gateway IDs are invalid
	if len(gatewayIDs) > 0 && len(gatewayUUIDs) == 0 {
		logger.GetLogger(ctx).Error("LLMProviderService.UpdateAndSync: all gateway UUIDs are invalid", "provider_id", providerID, "total_requested", len(gatewayIDs))
		return nil, fmt.Errorf("all %d gateway IDs are invalid", len(gatewayIDs))
	}

	currentGateways, err := deploymentService.deploymentRepo.GetDeployedGatewaysByProvider(providerUUID, ouID)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProviderService.UpdateAndSync: failed to get deployed gateways", "provider_id", providerID, "error", err)
		return nil, err
	}

	logger.GetLogger(ctx).Info("LLMProviderService.UpdateAndSync: current deployed gateways retrieved", "provider_id", providerID, "new_count", len(gatewayUUIDs), "old_count", len(currentGateways))

	// Determine which gateways to add and which to remove
	currentGatewayMap := make(map[string]bool)
	for _, gwID := range currentGateways {
		currentGatewayMap[gwID] = true
	}

	// Validate egress placement for every requested gateway before the deploy/undeploy loops
	// run below. placementAccumulator starts as the provider's current deployments and grows
	// with each newly-validated gateway, so two new gateways sharing an environment in the
	// same request are also caught, not just clashes against pre-existing deployments.
	//
	// This must hard-fail on the first placement error rather than skip-and-continue: the
	// deploy/undeploy loops below compute removals from newGatewayMap, so silently dropping
	// an invalid gateway from the desired set would shrink it and undeploy every
	// currently-working gateway that isn't also named in this request. Naming an invalid
	// gateway is caller error and must leave existing deployments untouched.
	placementAccumulator := append([]string{}, currentGateways...)
	for _, gatewayUUID := range gatewayUUIDs {
		gatewayID := gatewayUUID.String()
		if currentGatewayMap[gatewayID] {
			// Already deployed here: idempotent, DeployLLMProvider re-validates anyway.
			continue
		}
		gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
		if err != nil {
			// Gateway-not-found is left to the deploy call below, unchanged from before this
			// task: it already performs its own GetByUUID and records a deployment failure.
			// Skip placement validation for it here; there's nothing to validate.
			logger.GetLogger(ctx).Warn("LLMProviderService.UpdateAndSync: could not resolve gateway for placement check, deferring to deploy step", "provider_id", providerID, "gateway_id", gatewayID, "error", err)
			continue
		}
		if gateway == nil || gateway.OUID != ouID {
			// Foreign-org gateway: never inspect or echo it here; the deploy step below
			// enforces org ownership and records the per-gateway failure.
			logger.GetLogger(ctx).Warn("LLMProviderService.UpdateAndSync: gateway not found in organization, deferring to deploy step", "provider_id", providerID, "gateway_id", gatewayID)
			continue
		}
		if err := validateEgressPlacement(s.gatewayRepo, gateway, placementAccumulator); err != nil {
			logger.GetLogger(ctx).Warn("LLMProviderService.UpdateAndSync: gateway failed egress placement check", "provider_id", providerID, "gateway_id", gatewayID, "error", err)
			return nil, fmt.Errorf("%w: %w", utils.ErrInvalidInput, err)
		}
		placementAccumulator = append(placementAccumulator, gatewayID)
	}

	newGatewayMap := make(map[string]bool)
	for _, gw := range gatewayUUIDs {
		newGatewayMap[gw.String()] = true
	}

	// Deploy to newly added gateways and track results
	deploymentResults := make([]DeploymentResult, 0)
	deploymentResults = append(deploymentResults, invalidGatewayResults...)
	deploymentIndex := 1
	successfulDeployments := 0
	attemptedDeployments := 0

	for _, gatewayUUID := range gatewayUUIDs {
		gatewayID := gatewayUUID.String()
		if !currentGatewayMap[gatewayUUID.String()] {
			attemptedDeployments++
			logger.GetLogger(ctx).Info("LLMProviderService.UpdateAndSync: deploying to new gateway", "provider_id", providerID, "gateway_id", gatewayID)

			deploymentName := fmt.Sprintf("%s-deployment-%d", updated.Configuration.Name, deploymentIndex)
			deployReq := &models.DeployAPIRequest{
				Name:      deploymentName,
				Base:      "current",
				GatewayID: gatewayID,
				Metadata: map[string]interface{}{
					"auto_deployed": true,
					"sync_update":   true,
				},
			}

			if _, err := deploymentService.DeployLLMProvider(ctx, providerID, deployReq, ouID); err != nil {
				logger.GetLogger(ctx).Error("LLMProviderService.UpdateAndSync: failed to deploy to new gateway", "provider_id", providerID, "gateway_id", gatewayID, "error", err)
				deploymentResults = append(deploymentResults, DeploymentResult{
					GatewayID: gatewayID,
					Success:   false,
					Error:     err.Error(),
				})
			} else {
				logger.GetLogger(ctx).Info("LLMProviderService.UpdateAndSync: deployed to new gateway successfully", "provider_id", providerID, "gateway_id", gatewayID)
				successfulDeployments++
				deploymentResults = append(deploymentResults, DeploymentResult{
					GatewayID: gatewayID,
					Success:   true,
				})
			}
			deploymentIndex++
		} else {
			attemptedDeployments++
			logger.GetLogger(ctx).Info("LLMProviderService.UpdateAndSync: updating the current deployment", "provider_id", providerID, "gateway_id", gatewayID)
			currentDeployment, err := deploymentService.deploymentRepo.GetCurrentByGateway(providerID, gatewayID, ouID)
			if err != nil {
				deploymentResults = append(deploymentResults, DeploymentResult{
					GatewayID: gatewayID,
					Success:   false,
					Error:     err.Error(),
				})
			}

			deployReq := &models.DeployAPIRequest{
				Name: currentDeployment.Name,
				// Use "current" so the deployment YAML is regenerated from the latest provider
				// configuration (including updated policies). Using the old deployment UUID as Base
				// would copy the stale YAML content, missing any policy or config changes.
				Base:      "current",
				GatewayID: gatewayID,
				Metadata: map[string]interface{}{
					"auto_deployed": true,
					"sync_update":   true,
				},
			}

			if _, err := deploymentService.DeployLLMProvider(ctx, providerID, deployReq, ouID); err != nil {
				logger.GetLogger(ctx).Error("LLMProviderService.UpdateAndSync: failed to update deployment in gateway", "provider_id", providerID, "gateway_id", gatewayID, "error", err)
				deploymentResults = append(deploymentResults, DeploymentResult{
					GatewayID: gatewayID,
					Success:   false,
					Error:     err.Error(),
				})
			} else {
				logger.GetLogger(ctx).Info("LLMProviderService.UpdateAndSync: deployed to new gateway successfully", "provider_id", providerID, "gateway_id", gatewayID)
				successfulDeployments++
				deploymentResults = append(deploymentResults, DeploymentResult{
					GatewayID: gatewayID,
					Success:   true,
				})
			}
			deploymentIndex++
		}
	}

	// Fail if ALL new deployments failed
	if attemptedDeployments > 0 && successfulDeployments == 0 {
		logger.GetLogger(ctx).Error("LLMProviderService.UpdateAndSync: all new deployments failed", "provider_id", providerID, "attempted", attemptedDeployments)
		return nil, fmt.Errorf("all %d new gateway deployments failed", attemptedDeployments)
	}

	// Undeploy from removed gateways and track results
	undeploymentResults := make([]DeploymentResult, 0)
	attemptedUndeployments := 0
	successfulUndeployments := 0

	for _, gatewayID := range currentGateways {
		if !newGatewayMap[gatewayID] {
			attemptedUndeployments++
			logger.GetLogger(ctx).Info("LLMProviderService.UpdateAndSync: undeploying from removed gateway", "provider_id", providerID, "gateway_id", gatewayID)

			// Get current deployment for this gateway
			deployments, err := deploymentService.GetLLMProviderDeployments(providerID, ouID, &gatewayID, nil)
			if err != nil {
				logger.GetLogger(ctx).Error("LLMProviderService.UpdateAndSync: failed to get deployments for gateway", "provider_id", providerID, "gateway_id", gatewayID, "error", err)
				undeploymentResults = append(undeploymentResults, DeploymentResult{
					GatewayID: gatewayID,
					Success:   false,
					Error:     fmt.Sprintf("failed to fetch deployments: %v", err),
				})
				continue
			}

			// Find the deployed deployment and undeploy it
			found := false
			for _, deployment := range deployments {
				if deployment.Status != nil && *deployment.Status == models.DeploymentStatusDeployed {
					found = true
					if _, err := deploymentService.UndeployLLMProviderDeployment(ctx, providerID, deployment.DeploymentID.String(), gatewayID, ouID); err != nil {
						logger.GetLogger(ctx).Error("LLMProviderService.UpdateAndSync: failed to undeploy from removed gateway", "provider_id", providerID, "gateway_id", gatewayID, "deployment_id", deployment.DeploymentID, "error", err)
						undeploymentResults = append(undeploymentResults, DeploymentResult{
							GatewayID: gatewayID,
							Success:   false,
							Error:     err.Error(),
						})
					} else {
						logger.GetLogger(ctx).Info("LLMProviderService.UpdateAndSync: undeployed from removed gateway successfully", "provider_id", providerID, "gateway_id", gatewayID)
						successfulUndeployments++
						undeploymentResults = append(undeploymentResults, DeploymentResult{
							GatewayID: gatewayID,
							Success:   true,
						})
					}
					break
				}
			}
			if !found {
				logger.GetLogger(ctx).Warn("LLMProviderService.UpdateAndSync: no deployed deployment found for gateway", "provider_id", providerID, "gateway_id", gatewayID)
				undeploymentResults = append(undeploymentResults, DeploymentResult{
					GatewayID: gatewayID,
					Success:   false,
					Error:     "no deployed deployment found",
				})
			}
		}
	}

	logger.GetLogger(ctx).Info("LLMProviderService.UpdateAndSync: completed",
		"provider_id", providerID,
		"new_gateway_count", len(gatewayUUIDs),
		"previous_gateway_count", len(currentGateways),
		"successful_deployments", successfulDeployments,
		"attempted_deployments", attemptedDeployments,
		"successful_undeployments", successfulUndeployments,
		"attempted_undeployments", attemptedUndeployments)

	return &UpdateAndSyncResponse{
		Provider:      updated,
		Deployments:   deploymentResults,
		Undeployments: undeploymentResults,
	}, nil
}

// ListProxiesByProvider lists all LLM proxies for a provider
func (s *LLMProviderService) ListProxiesByProvider(providerID, ouID string, limit, offset int) ([]*models.LLMProxy, int, error) {
	if providerID == "" {
		return nil, 0, utils.ErrInvalidInput
	}

	// Get provider to get its UUID
	provider, err := s.resolveProvider(providerID, ouID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		return nil, 0, utils.ErrLLMProviderNotFound
	}

	// List proxies by provider UUID
	proxies, err := s.proxyRepo.ListByProvider(ouID, provider.UUID.String(), limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list proxies by provider: %w", err)
	}

	totalCount, err := s.proxyRepo.CountByProvider(ouID, provider.UUID.String())
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count proxies by provider: %w", err)
	}

	return proxies, totalCount, nil
}

// CreateAndDeploy creates an LLM provider and deploys it to the specified gateways
func (s *LLMProviderService) CreateAndDeploy(ctx context.Context, ouID, createdBy string, provider *models.LLMProvider, gatewayIDs []string, deploymentService *LLMProviderDeploymentService) (*CreateAndDeployResponse, error) {
	logger.GetLogger(ctx).Info("LLMProviderService.CreateAndDeploy: starting", "ou_id", ouID, "created_by", createdBy, "gateway_count", len(gatewayIDs))

	// Validate gateway UUIDs
	deploymentResults := make([]DeploymentResult, 0, len(gatewayIDs))
	validGatewayIDs := make([]string, 0, len(gatewayIDs))

	for _, gatewayID := range gatewayIDs {
		_, err := uuid.Parse(gatewayID)
		if err != nil {
			logger.GetLogger(ctx).Error("LLMProviderService.CreateAndDeploy: invalid gateway UUID", "ou_id", ouID, "gateway_id", gatewayID, "error", err)
			deploymentResults = append(deploymentResults, DeploymentResult{
				GatewayID: gatewayID,
				Success:   false,
				Error:     fmt.Sprintf("invalid gateway UUID: %v", err),
			})
			continue
		}

		gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
		if err != nil {
			logger.GetLogger(ctx).Error("LLMProviderService.CreateAndDeploy: no gateway found for provided gateway", "ou_id", ouID, "gateway_id", gatewayID, "error", err)
			deploymentResults = append(deploymentResults, DeploymentResult{
				GatewayID: gatewayID,
				Success:   false,
				Error:     fmt.Sprintf("Gateway not found: %v", err),
			})
			continue
		}
		if gateway == nil || gateway.OUID != ouID {
			// Foreign-org gateway: treat as not found without inspecting or echoing it.
			logger.GetLogger(ctx).Error("LLMProviderService.CreateAndDeploy: gateway not found in organization", "ou_id", ouID, "gateway_id", gatewayID)
			deploymentResults = append(deploymentResults, DeploymentResult{
				GatewayID: gatewayID,
				Success:   false,
				Error:     "Gateway not found",
			})
			continue
		}

		// existingDeployments is validGatewayIDs-so-far: the provider doesn't exist yet, so the
		// only clash to catch here is two gateways in this same request sharing an environment.
		//
		// Unlike malformed UUIDs / gateway-not-found above, a placement failure is a hard
		// error, not a per-gateway skip: naming an invalid gateway is caller error, and
		// nothing has been written yet (no provider, no deployment), so there is no partial
		// state to leave behind by failing the whole request now.
		if err := validateEgressPlacement(s.gatewayRepo, gateway, validGatewayIDs); err != nil {
			logger.GetLogger(ctx).Warn("LLMProviderService.CreateAndDeploy: gateway failed egress placement check", "ou_id", ouID, "gateway_id", gatewayID, "error", err)
			return nil, fmt.Errorf("%w: %w", utils.ErrInvalidInput, err)
		}

		validGatewayIDs = append(validGatewayIDs, gatewayID)
	}

	// Return error if ALL gateway IDs are invalid
	if len(gatewayIDs) > 0 && len(validGatewayIDs) == 0 {
		logger.GetLogger(ctx).Error("LLMProviderService.CreateAndDeploy: all gateway UUIDs are invalid", "ou_id", ouID, "total_requested", len(gatewayIDs))
		return nil, fmt.Errorf("all %d gateway IDs are invalid", len(gatewayIDs))
	}

	// Create the provider using the existing Create method
	created, err := s.Create(ctx, ouID, createdBy, provider)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProviderService.CreateAndDeploy: failed to create provider", "ou_id", ouID, "error", err)
		return nil, err
	}

	logger.GetLogger(ctx).Info("LLMProviderService.CreateAndDeploy: provider created successfully", "ou_id", ouID, "provider_uuid", created.UUID)

	// Deploy to each valid gateway and track results
	successfulDeployments := 0
	for i, gatewayID := range validGatewayIDs {
		logger.GetLogger(ctx).Info("LLMProviderService.CreateAndDeploy: deploying to gateway", "ou_id", ouID, "provider_uuid", created.UUID, "gateway_id", gatewayID, "index", i+1, "total", len(validGatewayIDs))

		// Generate deployment name: provider-name-gateway-index
		deploymentName := fmt.Sprintf("%s-deployment-%d", created.Configuration.Name, i+1)

		// Create deployment request
		deployReq := &models.DeployAPIRequest{
			Name:      deploymentName,
			Base:      "current", // Use current provider configuration
			GatewayID: gatewayID,
			Metadata: map[string]interface{}{
				"auto_deployed": true,
				"gateway_index": i + 1,
			},
		}

		// Deploy to gateway
		deployment, err := deploymentService.DeployLLMProvider(ctx, created.UUID.String(), deployReq, ouID)
		if err != nil {
			logger.GetLogger(ctx).Error("LLMProviderService.CreateAndDeploy: failed to deploy to gateway", "ou_id", ouID, "provider_uuid", created.UUID, "gateway_id", gatewayID, "error", err)
			deploymentResults = append(deploymentResults, DeploymentResult{
				GatewayID: gatewayID,
				Success:   false,
				Error:     err.Error(),
			})
			continue
		}

		logger.GetLogger(ctx).Info("LLMProviderService.CreateAndDeploy: deployed to gateway successfully", "ou_id", ouID, "provider_uuid", created.UUID, "gateway_id", gatewayID, "deployment_id", deployment.DeploymentID)
		successfulDeployments++
		deploymentResults = append(deploymentResults, DeploymentResult{
			GatewayID: gatewayID,
			Success:   true,
		})
	}

	// Fail if ALL deployments failed (but only if we had valid gateways to deploy to).
	// The provider row is rolled back first: leaving it behind meant the caller saw a
	// failure and got an undeployed provider anyway, invisible until the next list.
	if len(validGatewayIDs) > 0 && successfulDeployments == 0 {
		slog.Error("LLMProviderService.CreateAndDeploy: all deployments failed", "ouID", ouID, "providerUUID", created.UUID, "attempted", len(validGatewayIDs))
		failure := fmt.Sprintf("all %d gateway deployments failed: %s",
			len(validGatewayIDs), summarizeDeploymentFailures(deploymentResults))

		// The rollback error is carried as text rather than wrapped: the caller maps
		// the HTTP status off this error, and a Delete sentinel in the chain would let
		// an unrelated rollback sub-failure choose the status for a failed create.
		if rollbackErr := s.rollbackCreatedProvider(ctx, created, ouID, deploymentService); rollbackErr != nil {
			return nil, fmt.Errorf("%s (provider %q was created and could not be rolled back, delete it manually: %s)",
				failure, created.Configuration.Handle, rollbackErr.Error())
		}
		return nil, errors.New(failure)
	}

	logger.GetLogger(ctx).Info("LLMProviderService.CreateAndDeploy: completed", "ou_id", ouID, "provider_uuid", created.UUID, "successful_deployments", successfulDeployments, "total_attempted", len(validGatewayIDs))

	return &CreateAndDeployResponse{
		Provider:    created,
		Deployments: deploymentResults,
	}, nil
}

func (s *LLMProviderService) GetProviderGatewayMapping(ctx context.Context, providerId uuid.UUID, ouID string, deploymentService *LLMProviderDeploymentService) ([]string, error) {
	gws, err := deploymentService.deploymentRepo.GetDeployedGatewaysByProvider(providerId, ouID)
	if err != nil {
		logger.GetLogger(ctx).Warn("error while fetching deployed gateways for provider", "provider_id", providerId.String(), "error", err)
		return nil, err
	}
	return gws, nil
}

// UpdateCatalogStatus updates the catalog visibility status of an LLM provider
func (s *LLMProviderService) UpdateCatalogStatus(ctx context.Context, providerID, ouID string, inCatalog bool) (*models.LLMProvider, error) {
	logger.GetLogger(ctx).Info("LLMProviderService.UpdateCatalogStatus: starting", "provider_id", providerID, "ou_id", ouID, "in_catalog", inCatalog)

	// Validate UUIDs
	_, err := uuid.Parse(providerID)
	if err != nil {
		logger.GetLogger(ctx).Error("LLMProviderService.UpdateCatalogStatus: invalid provider UUID", "provider_id", providerID, "error", err)
		return nil, utils.ErrInvalidInput
	}

	// Start transaction
	tx := s.db.Begin()
	if tx.Error != nil {
		logger.GetLogger(ctx).Error("LLMProviderService.UpdateCatalogStatus: failed to begin transaction", "error", tx.Error)
		return nil, tx.Error
	}

	// Ensure transaction is rolled back on panic or error
	committed := false
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			logger.GetLogger(ctx).Error("LLMProviderService.UpdateCatalogStatus: panic recovered, rolling back", "panic", r)
			panic(r) // Re-panic after rollback
		}
		if !committed {
			tx.Rollback()
		}
	}()

	// Verify provider exists and belongs to org (within transaction)
	// Note: We use the non-transactional repo here since GetByUUID doesn't support tx parameter
	// This is acceptable as the critical update happens within the transaction
	provider, err := s.resolveProvider(providerID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logger.GetLogger(ctx).Error("LLMProviderService.UpdateCatalogStatus: provider not found", "provider_id", providerID, "ou_id", ouID)
			return nil, utils.ErrLLMProviderNotFound
		}
		logger.GetLogger(ctx).Warn("LLMProviderService.UpdateCatalogStatus: failed to get provider", "provider_id", providerID, "error", err)
		return nil, err
	}
	if provider == nil {
		logger.GetLogger(ctx).Warn("LLMProviderService.UpdateCatalogStatus: provider not found", "provider_id", providerID, "ou_id", ouID)
		return nil, utils.ErrLLMProviderNotFound
	}

	// Update artifact catalog status within transaction
	err = s.artifactRepo.UpdateCatalogStatus(tx, providerID, ouID, inCatalog)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProviderService.UpdateCatalogStatus: failed to update artifact catalog status", "provider_id", providerID, "error", err)
		return nil, err
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		logger.GetLogger(ctx).Warn("LLMProviderService.UpdateCatalogStatus: failed to commit transaction", "error", err)
		return nil, err
	}
	committed = true

	// Update InCatalog field to reflect the committed change
	provider.InCatalog = inCatalog

	logger.GetLogger(ctx).Info("LLMProviderService.UpdateCatalogStatus: completed successfully", "provider_id", providerID, "in_catalog", inCatalog)
	return provider, nil
}

// LLMProviderConsumer describes a single agent or monitor that uses a proxy under this provider.
type LLMProviderConsumer struct {
	ProxyID      string
	ProxyName    string
	ProjectName  string
	ConsumerType string // "agent" or "monitor"
	ConsumerName string
}

// ListConsumers returns all agents and monitors consuming any proxy under the given provider.
func (s *LLMProviderService) ListConsumers(ctx context.Context, providerID, ouID string) ([]LLMProviderConsumer, error) {
	if providerID == "" || ouID == "" {
		return nil, utils.ErrInvalidInput
	}

	provider, err := s.resolveProvider(providerID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrLLMProviderNotFound
		}
		return nil, fmt.Errorf("resolveProvider: %w", err)
	}

	// Fetch all proxies for this provider (no pagination — consumers is a small set)
	proxies, err := s.proxyRepo.ListByProvider(ouID, provider.UUID.String(), 1000, 0)
	if err != nil {
		return nil, fmt.Errorf("ListByProvider: %w", err)
	}
	if len(proxies) == 0 {
		return nil, nil
	}

	proxyUUIDs := make([]uuid.UUID, len(proxies))
	for i, p := range proxies {
		proxyUUIDs[i] = p.UUID
	}

	agentConsumers, err := s.agentMappingRepo.ListAgentConsumersByProxyUUIDs(ctx, proxyUUIDs)
	if err != nil {
		return nil, fmt.Errorf("ListAgentConsumersByProxyUUIDs: %w", err)
	}

	monitorConsumers, err := s.monitorMappingRepo.ListMonitorConsumersByProxyUUIDs(ctx, proxyUUIDs)
	if err != nil {
		return nil, fmt.Errorf("ListMonitorConsumersByProxyUUIDs: %w", err)
	}

	consumers := make([]LLMProviderConsumer, 0, len(agentConsumers)+len(monitorConsumers))
	for _, ac := range agentConsumers {
		consumers = append(consumers, LLMProviderConsumer{
			ProxyID:      ac.ProxyHandle,
			ProxyName:    ac.ProxyName,
			ProjectName:  ac.ProjectName,
			ConsumerType: "agent",
			ConsumerName: ac.AgentID,
		})
	}
	for _, mc := range monitorConsumers {
		consumers = append(consumers, LLMProviderConsumer{
			ProxyID:      mc.ProxyHandle,
			ProxyName:    mc.ProxyName,
			ProjectName:  mc.ProjectName,
			ConsumerType: "monitor",
			ConsumerName: mc.MonitorName,
		})
	}
	return consumers, nil
}
