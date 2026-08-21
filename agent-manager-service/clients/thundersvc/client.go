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
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"golang.org/x/sync/singleflight"
)

// ThunderClient encapsulates the Thunder API calls needed to create OAuth2 applications.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg clientmocks -out ../clientmocks/thunder_client_fake.go . ThunderClient:ThunderClientMock
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

	// EnsureApp generalizes EnsurePublisherApp for a differently-named app (e.g. amp-scheduler-<ouID>).
	EnsureApp(ctx context.Context, appName, orgUUID string) (clientID, clientSecret string, created bool, err error)

	// DeleteApp deletes the OAuth2 app named appName from Thunder.
	// Returns true if the app was found and deleted, false if it didn't exist.
	DeleteApp(ctx context.Context, appName string) (bool, error)

	// RegenerateAppClientSecret regenerates the OAuth2 client secret for the app named appName.
	RegenerateAppClientSecret(ctx context.Context, appName string) (clientSecret string, err error)

	// CreateAgentIdentity creates a client_credentials AgentID via Thunder's /agents
	// endpoint (a distinct resource from /applications — see agent_client.go).
	// Idempotent: a 409 name conflict is resolved by looking up the existing agent;
	// created is false and clientSecret is empty in that case (Thunder only returns
	// a secret at creation time).
	CreateAgentIdentity(ctx context.Context, ouID, name, owner string) (thunderAgentID, clientID, clientSecret string, created bool, err error)

	// RegenerateAgentSecret generates and applies a new client secret for the
	// AgentID identified by thunderAgentID. Returns the new secret.
	RegenerateAgentSecret(ctx context.Context, thunderAgentID string) (clientSecret string, err error)

	// DeleteAgentIdentity deletes the AgentID by its Thunder internal ID.
	// Returns false (no error) if it did not exist.
	DeleteAgentIdentity(ctx context.Context, thunderAgentID string) (bool, error)

	// GetDefaultOUID returns the default organization unit ID from Thunder.
	GetDefaultOUID(ctx context.Context) (string, error)
}

type thunderClient struct {
	baseURL      string // Thunder API base URL (e.g. http://thunder:8090)
	clientID     string // OAuth2 client ID of the system app (with Administrator role)
	clientSecret string // OAuth2 client secret of the system app
	// systemResource is the resource indicator (RFC 8707) sent with the client_credentials
	// token request so the issued token is scoped to Thunder's built-in System resource
	// server (identifier "<issuer>/mcp"), which owns the "system" permission. Admin APIs
	// (/users, /applications, ...) require that permission, granted only when the token
	// is scoped to the System RS. Without this, the server-wide default resource server
	// (amp-resource-server) resolves the request instead, the "system" scope is dropped,
	// and every admin call fails with AUTH-4030 Forbidden.
	systemResource string
	httpClient     *http.Client

	mu          sync.RWMutex
	cachedToken string
	tokenExpiry time.Time
	tokenSfg    singleflight.Group // deduplicates concurrent token fetches

	ensureResourceServerMus sync.Map // proxyHandle -> *sync.Mutex; serializes each proxy's resource-server check-then-create
}

const httpClientTimeout = 30 * time.Second

// SystemResourceIdentifier returns the System resource server identifier ("<issuer>/mcp")
// for a Thunder instance reachable at the given public/issuer URL. This mirrors the
// identifier set by the bootstrap resource (deployments/.../70-fix-thunder-system-rs-identifier.yaml
// and add-environment-thunder.sh's render_system_rs_identifier_fix), both of which use
// "<publicUrl>/mcp".
func SystemResourceIdentifier(issuerURL string) string {
	return strings.TrimRight(issuerURL, "/") + "/mcp"
}

