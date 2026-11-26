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
	ordered := make([]SpanDetails, len(spans))
	copy(ordered, spans)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].StartTime.Before(ordered[j].StartTime) })
	attrs := make([]Attrs, len(ordered))
	for i, sp := range ordered {
		attrs[i] = StringAttrs(sp.Attributes)
	}
	root := rootIndex(ordered)

	switch f.Status {
	case "error", "ok":
		failed := root >= 0 && attrs[root].IsError(statusCode(ordered[root]))
		if failed != (f.Status == "error") {
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
		if n := tokens(ordered, attrs, root); n <= 0 || n < *f.MinTokens {
			return false
		}
	}
	if f.Model != "" && !anySpan(ordered, func(i int) bool {
		return IsLLMLeaf(ordered[i].SpanName) && containsFold(attrs[i].Model(), f.Model)
	}) {
		return false
	}
	if f.ConversationID != "" || f.MissingConversationID {
		id := conversationID(ordered, attrs, root)
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

// tokens sums the LLM leaves that report usage, leaving out a leaf whose
// parent leaf reports it too; with no such leaf, it is the root's usage.
func tokens(spans []SpanDetails, attrs []Attrs, root int) int64 {
	reporting := map[string]bool{}
	for i, sp := range spans {
		if IsLLMLeaf(sp.SpanName) && attrs[i].Tokens() > 0 {
			reporting[sp.SpanID] = true
		}
	}
	if len(reporting) == 0 {
		if root < 0 {
			return 0
		}
		return attrs[root].Tokens()
	}
	var sum int64
	for i, sp := range spans {
		if reporting[sp.SpanID] && !reporting[sp.ParentSpanID] {
			sum += attrs[i].Tokens()
		}
	}
	return sum
}

// conversationID is the root's conversation ID, else the earliest non-LLM
// child of the root's, else the first one on an LLM leaf.
func conversationID(spans []SpanDetails, attrs []Attrs, root int) string {
	if root < 0 {
		return ""
	}
	if id := attrs[root].ConversationID(); id != "" {
		return id
	}
	for i, sp := range spans {
		if sp.ParentSpanID != "" && sp.ParentSpanID == spans[root].SpanID && !IsLLMLeaf(sp.SpanName) {
			if id := attrs[i].ConversationID(); id != "" {
				return id
			}
			break
		}
	}
	for i, sp := range spans {
		if IsLLMLeaf(sp.SpanName) {
			if id := attrs[i].ConversationID(); id != "" {
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
