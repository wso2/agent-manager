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
	"errors"
	"fmt"
	"net/http"
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

const (
	conditionErrorStatus    = "error_status"
	conditionHighLatency    = "high_latency"
	conditionHighTokenUsage = "high_token_usage"
	conditionToolCallFails  = "tool_call_fails"
	conditionExcessiveSteps = "excessive_steps"

	statusError = "error"
	statusOK    = "ok"
)

type ListTracesOptions struct {
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
	StartTime string
	EndTime   string
	Limit     int
	SortOrder string

	Condition  string
	MaxLatency int
	MaxTokens  int
	MaxSpans   int

	// Server-side filters; nil or empty means unset.
	Status         string
	MinDurationMs  *int64
	MinTokens      *int64
	MinSpans       *int64
	Model          string
	ConversationID string
	NoConversation bool
	ShowModels     bool

	Tool           string
	ToolError      bool
	MCPServer      string
	ShowTools      bool
	ShowMCPServers bool

	// Cursor continues an earlier list; CommandPath starts the printed next-page command.
	Cursor      string
	CommandPath string

	// Since is --since when it set the window; ShowConsoleURL prints the console link.
	Since          string
	ShowConsoleURL bool
}

// consoleLinkFor is the list's console link, or "" without --console-url.
func consoleLinkFor(o *ListTracesOptions) string {
	if !o.ShowConsoleURL {
		return ""
	}
	return consoleLink(o, consoleParams, "")
}

