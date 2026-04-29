package agent

import (
	"context"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/internal/am/clierr"
	amsvc "github.com/wso2/agent-manager/internal/am/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/am/cmdutil"
	"github.com/wso2/agent-manager/internal/am/iostreams"
	"github.com/wso2/agent-manager/internal/am/prompter"
	"github.com/wso2/agent-manager/internal/am/render"
)

type agentDeleter interface {
	DeleteAgentWithResponse(ctx context.Context, orgName, projName, agentName string, reqEditors ...amsvc.RequestEditorFn) (*amsvc.DeleteAgentResp, error)
}

type DeleteOptions struct {
	IO        *iostreams.IOStreams
	Prompter  prompter.Prompter
	Client    func(context.Context) (agentDeleter, error)
	BaseRepo  func(*cobra.Command) (string, string, error)
	MakeScope func(org, proj string) render.Scope

	Org       string
	Proj      string
	Scope     render.Scope
	AgentName string
	Yes       bool
}

type DeleteResult struct {
	Name    string `json:"name"`
	Deleted bool   `json:"deleted"`
}

func NewDeleteCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &DeleteOptions{
		IO:        f.IOStreams,
		Prompter:  f.Prompter,
		Client:    func(ctx context.Context) (agentDeleter, error) { return f.AgentManager(ctx) },
		BaseRepo:  func(cmd *cobra.Command) (string, string, error) { return f.ResolveOrgProject(cmd, true, true) },
		MakeScope: f.Scope,
	}
	cmd := &cobra.Command{
		Use:   "delete <agent>",
		Short: "Delete an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, proj, err := opts.BaseRepo(cmd)
			scope := opts.MakeScope(org, proj)
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			opts.Org, opts.Proj, opts.Scope = org, proj, scope
			opts.AgentName = args[0]
			return runDelete(cmd.Context(), opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func runDelete(ctx context.Context, o *DeleteOptions) error {
	if !o.Yes {
		if !o.IO.CanPrompt() {
			return render.Error(o.IO, o.Scope, clierr.New(clierr.ConfirmationRequired, "deletion requires --yes when stdin is not a terminal"))
		}
		if err := o.Prompter.ConfirmDeletion(o.AgentName); err != nil {
			return render.Error(o.IO, o.Scope, clierr.Newf(clierr.ConfirmationRequired, "%v", err))
		}
	}

	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	resp, err := client.DeleteAgentWithResponse(ctx, o.Org, o.Proj, o.AgentName)
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.HTTPResponse != nil && resp.HTTPResponse.StatusCode == http.StatusNoContent {
		return render.Success(o.IO, o.Scope, DeleteResult{Name: o.AgentName, Deleted: true})
	}
	return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, firstNonNil(resp.JSON404, resp.JSON500)))
}
