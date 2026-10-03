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

package cmdutil

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/wso2/agent-manager/cli/pkg/clients"
	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clients/observersvc"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/prompter"
)

const refreshBuffer = 5 * time.Minute

type Factory struct {
	Config       func() (*config.Config, error)
	IOStreams    *iostreams.IOStreams
	Prompter     prompter.Prompter
	HTTPClient   func() *http.Client
	AgentManager func(ctx context.Context) (*amsvc.ClientWithResponses, error)
	Observer     func(ctx context.Context) (*observersvc.Client, error)
	Token        func(ctx context.Context) (string, error)

	observerOnce sync.Once
	observerURL  string
	observerErr  error
}

func NewFactory(cfg *config.Config, io *iostreams.IOStreams) *Factory {
	httpc := &http.Client{Timeout: 30 * time.Second}
	f := &Factory{
		Config:     func() (*config.Config, error) { return cfg, nil },
		IOStreams:  io,
		Prompter:   prompter.New(io.In, io.ErrOut),
		HTTPClient: func() *http.Client { return httpc },
	}
	f.AgentManager = func(ctx context.Context) (*amsvc.ClientWithResponses, error) {
		return f.agentManager(ctx)
	}
	f.Token = func(ctx context.Context) (string, error) {
		return f.currentAccessToken(ctx)
	}
	f.Observer = func(ctx context.Context) (*observersvc.Client, error) {
		return f.observer(ctx)
	}
	return f
}

func (f *Factory) currentAccessToken(ctx context.Context) (string, error) {
	cfg, err := f.Config()
	if err != nil {
		return "", clierr.Newf(clierr.ConfigNotLoaded, "%v", err)
	}
	inst, err := cfg.Current()
	if err != nil {
		return "", clierr.New(clierr.NoInstance, err.Error())
	}
	return f.ensureFreshToken(ctx, cfg, inst)
}

func (f *Factory) observer(ctx context.Context) (*observersvc.Client, error) {
	obsURL, err := f.discoverObserverURL(ctx)
	if err != nil {
		return nil, err
	}

	token, err := f.Token(ctx)
	if err != nil {
		return nil, err
	}

	return observersvc.NewClient(
		strings.TrimRight(obsURL, "/"),
		observersvc.WithHTTPClient(f.HTTPClient()),
		observersvc.WithRequestEditor(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+token)
			return nil
		}),
	)
}

func (f *Factory) discoverObserverURL(ctx context.Context) (string, error) {
	f.observerOnce.Do(func() {
		amClient, err := f.AgentManager(ctx)
		if err != nil {
			f.observerErr = err
			return
		}
		resp, err := amClient.GetConfigWithResponse(ctx)
		if err != nil {
			f.observerErr = clierr.Newf(clierr.Transport, "discover observer URL: %v", err)
			return
		}
		if resp.JSON200 == nil {
			f.observerErr = ErrorFromServer(resp.HTTPResponse, nil)
			return
		}
		raw := resp.JSON200.ObserverBaseUrl
		if raw == "" {
			f.observerErr = clierr.New(clierr.ServerInvalid, "server returned empty observerBaseUrl")
			return
		}
		f.observerURL = rewriteDockerHost(raw)
	})
	return f.observerURL, f.observerErr
}

// rewriteDockerHost swaps host.docker.internal for localhost — the server may
// advertise the container-network hostname, which does not resolve from the CLI.
func rewriteDockerHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	host := u.Hostname()
	if host != "host.docker.internal" {
		return raw
	}
	if port := u.Port(); port != "" {
		u.Host = "localhost:" + port
	} else {
		u.Host = "localhost"
	}
	return u.String()
}

func (f *Factory) agentManager(ctx context.Context) (*amsvc.ClientWithResponses, error) {
	cfg, err := f.Config()
	if err != nil {
		return nil, clierr.Newf(clierr.ConfigNotLoaded, "%v", err)
	}
	inst, err := cfg.Current()
	if err != nil {
		return nil, clierr.New(clierr.NoInstance, err.Error())
	}

	token, err := f.ensureFreshToken(ctx, cfg, inst)
	if err != nil {
		return nil, err
	}

	serverURL := strings.TrimRight(inst.URL, "/") + "/api/v1"
	return amsvc.NewClientWithResponses(
		serverURL,
		amsvc.WithHTTPClient(f.HTTPClient()),
		amsvc.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Accept", "application/json")
			return nil
		}),
	)
}

