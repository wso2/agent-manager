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

package gwsvc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

const (
	apiVersionLLMProxy = "gateway.api-platform.wso2.com/v1"
	kindLLMProxy       = "LlmProxy"
)

// LLMProxyDeploymentService handles LLM proxy deployment business logic
type LLMProxyDeploymentService struct {
	tx                   TxRunner
	deploymentRepo       gwrepo.DeploymentRepository
	proxyRepo            gwrepo.LLMProxyRepository
	providerRepo         gwrepo.LLMProviderRepository
	gatewayRepo          gwrepo.GatewayRepository
	gatewayEventsService *GatewayEventsService
	core                 *reconcile.Core
	kind                 *LLMProxyKind
}

// NewLLMProxyDeploymentService creates a new LLM proxy deployment service
func NewLLMProxyDeploymentService(
	tx TxRunner,
	deploymentRepo gwrepo.DeploymentRepository,
	proxyRepo gwrepo.LLMProxyRepository,
	providerRepo gwrepo.LLMProviderRepository,
	gatewayRepo gwrepo.GatewayRepository,
	gatewayEventsService *GatewayEventsService,
	core *reconcile.Core,
	kind *LLMProxyKind,
) *LLMProxyDeploymentService {
	return &LLMProxyDeploymentService{
		core:                 core,
		kind:                 kind,
		tx:                   tx,
		deploymentRepo:       deploymentRepo,
		proxyRepo:            proxyRepo,
		providerRepo:         providerRepo,
		gatewayRepo:          gatewayRepo,
		gatewayEventsService: gatewayEventsService,
	}
}

