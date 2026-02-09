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

package onpremise

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/gateway"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

const (
	defaultTimeout     = 30 * time.Second
	healthCheckTimeout = 5 * time.Second
)

// OnPremiseAdapter implements IGatewayAdapter for on-premise deployments
type OnPremiseAdapter struct {
	httpClient    *http.Client
	gatewayClient clients.GatewayControllerClient
	db            *gorm.DB
	encryptionKey []byte
	config        gateway.AdapterConfig
	logger        *slog.Logger
}

// NewOnPremiseAdapter creates a new on-premise adapter instance
func NewOnPremiseAdapter(config gateway.AdapterConfig, db *gorm.DB, encryptionKey []byte, logger *slog.Logger) (gateway.IGatewayAdapter, error) {
	timeout := defaultTimeout
	if params, ok := config.Parameters["defaultTimeout"].(time.Duration); ok {
		timeout = params
	}

	adapter := &OnPremiseAdapter{
		config:        config,
		db:            db,
		encryptionKey: encryptionKey,
		logger:        logger,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		gatewayClient: clients.NewGatewayControllerClient(),
	}

	return adapter, nil
}

// GetAdapterType returns the adapter type identifier
func (a *OnPremiseAdapter) GetAdapterType() string {
	return "on-premise"
}

// Close cleans up adapter resources
func (a *OnPremiseAdapter) Close() error {
	return nil
}

// ValidateGatewayEndpoint checks if a gateway endpoint is reachable
func (a *OnPremiseAdapter) ValidateGatewayEndpoint(ctx context.Context, controlPlaneURL string) error {
	healthURL := fmt.Sprintf("%s/health", controlPlaneURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gateway endpoint unreachable: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway health check failed with status %d", resp.StatusCode)
	}

	return nil
}

// CheckHealth performs a health check on a gateway
func (a *OnPremiseAdapter) CheckHealth(ctx context.Context, controlPlaneURL string) (*gateway.HealthStatus, error) {
	start := time.Now()

	err := a.ValidateGatewayEndpoint(ctx, controlPlaneURL)
	responseTime := time.Since(start)

	status := &gateway.HealthStatus{
		Status:       "ACTIVE",
		ResponseTime: responseTime,
		CheckedAt:    time.Now(),
	}

	if err != nil {
		status.Status = "ERROR"
		status.ErrorMessage = err.Error()
	}

	return status, nil
}

// ========================================================================
// LLM Provider Management (Phase 7)
// ========================================================================

// getGatewayWithCredentials retrieves gateway data with decrypted credentials
func (a *OnPremiseAdapter) getGatewayWithCredentials(ctx context.Context, gatewayID string) (*models.Gateway, *models.GatewayCredentials, error) {
	gatewayUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid gateway ID: %w", err)
	}

	var gw models.Gateway
	if err := a.db.WithContext(ctx).Where("uuid = ?", gatewayUUID).First(&gw).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, utils.ErrGatewayNotFound
		}
		return nil, nil, fmt.Errorf("failed to query gateway: %w", err)
	}

	// Decrypt credentials
	if len(gw.CredentialsEncrypted) == 0 {
		return nil, nil, fmt.Errorf("gateway has no credentials stored")
	}

	creds, err := utils.DecryptCredentials(gw.CredentialsEncrypted, a.encryptionKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decrypt credentials: %w", err)
	}

	return &gw, creds, nil
}

// DeployProvider deploys an LLM provider configuration to a gateway
func (a *OnPremiseAdapter) DeployProvider(ctx context.Context, gatewayID string, config *gateway.ProviderDeploymentConfig) (*gateway.ProviderDeploymentResult, error) {
	a.logger.Info("Deploying provider to gateway", "gatewayID", gatewayID, "handle", config.Handle)

	gw, creds, err := a.getGatewayWithCredentials(ctx, gatewayID)
	if err != nil {
		return nil, err
	}

	// Extract control plane URL from adapter config
	controlPlaneURL, ok := gw.AdapterConfig["controlPlaneUrl"].(string)
	if !ok {
		return nil, fmt.Errorf("controlPlaneUrl not found in gateway adapter config")
	}

	// Create request credentials
	reqCreds := &clients.RequestCredentials{
		Username: creds.Username,
		Password: creds.Password,
	}

	// Deploy provider via gateway client
	resp, err := a.gatewayClient.CreateLLMProvider(ctx, controlPlaneURL, config.Configuration, reqCreds)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider on gateway: %w", err)
	}

	return &gateway.ProviderDeploymentResult{
		DeploymentID: resp.ID,
		Status:       resp.Status,
		DeployedAt:   time.Now(),
	}, nil
}

// UpdateProvider updates an existing LLM provider on a gateway
func (a *OnPremiseAdapter) UpdateProvider(ctx context.Context, gatewayID string, providerID string, config *gateway.ProviderDeploymentConfig) (*gateway.ProviderDeploymentResult, error) {
	a.logger.Info("Updating provider on gateway", "gatewayID", gatewayID, "providerID", providerID)

	gw, creds, err := a.getGatewayWithCredentials(ctx, gatewayID)
	if err != nil {
		return nil, err
	}

	controlPlaneURL, ok := gw.AdapterConfig["controlPlaneUrl"].(string)
	if !ok {
		return nil, fmt.Errorf("controlPlaneUrl not found in gateway adapter config")
	}

	reqCreds := &clients.RequestCredentials{
		Username: creds.Username,
		Password: creds.Password,
	}

	resp, err := a.gatewayClient.UpdateLLMProvider(ctx, controlPlaneURL, providerID, config.Configuration, reqCreds)
	if err != nil {
		return nil, fmt.Errorf("failed to update provider on gateway: %w", err)
	}

	return &gateway.ProviderDeploymentResult{
		DeploymentID: resp.ID,
		Status:       resp.Status,
		DeployedAt:   time.Now(),
	}, nil
}

