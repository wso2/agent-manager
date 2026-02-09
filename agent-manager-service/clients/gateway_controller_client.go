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

package clients

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// RequestCredentials holds authentication credentials for gateway API calls
type RequestCredentials struct {
	Username string
	Password string
}

// GatewayControllerClient is a client for communicating with gateway-controller instances
type GatewayControllerClient interface {
	// HealthCheck performs a health check on a gateway controller
	HealthCheck(ctx context.Context, controlPlaneURL string) error

	// LLM Provider Management
	CreateLLMProvider(ctx context.Context, baseURL string, config map[string]interface{}, creds *RequestCredentials) (*LLMProviderResponse, error)
	GetLLMProvider(ctx context.Context, baseURL string, providerID string, creds *RequestCredentials) (*LLMProviderResponse, error)
	ListLLMProviders(ctx context.Context, baseURL string, creds *RequestCredentials) (*LLMProviderListResponse, error)
	UpdateLLMProvider(ctx context.Context, baseURL string, providerID string, config map[string]interface{}, creds *RequestCredentials) (*LLMProviderResponse, error)
	DeleteLLMProvider(ctx context.Context, baseURL string, providerID string, creds *RequestCredentials) error
	GetPolicies(ctx context.Context, baseURL string, creds *RequestCredentials) (*PoliciesResponse, error)
}

// LLMProviderResponse represents the response from gateway when creating/getting a provider
type LLMProviderResponse struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Kind       string                 `json:"kind"`
	Status     string                 `json:"status"`
	Spec       map[string]interface{} `json:"spec,omitempty"`
	CreatedAt  string                 `json:"created_at"`
	DeployedAt string                 `json:"deployed_at,omitempty"`
}

// LLMProviderListResponse represents the response when listing providers
type LLMProviderListResponse struct {
	Providers []LLMProviderResponse `json:"providers"`
}

// PoliciesResponse represents the response from gateway when fetching policies
type PoliciesResponse struct {
	Status   string       `json:"status"`
	Count    int          `json:"count"`
	Policies []PolicyInfo `json:"policies"`
}

// PolicyInfo holds information about an available policy
type PolicyInfo struct {
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type gatewayControllerClient struct {
	httpClient *http.Client
}

// NewGatewayControllerClient creates a new gateway controller client
func NewGatewayControllerClient() GatewayControllerClient {
	return &gatewayControllerClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// HealthCheck performs a health check on a gateway controller
func (c *gatewayControllerClient) HealthCheck(ctx context.Context, controlPlaneURL string) error {
	healthURL := fmt.Sprintf("%s/health", controlPlaneURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed with status %d", resp.StatusCode)
	}

	return nil
}

// CreateLLMProvider creates a new LLM provider on the gateway
func (c *gatewayControllerClient) CreateLLMProvider(ctx context.Context, baseURL string, config map[string]interface{}, creds *RequestCredentials) (*LLMProviderResponse, error) {
	url := fmt.Sprintf("%s/llm-providers", baseURL)

	payload, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal provider config: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if creds != nil {
		req.Header.Set("Authorization", c.basicAuth(creds.Username, creds.Password))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create provider failed with status %d", resp.StatusCode)
	}

	var result LLMProviderResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetLLMProvider retrieves an LLM provider from the gateway
func (c *gatewayControllerClient) GetLLMProvider(ctx context.Context, baseURL string, providerID string, creds *RequestCredentials) (*LLMProviderResponse, error) {
	url := fmt.Sprintf("%s/llm-providers/%s", baseURL, providerID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if creds != nil {
		req.Header.Set("Authorization", c.basicAuth(creds.Username, creds.Password))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("provider not found: %s", providerID)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get provider failed with status %d", resp.StatusCode)
	}

	var result LLMProviderResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// ListLLMProviders lists all LLM providers on the gateway
func (c *gatewayControllerClient) ListLLMProviders(ctx context.Context, baseURL string, creds *RequestCredentials) (*LLMProviderListResponse, error) {
	url := fmt.Sprintf("%s/llm-providers", baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if creds != nil {
		req.Header.Set("Authorization", c.basicAuth(creds.Username, creds.Password))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list providers failed with status %d", resp.StatusCode)
	}

	var result LLMProviderListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// UpdateLLMProvider updates an existing LLM provider on the gateway
func (c *gatewayControllerClient) UpdateLLMProvider(ctx context.Context, baseURL string, providerID string, config map[string]interface{}, creds *RequestCredentials) (*LLMProviderResponse, error) {
	url := fmt.Sprintf("%s/llm-providers/%s", baseURL, providerID)

	payload, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal provider config: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if creds != nil {
		req.Header.Set("Authorization", c.basicAuth(creds.Username, creds.Password))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
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

	var result LLMProviderResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// DeleteLLMProvider deletes an LLM provider from the gateway
func (c *gatewayControllerClient) DeleteLLMProvider(ctx context.Context, baseURL string, providerID string, creds *RequestCredentials) error {
	url := fmt.Sprintf("%s/llm-providers/%s", baseURL, providerID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if creds != nil {
		req.Header.Set("Authorization", c.basicAuth(creds.Username, creds.Password))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
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

// GetPolicies retrieves available policies from the gateway
func (c *gatewayControllerClient) GetPolicies(ctx context.Context, baseURL string, creds *RequestCredentials) (*PoliciesResponse, error) {
	url := fmt.Sprintf("%s/policies", baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if creds != nil {
		req.Header.Set("Authorization", c.basicAuth(creds.Username, creds.Password))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get policies failed with status %d", resp.StatusCode)
	}

	var result PoliciesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// basicAuth generates a Basic Authentication header value
func (c *gatewayControllerClient) basicAuth(username, password string) string {
	auth := username + ":" + password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
}
