package org

import (
	"context"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/internal/amctl/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/amctl/clierr"
	"github.com/wso2/agent-manager/internal/amctl/cmdutil"
	"github.com/wso2/agent-manager/internal/amctl/config"
	"github.com/wso2/agent-manager/internal/amctl/iostreams"
	"github.com/wso2/agent-manager/internal/amctl/render"
	"github.com/wso2/agent-manager/internal/amctl/tableprinter"
)

type ListOptions struct {
	IO     *iostreams.IOStreams
	Config func() (*config.Config, error)
	Client func(context.Context) (*amsvc.ClientWithResponses, error)
}

func NewListCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{
		IO:     f.IOStreams,
		Config: f.Config,
		Client: f.AgentManager,
	}
	return &cobra.Command{
		Use:   "list",
		Short: "List organizations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context(), opts)
		},
	}
}

func runList(ctx context.Context, o *ListOptions) error {
	scope := render.Scope{}

	cfg, err := o.Config()
	if err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.ConfigNotLoaded, "%v", err))
	}
	if cfg.CurrentInstance == "" {
		return render.Error(o.IO, scope, clierr.New(clierr.NoInstance, "no instance configured"))
	}
	scope.Instance = cfg.CurrentInstance

	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, scope, err)
	}

	// TODO: paginate
	resp, err := client.ListOrganizationsWithResponse(ctx, &amsvc.ListOrganizationsParams{})
	if err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON200 == nil {
		return render.Error(o.IO, scope, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON400, resp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, scope, resp.JSON200)
	}

	tp := tableprinter.New(o.IO, "name", "created")
	cs := o.IO.ColorScheme()
	for _, org := range resp.JSON200.Organizations {
		tp.AddField(org.Name, tableprinter.WithColor(cs.Bold))
		tp.AddField(org.CreatedAt.Format("2006-01-02"), tableprinter.WithColor(cs.Gray))
		tp.EndRow()
	}
	return tp.Render()
}
