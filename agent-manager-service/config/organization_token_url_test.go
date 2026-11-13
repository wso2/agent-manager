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

package config

import (
	"strings"
	"testing"
)

func TestOrganizationTokenURL(t *testing.T) {
	const org = "6f1e2c3d-0000-4000-8000-000000000001"
	tests := []struct {
		name     string
		tokenURL string
		ouID     string
		want     string
		wantErr  string
	}{
		{
			name:     "inserts the organization before /oauth2/token",
			tokenURL: "https://platform-idp.example.com/oauth2/token",
			ouID:     org,
			want:     "https://platform-idp.example.com/ou/" + org + "/oauth2/token",
		},
		{
			name:     "keeps a path prefix and port",
			tokenURL: "http://thunder.amp.localhost:8080/tenant/oauth2/token",
			ouID:     org,
			want:     "http://thunder.amp.localhost:8080/tenant/ou/" + org + "/oauth2/token",
		},
		{
			name:     "trims surrounding whitespace from the token URL",
			tokenURL: "  https://idp.example.com/oauth2/token  ",
			ouID:     org,
			want:     "https://idp.example.com/ou/" + org + "/oauth2/token",
		},
		{name: "requires an organization", tokenURL: "https://idp.example.com/oauth2/token", ouID: "", wantErr: "invalid organization ID"},
		{name: "refuses a path separator in the organization", tokenURL: "https://idp.example.com/oauth2/token", ouID: "a/b", wantErr: "invalid organization ID"},
		{name: "refuses a parent segment as the organization", tokenURL: "https://idp.example.com/oauth2/token", ouID: "..", wantErr: "invalid organization ID"},
		{name: "refuses characters that need escaping", tokenURL: "https://idp.example.com/oauth2/token", ouID: "a%2fb", wantErr: "invalid organization ID"},
		{name: "refuses a relative URL", tokenURL: "/oauth2/token", ouID: org, wantErr: "absolute http(s) URL"},
		{name: "refuses a non-http scheme", tokenURL: "ftp://idp.example.com/oauth2/token", ouID: org, wantErr: "absolute http(s) URL"},
		{name: "refuses another token path", tokenURL: "https://idp.example.com/token", ouID: org, wantErr: "must end with /oauth2/token"},
		{name: "refuses a query", tokenURL: "https://idp.example.com/oauth2/token?x=1", ouID: org, wantErr: "query or fragment"},
		{name: "refuses an organization's token endpoint", tokenURL: "https://idp.example.com/ou/other/oauth2/token", ouID: org, wantErr: "not an organization's"},
		{name: "refuses a path ending in /ou", tokenURL: "https://idp.example.com/ou/oauth2/token", ouID: org, wantErr: "not an organization's"},
		{
			name:     "a path segment merely starting with ou is fine",
			tokenURL: "https://idp.example.com/outer/oauth2/token",
			ouID:     org,
			want:     "https://idp.example.com/outer/ou/" + org + "/oauth2/token",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := OrganizationTokenURL(tc.tokenURL, tc.ouID)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("OrganizationTokenURL() error = %v, want one containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("OrganizationTokenURL() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("OrganizationTokenURL() = %q, want %q", got, tc.want)
			}
		})
	}
}
