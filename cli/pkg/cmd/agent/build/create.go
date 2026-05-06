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

package build

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

type CreateOptions struct {
	IO           *iostreams.IOStreams
	Client       func(context.Context) (*amsvc.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope    func(org, proj string) render.Scope

	Org       string
	Proj      string
	Scope     render.Scope
	AgentName string
	Commit    string
}

func NewCreateCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &CreateOptions{
		IO:           f.IOStreams,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.Scope,
	}
	cmd := &cobra.Command{
		Use:   "create <agent>",
		Short: "Trigger a build for an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, proj, err := opts.ResolveScope(cmd, true, true)
			scope := opts.MakeScope(org, proj)
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			opts.Org, opts.Proj, opts.Scope = org, proj, scope
			opts.AgentName = args[0]
			return runCreate(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Commit, "commit", "", "Commit SHA to build from")
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return cmdutil.CompleteBuildableAgents(cmd, f), cobra.ShellCompDirectiveNoFileComp
	}
	return cmd
}

func runCreate(ctx context.Context, o *CreateOptions) error {
	if err := cmdutil.ValidatePathParam("agent name", o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	if err := cmdutil.ValidateBuildable(ctx, client, o.Org, o.Proj, o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	params := &amsvc.BuildAgentParams{}
	if o.Commit != "" {
		params.CommitId = &o.Commit
	}

	resp, err := client.BuildAgentWithResponse(ctx, o.Org, o.Proj, o.AgentName, params)
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON202 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON400, resp.JSON404, resp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, resp.JSON202)
	}

	cs := o.IO.StderrColorScheme()
	fmt.Fprintf(o.IO.ErrOut, "%s Build triggered for agent %q (build: %s)\n", cs.SuccessIcon(), o.AgentName, resp.JSON202.BuildName)
	return nil
}
