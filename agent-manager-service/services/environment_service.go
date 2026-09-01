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
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/google/uuid"

	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/utils/ssrf"
)

// EnvironmentService defines the interface for environment operations
type EnvironmentService interface {
	CreateEnvironment(ctx context.Context, ouID string, req *models.CreateEnvironmentRequest) (*models.GatewayEnvironmentResponse, error)
	GetEnvironment(ctx context.Context, ouID string, envID string) (*models.GatewayEnvironmentResponse, error)
	ListEnvironments(ctx context.Context, ouID string, limit, offset int32) (*models.EnvironmentListResponse, error)
	UpdateEnvironment(ctx context.Context, ouID string, envID string, req *models.UpdateEnvironmentRequest) (*models.GatewayEnvironmentResponse, error)
	DeleteEnvironment(ctx context.Context, ouID string, envID string) error
	GetEnvironmentGateways(ctx context.Context, ouID string, envID string) ([]models.GatewayResponse, error)
	ListThunderInstances(ctx context.Context, ouID string) (*models.ThunderInstanceListResponse, error)
	// SetThunderSystemClientSecret stores (encrypted) the env-Thunder
	// system-client credential used to reach an environment's Thunder admin API,
	// keyed by ouID (stable, multi-tenant-safe).
	SetThunderSystemClientSecret(ctx context.Context, ouID, envName, clientID, clientSecret string) error
	// DeleteThunderSystemClientSecret removes that credential (idempotent), looked up by ouID.
	DeleteThunderSystemClientSecret(ctx context.Context, ouID, envName string) error
	// SetThunderURL registers this environment's env-Thunder registration via
	// exactly one of two mutually-exclusive paths:
	//   - handle set (or both empty): on-prem path. An unguessable handle
	//     replaces the predictable <org>-<env> segment of the environment's
	//     hostname; AMS computes and stores the full origin server-side. An
	//     empty handle auto-generates one.
	//   - url set: the caller already knows the real, already-provisioned
	//     origin (useful when different environments live under different
	//     domains, so there is no single pattern AMS could compute); AMS
	//     validates and stores it verbatim, with no handle at all.
	// Returns utils.ErrThunderHandleAndURLBothSet if both are non-empty,
	// utils.ErrThunderHandleTaken/utils.ErrThunderURLTaken if the respective
	// value is already registered to a different environment.
	SetThunderURL(ctx context.Context, ouID, envName, handle, url string) (models.ThunderURLRecord, error)
	// GetThunderURL returns the registered record for (ouID, envName), or
	// utils.ErrThunderHandleNotFound if nothing has been registered.
	GetThunderURL(ctx context.Context, ouID, envName string) (models.ThunderURLRecord, error)
	// DeleteThunderURL removes the handle (idempotent), freeing it for reuse.
	DeleteThunderURL(ctx context.Context, ouID, envName string) error
	// IsThunderHandleAvailable checks whether handle passes format validation
	// and is not already registered to ANY environment. This is a UX
	// convenience for the console's Create Environment drawer, letting it
	// reject an obviously-taken handle before the user ever runs the
	// generated script — it is NOT authoritative: SetThunderURL's atomic
	// insert (via the DB's unique constraint) remains the only real
	// enforcement, since a handle could be claimed by someone else between
	// this check and the actual registration. Returns a validation error
	// (wrapping utils.ErrInvalidThunderHandle) if handle's format itself is
	// invalid, distinct from "valid but taken" (available=false, err=nil).
	IsThunderHandleAvailable(ctx context.Context, handle string) (bool, error)
	// ThunderHandleRegistered reports whether handle is registered to any
	// environment, without validating handle's format first — see doc comment
	// on the implementation for why that distinction matters.
	ThunderHandleRegistered(ctx context.Context, handle string) (bool, error)
}

type environmentService struct {
	logger             *slog.Logger
	ocClient           occlient.OpenChoreoClient
	gatewayRepo        repositories.GatewayRepository
	thunderProber      thundersvc.Prober
	agentConfigService AgentConfigurationService
	envThunderRepo     repositories.EnvThunderSystemClientRepository
	envThunderURLRepo  repositories.EnvThunderURLRepository
	encryptionKey      []byte
}

// NewEnvironmentService creates a new environment service
func NewEnvironmentService(logger *slog.Logger, gatewayRepo repositories.GatewayRepository, ocClient occlient.OpenChoreoClient, thunderProber thundersvc.Prober, agentConfigService AgentConfigurationService, envThunderRepo repositories.EnvThunderSystemClientRepository, envThunderURLRepo repositories.EnvThunderURLRepository, encryptionKey []byte) EnvironmentService {
	return &environmentService{
		logger:             logger,
		gatewayRepo:        gatewayRepo,
		ocClient:           ocClient,
		thunderProber:      thunderProber,
		agentConfigService: agentConfigService,
		envThunderRepo:     envThunderRepo,
		envThunderURLRepo:  envThunderURLRepo,
		encryptionKey:      encryptionKey,
	}
}

