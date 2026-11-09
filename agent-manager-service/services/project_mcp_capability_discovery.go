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
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// A project MCP proxy binding whose caller sent no capabilities has them discovered by AMS: once the binding is
// deployed (the MCP kind's AfterLive queues it), the reconciler lists the MCP server's tools, prompts and resources
// through the binding's public gateway URL and stores them on the binding. AMS cannot reach the MCP server itself:
// its upstream is an in-cluster address of the data plane. A binding secured with an API key is probed with a
// short-lived console-managed key, issued for the attempt and revoked after it.

const (
	mcpDiscoveryTickInterval = 10 * time.Second
	mcpDiscoveryBatch        = 20
	mcpDiscoveryBaseBackoff  = 10 * time.Second
	mcpDiscoveryMaxBackoff   = 5 * time.Minute
	// mcpDiscoveryAttemptTimeout bounds one row's attempt: the key's issue, its wait for the gateway, and the probe.
	mcpDiscoveryAttemptTimeout = 90 * time.Second
	// mcpDiscoveryBatchBudget leaves the claim lease a minute to record the last outcome.
	mcpDiscoveryBatchBudget = repositories.MCPCapabilityDiscoveryClaimLease - time.Minute
	// mcpDiscoveryAttemptBudget is about 30 minutes of backoff before the row is failed.
	mcpDiscoveryAttemptBudget = 10
	// mcpDiscoveryKeyWait is how long a probe keeps retrying a 401 while the gateway receives the attempt's key.
	mcpDiscoveryKeyWait = 30 * time.Second
	mcpDiscoveryKeyPoll = 2 * time.Second
	// mcpDiscoveryKeyName is the console-managed key a discovery attempt probes with. It is never listed, and its
	// prefix is one the binding key routes refuse, so no user key can share its name.
	mcpDiscoveryKeyName = models.APIKeyTestKeyPrefix + "capability-discovery"
	// mcpDiscoveryDefaultAPIKeyHeader is the header a binding's API key goes in when its security names none.
	mcpDiscoveryDefaultAPIKeyHeader = "X-API-Key"
)

// errMCPDiscoveryNotNeeded means the binding no longer wants discovery: it is gone, or its caller gave capabilities.
var errMCPDiscoveryNotNeeded = errors.New("capability discovery is not needed for this binding")

// MCPCapabilityProber lists an MCP server's capabilities at url, sending headerValue in headerName when set.
type MCPCapabilityProber func(ctx context.Context, url, headerName, headerValue string) (*models.MCPServerInfoFetchResponse, error)

// ProjectMCPCapabilityDiscoveryService drains the project MCP proxy capability discovery queue.
type ProjectMCPCapabilityDiscoveryService interface {
	Start(ctx context.Context) error
	Stop() error
	// RunOnce drains the currently-due batch once.
	RunOnce(ctx context.Context)
}

type projectMCPCapabilityDiscoveryService struct {
	discoveries repositories.MCPCapabilityDiscoveryRepository
	bindings    *ProjectMCPProxyService
	probe       MCPCapabilityProber
	logger      *slog.Logger
	stopCh      chan struct{}
	stopOnce    sync.Once

	attemptTimeout time.Duration
	batchBudget    time.Duration
	keyWait        time.Duration
	keyPoll        time.Duration
}

// NewProjectMCPCapabilityDiscoveryService creates the discovery reconciler. It probes the gateway with a plain HTTP
// client, not the SSRF-guarded one: the URL is built by AMS from a managed gateway's vhost, never given by a user,
// and resolves to loopback in local setups.
func NewProjectMCPCapabilityDiscoveryService(
	discoveries repositories.MCPCapabilityDiscoveryRepository, bindings *ProjectMCPProxyService, logger *slog.Logger,
) ProjectMCPCapabilityDiscoveryService {
	prober := &MCPProxyService{client: &http.Client{Timeout: mcpRequestTimeout}, logger: logger}
	return newProjectMCPCapabilityDiscoveryService(discoveries, bindings, prober.fetchMCPServerInfo, logger)
}

