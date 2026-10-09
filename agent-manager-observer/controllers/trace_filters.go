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

package controllers

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/opensearch"
)

// TraceStatusFilter selects traces by error status.
type TraceStatusFilter string

const (
	TraceStatusAny   TraceStatusFilter = ""
	TraceStatusError TraceStatusFilter = "error"
	TraceStatusOK    TraceStatusFilter = "ok"
)

// MaxFilterValueLen caps the model, conversationId, tool and mcpServer filters, in characters.
const MaxFilterValueLen = 256

// TraceFilters holds trace-list filters; set fields combine with AND.
// Min* are pointers so an explicit 0 still filters.
type TraceFilters struct {
	Status         TraceStatusFilter
	MinDurationMs  *int64
	MinTokens      *int64
	MinSpanCount   *int64
	Model          string
	ConversationID string
	// Tool matches a tool name; with ToolError, a failed tool's name.
	Tool string
	// ToolError keeps traces with a failed tool.
	ToolError bool
	// MCPServer matches an MCP server name.
	MCPServer string
	// MinScore and MaxScore bound the trace's mean evaluation score, inclusive.
	MinScore *float64
	MaxScore *float64
	// Evaluator scores the trace on one evaluator's rows only.
	Evaluator string
}

// ParseTraceStatus accepts an empty status, "error" or "ok".
func ParseTraceStatus(s string) (TraceStatusFilter, error) {
	switch status := TraceStatusFilter(s); status {
	case TraceStatusAny, TraceStatusError, TraceStatusOK:
		return status, nil
	default:
		return TraceStatusAny, fmt.Errorf("status must be 'error' or 'ok'")
	}
}

// CheckMinThreshold rejects a negative threshold filter.
func CheckMinThreshold(name string, v int64) error {
	if v < 0 {
		return fmt.Errorf("%s must be a non-negative integer", name)
	}
	return nil
}

// CheckFilterValue rejects a string filter longer than MaxFilterValueLen characters.
func CheckFilterValue(name, s string) error {
	if utf8.RuneCountInString(s) > MaxFilterValueLen {
		return fmt.Errorf("%s must be at most %d characters", name, MaxFilterValueLen)
	}
	return nil
}

// IsZero reports whether no filter is set.
func (f TraceFilters) IsZero() bool {
	return f == TraceFilters{}
}

// SummaryOnly reports whether only filters the trace list answers are set.
func (f TraceFilters) SummaryOnly() bool {
	return !f.IsZero() && f == TraceFilters{MinDurationMs: f.MinDurationMs, MinSpanCount: f.MinSpanCount}
}

// HasScoreFilter reports whether a score filter is set.
func (f TraceFilters) HasScoreFilter() bool {
	return f.MinScore != nil || f.MaxScore != nil || f.Evaluator != ""
}

// hasToolFilter reports whether a tool filter is set.
func (f TraceFilters) hasToolFilter() bool {
	return f.Tool != "" || f.ToolError
}

// hasSpanListFilter reports whether a filter judged on the span list's names is set.
func (f TraceFilters) hasSpanListFilter() bool {
	return f.hasToolFilter() || f.MCPServer != ""
}

// impliedInclude adds the includes f's model, tool and MCP server filters need.
func impliedInclude(include Include, f TraceFilters) Include {
	include.Models = include.Models || f.Model != ""
	include.Tools = include.Tools || f.hasToolFilter()
	include.MCPServers = include.MCPServers || f.MCPServer != ""
	return include
}

// LogValue logs only the set filters.
func (f TraceFilters) LogValue() slog.Value {
	var attrs []slog.Attr
	if f.Status != TraceStatusAny {
		attrs = append(attrs, slog.String("status", string(f.Status)))
	}
	for _, m := range []struct {
		key string
		val *int64
	}{{"minDurationMs", f.MinDurationMs}, {"minTokens", f.MinTokens}, {"minSpanCount", f.MinSpanCount}} {
		if m.val != nil {
			attrs = append(attrs, slog.Int64(m.key, *m.val))
		}
	}
	if f.Model != "" {
		attrs = append(attrs, slog.String("model", f.Model))
	}
	if f.ConversationID != "" {
		attrs = append(attrs, slog.String("conversationId", f.ConversationID))
	}
	if f.Tool != "" {
		attrs = append(attrs, slog.String("tool", f.Tool))
	}
	if f.ToolError {
		attrs = append(attrs, slog.Bool("toolError", true))
	}
	if f.MCPServer != "" {
		attrs = append(attrs, slog.String("mcpServer", f.MCPServer))
	}
	if f.MinScore != nil {
		attrs = append(attrs, slog.Float64("minScore", *f.MinScore))
	}
	if f.MaxScore != nil {
		attrs = append(attrs, slog.Float64("maxScore", *f.MaxScore))
	}
	if f.Evaluator != "" {
		attrs = append(attrs, slog.String("evaluator", f.Evaluator))
	}
	return slog.GroupValue(attrs...)
}

