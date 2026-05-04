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

package auth

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/wso2/agent-manager/cli/pkg/browser"
	"github.com/wso2/agent-manager/cli/pkg/clients"
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

const (
	defaultClientID    = "amctl"
	oauthTokenPath     = "/oauth2/token"
	oauthAuthorizePath = "/oauth2/authorize"
)

type LoginOptions struct {
	URL          string
	ClientID     string
	ClientSecret string
	AuthServer   string
	IO           *iostreams.IOStreams
	OpenBrowser  func(string) error
}

func Login(ctx context.Context, opts LoginOptions) (*config.Instance, error) {
	if opts.ClientSecret != "" {
		return loginClientCredentials(ctx, opts)
	}
	return loginPKCE(ctx, opts)
}

func loginClientCredentials(ctx context.Context, opts LoginOptions) (*config.Instance, error) {
	var tokenEndpoint string
	var scopes []string
	if opts.AuthServer == "" {
		disc, err := clients.Discover(ctx, opts.URL)
		if err != nil {
			return nil, err
		}
		tokenEndpoint = disc.TokenEndpoint
		scopes = disc.ScopesSupported
	} else {
		tokenEndpoint = strings.TrimRight(opts.AuthServer, "/") + oauthTokenPath
	}

	cc := clientcredentials.Config{
		ClientID:     opts.ClientID,
		ClientSecret: opts.ClientSecret,
		TokenURL:     tokenEndpoint,
		Scopes:       scopes,
	}
	tok, err := cc.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("client_credentials token exchange: %w", err)
	}

	return &config.Instance{
		URL:      opts.URL,
		TokenURL: tokenEndpoint,
		Auth: config.AuthConfig{
			GrantType:    "client_credentials",
			ClientID:     opts.ClientID,
			ClientSecret: opts.ClientSecret,
			AccessToken:  tok.AccessToken,
			RefreshToken: tok.RefreshToken,
			ExpiresAt:    tok.Expiry,
			Scopes:       scopes,
		},
	}, nil
}

func loginPKCE(ctx context.Context, opts LoginOptions) (*config.Instance, error) {
	clientID := opts.ClientID
	if clientID == "" {
		clientID = defaultClientID
	}

	var authEndpoint, tokenEndpoint string
	var scopes []string
	if opts.AuthServer == "" {
		disc, err := clients.Discover(ctx, opts.URL)
		if err != nil {
			return nil, err
		}
		authEndpoint = disc.AuthorizationEndpoint
		tokenEndpoint = disc.TokenEndpoint
		scopes = disc.ScopesSupported
	} else {
		base := strings.TrimRight(opts.AuthServer, "/")
		authEndpoint = base + oauthAuthorizePath
		tokenEndpoint = base + oauthTokenPath
	}

	oauthCfg := &oauth2.Config{
		ClientID: clientID,
		Endpoint: oauth2.Endpoint{
			AuthURL:   authEndpoint,
			TokenURL:  tokenEndpoint,
			AuthStyle: oauth2.AuthStyleInParams,
		},
		Scopes: scopes,
	}

	openBrowser := opts.OpenBrowser
	if openBrowser == nil {
		openBrowser = browser.Open
	}

	tok, err := authCodePKCE(ctx, oauthCfg, opts.IO, openBrowser)
	if err != nil {
		return nil, fmt.Errorf("authorization code exchange: %w", err)
	}

	return &config.Instance{
		URL:              opts.URL,
		TokenURL:         tokenEndpoint,
		AuthorizationURL: authEndpoint,
		Auth: config.AuthConfig{
			GrantType:    "authorization_code",
			ClientID:     clientID,
			AccessToken:  tok.AccessToken,
			RefreshToken: tok.RefreshToken,
			ExpiresAt:    tok.Expiry,
			Scopes:       scopes,
		},
	}, nil
}
