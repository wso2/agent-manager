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
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
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

	// RegenerateClientSecret regenerates the OAuth2 client secret for the app
	// named "amp-publisher-{orgName}". Returns the new client secret.
	// Use this when the app exists but the secret has been lost (e.g. not in the local DB).
	RegenerateClientSecret(ctx context.Context, orgName string) (clientSecret string, err error)
}

type thunderClient struct {
	baseURL      string // Thunder API base URL (e.g. http://thunder:8090)
	clientID     string // OAuth2 client ID of the system app (with Administrator role)
	clientSecret string // OAuth2 client secret of the system app
	httpClient   *http.Client

	mu          sync.RWMutex
	cachedToken string
	tokenExpiry time.Time
	tokenSfg    singleflight.Group // deduplicates concurrent token fetches
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
// Uses RWMutex for the cache-hit fast path and singleflight for the fetch
// so concurrent callers share a single network round-trip instead of blocking.
func (c *thunderClient) getSystemToken(ctx context.Context) (string, error) {
	// Fast path: read lock for cache hit
	c.mu.RLock()
	if c.cachedToken != "" && time.Now().Before(c.tokenExpiry) {
		token := c.cachedToken
		c.mu.RUnlock()
		return token, nil
	}
	c.mu.RUnlock()

	// Slow path: singleflight ensures only one goroutine fetches the token
	result, err, _ := c.tokenSfg.Do("system-token", func() (any, error) {
		// Re-check cache inside singleflight (another goroutine may have populated it)
		c.mu.RLock()
		if c.cachedToken != "" && time.Now().Before(c.tokenExpiry) {
			token := c.cachedToken
			c.mu.RUnlock()
			return token, nil
		}
		c.mu.RUnlock()

		token, expiresIn, err := c.fetchSystemToken(ctx)
		if err != nil {
			return nil, err
		}

		c.mu.Lock()
		c.cachedToken = token
		const skew = 30
		if expiresIn > skew {
			c.tokenExpiry = time.Now().Add(time.Duration(expiresIn-skew) * time.Second)
		} else {
			c.tokenExpiry = time.Now().Add(time.Duration(max(expiresIn, 1)) * time.Second)
		}
		c.mu.Unlock()

		return token, nil
	})
	if err != nil {
		return "", err
	}
	return result.(string), nil
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

// RegenerateClientSecret regenerates the OAuth2 client secret for the publisher app.
func (c *thunderClient) RegenerateClientSecret(ctx context.Context, orgName string) (string, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get system token: %w", err)
	}

	appName := "amp-publisher-" + orgName

	internalID, _, err := c.findApp(ctx, token, appName)
	if err != nil {
		return "", err
	}
	if internalID == "" {
		return "", fmt.Errorf("thunder app %s not found, cannot regenerate secret", appName)
	}

	return c.regenerateSecret(ctx, token, internalID)
}

// regenerateSecret regenerates the client secret by fetching the app, generating a new
// secret, and PUTting the full app payload back to Thunder with the updated secret.
func (c *thunderClient) regenerateSecret(ctx context.Context, token, appID string) (string, error) {
	// GET the existing app to get the full payload
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/applications/"+appID, nil)
	if err != nil {
		return "", err
	}
	getReq.Header.Set("Authorization", "Bearer "+token)

	getResp, err := c.httpClient.Do(getReq)
	if err != nil {
		return "", fmt.Errorf("thunder get app for secret regeneration: %w", err)
	}
	defer func() { _ = getResp.Body.Close() }()

	getBody, _ := io.ReadAll(getResp.Body)
	if getResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("thunder get app returned %d: %s", getResp.StatusCode, string(getBody))
	}

	// Decode into a generic map so we can inject the new secret without losing fields
	var app map[string]any
	if err := json.Unmarshal(getBody, &app); err != nil {
		return "", fmt.Errorf("thunder get app decode: %w", err)
	}

	// Generate a new random client secret
	newSecret, err := generateRandomSecret()
	if err != nil {
		return "", fmt.Errorf("failed to generate client secret: %w", err)
	}

	// Inject the new secret into inboundAuthConfig[0].config.clientSecret
	if err := setInboundClientSecret(app, newSecret); err != nil {
		return "", fmt.Errorf("failed to set client secret in app payload: %w", err)
	}

	// Remove the top-level "id" field — Thunder expects it in the URL, not the body
	delete(app, "id")

	// PUT the updated app back
	putBody, _ := json.Marshal(app)
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/applications/"+appID, bytes.NewReader(putBody))
	if err != nil {
		return "", err
	}
	putReq.Header.Set("Authorization", "Bearer "+token)
	putReq.Header.Set("Content-Type", "application/json")

	putResp, err := c.httpClient.Do(putReq)
	if err != nil {
		return "", fmt.Errorf("thunder put app for secret regeneration: %w", err)
	}
	defer func() { _ = putResp.Body.Close() }()

	putRespBody, _ := io.ReadAll(putResp.Body)
	if putResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("thunder put app returned %d: %s", putResp.StatusCode, string(putRespBody))
	}

	// Verify the response contains the new secret
	var result struct {
		InboundAuth []struct {
			Config struct {
				ClientSecret string `json:"clientSecret"`
			} `json:"config"`
		} `json:"inboundAuthConfig"`
	}
	if err := json.Unmarshal(putRespBody, &result); err != nil {
		return "", fmt.Errorf("thunder put app response decode: %w", err)
	}

	if len(result.InboundAuth) == 0 || result.InboundAuth[0].Config.ClientSecret == "" {
		return "", fmt.Errorf("thunder put app response missing clientSecret")
	}

	slog.Info("Thunder client secret regenerated", "appID", appID)
	return result.InboundAuth[0].Config.ClientSecret, nil
}

