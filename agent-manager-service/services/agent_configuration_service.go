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
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// AgentConfigurationService interface defines agent configuration business logic
type AgentConfigurationService interface {
	Create(ctx context.Context, ouID, projectName, agentID string,
		req models.CreateAgentModelConfigRequest, createdBy string) (*models.AgentModelConfigResponse, error)
	ValidateProvidersInCatalog(ctx context.Context, ouID string, providerHandles []string) error
	ValidateMCPProxiesInCatalog(ctx context.Context, ouID string, proxyHandles []string) error
	Get(ctx context.Context, configUUID uuid.UUID, ouID, projectName, agentName string) (*models.AgentModelConfigResponse, error)
	GetMCP(ctx context.Context, configUUID uuid.UUID, ouID, projectName, agentName string) (*models.AgentModelConfigResponse, error)
	GetByAgent(ctx context.Context, agentID, ouID string) (*models.AgentModelConfigResponse, error)
	List(ctx context.Context, ouID, projectName, agentName string, limit, offset int) (*models.AgentModelConfigListResponse, error)
	ListByType(ctx context.Context, ouID, projectName, agentName string, typeID uint, limit, offset int) (*models.AgentModelConfigListResponse, error)
	ListMCP(ctx context.Context, ouID, projectName, agentName string, limit, offset int) (*models.AgentModelConfigListResponse, error)
	Update(ctx context.Context, configUUID uuid.UUID, ouID, projectName, agentName string,
		req models.UpdateAgentModelConfigRequest) (*models.AgentModelConfigResponse, error)
	UpdateMCP(ctx context.Context, configUUID uuid.UUID, ouID, projectName, agentName string,
		req models.UpdateAgentModelConfigRequest) (*models.AgentModelConfigResponse, error)
	Delete(ctx context.Context, configUUID uuid.UUID, ouID, projectName, agentName string) error
	DeleteMCP(ctx context.Context, configUUID uuid.UUID, ouID, projectName, agentName string) error
	// DeleteForAgentDeletion removes all external proxy resources for a single LLM config during
	// agent deletion. It skips OC Component/Workload/ReleaseBinding env-var patching and
	// SecretReference CR deletion because the component itself is being torn down. isExternalAgent
	// must be resolved once by the caller to avoid a GetComponent call per config.
	DeleteForAgentDeletion(ctx context.Context, configUUID uuid.UUID, ouID, projectName, agentName string, isExternalAgent bool) error
	// ListAgentLLMConfigSecretReferences returns the set of SecretReference names persisted in the
	// DB for all LLM configurations of this agent in the given environment. Used during deploy to
	// identify which component env var secretRefs are system-managed (LLM config) vs user-provided.
	ListAgentLLMConfigSecretReferences(ctx context.Context, agentID, ouID, environmentName string) (map[string]struct{}, error)
	// ListSystemManagedEnvVarKeys returns the set of env var keys that are system-managed
	// (i.e. injected by agent LLM/MCP configurations) for the given agent and environment.
	// Used during promote to strip these keys from inherited workload overrides.
	ListSystemManagedEnvVarKeys(ctx context.Context, agentID, ouID, projectName, environmentName string) (map[string]bool, error)
	// BuildSystemManagedEnvVarsFromConfig constructs system-managed env vars for a given
	// agent and environment from all DB configs. Used during promotion when the target
	// environment's ReleaseBinding doesn't have these vars yet.
	BuildSystemManagedEnvVarsFromConfig(ctx context.Context, agentID, ouID, projectName, environmentName string) ([]client.EnvVar, error)

	// ReconcileMCPBindingsForProxy binds agents to this MCP proxy in environments that
	// have become deployable since their connection was configured — an agent promoted
	// into an environment before the proxy had an endpoint there has its MCP env vars
	// injected but empty, and nothing else ever revisits that binding. Called after an
	// MCP proxy update, whose endpoint changes are what make an environment deployable.
	ReconcileMCPBindingsForProxy(ctx context.Context, ouID, proxyHandle string) error

	// ListUnresolvedMCPBindings returns the names of the agent's MCP connections that are
	// configured for this environment but resolve to no proxy URL, so their injected
	// URL/API-key variables are empty. Used by promote to refuse a promotion that would
	// silently carry a working connection into an environment where it is dead.
	ListUnresolvedMCPBindings(ctx context.Context, agentID, ouID, projectName, environmentName string) (map[string]struct{}, error)

	// CleanupEnvironmentMCPArtifacts tears down all MCP-proxy data tied to a deleted
	// environment: every agent-scoped mapping/deployment/artifact/secret/key/env-var row
	// for that env, plus the environment's block in every org-level MCP proxy blueprint.
	// Best-effort: aggregates errors, never rolls back the environment deletion.
	CleanupEnvironmentMCPArtifacts(ctx context.Context, ouID string, envUUID uuid.UUID, envName string) error

	// MCP config API keys — the per-config, per-environment API key an external
	// agent uses to call its MCP server through the gateway. Keyed by the env
	// mapping artifact (not the shared source MCP proxy). Only one key is managed
	// per mapping from the console.
	ListMCPConfigAPIKeys(ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName string) (*models.ListAPIKeysResponse, error)
	CreateMCPConfigAPIKey(ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName string, req *models.CreateAPIKeyRequest) (*models.CreateAPIKeyResponse, error)
	RotateMCPConfigAPIKey(ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName, keyName string, req *models.RotateAPIKeyRequest) (*models.CreateAPIKeyResponse, error)
	RevokeMCPConfigAPIKey(ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName, keyName string) error

	// LLM config API keys — the per-config, per-environment API key an external
	// agent uses to call its LLM provider through the gateway. Resolved from the
	// config + environment to the backing LLM proxy server-side (the frontend
	// does not need to know the proxy handle).
	ListLLMConfigAPIKeys(ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName string) (*models.ListAPIKeysResponse, error)
	CreateLLMConfigAPIKey(ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName string, req *models.CreateAPIKeyRequest) (*models.CreateAPIKeyResponse, error)
	RotateLLMConfigAPIKey(ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName, keyName string, req *models.RotateAPIKeyRequest) (*models.CreateAPIKeyResponse, error)
	RevokeLLMConfigAPIKey(ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName, keyName string) error
}

type EnvConfigTemplate struct {
	Key             string `json:"key"`
	Name            string `json:"name"`
	IsSecret        bool   `json:"isSecret"`
	Value           string `json:"value"`
	SecretReference string `json:"secretReference"`
}

type agentConfigurationService struct {
	db                        *gorm.DB
	agentConfigRepo           repositories.AgentConfigurationRepository
	envMappingRepo            repositories.EnvAgentModelMappingRepository
	envMCPMappingRepo         repositories.EnvAgentMCPMappingRepository
	envVariableRepo           repositories.AgentEnvConfigVariableRepository
	llmProviderRepo           repositories.LLMProviderRepository
	mcpProxyRepo              repositories.MCPProxyRepository
	gatewayRepo               repositories.GatewayRepository
	llmProxyService           *LLMProxyService
	mcpProxyService           *MCPProxyService
	llmProxyDeploymentService *LLMProxyDeploymentService
	llmProxyAPIKeyService     *LLMProxyAPIKeyService
	apiKeyBroadcaster         *apiKeyBroadcaster
	llmProviderAPIKeyService  *LLMProviderAPIKeyService
	aiApplicationService      *AIApplicationService
	infraResourceManager      InfraResourceManager
	ocClient                  client.OpenChoreoClient
	agentIdentityInjection    AgentIdentityInjectionService
	logger                    *slog.Logger
	secretClient              secretmanagersvc.SecretManagementClient
	encryptionKey             []byte
}

// rollbackResource tracks a proxy, its deployment, and API keys for cleanup
type rollbackResource struct {
	proxyHandle       string
	deploymentID      uuid.UUID
	proxyAPIKeyID     string                           // API key created for the proxy
	providerAPIKeyID  string                           // API key name created for the provider
	providerUUID      string                           // UUID of the provider (needed to revoke the provider API key)
	mappingID         uint                             // ID of the env mapping to revert (HIGH-4, Scenario A only)
	oldProxyUUID      uuid.UUID                        // old proxy UUID to restore in the mapping on rollback (HIGH-4, Scenario A only)
	providerSecretLoc *secretmanagersvc.SecretLocation // Location for provider API key secret
	proxySecretLoc    *secretmanagersvc.SecretLocation // Location for proxy API key secret
	secretRefName     string                           // Name of the SecretReference CR to delete on rollback (internal agents only)
	gatewayID         string                           // Gateway hosting deploymentID — required to undeploy before delete
	// AI application rollback fields — only set when EnsureAndBind created a new app.
	createdNewApp  bool
	appAgentID     string
	appProjectName string
	appEnvName     string
	// Set only by processEnvProxyUpdate (Scenario B): the proxy already existed
	// before this operation, so a later failure must restore its prior
	// configuration instead of deleting it outright.
	priorProxyConfig *models.LLMProxy
	// restoreDeploymentID is the OLD deployment that was still Deployed before
	// a new one superseded it (Scenario B only). Deploying the new "current"
	// deployment atomically overwrites the shared deployment_status row, so the
	// old deployment is left ARCHIVED even though it was never explicitly
	// undeployed — on rollback it must be explicitly reactivated, not just left
	// alone, or the proxy ends up with no live deployment on this gateway.
	restoreDeploymentID uuid.UUID
}

// pendingAppBinding captures the arguments for a deferred AIApplication
// EnsureAndBind call. Binding is deferred until every environment in a
// create/update request has otherwise succeeded — see flushPendingAppBindings —
// so a later environment's failure never needs to restore a gateway-side
// key binding that an earlier, now-rolled-back environment already changed.
type pendingAppBinding struct {
	ouID, projectName, agentID, envName string
	appHandle, appName, apiKeyUUID      string
}

// flushPendingAppBindings executes deferred EnsureAndBind calls once every
// environment in the request has otherwise succeeded. If a bind fails partway
// through, the AI applications newly created by the binds that already
// succeeded are appended to rollbackResources so the caller's rollback also
// tears them down alongside everything else created by this request.
func (s *agentConfigurationService) flushPendingAppBindings(
	ctx context.Context, pending []pendingAppBinding, rollbackResources *[]rollbackResource,
) error {
	for _, p := range pending {
		_, created, err := s.aiApplicationService.EnsureAndBind(
			ctx, p.ouID, p.projectName, p.agentID, p.envName, p.appHandle, p.appName, p.apiKeyUUID,
		)
		if err != nil {
			return fmt.Errorf("failed to bind API key for environment %s: %w", p.envName, err)
		}
		if created {
			*rollbackResources = append(*rollbackResources, rollbackResource{
				createdNewApp:  true,
				appAgentID:     p.agentID,
				appProjectName: p.projectName,
				appEnvName:     p.envName,
			})
		}
	}
	return nil
}

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

const proxyNamePrefixMaxLen = 10

// agentConfigListAll disables the row cap on the per-agent configuration listing used when
// rebuilding or auditing an agent's system-managed env vars: a truncated listing there
// silently drops bindings from promotion and from the unresolved-binding audit. GORM omits
// the LIMIT clause for a negative value.
const agentConfigListAll = -1

// agentAppIdentifier builds a stable, collision-resistant handle for the per-agent-per-env
// AIApplication. Format: "<agentPrefix>-<16-hex-chars>".
func agentAppIdentifier(projectName, agentID, envName string) string {
	raw := fmt.Sprintf("%s/%s/%s", projectName, agentID, envName)
	hash := sha256.Sum256([]byte(raw))
	hashSuffix := hex.EncodeToString(hash[:8])
	prefix := sanitizeForK8sName(agentID)
	if len(prefix) > proxyNamePrefixMaxLen {
		prefix = prefix[:proxyNamePrefixMaxLen]
	}
	return fmt.Sprintf("%s-%s", prefix, hashSuffix)
}

// scopedProxyIdentifier builds a deterministic, collision-resistant identifier
// from the config name and a hash of all scoping segments (project, agent, config, env).
// Format: "<configPrefix>-<16-hex-chars>" where configPrefix is the first 10 chars
// of the sanitized config name.
func scopedProxyIdentifier(projectName, agentName, configName, envName string) string {
	raw := fmt.Sprintf("%s/%s/%s/%s", projectName, agentName, configName, envName)
	hash := sha256.Sum256([]byte(raw))
	hashSuffix := hex.EncodeToString(hash[:8])

	prefix := sanitizeForK8sName(configName)
	if len(prefix) > proxyNamePrefixMaxLen {
		prefix = strings.TrimRight(prefix[:proxyNamePrefixMaxLen], "-")
	}
	return fmt.Sprintf("%s-%s", prefix, hashSuffix)
}

// mcpMappingProxyName builds the artifact handle/name and deployed proxy name for an
// agent-scoped MCP proxy mapping. It mirrors the LLM proxy naming scheme exactly:
// "<scopedID>-proxy", where scopedID is scopedProxyIdentifier(project, agent, config, env).
// Handle and name are identical, matching how LLM proxies derive both from the same value.
func mcpMappingProxyName(projectName, agentID, configName, envName string) string {
	return fmt.Sprintf("%s-proxy", scopedProxyIdentifier(projectName, agentID, configName, envName))
}

// agentProxyAPIKeyPurpose returns the purpose for the API key auto-generated for
// an agent's LLM proxy. External agents manage this key themselves (view the
// masked key, regenerate or delete it from the console), so it is user-managed.
// Managed agents have the platform inject the key, so it stays console-managed.
func agentProxyAPIKeyPurpose(isExternalAgent bool) int {
	if isExternalAgent {
		return models.APIKeyPurposeUserManaged
	}
	return models.APIKeyPurposeConsoleManaged
}

// ensureExternalAgentForAPIKey gates the user-managed API key flows
// (create/rotate/revoke) for a config-scoped proxy. These keys are only
// user-managed for external agents (see agentProxyAPIKeyPurpose); managed/internal
// agents have the platform inject console-managed keys, so user-driven actions must
// not proceed. When the agent backing the config is not external it returns
// utils.ErrAgentConfigNotExternal, which the controller maps to a 403. The check
// fails closed: if the agent type cannot be resolved the underlying error is
// returned and the action is rejected.
func (s *agentConfigurationService) ensureExternalAgentForAPIKey(ctx context.Context, ouID, projectName, agentName string) error {
	agent, err := s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
	if err != nil {
		if errors.Is(err, utils.ErrAgentNotFound) {
			return utils.ErrAgentConfigNotFound
		}
		return fmt.Errorf("failed to determine agent type: %w", err)
	}
	if agent.Provisioning.Type != string(utils.ExternalAgent) {
		return utils.ErrAgentConfigNotExternal
	}
	return nil
}

// ensureURLScheme returns the host as an absolute URL, defaulting scheme-less
// hosts to https. Gateways may be registered with bare vhosts.
func ensureURLScheme(host string) string {
	if strings.Contains(host, "://") {
		return host
	}
	return "https://" + host
}

// buildProxyURL constructs the proxy base URL from the gateway's public vhost and
// an optional context path. Every agent — sandboxed included — gets the public
// URL so the address always matches the identity resource identifier.
func buildProxyURL(gateway *models.Gateway, contextPath *string) string {
	base := ensureURLScheme(gateway.Vhost)
	if contextPath != nil {
		return fmt.Sprintf("%s%s", base, *contextPath)
	}
	return base
}

// buildLLMEnvVars constructs the two env vars (URL and API key) from the env config templates.
func buildLLMEnvVars(templates []EnvConfigTemplate, proxyURL, secretRefName string) []client.EnvVar {
	var urlTemplate, apiKeyTemplate EnvConfigTemplate
	for _, t := range templates {
		switch t.Key {
		case "url":
			urlTemplate = t
		case "apikey":
			apiKeyTemplate = t
		}
	}
	return []client.EnvVar{
		{Key: urlTemplate.Name, Value: proxyURL},
		{
			Key: apiKeyTemplate.Name,
			ValueFrom: &client.EnvVarValueFrom{
				SecretKeyRef: &client.SecretKeyRef{
					Name: secretRefName,
					Key:  secretmanagersvc.SecretKeyAPIKey,
				},
			},
		},
	}
}

func buildMCPEnvVars(templates []EnvConfigTemplate, proxyURL, secretRefName string) []client.EnvVar {
	var envVars []client.EnvVar
	for _, t := range templates {
		switch t.Key {
		case "url":
			envVars = append(envVars, client.EnvVar{Key: t.Name, Value: proxyURL})
		case "apikey":
			if secretRefName != "" {
				envVars = append(envVars, client.EnvVar{
					Key: t.Name,
					ValueFrom: &client.EnvVarValueFrom{
						SecretKeyRef: &client.SecretKeyRef{
							Name: secretRefName,
							Key:  secretmanagersvc.SecretKeyAPIKey,
						},
					},
				})
			}
		}
	}
	return envVars
}

// buildEmptyMCPEnvVars emits every env var template (url and apikey) with an empty
// string value. It is used for an environment the MCP proxy is not configured for, so
// the agent still has the variable names defined but blank. Unlike buildMCPEnvVars it
// never uses a SecretKeyRef — there is no secret for an unconfigured environment.
func buildEmptyMCPEnvVars(templates []EnvConfigTemplate) []client.EnvVar {
	envVars := make([]client.EnvVar, 0, len(templates))
	for _, t := range templates {
		envVars = append(envVars, client.EnvVar{Key: t.Name, Value: ""})
	}
	return envVars
}

// mcpProxyServingBase is the fully normalized public base the gateway actually
// serves the proxy on: the proxy's own vhost override when set (the deployment
// spec forwards it), else the gateway's vhost. Bare hosts default to https and
// any trailing slash is dropped.
func mcpProxyServingBase(gateway *models.Gateway, override *string) string {
	base := gateway.Vhost
	if override != nil {
		if host := strings.TrimSpace(*override); host != "" {
			base = host
		}
	}
	return strings.TrimRight(ensureURLScheme(strings.TrimSpace(base)), "/")
}

// buildMCPProxyURL constructs the MCP proxy URL from the serving base and the
// proxy's optional context path, appending the "/mcp" route.
func buildMCPProxyURL(gateway *models.Gateway, cfg models.MCPProxyConfig) string {
	path := "/mcp"
	if cfg.Context != nil {
		if trimmed := strings.TrimSpace(*cfg.Context); trimmed != "" {
			path = strings.TrimRight(trimmed, "/") + "/mcp"
		}
	}
	return mcpProxyServingBase(gateway, cfg.Vhost) + path
}

// mcpProxyAPIKeySecurityEnabled reports whether the source MCP proxy requires API
// key security for the given environment. When it returns false, agent mappings
// derived from the proxy are deployed without minting a gateway key, binding an app
// key, or injecting the apikey env var — mirroring how an LLM provider with security
// disabled yields proxies with no API key wired in the gateway. Security is stored
// per-environment on the blueprint, so the environment UUID selects the block.
func mcpProxyAPIKeySecurityEnabled(proxy *models.MCPProxy, envID string) bool {
	security := mcpProxySecurityForEnv(proxy, envID)
	if security == nil || !isBoolTrue(security.Enabled) {
		return false
	}
	return security.APIKey != nil && isBoolTrue(security.APIKey.Enabled)
}

func mcpProxyAPIKeyHeaderName(proxy *models.MCPProxy, envID string) string {
	security := mcpProxySecurityForEnv(proxy, envID)
	if security == nil || security.APIKey == nil {
		return "X-API-Key"
	}
	header := strings.TrimSpace(security.APIKey.Key)
	if header == "" {
		return "X-API-Key"
	}
	return header
}

// mcpProxySecurityForEnv returns the security config from the endpoint bound to the given
// environment, or nil when the proxy has no endpoint bound to that environment.
func mcpProxySecurityForEnv(proxy *models.MCPProxy, envID string) *models.SecurityConfig {
	endpoint, _ := resolveMCPEndpointForEnv(proxy, envID)
	if endpoint == nil {
		return nil
	}
	return endpoint.Configuration.Security
}

// mcpProxyEnvArtifactUUID returns the stable per-environment gateway artifact UUID for the
// given environment from the endpoint→environment binding, or uuid.Nil when the proxy has
// no endpoint bound to that environment. This UUID is the gateway-facing apiID that
// per-agent inbound API keys are minted against, so the gateway validates them against the
// single shared artifact the endpoint deployed for that environment.
func mcpProxyEnvArtifactUUID(proxy *models.MCPProxy, envID string) uuid.UUID {
	_, ee := resolveMCPEndpointForEnv(proxy, envID)
	if ee == nil {
		return uuid.Nil
	}
	return ee.ArtifactUUID
}

// resolveMCPMappingAPIID resolves the shared per-environment artifact UUID that a mapping's
// inbound API key must target on the gateway (the apiID). It prefers the mapping's preloaded
// source proxy and falls back to loading the proxy by UUID.
func (s *agentConfigurationService) resolveMCPMappingAPIID(ctx context.Context, mapping *models.EnvAgentMCPMapping, ouID string) uuid.UUID {
	if id := mcpProxyEnvArtifactUUID(mapping.MCPProxy, mapping.EnvironmentUUID.String()); id != uuid.Nil {
		return id
	}
	if s.mcpProxyRepo != nil && mapping.MCPProxyUUID != uuid.Nil {
		if proxy, err := s.mcpProxyRepo.GetByUUID(ctx, mapping.MCPProxyUUID.String(), ouID); err == nil {
			return mcpProxyEnvArtifactUUID(proxy, mapping.EnvironmentUUID.String())
		}
	}
	return uuid.Nil
}

// createMCPMappingAPIKey mints a per-agent inbound API key. apiID is the shared
// per-environment proxy artifact the gateway validates the key against; storageUUID is the
// per-agent key-holder artifact under which the key is persisted and later listed/revoked.
// Keeping them distinct lets many agents share one gateway artifact while retaining
// per-agent key issuance and revocation.
func (s *agentConfigurationService) createMCPMappingAPIKey(ctx context.Context, ouID string, apiID, storageUUID uuid.UUID, keyName string) (*models.CreateAPIKeyResponse, error) {
	if s.apiKeyBroadcaster == nil {
		return nil, fmt.Errorf("API key service is not configured")
	}
	if apiID == uuid.Nil {
		return nil, fmt.Errorf("MCP proxy shared artifact not found")
	}
	return s.apiKeyBroadcaster.broadcastCreate(ctx, ouID, apiID.String(), storageUUID.String(), &models.CreateAPIKeyRequest{
		Name: keyName,
	})
}

func (s *agentConfigurationService) revokeMCPMappingAPIKey(ctx context.Context, ouID string, apiID, storageUUID uuid.UUID, keyName string) error {
	if s.apiKeyBroadcaster == nil {
		return fmt.Errorf("API key service is not configured")
	}
	if apiID == uuid.Nil {
		if s.apiKeyBroadcaster.apiKeyRepo == nil {
			return nil
		}
		return s.apiKeyBroadcaster.apiKeyRepo.Delete(storageUUID.String(), keyName)
	}
	return s.apiKeyBroadcaster.broadcastRevoke(ctx, ouID, apiID.String(), storageUUID.String(), keyName)
}

func mcpMappingScopedID(config *models.AgentConfiguration, envName string) string {
	return scopedProxyIdentifier(config.ProjectName, config.AgentID, config.Name, envName)
}

func mcpMappingAPIKeyName(config *models.AgentConfiguration, envName string) string {
	return fmt.Sprintf("%s-key", mcpMappingScopedID(config, envName))
}

func mcpMappingSecretLocation(config *models.AgentConfiguration, ouID, envName string) secretmanagersvc.SecretLocation {
	scopedID := mcpMappingScopedID(config, envName)
	return secretmanagersvc.SecretLocation{
		OrgName:         ouID,
		ProjectName:     config.ProjectName,
		AgentName:       config.AgentID,
		EnvironmentName: envName,
		ConfigName:      config.Name,
		EntityName:      fmt.Sprintf("%s-proxy", scopedID),
		SecretKey:       secretmanagersvc.SecretKeyAPIKey,
	}
}

func (s *agentConfigurationService) mcpMappingAPIKeyExists(mappingUUID uuid.UUID, keyName string) (bool, error) {
	if s.apiKeyBroadcaster == nil || s.apiKeyBroadcaster.apiKeyRepo == nil {
		return false, nil
	}
	_, err := s.apiKeyBroadcaster.apiKeyRepo.GetByArtifactAndName(mappingUUID.String(), keyName)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

// revokeStaleMCPMappingAPIKeys revokes every key persisted under the per-agent key-holder
// artifact (storageUUID) except keepName, broadcasting revocation against the shared
// gateway artifact (apiID). storageUUID scopes the listing to this agent's keys; apiID
// tells the gateway which deployed artifact to drop the key from.
func (s *agentConfigurationService) revokeStaleMCPMappingAPIKeys(ctx context.Context, ouID string, apiID, storageUUID uuid.UUID, keepName string) error {
	if s.apiKeyBroadcaster == nil || s.apiKeyBroadcaster.apiKeyRepo == nil {
		return nil
	}
	keys, err := s.apiKeyBroadcaster.apiKeyRepo.ListByArtifact(ctx, storageUUID.String())
	if err != nil {
		return err
	}
	var errs []error
	for _, key := range keys {
		if key.Name == keepName {
			continue
		}
		if err := s.revokeMCPMappingAPIKey(ctx, ouID, apiID, storageUUID, key.Name); err != nil {
			errs = append(errs, fmt.Errorf("key %s: %w", key.Name, err))
		}
	}
	return errors.Join(errs...)
}

func (s *agentConfigurationService) revokeAllMCPMappingAPIKeys(ctx context.Context, ouID string, apiID, storageUUID uuid.UUID) error {
	return s.revokeStaleMCPMappingAPIKeys(ctx, ouID, apiID, storageUUID, "")
}

// resolveConfigAndEnvUUID loads an agent configuration, validates it belongs to
// the given agent/org/project, and resolves the environment name to its UUID.
// Shared prologue for the per-config key resolvers; the returned config has its
// env mappings preloaded (see AgentConfigurationRepository.GetByUUID).
func (s *agentConfigurationService) resolveConfigAndEnvUUID(
	ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName string,
) (*models.AgentConfiguration, string, error) {
	config, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", utils.ErrAgentConfigNotFound
		}
		return nil, "", fmt.Errorf("failed to get agent configuration: %w", err)
	}
	if config == nil || config.AgentID != agentName || config.ProjectName != projectName {
		return nil, "", utils.ErrAgentConfigNotFound
	}

	envs, err := s.infraResourceManager.ListOrgEnvironments(ctx, ouID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list environments: %w", err)
	}
	for _, env := range envs {
		if env.Name == envName {
			return config, env.UUID, nil
		}
	}
	return nil, "", utils.ErrAgentConfigNotFound
}

// resolveMCPMappingArtifactUUID resolves the env mapping artifact UUID that backs
// an external agent's MCP configuration in the given environment. This artifact is
// the key holder for the per-config MCP API key (distinct from the shared source
// MCP proxy). It validates that the configuration belongs to the agent/org.
func (s *agentConfigurationService) resolveMCPMappingArtifactUUID(
	ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName string,
) (uuid.UUID, error) {
	config, envUUID, err := s.resolveConfigAndEnvUUID(ctx, ouID, projectName, agentName, configUUID, envName)
	if err != nil {
		return uuid.Nil, err
	}
	for _, mapping := range config.EnvMCPMappings {
		if mapping.EnvironmentUUID.String() == envUUID {
			return mapping.ArtifactUUID, nil
		}
	}
	return uuid.Nil, utils.ErrAgentConfigNotFound
}

// resolveMCPMappingKeyBinding resolves both the per-agent key-holder artifact UUID
// (storageUUID, under which the key is persisted/listed) and the shared per-environment
// proxy artifact UUID (apiID, which the gateway validates the key against) for an
// external agent's MCP configuration in the given environment.
func (s *agentConfigurationService) resolveMCPMappingKeyBinding(
	ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName string,
) (storageUUID, apiID uuid.UUID, err error) {
	config, envUUID, err := s.resolveConfigAndEnvUUID(ctx, ouID, projectName, agentName, configUUID, envName)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	for i := range config.EnvMCPMappings {
		mapping := &config.EnvMCPMappings[i]
		if mapping.EnvironmentUUID.String() == envUUID {
			apiID = s.resolveMCPMappingAPIID(ctx, mapping, ouID)
			if apiID == uuid.Nil {
				return uuid.Nil, uuid.Nil, fmt.Errorf("MCP proxy shared artifact not found for environment %s", envName)
			}
			return mapping.ArtifactUUID, apiID, nil
		}
	}
	return uuid.Nil, uuid.Nil, utils.ErrAgentConfigNotFound
}

// ListMCPConfigAPIKeys returns the masked, user-managed API key(s) for an external
// agent's MCP configuration in the given environment.
func (s *agentConfigurationService) ListMCPConfigAPIKeys(
	ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName string,
) (*models.ListAPIKeysResponse, error) {
	if s.apiKeyBroadcaster == nil || s.apiKeyBroadcaster.apiKeyRepo == nil {
		return nil, fmt.Errorf("API key service is not configured")
	}
	mappingUUID, err := s.resolveMCPMappingArtifactUUID(ctx, ouID, projectName, agentName, configUUID, envName)
	if err != nil {
		return nil, err
	}

	stored, err := s.apiKeyBroadcaster.apiKeyRepo.ListByArtifact(ctx, mappingUUID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}

	return &models.ListAPIKeysResponse{Keys: models.ToUserManagedAPIKeyInfos(stored)}, nil
}

// CreateMCPConfigAPIKey generates the per-config MCP API key and broadcasts it to
// the gateways. The key is returned once.
func (s *agentConfigurationService) CreateMCPConfigAPIKey(
	ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName string, req *models.CreateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	if s.apiKeyBroadcaster == nil {
		return nil, fmt.Errorf("API key service is not configured")
	}
	if err := s.ensureExternalAgentForAPIKey(ctx, ouID, projectName, agentName); err != nil {
		return nil, err
	}
	storageUUID, apiID, err := s.resolveMCPMappingKeyBinding(ctx, ouID, projectName, agentName, configUUID, envName)
	if err != nil {
		return nil, err
	}
	return s.apiKeyBroadcaster.broadcastCreate(ctx, ouID, apiID.String(), storageUUID.String(), req)
}

// RotateMCPConfigAPIKey revokes the current per-config MCP API key and generates a
// new value under the same key name. The new key is returned once.
func (s *agentConfigurationService) RotateMCPConfigAPIKey(
	ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName, keyName string, req *models.RotateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	if s.apiKeyBroadcaster == nil {
		return nil, fmt.Errorf("API key service is not configured")
	}
	if err := s.ensureExternalAgentForAPIKey(ctx, ouID, projectName, agentName); err != nil {
		return nil, err
	}
	storageUUID, apiID, err := s.resolveMCPMappingKeyBinding(ctx, ouID, projectName, agentName, configUUID, envName)
	if err != nil {
		return nil, err
	}
	return s.apiKeyBroadcaster.broadcastRotate(ctx, ouID, apiID.String(), storageUUID.String(), keyName, req)
}

