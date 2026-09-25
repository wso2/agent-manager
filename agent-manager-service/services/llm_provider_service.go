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
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

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

	// proxySyncLocks serialises dependent-proxy syncs per provider. Zero value is
	// usable, so it needs no wiring in the constructor.
	proxySyncLocks sync.Map
}

// lockProviderSync serialises syncs for one provider and returns the release func.
// Without it two overlapping edits race: the slower goroutine can read valid state,
// then write it after the faster one, leaving a proxy unsecured while its provider
// requires a key. Process-local — multiple replicas would need the same guarantee
// in the database.
func (s *LLMProviderService) lockProviderSync(providerUUID string) func() {
	v, _ := s.proxySyncLocks.LoadOrStore(providerUUID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
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

// validateProxyAPIKeyLocation adapts models.SecurityConfig.ValidateAPIKeyLocation — the
// single definition of which locations the gateway can enforce — to the ErrInvalidInput
// convention this package's callers expect, so a rejected location surfaces as a 400
// rather than a 500.
func validateProxyAPIKeyLocation(provider *models.LLMProvider) error {
	if provider == nil {
		return nil
	}
	if err := provider.Configuration.Security.ValidateAPIKeyLocation(); err != nil {
		return fmt.Errorf("%w: %w", utils.ErrInvalidInput, err)
	}
	return nil
}

// applyDefaultProxyAPIKeyHeader defaults a blank ingress API key name to the same
// value providerProxyAPIKeySecurity would resolve, so the provider's stored security
// config (and therefore the console's security tab, deployment YAML, and provisioned
// proxies) all agree on one name up front instead of each resolving a blank value
// separately. Only the name defaults; the location is left as the caller set it, having
// already been held to header-only by validateProxyAPIKeyLocation.
func applyDefaultProxyAPIKeyHeader(provider *models.LLMProvider) {
	sec := provider.Configuration.Security
	if !isAPIKeyAuthEnabled(sec) {
		return
	}
	if strings.TrimSpace(sec.APIKey.Key) == "" {
		name, _ := providerProxyAPIKeySecurity(provider)
		sec.APIKey.Key = name
	}
}

// Create creates a new LLM provider
func (s *LLMProviderService) Create(ctx context.Context, ouID, createdBy string, provider *models.LLMProvider) (*models.LLMProvider, error) {
	slog.Info("LLMProviderService.Create: starting", "ouID", ouID, "createdBy", createdBy)

	if provider == nil {
		slog.Error("LLMProviderService.Create: provider is nil", "ouID", ouID)
		return nil, utils.ErrInvalidInput
	}

	// Extract handle, name, and version from configuration
	// Note: handle is not in Configuration, so we use name as handle
	name := provider.Configuration.Name
	version := provider.Configuration.Version

	// Use name as handle (artifact identifier)
	handle := provider.Configuration.Handle

	slog.Info("LLMProviderService.Create: extracted configuration", "ouID", ouID, "handle", handle, "name", name, "version", version)

	if handle == "" || name == "" || version == "" {
		slog.Error("LLMProviderService.Create: missing required fields", "ouID", ouID, "handle", handle, "name", name, "version", version)
		return nil, utils.ErrInvalidInput
	}

	// Fail fast if a provider with this handle already exists, before touching KV.
	if _, err := s.providerRepo.GetByHandle(handle, ouID); err == nil {
		slog.Warn("LLMProviderService.Create: provider already exists", "ouID", ouID, "handle", handle)
		return nil, utils.ErrLLMProviderExists
	}

	// Validate template exists
	template := provider.Configuration.Template
	if template == "" {
		slog.Error("LLMProviderService.Create: template not specified", "ouID", ouID, "handle", handle)
		return nil, utils.ErrInvalidInput
	}

	if err := validateProxyAPIKeyLocation(provider); err != nil {
		slog.Warn("LLMProviderService.Create: unsupported api key location", "ouID", ouID, "handle", handle, "error", err)
		return nil, err
	}

	// Set default values
	provider.CreatedBy = createdBy
	if provider.Configuration.Context == nil {
		defaultContext := "/"
		provider.Configuration.Context = &defaultContext
	}
	applyDefaultProxyAPIKeyHeader(provider)

	slog.Info("LLMProviderService.Create: set default values", "ouID", ouID, "handle", handle, "context", *provider.Configuration.Context)

	// Serialize model providers to ModelList
	if len(provider.ModelProviders) > 0 {
		slog.Info("LLMProviderService.Create: serializing model providers", "ouID", ouID, "handle", handle, "count", len(provider.ModelProviders))
		modelListBytes, err := json.Marshal(provider.ModelProviders)
		if err != nil {
			slog.Error("LLMProviderService.Create: failed to serialize model providers", "ouID", ouID, "handle", handle, "error", err)
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
			slog.Error("LLMProviderService.Create: failed to encrypt upstream key",
				"ouID", ouID, "handle", handle, "error", err)
			return nil, fmt.Errorf("failed to encrypt upstream API key: %w", err)
		}
		encoded := base64.StdEncoding.EncodeToString(encrypted)

		// Replace plaintext with encrypted reference
		provider.Configuration.Upstream.Main.Auth.SecretRef = &encoded
		provider.Configuration.Upstream.Main.Auth.Value = nil

		slog.Info("LLMProviderService.Create: encrypted upstream key",
			"ouID", ouID, "handle", handle)
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
			slog.Warn("LLMProviderService.Create: provider already exists (unique constraint)", "ouID", ouID, "handle", handle)
			return nil, utils.ErrLLMProviderExists
		}
		// Return template not found error directly
		if errors.Is(err, utils.ErrLLMProviderTemplateNotFound) {
			return nil, err
		}
		slog.Error("LLMProviderService.Create: failed to create provider", "ouID", ouID, "handle", handle, "error", err)
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	slog.Info("LLMProviderService.Create: provider created, fetching details", "ouID", ouID, "handle", handle, "uuid", provider.UUID)

	// Fetch created provider by UUID
	created, err := s.providerRepo.GetByUUID(provider.UUID.String(), ouID)
	if err != nil {
		slog.Error("LLMProviderService.Create: failed to fetch created provider", "ouID", ouID, "uuid", provider.UUID, "error", err)
		return nil, fmt.Errorf("failed to fetch created provider: %w", err)
	}

	// Parse model providers from ModelList
	if created.ModelList != "" {
		slog.Info("LLMProviderService.Create: parsing model providers from ModelList", "ouID", ouID, "handle", handle)
		if err := json.Unmarshal([]byte(created.ModelList), &created.ModelProviders); err != nil {
			slog.Error("LLMProviderService.Create: failed to parse model providers", "ouID", ouID, "handle", handle, "error", err)
			return nil, fmt.Errorf("failed to parse model providers: %w", err)
		}
	}

	slog.Info("LLMProviderService.Create: completed successfully", "ouID", ouID, "handle", handle, "providerUUID", created.UUID)
	return created, nil
}

// List lists all LLM providers for an organization
func (s *LLMProviderService) List(ouID string, limit, offset int) ([]*models.LLMProvider, int, error) {
	slog.Info("LLMProviderService.List: starting", "ouID", ouID, "limit", limit, "offset", offset)

	providers, err := s.providerRepo.List(ouID, limit, offset)
	if err != nil {
		slog.Error("LLMProviderService.List: failed to list providers", "ouID", ouID, "error", err)
		return nil, 0, fmt.Errorf("failed to list providers: %w", err)
	}

	slog.Info("LLMProviderService.List: providers retrieved from repository", "ouID", ouID, "count", len(providers))

	// Parse model providers for each provider
	for i, p := range providers {
		if p.ModelList != "" {
			if err := json.Unmarshal([]byte(p.ModelList), &p.ModelProviders); err != nil {
				slog.Error("LLMProviderService.List: failed to parse model providers", "ouID", ouID, "providerIndex", i, "providerUUID", p.UUID, "error", err)
				return nil, 0, fmt.Errorf("failed to parse model providers: %w", err)
			}
		}
	}

	totalCount, err := s.providerRepo.Count(ouID)
	if err != nil {
		slog.Error("LLMProviderService.List: failed to count providers", "ouID", ouID, "error", err)
		return nil, 0, fmt.Errorf("failed to count providers: %w", err)
	}

	slog.Info("LLMProviderService.List: completed successfully", "ouID", ouID, "count", len(providers), "total", totalCount)
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
func (s *LLMProviderService) Get(providerID, ouID string) (*models.LLMProvider, error) {
	slog.Info("LLMProviderService.Get: starting", "ouID", ouID, "providerID", providerID)

	if providerID == "" {
		slog.Error("LLMProviderService.Get: providerID is empty", "ouID", ouID)
		return nil, utils.ErrInvalidInput
	}

	provider, err := s.resolveProvider(providerID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProviderService.Get: provider not found", "ouID", ouID, "providerID", providerID)
			return nil, utils.ErrLLMProviderNotFound
		}
		slog.Error("LLMProviderService.Get: failed to get provider", "ouID", ouID, "providerID", providerID, "error", err)
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		slog.Warn("LLMProviderService.Get: provider not found", "ouID", ouID, "providerID", providerID)
		return nil, utils.ErrLLMProviderNotFound
	}

	// Parse model providers from ModelList
	if provider.ModelList != "" {
		slog.Info("LLMProviderService.Get: parsing model providers", "ouID", ouID, "providerID", providerID, "providerUUID", provider.UUID)
		if err := json.Unmarshal([]byte(provider.ModelList), &provider.ModelProviders); err != nil {
			slog.Error("LLMProviderService.Get: failed to parse model providers", "ouID", ouID, "providerID", providerID, "error", err)
			return nil, fmt.Errorf("failed to parse model providers: %w", err)
		}
	}

	slog.Info("LLMProviderService.Get: completed successfully", "ouID", ouID, "providerID", providerID, "providerUUID", provider.UUID)
	return provider, nil
}

// Update updates an existing LLM provider
func (s *LLMProviderService) Update(ctx context.Context, providerID, ouID string, updates *models.LLMProvider) (*models.LLMProvider, error) {
	slog.Info("LLMProviderService.Update: starting", "ouID", ouID, "providerID", providerID)

	if providerID == "" || updates == nil {
		slog.Error("LLMProviderService.Update: invalid input", "ouID", ouID, "providerID", providerID, "updatesIsNil", updates == nil)
		return nil, utils.ErrInvalidInput
	}

	// Validate template exists (check both built-in and user templates)
	template := updates.Configuration.Template
	if template != "" {
		slog.Info("LLMProviderService.Update: validating template", "ouID", ouID, "providerID", providerID, "template", template)
		templateExists := s.templateStore.Exists(template)
		if !templateExists {
			// Check user templates in database
			userTemplateExists, err := s.templateRepo.Exists(template, ouID)
			if err != nil {
				slog.Error("LLMProviderService.Update: failed to validate user template", "ouID", ouID, "providerID", providerID, "template", template, "error", err)
				return nil, fmt.Errorf("failed to validate template: %w", err)
			}
			if !userTemplateExists {
				slog.Warn("LLMProviderService.Update: template not found", "ouID", ouID, "providerID", providerID, "template", template)
				return nil, utils.ErrLLMProviderTemplateNotFound
			}
		}
		// Set template handle in updates
		updates.TemplateHandle = template
	}

	// Serialize model providers to ModelList
	if len(updates.ModelProviders) > 0 {
		slog.Info("LLMProviderService.Update: serializing model providers", "ouID", ouID, "providerID", providerID, "count", len(updates.ModelProviders))
		modelListBytes, err := json.Marshal(updates.ModelProviders)
		if err != nil {
			slog.Error("LLMProviderService.Update: failed to serialize model providers", "ouID", ouID, "providerID", providerID, "error", err)
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
			slog.Error("LLMProviderService.Update: failed to encrypt upstream key",
				"ouID", ouID, "providerID", providerID, "error", err)
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
			slog.Warn("LLMProviderService.Update: provider not found", "ouID", ouID, "providerID", providerID)
			return nil, utils.ErrLLMProviderNotFound
		}
		slog.Error("LLMProviderService.Update: failed to read provider before update", "ouID", ouID, "providerID", providerID, "error", err)
		return nil, fmt.Errorf("failed to read provider before update: %w", err)
	}
	if existing == nil {
		slog.Warn("LLMProviderService.Update: provider not found", "ouID", ouID, "providerID", providerID)
		return nil, utils.ErrLLMProviderNotFound
	}
	apiKeyAuthWasEnabled := isAPIKeyAuthEnabled(existing.Configuration.Security)

	// Only vet the location when the update touches security — a legacy row stored with
	// an unsupported one would otherwise be uneditable.
	if ProviderAuthHeadersChanged(existing, updates) {
		if err := validateProxyAPIKeyLocation(updates); err != nil {
			slog.Warn("LLMProviderService.Update: unsupported api key location", "ouID", ouID, "providerID", providerID, "error", err)
			return nil, err
		}
	}
	applyDefaultProxyAPIKeyHeader(updates)

	// Update provider
	slog.Info("LLMProviderService.Update: updating provider in database", "ouID", ouID, "providerID", providerID)
	if err := s.providerRepo.Update(ctx, updates, providerID, ouID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProviderService.Update: provider not found", "ouID", ouID, "providerID", providerID)
			return nil, utils.ErrLLMProviderNotFound
		}
		slog.Error("LLMProviderService.Update: failed to update provider", "ouID", ouID, "providerID", providerID, "error", err)
		return nil, fmt.Errorf("failed to update provider: %w", err)
	}

	// Fetch updated provider
	slog.Info("LLMProviderService.Update: fetching updated provider", "ouID", ouID, "providerID", providerID)
	updated, err := s.resolveProvider(providerID, ouID)
	if err != nil {
		slog.Error("LLMProviderService.Update: failed to fetch updated provider", "ouID", ouID, "providerID", providerID, "error", err)
		return nil, fmt.Errorf("failed to fetch updated provider: %w", err)
	}
	if updated == nil {
		slog.Warn("LLMProviderService.Update: updated provider not found", "ouID", ouID, "providerID", providerID)
		return nil, utils.ErrLLMProviderNotFound
	}

	// Parse model providers from ModelList
	if updated.ModelList != "" {
		slog.Info("LLMProviderService.Update: parsing model providers", "ouID", ouID, "providerID", providerID)
		if err := json.Unmarshal([]byte(updated.ModelList), &updated.ModelProviders); err != nil {
			slog.Error("LLMProviderService.Update: failed to parse model providers", "ouID", ouID, "providerID", providerID, "error", err)
			return nil, fmt.Errorf("failed to parse model providers: %w", err)
		}
	}

	// If API key authentication was just turned off, revoke all user-managed API keys for
	// this provider. Best-effort: log and continue so a revoke failure doesn't fail the update.
	if apiKeyAuthWasEnabled && !isAPIKeyAuthEnabled(updated.Configuration.Security) && s.apiKeyService != nil {
		if err := s.apiKeyService.RevokeAllUserManagedKeys(ctx, ouID, providerID); err != nil {
			slog.Warn("LLMProviderService.Update: failed to revoke user-managed API keys after disabling API key security",
				"ouID", ouID, "providerID", providerID, "error", err)
		}
	}

	slog.Info("LLMProviderService.Update: completed successfully", "ouID", ouID, "providerID", providerID, "providerUUID", updated.UUID)
	return updated, nil
}

// Delete deletes an LLM provider after undeploying from all gateways
func (s *LLMProviderService) Delete(ctx context.Context, providerID, ouID string, deploymentService *LLMProviderDeploymentService) error {
	slog.Info("LLMProviderService.Delete: starting", "ouID", ouID, "providerID", providerID)

	if providerID == "" {
		slog.Error("LLMProviderService.Delete: providerID is empty", "ouID", ouID)
		return utils.ErrInvalidInput
	}

	// Verify provider exists
	provider, err := s.resolveProvider(providerID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProviderService.Delete: provider not found", "ouID", ouID, "providerID", providerID)
			return utils.ErrLLMProviderNotFound
		}
		slog.Error("LLMProviderService.Delete: failed to get provider", "ouID", ouID, "providerID", providerID, "error", err)
		return fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		slog.Warn("LLMProviderService.Delete: provider not found", "ouID", ouID, "providerID", providerID)
		return utils.ErrLLMProviderNotFound
	}

	// Get all deployed gateways for this provider. providerID may be a handle, so
	// use the UUID resolved above rather than re-parsing the raw identifier.
	providerUUID := provider.UUID

	// Atomically claim the provider for deletion before any gateway I/O or the
	// associated-proxies check. This single UPDATE is the sole source of truth for
	// "a delete is in flight": LLMProxyService.Create locks and rechecks this same
	// status inside its own insert transaction, so a proxy created concurrently
	// either lands before this claim (caught by HasAssociatedProxies just below) or
	// is rejected by Create once the claim is visible — there is no gap where a
	// proxy can slip in unseen by both sides.
	marked, err := s.providerRepo.MarkDeleting(providerUUID)
	if err != nil {
		slog.Error("LLMProviderService.Delete: failed to mark provider deleting", "ouID", ouID, "providerID", providerID, "error", err)
		return fmt.Errorf("failed to mark provider deleting: %w", err)
	}
	if !marked {
		slog.Warn("LLMProviderService.Delete: provider already has a delete in progress", "ouID", ouID, "providerID", providerID)
		return utils.ErrLLMProviderDeleteInProgress
	}
	// Cleared on any early return so a failed/aborted delete leaves the provider
	// usable again; deleteCommitted suppresses this once providerRepo.Delete below
	// removes the row (clearing a nonexistent row is a harmless no-op, but skipping
	// it avoids a pointless query on the common success path).
	deleteCommitted := false
	defer func() {
		if !deleteCommitted {
			if clearErr := s.providerRepo.ClearDeleting(providerUUID); clearErr != nil {
				slog.Error("LLMProviderService.Delete: failed to clear deleting flag after aborted delete", "ouID", ouID, "providerID", providerID, "error", clearErr)
			}
		}
	}()

	// Reject before touching any gateway: undeploying is not transactional with the
	// DB delete, so a rejected delete must not leave the provider undeployed. Runs
	// after the claim above so a proxy inserted just before the claim is still seen.
	hasProxies, err := s.providerRepo.HasAssociatedProxies(ctx, providerUUID)
	if err != nil {
		slog.Error("LLMProviderService.Delete: failed to check associated proxies", "ouID", ouID, "providerID", providerID, "error", err)
		return fmt.Errorf("failed to check associated proxies: %w", err)
	}
	if hasProxies {
		slog.Warn("LLMProviderService.Delete: provider has associated proxies", "ouID", ouID, "providerID", providerID)
		return utils.ErrLLMProviderHasProxies
	}

	// Resolve all cleanup targets before any irreversible gateway or database changes.
	deletionGatewayIDs, err := deploymentService.GatewayIDsForProviderDeletion(ctx, providerUUID, ouID)
	if err != nil {
		return fmt.Errorf("resolve provider deletion gateways: %w", err)
	}

	deployedGatewayIDs, err := deploymentService.deploymentRepo.GetDeployedGatewaysByProvider(providerUUID, ouID)
	if err != nil {
		slog.Error("LLMProviderService.Delete: failed to get deployed gateways", "ouID", ouID, "providerID", providerID, "error", err)
		return fmt.Errorf("failed to get deployed gateways: %w", err)
	}

	slog.Info("LLMProviderService.Delete: found deployed gateways", "ouID", ouID, "providerID", providerID, "gatewayCount", len(deployedGatewayIDs))

	// Undeploy from all gateways before deleting. Once any undeployment succeeds,
	// the workflow must finish with a context independent of request cancellation;
	// otherwise the provider can remain in the database after its gateway state has
	// already changed.
	successfulUndeployments := 0
	if len(deployedGatewayIDs) > 0 {
		undeploymentErrors := []string{}

		for _, gatewayID := range deployedGatewayIDs {
			slog.Info("LLMProviderService.Delete: undeploying from gateway", "ouID", ouID, "providerID", providerID, "gatewayID", gatewayID)

			// Get current deployment for this gateway
			deployments, err := deploymentService.GetLLMProviderDeployments(providerID, ouID, &gatewayID, nil)
			if err != nil {
				slog.Error("LLMProviderService.Delete: failed to get deployments for gateway", "ouID", ouID, "providerID", providerID, "gatewayID", gatewayID, "error", err)
				undeploymentErrors = append(undeploymentErrors, fmt.Sprintf("gateway %s: failed to fetch deployments: %v", gatewayID, err))
				continue
			}

			// Find the deployed deployment and undeploy it
			found := false
			for _, deployment := range deployments {
				if deployment.Status != nil && *deployment.Status == models.DeploymentStatusDeployed {
					found = true
					if _, err := deploymentService.UndeployLLMProviderDeployment(providerID, deployment.DeploymentID.String(), gatewayID, ouID); err != nil {
						slog.Error("LLMProviderService.Delete: failed to undeploy from gateway", "ouID", ouID, "providerID", providerID, "gatewayID", gatewayID, "deploymentID", deployment.DeploymentID, "error", err)
						undeploymentErrors = append(undeploymentErrors, fmt.Sprintf("gateway %s: %v", gatewayID, err))
					} else {
						slog.Info("LLMProviderService.Delete: undeployed from gateway successfully", "ouID", ouID, "providerID", providerID, "gatewayID", gatewayID)
						successfulUndeployments++
					}
					break
				}
			}
			if !found {
				slog.Warn("LLMProviderService.Delete: no deployed deployment found for gateway", "ouID", ouID, "providerID", providerID, "gatewayID", gatewayID)
			}
		}

		slog.Info("LLMProviderService.Delete: undeployment results", "ouID", ouID, "providerID", providerID, "successfulUndeployments", successfulUndeployments, "totalGateways", len(deployedGatewayIDs), "errorCount", len(undeploymentErrors))

		// If all undeployments failed, return error. Wrapped in a sentinel so the
		// controller can report an actionable 409 instead of flattening a
		// caller-fixable condition into an opaque 500.
		if len(undeploymentErrors) > 0 && successfulUndeployments == 0 {
			slog.Error("LLMProviderService.Delete: all undeployments failed", "ouID", ouID, "providerID", providerID, "errors", undeploymentErrors)
			return fmt.Errorf("%w: %d of %d gateways: %s", utils.ErrLLMProviderUndeployFailed,
				len(undeploymentErrors), len(deployedGatewayIDs), strings.Join(undeploymentErrors, "; "))
		}

		// If some undeployments failed, log warning but continue with deletion
		if len(undeploymentErrors) > 0 {
			slog.Warn("LLMProviderService.Delete: some undeployments failed, continuing with deletion", "ouID", ouID, "providerID", providerID, "errors", undeploymentErrors)
		}
	}

	deleteCtx := ctx
	deleteCancel := func() {}
	if successfulUndeployments > 0 {
		deleteCtx, deleteCancel = context.WithTimeout(context.WithoutCancel(ctx), gatewayDeletionTimeout)
	}
	defer deleteCancel()

	// Now delete the provider from database (cascade deletes mappings)
	slog.Info("LLMProviderService.Delete: deleting provider from database", "ouID", ouID, "providerID", providerID)
	if err := s.providerRepo.DeleteCtx(deleteCtx, provider.UUID.String(), ouID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProviderService.Delete: provider not found", "ouID", ouID, "providerID", providerID)
			return utils.ErrLLMProviderNotFound
		}
		slog.Error("LLMProviderService.Delete: failed to delete provider", "ouID", ouID, "providerID", providerID, "error", err)
		return fmt.Errorf("failed to delete provider: %w", err)
	}
	deleteCommitted = true

	// No KV cleanup needed — encrypted value is stored in the DB and deleted with the provider record

	// The undeploy above only flips the gateway's desired state — it deliberately
	// preserves the config, its keys and its policies. This is the event that makes
	// the gateway actually drop the config; without it every deleted provider leaves
	// its policy chains in the gateway's xDS snapshot forever. Sent only after the
	// delete is committed, and best-effort: a committed delete must not be failed
	// because a gateway is unreachable.
	cleanupCtx := deleteCtx
	cleanupCancel := func() {}
	if successfulUndeployments == 0 {
		cleanupCtx, cleanupCancel = context.WithTimeout(context.WithoutCancel(ctx), gatewayDeletionTimeout)
	}
	defer cleanupCancel()
	deploymentService.BroadcastLLMProviderDeletion(cleanupCtx, provider.UUID.String(), ouID, deletionGatewayIDs)

	slog.Info("LLMProviderService.Delete: completed successfully", "ouID", ouID, "providerID", providerID)
	return nil
}