// LLMProxyDeploymentYAML represents the deployment YAML
type LLMProxyDeploymentYAML struct {
	ApiVersion string                 `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                 `yaml:"kind" json:"kind"`
	Metadata   DeploymentMetadata     `yaml:"metadata" json:"metadata"`
	Spec       LLMProxyDeploymentSpec `yaml:"spec" json:"spec"`
}

// LLMProxyDeploymentSpec represents the spec section
type LLMProxyDeploymentSpec struct {
	DisplayName string                     `yaml:"displayName" json:"displayName"`
	Version     string                     `yaml:"version" json:"version"`
	Context     string                     `yaml:"context,omitempty" json:"context,omitempty"`
	VHost       string                     `yaml:"vhost,omitempty" json:"vhost,omitempty"`
	Provider    LLMProxyDeploymentProvider `yaml:"provider" json:"provider"`
	Resilience  *gwmodels.Resilience       `yaml:"resilience,omitempty" json:"resilience,omitempty"`
	Policies    []gwmodels.LLMPolicy       `yaml:"policies,omitempty" json:"policies,omitempty"`
	Security    *gwmodels.SecurityConfig   `yaml:"security,omitempty" json:"security,omitempty"`
}

// LLMProxyDeploymentProvider represents the provider configuration in the spec
type LLMProxyDeploymentProvider struct {
	ID   string                 `yaml:"id" json:"id"`
	Auth *gwmodels.UpstreamAuth `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// DeployLLMProxy deploys an LLM proxy to a gateway
func (s *LLMProxyDeploymentService) DeployLLMProxy(proxyID string, req *gwmodels.DeployAPIRequest, ouID string) (*gwmodels.Deployment, error) {
	slog.Info("LLMProxyDeploymentService.DeployLLMProxy: starting", "proxyID", proxyID, "ouID", ouID,
		"base", req.Base, "gatewayID", req.GatewayID)

	if err := validateDeployBase(req.Base); err != nil {
		return nil, err
	}
	if req.GatewayID == "" {
		slog.Error("LLMProxyDeploymentService.DeployLLMProxy: gateway ID is required", "proxyID", proxyID)
		return nil, gwerr.ErrDeploymentGatewayIDRequired
	}

	// Parse UUIDs
	gatewayUUID, err := uuid.Parse(req.GatewayID)
	if err != nil {
		slog.Error("LLMProxyDeploymentService.DeployLLMProxy: invalid gateway UUID", "proxyID", proxyID, "gatewayID", req.GatewayID, "error", err)
		return nil, fmt.Errorf("invalid gateway UUID: %w", err)
	}

	// Validate gateway exists
	slog.Info("LLMProxyDeploymentService.DeployLLMProxy: validating gateway", "proxyID", proxyID, "gatewayID", req.GatewayID)
	gateway, err := normalizeGatewayLookup(s.gatewayRepo.GetByUUID(req.GatewayID))
	if err != nil && !errors.Is(err, gwerr.ErrGatewayNotFound) {
		slog.Error("LLMProxyDeploymentService.DeployLLMProxy: failed to get gateway", "proxyID", proxyID, "gatewayID", req.GatewayID, "error", err)
		return nil, err
	}
	if gateway == nil || gateway.OUID != ouID {
		slog.Warn("LLMProxyDeploymentService.DeployLLMProxy: gateway not found or org mismatch", "proxyID", proxyID, "gatewayID", req.GatewayID, "ouID", ouID)
		return nil, gwerr.ErrGatewayNotFound
	}

	// Get LLM proxy
	slog.Info("LLMProxyDeploymentService.DeployLLMProxy: getting proxy", "proxyID", proxyID, "ouID", ouID)
	proxy, err := s.proxyRepo.GetByID(proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProxyDeploymentService.DeployLLMProxy: proxy not found", "proxyID", proxyID, "ouID", ouID)
			return nil, gwerr.ErrLLMProxyNotFound
		}
		slog.Error("LLMProxyDeploymentService.DeployLLMProxy: failed to get proxy", "proxyID", proxyID, "ouID", ouID, "error", err)
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}
	if proxy == nil {
		slog.Warn("LLMProxyDeploymentService.DeployLLMProxy: proxy not found", "proxyID", proxyID, "ouID", ouID)
		return nil, gwerr.ErrLLMProxyNotFound
	}

	slog.Info("LLMProxyDeploymentService.DeployLLMProxy: proxy retrieved", "proxyID", proxyID, "proxyUUID", proxy.UUID)

	ctx := context.Background()
	existing, err := s.deploymentRepo.PlacedGateways(ctx, ouID, proxy.UUID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the gateways the proxy is placed on: %w", err)
	}
	if err := validateEgressPlacement(s.gatewayRepo, gateway, existing); err != nil {
		return nil, fmt.Errorf("%w: %w", httpx.ErrInvalidInput, err)
	}

	var applied *reconcile.Applied
	err = s.tx(ctx, func(tx *gorm.DB) error {
		applied, err = s.core.Deploy(ctx, tx, proxyPlacement(proxy, proxyID, ouID, gatewayUUID))
		return err
	})
	if err != nil {
		slog.Error("LLMProxyDeploymentService.DeployLLMProxy: failed to deploy", "proxyID", proxyID, "error", err)
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	slog.Info("LLMProxyDeploymentService.DeployLLMProxy: deployment started", "proxyID", proxyID, "deploymentID", applied.DeploymentID)
	return applied.Deployment, nil
}

// DeployForEnvironment deploys an agent's or monitor's proxy, whose per-environment record the caller has saved,
// on gateway, the environment's egress gateway the caller resolved. A render error is returned before anything is
// written.
func (s *LLMProxyDeploymentService) DeployForEnvironment(
	ctx context.Context, proxy *gwmodels.LLMProxy, ouID string, envUUID, gateway uuid.UUID,
) (*gwmodels.Deployment, error) {
	record := reconcile.Record{
		ArtifactRef:      proxyPlacement(proxy, proxy.Handle, ouID, gateway).ArtifactRef,
		EnvironmentUUID:  envUUID,
		RequestedGateway: &gateway,
	}
	var applied *reconcile.Applied
	err := s.tx(ctx, func(tx *gorm.DB) error {
		var err error
		applied, err = s.core.DeployRecord(ctx, tx, s.kind, record)
		return err
	})
	if err != nil {
		return nil, err
	}
	if applied.NoGateway {
		return nil, fmt.Errorf("gateway %s is no longer mapped to environment %s", gateway, envUUID)
	}
	return applied.Deployment, nil
}

// CurrentDeployment is the proxy's deployment the gateway is to serve; nil when none is.
func (s *LLMProxyDeploymentService) CurrentDeployment(
	ctx context.Context, proxy *gwmodels.LLMProxy, ouID string, gateway uuid.UUID,
) (*gwmodels.Deployment, error) {
	return s.deploymentRepo.GetPullable(ctx, ouID, proxy.UUID, gateway.String())
}

// Redeploy re-renders the proxy on gateway because an input outside it changed. An invalid configuration is
// returned before anything is written; a render that fails after the redeploy is queued is a
// reconcile.RenderFailedError, with the redeploy left to the worker. The new deployment is nil when the proxy isn't
// placed on gateway.
func (s *LLMProxyDeploymentService) Redeploy(
	ctx context.Context, proxy *gwmodels.LLMProxy, ouID string, gateway uuid.UUID, reason string,
) (*gwmodels.Deployment, error) {
	ref := proxyPlacement(proxy, proxy.Handle, ouID, gateway).ArtifactRef
	if _, err := s.kind.Render(ctx, ref); err != nil {
		return nil, err
	}
	if err := s.core.MarkStale(ctx, gwmodels.KindLLMProxy, ouID, proxy.UUID, []uuid.UUID{gateway}, reason); err != nil {
		return nil, err
	}
	return s.CurrentDeployment(ctx, proxy, ouID, gateway)
}

// RedeployEverywhere queues a re-render on every gateway the proxy is placed on and renders them inline. Failures
// are returned joined: a render failure is a reconcile.RenderFailedError, a DB write-back failure a plain error. A
// gateway that fails keeps what it was last sent and stays owed, and the worker retries it unless the render
// failed permanently.
func (s *LLMProxyDeploymentService) RedeployEverywhere(ctx context.Context, proxy *gwmodels.LLMProxy, ouID, reason string) error {
	return s.core.MarkStale(ctx, gwmodels.KindLLMProxy, ouID, proxy.UUID, nil, reason)
}

// proxyPlacement is the proxy's artifact on the gateway. handle is what the gateway stores the proxy under.
func proxyPlacement(proxy *gwmodels.LLMProxy, handle, ouID string, gatewayUUID uuid.UUID) gwmodels.Placement {
	return gwmodels.Placement{
		ArtifactRef: gwmodels.ArtifactRef{OUID: ouID, ArtifactUUID: proxy.UUID, Kind: gwmodels.KindLLMProxy, Handle: handle},
		GatewayUUID: gatewayUUID,
	}
}

func proxyVhost(proxy *gwmodels.LLMProxy) string {
	if proxy.Configuration.Vhost == nil {
		return ""
	}
	return *proxy.Configuration.Vhost
}

// UndeployLLMProxyDeployment undeploys a deployment
func (s *LLMProxyDeploymentService) UndeployLLMProxyDeployment(ctx context.Context, proxyID, deploymentID, gatewayID, ouID string) (*gwmodels.Deployment, error) {
	slog.Info("LLMProxyDeploymentService.UndeployLLMProxyDeployment: starting", "proxyID", proxyID,
		"deploymentID", deploymentID, "gatewayID", gatewayID, "ouID", ouID)

	// Get proxy
	slog.Info("LLMProxyDeploymentService.UndeployLLMProxyDeployment: getting proxy", "proxyID", proxyID, "ouID", ouID)
	proxy, err := s.proxyRepo.GetByIDCtx(ctx, proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProxyDeploymentService.UndeployLLMProxyDeployment: proxy not found", "proxyID", proxyID)
			return nil, gwerr.ErrLLMProxyNotFound
		}
		slog.Error("LLMProxyDeploymentService.UndeployLLMProxyDeployment: failed to get proxy", "proxyID", proxyID, "error", err)
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}
	if proxy == nil {
		slog.Warn("LLMProxyDeploymentService.UndeployLLMProxyDeployment: proxy not found", "proxyID", proxyID)
		return nil, gwerr.ErrLLMProxyNotFound
	}

	// Get deployment
	slog.Info("LLMProxyDeploymentService.UndeployLLMProxyDeployment: getting deployment", "proxyID", proxyID, "deploymentID", deploymentID)
	deployment, err := s.deploymentRepo.GetWithStateCtx(ctx, deploymentID, proxy.UUID.String(), ouID)
	if err != nil {
		slog.Error("LLMProxyDeploymentService.UndeployLLMProxyDeployment: failed to get deployment", "proxyID", proxyID, "deploymentID", deploymentID, "error", err)
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		slog.Warn("LLMProxyDeploymentService.UndeployLLMProxyDeployment: deployment not found", "proxyID", proxyID, "deploymentID", deploymentID)
		return nil, gwerr.ErrDeploymentNotFound
	}
	if deployment.GatewayUUID.String() != gatewayID {
		slog.Error("LLMProxyDeploymentService.UndeployLLMProxyDeployment: gateway ID mismatch", "proxyID", proxyID,
			"deploymentID", deploymentID, "expectedGatewayID", gatewayID, "actualGatewayID", deployment.GatewayUUID.String())
		return nil, gwerr.ErrGatewayIDMismatch
	}
	if deployment.Status == nil || *deployment.Status == gwmodels.DeploymentStatusArchived {
		slog.Warn("LLMProxyDeploymentService.UndeployLLMProxyDeployment: deployment not current", "proxyID", proxyID,
			"deploymentID", deploymentID, "status", deployment.Status)
		return nil, gwerr.ErrDeploymentNotActive
	}

	placement := proxyPlacement(proxy, proxyID, ouID, deployment.GatewayUUID)
	row, err := undeployAndRead(ctx, s.tx, s.core, s.deploymentRepo, placement)
	if errors.Is(err, gwerr.ErrDeploymentNotActive) {
		slog.Warn("LLMProxyDeploymentService.UndeployLLMProxyDeployment: deployment not active", "proxyID", proxyID,
			"deploymentID", deploymentID, "status", deployment.Status)
		return nil, err
	}
	if err != nil {
		slog.Error("LLMProxyDeploymentService.UndeployLLMProxyDeployment: failed to undeploy", "proxyID", proxyID, "deploymentID", deploymentID, "error", err)
		return nil, fmt.Errorf("failed to undeploy: %w", err)
	}

	deployment.SetStatusRow(row)
	deployment.UpdatedAt = &row.PerformedAt
	slog.Info("LLMProxyDeploymentService.UndeployLLMProxyDeployment: undeploy started", "proxyID", proxyID, "deploymentID", deploymentID)
	return deployment, nil
}

