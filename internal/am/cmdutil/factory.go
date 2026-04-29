package cmdutil

import (
	"context"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2/clientcredentials"

	amsvc "github.com/wso2/agent-manager/internal/am/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/am/config"
	"github.com/wso2/agent-manager/internal/am/iostreams"
	"github.com/wso2/agent-manager/internal/am/prompter"
	"github.com/wso2/agent-manager/internal/am/render"
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
		return nil, render.NewErrorf(render.CodeConfigNotLoaded, "%v", err)
	}
	inst, err := cfg.Current()
	if err != nil {
		return nil, render.NewError(render.CodeNoInstance, err.Error())
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
	if !inst.Auth.ExpiresAt.IsZero() && time.Now().Before(inst.Auth.ExpiresAt.Add(-refreshBuffer)) {
		return inst.Auth.AccessToken, nil
	}
	if inst.Auth.ClientID == "" || inst.Auth.ClientSecret == "" || inst.TokenURL == "" {
		return "", render.NewError(render.CodeAuthRefreshFailed, "missing credentials for token refresh")
	}

	cc := clientcredentials.Config{
		ClientID:     inst.Auth.ClientID,
		ClientSecret: inst.Auth.ClientSecret,
		TokenURL:     inst.TokenURL,
	}
	tok, err := cc.Token(ctx)
	if err != nil {
		return "", render.NewErrorf(render.CodeAuthRefreshFailed, "client_credentials refresh: %v", err)
	}

	name := cfg.CurrentInstance
	updated := *inst
	updated.Auth.AccessToken = tok.AccessToken
	updated.Auth.RefreshToken = tok.RefreshToken
	updated.Auth.ExpiresAt = tok.Expiry
	cfg.Instances[name] = updated

	if err := cfg.Save(); err != nil {
		return "", render.NewErrorf(render.CodeAuthRefreshFailed, "save refreshed config: %v", err)
	}
	return tok.AccessToken, nil
}
