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

package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func TestResolveURL(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		endpoint string
		want     string
		wantErr  bool
	}{
		{"prefixes api/v1 for leading-slash path", "https://x.com", "/agents", "https://x.com/api/v1/agents", false},
		{"prefixes api/v1 for bare path", "https://x.com", "agents", "https://x.com/api/v1/agents", false},
		{"trims trailing slash on base", "https://x.com/", "agents", "https://x.com/api/v1/agents", false},
		{"does not double-prefix api/v1", "https://x.com", "api/v1/agents", "https://x.com/api/v1/agents", false},
		{"does not double-prefix leading-slash api/v1", "https://x.com", "/api/v1/orgs/1", "https://x.com/api/v1/orgs/1", false},
		{"preserves existing query string", "https://x.com", "/agents?limit=2", "https://x.com/api/v1/agents?limit=2", false},
		{"keeps a module's API root", "https://x.com", "/m/ai-gateway/orgs/acme/llm-providers", "https://x.com/m/ai-gateway/orgs/acme/llm-providers", false},
		{"errors on empty base", "", "agents", "", true},
		{"errors on empty endpoint", "https://x.com", "  ", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveURL(tc.baseURL, tc.endpoint)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (result %q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveURL(%q,%q) = %q, want %q", tc.baseURL, tc.endpoint, got, tc.want)
			}
		})
	}
}

func TestInferMethod(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		methodPass bool
		hasParams  bool
		inputFile  string
		want       string
	}{
		{"defaults to GET", "GET", false, false, "", "GET"},
		{"uppercases method", "get", false, false, "", "GET"},
		{"infers POST when params present", "GET", false, true, "", "POST"},
		{"infers POST when input present", "GET", false, false, "body.json", "POST"},
		{"honors explicit method despite params", "DELETE", true, true, "", "DELETE"},
		{"honors explicit GET despite params", "GET", true, true, "", "GET"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := inferMethod(tc.method, tc.methodPass, tc.hasParams, tc.inputFile)
			if got != tc.want {
				t.Errorf("inferMethod(%q,%v,%v,%q) = %q, want %q",
					tc.method, tc.methodPass, tc.hasParams, tc.inputFile, got, tc.want)
			}
		})
	}
}

func TestMagicFieldValue(t *testing.T) {
	if v, _ := magicFieldValue("true", nil); v != true {
		t.Errorf(`"true" = %#v, want true`, v)
	}
	if v, _ := magicFieldValue("false", nil); v != false {
		t.Errorf(`"false" = %#v, want false`, v)
	}
	if v, _ := magicFieldValue("null", nil); v != nil {
		t.Errorf(`"null" = %#v, want nil`, v)
	}
	if v, _ := magicFieldValue("42", nil); v != 42 {
		t.Errorf(`"42" = %#v, want 42`, v)
	}
	if v, _ := magicFieldValue("hello", nil); v != "hello" {
		t.Errorf(`"hello" = %#v, want "hello"`, v)
	}
}

func TestMagicFieldValue_FileAndStdin(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "payload.txt")
	if err := os.WriteFile(fp, []byte("file-contents"), 0o600); err != nil {
		t.Fatal(err)
	}
	if v, err := magicFieldValue("@"+fp, nil); err != nil || v != "file-contents" {
		t.Errorf(`"@file" = %#v, err %v; want "file-contents"`, v, err)
	}

	v, err := magicFieldValue("@-", strings.NewReader("stdin-contents"))
	if err != nil || v != "stdin-contents" {
		t.Errorf(`"@-" = %#v, err %v; want "stdin-contents"`, v, err)
	}
}

