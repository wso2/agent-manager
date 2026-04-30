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

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/internal/amctl/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/amctl/clierr"
	"github.com/wso2/agent-manager/internal/amctl/cmdutil"
	"github.com/wso2/agent-manager/internal/amctl/iostreams"
	"github.com/wso2/agent-manager/internal/amctl/render"
)

type GetOptions struct {
	IO           *iostreams.IOStreams
	Client       func(context.Context) (*amsvc.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope    func(org, proj string) render.Scope

	Org         string
	Scope       render.Scope
	ProjectName string
}

func NewGetCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &GetOptions{
		IO:           f.IOStreams,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.Scope,
	}
	return &cobra.Command{
		Use:   "get <project>",
		Short: "Get details of a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, _, err := opts.ResolveScope(cmd, true, false)
			scope := opts.MakeScope(org, "")
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			opts.Org, opts.Scope = org, scope
			opts.ProjectName = args[0]
			return runGet(cmd.Context(), opts)
		},
	}
}

func runGet(ctx context.Context, o *GetOptions) error {
	if err := cmdutil.ValidatePathParam("project name", o.ProjectName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	resp, err := client.GetProjectWithResponse(ctx, o.Org, o.ProjectName)
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON200 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON404, resp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, resp.JSON200)
	}

	p := resp.JSON200
	w := o.IO.Out
	cs := o.IO.ColorScheme()
	fmt.Fprintf(w, "name:          %s\n", cs.Bold(p.Name))
	fmt.Fprintf(w, "display name:  %s\n", p.DisplayName)
	fmt.Fprintf(w, "description:   %s\n", p.Description)
	fmt.Fprintf(w, "pipeline:      %s\n", p.DeploymentPipeline)
	fmt.Fprintf(w, "org:           %s\n", p.OrgName)
	fmt.Fprintf(w, "created:       %s\n", cs.Gray(p.CreatedAt.Format("2006-01-02T15:04:05Z07:00")))
	return nil
}
