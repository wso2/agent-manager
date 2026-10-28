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
	"strings"
	"time"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clients/observersvc"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
	"github.com/wso2/agent-manager/cli/pkg/tableprinter"
)

type TraceOptions struct {
	IO           *iostreams.IOStreams
	TraceClient  func(context.Context) (*observersvc.Client, error)
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
	TraceID   string
	SpanID    string
	StartTime string
	EndTime   string
	Limit     int

	// Since is --since when it set the window; ShowConsoleURL prints the console link.
	Since          string
	ShowConsoleURL bool
}

// NewTraceCmd returns the `agent trace` command.
func NewTraceCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &TraceOptions{
		IO:           f.IOStreams,
		TraceClient:  f.Observer,
		AMClient:     f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		ResolveAgent: f.ResolveAgent,
		ResolveEnv:   f.ResolveEnvironment,
		MakeScope:    f.EnvScope,
	}
	var window windowFlags
	var spanID string
	var limit int
	var fromURL string

	cmd := &cobra.Command{
		Use:   "trace <agent> <traceId>",
		Short: "Get span details for a single trace",
		Long: "Get span details for a single trace.\n\n" +
			"--console-url prints the console path that opens this trace; paste it after your console's host. " +
			"--from-url takes a console URL, path or \"?...\" query with a trace open and shows that trace " +
			"(selectedTrace), searched over the URL's time range, or the console's 1h when it has none. Its path " +
			"sets the org, project, agent and environment, so with a path one argument is the trace ID (unless it " +
			"names the URL's agent), and it overrides selectedTrace. --since, --start and --end override the URL's time range.\n\n" +
			"Examples:\n" +
			"  amctl agent trace my-agent 4bf92f3577b34da6a3ce929d0e0e4736 --env dev --console-url\n" +
			"  amctl agent trace --from-url 'https://console.example.com/org/acme/project/triage/agents/my-agent/" +
			"environment/dev/observability/traces?timeRange=1d&selectedTrace=4bf92f3577b34da6a3ce929d0e0e4736'",
		Args: func(cmd *cobra.Command, args []string) error {
			if fromURL != "" {
				return cobra.MaximumNArgs(2)(cmd, args)
			}
			return cobra.MinimumNArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var selected string
			if fromURL != "" {
				u, err := parseConsoleURL(fromURL)
				if err == nil {
					err = u.applyScope(cmd.Flags())
				}
				if err == nil {
					args, err = u.traceArgs(args)
				}
				if err == nil {
					err = u.applyQuery(cmd.Flags(), windowParams(), "")
				}
				if err != nil {
					return render.Error(opts.IO, render.Scope{}, err)
				}
				u.warnUnknown(opts.IO.ErrOut)
				selected = u.get(selectedTraceParam)
			}
			org, proj, err := opts.ResolveScope(cmd, true, true)
			if err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}
			agentName, remaining, err := opts.ResolveAgent(args)
			if err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}
			if len(remaining) > 0 {
				selected = remaining[0]
			}
			if selected == "" && fromURL != "" {
				return render.Error(opts.IO, render.Scope{},
					cmdutil.FlagErrorf("--from-url has no selectedTrace; open the trace in the console first, or pass its ID"))
			}
			if selected == "" {
				return render.Error(opts.IO, render.Scope{}, cmdutil.FlagErrorf("trace ID is required"))
			}
			env, err := opts.ResolveEnv(cmd)
			if err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}

			scope := opts.MakeScope(org, proj, agentName, env)
			opts.Org, opts.Proj, opts.AgentName, opts.Env, opts.Scope = org, proj, agentName, env, scope
			opts.TraceID = selected
			opts.SpanID = spanID
			opts.Limit = limit

			if err := preflightEnv(cmd.Context(), opts.AMClient, org, env); err != nil {
				return render.Error(opts.IO, scope, err)
			}

			opts.StartTime, opts.EndTime, err = resolveWindow(&window, cmd.Flags().Changed("since"))
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			if window.start == "" {
				opts.Since = window.since
			}

			if spanID != "" {
				return runSpanDetail(cmd.Context(), opts)
			}
			return runTrace(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&window.since, "since", "24h", "Time window (e.g. 1h, 30m, 7d)")
	addWindowFlags(cmd, &window)
	cmd.Flags().StringVar(&spanID, "span", "", "Show full detail for a specific span ID")
	cmd.Flags().IntVar(&limit, "limit", 1000, "Max spans to return")
	addConsoleFlags(cmd, &opts.ShowConsoleURL, &fromURL)
	cmdutil.AddEnvFlag(cmd)
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return cmdutil.CompleteAgents(cmd, f), cobra.ShellCompDirectiveNoFileComp
	}
	return cmd
}

