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
	"time"

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
	ResolveAgent func([]string) (string, []string, error)
	ResolveEnv   func(*cobra.Command) (string, error)
	MakeScope    func(string, string, string, string) render.Scope

	Org       string
	Proj      string
	AgentName string
	Env       string
	Scope     render.Scope
	StartTime string
	EndTime   string
	Limit     *int
	SortOrder *amsvc.LogFilterRequestSortOrder
	LogLevels *[]amsvc.LogFilterRequestLogLevels
	Grep      *string
}

func NewLogsCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &LogsOptions{
		IO:           f.IOStreams,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		ResolveAgent: f.ResolveAgent,
		ResolveEnv:   f.ResolveEnvironment,
		MakeScope:    f.EnvScope,
	}
	var since string
	var levels []string
	var grep string
	var limit int
	var sort string

	cmd := &cobra.Command{
		Use:   "logs <agent>",
		Short: "Fetch runtime logs for a deployed agent",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, proj, err := opts.ResolveScope(cmd, true, true)
			if err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}
			agentName, _, err := opts.ResolveAgent(args)
			if err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}
			env, err := opts.ResolveEnv(cmd)
			if err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}

			scope := opts.MakeScope(org, proj, agentName, env)
			opts.Org, opts.Proj, opts.AgentName, opts.Env, opts.Scope = org, proj, agentName, env, scope

			opts.StartTime, opts.EndTime, err = cmdutil.ResolveSinceWindow(since)
			if err != nil {
				return render.Error(opts.IO, scope, cmdutil.FlagErrorf("--since: %v", err))
			}

			if cmd.Flags().Changed("limit") {
				if limit < 1 || limit > 10000 {
					return render.Error(opts.IO, scope, cmdutil.FlagErrorf("--limit must be between 1 and 10000"))
				}
				opts.Limit = &limit
			}
			if sort != "" {
				s := amsvc.LogFilterRequestSortOrder(sort)
				opts.SortOrder = &s
			}
			if len(levels) > 0 {
				ll := make([]amsvc.LogFilterRequestLogLevels, len(levels))
				for i, l := range levels {
					ll[i] = amsvc.LogFilterRequestLogLevels(l)
				}
				opts.LogLevels = &ll
			}
			if grep != "" {
				opts.Grep = &grep
			}

			return runLogs(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&since, "since", "24h", "Time window (e.g. 1h, 30m, 7d)")
	cmd.Flags().StringSliceVar(&levels, "level", nil, "Filter by log level (DEBUG, INFO, WARN, ERROR)")
	cmd.Flags().StringVar(&grep, "grep", "", "Search phrase to filter logs")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of log entries")
	cmd.Flags().StringVar(&sort, "sort", "desc", "Sort order: asc or desc")
	cmdutil.AddEnvFlag(cmd)
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return cmdutil.CompleteAgents(cmd, f), cobra.ShellCompDirectiveNoFileComp
	}
	return cmd
}

func runLogs(ctx context.Context, o *LogsOptions) error {
	if err := cmdutil.ValidatePathParam("agent name", o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	if err := cmdutil.ValidateRuntimeManaged(ctx, client, o.Org, o.Proj, o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	resp, err := client.FilterAgentRuntimeLogsWithResponse(ctx, o.Org, o.Proj, o.AgentName,
		amsvc.LogFilterRequest{
			EnvironmentName: o.Env,
			StartTime:       o.StartTime,
			EndTime:         o.EndTime,
			Limit:           o.Limit,
			SortOrder:       o.SortOrder,
			LogLevels:       o.LogLevels,
			SearchPhrase:    o.Grep,
		},
	)
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON200 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse,
			cmdutil.FirstNonNil(resp.JSON400, resp.JSON404, resp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, resp.JSON200)
	}

	for _, entry := range resp.JSON200.Logs {
		fmt.Fprintf(o.IO.Out, "%s  %-5s  %s\n",
			entry.Timestamp.Format(time.RFC3339),
			entry.LogLevel,
			entry.Log,
		)
	}
	return nil
}
