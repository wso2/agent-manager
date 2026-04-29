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

// NewAMPClient creates a new API client. It fetches an OAuth2 token from Thunder IDP
// and configures the HTTP client for use throughout the test suite.
func NewAMPClient(cfg *Config) (*AMPClient, error) {
	token, err := FetchToken(cfg)
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

// Do sends an HTTP request to the AMP API. If body is non-nil it is marshaled to JSON.
// The path is appended to the base URL (e.g., "/api/v1/orgs").
func (c *AMPClient) Do(method, path string, body any) (*http.Response, error) {
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
	req, err := http.NewRequest(method, url, reqBody)
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

// Post sends a POST request with a JSON body.
func (c *AMPClient) Post(path string, body any) (*http.Response, error) {
	return c.Do(http.MethodPost, path, body)
}

// Put sends a PUT request with a JSON body.
func (c *AMPClient) Put(path string, body any) (*http.Response, error) {
	return c.Do(http.MethodPut, path, body)
}

// Patch sends a PATCH request with a JSON body.
func (c *AMPClient) Patch(path string, body any) (*http.Response, error) {
	return c.Do(http.MethodPatch, path, body)
}

// Delete sends a DELETE request.
func (c *AMPClient) Delete(path string) (*http.Response, error) {
	return c.Do(http.MethodDelete, path, nil)
}

// DoRaw sends an authenticated request to an absolute URL (not relative to baseURL).
// Useful for calling other services like traces-observer-service.
func (c *AMPClient) DoRaw(method, absoluteURL string) (*http.Response, error) {
	req, err := http.NewRequest(method, absoluteURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	return c.httpClient.Do(req)
}

// GetUnauthenticated sends a GET request without the Authorization header.
// Useful for health checks and public endpoints.
func (c *AMPClient) GetUnauthenticated(path string) (*http.Response, error) {
	url := c.baseURL + path
	return c.httpClient.Get(url)
}
