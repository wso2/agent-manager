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

package jwtassertion

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/config"
)

// openIDConfigurationPath is where an OpenID Connect provider publishes its
// discovery document, relative to its issuer (OpenID Connect Discovery 1.0 §4).
const openIDConfigurationPath = "/.well-known/openid-configuration"

var (
	discoveredJWKSMu  sync.Mutex
	discoveredJWKSURL = map[string]string{}
	discoveryClient   = &http.Client{Timeout: 10 * time.Second}
)

// keySetURL is the URL of the key set tokens are checked with: KEY_MANAGER_JWKS_URL
// when set, else the jwks_uri of the identity provider's discovery document, or ""
// when neither is configured.
func keySetURL(km config.KeyManagerConfigurations) (string, error) {
	if u := strings.TrimSpace(km.JWKSUrl); u != "" {
		return u, nil
	}
	issuer := strings.TrimSpace(km.DiscoveryIssuer)
	if issuer == "" {
		return "", nil
	}
	discoveredJWKSMu.Lock()
	defer discoveredJWKSMu.Unlock()
	if u, ok := discoveredJWKSURL[issuer]; ok {
		return u, nil
	}
	// A failure is not remembered, so the next token tries again: the
	// identity provider may simply not have been up yet.
	u, err := discoverJWKSURL(issuer)
	if err != nil {
		return "", fmt.Errorf("%w: discover the key set of %s: %w", ErrKeySetUnavailable, issuer, err)
	}
	discoveredJWKSURL[issuer] = u
	return u, nil
}

// discoverJWKSURL reads issuer's discovery document and returns its jwks_uri. The
// document must name issuer as its issuer, as OpenID Connect Discovery 1.0 §4.3
// requires, so a document served for another provider is refused.
func discoverJWKSURL(issuer string) (string, error) {
	resp, err := discoveryClient.Get(strings.TrimRight(issuer, "/") + openIDConfigurationPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("discovery document returned status %d", resp.StatusCode)
	}
	var doc struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return "", fmt.Errorf("decode discovery document: %w", err)
	}
	if doc.Issuer != issuer {
		return "", fmt.Errorf("discovery document names issuer %q, not %q", doc.Issuer, issuer)
	}
	u, err := url.Parse(doc.JWKSURI)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", fmt.Errorf("discovery document has no usable jwks_uri (%q)", doc.JWKSURI)
	}
	return doc.JWKSURI, nil
}
