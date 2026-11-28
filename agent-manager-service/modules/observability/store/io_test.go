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

import "testing"

func TestTraceIO(t *testing.T) {
	leaf := func(in, out string) Attrs {
		a := Attrs{}
		if in != "" {
			a[AttrInputMessages] = `[{"role":"user","content":"` + in + `"}]`
		}
		if out != "" {
			a[AttrOutputMessages] = `[{"role":"assistant","content":"` + out + `"}]`
		}
		return a
	}
	pastCap := make([]Attrs, MaxIOLeaves+1)
	for i := range pastCap {
		pastCap[i] = Attrs{}
	}
	pastCap[MaxIOLeaves-1] = leaf("", "last read")
	pastCap[MaxIOLeaves] = leaf("unread", "unread")

	for _, tc := range []struct {
		name          string
		root, child   Attrs
		leaves        []Attrs
		spanCount     int
		input, output string
	}{
		{name: "none"},
		{
			name: "root fills both; leaves unread",
			root: Attrs{AttrEntityInput: "q", AttrEntityOutput: "a"}, leaves: []Attrs{leaf("x", "y")}, spanCount: 2,
			input: "q", output: "a",
		},
		{
			name: "child fills what the root lacks",
			root: Attrs{AttrEntityInput: "q"}, child: Attrs{AttrEntityInput: "child q", AttrEntityOutput: "child a"},
			input: "q", output: "child a",
		},
		{
			name:  "input_str with no metadata",
			root:  Attrs{AttrEntityInput: `{"input_str":"hey"}`, AttrEntityOutput: `{"outputs":{"answer":"42"}}`},
			input: `"hey"`, output: `{"answer":"42"}`,
		},
		{
			name:  "outputs with no messages, and JSON that is not an object",
			root:  Attrs{AttrEntityInput: `["a"]`, AttrEntityOutput: `{"outputs":"done"}`},
			input: `["a"]`, output: `"done"`,
		},
		{
			name:      "stringified messages, a user message past the system prompt",
			leaves:    []Attrs{{AttrInputMessages: `"[{\"role\":\"system\",\"content\":\"s\"},{\"role\":\" User \",\"content\":\"u\"}]"`}},
			spanCount: 2, input: "u",
		},
		{
			name:      "no user message reads the first with text",
			leaves:    []Attrs{{AttrInputMessages: `[{"role":"system","content":""},{"role":"system","content":"s"}]`}},
			spanCount: 2, input: "s",
		},
		{
			name:      "tool replies in parts",
			leaves:    []Attrs{{AttrOutputMessages: `[{"role":"tool","parts":[{"type":"tool_call_response","result":"r1"},{"type":"tool_call_response","response":"r2"}]}]`}},
			spanCount: 2, output: "r1\nr2",
		},
		{name: "only the first leaves are read", leaves: pastCap, spanCount: MaxIOLeaves + 2, output: "last read"},
		{name: "no leaf past the span count", leaves: []Attrs{leaf("x", "y")}, spanCount: MaxIOSpanCount + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in, out := TraceIO(tc.root, tc.child, tc.leaves, tc.spanCount)
			if in != tc.input || out != tc.output {
				t.Errorf("TraceIO = %q, %q; want %q, %q", in, out, tc.input, tc.output)
			}
		})
	}
}
