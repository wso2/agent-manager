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

package gatewaymgmtsvc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/clients/requests"
)

const (
	artifactIDAnnotation = "gateway.api-platform.wso2.com/artifact-id"
	componentUIDLabel    = "openchoreo.dev/component-uid"
)

// Client defines gateway management API operations needed by Agent Manager.
type Client interface {
	FindRestAPIByArtifactID(ctx context.Context, artifactID string) (restAPIID string, err error)
	CreateAPIKey(ctx context.Context, restAPIID, keyName string, expiresAt *string) (keyValue string, effectiveExpiresAt *time.Time, err error)
	RevokeAPIKey(ctx context.Context, restAPIID, keyName string) error
	ListAPIKeys(ctx context.Context, restAPIID string) ([]GatewayAPIKey, error)
}

// Config contains configuration for the gateway management client.
type Config struct {
	BaseURL     string
	Username    string
	Password    string
	HTTPClient  requests.HttpClient
	RetryConfig requests.RequestRetryConfig
}

type gatewayManagementClient struct {
	baseURL    string
	authHeader string
	httpClient requests.HttpClient
}

// GatewayAPIKey represents an API key returned by the gateway management API.
type GatewayAPIKey struct {
	Name        string     `json:"name"`
	DisplayName string     `json:"displayName"`
	APIKey      string     `json:"apiKey"`
	APIID       string     `json:"apiId"`
	Status      string     `json:"status"`
	CreatedAt   *time.Time `json:"createdAt"`
	CreatedBy   string     `json:"createdBy"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	Source      string     `json:"source"`
}

type restAPIListResponse struct {
	Status string    `json:"status"`
	Count  int       `json:"count"`
	APIs   []restAPI `json:"apis"`
}

type restAPI struct {
	Metadata restAPIMetadata `json:"metadata"`
	Status   restAPIStatus   `json:"status"`
	ID       string          `json:"id"`
}

type restAPIMetadata struct {
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

type restAPIStatus struct {
	ID string `json:"id"`
}

func (s *restAPIStatus) UnmarshalJSON(data []byte) error {
	var statusText string
	if err := json.Unmarshal(data, &statusText); err == nil {
		return nil
	}

	var statusObject struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &statusObject); err != nil {
		return err
	}
	s.ID = statusObject.ID
	return nil
}

type restAPIDetailResponse struct {
	Status string        `json:"status"`
	API    restAPIDetail `json:"api"`
}

type restAPIDetail struct {
	ID            string               `json:"id"`
	Configuration restAPIConfiguration `json:"configuration"`
}

type restAPIConfiguration struct {
	Metadata restAPIMetadata `json:"metadata"`
}

type apiKeyCreationResponse struct {
	Status string        `json:"status"`
	APIKey GatewayAPIKey `json:"apiKey"`
}

type apiKeyCreationRequest struct {
	Name      string  `json:"name,omitempty"`
	ExpiresAt *string `json:"expiresAt,omitempty"`
}

type apiKeyListResponse struct {
	APIKeys []GatewayAPIKey `json:"apiKeys"`
}

// NewClient creates a new gateway management API client.
func NewClient(cfg Config) (Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("gateway management base URL is required")
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("invalid gateway management base URL: %w", err)
	}
	if cfg.Username == "" {
		return nil, fmt.Errorf("gateway management username is required")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = requests.NewRetryableHTTPClient(&http.Client{}, cfg.RetryConfig)
	}

	credentials := base64.StdEncoding.EncodeToString([]byte(cfg.Username + ":" + cfg.Password))
	return &gatewayManagementClient{
		baseURL:    baseURL,
		authHeader: "Basic " + credentials,
		httpClient: httpClient,
	}, nil
}

// NewClientWithCredentials creates a client using a base URL and Basic Auth credentials.
func NewClientWithCredentials(baseURL, username, password string) (Client, error) {
	return NewClient(Config{
		BaseURL:  baseURL,
		Username: username,
		Password: password,
	})
}

func (c *gatewayManagementClient) FindRestAPIByArtifactID(ctx context.Context, artifactID string) (string, error) {
	artifactID = strings.TrimSpace(artifactID)
	if artifactID == "" {
		return "", fmt.Errorf("artifact ID is required")
	}

	var response restAPIListResponse
	err := requests.SendRequest(ctx, c.httpClient, c.newRequest("gatewaymgmtsvc.FindRestAPIByArtifactID", http.MethodGet, "/rest-apis").
		SetHeader("Accept", "application/json")).
		ScanResponse(&response, http.StatusOK)
	if err != nil {
		return "", err
	}

	for _, api := range response.APIs {
		restAPIID := apiRestAPIID(api)
		if restAPIMatchesArtifact(api.Metadata, artifactID) {
			if restAPIID == "" {
				return "", fmt.Errorf("RestAPI matched artifact ID %q but response id is empty", artifactID)
			}
			return restAPIID, nil
		}
		if restAPIID == "" || restAPIHasArtifactMetadata(api.Metadata) {
			continue
		}
		detail, detailErr := c.getRestAPI(ctx, restAPIID)
		if detailErr != nil {
			return "", detailErr
		}
		if restAPIMatchesArtifact(detail.Configuration.Metadata, artifactID) {
			if detail.ID != "" {
				return detail.ID, nil
			}
			return restAPIID, nil
		}
	}
	return "", fmt.Errorf("RestAPI not found for artifact ID %q", artifactID)
}

func (c *gatewayManagementClient) getRestAPI(ctx context.Context, restAPIID string) (restAPIDetail, error) {
	var response restAPIDetailResponse
	err := requests.SendRequest(ctx, c.httpClient, c.newRequest("gatewaymgmtsvc.GetRestAPI", http.MethodGet, "/rest-apis/"+url.PathEscape(restAPIID)).
		SetHeader("Accept", "application/json")).
		ScanResponse(&response, http.StatusOK)
	if err != nil {
		return restAPIDetail{}, err
	}
	return response.API, nil
}

func restAPIMatchesArtifact(metadata restAPIMetadata, artifactID string) bool {
	return metadata.Annotations[artifactIDAnnotation] == artifactID || metadata.Labels[componentUIDLabel] == artifactID
}

func restAPIHasArtifactMetadata(metadata restAPIMetadata) bool {
	return len(metadata.Annotations) > 0 || len(metadata.Labels) > 0
}

func apiRestAPIID(api restAPI) string {
	if api.Status.ID != "" {
		return api.Status.ID
	}
	return api.ID
}

func (c *gatewayManagementClient) CreateAPIKey(ctx context.Context, restAPIID, keyName string, expiresAt *string) (string, *time.Time, error) {
	restAPIID = strings.TrimSpace(restAPIID)
	keyName = strings.TrimSpace(keyName)
	if restAPIID == "" {
		return "", nil, fmt.Errorf("RestAPI ID is required")
	}
	if keyName == "" {
		return "", nil, fmt.Errorf("API key name is required")
	}

	var response apiKeyCreationResponse
	err := requests.SendRequest(ctx, c.httpClient, c.newRequest("gatewaymgmtsvc.CreateAPIKey", http.MethodPost, "/rest-apis/"+url.PathEscape(restAPIID)+"/api-keys").
		SetHeader("Accept", "application/json").
		SetJson(apiKeyCreationRequest{
			Name:      keyName,
			ExpiresAt: expiresAt,
		})).
		ScanResponse(&response, http.StatusCreated)
	if err != nil {
		return "", nil, err
	}
	if response.APIKey.APIKey == "" {
		return "", nil, fmt.Errorf("gateway API key creation response did not include apiKey")
	}
	return response.APIKey.APIKey, response.APIKey.ExpiresAt, nil
}

func (c *gatewayManagementClient) RevokeAPIKey(ctx context.Context, restAPIID, keyName string) error {
	restAPIID = strings.TrimSpace(restAPIID)
	keyName = strings.TrimSpace(keyName)
	if restAPIID == "" {
		return fmt.Errorf("RestAPI ID is required")
	}
	if keyName == "" {
		return fmt.Errorf("API key name is required")
	}

	var response map[string]interface{}
	return requests.SendRequest(ctx, c.httpClient, c.newRequest("gatewaymgmtsvc.RevokeAPIKey", http.MethodDelete, "/rest-apis/"+url.PathEscape(restAPIID)+"/api-keys/"+url.PathEscape(keyName)).
		SetHeader("Accept", "application/json")).
		ScanResponse(&response, http.StatusOK)
}

func (c *gatewayManagementClient) ListAPIKeys(ctx context.Context, restAPIID string) ([]GatewayAPIKey, error) {
	restAPIID = strings.TrimSpace(restAPIID)
	if restAPIID == "" {
		return nil, fmt.Errorf("RestAPI ID is required")
	}

	var response apiKeyListResponse
	err := requests.SendRequest(ctx, c.httpClient, c.newRequest("gatewaymgmtsvc.ListAPIKeys", http.MethodGet, "/rest-apis/"+url.PathEscape(restAPIID)+"/api-keys").
		SetHeader("Accept", "application/json")).
		ScanResponse(&response, http.StatusOK)
	if err != nil {
		return nil, err
	}
	return response.APIKeys, nil
}

func (c *gatewayManagementClient) newRequest(name, method, path string) *requests.HttpRequest {
	return (&requests.HttpRequest{
		Name:   name,
		Method: method,
		URL:    c.baseURL + path,
	}).SetHeader("Authorization", c.authHeader)
}