// UndeployProvider removes an LLM provider from a gateway
func (a *OnPremiseAdapter) UndeployProvider(ctx context.Context, gatewayID string, providerID string) error {
	a.logger.Info("Undeploying provider from gateway", "gatewayID", gatewayID, "providerID", providerID)

	gw, creds, err := a.getGatewayWithCredentials(ctx, gatewayID)
	if err != nil {
		return err
	}

	controlPlaneURL, ok := gw.AdapterConfig["controlPlaneUrl"].(string)
	if !ok {
		return fmt.Errorf("controlPlaneUrl not found in gateway adapter config")
	}

	reqCreds := &clients.RequestCredentials{
		Username: creds.Username,
		Password: creds.Password,
	}

	if err := a.gatewayClient.DeleteLLMProvider(ctx, controlPlaneURL, providerID, reqCreds); err != nil {
		return fmt.Errorf("failed to delete provider on gateway: %w", err)
	}

	return nil
}

// GetProviderStatus retrieves the status of a provider deployment on a gateway
func (a *OnPremiseAdapter) GetProviderStatus(ctx context.Context, gatewayID string, providerID string) (*gateway.ProviderStatus, error) {
	gw, creds, err := a.getGatewayWithCredentials(ctx, gatewayID)
	if err != nil {
		return nil, err
	}

	controlPlaneURL, ok := gw.AdapterConfig["controlPlaneUrl"].(string)
	if !ok {
		return nil, fmt.Errorf("controlPlaneUrl not found in gateway adapter config")
	}

	reqCreds := &clients.RequestCredentials{
		Username: creds.Username,
		Password: creds.Password,
	}

	resp, err := a.gatewayClient.GetLLMProvider(ctx, controlPlaneURL, providerID, reqCreds)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider status: %w", err)
	}

	var deployedAt *time.Time
	if resp.DeployedAt != "" {
		if t, err := time.Parse(time.RFC3339, resp.DeployedAt); err == nil {
			deployedAt = &t
		}
	}

	return &gateway.ProviderStatus{
		ID:         resp.ID,
		Name:       resp.Name,
		Kind:       resp.Kind,
		Status:     resp.Status,
		Spec:       resp.Spec,
		DeployedAt: deployedAt,
	}, nil
}

// ListProviders lists all LLM providers deployed on a gateway
func (a *OnPremiseAdapter) ListProviders(ctx context.Context, gatewayID string) ([]*gateway.ProviderStatus, error) {
	gw, creds, err := a.getGatewayWithCredentials(ctx, gatewayID)
	if err != nil {
		return nil, err
	}

	controlPlaneURL, ok := gw.AdapterConfig["controlPlaneUrl"].(string)
	if !ok {
		return nil, fmt.Errorf("controlPlaneUrl not found in gateway adapter config")
	}

	reqCreds := &clients.RequestCredentials{
		Username: creds.Username,
		Password: creds.Password,
	}

	resp, err := a.gatewayClient.ListLLMProviders(ctx, controlPlaneURL, reqCreds)
	if err != nil {
		return nil, fmt.Errorf("failed to list providers: %w", err)
	}

	var providers []*gateway.ProviderStatus
	for _, p := range resp.Providers {
		var deployedAt *time.Time
		if p.DeployedAt != "" {
			if t, err := time.Parse(time.RFC3339, p.DeployedAt); err == nil {
				deployedAt = &t
			}
		}

		providers = append(providers, &gateway.ProviderStatus{
			ID:         p.ID,
			Name:       p.Name,
			Kind:       p.Kind,
			Status:     p.Status,
			Spec:       p.Spec,
			DeployedAt: deployedAt,
		})
	}

	return providers, nil
}

// GetPolicies retrieves available policies from a gateway
func (a *OnPremiseAdapter) GetPolicies(ctx context.Context, gatewayID string) ([]*gateway.PolicyInfo, error) {
	gw, creds, err := a.getGatewayWithCredentials(ctx, gatewayID)
	if err != nil {
		return nil, err
	}

	controlPlaneURL, ok := gw.AdapterConfig["controlPlaneUrl"].(string)
	if !ok {
		return nil, fmt.Errorf("controlPlaneUrl not found in gateway adapter config")
	}

	reqCreds := &clients.RequestCredentials{
		Username: creds.Username,
		Password: creds.Password,
	}

	resp, err := a.gatewayClient.GetPolicies(ctx, controlPlaneURL, reqCreds)
	if err != nil {
		return nil, fmt.Errorf("failed to get policies: %w", err)
	}

	var policies []*gateway.PolicyInfo
	for _, p := range resp.Policies {
		policies = append(policies, &gateway.PolicyInfo{
			Name:        p.Name,
			Version:     p.Version,
			Description: p.Description,
			Parameters:  p.Parameters,
		})
	}

	return policies, nil
}

// init registers the adapter with the factory
func init() {
	// This will be called when the package is imported
	// Registration happens in wiring
}
