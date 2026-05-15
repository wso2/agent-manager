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

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clients/traceobssvc"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
	"github.com/wso2/agent-manager/cli/pkg/tableprinter"
)

const (
	conditionErrorStatus    = "error_status"
	conditionHighLatency    = "high_latency"
	conditionHighTokenUsage = "high_token_usage"
	conditionToolCallFails  = "tool_call_fails"
	conditionExcessiveSteps = "excessive_steps"
)

type ListTracesOptions struct {
	IO           *iostreams.IOStreams
	TraceClient  func(context.Context) (*traceobssvc.Client, error)
	AMClient     func(context.Context) (*amsvc.ClientWithResponses, error)
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

func parseTimeOrZero(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func runListTraces(ctx context.Context, o *ListTracesOptions) error {
	if err := cmdutil.ValidatePathParam("agent name", o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
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

	return renderOverviewTable(o, resp.Traces)
}

func renderOverviewTable(o *ListTracesOptions, traces []traceobssvc.TraceOverview) error {
	tp := tableprinter.New(o.IO, "trace id", "status", "duration", "spans", "tokens", "root span", "started")
	for _, tr := range traces {
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

func runFilteredTraces(ctx context.Context, o *ListTracesOptions) error {
	if err := cmdutil.ValidatePathParam("agent name", o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	client, err := o.TraceClient(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	limit := o.Limit
	sortOrder := o.SortOrder
	params := &traceobssvc.ListTracesParams{
		Organization: o.Org,
		Project:      o.Proj,
		Agent:        o.AgentName,
		Environment:  o.Env,
		StartTime:    parseTimeOrZero(o.StartTime),
		EndTime:      parseTimeOrZero(o.EndTime),
		Limit:        &limit,
		SortOrder:    &sortOrder,
	}

	// tool_call_fails needs span attributes; everything else can filter from overview.
	if o.Condition == conditionToolCallFails {
		resp, err := client.ExportTraces(ctx, params)
		if err != nil {
			return render.Error(o.IO, o.Scope, cmdutil.TraceObserverErrorFromResponse(err))
		}
		filtered := make([]traceobssvc.TraceOverview, 0, len(resp.Traces))
		for _, tr := range resp.Traces {
			if matchesFullCondition(tr) {
				filtered = append(filtered, tr.TraceOverview)
			}
		}
		return renderFilteredOverview(o, filtered)
	}

	resp, err := client.ListTraces(ctx, params)
	if err != nil {
		return render.Error(o.IO, o.Scope, cmdutil.TraceObserverErrorFromResponse(err))
	}
	filtered := make([]traceobssvc.TraceOverview, 0, len(resp.Traces))
	for _, tr := range resp.Traces {
		if matchesOverviewCondition(tr, o) {
			filtered = append(filtered, tr)
		}
	}
	return renderFilteredOverview(o, filtered)
}

func renderFilteredOverview(o *ListTracesOptions, traces []traceobssvc.TraceOverview) error {
	if o.IO.JSON {
		if traces == nil {
			traces = []traceobssvc.TraceOverview{}
		}
		return render.JSONSuccess(o.IO, o.Scope, map[string]any{
			"traces": traces,
			"count":  len(traces),
		})
	}
	if len(traces) == 0 {
		fmt.Fprintln(o.IO.Out, "No traces match the condition.")
		return nil
	}
	return renderOverviewTable(o, traces)
}

var validConditions = []string{
	conditionErrorStatus,
	conditionHighLatency,
	conditionHighTokenUsage,
	conditionToolCallFails,
	conditionExcessiveSteps,
}

func validateCondition(c string) error {
	if c == "" {
		return nil
	}
	for _, v := range validConditions {
		if c == v {
			return nil
		}
	}
	return cmdutil.FlagErrorf("--condition: %q is not valid; must be one of %s", c, strings.Join(validConditions, ", "))
}

func matchesOverviewCondition(tr traceobssvc.TraceOverview, o *ListTracesOptions) bool {
	switch o.Condition {
	case conditionErrorStatus:
		return tr.Status != nil && tr.Status.ErrorCount > 0
	case conditionHighLatency:
		return tr.DurationInNanos/1_000_000 > int64(o.MaxLatency)
	case conditionHighTokenUsage:
		return tr.TokenUsage != nil && tr.TokenUsage.TotalTokens > o.MaxTokens
	case conditionExcessiveSteps:
		return tr.SpanCount > o.MaxSpans
	default:
		return false
	}
}

func matchesFullCondition(tr traceobssvc.FullTrace) bool {
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
