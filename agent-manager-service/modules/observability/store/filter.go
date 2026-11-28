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

package store

import (
	"sort"
	"strings"
)

// MatchTrace reports whether trace t, with spans, passes every filter in f.
func MatchTrace(f TraceFilters, t Trace, spans []SpanDetails) bool {
	if f == (TraceFilters{}) {
		return true
	}
	ts := newTraceSpans(spans)
	ordered, attrs := ts.spans, ts.attrs

	switch f.Status {
	case "error", "ok":
		if ts.rootError() != (f.Status == "error") {
			return false
		}
	}
	if f.MinDurationMs != nil && t.DurationNs/1e6 < *f.MinDurationMs {
		return false
	}
	if f.MinSpanCount != nil && int64(t.SpanCount) < *f.MinSpanCount {
		return false
	}
	if f.MinTokens != nil {
		in, out := ts.tokens()
		if n := in + out; n <= 0 || n < *f.MinTokens {
			return false
		}
	}
	if f.Model != "" && !anySpan(ordered, func(i int) bool {
		return IsLLMLeaf(ordered[i].SpanName) && containsFold(attrs[i].Model(), f.Model)
	}) {
		return false
	}
	if f.ConversationID != "" || f.MissingConversationID {
		id := ts.conversationID()
		if f.MissingConversationID && id != "" || f.ConversationID != "" && id != f.ConversationID {
			return false
		}
	}
	if (f.Tool != "" || f.ToolError) && !anySpan(ordered, func(i int) bool {
		name := ToolName(ordered[i].SpanName)
		return name != "" && (!f.ToolError || IsErrorCode(statusCode(ordered[i]))) &&
			(f.Tool == "" || containsFold(name, f.Tool))
	}) {
		return false
	}
	if f.MCPServer != "" && !anySpan(ordered, func(i int) bool {
		return containsFold(attrs[i].MCPServer(ordered[i].SpanName), f.MCPServer)
	}) {
		return false
	}
	return true
}

// Overview is t with the status, tokens, conversation, input and output its spans give.
func Overview(t Trace, spans []SpanDetails) Trace {
	ts := newTraceSpans(spans)
	if ts.root < 0 {
		return t
	}
	t.Status = &TraceStatus{}
	if ts.rootError() {
		t.Status.ErrorCount = 1
	}
	if in, out := ts.tokens(); in+out > 0 {
		t.Tokens = &TokenUsage{InputTokens: in, OutputTokens: out, TotalTokens: in + out}
	}
	t.ConversationID = ts.conversationID()
	var leaves []Attrs
	for i, sp := range ts.spans {
		if IsLLMLeaf(sp.SpanName) {
			leaves = append(leaves, ts.attrs[i])
		}
	}
	t.Input, t.Output = TraceIO(ts.attrs[ts.root], ts.at(ts.chainChild()), leaves, t.SpanCount)
	return t
}

// traceSpans are a trace's spans in start order, with their attributes and root.
type traceSpans struct {
	spans []SpanDetails
	attrs []Attrs
	root  int
}

func newTraceSpans(spans []SpanDetails) traceSpans {
	ordered := make([]SpanDetails, len(spans))
	copy(ordered, spans)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].StartTime.Before(ordered[j].StartTime) })
	attrs := make([]Attrs, len(ordered))
	for i, sp := range ordered {
		attrs[i] = StringAttrs(sp.Attributes)
	}
	return traceSpans{spans: ordered, attrs: attrs, root: rootIndex(ordered)}
}

// at is span i's attributes, or nil for -1.
func (ts traceSpans) at(i int) Attrs {
	if i < 0 {
		return nil
	}
	return ts.attrs[i]
}

// rootIndex is the earliest span with no parent, else the earliest span; -1 for none.
func rootIndex(spans []SpanDetails) int {
	for i, sp := range spans {
		if sp.ParentSpanID == "" {
			return i
		}
	}
	if len(spans) == 0 {
		return -1
	}
	return 0
}

// rootError reports whether the root span failed.
func (ts traceSpans) rootError() bool {
	return ts.root >= 0 && ts.attrs[ts.root].IsError(statusCode(ts.spans[ts.root]))
}

// tokens sums the LLM leaves that report usage, leaving out a leaf whose
// parent leaf reports it too; with no such leaf, it is the root's usage.
func (ts traceSpans) tokens() (in, out int64) {
	reporting := map[string]bool{}
	for i, sp := range ts.spans {
		if IsLLMLeaf(sp.SpanName) && ts.attrs[i].Tokens() > 0 {
			reporting[sp.SpanID] = true
		}
	}
	if len(reporting) == 0 {
		if ts.root < 0 {
			return 0, 0
		}
		return ts.attrs[ts.root].Usage()
	}
	for i, sp := range ts.spans {
		if reporting[sp.SpanID] && !reporting[sp.ParentSpanID] {
			a, b := ts.attrs[i].Usage()
			in, out = in+a, out+b
		}
	}
	return in, out
}

// chainChild is the earliest child of the root that is not an LLM leaf; -1 for none.
func (ts traceSpans) chainChild() int {
	if ts.root < 0 {
		return -1
	}
	for i, sp := range ts.spans {
		if sp.ParentSpanID != "" && sp.ParentSpanID == ts.spans[ts.root].SpanID && !IsLLMLeaf(sp.SpanName) {
			return i
		}
	}
	return -1
}

// conversationID is the root's conversation ID, else its chain child's, else
// the first one on an LLM leaf.
func (ts traceSpans) conversationID() string {
	if ts.root < 0 {
		return ""
	}
	if id := ts.attrs[ts.root].ConversationID(); id != "" {
		return id
	}
	if id := ts.at(ts.chainChild()).ConversationID(); id != "" {
		return id
	}
	for i, sp := range ts.spans {
		if IsLLMLeaf(sp.SpanName) {
			if id := ts.attrs[i].ConversationID(); id != "" {
				return id
			}
		}
	}
	return ""
}

func anySpan(spans []SpanDetails, match func(i int) bool) bool {
	for i := range spans {
		if match(i) {
			return true
		}
	}
	return false
}

func statusCode(sp SpanDetails) string {
	if sp.Status == nil {
		return ""
	}
	return sp.Status.Code
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