// NewThunderClient creates a new Thunder API client.
// clientID/clientSecret are the OAuth2 credentials for the system app
// that has the Administrator role assigned (created at bootstrap).
// baseURL is the platform Thunder public/issuer URL, so it also derives the System
// resource indicator used to obtain system-scoped tokens.
func NewThunderClient(baseURL, clientID, clientSecret string) ThunderClient {
	return &thunderClient{
		baseURL:        baseURL,
		clientID:       clientID,
		clientSecret:   clientSecret,
		systemResource: SystemResourceIdentifier(baseURL),
		httpClient:     &http.Client{Timeout: httpClientTimeout},
	}
}

// NewThunderClientWithDialOverride creates a Thunder API client that connects to
// resolveToHost instead of baseURL's own host, while every request still carries
// baseURL's host as the HTTP Host header — so Kgateway's host-based routing still
// selects the right env-Thunder backend. Used by EnvThunderResolver when the
// direct base URL (cluster-internal DNS or the public ingress hostname) isn't
// dialable from the caller's network, e.g. a docker-compose container that can't
// resolve either — and, for platform Thunder, when agent-manager-service itself
// runs in-cluster while Thunder's public/issuer URL only resolves on the host
// machine. An empty resolveToHost behaves exactly like NewThunderClient.
//
// systemResource is passed explicitly (rather than derived from baseURL) because the
// selected baseURL may be a cluster-internal DNS name, which differs from the
// instance's issuer. The System RS identifier is always "<issuer>/mcp", so callers
// derive it from the instance's issuer URL, not the dialable base URL.
func NewThunderClientWithDialOverride(baseURL, clientID, clientSecret, resolveToHost, systemResource string) ThunderClient {
	httpClient := &http.Client{Timeout: httpClientTimeout}
	if resolveToHost != "" {
		httpClient.Transport = &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, resolveToHost)
			},
		}
		// Thunder never terminates TLS itself — it always runs its own listener
		// plain HTTP and relies on whatever fronts it publicly (Caddy, an
		// HTTPRoute, ...) to add TLS. baseURL may still be an https:// public/
		// issuer URL (required for the resource-identifier derivation above), but
		// the dialed connection itself must speak plain HTTP, or the TLS
		// handshake this client attempts gets rejected by the plain-HTTP backend
		// ("server gave HTTP response to HTTPS client"). Only the scheme changes
		// here — baseURL's host is untouched, so the Host header Kgateway's
		// host-based routing relies on is unaffected.
		baseURL = "http://" + strings.TrimPrefix(strings.TrimPrefix(baseURL, "https://"), "http://")
	}
	return &thunderClient{
		baseURL:        baseURL,
		clientID:       clientID,
		clientSecret:   clientSecret,
		systemResource: systemResource,
		httpClient:     httpClient,
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
	// Scope the token to Thunder's System resource server so the "system" permission is
	// actually granted (see systemResource field). Omitting this drops the scope and the
	// admin APIs reject the call with AUTH-4030.
	if c.systemResource != "" {
		data.Set("resource", c.systemResource)
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
	return c.EnsureApp(ctx, "amp-publisher-"+orgName, orgUUID)
}

// DeletePublisherApp deletes the OAuth2 app named "amp-publisher-{orgName}" from Thunder.
func (c *thunderClient) DeletePublisherApp(ctx context.Context, orgName string) (bool, error) {
	return c.DeleteApp(ctx, "amp-publisher-"+orgName)
}

// RegenerateClientSecret regenerates the OAuth2 client secret for the publisher app.
func (c *thunderClient) RegenerateClientSecret(ctx context.Context, orgName string) (string, error) {
	return c.RegenerateAppClientSecret(ctx, "amp-publisher-"+orgName)
}

// EnsureApp creates or returns an existing OAuth2 app named appName in Thunder.
// orgUUID is the Thunder organization unit UUID. If empty, the default OU is used.
func (c *thunderClient) EnsureApp(ctx context.Context, appName, orgUUID string) (clientID, clientSecret string, created bool, err error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return "", "", false, fmt.Errorf("failed to get system token: %w", err)
	}

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

// DeleteApp deletes the OAuth2 app named appName from Thunder.
func (c *thunderClient) DeleteApp(ctx context.Context, appName string) (bool, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get system token: %w", err)
	}

	internalID, _, err := c.findApp(ctx, token, appName)
	if err != nil {
		return false, err
	}
	if internalID == "" {
		return false, nil
	}

	return c.deleteApp(ctx, token, internalID)
}

