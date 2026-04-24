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
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// nonK8sNameChar matches any character not valid in a Kubernetes resource name segment.
var nonK8sNameChar = regexp.MustCompile(`[^a-z0-9-]`)

// multiHyphenRe matches two or more consecutive hyphens.
var multiHyphenRe = regexp.MustCompile(`-{2,}`)

// sanitizeForK8sName converts a string to a valid Kubernetes resource name segment.
// It lowercases the input, replaces spaces and underscores with hyphens, strips
// remaining invalid characters, collapses consecutive hyphens, trims leading/trailing
// hyphens, and caps the result at 63 characters.
func sanitizeForK8sName(s string) string {
	s = strings.ToLower(s)
	s = strings.NewReplacer(" ", "-", "_", "-").Replace(s)
	s = nonK8sNameChar.ReplaceAllString(s, "")
	s = multiHyphenRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 63 {
		s = strings.TrimRight(s[:63], "-")
	}
	return s
}

// buildProxyURL constructs the proxy base URL from a gateway vhost and an optional context path.
func buildProxyURL(vhost string, contextPath *string) string {
	if contextPath != nil {
		return fmt.Sprintf("%s%s", vhost, *contextPath)
	}
	return vhost
}

// ProxySecretContext holds optional KV path fields for secret storage.
// Agent callers populate all fields for their deeper per-env/config path.
// Monitor callers leave all fields empty to use the org-scoped path.
type ProxySecretContext struct {
	ProjectName string
	AgentName   string
	EnvName     string
	ConfigName  string
}

// ProxyRollbackState tracks resources created during ProvisionProxy for cleanup on failure.
type ProxyRollbackState struct {
	ProxyHandle      string
	DeploymentID     uuid.UUID
	ProxyAPIKeyID    string
	ProviderAPIKeyID string
	ProviderUUID     string
	ProxySecretLoc   *secretmanagersvc.SecretLocation
}

// ProvisionProxyParams is the caller-provided input to ProvisionProxy.
// ProxyName must already be sanitized for Kubernetes (e.g. via sanitizeForK8sName).
// Deployment and API key names are derived from ProxyName by suffix substitution.
type ProvisionProxyParams struct {
	OrgName        string
	ProviderHandle string
	ProxyName      string // e.g. "my-monitor-openai-proxy"
	ProjectUUID    uuid.UUID
	Gateway        *models.Gateway
	Policies       models.LLMPolicies // nil/empty for monitors
	Description    string
	CreatedBy      string // defaults to models.UserRoleSystem when empty
	SecretCtx      ProxySecretContext
	// SkipKVSecret skips storing the proxy API key in OpenBao KV.
	// Set by callers (e.g. monitors) that manage their own composite secret.
	SkipKVSecret bool
}

// ProvisionedProxy is returned from a successful ProvisionProxy call.
type ProvisionedProxy struct {
	Proxy         *models.LLMProxy
	ProxyAPIKey   string // raw key — caller decides how/where to store it
	ProxyURL      string
	SecretRefName string // K8s SecretReference name (empty for non-K8s callers)
	RollbackState ProxyRollbackState
}

// LLMProxyProvisioner manages the shared LLM proxy lifecycle used by both
// agentConfigurationService and monitorManagerService.
type LLMProxyProvisioner struct {
	logger                    *slog.Logger
	llmProviderRepo           repositories.LLMProviderRepository
	gatewayRepo               repositories.GatewayRepository
	llmProxyService           *LLMProxyService
	llmProxyDeploymentService *LLMProxyDeploymentService
	llmProxyAPIKeyService     *LLMProxyAPIKeyService
	llmProviderAPIKeyService  *LLMProviderAPIKeyService
	secretClient              secretmanagersvc.SecretManagementClient
	encryptionKey             []byte
}

// NewLLMProxyProvisioner creates a new LLMProxyProvisioner.
func NewLLMProxyProvisioner(
	logger *slog.Logger,
	llmProviderRepo repositories.LLMProviderRepository,
	gatewayRepo repositories.GatewayRepository,
	llmProxyService *LLMProxyService,
	llmProxyDeploymentService *LLMProxyDeploymentService,
	llmProxyAPIKeyService *LLMProxyAPIKeyService,
	llmProviderAPIKeyService *LLMProviderAPIKeyService,
	secretClient secretmanagersvc.SecretManagementClient,
	encryptionKey []byte,
) *LLMProxyProvisioner {
	return &LLMProxyProvisioner{
		logger:                    logger,
		llmProviderRepo:           llmProviderRepo,
		gatewayRepo:               gatewayRepo,
		llmProxyService:           llmProxyService,
		llmProxyDeploymentService: llmProxyDeploymentService,
		llmProxyAPIKeyService:     llmProxyAPIKeyService,
		llmProviderAPIKeyService:  llmProviderAPIKeyService,
		secretClient:              secretClient,
		encryptionKey:             encryptionKey,
	}
}