// RevokeMCPConfigAPIKey revokes and removes the per-config MCP API key.
func (s *agentConfigurationService) RevokeMCPConfigAPIKey(
	ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName, keyName string,
) error {
	if s.apiKeyBroadcaster == nil {
		return fmt.Errorf("API key service is not configured")
	}
	if err := s.ensureExternalAgentForAPIKey(ctx, ouID, projectName, agentName); err != nil {
		return err
	}
	storageUUID, apiID, err := s.resolveMCPMappingKeyBinding(ctx, ouID, projectName, agentName, configUUID, envName)
	if err != nil {
		return err
	}
	return s.apiKeyBroadcaster.broadcastRevoke(ctx, ouID, apiID.String(), storageUUID.String(), keyName)
}

// resolveLLMProxyHandleForConfig resolves the LLM proxy handle backing an external
// agent's LLM configuration in the given environment. The proxy API key
// operations are keyed by this handle. Validates the config belongs to the agent/org.
func (s *agentConfigurationService) resolveLLMProxyHandleForConfig(
	ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName string,
) (string, error) {
	config, envUUID, err := s.resolveConfigAndEnvUUID(ctx, ouID, projectName, agentName, configUUID, envName)
	if err != nil {
		return "", err
	}
	for _, mapping := range config.EnvMappings {
		if mapping.EnvironmentUUID.String() == envUUID && mapping.LLMProxy != nil {
			return mapping.LLMProxy.Handle, nil
		}
	}
	return "", utils.ErrAgentConfigNotFound
}

// ListLLMConfigAPIKeys returns the masked, user-managed API key(s) for an external
// agent's LLM configuration in the given environment.
func (s *agentConfigurationService) ListLLMConfigAPIKeys(
	ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName string,
) (*models.ListAPIKeysResponse, error) {
	if s.llmProxyAPIKeyService == nil {
		return nil, fmt.Errorf("API key service is not configured")
	}
	handle, err := s.resolveLLMProxyHandleForConfig(ctx, ouID, projectName, agentName, configUUID, envName)
	if err != nil {
		return nil, err
	}
	return s.llmProxyAPIKeyService.ListAPIKeys(ctx, ouID, projectName, handle)
}

// CreateLLMConfigAPIKey generates the per-config LLM API key and broadcasts it. The
// key is returned once.
func (s *agentConfigurationService) CreateLLMConfigAPIKey(
	ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName string, req *models.CreateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	if s.llmProxyAPIKeyService == nil {
		return nil, fmt.Errorf("API key service is not configured")
	}
	if err := s.ensureExternalAgentForAPIKey(ctx, ouID, projectName, agentName); err != nil {
		return nil, err
	}
	handle, err := s.resolveLLMProxyHandleForConfig(ctx, ouID, projectName, agentName, configUUID, envName)
	if err != nil {
		return nil, err
	}
	return s.llmProxyAPIKeyService.CreateAPIKey(ctx, ouID, handle, req)
}

// RotateLLMConfigAPIKey revokes the current per-config LLM API key and generates a
// new value under the same key name. The new key is returned once.
func (s *agentConfigurationService) RotateLLMConfigAPIKey(
	ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName, keyName string, req *models.RotateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	if s.llmProxyAPIKeyService == nil {
		return nil, fmt.Errorf("API key service is not configured")
	}
	if err := s.ensureExternalAgentForAPIKey(ctx, ouID, projectName, agentName); err != nil {
		return nil, err
	}
	handle, err := s.resolveLLMProxyHandleForConfig(ctx, ouID, projectName, agentName, configUUID, envName)
	if err != nil {
		return nil, err
	}
	return s.llmProxyAPIKeyService.RotateAPIKey(ctx, ouID, handle, keyName, req)
}

// RevokeLLMConfigAPIKey revokes and removes the per-config LLM API key.
func (s *agentConfigurationService) RevokeLLMConfigAPIKey(
	ctx context.Context, ouID, projectName, agentName string, configUUID uuid.UUID, envName, keyName string,
) error {
	if s.llmProxyAPIKeyService == nil {
		return fmt.Errorf("API key service is not configured")
	}
	if err := s.ensureExternalAgentForAPIKey(ctx, ouID, projectName, agentName); err != nil {
		return err
	}
	handle, err := s.resolveLLMProxyHandleForConfig(ctx, ouID, projectName, agentName, configUUID, envName)
	if err != nil {
		return err
	}
	return s.llmProxyAPIKeyService.RevokeAPIKey(ctx, ouID, handle, keyName)
}

// envCredentialData tracks proxy credentials for external agents
type envCredentialData struct {
	apiKey   string
	proxyURL string
}

// NewAgentConfigurationService creates a new agent configuration service
func NewAgentConfigurationService(
	db *gorm.DB,
	agentConfigRepo repositories.AgentConfigurationRepository,
	envMappingRepo repositories.EnvAgentModelMappingRepository,
	envMCPMappingRepo repositories.EnvAgentMCPMappingRepository,
	envVariableRepo repositories.AgentEnvConfigVariableRepository,
	llmProviderRepo repositories.LLMProviderRepository,
	mcpProxyRepo repositories.MCPProxyRepository,
	gatewayRepo repositories.GatewayRepository,
	llmProxyService *LLMProxyService,
	mcpProxyService *MCPProxyService,
	llmProxyDeploymentService *LLMProxyDeploymentService,
	llmProxyAPIKeyService *LLMProxyAPIKeyService,
	gatewayService *GatewayEventsService,
	apiKeyRepo repositories.APIKeyRepository,
	infraResourceManager InfraResourceManager,
	ocClient client.OpenChoreoClient,
	llmProviderAPIKeyService *LLMProviderAPIKeyService,
	aiApplicationService *AIApplicationService,
	agentIdentityInjection AgentIdentityInjectionService,
	logger *slog.Logger,
	secretClient secretmanagersvc.SecretManagementClient,
	encryptionKey []byte,
) AgentConfigurationService {
	svc := &agentConfigurationService{
		db:                        db,
		agentConfigRepo:           agentConfigRepo,
		envMappingRepo:            envMappingRepo,
		envMCPMappingRepo:         envMCPMappingRepo,
		envVariableRepo:           envVariableRepo,
		llmProviderRepo:           llmProviderRepo,
		mcpProxyRepo:              mcpProxyRepo,
		gatewayRepo:               gatewayRepo,
		llmProxyService:           llmProxyService,
		mcpProxyService:           mcpProxyService,
		llmProxyDeploymentService: llmProxyDeploymentService,
		llmProxyAPIKeyService:     llmProxyAPIKeyService,
		apiKeyBroadcaster: &apiKeyBroadcaster{
			gatewayRepo:    gatewayRepo,
			gatewayService: gatewayService,
			apiKeyRepo:     apiKeyRepo,
		},
		aiApplicationService:     aiApplicationService,
		infraResourceManager:     infraResourceManager,
		ocClient:                 ocClient,
		llmProviderAPIKeyService: llmProviderAPIKeyService,
		agentIdentityInjection:   agentIdentityInjection,
		logger:                   logger,
		secretClient:             secretClient,
		encryptionKey:            encryptionKey,
	}
	// Register the deletion reconciler now that this service exists; MCPProxyService is
	// constructed first and calls back into here when a proxy is deleted to strip the
	// proxy's injected env vars from dependent agents.
	return svc
}

// compensatingDeleteConfig performs a best-effort DELETE of the config row committed in Phase 1,
// when a later phase fails. CASCADE on EnvMappings/EnvVariables removes any partially-written rows.
func (s *agentConfigurationService) compensatingDeleteConfig(ctx context.Context, configUUID uuid.UUID, ouID string) {
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return s.agentConfigRepo.Delete(ctx, tx, configUUID, ouID)
	}); err != nil {
		s.logger.Error("CRITICAL: Failed to compensate config creation - orphaned config record",
			"config_uuid", configUUID, "ou_id", ouID, "error", err, "action", "manual cleanup required")
	} else {
		s.logger.Info("Compensating delete of config record succeeded", "config_uuid", configUUID)
	}
}

// ValidateProvidersInCatalog verifies each handle resolves to an existing provider
// that is in catalog. Returns ErrLLMProviderNotFound (missing) or ErrInvalidInput
// (empty handle / not in catalog). Handles are deduped.
func (s *agentConfigurationService) ValidateProvidersInCatalog(
	_ context.Context, ouID string, providerHandles []string,
) error {
	seen := make(map[string]struct{}, len(providerHandles))
	for _, handle := range providerHandles {
		if handle == "" {
			return fmt.Errorf("%w: provider name is required", utils.ErrInvalidInput)
		}
		if _, dup := seen[handle]; dup {
			continue
		}
		seen[handle] = struct{}{}

		provider, err := s.llmProviderRepo.GetByHandle(handle, ouID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("provider %s not found: %w", handle, utils.ErrLLMProviderNotFound)
			}
			return fmt.Errorf("failed to validate provider %s: %w", handle, err)
		}
		if !provider.InCatalog {
			return fmt.Errorf("%w: provider %s must be in catalog", utils.ErrInvalidInput, handle)
		}
	}
	return nil
}

// ValidateMCPProxiesInCatalog verifies each handle resolves to an existing MCP proxy
// that is published in the catalog. Mirrors ValidateProvidersInCatalog and is used by the
// MCP auto-wiring preflight so a bad proxy fails fast before the component is created.
func (s *agentConfigurationService) ValidateMCPProxiesInCatalog(
	ctx context.Context, ouID string, proxyHandles []string,
) error {
	if s.mcpProxyRepo == nil {
		return fmt.Errorf("MCP configuration service is not fully configured")
	}
	seen := make(map[string]struct{}, len(proxyHandles))
	for _, handle := range proxyHandles {
		handle = strings.TrimSpace(handle)
		if handle == "" {
			return fmt.Errorf("%w: MCP proxy name is required", utils.ErrInvalidInput)
		}
		if _, dup := seen[handle]; dup {
			continue
		}
		seen[handle] = struct{}{}

		proxy, err := s.mcpProxyRepo.GetByHandle(ctx, handle, ouID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("MCP proxy %s not found: %w", handle, utils.ErrMCPProxyNotFound)
			}
			return fmt.Errorf("failed to validate MCP proxy %s: %w", handle, err)
		}
		if proxy.Artifact == nil || !proxy.Artifact.InCatalog {
			return fmt.Errorf("%w: MCP proxy %s must be in catalog", utils.ErrInvalidInput, handle)
		}
	}
	return nil
}

// Create creates a new agent model configuration
func (s *agentConfigurationService) Create(ctx context.Context, ouID, projectName, agentID string,
	req models.CreateAgentModelConfigRequest, createdBy string,
) (*models.AgentModelConfigResponse, error) {
	// Validate agent exists and determine type
	agent, err := s.ocClient.GetComponent(ctx, ouID, projectName, agentID)
	if err != nil {
		// Check if it's a 404 error (agent not found) vs other errors
		if errors.Is(err, utils.ErrAgentNotFound) {
			return nil, utils.ErrAgentNotFound
		}
		// For other errors (unauthorized, internal, etc), return as-is
		return nil, fmt.Errorf("failed to validate agent: %w", err)
	}

	// Determine if this is an external agent
	isExternalAgent := agent.Provisioning.Type == string(utils.ExternalAgent)

	switch req.Type {
	case models.AgentConfigTypeMCP:
		return s.createMCPConfig(ctx, ouID, projectName, agentID, req, createdBy, isExternalAgent)
	default:
		return s.createLLMConfig(ctx, ouID, projectName, agentID, req, createdBy, isExternalAgent)
	}
}

func (s *agentConfigurationService) createLLMConfig(ctx context.Context, ouID, projectName, agentID string,
	req models.CreateAgentModelConfigRequest, createdBy string, isExternalAgent bool,
) (*models.AgentModelConfigResponse, error) {
	// Validate that at least one environment mapping is provided (CRIT-5).
	// The binding:"required,min=1" tag on the DTO is ignored by net/http + json.NewDecoder,
	// so we enforce it explicitly here.
	if len(req.EnvMappings) == 0 {
		return nil, fmt.Errorf("%w: at least one environment mapping is required", utils.ErrInvalidInput)
	}

	// Fail fast: validate env var names before any I/O.
	// If the config name would generate a reserved env var prefix the error is returned here,
	// before any gateway/proxy/deployment resources have been created.
	// The returned slice is intentionally discarded; it is rebuilt at deployment time.
	if _, err := s.buildEnvironmentVariables(req.Name, req.EnvironmentVariables); err != nil {
		return nil, errors.Join(utils.ErrInvalidInput, err)
	}

	// Validate all providers exist and are in catalog (shared with the create-time preflight).
	handles := make([]string, 0, len(req.EnvMappings))
	for envName, envMapping := range req.EnvMappings {
		handles = append(handles, envMapping.ProviderName)
		if err := envMapping.Configuration.Resilience.Validate(); err != nil {
			return nil, fmt.Errorf("%w: environment %s: %w", utils.ErrInvalidInput, envName, err)
		}
	}
	if err := s.ValidateProvidersInCatalog(ctx, ouID, handles); err != nil {
		return nil, err
	}

	// Validate environment UUIDs exist
	envs, err := s.infraResourceManager.ListOrgEnvironments(ctx, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	envMap := make(map[string]*models.EnvironmentResponse)
	for _, env := range envs {
		envMap[env.Name] = env
	}

	for envName := range req.EnvMappings {
		if _, exists := envMap[envName]; !exists {
			return nil, fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, envName)
		}
	}

	// Build config struct (UUID assigned on Create)
	config := &models.AgentConfiguration{
		Name:        req.Name,
		Description: req.Description,
		AgentID:     agentID,
		TypeID:      models.AgentConfigTypeToID(req.Type),
		OUID:        ouID,
		ProjectName: projectName,
	}

	// Phase 1 — Short TX: persist config row only.
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.agentConfigRepo.Create(ctx, tx, config); err != nil {
			if errors.Is(err, utils.ErrAgentConfigAlreadyExists) {
				return err
			}
			return fmt.Errorf("failed to create configuration: %w", err)
		}
		return nil
	}); err != nil {
		if errors.Is(err, utils.ErrAgentConfigAlreadyExists) {
			return nil, utils.ErrAgentConfigAlreadyExists
		}
		return nil, err
	}

	// Track created resources for rollback across all environments.
	var rollbackResources []rollbackResource

	// AI application bindings are deferred until every environment below
	// succeeds — see flushPendingAppBindings.
	var pendingAppBindings []pendingAppBinding

	// Track credentials for external agents.
	var envCredentials map[string]envCredentialData
	if isExternalAgent {
		envCredentials = make(map[string]envCredentialData)
	}

	// Resolve first/dev environment name for ReleaseBinding patch (internal agents only).
	firstEnvName := ""
	if !isExternalAgent {
		pipeline, pipelineErr := s.ocClient.GetProjectDeploymentPipeline(ctx, ouID, projectName)
		if pipelineErr != nil {
			s.logger.Warn("failed to get deployment pipeline; ReleaseBinding patch will be skipped", "error", pipelineErr)
		} else if pipeline != nil {
			firstEnvName = client.FindFirstEnvironment(pipeline.PromotionPaths)
		}
	}

	// Phase 2 — Loop over environments: external ops first, then short per-env TX.
	// NOTE: map iteration order is non-deterministic; partial failures leave a random subset processed.
	for envName, envMapping := range req.EnvMappings {
		// Context cancellation check before each env.
		select {
		case <-ctx.Done():
			// Detach from cancellation so a cancelled ctx doesn't prevent rollback
			// (CRIT-2) — but keep the values, so the rollback's log records still
			// carry the correlation ID of the request that triggered it.
			// context.Background() dropped them, leaving the failure path, which is
			// the one worth tracing, as orphan log lines.
			cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cleanupCancel()
			s.processRollBack(cleanupCtx, rollbackResources, ouID, config.UUID)
			return nil, fmt.Errorf("operation cancelled: %w", ctx.Err())
		default:
		}

		env, exists := envMap[envName]
		if !exists {
			s.processRollBack(ctx, rollbackResources, ouID, config.UUID)
			return nil, fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, envName)
		}

		envUUID, err := uuid.Parse(env.UUID)
		if err != nil {
			s.processRollBack(ctx, rollbackResources, ouID, config.UUID)
			return nil, fmt.Errorf("invalid environment id %q: %w", envName, err)
		}

		// External ops — no transaction held.
		proxyConfig, providerAPIKeyID, providerUUID, providerSecretLoc, scopedID, err := s.buildLLMProxyConfig(ctx, config, env.Name, envMapping)
		if err != nil {
			s.processRollBack(ctx, rollbackResources, ouID, config.UUID)
			return nil, fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
		}

		// Resolve gateway where the provider is deployed (ensures proxy uses the same gateway)
		gateway, err := s.resolveGatewayForProvider(ctx, providerUUID, ouID, envUUID)
		if err != nil {
			s.processRollBack(ctx, rollbackResources, ouID, config.UUID)
			return nil, fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, err)
		}
		// Track provider credentials immediately so they are cleaned up even if proxy creation fails.
		rollbackResources = append(rollbackResources, rollbackResource{
			providerAPIKeyID:  providerAPIKeyID,
			providerUUID:      providerUUID,
			providerSecretLoc: providerSecretLoc,
		})
		// Capture index immediately after append to avoid fragile len(slice)-1 indexing below.
		rbIdx := len(rollbackResources) - 1

		proxy, err := s.llmProxyService.Create(ctx, ouID, createdBy, proxyConfig)
		if err != nil {
			s.processRollBack(ctx, rollbackResources, ouID, config.UUID)
			return nil, fmt.Errorf("failed to create proxy for environment %s: %w", envName, err)
		}
		// Update the rollback entry with the proxy handle now that it was created.
		rollbackResources[rbIdx].proxyHandle = proxy.Handle

		deployment, err := s.llmProxyDeploymentService.DeployLLMProxy(ctx, proxy.Handle, &models.DeployAPIRequest{
			Name:      fmt.Sprintf("%s-deployment", scopedID),
			Base:      "current",
			GatewayID: gateway.UUID.String(),
		}, ouID)
		if err != nil {
			s.processRollBack(ctx, rollbackResources, ouID, config.UUID)
			return nil, fmt.Errorf("failed to deploy proxy for environment %s: %w", envName, err)
		}
		rollbackResources[rbIdx].deploymentID = deployment.DeploymentID
		rollbackResources[rbIdx].gatewayID = gateway.UUID.String()

		proxyAPIKey, err := s.llmProxyAPIKeyService.CreateAPIKey(ctx, ouID, proxy.Handle, &models.CreateAPIKeyRequest{
			Name:    fmt.Sprintf("%s-key", scopedID),
			Purpose: agentProxyAPIKeyPurpose(isExternalAgent),
		})
		if err != nil {
			s.rollbackProxies(ctx, rollbackResources, ouID)
			s.compensatingDeleteConfig(ctx, config.UUID, ouID)
			return nil, fmt.Errorf("failed to generate API key for environment %s: %w", envName, err)
		}
		s.logger.Info("Created proxy API key", "proxy_handle", proxy.Handle, "proxy_key_name", proxyAPIKey.KeyID, "name", fmt.Sprintf("%s-key", scopedID))
		rollbackResources[rbIdx].proxyAPIKeyID = proxyAPIKey.KeyID

		// Ensure one AI application exists per agent+env and bind the proxy API
		// key — deferred until every environment succeeds (flushPendingAppBindings).
		agentAppHandle := agentAppIdentifier(config.ProjectName, config.AgentID, env.Name)
		pendingAppBindings = append(pendingAppBindings, pendingAppBinding{
			ouID: ouID, projectName: config.ProjectName, agentID: config.AgentID, envName: env.Name,
			appHandle:  agentAppHandle,
			appName:    fmt.Sprintf("%s Application", config.AgentID),
			apiKeyUUID: proxyAPIKey.KeyID,
		})

		// Store proxy API key via the secret management client (provider manages the SecretReference)
		proxySecretLoc := secretmanagersvc.SecretLocation{
			OrgName:         ouID,
			ProjectName:     projectName,
			AgentName:       agentID,
			EnvironmentName: env.Name,
			ConfigName:      config.Name,
			EntityName:      proxy.Handle,
			SecretKey:       secretmanagersvc.SecretKeyAPIKey,
		}
		secretRefName, err := s.secretClient.CreateSecret(ctx, proxySecretLoc,
			map[string]string{secretmanagersvc.SecretKeyAPIKey: proxyAPIKey.APIKey})
		if err != nil {
			s.rollbackProxies(ctx, rollbackResources, ouID)
			s.compensatingDeleteConfig(ctx, config.UUID, ouID)
			return nil, fmt.Errorf("failed to store proxy API key in KV for environment %s: %w", envName, err)
		}
		rollbackResources[rbIdx].proxySecretLoc = &proxySecretLoc
		rollbackResources[rbIdx].secretRefName = secretRefName

		// Build proxy URL with nil-safe context access.
		var proxyContext *string
		if proxy != nil {
			proxyContext = proxy.Configuration.Context
		}
		proxyURL := buildProxyURL(gateway, proxyContext)

		// Capture credentials for external agents.
		if isExternalAgent {
			envCredentials[envUUID.String()] = envCredentialData{
				apiKey:   proxyAPIKey.APIKey,
				proxyURL: proxyURL,
			}
		}

		// Build environment variables (pure computation, no I/O).
		envConfigTemplates, err := s.buildEnvironmentVariables(config.Name, req.EnvironmentVariables)
		if err != nil {
			s.rollbackProxies(ctx, rollbackResources, ouID)
			s.compensatingDeleteConfig(ctx, config.UUID, ouID)
			return nil, fmt.Errorf("failed to build environment variables for %s: %w", envName, err)
		}
		variables := []models.AgentEnvConfigVariable{}
		for _, envConfigTemplate := range envConfigTemplates {
			secretReference := ""
			if envConfigTemplate.IsSecret {
				secretReference = secretRefName
			}
			variables = append(variables, models.AgentEnvConfigVariable{
				ConfigUUID:      config.UUID,
				EnvironmentUUID: envUUID,
				VariableName:    envConfigTemplate.Name,
				VariableKey:     envConfigTemplate.Key,
				SecretReference: secretReference,
			})
		}

		// Short per-env TX: DB writes only.
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			mapping := &models.EnvAgentModelMapping{
				ConfigUUID:          config.UUID,
				EnvironmentUUID:     envUUID,
				LLMProxyUUID:        proxy.UUID,
				PolicyConfiguration: models.LLMPolicies(envMapping.Configuration.Policies),
			}
			if err := s.envMappingRepo.Create(ctx, tx, mapping); err != nil {
				return fmt.Errorf("failed to create environment mapping for %s: %w", envName, err)
			}
			if err := s.envVariableRepo.CreateBatch(ctx, tx, variables); err != nil {
				return fmt.Errorf("failed to create environment variables for %s: %w", envName, err)
			}
			return nil
		}); err != nil {
			// CASCADE on config row will clean up any mappings/variables written for earlier envs.
			s.processRollBack(ctx, rollbackResources, ouID, config.UUID)
			return nil, err
		}

		// Internal-agent only: inject per-env vars into ReleaseBinding.
		// SecretReference is already created by secretClient.CreateSecret above.
		// The Component CR (global, shared across envs) is updated once after the loop using the
		// first-environment's vars to avoid last-write-wins clobbering (HIGH-3).
		if !isExternalAgent {
			// Build the two env vars (URL plain, API key via secretKeyRef).
			envVarsToInject := buildLLMEnvVars(envConfigTemplates, proxyURL, secretRefName)

			// Step 3: Inject per-environment URL and API key ref into the ReleaseBinding.
			// Each environment gets its own ReleaseBinding with the correct per-env proxy URL,
			// avoiding last-write-wins clobbering in the global Component CR.
			if err := s.ocClient.UpdateReleaseBindingEnvVars(ctx, ouID, projectName, agentID, envName, envVarsToInject); err != nil {
				s.logger.Warn("failed to patch ReleaseBinding for env var injection (will apply on next deploy)",
					"environment", envName, "error", err)
			}

			// Step 4: For the first/dev environment, also update the Component CR once as a bootstrap
			// default so agents with no ReleaseBinding yet have a working config.
			if firstEnvName != "" && envName == firstEnvName {
				if err := s.ocClient.UpdateComponentEnvVars(ctx, ouID, projectName, agentID, envVarsToInject); err != nil {
					s.logger.Error("failed to update Component CR env vars for internal agent — Component CR in inconsistent state",
						"environment", envName, "error", err)
				}
			}
		}

		s.logger.Info(
			"Created proxy and deployment for environment",
			"environment", envName,
			"proxy_url", proxyURL,
			"proxy_uuid", proxy.UUID,
		)
	}

	// Phase 2b — Bind AI applications now that every environment has otherwise
	// succeeded; a bind failure here still rolls back everything created above.
	if err := s.flushPendingAppBindings(ctx, pendingAppBindings, &rollbackResources); err != nil {
		s.rollbackProxies(ctx, rollbackResources, ouID)
		s.compensatingDeleteConfig(ctx, config.UUID, ouID)
		return nil, err
	}

	// Phase 3 — Success.
	s.logger.Info(
		"Agent configuration created successfully",
		"config_uuid", config.UUID,
		"config_name", config.Name,
		"agent_id", agentID,
		"ou_id", ouID,
		"project_name", projectName,
		"created_by", createdBy,
		"environment_count", len(req.EnvMappings),
	)

	// Return created configuration with credentials for external agents
	if isExternalAgent {
		return s.buildExternalAgentConfigResponse(ctx, config, envCredentials)
	}
	return s.Get(ctx, config.UUID, ouID, projectName, agentID)
}

