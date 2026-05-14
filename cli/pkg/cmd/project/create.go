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

package project

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
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

func validateCreate(opts *CreateOptions) error {
	var v []string
	if opts.Name == "" {
		v = append(v, "name argument is required")
	} else if strings.Contains(opts.Name, "/") {
		v = append(v, "name must not contain '/'")
	}
	if opts.DisplayName == "" {
		v = append(v, "--display-name is required")
	}
	if len(v) == 0 {
		return nil
	}
	return cmdutil.FlagErrors(v)
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
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Name = args[0]
			}
			if err := validateCreate(opts); err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}
			org, _, err := opts.ResolveScope(cmd, true, false)
			scope := opts.MakeScope(org, "")
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			opts.Org, opts.Scope = org, scope
			return runCreate(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.DisplayName, "display-name", "", "Display name for the project (required)")
	cmd.Flags().StringVar(&opts.Description, "description", "", "Project description")
	return cmd
}

func runCreate(ctx context.Context, o *CreateOptions) error {
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
