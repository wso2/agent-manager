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

package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/cli/pkg/auth"
	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cloud"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/prompter"
	"github.com/wso2/agent-manager/cli/pkg/render"
	"github.com/wso2/agent-manager/cli/pkg/update"
	"github.com/wso2/agent-manager/cli/pkg/version"
)

// orgPeekLimit is the page size used when probing organizations after login.
// Two is enough to distinguish "exactly one" from "more than one" without
// fetching the full list.
const orgPeekLimit = 2

// updateCheckTimeout bounds the release check on login.
const updateCheckTimeout = 3 * time.Second

type loginData struct {
	URL           string                       `json:"url"`
	ExpiresAt     time.Time                    `json:"expires_at"`
	OrgsAvailable []amsvc.OrganizationListItem `json:"orgs_available"`
	ClearedLinks  int                          `json:"cleared_links,omitempty"`
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

	// Cloud targets Agent Manager on WSO2 Cloud whatever the current context is; Getenv is
	// consulted for the gateway override (see the cloud package).
	Cloud    bool
	Getenv   func(string) string
	Resource string
	// Region picks the WSO2 Cloud production region; empty means the stored cloud context's, or
	// ask via Selector when the terminal can prompt, else the US.
	Region   string
	Selector prompter.Selector
	// HTTPClient checks for a newer amctl release on cloud logins.
	HTTPClient func() *http.Client
}

func NewLoginCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &LoginOptions{
		IO:           f.IOStreams,
		Config:       f.Config,
		Authenticate: auth.Login,
		AgentManager: f.AgentManager,
		Getenv:       os.Getenv,
		HTTPClient:   f.HTTPClient,
		Selector:     prompter.NewSelector(f.IOStreams.In, f.IOStreams.ErrOut),
	}

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login to an instance",
		Long: fmt.Sprintf(`Login to Agent Manager.

Without flags, signs in again to the current context, or on first use logs in to
Agent Manager on WSO2 Cloud. --cloud always logs in to WSO2 Cloud. Cloud logins are
saved as the '%[1]s' context.

The first cloud login asks which production region to use (--region picks it without
asking, and changes it later); signing in again keeps the region. Without a terminal
to ask on, it is the US. $%[2]s overrides the gateway, e.g. for dev or stage.

To log in to another instance, pass --url with a --name for the context; sign in
to it again later with --name alone.`, cloud.InstanceName, cloud.GatewayURLEnv),
		Example: `  amctl login
  amctl login --cloud
  amctl login --region eu
  amctl login --url https://amp.example.com --name work
  amctl login --name work`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogin(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.URL, "url", "", "Agent Manager instance URL; requires --name (omit to sign in again to a stored context)")
	cmd.Flags().StringVar(&opts.Name, "name", "", "Context to save the instance as, or the stored context to sign in to again (default: the current context)")
	cmd.Flags().StringVar(&opts.ClientID, "client-id", "", "OAuth client ID (default \"amctl\" for interactive login)")
	cmd.Flags().StringVar(&opts.ClientSecret, "client-secret", "", "OAuth client secret; when set, uses client_credentials grant instead of browser login")
	cmd.Flags().StringVar(&opts.AuthServer, "auth-server", "", "Authorization server base URL; skips OAuth metadata discovery")
	cmd.Flags().BoolVar(&opts.Cloud, "cloud", false, fmt.Sprintf("Log in to Agent Manager on WSO2 Cloud as the %q context", cloud.InstanceName))
	cmd.Flags().StringVar(&opts.Region, "region", "", fmt.Sprintf("WSO2 Cloud region to log in to: %s (implies --cloud)", cloud.RegionNames()))

	return cmd
}