// setInboundClientSecret sets the clientSecret in inboundAuthConfig[0].config.
func setInboundClientSecret(app map[string]any, secret string) error {
	inbound, ok := app["inboundAuthConfig"].([]any)
	if !ok || len(inbound) == 0 {
		return fmt.Errorf("inboundAuthConfig missing or empty")
	}
	entry, ok := inbound[0].(map[string]any)
	if !ok {
		return fmt.Errorf("inboundAuthConfig[0] is not an object")
	}
	cfg, ok := entry["config"].(map[string]any)
	if !ok {
		return fmt.Errorf("inboundAuthConfig[0].config is not an object")
	}
	cfg["clientSecret"] = secret
	return nil
}

// generateRandomSecret generates a 64-character cryptographically secure random
// string using crypto/rand (384-bit entropy, URL-safe base64 encoding).
func generateRandomSecret() (string, error) {
	b := make([]byte, 48) // 48 bytes = 384 bits of entropy
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(b), nil
}

// thunderApp represents the fields we need from a Thunder application response.
type thunderApp struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ClientID string `json:"clientId"`
}

// findApp checks if a Thunder application with the given name exists.
// Returns the internal ID and clientId of the matching app, or empty strings if not found.
// Paginates through results since the API does not support filtering.
func (c *thunderClient) findApp(ctx context.Context, token, appName string) (internalID, clientID string, err error) {
	const (
		pageSize = 100
		maxPages = 100 // safety cap: 10k apps max
	)
	for page := 0; page < maxPages; page++ {
		offset := page * pageSize
		apps, err := c.listAppsPage(ctx, token, offset, pageSize)
		if err != nil {
			return "", "", err
		}
		for _, app := range apps {
			if app.Name == appName {
				return app.ID, app.ClientID, nil
			}
		}
		if len(apps) < pageSize {
			return "", "", nil
		}
	}
	return "", "", fmt.Errorf("thunder list apps exceeded %d pages looking for %s", maxPages, appName)
}

// listAppsPage fetches a single page of Thunder applications.
func (c *thunderClient) listAppsPage(ctx context.Context, token string, offset, limit int) ([]thunderApp, error) {
	reqURL := fmt.Sprintf("%s/applications?offset=%d&limit=%d", c.baseURL, offset, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("thunder list apps: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("thunder list apps returned %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("thunder list apps read body: %w", err)
	}

	// Try parsing as a direct array first
	var apps []thunderApp
	if err := json.Unmarshal(body, &apps); err != nil {
		// Try parsing as wrapped object
		var wrapped struct {
			Applications []thunderApp `json:"applications"`
		}
		if err := json.Unmarshal(body, &wrapped); err != nil {
			return nil, fmt.Errorf("thunder list apps decode: %w", err)
		}
		apps = wrapped.Applications
	}
	return apps, nil
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

	slog.Info("Thunder app created", "appName", appName, "status", resp.StatusCode)

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
