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

// FetchToken obtains an OAuth2 access token from the Thunder IDP using the
// client_credentials grant type. It retries on transient errors.
func FetchToken(cfg *Config) (string, error) {
	var lastErr error
	backoff := 2 * time.Second

	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			fmt.Printf("token fetch failed: %v, retrying in %v...\n", lastErr, backoff)
			time.Sleep(backoff)
			if backoff < 15*time.Second {
				backoff = backoff * 3 / 2
			}
		}

		token, err := fetchTokenOnce(cfg)
		if err == nil {
			return token, nil
		}
		lastErr = err
	}

	return "", lastErr
}

func fetchTokenOnce(cfg *Config) (string, error) {
	form := url.Values{
		"grant_type": {"client_credentials"},
	}

	req, err := http.NewRequest(http.MethodPost, cfg.IDPTokenURL, strings.NewReader(form.Encode()))
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
