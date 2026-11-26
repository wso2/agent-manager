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

package utils

import (
	"regexp"
	"testing"
)

func TestGenerateCandidateName_AlwaysValid(t *testing.T) {
	tests := []struct {
		displayName string
		want        string // empty means a random fallback is expected
	}{
		{"My Agent", "my-agent"},
		{"  Hello   World  ", "hello-world"},
		{"123 Agent", "agent"},
		{"9lives", "lives"},
		{"-- 42 - Agent X", "agent-x"},
		{"a-very-long-display-name-that-exceeds", "a-very-long-display-name"},
		{"我的代理", ""},
		{"!!!", ""},
		{"123", ""},
		{"   ", ""},
	}
	letters := regexp.MustCompile(`^[a-z]+$`)
	for _, tt := range tests {
		t.Run(tt.displayName, func(t *testing.T) {
			got := GenerateCandidateName(tt.displayName)
			if err := ValidateResourceName(got, "agent"); err != nil {
				t.Fatalf("GenerateCandidateName(%q) = %q, which fails validation: %v", tt.displayName, got, err)
			}
			if tt.want != "" && got != tt.want {
				t.Errorf("GenerateCandidateName(%q) = %q, want %q", tt.displayName, got, tt.want)
			}
			if tt.want == "" && (len(got) != FallbackCandidateNameLength || !letters.MatchString(got)) {
				t.Errorf("GenerateCandidateName(%q) = %q, want a %d-letter fallback", tt.displayName, got, FallbackCandidateNameLength)
			}
		})
	}
}

func TestGenerateUniqueNameWithSuffix_AlwaysValid(t *testing.T) {
	alwaysFree := func(string) (bool, error) { return true, nil }
	for _, base := range []string{"", "my-agent", "abcdefghijklmnopqrstu-wxy"} {
		t.Run(base, func(t *testing.T) {
			got, err := GenerateUniqueNameWithSuffix(base, alwaysFree)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err := ValidateResourceName(got, "agent"); err != nil {
				t.Errorf("GenerateUniqueNameWithSuffix(%q) = %q, which fails validation: %v", base, got, err)
			}
		})
	}
}
