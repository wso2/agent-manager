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

package gwsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/utils/ssrf"
)

const (
	oidcDiscoveryTimeout = 10 * time.Second
	oidcWellKnownPath    = "/.well-known/openid-configuration"
	maxOidcDiscoveryBody = 1 << 20 // 1 MiB
)

// DiscoverOIDC fetches the OpenID Connect discovery document for rawURL and
// returns its issuer and jwks_uri. rawURL may be an issuer base URL or a full
// .well-known/openid-configuration URL. The fetch is SSRF-protected (the host
// must resolve to a public IP). Client-fixable failures — a bad or unreachable
// URL, or a malformed document — wrap utils.ErrInvalidURL so the controller can
// map them to HTTP 400.
func (s *PlatformGatewayService) DiscoverOIDC(ctx context.Context, rawURL string) (string, string, error) {
	discoveryURL, err := buildOidcDiscoveryURL(rawURL)
	if err != nil {
		return "", "", err
	}
	if err := ssrf.ValidateURL(ctx, discoveryURL); err != nil {
		return "", "", fmt.Errorf("%w: %w", utils.ErrInvalidURL, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("%w: %w", utils.ErrInvalidURL, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := ssrf.NewClient(oidcDiscoveryTimeout).Do(req)
	if err != nil {
		// Do not surface the raw error: it may leak internal probe results.
		return "", "", fmt.Errorf("%w: could not reach the discovery endpoint", utils.ErrInvalidURL)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("%w: discovery endpoint returned status %d", utils.ErrInvalidURL, resp.StatusCode)
	}

	var doc struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxOidcDiscoveryBody+1)).Decode(&doc); err != nil {
		return "", "", fmt.Errorf("%w: discovery document is not valid JSON", utils.ErrInvalidURL)
	}
	if doc.Issuer == "" || doc.JWKSURI == "" {
		return "", "", fmt.Errorf("%w: discovery document is missing issuer or jwks_uri", utils.ErrInvalidURL)
	}
	return doc.Issuer, doc.JWKSURI, nil
}

// buildOidcDiscoveryURL normalizes rawURL to an OIDC discovery URL: it returns
// the URL unchanged if it already targets the well-known path, otherwise it
// appends the standard suffix to the issuer.
func buildOidcDiscoveryURL(rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", fmt.Errorf("%w: url is required", utils.ErrInvalidURL)
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%w: url must be an absolute http(s) URL", utils.ErrInvalidURL)
	}
	if strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), oidcWellKnownPath) {
		return trimmed, nil
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + oidcWellKnownPath
	return parsed.String(), nil
}
