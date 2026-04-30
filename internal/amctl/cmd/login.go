package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/internal/amctl/auth"
	amsvc "github.com/wso2/agent-manager/internal/amctl/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/amctl/clierr"
	"github.com/wso2/agent-manager/internal/amctl/cmdutil"
	"github.com/wso2/agent-manager/internal/amctl/config"
	"github.com/wso2/agent-manager/internal/amctl/iostreams"
	"github.com/wso2/agent-manager/internal/amctl/render"
)

// orgPeekLimit is the page size used when probing organizations after login.
// Two is enough to distinguish "exactly one" from "more than one" without
// fetching the full list.
const orgPeekLimit = 2

type loginData struct {
	URL           string                       `json:"url"`
	ExpiresAt     time.Time                    `json:"expires_at"`
	OrgsAvailable []amsvc.OrganizationListItem `json:"orgs_available"`
}

type LoginOptions struct {
	IO           *iostreams.IOStreams
	Config       func() (*config.Config, error)
	Authenticate func(context.Context, auth.LoginOptions) (*config.Instance, error)
	AgentManager func(context.Context) (*amsvc.ClientWithResponses, error)

	URL          string
	Name         string
	ClientID     string
	ClientSecret string
	AuthServer   string
	OpenBrowser  func(string) error
}

func NewLoginCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &LoginOptions{
		IO:           f.IOStreams,
		Config:       f.Config,
		Authenticate: auth.Login,
		AgentManager: f.AgentManager,
	}

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login to an instance",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogin(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.URL, "url", "", "Agent Manager instance URL")
	cmd.Flags().StringVar(&opts.Name, "name", "", "Agent Manager instance name")
	cmd.Flags().StringVar(&opts.ClientID, "client-id", "", "OAuth client ID (default \"amctl\" for interactive login)")
	cmd.Flags().StringVar(&opts.ClientSecret, "client-secret", "", "OAuth client secret; when set, uses client_credentials grant instead of browser login")
	cmd.Flags().StringVar(&opts.AuthServer, "auth-server", "", "Authorization server base URL; skips OAuth metadata discovery")

	return cmd
}

func runLogin(ctx context.Context, opts *LoginOptions) error {
	if opts.URL == "" {
		return render.Error(opts.IO, render.Scope{}, cmdutil.FlagErrorf("--url is required"))
	}
	if opts.ClientSecret != "" && opts.ClientID == "" {
		return render.Error(opts.IO, render.Scope{}, cmdutil.FlagErrorf("--client-id is required when --client-secret is set"))
	}
	if opts.Name == "" {
		opts.Name = "default"
	}
	scope := render.Scope{Instance: opts.Name}

	inst, err := opts.Authenticate(ctx, auth.LoginOptions{
		URL:          opts.URL,
		ClientID:     opts.ClientID,
		ClientSecret: opts.ClientSecret,
		AuthServer:   opts.AuthServer,
		IO:           opts.IO,
		OpenBrowser:  opts.OpenBrowser,
	})
	if err != nil {
		return render.Error(opts.IO, scope, clierr.Newf(clierr.Transport, "%v", err))
	}

	cfg, err := opts.Config()
	if err != nil {
		return render.Error(opts.IO, scope, clierr.Newf(clierr.ConfigNotLoaded, "%v", err))
	}
	cfg.AddInstance(opts.Name, *inst)
	if err := cfg.Save(); err != nil {
		return render.Error(opts.IO, scope, clierr.Newf(clierr.ConfigSaveFailed, "save config: %v", err))
	}

	orgs, ferr := fetchOrgs(ctx, opts)
	if ferr != nil {
		fmt.Fprintf(opts.IO.ErrOut, "warning: failed to fetch organizations: %v\n", ferr)
		orgs = nil
	}

	switch len(orgs) {
	case 1:
		updated := cfg.Instances[opts.Name]
		updated.CurrentOrg = orgs[0].Name
		cfg.Instances[opts.Name] = updated
		if err := cfg.Save(); err != nil {
			fmt.Fprintf(opts.IO.ErrOut, "warning: failed to save current_org: %v\n", err)
		}
	case 0:
		if ferr == nil {
			fmt.Fprintln(opts.IO.ErrOut, "warning: no organizations available; pass --org on subsequent commands")
		}
	default:
		fmt.Fprintf(opts.IO.ErrOut, "warning: %d organizations available; pass --org on subsequent commands\n", len(orgs))
	}

	scope.Org = cfg.Instances[opts.Name].CurrentOrg

	if opts.IO.JSON {
		return render.JSONSuccess(opts.IO, scope, loginData{
			URL:           inst.URL,
			ExpiresAt:     inst.Auth.ExpiresAt,
			OrgsAvailable: orgs,
		})
	}

	cs := opts.IO.StderrColorScheme()
	fmt.Fprintf(opts.IO.ErrOut, "%s Logged in to %s as %s\n", cs.SuccessIcon(), inst.URL, cs.Bold(opts.Name))
	if scope.Org != "" {
		fmt.Fprintf(opts.IO.ErrOut, "%s Organization set to %s\n", cs.SuccessIcon(), cs.Bold(scope.Org))
	}
	return nil
}

func fetchOrgs(ctx context.Context, opts *LoginOptions) ([]amsvc.OrganizationListItem, error) {
	client, err := opts.AgentManager(ctx)
	if err != nil {
		return nil, err
	}
	limit := orgPeekLimit
	resp, err := client.ListOrganizationsWithResponse(ctx, &amsvc.ListOrganizationsParams{Limit: &limit})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 != nil {
		return resp.JSON200.Organizations, nil
	}
	return nil, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON400, resp.JSON500))
}
