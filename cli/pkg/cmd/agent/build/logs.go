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

type LogsOptions struct {
	IO           *iostreams.IOStreams
	Client       func(context.Context) (*amsvc.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope    func(org, proj string) render.Scope

	Org       string
	Proj      string
	Scope     render.Scope
	AgentName string
	BuildName   string
}

func NewLogsCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &LogsOptions{
		IO:           f.IOStreams,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.Scope,
	}

	cmd := &cobra.Command{
		Use:   "logs <agent> [build-name]",
		Short: "Show build logs",
		Long:  "Show logs for a build. If build name is omitted, shows logs for the latest build.",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, proj, err := opts.ResolveScope(cmd, true, true)
			scope := opts.MakeScope(org, proj)
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			opts.Org, opts.Proj, opts.Scope = org, proj, scope
			opts.AgentName = args[0]
			if len(args) > 1 {
				opts.BuildName = args[1]
			}
			return runLogs(cmd.Context(), opts)
		},
	}
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		switch len(args) {
		case 0:
			return cmdutil.CompleteBuildableAgents(cmd, f), cobra.ShellCompDirectiveNoFileComp
		case 1:
			return cmdutil.CompleteBuilds(cmd, f, args[0]), cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return cmd
}

func runLogs(ctx context.Context, o *LogsOptions) error {
	if err := cmdutil.ValidatePathParam("agent name", o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	if o.BuildName != "" {
		if err := cmdutil.ValidatePathParam("build name", o.BuildName); err != nil {
			return render.Error(o.IO, o.Scope, err)
		}
	}

	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	if err := cmdutil.ValidateBuildable(ctx, client, o.Org, o.Proj, o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	buildName := o.BuildName
	if buildName == "" {
		resolved, err := resolveLatestBuild(ctx, client, o)
		if err != nil {
			return render.Error(o.IO, o.Scope, err)
		}
		buildName = resolved
	}

	resp, err := client.GetBuildLogsWithResponse(ctx, o.Org, o.Proj, o.AgentName, buildName)
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON200 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON404, resp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, resp.JSON200)
	}

	for _, entry := range resp.JSON200.Logs {
		fmt.Fprintln(o.IO.Out, entry.Log)
	}
	return nil
}

func resolveLatestBuild(ctx context.Context, client *amsvc.ClientWithResponses, o *LogsOptions) (string, error) {
	limit := 1
	resp, err := client.GetAgentBuildsWithResponse(ctx, o.Org, o.Proj, o.AgentName, &amsvc.GetAgentBuildsParams{
		Limit: &limit,
	})
	if err != nil {
		return "", clierr.Newf(clierr.Transport, "%v", err)
	}
	if resp.JSON200 == nil {
		return "", cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON400, resp.JSON404, resp.JSON500))
	}
	if len(resp.JSON200.Builds) == 0 {
		return "", clierr.New(clierr.NotFound, "no builds found for agent")
	}
	return resp.JSON200.Builds[0].BuildName, nil
}
