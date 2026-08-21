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

package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// GatewayInternalController defines interface for gateway internal API HTTP handlers
type GatewayInternalController interface {
	GetLLMProvider(w http.ResponseWriter, r *http.Request)
	GetLLMProxy(w http.ResponseWriter, r *http.Request)
	GetMCPProxy(w http.ResponseWriter, r *http.Request)
	GetLLMProviderAPIKeys(w http.ResponseWriter, r *http.Request)
	GetLLMProxyAPIKeys(w http.ResponseWriter, r *http.Request)
	GetAPIKeys(w http.ResponseWriter, r *http.Request)
	GetSubscriptionPlans(w http.ResponseWriter, r *http.Request)
	GetDeployments(w http.ResponseWriter, r *http.Request)
	GetApplications(w http.ResponseWriter, r *http.Request)
	PushGatewayManifest(w http.ResponseWriter, r *http.Request)
}

type gatewayInternalController struct {
	gatewayService         *services.PlatformGatewayService
	gatewayInternalService *services.GatewayInternalAPIService
	apiKeyRepo             repositories.APIKeyRepository
	aiApplicationRepo      repositories.AIApplicationRepository
}

// NewGatewayInternalController creates a new gateway internal controller
func NewGatewayInternalController(
	gatewayService *services.PlatformGatewayService,
	gatewayInternalService *services.GatewayInternalAPIService,
	apiKeyRepo repositories.APIKeyRepository,
	aiApplicationRepo repositories.AIApplicationRepository,
) GatewayInternalController {
	return &gatewayInternalController{
		gatewayService:         gatewayService,
		gatewayInternalService: gatewayInternalService,
		apiKeyRepo:             apiKeyRepo,
		aiApplicationRepo:      aiApplicationRepo,
	}
}

// GetLLMProvider handles GET /api/internal/v1/llm-providers/:providerId
func (c *gatewayInternalController) GetLLMProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	identity, ok := c.authenticateGateway(w, r, "internal-read")
	if !ok {
		return
	}

	ouID := identity.OUID
	gatewayID := identity.ID
	providerID := r.PathValue("providerId")
	if providerID == "" {
		http.Error(w, "Provider ID is required", http.StatusBadRequest)
		return
	}

	provider, err := c.gatewayInternalService.GetActiveLLMProviderDeploymentByGateway(ctx, providerID, ouID, gatewayID)
	if err != nil {
		if errors.Is(err, utils.ErrDeploymentNotActive) {
			http.Error(w, "No active deployment found for this LLM provider on this gateway", http.StatusNotFound)
			return
		}
		if errors.Is(err, utils.ErrLLMProviderNotFound) {
			http.Error(w, "LLM provider not found", http.StatusNotFound)
			return
		}
		log.Error("Failed to get LLM provider", "error", err)
		http.Error(w, "Failed to get LLM provider", http.StatusInternalServerError)
		return
	}

	// Create ZIP file from LLM provider YAML file
	zipData, err := utils.CreateLLMProviderYamlZip(provider)
	if err != nil {
		log.Error("Failed to create ZIP file for LLM provider", "provider_id", providerID, "error", err)
		http.Error(w, "Failed to create LLM provider package", http.StatusInternalServerError)
		return
	}

	// Set headers for ZIP file download
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"llm-provider-%s.zip\"", providerID))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipData)))

	// Return ZIP file
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(zipData); err != nil {
		log.Error("Failed to write ZIP response", "provider_id", providerID, "error", err)
	}
}