func runLogin(ctx context.Context, opts *LoginOptions) error {
	if opts.ClientSecret != "" && opts.ClientID == "" {
		return render.Error(opts.IO, render.Scope{}, cmdutil.FlagErrorf("--client-id is required when --client-secret is set"))
	}
	if opts.Region != "" {
		if _, ok := cloud.RegionByName(opts.Region); !ok {
			return render.Error(opts.IO, render.Scope{}, cmdutil.FlagErrorf("unknown --region %q: use one of %s", opts.Region, cloud.RegionNames()))
		}
		opts.Cloud = true
	}
	if opts.Cloud {
		if opts.URL != "" {
			return render.Error(opts.IO, render.Scope{}, cmdutil.FlagErrorf("--url cannot be used with --cloud or --region"))
		}
		if opts.Name != "" && opts.Name != cloud.InstanceName {
			return render.Error(opts.IO, render.Scope{}, cmdutil.FlagErrorf("--name cannot be used with --cloud or --region: WSO2 Cloud is always saved as '%s'", cloud.InstanceName))
		}
	}
	if opts.URL != "" {
		if cloud.IsAPIURL(opts.URL) {
			return render.Error(opts.IO, render.Scope{}, cmdutil.FlagErrorf("%s", cloud.LoginHint(opts.URL, update.ReleasesPageURL)))
		}
		if opts.Name == "" {
			return render.Error(opts.IO, render.Scope{}, cmdutil.FlagErrorf("--name is required with --url: the context to save this instance as"))
		}
		if opts.Name == cloud.InstanceName {
			return render.Error(opts.IO, render.Scope{Instance: opts.Name}, cmdutil.FlagErrorf("context name '%s' is reserved for WSO2 Cloud; pick another --name", cloud.InstanceName))
		}
	}
	cfg, err := opts.Config()
	if err != nil {
		return render.Error(opts.IO, render.Scope{Instance: opts.Name}, clierr.Newf(clierr.ConfigNotLoaded, "%v", err))
	}
	// A bare login on a self-hosted context stays there; tell the user how to reach cloud instead.
	cloudTip := false
	if opts.URL == "" {
		if !opts.Cloud && opts.Name == "" {
			if cur, ok := cfg.Instances[cfg.CurrentInstance]; ok && cfg.CurrentInstance != cloud.InstanceName {
				opts.Name = cfg.CurrentInstance
				cloudTip = !cloud.IsAPIURL(cur.URL)
			}
		}
		if opts.Cloud || opts.Name == "" || opts.Name == cloud.InstanceName {
			if err := fillCloud(opts, cfg); err != nil {
				return render.Error(opts.IO, render.Scope{Instance: cloud.InstanceName}, err)
			}
		} else if err := fillFromStoredInstance(opts, cfg); err != nil {
			return render.Error(opts.IO, render.Scope{Instance: opts.Name}, err)
		}
	}
	scope := render.Scope{Instance: opts.Name}
	outdated := startUpdateCheck(ctx, opts)

	inst, err := opts.Authenticate(ctx, auth.LoginOptions{
		URL:          opts.URL,
		ClientID:     opts.ClientID,
		ClientSecret: opts.ClientSecret,
		AuthServer:   opts.AuthServer,
		Resource:     opts.Resource,
		IO:           opts.IO,
		OpenBrowser:  opts.OpenBrowser,
	})
	if err != nil {
		return render.Error(opts.IO, scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	inst.AuthServer = opts.AuthServer
	select {
	case msg := <-outdated:
		if msg != "" {
			fmt.Fprintln(opts.IO.ErrOut, msg)
		}
	default:
	}

	cleared := cfg.ClearLinksIfSwitching(opts.Name)
	if cleared == 0 {
		if prev, ok := cfg.Instances[opts.Name]; ok && prev.URL != inst.URL {
			cleared = cfg.ClearLinkedProjects()
		}
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
			ClearedLinks:  cleared,
		})
	}

	cs := opts.IO.StderrColorScheme()
	fmt.Fprintf(opts.IO.ErrOut, "%s Logged in to %s as %s\n", cs.SuccessIcon(), inst.URL, cs.Bold(opts.Name))
	if scope.Org != "" {
		fmt.Fprintf(opts.IO.ErrOut, "%s Organization set to %s\n", cs.SuccessIcon(), cs.Bold(scope.Org))
	}
	if cleared > 0 {
		fmt.Fprintf(opts.IO.ErrOut, "%s Cleared %d linked project(s). Run 'amctl link' to re-link.\n", cs.SuccessIcon(), cleared)
	}
	if cloudTip {
		fmt.Fprintf(opts.IO.ErrOut, "To log in to Agent Manager on WSO2 Cloud instead, run 'amctl login --cloud'.\n")
	}
	return nil
}

