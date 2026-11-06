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

func TestExtractLogLevel(t *testing.T) {
	cases := map[string]string{
		"ERROR boom":                  "ERROR",
		`{"level":"warn","msg":"x"}`:  "WARN",
		"level=debug msg=hi":          "DEBUG",
		"E0501 12:34:56.123 failure":  "ERROR",
		`msg="x" error="<nil>"`:       "INFO",
		"plain":                       "INFO",
		`{"severity":"CRITICAL"}`:     "FATAL",
		"2026 WARNING disk almost up": "WARN",
	}
	for line, want := range cases {
		if got := extractLogLevel(line); got != want {
			t.Errorf("extractLogLevel(%q) = %q, want %q", line, got, want)
		}
	}
}