func TestParseFields(t *testing.T) {
	got, err := parseFields(
		[]string{"name=order-triage", "num=123"},
		[]string{"count=3", "active=true"},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Raw fields stay strings even when numeric-looking.
	if got["name"] != "order-triage" {
		t.Errorf("name = %#v, want string order-triage", got["name"])
	}
	if got["num"] != "123" {
		t.Errorf("num = %#v, want string \"123\" (raw fields are not typed)", got["num"])
	}
	// Magic fields are typed.
	if got["count"] != 3 {
		t.Errorf("count = %#v, want int 3", got["count"])
	}
	if got["active"] != true {
		t.Errorf("active = %#v, want bool true", got["active"])
	}
}

func TestParseFields_ValueWithEquals(t *testing.T) {
	got, err := parseFields([]string{"filter=a=b=c"}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["filter"] != "a=b=c" {
		t.Errorf("filter = %#v, want \"a=b=c\" (split on first =)", got["filter"])
	}
}

func TestParseFields_MissingEquals(t *testing.T) {
	if _, err := parseFields([]string{"oops"}, nil, nil); err == nil {
		t.Error("expected error for field without '='")
	}
}

func TestParseFields_EmptyKey(t *testing.T) {
	if _, err := parseFields([]string{"=value"}, nil, nil); err == nil {
		t.Error("expected error for field with an empty key")
	}
	if _, err := parseFields(nil, []string{"  =true"}, nil); err == nil {
		t.Error("expected error for field with a whitespace-only key")
	}
}

func TestHasPlaceholders(t *testing.T) {
	if !hasPlaceholders("/orgs/{org}/projects/{project}/agents") {
		t.Error("expected placeholders to be detected")
	}
	if hasPlaceholders("/orgs/default/projects/default/agents") {
		t.Error("literal path should report no placeholders")
	}
	if hasPlaceholders("/projects") {
		t.Error("plain path should report no placeholders")
	}
}

func TestSubstituteContext(t *testing.T) {
	vals := map[string]string{"org": "acme", "project": "bot"}

	got, err := substituteContext("/orgs/{org}/projects/{project}/agents", vals)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/orgs/acme/projects/bot/agents" {
		t.Errorf("got %q, want /orgs/acme/projects/bot/agents", got)
	}

	// Literal paths pass through untouched.
	if got, err := substituteContext("/orgs/default/projects/default", vals); err != nil || got != "/orgs/default/projects/default" {
		t.Errorf("literal path = %q, err %v; want unchanged", got, err)
	}
}

func TestSubstituteContext_UnknownPlaceholderErrors(t *testing.T) {
	_, err := substituteContext("/orgs/{org}/projects/{proj}/agents", map[string]string{"org": "acme", "project": "bot"})
	if err == nil {
		t.Fatal("expected an error for unknown placeholder {proj}")
	}
	if !strings.Contains(err.Error(), "{proj}") {
		t.Errorf("error %q should name the offending placeholder", err)
	}
}

func TestSubstituteContext_EmptyValueErrors(t *testing.T) {
	_, err := substituteContext("/orgs/{org}/projects/{project}/agents", map[string]string{"org": "acme", "project": ""})
	if err == nil {
		t.Fatal("expected an error when {project} resolves to empty")
	}
	if !strings.Contains(err.Error(), "{project}") {
		t.Errorf("error %q should name the unresolved placeholder", err)
	}
}

// TestAPIExamplesUseRealEndpoints guards against documenting endpoints that do
// not exist: every core "amctl api <endpoint>" example must address a core collection that exists.
func TestAPIExamplesUseRealEndpoints(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cmd := NewAPICmd(&cmdutil.Factory{IOStreams: ios})

	var endpoints []string
	for _, line := range strings.Split(cmd.Example, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "$ amctl api") {
			continue
		}
		for _, tok := range strings.Fields(line) {
			tok = strings.Trim(tok, `"`)
			if strings.HasPrefix(tok, "/") {
				endpoints = append(endpoints, tok)
			}
		}
	}
	if len(endpoints) == 0 {
		t.Fatal("found no example endpoints to check")
	}
	for _, ep := range endpoints {
		if strings.HasPrefix(ep, "/m/") {
			if _, rest, ok := strings.Cut(strings.TrimPrefix(ep, "/m/"), "/"); !ok || rest == "" {
				t.Errorf("module example endpoint %q must name a resource after its root", ep)
			}
			continue
		}
		root, _, _ := strings.Cut(strings.TrimPrefix(ep, "/"), "/")
		root, _, _ = strings.Cut(root, "?")
		if root != "organizations" && root != "projects" && root != "agents" {
			t.Errorf("example endpoint %q must begin with /organizations, /projects or /agents", ep)
		}
	}
}

func TestAddQuery(t *testing.T) {
	if got := addQuery("https://x/api/v1/agents", nil); got != "https://x/api/v1/agents" {
		t.Errorf("empty params should be unchanged, got %q", got)
	}
	if got := addQuery("https://x/api/v1/agents", map[string]any{"limit": "2"}); got != "https://x/api/v1/agents?limit=2" {
		t.Errorf("got %q", got)
	}
	// Sorted, typed values.
	if got := addQuery("https://x/api/v1/agents", map[string]any{"n": 5, "active": true}); got != "https://x/api/v1/agents?active=true&n=5" {
		t.Errorf("got %q, want sorted typed query", got)
	}
	// Appends to an existing query with '&'.
	if got := addQuery("https://x/api/v1/agents?x=1", map[string]any{"limit": "2"}); got != "https://x/api/v1/agents?x=1&limit=2" {
		t.Errorf("got %q, want '&' join", got)
	}
}