// fillCloud points a login at WSO2 Cloud, saved as the cloud context. A stored cloud
// login on the same gateway keeps its client, so a client_credentials login stays one on re-login.
func fillCloud(opts *LoginOptions, cfg *config.Config) error {
	stored, hasStored := cfg.Instances[cloud.InstanceName]
	url, err := cloudAPIURL(opts, stored, hasStored)
	if err != nil {
		return err
	}
	opts.Name = cloud.InstanceName
	opts.URL = url
	opts.Resource = cloud.Resource
	if hasStored && stored.URL == opts.URL {
		reuseStoredClient(opts, stored)
	}
	return nil
}

// cloudAPIURL picks the cloud API URL: --region, then the AMCTL_CLOUD_URL gateway, then the stored
// cloud context's region, then the user's choice when the terminal can ask, else the US.
func cloudAPIURL(opts *LoginOptions, stored config.Instance, hasStored bool) (string, error) {
	if opts.Region != "" {
		r, _ := cloud.RegionByName(opts.Region)
		return r.APIURL(), nil
	}
	if cloud.GatewayOverride(opts.Getenv) != "" {
		return cloud.APIURL(opts.Getenv), nil
	}
	if hasStored {
		if r, ok := cloud.RegionOfAPIURL(stored.URL); ok {
			return r.APIURL(), nil
		}
	}
	if opts.Selector == nil || opts.IO.JSON || !opts.IO.CanPrompt() {
		return cloud.Regions[0].APIURL(), nil
	}
	labels := make([]string, len(cloud.Regions))
	for i, r := range cloud.Regions {
		labels[i] = fmt.Sprintf("%s (--region %s)", r.Label, r.Name)
	}
	i, err := opts.Selector.Select("Which WSO2 Cloud region do you want to log in to?", labels, 0)
	if err != nil {
		return "", cmdutil.FlagErrorf("choose a region: %v; or pass --region %s", err, cloud.RegionNames())
	}
	return cloud.Regions[i].APIURL(), nil
}

// fillFromStoredInstance re-logs into the named instance with its stored URL and client.
func fillFromStoredInstance(opts *LoginOptions, cfg *config.Config) error {
	stored, ok := cfg.Instances[opts.Name]
	if !ok || stored.URL == "" {
		return cmdutil.FlagErrorf("--url is required to log in to new instance '%s'", opts.Name)
	}
	opts.URL = stored.URL
	if opts.Resource == "" {
		opts.Resource = cloud.UpdateResource(stored.Auth.Resource)
	}
	reuseStoredClient(opts, stored)
	return nil
}

// reuseStoredClient fills the auth server and client the user did not pass from a stored instance.
func reuseStoredClient(opts *LoginOptions, stored config.Instance) {
	if opts.AuthServer == "" {
		opts.AuthServer = stored.AuthServer
	}
	if opts.ClientID == "" {
		opts.ClientID = stored.Auth.ClientID
		if stored.Auth.GrantType == "client_credentials" {
			opts.ClientSecret = stored.Auth.ClientSecret
		}
	}
}

// startUpdateCheck looks for a newer amctl in the background so it never delays login.
// It returns nil, and makes no request, off cloud or for builds without a release version.
func startUpdateCheck(ctx context.Context, opts *LoginOptions) <-chan string {
	if opts.HTTPClient == nil || !cloud.IsAPIURL(opts.URL) || !update.IsRelease(version.Version) {
		return nil
	}
	result := make(chan string, 1)
	go func() {
		ctx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
		defer cancel()
		rel, err := update.Latest(ctx, opts.HTTPClient())
		if err != nil || !update.Newer(version.Version, rel.Version) {
			result <- ""
			return
		}
		result <- fmt.Sprintf("warning: amctl %s is available (you have %s); download it from %s", rel.Version, version.Version, rel.URL)
	}()
	return result
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