func (s *agentConfigurationService) createMCPConfig(ctx context.Context, ouID, projectName, agentID string,
	req models.CreateAgentModelConfigRequest, createdBy string, isExternalAgent bool,
) (*models.AgentModelConfigResponse, error) {
	if s.mcpProxyRepo == nil || s.envMCPMappingRepo == nil || s.mcpProxyService == nil {
		return nil, fmt.Errorf("MCP configuration service is not fully configured")
	}
	if len(req.EnvMappings) == 0 {
		return nil, fmt.Errorf("%w: at least one environment mapping is required", utils.ErrInvalidInput)
	}
	if _, err := s.buildMCPMappingEnvironmentVariables(req.Name, req.EnvironmentVariables); err != nil {
		return nil, errors.Join(utils.ErrInvalidInput, err)
	}

	proxiesByEnv := make(map[string]*models.MCPProxy, len(req.EnvMappings))
	for envName, envMapping := range req.EnvMappings {
		proxyHandle := strings.TrimSpace(envMapping.ProviderName)
		if proxyHandle == "" {
			return nil, fmt.Errorf("%w: MCP proxy is required for environment %s", utils.ErrInvalidInput, envName)
		}
		proxy, err := s.mcpProxyRepo.GetByHandle(ctx, proxyHandle, ouID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("MCP proxy for environment %s not found: %w", envName, utils.ErrMCPProxyNotFound)
			}
			return nil, fmt.Errorf("failed to validate MCP proxy for environment %s: %w", envName, err)
		}
		proxiesByEnv[envName] = proxy
	}

	envs, err := s.infraResourceManager.ListOrgEnvironments(ctx, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	envMap := make(map[string]*models.EnvironmentResponse)
	for _, env := range envs {
		envMap[env.Name] = env
	}
	for envName := range req.EnvMappings {
		if _, exists := envMap[envName]; !exists {
			return nil, fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, envName)
		}
	}

	config := &models.AgentConfiguration{
		Name:        req.Name,
		Description: req.Description,
		AgentID:     agentID,
		TypeID:      models.AgentConfigTypeIDMCP,
		OUID:        ouID,
		ProjectName: projectName,
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return s.agentConfigRepo.Create(ctx, tx, config)
	}); err != nil {
		if errors.Is(err, utils.ErrAgentConfigAlreadyExists) {
			return nil, utils.ErrAgentConfigAlreadyExists
		}
		return nil, fmt.Errorf("failed to create MCP configuration: %w", err)
	}

	firstEnvName := ""
	if !isExternalAgent {
		if pipeline, pipelineErr := s.ocClient.GetProjectDeploymentPipeline(ctx, ouID, projectName); pipelineErr == nil && pipeline != nil {
			firstEnvName = client.FindFirstEnvironment(pipeline.PromotionPaths)
		}
	}

	var envCredentials map[string]envCredentialData
	if isExternalAgent {
		envCredentials = make(map[string]envCredentialData)
	}

	// AI application bindings are deferred until every environment below
	// succeeds — see flushPendingAppBindings.
	var pendingAppBindings []pendingAppBinding

	for envName := range req.EnvMappings {
		env := envMap[envName]
		envUUID, err := uuid.Parse(env.UUID)
		if err != nil {
			s.cleanupMCPConfig(ctx, config.UUID, ouID)
			return nil, fmt.Errorf("invalid environment id %q: %w", envName, err)
		}
		sourceProxy := proxiesByEnv[envName]

		envTemplates, err := s.buildMCPMappingEnvironmentVariables(config.Name, req.EnvironmentVariables)
		if err != nil {
			s.cleanupMCPConfig(ctx, config.UUID, ouID)
			return nil, fmt.Errorf("failed to build MCP environment variables for %s: %w", envName, err)
		}

		// The proxy is selected for the agent regardless of environment, but it is only
		// deployable in some of them. Elsewhere, create no mapping/deployment and inject empty
		// env vars.
		gateway, gwErr := s.resolveDeployableMCPGateway(ctx, sourceProxy, ouID, envUUID)
		if gwErr != nil {
			if !errors.Is(gwErr, errMCPEnvNotDeployable) {
				s.cleanupMCPConfig(ctx, config.UUID, ouID)
				return nil, fmt.Errorf("failed to resolve gateway for MCP environment %s: %w", envName, gwErr)
			}
			if err := s.provisionUnconfiguredMCPEnv(ctx, config, envUUID, envName, ouID, projectName, agentID,
				envTemplates, isExternalAgent, firstEnvName, envCredentials); err != nil {
				s.cleanupMCPConfig(ctx, config.UUID, ouID)
				return nil, err
			}
			continue
		}

		handle := mcpMappingProxyName(projectName, agentID, config.Name, envName)
		artifactName := handle
		sourceProxyVersion := mcpProxyArtifactVersion(sourceProxy)
		mapping := &models.EnvAgentMCPMapping{
			ConfigUUID:      config.UUID,
			EnvironmentUUID: envUUID,
			MCPProxyUUID:    sourceProxy.UUID,
			ArtifactUUID:    uuid.New(),
		}
		deployedProxy := buildAgentMCPConfigProxy(config, mapping, sourceProxy, envName, ouID, handle)
		proxyMapping := buildMCPProxyMapping(sourceProxy.UUID, deployedProxy)
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			if err := s.envMCPMappingRepo.Create(ctx, tx, mapping, proxyMapping, handle, artifactName, sourceProxyVersion, ouID); err != nil {
				return err
			}
			return nil
		}); err != nil {
			s.cleanupMCPConfig(ctx, config.UUID, ouID)
			return nil, err
		}

		// The agent configuration deploys nothing: the proxy already deployed the single
		// gateway artifact for this environment. We only mint the per-agent inbound key
		// (against the shared artifact) and inject the env vars pointing at its URL.
		sharedArtifactUUID := mcpProxyEnvArtifactUUID(sourceProxy, env.UUID)
		scopedID := scopedProxyIdentifier(config.ProjectName, config.AgentID, config.Name, env.Name)
		// Only provision an inbound API key when the source MCP proxy has api-key
		// security enabled. When disabled, no gateway key / app binding is created and no
		// apikey env var is injected (only the URL).
		secured := mcpProxyAPIKeySecurityEnabled(sourceProxy, mapping.EnvironmentUUID.String())
		var proxyAPIKey *models.CreateAPIKeyResponse
		var proxySecretLoc secretmanagersvc.SecretLocation
		secretRefName := ""
		if secured {
			var err error
			proxyAPIKey, err = s.createMCPMappingAPIKey(ctx, ouID, sharedArtifactUUID, mapping.ArtifactUUID, fmt.Sprintf("%s-key", scopedID))
			if err != nil {
				s.cleanupMCPConfig(ctx, config.UUID, ouID)
				return nil, fmt.Errorf("failed to generate MCP API key for environment %s: %w", envName, err)
			}
			// Ensure one AI application exists per agent+env and bind this key —
			// deferred until every environment succeeds (flushPendingAppBindings).
			agentAppHandle := agentAppIdentifier(config.ProjectName, config.AgentID, env.Name)
			pendingAppBindings = append(pendingAppBindings, pendingAppBinding{
				ouID: ouID, projectName: config.ProjectName, agentID: config.AgentID, envName: env.Name,
				appHandle:  agentAppHandle,
				appName:    fmt.Sprintf("%s Application", config.AgentID),
				apiKeyUUID: proxyAPIKey.KeyID,
			})
			proxySecretLoc = secretmanagersvc.SecretLocation{
				OrgName:         ouID,
				ProjectName:     projectName,
				AgentName:       agentID,
				EnvironmentName: env.Name,
				ConfigName:      config.Name,
				EntityName:      fmt.Sprintf("%s-proxy", scopedID),
				SecretKey:       secretmanagersvc.SecretKeyAPIKey,
			}
			secretRefName, err = s.secretClient.CreateSecret(ctx, proxySecretLoc,
				map[string]string{secretmanagersvc.SecretKeyAPIKey: proxyAPIKey.APIKey})
			if err != nil {
				if revokeErr := s.revokeMCPMappingAPIKey(ctx, ouID, sharedArtifactUUID, mapping.ArtifactUUID, proxyAPIKey.KeyID); revokeErr != nil {
					s.logger.Warn("failed to revoke MCP API key after secret persistence failure", "environment", envName, "error", revokeErr)
				}
				s.cleanupMCPConfig(ctx, config.UUID, ouID)
				return nil, fmt.Errorf("failed to store MCP API key in KV for environment %s: %w", envName, err)
			}
		}
		variables := make([]models.AgentEnvConfigVariable, 0, len(envTemplates))
		for _, envTemplate := range envTemplates {
			secretReference := ""
			if envTemplate.IsSecret {
				secretReference = secretRefName
			}
			variables = append(variables, models.AgentEnvConfigVariable{
				ConfigUUID:      config.UUID,
				EnvironmentUUID: envUUID,
				VariableName:    envTemplate.Name,
				VariableKey:     envTemplate.Key,
				SecretReference: secretReference,
			})
		}
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			return s.envVariableRepo.CreateBatch(ctx, tx, variables)
		}); err != nil {
			if secured {
				if delErr := s.secretClient.DeleteSecret(ctx, proxySecretLoc, secretRefName); delErr != nil {
					s.logger.Warn("failed to delete MCP API key secret after env var persistence failure", "environment", envName, "error", delErr)
				}
				if revokeErr := s.revokeMCPMappingAPIKey(ctx, ouID, sharedArtifactUUID, mapping.ArtifactUUID, proxyAPIKey.KeyID); revokeErr != nil {
					s.logger.Warn("failed to revoke MCP API key after env var persistence failure", "environment", envName, "error", revokeErr)
				}
			}
			s.cleanupMCPConfig(ctx, config.UUID, ouID)
			return nil, fmt.Errorf("failed to create MCP environment variables for %s: %w", envName, err)
		}

		proxyURL := buildMCPProxyURL(gateway, deployedProxy.Configuration)
		if isExternalAgent {
			apiKey := ""
			if proxyAPIKey != nil {
				apiKey = proxyAPIKey.APIKey
			}
			envCredentials[envUUID.String()] = envCredentialData{
				apiKey:   apiKey,
				proxyURL: proxyURL,
			}
		}
		if !isExternalAgent {
			envVarsToInject := buildMCPEnvVars(envTemplates, proxyURL, secretRefName)
			if err := s.ocClient.UpdateReleaseBindingEnvVars(ctx, ouID, projectName, agentID, envName, envVarsToInject); err != nil {
				s.logger.Warn("failed to patch ReleaseBinding for MCP env var injection", "environment", envName, "error", err)
			}
			if firstEnvName != "" && envName == firstEnvName {
				if err := s.ocClient.UpdateComponentEnvVars(ctx, ouID, projectName, agentID, envVarsToInject); err != nil {
					s.logger.Warn("failed to patch Component for MCP env var bootstrap", "environment", envName, "error", err)
				}
			}
		}
	}

	// Bind AI applications now that every environment has otherwise succeeded;
	// a bind failure here still cleans up everything created above.
	var mcpAppRollback []rollbackResource
	if err := s.flushPendingAppBindings(ctx, pendingAppBindings, &mcpAppRollback); err != nil {
		// mcpAppRollback holds any AI applications a partially-successful flush
		// already created — cleanupMCPConfig only tears down the MCP config
		// itself, so those apps must be rolled back separately or they leak.
		s.rollbackProxies(ctx, mcpAppRollback, ouID)
		s.cleanupMCPConfig(ctx, config.UUID, ouID)
		return nil, err
	}

	// A newly-created MCP config changes the agent's AgentID scope union just as
	// much as editing an existing one does (see updateMCPConfig's identical call)
	// — refresh every environment this config was just bound to so an
	// already-running pod picks up the new scopes right away instead of waiting
	// for its next deploy/promote/rotation. refreshTouchedMCPEnvironments already
	// no-ops safely for external/unprovisioned agents, so this is safe to call
	// unconditionally rather than duplicating the isExternalAgent branch here.
	touchedEnvNames := make(map[string]struct{}, len(req.EnvMappings))
	for envName := range req.EnvMappings {
		touchedEnvNames[envName] = struct{}{}
	}
	go func() {
		refreshCtx, cancel := detachedRefreshContext(ctx)
		defer cancel()
		s.refreshTouchedMCPEnvironments(refreshCtx, ouID, projectName, agentID, touchedEnvNames)
	}()

	if isExternalAgent {
		return s.buildExternalAgentConfigResponse(ctx, config, envCredentials)
	}
	return s.GetMCP(ctx, config.UUID, ouID, projectName, agentID)
}

// Get retrieves a configuration by UUID with project and agent scoping validation
func (s *agentConfigurationService) Get(ctx context.Context, configUUID uuid.UUID, ouID, projectName, agentName string) (*models.AgentModelConfigResponse, error) {
	config, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentConfigNotFound
		}
		return nil, fmt.Errorf("failed to get configuration: %w", err)
	}

	// Validate project and agent scoping
	if config.ProjectName != projectName || config.AgentID != agentName {
		return nil, utils.ErrAgentConfigNotFound
	}

	// Check if agent is external
	agent, err := s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
	if err != nil {
		// If we can't determine agent type, assume internal (safer default)
		s.logger.Warn("Failed to get agent type, assuming internal", "error", err)
		return s.buildConfigResponse(ctx, config, false)
	}
	isExternal := agent.Provisioning.Type == string(utils.ExternalAgent)

	return s.buildConfigResponse(ctx, config, isExternal)
}

// GetMCP retrieves an MCP proxy mapping by UUID with project and agent scoping validation.
func (s *agentConfigurationService) GetMCP(ctx context.Context, configUUID uuid.UUID, ouID, projectName, agentName string) (*models.AgentModelConfigResponse, error) {
	config, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentConfigNotFound
		}
		return nil, fmt.Errorf("failed to get MCP configuration: %w", err)
	}
	if config.ProjectName != projectName || config.AgentID != agentName || config.TypeID != models.AgentConfigTypeIDMCP {
		return nil, utils.ErrAgentConfigNotFound
	}

	agent, err := s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
	if err != nil {
		s.logger.Warn("Failed to get agent type, assuming internal", "error", err)
		return s.buildConfigResponse(ctx, config, false)
	}
	isExternal := agent.Provisioning.Type == string(utils.ExternalAgent)
	return s.buildConfigResponse(ctx, config, isExternal)
}

// GetByAgent retrieves configuration by agent ID
func (s *agentConfigurationService) GetByAgent(ctx context.Context, agentID, ouID string) (*models.AgentModelConfigResponse, error) {
	config, err := s.agentConfigRepo.GetByAgentID(ctx, agentID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentConfigNotFound
		}
		return nil, fmt.Errorf("failed to get configuration: %w", err)
	}

	// Check if agent is external
	agent, err := s.ocClient.GetComponent(ctx, ouID, config.ProjectName, agentID)
	if err != nil {
		// If we can't determine agent type, assume internal (safer default)
		s.logger.Warn("Failed to get agent type, assuming internal", "error", err)
		return s.buildConfigResponse(ctx, config, false)
	}
	isExternal := agent.Provisioning.Type == string(utils.ExternalAgent)

	return s.buildConfigResponse(ctx, config, isExternal)
}

// List lists all configurations for an organization, project, and agent
func (s *agentConfigurationService) List(ctx context.Context, ouID, projectName, agentName string, limit, offset int) (*models.AgentModelConfigListResponse, error) {
	configs, err := s.agentConfigRepo.ListByAgent(ctx, ouID, projectName, agentName, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list configurations: %w", err)
	}

	count, err := s.agentConfigRepo.CountByAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		return nil, fmt.Errorf("failed to count configurations: %w", err)
	}

	items := make([]models.AgentModelConfigListItem, len(configs))
	for i, cfg := range configs {
		items[i] = models.AgentModelConfigListItem{
			UUID:        cfg.UUID.String(),
			Name:        cfg.Name,
			Description: cfg.Description,
			AgentID:     cfg.AgentID,
			Type:        models.AgentConfigTypeFromID(cfg.TypeID),
			ProjectName: cfg.ProjectName,
			CreatedAt:   cfg.CreatedAt,
		}
	}

	return &models.AgentModelConfigListResponse{
		Configs: items,
		Pagination: models.PaginationInfo{
			Count:  int(count),
			Offset: offset,
			Limit:  limit,
		},
	}, nil
}

// ListByType lists configurations for an organization, project, agent, and config type.
func (s *agentConfigurationService) ListByType(
	ctx context.Context, ouID, projectName, agentName string, typeID uint, limit, offset int,
) (*models.AgentModelConfigListResponse, error) {
	configs, err := s.agentConfigRepo.ListByAgentAndType(ctx, ouID, projectName, agentName, typeID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list configurations by type: %w", err)
	}

	count, err := s.agentConfigRepo.CountByAgentAndType(ctx, ouID, projectName, agentName, typeID)
	if err != nil {
		return nil, fmt.Errorf("failed to count configurations by type: %w", err)
	}

	items := make([]models.AgentModelConfigListItem, len(configs))
	for i, cfg := range configs {
		items[i] = models.AgentModelConfigListItem{
			UUID:        cfg.UUID.String(),
			Name:        cfg.Name,
			Description: cfg.Description,
			AgentID:     cfg.AgentID,
			Type:        models.AgentConfigTypeFromID(cfg.TypeID),
			ProjectName: cfg.ProjectName,
			CreatedAt:   cfg.CreatedAt,
		}
	}

	return &models.AgentModelConfigListResponse{
		Configs: items,
		Pagination: models.PaginationInfo{
			Count:  int(count),
			Offset: offset,
			Limit:  limit,
		},
	}, nil
}

// ListMCP lists all MCP proxy mappings for an organization, project, and agent.
func (s *agentConfigurationService) ListMCP(ctx context.Context, ouID, projectName, agentName string, limit, offset int) (*models.AgentModelConfigListResponse, error) {
	return s.ListByType(ctx, ouID, projectName, agentName, models.AgentConfigTypeIDMCP, limit, offset)
}

// processEnvProviderChange handles Scenario A: provider changed for an existing environment.
// External ops run outside any transaction; a short per-env TX follows.
// Returns the old proxy handle (for later cleanup) and the rollback resource for the new proxy.
func (s *agentConfigurationService) processEnvProviderChange(
	ctx context.Context,
	configUUID uuid.UUID,
	config *models.AgentConfiguration,
	env *models.EnvironmentResponse,
	envUUID uuid.UUID,
	envName string,
	envMapping models.EnvModelConfigRequest,
	existingMapping *models.EnvAgentModelMapping,
	ouID string,
	existingVarNames map[string]string,
	isExternalAgent bool,
	firstEnvName string,
) (oldProxyHandle string, rbRes rollbackResource, pendingBind pendingAppBinding, err error) {
	s.logger.Info("Provider changed for environment, recreating proxy",
		"environment", envName,
		"old_provider_uuid", existingMapping.LLMProxy.Configuration.Provider,
		"new_provider_name", envMapping.ProviderName)

	proxyConfig, providerAPIKeyID, providerUUID, providerSecretLoc, scopedID, err := s.buildLLMProxyConfig(ctx, config, env.Name, envMapping)
	if err != nil {
		return "", rollbackResource{}, pendingAppBinding{}, fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
	}

	// Resolve gateway where the new provider is deployed
	gateway, err := s.resolveGatewayForProvider(ctx, providerUUID, ouID, envUUID)
	if err != nil {
		return "", rollbackResource{}, pendingAppBinding{}, fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, err)
	}

	// Register provider credentials immediately so they are cleaned up on any subsequent failure.
	rbRes = rollbackResource{
		providerAPIKeyID:  providerAPIKeyID,
		providerUUID:      providerUUID,
		providerSecretLoc: providerSecretLoc,
		mappingID:         existingMapping.ID,
		oldProxyUUID:      existingMapping.LLMProxyUUID,
	}

	proxy, err := s.llmProxyService.Create(ctx, ouID, models.UserRoleSystem, proxyConfig)
	if err != nil {
		return "", rbRes, pendingAppBinding{}, fmt.Errorf("failed to create proxy for environment %s: %w", envName, err)
	}
	rbRes.proxyHandle = proxy.Handle

	deployment, err := s.llmProxyDeploymentService.DeployLLMProxy(ctx, proxy.Handle, &models.DeployAPIRequest{
		Name:      fmt.Sprintf("%s-deployment", scopedID),
		Base:      "current",
		GatewayID: gateway.UUID.String(),
	}, ouID)
	if err != nil {
		return "", rbRes, pendingAppBinding{}, fmt.Errorf("failed to deploy proxy for environment %s: %w", envName, err)
	}
	rbRes.deploymentID = deployment.DeploymentID
	rbRes.gatewayID = gateway.UUID.String()

	proxyAPIKey, err := s.llmProxyAPIKeyService.CreateAPIKey(ctx, ouID, proxy.Handle, &models.CreateAPIKeyRequest{
		Name:    fmt.Sprintf("%s-key", scopedID),
		Purpose: agentProxyAPIKeyPurpose(isExternalAgent),
	})
	if err != nil {
		return "", rbRes, pendingAppBinding{}, fmt.Errorf("failed to generate API key for environment %s: %w", envName, err)
	}
	rbRes.proxyAPIKeyID = proxyAPIKey.KeyID

	// Ensure one AI application exists per agent+env and bind the proxy API key
	// — deferred until every environment succeeds (flushPendingAppBindings in Update).
	agentAppHandle := agentAppIdentifier(config.ProjectName, config.AgentID, envName)
	pendingBind = pendingAppBinding{
		ouID: ouID, projectName: config.ProjectName, agentID: config.AgentID, envName: envName,
		appHandle:  agentAppHandle,
		appName:    fmt.Sprintf("%s Application", config.AgentID),
		apiKeyUUID: proxyAPIKey.KeyID,
	}

	// Store proxy API key via the secret management client (provider manages the SecretReference)
	proxySecretLoc := secretmanagersvc.SecretLocation{
		OrgName:         ouID,
		ProjectName:     config.ProjectName,
		AgentName:       config.AgentID,
		EnvironmentName: env.Name,
		ConfigName:      config.Name,
		EntityName:      proxy.Handle,
		SecretKey:       secretmanagersvc.SecretKeyAPIKey,
	}
	secretRefName, err := s.secretClient.CreateSecret(ctx, proxySecretLoc,
		map[string]string{secretmanagersvc.SecretKeyAPIKey: proxyAPIKey.APIKey})
	if err != nil {
		s.rollbackProxies(ctx, []rollbackResource{rbRes}, ouID)
		return "", rollbackResource{}, pendingAppBinding{}, fmt.Errorf("processEnvProviderChange: failed to store proxy API key in KV for environment %s: %w", envName, err)
	}
	rbRes.proxySecretLoc = &proxySecretLoc
	rbRes.secretRefName = secretRefName

	envConfigTemplates, err := s.buildEnvironmentVariables(config.Name, varNamesToOverrides(existingVarNames))
	if err != nil {
		s.rollbackProxies(ctx, []rollbackResource{rbRes}, ouID)
		return "", rollbackResource{}, pendingAppBinding{}, fmt.Errorf("failed to build environment variables for %s: %w", envName, err)
	}
	variables := []models.AgentEnvConfigVariable{}
	for _, envConfigTemplate := range envConfigTemplates {
		secretReference := ""
		if envConfigTemplate.IsSecret {
			secretReference = secretRefName
		}
		variables = append(variables, models.AgentEnvConfigVariable{
			ConfigUUID:      config.UUID,
			EnvironmentUUID: envUUID,
			VariableName:    envConfigTemplate.Name,
			VariableKey:     envConfigTemplate.Key,
			SecretReference: secretReference,
		})
	}

	// Capture the old proxy's handle before the association gets repointed below.
	if existingMapping.LLMProxy != nil {
		oldProxyHandle = existingMapping.LLMProxy.Handle
	}

	// Short per-env TX: DB writes only.
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		existingMapping.LLMProxyUUID = proxy.UUID
		// Keep the LLMProxy association in sync with the FK. existingMapping.LLMProxy
		// still points to the OLD proxy struct; GORM's Save() on a belongs-to
		// association re-derives the FK column from the association's own primary
		// key, which would otherwise silently revert LLMProxyUUID back to the old
		// proxy right after we set it. Combined with the ON DELETE CASCADE on
		// fk_env_mapping_proxy, that reverted FK means this mapping row gets
		// cascade-deleted outright once Phase 5 cleans up the old (still-referenced)
		// proxy — the config ends up with no LLM provider configured at all.
		existingMapping.LLMProxy = proxy
		if err := s.envMappingRepo.Update(ctx, tx, existingMapping); err != nil {
			return fmt.Errorf("failed to update environment mapping for %s: %w", envName, err)
		}
		if err := s.envVariableRepo.DeleteByConfigAndEnv(ctx, tx, configUUID, envUUID); err != nil {
			return fmt.Errorf("failed to delete old environment variables for %s: %w", envName, err)
		}
		if err := s.envVariableRepo.CreateBatch(ctx, tx, variables); err != nil {
			return fmt.Errorf("failed to create environment variables for %s: %w", envName, err)
		}
		return nil
	}); err != nil {
		return "", rbRes, pendingAppBinding{}, err
	}

	// Internal-agent only: inject env vars into ReleaseBinding/Component.
	// SecretReference is already created/updated by secretClient.CreateSecret above.
	//
	// The ReleaseBinding for THIS environment is the target — the proxy URL and API key ref are
	// per-environment, and the Component CR is component-wide, so writing them there for anything
	// but a bootstrap default is last-write-wins across environments. The Component CR write is
	// therefore scoped to the first environment, matching every other injection site in this file.
	if !isExternalAgent {
		proxyURL := buildProxyURL(gateway, proxy.Configuration.Context)
		envVarsToInject := buildLLMEnvVars(envConfigTemplates, proxyURL, secretRefName)
		if rbErr := s.ocClient.UpdateReleaseBindingEnvVars(ctx, ouID, config.ProjectName, config.AgentID, envName, envVarsToInject); rbErr != nil {
			s.logger.Error("failed to patch ReleaseBinding in Scenario A", "env", envName, "err", rbErr)
			return "", rbRes, pendingAppBinding{}, fmt.Errorf("failed to update release binding env vars for environment %s: %w", envName, rbErr)
		}
		// Bootstrap default for agents with no ReleaseBinding yet.
		if firstEnvName != "" && envName == firstEnvName {
			if uvErr := s.ocClient.UpdateComponentEnvVars(ctx, ouID, config.ProjectName, config.AgentID, envVarsToInject); uvErr != nil {
				s.logger.Error("failed to update Component CR env vars in Scenario A — Component CR in inconsistent state", "env", envName, "err", uvErr)
			}
		}
	}

	return oldProxyHandle, rbRes, pendingBind, nil
}

// processEnvProxyUpdate handles Scenario B: same provider, update proxy config and redeploy.
// No DB TX needed — mapping already points to the same proxy UUID.
// Returns a non-nil rollback resource only if a new providerAPIKeyID was created.
func (s *agentConfigurationService) processEnvProxyUpdate(
	ctx context.Context,
	config *models.AgentConfiguration,
	env *models.EnvironmentResponse,
	envUUID uuid.UUID,
	envName string,
	envMapping models.EnvModelConfigRequest,
	existingMapping *models.EnvAgentModelMapping,
	ouID string,
) (rollbackResource, error) {
	s.logger.Info("Updating proxy configuration for environment",
		"environment", envName,
		"provider_name", envMapping.ProviderName)

	if existingMapping.LLMProxy == nil {
		return rollbackResource{}, fmt.Errorf("existing proxy not found for environment %s", envName)
	}

	gateway, err := s.resolveGatewayForProxy(ctx, existingMapping.LLMProxy.Handle, ouID, envUUID)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, err)
	}

	proxyConfig, providerUUID, err := s.buildLLMProxyUpdateConfig(config, envMapping, existingMapping.LLMProxy)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
	}

	// LLMProxy.Handle is gorm:"-"; the repository backfills it from Configuration.Name.
	// Use the existing proxy's handle rather than recomputing it, so the proxy identity
	// is preserved exactly as created.
	proxyHandle := existingMapping.LLMProxy.Handle
	proxyConfig.UUID = existingMapping.LLMProxy.UUID
	proxyConfig.Handle = proxyHandle
	proxyConfig.CreatedBy = existingMapping.LLMProxy.CreatedBy
	proxyConfig.Status = existingMapping.LLMProxy.Status

	updatedProxy, err := s.llmProxyService.Update(proxyHandle, ouID, proxyConfig)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to update proxy for environment %s: %w", envName, err)
	}

	// The proxy's configuration was already persisted by the Update call above,
	// so every failure from here on must carry existingMapping.LLMProxy (the
	// pre-update snapshot) back to the caller — otherwise rollbackProxies has
	// no way to restore it and the proxy is left holding the new config with
	// no matching deployment.
	gatewayID := gateway.UUID.String()
	deployments, err := s.llmProxyDeploymentService.GetLLMProxyDeployments(updatedProxy.Handle, ouID, &gatewayID, nil)
	if err != nil {
		return rollbackResource{
			proxyHandle:      proxyHandle,
			priorProxyConfig: existingMapping.LLMProxy,
			providerUUID:     providerUUID,
		}, fmt.Errorf("failed to get deployments for environment %s: %w", envName, err)
	}

	var existingDeployment *models.Deployment
	for _, dep := range deployments {
		if dep.Status != nil && *dep.Status == models.DeploymentStatusDeployed {
			existingDeployment = dep
			break
		}
	}

	deployBase := "current"
	scopedID := scopedProxyIdentifier(config.ProjectName, config.AgentID, config.Name, env.Name)
	newDeployment, err := s.llmProxyDeploymentService.DeployLLMProxy(ctx, updatedProxy.Handle, &models.DeployAPIRequest{
		Name:      fmt.Sprintf("%s-deployment", scopedID),
		Base:      deployBase,
		GatewayID: gateway.UUID.String(),
	}, ouID)
	if err != nil {
		return rollbackResource{
			proxyHandle:      proxyHandle,
			priorProxyConfig: existingMapping.LLMProxy,
			providerUUID:     providerUUID,
		}, fmt.Errorf("failed to redeploy proxy for environment %s: %w", envName, err)
	}

	s.logger.Info("Proxy configuration updated and redeployed",
		"environment", envName,
		"proxy_handle", updatedProxy.Handle,
		"new_deployment_id", newDeployment.DeploymentID)

	// Persist updated PolicyConfiguration to DB.
	existingMapping.PolicyConfiguration = models.LLMPolicies(envMapping.Configuration.Policies)
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return s.envMappingRepo.Update(ctx, tx, existingMapping)
	}); err != nil {
		// Carry the new deployment's identity and the pre-update proxy snapshot
		// back to the caller so rollbackProxies can remove the deployment that
		// was just created and restore the proxy's prior configuration. Deploying
		// newDeployment already superseded existingDeployment's Deployed status
		// (see restoreDeploymentID doc comment), so its ID must travel too or the
		// proxy is left with no live deployment after rollback.
		var restoreDeploymentID uuid.UUID
		if existingDeployment != nil {
			restoreDeploymentID = existingDeployment.DeploymentID
		}
		return rollbackResource{
			proxyHandle:         proxyHandle,
			deploymentID:        newDeployment.DeploymentID,
			gatewayID:           gateway.UUID.String(),
			priorProxyConfig:    existingMapping.LLMProxy,
			providerUUID:        providerUUID,
			restoreDeploymentID: restoreDeploymentID,
		}, fmt.Errorf("failed to update policy configuration for environment %s: %w", envName, err)
	}

	if existingDeployment != nil && existingDeployment.DeploymentID != newDeployment.DeploymentID {
		if err := s.llmProxyDeploymentService.DeleteLLMProxyDeployment(updatedProxy.Handle, existingDeployment.DeploymentID.String(), ouID); err != nil {
			s.logger.Warn("Failed to clean up old deployment after redeployment",
				"environment", envName,
				"old_deployment_id", existingDeployment.DeploymentID,
				"error", err)
		}
	}

	// Scenario B preserves the proxy handle and context path, so the proxy URL and secret reference
	// are identical to what is already injected. Skip Component CR and ReleaseBinding updates to
	// avoid triggering an unnecessary agent pod restart.

	return rollbackResource{providerUUID: providerUUID}, nil
}

// processNewEnv handles Scenario C: new environment added during update.
// Mirrors Create() per-env logic: external ops then a short per-env TX.
func (s *agentConfigurationService) processNewEnv(
	ctx context.Context,
	configUUID uuid.UUID,
	config *models.AgentConfiguration,
	env *models.EnvironmentResponse,
	envUUID uuid.UUID,
	envName string,
	envMapping models.EnvModelConfigRequest,
	ouID string,
	existingVarNames map[string]string,
	isExternalAgent bool,
	firstEnvName string,
) (rollbackResource, pendingAppBinding, error) {
	s.logger.Info("Adding new environment to configuration",
		"environment", envName,
		"provider_name", envMapping.ProviderName)

	proxyConfig, providerAPIKeyID, providerUUID, providerSecretLoc, scopedID, err := s.buildLLMProxyConfig(ctx, config, env.Name, envMapping)
	if err != nil {
		return rollbackResource{}, pendingAppBinding{}, fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
	}

	// Resolve gateway where the provider is deployed
	gateway, err := s.resolveGatewayForProvider(ctx, providerUUID, ouID, envUUID)
	if err != nil {
		return rollbackResource{}, pendingAppBinding{}, fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, err)
	}

	// Register provider credentials immediately so they are cleaned up on any subsequent failure.
	rbRes := rollbackResource{providerAPIKeyID: providerAPIKeyID, providerUUID: providerUUID, providerSecretLoc: providerSecretLoc}

	proxy, err := s.llmProxyService.Create(ctx, ouID, models.UserRoleSystem, proxyConfig)
	if err != nil {
		return rbRes, pendingAppBinding{}, fmt.Errorf("failed to create proxy for environment %s: %w", envName, err)
	}
	rbRes.proxyHandle = proxy.Handle

	deployment, err := s.llmProxyDeploymentService.DeployLLMProxy(ctx, proxy.Handle, &models.DeployAPIRequest{
		Name:      fmt.Sprintf("%s-deployment", scopedID),
		Base:      "current",
		GatewayID: gateway.UUID.String(),
	}, ouID)
	if err != nil {
		return rbRes, pendingAppBinding{}, fmt.Errorf("failed to deploy proxy for environment %s: %w", envName, err)
	}
	rbRes.deploymentID = deployment.DeploymentID
	rbRes.gatewayID = gateway.UUID.String()

	proxyAPIKey, err := s.llmProxyAPIKeyService.CreateAPIKey(ctx, ouID, proxy.Handle, &models.CreateAPIKeyRequest{
		Name:    fmt.Sprintf("%s-key", scopedID),
		Purpose: agentProxyAPIKeyPurpose(isExternalAgent),
	})
	if err != nil {
		return rbRes, pendingAppBinding{}, fmt.Errorf("failed to generate API key for environment %s: %w", envName, err)
	}
	rbRes.proxyAPIKeyID = proxyAPIKey.KeyID

	// Ensure one AI application exists per agent+env and bind the proxy API key
	// — deferred until every environment succeeds (flushPendingAppBindings in Update).
	agentAppHandle := agentAppIdentifier(config.ProjectName, config.AgentID, envName)
	pendingBind := pendingAppBinding{
		ouID: ouID, projectName: config.ProjectName, agentID: config.AgentID, envName: envName,
		appHandle:  agentAppHandle,
		appName:    fmt.Sprintf("%s Application", config.AgentID),
		apiKeyUUID: proxyAPIKey.KeyID,
	}

	// Store proxy API key via the secret management client (provider manages the SecretReference)
	proxySecretLoc := secretmanagersvc.SecretLocation{
		OrgName:         ouID,
		ProjectName:     config.ProjectName,
		AgentName:       config.AgentID,
		EnvironmentName: env.Name,
		ConfigName:      config.Name,
		EntityName:      proxy.Handle,
		SecretKey:       secretmanagersvc.SecretKeyAPIKey,
	}
	secretRefName, err := s.secretClient.CreateSecret(ctx, proxySecretLoc,
		map[string]string{secretmanagersvc.SecretKeyAPIKey: proxyAPIKey.APIKey})
	if err != nil {
		s.rollbackProxies(ctx, []rollbackResource{rbRes}, ouID)
		return rollbackResource{}, pendingAppBinding{}, fmt.Errorf("processNewEnv: failed to store proxy API key in KV for environment %s: %w", envName, err)
	}
	rbRes.proxySecretLoc = &proxySecretLoc
	rbRes.secretRefName = secretRefName

	envConfigTemplates, err := s.buildEnvironmentVariables(config.Name, varNamesToOverrides(existingVarNames))
	if err != nil {
		s.rollbackProxies(ctx, []rollbackResource{rbRes}, ouID)
		return rollbackResource{}, pendingAppBinding{}, fmt.Errorf("failed to build environment variables for %s: %w", envName, err)
	}
	variables := []models.AgentEnvConfigVariable{}
	for _, envConfigTemplate := range envConfigTemplates {
		secretReference := ""
		if envConfigTemplate.IsSecret {
			secretReference = secretRefName
		}
		variables = append(variables, models.AgentEnvConfigVariable{
			ConfigUUID:      config.UUID,
			EnvironmentUUID: envUUID,
			VariableName:    envConfigTemplate.Name,
			VariableKey:     envConfigTemplate.Key,
			SecretReference: secretReference,
		})
	}

	// Short per-env TX: DB writes only.
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		mapping := &models.EnvAgentModelMapping{
			ConfigUUID:      configUUID,
			EnvironmentUUID: envUUID,
			LLMProxyUUID:    proxy.UUID,
		}
		if err := s.envMappingRepo.Create(ctx, tx, mapping); err != nil {
			return fmt.Errorf("failed to create environment mapping for %s: %w", envName, err)
		}
		if err := s.envVariableRepo.CreateBatch(ctx, tx, variables); err != nil {
			return fmt.Errorf("failed to create environment variables for %s: %w", envName, err)
		}
		return nil
	}); err != nil {
		return rbRes, pendingAppBinding{}, err
	}

	// Internal-agent only: inject per-env vars into ReleaseBinding.
	// SecretReference is already created by secretClient.CreateSecret above.
	// The Component CR (global) is updated only for the first/dev environment to avoid
	// last-write-wins clobbering across multiple environments (HIGH-3).
	if !isExternalAgent {
		// Reuse the gateway already resolved for deployment (resolveGatewayForProvider)
		proxyURL := buildProxyURL(gateway, proxy.Configuration.Context)

		envVarsToInject := buildLLMEnvVars(envConfigTemplates, proxyURL, secretRefName)
		// Inject per-env URL into the ReleaseBinding for this specific environment.
		if rbErr := s.ocClient.UpdateReleaseBindingEnvVars(ctx, ouID, config.ProjectName, config.AgentID, envName, envVarsToInject); rbErr != nil {
			s.logger.Warn("failed to patch ReleaseBinding in Scenario C", "env", envName, "error", rbErr)
		}
		// Update Component CR only for the first/dev environment as a bootstrap default.
		if firstEnvName != "" && envName == firstEnvName {
			if uvErr := s.ocClient.UpdateComponentEnvVars(ctx, ouID, config.ProjectName, config.AgentID, envVarsToInject); uvErr != nil {
				s.logger.Error("failed to update Component CR env vars in Scenario C — Component CR in inconsistent state", "env", envName, "error", uvErr)
			}
		}
	}

	return rbRes, pendingBind, nil
}