func newProjectMCPCapabilityDiscoveryService(
	discoveries repositories.MCPCapabilityDiscoveryRepository, bindings *ProjectMCPProxyService,
	probe MCPCapabilityProber, logger *slog.Logger,
) *projectMCPCapabilityDiscoveryService {
	return &projectMCPCapabilityDiscoveryService{
		discoveries: discoveries,
		bindings:    bindings,
		probe:       probe,
		logger:      logger,
		stopCh:      make(chan struct{}),

		attemptTimeout: mcpDiscoveryAttemptTimeout,
		batchBudget:    mcpDiscoveryBatchBudget,
		keyWait:        mcpDiscoveryKeyWait,
		keyPoll:        mcpDiscoveryKeyPoll,
	}
}

func (s *projectMCPCapabilityDiscoveryService) Start(ctx context.Context) error {
	go s.runLoop(ctx)
	s.logger.Info("MCP capability discovery started")
	return nil
}

func (s *projectMCPCapabilityDiscoveryService) Stop() error {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.logger.Info("MCP capability discovery stopped")
	})
	return nil
}

func (s *projectMCPCapabilityDiscoveryService) runLoop(ctx context.Context) {
	ticker := time.NewTicker(mcpDiscoveryTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.RunOnce(ctx)
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *projectMCPCapabilityDiscoveryService) RunOnce(ctx context.Context) {
	claimedAt := time.Now()
	due, err := s.discoveries.ClaimDue(ctx, claimedAt, mcpDiscoveryBatch)
	if err != nil {
		s.logger.Error("Failed to claim due MCP capability discoveries", "error", err)
		return
	}
	drainWithinLease(ctx, s.logger, claimedAt.Add(s.batchBudget), due, "MCP capability discovery", s.discoverOne)
}

// discoverOne runs one row's attempt and records its outcome.
func (s *projectMCPCapabilityDiscoveryService) discoverOne(ctx context.Context, row models.MCPCapabilityDiscovery) {
	attemptCtx, recordCtx, cancel := a2aAttemptContexts(ctx, s.attemptTimeout)
	defer cancel()
	log := s.logger.With("ouID", row.OUID, "artifactUUID", row.ArtifactUUID)

	capabilities, err := s.attempt(attemptCtx, row)
	var permanent *mcpDiscoveryPermanentError
	switch {
	case errors.Is(err, errMCPDiscoveryNotNeeded):
		s.record(log, s.discoveries.Drop(recordCtx, row))
		return
	case errors.As(err, &permanent):
		log.Info("MCP capability discovery cannot succeed for this binding", "reason", permanent.Error())
		s.record(log, s.discoveries.MarkFailed(recordCtx, row, permanent.Error()))
		return
	case err != nil:
		s.recordAttemptFailure(recordCtx, log, row, err)
		return
	}
	err = s.bindings.storeDiscoveredCapabilities(recordCtx, row, capabilities)
	switch {
	case errors.Is(err, errMCPDiscoveryNotNeeded):
		s.record(log, s.discoveries.Drop(recordCtx, row))
	case err != nil && !errors.Is(err, repositories.ErrMCPCapabilityDiscoverySuperseded):
		s.recordAttemptFailure(recordCtx, log, row, fmt.Errorf("failed to store the discovered capabilities: %w", err))
	default:
		s.record(log, err)
		if err == nil {
			log.Info("Discovered the MCP server's capabilities", "tools", capabilityCount(capabilities))
		}
	}
}

// mcpDiscoveryPermanentError is a reason discovery cannot succeed until the binding changes; the row is failed
// without retrying.
type mcpDiscoveryPermanentError struct{ reason string }

func (e *mcpDiscoveryPermanentError) Error() string { return e.reason }

// attempt lists the binding's capabilities through its gateway.
func (s *projectMCPCapabilityDiscoveryService) attempt(
	ctx context.Context, row models.MCPCapabilityDiscovery,
) (*models.MCPProxyCapabilities, error) {
	target, err := s.bindings.discoveryTarget(ctx, row.OUID, row.ArtifactUUID)
	if err != nil {
		return nil, err
	}
	headerName, headerValue := "", ""
	if target.security.RequiresAPIKey() {
		name, in := target.security.APIKeyNameAndLocation(mcpDiscoveryDefaultAPIKeyHeader)
		// The key only ever goes in a header: the gateway's api-key policy checks no other location (see
		// SecurityConfig.ValidateAPIKeyLocation), and a key in the URL would surface in transport errors, which are
		// logged and shown as lastError.
		if in != "header" {
			return nil, &mcpDiscoveryPermanentError{reason: fmt.Sprintf(
				"the binding's API key is read from the %s, which the gateway does not check; use a header", in,
			)}
		}
		value, revoke, err := s.bindings.issueDiscoveryKey(ctx, row.OUID, target.artifact)
		if err != nil {
			return nil, fmt.Errorf("failed to issue the discovery key: %w", err)
		}
		defer revoke()
		headerName, headerValue = name, value
	}
	info, err := s.probeUntilKeyArrives(ctx, target.publicURL, headerName, headerValue)
	if err != nil {
		return nil, fmt.Errorf("failed to list the MCP server's capabilities at %s: %w",
			target.publicURL, redactSecret(err, headerValue))
	}
	capabilities, err := normalizeBindingCapabilities(&models.MCPProxyCapabilities{
		Tools: info.Tools, Prompts: info.Prompts, Resources: info.Resources,
	})
	if err != nil {
		return nil, &mcpDiscoveryPermanentError{reason: fmt.Sprintf("the MCP server's capabilities cannot be stored: %v", err)}
	}
	return capabilities, nil
}

// probeUntilKeyArrives probes, retrying a 401 for keyWait: a key issued for the attempt reaches the gateway through
// the event hub, a moment after it is stored.
func (s *projectMCPCapabilityDiscoveryService) probeUntilKeyArrives(
	ctx context.Context, probeURL, headerName, headerValue string,
) (*models.MCPServerInfoFetchResponse, error) {
	deadline := time.Now().Add(s.keyWait)
	for {
		info, err := s.probe(ctx, probeURL, headerName, headerValue)
		if err == nil || headerValue == "" || !errors.Is(err, utils.ErrMCPServerUnauthorized) {
			return info, err
		}
		if time.Now().Add(s.keyPoll).After(deadline) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(s.keyPoll):
		}
	}
}