// runTrace prints a trace's spans.
func runTrace(ctx context.Context, o *TraceOptions) error {
	if err := cmdutil.ValidatePathParam("agent name", o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	o.TraceID = strings.ToLower(o.TraceID)
	if err := cmdutil.ValidatePathParam("trace id", o.TraceID); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	client, err := o.TraceClient(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	limit := o.Limit
	resp, err := client.GetTraceSpans(ctx, o.TraceID, &observersvc.GetTraceSpansParams{
		Organization: o.Org,
		StartTime:    parseTimeOrZero(o.StartTime),
		EndTime:      parseTimeOrZero(o.EndTime),
		Project:      &o.Proj,
		Agent:        &o.AgentName,
		Environment:  &o.Env,
		Limit:        &limit,
	})
	if err != nil {
		return render.Error(o.IO, o.Scope, cmdutil.ObserverErrorFromResponse(err))
	}

	link := traceConsoleLink(o)
	if o.IO.JSON {
		data, err := withConsoleURL(resp, link)
		if err != nil {
			return render.Error(o.IO, o.Scope, err)
		}
		return render.JSONSuccess(o.IO, o.Scope, data)
	}

	fmt.Fprintf(o.IO.Out, "Trace %s  Spans: %d\n\n", o.TraceID, resp.TotalCount)

	if len(resp.Spans) == 0 {
		fmt.Fprintln(o.IO.Out, "No spans found.")
		printConsoleLink(o.IO, link)
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
	if err := tp.Render(); err != nil {
		return err
	}
	printConsoleLink(o.IO, link)
	return nil
}

// traceConsoleLink is the console link that opens o's trace, or "" without --console-url.
func traceConsoleLink(o *TraceOptions) string {
	if !o.ShowConsoleURL {
		return ""
	}
	return consoleLink(&ListTracesOptions{
		Org: o.Org, Proj: o.Proj, AgentName: o.AgentName, Env: o.Env,
		Since: o.Since, StartTime: o.StartTime, EndTime: o.EndTime,
	}, windowParams(), o.TraceID)
}

// runSpanDetail prints one span's details.
func runSpanDetail(ctx context.Context, o *TraceOptions) error {
	o.TraceID = strings.ToLower(o.TraceID)
	o.SpanID = strings.ToLower(o.SpanID)
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
		return render.Error(o.IO, o.Scope, cmdutil.ObserverErrorFromResponse(err))
	}

	link := traceConsoleLink(o)
	if o.IO.JSON {
		data, err := withConsoleURL(span, link)
		if err != nil {
			return render.Error(o.IO, o.Scope, err)
		}
		return render.JSONSuccess(o.IO, o.Scope, data)
	}

	if err := printSpanDetail(o.IO, span); err != nil {
		return err
	}
	printConsoleLink(o.IO, link)
	return nil
}

// printSpanDetail prints s's fields.
func printSpanDetail(io *iostreams.IOStreams, s *observersvc.Span) error {
	parent := s.ParentSpanID
	if parent == "" {
		parent = "-"
	}
	status := s.Status
	if s.AmpAttributes != nil && s.AmpAttributes.Status != nil && s.AmpAttributes.Status.Error {
		status = "error"
	}
	service := s.Service
	if s.Resource != nil {
		if name, ok := s.Resource["service.name"].(string); ok && name != "" {
			service = name
		}
	}
	kind := s.Kind
	if s.AmpAttributes != nil && s.AmpAttributes.Kind != "" {
		kind = s.AmpAttributes.Kind
	}
	fmt.Fprintf(io.Out, "Span ID:        %s\n", s.SpanID)
	fmt.Fprintf(io.Out, "Parent Span ID: %s\n", parent)
	fmt.Fprintf(io.Out, "Name:           %s\n", s.Name)
	fmt.Fprintf(io.Out, "Service:        %s\n", service)
	fmt.Fprintf(io.Out, "Kind:           %s\n", kind)
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