// processEnvRemoval handles Scenario D: environment removed from the request.
// Removes env vars from the ReleaseBinding and, only when this is the last
// remaining environment (isLastEnv == true), also clears the Component CR.
func (s *agentConfigurationService) processEnvRemoval(
	ctx context.Context,
	configUUID uuid.UUID,
	envUUIDStr string,
	mapping *models.EnvAgentModelMapping,
	configName string,
	envName string,
	ouID string,
	projectName string,
	agentName string,
	isExternalAgent bool,
	existingVarNames map[string]string,
	isLastEnv bool,
) error {
	proxyHandle := "<nil>"
	if mapping.LLMProxy != nil {
		proxyHandle = mapping.LLMProxy.Handle
	}
	s.logger.Info("Removing environment from configuration",
		"environment", envUUIDStr,
		"proxy_handle", proxyHandle)

	envUUIDParsed, err := uuid.Parse(envUUIDStr)
	if err != nil {
		return fmt.Errorf("invalid environment UUID %q: %w", envUUIDStr, err)
	}

	// Internal-agent only: remove env vars from Component CR and the removed environment's ReleaseBinding.
	if !isExternalAgent && envName != "" {
		// Build the list of env var keys from DB-persisted names so user-overridden names are respected.
		envConfigTemplates, buildErr := s.buildEnvironmentVariables(configName, varNamesToOverrides(existingVarNames))
		if buildErr != nil {
			s.logger.Warn("failed to build env var keys for Scenario D cleanup, skipping env var removal", "error", buildErr)
		} else {
			keysToRemove := make([]string, 0, len(envConfigTemplates))
			for _, t := range envConfigTemplates {
				keysToRemove = append(keysToRemove, t.Name)
			}
			// Remove from the removed environment's ReleaseBinding.
			if rbErr := s.ocClient.RemoveReleaseBindingEnvVars(ctx, ouID, projectName, agentName, envName, keysToRemove); rbErr != nil {
				s.logger.Warn("failed to remove env vars from ReleaseBinding in Scenario D", "environment", envName, "error", rbErr)
			}
			// Remove from the Component CR only when this is the last environment.
			// If other environments survive, their ReleaseBindings still hold the
			// correct per-env values and the Component CR should be left intact.
			if isLastEnv {
				if compErr := s.ocClient.RemoveComponentEnvironmentVariables(ctx, ouID, projectName, agentName, keysToRemove); compErr != nil {
					s.logger.Warn("failed to remove env vars from Component CR in Scenario D", "environment", envName, "error", compErr)
				}
			}
		}

		// Delete SecretReference CR after consumer refs have been cleaned up (best-effort).
		// Use the persisted SecretReference from AgentEnvConfigVariable (set at creation time)
		// rather than deriving it from mutable fields like configName which may have been renamed.
		vars, varLoadErr := s.envVariableRepo.ListByConfigAndEnv(ctx, configUUID, envUUIDParsed)
		if varLoadErr != nil {
			s.logger.Warn("failed to load env config variables for SecretReference lookup in Scenario D", "error", varLoadErr)
		} else {
			for _, v := range vars {
				if v.SecretReference != "" {
					s.logger.Info("Scenario D: using persisted SecretReference for deletion",
						"secret_ref", v.SecretReference, "variable_name", v.VariableName,
						"config_uuid", configUUID, "environment", envName)
					if delErr := s.ocClient.DeleteSecretReference(ctx, ouID, v.SecretReference); delErr != nil {
						s.logger.Warn("failed to delete SecretReference in Scenario D", "name", v.SecretReference, "error", delErr)
					}
					break // Only one secret ref per config+env
				}
			}
		}
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.envVariableRepo.DeleteByConfigAndEnv(ctx, tx, configUUID, envUUIDParsed); err != nil {
			return fmt.Errorf("failed to delete environment variables for %s: %w", envUUIDStr, err)
		}
		if err := s.envMappingRepo.Delete(ctx, tx, mapping.ID); err != nil {
			return fmt.Errorf("failed to delete environment mapping for %s: %w", envUUIDStr, err)
		}
		return nil
	})
}

func (s *agentConfigurationService) updateMCPConfig(ctx context.Context, existingConfig *models.AgentConfiguration, ouID, projectName, agentName string,
	req models.UpdateAgentModelConfigRequest,
) (*models.AgentModelConfigResponse, error) {
	if s.mcpProxyRepo == nil || s.envMCPMappingRepo == nil || s.mcpProxyService == nil {
		return nil, fmt.Errorf("MCP configuration service is not fully configured")
	}

	allEnvs, err := s.infraResourceManager.ListOrgEnvironments(ctx, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	envMap := make(map[string]*models.EnvironmentResponse, len(allEnvs))
	uuidToEnvName := make(map[string]string, len(allEnvs))
	for _, e := range allEnvs {
		envMap[e.Name] = e
		uuidToEnvName[e.UUID] = e.Name
	}

	// Resolved before anything is persisted: a failure here aborts the update, and the name
	// and description must not already be written when it does.
	isExternalAgent, firstEnvName, agentErr := s.agentDeploymentShape(ctx, ouID, projectName, agentName)
	if agentErr != nil {
		return nil, agentErr
	}

	nameChanged := req.Name != "" && req.Name != existingConfig.Name
	if req.Name != "" {
		existingConfig.Name = req.Name
	}
	if req.Description != "" {
		existingConfig.Description = req.Description
	}
	if req.Name != "" || req.Description != "" {
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			return s.agentConfigRepo.Update(ctx, tx, existingConfig)
		}); err != nil {
			return nil, fmt.Errorf("failed to update MCP configuration: %w", err)
		}
	}

	if len(req.EnvironmentVariables) > 0 {
		if err := s.updateMCPConfigEnvironmentVariableNames(ctx, existingConfig, ouID, projectName, agentName, uuidToEnvName, isExternalAgent, firstEnvName, req.EnvironmentVariables); err != nil {
			return nil, err
		}
	}

	envTemplates, err := s.mcpEnvTemplatesForConfig(ctx, existingConfig)
	if err != nil {
		return nil, err
	}

	if req.EnvMappings == nil {
		if nameChanged {
			if err := s.refreshAllMCPMappings(ctx, existingConfig, ouID, uuidToEnvName, envTemplates, isExternalAgent, firstEnvName); err != nil {
				return nil, err
			}
		}
		return s.GetMCP(ctx, existingConfig.UUID, ouID, projectName, agentName)
	}

	proxiesByEnv := make(map[string]*models.MCPProxy, len(req.EnvMappings))
	for envName, envMapping := range req.EnvMappings {
		if _, exists := envMap[envName]; !exists {
			return nil, fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, envName)
		}
		proxyHandle := strings.TrimSpace(envMapping.ProviderName)
		if proxyHandle == "" {
			return nil, fmt.Errorf("%w: MCP proxy is required for environment %s", utils.ErrInvalidInput, envName)
		}
		proxy, err := s.mcpProxyRepo.GetByHandle(ctx, proxyHandle, ouID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("MCP proxy for environment %s not found: %w", envName, utils.ErrMCPProxyNotFound)
			}
			return nil, fmt.Errorf("failed to validate MCP proxy for environment %s: %w", envName, err)
		}
		proxiesByEnv[envName] = proxy
	}

	existingEnvMap := make(map[string]*models.EnvAgentMCPMapping, len(existingConfig.EnvMCPMappings))
	for i := range existingConfig.EnvMCPMappings {
		envUUID := existingConfig.EnvMCPMappings[i].EnvironmentUUID.String()
		name := uuidToEnvName[envUUID]
		if name == "" {
			name = envUUID
		}
		existingEnvMap[name] = &existingConfig.EnvMCPMappings[i]
	}

	// touchedEnvNames tracks every environment whose MCP binding (proxy, tool-scope
	// requirements, or deployability) this call may have changed — the union of what
	// existed before (existingEnvMap, snapshotted here before the loop below deletes
	// from it) and what's requested now (req.EnvMappings). Used after the loop to
	// refresh each affected agent's injected AgentID scopes so a running pod picks up
	// the new binding immediately instead of waiting for its next deploy/promote.
	touchedEnvNames := make(map[string]struct{}, len(existingEnvMap)+len(req.EnvMappings))
	for envName := range existingEnvMap {
		touchedEnvNames[envName] = struct{}{}
	}
	for envName := range req.EnvMappings {
		touchedEnvNames[envName] = struct{}{}
	}

	for envName := range req.EnvMappings {
		env := envMap[envName]
		envUUID, err := uuid.Parse(env.UUID)
		if err != nil {
			return nil, fmt.Errorf("invalid environment id %q: %w", envName, err)
		}
		sourceProxy := proxiesByEnv[envName]
		handle := mcpMappingProxyName(projectName, agentName, existingConfig.Name, envName)
		artifactName := handle
		sourceVersion := mcpProxyArtifactVersion(sourceProxy)
		// A non-deployable environment gets its mapping torn down / never created and its env
		// vars injected empty.
		_, gwErr := s.resolveDeployableMCPGateway(ctx, sourceProxy, ouID, envUUID)
		if gwErr != nil && !errors.Is(gwErr, errMCPEnvNotDeployable) {
			return nil, fmt.Errorf("failed to resolve gateway for MCP environment %s: %w", envName, gwErr)
		}
		deployable := gwErr == nil

		if mapping, ok := existingEnvMap[envName]; ok {
			if deployable {
				sourceChanged := mapping.MCPProxyUUID != sourceProxy.UUID
				shouldRefresh := sourceChanged || nameChanged
				if err := s.updateExistingMCPMapping(ctx, existingConfig, mapping, sourceProxy, envName, ouID, handle, artifactName, sourceVersion, false); err != nil {
					return nil, err
				}
				if err := s.reconcileMCPMappingCredentials(ctx, existingConfig, mapping, sourceProxy, envName, ouID, envTemplates, isExternalAgent, firstEnvName); err != nil {
					return nil, err
				}
				// No per-agent deployment; refresh the injected env vars when the proxy or
				// its name changed so the URL / api key reference stays correct.
				if shouldRefresh && !isExternalAgent {
					if err := s.injectMCPMappingEnvVars(ctx, existingConfig, mapping, sourceProxy, envName, ouID, envTemplates, firstEnvName); err != nil {
						s.logger.Warn("failed to inject updated MCP mapping env vars", "environment", envName, "error", err)
					}
				}
			} else {
				// The environment is no longer deployable (the proxy blueprint no longer
				// configures it, or it has no active gateway): tear down the deployment/
				// credentials but keep the env var names, re-pointed to empty.
				if err := s.teardownMCPMappingKeepEnvVars(ctx, existingConfig, mapping, envName, ouID); err != nil {
					return nil, err
				}
				if !isExternalAgent {
					if err := s.updateMCPMappingSecretReference(ctx, existingConfig.UUID, mapping.EnvironmentUUID, ""); err != nil {
						s.logger.Warn("failed to clear MCP secret reference after teardown", "environment", envName, "error", err)
					}
					emptyVars := buildEmptyMCPEnvVars(envTemplates)
					if err := s.ocClient.UpdateReleaseBindingEnvVars(ctx, ouID, projectName, agentName, envName, emptyVars); err != nil {
						s.logger.Warn("failed to inject empty MCP env vars after teardown", "environment", envName, "error", err)
					}
					if firstEnvName != "" && envName == firstEnvName {
						if err := s.ocClient.UpdateComponentEnvVars(ctx, ouID, projectName, agentName, emptyVars); err != nil {
							s.logger.Warn("failed to bootstrap empty MCP env vars after teardown", "environment", envName, "error", err)
						}
					}
				}
			}
			delete(existingEnvMap, envName)
			continue
		}

		if !deployable {
			if err := s.provisionUnconfiguredMCPEnv(ctx, existingConfig, envUUID, envName, ouID, projectName, agentName,
				envTemplates, isExternalAgent, firstEnvName, nil); err != nil {
				return nil, err
			}
			continue
		}

		if err := s.activateMCPMappingForEnv(ctx, existingConfig, sourceProxy, envUUID, envName, ouID,
			mcpActivationInputs{
				envTemplates:    envTemplates,
				isExternalAgent: isExternalAgent,
				firstEnvName:    firstEnvName,
			}); err != nil {
			return nil, fmt.Errorf("failed to bind MCP proxy for environment %s: %w", envName, err)
		}
	}

	survivingEnvCount := len(req.EnvMappings)
	for envName, mapping := range existingEnvMap {
		isLastEnv := survivingEnvCount == 0
		if err := s.removeMCPMappingEnvironment(ctx, existingConfig, mapping, envName, ouID, projectName, agentName, envTemplates, isExternalAgent, isLastEnv); err != nil {
			return nil, err
		}
	}

	// Detached onto its own goroutine, off the request path — cost scales
	// with the number of touched environments, and this is already a
	// best-effort step. See detachedRefreshContext for why it's built the way
	// it is.
	go func() {
		refreshCtx, cancel := detachedRefreshContext(ctx)
		defer cancel()
		s.refreshTouchedMCPEnvironments(refreshCtx, ouID, projectName, agentName, touchedEnvNames)
	}()

	return s.GetMCP(ctx, existingConfig.UUID, ouID, projectName, agentName)
}

// mcpRefreshTimeout bounds how long a detached refreshTouchedMCPEnvironments
// goroutine may run after its triggering request has already returned a
// response. The OpenChoreo HTTP client it calls through has no timeout of
// its own, so without this bound a single hung call would tie up the
// goroutine indefinitely; 30s matches the timeout convention already used
// elsewhere in this codebase for external-call bounds (Thunder's HTTP
// client).
const mcpRefreshTimeout = 30 * time.Second

// detachedRefreshContext derives the context a refreshTouchedMCPEnvironments
// goroutine runs with: WithoutCancel so the refresh survives the triggering
// request's own cancellation (it's deliberately best-effort work that
// outlives the response, not tied to the handler's lifecycle) while still
// carrying request-scoped values like a correlation ID, and WithTimeout so a
// hung external call can't run forever. Callers must defer the returned
// cancel func to release the timer once the goroutine finishes.
func detachedRefreshContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), mcpRefreshTimeout)
}

// refreshTouchedMCPEnvironments brings every given environment's live AgentID
// scope list in line with what it should now be, so an already-running pod
// gets rolled right away if the scope list actually changed — rather than
// only picking it up on its next deploy/promote. Uses ReconcileForEnvironment
// (not InjectForEnvironment) so touching an environment whose scopes didn't
// actually change (touchedEnvNames is the union of all existing and all
// requested mappings, so this includes plenty of no-ops) never causes a
// needless pod rollout. Best-effort: the caller runs this on a detached
// goroutine (see detachedRefreshContext) after the MCP config change itself
// already succeeded, so a refresh failure here must never turn that success
// into an error response — it's logged and the agent simply picks up the
// change on its next deploy/promote/rotation instead.
func (s *agentConfigurationService) refreshTouchedMCPEnvironments(ctx context.Context, ouID, projectName, agentName string, touchedEnvNames map[string]struct{}) {
	for envName := range touchedEnvNames {
		if err := s.agentIdentityInjection.ReconcileForEnvironment(ctx, ouID, projectName, agentName, envName); err != nil {
			s.logger.Warn("Failed to refresh agent identity credentials after MCP config change",
				"agent_name", agentName, "env_name", envName, "error", err)
		}
	}
}

func (s *agentConfigurationService) updateMCPConfigEnvironmentVariableNames(
	ctx context.Context,
	config *models.AgentConfiguration,
	ouID, projectName, agentName string,
	uuidToEnvName map[string]string,
	isExternalAgent bool,
	firstEnvName string,
	overrides []models.EnvironmentVariableConfig,
) error {
	var oldVarNames map[string]string
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		vars, err := s.envVariableRepo.ListByConfigForUpdate(ctx, tx, config.UUID)
		if err != nil {
			return fmt.Errorf("failed to load existing variable names: %w", err)
		}
		persisted := make(map[string]string)
		for _, v := range vars {
			if _, ok := persisted[v.VariableKey]; !ok {
				persisted[v.VariableKey] = v.VariableName
			}
		}
		oldVarNames = persisted
		merged := varNamesToOverrides(persisted)
		for _, ev := range overrides {
			found := false
			for i := range merged {
				if merged[i].Key == ev.Key {
					merged[i].Name = ev.Name
					found = true
					break
				}
			}
			if !found {
				merged = append(merged, ev)
			}
		}
		if _, err := s.buildMCPMappingEnvironmentVariables(config.Name, merged); err != nil {
			return errors.Join(utils.ErrInvalidInput, err)
		}
		keyNameMap := make(map[string]string, len(overrides))
		for _, ev := range overrides {
			keyNameMap[ev.Key] = ev.Name
		}
		return s.envVariableRepo.UpdateVariableNames(ctx, tx, config.UUID, keyNameMap)
	}); err != nil {
		return fmt.Errorf("failed to update MCP environment variable names: %w", err)
	}
	if isExternalAgent || len(oldVarNames) == 0 {
		return nil
	}

	changedOldKeys := make([]string, 0, len(overrides))
	mergedNames := varNamesToOverrides(oldVarNames)
	for _, ev := range overrides {
		if old, ok := oldVarNames[ev.Key]; ok && old != ev.Name {
			changedOldKeys = append(changedOldKeys, old)
		}
		for i := range mergedNames {
			if mergedNames[i].Key == ev.Key {
				mergedNames[i].Name = ev.Name
			}
		}
	}
	if len(changedOldKeys) == 0 {
		return nil
	}
	if err := s.ocClient.RemoveComponentEnvironmentVariables(ctx, ouID, projectName, agentName, changedOldKeys); err != nil {
		s.logger.Warn("failed to remove old MCP env vars from Component CR", "error", err)
	}
	newTemplates, err := s.buildMCPMappingEnvironmentVariables(config.Name, mergedNames)
	if err != nil {
		return err
	}
	for i := range config.EnvMCPMappings {
		mapping := &config.EnvMCPMappings[i]
		envName := uuidToEnvName[mapping.EnvironmentUUID.String()]
		if envName == "" || mapping.MCPProxy == nil {
			continue
		}
		sharedArtifactUUID := mcpProxyEnvArtifactUUID(mapping.MCPProxy, mapping.EnvironmentUUID.String())
		if sharedArtifactUUID == uuid.Nil {
			s.logger.Warn("failed to resolve MCP gateway for env var rename; missing shared artifact", "environment", envName)
			continue
		}
		gateway, gwErr := s.resolveGatewayForMCPArtifact(ctx, sharedArtifactUUID, ouID, mapping.EnvironmentUUID)
		if gwErr != nil {
			s.logger.Warn("failed to resolve MCP gateway for env var rename", "environment", envName, "error", gwErr)
			continue
		}
		handle := mcpMappingProxyName(config.ProjectName, config.AgentID, config.Name, envName)
		deployedProxy := buildAgentMCPConfigProxy(config, mapping, mapping.MCPProxy, envName, ouID, handle)
		secretRefName, refErr := s.loadSecretRefForConfigEnv(ctx, config.UUID, mapping.EnvironmentUUID)
		if refErr != nil {
			s.logger.Warn("failed to load MCP SecretReference for env var rename", "environment", envName, "error", refErr)
			continue
		}
		envVarsToInject := buildMCPEnvVars(newTemplates, buildMCPProxyURL(gateway, deployedProxy.Configuration), secretRefName)
		if err := s.ocClient.ReplaceReleaseBindingEnvVars(ctx, ouID, projectName, agentName, envName, changedOldKeys, envVarsToInject); err != nil {
			s.logger.Warn("failed to replace MCP env vars in ReleaseBinding", "environment", envName, "error", err)
		}
		if firstEnvName != "" && envName == firstEnvName {
			if err := s.ocClient.UpdateComponentEnvVars(ctx, ouID, projectName, agentName, envVarsToInject); err != nil {
				s.logger.Warn("failed to update Component CR with renamed MCP env vars", "environment", envName, "error", err)
			}
		}
	}
	return nil
}

func (s *agentConfigurationService) refreshAllMCPMappings(ctx context.Context, config *models.AgentConfiguration, ouID string, uuidToEnvName map[string]string,
	envTemplates []EnvConfigTemplate, isExternalAgent bool, firstEnvName string,
) error {
	for i := range config.EnvMCPMappings {
		mapping := &config.EnvMCPMappings[i]
		envName := uuidToEnvName[mapping.EnvironmentUUID.String()]
		if envName == "" || mapping.MCPProxy == nil {
			continue
		}
		if mcpProxyEnvArtifactUUID(mapping.MCPProxy, mapping.EnvironmentUUID.String()) == uuid.Nil {
			s.logger.Warn("skipping MCP mapping refresh; missing shared artifact", "environment", envName)
			continue
		}
		handle := mcpMappingProxyName(config.ProjectName, config.AgentID, config.Name, envName)
		artifactName := handle
		version := mcpProxyArtifactVersion(mapping.MCPProxy)
		if err := s.updateExistingMCPMapping(ctx, config, mapping, mapping.MCPProxy, envName, ouID, handle, artifactName, version, false); err != nil {
			return err
		}
		if err := s.reconcileMCPMappingCredentials(ctx, config, mapping, mapping.MCPProxy, envName, ouID, envTemplates, isExternalAgent, firstEnvName); err != nil {
			return err
		}
		// No per-agent gateway deployment: the proxy owns the single per-environment
		// artifact. We only refresh the injected env vars (URL / api key reference).
		if !isExternalAgent {
			if err := s.injectMCPMappingEnvVars(ctx, config, mapping, mapping.MCPProxy, envName, ouID, envTemplates, firstEnvName); err != nil {
				s.logger.Warn("failed to inject refreshed MCP mapping env vars", "environment", envName, "error", err)
			}
		}
	}
	return nil
}

func (s *agentConfigurationService) updateExistingMCPMapping(ctx context.Context, config *models.AgentConfiguration, mapping *models.EnvAgentMCPMapping,
	sourceProxy *models.MCPProxy, envName, ouID, handle, artifactName, version string, redeploy bool,
) error {
	mapping.MCPProxyUUID = sourceProxy.UUID
	mapping.MCPProxy = sourceProxy
	deployedProxy := buildAgentMCPConfigProxy(config, mapping, sourceProxy, envName, ouID, handle)
	proxyMapping := buildMCPProxyMapping(sourceProxy.UUID, deployedProxy)
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Model(&models.Artifact{}).
			Where("uuid = ?", mapping.ArtifactUUID).
			Updates(map[string]interface{}{
				"handle":     handle,
				"name":       artifactName,
				"version":    version,
				"kind":       models.KindMCPMapping,
				"in_catalog": false,
				"updated_at": time.Now(),
			}).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Model(&models.MCPProxyMapping{}).
			Where("uuid = ?", mapping.ArtifactUUID).
			Updates(map[string]interface{}{
				"source_mcp_proxy_uuid": proxyMapping.SourceMCPProxyUUID,
				"description":           proxyMapping.Description,
				"configuration":         proxyMapping.Configuration,
			}).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Model(&models.EnvAgentMCPMapping{}).
			Where("id = ?", mapping.ID).
			Updates(map[string]interface{}{
				"mcp_proxy_uuid": mapping.MCPProxyUUID,
				"artifact_uuid":  mapping.ArtifactUUID,
			}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to update MCP mapping for environment %s: %w", envName, err)
	}
	// redeploy is retained for signature compatibility; agent configurations no longer
	// deploy their own gateway artifacts (the proxy owns the per-environment artifact).
	_ = redeploy
	return nil
}

func (s *agentConfigurationService) injectMCPMappingEnvVars(ctx context.Context, config *models.AgentConfiguration, mapping *models.EnvAgentMCPMapping,
	sourceProxy *models.MCPProxy, envName, ouID string, envTemplates []EnvConfigTemplate, firstEnvName string,
) error {
	sharedArtifactUUID := mcpProxyEnvArtifactUUID(sourceProxy, mapping.EnvironmentUUID.String())
	if sharedArtifactUUID == uuid.Nil {
		return fmt.Errorf("MCP proxy shared artifact not found for environment %s", envName)
	}
	gateway, err := s.resolveGatewayForMCPArtifact(ctx, sharedArtifactUUID, ouID, mapping.EnvironmentUUID)
	if err != nil {
		return err
	}
	handle := mcpMappingProxyName(config.ProjectName, config.AgentID, config.Name, envName)
	deployedProxy := buildAgentMCPConfigProxy(config, mapping, sourceProxy, envName, ouID, handle)
	secretRefName, err := s.loadSecretRefForConfigEnv(ctx, config.UUID, mapping.EnvironmentUUID)
	if err != nil {
		return err
	}
	envVarsToInject := buildMCPEnvVars(envTemplates, buildMCPProxyURL(gateway, deployedProxy.Configuration), secretRefName)
	if err := s.ocClient.UpdateReleaseBindingEnvVars(ctx, ouID, config.ProjectName, config.AgentID, envName, envVarsToInject); err != nil {
		return err
	}
	if firstEnvName != "" && envName == firstEnvName {
		return s.ocClient.UpdateComponentEnvVars(ctx, ouID, config.ProjectName, config.AgentID, envVarsToInject)
	}
	return nil
}

// provisionUnconfiguredMCPEnv handles an environment the selected MCP proxy has no
// blueprint block for: no mapping is created, nothing is deployed and no API key is
// minted. It persists the per-environment env var name rows (with empty secret
// references) and injects the URL + API-key env vars as empty strings for internal
// agents, or records empty credentials for external agents — so the agent still has the
// variable names defined but blank in that environment. Env var name persistence is
// hard (returns an error); the runtime injection is best-effort (logged and continued),
// mirroring the configured path.
func (s *agentConfigurationService) provisionUnconfiguredMCPEnv(ctx context.Context,
	config *models.AgentConfiguration, envUUID uuid.UUID, envName, ouID, projectName, agentID string,
	envTemplates []EnvConfigTemplate, isExternalAgent bool, firstEnvName string,
	envCredentials map[string]envCredentialData,
) error {
	// Reuse the idempotent row-creation helper so repeated updateMCPConfig calls for an
	// unconfigured environment do not accumulate duplicate env var rows for the same
	// config/environment pair.
	if err := s.ensureMCPEnvVarRows(ctx, config.UUID, envUUID, envTemplates); err != nil {
		return fmt.Errorf("failed to create MCP environment variables for %s: %w", envName, err)
	}

	if isExternalAgent {
		if envCredentials != nil {
			envCredentials[envUUID.String()] = envCredentialData{apiKey: "", proxyURL: ""}
		}
		return nil
	}

	envVarsToInject := buildEmptyMCPEnvVars(envTemplates)
	if err := s.ocClient.UpdateReleaseBindingEnvVars(ctx, ouID, projectName, agentID, envName, envVarsToInject); err != nil {
		s.logger.Warn("failed to patch ReleaseBinding for empty MCP env var injection", "environment", envName, "error", err)
	}
	if firstEnvName != "" && envName == firstEnvName {
		if err := s.ocClient.UpdateComponentEnvVars(ctx, ouID, projectName, agentID, envVarsToInject); err != nil {
			s.logger.Warn("failed to patch Component for empty MCP env var bootstrap", "environment", envName, "error", err)
		}
	}
	return nil
}

