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
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/cli/pkg/clients/traceobssvc"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
	"github.com/wso2/agent-manager/cli/pkg/tableprinter"
)

type TraceOptions struct {
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
	TraceID   string
	SpanID    string
	StartTime string
	EndTime   string
	Limit     int
}

func NewTraceCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &TraceOptions{
		IO:           f.IOStreams,
		TraceClient:  f.TraceObserver,
		ResolveScope: f.ResolveOrgProject,
		ResolveAgent: f.ResolveAgent,
		ResolveEnv:   f.ResolveEnvironment,
		MakeScope:    f.EnvScope,
	}
	var since string
	var spanID string
	var limit int

	cmd := &cobra.Command{
		Use:   "trace <agent> <traceId>",
		Short: "Get span details for a single trace",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, proj, err := opts.ResolveScope(cmd, true, true)
			if err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}
			agentName, remaining, err := opts.ResolveAgent(args)
			if err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}
			if len(remaining) < 1 {
				return render.Error(opts.IO, render.Scope{}, cmdutil.FlagErrorf("trace ID is required"))
			}
			env, err := opts.ResolveEnv(cmd)
			if err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}

			scope := opts.MakeScope(org, proj, agentName, env)
			opts.Org, opts.Proj, opts.AgentName, opts.Env, opts.Scope = org, proj, agentName, env, scope
			opts.TraceID = remaining[0]
			opts.SpanID = spanID
			opts.Limit = limit

			end := time.Now().UTC()
			dur, err := cmdutil.ParseDuration(since)
			if err != nil {
				return render.Error(opts.IO, scope, cmdutil.FlagErrorf("--since: %v", err))
			}
			start := end.Add(-dur)
			opts.StartTime = start.Format(time.RFC3339)
			opts.EndTime = end.Format(time.RFC3339)

			if spanID != "" {
				return runSpanDetail(cmd.Context(), opts)
			}
			return runTrace(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&since, "since", "24h", "Time window (e.g. 1h, 30m, 7d)")
	cmd.Flags().StringVar(&spanID, "span", "", "Show full detail for a specific span ID")
	cmd.Flags().IntVar(&limit, "limit", 1000, "Max spans to return")
	cmdutil.AddEnvFlag(cmd)
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return cmdutil.CompleteAgents(cmd, f), cobra.ShellCompDirectiveNoFileComp
	}
	return cmd
}

func runTrace(ctx context.Context, o *TraceOptions) error {
	if err := cmdutil.ValidatePathParam("agent name", o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	if err := cmdutil.ValidatePathParam("trace id", o.TraceID); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	client, err := o.TraceClient(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	limit := o.Limit
	resp, err := client.GetTraceSpans(ctx, o.TraceID, &traceobssvc.GetTraceSpansParams{
		Organization: o.Org,
		StartTime:    parseTimeOrZero(o.StartTime),
		EndTime:      parseTimeOrZero(o.EndTime),
		Project:      &o.Proj,
		Agent:        &o.AgentName,
		Environment:  &o.Env,
		Limit:        &limit,
	})
	if err != nil {
		return render.Error(o.IO, o.Scope, cmdutil.TraceObserverErrorFromResponse(err))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, resp)
	}

	fmt.Fprintf(o.IO.Out, "Trace %s  Spans: %d\n\n", o.TraceID, resp.TotalCount)

	if len(resp.Spans) == 0 {
		fmt.Fprintln(o.IO.Out, "No spans found.")
		return nil
	}

	sort.Slice(resp.Spans, func(i, j int) bool { return resp.Spans[i].StartTime.Before(resp.Spans[j].StartTime) })

	tp := tableprinter.New(o.IO, "span id", "parent", "name", "duration")
	for _, s := range resp.Spans {
		tp.AddField(s.SpanID)
		parent := s.ParentSpanID
		if parent == "" {
			parent = "-"
		}
		tp.AddField(parent)
		tp.AddField(s.SpanName)
		tp.AddField(formatDuration(s.DurationNs))
		tp.EndRow()
	}
	return tp.Render()
}

func runSpanDetail(ctx context.Context, o *TraceOptions) error {
	if err := cmdutil.ValidatePathParam("trace id", o.TraceID); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	if err := cmdutil.ValidatePathParam("span id", o.SpanID); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	client, err := o.TraceClient(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	span, err := client.GetSpanDetail(ctx, o.TraceID, o.SpanID)
	if err != nil {
		return render.Error(o.IO, o.Scope, cmdutil.TraceObserverErrorFromResponse(err))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, span)
	}

	return printSpanDetail(o.IO, span)
}

func printSpanDetail(io *iostreams.IOStreams, s *traceobssvc.Span) error {
	parent := s.ParentSpanID
	if parent == "" {
		parent = "-"
	}
	status := s.Status
	if s.AmpAttributes != nil && s.AmpAttributes.Status != nil && s.AmpAttributes.Status.Error {
		status = "error"
	}
	fmt.Fprintf(io.Out, "Span ID:        %s\n", s.SpanID)
	fmt.Fprintf(io.Out, "Parent Span ID: %s\n", parent)
	fmt.Fprintf(io.Out, "Name:           %s\n", s.Name)
	fmt.Fprintf(io.Out, "Service:        %s\n", s.Service)
	fmt.Fprintf(io.Out, "Kind:           %s\n", s.Kind)
	fmt.Fprintf(io.Out, "Status:         %s\n", status)
	fmt.Fprintf(io.Out, "Duration:       %s\n", formatDuration(s.DurationInNanos))
	fmt.Fprintf(io.Out, "Start:          %s\n", s.StartTime.Format(time.RFC3339))
	fmt.Fprintf(io.Out, "End:            %s\n", s.EndTime.Format(time.RFC3339))
	if len(s.Attributes) > 0 {
		fmt.Fprintln(io.Out, "Attributes:")
		keys := make([]string, 0, len(s.Attributes))
		for k := range s.Attributes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(io.Out, "  %s: %v\n", k, s.Attributes[k])
		}
	}
	return nil
}
