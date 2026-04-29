package project

import (
	"context"
	"net/http"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/internal/am/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/am/clierr"
	"github.com/wso2/agent-manager/internal/am/cmdutil"
	"github.com/wso2/agent-manager/internal/am/iostreams"
	"github.com/wso2/agent-manager/internal/am/prompter"
	"github.com/wso2/agent-manager/internal/am/render"
)

type DeleteOptions struct {
	IO           *iostreams.IOStreams
	Prompter     prompter.Prompter
	Client       func(context.Context) (*amsvc.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope    func(org, proj string) render.Scope

	Org         string
	Scope       render.Scope
	ProjectName string
	Yes         bool
}

type DeleteResult struct {
	Name    string `json:"name"`
	Deleted bool   `json:"deleted"`
}

func NewDeleteCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &DeleteOptions{
		IO:           f.IOStreams,
		Prompter:     f.Prompter,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.Scope,
	}
	cmd := &cobra.Command{
		Use:   "delete <project>",
		Short: "Delete a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, _, err := opts.ResolveScope(cmd, true, false)
			scope := opts.MakeScope(org, "")
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			opts.Org, opts.Scope = org, scope
			opts.ProjectName = args[0]
			return runDelete(cmd.Context(), opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func runDelete(ctx context.Context, o *DeleteOptions) error {
	if err := cmdutil.ValidatePathParam("project name", o.ProjectName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	if !o.Yes {
		if !o.IO.CanPrompt() {
			return render.Error(o.IO, o.Scope, clierr.New(clierr.ConfirmationRequired, "deletion requires --yes when stdin is not a terminal"))
		}
		if err := o.Prompter.ConfirmDeletion(o.ProjectName); err != nil {
			return render.Error(o.IO, o.Scope, clierr.Newf(clierr.ConfirmationRequired, "%v", err))
		}
	}

	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	resp, err := client.DeleteProjectWithResponse(ctx, o.Org, o.ProjectName)
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.HTTPResponse != nil && resp.HTTPResponse.StatusCode == http.StatusNoContent {
		return render.Success(o.IO, o.Scope, DeleteResult{Name: o.ProjectName, Deleted: true})
	}
	return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON404, resp.JSON500)))
}