func (s *agentConfigurationService) cleanupMCPMappingCredentials(ctx context.Context, config *models.AgentConfiguration, mapping *models.EnvAgentMCPMapping, envName, ouID string) {
	if config == nil || mapping == nil || envName == "" {
		return
	}
	handle := mcpMappingProxyName(config.ProjectName, config.AgentID, config.Name, envName)
	scopedID := scopedProxyIdentifier(config.ProjectName, config.AgentID, config.Name, envName)
	keyName := fmt.Sprintf("%s-key", scopedID)
	if err := s.revokeAllMCPMappingAPIKeys(ctx, ouID, s.resolveMCPMappingAPIID(ctx, mapping, ouID), mapping.ArtifactUUID); err != nil {
		s.logger.Warn("failed to revoke MCP mapping API key", "mapping_handle", handle, "key_name", keyName, "error", err)
	}

	secretRefName, err := s.loadSecretRefForConfigEnv(ctx, config.UUID, mapping.EnvironmentUUID)
	if err != nil {
		s.logger.Warn("failed to load MCP SecretReference for cleanup", "environment", envName, "error", err)
	}
	proxySecretLoc := secretmanagersvc.SecretLocation{
		OrgName:         ouID,
		ProjectName:     config.ProjectName,
		AgentName:       config.AgentID,
		EnvironmentName: envName,
		ConfigName:      config.Name,
		EntityName:      fmt.Sprintf("%s-proxy", scopedID),
		SecretKey:       secretmanagersvc.SecretKeyAPIKey,
	}
	secretRefForDelete := secretRefName
	if secretRefForDelete == "" {
		secretRefForDelete = proxySecretLoc.SecretRefName()
	}
	if err := s.secretClient.DeleteSecret(ctx, proxySecretLoc, secretRefForDelete); err != nil {
		s.logger.Warn("failed to delete MCP mapping API key secret", "mapping_handle", handle, "secret_ref", secretRefForDelete, "error", err)
	}
}

func (s *agentConfigurationService) removeMCPMappingEnvironment(ctx context.Context, config *models.AgentConfiguration, mapping *models.EnvAgentMCPMapping,
	envName, ouID, projectName, agentName string, envTemplates []EnvConfigTemplate, isExternalAgent, isLastEnv bool,
) error {
	if !isExternalAgent && envName != "" {
		keysToRemove := make([]string, 0, len(envTemplates))
		for _, t := range envTemplates {
			keysToRemove = append(keysToRemove, t.Name)
		}
		if len(keysToRemove) > 0 {
			if err := s.ocClient.RemoveReleaseBindingEnvVars(ctx, ouID, projectName, agentName, envName, keysToRemove); err != nil {
				s.logger.Warn("failed to remove MCP env vars from ReleaseBinding", "environment", envName, "error", err)
			}
			if isLastEnv {
				if err := s.ocClient.RemoveComponentEnvironmentVariables(ctx, ouID, projectName, agentName, keysToRemove); err != nil {
					s.logger.Warn("failed to remove MCP env vars from Component CR", "environment", envName, "error", err)
				}
			}
		}
	}

	if s.mcpProxyService != nil {
		s.mcpProxyService.BroadcastMCPArtifactDeletion(ctx, mapping.ArtifactUUID, ouID)
	}
	s.cleanupMCPMappingCredentials(ctx, config, mapping, envName, ouID)
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.envVariableRepo.DeleteByConfigAndEnv(ctx, tx, config.UUID, mapping.EnvironmentUUID); err != nil {
			return err
		}
		if err := s.envMCPMappingRepo.Delete(ctx, tx, mapping.ID); err != nil {
			return err
		}
		if err := tx.Where("artifact_uuid = ? AND ou_id = ?", mapping.ArtifactUUID, ouID).
			Delete(&models.DeploymentStatusRecord{}).Error; err != nil {
			return err
		}
		if err := tx.Where("artifact_uuid = ? AND ou_id = ?", mapping.ArtifactUUID, ouID).
			Delete(&models.Deployment{}).Error; err != nil {
			return err
		}
		return tx.Where("uuid = ?", mapping.ArtifactUUID).Delete(&models.Artifact{}).Error
	})
}

// teardownMCPMappingKeepEnvVars removes an MCP mapping's gateway deployment and
// credentials for an environment whose proxy blueprint no longer has a block, but KEEPS
// the env var name rows (the caller re-points their values to empty). It mirrors
// removeMCPMappingEnvironment minus the ReleaseBinding/Component env var removal and
// minus deleting the env var rows.
func (s *agentConfigurationService) teardownMCPMappingKeepEnvVars(ctx context.Context, config *models.AgentConfiguration,
	mapping *models.EnvAgentMCPMapping, envName, ouID string,
) error {
	if s.mcpProxyService != nil {
		s.mcpProxyService.BroadcastMCPArtifactDeletion(ctx, mapping.ArtifactUUID, ouID)
	}
	s.cleanupMCPMappingCredentials(ctx, config, mapping, envName, ouID)
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.envMCPMappingRepo.Delete(ctx, tx, mapping.ID); err != nil {
			return err
		}
		if err := tx.Where("artifact_uuid = ? AND ou_id = ?", mapping.ArtifactUUID, ouID).
			Delete(&models.DeploymentStatusRecord{}).Error; err != nil {
			return err
		}
		if err := tx.Where("artifact_uuid = ? AND ou_id = ?", mapping.ArtifactUUID, ouID).
			Delete(&models.Deployment{}).Error; err != nil {
			return err
		}
		return tx.Where("uuid = ?", mapping.ArtifactUUID).Delete(&models.Artifact{}).Error
	})
}

func mcpProxyArtifactVersion(source *models.MCPProxy) string {
	if source == nil {
		return models.DefaultProxyVersion
	}
	if source.Artifact != nil && source.Artifact.Version != "" {
		return source.Artifact.Version
	}
	if source.Version != "" {
		return source.Version
	}
	if source.Configuration.Version != "" {
		return source.Configuration.Version
	}
	return models.DefaultProxyVersion
}

// UpdateMCP updates an existing MCP proxy mapping with project and agent scoping validation.
func (s *agentConfigurationService) UpdateMCP(ctx context.Context, configUUID uuid.UUID, ouID, projectName, agentName string,
	req models.UpdateAgentModelConfigRequest,
) (*models.AgentModelConfigResponse, error) {
	config, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentConfigNotFound
		}
		return nil, fmt.Errorf("failed to get MCP configuration: %w", err)
	}
	if config.ProjectName != projectName || config.AgentID != agentName || config.TypeID != models.AgentConfigTypeIDMCP {
		return nil, utils.ErrAgentConfigNotFound
	}
	return s.updateMCPConfig(ctx, config, ouID, projectName, agentName, req)
}

// Update updates an existing configuration with project and agent scoping validation.
// External network calls (proxy create/update/deploy, API key generation) are performed outside
// transactions. Only pure DB writes use short, focused transactions.
//
// NOTE: Partial failure across multiple environments is an accepted limitation (see SAGA.md).
// On failure in env N, envs 1..N-1 may already be updated. Retry is possible but not idempotent.
func (s *agentConfigurationService) Update(ctx context.Context, configUUID uuid.UUID, ouID, projectName, agentName string,
	req models.UpdateAgentModelConfigRequest,
) (*models.AgentModelConfigResponse, error) {
	// Get existing configuration with all mappings
	existingConfig, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentConfigNotFound
		}
		return nil, fmt.Errorf("failed to get configuration: %w", err)
	}

	// Validate project and agent scoping
	if existingConfig.ProjectName != projectName || existingConfig.AgentID != agentName {
		return nil, utils.ErrAgentConfigNotFound
	}

	if existingConfig.TypeID == models.AgentConfigTypeIDMCP {
		return s.updateMCPConfig(ctx, existingConfig, ouID, projectName, agentName, req)
	}

	// Load environments once; used to key existingEnvMap by name and to validate request envs.
	allEnvs, err := s.infraResourceManager.ListOrgEnvironments(ctx, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	envMap := make(map[string]*models.EnvironmentResponse, len(allEnvs))
	uuidToEnvName := make(map[string]string, len(allEnvs))
	for _, e := range allEnvs {
		envMap[e.Name] = e
		uuidToEnvName[e.UUID] = e.Name
	}

	// Build map of existing environment mappings for comparison, keyed by environment name.
	// The request uses env names, so we must match by name (not UUID).
	existingEnvMap := make(map[string]*models.EnvAgentModelMapping, len(existingConfig.EnvMappings))
	for i := range existingConfig.EnvMappings {
		envUUID := existingConfig.EnvMappings[i].EnvironmentUUID.String()
		name := uuidToEnvName[envUUID]
		if name == "" {
			name = envUUID // fall back to UUID if env was deleted
		}
		existingEnvMap[name] = &existingConfig.EnvMappings[i]
	}

	// Validate all providers exist and are in catalog (if envMappings provided)
	if req.EnvMappings != nil {
		for envName, envMapping := range req.EnvMappings {
			if err := envMapping.Configuration.Resilience.Validate(); err != nil {
				return nil, fmt.Errorf("%w: environment %s: %w", utils.ErrInvalidInput, envName, err)
			}
			provider, err := s.llmProviderRepo.GetByHandle(envMapping.ProviderName, ouID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					s.logger.Warn("Provider not found", "env", envName, "error", err)
					return nil, fmt.Errorf("provider for environment %s not found: %w", envName, utils.ErrLLMProviderNotFound)
				}
				return nil, fmt.Errorf("failed to validate provider for environment %s: %w", envName, err)
			}
			if !provider.InCatalog {
				return nil, fmt.Errorf("%w: provider %s must be in catalog for environment %s", utils.ErrInvalidInput, envMapping.ProviderName, envName)
			}
		}
	}

	// Phase 1 — Short TX: update name/description only.
	if req.Name != "" {
		existingConfig.Name = req.Name
	}
	if req.Description != "" {
		existingConfig.Description = req.Description
	}
	if req.Name != "" || req.Description != "" {
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			return s.agentConfigRepo.Update(ctx, tx, existingConfig)
		}); err != nil {
			return nil, fmt.Errorf("failed to update configuration: %w", err)
		}
	}

	// Phase 1b — Update env var names if provided (global rename across all environments).
	// Read, validate, and write happen inside a single transaction with a row-level lock to
	// prevent concurrent rename requests from bypassing uniqueness checks.
	if len(req.EnvironmentVariables) > 0 {
		// oldVarNames is populated inside the transaction (under the row lock) so the
		// snapshot is consistent with the locked state used for the rename.
		var oldVarNames map[string]string

		if err := s.db.Transaction(func(tx *gorm.DB) error {
			// Lock the rows so concurrent renames on the same config are serialised.
			vars, err := s.envVariableRepo.ListByConfigForUpdate(ctx, tx, configUUID)
			if err != nil {
				return fmt.Errorf("failed to load existing variable names: %w", err)
			}
			// Build key→name map from locked rows (first-occurrence wins per key).
			persistedVarNames := make(map[string]string)
			for _, v := range vars {
				if _, already := persistedVarNames[v.VariableKey]; !already {
					persistedVarNames[v.VariableKey] = v.VariableName
				}
			}
			// Capture old names under the same lock used for the rename.
			oldVarNames = persistedVarNames
			// Merge requested renames over persisted names.
			mergedOverrides := make([]models.EnvironmentVariableConfig, 0, len(persistedVarNames))
			for key, name := range persistedVarNames {
				mergedOverrides = append(mergedOverrides, models.EnvironmentVariableConfig{Key: key, Name: name})
			}
			for _, ev := range req.EnvironmentVariables {
				found := false
				for i, mo := range mergedOverrides {
					if mo.Key == ev.Key {
						mergedOverrides[i].Name = ev.Name
						found = true
						break
					}
				}
				if !found {
					mergedOverrides = append(mergedOverrides, ev)
				}
			}
			// Validate using the merged result (catches uniqueness and format errors against locked names).
			var validateErr error
			if existingConfig.TypeID == models.AgentConfigTypeIDMCP {
				_, validateErr = s.buildMCPMappingEnvironmentVariables(existingConfig.Name, mergedOverrides)
			} else {
				_, validateErr = s.buildEnvironmentVariables(existingConfig.Name, mergedOverrides)
			}
			if validateErr != nil {
				return errors.Join(utils.ErrInvalidInput, validateErr)
			}
			keyNameMap := make(map[string]string, len(req.EnvironmentVariables))
			for _, ev := range req.EnvironmentVariables {
				keyNameMap[ev.Key] = ev.Name
			}
			return s.envVariableRepo.UpdateVariableNames(ctx, tx, configUUID, keyNameMap)
		}); err != nil {
			return nil, fmt.Errorf("failed to update environment variable names: %w", err)
		}

		// For internal agents: remove old env var names from the Component CR and all
		// per-environment ReleaseBindings so stale variables don't linger after a rename.
		// Only runs when at least one name actually changed; skipped entirely if nothing differed.
		// Best-effort — failures are logged but do not abort the update.
		if len(oldVarNames) > 0 {
			// Collect names that were actually renamed (old name != new name).
			changedOldKeys := make([]string, 0, len(req.EnvironmentVariables))
			for _, ev := range req.EnvironmentVariables {
				if existing, ok := oldVarNames[ev.Key]; ok && existing != ev.Name {
					changedOldKeys = append(changedOldKeys, existing)
				}
			}
			if len(changedOldKeys) > 0 {
				agentComp, compErr := s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
				if compErr != nil {
					s.logger.Warn("Phase 1b: failed to determine agent type for env var cleanup", "error", compErr)
				} else if agentComp.Provisioning.Type != string(utils.ExternalAgent) {
					// Remove old names from Component CR.
					if rmErr := s.ocClient.RemoveComponentEnvironmentVariables(ctx, ouID, projectName, agentName, changedOldKeys); rmErr != nil {
						s.logger.Warn("Phase 1b: failed to remove old env vars from Component CR", "error", rmErr)
					}

					// Build new env var templates for re-injection.
					newOverrides := make([]models.EnvironmentVariableConfig, 0, len(oldVarNames))
					for key, name := range oldVarNames {
						newOverrides = append(newOverrides, models.EnvironmentVariableConfig{Key: key, Name: name})
					}
					for _, ev := range req.EnvironmentVariables {
						for j, o := range newOverrides {
							if o.Key == ev.Key {
								newOverrides[j].Name = ev.Name
								break
							}
						}
					}
					var newEnvConfigTemplates []EnvConfigTemplate
					var buildErr error
					if existingConfig.TypeID == models.AgentConfigTypeIDMCP {
						newEnvConfigTemplates, buildErr = s.buildMCPMappingEnvironmentVariables(existingConfig.Name, newOverrides)
					} else {
						newEnvConfigTemplates, buildErr = s.buildEnvironmentVariables(existingConfig.Name, newOverrides)
					}
					if buildErr != nil {
						s.logger.Warn("Phase 1b: failed to build new env var templates for re-injection after rename", "error", buildErr)
					}

					// Determine first env for Component CR bootstrap update.
					firstEnvName1b := ""
					if pipeline, pipelineErr := s.ocClient.GetProjectDeploymentPipeline(ctx, ouID, projectName); pipelineErr == nil && pipeline != nil {
						firstEnvName1b = client.FindFirstEnvironment(pipeline.PromotionPaths)
					}

					// Atomic per-environment: remove old keys + inject new env vars in a single
					// ReleaseBinding Get/Update cycle to avoid resource version conflicts that
					// cause 500 errors when remove and add are separate API calls.
					if existingConfig.TypeID == models.AgentConfigTypeIDMCP {
						for i := range existingConfig.EnvMCPMappings {
							mapping := &existingConfig.EnvMCPMappings[i]
							envUUID := mapping.EnvironmentUUID.String()
							envName := uuidToEnvName[envUUID]
							if envName == "" || buildErr != nil || mapping.MCPProxy == nil {
								continue
							}
							envEnvUUID, parseErr := uuid.Parse(envUUID)
							if parseErr != nil {
								continue
							}
							sharedArtifactUUID := s.resolveMCPMappingAPIID(ctx, mapping, ouID)
							if sharedArtifactUUID == uuid.Nil {
								s.logger.Warn("Phase 1b: missing MCP shared artifact for re-injection", "environment", envName)
								continue
							}
							gateway, gwErr := s.resolveGatewayForMCPArtifact(ctx, sharedArtifactUUID, ouID, envEnvUUID)
							if gwErr != nil {
								s.logger.Warn("Phase 1b: failed to resolve MCP gateway for re-injection", "environment", envName, "error", gwErr)
								continue
							}
							deployedProxy := buildAgentMCPConfigProxy(existingConfig, mapping, mapping.MCPProxy, envName, ouID,
								mcpMappingProxyName(existingConfig.ProjectName, existingConfig.AgentID, existingConfig.Name, envName))
							secretRefName, refErr := s.loadSecretRefForConfigEnv(ctx, existingConfig.UUID, mapping.EnvironmentUUID)
							if refErr != nil {
								s.logger.Warn("Phase 1b: failed to load MCP SecretReference for re-injection", "environment", envName, "error", refErr)
								continue
							}
							envVarsToInject := buildMCPEnvVars(newEnvConfigTemplates, buildMCPProxyURL(gateway, deployedProxy.Configuration), secretRefName)
							s.logger.Info("Phase 1b: atomically replacing MCP env vars in ReleaseBinding",
								"environment", envName, "keys_to_remove", changedOldKeys, "env_vars_to_add", len(envVarsToInject))
							if rbErr := s.ocClient.ReplaceReleaseBindingEnvVars(ctx, ouID, projectName, agentName, envName, changedOldKeys, envVarsToInject); rbErr != nil {
								s.logger.Warn("Phase 1b: failed to replace MCP env vars in ReleaseBinding", "environment", envName, "error", rbErr)
							}
							if firstEnvName1b != "" && envName == firstEnvName1b {
								if uvErr := s.ocClient.UpdateComponentEnvVars(ctx, ouID, projectName, agentName, envVarsToInject); uvErr != nil {
									s.logger.Warn("Phase 1b: failed to re-inject new MCP env var names into Component CR", "environment", envName, "error", uvErr)
								}
							}
						}
					} else {
						for i := range existingConfig.EnvMappings {
							mapping := &existingConfig.EnvMappings[i]
							envUUID := mapping.EnvironmentUUID.String()
							envName := uuidToEnvName[envUUID]
							if envName == "" || buildErr != nil || mapping.LLMProxy == nil {
								continue
							}
							envEnvUUID, parseErr := uuid.Parse(envUUID)
							if parseErr != nil {
								continue
							}
							gateway, gwErr := s.resolveGatewayForProxy(ctx, mapping.LLMProxy.Handle, ouID, envEnvUUID)
							if gwErr != nil {
								s.logger.Warn("Phase 1b: failed to resolve gateway for re-injection", "environment", envName, "error", gwErr)
								continue
							}
							proxyURL := buildProxyURL(gateway, mapping.LLMProxy.Configuration.Context)
							// Use persisted SecretReference from DB rather than deriving from mutable config name.
							envVars1b, varErr1b := s.envVariableRepo.ListByConfigAndEnv(ctx, existingConfig.UUID, mapping.EnvironmentUUID)
							secretRefName := ""
							if varErr1b != nil {
								s.logger.Warn("Phase 1b: failed to load persisted SecretReference", "environment", envName, "error", varErr1b)
								continue
							}
							for _, v := range envVars1b {
								if v.SecretReference != "" {
									secretRefName = v.SecretReference
									s.logger.Info("Phase 1b: using persisted SecretReference for re-injection",
										"secret_ref", secretRefName, "variable_name", v.VariableName,
										"config_uuid", existingConfig.UUID, "environment", envName)
									break
								}
							}
							if secretRefName == "" {
								s.logger.Warn("Phase 1b: no persisted SecretReference found, skipping re-injection", "environment", envName)
								continue
							}
							envVarsToInject := buildLLMEnvVars(newEnvConfigTemplates, proxyURL, secretRefName)
							s.logger.Info("Phase 1b: atomically replacing env vars in ReleaseBinding",
								"environment", envName, "keys_to_remove", changedOldKeys, "env_vars_to_add", len(envVarsToInject))
							if rbErr := s.ocClient.ReplaceReleaseBindingEnvVars(ctx, ouID, projectName, agentName, envName, changedOldKeys, envVarsToInject); rbErr != nil {
								s.logger.Warn("Phase 1b: failed to replace env vars in ReleaseBinding", "environment", envName, "error", rbErr)
							}
							if firstEnvName1b != "" && envName == firstEnvName1b {
								if uvErr := s.ocClient.UpdateComponentEnvVars(ctx, ouID, projectName, agentName, envVarsToInject); uvErr != nil {
									s.logger.Warn("Phase 1b: failed to re-inject new env var names into Component CR", "environment", envName, "error", uvErr)
								}
							}
						}
					}
				}
			}
		}
	}

	// If no envMappings provided, return the updated config immediately.
	if req.EnvMappings == nil {
		s.recordConfigUpdate(ctx, configUUID, ouID, projectName, agentName, existingConfig, req)
		return s.Get(ctx, configUUID, ouID, projectName, agentName)
	}

	// Load existing variable names so new/replaced envs get consistent names.
	existingVarNames, err := s.loadExistingVarNames(ctx, configUUID)
	if err != nil {
		return nil, err
	}

	// Determine agent type and first env for internal-agent env var injection.
	// Fail closed: if GetComponent errors, return rather than defaulting to internal (which could corrupt CRs).
	agentComp, agentErr := s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
	if agentErr != nil {
		return nil, fmt.Errorf("failed to determine agent type: %w", agentErr)
	}
	isExternalAgent := agentComp.Provisioning.Type == string(utils.ExternalAgent)
	firstEnvName := ""
	if !isExternalAgent {
		if pipeline, pipelineErr := s.ocClient.GetProjectDeploymentPipeline(ctx, ouID, projectName); pipelineErr == nil && pipeline != nil {
			firstEnvName = client.FindFirstEnvironment(pipeline.PromotionPaths)
		}
	}

	// Track resources for rollback and old proxies to clean up post-success.
	var rollbackResources []rollbackResource
	var proxiesToDelete []string

	// AI application bindings are deferred until every environment below
	// succeeds — see flushPendingAppBindings.
	var pendingAppBindings []pendingAppBinding

	// Phase 2/3 — Loop over requested environments, calling scenario helpers.
	// NOTE: map iteration order is non-deterministic; partial failures leave a random subset processed.
	for envName, envMapping := range req.EnvMappings {
		select {
		case <-ctx.Done():
			// Detach from cancellation so a cancelled ctx doesn't prevent rollback
			// (CRIT-2) — but keep the values, so the rollback's log records still
			// carry the correlation ID of the request that triggered it.
			// context.Background() dropped them, leaving the failure path, which is
			// the one worth tracing, as orphan log lines.
			cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cleanupCancel()
			s.rollbackProxies(cleanupCtx, rollbackResources, ouID)
			return nil, fmt.Errorf("operation cancelled: %w", ctx.Err())
		default:
		}

		env, exists := envMap[envName]
		if !exists {
			s.rollbackProxies(ctx, rollbackResources, ouID)
			return nil, fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, envName)
		}

		envUUID, err := uuid.Parse(env.UUID)
		if err != nil {
			s.rollbackProxies(ctx, rollbackResources, ouID)
			return nil, fmt.Errorf("invalid environment id %q: %w", envName, err)
		}

		existingMapping, hasExisting := existingEnvMap[envName]

		if hasExisting {
			var newProviderUUID string
			if existingMapping.LLMProxy != nil {
				newProvider, err := s.llmProviderRepo.GetByHandle(envMapping.ProviderName, ouID)
				if err == nil {
					newProviderUUID = newProvider.UUID.String()
				}
			}
			providerChanged := existingMapping.LLMProxy != nil &&
				existingMapping.LLMProxy.Configuration.Provider != newProviderUUID

			if providerChanged {
				// Scenario A: provider changed — create new proxy, update mapping, schedule old proxy for cleanup.
				oldHandle, rbRes, pendingBind, err := s.processEnvProviderChange(
					ctx, configUUID, existingConfig, env, envUUID, envName, envMapping, existingMapping, ouID, existingVarNames, isExternalAgent, firstEnvName,
				)
				if err != nil {
					// rbRes may already hold resources created before the failure (e.g. a
					// proxy row from a partially-successful Create) — include it so they don't
					// leak as orphans that deterministically collide with the next retry.
					s.rollbackProxies(ctx, append(rollbackResources, rbRes), ouID)
					return nil, err
				}
				rollbackResources = append(rollbackResources, rbRes)
				pendingAppBindings = append(pendingAppBindings, pendingBind)
				if oldHandle != "" {
					proxiesToDelete = append(proxiesToDelete, oldHandle)
				}
			} else {
				// Scenario B: same provider — update proxy config and redeploy. No DB TX needed.
				rbRes, err := s.processEnvProxyUpdate(
					ctx, existingConfig, env, envUUID, envName, envMapping, existingMapping, ouID,
				)
				if err != nil {
					s.rollbackProxies(ctx, append(rollbackResources, rbRes), ouID)
					return nil, err
				}
				if rbRes.providerAPIKeyID != "" {
					rollbackResources = append(rollbackResources, rbRes)
				}
			}
			delete(existingEnvMap, envName)
		} else {
			// Scenario C: new environment — create proxy and mapping.
			rbRes, pendingBind, err := s.processNewEnv(
				ctx, configUUID, existingConfig, env, envUUID, envName, envMapping, ouID, existingVarNames, isExternalAgent, firstEnvName,
			)
			if err != nil {
				s.rollbackProxies(ctx, append(rollbackResources, rbRes), ouID)
				return nil, err
			}
			rollbackResources = append(rollbackResources, rbRes)
			pendingAppBindings = append(pendingAppBindings, pendingBind)
		}
	}

	// Phase 3b — Bind AI applications now that every environment above has
	// otherwise succeeded; a bind failure here still rolls back everything
	// created above, before Phase 4 touches anything else.
	if err := s.flushPendingAppBindings(ctx, pendingAppBindings, &rollbackResources); err != nil {
		s.rollbackProxies(ctx, rollbackResources, ouID)
		return nil, err
	}

	// Phase 4 — Remove environments not in the request (Scenario D).
	// survivingEnvCount is the number of environments that will remain after all
	// removals — used to decide whether to clear the Component CR.
	survivingEnvCount := len(req.EnvMappings)
	for _, mapping := range existingEnvMap {
		if mapping.LLMProxy != nil {
			proxiesToDelete = append(proxiesToDelete, mapping.LLMProxy.Handle)
		}
		removedEnvName := uuidToEnvName[mapping.EnvironmentUUID.String()]
		isLastEnv := survivingEnvCount == 0
		if err := s.processEnvRemoval(ctx, configUUID, mapping.EnvironmentUUID.String(), mapping, existingConfig.Name, removedEnvName, ouID, projectName, agentName, isExternalAgent, existingVarNames, isLastEnv); err != nil {
			// HIGH-6: Phase 2-3 DB changes are already committed. Log enough information for manual reconciliation.
			s.logger.Error(
				"Partial update failure — manual reconciliation required",
				"config_uuid", configUUID,
				"action", "manual_cleanup_required",
				"failed_at_env", mapping.EnvironmentUUID.String(),
				"error", err,
			)
			s.rollbackProxies(ctx, rollbackResources, ouID)
			return nil, err
		}
	}

	// Phase 5 — Post-success proxy cleanup (outside any transaction, best effort).
	cleanupErrors := 0
	for _, proxyHandle := range proxiesToDelete {
		s.logger.Info("Cleaning up replaced proxy", "proxy_handle", proxyHandle)

		deployments, err := s.llmProxyDeploymentService.GetLLMProxyDeployments(proxyHandle, ouID, nil, nil)
		if err != nil {
			s.logger.Error(
				"Failed to get deployments for proxy cleanup",
				"proxy_handle", proxyHandle,
				"error", err,
			)
			cleanupErrors++
		} else {
			for _, dep := range deployments {
				// The replaced proxy's deployment is still DEPLOYED — DeleteLLMProxyDeployment
				// rejects that (ErrDeploymentIsDeployed). It must be undeployed first, same as
				// the whole-config deletion path (deleteLLMConfig) already does.
				if dep.Status == nil || *dep.Status != models.DeploymentStatusDeployed {
					continue
				}
				if _, err := s.llmProxyDeploymentService.UndeployLLMProxyDeployment(ctx, proxyHandle, dep.DeploymentID.String(), dep.GatewayUUID.String(), ouID); err != nil {
					s.logger.Error(
						"Failed to undeploy deployment during cleanup",
						"proxy_handle", proxyHandle,
						"deployment_id", dep.DeploymentID,
						"error", err,
					)
					cleanupErrors++
				}
			}
		}

		if err := s.llmProxyService.Delete(proxyHandle, ouID); err != nil {
			s.logger.Error(
				"Failed to delete proxy during cleanup",
				"proxy_handle", proxyHandle,
				"error", err,
			)
			cleanupErrors++
		}
	}

	if cleanupErrors > 0 {
		s.logger.Warn(
			"Cleanup completed with errors",
			"total_proxies", len(proxiesToDelete),
			"errors", cleanupErrors,
		)
	}

	s.recordConfigUpdate(ctx, configUUID, ouID, projectName, agentName, existingConfig, req)

	// Return updated configuration
	return s.Get(ctx, configUUID, ouID, projectName, agentName)
}

// recordConfigUpdate records a completed configuration update. It is called on
// every successful return from Update, not only the one that rewrites the
// environment mappings: a rename or a description change is still a change
// somebody has to be able to attribute.
//
// The resource name comes from existingConfig rather than from the request,
// because req.Name is empty on any update that is not a rename — recording it
// directly produced records that named no configuration at all.
func (s *agentConfigurationService) recordConfigUpdate(ctx context.Context, configUUID uuid.UUID,
	ouID, projectName, agentName string, existingConfig *models.AgentConfiguration,
	req models.UpdateAgentModelConfigRequest,
) {
	configName := req.Name
	if configName == "" && existingConfig != nil {
		configName = existingConfig.Name
	}

	updatedFields := []string{}
	if req.Name != "" {
		updatedFields = append(updatedFields, "name")
	}
	if req.Description != "" {
		updatedFields = append(updatedFields, "description")
	}
	if req.EnvMappings != nil {
		updatedFields = append(updatedFields, "envMappings")
	}
	if req.EnvironmentVariables != nil {
		updatedFields = append(updatedFields, "environmentVariables")
	}

	// A real audit record. This previously wrote only an slog line labelled as
	// an audit log: it carried no actor, no outcome and no durability, so it
	// could not answer who changed the configuration.
	audit.Record(
		ctx, audit.ActionAgentConfigUpdate,
		audit.Org(ouID),
		audit.ResourceNamed("agent-config", configUUID.String(), configName),
		audit.Project(projectName),
		audit.Detail("agentName", agentName),
		audit.Detail("configName", configName),
		audit.Detail("updatedFields", updatedFields),
	)
	s.logger.Info(
		"Agent configuration updated successfully",
		"config_uuid", configUUID,
		"ou_id", ouID,
		"updated_fields", updatedFields,
	)
}

// DeleteMCP deletes an MCP proxy mapping and all associated resources.
func (s *agentConfigurationService) DeleteMCP(ctx context.Context, configUUID uuid.UUID, ouID, projectName, agentName string) error {
	config, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrAgentConfigNotFound
		}
		return fmt.Errorf("failed to get MCP configuration: %w", err)
	}
	if config.ProjectName != projectName || config.AgentID != agentName || config.TypeID != models.AgentConfigTypeIDMCP {
		return utils.ErrAgentConfigNotFound
	}
	return s.deleteMCPConfig(ctx, config, ouID, projectName, agentName)
}

