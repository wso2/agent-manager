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
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

type capturedReq struct {
	method   string
	path     string
	rawQuery string
	headers  http.Header
	body     []byte
}

// newTestServer returns a server that records the inbound request and replies
// with the given status and body, plus the capture and the constructed options
// pointed at the server.
func newTestServer(t *testing.T, status int, respHeaders map[string]string, respBody string) (*httptest.Server, *capturedReq, *APIOptions, *iostreams.IOStreams) {
	t.Helper()
	captured := &capturedReq{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.method = r.Method
		captured.path = r.URL.Path
		captured.rawQuery = r.URL.RawQuery
		captured.headers = r.Header.Clone()
		captured.body, _ = io.ReadAll(r.Body)
		for k, v := range respHeaders {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, respBody)
	}))
	t.Cleanup(srv.Close)

	ios, _, _, _ := iostreams.Test()
	o := &APIOptions{
		IO:         ios,
		HTTPClient: func() *http.Client { return srv.Client() },
		Token:      func(context.Context) (string, error) { return "test-token", nil },
		BaseURL:    func() (string, error) { return srv.URL, nil },
	}
	return srv, captured, o, ios
}

func bufOut(ios *iostreams.IOStreams) string { return ios.Out.(interface{ String() string }).String() }
func bufErr(ios *iostreams.IOStreams) string {
	return ios.ErrOut.(interface{ String() string }).String()
}