// GetLLMProxy handles GET /api/internal/v1/llm-proxies/:proxyId
func (c *gatewayInternalController) GetLLMProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	identity, ok := c.authenticateGateway(w, r, "internal-read")
	if !ok {
		return
	}

	ouID := identity.OUID
	gatewayID := identity.ID
	proxyID := r.PathValue("proxyId")
	if proxyID == "" {
		http.Error(w, "Proxy ID is required", http.StatusBadRequest)
		return
	}

	proxy, err := c.gatewayInternalService.GetActiveLLMProxyDeploymentByGateway(ctx, proxyID, ouID, gatewayID)
	if err != nil {
		if errors.Is(err, utils.ErrDeploymentNotActive) {
			http.Error(w, "No active deployment found for this LLM proxy on this gateway", http.StatusNotFound)
			return
		}
		if errors.Is(err, utils.ErrLLMProxyNotFound) {
			http.Error(w, "LLM proxy not found", http.StatusNotFound)
			return
		}
		log.Error("Failed to get LLM proxy", "error", err)
		http.Error(w, "Failed to get LLM proxy", http.StatusInternalServerError)
		return
	}

	// Create ZIP file from LLM proxy YAML file
	zipData, err := utils.CreateLLMProxyYamlZip(proxy)
	if err != nil {
		log.Error("Failed to create ZIP file for LLM proxy", "proxy_id", proxyID, "error", err)
		http.Error(w, "Failed to create LLM proxy package", http.StatusInternalServerError)
		return
	}

	// Set headers for ZIP file download
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"llm-proxy-%s.zip\"", proxyID))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipData)))

	// Return ZIP file
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(zipData); err != nil {
		log.Error("Failed to write ZIP response", "proxy_id", proxyID, "error", err)
	}
}

// GetMCPProxy handles GET /api/internal/v1/mcp-proxies/:proxyId
func (c *gatewayInternalController) GetMCPProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	identity, ok := c.authenticateGateway(w, r, "internal-read")
	if !ok {
		return
	}

	ouID := identity.OUID
	gatewayID := identity.ID
	proxyID := r.PathValue("proxyId")
	if proxyID == "" {
		http.Error(w, "Proxy ID is required", http.StatusBadRequest)
		return
	}
	if _, err := uuid.Parse(proxyID); err != nil {
		http.Error(w, "MCP proxy ID must be an artifact UUID", http.StatusBadRequest)
		return
	}

	proxy, err := c.gatewayInternalService.GetActiveMCPProxyDeploymentByGateway(ctx, proxyID, ouID, gatewayID)
	if err != nil {
		if errors.Is(err, utils.ErrDeploymentNotActive) {
			http.Error(w, "No active deployment found for this MCP proxy on this gateway", http.StatusNotFound)
			return
		}
		if errors.Is(err, utils.ErrMCPProxyNotFound) {
			http.Error(w, "MCP proxy not found", http.StatusNotFound)
			return
		}
		log.Error("Failed to get MCP proxy", "error", err)
		http.Error(w, "Failed to get MCP proxy", http.StatusInternalServerError)
		return
	}

	zipData, err := utils.CreateMCPProxyYamlZip(proxy)
	if err != nil {
		log.Error("Failed to create ZIP file for MCP proxy", "proxy_id", proxyID, "error", err)
		http.Error(w, "Failed to create MCP proxy package", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"mcp-proxy-%s.zip\"", proxyID))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipData)))

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(zipData); err != nil {
		log.Error("Failed to write ZIP response", "proxy_id", proxyID, "error", err)
	}
}

// GetLLMProviderAPIKeys handles GET /api/internal/v1/llm-providers/api-keys
func (c *gatewayInternalController) GetLLMProviderAPIKeys(w http.ResponseWriter, r *http.Request) {
	c.getAPIKeysByKind(w, r, models.KindLLMProvider)
}

// GetLLMProxyAPIKeys handles GET /api/internal/v1/llm-proxies/api-keys
func (c *gatewayInternalController) GetLLMProxyAPIKeys(w http.ResponseWriter, r *http.Request) {
	c.getAPIKeysByKinds(w, r, models.KindLLMProxy, models.KindMCPProxy, models.KindMCPMapping)
}

// GetAPIKeys handles GET /api/internal/v1/apis/api-keys
func (c *gatewayInternalController) GetAPIKeys(w http.ResponseWriter, r *http.Request) {
	c.getAPIKeysByKind(w, r, models.KindAgent)
}

// controlPlaneAPIKeyResponse matches the structure expected by the gateway controller's bulk-sync
type controlPlaneAPIKeyResponse struct {
	ETag         string            `json:"etag"`
	UUID         string            `json:"uuid"`
	Name         string            `json:"name"`
	MaskedAPIKey string            `json:"maskedApiKey"`
	APIKeyHashes map[string]string `json:"apiKeyHashes"`
	ArtifactUUID string            `json:"artifactUuid"`
	Status       string            `json:"status"`
	CreatedAt    string            `json:"createdAt"`
	UpdatedAt    string            `json:"updatedAt"`
	ExpiresAt    *string           `json:"expiresAt,omitempty"`
	Source       string            `json:"source"`
}

