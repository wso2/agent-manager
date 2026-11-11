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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// AMPClient is an HTTP client pre-configured with authentication and the AMP API base URL.
type AMPClient struct {
	httpClient *http.Client
	baseURL    string
	token      string
	cfg        *Config
}

// NewAMPClient creates a new API client. It fetches a scoped OAuth2 token from
// Thunder IDP via the client_credentials grant.
func NewAMPClient(cfg *Config) (*AMPClient, error) {
	return NewAMPClientWithContext(context.Background(), cfg)
}

// NewAMPClientWithContext creates a new API client and propagates cancellation
// while fetching its OAuth2 token.
func NewAMPClientWithContext(ctx context.Context, cfg *Config) (*AMPClient, error) {
	token, err := FetchTokenWithContext(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("fetch auth token: %w", err)
	}

	return &AMPClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    cfg.AMPBaseURL,
		token:      token,
		cfg:        cfg,
	}, nil
}

// Cfg returns the test configuration.
func (c *AMPClient) Cfg() *Config {
	return c.cfg
}

// Token returns the bearer token the client authenticates with.
func (c *AMPClient) Token() string {
	return c.token
}

// Do sends an HTTP request to the AMP API. If body is non-nil it is marshaled to JSON.
// The path is appended to the base URL (e.g., "/api/v1/orgs").
func (c *AMPClient) Do(method, path string, body any) (*http.Response, error) {
	return c.DoWithContext(context.Background(), method, path, body)
}

// DoWithContext sends an HTTP request to the AMP API and propagates cancellation.
func (c *AMPClient) DoWithContext(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reqBody *bytes.Buffer
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(data)
	} else {
		reqBody = &bytes.Buffer{}
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}

// Get sends a GET request to the given path.
func (c *AMPClient) Get(path string) (*http.Response, error) {
	return c.Do(http.MethodGet, path, nil)
}

// GetWithContext sends a GET request and propagates cancellation.
func (c *AMPClient) GetWithContext(ctx context.Context, path string) (*http.Response, error) {
	return c.DoWithContext(ctx, http.MethodGet, path, nil)
}

// Post sends a POST request with a JSON body.
func (c *AMPClient) Post(path string, body any) (*http.Response, error) {
	return c.Do(http.MethodPost, path, body)
}

// PostWithContext sends a POST request and propagates cancellation.
func (c *AMPClient) PostWithContext(ctx context.Context, path string, body any) (*http.Response, error) {
	return c.DoWithContext(ctx, http.MethodPost, path, body)
}

// Put sends a PUT request with a JSON body.
func (c *AMPClient) Put(path string, body any) (*http.Response, error) {
	return c.Do(http.MethodPut, path, body)
}

// PutWithContext sends a PUT request and propagates cancellation.
func (c *AMPClient) PutWithContext(ctx context.Context, path string, body any) (*http.Response, error) {
	return c.DoWithContext(ctx, http.MethodPut, path, body)
}

// Patch sends a PATCH request with a JSON body.
func (c *AMPClient) Patch(path string, body any) (*http.Response, error) {
	return c.Do(http.MethodPatch, path, body)
}

// PatchWithContext sends a PATCH request and propagates cancellation.
func (c *AMPClient) PatchWithContext(ctx context.Context, path string, body any) (*http.Response, error) {
	return c.DoWithContext(ctx, http.MethodPatch, path, body)
}

// Delete sends a DELETE request.
func (c *AMPClient) Delete(path string) (*http.Response, error) {
	return c.Do(http.MethodDelete, path, nil)
}

// DeleteWithContext sends a DELETE request and propagates cancellation.
func (c *AMPClient) DeleteWithContext(ctx context.Context, path string) (*http.Response, error) {
	return c.DoWithContext(ctx, http.MethodDelete, path, nil)
}

// DoRaw sends an authenticated request to an absolute URL (not relative to baseURL).
// Useful for calling other services like agent-manager-observer.
func (c *AMPClient) DoRaw(method, absoluteURL string) (*http.Response, error) {
	return c.DoRawWithContext(context.Background(), method, absoluteURL)
}

// DoRawWithContext sends an authenticated absolute-URL request and propagates cancellation.
func (c *AMPClient) DoRawWithContext(ctx context.Context, method, absoluteURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, absoluteURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	return c.httpClient.Do(req)
}

// GetUnauthenticated sends a GET request without the Authorization header.
// Useful for health checks and public endpoints.
func (c *AMPClient) GetUnauthenticated(path string) (*http.Response, error) {
	return c.GetUnauthenticatedWithContext(context.Background(), path)
}

// GetUnauthenticatedWithContext sends an unauthenticated GET and propagates cancellation.
func (c *AMPClient) GetUnauthenticatedWithContext(ctx context.Context, path string) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create unauthenticated request: %w", err)
	}
	return c.httpClient.Do(req)
}