// Delete deletes a configuration and all associated resources with project and agent scoping validation
func (s *agentConfigurationService) Delete(ctx context.Context, configUUID uuid.UUID, ouID, projectName, agentName string) error {
	// Get configuration and mappings
	existingConfig, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrAgentConfigNotFound
		}
		return fmt.Errorf("failed to get configuration: %w", err)
	}

	// Validate project and agent scoping
	if existingConfig.ProjectName != projectName || existingConfig.AgentID != agentName {
		return utils.ErrAgentConfigNotFound
	}

	switch existingConfig.TypeID {
	case models.AgentConfigTypeIDMCP:
		return s.deleteMCPConfig(ctx, existingConfig, ouID, projectName, agentName)
	default:
		return s.deleteLLMConfig(ctx, existingConfig, ouID, projectName, agentName)
	}
}

func (s *agentConfigurationService) deleteLLMConfig(ctx context.Context, existingConfig *models.AgentConfiguration, ouID, projectName, agentName string) error {
	configUUID := existingConfig.UUID

	// Determine agent type for internal-agent cleanup decisions.
	// Fail closed: if GetComponent errors, return rather than defaulting to internal (which could corrupt CRs).
	agentComp, agentErr := s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
	if agentErr != nil {
		return fmt.Errorf("failed to determine agent type: %w", agentErr)
	}
	isExternalAgent := agentComp.Provisioning.Type == string(utils.ExternalAgent)

	s.logger.Info("Deleting agent configuration", "config_uuid", existingConfig.UUID, "name", existingConfig.Name)

	// Get all environment mappings
	mappings, err := s.envMappingRepo.ListByConfig(ctx, configUUID)
	if err != nil {
		return fmt.Errorf("failed to list environment mappings: %w", err)
	}

	environments, err := s.ocClient.ListEnvironments(ctx, ouID)
	if err != nil {
		return fmt.Errorf("error while list environments from open choreo. %w", err)
	}

	envIDNameMap := make(map[string]string)

	for _, env := range environments {
		envIDNameMap[env.UUID] = env.Name
	}

	// Steps 1-4: Per-mapping cleanup in strict order before DB deletion.
	// External resources are cleaned up before DB deletion so that if any step fails,
	// the DB row remains and the caller can retry. On retry, already-deleted external
	// resources are skipped gracefully.
	// Order matters: revoke API keys (1) before undeploying (2) so the gateway still has
	// the proxy config when it processes the revocation event.
	//
	// Key names mirror the naming convention used during Create/buildLLMProxyConfig:
	//   proxyHandle       = "{configPrefix}-{hash}-{providerHash}-proxy"  (= Configuration.Name)
	//   proxy API key     = "{configPrefix}-{hash}-{providerHash}-key"
	//   provider API key  = "{configPrefix}-{hash}-{providerHash}-proxy"  (= proxyHandle)
	for _, mapping := range mappings {
		if mapping.LLMProxy == nil {
			continue
		}
		env, ok := envIDNameMap[mapping.EnvironmentUUID.String()]
		if !ok {
			s.logger.Warn("environment is not available in openchoreo")
			continue
		}

		// Handle is backfilled from Configuration.Name by the repository
		// ("{configPrefix}-{hash}-{providerHash}-proxy"), since LLMProxy.Handle is gorm:"-".
		proxyHandle := mapping.LLMProxy.Handle

		// Step 1: Revoke API keys (must happen before undeployment so the gateway still has
		// the proxy config when it processes the revocation event).
		proxyKeyName := fmt.Sprintf("%s-key", strings.TrimSuffix(proxyHandle, "-proxy"))
		providerKeyName := proxyHandle

		s.logger.Info("Revoking API keys", "proxy_handle", proxyHandle, "proxy_key_name", proxyKeyName, "provider_key_name", providerKeyName)

		if err := s.llmProxyAPIKeyService.RevokeAPIKey(ctx, ouID, proxyHandle, proxyKeyName); err != nil {
			s.logger.Warn(
				"Failed to revoke proxy API key during deletion (best-effort)",
				"proxy_handle", proxyHandle,
				"key_name", proxyKeyName,
				"error", err,
			)
		}

		// Revoke provider API key (only if provider auth was configured).
		if mapping.LLMProxy.Configuration.UpstreamAuth != nil {
			providerUUID := mapping.LLMProxy.ProviderUUID.String()
			if err := s.llmProviderAPIKeyService.RevokeAPIKey(ctx, ouID, providerUUID, providerKeyName); err != nil {
				s.logger.Warn(
					"Failed to revoke provider API key during deletion (best-effort)",
					"provider_uuid", providerUUID,
					"key_name", providerKeyName,
					"error", err,
				)
			}
		}

		// Load the persisted SecretReference name from DB. This is the name returned by the
		// secret management system at creation time (e.g., "cred-wc-..." from the Secret Manager API)
		// and must be used instead of recomputing via SecretRefName() which may produce a different name.
		var persistedSecretRefName string
		vars, varLoadErr := s.envVariableRepo.ListByConfigAndEnv(ctx, configUUID, mapping.EnvironmentUUID)
		if varLoadErr != nil {
			s.logger.Warn("failed to load env config variables for SecretReference lookup on delete", "error", varLoadErr)
		} else {
			for _, v := range vars {
				if v.SecretReference != "" {
					persistedSecretRefName = v.SecretReference
					break
				}
			}
		}
		if persistedSecretRefName == "" {
			s.logger.Warn("no persisted SecretReference found for config, skipping SecretReference deletion",
				"config_uuid", configUUID, "environment", env)
		}

		// Step 1b: Delete SecretReference CR (internal agents only, best-effort).
		if !isExternalAgent && persistedSecretRefName != "" {
			s.logger.Info("Delete: using persisted SecretReference for deletion",
				"secret_ref", persistedSecretRefName,
				"config_uuid", configUUID, "environment", env)
			if err := s.ocClient.DeleteSecretReference(ctx, ouID, persistedSecretRefName); err != nil {
				s.logger.Warn("failed to delete SecretReference on config delete",
					"name", persistedSecretRefName, "error", err)
			}
		}

		// Step 2: Undeploy proxy deployments.
		s.logger.Info(
			"Cleaning up proxy deployments for deleted config",
			"config_uuid", configUUID,
			"proxy_handle", proxyHandle,
		)

		deployments, err := s.llmProxyDeploymentService.GetLLMProxyDeployments(proxyHandle, ouID, nil, nil)
		if err != nil {
			if errors.Is(err, utils.ErrLLMProxyNotFound) {
				// Proxy already gone — skip deployment cleanup for this mapping.
				s.logger.Info(
					"Proxy already deleted, skipping deployment cleanup",
					"proxy_handle", proxyHandle,
				)
			} else {
				return fmt.Errorf("failed to get deployments for proxy %q: %w", proxyHandle, err)
			}
		} else {
			for _, dep := range deployments {
				if _, err := s.llmProxyDeploymentService.UndeployLLMProxyDeployment(ctx, proxyHandle, dep.DeploymentID.String(), dep.GatewayUUID.String(), ouID); err != nil {
					s.logger.Error(
						"Failed to undeploy deployment during cleanup",
						"proxy_handle", proxyHandle,
						"deployment_id", dep.DeploymentID,
						"gateway_id", dep.GatewayUUID,
						"error", err,
					)
				}
			}
		}

		// Step 3: Delete proxy record.
		if err := s.llmProxyService.Delete(proxyHandle, ouID); err != nil {
			// ErrLLMProxyNotFound means already deleted — treat as success.
			if !errors.Is(err, utils.ErrLLMProxyNotFound) {
				return fmt.Errorf("failed to delete proxy %q: %w", proxyHandle, err)
			}
			s.logger.Info("Proxy already deleted, skipping", "proxy_handle", proxyHandle)
		}

		// Delete proxy API key secret
		// Step 4: Delete KV secrets for proxy API key (used by SecretReference CR).
		// Note: provider upstream auth is encrypted in the DB and deleted with the proxy record.
		// SecretReference CR is already deleted in Step 1b above, so we pass the persisted name
		// to avoid a redundant (and potentially incorrect) deletion attempt.
		proxySecretLoc := secretmanagersvc.SecretLocation{
			OrgName:         existingConfig.OUID,
			ProjectName:     existingConfig.ProjectName,
			AgentName:       existingConfig.AgentID,
			EnvironmentName: env,
			ConfigName:      existingConfig.Name,
			EntityName:      proxyHandle,
			SecretKey:       secretmanagersvc.SecretKeyAPIKey,
		}
		// Use persisted name when available; fall back to computed name so the
		// KV secret deletion (location-based) still proceeds and DeleteSecret
		// receives a valid SecretReference name for its internal cleanup.
		secretRefForDelete := persistedSecretRefName
		if secretRefForDelete == "" {
			secretRefForDelete = proxySecretLoc.SecretRefName()
		}
		if err := s.secretClient.DeleteSecret(ctx, proxySecretLoc, secretRefForDelete); err != nil {
			return fmt.Errorf("failed to delete proxy API key from KV for proxy %q: %w",
				proxyHandle, err)
		}
	}

	// Step 4b: Remove env vars from Component CR and all ReleaseBindings (internal agents only, best-effort).
	// Must use names from DB (not auto-generated) to handle user-overridden names correctly.
	if !isExternalAgent {
		existingVarNames, varErr := s.loadExistingVarNames(ctx, configUUID)
		if varErr != nil {
			s.logger.Warn("failed to load var names for cleanup, skipping env var removal", "error", varErr)
		} else {
			envConfigTemplates, _ := s.buildEnvironmentVariables(existingConfig.Name, varNamesToOverrides(existingVarNames))
			keysToRemove := make([]string, 0, len(envConfigTemplates))
			for _, t := range envConfigTemplates {
				keysToRemove = append(keysToRemove, t.Name)
			}
			// Remove from Component CR.
			if err := s.ocClient.RemoveComponentEnvironmentVariables(ctx, ouID, projectName, agentName, keysToRemove); err != nil {
				s.logger.Warn("failed to remove env vars from Component CR on config delete", "error", err)
			}
			// Remove from Workload (live runtime resource) so stale env vars don't persist
			// and get re-injected by getSystemManagedEnvVars on the next deploy.
			if err := s.ocClient.RemoveWorkloadEnvVars(ctx, ouID, agentName, keysToRemove); err != nil {
				s.logger.Warn("failed to remove env vars from Workload on config delete", "error", err)
			}
			// Remove from each environment's ReleaseBinding.
			for _, mapping := range mappings {
				env, ok := envIDNameMap[mapping.EnvironmentUUID.String()]
				if !ok {
					continue
				}
				if err := s.ocClient.RemoveReleaseBindingEnvVars(ctx, ouID, projectName, agentName, env, keysToRemove); err != nil {
					s.logger.Warn("failed to remove env vars from ReleaseBinding on config delete",
						"environment", env, "error", err)
				}
			}
		}
	}

	// Step 5: Delete DB records only after all external resources are confirmed cleaned up.
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Delete configuration (cascades to mappings and variables)
		if err := s.agentConfigRepo.Delete(ctx, tx, configUUID, ouID); err != nil {
			return fmt.Errorf("failed to delete configuration: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	audit.Record(
		ctx, audit.ActionAgentConfigDelete,
		audit.Org(ouID),
		audit.ResourceNamed("agent-config", configUUID.String(), existingConfig.Name),
		audit.Project(projectName),
		audit.Detail("agentName", agentName),
		audit.Detail("configName", existingConfig.Name),
		audit.Detail("environmentCount", len(mappings)),
	)
	s.logger.Info(
		"Agent configuration deleted successfully",
		"config_uuid", configUUID,
		"config_name", existingConfig.Name,
		"ou_id", ouID,
		"environment_count", len(mappings),
	)

	return nil
}

// DeleteForAgentDeletion cleans up all external proxy resources for a single LLM config as part
// of agent deletion. Compared to Delete, it skips:
//   - GetComponent (caller resolves isExternalAgent once for all configs)
//   - SecretReference CR deletion (component teardown handles it)
//   - Component/Workload/ReleaseBinding env-var patching (component is being deleted)
//
// Steps retained: revoke API keys → undeploy proxy deployments → delete proxy record → delete KV secret → delete DB record.
// Best-effort: individual step failures are logged but do not abort the overall agent deletion.
func (s *agentConfigurationService) DeleteForAgentDeletion(ctx context.Context, configUUID uuid.UUID, ouID, projectName, agentName string, isExternalAgent bool) error {
	existingConfig, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrAgentConfigNotFound
		}
		return fmt.Errorf("failed to get configuration: %w", err)
	}

	if existingConfig.ProjectName != projectName || existingConfig.AgentID != agentName {
		return utils.ErrAgentConfigNotFound
	}

	if existingConfig.TypeID == models.AgentConfigTypeIDMCP {
		return s.deleteMCPConfigForAgentDeletion(ctx, existingConfig, ouID)
	}

	s.logger.Info("Deleting agent configuration for agent deletion", "config_uuid", existingConfig.UUID, "name", existingConfig.Name)

	mappings, err := s.envMappingRepo.ListByConfig(ctx, configUUID)
	if err != nil {
		return fmt.Errorf("failed to list environment mappings: %w", err)
	}

	environments, err := s.ocClient.ListEnvironments(ctx, ouID)
	if err != nil {
		return fmt.Errorf("failed to list environments: %w", err)
	}
	envIDNameMap := make(map[string]string, len(environments))
	for _, env := range environments {
		envIDNameMap[env.UUID] = env.Name
	}

	var cleanupErrs []string
	for _, mapping := range mappings {
		if mapping.LLMProxy == nil {
			continue
		}
		env, ok := envIDNameMap[mapping.EnvironmentUUID.String()]
		if !ok {
			s.logger.Warn("environment not available in openchoreo, skipping mapping", "environment_uuid", mapping.EnvironmentUUID)
			continue
		}

		proxyHandle := mapping.LLMProxy.Handle
		proxyKeyName := fmt.Sprintf("%s-key", strings.TrimSuffix(proxyHandle, "-proxy"))
		providerKeyName := proxyHandle

		// Step 1: Revoke proxy API key. ErrLLMProxyNotFound means already gone — idempotent.
		if err := s.llmProxyAPIKeyService.RevokeAPIKey(ctx, ouID, proxyHandle, proxyKeyName); err != nil {
			if !errors.Is(err, utils.ErrLLMProxyNotFound) {
				s.logger.Warn("Failed to revoke proxy API key during agent deletion",
					"proxy_handle", proxyHandle, "key_name", proxyKeyName, "error", err)
				cleanupErrs = append(cleanupErrs, fmt.Sprintf("revoke proxy key %s: %v", proxyKeyName, err))
			}
		}

		// Step 2: Revoke provider API key (only if provider auth was configured).
		// ErrLLMProviderNotFound means already gone — idempotent.
		if mapping.LLMProxy.Configuration.UpstreamAuth != nil {
			providerUUID := mapping.LLMProxy.ProviderUUID.String()
			if err := s.llmProviderAPIKeyService.RevokeAPIKey(ctx, ouID, providerUUID, providerKeyName); err != nil {
				if !errors.Is(err, utils.ErrLLMProviderNotFound) {
					s.logger.Warn("Failed to revoke provider API key during agent deletion",
						"provider_uuid", providerUUID, "key_name", providerKeyName, "error", err)
					cleanupErrs = append(cleanupErrs, fmt.Sprintf("revoke provider key %s: %v", providerKeyName, err))
				}
			}
		}

		// Step 3: Undeploy proxy deployments.
		deployments, err := s.llmProxyDeploymentService.GetLLMProxyDeployments(proxyHandle, ouID, nil, nil)
		if err != nil {
			if !errors.Is(err, utils.ErrLLMProxyNotFound) {
				s.logger.Warn("Failed to get proxy deployments during agent deletion",
					"proxy_handle", proxyHandle, "error", err)
				cleanupErrs = append(cleanupErrs, fmt.Sprintf("get deployments for proxy %s: %v", proxyHandle, err))
			}
		} else {
			for _, dep := range deployments {
				if _, err := s.llmProxyDeploymentService.UndeployLLMProxyDeployment(ctx, proxyHandle, dep.DeploymentID.String(), dep.GatewayUUID.String(), ouID); err != nil {
					s.logger.Warn("Failed to undeploy proxy deployment during agent deletion",
						"proxy_handle", proxyHandle, "deployment_id", dep.DeploymentID, "error", err)
					cleanupErrs = append(cleanupErrs, fmt.Sprintf("undeploy %s deployment %s: %v", proxyHandle, dep.DeploymentID, err))
				}
			}
		}

		// Step 4: Delete proxy record.
		if err := s.llmProxyService.Delete(proxyHandle, ouID); err != nil {
			if !errors.Is(err, utils.ErrLLMProxyNotFound) {
				s.logger.Warn("Failed to delete proxy record during agent deletion",
					"proxy_handle", proxyHandle, "error", err)
				cleanupErrs = append(cleanupErrs, fmt.Sprintf("delete proxy %s: %v", proxyHandle, err))
			}
		}

		// Step 5: Delete KV secret for proxy API key. Load the persisted SecretReference name
		// from DB so DeleteSecret receives the correct name even if recomputation would differ.
		var persistedSecretRefName string
		vars, varLoadErr := s.envVariableRepo.ListByConfigAndEnv(ctx, configUUID, mapping.EnvironmentUUID)
		if varLoadErr != nil {
			s.logger.Warn("failed to load env config variables for KV secret deletion", "error", varLoadErr)
		} else {
			for _, v := range vars {
				if v.SecretReference != "" {
					persistedSecretRefName = v.SecretReference
					break
				}
			}
		}

		proxySecretLoc := secretmanagersvc.SecretLocation{
			OrgName:         existingConfig.OUID,
			ProjectName:     existingConfig.ProjectName,
			AgentName:       existingConfig.AgentID,
			EnvironmentName: env,
			ConfigName:      existingConfig.Name,
			EntityName:      proxyHandle,
			SecretKey:       secretmanagersvc.SecretKeyAPIKey,
		}
		secretRefForDelete := persistedSecretRefName
		if secretRefForDelete == "" {
			secretRefForDelete = proxySecretLoc.SecretRefName()
		}
		if err := s.secretClient.DeleteSecret(ctx, proxySecretLoc, secretRefForDelete); err != nil {
			s.logger.Warn("Failed to delete proxy API key from KV during agent deletion",
				"proxy_handle", proxyHandle, "error", err)
			cleanupErrs = append(cleanupErrs, fmt.Sprintf("delete KV secret for proxy %s: %v", proxyHandle, err))
		}
	}

	// Step 6: Delete DB record only when all external resources were cleaned up successfully.
	// If any step above failed, return an error so the DB row is preserved and the caller
	// (deleteAgentLLMConfigurations) can log it — the row will be retried on the next
	// agent deletion attempt via the idempotent delete path.
	if len(cleanupErrs) > 0 {
		return fmt.Errorf("external cleanup incomplete for config %s, DB record preserved for retry: %s",
			configUUID, strings.Join(cleanupErrs, "; "))
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return s.agentConfigRepo.Delete(ctx, tx, configUUID, ouID)
	}); err != nil {
		return fmt.Errorf("failed to delete configuration from DB: %w", err)
	}

	s.logger.Info("Agent configuration deleted for agent deletion",
		"config_uuid", configUUID, "config_name", existingConfig.Name, "ou_id", ouID)

	return nil
}

