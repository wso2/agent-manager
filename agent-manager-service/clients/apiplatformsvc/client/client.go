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

// Package client provides the API Platform service client wrapper.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg clientmocks -out ../../clientmocks/apiplatform_client_fake.go . APIPlatformClient:APIPlatformClientMock
package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/apiplatformsvc/gen"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/requests"
)

// Config contains configuration for the API Platform client
type Config struct {
	BaseURL      string
	AuthProvider AuthProvider
	RetryConfig  requests.RequestRetryConfig
}

// APIPlatformClient defines the interface for API Platform operations
type APIPlatformClient interface {
	// Gateway Operations
	CreateGateway(ctx context.Context, req CreateGatewayRequest) (*GatewayResponse, error)
	GetGateway(ctx context.Context, gatewayID string) (*GatewayResponse, error)
	ListGateways(ctx context.Context) ([]*GatewayResponse, error)
	UpdateGateway(ctx context.Context, gatewayID string, req UpdateGatewayRequest) (*GatewayResponse, error)
	DeleteGateway(ctx context.Context, gatewayID string) error

	// Gateway Token Operations
	RotateGatewayToken(ctx context.Context, gatewayID string) (*GatewayTokenResponse, error)
	RevokeGatewayToken(ctx context.Context, gatewayID string, tokenID string) error

	// Organization Operations
	GetOrganization(ctx context.Context) (*OrganizationResponse, error)
	RegisterOrganization(ctx context.Context, req RegisterOrganizationRequest) (*OrganizationResponse, error)
}

type apiPlatformClient struct {
	baseURL   string
	genClient *gen.ClientWithResponses
}

// NewapiPlatformClient creates a new API Platform gateway client
func NewAPIPlatformClient(cfg *Config) (APIPlatformClient, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	if cfg.AuthProvider == nil {
		return nil, fmt.Errorf("auth provider is required")
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	// Create the retryable HTTP client (uses defaults if RetryConfig is zero-value)
	httpClient := requests.NewRetryableHTTPClient(&http.Client{
		Transport: tr,
	}, cfg.RetryConfig)

	// Create auth request editor
	authEditor := func(ctx context.Context, req *http.Request) error {
		slog.Debug("Adding auth token to request")
		token, err := cfg.AuthProvider.GetToken(ctx)
		if err != nil {
			return fmt.Errorf("failed to get auth token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}

	// Create the generated OpenAPI client with retryable HTTP client and auth
	genClient, err := gen.NewClientWithResponses(
		cfg.BaseURL,
		gen.WithHTTPClient(httpClient),
		gen.WithRequestEditorFn(authEditor),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create API Platform client: %w", err)
	}

	return &apiPlatformClient{
		baseURL:   cfg.BaseURL,
		genClient: genClient,
	}, nil
}

// CreateGateway creates a new gateway in API Platform
func (c *apiPlatformClient) CreateGateway(ctx context.Context, req CreateGatewayRequest) (*GatewayResponse, error) {
	slog.Debug("Creating gateway via API Platform", "name", req.Name)

	// Convert to API Platform request type
	apiReq := gen.CreateGatewayJSONRequestBody{
		Name:              req.Name,
		DisplayName:       req.DisplayName,
		Vhost:             req.Vhost,
		FunctionalityType: convertToGenFunctionalityType(req.FunctionalityType),
		Description:       req.Description,
		IsCritical:        req.IsCritical,
		Properties:        req.Properties,
	}

	resp, err := c.genClient.CreateGatewayWithResponse(ctx, apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create gateway: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated && resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{})
	}

	if resp.JSON201 == nil {
		return nil, fmt.Errorf("empty response from create gateway")
	}

	return convertFromGenGatewayResponse(resp.JSON201), nil
}

// GetGateway retrieves a gateway by ID from API Platform
func (c *apiPlatformClient) GetGateway(ctx context.Context, gatewayID string) (*GatewayResponse, error) {
	slog.Debug("Getting gateway via API Platform", "gatewayID", gatewayID)

	// Convert string to UUID
	uuid, err := parseUUID(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("invalid gateway ID: %w", err)
	}

	resp, err := c.genClient.GetGatewayWithResponse(ctx, uuid)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{})
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from get gateway")
	}

	return convertFromGenGatewayResponse(resp.JSON200), nil
}

// ListGateways retrieves all gateways from API Platform
func (c *apiPlatformClient) ListGateways(ctx context.Context) ([]*GatewayResponse, error) {
	slog.Debug("Listing gateways via API Platform")

	resp, err := c.genClient.ListGatewaysWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list gateways: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{})
	}

	if resp.JSON200 == nil {
		return []*GatewayResponse{}, nil
	}

	gateways := make([]*GatewayResponse, len(resp.JSON200.List))
	for i, gw := range resp.JSON200.List {
		gateways[i] = convertFromGenGatewayResponse(&gw)
	}

	return gateways, nil
}