// RegenerateAppClientSecret regenerates the OAuth2 client secret for the app named appName.
func (c *thunderClient) RegenerateAppClientSecret(ctx context.Context, appName string) (string, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get system token: %w", err)
	}

	internalID, _, err := c.findApp(ctx, token, appName)
	if err != nil {
		return "", err
	}
	if internalID == "" {
		return "", fmt.Errorf("thunder app %s not found, cannot regenerate secret", appName)
	}

	return c.regenerateSecret(ctx, token, internalID)
}

// regenerateSecret regenerates the client secret for a Thunder application.
func (c *thunderClient) regenerateSecret(ctx context.Context, token, appID string) (string, error) {
	secret, err := c.regenerateResourceSecret(ctx, token, "applications", appID)
	if err != nil {
		return "", err
	}
	logger.GetLogger(ctx).Info("Thunder client secret regenerated", "app_id", appID)
	return secret, nil
}

// regenerateResourceSecret regenerates the OAuth2 client secret for a Thunder
// application or agent (resource is "applications" or "agents"): GET the
// full payload, inject a new random secret, then PUT it back. Thunder's PUT
// only auto-regenerates a secret when transitioning from a non-secret auth
// method, which doesn't apply to an already-secret-based client_credentials
// resource — so the new value is explicitly supplied.
func (c *thunderClient) regenerateResourceSecret(ctx context.Context, token, resource, id string) (string, error) {
	getBody, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/"+resource+"/"+id, token, nil)
	if err != nil {
		return "", fmt.Errorf("thunder get %s for secret regeneration: %w", resource, err)
	}

	// Decode into a generic map so we can inject the new secret without losing fields
	var item map[string]any
	if err := json.Unmarshal(getBody, &item); err != nil {
		return "", fmt.Errorf("thunder get %s decode: %w", resource, err)
	}

	newSecret, err := generateRandomSecret()
	if err != nil {
		return "", fmt.Errorf("failed to generate client secret: %w", err)
	}
	if err := setInboundClientSecret(item, newSecret); err != nil {
		return "", fmt.Errorf("failed to set client secret in %s payload: %w", resource, err)
	}
	delete(item, "id") // Thunder expects id in the URL, not the body

	putBody, err := c.doRequest(ctx, http.MethodPut, c.baseURL+"/"+resource+"/"+id, token, item)
	if err != nil {
		return "", fmt.Errorf("thunder put %s for secret regeneration: %w", resource, err)
	}

	var result struct {
		InboundAuth []struct {
			Config struct {
				ClientSecret string `json:"clientSecret"`
			} `json:"config"`
		} `json:"inboundAuthConfig"`
	}
	if err := json.Unmarshal(putBody, &result); err != nil {
		return "", fmt.Errorf("thunder put %s response decode: %w", resource, err)
	}
	if len(result.InboundAuth) == 0 || result.InboundAuth[0].Config.ClientSecret == "" {
		return "", fmt.Errorf("thunder put %s response missing clientSecret", resource)
	}

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

// thunderNamedResource represents the fields needed from one page of a
// Thunder /applications or /agents list response.
type thunderNamedResource struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ClientID string `json:"clientId"`
}

// findApp checks if a Thunder application with the given name exists.
// Returns the internal ID and clientId of the matching app, or empty strings if not found.
func (c *thunderClient) findApp(ctx context.Context, token, appName string) (internalID, clientID string, err error) {
	return c.findResourceByName(ctx, token, "applications", appName)
}

