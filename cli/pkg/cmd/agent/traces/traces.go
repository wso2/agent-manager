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
	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

// NewTracesCmd creates the `agent traces` command.
func NewTracesCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &ListTracesOptions{
		IO:           f.IOStreams,
		TraceClient:  f.Observer,
		AMClient:     f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		ResolveAgent: f.ResolveAgent,
		ResolveEnv:   f.ResolveEnvironment,
		MakeScope:    f.EnvScope,
	}
	var (
		window  windowFlags
		filters filterFlagValues
		fromURL string
	)

	cmd := &cobra.Command{
		Use:   "traces <agent>",
		Short: "List and manage traces for an agent",
		Long: "List traces for an agent.\n\n" +
			"Filters run on the server across the whole --since window, not just one page, and AND together. " +
			"If the server stops searching early, a notice on stderr says so; a narrower time range shows more.\n\n" +
			"--condition is a shorthand for one filter: error_status is --status error, " +
			"high_latency is a duration above --max-latency, high_token_usage is tokens above --max-tokens, " +
			"and excessive_steps is spans above --max-spans. tool_call_fails is checked in the CLI " +
			"over one page of exported traces.\n\n" +
			"--tool and --mcp-server match any tool or MCP server whose name contains the value, ignoring case. " +
			"--tool-error keeps traces with a failed tool; with --tool, the failed tool must be one --tool matches. " +
			"--tool-error searches the whole window on the server, but it knows a tool span only by its name " +
			"and a failure only by the span's OTel status. tool_call_fails reads span attributes instead, so it also " +
			"finds tool spans named after the function and failures marked only by error.type or gen_ai.tool.status. " +
			"MCP servers show only for traces that open an MCP session.\n\n" +
			"--no-conversation keeps traces with no conversation ID. --start and --end set a fixed window " +
			"instead of --since. When the server stops searching early in table output, the notice prints the " +
			"command for the next page; JSON output carries nextCursor, which --cursor takes with the same " +
			"window, sort and filters.\n\n" +
			"--console-url prints the console path that shows the same list; paste it after your console's host. " +
			"--from-url takes a console URL, path or \"?...\" query and lists the same traces. Its path sets the " +
			"org, project, agent and environment, so <agent> is optional; with no time range in the URL, the window " +
			"is the console's 1h. A filter can't come from both the URL and its flag; --limit, --sort, --since, " +
			"--start and --end override the URL.\n\n" +
			"Examples:\n" +
			"  amctl agent traces my-agent --env dev --min-duration 5s --since 3h --console-url\n" +
			"  amctl agent traces --from-url 'https://console.example.com/org/acme/project/triage/agents/my-agent/" +
			"environment/dev/observability/traces?minDurationMs=5000&timeRange=3h'",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if fromURL != "" {
				var err error
				if args, err = applyFromURL(cmd, opts.IO.ErrOut, fromURL, args, opts.Condition); err != nil {
					return render.Error(opts.IO, render.Scope{}, err)
				}
			}
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

			opts.StartTime, opts.EndTime, err = resolveWindow(&window, cmd.Flags().Changed("since"))
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			if window.start == "" {
				opts.Since = window.since
			}
			opts.CommandPath = cmd.CommandPath()

			if opts.Limit < 1 || opts.Limit > 100 {
				return render.Error(opts.IO, scope, cmdutil.FlagErrorf("--limit must be between 1 and 100"))
			}

			applyFilterFlags(cmd.Flags().Changed, opts, &filters)
			if err := resolveFilters(opts); err != nil {
				return render.Error(opts.IO, scope, err)
			}

			if err := preflightEnv(cmd.Context(), opts.AMClient, org, env); err != nil {
				return render.Error(opts.IO, scope, err)
			}

			if opts.Condition == conditionToolCallFails {
				return runToolCallFails(cmd.Context(), opts)
			}
			return runListTraces(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&window.since, "since", "24h", "Time window (e.g. 1h, 30m, 7d)")
	addWindowFlags(cmd, &window)
	cmd.Flags().IntVar(&opts.Limit, "limit", 10, "Max traces to return (1-100)")
	cmd.Flags().StringVar(&opts.SortOrder, "sort", "desc", "Sort order: asc or desc")
	cmd.Flags().StringVar(&opts.Cursor, "cursor", "", "Continue from the nextCursor of an earlier run; keep its window, sort and filters")
	addFilterFlags(cmd, opts, &filters, true)
	cmd.Flags().BoolVar(&opts.ShowModels, "show-models", false, "Add a Model column (costs the server one extra call per trace)")
	cmd.Flags().BoolVar(&opts.ShowTools, "show-tools", false, "Add a Tools column, with failed tools marked (costs the server up to one extra call per trace)")
	cmd.Flags().BoolVar(&opts.ShowMCPServers, "show-mcp-servers", false, "Add an MCP Servers column (costs the server extra calls per trace)")
	cmd.Flags().StringVar(&opts.Condition, "condition", "", "Shorthand filter: error_status, high_latency, high_token_usage, tool_call_fails, excessive_steps")
	cmd.Flags().IntVar(&opts.MaxLatency, "max-latency", 30000, "Latency threshold in ms; high_latency matches traces above it")
	cmd.Flags().IntVar(&opts.MaxTokens, "max-tokens", 10000, "Token threshold; high_token_usage matches traces above it")
	cmd.Flags().IntVar(&opts.MaxSpans, "max-spans", 40, "Span count threshold; excessive_steps matches traces above it")
	addConsoleFlags(cmd, &opts.ShowConsoleURL, &fromURL)
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
