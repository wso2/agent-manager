package org

import (
	"context"
	"net/http"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/internal/am/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/am/clierr"
	"github.com/wso2/agent-manager/internal/am/cmdutil"
	"github.com/wso2/agent-manager/internal/am/config"
	"github.com/wso2/agent-manager/internal/am/iostreams"
	"github.com/wso2/agent-manager/internal/am/render"
)

type UseOptions struct {
	IO     *iostreams.IOStreams
	Config func() (*config.Config, error)
	Client func(context.Context) (*amsvc.ClientWithResponses, error)

	Name string
}

type UseResult struct {
	Org string `json:"org"`
}

func NewUseCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &UseOptions{
		IO:     f.IOStreams,
		Config: f.Config,
		Client: f.AgentManager,
	}
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Set the active organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]
			return runUse(cmd.Context(), opts)
		},
	}
}

func runUse(ctx context.Context, o *UseOptions) error {
	scope := render.Scope{}

	cfg, err := o.Config()
	if err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.ConfigNotLoaded, "%v", err))
	}
	if cfg.CurrentInstance == "" {
		return render.Error(o.IO, scope, clierr.New(clierr.NoInstance, "no instance configured"))
	}
	scope.Instance = cfg.CurrentInstance

	if err := cmdutil.ValidatePathParam("org name", o.Name); err != nil {
		return render.Error(o.IO, scope, err)
	}

	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, scope, err)
	}

	resp, err := client.GetOrganizationWithResponse(ctx, o.Name)
	if err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.HTTPResponse == nil || resp.HTTPResponse.StatusCode != http.StatusOK {
		return render.Error(o.IO, scope, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON404, resp.JSON500)))
	}

	inst := cfg.Instances[cfg.CurrentInstance]
	inst.CurrentOrg = o.Name
	cfg.Instances[cfg.CurrentInstance] = inst

	if err := cfg.Save(); err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.ConfigSaveFailed, "save config: %v", err))
	}

	scope.Org = o.Name
	return render.Success(o.IO, scope, UseResult{Org: o.Name})
}