// findResourceByName checks if a Thunder application or agent (resource is
// "applications" or "agents") with the given name exists, paginating through
// results since the API does not support filtering. Returns the internal ID
// and clientId of the matching resource, or empty strings if not found.
func (c *thunderClient) findResourceByName(ctx context.Context, token, resource, name string) (internalID, clientID string, err error) {
	const (
		pageSize = 100
		maxPages = 100 // safety cap: 10k items max
	)
	for page := 0; page < maxPages; page++ {
		offset := page * pageSize
		items, err := c.listResourcePage(ctx, token, resource, offset, pageSize)
		if err != nil {
			return "", "", err
		}
		for _, item := range items {
			if item.Name == name {
				return item.ID, item.ClientID, nil
			}
		}
		if len(items) < pageSize {
			return "", "", nil
		}
	}
	return "", "", fmt.Errorf("thunder list %s exceeded %d pages looking for %s", resource, maxPages, name)
}

// listResourcePage fetches a single page of resource ("applications" or
// "agents"), tolerating both a raw JSON array response and an object with a
// same-named array field alongside other fields (e.g. {"agents": [...],
// "totalResults": N, "count": N}) — only that one field is decoded, so
// unrelated fields of a different shape don't break parsing.
func (c *thunderClient) listResourcePage(ctx context.Context, token, resource string, offset, limit int) ([]thunderNamedResource, error) {
	reqURL := fmt.Sprintf("%s/%s?offset=%d&limit=%d", c.baseURL, resource, offset, limit)
	body, err := c.doRequest(ctx, http.MethodGet, reqURL, token, nil)
	if err != nil {
		return nil, fmt.Errorf("thunder list %s: %w", resource, err)
	}

	// Try parsing as a direct array first
	var items []thunderNamedResource
	if err := json.Unmarshal(body, &items); err == nil {
		return items, nil
	}
	// Fall back to a wrapped object; only decode the resource-named field.
	var wrapped map[string]json.RawMessage
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, fmt.Errorf("thunder list %s decode: %w", resource, err)
	}
	raw, ok := wrapped[resource]
	if !ok {
		return nil, nil
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("thunder list %s decode: %w", resource, err)
	}
	return items, nil
}

// deleteApp deletes a Thunder application by its internal ID.
func (c *thunderClient) deleteApp(ctx context.Context, token, appID string) (bool, error) {
	return c.deleteThunderResource(ctx, token, "applications", appID)
}

// deleteThunderResource deletes a Thunder application or agent (resource is
// "applications" or "agents") by its internal ID. Returns false (no error)
// if it did not exist — deletion is idempotent.
func (c *thunderClient) deleteThunderResource(ctx context.Context, token, resource, id string) (bool, error) {
	_, err := c.doRequest(ctx, http.MethodDelete, c.baseURL+"/"+resource+"/"+id, token, nil)
	if err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("thunder delete %s: %w", resource, err)
	}

	return true, nil
}

// createApp creates a new Thunder OAuth2 application.
// Uses the same payload structure as the Thunder bootstrap scripts.
func (c *thunderClient) createApp(ctx context.Context, token, appName, ouID string) (string, string, error) {
	payload := map[string]any{
		"name": appName,
		"ouId": ouID,
		"type": "m2m", // client_credentials only, no interactive login
		"inboundAuthConfig": []map[string]any{
			{
				"type": "oauth2",
				"config": map[string]any{
					"clientId":                appName,
					"grantTypes":              []string{"client_credentials"},
					"tokenEndpointAuthMethod": "client_secret_basic",
					// Thunder only embeds ouId/ouHandle as token claims when a client's own
					// clientConfig.attributes opts in — without this, RequireOrgMatch in
					// agent-manager-service rejects this app's tokens as "missing ou
					// identity in token" even though ouId above is set correctly.
					"token": map[string]any{
						"accessToken": map[string]any{
							"clientConfig": map[string]any{
								"attributes": []string{"ouId", "ouHandle"},
							},
						},
					},
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

	logger.GetLogger(ctx).Info("Thunder app created", "app_name", appName, "status", resp.StatusCode)

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
