package project

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/internal/am/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/am/clierr"
	"github.com/wso2/agent-manager/internal/am/cmdutil"
	"github.com/wso2/agent-manager/internal/am/iostreams"
	"github.com/wso2/agent-manager/internal/am/render"
)

// defaultDeploymentPipeline is the only pipeline currently supported by the
// platform. The CLI hard-codes it until pipeline selection becomes meaningful.
const defaultDeploymentPipeline = "default"

type CreateOptions struct {
	IO           *iostreams.IOStreams
	Client       func(context.Context) (*amsvc.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope    func(org, proj string) render.Scope

	Org         string
	Scope       render.Scope
	Name        string
	DisplayName string
	Description string
}

func NewCreateCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &CreateOptions{
		IO:           f.IOStreams,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.Scope,
	}
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, _, err := opts.ResolveScope(cmd, true, false)
			scope := opts.MakeScope(org, "")
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			opts.Org, opts.Scope = org, scope
			opts.Name = args[0]
			return runCreate(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.DisplayName, "display-name", "", "Display name for the project (required)")
	cmd.Flags().StringVar(&opts.Description, "description", "", "Project description")
	_ = cmd.MarkFlagRequired("display-name")
	return cmd
}

func runCreate(ctx context.Context, o *CreateOptions) error {
	if err := cmdutil.ValidatePathParam("project name", o.Name); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	body := amsvc.CreateProjectJSONRequestBody{
		Name:               o.Name,
		DisplayName:        o.DisplayName,
		DeploymentPipeline: defaultDeploymentPipeline,
	}
	if o.Description != "" {
		body.Description = &o.Description
	}

	resp, err := client.CreateProjectWithResponse(ctx, o.Org, body)
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON202 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON400, resp.JSON404, resp.JSON409, resp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, resp.JSON202)
	}

	cs := o.IO.StderrColorScheme()
	fmt.Fprintf(o.IO.ErrOut, "%s Created project %s\n", cs.SuccessIcon(), o.Name)
	return nil
}
