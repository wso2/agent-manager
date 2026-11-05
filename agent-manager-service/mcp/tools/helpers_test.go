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

package tools

import "testing"

func TestRedactURL(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"clean URL unchanged", "https://api.openai.com/v1", "https://api.openai.com/v1"},
		{"keeps port and path", "http://llm.internal:8080/openai/v1/", "http://llm.internal:8080/openai/v1/"},
		{"drops userinfo", "https://user:s3cret@api.example.com/v1", "https://api.example.com/v1"},
		{"drops username only", "https://s3cret@api.example.com/v1", "https://api.example.com/v1"},
		{"drops credential params, keeps others", "https://x.openai.azure.com/openai?api-version=2024-02-01&api-key=s3cret", "https://x.openai.azure.com/openai?api-version=2024-02-01"},
		{"matches param names case-insensitively", "https://api.example.com/v1?Access_Token=s3cret&X-API-KEY=s3cret&Sig=s3cret", "https://api.example.com/v1"},
		{"drops fragment", "https://api.example.com/v1#access_token=s3cret", "https://api.example.com/v1"},
		{"unparseable URL dropped whole", "https://user:s3cret@api.example.com:port/v1", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := redactURL(tc.in); got != tc.want {
				t.Errorf("redactURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
