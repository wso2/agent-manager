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

package thundersvc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ThunderClient encapsulates the Thunder API calls needed to create OAuth2 applications.
type ThunderClient interface {
	// EnsurePublisherApp creates an OAuth2 app named "amp-publisher-{orgName}" in Thunder
	// if it doesn't already exist. orgUUID is the Thunder organization unit UUID to assign
	// the app to. Returns clientId and clientSecret.
	// Idempotent: if app already exists, returns its existing clientId
	// (clientSecret is only available at creation time).
	EnsurePublisherApp(ctx context.Context, orgName, orgUUID string) (clientID, clientSecret string, created bool, err error)

	// DeletePublisherApp deletes the OAuth2 app named "amp-publisher-{orgName}" from Thunder.
	// Returns true if the app was found and deleted, false if it didn't exist.
	DeletePublisherApp(ctx context.Context, orgName string) (bool, error)
}

type thunderClient struct {
	baseURL      string // Thunder API base URL (e.g. http://thunder:8090)
	clientID     string // OAuth2 client ID of the system app (with Administrator role)
	clientSecret string // OAuth2 client secret of the system app
	httpClient   *http.Client

	mu          sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

// NewThunderClient creates a new Thunder API client.
// clientID/clientSecret are the OAuth2 credentials for the system app
// that has the Administrator role assigned (created at bootstrap).
func NewThunderClient(baseURL, clientID, clientSecret string) ThunderClient {
	return &thunderClient{
		baseURL:      baseURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

// getSystemToken returns a cached system token or fetches a new one.
func (c *thunderClient) getSystemToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cachedToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.cachedToken, nil
	}

	token, expiresIn, err := c.fetchSystemToken(ctx)
	if err != nil {
		return "", err
	}

	c.cachedToken = token
	// Refresh before expiry, but clamp skew so we never go negative
	const skew = 30
	if expiresIn > skew {
		c.tokenExpiry = time.Now().Add(time.Duration(expiresIn-skew) * time.Second)
	} else {
		c.tokenExpiry = time.Now().Add(time.Duration(max(expiresIn, 1)) * time.Second)
	}
	return c.cachedToken, nil
}

// fetchSystemToken obtains a system-scoped access token from Thunder's OAuth2 token endpoint
// using client_credentials grant with scope=system.
// The system app must have the Administrator role assigned in Thunder.
func (c *thunderClient) fetchSystemToken(ctx context.Context) (string, int, error) {
	data := url.Values{
		"grant_type": {"client_credentials"},
		"scope":      {"system"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/oauth2/token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("thunder token request build: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.clientID, c.clientSecret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("thunder token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("thunder token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", 0, fmt.Errorf("thunder token decode: %w", err)
	}

	if result.AccessToken == "" {
		return "", 0, fmt.Errorf("thunder returned empty access_token")
	}

	return result.AccessToken, result.ExpiresIn, nil
}

// getDefaultOUID fetches the default organization unit ID from Thunder.
func (c *thunderClient) getDefaultOUID(ctx context.Context, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/organization-units/tree/default", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("thunder get default OU: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("thunder get default OU returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("thunder OU decode: %w", err)
	}
	return result.ID, nil
}

// EnsurePublisherApp creates or returns an existing OAuth2 app for the given org.
// orgUUID is the Thunder organization unit UUID. If empty, the default OU is used.
func (c *thunderClient) EnsurePublisherApp(ctx context.Context, orgName, orgUUID string) (clientID, clientSecret string, created bool, err error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return "", "", false, fmt.Errorf("failed to get system token: %w", err)
	}

	appName := "amp-publisher-" + orgName

	// Check if app already exists
	_, existingClientID, err := c.findApp(ctx, token, appName)
	if err != nil {
		return "", "", false, err
	}
	if existingClientID != "" {
		return existingClientID, "", false, nil
	}

	// Resolve OU ID
	ouID := orgUUID
	if ouID == "" {
		ouID, err = c.getDefaultOUID(ctx, token)
		if err != nil {
			return "", "", false, err
		}
	}

	// Create new application
	id, secret, err := c.createApp(ctx, token, appName, ouID)
	if err != nil {
		return "", "", false, err
	}

	return id, secret, true, nil
}

// DeletePublisherApp deletes the OAuth2 app named "amp-publisher-{orgName}" from Thunder.
func (c *thunderClient) DeletePublisherApp(ctx context.Context, orgName string) (bool, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get system token: %w", err)
	}

	appName := "amp-publisher-" + orgName

	internalID, _, err := c.findApp(ctx, token, appName)
	if err != nil {
		return false, err
	}
	if internalID == "" {
		return false, nil
	}

	return c.deleteApp(ctx, token, internalID)
}

// thunderApp represents the fields we need from a Thunder application response.
type thunderApp struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ClientID string `json:"clientId"`
}

// findApp checks if a Thunder application with the given name exists.
// Returns the internal ID and clientId of the matching app, or empty strings if not found.
func (c *thunderClient) findApp(ctx context.Context, token, appName string) (internalID, clientID string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/applications", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("thunder find app: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("thunder find app returned %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("thunder find app read body: %w", err)
	}

	// Try parsing as a direct array first
	var apps []thunderApp
	if err := json.Unmarshal(body, &apps); err != nil {
		// Try parsing as wrapped object
		var wrapped struct {
			Applications []thunderApp `json:"applications"`
		}
		if err := json.Unmarshal(body, &wrapped); err != nil {
			return "", "", fmt.Errorf("thunder find app decode: %w", err)
		}
		apps = wrapped.Applications
	}

	for _, app := range apps {
		if app.Name == appName {
			return app.ID, app.ClientID, nil
		}
	}
	return "", "", nil
}

// deleteApp deletes a Thunder application by its internal ID.
func (c *thunderClient) deleteApp(ctx context.Context, token, appID string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/applications/"+appID, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("thunder delete app: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("thunder delete app returned %d: %s", resp.StatusCode, string(body))
	}

	return true, nil
}

