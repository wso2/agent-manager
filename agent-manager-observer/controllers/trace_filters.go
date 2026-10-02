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
	"log/slog"
	"slices"
	"strings"
	"time"

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

// TraceFilters holds trace-list filters; set fields combine with AND.
// Min* are pointers so an explicit 0 still filters.
type TraceFilters struct {
	Status         TraceStatusFilter
	MinDurationMs  *int64
	MinTokens      *int64
	MinSpanCount   *int64
	Model          string
	ConversationID string
}

// IsZero reports whether no filter is set.
func (f TraceFilters) IsZero() bool {
	return f == TraceFilters{}
}

// SummaryOnly reports whether only filters the trace list answers are set.
func (f TraceFilters) SummaryOnly() bool {
	return !f.IsZero() && f == TraceFilters{MinDurationMs: f.MinDurationMs, MinSpanCount: f.MinSpanCount}
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
	return slog.GroupValue(attrs...)
}

// matchesFilters reports whether overview satisfies f.
func matchesFilters(overview opensearch.TraceOverview, f TraceFilters) bool {
	if !matchesRootFilters(overview.Status, overview.ConversationID, f) {
		return false
	}
	if !matchesSummary(overview.DurationInNanos, overview.SpanCount, f) {
		return false
	}
	if f.MinTokens != nil && (overview.TokenUsage == nil || int64(overview.TokenUsage.TotalTokens) < *f.MinTokens) {
		return false
	}
	return matchesModel(overview.Models, f)
}

// matchesModel checks the model filter; a trace with no models fails it.
func matchesModel(models []string, f TraceFilters) bool {
	return f.Model == "" || slices.ContainsFunc(models, containsFold(f.Model))
}

// matchesRootFilters checks the filters the root span alone can answer.
func matchesRootFilters(status *opensearch.TraceStatus, conversationID string, f TraceFilters) bool {
	hasErrors := status != nil && status.ErrorCount > 0
	switch f.Status {
	case TraceStatusError:
		if !hasErrors {
			return false
		}
	case TraceStatusOK:
		if hasErrors {
			return false
		}
	}
	if f.ConversationID != "" && conversationID != f.ConversationID {
		return false
	}
	return true
}

// containsFold reports whether a model name contains sub, ignoring case.
func containsFold(sub string) func(string) bool {
	sub = strings.ToLower(sub)
	return func(model string) bool { return strings.Contains(strings.ToLower(model), sub) }
}

// matchesSummary checks the filters the trace list alone can answer.
func matchesSummary(durationNs int64, spanCount int, f TraceFilters) bool {
	// Compare in ms to avoid overflow.
	if f.MinDurationMs != nil && durationNs/int64(time.Millisecond) < *f.MinDurationMs {
		return false
	}
	if f.MinSpanCount != nil && int64(spanCount) < *f.MinSpanCount {
		return false
	}
	return true
}

// summaryChunkLen is the shortest prefix of traces holding need survivors past the cursor time.
func summaryChunkLen(traces []observer.TraceInfo, f TraceFilters, cur *TraceCursor, need int) int {
	for i, t := range traces {
		if matchesSummary(t.DurationNs, t.SpanCount, f) && !atCursor(t.StartTime, cur) {
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
