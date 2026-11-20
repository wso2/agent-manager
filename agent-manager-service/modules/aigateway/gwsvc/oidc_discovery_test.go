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

package gwsvc

import "testing"

func TestBuildOidcDiscoveryURL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{
			name: "issuer base url",
			in:   "https://accounts.google.com",
			want: "https://accounts.google.com/.well-known/openid-configuration",
		},
		{
			name: "issuer base url with trailing slash",
			in:   "https://accounts.google.com/",
			want: "https://accounts.google.com/.well-known/openid-configuration",
		},
		{
			name: "issuer with path",
			in:   "https://example.com/realms/acme",
			want: "https://example.com/realms/acme/.well-known/openid-configuration",
		},
		{
			name: "already a well-known url",
			in:   "https://idp.example.com/.well-known/openid-configuration",
			want: "https://idp.example.com/.well-known/openid-configuration",
		},
		{
			name:    "empty",
			in:      "   ",
			wantErr: true,
		},
		{
			name:    "not a url",
			in:      "not-a-url",
			wantErr: true,
		},
		{
			name:    "missing host",
			in:      "https://",
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := buildOidcDiscoveryURL(c.in)
			if (err != nil) != c.wantErr {
				t.Fatalf("buildOidcDiscoveryURL(%q) err = %v, wantErr %v", c.in, err, c.wantErr)
			}
			if !c.wantErr && got != c.want {
				t.Errorf("buildOidcDiscoveryURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