func (s *projectMCPCapabilityDiscoveryService) recordAttemptFailure(
	ctx context.Context, log *slog.Logger, row models.MCPCapabilityDiscovery, cause error,
) {
	if row.AttemptCount+1 >= mcpDiscoveryAttemptBudget {
		log.Warn("MCP capabilities could not be discovered within the attempt budget", "error", cause)
		s.record(log, s.discoveries.MarkFailed(ctx, row, cause.Error()))
		return
	}
	log.Debug("MCP capabilities not discoverable yet, will retry", "attempt", row.AttemptCount+1, "reason", cause)
	next := time.Now().Add(utils.ExponentialBackoff(row.AttemptCount, mcpDiscoveryBaseBackoff, mcpDiscoveryMaxBackoff))
	s.record(log, s.discoveries.MarkAttemptFailed(ctx, row, cause.Error(), next))
}

// record logs a failure to record an outcome. A row queued again meanwhile is left for its next claim.
func (s *projectMCPCapabilityDiscoveryService) record(log *slog.Logger, err error) {
	switch {
	case errors.Is(err, repositories.ErrMCPCapabilityDiscoverySuperseded):
		log.Info("MCP capability discovery was queued again during the attempt; leaving it for the next claim")
	case err != nil:
		log.Error("Failed to record the MCP capability discovery outcome", "error", err)
	}
}

