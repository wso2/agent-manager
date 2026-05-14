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
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/cli/pkg/clients/traceobssvc"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
	"github.com/wso2/agent-manager/cli/pkg/tableprinter"
)

type ListTracesOptions struct {
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

	Condition  string
	MaxLatency int
	MaxTokens  int
	MaxSpans   int
}

func newListRunE(opts *ListTracesOptions, since *string, limit *int, sort *string, condition *string, maxLatency *int, maxTokens *int, maxSpans *int) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
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
		dur, err := parseDuration(*since)
		if err != nil {
			return render.Error(opts.IO, scope, cmdutil.FlagErrorf("--since: %v", err))
		}
		start := end.Add(-dur)
		opts.StartTime = start.Format(time.RFC3339)
		opts.EndTime = end.Format(time.RFC3339)

		if *limit < 1 || *limit > 100 {
			return render.Error(opts.IO, scope, cmdutil.FlagErrorf("--limit must be between 1 and 100"))
		}
		opts.Limit = *limit
		opts.SortOrder = *sort
		opts.Condition = *condition
		opts.MaxLatency = *maxLatency
		opts.MaxTokens = *maxTokens
		opts.MaxSpans = *maxSpans

		if opts.Condition != "" {
			return runFilteredTraces(cmd.Context(), opts)
		}
		return runListTraces(cmd.Context(), opts)
	}
}

func parseTimeOrZero(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func runListTraces(ctx context.Context, o *ListTracesOptions) error {
	client, err := o.TraceClient(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	limit := o.Limit
	sortOrder := o.SortOrder
	resp, err := client.ListTraces(ctx, &traceobssvc.ListTracesParams{
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

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, resp)
	}

	if len(resp.Traces) == 0 {
		fmt.Fprintln(o.IO.Out, "No traces found.")
		return nil
	}

	tp := tableprinter.New(o.IO, "trace id", "status", "duration", "spans", "tokens", "root span", "started")
	for _, tr := range resp.Traces {
		tp.AddField(truncate(tr.TraceID, 16))
		tp.AddField(traceStatus(tr.Status))
		tp.AddField(formatDuration(tr.DurationInNanos))
		tp.AddField(fmt.Sprintf("%d", tr.SpanCount))
		tp.AddField(tokenCount(tr.TokenUsage))
		tp.AddField(truncate(tr.RootSpanName, 20))
		tp.AddField(timeAgo(tr.StartTime))
		tp.EndRow()
	}
	return tp.Render()
}

// runFilteredTraces uses the export endpoint + client-side filtering (matches MCP get_traces behavior)
func runFilteredTraces(ctx context.Context, o *ListTracesOptions) error {
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

	var filtered []traceobssvc.FullTrace
	for _, tr := range resp.Traces {
		if matchesCondition(tr, o) {
			filtered = append(filtered, tr)
		}
	}

	if len(filtered) == 0 {
		if o.IO.JSON {
			return render.JSONSuccess(o.IO, o.Scope, traceobssvc.TraceOverviewResponse{})
		}
		fmt.Fprintln(o.IO.Out, "No traces match the condition.")
		return nil
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, map[string]any{
			"traces": filtered,
			"count":  len(filtered),
		})
	}

	tp := tableprinter.New(o.IO, "trace id", "status", "duration", "spans", "tokens", "root span")
	for _, tr := range filtered {
		tp.AddField(truncate(tr.TraceID, 16))
		tp.AddField(traceStatus(tr.Status))
		tp.AddField(formatDuration(tr.DurationInNanos))
		tp.AddField(fmt.Sprintf("%d", tr.SpanCount))
		tp.AddField(tokenCount(tr.TokenUsage))
		tp.AddField(truncate(tr.RootSpanID, 16))
		tp.EndRow()
	}
	return tp.Render()
}

func matchesCondition(tr traceobssvc.FullTrace, o *ListTracesOptions) bool {
	switch o.Condition {
	case "error_status":
		return tr.Status != nil && tr.Status.ErrorCount > 0
	case "high_latency":
		return tr.DurationInNanos/1_000_000 > int64(o.MaxLatency)
	case "high_token_usage":
		if tr.TokenUsage == nil {
			return false
		}
		return tr.TokenUsage.TotalTokens > o.MaxTokens
	case "tool_call_fails":
		for _, span := range tr.Spans {
			attrs := span.AmpAttributes
			if attrs == nil {
				continue
			}
			if strings.ToLower(attrs.Kind) != "tool" {
				continue
			}
			if attrs.Status != nil && attrs.Status.Error {
				return true
			}
		}
		return false
	case "excessive_steps":
		return tr.SpanCount > o.MaxSpans
	default:
		return false
	}
}

func traceStatus(status *traceobssvc.TraceStatus) string {
	if status != nil && status.ErrorCount > 0 {
		return "error"
	}
	return "ok"
}

func tokenCount(usage *traceobssvc.TokenUsage) string {
	if usage == nil {
		return "0"
	}
	return fmt.Sprintf("%d", usage.TotalTokens)
}
