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

package traces

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

func NewTracesCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &ListTracesOptions{
		IO:           f.IOStreams,
		TraceClient:  f.TraceObserver,
		ResolveScope: f.ResolveOrgProject,
		ResolveAgent: f.ResolveAgent,
		ResolveEnv:   f.ResolveEnvironment,
		MakeScope:    f.EnvScope,
	}
	var since string

	cmd := &cobra.Command{
		Use:   "traces <agent>",
		Short: "List and manage traces for an agent",
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

			end := time.Now().UTC()
			dur, err := cmdutil.ParseDuration(since)
			if err != nil {
				return render.Error(opts.IO, scope, cmdutil.FlagErrorf("--since: %v", err))
			}
			start := end.Add(-dur)
			opts.StartTime = start.Format(time.RFC3339)
			opts.EndTime = end.Format(time.RFC3339)

			if opts.Limit < 1 || opts.Limit > 100 {
				return render.Error(opts.IO, scope, cmdutil.FlagErrorf("--limit must be between 1 and 100"))
			}

			if opts.Condition != "" {
				return runFilteredTraces(cmd.Context(), opts)
			}
			return runListTraces(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&since, "since", "24h", "Time window (e.g. 1h, 30m, 7d)")
	cmd.Flags().IntVar(&opts.Limit, "limit", 10, "Max traces to return (1-100)")
	cmd.Flags().StringVar(&opts.SortOrder, "sort", "desc", "Sort order: asc or desc")
	cmd.Flags().StringVar(&opts.Condition, "condition", "", "Filter: error_status, high_latency, high_token_usage, tool_call_fails, excessive_steps")
	cmd.Flags().IntVar(&opts.MaxLatency, "max-latency", 30000, "Latency threshold in ms (for high_latency condition)")
	cmd.Flags().IntVar(&opts.MaxTokens, "max-tokens", 10000, "Token threshold (for high_token_usage condition)")
	cmd.Flags().IntVar(&opts.MaxSpans, "max-spans", 40, "Span count threshold (for excessive_steps condition)")
	cmdutil.AddEnvFlag(cmd)
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return cmdutil.CompleteAgents(cmd, f), cobra.ShellCompDirectiveNoFileComp
	}

	cmd.AddCommand(NewExportCmd(f))
	return cmd
}
