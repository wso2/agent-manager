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

package secretmgmtsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/requests"
)

//go:generate moq -rm -fmt goimports -skip-ensure -pkg clientmocks -out ../clientmocks/secret_mgmt_client_fake.go . SecretManagementClient:SecretManagementClientMock

// Config contains configuration for the secret management service client
type Config struct {
	BaseURL      string
	AuthProvider client.AuthProvider
	RetryConfig  requests.RequestRetryConfig
}

// SecretManagementClient defines the interface for secret management operations
type SecretManagementClient interface {
	// CreateSecret creates a new secret in the KV store
	CreateSecret(ctx context.Context, orgName string, req CreateSecretRequest) (*SecretResponse, error)

	// GetSecret retrieves a secret from the KV store
	GetSecret(ctx context.Context, orgName, secretPath string) (*SecretResponse, error)

	// UpdateSecret updates an existing secret in the KV store
	UpdateSecret(ctx context.Context, orgName, secretPath string, req UpdateSecretRequest) (*SecretResponse, error)

	// DeleteSecret deletes a secret from the KV store
	DeleteSecret(ctx context.Context, orgName, secretPath string) error

	// ListSecrets lists secrets under the specified path prefix
	ListSecrets(ctx context.Context, orgName, pathPrefix string) ([]SecretMetadata, error)
}

// CreateSecretRequest for creating a secret
type CreateSecretRequest struct {
	Path string            `json:"path"`
	Data map[string]string `json:"data"`
}

// UpdateSecretRequest for updating a secret
type UpdateSecretRequest struct {
	Data map[string]string `json:"data"`
}

// SecretResponse represents a secret in responses
type SecretResponse struct {
	Path     string            `json:"path"`
	Data     map[string]string `json:"data"`
	Version  int               `json:"version"`
	Metadata SecretMetadata    `json:"metadata"`
}

// SecretMetadata represents secret metadata
type SecretMetadata struct {
	Path        string    `json:"path"`
	Version     int       `json:"version"`
	CreatedTime time.Time `json:"createdTime"`
	UpdatedTime time.Time `json:"updatedTime"`
}

// ErrorResponse represents an error from the secret management service
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type secretManagementClient struct {
	baseURL      string
	httpClient   requests.HttpClient
	authProvider client.AuthProvider
}

// NewSecretManagementClient creates a new secret management service client
func NewSecretManagementClient(cfg *Config) (SecretManagementClient, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	if cfg.AuthProvider == nil {
		return nil, fmt.Errorf("auth provider is required")
	}

	// Configure retry with 401 handling
	retryConfig := cfg.RetryConfig
	if retryConfig.RetryOnStatus == nil {
		retryConfig.RetryOnStatus = func(statusCode int) bool {
			if statusCode == http.StatusUnauthorized {
				slog.Info("Received 401 Unauthorized, invalidating cached token")
				cfg.AuthProvider.InvalidateToken()
				return true
			}
			return slices.Contains(requests.TransientHTTPErrorCodes, statusCode)
		}
	}

	httpClient := requests.NewRetryableHTTPClient(&http.Client{
		Timeout: 30 * time.Second,
	}, retryConfig)

	return &secretManagementClient{
		baseURL:      cfg.BaseURL,
		httpClient:   httpClient,
		authProvider: cfg.AuthProvider,
	}, nil
}

func (c *secretManagementClient) CreateSecret(ctx context.Context, orgName string, req CreateSecretRequest) (*SecretResponse, error) {
	endpoint := fmt.Sprintf("%s/api/v1/organizations/%s/secrets", c.baseURL, url.PathEscape(orgName))

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	if err := c.addAuthHeader(ctx, httpReq); err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, c.parseError(resp)
	}

	var result SecretResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

func (c *secretManagementClient) GetSecret(ctx context.Context, orgName, secretPath string) (*SecretResponse, error) {
	endpoint := fmt.Sprintf("%s/api/v1/organizations/%s/secrets/%s", c.baseURL, url.PathEscape(orgName), url.PathEscape(secretPath))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if err := c.addAuthHeader(ctx, httpReq); err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var result SecretResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

func (c *secretManagementClient) UpdateSecret(ctx context.Context, orgName, secretPath string, req UpdateSecretRequest) (*SecretResponse, error) {
	endpoint := fmt.Sprintf("%s/api/v1/organizations/%s/secrets/%s", c.baseURL, url.PathEscape(orgName), url.PathEscape(secretPath))

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	if err := c.addAuthHeader(ctx, httpReq); err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var result SecretResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

func (c *secretManagementClient) DeleteSecret(ctx context.Context, orgName, secretPath string) error {
	endpoint := fmt.Sprintf("%s/api/v1/organizations/%s/secrets/%s", c.baseURL, url.PathEscape(orgName), url.PathEscape(secretPath))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if err := c.addAuthHeader(ctx, httpReq); err != nil {
		return err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.parseError(resp)
	}

	return nil
}

func (c *secretManagementClient) ListSecrets(ctx context.Context, orgName, pathPrefix string) ([]SecretMetadata, error) {
	endpoint := fmt.Sprintf("%s/api/v1/organizations/%s/secrets", c.baseURL, url.PathEscape(orgName))
	if pathPrefix != "" {
		endpoint = fmt.Sprintf("%s?path=%s", endpoint, url.QueryEscape(pathPrefix))
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if err := c.addAuthHeader(ctx, httpReq); err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var result []SecretMetadata
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

func (c *secretManagementClient) addAuthHeader(ctx context.Context, req *http.Request) error {
	token, err := c.authProvider.GetToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get auth token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

func (c *secretManagementClient) parseError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("request failed with status %d", resp.StatusCode)
	}

	var errResp ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return fmt.Errorf("%s: %s", errResp.Error, errResp.Message)
}
