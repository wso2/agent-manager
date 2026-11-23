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
		TraceClient:  f.Traces,
		ResolveScope: f.ResolveOrgProject,
		ResolveAgent: f.ResolveAgent,
		MakeScope:    f.AgentScope,
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
			"Traces come from the observability module, which lists the window's traces by start time. " +
			"It doesn't filter them yet, so the filter flags are hidden and refused. --condition tool_call_fails " +
			"still works: the CLI checks it over one page of exported traces, by span attributes. " +
			"For older traces, narrow the window with --start and --end, which set a fixed window instead of --since.\n\n" +
			"--console-url prints the console path that shows the same list; paste it after your console's host. " +
			"--from-url takes a console URL, path or \"?...\" query and lists the same traces. Its path sets the " +
			"org, project and agent, so <agent> is optional; with no time range in the URL, the window " +
			"is the console's 1h. --limit, --sort, --since, --start and --end override the URL.\n\n" +
			"Examples:\n" +
			"  amctl agent traces my-agent --since 3h --console-url\n" +
			"  amctl agent traces --from-url 'https://console.example.com/org/acme/project/triage/agents/my-agent/" +
			"environment/default/observability/traces?timeRange=3h'",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if fromURL != "" {
				var err error
				if args, err = applyFromURL(cmd, opts.IO.ErrOut, fromURL, args, opts.Condition); err != nil {
					return render.Error(opts.IO, render.Scope{}, err)
				}
			}
			if err := checkModuleSupport(cmd.Flags(), opts.Condition); err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}
			org, proj, err := opts.ResolveScope(cmd, true, true)
			if err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}
			agentName, _, err := opts.ResolveAgent(args)
			if err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}

			scope := opts.MakeScope(org, proj, agentName)
			opts.Org, opts.Proj, opts.AgentName, opts.Scope = org, proj, agentName, scope

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
	cmd.Flags().StringVar(&opts.Condition, "condition", "", "Shorthand filter: tool_call_fails")
	cmd.Flags().IntVar(&opts.MaxLatency, "max-latency", 30000, "Latency threshold in ms; high_latency matches traces above it")
	cmd.Flags().IntVar(&opts.MaxTokens, "max-tokens", 10000, "Token threshold; high_token_usage matches traces above it")
	cmd.Flags().IntVar(&opts.MaxSpans, "max-spans", 40, "Span count threshold; excessive_steps matches traces above it")
	addConsoleFlags(cmd, &opts.ShowConsoleURL, &fromURL)
	hideUnsupportedFlags(cmd)
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return cmdutil.CompleteAgents(cmd, f), cobra.ShellCompDirectiveNoFileComp
	}

	cmd.AddCommand(NewExportCmd(f))
	return cmd
}
