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

package clickhouse

import (
	"strconv"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store"
)

// The SQL below is store.MatchTrace over the span table, built from the
// names and keys in package store. Only those constants are written into
// the SQL; every filter value is a bound parameter.

// lit is a constant as a SQL string literal.
func lit(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

func litList(ss []string) string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = lit(s)
	}
	return "(" + strings.Join(out, ", ") + ")"
}

func attr(key string) string {
	return "SpanAttributes[" + lit(key) + "]"
}

// firstPresent is the value of the first key present, or ”.
func firstPresent(keys []string) string {
	args := make([]string, 0, 2*len(keys)+1)
	for _, k := range keys {
		args = append(args, "mapContains(SpanAttributes, "+lit(k)+")", attr(k))
	}
	return "multiIf(" + strings.Join(append(args, "''"), ", ") + ")"
}

// firstNonEmpty is the first non-empty value of keys, or ”.
func firstNonEmpty(keys []string) string {
	args := make([]string, 0, 2*len(keys)+1)
	for _, k := range keys {
		args = append(args, attr(k)+" != ''", attr(k))
	}
	return "multiIf(" + strings.Join(append(args, "''"), ", ") + ")"
}

const lowerName = "lowerUTF8(trimBoth(SpanName))"

var (
	isLLMLeaf = "(endsWith(" + lowerName + ", '.chat') OR " + lowerName + " = 'chat' OR startsWith(" + lowerName + ", 'chat '))"

	spanError = "(mapContains(SpanAttributes, " + lit(store.AttrErrorType) + ")" +
		" OR lowerUTF8(" + attr(store.AttrToolStatus) + ") IN " + litList(store.ErrorStatuses) +
		" OR toFloat64OrZero(" + attr(store.AttrHTTPStatus) + ") >= 400" +
		" OR lowerUTF8(StatusCode) IN " + litList(store.ErrorStatuses) + ")"

	spanTokens = tokenSum(firstPresent(store.InputTokenKeys), firstPresent(store.OutputTokenKeys))

	spanModel = firstPresent(store.ModelKeys)

	spanConversation = firstNonEmpty(store.ConversationIDKeys)

	spanTool = toolName()

	spanMCPServer = mcpServer()
)

// tokenSum is input plus output tokens, read as the integer each value starts with; 0 when neither is positive.
func tokenSum(in, out string) string {
	n := func(v string) string {
		return "toInt64OrZero(extract(trimLeft(" + v + "), " + lit(store.LeadingIntPattern) + "))"
	}
	return "if(" + n(in) + " > 0 OR " + n(out) + " > 0, " + n(in) + " + " + n(out) + ", 0)"
}

// toolName is the tool a span's name calls, or ” (store.ToolName).
func toolName() string {
	prefix := lit(store.ToolPrefix)
	nonTool := make([]string, len(store.NonToolPrefixes))
	for i, p := range store.NonToolPrefixes {
		nonTool[i] = "startsWith(" + lowerName + ", " + lit(p) + ")"
	}
	ops := append(append([]string{}, store.OperationLLM...), store.OperationEmbedding...)
	isTool := "(startsWith(" + lowerName + ", " + prefix + ") OR (NOT (" + strings.Join(nonTool, " OR ") + ")" +
		" AND NOT (position(" + lowerName + ", ' ') > 1 AND splitByChar(' ', " + lowerName + ")[1] IN " + litList(ops) + ")" +
		" AND arrayElement(splitByChar('.', " + lowerName + "), -1) IN " + litList(store.ToolSegments) + "))"
	trimmed := "trimBoth(SpanName)"
	after := strconv.Itoa(len(store.ToolPrefix) + 1)
	name := "if(startsWith(" + lowerName + ", " + prefix + ")," +
		" if(substring(" + trimmed + ", " + after + ", 1) = ' ', trimBoth(substring(" + trimmed + ", " + after + ")), '')," +
		" if(position(" + trimmed + ", '.') > 0," +
		" trimBoth(substring(" + trimmed + ", 1, length(" + trimmed + ") - position(reverse(" + trimmed + "), '.'))), ''))"
	return "if(" + isTool + ", " + name + ", '')"
}