func TestRunAPI_GETPassthrough(t *testing.T) {
	_, captured, o, ios := newTestServer(t, http.StatusOK, nil, `{"ok":true}`)
	o.RequestPath = "/agents"
	o.RequestMethod = "GET"

	if err := runAPI(context.Background(), o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := bufOut(ios); got != `{"ok":true}` {
		t.Errorf("stdout = %q, want raw body", got)
	}
	if captured.method != "GET" {
		t.Errorf("method = %q, want GET", captured.method)
	}
	if captured.path != "/api/v1/agents" {
		t.Errorf("path = %q, want /api/v1/agents", captured.path)
	}
	if got := captured.headers.Get("Authorization"); got != "Bearer test-token" {
		t.Errorf("Authorization = %q, want Bearer test-token", got)
	}
}

// With an explicit -X GET, fields become query parameters rather than a body.
// (Without -X, fields imply POST — see TestRunAPI_POSTJSONBodyFromFields.)
func TestRunAPI_GETQueryFromFields(t *testing.T) {
	_, captured, o, _ := newTestServer(t, http.StatusOK, nil, `[]`)
	o.RequestPath = "/agents"
	o.RequestMethod = "GET"
	o.RequestMethodPassed = true
	o.RawFields = []string{"limit=2"}

	if err := runAPI(context.Background(), o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.method != "GET" {
		t.Errorf("method = %q, want GET (explicit -X GET)", captured.method)
	}
	if captured.rawQuery != "limit=2" {
		t.Errorf("query = %q, want limit=2", captured.rawQuery)
	}
	if len(captured.body) != 0 {
		t.Errorf("GET should have no body, got %q", captured.body)
	}
}

func TestRunAPI_POSTJSONBodyFromFields(t *testing.T) {
	_, captured, o, ios := newTestServer(t, http.StatusCreated, nil, `{"id":"a1"}`)
	o.RequestPath = "/agents"
	o.RequestMethod = "GET" // default; not passed -> inferred POST because fields present
	o.RequestMethodPassed = false
	o.RawFields = []string{"name=order"}
	o.MagicFields = []string{"count=2", "active=true"}

	if err := runAPI(context.Background(), o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.method != "POST" {
		t.Errorf("method = %q, want POST (inferred)", captured.method)
	}
	if ct := captured.headers.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]any
	if err := json.Unmarshal(captured.body, &body); err != nil {
		t.Fatalf("body not JSON: %v (%q)", err, captured.body)
	}
	if body["name"] != "order" || body["count"] != float64(2) || body["active"] != true {
		t.Errorf("body = %#v, want typed fields", body)
	}
	if got := bufOut(ios); got != `{"id":"a1"}` {
		t.Errorf("stdout = %q, want raw body", got)
	}
}

func TestRunAPI_ErrorStatusRawBodyAndExit(t *testing.T) {
	_, _, o, ios := newTestServer(t, http.StatusNotFound, nil, `{"message":"nope"}`)
	o.RequestPath = "/missing"
	o.RequestMethod = "GET"

	err := runAPI(context.Background(), o)
	if err == nil {
		t.Fatal("expected non-nil error for HTTP 404")
	}
	if !render.IsRendered(err) {
		t.Error("expected the error to be Rendered (no re-print / usage)")
	}
	if got := bufOut(ios); got != `{"message":"nope"}` {
		t.Errorf("stdout = %q, want raw error body", got)
	}
	if got := bufErr(ios); !strings.Contains(got, "HTTP 404") {
		t.Errorf("stderr = %q, want it to mention HTTP 404", got)
	}
}

func TestRunAPI_IncludeHeaders(t *testing.T) {
	_, _, o, ios := newTestServer(t, http.StatusOK, map[string]string{"X-Test": "yes"}, `{"ok":true}`)
	o.RequestPath = "/agents"
	o.RequestMethod = "GET"
	o.ShowResponseHeaders = true

	if err := runAPI(context.Background(), o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := bufOut(ios)
	if !strings.Contains(got, "HTTP") || !strings.Contains(got, "200") {
		t.Errorf("stdout = %q, want a status line", got)
	}
	if !strings.Contains(got, "X-Test: yes") {
		t.Errorf("stdout = %q, want the X-Test header", got)
	}
	if !strings.Contains(got, `{"ok":true}`) {
		t.Errorf("stdout = %q, want the body after headers", got)
	}
}

func TestRunAPI_CustomHeaderAndAuthOverride(t *testing.T) {
	_, captured, o, _ := newTestServer(t, http.StatusOK, nil, `{}`)
	o.RequestPath = "/agents"
	o.RequestMethod = "GET"
	o.RequestHeaders = []string{"X-Custom: hi", "Authorization: Bearer override"}

	if err := runAPI(context.Background(), o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := captured.headers.Get("X-Custom"); got != "hi" {
		t.Errorf("X-Custom = %q, want hi", got)
	}
	if got := captured.headers.Get("Authorization"); got != "Bearer override" {
		t.Errorf("Authorization = %q, want the user-supplied override (not the token)", got)
	}
}

func TestRunAPI_SubstitutesPlaceholders(t *testing.T) {
	_, captured, o, _ := newTestServer(t, http.StatusOK, nil, `{"agents":[]}`)
	o.RequestPath = "/orgs/{org}/projects/{project}/agents"
	o.RequestMethod = "GET"
	o.Scope = func() (string, string, error) { return "acme", "bot", nil }

	if err := runAPI(context.Background(), o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.path != "/api/v1/orgs/acme/projects/bot/agents" {
		t.Errorf("path = %q, want /api/v1/orgs/acme/projects/bot/agents", captured.path)
	}
}

// A literal path must never trigger context resolution.
func TestRunAPI_LiteralPathSkipsScope(t *testing.T) {
	_, captured, o, _ := newTestServer(t, http.StatusOK, nil, `{}`)
	o.RequestPath = "/projects"
	o.RequestMethod = "GET"
	o.Scope = func() (string, string, error) {
		t.Fatal("Scope must not be called for a literal path")
		return "", "", nil
	}

	if err := runAPI(context.Background(), o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.path != "/api/v1/projects" {
		t.Errorf("path = %q, want /api/v1/projects", captured.path)
	}
}

// When a placeholder cannot be resolved, the command errors before making any
// HTTP request.
func TestRunAPI_PlaceholderMissingValueErrors(t *testing.T) {
	srv, captured, o, _ := newTestServer(t, http.StatusOK, nil, `{}`)
	_ = srv
	o.RequestPath = "/orgs/{org}/projects/{project}/agents"
	o.RequestMethod = "GET"
	o.Scope = func() (string, string, error) { return "acme", "", nil }

	err := runAPI(context.Background(), o)
	if err == nil {
		t.Fatal("expected an error when {project} is unresolved")
	}
	if captured.method != "" {
		t.Errorf("no HTTP request should be made; got method %q", captured.method)
	}
}

func TestApplyHeaders_Malformed(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://x", nil)
	if err := applyHeaders(req, []string{"NoColonHere"}); err == nil {
		t.Error("expected error for header without ':'")
	}
}

// A user-supplied Authorization header is preserved and token resolution is
// skipped entirely (so it works even when not logged in).
func TestRunAPI_AuthHeaderSkipsTokenResolution(t *testing.T) {
	_, captured, o, _ := newTestServer(t, http.StatusOK, nil, `{}`)
	o.RequestPath = "/agents"
	o.RequestMethod = "GET"
	o.RequestHeaders = []string{"Authorization: Bearer mine"}
	o.Token = func(context.Context) (string, error) {
		t.Fatal("Token must not be called when an Authorization header is supplied")
		return "", nil
	}

	if err := runAPI(context.Background(), o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := captured.headers.Get("Authorization"); got != "Bearer mine" {
		t.Errorf("Authorization = %q, want Bearer mine", got)
	}
}

// --input - and an @- field both read stdin; requesting both must error before
// any HTTP request rather than silently sending malformed data.
func TestRunAPI_RejectsDoubleStdin(t *testing.T) {
	srv, captured, o, ios := newTestServer(t, http.StatusOK, nil, `{}`)
	_ = srv
	ios.In = strings.NewReader(`payload`)
	o.RequestPath = "/agents"
	o.RequestMethod = "POST"
	o.RequestMethodPassed = true
	o.RequestInputFile = "-"
	o.MagicFields = []string{"data=@-"}

	err := runAPI(context.Background(), o)
	if err == nil {
		t.Fatal("expected an error when --input - and an @- field both consume stdin")
	}
	if captured.method != "" {
		t.Errorf("no HTTP request should be made; got method %q", captured.method)
	}
}

func TestRunAPI_InputFromStdin(t *testing.T) {
	srv, captured, o, ios := newTestServer(t, http.StatusOK, nil, `{}`)
	_ = srv
	ios.In = strings.NewReader(`{"raw":1}`)
	o.RequestPath = "/agents"
	o.RequestMethod = "GET" // not passed -> inferred POST because input present
	o.RequestInputFile = "-"

	if err := runAPI(context.Background(), o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.method != "POST" {
		t.Errorf("method = %q, want POST (inferred from --input)", captured.method)
	}
	if string(captured.body) != `{"raw":1}` {
		t.Errorf("body = %q, want raw stdin passthrough", captured.body)
	}
}