// createApp creates a new Thunder OAuth2 application.
// Uses the same payload structure as the Thunder bootstrap scripts.
func (c *thunderClient) createApp(ctx context.Context, token, appName, ouID string) (string, string, error) {
	payload := map[string]any{
		"name": appName,
		"ouId": ouID,
		"inboundAuthConfig": []map[string]any{
			{
				"type": "oauth2",
				"config": map[string]any{
					"clientId":                appName,
					"grantTypes":              []string{"client_credentials"},
					"tokenEndpointAuthMethod": "client_secret_basic",
				},
			},
		},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/applications", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("thunder create app: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", "", fmt.Errorf("thunder create app returned %d: %s", resp.StatusCode, string(respBody))
	}

	slog.Info("Thunder created", "status", resp.StatusCode)

	// Thunder may return the app directly or nested — extract clientId and clientSecret
	var result struct {
		ClientID     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
		InboundAuth  []struct {
			Config struct {
				ClientID     string `json:"clientId"`
				ClientSecret string `json:"clientSecret"`
			} `json:"config"`
		} `json:"inboundAuthConfig"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", "", fmt.Errorf("thunder create app decode: %w", err)
	}

	clientID := result.ClientID
	clientSecret := result.ClientSecret

	// Extract from inboundAuthConfig if top-level fields are missing.
	// Thunder returns clientId at top level but clientSecret only inside inboundAuthConfig.
	if len(result.InboundAuth) > 0 {
		if clientID == "" {
			clientID = result.InboundAuth[0].Config.ClientID
		}
		if clientSecret == "" {
			clientSecret = result.InboundAuth[0].Config.ClientSecret
		}
	}

	if clientID == "" {
		return "", "", fmt.Errorf("thunder create app: clientId not found in response: %s", string(respBody))
	}

	return clientID, clientSecret, nil
}
