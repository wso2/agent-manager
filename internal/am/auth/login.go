package auth

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/wso2/agent-manager/internal/am/browser"
	"github.com/wso2/agent-manager/internal/am/clients"
	"github.com/wso2/agent-manager/internal/am/config"
	"github.com/wso2/agent-manager/internal/am/iostreams"
)

const (
	defaultClientID    = "am-cli"
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
		},
	}, nil
}
