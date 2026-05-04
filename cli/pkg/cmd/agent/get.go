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

package agent

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

	Org       string
	Proj      string
	Scope     render.Scope
	AgentName string
}

func NewGetCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &GetOptions{
		IO:           f.IOStreams,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.Scope,
	}
	cmd := &cobra.Command{
		Use:   "get <agent>",
		Short: "Show details of an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, proj, err := opts.ResolveScope(cmd, true, true)
			scope := opts.MakeScope(org, proj)
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			opts.Org, opts.Proj, opts.Scope = org, proj, scope
			opts.AgentName = args[0]
			return runGet(cmd.Context(), opts)
		},
	}
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return cmdutil.CompleteAgents(cmd, f), cobra.ShellCompDirectiveNoFileComp
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
	if resp.JSON200 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON404, resp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, resp.JSON200)
	}

	a := resp.JSON200
	w := o.IO.Out
	cs := o.IO.ColorScheme()
	fmt.Fprintf(w, "name:          %s\n", cs.Bold(a.Name))
	fmt.Fprintf(w, "display name:  %s\n", a.DisplayName)
	fmt.Fprintf(w, "description:   %s\n", a.Description)
	status := "-"
	if a.Status != nil {
		status = *a.Status
	}
	fmt.Fprintf(w, "status:        %s\n", status)
	fmt.Fprintf(w, "project:       %s\n", a.ProjectName)
	fmt.Fprintf(w, "created:       %s\n", cs.Gray(a.CreatedAt.Format("2006-01-02T15:04:05Z07:00")))
	return nil
}