// RestoreLLMProxyDeployment restores a previous deployment
func (s *LLMProxyDeploymentService) RestoreLLMProxyDeployment(proxyID, deploymentID, gatewayID, ouID string) (*gwmodels.Deployment, error) {
	// Get proxy
	proxy, err := s.proxyRepo.GetByID(proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gwerr.ErrLLMProxyNotFound
		}
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}
	if proxy == nil {
		return nil, gwerr.ErrLLMProxyNotFound
	}

	deployment, err := getRestorableDeployment(s.deploymentRepo, deploymentID, proxy.UUID.String(), ouID)
	if err != nil {
		return nil, err
	}
	if deployment.GatewayUUID.String() != gatewayID {
		return nil, gwerr.ErrGatewayIDMismatch
	}

	ctx := context.Background()
	placement := proxyPlacement(proxy, proxyID, ouID, deployment.GatewayUUID)
	current, err := s.deploymentRepo.GetStatusRecord(ctx, placement)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment status: %w", err)
	}
	if restoresItself(current, deployment.DeploymentID) {
		return nil, gwerr.ErrDeploymentAlreadyDeployed
	}

	row, err := restoreAndRead(ctx, s.tx, s.core, s.deploymentRepo, placement, deployment.DeploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to restore deployment: %w", err)
	}

	deployment.SetStatusRow(row)
	deployment.UpdatedAt = &row.PerformedAt
	return deployment, nil
}

