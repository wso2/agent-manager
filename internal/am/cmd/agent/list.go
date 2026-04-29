package agent

import (
	"context"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/internal/am/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/am/cmdutil"
	"github.com/wso2/agent-manager/internal/am/iostreams"
	"github.com/wso2/agent-manager/internal/am/render"
)

type agentLister interface {
	ListAgentsWithResponse(ctx context.Context, orgName, projName string, params *amsvc.ListAgentsParams, reqEditors ...amsvc.RequestEditorFn) (*amsvc.ListAgentsResp, error)
}

type ListOptions struct {
	IO        *iostreams.IOStreams
	Client    func(context.Context) (agentLister, error)
	BaseRepo  func(*cobra.Command) (string, string, error)
	MakeScope func(org, proj string) render.Scope

	Org    string
	Proj   string
	Scope  render.Scope
	Limit  *int
	Offset *int
}

func NewListCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{
		IO:        f.IOStreams,
		Client:    func(ctx context.Context) (agentLister, error) { return f.AgentManager(ctx) },
		BaseRepo:  func(cmd *cobra.Command) (string, string, error) { return f.ResolveOrgProject(cmd, true, true) },
		MakeScope: f.Scope,
	}
	var limit, offset int
	var limitSet, offsetSet bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agents in a project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, proj, err := opts.BaseRepo(cmd)
			scope := opts.MakeScope(org, proj)
			if err != nil {
				return render.Emit(opts.IO, scope, err)
			}
			if limitSet && limit < 1 {
				return render.Emit(opts.IO, scope, render.NewError(render.CodeInvalidFlag, "--limit must be >= 1"))
			}
			if offsetSet && offset < 0 {
				return render.Emit(opts.IO, scope, render.NewError(render.CodeInvalidFlag, "--offset must be >= 0"))
			}
			opts.Org, opts.Proj, opts.Scope = org, proj, scope
			if limitSet {
				v := limit
				opts.Limit = &v
			}
			if offsetSet {
				v := offset
				opts.Offset = &v
			}
			return runList(cmd.Context(), opts)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of results to return")
	cmd.Flags().IntVar(&offset, "offset", 0, "Number of results to skip")
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		limitSet = cmd.Flags().Changed("limit")
		offsetSet = cmd.Flags().Changed("offset")
		return nil
	}
	return cmd
}

func runList(ctx context.Context, o *ListOptions) error {
	client, err := o.Client(ctx)
	if err != nil {
		return render.Emit(o.IO, o.Scope, err)
	}
	resp, err := client.ListAgentsWithResponse(ctx, o.Org, o.Proj, &amsvc.ListAgentsParams{
		Limit:  o.Limit,
		Offset: o.Offset,
	})
	if err != nil {
		return render.Emit(o.IO, o.Scope, render.NewErrorf(render.CodeTransport, "%v", err))
	}
	if resp.JSON200 != nil {
		return render.Success(o.IO, o.Scope, resp.JSON200)
	}
	return render.Emit(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, firstNonNil(resp.JSON400, resp.JSON500)))
}
