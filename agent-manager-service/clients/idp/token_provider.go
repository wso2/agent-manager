// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package idp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/requests"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
)

// TokenProvider manages OAuth2 client credentials tokens with caching
type TokenProvider interface {
	// GetToken returns a valid access token, fetching a new one if needed
	GetToken(ctx context.Context) (string, error)
}

type tokenProvider struct {
	config     config.IDPConfig
	httpClient *http.Client

	mu          sync.RWMutex
	accessToken string
	expiresAt   time.Time
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"` // seconds
	Scope       string `json:"scope,omitempty"`
}

// expiryBuffer is the time before actual expiry when we consider the token expired
// This prevents using tokens that are about to expire during in-flight requests
const expiryBuffer = 30 * time.Second

// NewTokenProvider creates a new token provider with the given configuration
func NewTokenProvider(cfg config.IDPConfig) TokenProvider {
	return &tokenProvider{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// GetToken returns a valid access token, fetching a new one if the cached token is expired
func (p *tokenProvider) GetToken(ctx context.Context) (string, error) {
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

	// Double-check after acquiring write lock (another goroutine might have refreshed it)
	if p.isTokenValid() {
		return p.accessToken, nil
	}

	// Fetch new token
	token, expiresIn, err := p.fetchToken(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to fetch token: %w", err)
	}

	// Cache the token with expiry
	p.accessToken = token
	p.expiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)

	slog.Info("idp: fetched new access token",
		"expires_at", p.expiresAt.Format(time.RFC3339))

	return p.accessToken, nil
}

// isTokenValid checks if the cached token is still valid (not expired)
// Must be called with at least a read lock held
func (p *tokenProvider) isTokenValid() bool {
	if p.accessToken == "" {
		return false
	}
	// Consider token invalid if it expires within the buffer period
	return time.Now().Add(expiryBuffer).Before(p.expiresAt)
}

// fetchToken fetches a new token from the IDP token endpoint using client credentials
func (p *tokenProvider) fetchToken(ctx context.Context) (string, int64, error) {
	req := &requests.HttpRequest{
		Name:   "idp.fetchToken",
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
		return "", 0, fmt.Errorf("idp.fetchToken: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", 0, fmt.Errorf("empty access token in response")
	}
	if tokenResp.ExpiresIn <= 0 {
		return "", 0, fmt.Errorf("invalid expires_in value: %d (must be positive)", tokenResp.ExpiresIn)
	}

	return tokenResp.AccessToken, tokenResp.ExpiresIn, nil
}