// matchesFilters reports whether overview satisfies f.
func matchesFilters(overview opensearch.TraceOverview, f TraceFilters) bool {
	if !matchesStatus(overview.Status, f) || !matchesConversation(overview.ConversationID, f) {
		return false
	}
	if !matchesSummary(overview.DurationInNanos, overview.SpanCount, f) {
		return false
	}
	if !matchesMinTokens(overview.TokenUsage, f) {
		return false
	}
	return matchesTools(overview.Tools, overview.FailedTools, f) && matchesModel(overview.Models, f) &&
		matchesMCPServer(overview.MCPServers, f)
}

// matchesMCPServer checks the mcpServer filter; a trace with no MCP servers fails it.
func matchesMCPServer(servers []string, f TraceFilters) bool {
	return f.MCPServer == "" || slices.ContainsFunc(servers, containsFold(f.MCPServer))
}

// matchesScore checks the minScore and maxScore filters; a trace with no score fails them.
func matchesScore(score *float64, f TraceFilters) bool {
	if !f.HasScoreFilter() {
		return true
	}
	return score != nil && (f.MinScore == nil || *score >= *f.MinScore) && (f.MaxScore == nil || *score <= *f.MaxScore)
}

// matchesMinTokens checks the minTokens filter; a trace with no token usage fails it.
func matchesMinTokens(tokens *opensearch.TokenUsage, f TraceFilters) bool {
	return f.MinTokens == nil || (tokens != nil && int64(tokens.TotalTokens) >= *f.MinTokens)
}

// matchesModel checks the model filter; a trace with no models fails it.
func matchesModel(models []string, f TraceFilters) bool {
	return f.Model == "" || slices.ContainsFunc(models, containsFold(f.Model))
}

// matchesTools checks the tool and toolError filters; a trace with no tools fails them.
// With both set, a failed tool must match the tool filter.
func matchesTools(tools, failedTools []string, f TraceFilters) bool {
	switch {
	case f.ToolError && f.Tool != "":
		return slices.ContainsFunc(failedTools, containsFold(f.Tool))
	case f.ToolError:
		return len(failedTools) > 0
	case f.Tool != "":
		return slices.ContainsFunc(tools, containsFold(f.Tool))
	}
	return true
}

// matchesStatus checks the status filter, which the root span alone answers.
func matchesStatus(status *opensearch.TraceStatus, f TraceFilters) bool {
	hasErrors := status != nil && status.ErrorCount > 0
	switch f.Status {
	case TraceStatusError:
		return hasErrors
	case TraceStatusOK:
		return !hasErrors
	}
	return true
}

// matchesConversation checks the conversationId filter; a trace with no conversation ID fails it.
func matchesConversation(conversationID string, f TraceFilters) bool {
	return f.ConversationID == "" || conversationID == f.ConversationID
}

// conversationRulesOut reports whether a conversation ID found so far fails
// the filter. "" doesn't, since a later span may carry one.
func conversationRulesOut(conversationID string, f TraceFilters) bool {
	return conversationID != "" && !matchesConversation(conversationID, f)
}

// conversationPending reports whether a conversationId filter still lacks the trace's ID.
func conversationPending(conversationID string, f TraceFilters) bool {
	return f.ConversationID != "" && conversationID == ""
}

// containsFold reports whether a model, tool or server name contains sub, ignoring case.
func containsFold(sub string) func(string) bool {
	sub = strings.ToLower(sub)
	return func(model string) bool { return strings.Contains(strings.ToLower(model), sub) }
}

// matchesSummary checks the filters the trace list alone can answer. A trace
// over maxToolListSpans has no tools or MCP servers, so it fails their filters.
func matchesSummary(durationNs int64, spanCount int, f TraceFilters) bool {
	// Compare in ms to avoid overflow.
	if f.MinDurationMs != nil && durationNs/int64(time.Millisecond) < *f.MinDurationMs {
		return false
	}
	if f.MinSpanCount != nil && int64(spanCount) < *f.MinSpanCount {
		return false
	}
	if f.hasSpanListFilter() && spanCount > maxToolListSpans {
		return false
	}
	return true
}

// summaryChunkLen is the shortest prefix of traces holding need survivors.
func summaryChunkLen(traces []observer.TraceInfo, f TraceFilters, need int) int {
	for i, t := range traces {
		if matchesSummary(t.DurationNs, t.SpanCount, f) {
			need--
			if need == 0 {
				return i + 1
			}
		}
	}
	return len(traces)
}

// filterTraceInfos drops traces the summary already rules out, before enrichment.
func filterTraceInfos(traces []observer.TraceInfo, f TraceFilters) []observer.TraceInfo {
	matched := make([]observer.TraceInfo, 0, len(traces))
	for _, t := range traces {
		if matchesSummary(t.DurationNs, t.SpanCount, f) {
			matched = append(matched, t)
		}
	}
	return matched
}
