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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/gatewaysvc/gen"
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

// newGatewayClientForURL creates a new gateway client instance for a specific URL
// This prevents data races from shared client mutation across concurrent requests
func (a *OnPremiseAdapter) newGatewayClientForURL(controlPlaneURL string) (*gen.Client, error) {
	return gen.NewClient(controlPlaneURL, func(c *gen.Client) error {
		c.Client = a.httpClient
		return nil
	})
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

// createAuthEditor creates a RequestEditorFn that adds Basic Authentication
func createAuthEditor(username, password string) gen.RequestEditorFn {
	return func(ctx context.Context, req *http.Request) error {
		auth := username + ":" + password
		encoded := base64.StdEncoding.EncodeToString([]byte(auth))
		req.Header.Set("Authorization", "Basic "+encoded)
		return nil
	}
}

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

	// Create a new client for this specific URL to avoid data races
	client, err := a.newGatewayClientForURL(controlPlaneURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create gateway client: %w", err)
	}

	// Convert configuration map to LLMProviderConfiguration
	// The config.Configuration should already be a proper structure matching the API
	var providerConfig gen.LLMProviderConfiguration
	configBytes, err := json.Marshal(config.Configuration)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal configuration: %w", err)
	}
	if err := json.Unmarshal(configBytes, &providerConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	// Deploy provider via gateway client
	resp, err := client.CreateLLMProvider(ctx, providerConfig, createAuthEditor(creds.Username, creds.Password))
	if err != nil {
		return nil, fmt.Errorf("failed to create provider on gateway: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create provider failed with status %d", resp.StatusCode)
	}

	var createResp gen.LLMProviderCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	status := ""
	if createResp.Status != nil {
		status = *createResp.Status
	}

	deploymentID := ""
	if createResp.Id != nil {
		deploymentID = *createResp.Id
	}

	return &gateway.ProviderDeploymentResult{
		DeploymentID: deploymentID,
		Status:       status,
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

	// Create a new client for this specific URL to avoid data races
	client, err := a.newGatewayClientForURL(controlPlaneURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create gateway client: %w", err)
	}

	// Convert configuration map to LLMProviderConfiguration
	var providerConfig gen.LLMProviderConfiguration
	configBytes, err := json.Marshal(config.Configuration)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal configuration: %w", err)
	}
	if err := json.Unmarshal(configBytes, &providerConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	resp, err := client.UpdateLLMProvider(ctx, providerID, providerConfig, createAuthEditor(creds.Username, creds.Password))
	if err != nil {
		return nil, fmt.Errorf("failed to update provider on gateway: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("provider not found: %s", providerID)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update provider failed with status %d", resp.StatusCode)
	}

	var updateResp gen.LLMProviderUpdateResponse
	if err := json.NewDecoder(resp.Body).Decode(&updateResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	status := ""
	if updateResp.Status != nil {
		status = *updateResp.Status
	}

	deploymentID := ""
	if updateResp.Id != nil {
		deploymentID = *updateResp.Id
	}

	return &gateway.ProviderDeploymentResult{
		DeploymentID: deploymentID,
		Status:       status,
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

	// Create a new client for this specific URL to avoid data races
	client, err := a.newGatewayClientForURL(controlPlaneURL)
	if err != nil {
		return fmt.Errorf("failed to create gateway client: %w", err)
	}

	resp, err := client.DeleteLLMProvider(ctx, providerID, createAuthEditor(creds.Username, creds.Password))
	if err != nil {
		return fmt.Errorf("failed to delete provider on gateway: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("provider not found: %s", providerID)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete provider failed with status %d", resp.StatusCode)
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

	// Create a new client for this specific URL to avoid data races
	client, err := a.newGatewayClientForURL(controlPlaneURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create gateway client: %w", err)
	}

	resp, err := client.GetLLMProviderById(ctx, providerID, createAuthEditor(creds.Username, creds.Password))
	if err != nil {
		return nil, fmt.Errorf("failed to get provider status: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get provider failed with status %d", resp.StatusCode)
	}

	var detailResp gen.LLMProviderDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&detailResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if detailResp.Provider == nil {
		return nil, fmt.Errorf("provider data not found in response")
	}

	providerStatus := &gateway.ProviderStatus{}

	if detailResp.Provider.Id != nil {
		providerStatus.ID = *detailResp.Provider.Id
	}

	if detailResp.Provider.Configuration != nil {
		providerStatus.Name = detailResp.Provider.Configuration.Metadata.Name
	}

	if detailResp.Provider.Configuration != nil {
		providerStatus.Kind = string(detailResp.Provider.Configuration.Kind)
	}

	if detailResp.Provider.DeploymentStatus != nil {
		providerStatus.Status = string(*detailResp.Provider.DeploymentStatus)
	}

	if detailResp.Provider.Metadata != nil && detailResp.Provider.Metadata.DeployedAt != nil {
		providerStatus.DeployedAt = detailResp.Provider.Metadata.DeployedAt
	}

	// Convert spec to map
	if detailResp.Provider.Configuration != nil {
		specBytes, err := json.Marshal(detailResp.Provider.Configuration.Spec)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal provider spec: %w", err)
		}
		var specMap map[string]interface{}
		if err := json.Unmarshal(specBytes, &specMap); err == nil {
			providerStatus.Spec = specMap
		}
	}

	return providerStatus, nil
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

	// Create a new client for this specific URL to avoid data races
	client, err := a.newGatewayClientForURL(controlPlaneURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create gateway client: %w", err)
	}

	params := &gen.ListLLMProvidersParams{}
	resp, err := client.ListLLMProviders(ctx, params, createAuthEditor(creds.Username, creds.Password))
	if err != nil {
		return nil, fmt.Errorf("failed to list providers: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list providers failed with status %d", resp.StatusCode)
	}

	var listResp struct {
		Providers []gen.LLMProviderListItem `json:"providers"`
		Status    *string                   `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var providers []*gateway.ProviderStatus
	for _, p := range listResp.Providers {
		providerStatus := &gateway.ProviderStatus{}

		if p.Id != nil {
			providerStatus.ID = *p.Id
		}

		if p.DisplayName != nil {
			providerStatus.Name = *p.DisplayName
		}

		if p.Template != nil {
			providerStatus.Kind = *p.Template
		}

		if p.Status != nil {
			providerStatus.Status = string(*p.Status)
		}

		providerStatus.DeployedAt = p.CreatedAt

		providers = append(providers, providerStatus)
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

	// Create a new client for this specific URL to avoid data races
	client, err := a.newGatewayClientForURL(controlPlaneURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create gateway client: %w", err)
	}

	resp, err := client.ListPolicies(ctx, createAuthEditor(creds.Username, creds.Password))
	if err != nil {
		return nil, fmt.Errorf("failed to list policies: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list policies failed with status %d", resp.StatusCode)
	}

	var policiesResp gen.PolicyListResponse
	if err := json.NewDecoder(resp.Body).Decode(&policiesResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var policies []*gateway.PolicyInfo
	if policiesResp.Policies != nil {
		for _, p := range *policiesResp.Policies {
			policyInfo := &gateway.PolicyInfo{
				Name:    p.Name,
				Version: "", // Version not provided in list response
			}

			if p.Description != nil {
				policyInfo.Description = *p.Description
			}

			// Convert parameters to map
			if p.Parameters != nil {
				policyInfo.Parameters = *p.Parameters
			}

			policies = append(policies, policyInfo)
		}
	}

	return policies, nil
}

// init registers the adapter with the factory
func init() {
	// This will be called when the package is imported
	// Registration happens in wiring
}