func (s *environmentService) CreateEnvironment(ctx context.Context, ouID string, req *models.CreateEnvironmentRequest) (*models.GatewayEnvironmentResponse, error) {
	s.logger.Info("Creating environment in OpenChoreo", "name", req.Name, "ouID", ouID)

	if req.DataplaneRef == "" {
		s.logger.Warn("No dataplaneRef provided", "name", req.Name, "ouID", ouID)
	}

	// Reject unsupported isolation tiers up front. An unknown value would otherwise be
	// persisted verbatim on the Environment CR and silently fall back to runc at render
	// time (runtimeClassForIsolationTier returns "" for anything unrecognised), hiding the
	// typo from the user until they notice their agents are not actually isolated.
	switch req.IsolationTier {
	case "", utils.IsolationTierGvisor, utils.IsolationTierKata:
	default:
		return nil, fmt.Errorf("%w: unsupported isolation tier %q (allowed: gvisor, kata)", utils.ErrInvalidInput, req.IsolationTier)
	}

	ocReq := occlient.CreateEnvironmentRequest{
		Name:          req.Name,
		DisplayName:   req.DisplayName,
		Description:   req.Description,
		IsolationTier: req.IsolationTier,
		DataplaneRef:  req.DataplaneRef,
		IsProduction:  req.IsProduction,
		Gateway:       toOCClientGatewaySpec(req.Gateway),
	}

	env, err := s.ocClient.CreateEnvironment(ctx, ouID, ocReq)
	if err != nil {
		s.logger.Error("Failed to create environment in OpenChoreo", "ouID", ouID, "name", req.Name, "error", err)
		return nil, fmt.Errorf("failed to create environment: %w", err)
	}

	return &models.GatewayEnvironmentResponse{
		UUID:             env.UUID,
		OrganizationName: ouID,
		Name:             env.Name,
		DisplayName:      env.DisplayName,
		Description:      req.Description,
		IsolationTier:    env.IsolationTier,
		DataplaneRef:     env.DataplaneRef,
		IsProduction:     env.IsProduction,
		Gateway:          env.Gateway,
		CreatedAt:        env.CreatedAt,
		UpdatedAt:        env.CreatedAt,
	}, nil
}