func (c *gatewayInternalController) getAPIKeysByKind(w http.ResponseWriter, r *http.Request, kind string) {
	c.getAPIKeysByKinds(w, r, kind)
}

func (c *gatewayInternalController) getAPIKeysByKinds(w http.ResponseWriter, r *http.Request, kinds ...string) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	identity, ok := c.authenticateGateway(w, r, "internal-read")
	if !ok {
		return
	}

	keys := make([]models.StoredAPIKey, 0)
	var envUUIDs []string
	for _, kind := range kinds {
		var (
			kindKeys []models.StoredAPIKey
			err      error
		)
		if kind == models.KindAgent {
			// Agent API keys are environment-scoped: only return keys for environments
			// assigned to this gateway so the gateway controller doesn't try to load
			// API configurations that belong to a different environment's data plane.
			if envUUIDs == nil {
				mappings, mappingErr := c.gatewayService.GetGatewayEnvironmentMappings(identity.ID)
				if mappingErr != nil {
					log.Error("Failed to get gateway environment mappings", "gateway_id", identity.ID, "error", mappingErr)
					http.Error(w, "Failed to list API keys", http.StatusInternalServerError)
					return
				}
				envUUIDs = make([]string, 0, len(mappings))
				for _, m := range mappings {
					envUUIDs = append(envUUIDs, m.EnvironmentUUID.String())
				}
			}
			kindKeys, err = c.apiKeyRepo.ListByArtifactKindAndEnvs(identity.OUID, kind, envUUIDs)
		} else {
			kindKeys, err = c.apiKeyRepo.ListByArtifactKind(identity.OUID, kind)
		}
		if err != nil {
			log.Error("Failed to list API keys", "kind", kind, "error", err)
			http.Error(w, "Failed to list API keys", http.StatusInternalServerError)
			return
		}
		keys = append(keys, kindKeys...)
	}

	result := make([]controlPlaneAPIKeyResponse, 0, len(keys))
	for _, k := range keys {
		resp := controlPlaneAPIKeyResponse{
			UUID:         k.UUID.String(),
			Name:         k.Name,
			MaskedAPIKey: k.MaskedAPIKey,
			APIKeyHashes: map[string]string{"sha256": k.APIKeyHash},
			ArtifactUUID: k.ArtifactUUID.String(),
			Status:       k.Status,
			CreatedAt:    k.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:    k.UpdatedAt.UTC().Format(time.RFC3339),
			Source:       "external",
		}
		if k.ExpiresAt != nil {
			exp := k.ExpiresAt.UTC().Format(time.RFC3339)
			resp.ExpiresAt = &exp
		}
		result = append(result, resp)
	}

	utils.WriteSuccessResponse(w, http.StatusOK, result)
}

// GetSubscriptionPlans handles GET /api/internal/v1/subscription-plans
// Returns subscription plans for the authenticated gateway's organization.
// Currently returns an empty list as subscription-based rate limiting is not used.
func (c *gatewayInternalController) GetSubscriptionPlans(w http.ResponseWriter, r *http.Request) {
	if _, ok := c.authenticateGateway(w, r, "internal-read"); !ok {
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, []struct{}{})
}

type controlPlaneDeploymentsResponse struct {
	Deployments []struct{} `json:"deployments"`
}

// GetDeployments handles GET /api/internal/v1/deployments
// gateway-controller calls this on connect (and periodically) to reconcile its
// local deployment cache with what the control plane believes is deployed, via
// pkg/controlplane.FetchControlPlaneDeployments. Deployment state currently
// lives entirely in the RestApi/LlmProvider CRs the gateway-operator manages,
// not in agent-manager's own DB, so there is nothing yet to report here —
// returns an empty list, matching GetSubscriptionPlans/GetApplications above.
func (c *gatewayInternalController) GetDeployments(w http.ResponseWriter, r *http.Request) {
	if _, ok := c.authenticateGateway(w, r, "internal-read"); !ok {
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, controlPlaneDeploymentsResponse{Deployments: []struct{}{}})
}

// gatewayApplicationResponse is the bulk-sync response format for AI applications.
// Mappings lists the API key UUIDs bound to the application so the gateway can
// rebuild application→API-key bindings on reconnect without a separate request.
type gatewayApplicationResponse struct {
	ApplicationID   string                            `json:"applicationId"`
	ApplicationUUID string                            `json:"applicationUuid"`
	ApplicationName string                            `json:"applicationName"`
	ApplicationType string                            `json:"applicationType"`
	Mappings        []models.ApplicationAPIKeyMapping `json:"mappings"`
	CreatedAt       time.Time                         `json:"createdAt"`
	UpdatedAt       time.Time                         `json:"updatedAt"`
}

