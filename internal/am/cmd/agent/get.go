package agent

import (
	"context"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/internal/am/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/am/cmdutil"
	"github.com/wso2/agent-manager/internal/am/iostreams"
	"github.com/wso2/agent-manager/internal/am/render"
)

type agentGetter interface {
	GetAgentWithResponse(ctx context.Context, orgName, projName, agentName string, reqEditors ...amsvc.RequestEditorFn) (*amsvc.GetAgentResp, error)
}

type GetOptions struct {
	IO        *iostreams.IOStreams
	Client    func(context.Context) (agentGetter, error)
	BaseRepo  func(*cobra.Command) (string, string, error)
	MakeScope func(org, proj string) render.Scope

	Org       string
	Proj      string
	Scope     render.Scope
	AgentName string
}

func NewGetCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &GetOptions{
		IO:        f.IOStreams,
		Client:    func(ctx context.Context) (agentGetter, error) { return f.AgentManager(ctx) },
		BaseRepo:  func(cmd *cobra.Command) (string, string, error) { return f.ResolveOrgProject(cmd, true, true) },
		MakeScope: f.Scope,
	}
	cmd := &cobra.Command{
		Use:   "get <agent>",
		Short: "Show details of an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, proj, err := opts.BaseRepo(cmd)
			scope := opts.MakeScope(org, proj)
			if err != nil {
				return render.Emit(opts.IO, scope, err)
			}
			opts.Org, opts.Proj, opts.Scope = org, proj, scope
			opts.AgentName = args[0]
			return runGet(cmd.Context(), opts)
		},
	}
	return cmd
}

func runGet(ctx context.Context, o *GetOptions) error {
	client, err := o.Client(ctx)
	if err != nil {
		return render.Emit(o.IO, o.Scope, err)
	}
	resp, err := client.GetAgentWithResponse(ctx, o.Org, o.Proj, o.AgentName)
	if err != nil {
		return render.Emit(o.IO, o.Scope, render.NewErrorf(render.CodeTransport, "%v", err))
	}
	if resp.JSON200 != nil {
		return render.Success(o.IO, o.Scope, resp.JSON200)
	}
	return render.Emit(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, firstNonNil(resp.JSON404, resp.JSON500)))
}
