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

package audit

import (
	"strings"
	"testing"
)

// TestSanitizeURLStripsCredentialBearingComponents covers the two places a URL
// can hold a credential in its own syntax: RFC 3986 userinfo in the authority,
// and a query parameter. Neither is what makes a JWKS URI worth recording.
func TestSanitizeURLStripsCredentialBearingComponents(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		mustNot []string
	}{
		{
			name: "plain url is unchanged",
			raw:  "https://idp.example/.well-known/jwks.json",
			want: "https://idp.example/.well-known/jwks.json",
		},
		{
			name:    "userinfo is removed",
			raw:     "https://admin:hunter2@idp.example/jwks",
			want:    "https://idp.example/jwks[redacted-components]",
			mustNot: []string{"admin", "hunter2"},
		},
		{
			name:    "query is removed",
			raw:     "https://idp.example/jwks?access_token=s3cr3t-value",
			want:    "https://idp.example/jwks[redacted-components]",
			mustNot: []string{"access_token", "s3cr3t-value"},
		},
		{
			name:    "fragment is removed",
			raw:     "https://idp.example/jwks#tok=abc",
			want:    "https://idp.example/jwks[redacted-components]",
			mustNot: []string{"abc"},
		},
		{
			name:    "userinfo and query together",
			raw:     "https://u:p@idp.example/jwks?k=v",
			want:    "https://idp.example/jwks[redacted-components]",
			mustNot: []string{"u:p", "k=v"},
		},
		{
			name: "empty stays empty",
			raw:  "",
			want: "",
		},
		{
			name:    "unparseable is reported, not echoed",
			raw:     "not a url at all with a s3cr3t in it",
			want:    "[unparseable-url]",
			mustNot: []string{"s3cr3t"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeURL(tt.raw)
			if got != tt.want {
				t.Errorf("sanitizeURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
			for _, forbidden := range tt.mustNot {
				if strings.Contains(got, forbidden) {
					t.Errorf("sanitizeURL(%q) leaked %q in %q", tt.raw, forbidden, got)
				}
			}
		})
	}
}

// TestURLFieldsAreSanitizedAtRedaction is the guarantee that matters: the emit
// site passes the URI verbatim, and it is the schema kind — not the caller's
// memory — that keeps a credential out of the record.
func TestURLFieldsAreSanitizedAtRedaction(t *testing.T) {
	e := Event{
		Action: ActionEventDestinationCreate,
		Details: map[string]any{
			"kind":        "webhook",
			"destination": "https://svc:p@ssw0rd@idp.example/jwks?token=abcdef123456",
		},
	}

	redact(&e)

	got, ok := e.Details["destination"].(string)
	if !ok {
		t.Fatalf("destination missing or not a string: %#v", e.Details["destination"])
	}
	for _, forbidden := range []string{"p@ssw0rd", "abcdef123456", "token=", "svc:"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("recorded destination %q still contains %q", got, forbidden)
		}
	}
	if !strings.Contains(got, "idp.example") {
		t.Errorf("recorded destination %q lost the host, which is the reason it is recorded", got)
	}
}

// TestURLKindIsDeclaredWhereItIsNeeded stops a future URL-valued detail from
// being declared as a plain name, which would silently opt it out of
// sanitisation. The check is by key suffix because that is what a reviewer
// looking at the schema would notice.
func TestURLKindIsDeclaredWhereItIsNeeded(t *testing.T) {
	for _, action := range SchemaActions() {
		for key, kind := range DetailSchema(action) {
			lower := strings.ToLower(key)
			looksLikeURL := strings.HasSuffix(lower, "uri") ||
				strings.HasSuffix(lower, "url") ||
				strings.HasSuffix(lower, "endpoint")
			if looksLikeURL && kind != KindURL {
				t.Errorf("action %q declares %q as %q; a URL-valued detail must be KindURL "+
					"so credentials in userinfo or the query cannot reach a record", action, key, kind)
			}
		}
	}
}
