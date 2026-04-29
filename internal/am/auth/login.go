package auth

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/oauth2/clientcredentials"

	"github.com/wso2/agent-manager/internal/am/clients"
	"github.com/wso2/agent-manager/internal/am/config"
)

const oauthTokenPath = "/oauth2/token"

type LoginOptions struct {
	URL          string
	ClientID     string
	ClientSecret string
	AuthServer   string
}

func Login(ctx context.Context, opts LoginOptions) (*config.Instance, error) {
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
			ClientID:     opts.ClientID,
			ClientSecret: opts.ClientSecret,
			AccessToken:  tok.AccessToken,
			RefreshToken: tok.RefreshToken,
			ExpiresAt:    tok.Expiry,
		},
	}, nil
}
