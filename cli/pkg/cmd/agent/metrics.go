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

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
	"github.com/wso2/agent-manager/cli/pkg/tableprinter"
)

type MetricsOptions struct {
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
}

func NewMetricsCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &MetricsOptions{
		IO:           f.IOStreams,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		ResolveAgent: f.ResolveAgent,
		ResolveEnv:   f.ResolveEnvironment,
		MakeScope:    f.EnvScope,
	}
	var since string

	cmd := &cobra.Command{
		Use:   "metrics <agent>",
		Short: "Fetch CPU and memory usage metrics for an agent",
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

			return runMetrics(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&since, "since", "24h", "Time window (e.g. 1h, 30m, 7d)")
	cmdutil.AddEnvFlag(cmd)
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return cmdutil.CompleteAgents(cmd, f), cobra.ShellCompDirectiveNoFileComp
	}
	return cmd
}

func runMetrics(ctx context.Context, o *MetricsOptions) error {
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

	resp, err := client.GetAgentMetricsWithResponse(ctx, o.Org, o.Proj, o.AgentName,
		amsvc.MetricsFilterRequest{
			EnvironmentName: o.Env,
			StartTime:       o.StartTime,
			EndTime:         o.EndTime,
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

	m := resp.JSON200
	tp := tableprinter.New(o.IO, "metric", "current", "request", "limit")
	tp.AddField("CPU")
	tp.AddField(formatCPU(lastValue(m.CpuUsage)))
	tp.AddField(formatCPU(lastValue(m.CpuRequests)))
	tp.AddField(formatCPU(lastValue(m.CpuLimits)))
	tp.EndRow()
	tp.AddField("Memory")
	tp.AddField(formatMemory(lastValue(m.Memory)))
	tp.AddField(formatMemory(lastValue(m.MemoryRequests)))
	tp.AddField(formatMemory(lastValue(m.MemoryLimits)))
	tp.EndRow()
	return tp.Render()
}

func lastValue(points []amsvc.MetricDataPoint) float64 {
	if len(points) == 0 {
		return 0
	}
	return points[len(points)-1].Value
}

func formatCPU(cores float64) string {
	if cores < 1 {
		return fmt.Sprintf("%.0fm", cores*1000)
	}
	return fmt.Sprintf("%.2f", cores)
}

func formatMemory(bytes float64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1fGi", bytes/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.0fMi", bytes/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.0fKi", bytes/(1<<10))
	default:
		return fmt.Sprintf("%.0fB", bytes)
	}
}