// GetApplications handles GET /api/internal/v1/applications
// Returns all AI application records for the authenticated gateway's organization so
// the gateway-controller can bulk-sync application-to-API-key bindings on reconnect.
func (c *gatewayInternalController) GetApplications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	identity, ok := c.authenticateGateway(w, r, "internal-read")
	if !ok {
		return
	}

	apps, err := c.aiApplicationRepo.ListByOrg(ctx, identity.OUID)
	if err != nil {
		log.Error("Failed to list AI applications", "error", err)
		http.Error(w, "Failed to list applications", http.StatusInternalServerError)
		return
	}

	result := make([]gatewayApplicationResponse, 0, len(apps))
	for _, app := range apps {
		keys, err := c.apiKeyRepo.ListByApplicationUUID(app.UUID.String())
		if err != nil {
			log.Error("Failed to list API keys for application", "application_uuid", app.UUID, "error", err)
			http.Error(w, "Failed to list application key bindings", http.StatusInternalServerError)
			return
		}
		mappings := make([]models.ApplicationAPIKeyMapping, 0, len(keys))
		for _, k := range keys {
			mappings = append(mappings, models.ApplicationAPIKeyMapping{APIKeyUUID: k.UUID.String()})
		}
		result = append(result, gatewayApplicationResponse{
			ApplicationID:   app.Handle,
			ApplicationUUID: app.UUID.String(),
			ApplicationName: app.Name,
			ApplicationType: "ai-agent",
			Mappings:        mappings,
			CreatedAt:       app.CreatedAt,
			UpdatedAt:       app.UpdatedAt,
		})
	}

	utils.WriteSuccessResponse(w, http.StatusOK, result)
}

// PushGatewayManifest handles POST /api/internal/v1/gateways/{gatewayId}/manifest
// Receives the gateway's installed policy manifest.
func (c *gatewayInternalController) PushGatewayManifest(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())
	gatewayID := r.PathValue("gatewayId")
	if gatewayID == "" {
		http.Error(w, "Gateway ID is required", http.StatusBadRequest)
		return
	}

	identity, ok := c.authenticateGateway(w, r, "push-manifest")
	if !ok {
		return
	}
	if identity.ID != gatewayID {
		// A gateway presenting a valid key for a different gateway. Recorded
		// separately from a bad key: it is a valid credential used out of
		// scope, which is a stronger signal than an invalid one.
		log.Warn("Gateway manifest token does not match gateway", "gateway_id", gatewayID, "token_gateway_id", identity.ID)
		recordGatewayAuthFailure(r, "gateway-id-mismatch", identity.ID)
		http.Error(w, "Gateway token does not match gateway ID", http.StatusForbidden)
		return
	}

	var manifest map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&manifest); err != nil {
		log.Warn("Failed to decode gateway manifest", "gateway_id", gatewayID, "error", err)
		http.Error(w, "Invalid gateway manifest", http.StatusBadRequest)
		return
	}

	// The only mutating route on the internal server: it replaces the policy
	// manifest the gateway reports as installed. Recorded after the fact — the
	// gateway is a machine client that retries, so refusing the push when the
	// trail is unavailable would leave the mirror stale for no gain.
	err := c.gatewayService.SaveGatewayPolicyManifest(r.Context(), gatewayID, manifest)
	audit.Record(
		r.Context(), audit.ActionGatewayPushManifest,
		audit.Org(identity.OUID),
		audit.Resource(audit.ResourceGateway, gatewayID),
		audit.SurfaceOpt(audit.SurfaceInternal),
		audit.Actor(audit.ActorGateway, gatewayID, ""),
		audit.AuthMethod("api-key"),
		audit.Detail("policyCount", len(manifest)),
		audit.Result(err),
	)
	if err != nil {
		if errors.Is(err, utils.ErrGatewayNotFound) {
			http.Error(w, "Gateway not found", http.StatusNotFound)
			return
		}
		log.Error("Failed to store gateway manifest", "gateway_id", gatewayID, "error", err)
		http.Error(w, "Failed to store gateway manifest", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
