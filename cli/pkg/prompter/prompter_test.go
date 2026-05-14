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
