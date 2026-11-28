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
	"encoding/json"
	"slices"
	"strings"
)

// The span attributes a trace's input and output are read from, as the
// observer's trace list reads them.
const (
	AttrEntityInput      = "traceloop.entity.input"
	AttrInputMessages    = "gen_ai.input.messages"
	AttrOutputMessages   = "gen_ai.output.messages"
	AttrCrewTasksOutput  = "crewai.crew.tasks_output"
	AttrCrewResult       = "crewai.crew.result"
	AttrGenAISystem      = "gen_ai.system"
	AttrGenAIProvider    = "gen_ai.provider.name"
	CrewAIPrefix         = "crewai."
	crewAIVendor         = "crewai"
	maxMessageJSONDepth  = 10
	messagePartText      = "text"
	messagePartToolReply = "tool_call_response"
)

// MaxIOLeaves and MaxIOSpanCount are the observer's caps: it reads the first
// 50 LLM leaves, and no leaf of a trace with more than 100 spans.
const (
	MaxIOLeaves    = 50
	MaxIOSpanCount = 100
)

// IOKeys are the attributes TraceIO reads besides CrewAIPrefix's.
var IOKeys = []string{
	AttrEntityInput, AttrEntityOutput, AttrInputMessages, AttrOutputMessages,
	AttrCrewTasksOutput, AttrCrewResult, AttrGenAISystem, AttrGenAIProvider,
}

// TraceIO is a trace's input and output: the root's, then its chain child's,
// then the first LLM leaf's user message and the last leaf's answer. leaves
// are the trace's LLM leaves in start order.
func TraceIO(root, child Attrs, leaves []Attrs, spanCount int) (input, output string) {
	var in, out *string
	if root.isCrewAI() {
		in, out = root.str(AttrCrewTasksOutput), root.str(AttrCrewResult)
	} else {
		in, out = root.entityIO()
	}
	if in == nil || out == nil {
		ci, co := child.entityIO()
		in, out = cmpOr(in, ci), cmpOr(out, co)
	}
	if (in == nil || out == nil) && spanCount <= MaxIOSpanCount && len(leaves) > 0 {
		leaves = leaves[:min(len(leaves), MaxIOLeaves)]
		for _, leaf := range leaves {
			if in != nil {
				break
			}
			in = leaf.messagePreview(AttrInputMessages, "user", false)
		}
		out = cmpOr(out, leaves[len(leaves)-1].messagePreview(AttrOutputMessages, "assistant", true))
	}
	return deref(in), deref(out)
}

func cmpOr(a, b *string) *string {
	if a != nil {
		return a
	}
	return b
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// str is the value at key, or nil when it is absent.
func (a Attrs) str(key string) *string {
	if v, ok := a[key]; ok {
		return &v
	}
	return nil
}

// isCrewAI reports whether a span is CrewAI's, by its vendor or a crewai.* attribute.
func (a Attrs) isCrewAI() bool {
	vendor := a[AttrGenAISystem]
	if vendor == "" {
		vendor = a[AttrGenAIProvider]
	}
	if strings.EqualFold(vendor, crewAIVendor) {
		return true
	}
	for k := range a {
		if strings.HasPrefix(k, CrewAIPrefix) {
			return true
		}
	}
	return false
}

// entityIO is a span's Traceloop entity input and output.
func (a Attrs) entityIO() (input, output *string) {
	if v := a.str(AttrEntityInput); v != nil {
		s := entityInput(*v)
		input = &s
	}
	if v := a.str(AttrEntityOutput); v != nil {
		output = entityOutput(*v)
	}
	return input, output
}

// entityInput is the inputs (or input_str) of an entity input, with its metadata when it has one.
func entityInput(v string) string {
	var m map[string]any
	if json.Unmarshal([]byte(v), &m) != nil {
		return v
	}
	nested, _ := m["inputs"].(string)
	if nested == "" {
		nested, _ = m["input_str"].(string)
	}
	if nested == "" {
		return v
	}
	var body any = nested
	if metadata, ok := m["metadata"]; ok {
		body = map[string]any{"inputs": nested, "metadata": metadata}
	}
	b, err := json.Marshal(body)
	if err != nil {
		return v
	}
	return string(b)
}

// entityOutput is the content of an entity output's last message, else its outputs.
func entityOutput(v string) *string {
	var m map[string]any
	if json.Unmarshal([]byte(v), &m) != nil {
		return &v
	}
	outputs, ok := m["outputs"]
	if !ok {
		return &v
	}
	om, ok := outputs.(map[string]any)
	if !ok {
		return marshal(outputs)
	}
	messages, ok := om["messages"].([]any)
	if !ok || len(messages) == 0 {
		return marshal(outputs)
	}
	last, ok := messages[len(messages)-1].(map[string]any)
	if !ok {
		return nil
	}
	kwargs, ok := last["kwargs"].(map[string]any)
	if !ok {
		return marshal(last)
	}
	if content, ok := kwargs["content"].(string); ok {
		return &content
	}
	return marshal(kwargs["content"])
}

func marshal(v any) *string {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}

// message is one OTel GenAI message's role and text.
type message struct {
	role, content string
}

// messagePreview is the text of the first (or, with last, the last) message
// in key with role, else of any message; nil when none has text.
func (a Attrs) messagePreview(key, role string, last bool) *string {
	messages := parseMessages(a[key])
	if last {
		slices.Reverse(messages)
	}
	for _, match := range []bool{true, false} {
		for _, m := range messages {
			if m.content != "" && (!match || strings.EqualFold(strings.TrimSpace(m.role), role)) {
				return &m.content
			}
		}
	}
	return nil
}

// parseMessages reads a JSON message list or single message, unwrapping JSON held in strings.
func parseMessages(v string) []message {
	var raw any
	for range maxMessageJSONDepth {
		if json.Unmarshal([]byte(v), &raw) != nil {
			return nil
		}
		s, ok := raw.(string)
		if !ok {
			break
		}
		if t := strings.TrimSpace(s); t == "" || (t[0] != '{' && t[0] != '[') {
			return nil
		}
		v, raw = s, nil
	}
	var items []any
	switch r := raw.(type) {
	case []any:
		items = r
	case map[string]any:
		items = []any{r}
	default:
		return nil
	}
	out := make([]message, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, toMessage(m))
		}
	}
	return out
}

// toMessage is a message's role and content, or the text of its parts.
func toMessage(m map[string]any) message {
	msg := message{}
	msg.role, _ = m["role"].(string)
	msg.content, _ = m["content"].(string)
	parts, ok := m["parts"].([]any)
	if !ok || msg.content != "" {
		return msg
	}
	var b strings.Builder
	for _, p := range parts {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		switch kind, _ := pm["type"].(string); kind {
		case "", messagePartText:
			if c, ok := pm["content"].(string); ok {
				b.WriteString(c)
			}
		case messagePartToolReply:
			reply, _ := pm["response"].(string)
			if reply == "" {
				reply, _ = pm["result"].(string)
			}
			if reply != "" {
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				b.WriteString(reply)
			}
		}
	}
	msg.content = b.String()
	return msg
}
