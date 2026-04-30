package project

import (
	"context"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/internal/am/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/am/clierr"
	"github.com/wso2/agent-manager/internal/am/cmdutil"
	"github.com/wso2/agent-manager/internal/am/iostreams"
	"github.com/wso2/agent-manager/internal/am/render"
	"github.com/wso2/agent-manager/internal/am/tableprinter"
)

type ListOptions struct {
	IO           *iostreams.IOStreams
	Client       func(context.Context) (*amsvc.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope    func(org, proj string) render.Scope

	Org   string
	Scope render.Scope
}

func NewListCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{
		IO:           f.IOStreams,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.Scope,
	}
	return &cobra.Command{
		Use:   "list",
		Short: "List projects in an organization",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, _, err := opts.ResolveScope(cmd, true, false)
			scope := opts.MakeScope(org, "")
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			opts.Org, opts.Scope = org, scope
			return runList(cmd.Context(), opts)
		},
	}
}

func runList(ctx context.Context, o *ListOptions) error {
	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	// TODO: paginate
	resp, err := client.ListProjectsWithResponse(ctx, o.Org, &amsvc.ListProjectsParams{})
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON200 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON400, resp.JSON404, resp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, resp.JSON200)
	}

	tp := tableprinter.New(o.IO, "name", "display name", "created")
	cs := o.IO.ColorScheme()
	for _, p := range resp.JSON200.Projects {
		tp.AddField(p.Name, tableprinter.WithColor(cs.Bold))
		tp.AddField(p.DisplayName)
		tp.AddField(p.CreatedAt.Format("2006-01-02"), tableprinter.WithColor(cs.Gray))
		tp.EndRow()
	}
	return tp.Render()
}
