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
	since := "24h"
	limit := 10
	sort := "desc"
	condition := ""
	maxLatency := 30000
	maxTokens := 10000
	maxSpans := 40

	cmd := &cobra.Command{
		Use:   "traces <agent>",
		Short: "List and manage traces for an agent",
		Args:  cobra.MaximumNArgs(1),
		RunE:  newListRunE(opts, &since, &limit, &sort, &condition, &maxLatency, &maxTokens, &maxSpans),
	}
	cmd.Flags().StringVar(&since, "since", "24h", "Time window (e.g. 1h, 30m, 7d)")
	cmd.Flags().IntVar(&limit, "limit", 10, "Max traces to return (1-100)")
	cmd.Flags().StringVar(&sort, "sort", "desc", "Sort order: asc or desc")
	cmd.Flags().StringVar(&condition, "condition", "", "Filter: error_status, high_latency, high_token_usage, tool_call_fails, excessive_steps")
	cmd.Flags().IntVar(&maxLatency, "max-latency", 30000, "Latency threshold in ms (for high_latency condition)")
	cmd.Flags().IntVar(&maxTokens, "max-tokens", 10000, "Token threshold (for high_token_usage condition)")
	cmd.Flags().IntVar(&maxSpans, "max-spans", 40, "Span count threshold (for excessive_steps condition)")
	cmdutil.AddEnvFlag(cmd)

	cmd.AddCommand(NewExportCmd(f))
	return cmd
}
