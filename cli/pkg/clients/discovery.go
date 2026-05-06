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

package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ProtectedResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
	ScopesSupported      []string `json:"scopes_supported"`
}

type AuthorizationServerMetadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

type Discovery struct {
	Resource              string
	AuthorizationServer   string
	AuthorizationEndpoint string
	TokenEndpoint         string
	ScopesSupported       []string
}

func Discover(ctx context.Context, baseURL string) (*Discovery, error) {
	httpc := &http.Client{Timeout: 10 * time.Second}

	prURL, err := joinWellKnown(baseURL, "oauth-protected-resource")
	if err != nil {
		return nil, fmt.Errorf("build protected-resource URL: %w", err)
	}
	var pr ProtectedResourceMetadata
	if err := getJSON(ctx, httpc, prURL, &pr); err != nil {
		return nil, fmt.Errorf("fetch protected-resource metadata: %w", err)
	}
	if len(pr.AuthorizationServers) == 0 {
		return nil, fmt.Errorf("protected-resource metadata at %s lists no authorization_servers", prURL)
	}
	authServer := pr.AuthorizationServers[0]

	asURL, err := joinWellKnown(authServer, "oauth-authorization-server")
	if err != nil {
		return nil, fmt.Errorf("build authorization-server URL: %w", err)
	}
	var as AuthorizationServerMetadata
	if err := getJSON(ctx, httpc, asURL, &as); err != nil {
		return nil, fmt.Errorf("fetch authorization-server metadata: %w", err)
	}
	if as.AuthorizationEndpoint == "" {
		return nil, fmt.Errorf("authorization-server metadata at %s has no authorization_endpoint", asURL)
	}
	if as.TokenEndpoint == "" {
		return nil, fmt.Errorf("authorization-server metadata at %s has no token_endpoint", asURL)
	}

	return &Discovery{
		Resource:              pr.Resource,
		AuthorizationServer:   authServer,
		AuthorizationEndpoint: as.AuthorizationEndpoint,
		TokenEndpoint:         as.TokenEndpoint,
		ScopesSupported:       pr.ScopesSupported,
	}, nil
}

func joinWellKnown(base, name string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/.well-known/" + name
	return u.String(), nil
}

func getJSON(ctx context.Context, httpc *http.Client, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", u, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", u, err)
	}
	return nil
}