// UpdateAndSync updates an LLM provider and syncs its gateway deployments
func (s *LLMProviderService) UpdateAndSync(ctx context.Context, providerID, ouID string, updates *models.LLMProvider, gatewayIDs []string, deploymentService *LLMProviderDeploymentService) (*UpdateAndSyncResponse, error) {
	slog.Info("LLMProviderService.UpdateAndSync: starting", "providerID", providerID, "ouID", ouID, "gatewayCount", len(gatewayIDs))

	// First, update the provider using the existing Update method
	updated, err := s.Update(ctx, providerID, ouID, updates)
	if err != nil {
		slog.Error("LLMProviderService.UpdateAndSync: failed to update provider", "providerID", providerID, "ouID", ouID, "error", err)
		return nil, err
	}

	slog.Info("LLMProviderService.UpdateAndSync: provider updated successfully", "providerID", providerID, "providerUUID", updated.UUID)

	// Parse UUIDs
	providerUUID, err := uuid.Parse(providerID)
	if err != nil {
		slog.Error("LLMProviderService.UpdateAndSync: invalid provider UUID", "providerID", providerID, "error", err)
		return nil, fmt.Errorf("invalid provider UUID: %w", err)
	}

	// Convert gateway IDs to UUIDs and track invalid ones
	gatewayUUIDs := make([]uuid.UUID, 0, len(gatewayIDs))
	invalidGatewayResults := []DeploymentResult{}
	for _, gatewayID := range gatewayIDs {
		gatewayUUID, err := uuid.Parse(gatewayID)
		if err != nil {
			slog.Error("LLMProviderService.UpdateAndSync: invalid gateway UUID", "ouID", ouID, "gatewayID", gatewayID, "error", err)
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
		slog.Error("LLMProviderService.UpdateAndSync: all gateway UUIDs are invalid", "providerID", providerID, "totalRequested", len(gatewayIDs))
		return nil, fmt.Errorf("all %d gateway IDs are invalid", len(gatewayIDs))
	}

	currentGateways, err := deploymentService.deploymentRepo.GetDeployedGatewaysByProvider(providerUUID, ouID)
	if err != nil {
		slog.Error("LLMProviderService.UpdateAndSync: failed to get deployed gateways", "providerID", providerID, "error", err)
		return nil, err
	}

	slog.Info("LLMProviderService.UpdateAndSync: current deployed gateways retrieved", "providerID", providerID, "newCount", len(gatewayUUIDs), "oldCount", len(currentGateways))

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
			slog.Warn("LLMProviderService.UpdateAndSync: could not resolve gateway for placement check, deferring to deploy step", "providerID", providerID, "gatewayID", gatewayID, "error", err)
			continue
		}
		if gateway == nil || gateway.OUID != ouID {
			// Foreign-org gateway: never inspect or echo it here; the deploy step below
			// enforces org ownership and records the per-gateway failure.
			slog.Warn("LLMProviderService.UpdateAndSync: gateway not found in organization, deferring to deploy step", "providerID", providerID, "gatewayID", gatewayID)
			continue
		}
		if err := validateEgressPlacement(s.gatewayRepo, gateway, placementAccumulator); err != nil {
			slog.Error("LLMProviderService.UpdateAndSync: gateway failed egress placement check", "providerID", providerID, "gatewayID", gatewayID, "error", err)
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
			slog.Info("LLMProviderService.UpdateAndSync: deploying to new gateway", "providerID", providerID, "gatewayID", gatewayID)

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

			if _, err := deploymentService.DeployLLMProvider(providerID, deployReq, ouID); err != nil {
				slog.Error("LLMProviderService.UpdateAndSync: failed to deploy to new gateway", "providerID", providerID, "gatewayID", gatewayID, "error", err)
				deploymentResults = append(deploymentResults, DeploymentResult{
					GatewayID: gatewayID,
					Success:   false,
					Error:     err.Error(),
				})
			} else {
				slog.Info("LLMProviderService.UpdateAndSync: deployed to new gateway successfully", "providerID", providerID, "gatewayID", gatewayID)
				successfulDeployments++
				deploymentResults = append(deploymentResults, DeploymentResult{
					GatewayID: gatewayID,
					Success:   true,
				})
			}
			deploymentIndex++
		} else {
			attemptedDeployments++
			slog.Info("LLMProviderService.UpdateAndSync: updating the current deployment", "providerID", providerID, "gatewayID", gatewayID)
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

			if _, err := deploymentService.DeployLLMProvider(providerID, deployReq, ouID); err != nil {
				slog.Error("LLMProviderService.UpdateAndSync: failed to update deployment in gateway", "providerID", providerID, "gatewayID", gatewayID, "error", err)
				deploymentResults = append(deploymentResults, DeploymentResult{
					GatewayID: gatewayID,
					Success:   false,
					Error:     err.Error(),
				})
			} else {
				slog.Info("LLMProviderService.UpdateAndSync: deployed to new gateway successfully", "providerID", providerID, "gatewayID", gatewayID)
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
		slog.Error("LLMProviderService.UpdateAndSync: all new deployments failed", "providerID", providerID, "attempted", attemptedDeployments)
		return nil, fmt.Errorf("all %d new gateway deployments failed", attemptedDeployments)
	}

	// Undeploy from removed gateways and track results
	undeploymentResults := make([]DeploymentResult, 0)
	attemptedUndeployments := 0
	successfulUndeployments := 0

	for _, gatewayID := range currentGateways {
		if !newGatewayMap[gatewayID] {
			attemptedUndeployments++
			slog.Info("LLMProviderService.UpdateAndSync: undeploying from removed gateway", "providerID", providerID, "gatewayID", gatewayID)

			// Get current deployment for this gateway
			deployments, err := deploymentService.GetLLMProviderDeployments(providerID, ouID, &gatewayID, nil)
			if err != nil {
				slog.Error("LLMProviderService.UpdateAndSync: failed to get deployments for gateway", "providerID", providerID, "gatewayID", gatewayID, "error", err)
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
					if _, err := deploymentService.UndeployLLMProviderDeployment(providerID, deployment.DeploymentID.String(), gatewayID, ouID); err != nil {
						slog.Error("LLMProviderService.UpdateAndSync: failed to undeploy from removed gateway", "providerID", providerID, "gatewayID", gatewayID, "deploymentID", deployment.DeploymentID, "error", err)
						undeploymentResults = append(undeploymentResults, DeploymentResult{
							GatewayID: gatewayID,
							Success:   false,
							Error:     err.Error(),
						})
					} else {
						slog.Info("LLMProviderService.UpdateAndSync: undeployed from removed gateway successfully", "providerID", providerID, "gatewayID", gatewayID)
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
				slog.Warn("LLMProviderService.UpdateAndSync: no deployed deployment found for gateway", "providerID", providerID, "gatewayID", gatewayID)
				undeploymentResults = append(undeploymentResults, DeploymentResult{
					GatewayID: gatewayID,
					Success:   false,
					Error:     "no deployed deployment found",
				})
			}
		}
	}

	slog.Info("LLMProviderService.UpdateAndSync: completed",
		"providerID", providerID,
		"newGatewayCount", len(gatewayUUIDs),
		"previousGatewayCount", len(currentGateways),
		"successfulDeployments", successfulDeployments,
		"attemptedDeployments", attemptedDeployments,
		"successfulUndeployments", successfulUndeployments,
		"attemptedUndeployments", attemptedUndeployments)

	return &UpdateAndSyncResponse{
		Provider:      updated,
		Deployments:   deploymentResults,
		Undeployments: undeploymentResults,
	}, nil
}

// proxyAuthSyncPageSize bounds each page of dependent proxies the sync walks.
const proxyAuthSyncPageSize = 100

// providerUpstreamAPIKeyHeader is the name the provider itself carries its api-key
// credential under, blank when it names no api-key security at all. A provider that
// enables api-key auth without naming a key is deployed with the same default
// providerProxyAPIKeySecurity resolves (see llm_deployment_service), so the upstream hop
// has to resolve it the same way: leaving it blank would forward the credential under no
// name at all, to a gateway that is checking for the default.
func providerUpstreamAPIKeyHeader(provider *models.LLMProvider) string {
	if provider == nil {
		return ""
	}
	sec := provider.Configuration.Security
	if sec == nil || sec.APIKey == nil {
		return ""
	}
	// An unnamed key on a provider that never turned api-key auth on names nothing to
	// forward, and must not be rewritten to a default the provider never asked for.
	if strings.TrimSpace(sec.APIKey.Key) == "" && !isAPIKeyAuthEnabled(sec) {
		return ""
	}
	name, _ := providerProxyAPIKeySecurity(provider)
	return name
}

// providerUpstreamAPIKeyAuth resolves the header a freshly provisioned proxy must forward
// the provider's credential under, refusing a location models.UpstreamAuth cannot express
// — it can only name a header, so such a proxy would send the key where the provider's
// gateway never looks and every call through it would fail to authenticate. Failing
// provisioning surfaces that limit instead of shipping a broken proxy.
//
// It shares one rule with the provider write path, so a location refused at save is
// refused here too, on the legacy rows that were stored before that rule existed.
func providerUpstreamAPIKeyAuth(provider *models.LLMProvider) (string, error) {
	if err := validateProxyAPIKeyLocation(provider); err != nil {
		return "", fmt.Errorf("provider %q: %w", provider.Configuration.Handle, err)
	}
	return providerUpstreamAPIKeyHeader(provider), nil
}

// ProviderAuthHeadersChanged reports whether an update actually changed a header (or
// its location) a dependent proxy would need to match, so a caller can skip triggering
// a sync for an edit that left provider security untouched — every other field update
// should not redeploy the fleet. A nil provider on either side is treated as changed,
// since there is nothing to compare against.
func ProviderAuthHeadersChanged(existing, updated *models.LLMProvider) bool {
	if existing == nil || updated == nil {
		return true
	}
	if existing.Configuration.Security.RequiresAPIKey() != updated.Configuration.Security.RequiresAPIKey() {
		return true
	}
	existingName, existingIn := providerProxyAPIKeySecurity(existing)
	updatedName, updatedIn := providerProxyAPIKeySecurity(updated)
	return existingName != updatedName || existingIn != updatedIn ||
		providerUpstreamAPIKeyHeader(existing) != providerUpstreamAPIKeyHeader(updated)
}

// SyncDependentProxyAuthHeaders rewrites the API-key header on every proxy provisioned in
// front of the given provider, so the header name an admin configures on the provider is
// the one their agents authenticate with. Two headers move together: the one agents send
// to the proxy, and the one the proxy forwards to the provider — leaving either behind
// breaks that hop.
//
// A proxy already carrying both names is skipped, so a provider edit that leaves security
// alone redeploys nothing. It is best-effort: a proxy that fails is logged and the rest
// continue, since stopping partway strands more of the fleet than finishing does. The
// returned error covers only a failure to enumerate dependents.
func (s *LLMProviderService) SyncDependentProxyAuthHeaders(
	ctx context.Context,
	provider *models.LLMProvider,
	ouID string,
	proxyService *LLMProxyService,
	proxyDeploymentService *LLMProxyDeploymentService,
) error {
	if provider == nil || proxyService == nil || proxyDeploymentService == nil {
		return utils.ErrInvalidInput
	}

	// Serialise before reading: the re-read below only helps if no other sync for this
	// provider can interleave its writes with ours. Holding the lock across the whole
	// walk means the sync that runs last both reads and writes last, so the proxies end
	// up matching whichever edit committed last.
	defer s.lockProviderSync(provider.UUID.String())()

	// Re-read rather than trust the caller's snapshot: this runs detached in its own
	// goroutine, so a second edit can commit and start its own sync before this one runs.
	current, err := s.providerRepo.GetByUUID(provider.UUID.String(), ouID)
	if err != nil {
		return fmt.Errorf("failed to refetch provider %s before syncing dependent proxies: %w", provider.UUID.String(), err)
	}
	provider = current

	ingressName, ingressIn := providerProxyAPIKeySecurity(provider)
	ingressRequired := provider.Configuration.Security.RequiresAPIKey()
	upstreamHeader := providerUpstreamAPIKeyHeader(provider)

	providerUUID := provider.UUID.String()
	synced, failed := 0, 0

	for offset := 0; ; offset += proxyAuthSyncPageSize {
		proxies, err := s.proxyRepo.ListByProvider(ouID, providerUUID, proxyAuthSyncPageSize, offset)
		if err != nil {
			return fmt.Errorf("failed to list proxies for provider %s: %w", providerUUID, err)
		}

		for _, proxy := range proxies {
			changed, syncErr := s.syncProxyAuthHeader(
				ctx, proxy, ouID, provider, upstreamHeader, proxyService, proxyDeploymentService)
			switch {
			case syncErr != nil:
				failed++
				slog.Error("LLMProviderService.SyncDependentProxyAuthHeaders: proxy sync failed",
					"providerUUID", providerUUID, "proxyHandle", proxy.Handle, "error", syncErr)
			case changed:
				synced++
			}
		}

		if len(proxies) < proxyAuthSyncPageSize {
			break
		}
	}

	slog.Info("LLMProviderService.SyncDependentProxyAuthHeaders: finished",
		"providerUUID", providerUUID, "ingressName", ingressName, "ingressIn", ingressIn,
		"ingressRequiresAPIKey", ingressRequired,
		"syncedCount", synced, "failedCount", failed)
	return nil
}

// proxyAuthHeadersStale reports which of a proxy's two hops differ from what its provider
// now implies, so an edit that leaves security alone redeploys nothing.
func proxyAuthHeadersStale(proxy *models.LLMProxy, ingress *models.SecurityConfig, upstreamHeader string) (bool, bool) {
	return proxyIngressStale(proxy, ingress),
		proxyUpstreamAuthAction(proxy, upstreamHeader) != upstreamAuthUnchanged
}

// proxyIngressStale compares the requirement first, then name and location. Requirement
// has to come first: a disabled config resolves to the same default name as an enabled
// one, so comparing names alone couldn't see auth being turned off.
func proxyIngressStale(proxy *models.LLMProxy, ingress *models.SecurityConfig) bool {
	// A gateway with no recorded deployment of the stored config has nothing queued to
	// replay, so it stays on the previous auth policy until another sync redeploys it.
	// The stored config already matches the target, hence this cannot be inferred from
	// the comparison below.
	if len(proxy.Configuration.AuthRolloutPendingGateways) > 0 {
		return true
	}
	current := proxy.Configuration.Security
	if current.RequiresAPIKey() != ingress.RequiresAPIKey() {
		return true
	}
	if !ingress.RequiresAPIKey() {
		return false
	}
	currentName, currentIn := current.APIKeyNameAndLocation(models.DefaultLLMProxyAPIKeyHeader)
	targetName, targetIn := ingress.APIKeyNameAndLocation(models.DefaultLLMProxyAPIKeyHeader)
	return currentName != targetName || currentIn != targetIn
}

// upstreamAuthAction is what the proxy's upstream hop needs to match its provider.
type upstreamAuthAction int

const (
	// upstreamAuthUnchanged: the hop already matches.
	upstreamAuthUnchanged upstreamAuthAction = iota
	// upstreamAuthRename: a credential is present but forwarded under the wrong name.
	upstreamAuthRename
	// upstreamAuthProvision: the provider requires a credential the proxy does not
	// carry at all, so one has to be minted before the hop can authenticate.
	upstreamAuthProvision
	// upstreamAuthClear: the provider no longer requires a credential, so the proxy
	// must stop forwarding one.
	upstreamAuthClear
)

// proxyUpstreamAuthAction decides what the upstream hop needs. Renaming alone isn't
// enough: a proxy provisioned while its provider was unsecured has no UpstreamAuth at
// all, so enabling auth later left it calling the provider anonymously.
func proxyUpstreamAuthAction(proxy *models.LLMProxy, upstreamHeader string) upstreamAuthAction {
	upstream := proxy.Configuration.UpstreamAuth
	hasCredential := upstream != nil && (upstream.SecretRef != nil || upstream.Value != nil)

	if upstreamHeader == "" {
		if upstream != nil && upstream.Header != nil && strings.TrimSpace(*upstream.Header) != "" {
			return upstreamAuthClear
		}
		return upstreamAuthUnchanged
	}
	if !hasCredential {
		return upstreamAuthProvision
	}
	if upstream.Header == nil || *upstream.Header != upstreamHeader {
		return upstreamAuthRename
	}
	return upstreamAuthUnchanged
}

// nextProxyUpstreamAuth builds the upstream auth block. A rename reuses the stored
// credential; a provision mints a fresh one. Clearing is the caller's business. Nothing
// is written here, so a failure leaves the proxy as it was.
func (s *LLMProviderService) nextProxyUpstreamAuth(
	ctx context.Context,
	proxy *models.LLMProxy,
	ouID string,
	provider *models.LLMProvider,
	upstreamHeader string,
	action upstreamAuthAction,
) (*models.UpstreamAuth, error) {
	switch action {
	case upstreamAuthRename:
		renamed := *proxy.Configuration.UpstreamAuth
		renamed.Header = utils.StrAsStrPointer(upstreamHeader)
		return &renamed, nil

	case upstreamAuthProvision:
		if s.apiKeyService == nil {
			return nil, fmt.Errorf(
				"proxy %s needs an upstream credential for provider %s but no api key service is configured",
				proxy.Handle, provider.Configuration.Handle)
		}
		keyName := proxy.Configuration.Name
		apiKey, err := s.apiKeyService.CreateAPIKey(ctx, ouID, provider.UUID.String(), &models.CreateAPIKeyRequest{
			Name:        keyName,
			DisplayName: keyName,
			Purpose:     models.APIKeyPurposeConsoleManaged,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to mint upstream api key for proxy %s: %w", proxy.Handle, err)
		}
		encrypted, err := utils.EncryptBytes([]byte(apiKey.APIKey), s.encryptionKey)
		if err != nil {
			if revokeErr := s.apiKeyService.RevokeAPIKey(ctx, ouID, provider.UUID.String(), keyName); revokeErr != nil {
				slog.Error("nextProxyUpstreamAuth: failed to revoke api key after encryption failure",
					"proxyHandle", proxy.Handle, "providerUUID", provider.UUID.String(), "error", revokeErr)
			}
			return nil, fmt.Errorf("failed to encrypt upstream api key for proxy %s: %w", proxy.Handle, err)
		}
		encoded := base64.StdEncoding.EncodeToString(encrypted)
		return &models.UpstreamAuth{
			Type:      utils.StrAsStrPointer(models.AuthTypeAPIKey),
			Header:    utils.StrAsStrPointer(upstreamHeader),
			SecretRef: &encoded,
		}, nil
	}
	return nil, fmt.Errorf(
		"proxy %s: upstream auth action %d does not build a credential", proxy.Handle, action)
}

// syncProxyAuthHeader brings one proxy's ingress and upstream auth in line with its
// provider's and redeploys it, reporting whether anything actually changed. The ingress
// target is rebuilt per proxy so each carries its own block rather than sharing one.
func (s *LLMProviderService) syncProxyAuthHeader(
	ctx context.Context,
	proxy *models.LLMProxy,
	ouID string,
	provider *models.LLMProvider,
	upstreamHeader string,
	proxyService *LLMProxyService,
	proxyDeploymentService *LLMProxyDeploymentService,
) (bool, error) {
	ingress := newProxyIngressSecurity(provider)
	ingressStale := proxyIngressStale(proxy, ingress)
	upstreamAction := proxyUpstreamAuthAction(proxy, upstreamHeader)
	priorPending := proxy.Configuration.AuthRolloutPendingGateways
	if !ingressStale && upstreamAction == upstreamAuthUnchanged {
		return false, nil
	}

	// Store moves first, since a redeploy regenerates gateway config from it; prior
	// values are kept so a failed redeploy can roll back. The whole security block is
	// swapped, not its fields — a proxy changing requirement may have no api-key block.
	var priorSecurity *models.SecurityConfig
	var priorUpstreamAuth *models.UpstreamAuth
	if ingressStale {
		priorSecurity = proxy.Configuration.Security
		proxy.Configuration.Security = ingress
	}
	// A key minted below has no owner until the proxy row referencing it is stored, so
	// every path that abandons this sync has to hand it back.
	mintedKeyName := ""
	revokeMintedKey := func() {
		if mintedKeyName == "" || s.apiKeyService == nil {
			return
		}
		if err := s.apiKeyService.RevokeAPIKey(ctx, ouID, provider.UUID.String(), mintedKeyName); err != nil {
			slog.Error("syncProxyAuthHeader: failed to revoke orphaned upstream key",
				"proxyHandle", proxy.Handle, "keyName", mintedKeyName, "error", err)
			return
		}
		mintedKeyName = ""
	}

	upstreamChanged := upstreamAction != upstreamAuthUnchanged
	if upstreamChanged {
		priorUpstreamAuth = proxy.Configuration.UpstreamAuth
		if upstreamAction == upstreamAuthClear {
			// The provider asks for no credential, so the proxy must stop sending one.
			// The key itself is left minted: revoking it belongs with the proxy's own
			// lifecycle, and a revoke failure here would strand the proxy mid-sync.
			proxy.Configuration.UpstreamAuth = nil
		} else {
			next, err := s.nextProxyUpstreamAuth(ctx, proxy, ouID, provider, upstreamHeader, upstreamAction)
			if err != nil {
				// Nothing has been written yet, so the proxy is untouched. Put the
				// ingress block back in memory so a caller reusing this struct isn't
				// left with a half-applied config.
				if ingressStale {
					proxy.Configuration.Security = priorSecurity
				}
				return false, err
			}
			if upstreamAction == upstreamAuthProvision {
				mintedKeyName = proxy.Configuration.Name
			}
			proxy.Configuration.UpstreamAuth = next
		}
	}

	if _, err := proxyService.Update(proxy.Handle, ouID, proxy); err != nil {
		revokeMintedKey()
		return false, fmt.Errorf("failed to update proxy %s: %w", proxy.Handle, err)
	}

	restoreStoredHeaders := func() {
		if ingressStale {
			proxy.Configuration.Security = priorSecurity
		}
		if upstreamChanged {
			proxy.Configuration.UpstreamAuth = priorUpstreamAuth
		}
		proxy.Configuration.AuthRolloutPendingGateways = priorPending
		if _, err := proxyService.Update(proxy.Handle, ouID, proxy); err != nil {
			slog.Error("syncProxyAuthHeader: failed to restore headers after a failed redeploy",
				"proxyHandle", proxy.Handle, "error", err)
			// The row may still reference the minted key, so leave it alone.
			return
		}
		revokeMintedKey()
	}

	deployments, err := proxyDeploymentService.GetLLMProxyDeployments(proxy.Handle, ouID, nil, nil)
	if err != nil {
		restoreStoredHeaders()
		return false, fmt.Errorf("failed to list deployments for proxy %s: %w", proxy.Handle, err)
	}

	redeployed := 0
	var stragglers []error
	var straggleGateways []string
	for _, deployment := range deployments {
		if deployment.Status == nil || *deployment.Status != models.DeploymentStatusDeployed {
			continue
		}
		if _, err := proxyDeploymentService.DeployLLMProxy(proxy.Handle, &models.DeployAPIRequest{
			Name:      deployment.Name,
			Base:      "current",
			GatewayID: deployment.GatewayUUID.String(),
		}, ouID); err != nil {
			// Nothing took the new config yet, so putting the store back leaves the
			// proxy exactly as it was.
			if redeployed == 0 && len(stragglers) == 0 {
				restoreStoredHeaders()
				return false, fmt.Errorf("failed to redeploy proxy %s on gateway %s: %w",
					proxy.Handle, deployment.GatewayUUID, err)
			}
			// A gateway has already accepted the new config, so rolling back would
			// strand it. Carry on so the rest converge; the errors name the stragglers.
			stragglers = append(stragglers, fmt.Errorf("gateway %s: %w", deployment.GatewayUUID, err))
			straggleGateways = append(straggleGateways, deployment.GatewayUUID.String())
			continue
		}
		redeployed++
	}

	// Record the outcome so the stored row stops claiming a convergence it doesn't have.
	// Without this a partial rollout reads as fully synced and no later sync retries the
	// gateways still on the old policy.
	if !slices.Equal(priorPending, straggleGateways) {
		// Non-nil even when empty: nil reads as "not specified" and would preserve the
		// stored list instead of clearing it after a rollout that fully succeeded.
		proxy.Configuration.AuthRolloutPendingGateways = append([]string{}, straggleGateways...)
		if _, err := proxyService.Update(proxy.Handle, ouID, proxy); err != nil {
			slog.Error("syncProxyAuthHeader: failed to record rollout outcome",
				"proxyHandle", proxy.Handle, "staleGateways", strings.Join(straggleGateways, ","),
				"error", err)
		}
	}

	if len(stragglers) > 0 {
		storedName, storedIn := ingress.APIKeyNameAndLocation(models.DefaultLLMProxyAPIKeyHeader)
		slog.Error("syncProxyAuthHeader: proxy left serving different auth config per gateway",
			"proxyHandle", proxy.Handle, "staleGateways", strings.Join(straggleGateways, ","),
			"redeployedCount", redeployed, "storedHeader", storedName, "storedIn", storedIn,
			"storedRequiresAPIKey", ingress.RequiresAPIKey())
		return true, fmt.Errorf("proxy %s: %d of %d gateways left on the previous auth config: %w",
			proxy.Handle, len(stragglers), redeployed+len(stragglers), errors.Join(stragglers...))
	}

	return true, nil
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
	slog.Info("LLMProviderService.CreateAndDeploy: starting", "ouID", ouID, "createdBy", createdBy, "gatewayCount", len(gatewayIDs))

	// Validate gateway UUIDs
	deploymentResults := make([]DeploymentResult, 0, len(gatewayIDs))
	validGatewayIDs := make([]string, 0, len(gatewayIDs))

	for _, gatewayID := range gatewayIDs {
		_, err := uuid.Parse(gatewayID)
		if err != nil {
			slog.Error("LLMProviderService.CreateAndDeploy: invalid gateway UUID", "ouID", ouID, "gatewayID", gatewayID, "error", err)
			deploymentResults = append(deploymentResults, DeploymentResult{
				GatewayID: gatewayID,
				Success:   false,
				Error:     fmt.Sprintf("invalid gateway UUID: %v", err),
			})
			continue
		}

		gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
		if err != nil {
			slog.Error("LLMProviderService.CreateAndDeploy: no gateway found for provided gateway", "ouID", ouID, "gatewayID", gatewayID, "error", err)
			deploymentResults = append(deploymentResults, DeploymentResult{
				GatewayID: gatewayID,
				Success:   false,
				Error:     fmt.Sprintf("Gateway not found: %v", err),
			})
			continue
		}
		if gateway == nil || gateway.OUID != ouID {
			// Foreign-org gateway: treat as not found without inspecting or echoing it.
			slog.Error("LLMProviderService.CreateAndDeploy: gateway not found in organization", "ouID", ouID, "gatewayID", gatewayID)
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
			slog.Error("LLMProviderService.CreateAndDeploy: gateway failed egress placement check", "ouID", ouID, "gatewayID", gatewayID, "error", err)
			return nil, fmt.Errorf("%w: %w", utils.ErrInvalidInput, err)
		}

		validGatewayIDs = append(validGatewayIDs, gatewayID)
	}

	// Return error if ALL gateway IDs are invalid
	if len(gatewayIDs) > 0 && len(validGatewayIDs) == 0 {
		slog.Error("LLMProviderService.CreateAndDeploy: all gateway UUIDs are invalid", "ouID", ouID, "totalRequested", len(gatewayIDs))
		return nil, fmt.Errorf("all %d gateway IDs are invalid", len(gatewayIDs))
	}

	// Create the provider using the existing Create method
	created, err := s.Create(ctx, ouID, createdBy, provider)
	if err != nil {
		slog.Error("LLMProviderService.CreateAndDeploy: failed to create provider", "ouID", ouID, "error", err)
		return nil, err
	}

	slog.Info("LLMProviderService.CreateAndDeploy: provider created successfully", "ouID", ouID, "providerUUID", created.UUID)

	// Deploy to each valid gateway and track results
	successfulDeployments := 0
	for i, gatewayID := range validGatewayIDs {
		slog.Info("LLMProviderService.CreateAndDeploy: deploying to gateway", "ouID", ouID, "providerUUID", created.UUID, "gatewayID", gatewayID, "index", i+1, "total", len(validGatewayIDs))

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
		deployment, err := deploymentService.DeployLLMProvider(created.UUID.String(), deployReq, ouID)
		if err != nil {
			slog.Error("LLMProviderService.CreateAndDeploy: failed to deploy to gateway", "ouID", ouID, "providerUUID", created.UUID, "gatewayID", gatewayID, "error", err)
			deploymentResults = append(deploymentResults, DeploymentResult{
				GatewayID: gatewayID,
				Success:   false,
				Error:     err.Error(),
			})
			continue
		}

		slog.Info("LLMProviderService.CreateAndDeploy: deployed to gateway successfully", "ouID", ouID, "providerUUID", created.UUID, "gatewayID", gatewayID, "deploymentID", deployment.DeploymentID)
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

	slog.Info("LLMProviderService.CreateAndDeploy: completed", "ouID", ouID, "providerUUID", created.UUID, "successfulDeployments", successfulDeployments, "totalAttempted", len(validGatewayIDs))

	return &CreateAndDeployResponse{
		Provider:    created,
		Deployments: deploymentResults,
	}, nil
}

func (s *LLMProviderService) GetProviderGatewayMapping(providerId uuid.UUID, ouID string, deploymentService *LLMProviderDeploymentService) ([]string, error) {
	gws, err := deploymentService.deploymentRepo.GetDeployedGatewaysByProvider(providerId, ouID)
	if err != nil {
		slog.Error("error while fetching deployed gateways for provider", "providerID", providerId.String(), "error", err)
		return nil, err
	}
	return gws, nil
}

// UpdateCatalogStatus updates the catalog visibility status of an LLM provider
func (s *LLMProviderService) UpdateCatalogStatus(providerID, ouID string, inCatalog bool) (*models.LLMProvider, error) {
	slog.Info("LLMProviderService.UpdateCatalogStatus: starting", "providerID", providerID, "ouID", ouID, "inCatalog", inCatalog)

	// Validate UUIDs
	_, err := uuid.Parse(providerID)
	if err != nil {
		slog.Error("LLMProviderService.UpdateCatalogStatus: invalid provider UUID", "providerID", providerID, "error", err)
		return nil, utils.ErrInvalidInput
	}

	// Start transaction
	tx := s.db.Begin()
	if tx.Error != nil {
		slog.Error("LLMProviderService.UpdateCatalogStatus: failed to begin transaction", "error", tx.Error)
		return nil, tx.Error
	}

	// Ensure transaction is rolled back on panic or error
	committed := false
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			slog.Error("LLMProviderService.UpdateCatalogStatus: panic recovered, rolling back", "panic", r)
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
			slog.Error("LLMProviderService.UpdateCatalogStatus: provider not found", "providerID", providerID, "ouID", ouID)
			return nil, utils.ErrLLMProviderNotFound
		}
		slog.Error("LLMProviderService.UpdateCatalogStatus: failed to get provider", "providerID", providerID, "error", err)
		return nil, err
	}
	if provider == nil {
		slog.Warn("LLMProviderService.UpdateCatalogStatus: provider not found", "providerID", providerID, "ouID", ouID)
		return nil, utils.ErrLLMProviderNotFound
	}

	// Update artifact catalog status within transaction
	err = s.artifactRepo.UpdateCatalogStatus(tx, providerID, ouID, inCatalog)
	if err != nil {
		slog.Error("LLMProviderService.UpdateCatalogStatus: failed to update artifact catalog status", "providerID", providerID, "error", err)
		return nil, err
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		slog.Error("LLMProviderService.UpdateCatalogStatus: failed to commit transaction", "error", err)
		return nil, err
	}
	committed = true

	// Update InCatalog field to reflect the committed change
	provider.InCatalog = inCatalog

	slog.Info("LLMProviderService.UpdateCatalogStatus: completed successfully", "providerID", providerID, "inCatalog", inCatalog)
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