func capabilityCount(c *models.MCPProxyCapabilities) int {
	if c == nil || c.Tools == nil {
		return 0
	}
	return len(*c.Tools)
}

// redactedError is an error whose text has a secret replaced; errors.Is and errors.As still see the original.
type redactedError struct {
	err  error
	text string
}

func (e *redactedError) Error() string { return e.text }
func (e *redactedError) Unwrap() error { return e.err }

// redactSecret hides secret in err's text, so an error that echoes a credential (a server repeating a header back,
// say) is never logged or stored as lastError with it.
func redactSecret(err error, secret string) error {
	if err == nil || secret == "" || !strings.Contains(err.Error(), secret) {
		return err
	}
	return &redactedError{err: err, text: strings.ReplaceAll(err.Error(), secret, "[REDACTED]")}
}

// mcpDiscoveryTarget is what a discovery attempt needs of a binding.
type mcpDiscoveryTarget struct {
	artifact  string
	publicURL string
	security  *models.SecurityConfig
}

// bindingByArtifact finds the project proxy binding whose gateway artifact is artifactUUID. errMCPDiscoveryNotNeeded
// when there is none, or it belongs to an org proxy.
func (s *ProjectMCPProxyService) bindingByArtifact(
	ctx context.Context, ouID string, artifactUUID uuid.UUID,
) (*models.MCPProxy, *models.MCPProxyEndpoint, *models.MCPProxyEndpointEnvironment, error) {
	ee, err := s.base.endpointRepo.GetEndpointEnvByArtifact(ctx, artifactUUID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil, errMCPDiscoveryNotNeeded
	}
	if err != nil {
		return nil, nil, nil, err
	}
	proxy, err := s.base.repo.GetByUUID(ctx, ee.MCPProxyUUID.String(), ouID)
	if errors.Is(err, utils.ErrMCPProxyNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil, errMCPDiscoveryNotNeeded
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read the MCP proxy: %w", err)
	}
	if proxy.ProjectUUID == nil {
		return nil, nil, nil, errMCPDiscoveryNotNeeded
	}
	for i := range proxy.Endpoints {
		if proxy.Endpoints[i].UUID == ee.EndpointUUID {
			return proxy, &proxy.Endpoints[i], ee, nil
		}
	}
	return nil, nil, nil, errMCPDiscoveryNotNeeded
}

// callerGaveCapabilities reports a binding whose capabilities came from its caller, which discovery leaves alone.
func callerGaveCapabilities(cfg models.MCPEndpointConfig) bool {
	return cfg.Capabilities != nil && !cfg.CapabilitiesDiscovered
}

// discoveryTarget is the deployed binding's public URL and security. A binding not deployed yet is an attempt
// failure, retried; a JWT-only binding cannot be probed and is a permanent one.
func (s *ProjectMCPProxyService) discoveryTarget(ctx context.Context, ouID string, artifactUUID uuid.UUID) (*mcpDiscoveryTarget, error) {
	proxy, endpoint, ee, err := s.bindingByArtifact(ctx, ouID, artifactUUID)
	if err != nil {
		return nil, err
	}
	if callerGaveCapabilities(endpoint.Configuration) {
		return nil, errMCPDiscoveryNotNeeded
	}
	resp, err := s.environmentResponse(ctx, ouID, proxy, endpoint, ee)
	if err != nil {
		return nil, err
	}
	if resp.DeploymentStatus != models.MCPDeploymentStatusDeployed {
		status := resp.DeploymentStatus
		if status == "" {
			status = resp.DeploymentStatusReason
		}
		return nil, fmt.Errorf("the binding is not deployed (%s)", status)
	}
	if resp.PublicURL == "" {
		return nil, fmt.Errorf("the binding's gateway has no public URL")
	}
	security := endpoint.Configuration.Security
	if security != nil && (security.Enabled == nil || *security.Enabled) && !security.RequiresAPIKey() {
		return nil, &mcpDiscoveryPermanentError{
			reason: "the binding is secured with a scheme AMS cannot authenticate to; send capabilities with the binding",
		}
	}
	return &mcpDiscoveryTarget{artifact: ee.ArtifactUUID.String(), publicURL: resp.PublicURL, security: security}, nil
}