// PlacedGateways returns the gateways the artifact (a provider or a proxy) is placed on, whatever their status.
func (s *LLMProxyDeploymentService) PlacedGateways(ctx context.Context, ouID string, artifactUUID uuid.UUID) ([]string, error) {
	return s.deploymentRepo.PlacedGateways(ctx, ouID, artifactUUID)
}

// PlacedGatewaysOfProxy returns the gateways the proxy is placed on, whatever their status.
func (s *LLMProxyDeploymentService) PlacedGatewaysOfProxy(ctx context.Context, proxyHandle, ouID string) ([]string, error) {
	proxy, err := s.proxyByID(proxyHandle, ouID)
	if err != nil {
		return nil, err
	}
	return s.deploymentRepo.PlacedGateways(ctx, ouID, proxy.UUID)
}

// UndeployEverywhere undeploys the proxy from every gateway it is placed on, whatever the placement's status, in one
// transaction. gwerr.ErrLLMProxyNotFound when the proxy is gone.
func (s *LLMProxyDeploymentService) UndeployEverywhere(ctx context.Context, proxyHandle, ouID string) error {
	proxy, err := s.proxyByID(proxyHandle, ouID)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		gateways, err := s.deploymentRepo.WithTx(tx).PlacedGateways(ctx, ouID, proxy.UUID)
		if err != nil {
			return err
		}
		for _, gatewayID := range gateways {
			gatewayUUID, err := uuid.Parse(gatewayID)
			if err != nil {
				return fmt.Errorf("invalid gateway UUID %q: %w", gatewayID, err)
			}
			if err := s.core.Undeploy(ctx, tx, proxyPlacement(proxy, proxy.Handle, ouID, gatewayUUID)); err != nil {
				return fmt.Errorf("failed to undeploy from gateway %s: %w", gatewayID, err)
			}
		}
		return nil
	})
}