// UpdateGateway updates an existing gateway in API Platform
func (c *apiPlatformClient) UpdateGateway(ctx context.Context, gatewayID string, req UpdateGatewayRequest) (*GatewayResponse, error) {
	slog.Debug("Updating gateway via API Platform", "gatewayID", gatewayID)

	// Convert string to UUID
	uuid, err := parseUUID(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("invalid gateway ID: %w", err)
	}

	apiReq := gen.UpdateGatewayJSONRequestBody{
		DisplayName: req.DisplayName,
		Description: req.Description,
		IsCritical:  req.IsCritical,
		Properties:  req.Properties,
	}

	resp, err := c.genClient.UpdateGatewayWithResponse(ctx, uuid, apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to update gateway: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{})
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from update gateway")
	}

	return convertFromGenGatewayResponse(resp.JSON200), nil
}

// DeleteGateway deletes a gateway from API Platform
func (c *apiPlatformClient) DeleteGateway(ctx context.Context, gatewayID string) error {
	slog.Debug("Deleting gateway via API Platform", "gatewayID", gatewayID)

	// Convert string to UUID
	uuid, err := parseUUID(gatewayID)
	if err != nil {
		return fmt.Errorf("invalid gateway ID: %w", err)
	}

	resp, err := c.genClient.DeleteGatewayWithResponse(ctx, uuid)
	if err != nil {
		return fmt.Errorf("failed to delete gateway: %w", err)
	}

	// API Platform returns 204 No Content on success
	if resp.StatusCode() != http.StatusNoContent && resp.StatusCode() != http.StatusOK {
		return handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{})
	}

	return nil
}

// RotateGatewayToken generates a new token for the gateway and invalidates the old one
func (c *apiPlatformClient) RotateGatewayToken(ctx context.Context, gatewayID string) (*GatewayTokenResponse, error) {
	slog.Debug("Rotating gateway token via API Platform", "gatewayID", gatewayID)

	// Convert string to UUID
	uuid, err := parseUUID(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("invalid gateway ID: %w", err)
	}

	resp, err := c.genClient.RotateGatewayTokenWithResponse(ctx, uuid)
	if err != nil {
		return nil, fmt.Errorf("failed to rotate gateway token: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated && resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{})
	}

	if resp.JSON201 == nil {
		return nil, fmt.Errorf("empty response from rotate gateway token")
	}

	return convertFromGenTokenRotationResponse(resp.JSON201, gatewayID), nil
}

// RevokeGatewayToken revokes a specific gateway token
func (c *apiPlatformClient) RevokeGatewayToken(ctx context.Context, gatewayID string, tokenID string) error {
	slog.Debug("Revoking gateway token via API Platform", "gatewayID", gatewayID, "tokenID", tokenID)

	// Convert gateway ID string to UUID
	gwUUID, err := parseUUID(gatewayID)
	if err != nil {
		return fmt.Errorf("invalid gateway ID: %w", err)
	}

	// Convert token ID string to UUID
	tokenUUID, err := parseUUID(tokenID)
	if err != nil {
		return fmt.Errorf("invalid token ID: %w", err)
	}

	resp, err := c.genClient.RevokeGatewayTokenWithResponse(ctx, gwUUID, tokenUUID)
	if err != nil {
		return fmt.Errorf("failed to revoke gateway token: %w", err)
	}

	// API Platform returns 204 No Content on success
	if resp.StatusCode() != http.StatusNoContent && resp.StatusCode() != http.StatusOK {
		return handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{})
	}

	return nil
}

// GetOrganization retrieves the current organization from API Platform
func (c *apiPlatformClient) GetOrganization(ctx context.Context) (*OrganizationResponse, error) {
	slog.Debug("Getting organization via API Platform")

	resp, err := c.genClient.GetOrganizationWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{})
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("empty response from get organization")
	}

	return convertFromGenOrganizationResponse(resp.JSON200), nil
}

// RegisterOrganization registers/creates a new organization in API Platform
func (c *apiPlatformClient) RegisterOrganization(ctx context.Context, req RegisterOrganizationRequest) (*OrganizationResponse, error) {
	slog.Debug("Registering organization via API Platform", "name", req.Name)

	// Parse UUID
	uuid, err := parseUUID(req.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid organization ID: %w", err)
	}

	// Convert to API Platform request type
	apiReq := gen.RegisterOrganizationJSONRequestBody{
		Id:     &uuid,
		Name:   req.Name,
		Handle: req.Handle,
		Region: req.Region,
	}

	resp, err := c.genClient.RegisterOrganizationWithResponse(ctx, apiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to register organization: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated && resp.StatusCode() != http.StatusOK {
		return nil, handleErrorResponse(resp.StatusCode(), resp.Body, ErrorContext{})
	}

	if resp.JSON201 == nil {
		return nil, fmt.Errorf("empty response from register organization")
	}

	return convertFromGenOrganizationResponse(resp.JSON201), nil
}