// issueDiscoveryKey issues the console-managed key a discovery attempt probes with, sent to the organization's
// gateways like the binding's other keys. revoke removes it; it is called once the attempt is done, whatever its
// outcome, with a context of its own so a timed-out attempt still revokes.
func (s *ProjectMCPProxyService) issueDiscoveryKey(ctx context.Context, ouID, artifact string) (string, func(), error) {
	gateways, err := s.organizationGateways(ctx, ouID)
	if err != nil {
		return "", nil, err
	}
	expiresAt := time.Now().UTC().Add(mcpDiscoveryAttemptTimeout + time.Minute).Format(time.RFC3339)
	resp, err := s.base.apiKeyBroadcaster.broadcastCreateToGateways(gateways, ouID, artifact, artifact, &models.CreateAPIKeyRequest{
		Name:        mcpDiscoveryKeyName,
		DisplayName: "Capability discovery",
		Purpose:     models.APIKeyPurposeConsoleManaged,
		ExpiresAt:   &expiresAt,
	})
	if err != nil {
		return "", nil, err
	}
	revoke := func() {
		revokeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if err := s.base.apiKeyBroadcaster.broadcastRevokeToGateways(revokeCtx, gateways, artifact, artifact, mcpDiscoveryKeyName); err != nil {
			s.base.logger.Warn("Failed to revoke the MCP capability discovery key; it expires on its own",
				"ouID", ouID, "artifactUUID", artifact, "error", err)
		}
	}
	return resp.APIKey, revoke, nil
}

// storeDiscoveredCapabilities writes discovered capabilities onto the binding and marks its discovery discovered. The
// proxy row is locked, so a binding PUT cannot interleave, and the write is one statement conditional on the claim's
// token (StoreDiscovered), so a discovery queued again since the claim, which does not take the proxy lock, makes it
// a no-op (ErrMCPCapabilityDiscoverySuperseded) instead of being overwritten. A binding whose caller has since given
// capabilities, or that is gone, is left alone (errMCPDiscoveryNotNeeded). The binding is not redeployed: the
// gateway does not carry capabilities.
func (s *ProjectMCPProxyService) storeDiscoveredCapabilities(
	ctx context.Context, row models.MCPCapabilityDiscovery, capabilities *models.MCPProxyCapabilities,
) error {
	proxy, _, _, err := s.bindingByArtifact(ctx, row.OUID, row.ArtifactUUID)
	if err != nil {
		return err
	}
	return s.base.tx(ctx, func(tx *gorm.DB) error {
		locked, err := s.base.repo.GetByHandleForUpdate(ctx, tx, proxyHandleOf(proxy), row.OUID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errMCPDiscoveryNotNeeded
		}
		if err != nil {
			return fmt.Errorf("failed to lock the MCP proxy: %w", err)
		}
		var endpoint *models.MCPProxyEndpoint
		for i := range locked.Endpoints {
			for _, env := range locked.Endpoints[i].Environments {
				if env.ArtifactUUID == row.ArtifactUUID {
					endpoint = &locked.Endpoints[i]
				}
			}
		}
		if endpoint == nil || callerGaveCapabilities(endpoint.Configuration) {
			return errMCPDiscoveryNotNeeded
		}
		return s.discoveries.WithTx(tx).StoreDiscovered(ctx, row, capabilities, time.Now())
	})
}
