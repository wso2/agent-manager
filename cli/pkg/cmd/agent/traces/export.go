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
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/cli/pkg/clients/traceobssvc"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

type ExportTracesOptions struct {
	IO           *iostreams.IOStreams
	TraceClient  func(context.Context) (*traceobssvc.Client, error)
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
	Limit     int
	SortOrder string
}

func NewExportCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &ExportTracesOptions{
		IO:           f.IOStreams,
		TraceClient:  f.TraceObserver,
		ResolveScope: f.ResolveOrgProject,
		ResolveAgent: f.ResolveAgent,
		ResolveEnv:   f.ResolveEnvironment,
		MakeScope:    f.EnvScope,
	}
	var since string

	cmd := &cobra.Command{
		Use:   "export <agent>",
		Short: "Export traces with full span data as JSON",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IO.JSON = true
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

			return runExportTraces(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "Time window (required, e.g. 1h, 30m, 7d)")
	_ = cmd.MarkFlagRequired("since")
	cmd.Flags().IntVar(&opts.Limit, "limit", 100, "Max traces to export (1-100)")
	cmd.Flags().StringVar(&opts.SortOrder, "sort", "desc", "Sort order: asc or desc")
	cmdutil.AddEnvFlag(cmd)
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return cmdutil.CompleteAgents(cmd, f), cobra.ShellCompDirectiveNoFileComp
	}
	return cmd
}

func runExportTraces(ctx context.Context, o *ExportTracesOptions) error {
	if err := cmdutil.ValidatePathParam("agent name", o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	client, err := o.TraceClient(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	limit := o.Limit
	sortOrder := o.SortOrder
	resp, err := client.ExportTraces(ctx, &traceobssvc.ExportTracesParams{
		Organization: o.Org,
		Project:      o.Proj,
		Agent:        o.AgentName,
		Environment:  o.Env,
		StartTime:    parseTimeOrZero(o.StartTime),
		EndTime:      parseTimeOrZero(o.EndTime),
		Limit:        &limit,
		SortOrder:    &sortOrder,
	})
	if err != nil {
		return render.Error(o.IO, o.Scope, cmdutil.TraceObserverErrorFromResponse(err))
	}

	return render.JSONSuccess(o.IO, o.Scope, resp)
}