// Accessors for caller-specific operations not covered by the shared lifecycle methods.

func (p *LLMProxyProvisioner) ProviderRepo() repositories.LLMProviderRepository {
	return p.llmProviderRepo
}

func (p *LLMProxyProvisioner) ProxyService() *LLMProxyService {
	return p.llmProxyService
}

func (p *LLMProxyProvisioner) ProxyDeploymentService() *LLMProxyDeploymentService {
	return p.llmProxyDeploymentService
}

func (p *LLMProxyProvisioner) SecretClient() secretmanagersvc.SecretManagementClient {
	return p.secretClient
}

// ResolveGateway selects an active gateway for the given environment, preferring AI gateways.
func (p *LLMProxyProvisioner) ResolveGateway(ctx context.Context, envUUID uuid.UUID, orgName string) (*models.Gateway, error) {
	envIDStr := envUUID.String()
	aiType := "ai"
	activeStatus := true

	gateways, err := p.gatewayRepo.ListWithFilters(repositories.GatewayFilterOptions{
		OrganizationID:    orgName,
		FunctionalityType: &aiType,
		Status:            &activeStatus,
		EnvironmentID:     &envIDStr,
		Limit:             1,
	})
	if err == nil && len(gateways) > 0 {
		return gateways[0], nil
	}

	gateways, err = p.gatewayRepo.ListWithFilters(repositories.GatewayFilterOptions{
		OrganizationID: orgName,
		Status:         &activeStatus,
		EnvironmentID:  &envIDStr,
		Limit:          1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find gateway: %w", err)
	}
	if len(gateways) == 0 {
		return nil, errors.New("no active gateway found for environment")
	}
	return gateways[0], nil
}

// ProvisionProxy runs the full proxy provisioning sequence:
//  1. Resolve provider
//  2. Build proxy config + upstream auth (if provider is secured)
//  3. Create proxy
//  4. Deploy proxy to gateway
//  5. Create proxy API key
//  6. Store proxy API key in KV
//
// On any failure the provisioner rolls back all resources it created before returning the error.
func (p *LLMProxyProvisioner) ProvisionProxy(ctx context.Context, params ProvisionProxyParams) (*ProvisionedProxy, error) {
	rb := ProxyRollbackState{}

	provider, err := p.llmProviderRepo.GetByHandle(params.ProviderHandle, params.OrgName)
	if err != nil {
		return nil, fmt.Errorf("provider %q not found: %w", params.ProviderHandle, err)
	}

	contextPath := fmt.Sprintf("/%s", uuid.New())
	enabled := true
	proxyConfig := &models.LLMProxy{
		Description: params.Description,
		ProjectUUID: params.ProjectUUID,
		Configuration: models.LLMProxyConfig{
			Name:     params.ProxyName,
			Version:  models.DefaultProxyVersion,
			Context:  &contextPath,
			Provider: provider.UUID.String(),
			Policies: params.Policies,
			Security: &models.SecurityConfig{
				Enabled: &enabled,
				APIKey: &models.APIKeySecurity{
					Enabled: &enabled,
					Key:     "API-Key",
					In:      "header",
				},
			},
		},
	}

	// Set up upstream auth if the provider requires an API key.
	if sec := provider.Configuration.Security; sec != nil && sec.Enabled != nil && *sec.Enabled {
		if akSec := sec.APIKey; akSec != nil && akSec.Enabled != nil && *akSec.Enabled {
			apiKey, err := p.llmProviderAPIKeyService.CreateAPIKey(ctx, params.OrgName, provider.UUID.String(), &models.CreateAPIKeyRequest{
				Name:        params.ProxyName,
				DisplayName: params.ProxyName,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to create provider API key: %w", err)
			}
			rb.ProviderAPIKeyID = apiKey.KeyID
			rb.ProviderUUID = provider.UUID.String()

			artifactHandle := ""
			if provider.Artifact != nil {
				artifactHandle = provider.Artifact.Handle
			}
			if artifactHandle == "" {
				p.RollbackProxy(ctx, rb, params.OrgName)
				return nil, fmt.Errorf("provider %s has no artifact handle", provider.UUID.String())
			}

			encrypted, err := utils.EncryptBytes([]byte(apiKey.APIKey), p.encryptionKey)
			if err != nil {
				p.RollbackProxy(ctx, rb, params.OrgName)
				return nil, fmt.Errorf("failed to encrypt provider API key: %w", err)
			}
			encoded := base64.StdEncoding.EncodeToString(encrypted)
			proxyConfig.Configuration.UpstreamAuth = &models.UpstreamAuth{
				Type:      utils.StrAsStrPointer(models.AuthTypeAPIKey),
				Header:    utils.StrAsStrPointer(akSec.Key),
				SecretRef: &encoded,
				Value:     nil,
			}
		}
	}

	createdBy := params.CreatedBy
	if createdBy == "" {
		createdBy = models.UserRoleSystem
	}
	proxy, err := p.llmProxyService.Create(params.OrgName, createdBy, proxyConfig)
	if err != nil {
		p.RollbackProxy(ctx, rb, params.OrgName)
		return nil, fmt.Errorf("failed to create proxy: %w", err)
	}
	rb.ProxyHandle = proxy.Handle

	baseName := strings.TrimSuffix(params.ProxyName, "-proxy")
	deployment, err := p.llmProxyDeploymentService.DeployLLMProxy(proxy.Handle, &models.DeployAPIRequest{
		Name:      baseName + "-deployment",
		Base:      "current",
		GatewayID: params.Gateway.UUID.String(),
	}, params.OrgName)
	if err != nil {
		p.RollbackProxy(ctx, rb, params.OrgName)
		return nil, fmt.Errorf("failed to deploy proxy: %w", err)
	}
	rb.DeploymentID = deployment.DeploymentID

	proxyAPIKey, err := p.llmProxyAPIKeyService.CreateAPIKey(ctx, params.OrgName, proxy.Handle, &models.CreateAPIKeyRequest{
		Name: baseName + "-key",
	})
	if err != nil {
		p.RollbackProxy(ctx, rb, params.OrgName)
		return nil, fmt.Errorf("failed to create proxy API key: %w", err)
	}
	rb.ProxyAPIKeyID = proxyAPIKey.KeyID

	var secretRefName string
	if !params.SkipKVSecret {
		proxySecretLoc := secretmanagersvc.SecretLocation{
			OrgName:         params.OrgName,
			ProjectName:     params.SecretCtx.ProjectName,
			AgentName:       params.SecretCtx.AgentName,
			EnvironmentName: params.SecretCtx.EnvName,
			ConfigName:      params.SecretCtx.ConfigName,
			EntityName:      proxy.Handle,
			SecretKey:       secretmanagersvc.SecretKeyAPIKey,
		}
		secretRefName, err = p.secretClient.CreateSecret(ctx, proxySecretLoc,
			map[string]string{secretmanagersvc.SecretKeyAPIKey: proxyAPIKey.APIKey})
		if err != nil {
			p.RollbackProxy(ctx, rb, params.OrgName)
			return nil, fmt.Errorf("failed to store proxy API key in KV: %w", err)
		}
		rb.ProxySecretLoc = &proxySecretLoc
	}

	proxyURL := buildProxyURL(params.Gateway.Vhost, proxy.Configuration.Context)

	p.logger.Info("Provisioned LLM proxy",
		"proxyName", params.ProxyName,
		"providerHandle", params.ProviderHandle,
		"proxyURL", proxyURL,
	)

	return &ProvisionedProxy{
		Proxy:         proxy,
		ProxyAPIKey:   proxyAPIKey.APIKey,
		ProxyURL:      proxyURL,
		SecretRefName: secretRefName,
		RollbackState: rb,
	}, nil
}

// RollbackProxy reverts all resources tracked in state. It is best-effort: each step is
// attempted independently and errors are logged rather than returned.
func (p *LLMProxyProvisioner) RollbackProxy(ctx context.Context, state ProxyRollbackState, orgName string) {
	p.logger.Warn("Rolling back LLM proxy resources", "proxyHandle", state.ProxyHandle)

	if state.ProxySecretLoc != nil {
		if err := p.secretClient.DeleteSecret(ctx, *state.ProxySecretLoc, ""); err != nil {
			kvPath, _ := state.ProxySecretLoc.KVPath()
			p.logger.Error("Failed to delete proxy secret during rollback", "kvPath", kvPath, "error", err)
		}
	}
	if state.ProxyAPIKeyID != "" && state.ProxyHandle != "" {
		if err := p.llmProxyAPIKeyService.RevokeAPIKey(ctx, orgName, state.ProxyHandle, state.ProxyAPIKeyID); err != nil {
			p.logger.Error("Failed to revoke proxy API key during rollback",
				"proxyHandle", state.ProxyHandle, "apiKeyID", state.ProxyAPIKeyID, "error", err)
		}
	}
	if state.ProxyHandle != "" && state.DeploymentID != uuid.Nil {
		if err := p.llmProxyDeploymentService.DeleteLLMProxyDeployment(state.ProxyHandle, state.DeploymentID.String(), orgName); err != nil {
			p.logger.Error("Failed to undeploy proxy during rollback",
				"proxyHandle", state.ProxyHandle, "deploymentID", state.DeploymentID, "error", err)
		}
	}
	if state.ProviderAPIKeyID != "" && state.ProviderUUID != "" {
		if err := p.llmProviderAPIKeyService.RevokeAPIKey(ctx, orgName, state.ProviderUUID, state.ProviderAPIKeyID); err != nil {
			p.logger.Error("Failed to revoke provider API key during rollback",
				"providerUUID", state.ProviderUUID, "apiKeyID", state.ProviderAPIKeyID, "error", err)
		}
	}
	if state.ProxyHandle != "" {
		if err := p.llmProxyService.Delete(state.ProxyHandle, orgName); err != nil {
			if !errors.Is(err, utils.ErrLLMProxyNotFound) {
				p.logger.Error("Failed to delete proxy during rollback", "proxyHandle", state.ProxyHandle, "error", err)
			}
		}
	}
}

// CleanupProxy tears down a deployed proxy and its associated secrets.
// It is used in delete/update flows where an existing proxy must be removed.
// secretCtx must match the context used when the proxy was provisioned so that KV paths resolve correctly.
func (p *LLMProxyProvisioner) CleanupProxy(ctx context.Context, proxy *models.LLMProxy, orgName string, secretCtx ProxySecretContext) error {
	proxyHandle := proxy.Configuration.Name
	baseName := strings.TrimSuffix(proxyHandle, "-proxy")

	// Revoke proxy API key (best-effort).
	if err := p.llmProxyAPIKeyService.RevokeAPIKey(ctx, orgName, proxyHandle, baseName+"-key"); err != nil {
		p.logger.Warn("Failed to revoke proxy API key during cleanup",
			"proxyHandle", proxyHandle, "error", err)
	}

	// Revoke provider API key if upstream auth was configured (best-effort).
	if proxy.Configuration.UpstreamAuth != nil {
		providerUUID := proxy.ProviderUUID.String()
		if err := p.llmProviderAPIKeyService.RevokeAPIKey(ctx, orgName, providerUUID, proxyHandle); err != nil {
			p.logger.Warn("Failed to revoke provider API key during cleanup",
				"providerUUID", providerUUID, "error", err)
		}
	}

	// Undeploy all proxy deployments.
	deployments, err := p.llmProxyDeploymentService.GetLLMProxyDeployments(proxyHandle, orgName, nil, nil)
	if err != nil {
		if !errors.Is(err, utils.ErrLLMProxyNotFound) {
			return fmt.Errorf("failed to get deployments for proxy %q: %w", proxyHandle, err)
		}
		p.logger.Info("Proxy already deleted, skipping deployment cleanup", "proxyHandle", proxyHandle)
	} else {
		for _, dep := range deployments {
			if _, err := p.llmProxyDeploymentService.UndeployLLMProxyDeployment(
				proxyHandle, dep.DeploymentID.String(), dep.GatewayUUID.String(), orgName,
			); err != nil {
				p.logger.Error("Failed to undeploy proxy during cleanup",
					"proxyHandle", proxyHandle, "deploymentID", dep.DeploymentID, "error", err)
			}
		}
	}

	// Delete proxy record.
	if err := p.llmProxyService.Delete(proxyHandle, orgName); err != nil {
		if !errors.Is(err, utils.ErrLLMProxyNotFound) {
			return fmt.Errorf("failed to delete proxy %q: %w", proxyHandle, err)
		}
	}

	// Delete proxy KV secret.
	proxySecretLoc := secretmanagersvc.SecretLocation{
		OrgName:         orgName,
		ProjectName:     secretCtx.ProjectName,
		AgentName:       secretCtx.AgentName,
		EnvironmentName: secretCtx.EnvName,
		ConfigName:      secretCtx.ConfigName,
		EntityName:      proxyHandle,
		SecretKey:       secretmanagersvc.SecretKeyAPIKey,
	}
	if err := p.secretClient.DeleteSecret(ctx, proxySecretLoc, ""); err != nil {
		p.logger.Warn("Failed to delete proxy secret from KV during cleanup",
			"proxyHandle", proxyHandle, "error", err)
	}

	// Provider upstream auth key is encrypted in the DB — deleted with the proxy record, no KV cleanup needed.

	return nil
}
