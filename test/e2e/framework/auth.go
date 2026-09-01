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
// token that are EXPLICITLY requested (it returns requested ∩ allowed), so when
// RBAC_ENABLED=true on Agent Manager, omitting these yields an unscoped token and
// every guarded route returns 403. Scopes are amp:-prefixed to match what Thunder
// (v0.44+) issues and what RBAC checks — i.e. Permission.Scope() in
// agent-manager-service/rbac/permissions.go; the IDP grants only the ones the
// client app is actually allowed, so requesting the superset is safe.
const ampScopes = "amp:agent-identity:create amp:agent-identity:delete amp:agent-identity:read amp:agent-identity:update " +
	"amp:agent-kind:create amp:agent-kind:delete amp:agent-kind:read amp:agent-kind:update " +
	"amp:agent:api-key-manage amp:agent:build amp:agent:create amp:agent:delete " +
	"amp:agent:env-non-production amp:agent:env-production amp:agent:read amp:agent:rollback amp:agent:suspend " +
	"amp:agent:token-manage amp:agent:update amp:catalog:read amp:data-plane:read " +
	"amp:deployment-pipeline:create amp:deployment-pipeline:delete amp:deployment-pipeline:read amp:deployment-pipeline:update " +
	"amp:environment:create amp:environment:delete amp:environment:read amp:environment:update " +
	"amp:evaluator:create amp:evaluator:delete amp:evaluator:read amp:evaluator:update " +
	"amp:gateway:create amp:gateway:delete amp:gateway:read amp:gateway:token-manage amp:gateway:update " +
	"amp:git-secret:create amp:git-secret:delete amp:git-secret:read " +
	"amp:group:create amp:group:delete amp:group:read amp:group:update " +
	"amp:llm-provider-template:create amp:llm-provider-template:delete amp:llm-provider-template:read amp:llm-provider-template:update " +
	"amp:llm-provider:api-key-manage amp:llm-provider:configure-guardrail amp:llm-provider:connect amp:llm-provider:create " +
	"amp:llm-provider:delete amp:llm-provider:deploy amp:llm-provider:read amp:llm-provider:update " +
	"amp:llm-proxy:api-key-manage amp:llm-proxy:create amp:llm-proxy:delete amp:llm-proxy:deploy amp:llm-proxy:read amp:llm-proxy:update " +
	"amp:mcp-server:api-key-manage amp:mcp-server:configure-guardrail amp:mcp-server:connect amp:mcp-server:create amp:mcp-server:delete amp:mcp-server:read amp:mcp-server:update " +
	"amp:monitor:create amp:monitor:delete amp:monitor:execute amp:monitor:read amp:monitor:score-publish amp:monitor:score-read amp:monitor:update " +
	"amp:observability:build-log-read amp:observability:metric-read amp:observability:trace-read amp:observability:log-read " +
	"amp:org:assign-role amp:org:invite-member amp:org:manage-idp amp:org:manage-service-account amp:org:modify-settings amp:org:remove-member amp:org:view " +
	"amp:project:create amp:project:delete amp:project:read amp:project:update amp:repository:read " +
	"amp:role:create amp:role:delete amp:role:read amp:role:update " +
	"amp:scope:create amp:scope:delete amp:scope:read amp:scope:update " +
	"amp:profile:read amp:profile:update-attributes"

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

	// kgateway routes by Host header; ensure it reaches Thunder
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