// RequireUserManaged refuses a proxy AMS created for an agent's LLM configuration or a monitor. The public routes
// call it; AMS's own lifecycle calls the service functions behind them directly. A missing proxy passes, so the
// caller reports it as it would anyway.
func (s *LLMProxyDeploymentService) RequireUserManaged(ctx context.Context, proxyHandle, ouID string) error {
	owned, err := s.proxyRepo.IsAgentOrMonitorProxy(ctx, proxyHandle, ouID)
	if err != nil {
		return fmt.Errorf("check LLM proxy ownership: %w", err)
	}
	if owned {
		return gwerr.ErrLLMProxyManagedByAMS
	}
	return nil
}

// proxyByID is the proxy with the id or handle; gwerr.ErrLLMProxyNotFound when there is none.
func (s *LLMProxyDeploymentService) proxyByID(proxyID, ouID string) (*gwmodels.LLMProxy, error) {
	proxy, err := s.proxyRepo.GetByID(proxyID, ouID)
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && proxy == nil) {
		return nil, gwerr.ErrLLMProxyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}
	return proxy, nil
}

// GetLLMProxyDeployments retrieves all deployments for a proxy
func (s *LLMProxyDeploymentService) GetLLMProxyDeployments(proxyID, ouID string, gatewayID *string, status *string) ([]*gwmodels.Deployment, error) {
	// Get proxy
	proxy, err := s.proxyRepo.GetByID(proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gwerr.ErrLLMProxyNotFound
		}
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}
	if proxy == nil {
		return nil, gwerr.ErrLLMProxyNotFound
	}

	if status != nil && !gwmodels.IsDeploymentStatusFilter(*status) {
		return nil, gwerr.ErrInvalidDeploymentStatus
	}

	// Get deployments
	deployments, err := s.deploymentRepo.GetDeploymentsWithState(proxy.UUID.String(), ouID, gatewayID, status, maxDeploymentsPerAPI)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployments: %w", err)
	}

	return deployments, nil
}

// GetLLMProxyDeployment retrieves a specific deployment
func (s *LLMProxyDeploymentService) GetLLMProxyDeployment(proxyID, deploymentID, ouID string) (*gwmodels.Deployment, error) {
	// Get proxy
	proxy, err := s.proxyRepo.GetByID(proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gwerr.ErrLLMProxyNotFound
		}
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}
	if proxy == nil {
		return nil, gwerr.ErrLLMProxyNotFound
	}

	// Get deployment
	deployment, err := s.deploymentRepo.GetWithState(deploymentID, proxy.UUID.String(), ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return nil, gwerr.ErrDeploymentNotFound
	}

	return deployment, nil
}

// DeleteLLMProxyDeployment deletes a deployment
func (s *LLMProxyDeploymentService) DeleteLLMProxyDeployment(proxyID, deploymentID, ouID string) error {
	// Get proxy
	proxy, err := s.proxyRepo.GetByID(proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return gwerr.ErrLLMProxyNotFound
		}
		return fmt.Errorf("failed to get proxy: %w", err)
	}
	if proxy == nil {
		return gwerr.ErrLLMProxyNotFound
	}

	// Get deployment
	deployment, err := s.deploymentRepo.GetWithState(deploymentID, proxy.UUID.String(), ouID)
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return gwerr.ErrDeploymentNotFound
	}
	// The gateway is to serve the current deployment, or may still hold it; deleting it would drop its status row
	// and orphan it there.
	if deployment.Current() {
		return gwerr.ErrDeploymentIsDeployed
	}

	// Delete deployment
	if err := s.deploymentRepo.Delete(deploymentID, proxy.UUID.String(), ouID); err != nil {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	return nil
}

// generateLLMProxyDeploymentYAML generates deployment YAML for an LLM proxy
func (s *LLMProxyDeploymentService) generateLLMProxyDeploymentYAML(proxy *gwmodels.LLMProxy, ouID string) (string, error) {
	if proxy == nil {
		return "", errors.New("proxy is required")
	}
	if proxy.Configuration.Provider == "" {
		return "", httpx.ErrInvalidInput
	}

	// Get provider to validate it exists
	provider, err := s.providerRepo.GetByUUID(proxy.Configuration.Provider, ouID)
	if err != nil {
		return "", fmt.Errorf("failed to get provider: %w", err)
	}
	return renderLLMProxy(proxy, provider, proxy.Configuration.Annotations)
}