func (s *agentConfigurationService) deleteMCPConfig(ctx context.Context, existingConfig *models.AgentConfiguration, ouID, projectName, agentName string) error {
	if s.envMCPMappingRepo == nil {
		return fmt.Errorf("MCP configuration repository is not configured")
	}
	mappings, err := s.envMCPMappingRepo.ListByConfig(ctx, existingConfig.UUID)
	if err != nil {
		return fmt.Errorf("failed to list MCP environment mappings: %w", err)
	}

	envs, err := s.ocClient.ListEnvironments(ctx, ouID)
	if err != nil {
		return fmt.Errorf("error while list environments from open choreo. %w", err)
	}
	envIDNameMap := make(map[string]string, len(envs))
	for _, env := range envs {
		envIDNameMap[env.UUID] = env.Name
	}

	agentComp, agentErr := s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
	if agentErr != nil {
		return fmt.Errorf("failed to determine agent type: %w", agentErr)
	}
	isExternalAgent := agentComp.Provisioning.Type == string(utils.ExternalAgent)
	if !isExternalAgent {
		existingVars, varErr := s.envVariableRepo.ListByConfig(ctx, existingConfig.UUID)
		if varErr != nil {
			s.logger.Warn("failed to load MCP var names for cleanup, skipping env var removal", "error", varErr)
		} else {
			componentKeysToRemove := uniqueVariableNames(existingVars)
			if len(componentKeysToRemove) > 0 {
				if err := s.ocClient.RemoveComponentEnvironmentVariables(ctx, ouID, projectName, agentName, componentKeysToRemove); err != nil {
					s.logger.Warn("failed to remove MCP env vars from Component CR on config delete", "error", err)
				}
				if err := s.ocClient.RemoveWorkloadEnvVars(ctx, ouID, agentName, componentKeysToRemove); err != nil {
					s.logger.Warn("failed to remove MCP env vars from Workload on config delete", "error", err)
				}
			}
			for _, mapping := range mappings {
				envName := envIDNameMap[mapping.EnvironmentUUID.String()]
				if envName == "" {
					continue
				}
				keysToRemove := variableNamesForEnv(existingVars, mapping.EnvironmentUUID)
				if len(keysToRemove) == 0 {
					continue
				}
				if err := s.ocClient.RemoveReleaseBindingEnvVars(ctx, ouID, projectName, agentName, envName, keysToRemove); err != nil {
					s.logger.Warn("failed to remove MCP env vars from ReleaseBinding on config delete",
						"environment", envName, "error", err)
				}
			}
		}
	}

	for _, mapping := range mappings {
		if s.mcpProxyService != nil {
			s.mcpProxyService.BroadcastMCPArtifactDeletion(ctx, mapping.ArtifactUUID, ouID)
		}
		envName := envIDNameMap[mapping.EnvironmentUUID.String()]
		s.cleanupMCPMappingCredentials(ctx, existingConfig, &mapping, envName, ouID)
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.agentConfigRepo.Delete(ctx, tx, existingConfig.UUID, ouID); err != nil {
			return err
		}
		for _, mapping := range mappings {
			if err := tx.Where("artifact_uuid = ? AND ou_id = ?", mapping.ArtifactUUID, ouID).
				Delete(&models.DeploymentStatusRecord{}).Error; err != nil {
				return err
			}
			if err := tx.Where("artifact_uuid = ? AND ou_id = ?", mapping.ArtifactUUID, ouID).
				Delete(&models.Deployment{}).Error; err != nil {
				return err
			}
			if err := tx.Where("uuid = ?", mapping.ArtifactUUID).Delete(&models.Artifact{}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to delete MCP configuration: %w", err)
	}

	// Removing this config drops its scopes from the agent's AgentID scope union
	// just as much as adding one grows it (see createMCPConfig's identical call)
	// — refresh every environment this config was bound to so an already-running
	// pod stops requesting the now-removed MCP's scopes right away, instead of
	// only on its next deploy/promote/rotation.
	touchedEnvNames := make(map[string]struct{}, len(mappings))
	for _, mapping := range mappings {
		if envName := envIDNameMap[mapping.EnvironmentUUID.String()]; envName != "" {
			touchedEnvNames[envName] = struct{}{}
		}
	}
	go func() {
		refreshCtx, cancel := detachedRefreshContext(ctx)
		defer cancel()
		s.refreshTouchedMCPEnvironments(refreshCtx, ouID, projectName, agentName, touchedEnvNames)
	}()

	return nil
}

func (s *agentConfigurationService) deleteMCPConfigForAgentDeletion(ctx context.Context, existingConfig *models.AgentConfiguration, ouID string) error {
	if s.envMCPMappingRepo == nil {
		return fmt.Errorf("MCP configuration repository is not configured")
	}
	mappings, err := s.envMCPMappingRepo.ListByConfig(ctx, existingConfig.UUID)
	if err != nil {
		return fmt.Errorf("failed to list MCP environment mappings: %w", err)
	}
	envs, err := s.ocClient.ListEnvironments(ctx, ouID)
	if err != nil {
		s.logger.Warn("failed to list environments for MCP credential cleanup during agent deletion", "error", err)
	}
	envIDNameMap := make(map[string]string, len(envs))
	for _, env := range envs {
		envIDNameMap[env.UUID] = env.Name
	}

	for _, mapping := range mappings {
		if s.mcpProxyService != nil {
			s.mcpProxyService.BroadcastMCPArtifactDeletion(ctx, mapping.ArtifactUUID, ouID)
		}
		envName := envIDNameMap[mapping.EnvironmentUUID.String()]
		s.cleanupMCPMappingCredentials(ctx, existingConfig, &mapping, envName, ouID)
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.agentConfigRepo.Delete(ctx, tx, existingConfig.UUID, ouID); err != nil {
			return err
		}
		for _, mapping := range mappings {
			if err := tx.Where("artifact_uuid = ? AND ou_id = ?", mapping.ArtifactUUID, ouID).
				Delete(&models.DeploymentStatusRecord{}).Error; err != nil {
				return err
			}
			if err := tx.Where("artifact_uuid = ? AND ou_id = ?", mapping.ArtifactUUID, ouID).
				Delete(&models.Deployment{}).Error; err != nil {
				return err
			}
			if err := tx.Where("uuid = ?", mapping.ArtifactUUID).Delete(&models.Artifact{}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to delete MCP configuration for agent deletion: %w", err)
	}

	s.logger.Info("MCP agent configuration deleted for agent deletion",
		"config_uuid", existingConfig.UUID, "config_name", existingConfig.Name, "ou_id", ouID)
	return nil
}

// Helper methods

// resolveGatewayForProvider returns the gateway hosting this LLM provider in the given
// environment, or the environment's single egress gateway when it has no deployment there.
func (s *agentConfigurationService) resolveGatewayForProvider(
	ctx context.Context, providerUUIDStr, ouID string, envUUID uuid.UUID,
) (*models.Gateway, error) {
	_ = ctx
	var deployed []string
	if providerUUID, err := uuid.Parse(providerUUIDStr); err == nil {
		ids, gwErr := s.llmProxyDeploymentService.GetDeployedGatewaysByProvider(providerUUID, ouID)
		if gwErr != nil {
			return nil, fmt.Errorf("failed to list deployed gateways for provider %s: %w", providerUUIDStr, gwErr)
		}
		deployed = ids
	}
	return resolveEgressGatewayForArtifact(s.gatewayRepo, ouID, envUUID, deployed, nil)
}

// resolveGatewayForProxy returns the gateway the LLM proxy is actually deployed to.
func (s *agentConfigurationService) resolveGatewayForProxy(
	ctx context.Context, proxyHandle, ouID string, envUUID uuid.UUID,
) (*models.Gateway, error) {
	_ = ctx
	deployedStatus := string(models.DeploymentStatusDeployed)
	var deployed []string
	deployments, err := s.llmProxyDeploymentService.GetLLMProxyDeployments(proxyHandle, ouID, nil, &deployedStatus)
	if err != nil && !errors.Is(err, utils.ErrLLMProxyNotFound) {
		return nil, fmt.Errorf("failed to list deployments for proxy %s: %w", proxyHandle, err)
	}
	for _, dep := range deployments {
		deployed = append(deployed, dep.GatewayUUID.String())
	}
	return resolveEgressGatewayForArtifact(s.gatewayRepo, ouID, envUUID, deployed, nil)
}

// errMCPEnvNotDeployable reports that an MCP proxy cannot back an agent binding in an
// environment. Not a failure: the connection is still recorded and its env vars are still
// injected, they just resolve to nothing until the proxy gains an endpoint there.
var errMCPEnvNotDeployable = errors.New("MCP proxy has no deployable endpoint in this environment")

// resolveDeployableMCPGateway returns the gateway that can back an agent binding to proxy in
// envUUID. It is the single definition of deployability: the proxy needs an endpoint bound to
// the environment, a shared gateway artifact owned by that binding, and a gateway mapped to
// the environment. Any of those missing yields errMCPEnvNotDeployable; every other error is
// unexpected and worth surfacing.
func (s *agentConfigurationService) resolveDeployableMCPGateway(
	ctx context.Context, proxy *models.MCPProxy, ouID string, envUUID uuid.UUID,
) (*models.Gateway, error) {
	if endpoint, _ := resolveMCPEndpointForEnv(proxy, envUUID.String()); endpoint == nil {
		return nil, errMCPEnvNotDeployable
	}
	sharedArtifactUUID := mcpProxyEnvArtifactUUID(proxy, envUUID.String())
	if sharedArtifactUUID == uuid.Nil {
		s.logger.Warn("Treating MCP environment as non-deployable; missing shared artifact",
			"ou_id", ouID, "correlation_id", utils.GetCorrelationId(ctx),
			"environment_uuid", envUUID, "mcp_proxy_uuid", proxy.UUID)
		return nil, errMCPEnvNotDeployable
	}
	gateway, err := s.resolveGatewayForMCPArtifact(ctx, sharedArtifactUUID, ouID, envUUID)
	if err != nil {
		if errors.Is(err, errNoGatewayForEnvironment) {
			return nil, errMCPEnvNotDeployable
		}
		return nil, err
	}
	return gateway, nil
}

// resolveGatewayForMCPArtifact returns the gateway the MCP proxy's shared per-environment
// artifact is deployed to.
func (s *agentConfigurationService) resolveGatewayForMCPArtifact(
	ctx context.Context, artifactUUID uuid.UUID, ouID string, envUUID uuid.UUID,
) (*models.Gateway, error) {
	_ = ctx
	var deployed []string
	if s.mcpProxyService != nil && s.mcpProxyService.deploymentRepo != nil {
		ids, err := s.mcpProxyService.deploymentRepo.GetDeployedGatewaysByProvider(artifactUUID, ouID)
		if err != nil {
			return nil, fmt.Errorf("failed to list deployed gateways for MCP artifact %s: %w", artifactUUID, err)
		}
		deployed = ids
	}
	return resolveEgressGatewayForArtifact(s.gatewayRepo, ouID, envUUID, deployed, nil)
}

// buildLLMProxyConfig constructs proxy configuration from request.
// Returns the proxy config, provider API key ID, provider UUID, provider secret KV path, the
// scoped ID used to derive the proxy's handle, and any error. The provider UUID is needed by
// rollbackProxies to revoke the provider API key on failure. The scoped ID is returned so callers
// can name related resources (deployment, proxy API key) without re-deriving it from proxy.Handle.
//
// The proxy handle folds in the target provider's UUID, so a proxy's identity is always
// provider-specific. This matters most on a provider swap (processEnvProviderChange): the
// replacement proxy is created before the old one is deleted, so if the handle didn't vary by
// provider, the new proxy would collide with the still-live old one under the exact same handle.
func (s *agentConfigurationService) buildLLMProxyConfig(
	ctx context.Context,
	config *models.AgentConfiguration,
	envName string,
	envMapping models.EnvModelConfigRequest,
) (*models.LLMProxy, string, string, *secretmanagersvc.SecretLocation, string, error) {
	project, err := s.ocClient.GetProject(ctx, config.OUID, config.ProjectName)
	if err != nil {
		return nil, "", "", nil, "", fmt.Errorf("failed to get project from openchoreo: %w", err)
	}

	// Get provider details
	provider, err := s.llmProviderRepo.GetByHandle(envMapping.ProviderName, config.OUID)
	if err != nil {
		return nil, "", "", nil, "", fmt.Errorf("failed to get provider: %w", err)
	}

	providerHash := sha256.Sum256([]byte(provider.UUID.String()))
	scopedID := fmt.Sprintf("%s-%s",
		scopedProxyIdentifier(config.ProjectName, config.AgentID, config.Name, envName),
		hex.EncodeToString(providerHash[:4]))
	proxyName := fmt.Sprintf("%s-proxy", scopedID)
	contextPath := fmt.Sprintf("/%s", scopedID)

	apiKeyId := ""
	providerUUID := provider.UUID.String()
	var providerSecretLoc *secretmanagersvc.SecretLocation

	// Parse project UUID
	projectUUID, err := uuid.Parse(project.UUID)
	if err != nil {
		return nil, "", "", nil, "", fmt.Errorf("invalid project UUID from openchoreo: %w", err)
	}

	enabled := true
	// Build proxy configuration
	proxyConfig := &models.LLMProxy{
		Description: fmt.Sprintf("LLM proxy for agent %s", config.AgentID),
		ProjectUUID: projectUUID,
		Configuration: models.LLMProxyConfig{
			Name:     proxyName,
			Version:  models.DefaultProxyVersion,
			Context:  &contextPath,
			Provider: provider.UUID.String(),
			Security: &models.SecurityConfig{
				Enabled: &enabled,
				APIKey: &models.APIKeySecurity{
					Enabled: &enabled,
					Key:     "API-Key",
					In:      "header",
				},
			},
			Policies:   envMapping.Configuration.Policies,
			Resilience: envMapping.Configuration.Resilience,
		},
	}

	var upstreamAuthConfig models.UpstreamAuth

	providerSecurityConfig := provider.Configuration.Security
	if providerSecurityConfig != nil && providerSecurityConfig.Enabled != nil && *providerSecurityConfig.Enabled {
		// Provider is secured.
		providerApiKeyConfig := providerSecurityConfig.APIKey

		if providerApiKeyConfig != nil && providerApiKeyConfig.Enabled != nil && *providerApiKeyConfig.Enabled {
			// Provider api key security is enabled.
			apiKey, err := s.llmProviderAPIKeyService.CreateAPIKey(ctx, config.OUID, provider.UUID.String(), &models.CreateAPIKeyRequest{
				Name:        proxyName,
				DisplayName: proxyName,
				Purpose:     models.APIKeyPurposeConsoleManaged,
			})
			s.logger.Info("Created provider API key", "provider_uuid", provider.UUID.String(), "provider_key_name", proxyName)
			if err != nil {
				return nil, "", "", nil, "", fmt.Errorf("failed to create api key for provider: %w", err)
			}

			apiKeyId = apiKey.KeyID

			// Encrypt the provider API key for storage in UpstreamAuth.SecretRef
			encrypted, err := utils.EncryptBytes([]byte(apiKey.APIKey), s.encryptionKey)
			if err != nil {
				// revoke created api key
				if revokeErr := s.llmProviderAPIKeyService.RevokeAPIKey(ctx, config.OUID, provider.UUID.String(), proxyName); revokeErr != nil {
					s.logger.Error(
						"Failed to revoke provider API key after encryption failure",
						"provider_uuid", provider.UUID.String(),
						"provider_key_name", proxyName,
						"error", revokeErr,
					)
				}
				return nil, "", "", nil, "", fmt.Errorf("failed to encrypt provider API key: %w", err)
			}
			encoded := base64.StdEncoding.EncodeToString(encrypted)
			upstreamAuthConfig.Type = utils.StrAsStrPointer(models.AuthTypeAPIKey)
			upstreamAuthConfig.Header = utils.StrAsStrPointer(providerApiKeyConfig.Key)
			upstreamAuthConfig.SecretRef = &encoded // Store encrypted value instead of plaintext
			upstreamAuthConfig.Value = nil          // No plaintext in DB
			proxyConfig.Configuration.UpstreamAuth = &upstreamAuthConfig
		}
	}

	return proxyConfig, apiKeyId, providerUUID, providerSecretLoc, scopedID, nil
}

// buildLLMProxyUpdateConfig builds a proxy config for the Update flow (Scenario B).
// It preserves the existing proxy's Name, Context, Security, and ProjectUUID —
// only mutable fields (Provider, UpstreamAuth, Policies) are updated.
func (s *agentConfigurationService) buildLLMProxyUpdateConfig(
	config *models.AgentConfiguration,
	envMapping models.EnvModelConfigRequest,
	existingProxy *models.LLMProxy,
) (*models.LLMProxy, string, error) {
	provider, err := s.llmProviderRepo.GetByHandle(envMapping.ProviderName, config.OUID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get provider: %w", err)
	}
	providerUUID := provider.UUID.String()

	proxyConfig := &models.LLMProxy{
		Description: fmt.Sprintf("LLM proxy for agent %s", config.AgentID),
		ProjectUUID: existingProxy.ProjectUUID,
		Configuration: models.LLMProxyConfig{
			Name:         existingProxy.Configuration.Name,
			Version:      models.DefaultProxyVersion,
			Context:      existingProxy.Configuration.Context,
			Provider:     provider.UUID.String(),
			Security:     existingProxy.Configuration.Security,
			Policies:     envMapping.Configuration.Policies,
			UpstreamAuth: existingProxy.Configuration.UpstreamAuth,
			Resilience:   envMapping.Configuration.Resilience,
		},
	}

	return proxyConfig, providerUUID, nil
}

// buildEnvironmentVariables generates environment variable templates from config name.
// If overrides are provided, user-supplied names take precedence over auto-generated ones.
// Validates all names using ValidateEnvironmentVariableName.
func (s *agentConfigurationService) buildEnvironmentVariables(configName string, overrides []models.EnvironmentVariableConfig) ([]EnvConfigTemplate, error) {
	// Sanitize: Replace any character not in A-Za-z0-9_ with '_'
	prefix := strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, configName)

	// Convert to uppercase
	prefix = strings.ToUpper(prefix)

	// If prefix starts with a digit, prepend underscore
	if len(prefix) > 0 && prefix[0] >= '0' && prefix[0] <= '9' {
		prefix = "_" + prefix
	}

	// Known keys with their secrets flag and auto-generated name
	type keyMeta struct {
		isSecret bool
		autoName string
	}
	knownKeys := map[string]keyMeta{
		"url":    {isSecret: false, autoName: fmt.Sprintf("%s_URL", prefix)},
		"apikey": {isSecret: true, autoName: fmt.Sprintf("%s_API_KEY", prefix)},
	}

	// Build override map from user input; reject unknown keys
	overrideMap := make(map[string]string)
	seen := make(map[string]bool)
	for _, ov := range overrides {
		if _, known := knownKeys[ov.Key]; !known {
			return nil, fmt.Errorf("unknown environment variable key %q: must be one of url, apikey", ov.Key)
		}
		if seen[ov.Key] {
			return nil, fmt.Errorf("duplicate environment variable key %q", ov.Key)
		}
		seen[ov.Key] = true
		overrideMap[ov.Key] = ov.Name
	}

	// Determine final name for each key (override wins, then auto-generate).
	// Iterate in a fixed order so the returned slice is deterministic.
	keyOrder := []string{"url", "apikey"}
	envConfigTemplates := make([]EnvConfigTemplate, 0, len(knownKeys))
	usedNames := make(map[string]string) // name -> key, for duplicate detection
	for _, key := range keyOrder {
		meta := knownKeys[key]
		name := meta.autoName
		if customName, ok := overrideMap[key]; ok {
			name = customName
		}
		if err := utils.ValidateEnvironmentVariableName(name); err != nil {
			return nil, fmt.Errorf("invalid environment variable name %q for key %q: %w", name, key, err)
		}
		if conflictKey, exists := usedNames[name]; exists {
			return nil, fmt.Errorf("duplicate environment variable name %q for keys %q and %q", name, conflictKey, key)
		}
		usedNames[name] = key
		envConfigTemplates = append(envConfigTemplates, EnvConfigTemplate{
			Key:             key,
			Name:            name,
			IsSecret:        meta.isSecret,
			Value:           "",
			SecretReference: "",
		})
	}

	return envConfigTemplates, nil
}

// buildMCPMappingEnvironmentVariables produces the URL and API key env var templates for an MCP mapping.
func (s *agentConfigurationService) buildMCPMappingEnvironmentVariables(mappingName string, overrides []models.EnvironmentVariableConfig) ([]EnvConfigTemplate, error) {
	prefix := strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, mappingName)
	prefix = strings.ToUpper(prefix)
	if len(prefix) > 0 && prefix[0] >= '0' && prefix[0] <= '9' {
		prefix = "_" + prefix
	}

	type keyMeta struct {
		isSecret bool
		autoName string
	}
	knownKeys := map[string]keyMeta{
		"url":    {isSecret: false, autoName: fmt.Sprintf("%s_URL", prefix)},
		"apikey": {isSecret: true, autoName: fmt.Sprintf("%s_API_KEY", prefix)},
	}
	overrideMap := make(map[string]string)
	seen := make(map[string]bool)
	for _, ov := range overrides {
		if _, known := knownKeys[ov.Key]; !known {
			return nil, fmt.Errorf("unknown environment variable key %q for mapping %q: must be one of url, apikey", ov.Key, mappingName)
		}
		if seen[ov.Key] {
			return nil, fmt.Errorf("duplicate environment variable key %q", ov.Key)
		}
		seen[ov.Key] = true
		overrideMap[ov.Key] = ov.Name
	}

	keyOrder := []string{"url", "apikey"}
	envConfigTemplates := make([]EnvConfigTemplate, 0, len(keyOrder))
	usedNames := make(map[string]string)
	for _, key := range keyOrder {
		meta := knownKeys[key]
		name := meta.autoName
		if customName, ok := overrideMap[key]; ok {
			name = customName
		}
		if err := utils.ValidateEnvironmentVariableName(name); err != nil {
			return nil, fmt.Errorf("invalid environment variable name %q for key %q: %w", name, key, err)
		}
		if conflictKey, exists := usedNames[name]; exists {
			return nil, fmt.Errorf("duplicate environment variable name %q for keys %q and %q", name, conflictKey, key)
		}
		usedNames[name] = key
		envConfigTemplates = append(envConfigTemplates, EnvConfigTemplate{
			Key:      key,
			Name:     name,
			IsSecret: meta.isSecret,
		})
	}
	return envConfigTemplates, nil
}

func buildAgentMCPConfigProxy(
	config *models.AgentConfiguration,
	mapping *models.EnvAgentMCPMapping,
	source *models.MCPProxy,
	envName string,
	ouID string,
	handle string,
) *models.MCPProxy {
	// The agent configuration no longer deploys its own artifact: it reuses the single
	// gateway artifact the proxy deployed for this environment. That artifact lives at the
	// proxy's base context, so the injected proxy URL derived from this value is the shared
	// per-environment URL (identical for every agent that references the proxy).
	context := ""
	if source.Configuration.Context != nil {
		context = *source.Configuration.Context
	}
	name := handle
	version := source.Version
	if source.Artifact != nil && source.Artifact.Version != "" {
		version = source.Artifact.Version
	}
	if version == "" {
		version = source.Configuration.Version
	}

	// The source proxy groups one or more endpoints; each endpoint is bound to at most
	// one environment (uq_proxy_env_single). Flatten the endpoint bound to this mapping's
	// environment into the flat, single-environment config that the deployment YAML
	// builder consumes. If no endpoint is bound to this environment the upstream stays
	// empty and deployment fails clearly ("upstream URL is required").
	endpoint, _ := resolveMCPEndpointForEnv(source, mapping.EnvironmentUUID.String())
	var upstream models.UpstreamConfig
	var resilience *models.Resilience
	var policies []models.MCPPolicy
	var capabilities *models.MCPProxyCapabilities
	var security *models.SecurityConfig
	if endpoint != nil {
		cfg := endpoint.Configuration
		if cfg.Upstream != nil {
			upstreamEndpoint := *cfg.Upstream
			upstream.Main = &upstreamEndpoint
		}
		resilience = cfg.Resilience
		policies = cfg.Policies
		capabilities = cfg.Capabilities
		security = cfg.Security
	}

	return &models.MCPProxy{
		UUID:        mapping.ArtifactUUID,
		Description: config.Description,
		Status:      source.Status,
		Configuration: models.MCPProxyConfig{
			Name:         name,
			Version:      version,
			Context:      &context,
			Vhost:        source.Configuration.Vhost,
			SpecVersion:  source.Configuration.SpecVersion,
			Upstream:     upstream,
			Resilience:   resilience,
			Policies:     policies,
			Capabilities: capabilities,
			Security:     security,
		},
		OrganizationName: ouID,
		ID:               handle,
		Name:             name,
		Handle:           handle,
		Version:          version,
	}
}

// resolveMCPEndpointForEnv finds the endpoint bound to the given environment on a
// preloaded source proxy, returning that endpoint and its environment-binding join row.
// The uq_proxy_env_single constraint guarantees at most one endpoint per environment, so
// the first match is the only match. Returns (nil, nil) when the proxy has no endpoint
// bound to that environment (or the proxy / its Endpoints graph was not preloaded).
func resolveMCPEndpointForEnv(proxy *models.MCPProxy, envID string) (*models.MCPProxyEndpoint, *models.MCPProxyEndpointEnvironment) {
	if proxy == nil {
		return nil, nil
	}
	envID = strings.TrimSpace(envID)
	if envID == "" {
		return nil, nil
	}
	for i := range proxy.Endpoints {
		endpoint := &proxy.Endpoints[i]
		for j := range endpoint.Environments {
			ee := &endpoint.Environments[j]
			if ee.EnvironmentUUID.String() == envID {
				return endpoint, ee
			}
		}
	}
	return nil, nil
}

func buildMCPProxyMapping(sourceProxyUUID uuid.UUID, deployedProxy *models.MCPProxy) *models.MCPProxyMapping {
	return &models.MCPProxyMapping{
		UUID:               deployedProxy.UUID,
		SourceMCPProxyUUID: sourceProxyUUID,
		Description:        deployedProxy.Description,
		Status:             models.StatusCreated,
		Configuration:      deployedProxy.Configuration,
	}
}

// ensureMCPEnvVarRows creates the per-environment MCP env var name rows (with empty secret
// references) for a config/environment. The repository uses ON CONFLICT DO NOTHING on
// the config/environment/name/key unique constraint, so concurrent callers race safely.
func (s *agentConfigurationService) ensureMCPEnvVarRows(ctx context.Context, configUUID, envUUID uuid.UUID, envTemplates []EnvConfigTemplate) error {
	variables := make([]models.AgentEnvConfigVariable, 0, len(envTemplates))
	for _, envTemplate := range envTemplates {
		variables = append(variables, models.AgentEnvConfigVariable{
			ConfigUUID:      configUUID,
			EnvironmentUUID: envUUID,
			VariableName:    envTemplate.Name,
			VariableKey:     envTemplate.Key,
			SecretReference: "",
		})
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		return s.envVariableRepo.CreateBatch(ctx, tx, variables)
	})
}

// CleanupEnvironmentMCPArtifacts removes all MCP-proxy data tied to a deleted environment.
// (a) Every agent-scoped MCP mapping deployed into the env is fully torn down (gateway
// artifact deletion broadcast, credential/secret cleanup, and DB rows). (b) The env's block
// is stripped from every org-level MCP proxy blueprint. Best-effort: per-item transactions,
// errors aggregated and returned but never fatal to the caller.
func (s *agentConfigurationService) CleanupEnvironmentMCPArtifacts(ctx context.Context, ouID string, envUUID uuid.UUID, envName string) error {
	if s.envMCPMappingRepo == nil {
		return nil
	}

	var errs []error

	// (a) Agent-scoped mappings deployed into this environment.
	mappings, err := s.envMCPMappingRepo.ListByEnvironment(ctx, envUUID)
	if err != nil {
		errs = append(errs, fmt.Errorf("list MCP mappings for environment %s: %w", envUUID, err))
	}
	for i := range mappings {
		mapping := &mappings[i]
		config, err := s.agentConfigRepo.GetByUUID(ctx, mapping.ConfigUUID, ouID)
		if err != nil {
			errs = append(errs, fmt.Errorf("mapping %s: load config %s: %w", mapping.ArtifactUUID, mapping.ConfigUUID, err))
			continue
		}
		if config == nil {
			errs = append(errs, fmt.Errorf("mapping %s: config %s not found", mapping.ArtifactUUID, mapping.ConfigUUID))
			continue
		}

		isExternalAgent := false
		if agentComp, agentErr := s.ocClient.GetComponent(ctx, ouID, config.ProjectName, config.AgentID); agentErr != nil {
			s.logger.Warn("failed to determine agent type during MCP env cleanup, assuming internal", "config_uuid", config.UUID, "error", agentErr)
		} else {
			isExternalAgent = agentComp.Provisioning.Type == string(utils.ExternalAgent)
		}

		existingVarNames, err := s.loadExistingVarNames(ctx, config.UUID)
		if err != nil {
			errs = append(errs, fmt.Errorf("mapping %s: load env var names: %w", mapping.ArtifactUUID, err))
			continue
		}
		envTemplates, err := s.buildMCPMappingEnvironmentVariables(config.Name, varNamesToOverrides(existingVarNames))
		if err != nil {
			errs = append(errs, fmt.Errorf("mapping %s: build env vars: %w", mapping.ArtifactUUID, err))
			continue
		}

		// isLastEnv: only strip the shared Component-CR env vars when this env was the
		// config's last remaining MCP mapping.
		isLastEnv := true
		if siblings, listErr := s.envMCPMappingRepo.ListByConfig(ctx, config.UUID); listErr != nil {
			s.logger.Warn("failed to list sibling MCP mappings during env cleanup; treating as last env", "config_uuid", config.UUID, "error", listErr)
		} else {
			for j := range siblings {
				if siblings[j].EnvironmentUUID != envUUID {
					isLastEnv = false
					break
				}
			}
		}

		if err := s.removeMCPMappingEnvironment(ctx, config, mapping, envName, ouID, config.ProjectName, config.AgentID, envTemplates, isExternalAgent, isLastEnv); err != nil {
			errs = append(errs, fmt.Errorf("mapping %s: teardown: %w", mapping.ArtifactUUID, err))
		}
	}

	// (b) Org-level MCP proxies — remove the vanished environment's endpoint bindings.
	if s.mcpProxyRepo != nil && s.mcpProxyService != nil {
		const pageSize = 100
		for offset := 0; ; offset += pageSize {
			proxies, listErr := s.mcpProxyRepo.List(ctx, ouID, pageSize, offset)
			if listErr != nil {
				errs = append(errs, fmt.Errorf("list MCP proxies (offset %d): %w", offset, listErr))
				break
			}
			for _, proxy := range proxies {
				if proxy == nil {
					continue
				}
				if err := s.mcpProxyService.RemoveEnvironmentFromEndpoints(ctx, proxy, envUUID, ouID); err != nil {
					errs = append(errs, fmt.Errorf("remove env %s from proxy %s: %w", envUUID, proxy.UUID, err))
				}
			}
			if len(proxies) < pageSize {
				break
			}
		}
	}

	return errors.Join(errs...)
}

// varNamesToOverrides converts a key→name map to a slice of EnvironmentVariableConfig.
// Used when passing existing DB names as overrides to buildEnvironmentVariables.
func varNamesToOverrides(names map[string]string) []models.EnvironmentVariableConfig {
	if len(names) == 0 {
		return nil
	}
	overrides := make([]models.EnvironmentVariableConfig, 0, len(names))
	for key, name := range names {
		overrides = append(overrides, models.EnvironmentVariableConfig{Key: key, Name: name})
	}
	return overrides
}

// mcpEnvTemplatesForConfig rebuilds the config's MCP env var templates from the variable
// names currently persisted for it, so the names the agent already runs with (including user
// overrides) survive instead of being re-derived from the config name.
func (s *agentConfigurationService) mcpEnvTemplatesForConfig(
	ctx context.Context, config *models.AgentConfiguration,
) ([]EnvConfigTemplate, error) {
	vars, err := s.envVariableRepo.ListByConfig(ctx, config.UUID)
	if err != nil {
		return nil, fmt.Errorf("failed to load existing variable names: %w", err)
	}
	return s.mcpEnvTemplatesFromVars(config, vars)
}

// mcpEnvTemplatesFromVars is mcpEnvTemplatesForConfig for callers that already hold the
// config's variable rows.
func (s *agentConfigurationService) mcpEnvTemplatesFromVars(
	config *models.AgentConfiguration, vars []models.AgentEnvConfigVariable,
) ([]EnvConfigTemplate, error) {
	envTemplates, err := s.buildMCPMappingEnvironmentVariables(config.Name, varNamesToOverrides(s.variableNameMap(config.UUID, vars)))
	if err != nil {
		return nil, errors.Join(utils.ErrInvalidInput, err)
	}
	return envTemplates, nil
}

// resolveEnvironmentUUID resolves an environment name to its UUID.
func (s *agentConfigurationService) resolveEnvironmentUUID(
	ctx context.Context, ouID, environmentName string,
) (uuid.UUID, error) {
	env, err := s.ocClient.GetEnvironment(ctx, ouID, environmentName)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to get environment %q: %w", environmentName, err)
	}
	envUUID, err := uuid.Parse(env.UUID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid environment UUID %q: %w", env.UUID, err)
	}
	return envUUID, nil
}

// loadExistingVarNames loads the variable key→name mapping from DB for a config.
// Names are config-level (identical across all environments). The first occurrence per key
// is used; a warning is logged if divergence is detected (indicates a data integrity problem).
func (s *agentConfigurationService) loadExistingVarNames(ctx context.Context, configUUID uuid.UUID) (map[string]string, error) {
	vars, err := s.envVariableRepo.ListByConfig(ctx, configUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to load existing variable names: %w", err)
	}
	return s.variableNameMap(configUUID, vars), nil
}

func (s *agentConfigurationService) variableNameMap(configUUID uuid.UUID, vars []models.AgentEnvConfigVariable) map[string]string {
	result := make(map[string]string)
	for _, v := range vars {
		if existing, already := result[v.VariableKey]; already {
			if existing != v.VariableName {
				s.logger.Warn(
					"environment variable name diverged across environments — using first-occurrence value",
					"config_uuid", configUUID,
					"key", v.VariableKey,
					"first_value", existing,
					"diverged_value", v.VariableName,
				)
			}
		} else {
			result[v.VariableKey] = v.VariableName
		}
	}
	return result
}

func (s *agentConfigurationService) loadSecretRefForConfigEnv(ctx context.Context, configUUID, envUUID uuid.UUID) (string, error) {
	vars, err := s.envVariableRepo.ListByConfigAndEnv(ctx, configUUID, envUUID)
	if err != nil {
		return "", fmt.Errorf("failed to load environment variables for SecretReference lookup: %w", err)
	}
	for _, v := range vars {
		if v.SecretReference != "" {
			return v.SecretReference, nil
		}
	}
	return "", nil
}

func (s *agentConfigurationService) updateMCPMappingSecretReference(ctx context.Context, configUUID, envUUID uuid.UUID, secretRefName string) error {
	result := s.db.WithContext(ctx).Model(&models.AgentEnvConfigVariable{}).
		Where("config_uuid = ? AND environment_uuid = ? AND variable_key = ?", configUUID, envUUID, "apikey").
		Update("secret_reference", secretRefName)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.AgentEnvConfigVariable{}).
		Where("config_uuid = ? AND environment_uuid = ? AND variable_key = ?", configUUID, envUUID, "apikey").
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("apikey environment variable row not found")
	}
	return nil
}

func (s *agentConfigurationService) removeMCPMappingAPIKeyEnvVar(ctx context.Context, config *models.AgentConfiguration, envName string, envTemplates []EnvConfigTemplate, firstEnvName string) {
	var keysToRemove []string
	for _, t := range envTemplates {
		if t.Key == "apikey" && strings.TrimSpace(t.Name) != "" {
			keysToRemove = append(keysToRemove, t.Name)
		}
	}
	if len(keysToRemove) == 0 {
		return
	}
	if err := s.ocClient.RemoveReleaseBindingEnvVars(ctx, config.OUID, config.ProjectName, config.AgentID, envName, keysToRemove); err != nil {
		s.logger.Warn("failed to remove MCP API key env var from ReleaseBinding", "environment", envName, "error", err)
	}
	if firstEnvName != "" && envName == firstEnvName {
		if err := s.ocClient.RemoveComponentEnvironmentVariables(ctx, config.OUID, config.ProjectName, config.AgentID, keysToRemove); err != nil {
			s.logger.Warn("failed to remove MCP API key env var from Component CR", "environment", envName, "error", err)
		}
	}
}

func (s *agentConfigurationService) ensureMCPMappingCredentials(ctx context.Context, config *models.AgentConfiguration, mapping *models.EnvAgentMCPMapping, envName, ouID string) (string, error) {
	keyName := mcpMappingAPIKeyName(config, envName)
	// storageUUID scopes the key to this agent (persistence + listing); apiID is the shared
	// per-environment proxy artifact the gateway validates the key against.
	storageUUID := mapping.ArtifactUUID
	apiID := s.resolveMCPMappingAPIID(ctx, mapping, ouID)
	if apiID == uuid.Nil {
		return "", fmt.Errorf("MCP proxy shared artifact not found for environment %s", envName)
	}
	secretRefName, err := s.loadSecretRefForConfigEnv(ctx, config.UUID, mapping.EnvironmentUUID)
	if err != nil {
		return "", err
	}
	keyExists, err := s.mcpMappingAPIKeyExists(storageUUID, keyName)
	if err != nil {
		return "", fmt.Errorf("failed to inspect MCP API key for environment %s: %w", envName, err)
	}
	if secretRefName != "" && keyExists {
		if err := s.revokeStaleMCPMappingAPIKeys(ctx, ouID, apiID, storageUUID, keyName); err != nil {
			return "", fmt.Errorf("failed to revoke stale MCP API keys for environment %s: %w", envName, err)
		}
		return secretRefName, nil
	}

	proxyAPIKey, err := s.createMCPMappingAPIKey(ctx, ouID, apiID, storageUUID, keyName)
	if err != nil {
		return "", fmt.Errorf("failed to generate MCP API key for environment %s: %w", envName, err)
	}
	createdKeyName := proxyAPIKey.KeyID

	agentAppHandle := agentAppIdentifier(config.ProjectName, config.AgentID, envName)
	if _, _, err = s.aiApplicationService.EnsureAndBind(
		ctx, ouID, config.ProjectName, config.AgentID, envName,
		agentAppHandle,
		fmt.Sprintf("%s Application", config.AgentID),
		proxyAPIKey.KeyID,
	); err != nil {
		if revokeErr := s.revokeMCPMappingAPIKey(ctx, ouID, apiID, storageUUID, createdKeyName); revokeErr != nil {
			s.logger.Warn("failed to revoke MCP API key after AI application failure", "environment", envName, "error", revokeErr)
		}
		return "", fmt.Errorf("failed to ensure AI application for MCP environment %s: %w", envName, err)
	}

	secretLoc := mcpMappingSecretLocation(config, ouID, envName)
	newSecretRefName, err := s.secretClient.CreateSecret(ctx, secretLoc,
		map[string]string{secretmanagersvc.SecretKeyAPIKey: proxyAPIKey.APIKey})
	if err != nil {
		if revokeErr := s.revokeMCPMappingAPIKey(ctx, ouID, apiID, storageUUID, createdKeyName); revokeErr != nil {
			s.logger.Warn("failed to revoke MCP API key after secret persistence failure", "environment", envName, "error", revokeErr)
		}
		return "", fmt.Errorf("failed to store MCP API key in KV for environment %s: %w", envName, err)
	}

	if err := s.updateMCPMappingSecretReference(ctx, config.UUID, mapping.EnvironmentUUID, newSecretRefName); err != nil {
		if delErr := s.secretClient.DeleteSecret(ctx, secretLoc, newSecretRefName); delErr != nil {
			s.logger.Warn("failed to delete MCP API key secret after env var update failure", "environment", envName, "error", delErr)
		}
		if revokeErr := s.revokeMCPMappingAPIKey(ctx, ouID, apiID, storageUUID, createdKeyName); revokeErr != nil {
			s.logger.Warn("failed to revoke MCP API key after env var update failure", "environment", envName, "error", revokeErr)
		}
		return "", fmt.Errorf("failed to update MCP API key env reference for %s: %w", envName, err)
	}
	if secretRefName != "" && secretRefName != newSecretRefName {
		s.logger.Info("MCP mapping SecretReference replaced", "environment", envName, "old_secret_ref", secretRefName, "new_secret_ref", newSecretRefName)
	}
	if err := s.revokeStaleMCPMappingAPIKeys(ctx, ouID, apiID, storageUUID, keyName); err != nil {
		return "", fmt.Errorf("failed to revoke stale MCP API keys for environment %s: %w", envName, err)
	}
	return newSecretRefName, nil
}

func (s *agentConfigurationService) reconcileMCPMappingCredentials(ctx context.Context, config *models.AgentConfiguration, mapping *models.EnvAgentMCPMapping, sourceProxy *models.MCPProxy, envName, ouID string, envTemplates []EnvConfigTemplate, isExternalAgent bool, firstEnvName string) error {
	if mcpProxyAPIKeySecurityEnabled(sourceProxy, mapping.EnvironmentUUID.String()) {
		if _, err := s.ensureMCPMappingCredentials(ctx, config, mapping, envName, ouID); err != nil {
			return err
		}
		return nil
	}

	s.cleanupMCPMappingCredentials(ctx, config, mapping, envName, ouID)
	if err := s.updateMCPMappingSecretReference(ctx, config.UUID, mapping.EnvironmentUUID, ""); err != nil {
		return fmt.Errorf("failed to clear MCP API key env reference for %s: %w", envName, err)
	}
	if !isExternalAgent {
		s.removeMCPMappingAPIKeyEnvVar(ctx, config, envName, envTemplates, firstEnvName)
	}
	return nil
}

