package agent

import (
	"context"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/internal/am/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/am/clierr"
	"github.com/wso2/agent-manager/internal/am/cmdutil"
	"github.com/wso2/agent-manager/internal/am/iostreams"
	"github.com/wso2/agent-manager/internal/am/render"
)

type GetOptions struct {
	IO        *iostreams.IOStreams
	Client    func(context.Context) (*amsvc.ClientWithResponses, error)
	ResolveScope  func(*cobra.Command) (string, string, error)
	MakeScope func(org, proj string) render.Scope

	Org       string
	Proj      string
	Scope     render.Scope
	AgentName string
}

func NewGetCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &GetOptions{
		IO:        f.IOStreams,
		Client:    f.AgentManager,
		ResolveScope:  func(cmd *cobra.Command) (string, string, error) { return f.ResolveOrgProject(cmd, true, true) },
		MakeScope: f.Scope,
	}
	cmd := &cobra.Command{
		Use:   "get <agent>",
		Short: "Show details of an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, proj, err := opts.ResolveScope(cmd)
			scope := opts.MakeScope(org, proj)
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			opts.Org, opts.Proj, opts.Scope = org, proj, scope
			opts.AgentName = args[0]
			return runGet(cmd.Context(), opts)
		},
	}
	return cmd
}

func runGet(ctx context.Context, o *GetOptions) error {
	if err := cmdutil.ValidatePathParam("agent name", o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	resp, err := client.GetAgentWithResponse(ctx, o.Org, o.Proj, o.AgentName)
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON200 != nil {
		return render.Success(o.IO, o.Scope, resp.JSON200)
	}
	return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON404, resp.JSON500)))
}