func (f *Factory) ensureFreshToken(ctx context.Context, cfg *config.Config, inst *config.Instance) (string, error) {
	if inst.Auth.AccessToken != "" && inst.Auth.ExpiresAt.IsZero() {
		return inst.Auth.AccessToken, nil
	}
	if !inst.Auth.ExpiresAt.IsZero() && time.Now().Before(inst.Auth.ExpiresAt.Add(-refreshBuffer)) {
		return inst.Auth.AccessToken, nil
	}

	switch inst.Auth.GrantType {
	case "authorization_code":
		return f.refreshWithRefreshToken(ctx, cfg, inst)
	default:
		return f.refreshWithClientCredentials(ctx, cfg, inst)
	}
}

func (f *Factory) refreshWithClientCredentials(ctx context.Context, cfg *config.Config, inst *config.Instance) (string, error) {
	if inst.Auth.ClientID == "" || inst.Auth.ClientSecret == "" || inst.TokenURL == "" {
		return "", clierr.New(clierr.AuthRefreshFailed, "missing credentials for token refresh")
	}

	scopes := inst.Auth.Scopes
	if len(scopes) == 0 {
		disc, err := clients.Discover(ctx, inst.URL)
		if err == nil {
			scopes = disc.ScopesSupported
		}
	}

	cc := clientcredentials.Config{
		ClientID:     inst.Auth.ClientID,
		ClientSecret: inst.Auth.ClientSecret,
		TokenURL:     inst.TokenURL,
		Scopes:       scopes,
	}
	tok, err := cc.Token(ctx)
	if err != nil {
		return "", clierr.Newf(clierr.AuthRefreshFailed, "client_credentials refresh: %v", err)
	}

	return f.persistToken(cfg, inst, tok)
}

func (f *Factory) refreshWithRefreshToken(ctx context.Context, cfg *config.Config, inst *config.Instance) (string, error) {
	if inst.Auth.RefreshToken == "" || inst.TokenURL == "" {
		return "", sessionExpired(cfg.CurrentInstance, inst, "no refresh token stored")
	}

	oauthCfg := &oauth2.Config{
		ClientID: inst.Auth.ClientID,
		Endpoint: oauth2.Endpoint{
			TokenURL:  inst.TokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
	oldTok := &oauth2.Token{RefreshToken: inst.Auth.RefreshToken}
	tok, err := oauthCfg.TokenSource(ctx, oldTok).Token()
	if err != nil {
		return "", refreshFailure(cfg.CurrentInstance, inst, err)
	}

	return f.persistToken(cfg, inst, tok)
}

// refreshFailure turns a refresh_token grant error into a user-facing message, keeping the raw error as the cause.
func refreshFailure(name string, inst *config.Instance, err error) clierr.CLIError {
	var retrieveErr *oauth2.RetrieveError
	if !errors.As(err, &retrieveErr) {
		e := clierr.Newf(clierr.AuthRefreshFailed,
			"Could not reach the authorization server to refresh your session for '%s' (%s).", name, inst.URL)
		e.AdditionalData["cause"] = err.Error()
		return e
	}
	// Thunder answers invalid_grant for expired, revoked and malformed refresh tokens alike.
	if retrieveErr.ErrorCode == "invalid_grant" {
		return sessionExpired(name, inst, err.Error())
	}
	e := clierr.Newf(clierr.AuthRefreshFailed,
		"Could not refresh your session for '%s' (%s). Run 'amctl login' to sign in again.", name, inst.URL)
	e.AdditionalData["cause"] = err.Error()
	return e
}

func sessionExpired(name string, inst *config.Instance, cause string) clierr.CLIError {
	e := clierr.Newf(clierr.AuthTokenExpired,
		"Your session for '%s' (%s) has expired. Run 'amctl login' to sign in again.", name, inst.URL)
	e.AdditionalData["cause"] = cause
	return e
}

func (f *Factory) persistToken(cfg *config.Config, inst *config.Instance, tok *oauth2.Token) (string, error) {
	name := cfg.CurrentInstance
	updated := *inst
	updated.Auth.AccessToken = tok.AccessToken
	if tok.RefreshToken != "" {
		updated.Auth.RefreshToken = tok.RefreshToken
	}
	updated.Auth.ExpiresAt = tok.Expiry
	cfg.Instances[name] = updated

	if err := cfg.Save(); err != nil {
		return "", clierr.Newf(clierr.AuthRefreshFailed, "save refreshed config: %v", err)
	}
	return tok.AccessToken, nil
}
