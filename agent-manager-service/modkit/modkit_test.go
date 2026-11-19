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

package modkit

import (
	"strings"
	"testing"
)

func TestCheckName(t *testing.T) {
	for _, tc := range []struct {
		name   string
		others []string
		want   string
	}{
		{name: "ai-gateway", others: []string{"evaluation", "observability"}},
		{name: "t-store", others: []string{"t-consumer"}},
		{name: "a_b", want: "lowercase letters, digits and dashes"},
		{name: "AiGateway", want: "lowercase letters"},
		{name: "1st", want: "starting with a letter"},
		{name: "", want: "lowercase letters"},
		{name: strings.Repeat("a", 25), want: "at most 24"},
		{name: "a-b", others: []string{"a-b"}, want: "overlapping"},
		{name: "t-sneaky", others: []string{"t"}, want: "overlapping"},
		{name: "t", others: []string{"t-sneaky"}, want: "overlapping"},
		{name: "a-", others: []string{"a"}, want: "overlapping"},
	} {
		err := CheckName(tc.name, tc.others)
		if tc.want == "" && err != nil {
			t.Errorf("%q next to %v: %v", tc.name, tc.others, err)
		}
		if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%q next to %v: want error containing %q, got %v", tc.name, tc.others, tc.want, err)
		}
	}
}
