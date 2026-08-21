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
	"net/http"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// toSpecIdentityProvider maps a mirror row to the API representation.
func toSpecIdentityProvider(p models.GatewayIdentityProvider) spec.IdentityProvider {
	issuer := p.Issuer
	jwksURI := p.JWKSUri
	skip := p.JWKSSkipTLSVerify
	providerType := p.Type
	out := spec.IdentityProvider{
		Name:          p.Name,
		Issuer:        &issuer,
		JwksUri:       &jwksURI,
		SkipTlsVerify: &skip,
		Type:          &providerType,
	}
	if p.Description != "" {
		desc := p.Description
		out.Description = &desc
	}
	return out
}

// identityProviderListResponse wraps a slice of providers in the list envelope.
func identityProviderListResponse(providers []spec.IdentityProvider) spec.IdentityProviderListResponse {
	return spec.IdentityProviderListResponse{
		Count: int32(len(providers)),
		List:  providers,
	}
}

// ListIdentityProviders lists every identity provider across the org's gateways,
// enriched with environment and gateway context (org-level Security table).
func (c *gatewayController) ListIdentityProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)

	rows, err := c.gatewayService.ListIdentityProvidersByOrg(ouID)
	if err != nil {
		log.Error("ListIdentityProviders: failed to list", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list identity providers")
		return
	}

	// Environments live in OpenChoreo, not the AMS DB. Resolve the env UUID on each
	// mapping to a display name via the OpenChoreo client (same source the gateways
	// list uses). Best-effort: rows still render without an env name on failure.
	envNames := make(map[string]string)
	if envs, envErr := c.ocClient.ListEnvironments(ctx, ouID); envErr == nil {
		for _, env := range envs {
			envNames[env.UUID] = env.Name
		}
	} else {
		log.Warn("ListIdentityProviders: failed to list environments", "error", envErr)
	}

	providers := make([]spec.IdentityProvider, 0, len(rows))
	for _, row := range rows {
		providers = append(providers, enrichSpecIdentityProvider(row, envNames))
	}
	utils.WriteSuccessResponse(w, http.StatusOK, identityProviderListResponse(providers))
}

// enrichSpecIdentityProvider maps an org-wide row, attaching gateway/env context.
func enrichSpecIdentityProvider(row repositories.IdentityProviderWithContext, envNames map[string]string) spec.IdentityProvider {
	out := toSpecIdentityProvider(row.GatewayIdentityProvider)
	gatewayID := row.GatewayUUID.String()
	out.GatewayId = &gatewayID
	if row.GatewayName != "" {
		name := row.GatewayName
		out.GatewayName = &name
	}
	if row.EnvironmentUUID != "" {
		if name := envNames[row.EnvironmentUUID]; name != "" {
			out.EnvironmentName = &name
		}
	}
	return out
}

// ListGatewayIdentityProviders lists the identity providers mirrored for a gateway.
func (c *gatewayController) ListGatewayIdentityProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	providers, err := c.gatewayService.ListIdentityProvidersByGateway(gatewayID, ouID)
	if err != nil {
		log.Error("ListGatewayIdentityProviders: failed to list", "error", err)
		handleGatewayErrors(w, err, "Failed to list gateway identity providers")
		return
	}

	specProviders := make([]spec.IdentityProvider, 0, len(providers))
	for _, p := range providers {
		specProviders = append(specProviders, toSpecIdentityProvider(p))
	}
	utils.WriteSuccessResponse(w, http.StatusOK, identityProviderListResponse(specProviders))
}

