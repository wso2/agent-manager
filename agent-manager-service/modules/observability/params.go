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

package observability

import (
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/tracesv1"
)

// maxFilterValueLen caps the model, conversationId, tool and mcpServer filters, in characters.
const maxFilterValueLen = 256

// conversationKey is the conversation ID's name in the query and in missing.
const conversationKey = "conversationId"

// parseTraceFilters reads a trace list's filters, with the observer's rules and messages.
func parseTraceFilters(query url.Values) (tracesv1.TraceFilters, error) {
	var f tracesv1.TraceFilters
	switch f.Status = query.Get("status"); f.Status {
	case "", "error", "ok":
	default:
		return tracesv1.TraceFilters{}, badRequest("status must be 'error' or 'ok'")
	}
	for _, p := range []struct {
		name string
		dst  **int64
	}{
		{"minDurationMs", &f.MinDurationMs},
		{"minTokens", &f.MinTokens},
		{"minSpanCount", &f.MinSpanCount},
	} {
		if err := parseMinThreshold(p.name, query.Get(p.name), p.dst); err != nil {
			return tracesv1.TraceFilters{}, err
		}
	}
	for _, p := range []struct {
		name string
		dst  *string
	}{
		{"model", &f.Model},
		{conversationKey, &f.ConversationID},
		{"tool", &f.Tool},
	} {
		if *p.dst = query.Get(p.name); utf8.RuneCountInString(*p.dst) > maxFilterValueLen {
			return tracesv1.TraceFilters{}, badRequest("%s must be at most %d characters", p.name, maxFilterValueLen)
		}
	}
	switch query.Get("toolError") {
	case "", "false":
	case "true":
		f.ToolError = true
	default:
		return tracesv1.TraceFilters{}, badRequest("toolError must be 'true' or 'false'")
	}
	if f.MCPServer = query.Get("mcpServer"); utf8.RuneCountInString(f.MCPServer) > maxFilterValueLen {
		return tracesv1.TraceFilters{}, badRequest("mcpServer must be at most %d characters", maxFilterValueLen)
	}
	for _, list := range query["missing"] {
		for _, item := range strings.Split(list, ",") {
			switch item = strings.TrimSpace(item); item {
			case "":
			case conversationKey:
				f.MissingConversationID = true
			default:
				return tracesv1.TraceFilters{}, badRequest("invalid missing value %q: must be '%s'", item, conversationKey)
			}
		}
	}
	if f.MissingConversationID && f.ConversationID != "" {
		return tracesv1.TraceFilters{}, badRequest("%s and missing=%s can't be combined", conversationKey, conversationKey)
	}
	return f, nil
}

// parseMinThreshold sets dst to an optional non-negative integer filter.
func parseMinThreshold(name, s string, dst **int64) error {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v < 0 {
		return badRequest("%s must be a non-negative integer", name)
	}
	*dst = &v
	return nil
}
