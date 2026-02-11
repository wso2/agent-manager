//
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
//

// Package auth provides authentication for OpenChoreo API.
// This module is swapped at build time for cloud deployments via replace directive.
package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/requests"
)

// Compile-time check that AuthProvider implements client.AuthProvider
var _ client.AuthProvider = (*AuthProvider)(nil)

// Config contains configuration for the auth provider
type Config struct {
	// TokenURL is the OAuth2 token endpoint
	TokenURL string

	// ClientID is the OAuth2 client ID
	ClientID string

	// ClientSecret is the OAuth2 client secret
	ClientSecret string
}

// AuthProvider implements client.AuthProvider for on-prem deployments using IDP
type AuthProvider struct {
	config     Config
	httpClient requests.HttpClient

	mu          sync.RWMutex
	accessToken string
	expiresAt   time.Time
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Scope       string `json:"scope,omitempty"`
}

// expiryBuffer is the time before actual expiry when we consider the token expired
const expiryBuffer = 30 * time.Second

// NewAuthProvider creates a new auth provider with the given configuration
func NewAuthProvider(cfg Config) client.AuthProvider {
	return &AuthProvider{
		config:     cfg,
		httpClient: requests.NewRetryableHTTPClient(&http.Client{}),
	}
}

// GetToken returns a valid access token, fetching a new one if needed
func (p *AuthProvider) GetToken(ctx context.Context) (string, error) {
	// First, try to get cached token with read lock
	p.mu.RLock()
	if p.isTokenValid() {
		token := p.accessToken
		p.mu.RUnlock()
		return token, nil
	}
	p.mu.RUnlock()

	// Token is expired or not present, acquire write lock and fetch new token
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if p.isTokenValid() {
		return p.accessToken, nil
	}

	slog.Debug("openchoreo auth: fetching new token")

	// Fetch new token
	token, expiresIn, err := p.fetchToken(ctx)
	if err != nil {
		slog.Error("openchoreo auth: failed to fetch token", "error", err)
		return "", fmt.Errorf("failed to fetch token: %w", err)
	}

	// Cache the token with expiry
	p.accessToken = token
	p.expiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)

	slog.Info("openchoreo auth: fetched new access token",
		"expires_at", p.expiresAt.Format(time.RFC3339))

	return p.accessToken, nil
}

// InvalidateToken clears the cached token
func (p *AuthProvider) InvalidateToken() {
	p.mu.Lock()
	defer p.mu.Unlock()
	slog.Debug("openchoreo auth: invalidating cached token")
	p.accessToken = ""
	p.expiresAt = time.Time{}
}

// isTokenValid checks if the cached token is still valid
func (p *AuthProvider) isTokenValid() bool {
	if p.accessToken == "" {
		slog.Debug("openchoreo auth: no cached token")
		return false
	}
	isValid := time.Now().Add(expiryBuffer).Before(p.expiresAt)
	slog.Debug("openchoreo auth: token validation check",
		"is_valid", isValid,
		"expires_at", p.expiresAt.Format(time.RFC3339))
	return isValid
}

// fetchToken fetches a new token using client credentials
func (p *AuthProvider) fetchToken(ctx context.Context) (string, int64, error) {
	req := &requests.HttpRequest{
		Name:   "openchoreo.auth.fetchToken",
		URL:    p.config.TokenURL,
		Method: http.MethodPost,
	}
	req.SetFormData(map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     p.config.ClientID,
		"client_secret": p.config.ClientSecret,
	})

	var tokenResp tokenResponse
	if err := requests.SendRequest(ctx, p.httpClient, req).ScanResponse(&tokenResp, http.StatusOK); err != nil {
		return "", 0, fmt.Errorf("openchoreo.auth.fetchToken: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", 0, fmt.Errorf("empty access token in response")
	}
	if tokenResp.ExpiresIn <= 0 {
		return "", 0, fmt.Errorf("invalid expires_in value: %d", tokenResp.ExpiresIn)
	}

	return tokenResp.AccessToken, tokenResp.ExpiresIn, nil
}