// parseTimeOrZero parses an RFC 3339 time, or returns the zero time.
func parseTimeOrZero(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// listParams builds the request shared by the list and tool_call_fails paths.
func listParams(o *ListTracesOptions) *observersvc.ListTracesParams {
	limit := o.Limit
	sortOrder := o.SortOrder
	p := &observersvc.ListTracesParams{
		Organization: o.Org,
		Project:      o.Proj,
		Agent:        o.AgentName,
		Environment:  o.Env,
		StartTime:    parseTimeOrZero(o.StartTime),
		EndTime:      parseTimeOrZero(o.EndTime),
		Limit:        &limit,
		SortOrder:    &sortOrder,
	}
	setFilterParams(p, o)
	return p
}

// setFilterParams copies o's filters onto p.
func setFilterParams(p *observersvc.ListTracesParams, o *ListTracesOptions) {
	p.Status = o.Status
	p.MinDurationMs = o.MinDurationMs
	p.MinTokens = o.MinTokens
	p.MinSpanCount = o.MinSpans
	p.Model = o.Model
	p.ConversationID = o.ConversationID
	if o.NoConversation {
		p.Missing = []string{"conversationId"}
	}
	p.Tool = o.Tool
	p.ToolError = o.ToolError
	p.MCPServer = o.MCPServer
}

// hasFilters reports whether any filter flag is set.
func hasFilters(o *ListTracesOptions) bool {
	return o.Status != "" || o.MinDurationMs != nil || o.MinTokens != nil || o.MinSpans != nil ||
		o.Model != "" || o.ConversationID != "" || o.NoConversation || o.Tool != "" || o.ToolError ||
		o.MCPServer != ""
}

// runListTraces lists trace overviews, filtered server-side.
func runListTraces(ctx context.Context, o *ListTracesOptions) error {
	if err := cmdutil.ValidatePathParam("agent name", o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	client, err := o.TraceClient(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	params := listParams(o)
	params.IncludeModels = o.ShowModels
	params.IncludeTools = o.ShowTools
	params.IncludeMCPServers = o.ShowMCPServers
	params.Cursor = o.Cursor
	resp, err := client.ListTraces(ctx, params)
	if err != nil {
		if o.Cursor != "" && isInvalidCursor(err) {
			return render.Error(o.IO, o.Scope, cmdutil.FlagErrorf("--cursor: not a nextCursor the server accepts"))
		}
		return render.Error(o.IO, o.Scope, cmdutil.ObserverErrorFromResponse(err))
	}

	link := consoleLinkFor(o)
	if o.IO.JSON {
		data, err := withConsoleURL(resp, link)
		if err != nil {
			return render.Error(o.IO, o.Scope, err)
		}
		return render.JSONSuccess(o.IO, o.Scope, data)
	}

	if len(resp.Traces) == 0 {
		if hasFilters(o) {
			fmt.Fprintln(o.IO.Out, "No traces match the filters.")
		} else {
			fmt.Fprintln(o.IO.Out, "No traces found.")
		}
	} else if err := renderOverviewTable(o, resp.Traces, overviewColumns{
		conversation: true,
		models:       o.ShowModels || o.Model != "",
		tools:        o.ShowTools || o.Tool != "" || o.ToolError,
		mcpServers:   o.ShowMCPServers || o.MCPServer != "",
	}); err != nil {
		return err
	}
	printConsoleLink(o.IO, link)
	if resp.Truncated {
		fmt.Fprintln(o.IO.ErrOut, truncatedNotice(o, resp.NextCursor))
	}
	return nil
}

// isInvalidCursor reports whether the server rejected the request's cursor.
func isInvalidCursor(err error) bool {
	var herr *observersvc.HTTPError
	// Must match the message in agent-manager-observer/handlers/handlers.go.
	return errors.As(err, &herr) && herr.StatusCode == http.StatusBadRequest &&
		herr.Body != nil && herr.Body.Message == "invalid cursor"
}

// truncatedNotice explains a truncated list. With a cursor the examine cap or
// the time budget stopped the search, and the notice gives the next page's
// command; without one the list hit the depth cap.
func truncatedNotice(o *ListTracesOptions, cursor string) string {
	if cursor != "" {
		return "Showing matches from the traces searched so far; for the next page, run:\n  " + nextPageCommand(o, cursor)
	}
	return "The list stops here: the remaining traces are more than 1,000 traces into this time range; narrow the time range to see more."
}

// overviewColumns picks the optional columns of the overview table.
type overviewColumns struct {
	conversation, models, tools, mcpServers bool
}

// renderOverviewTable prints traces as a table, with the optional columns cols picks.
func renderOverviewTable(o *ListTracesOptions, traces []observersvc.TraceOverview, cols overviewColumns) error {
	headers := []string{"status", "trace id"}
	if cols.conversation {
		headers = append(headers, "conversation")
	}
	headers = append(headers, "started", "duration", "tokens", "spans")
	if cols.models {
		headers = append(headers, "model")
	}
	if cols.tools {
		headers = append(headers, "tools")
	}
	if cols.mcpServers {
		headers = append(headers, "mcp servers")
	}
	tp := tableprinter.New(o.IO, headers...)
	for _, tr := range traces {
		tp.AddField(traceStatus(tr.Status))
		tp.AddField(tr.TraceID)
		if cols.conversation {
			tp.AddField(orDash(tr.ConversationID))
		}
		tp.AddField(timeAgo(tr.StartTime))
		tp.AddField(formatDuration(tr.DurationInNanos))
		tp.AddField(tokenCount(tr.TokenUsage))
		tp.AddField(fmt.Sprintf("%d", tr.SpanCount))
		if cols.models {
			tp.AddField(modelList(tr.Models))
		}
		if cols.tools {
			tp.AddField(toolList(tr.Tools, tr.FailedTools))
		}
		if cols.mcpServers {
			tp.AddField(orDash(strings.Join(tr.MCPServers, ", ")))
		}
		tp.EndRow()
	}
	return tp.Render()
}

// runToolCallFails lists traces with a failed tool span, checked client-side
// over one exported page by span attributes; the other filters run server-side.
func runToolCallFails(ctx context.Context, o *ListTracesOptions) error {
	if err := cmdutil.ValidatePathParam("agent name", o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	client, err := o.TraceClient(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	resp, err := client.ExportTraces(ctx, listParams(o))
	if err != nil {
		return render.Error(o.IO, o.Scope, cmdutil.ObserverErrorFromResponse(err))
	}
	var filtered []observersvc.FullTrace
	for _, tr := range resp.Traces {
		if matchesFullCondition(tr) {
			filtered = append(filtered, tr)
		}
	}
	return renderFilteredOverview(o, resp, filtered)
}

// renderFilteredOverview prints the tool_call_fails result. Export carries no
// conversation ID or models, so the table leaves those columns out.
func renderFilteredOverview(o *ListTracesOptions, resp *observersvc.TraceExportResponse, traces []observersvc.FullTrace) error {
	overviews := make([]observersvc.TraceOverview, 0, len(traces))
	for _, tr := range traces {
		overviews = append(overviews, tr.TraceOverview)
	}
	link := consoleLinkFor(o)
	if o.IO.JSON {
		data, err := withConsoleURL(observersvc.TraceOverviewResponse{
			Traces:       overviews,
			TotalCount:   len(overviews),
			LookedBackTo: resp.LookedBackTo,
			Truncated:    resp.Truncated,
		}, link)
		if err != nil {
			return render.Error(o.IO, o.Scope, err)
		}
		return render.JSONSuccess(o.IO, o.Scope, data)
	}
	if len(traces) == 0 {
		fmt.Fprintln(o.IO.Out, "No traces match the condition.")
	} else {
		rows := make([]observersvc.TraceOverview, 0, len(traces))
		for _, tr := range traces {
			row := tr.TraceOverview
			row.Status = rootStatus(tr)
			rows = append(rows, row)
		}
		if err := renderOverviewTable(o, rows, overviewColumns{}); err != nil {
			return err
		}
	}
	printConsoleLink(o.IO, link)
	if resp.Truncated {
		fmt.Fprintln(o.IO.ErrOut, truncatedNotice(o, ""))
	}
	if n := len(resp.FailedTraceIDs); n == 1 {
		fmt.Fprintln(o.IO.ErrOut, "1 trace couldn't be read, so the list may be missing it.")
	} else if n > 1 {
		fmt.Fprintf(o.IO.ErrOut, "%d traces couldn't be read, so the list may be missing them.\n", n)
	}
	return nil
}

// rootStatus is the root span's status, as the server's --status filter reads it.
func rootStatus(tr observersvc.FullTrace) *observersvc.TraceStatus {
	for _, sp := range tr.Spans {
		if sp.SpanID == tr.RootSpanID {
			if sp.AmpAttributes != nil && sp.AmpAttributes.Status != nil && sp.AmpAttributes.Status.Error {
				return &observersvc.TraceStatus{ErrorCount: 1}
			}
			return &observersvc.TraceStatus{}
		}
	}
	return tr.Status
}

var validConditions = []string{
	conditionErrorStatus,
	conditionHighLatency,
	conditionHighTokenUsage,
	conditionToolCallFails,
	conditionExcessiveSteps,
}

// validateCondition checks c is empty or a known --condition.
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

// resolveFilters checks the filter flags and maps --condition onto them.
func resolveFilters(o *ListTracesOptions) error {
	if err := validateCondition(o.Condition); err != nil {
		return err
	}
	if o.Status != "" && o.Status != statusError && o.Status != statusOK {
		return cmdutil.FlagErrorf("--status: %q is not valid; must be error or ok", o.Status)
	}
	for _, m := range []struct {
		flag     string
		negative bool
	}{
		{"--min-duration", o.MinDurationMs != nil && *o.MinDurationMs < 0},
		{"--min-tokens", o.MinTokens != nil && *o.MinTokens < 0},
		{"--min-spans", o.MinSpans != nil && *o.MinSpans < 0},
		{"--max-latency", o.MaxLatency < 0},
		{"--max-tokens", o.MaxTokens < 0},
		{"--max-spans", o.MaxSpans < 0},
	} {
		if m.negative {
			return cmdutil.FlagErrorf("%s must not be negative", m.flag)
		}
	}
	if o.NoConversation && o.ConversationID != "" {
		return cmdutil.FlagErrorf("--no-conversation can't be used with --conversation")
	}
	if o.ShowModels && o.Condition == conditionToolCallFails {
		return cmdutil.FlagErrorf("--show-models can't be used with --condition %s", conditionToolCallFails)
	}
	if err := checkToolFilters(o); err != nil {
		return err
	}
	return applyCondition(o)
}

// checkToolFilters rejects the flags --condition tool_call_fails can't take.
func checkToolFilters(o *ListTracesOptions) error {
	if o.Condition != conditionToolCallFails {
		return nil
	}
	for _, f := range []struct {
		flag string
		set  bool
	}{
		{"--tool-error", o.ToolError},
		{"--show-tools", o.ShowTools},
		{"--show-mcp-servers", o.ShowMCPServers},
		{"--cursor", o.Cursor != ""},
	} {
		if f.set {
			return cmdutil.FlagErrorf("%s can't be used with --condition %s", f.flag, conditionToolCallFails)
		}
	}
	return nil
}

// applyCondition turns a --condition shorthand into the filter it stands for.
// The thresholds were checked with >, so each minimum is one above its threshold.
func applyCondition(o *ListTracesOptions) error {
	conflict := func(flag string) error {
		return cmdutil.FlagErrorf("--condition %s and %s set the same filter; use one", o.Condition, flag)
	}
	switch o.Condition {
	case conditionErrorStatus:
		if o.Status != "" {
			return conflict("--status")
		}
		o.Status = statusError
	case conditionHighLatency:
		if o.MinDurationMs != nil {
			return conflict("--min-duration")
		}
		o.MinDurationMs = int64Ptr(int64(o.MaxLatency) + 1)
	case conditionHighTokenUsage:
		if o.MinTokens != nil {
			return conflict("--min-tokens")
		}
		o.MinTokens = int64Ptr(int64(o.MaxTokens) + 1)
	case conditionExcessiveSteps:
		if o.MinSpans != nil {
			return conflict("--min-spans")
		}
		o.MinSpans = int64Ptr(int64(o.MaxSpans) + 1)
	}
	return nil
}

// int64Ptr returns a pointer to v.
func int64Ptr(v int64) *int64 { return &v }

// matchesFullCondition reports whether tr has a failed tool span.
func matchesFullCondition(tr observersvc.FullTrace) bool {
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

// traceStatus is error when the trace has errors, else ok.
func traceStatus(status *observersvc.TraceStatus) string {
	if status != nil && status.ErrorCount > 0 {
		return "error"
	}
	return "ok"
}

// tokenCount is the trace's total tokens, or 0 when unset.
func tokenCount(usage *observersvc.TokenUsage) string {
	if usage == nil {
		return "0"
	}
	return fmt.Sprintf("%d", usage.TotalTokens)
}

// orDash returns s, or "-" when s is empty.
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// toolList shows every tool, marking the ones that failed.
func toolList(tools, failed []string) string {
	if len(tools) == 0 {
		return "-"
	}
	failedSet := make(map[string]bool, len(failed))
	for _, t := range failed {
		failedSet[t] = true
	}
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t
		if failedSet[t] {
			names[i] += " (failed)"
		}
	}
	return strings.Join(names, ", ")
}

// modelList shows the first model and how many more there are.
func modelList(models []string) string {
	switch len(models) {
	case 0:
		return "-"
	case 1:
		return truncate(models[0], 24)
	default:
		return fmt.Sprintf("%s +%d", truncate(models[0], 24), len(models)-1)
	}
}