// cleanupNewMCPMapping tears a partially created binding back down. deleteEnvVars must be
// false when the environment's env var rows predate this attempt — a backfill binds an
// environment that was already promoted, and dropping its variable rows would strip the
// agent's MCP variable names entirely, leaving it worse off than the failed binding.
func (s *agentConfigurationService) cleanupNewMCPMapping(ctx context.Context, config *models.AgentConfiguration, mapping *models.EnvAgentMCPMapping, envName, ouID string, deleteEnvVars bool) {
	if s.mcpProxyService != nil {
		s.mcpProxyService.BroadcastMCPArtifactDeletion(ctx, mapping.ArtifactUUID, ouID)
	}
	s.cleanupMCPMappingCredentials(ctx, config, mapping, envName, ouID)
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if deleteEnvVars {
			if err := s.envVariableRepo.DeleteByConfigAndEnv(ctx, tx, config.UUID, mapping.EnvironmentUUID); err != nil {
				return err
			}
		}
		if mapping.ID != 0 {
			if err := s.envMCPMappingRepo.Delete(ctx, tx, mapping.ID); err != nil {
				return err
			}
		}
		if err := tx.Where("artifact_uuid = ? AND ou_id = ?", mapping.ArtifactUUID, ouID).
			Delete(&models.DeploymentStatusRecord{}).Error; err != nil {
			return err
		}
		if err := tx.Where("artifact_uuid = ? AND ou_id = ?", mapping.ArtifactUUID, ouID).
			Delete(&models.Deployment{}).Error; err != nil {
			return err
		}
		return tx.Where("uuid = ?", mapping.ArtifactUUID).Delete(&models.Artifact{}).Error
	}); err != nil {
		s.logger.Warn("failed to clean up new MCP mapping after partial failure", "environment", envName, "artifact_uuid", mapping.ArtifactUUID, "error", err)
	}
}

func uniqueVariableNames(vars []models.AgentEnvConfigVariable) []string {
	seen := make(map[string]struct{}, len(vars))
	names := make([]string, 0, len(vars))
	for _, v := range vars {
		if strings.TrimSpace(v.VariableName) == "" {
			continue
		}
		if _, ok := seen[v.VariableName]; ok {
			continue
		}
		seen[v.VariableName] = struct{}{}
		names = append(names, v.VariableName)
	}
	return names
}

func variableNamesForEnv(vars []models.AgentEnvConfigVariable, envUUID uuid.UUID) []string {
	filtered := make([]models.AgentEnvConfigVariable, 0, len(vars))
	for _, v := range vars {
		if v.EnvironmentUUID == envUUID {
			filtered = append(filtered, v)
		}
	}
	return uniqueVariableNames(filtered)
}

// dedupeEnvVariablesByKey collapses per-environment DB rows into config-level entries.
// Rows are stored per-environment but the variable name is config-level — agent source code
// reads url/apikey by the same env var name regardless of which provider is bound to a given
// environment. First occurrence per key wins.
func (s *agentConfigurationService) dedupeEnvVariablesByKey(configUUID uuid.UUID, vars []models.AgentEnvConfigVariable) []models.EnvironmentVariableConfig {
	seen := make(map[string]string)
	result := make([]models.EnvironmentVariableConfig, 0, len(vars))
	for _, v := range vars {
		if existing, already := seen[v.VariableKey]; already {
			if existing != v.VariableName {
				s.logger.Warn(
					"environment variable name differs across environments — using first-occurrence value",
					"config_uuid", configUUID,
					"key", v.VariableKey,
					"first_value", existing,
					"other_value", v.VariableName,
				)
			}
			continue
		}
		seen[v.VariableKey] = v.VariableName
		result = append(result, models.EnvironmentVariableConfig{Name: v.VariableName, Key: v.VariableKey})
	}
	return result
}

// undeployAndDeleteDeployment removes a deployment created during a failed
// operation. DeleteLLMProxyDeployment rejects deployments still in the
// Deployed state (ErrDeploymentIsDeployed), so an active deployment must be
// undeployed first — mirroring the post-success cleanup path in Update().
// gatewayID may be empty for older rollback resources that predate gateway
// tracking; the undeploy step is skipped in that case and only delete is
// attempted (best-effort, matching the rest of this rollback path).
func (s *agentConfigurationService) undeployAndDeleteDeployment(ctx context.Context, proxyHandle string, deploymentID uuid.UUID, gatewayID, ouID string) {
	if gatewayID != "" {
		if _, err := s.llmProxyDeploymentService.UndeployLLMProxyDeployment(ctx, proxyHandle, deploymentID.String(), gatewayID, ouID); err != nil &&
			!errors.Is(err, utils.ErrDeploymentNotActive) {
			s.logger.Error(
				"Failed to undeploy deployment during rollback",
				"proxy_handle", proxyHandle,
				"deployment_id", deploymentID,
				"error", err,
			)
		}
	}
	if err := s.llmProxyDeploymentService.DeleteLLMProxyDeployment(proxyHandle, deploymentID.String(), ouID); err != nil {
		s.logger.Error(
			"Failed to delete deployment during rollback",
			"proxy_handle", proxyHandle,
			"deployment_id", deploymentID,
			"error", err,
		)
	}
}

// rollbackProxies cleans up created proxies, deployments, and API keys on failure
func (s *agentConfigurationService) rollbackProxies(ctx context.Context, resources []rollbackResource, ouID string) {
	s.logger.Warn("Rolling back created proxies and API keys", "count", len(resources))

	// Track unique proxies to delete
	proxyHandles := make(map[string]bool)

	// Clean up each resource
	for _, res := range resources {
		// Delete provider API key from KV and SecretReference
		if res.providerSecretLoc != nil {
			if err := s.secretClient.DeleteSecret(ctx, *res.providerSecretLoc, res.secretRefName); err != nil {
				kvPath, _ := res.providerSecretLoc.KVPath()
				s.logger.Error("Failed to delete provider API key during rollback",
					"kv_path", kvPath, "error", err)
			}
		}
		// Delete proxy API key from KV and SecretReference
		if res.proxySecretLoc != nil {
			if err := s.secretClient.DeleteSecret(ctx, *res.proxySecretLoc, res.secretRefName); err != nil {
				kvPath, _ := res.proxySecretLoc.KVPath()
				s.logger.Error("Failed to delete proxy API key during rollback",
					"kv_path", kvPath, "error", err)
			}
		}

		// Revoke the proxy API key if one was created
		if res.proxyAPIKeyID != "" {
			if err := s.llmProxyAPIKeyService.RevokeAPIKey(ctx, ouID, res.proxyHandle, res.proxyAPIKeyID); err != nil {
				s.logger.Error(
					"Failed to revoke proxy API key during rollback",
					"proxy_handle", res.proxyHandle,
					"api_key_id", res.proxyAPIKeyID,
					"error", err,
				)
			} else {
				s.logger.Info(
					"Revoked proxy API key during rollback",
					"proxy_handle", res.proxyHandle,
					"api_key_id", res.proxyAPIKeyID,
				)
			}
		}

		// Delete the AI application only if this rollback resource was the one that
		// created it (i.e. it didn't exist before this operation).
		if res.createdNewApp {
			if err := s.aiApplicationService.Delete(ctx, ouID, res.appProjectName, res.appAgentID, res.appEnvName); err != nil {
				s.logger.Warn("Failed to delete AI application during rollback (best-effort)",
					"agent_id", res.appAgentID, "env_name", res.appEnvName, "error", err)
			}
		}

		// Undeploy deployment — only if a deployment was actually created.
		if res.proxyHandle != "" && res.deploymentID != uuid.Nil {
			s.undeployAndDeleteDeployment(ctx, res.proxyHandle, res.deploymentID, res.gatewayID, ouID)
		}

		// Revoke provider API key if one was created (CRIT-3).
		if res.providerAPIKeyID != "" && res.providerUUID != "" {
			if err := s.llmProviderAPIKeyService.RevokeAPIKey(ctx, ouID, res.providerUUID, res.providerAPIKeyID); err != nil {
				s.logger.Error(
					"Failed to revoke provider API key during rollback",
					"provider_api_key_id", res.providerAPIKeyID,
					"provider_uuid", res.providerUUID,
					"error", err,
				)
			} else {
				s.logger.Info(
					"Revoked provider API key during rollback",
					"provider_api_key_id", res.providerAPIKeyID,
				)
			}
		}

		// Scenario B (in-place proxy update) rollback: the proxy pre-existed this
		// operation, so restore its prior configuration instead of deleting it —
		// the generic full-proxy teardown below assumes the proxy was newly created.
		if res.priorProxyConfig != nil {
			if _, err := s.llmProxyService.Update(res.proxyHandle, ouID, res.priorProxyConfig); err != nil {
				s.logger.Error("Failed to restore prior proxy configuration during rollback",
					"proxy_handle", res.proxyHandle, "error", err)
			} else {
				s.logger.Info("Restored prior proxy configuration during rollback",
					"proxy_handle", res.proxyHandle)
			}
			// The old deployment was silently superseded (ARCHIVED) the instant
			// the new deployment was created above — it was never explicitly
			// undeployed, so it must be explicitly reactivated here.
			if res.restoreDeploymentID != uuid.Nil {
				if _, err := s.llmProxyDeploymentService.RestoreLLMProxyDeployment(
					ctx,
					res.proxyHandle, res.restoreDeploymentID.String(), res.gatewayID, ouID,
				); err != nil {
					s.logger.Error("Failed to restore prior deployment during rollback",
						"proxy_handle", res.proxyHandle, "deployment_id", res.restoreDeploymentID, "error", err)
				} else {
					s.logger.Info("Restored prior deployment during rollback",
						"proxy_handle", res.proxyHandle, "deployment_id", res.restoreDeploymentID)
				}
			}
			continue
		}

		if res.proxyHandle != "" {
			proxyHandles[res.proxyHandle] = true
		}
	}

	// Revert DB mappings for Scenario A BEFORE deleting the replacement proxy
	// below (HIGH-4): the mapping's llm_proxy_uuid FK still points at the new
	// proxy at this point (processEnvProviderChange already committed that
	// repoint), and fk_env_mapping_proxy is ON DELETE CASCADE — deleting the
	// new proxy first would cascade-delete the mapping row outright, leaving
	// the environment with no LLM mapping at all instead of one merely
	// pointing at the old proxy. Reverting first repoints the FK away from the
	// new proxy so its deletion below only removes the orphaned proxy row.
	for _, res := range resources {
		if res.mappingID != 0 && res.oldProxyUUID != uuid.Nil {
			revertErr := s.db.Transaction(func(tx *gorm.DB) error {
				result := tx.Model(&models.EnvAgentModelMapping{}).
					Where("id = ?", res.mappingID).
					Update("llm_proxy_uuid", res.oldProxyUUID)
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected == 0 {
					return fmt.Errorf("mapping %d not found", res.mappingID)
				}
				return nil
			})
			if revertErr != nil {
				s.logger.Error(
					"Failed to revert DB mapping to old proxy UUID during rollback — mapping may be dangling",
					"mapping_id", res.mappingID,
					"old_proxy_uuid", res.oldProxyUUID,
					"error", revertErr,
				)
			}
		}
	}

	// Delete all unique proxies
	for handle := range proxyHandles {
		if err := s.llmProxyService.Delete(handle, ouID); err != nil {
			s.logger.Error(
				"Failed to delete proxy during rollback",
				"handle", handle,
				"error", err,
			)
		}
	}
}

// buildConfigResponse builds the full configuration response
func (s *agentConfigurationService) buildConfigResponse(ctx context.Context, config *models.AgentConfiguration, includeProxyURL bool) (*models.AgentModelConfigResponse, error) {
	// Get environment names from OpenChoreo
	envs, err := s.infraResourceManager.ListOrgEnvironments(ctx, config.OUID)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	envMap := make(map[string]string)
	for _, env := range envs {
		envMap[env.UUID] = env.Name
	}

	s.logger.Info("Building config response", "config_uuid", config.UUID, "env_count", len(envs))

	// Build environment model config map
	envModelConfig := make(map[string]models.EnvModelConfigResponse)
	for _, mapping := range config.EnvMappings {
		envName := envMap[mapping.EnvironmentUUID.String()]
		// Fall back to UUID if environment was deleted
		if envName == "" {
			envName = mapping.EnvironmentUUID.String()
		}

		var proxyInfo *models.LLMProxyInfo = nil
		if mapping.LLMProxy != nil {
			providerUUID := mapping.LLMProxy.ProviderUUID.String()
			proxyInfo = &models.LLMProxyInfo{
				ProxyUUID:    utils.StrAsStrPointer(mapping.LLMProxy.UUID.String()),
				ProxyName:    utils.StrAsStrPointer(mapping.LLMProxy.Handle),
				ProviderUUID: utils.StrAsStrPointer(providerUUID),
				Policies:     mapping.PolicyConfiguration,
				Resilience:   mapping.LLMProxy.Configuration.Resilience,
			}
			if provider, err := s.llmProviderRepo.GetByUUID(providerUUID, config.OUID); err == nil {
				if provider.Artifact != nil {
					proxyInfo.ProviderName = utils.StrAsStrPointer(provider.Artifact.Handle)
				}
				proxyInfo.ProviderPolicies = provider.Configuration.Policies
			}

			// Add proxy URL for external agents (subsequent GET calls)
			if includeProxyURL {
				gateway, err := s.resolveGatewayForProxy(ctx, mapping.LLMProxy.Handle, config.OUID, mapping.EnvironmentUUID)
				if err == nil && mapping.LLMProxy.Configuration.Context != nil {
					url := fmt.Sprintf("%s%s", gateway.Vhost, *mapping.LLMProxy.Configuration.Context)
					proxyInfo.URL = &url
				} else if err == nil {
					// If no context, just use gateway vhost
					url := gateway.Vhost
					proxyInfo.URL = &url
				}
			}
		}

		envModelConfig[envName] = models.EnvModelConfigResponse{
			EnvironmentName: envName,
			LLMProxy:        proxyInfo,
		}
	}
	for _, mapping := range config.EnvMCPMappings {
		envName := envMap[mapping.EnvironmentUUID.String()]
		if envName == "" {
			envName = mapping.EnvironmentUUID.String()
		}
		var proxyInfo *models.LLMProxyInfo
		if mapping.MCPProxy != nil {
			proxyName := ""
			if mapping.MCPProxy.Artifact != nil {
				proxyName = mapping.MCPProxy.Artifact.Handle
			}
			if proxyName == "" {
				proxyName = mapping.MCPProxy.Configuration.Name
			}
			proxyInfo = &models.LLMProxyInfo{
				ProxyUUID:      utils.StrAsStrPointer(mapping.ArtifactUUID.String()),
				ProviderName:   utils.StrAsStrPointer(proxyName),
				AuthHeaderName: utils.StrAsStrPointer(mcpProxyAPIKeyHeaderName(mapping.MCPProxy, mapping.EnvironmentUUID.String())),
			}
			sharedArtifactUUID := s.resolveMCPMappingAPIID(ctx, &mapping, config.OUID)
			if gateway, err := s.resolveGatewayForMCPArtifact(ctx, sharedArtifactUUID, config.OUID, mapping.EnvironmentUUID); err == nil && sharedArtifactUUID != uuid.Nil {
				deployedProxy := buildAgentMCPConfigProxy(config, &mapping, mapping.MCPProxy, envName, config.OUID,
					mcpMappingProxyName(config.ProjectName, config.AgentID, config.Name, envName))
				// User-facing invoke URL in the config response: externally-reachable vhost.
				url := buildMCPProxyURL(gateway, deployedProxy.Configuration)
				proxyInfo.URL = &url
			}
		}
		envModelConfig[envName] = models.EnvModelConfigResponse{
			EnvironmentName: envName,
			LLMProxy:        proxyInfo,
		}
	}

	// Variable rows are stored per-environment but names are config-level — collapse to one entry per key.
	envVars := s.dedupeEnvVariablesByKey(config.UUID, config.EnvVariables)

	return &models.AgentModelConfigResponse{
		UUID:                 config.UUID.String(),
		Name:                 config.Name,
		Description:          config.Description,
		AgentID:              config.AgentID,
		Type:                 models.AgentConfigTypeFromID(config.TypeID),
		ProjectName:          config.ProjectName,
		EnvModelConfig:       envModelConfig,
		EnvironmentVariables: envVars,
		CreatedAt:            config.CreatedAt,
		UpdatedAt:            config.UpdatedAt,
	}, nil
}

// envCredentialKeys returns the keys (environment UUIDs) of the credential map, for safe logging.
func envCredentialKeys(m map[string]envCredentialData) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// buildExternalAgentConfigResponse builds response with one-time credentials for external agents
func (s *agentConfigurationService) buildExternalAgentConfigResponse(
	ctx context.Context,
	config *models.AgentConfiguration,
	envCredentials map[string]envCredentialData,
) (*models.AgentModelConfigResponse, error) {
	// Reload configuration with relationships (EnvMappings, LLMProxy, etc.)
	reloadedConfig, err := s.agentConfigRepo.GetByUUID(ctx, config.UUID, config.OUID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload configuration: %w", err)
	}

	s.logger.Info(
		"Building external agent config response",
		"config_uuid", config.UUID,
		"env_mapping_count", len(reloadedConfig.EnvMappings),
		"env_mcp_mapping_count", len(reloadedConfig.EnvMCPMappings),
		"env_credential_count", len(envCredentials),
	)

	// Get environment names
	envs, err := s.infraResourceManager.ListOrgEnvironments(ctx, config.OUID)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	envMap := make(map[string]string)
	for _, env := range envs {
		envMap[env.UUID] = env.Name
	}

	// Build environment model config map WITH credentials
	envModelConfig := make(map[string]models.EnvModelConfigResponse)
	for _, mapping := range reloadedConfig.EnvMappings {
		envUUID := mapping.EnvironmentUUID.String()
		envName := envMap[envUUID]
		if envName == "" {
			envName = envUUID
		}

		var proxyInfo *models.LLMProxyInfo
		if mapping.LLMProxy != nil {
			providerUUID := mapping.LLMProxy.ProviderUUID.String()
			proxyInfo = &models.LLMProxyInfo{
				ProxyUUID:    utils.StrAsStrPointer(mapping.LLMProxy.UUID.String()),
				ProxyName:    utils.StrAsStrPointer(mapping.LLMProxy.Handle),
				ProviderUUID: utils.StrAsStrPointer(providerUUID),
				Policies:     mapping.PolicyConfiguration,
				Resilience:   mapping.LLMProxy.Configuration.Resilience,
			}
			if provider, err := s.llmProviderRepo.GetByUUID(providerUUID, config.OUID); err == nil {
				if provider.Artifact != nil {
					proxyInfo.ProviderName = utils.StrAsStrPointer(provider.Artifact.Handle)
				}
				proxyInfo.ProviderPolicies = provider.Configuration.Policies
			}

			// Add credentials for external agents
			if creds, ok := envCredentials[envUUID]; ok {
				proxyInfo.URL = &creds.proxyURL
				proxyInfo.APIKey = &creds.apiKey
				s.logger.Info(
					"Added credentials for external agent",
					"env_uuid", envUUID,
					"has_proxy_url", creds.proxyURL != "",
					"has_api_key", creds.apiKey != "",
				)
			} else {
				s.logger.Warn(
					"No credentials found for environment",
					"env_uuid", envUUID,
					"available_env_uui_ds", envCredentialKeys(envCredentials),
				)
			}
		}

		envModelConfig[envName] = models.EnvModelConfigResponse{
			EnvironmentName: envName,
			LLMProxy:        proxyInfo,
		}
	}
	for _, mapping := range reloadedConfig.EnvMCPMappings {
		envUUID := mapping.EnvironmentUUID.String()
		envName := envMap[envUUID]
		if envName == "" {
			envName = envUUID
		}

		var proxyInfo *models.LLMProxyInfo
		if mapping.MCPProxy != nil {
			proxyName := ""
			if mapping.MCPProxy.Artifact != nil {
				proxyName = mapping.MCPProxy.Artifact.Handle
			}
			if proxyName == "" {
				proxyName = mapping.MCPProxy.Configuration.Name
			}
			proxyInfo = &models.LLMProxyInfo{
				ProxyUUID:      utils.StrAsStrPointer(mapping.ArtifactUUID.String()),
				ProviderName:   utils.StrAsStrPointer(proxyName),
				AuthHeaderName: utils.StrAsStrPointer(mcpProxyAPIKeyHeaderName(mapping.MCPProxy, mapping.EnvironmentUUID.String())),
			}
			sharedArtifactUUID := s.resolveMCPMappingAPIID(ctx, &mapping, config.OUID)
			var gateway *models.Gateway
			var gwErr error
			if sharedArtifactUUID != uuid.Nil {
				gateway, gwErr = s.resolveGatewayForMCPArtifact(ctx, sharedArtifactUUID, config.OUID, mapping.EnvironmentUUID)
			} else {
				gwErr = errNoGatewayForEnvironment
			}
			if creds, ok := envCredentials[envUUID]; ok {
				proxyInfo.URL = &creds.proxyURL
				if creds.apiKey != "" {
					proxyInfo.APIKey = &creds.apiKey
				}
				s.logger.Info(
					"Added MCP credentials for external agent",
					"env_uuid", envUUID,
					"has_proxy_url", creds.proxyURL != "",
					"has_api_key", creds.apiKey != "",
				)
			} else if gwErr == nil {
				deployedProxy := buildAgentMCPConfigProxy(reloadedConfig, &mapping, mapping.MCPProxy, envName, config.OUID,
					mcpMappingProxyName(config.ProjectName, config.AgentID, config.Name, envName))
				// External agent's invoke URL: externally-reachable vhost.
				url := buildMCPProxyURL(gateway, deployedProxy.Configuration)
				proxyInfo.URL = &url
			} else {
				s.logger.Warn(
					"No MCP credentials found for environment",
					"env_uuid", envUUID,
					"available_env_uui_ds", envCredentialKeys(envCredentials),
				)
			}
		}

		envModelConfig[envName] = models.EnvModelConfigResponse{
			EnvironmentName: envName,
			LLMProxy:        proxyInfo,
		}
	}

	// Variable rows are stored per-environment but names are config-level — collapse to one entry per key.
	envVars := s.dedupeEnvVariablesByKey(reloadedConfig.UUID, reloadedConfig.EnvVariables)

	return &models.AgentModelConfigResponse{
		UUID:                 reloadedConfig.UUID.String(),
		Name:                 reloadedConfig.Name,
		Description:          reloadedConfig.Description,
		AgentID:              reloadedConfig.AgentID,
		Type:                 models.AgentConfigTypeFromID(reloadedConfig.TypeID),
		ProjectName:          reloadedConfig.ProjectName,
		EnvModelConfig:       envModelConfig,
		EnvironmentVariables: envVars,
		CreatedAt:            reloadedConfig.CreatedAt,
		UpdatedAt:            reloadedConfig.UpdatedAt,
	}, nil
}

func (s *agentConfigurationService) processRollBack(ctx context.Context, rollbackResources []rollbackResource, ouID string, configUUID uuid.UUID) {
	s.logger.Error("Rolling back created proxies and API keys", "count", len(rollbackResources))
	s.rollbackProxies(ctx, rollbackResources, ouID)
	s.compensatingDeleteConfig(ctx, configUUID, ouID)
	s.logger.Error("Rolled back created proxies and API keys", "count", len(rollbackResources))
}

func (s *agentConfigurationService) cleanupMCPConfig(ctx context.Context, configUUID uuid.UUID, ouID string) {
	if s.envMCPMappingRepo != nil && s.mcpProxyService != nil {
		if mappings, err := s.envMCPMappingRepo.ListByConfig(ctx, configUUID); err == nil {
			for _, mapping := range mappings {
				s.mcpProxyService.BroadcastMCPArtifactDeletion(ctx, mapping.ArtifactUUID, ouID)
			}
		}
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var mappings []models.EnvAgentMCPMapping
		if s.envMCPMappingRepo != nil {
			var err error
			mappings, err = s.envMCPMappingRepo.ListByConfig(ctx, configUUID)
			if err != nil {
				return err
			}
		}
		if err := s.agentConfigRepo.Delete(ctx, tx, configUUID, ouID); err != nil {
			return err
		}
		for _, mapping := range mappings {
			if err := tx.Where("artifact_uuid = ? AND ou_id = ?", mapping.ArtifactUUID, ouID).
				Delete(&models.DeploymentStatusRecord{}).Error; err != nil {
				return err
			}
			if err := tx.Where("artifact_uuid = ? AND ou_id = ?", mapping.ArtifactUUID, ouID).
				Delete(&models.Deployment{}).Error; err != nil {
				return err
			}
			if err := tx.Where("uuid = ?", mapping.ArtifactUUID).Delete(&models.Artifact{}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		s.logger.Warn("failed to clean up MCP configuration", "config_uuid", configUUID, "error", err)
	}
}

func (s *agentConfigurationService) ListAgentLLMConfigSecretReferences(ctx context.Context, agentID, ouID, environmentName string) (map[string]struct{}, error) {
	envUUID, err := s.resolveEnvironmentUUID(ctx, ouID, environmentName)
	if err != nil {
		return nil, err
	}
	refs, err := s.envVariableRepo.ListSecretReferencesByAgentAndEnv(ctx, agentID, ouID, envUUID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		result[ref] = struct{}{}
	}
	return result, nil
}

func (s *agentConfigurationService) ListSystemManagedEnvVarKeys(
	ctx context.Context, agentID, ouID, projectName, environmentName string,
) (map[string]bool, error) {
	envUUID, err := s.resolveEnvironmentUUID(ctx, ouID, environmentName)
	if err != nil {
		return nil, err
	}

	configs, err := s.agentConfigRepo.ListByAgent(ctx, ouID, projectName, agentID, agentConfigListAll, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent configurations: %w", err)
	}

	keys := make(map[string]bool)
	for _, config := range configs {
		vars, err := s.envVariableRepo.ListByConfigAndEnv(ctx, config.UUID, envUUID)
		if err != nil {
			return nil, fmt.Errorf("failed to list env config variables for config %s: %w", config.UUID, err)
		}
		for _, v := range vars {
			keys[v.VariableName] = true
		}
	}
	return keys, nil
}

// BuildSystemManagedEnvVarsFromConfig constructs system-managed env vars for a given
// agent and environment from every DB-backed agent config. Used during promotion when
// the target environment's ReleaseBinding doesn't have these vars yet, and when building
// a kind-sourced agent's Workload CR at creation. Returns no vars for an agent with no
// configs, so callers need not pre-check which config types the agent has.
func (s *agentConfigurationService) BuildSystemManagedEnvVarsFromConfig(
	ctx context.Context, agentID, ouID, projectName, environmentName string,
) ([]client.EnvVar, error) {
	configs, err := s.agentConfigRepo.ListByAgent(ctx, ouID, projectName, agentID, agentConfigListAll, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent configurations: %w", err)
	}
	// Resolve the environment only once we know there is a config to build vars for.
	// This keeps the no-config case free of a remote environment lookup, so callers can
	// invoke this unconditionally instead of pre-checking which config types exist.
	if len(configs) == 0 {
		return nil, nil
	}

	envUUID, err := s.resolveEnvironmentUUID(ctx, ouID, environmentName)
	if err != nil {
		return nil, err
	}

	var result []client.EnvVar
	for i := range configs {
		config := &configs[i]
		vars, err := s.envVariableRepo.ListByConfigAndEnv(ctx, config.UUID, envUUID)
		if err != nil {
			return nil, fmt.Errorf("failed to list env config variables for config %s: %w", config.UUID, err)
		}
		if len(vars) == 0 {
			continue
		}

		urlValue := ""
		switch config.TypeID {
		case models.AgentConfigTypeIDLLM:
			urlValue, err = s.systemManagedLLMURL(ctx, config, ouID, environmentName, envUUID)
		case models.AgentConfigTypeIDMCP:
			urlValue, err = s.systemManagedMCPURL(ctx, config, ouID, environmentName, envUUID)
		default:
			err = nil
		}
		if err != nil {
			return nil, err
		}

		for _, v := range vars {
			envVar := client.EnvVar{Key: v.VariableName}
			switch {
			case v.SecretReference != "":
				envVar.ValueFrom = &client.EnvVarValueFrom{
					SecretKeyRef: &client.SecretKeyRef{
						Name: v.SecretReference,
						Key:  secretmanagersvc.SecretKeyAPIKey,
					},
				}
			case v.VariableKey == "url":
				envVar.Value = urlValue
			default:
				envVar.Value = ""
			}
			result = append(result, envVar)
		}
	}

	return result, nil
}

func (s *agentConfigurationService) systemManagedLLMURL(
	ctx context.Context, config *models.AgentConfiguration, ouID, environmentName string, envUUID uuid.UUID,
) (string, error) {
	mapping, err := s.envMappingRepo.GetByConfigAndEnv(ctx, config.UUID, envUUID)
	if err != nil {
		return "", fmt.Errorf("failed to get LLM env mapping for %s: %w", environmentName, err)
	}
	if mapping.LLMProxy == nil {
		return "", fmt.Errorf("LLM proxy not found for mapping in environment %s", environmentName)
	}

	proxyHandle := strings.TrimSpace(mapping.LLMProxy.Handle)
	if proxyHandle == "" {
		proxyHandle = strings.TrimSpace(mapping.LLMProxy.Configuration.Name)
	}
	if proxyHandle == "" {
		return "", fmt.Errorf("LLM proxy handle not found for mapping in environment %s", environmentName)
	}

	gateway, err := s.resolveGatewayForProxy(ctx, proxyHandle, ouID, envUUID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve gateway for LLM proxy in %s: %w", environmentName, err)
	}
	return buildProxyURL(gateway, mapping.LLMProxy.Configuration.Context), nil
}

func (s *agentConfigurationService) systemManagedMCPURL(
	ctx context.Context, config *models.AgentConfiguration, ouID, environmentName string, envUUID uuid.UUID,
) (string, error) {
	mappings, err := s.envMCPMappingRepo.ListByConfig(ctx, config.UUID)
	if err != nil {
		return "", fmt.Errorf("failed to list MCP env mappings for config %s: %w", config.UUID, err)
	}
	for i := range mappings {
		mapping := &mappings[i]
		if mapping.EnvironmentUUID != envUUID {
			continue
		}
		if mapping.MCPProxy == nil {
			return "", fmt.Errorf("MCP proxy not found for mapping in environment %s", environmentName)
		}
		sharedArtifactUUID := s.resolveMCPMappingAPIID(ctx, mapping, ouID)
		if sharedArtifactUUID == uuid.Nil {
			return "", fmt.Errorf("MCP proxy shared artifact not found for mapping in environment %s", environmentName)
		}
		gateway, err := s.resolveGatewayForMCPArtifact(ctx, sharedArtifactUUID, ouID, envUUID)
		if err != nil {
			return "", fmt.Errorf("failed to resolve gateway for MCP proxy in %s: %w", environmentName, err)
		}
		handle := mcpMappingProxyName(config.ProjectName, config.AgentID, config.Name, environmentName)
		deployedProxy := buildAgentMCPConfigProxy(config, mapping, mapping.MCPProxy, environmentName, ouID, handle)
		return buildMCPProxyURL(gateway, deployedProxy.Configuration), nil
	}
	return "", nil
}