// UpsertGatewayIdentityProvider creates or updates an identity provider mirror row.
// This touches only the AMS mirror, not the gateway ConfigMap (the runtime source of
// truth). Writing here without patching the gateway lets the two diverge: agent OAuth
// deploy-time validation checks the mirror, so a mirror-only issuer passes validation
// while the gateway rejects its tokens at runtime. Use manage-identity-provider.sh.
func (c *gatewayController) UpsertGatewayIdentityProvider(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())
	ouID := middleware.OUIDFromRequest(r)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))
	name := strings.TrimSpace(r.PathValue("name"))

	var req spec.UpsertIdentityProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Warn("UpsertGatewayIdentityProvider: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Which issuers a gateway trusts decides whose tokens it will accept, so
	// this is recorded as a credential-class change and refused when the trail
	// is unavailable. The JWKS URI and skipTlsVerify travel with the issuer
	// because they decide where the signing keys come from and whether that
	// fetch validates TLS — an issuer on its own does not say who can actually
	// mint an accepted token.
	ctx := r.Context()
	attempt, ok := beginAuditOrFail(
		w, r, "UpsertGatewayIdentityProvider", "Failed to upsert identity provider", audit.ActionGatewaySetIdentityProvider,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceGateway, gatewayID, gatewayID),
		audit.Detail("identityProviderName", name),
		audit.Detail("issuer", derefString(req.Issuer)),
		audit.Detail("jwksUri", derefString(req.JwksUri)),
		audit.Detail("skipTlsVerify", derefBool(req.SkipTlsVerify)),
	)
	if !ok {
		return
	}

	provider, err := c.gatewayService.UpsertIdentityProvider(
		ctx, gatewayID, ouID, name,
		derefString(req.Issuer), derefString(req.JwksUri), derefString(req.Description), derefBool(req.SkipTlsVerify),
	)
	if err != nil {
		attempt.Complete(ctx, err)
		log.Error("UpsertGatewayIdentityProvider: failed to upsert", "error", err)
		handleGatewayErrors(w, err, "Failed to upsert identity provider")
		return
	}
	attempt.Complete(ctx, nil)
	utils.WriteSuccessResponse(w, http.StatusOK, toSpecIdentityProvider(*provider))
}

// DeleteGatewayIdentityProvider removes an identity provider mirror row. System
// providers cannot be deleted. Like the upsert, this touches only the AMS mirror, not
// the gateway ConfigMap (the runtime source of truth); deleting here alone leaves the
// issuer trusted by the gateway at runtime. Use manage-identity-provider.sh to keep
// the mirror and the gateway runtime in sync.
func (c *gatewayController) DeleteGatewayIdentityProvider(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())
	ouID := middleware.OUIDFromRequest(r)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))
	name := strings.TrimSpace(r.PathValue("name"))

	ctx := r.Context()
	attempt, ok := beginAuditOrFail(
		w, r, "DeleteGatewayIdentityProvider", "Failed to delete identity provider", audit.ActionGatewayRemoveIdentityProvider,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceGateway, gatewayID, gatewayID),
		audit.Detail("identityProviderName", name),
	)
	if !ok {
		return
	}

	if err := c.gatewayService.DeleteIdentityProvider(ctx, gatewayID, ouID, name); err != nil {
		attempt.Complete(ctx, err)
		log.Error("DeleteGatewayIdentityProvider: failed to delete", "error", err)
		handleGatewayErrors(w, err, "Failed to delete identity provider")
		return
	}
	attempt.Complete(ctx, nil)
	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

// ListEnvironmentIdentityProviders lists the identity providers available in an
// environment (union over the environment's gateways) — the agent OAuth options.
func (c *gatewayController) ListEnvironmentIdentityProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	environmentID := strings.TrimSpace(r.PathValue("environmentId"))

	envUUID, err := c.resolveEnvironmentUUID(ctx, ouID, environmentID)
	if err != nil {
		log.Warn("ListEnvironmentIdentityProviders: failed to resolve environment", "error", err)
		utils.WriteErrorResponse(w, http.StatusNotFound, "Environment not found")
		return
	}

	providers, err := c.gatewayService.ListIdentityProvidersByEnvironment(envUUID)
	if err != nil {
		log.Error("ListEnvironmentIdentityProviders: failed to list", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list environment identity providers")
		return
	}

	specProviders := make([]spec.IdentityProvider, 0, len(providers))
	for _, p := range providers {
		specProviders = append(specProviders, toSpecIdentityProvider(p))
	}
	utils.WriteSuccessResponse(w, http.StatusOK, identityProviderListResponse(specProviders))
}

// DiscoverOidcConfiguration fetches an OpenID Connect discovery document for the
// URL in the `url` query parameter and returns the issuer and JWKS URI, used to
// auto-populate the Add Identity Provider dialog. Manual entry remains available.
func (c *gatewayController) DiscoverOidcConfiguration(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if rawURL == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Query parameter 'url' is required")
		return
	}

	issuer, jwksURI, err := c.gatewayService.DiscoverOIDC(ctx, rawURL)
	if err != nil {
		if errors.Is(err, utils.ErrInvalidURL) {
			log.Warn("DiscoverOidcConfiguration: discovery failed", "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Could not discover OIDC configuration from the provided URL")
			return
		}
		log.Error("DiscoverOidcConfiguration: unexpected error", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to discover OIDC configuration")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, spec.OidcDiscoveryResponse{Issuer: issuer, JwksUri: jwksURI})
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefBool(b *bool) bool {
	return b != nil && *b
}