// Annotations on an agent's hidden LLM proxy naming its project and agent. The gateway
// makes the project-id the route's project (as it does for every artifact) and passes the
// agent-id to the budget system policy, which matches project and agent budget consumers
// by them.
const (
	AnnotationProjectID = "gateway.api-platform.wso2.com/project-id"
	AnnotationAgentID   = "gateway.api-platform.wso2.com/agent-id"
)

// AnnotationOrgID names the organization that owns a budget, by handle. Every budget
// carries it; one sent without it gets the organization in the path.
const AnnotationOrgID = "gateway.api-platform.wso2.com/org-id"

// renderLLMProxy renders an LLM proxy's deployment; annotations, when given, are set on
// its metadata.
func renderLLMProxy(proxy *gwmodels.LLMProxy, provider *gwmodels.LLMProvider, annotations map[string]string) (string, error) {
	if provider == nil {
		return "", gwerr.ErrLLMProviderNotFound
	}

	// Set default context if not provided
	contextValue := "/"
	if proxy.Configuration.Context != nil && *proxy.Configuration.Context != "" {
		contextValue = *proxy.Configuration.Context
	}

	vhostValue := ""
	if proxy.Configuration.Vhost != nil {
		vhostValue = *proxy.Configuration.Vhost
	}

	// Initialize policies slice
	policies := make([]gwmodels.LLMPolicy, 0, len(proxy.Configuration.Policies))

	// Transform security config to policy if enabled
	security := proxy.Configuration.Security
	if security != nil && isBoolTrue(security.Enabled) {
		if security.APIKey != nil && isBoolTrue(security.APIKey.Enabled) {
			key := strings.TrimSpace(security.APIKey.Key)
			if key == "" {
				return "", fmt.Errorf("invalid api key security configuration: key is required")
			}

			in := strings.ToLower(strings.TrimSpace(security.APIKey.In))
			if in != "header" && in != "query" {
				return "", fmt.Errorf("invalid api key security configuration: in must be 'header' or 'query', got %q", security.APIKey.In)
			}

			// Add API key auth as a policy
			addOrAppendPolicyPath(&policies, apiKeyAuthPolicyName, apiKeyAuthPolicyVersion, gwmodels.LLMPolicyPath{
				Path:    "/*",
				Methods: []string{"*"},
				Params: map[string]interface{}{
					"key": key,
					"in":  in,
				},
			})
		}
	}

	// Process and normalize policies
	for _, p := range proxy.Configuration.Policies {
		// Deep copy paths
		paths := make([]gwmodels.LLMPolicyPath, 0, len(p.Paths))
		for _, pp := range p.Paths {
			paths = append(paths, gwmodels.LLMPolicyPath{
				Path:    pp.Path,
				Methods: pp.Methods,
				Params:  pp.Params,
			})
		}
		// Add policy with normalized version
		policies = append(policies, gwmodels.LLMPolicy{
			Name:    p.Name,
			Version: normalizePolicyVersionToMajor(p.Version),
			Paths:   paths,
		})
	}

	// Build provider reference
	providerRef := LLMProxyDeploymentProvider{
		ID: provider.Artifact.Handle,
	}

	// Add upstream auth if configured
	if proxy.Configuration.UpstreamAuth != nil {
		providerRef.Auth = proxy.Configuration.UpstreamAuth
	}

	// Build deployment YAML
	deploymentYAML := LLMProxyDeploymentYAML{
		ApiVersion: apiVersionLLMProxy,
		Kind:       kindLLMProxy,
		Metadata: DeploymentMetadata{
			Name:        proxy.Handle, // Use handle (artifact identifier) for metadata.name
			Annotations: annotations,
		},
		Spec: LLMProxyDeploymentSpec{
			DisplayName: proxy.Configuration.Name,
			Version:     proxy.Configuration.Version,
			Context:     contextValue,
			VHost:       vhostValue,
			Provider:    providerRef,
			Resilience:  proxy.Configuration.Resilience,
			Policies:    policies,
		},
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(deploymentYAML)
	if err != nil {
		return "", fmt.Errorf("failed to marshal to YAML: %w", err)
	}

	return string(yamlBytes), nil
}
