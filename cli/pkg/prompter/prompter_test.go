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

package prompter

import (
	"bytes"
	"strings"
	"testing"
)

func TestLinePrompter_Confirm(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"yes lowercase", "y\n", true},
		{"yes word", "yes\n", true},
		{"YES uppercase", "YES\n", true},
		{"yes with whitespace", "  y  \n", true},
		{"no lowercase", "n\n", false},
		{"no word", "no\n", false},
		{"empty defaults to no", "\n", false},
		{"junk defaults to no", "maybe\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := &bytes.Buffer{}
			p := New(strings.NewReader(tc.input), out)
			got, err := p.Confirm("Proceed?")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("Confirm() = %v, want %v", got, tc.want)
			}
			if !strings.Contains(out.String(), "Proceed?") {
				t.Errorf("prompt not written to out: %q", out.String())
			}
		})
	}
}

func TestLinePrompter_ConfirmDeletion_StillWorks(t *testing.T) {
	out := &bytes.Buffer{}
	p := New(strings.NewReader("foo\n"), out)
	if err := p.ConfirmDeletion("foo"); err != nil {
		t.Fatalf("ConfirmDeletion: %v", err)
	}
}

func TestLinePrompter_Select(t *testing.T) {
	options := []string{"United States", "Europe"}
	cases := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{name: "empty picks the default", input: "\n", want: 0},
		{name: "no input at all picks the default", input: "", want: 0},
		{name: "number", input: "2\n", want: 1},
		{name: "number with whitespace", input: "  2 \n", want: 1},
		{name: "asks again after a bad answer", input: "3\n2\n", want: 1},
		{name: "last line without newline", input: "2", want: 1},
		{name: "gives up after repeated bad answers", input: "x\n0\n9\n1\n", wantErr: true},
		{name: "bad answer then end of input", input: "x\n", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			got, err := NewSelector(strings.NewReader(tc.input), &out).Select("Which region?", options, 0)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Select = %d, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Select: %v", err)
			}
			if got != tc.want {
				t.Errorf("Select = %d, want %d", got, tc.want)
			}
			for _, want := range []string{"Which region?", "1) United States", "2) Europe", "[1]"} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output %q missing %q", out.String(), want)
				}
			}
		})
	}
}
