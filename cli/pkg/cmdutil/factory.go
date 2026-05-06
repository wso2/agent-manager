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
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/wso2/agent-manager/cli/pkg/clients"
	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
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
	return f
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
		return "", clierr.New(clierr.AuthRefreshFailed, "missing refresh token; please run `amctl login` again")
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
		return "", clierr.Newf(clierr.AuthRefreshFailed, "refresh token grant failed (re-run `amctl login`): %v", err)
	}

	return f.persistToken(cfg, inst, tok)
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