// mcpServer is the server an MCP handshake span names, or ”.
func mcpServer() string {
	out := attr(store.AttrEntityOutput)
	name := "trimBoth(JSONExtractString(" + out + ", 'serverInfo', 'name'))"
	title := "trimBoth(JSONExtractString(" + out + ", 'serverInfo', 'title'))"
	return "if(" + lowerName + " = " + lit(strings.ToLower(store.MCPHandshakeName)) + ", if(" + name + " != '', " + name + ", " + title + "), '')"
}

// ofRoot is expr on the trace's root span: the earliest with no parent, else the earliest.
func ofRoot(expr string) string {
	return "if(has_root, argMinIf(" + expr + ", Timestamp, ParentSpanId = ''), argMin(" + expr + ", Timestamp))"
}

func containsParam(expr, param string) string {
	return "positionCaseInsensitiveUTF8(" + expr + ", {" + param + ":String}) > 0"
}

// traceFilters is the extra columns and HAVING conditions for f; params gets their values.
func traceFilters(f store.TraceFilters, params map[string]string) (columns, having []string) {
	switch f.Status {
	case "error":
		columns = append(columns, ofRoot(spanError)+" AS root_error")
		having = append(having, "root_error")
	case "ok":
		columns = append(columns, ofRoot(spanError)+" AS root_error")
		having = append(having, "NOT root_error")
	}
	if f.MinDurationMs != nil {
		params["min_duration_ms"] = strconv.FormatInt(*f.MinDurationMs, 10)
		having = append(having, "intDiv(end_ns - start_ns, 1000000) >= {min_duration_ms:Int64}")
	}
	if f.MinSpanCount != nil {
		params["min_span_count"] = strconv.FormatInt(*f.MinSpanCount, 10)
		having = append(having, "span_count >= {min_span_count:Int64}")
	}
	if f.MinTokens != nil {
		params["min_tokens"] = strconv.FormatInt(*f.MinTokens, 10)
		columns = append(columns,
			"groupArrayIf((SpanId, ParentSpanId, "+spanTokens+"), "+isLLMLeaf+" AND "+spanTokens+" > 0) AS token_leaves",
			"arrayMap(x -> x.1, token_leaves) AS token_leaf_ids",
			"if(length(token_leaves) > 0,"+
				" arraySum(arrayMap(x -> x.3, arrayFilter(x -> NOT has(token_leaf_ids, x.2), token_leaves))),"+
				" "+ofRoot(spanTokens)+") AS tokens")
		having = append(having, "tokens > 0 AND tokens >= {min_tokens:Int64}")
	}
	if f.Model != "" {
		params["model"] = f.Model
		having = append(having, "countIf("+isLLMLeaf+" AND "+containsParam(spanModel, "model")+") > 0")
	}
	if f.ConversationID != "" || f.MissingConversationID {
		columns = append(columns,
			ofRoot(spanConversation)+" AS root_conversation",
			"tupleElement(arrayFirst(x -> x.2 = root_span_id, arraySort(x -> x.1,"+
				" groupArrayIf((toUnixTimestamp64Nano(Timestamp), ParentSpanId, "+spanConversation+"),"+
				" ParentSpanId != '' AND NOT "+isLLMLeaf+"))), 3) AS child_conversation",
			"arrayFirst(x -> x != '', arrayMap(x -> x.2, arraySort(x -> x.1,"+
				" groupArrayIf((toUnixTimestamp64Nano(Timestamp), "+spanConversation+"), "+isLLMLeaf+")))) AS leaf_conversation",
			"multiIf(root_conversation != '', root_conversation, child_conversation != '', child_conversation, leaf_conversation) AS conversation")
		if f.MissingConversationID {
			having = append(having, "conversation = ''")
		} else {
			params["conversation"] = f.ConversationID
			having = append(having, "conversation = {conversation:String}")
		}
	}
	if f.Tool != "" || f.ToolError {
		cond := spanTool + " != ''"
		if f.ToolError {
			cond += " AND lowerUTF8(StatusCode) = 'error'"
		}
		if f.Tool != "" {
			params["tool"] = f.Tool
			cond += " AND " + containsParam(spanTool, "tool")
		}
		having = append(having, "countIf("+cond+") > 0")
	}
	if f.MCPServer != "" {
		params["mcp_server"] = f.MCPServer
		having = append(having, "countIf("+containsParam(spanMCPServer, "mcp_server")+") > 0")
	}
	return columns, having
}
