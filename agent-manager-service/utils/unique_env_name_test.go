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
	"strings"
	"testing"
)

func TestGenerateUniqueNameWithinLength_EnvLimits(t *testing.T) {
	for orgLen := 1; orgLen <= MaxResourceNameLength; orgLen++ {
		org := strings.Repeat("o", orgLen)
		maxLen := MaxEnvNameLength(org)
		for _, base := range []string{"p", "production", "my-very-long-environment"} {
			taken := map[string]bool{}
			// Reject the first two proposals to exercise the retry path.
			checker := func(name string) (bool, error) {
				if len(taken) < 2 {
					taken[name] = true
					return false, nil
				}
				return !taken[name], nil
			}
			got, err := GenerateUniqueNameWithinLength(base, maxLen, checker)
			if err != nil {
				t.Fatalf("org len %d, base %q: unexpected error: %v", orgLen, base, err)
			}
			if len(got) > maxLen {
				t.Errorf("org len %d, base %q: got %q (len %d), exceeds max %d", orgLen, base, got, len(got), maxLen)
			}
			if taken[got] {
				t.Errorf("org len %d, base %q: returned taken name %q", orgLen, base, got)
			}
			if err := ValidateResourceName(got, "environment"); err != nil {
				t.Errorf("org len %d, base %q: got %q, fails validation: %v", orgLen, base, got, err)
			}
		}
	}
}

func TestGenerateUniqueNameWithinLength_KeepsBaseWhenRoomAllows(t *testing.T) {
	got, err := GenerateUniqueNameWithinLength("production", MaxEnvNameLength("acme"), func(string) (bool, error) { return true, nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, "production-") || len(got) != len("production-")+RandomSuffixLength {
		t.Errorf("got %q, want production-<%d letters>", got, RandomSuffixLength)
	}
}

func TestGenerateUniqueNameWithinLength_TrimsHyphenAtCut(t *testing.T) {
	// maxLen 8 leaves a 5-char base; "abcd-efgh" cuts to "abcd-" which must not yield "abcd--xx".
	got, err := GenerateUniqueNameWithinLength("abcd-efgh", 8, func(string) (bool, error) { return true, nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "--") || len(got) > 8 {
		t.Errorf("got %q", got)
	}
}

func TestGenerateUniqueNameWithinLength_Errors(t *testing.T) {
	if _, err := GenerateUniqueNameWithinLength("p", 0, func(string) (bool, error) { return true, nil }); err == nil {
		t.Error("expected error for maxLen 0")
	}
	if _, err := GenerateUniqueNameWithinLength("p", 2, func(string) (bool, error) { return false, nil }); err == nil {
		t.Error("expected error when every name is taken")
	}
}