func (s *environmentService) GetEnvironment(ctx context.Context, ouID string, envID string) (*models.GatewayEnvironmentResponse, error) {
	s.logger.Info("Getting environment from OpenChoreo", "envID", envID, "ouID", ouID)

	// envID in this context is the environment name (not UUID)
	// since OpenChoreo API uses environment name as identifier
	env, err := s.ocClient.GetEnvironment(ctx, ouID, envID)
	if err != nil {
		s.logger.Error("Failed to get environment from OpenChoreo", "ouID", ouID, "envID", envID, "error", err)
		// Check if it's a not-found error
		if errors.Is(err, utils.ErrEnvironmentNotFound) {
			return nil, utils.ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	// Convert OpenChoreo EnvironmentResponse to GatewayEnvironmentResponse
	return &models.GatewayEnvironmentResponse{
		UUID:             env.UUID,
		OrganizationName: ouID,
		Name:             env.Name,
		DisplayName:      env.DisplayName,
		Description:      env.Description,
		IsolationTier:    env.IsolationTier,
		DataplaneRef:     env.DataplaneRef,
		DNSPrefix:        env.DNSPrefix,
		IsProduction:     env.IsProduction,
		Gateway:          env.Gateway,
		CreatedAt:        env.CreatedAt,
		UpdatedAt:        env.CreatedAt,
	}, nil
}

func (s *environmentService) ListEnvironments(ctx context.Context, ouID string, limit, offset int32) (*models.EnvironmentListResponse, error) {
	s.logger.Info("Listing environments from OpenChoreo", "ouID", ouID, "limit", limit, "offset", offset)

	// Fetch environments directly from OpenChoreo
	ocEnvironments, err := s.ocClient.ListEnvironments(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to list environments from OpenChoreo", "ouID", ouID, "error", err)
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	total := int32(len(ocEnvironments))

	// Apply pagination
	start := int(offset)
	end := start + int(limit)

	if start >= len(ocEnvironments) {
		// Offset is beyond available data
		return &models.EnvironmentListResponse{
			Environments: []models.GatewayEnvironmentResponse{},
			Total:        total,
			Limit:        limit,
			Offset:       offset,
		}, nil
	}

	if end > len(ocEnvironments) {
		end = len(ocEnvironments)
	}

	paginatedEnvs := ocEnvironments[start:end]

	// Convert OpenChoreo environment responses to gateway environment responses
	responses := make([]models.GatewayEnvironmentResponse, len(paginatedEnvs))
	for i, env := range paginatedEnvs {
		responses[i] = models.GatewayEnvironmentResponse{
			UUID:             env.UUID,
			OrganizationName: ouID,
			Name:             env.Name,
			DisplayName:      env.DisplayName,
			Description:      env.Description,
			IsolationTier:    env.IsolationTier,
			DataplaneRef:     env.DataplaneRef,
			DNSPrefix:        env.DNSPrefix,
			IsProduction:     env.IsProduction,
			Gateway:          env.Gateway,
			CreatedAt:        env.CreatedAt,
			UpdatedAt:        env.CreatedAt,
		}
	}

	return &models.EnvironmentListResponse{
		Environments: responses,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
	}, nil
}

func (s *environmentService) UpdateEnvironment(ctx context.Context, ouID string, envID string, req *models.UpdateEnvironmentRequest) (*models.GatewayEnvironmentResponse, error) {
	s.logger.Info("Updating environment in OpenChoreo", "envID", envID, "ouID", ouID)

	ocReq := occlient.UpdateEnvironmentRequest{
		DisplayName:  req.DisplayName,
		Description:  req.Description,
		IsProduction: req.IsProduction,
		Gateway:      toOCClientGatewaySpec(req.Gateway),
	}

	env, err := s.ocClient.UpdateEnvironment(ctx, ouID, envID, ocReq)
	if err != nil {
		s.logger.Error("Failed to update environment in OpenChoreo", "ouID", ouID, "envID", envID, "error", err)
		if errors.Is(err, utils.ErrNotFound) || errors.Is(err, utils.ErrEnvironmentNotFound) {
			return nil, utils.ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to update environment: %w", err)
	}

	description := ""
	if req.Description != nil {
		description = *req.Description
	}

	return &models.GatewayEnvironmentResponse{
		UUID:             env.UUID,
		OrganizationName: ouID,
		Name:             env.Name,
		DisplayName:      env.DisplayName,
		Description:      description,
		DataplaneRef:     env.DataplaneRef,
		IsProduction:     env.IsProduction,
		Gateway:          env.Gateway,
		CreatedAt:        env.CreatedAt,
		UpdatedAt:        env.CreatedAt,
	}, nil
}

// DeleteEnvironment removes an environment from OpenChoreo and cleans up local DB state.
//
// OpenChoreo is the source of truth for "is anything actually deployed here": it refuses
// Environment deletion while ReleaseBindings or workloads still reference it, so we let
// the OC API server enforce that and surface whatever it returns.
//
// On success: delete the OpenChoreo Environment CR, then delete any gateway↔env mapping rows
func (s *environmentService) DeleteEnvironment(ctx context.Context, ouID string, envID string) error {
	s.logger.Info("Deleting environment", "envID", envID, "ouID", ouID)

	// envID is the environment name (matching OpenChoreo's identifier); resolve the UUID via OC
	// because the local DB doesn't have its own environments table.
	env, err := s.ocClient.GetEnvironment(ctx, ouID, envID)
	if err != nil {
		s.logger.Error("Failed to look up environment", "ouID", ouID, "envID", envID, "error", err)
		if errors.Is(err, utils.ErrNotFound) || errors.Is(err, utils.ErrEnvironmentNotFound) {
			return utils.ErrEnvironmentNotFound
		}
		return fmt.Errorf("failed to look up environment: %w", err)
	}

	envUUID, parseErr := uuid.Parse(env.UUID)
	if parseErr != nil {
		s.logger.Error("Invalid env UUID from OpenChoreo", "uuid", env.UUID, "error", parseErr)
		return fmt.Errorf("invalid environment UUID: %w", parseErr)
	}

	// Block deletion if any deployment pipeline still references this environment in its
	// promotion paths (either as a source or a target environment).
	pipelines, err := s.ocClient.ListDeploymentPipelines(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to list deployment pipelines while checking environment references", "ouID", ouID, "envID", envID, "error", err)
		return fmt.Errorf("failed to verify environment references: %w", err)
	}
	var referencingPipelines []string
	for _, pipeline := range pipelines {
		if pipeline != nil && pipelineReferencesEnvironment(pipeline, env.Name) {
			referencingPipelines = append(referencingPipelines, pipeline.Name)
		}
	}
	if len(referencingPipelines) > 0 {
		s.logger.Warn("Cannot delete environment referenced by deployment pipelines", "ouID", ouID, "envID", envID, "pipelines", referencingPipelines)
		return fmt.Errorf("%w: %v", utils.ErrEnvironmentInUse, referencingPipelines)
	}

	// Delete in OpenChoreo first. If OC refuses (release bindings still exist, etc.) we surface
	// that error without having touched local state. A not-found from OC after UUID resolution
	// is treated as idempotent so we still clean up local gateway↔env mappings.
	if err := s.ocClient.DeleteEnvironment(ctx, ouID, env.Name); err != nil {
		s.logger.Error("Failed to delete environment in OpenChoreo", "ouID", ouID, "envID", envID, "error", err)
		if errors.Is(err, utils.ErrNotFound) || errors.Is(err, utils.ErrEnvironmentNotFound) {
			s.logger.Warn("Environment already absent in OpenChoreo; continuing local cleanup",
				"ouID", ouID, "envID", envID, "envUUID", envUUID)
		} else {
			return fmt.Errorf("failed to delete environment in OpenChoreo: %w", err)
		}
	}

	// Cascade MCP-proxy cleanup: tear down every agent-scoped MCP mapping deployed into this
	// env and strip the env block from every org-level MCP proxy blueprint. Best-effort — the
	// environment is already gone from OpenChoreo, so cleanup errors are logged, not fatal.
	if s.agentConfigService != nil {
		if err := s.agentConfigService.CleanupEnvironmentMCPArtifacts(ctx, ouID, envUUID, env.Name); err != nil {
			s.logger.Warn("environment deleted; MCP artifact cleanup had errors",
				"ouID", ouID, "envID", envID, "envUUID", envUUID, "error", err)
		}
	}

	// Local cleanup: gateway↔env mapping rows. The gateway themselves are unaffected.
	deleted, err := s.gatewayRepo.DeleteEnvironmentMappingsByEnvironmentID(envUUID.String())
	if err != nil {
		s.logger.Error("Environment deleted in OpenChoreo but local gateway-mapping cleanup failed",
			"envUUID", envUUID, "error", err)
		return fmt.Errorf("environment deleted but gateway mapping cleanup failed: %w", err)
	}
	s.logger.Info("Deleted environment", "envID", envID, "envUUID", envUUID, "gatewayMappingsDeleted", deleted)
	return nil
}

func (s *environmentService) GetEnvironmentGateways(ctx context.Context, ouID string, envID string) ([]models.GatewayResponse, error) {
	s.logger.Info("Getting environment gateways", "envID", envID, "ouID", ouID)

	// Verify environment exists in OpenChoreo (envID is environment name)
	env, err := s.ocClient.GetEnvironment(ctx, ouID, envID)
	if err != nil {
		s.logger.Error("Failed to get environment from OpenChoreo", "ouID", ouID, "envID", envID, "error", err)
		if errors.Is(err, utils.ErrEnvironmentNotFound) {
			return nil, utils.ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to verify environment: %w", err)
	}

	// Parse environment UUID
	envUUID, err := uuid.Parse(env.UUID)
	if err != nil {
		s.logger.Error("Failed to parse environment UUID", "uuid", env.UUID, "error", err)
		return nil, fmt.Errorf("invalid environment UUID: %w", err)
	}

	// Get gateway-environment mappings from repository
	mappings, err := s.gatewayRepo.GetEnvironmentMappingsByEnvironmentID(envUUID.String())
	if err != nil {
		s.logger.Error("Failed to get gateway mappings from repository", "environmentID", envUUID.String(), "error", err)
		return nil, fmt.Errorf("failed to get gateway mappings: %w", err)
	}

	// Fetch each gateway from the gateway repository
	responses := make([]models.GatewayResponse, 0, len(mappings))
	for _, mapping := range mappings {
		gatewayID := mapping.GatewayUUID.String()

		// Get gateway details from repository
		gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
		if err != nil {
			s.logger.Warn("Failed to get gateway from repository", "gatewayID", gatewayID, "error", err)
			continue
		}
		if gateway == nil {
			s.logger.Warn("Gateway not found", "gatewayID", gatewayID)
			continue
		}

		// Convert gateway model to response
		status := string(models.GatewayStatusInactive)
		if gateway.IsActive {
			status = string(models.GatewayStatusActive)
		}

		responses = append(responses, models.GatewayResponse{
			UUID:             gateway.UUID.String(),
			OrganizationName: ouID,
			Name:             gateway.Name,
			DisplayName:      gateway.DisplayName,
			GatewayType:      gateway.GatewayFunctionalityType,
			VHost:            gateway.Vhost,
			IsCritical:       gateway.IsCritical,
			Status:           status,
		})
	}

	return responses, nil
}

// pipelineReferencesEnvironment reports whether any promotion path in the pipeline
// references the given environment name, either as the source or as a target.
func pipelineReferencesEnvironment(pipeline *models.DeploymentPipelineResponse, envName string) bool {
	for _, path := range pipeline.PromotionPaths {
		if path.SourceEnvironmentRef == envName {
			return true
		}
		for _, target := range path.TargetEnvironmentRefs {
			if target.Name == envName {
				return true
			}
		}
	}
	return false
}

// -----------------------------------------------------------------------------
// Gateway spec translation: models.GatewaySpec (internal DTO) → occlient
// GatewaySpec (OC-bound request type). Direct field-by-field copy; the OC
// client layer constructs the runtime-only fields (gateway resource name/
// namespace, listener name).
// -----------------------------------------------------------------------------

func toOCClientGatewaySpec(g *models.GatewaySpec) *occlient.GatewaySpec {
	if g == nil {
		return nil
	}
	return &occlient.GatewaySpec{
		Ingress: toOCClientGatewayNetworkSpec(g.Ingress),
		Egress:  toOCClientGatewayNetworkSpec(g.Egress),
	}
}

func toOCClientGatewayNetworkSpec(n *models.GatewayNetworkSpec) *occlient.GatewayNetworkSpec {
	if n == nil {
		return nil
	}
	return &occlient.GatewayNetworkSpec{
		External: toOCClientGatewayEndpointSpec(n.External),
		Internal: toOCClientGatewayEndpointSpec(n.Internal),
	}
}

func toOCClientGatewayEndpointSpec(e *models.GatewayEndpointSpec) *occlient.GatewayEndpointSpec {
	if e == nil {
		return nil
	}
	return &occlient.GatewayEndpointSpec{
		HTTP:  toOCClientGatewayListenerSpec(e.HTTP),
		HTTPS: toOCClientGatewayListenerSpec(e.HTTPS),
		TLS:   toOCClientGatewayListenerSpec(e.TLS),
	}
}

func toOCClientGatewayListenerSpec(l *models.GatewayListenerSpec) *occlient.GatewayListenerSpec {
	if l == nil {
		return nil
	}
	return &occlient.GatewayListenerSpec{Port: l.Port, Host: l.Host}
}

// ListThunderInstances returns the Thunder OAuth2 identity provider info for every
// environment in the org whose env-Thunder instance is reachable.
//
// Reachability is determined by a live HTTP probe to the environment's JWKS endpoint,
// NOT by inferring from gateway mappings. Gateway mappings only prove a gateway was
// provisioned — not that Thunder was ever deployed (e.g. PROVISION_THUNDER=false or a
// failed provision both leave mappings behind with no Thunder instance). Advertising
// dead endpoints would cause the console Identity page to show broken issuer/token/JWKS
// URLs.
//
// maxThunderProbeConcurrency bounds how many environments' handle-read+probe run at
// once, so an org with hundreds of environments doesn't fire hundreds of simultaneous
// outbound HTTP probes (each itself fanning out to multiple candidate URLs — see
// thunderBaseURLCandidates).
const maxThunderProbeConcurrency = 10

func (s *environmentService) ListThunderInstances(ctx context.Context, ouID string) (*models.ThunderInstanceListResponse, error) {
	envs, err := s.ocClient.ListEnvironments(ctx, ouID)
	if err != nil {
		return nil, fmt.Errorf("list environments for org %s: %w", ouID, err)
	}

	// Thunder naming (release name, namespace, host, issuer URL) is keyed by the
	// org NAME the provisioning scripts deployed with (ORG_NAME, e.g. "default"),
	// which is the OpenChoreo namespace — not the OU id from the JWT. Probing or
	// advertising URLs derived from the OU id would point at instances that don't
	// exist.
	orgNamespace, err := ResolveNamespace(ctx, s.ocClient)
	if err != nil {
		return nil, err
	}

	// An environment with no registered handle has no address to probe, so it's
	// skipped without even trying. Probing is fanned out across goroutines (bounded
	// by maxThunderProbeConcurrency) rather than run sequentially, since each probe
	// can take a couple of seconds in the worst case.
	//
	// A real handle-read error (as opposed to "genuinely never provisioned", which
	// readThunderHandle returns as ("", nil)) must fail the whole call rather than
	// silently drop that one environment from the response — a transient DB hiccup
	// must never look identical to "this environment has no Thunder instance" on
	// the console's Identity page.
	reachable := make([]bool, len(envs))
	thunderURLs := make([]string, len(envs))
	sem := make(chan struct{}, maxThunderProbeConcurrency)
	var wg sync.WaitGroup
	var readErrOnce sync.Once
	var readErr error
	for i, env := range envs {
		if env == nil || env.Name == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, envName string) {
			defer wg.Done()
			defer func() { <-sem }()
			rec, err := s.resolveThunderRecord(ctx, ouID, envName)
			if err != nil {
				readErrOnce.Do(func() { readErr = fmt.Errorf("read thunder url for env %s: %w", envName, err) })
				return
			}
			thunderURLs[idx] = rec.URL
			if rec.URL == "" {
				return // not provisioned — reachable[idx] stays false, no probe needed
			}
			reachable[idx] = s.thunderProber.Probe(ctx, orgNamespace, envName, rec.URL, rec.Handle == "")
		}(i, env.Name)
	}
	wg.Wait()
	if readErr != nil {
		return nil, readErr
	}

	instances := make([]models.ThunderInstanceResponse, 0, len(envs))
	for i, env := range envs {
		if env == nil || env.Name == "" {
			continue
		}

		// Reachability is the only reliable signal: environments created with PROVISION_THUNDER=false,
		// environments whose provisioning failed silently (non-fatal by design), and
		// environments with no registered handle all pass the gateway-mappings check but
		// have no Thunder instance.
		if !reachable[i] {
			s.logger.Debug("env-Thunder not reachable, skipping", "envName", env.Name)
			continue
		}

		// All three URLs must be externally reachable — this response is developer-facing
		// (console Identity page, copy-buttons). The internal svc.cluster.local addresses
		// only resolve inside the cluster and would cause DNS failures for developers.
		instances = append(instances, models.ThunderInstanceResponse{
			EnvName:      env.Name,
			DisplayName:  env.DisplayName,
			IsProduction: env.IsProduction,
			IssuerURL:    thundersvc.ThunderIssuerURL(thunderURLs[i]),
			TokenURL:     thundersvc.ThunderExternalTokenURL(thunderURLs[i]),
			JWKSURL:      thundersvc.ThunderExternalJWKSURL(thunderURLs[i]),
			Namespace:    thundersvc.ThunderNamespace(orgNamespace, env.Name),
		})
	}
	return &models.ThunderInstanceListResponse{ThunderInstances: instances}, nil
}

// resolveThunderRecord looks up envName's env-Thunder registration via
// ResolveThunderURL, which distinguishes "never provisioned" (zero-value
// record, nil error) from a real read failure (wrapped error).
func (s *environmentService) resolveThunderRecord(ctx context.Context, ouID, envName string) (models.ThunderURLRecord, error) {
	return ResolveThunderURL(ctx, s.envThunderURLRepo, ouID, envName)
}

// thunderHandlePattern matches a DNS-label-safe handle: lowercase alphanumeric,
// hyphens allowed but not leading/trailing — the same shape as ENV_NAME's own
// validation (deployments/scripts/add-environment.sh), so it's always a valid
// single DNS label once combined into "<handle>.<baseDomain>".
var thunderHandlePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// maxThunderHandleLen mirrors the 63-character DNS label limit any hostname
// built from the handle is subject to (see thundersvc.ThunderIssuerURL, which
// embeds the handle as-is into "<handle>.<baseDomain>").
const maxThunderHandleLen = 63

// minThunderHandleLen is the hard floor on a caller-supplied handle, not just
// UI advice — enforced server-side regardless of what any client sends.
const minThunderHandleLen = 3

// reservedThunderHandles blocks labels that identify a real platform component
// or namespace — allowing a handle to equal one risks hijacking or confusion
// once that component sits at the same hostname level (<handle>.<baseDomain>).
// Covers every fixed subdomain across all deployment flavors (VM, k3d/quick-start,
// docker-compose, the custom-domain guide, and the AI Gateway setup target), since
// several services go by different names depending on flavor. Mirrored on the
// console side by environmentSchema.ts's RESTRICTED_THUNDER_HANDLES; keep both in sync.
var reservedThunderHandles = map[string]bool{
	"kubernetes":           true,
	"kube-system":          true,
	"kube-public":          true,
	"kube-node-lease":      true,
	"openchoreo":           true,
	"opensearch":           true,
	"prometheus":           true,
	"otel-collector":       true,
	"fluent-bit":           true,
	"agent-manager":        true,
	"observability":        true,
	"console":              true,
	"api":                  true,
	"api-amp":              true,
	"thunder":              true,
	"observer":             true,
	"traces":               true,
	"gateway":              true,
	"api-platform-gateway": true,
	"ai-gateway":           true,
	"otel":                 true,
	"cp":                   true,
	"agents":               true,
}

// validateThunderHandle checks handle's format. It does not check uniqueness —
// that's enforced atomically by the DB's unique constraint on Upsert.
func validateThunderHandle(handle string) error {
	if handle == "" {
		return fmt.Errorf("%w: handle is required", utils.ErrInvalidThunderHandle)
	}
	if len(handle) < minThunderHandleLen {
		return fmt.Errorf("%w: handle must be at least %d characters", utils.ErrInvalidThunderHandle, minThunderHandleLen)
	}
	if len(handle) > maxThunderHandleLen {
		return fmt.Errorf("%w: handle exceeds %d characters", utils.ErrInvalidThunderHandle, maxThunderHandleLen)
	}
	if !thunderHandlePattern.MatchString(handle) {
		return fmt.Errorf("%w: must be lowercase alphanumeric with hyphens, no leading/trailing hyphen", utils.ErrInvalidThunderHandle)
	}
	if reservedThunderHandles[handle] {
		return fmt.Errorf("%w: %q is a reserved word", utils.ErrInvalidThunderHandle, handle)
	}
	return nil
}

// generatedThunderHandleLen matches the task's own spec: an auto-generated handle
// is exactly 10 characters.
const generatedThunderHandleLen = 10

// thunderHandleAlphabet is lowercase alphanumeric — every symbol is already
// valid against thunderHandlePattern, so no post-generation validation is needed.
const thunderHandleAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// maxGenerateThunderHandleAttempts bounds the collision-retry loop in
// SetThunderURL. A collision is extremely unlikely; this just guards against
// a pathological run of bad luck.
const maxGenerateThunderHandleAttempts = 5

// generateThunderHandle returns a random, generatedThunderHandleLen-character
// handle drawn from thunderHandleAlphabet.
func generateThunderHandle() (string, error) {
	// rand.Int, not a byte-mod-len(alphabet) reduction: 256 isn't a multiple of
	// len(thunderHandleAlphabet), so reducing a random byte mod 36 would make
	// the alphabet's first 4 characters slightly likelier than the rest —
	// avoidable bias in a value whose whole point is being unguessable.
	alphabetLen := big.NewInt(int64(len(thunderHandleAlphabet)))
	out := make([]byte, generatedThunderHandleLen)
	for i := range out {
		n, err := rand.Int(rand.Reader, alphabetLen)
		if err != nil {
			return "", fmt.Errorf("generate thunder url handle: %w", err)
		}
		out[i] = thunderHandleAlphabet[n.Int64()]
	}
	return string(out), nil
}

// maxThunderURLLen mirrors the thunder_url column's VARCHAR(2048) cap.
const maxThunderURLLen = 2048

// validateThunderURL checks rawURL's shape for the caller-supplied URL
// registration path and returns it normalized to a bare "scheme://host[:port]"
// origin (stripping any trailing "/", since callers append their own paths).
// Must be an absolute http(s) origin with no userinfo/path/query/fragment.
// Also runs ssrf.ValidateURL: unlike a handle (always AMS's own label under
// its own trusted domain), this exact value gets dialed later by
// EnvThunderResolver — genuinely attacker-influenced input, which a handle
// never was.
func validateThunderURL(ctx context.Context, rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("%w: url is required", utils.ErrInvalidThunderURL)
	}
	if len(rawURL) > maxThunderURLLen {
		return "", fmt.Errorf("%w: url exceeds %d characters", utils.ErrInvalidThunderURL, maxThunderURLLen)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("%w: %s", utils.ErrInvalidThunderURL, err.Error())
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%w: scheme must be http or https", utils.ErrInvalidThunderURL)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("%w: host is required", utils.ErrInvalidThunderURL)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("%w: must not contain userinfo", utils.ErrInvalidThunderURL)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", fmt.Errorf("%w: must be a bare origin with no path", utils.ErrInvalidThunderURL)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%w: must not contain a query or fragment", utils.ErrInvalidThunderURL)
	}
	// A url under THIS deployment's own base domain is still checked against
	// reservedThunderHandles, the same protection the handle path enforces:
	// without it, a caller could register e.g. "https://console.<baseDomain>"
	// and AMS would dial the platform's own console with the environment's
	// system-client credentials, and advertise it as the environment's
	// issuer/JWKS. A url under a DIFFERENT domain (the common case) never
	// matches this deployment's base domain, so this never fires for it.
	hostname := strings.TrimSuffix(parsed.Hostname(), ".")
	baseDomain := strings.TrimSuffix(config.GetConfig().ThunderHostBaseDomain, ".")
	if label, rest, ok := strings.Cut(hostname, "."); ok &&
		strings.EqualFold(rest, baseDomain) &&
		reservedThunderHandles[strings.ToLower(label)] {
		return "", fmt.Errorf("%w: %q is a reserved subdomain of %s", utils.ErrInvalidThunderURL, label, rest)
	}
	origin := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
	if err := ssrf.ValidateURL(ctx, origin); err != nil {
		return "", fmt.Errorf("%w: %s", utils.ErrInvalidThunderURL, err.Error())
	}
	return origin, nil
}

// SetThunderURL registers this environment's env-Thunder record via exactly
// one of two mutually-exclusive paths (rejecting both handle and url set at
// once with utils.ErrThunderHandleAndURLBothSet) — see the interface doc
// comment for the two paths' shapes.
//
// Idempotent, and never changes an already-registered record: Thunder's
// issuer is immutable once minted, so a blank or matching request reuses it
// as a no-op, and an explicit different one is rejected (409) — callers that
// want to change it must call DeleteThunderURL first.
//
// Safe under concurrent first-time provisioning: the up-front read is only a
// fast path. The real guarantee is claimThunderHandle/claimThunderURL's use
// of EnvThunderURLRepository.Insert (insert-only, not upsert) — whichever
// INSERT commits first wins, and the loser discovers that via the DB's own
// unique-constraint check.
func (s *environmentService) SetThunderURL(ctx context.Context, ouID, envName, handle, rawURL string) (models.ThunderURLRecord, error) {
	if ouID == "" {
		return models.ThunderURLRecord{}, fmt.Errorf("%w: ouID is required", utils.ErrInvalidInput)
	}
	if envName == "" {
		return models.ThunderURLRecord{}, fmt.Errorf("%w: envName is required", utils.ErrInvalidInput)
	}
	if handle != "" && rawURL != "" {
		return models.ThunderURLRecord{}, utils.ErrThunderHandleAndURLBothSet
	}

	existing, err := ResolveThunderURL(ctx, s.envThunderURLRepo, ouID, envName)
	if err != nil {
		return models.ThunderURLRecord{}, fmt.Errorf("failed to check for an existing thunder url for %s/%s: %w", ouID, envName, err)
	}

	if rawURL != "" {
		normalized, err := validateThunderURL(ctx, rawURL)
		if err != nil {
			return models.ThunderURLRecord{}, err
		}
		if existing.URL != "" {
			return s.reuseOrRejectThunderURL(existing, normalized, ouID, envName)
		}
		return s.claimThunderURL(ctx, ouID, envName, normalized)
	}

	if existing.URL != "" {
		return s.reuseOrRejectThunderHandle(existing, handle, ouID, envName)
	}

	if handle != "" {
		if err := validateThunderHandle(handle); err != nil {
			return models.ThunderURLRecord{}, err
		}
		return s.claimThunderHandle(ctx, ouID, envName, handle, false)
	}

	for attempt := 1; attempt <= maxGenerateThunderHandleAttempts; attempt++ {
		generated, err := generateThunderHandle()
		if err != nil {
			return models.ThunderURLRecord{}, err
		}
		resolved, err := s.claimThunderHandle(ctx, ouID, envName, generated, true)
		if err == nil {
			return resolved, nil
		}
		if errors.Is(err, utils.ErrThunderHandleTaken) || errors.Is(err, utils.ErrThunderURLTaken) {
			// The generated value's handle or computed URL collided with a
			// different environment's — try a fresh one.
			s.logger.Debug("generated thunder url handle collided, retrying", "ouID", ouID, "envName", envName, "attempt", attempt)
			continue
		}
		return models.ThunderURLRecord{}, err
	}
	return models.ThunderURLRecord{}, fmt.Errorf("failed to generate a unique thunder url handle for %s/%s after %d attempts", ouID, envName, maxGenerateThunderHandleAttempts)
}

// reuseOrRejectThunderHandle applies SetThunderURL's idempotency rule for the
// on-prem handle path once an existing registration is known (via an
// up-front read, or by losing an insert race — see claimThunderHandle): a
// blank or matching request reuses it as a no-op; a different EXPLICIT
// request is rejected. Also correctly rejects an explicit handle against an
// existing caller-supplied (handle-less) row, since existing.Handle is "" there.
func (s *environmentService) reuseOrRejectThunderHandle(existing models.ThunderURLRecord, requested, ouID, envName string) (models.ThunderURLRecord, error) {
	if requested == "" || requested == existing.Handle {
		s.logger.Info("Reusing already-registered env-thunder url handle", "ouID", ouID, "envName", envName)
		return existing, nil
	}
	if existing.Handle == "" {
		return models.ThunderURLRecord{}, fmt.Errorf("%w: %s/%s already has a registered thunder url %q (registered via the url path, no handle) — call DeleteThunderURL first to change it",
			utils.ErrThunderHandleTaken, ouID, envName, existing.URL)
	}
	return models.ThunderURLRecord{}, fmt.Errorf("%w: %s/%s already has a registered handle %q — call DeleteThunderURL first to change it",
		utils.ErrThunderHandleTaken, ouID, envName, existing.Handle)
}

// reuseOrRejectThunderURL is reuseOrRejectThunderHandle's counterpart for the
// caller-supplied URL path: a matching re-registration is a no-op; a
// different one is rejected, regardless of which path originally minted the
// existing record.
func (s *environmentService) reuseOrRejectThunderURL(existing models.ThunderURLRecord, requested, ouID, envName string) (models.ThunderURLRecord, error) {
	if requested == existing.URL {
		s.logger.Info("Reusing already-registered env-thunder url", "ouID", ouID, "envName", envName)
		return existing, nil
	}
	return models.ThunderURLRecord{}, fmt.Errorf("%w: %s/%s already has a registered thunder url %q — call DeleteThunderURL first to change it",
		utils.ErrThunderURLTaken, ouID, envName, existing.URL)
}

// toThunderURLRecord converts a stored row into the public ThunderURLRecord
// shape, dereferencing ThunderHandle (nil for a caller-supplied row) down
// to "" — see EnvThunderURL's doc comment for why the column is a *string.
func toThunderURLRecord(row *models.EnvThunderURL) models.ThunderURLRecord {
	rec := models.ThunderURLRecord{URL: row.ThunderURL}
	if row.ThunderHandle != nil {
		rec.Handle = *row.ThunderHandle
	}
	return rec
}

// claimThunderHandle attempts to insert-claim handle for (ouID, envName),
// computing its origin server-side (see thundersvc.ThunderOriginFromHandle)
// and storing both. If a concurrent request already won the same
// (ouID, envName) race first, it reads back the winning row: a generated
// handle always adopts the winner's value, while an explicit caller-supplied
// handle applies the same reuse-or-reject rule a pre-existing row would get.
func (s *environmentService) claimThunderHandle(ctx context.Context, ouID, envName, handle string, generated bool) (models.ThunderURLRecord, error) {
	thunderURL := thundersvc.ThunderOriginFromHandle(handle)
	rec := &models.EnvThunderURL{OUID: ouID, EnvName: envName, ThunderHandle: &handle, ThunderURL: thunderURL}
	err := s.envThunderURLRepo.Insert(ctx, rec)
	if err == nil {
		if generated {
			s.logger.Info("Generated and stored env-thunder url handle", "ouID", ouID, "envName", envName)
		} else {
			s.logger.Info("Stored env-thunder url handle", "ouID", ouID, "envName", envName)
		}
		return models.ThunderURLRecord{Handle: handle, URL: thunderURL}, nil
	}
	if errors.Is(err, utils.ErrEnvThunderURLAlreadyClaimed) {
		winner, getErr := s.envThunderURLRepo.Get(ctx, ouID, envName)
		if getErr != nil {
			return models.ThunderURLRecord{}, fmt.Errorf("failed to read the winning thunder url for %s/%s after a claim race: %w", ouID, envName, getErr)
		}
		winnerRec := toThunderURLRecord(winner)
		if generated {
			s.logger.Info("Lost a concurrent first-provisioning race; adopting the winning registration", "ouID", ouID, "envName", envName)
			return winnerRec, nil
		}
		return s.reuseOrRejectThunderHandle(winnerRec, handle, ouID, envName)
	}
	if errors.Is(err, utils.ErrThunderHandleTaken) || errors.Is(err, utils.ErrThunderURLTaken) {
		return models.ThunderURLRecord{}, err
	}
	return models.ThunderURLRecord{}, fmt.Errorf("failed to store thunder url for %s/%s: %w", ouID, envName, err)
}

// claimThunderURL attempts to insert-claim rawURL (already validated and
// normalized) for (ouID, envName), with no handle at all — the caller-supplied
// URL registration path. Mirrors claimThunderHandle's concurrency handling,
// one level simpler since there is no generated-value retry loop for this
// path: the caller already knows the exact URL it wants, so there is nothing
// to regenerate on collision.
func (s *environmentService) claimThunderURL(ctx context.Context, ouID, envName, rawURL string) (models.ThunderURLRecord, error) {
	rec := &models.EnvThunderURL{OUID: ouID, EnvName: envName, ThunderURL: rawURL}
	err := s.envThunderURLRepo.Insert(ctx, rec)
	if err == nil {
		s.logger.Info("Stored env-thunder url", "ouID", ouID, "envName", envName)
		return models.ThunderURLRecord{URL: rawURL}, nil
	}
	if errors.Is(err, utils.ErrEnvThunderURLAlreadyClaimed) {
		winner, getErr := s.envThunderURLRepo.Get(ctx, ouID, envName)
		if getErr != nil {
			return models.ThunderURLRecord{}, fmt.Errorf("failed to read the winning thunder url for %s/%s after a claim race: %w", ouID, envName, getErr)
		}
		return s.reuseOrRejectThunderURL(toThunderURLRecord(winner), rawURL, ouID, envName)
	}
	if errors.Is(err, utils.ErrThunderURLTaken) {
		return models.ThunderURLRecord{}, err
	}
	return models.ThunderURLRecord{}, fmt.Errorf("failed to store thunder url for %s/%s: %w", ouID, envName, err)
}

// GetThunderURL returns the registered record for (ouID, envName) — resolved
// via ResolveThunderURL — or utils.ErrThunderHandleNotFound if nothing has
// ever been registered.
func (s *environmentService) GetThunderURL(ctx context.Context, ouID, envName string) (models.ThunderURLRecord, error) {
	if ouID == "" {
		return models.ThunderURLRecord{}, fmt.Errorf("%w: ouID is required", utils.ErrInvalidInput)
	}
	rec, err := ResolveThunderURL(ctx, s.envThunderURLRepo, ouID, envName)
	if err != nil {
		return models.ThunderURLRecord{}, fmt.Errorf("failed to read thunder url for %s/%s: %w", ouID, envName, err)
	}
	if rec.URL == "" {
		return models.ThunderURLRecord{}, utils.ErrThunderHandleNotFound
	}
	return rec, nil
}

// DeleteThunderURL removes the handle for (ouID, envName). Idempotent — deleting a
// non-existent row is not an error. Freeing the handle lets a different environment
// (or a retry with a different handle for this same one) claim it.
func (s *environmentService) DeleteThunderURL(ctx context.Context, ouID, envName string) error {
	if ouID == "" {
		return fmt.Errorf("%w: ouID is required", utils.ErrInvalidInput)
	}
	if err := s.envThunderURLRepo.Delete(ctx, ouID, envName); err != nil {
		return fmt.Errorf("failed to delete thunder url handle for %s/%s: %w", ouID, envName, err)
	}
	s.logger.Info("Deleted env-thunder url handle", "ouID", ouID, "envName", envName)
	return nil
}

// IsThunderHandleAvailable validates handle's format, then checks it against
// every currently-registered handle (not just this caller's own environment —
// the handle is globally unique, see migration042). See the interface doc
// comment for why this is advisory, not authoritative.
func (s *environmentService) IsThunderHandleAvailable(ctx context.Context, handle string) (bool, error) {
	if err := validateThunderHandle(handle); err != nil {
		return false, err
	}
	exists, err := s.envThunderURLRepo.ExistsByHandle(ctx, handle)
	if err != nil {
		return false, fmt.Errorf("failed to check thunder url handle availability for %q: %w", handle, err)
	}
	return !exists, nil
}

// ThunderHandleRegistered reports whether handle is registered to ANY
// environment — the raw existence question Caddy's on-demand-TLS ask endpoint
// needs (see api/thunder_ask_routes.go), not the "is this a well-formed new
// handle" question IsThunderHandleAvailable answers.
func (s *environmentService) ThunderHandleRegistered(ctx context.Context, handle string) (bool, error) {
	exists, err := s.envThunderURLRepo.ExistsByHandle(ctx, handle)
	if err != nil {
		return false, fmt.Errorf("failed to check thunder url handle registration for %q: %w", handle, err)
	}
	return exists, nil
}

// SetThunderSystemClientSecret encrypts and upserts the env-Thunder system-client
// credential, keyed by ouID.
//
// Does NOT provision a thunder-url registration — that's SetThunderURL's job,
// called independently (by add-environment-thunder.sh's register_thunder_url,
// or by a caller's own PUT .../thunder-url). This method previously
// auto-generated a handle here if none was registered yet, to guarantee "a
// credential never exists without a registration." That guarantee doesn't
// hold for the url-registration path: a caller that stores the credential
// before registering its URL would have gotten a bogus handle-based origin
// permanently stamped, since SetThunderURL never overwrites an existing
// registration — the real URL registration would then fail forever with
// ErrThunderURLTaken. Silently fabricating a registration nobody asked for is
// worse than a transient "not provisioned": Resolve() already checks the
// credential and the URL as two independent pieces (see
// EnvThunderResolver.Resolve), so a credential existing without a URL row is
// a valid, correctly-handled state — it just reports ErrThunderNotProvisioned
// until the real registration lands.
func (s *environmentService) SetThunderSystemClientSecret(ctx context.Context, ouID, envName, clientID, clientSecret string) error {
	if clientSecret == "" {
		return fmt.Errorf("%w: clientSecret is required", utils.ErrInvalidInput)
	}
	if ouID == "" {
		return fmt.Errorf("%w: ouID is required", utils.ErrInvalidInput)
	}
	encrypted, err := utils.EncryptBytes([]byte(clientSecret), s.encryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt env-thunder system-client secret for %s/%s: %w", ouID, envName, err)
	}
	cred := &models.EnvThunderSystemClient{
		OUID:                  ouID,
		EnvName:               envName,
		ClientID:              clientID,
		ClientSecretEncrypted: encrypted,
	}
	if err := s.envThunderRepo.Upsert(ctx, cred); err != nil {
		return fmt.Errorf("failed to store env-thunder system-client secret for %s/%s: %w", ouID, envName, err)
	}
	s.logger.Info("Stored env-thunder system-client secret", "ouID", ouID, "envName", envName)
	return nil
}

// DeleteThunderSystemClientSecret removes the credential for (ouID, envName).
// Idempotent — deleting a non-existent row is not an error.
func (s *environmentService) DeleteThunderSystemClientSecret(ctx context.Context, ouID, envName string) error {
	if ouID == "" {
		return fmt.Errorf("%w: ouID is required", utils.ErrInvalidInput)
	}
	if err := s.envThunderRepo.Delete(ctx, ouID, envName); err != nil {
		return fmt.Errorf("failed to delete env-thunder system-client secret for %s/%s: %w", ouID, envName, err)
	}
	s.logger.Info("Deleted env-thunder system-client secret", "ouID", ouID, "envName", envName)
	return nil
}
