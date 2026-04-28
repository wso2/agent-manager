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

package observer

import (
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

// expiryBuffer is the time before actual expiry when the token is considered stale.
const expiryBuffer = 30 * time.Second

// AuthProvider manages OAuth2 client credentials tokens for the observer service.
// It is safe for concurrent use.
type AuthProvider struct {
	tokenURL     string
	clientID     string
	clientSecret string
	httpClient   *http.Client

	mu          sync.RWMutex
	accessToken string
	expiresAt   time.Time
	usePostAuth bool
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

type tokenErrorResponse struct {
	Error string `json:"error"`
}

// NewAuthProvider creates a new AuthProvider with the given credentials.
func NewAuthProvider(tokenURL, clientID, clientSecret string) *AuthProvider {
	return &AuthProvider{
		tokenURL:     tokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// GetToken returns a valid access token, fetching a new one if the cached token
// is expired or absent.
func (p *AuthProvider) GetToken(ctx context.Context) (string, error) {
	p.mu.RLock()
	if p.isTokenValid() {
		token := p.accessToken
		p.mu.RUnlock()
		return token, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock.
	if p.isTokenValid() {
		return p.accessToken, nil
	}

	slog.Debug("observer auth: fetching new token")

	token, expiresIn, err := p.fetchToken(ctx)
	if err != nil {
		return "", fmt.Errorf("observer auth: failed to fetch token: %w", err)
	}

	ttl := time.Duration(expiresIn) * time.Second
	buffer := expiryBuffer
	if ttl <= 2*expiryBuffer {
		buffer = ttl / 2
	}
	p.accessToken = token
	p.expiresAt = time.Now().Add(ttl - buffer)

	slog.Info("observer auth: fetched new access token",
		"expires_at", p.expiresAt.Format(time.RFC3339))

	return p.accessToken, nil
}

// InvalidateToken clears the cached token, forcing a refresh on the next call to GetToken.
func (p *AuthProvider) InvalidateToken() {
	p.mu.Lock()
	defer p.mu.Unlock()
	slog.Debug("observer auth: invalidating cached token")
	p.accessToken = ""
	p.expiresAt = time.Time{}
}

func (p *AuthProvider) isTokenValid() bool {
	return p.accessToken != "" && time.Now().Before(p.expiresAt)
}

func (p *AuthProvider) fetchToken(ctx context.Context) (string, int64, error) {
	if p.usePostAuth {
		return p.doTokenRequest(ctx, true)
	}

	token, expiresIn, statusCode, body, err := p.executeTokenRequest(ctx, false)
	if err != nil {
		return "", 0, err
	}

	if statusCode == http.StatusOK {
		return token, expiresIn, nil
	}

	if isUnauthorizedClientError(statusCode, body) {
		slog.Info("observer auth: client_secret_basic rejected, falling back to client_secret_post",
			"status_code", statusCode)
		tok, exp, retryErr := p.doTokenRequest(ctx, true)
		if retryErr != nil {
			return "", 0, retryErr
		}
		p.usePostAuth = true
		return tok, exp, nil
	}

	return "", 0, fmt.Errorf("token endpoint returned %d: %s", statusCode, string(body))
}

func (p *AuthProvider) doTokenRequest(ctx context.Context, postAuth bool) (string, int64, error) {
	token, expiresIn, statusCode, body, err := p.executeTokenRequest(ctx, postAuth)
	if err != nil {
		return "", 0, err
	}
	if statusCode != http.StatusOK {
		return "", 0, fmt.Errorf("token endpoint returned %d: %s", statusCode, string(body))
	}
	return token, expiresIn, nil
}

func (p *AuthProvider) executeTokenRequest(ctx context.Context, postAuth bool) (string, int64, int, []byte, error) {
	form := url.Values{
		"grant_type": {"client_credentials"},
	}
	if postAuth {
		form.Set("client_id", p.clientID)
		form.Set("client_secret", p.clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, 0, nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if !postAuth {
		req.SetBasicAuth(p.clientID, p.clientSecret)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", 0, 0, nil, fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, 0, nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, resp.StatusCode, body, nil
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", 0, 0, nil, fmt.Errorf("decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", 0, 0, nil, fmt.Errorf("empty access token in response")
	}
	if tr.ExpiresIn <= 0 {
		return "", 0, 0, nil, fmt.Errorf("invalid expires_in value: %d", tr.ExpiresIn)
	}

	return tr.AccessToken, tr.ExpiresIn, resp.StatusCode, body, nil
}

func isUnauthorizedClientError(statusCode int, body []byte) bool {
	if statusCode != http.StatusBadRequest {
		return false
	}
	var errResp tokenErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return false
	}
	return errResp.Error == "unauthorized_client"
}
