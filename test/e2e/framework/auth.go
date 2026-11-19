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

package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// ampScopes is the full set of RBAC scopes the e2e suite requests for its
// client_credentials token. Thunder only includes scopes in a client_credentials
// token that are EXPLICITLY requested (it returns requested ∩ allowed), so
// omitting these yields an unscoped token and every guarded route returns 403. Scopes are amp:-prefixed to match what Thunder
// (v0.44+) issues and what RBAC checks — i.e. Permission.Scope() in
// agent-manager-service/rbac/permissions.go and the AI Gateway module's
// modules/aigateway/gwperm; the IDP grants only the ones the
// client app is actually allowed, so requesting the superset is safe.
const ampScopes = "amp:agent:build amp:agent:create amp:agent:delete amp:agent:read amp:agent:suspend " +
	"amp:agent:token-manage amp:agent:update " +
	"amp:environment:create amp:environment:delete amp:environment:read amp:environment:update " +
	"amp:group:create amp:group:delete amp:group:read amp:group:update " +
	"amp:events:read amp:events:manage " +
	"amp:org:assign-role amp:org:invite-member amp:org:manage-idp amp:org:manage-service-account amp:org:modify-settings amp:org:remove-member amp:org:view " +
	"amp:project:create amp:project:delete amp:project:read amp:project:update " +
	"amp:role:create amp:role:delete amp:role:read amp:role:update " +
	"amp:profile:read amp:profile:update-attributes " +
	// The AI Gateway module's permissions.
	"amp:ai-gateway:gateways.create amp:ai-gateway:gateways.read amp:ai-gateway:gateways.update amp:ai-gateway:gateways.delete amp:ai-gateway:gateways.token-manage " +
	"amp:ai-gateway:llm-providers.create amp:ai-gateway:llm-providers.read amp:ai-gateway:llm-providers.update amp:ai-gateway:llm-providers.delete " +
	"amp:ai-gateway:llm-providers.configure-guardrail amp:ai-gateway:llm-providers.connect amp:ai-gateway:llm-providers.deploy amp:ai-gateway:llm-providers.api-key-manage " +
	"amp:ai-gateway:llm-provider-templates.create amp:ai-gateway:llm-provider-templates.read amp:ai-gateway:llm-provider-templates.update amp:ai-gateway:llm-provider-templates.delete " +
	"amp:ai-gateway:llm-proxies.create amp:ai-gateway:llm-proxies.read amp:ai-gateway:llm-proxies.update amp:ai-gateway:llm-proxies.delete " +
	"amp:ai-gateway:llm-proxies.deploy amp:ai-gateway:llm-proxies.api-key-manage " +
	"amp:ai-gateway:mcp-servers.create amp:ai-gateway:mcp-servers.read amp:ai-gateway:mcp-servers.update amp:ai-gateway:mcp-servers.delete " +
	"amp:ai-gateway:mcp-servers.configure-guardrail amp:ai-gateway:mcp-servers.connect amp:ai-gateway:mcp-servers.api-key-manage " +
	"amp:ai-gateway:scopes.create amp:ai-gateway:scopes.read amp:ai-gateway:scopes.update amp:ai-gateway:scopes.delete " +
	"amp:ai-gateway:catalog.read " +
	"amp:ai-gateway:agent-identities.create amp:ai-gateway:agent-identities.read amp:ai-gateway:agent-identities.update amp:ai-gateway:agent-identities.delete " +
	"amp:ai-gateway:agent-api-keys.manage amp:ai-gateway:deploy.non-production amp:ai-gateway:deploy.production"

// FetchToken obtains an OAuth2 access token from the Thunder IDP using the
// client_credentials grant type, carrying the full ampScopes superset. It
// retries on transient errors.
func FetchToken(cfg *Config) (string, error) {
	return FetchTokenWithContext(context.Background(), cfg)
}

// FetchTokenWithContext obtains a full-privilege OAuth2 access token and
// propagates cancellation through token requests and retry delays.
func FetchTokenWithContext(ctx context.Context, cfg *Config) (string, error) {
	return fetchTokenWithRetry(ctx, cfg, ampScopes)
}

// fetchTokenWithRetry fetches a token for the given space-delimited scope
// string, retrying on transient errors. An empty scope string requests no
// scopes at all, which yields an unscoped token.
func fetchTokenWithRetry(ctx context.Context, cfg *Config, scope string) (string, error) {
	var lastErr error
	backoff := 2 * time.Second

	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			fmt.Printf("token fetch failed: %v, retrying in %v...\n", lastErr, backoff)
			select {
			case <-ctx.Done():
				return "", fmt.Errorf("wait to retry token request: %w", ctx.Err())
			case <-time.After(backoff):
			}
			if backoff < 15*time.Second {
				backoff = backoff * 3 / 2
			}
		}

		token, err := fetchTokenOnce(ctx, cfg, scope)
		if err == nil {
			return token, nil
		}
		lastErr = err
	}

	return "", lastErr
}

func fetchTokenOnce(ctx context.Context, cfg *Config, scope string) (string, error) {
	form := url.Values{
		"grant_type": {"client_credentials"},
	}
	// Request the scopes explicitly — Thunder only embeds requested scopes in
	// a client_credentials token (returns requested ∩ allowed). Without this
	// the token is unscoped and RBAC-guarded routes return 403. An empty scope
	// is deliberate for the security suite's unscoped-token probe, and the
	// parameter is omitted entirely so Thunder does not see "scope=".
	if scope != "" {
		form.Set("scope", scope)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.IDPTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// amp-api-client uses client_secret_basic: credentials in Authorization header.
	req.SetBasicAuth(cfg.IDPClientID, cfg.IDPClientSecret)

	// Send the token URL's own host so Thunder sees its public name.
	parsedURL, err := url.Parse(cfg.IDPTokenURL)
	if err == nil && parsedURL.Hostname() != "" {
		req.Host = parsedURL.Host
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}

	if tok.AccessToken == "" {
		return "", fmt.Errorf("empty access_token in response: %s", string(body))
	}

	return tok.AccessToken, nil
}
