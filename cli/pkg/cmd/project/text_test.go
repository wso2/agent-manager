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

package project

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

func serverError(t *testing.T, status int, code, msg string) (func(context.Context) (*amsvc.ClientWithResponses, error), func()) {
	t.Helper()
	fn, _, closeFn := newTestClient(t, status, amsvc.ErrorResponse{Code: code, Message: msg})
	return fn, closeFn
}

// createdOn is a Project document created on the given day of 2026.
func createdOn(name, displayName string, month time.Month, day int) amsvc.ProjectDocument {
	doc := projectDoc(name, displayName)
	created := time.Date(2026, month, day, 0, 0, 0, 0, time.UTC)
	doc.Metadata.CreationTimestamp = &created
	return doc
}

func TestGet_TextOutput(t *testing.T) {
	io, out, _ := newTestIO(false)
	io.JSON = false
	created := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	clientFn, _, closeFn := newTestClient(t, http.StatusOK, func() amsvc.ProjectDocument {
		doc := projectDoc("alpha", "Alpha One")
		desc := "first"
		doc.Spec.Description = &desc
		doc.Metadata.CreationTimestamp = &created
		return doc
	}())
	defer closeFn()

	if err := runGet(context.Background(), &GetOptions{IO: io, Client: clientFn, Org: "acme", Scope: baseScope(), ProjectName: "alpha"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"alpha", "Alpha One", "first", "acme", "2026-03-04T05:06:07Z"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestGet_ClientAndTransportFailures(t *testing.T) {
	io, out, _ := newTestIO(false)
	err := runGet(context.Background(), &GetOptions{IO: io, Org: "acme", Scope: baseScope(), ProjectName: "alpha",
		Client: func(context.Context) (*amsvc.ClientWithResponses, error) {
			return nil, clierr.New(clierr.NoInstance, "no instance configured")
		}})
	if err == nil || !render.IsRendered(err) {
		t.Fatalf("err = %v", err)
	}
	if code := decodeEnvelope(t, out.String())["error"].(map[string]any)["code"]; code != clierr.NoInstance {
		t.Errorf("code = %v", code)
	}

	// Server that went away mid-flight: surfaced as CLI_TRANSPORT, not a panic.
	clientFn, _, closeFn := newTestClient(t, http.StatusOK, nil)
	closeFn()
	io2, out2, _ := newTestIO(false)
	if err := runGet(context.Background(), &GetOptions{IO: io2, Client: clientFn, Org: "acme", Scope: baseScope(), ProjectName: "alpha"}); err == nil {
		t.Fatal("expected transport error")
	}
	if code := decodeEnvelope(t, out2.String())["error"].(map[string]any)["code"]; code != clierr.Transport {
		t.Errorf("code = %v, want %s", code, clierr.Transport)
	}
}

func TestGet_PathTraversalNameRejected(t *testing.T) {
	io, out, _ := newTestIO(false)
	err := runGet(context.Background(), &GetOptions{IO: io, Client: unreachableClient, Org: "acme", Scope: baseScope(), ProjectName: "../other"})
	if err == nil {
		t.Fatal("expected error")
	}
	if code := decodeEnvelope(t, out.String())["error"].(map[string]any)["code"]; code != clierr.InvalidFlag {
		t.Errorf("code = %v", code)
	}
}

func TestList_TextTable(t *testing.T) {
	io, out, _ := newTestIO(false)
	io.JSON = false
	clientFn, captured, closeFn := newTestClient(t, http.StatusOK, amsvc.ProjectListResponse{
		Count: 2,
		List:  []amsvc.ProjectDocument{createdOn("alpha", "Alpha", 1, 2), createdOn("beta", "Beta", 2, 3)},
	})
	defer closeFn()

	if err := runList(context.Background(), &ListOptions{IO: io, Client: clientFn, Org: "acme", Scope: baseScope()}); err != nil {
		t.Fatal(err)
	}
	if captured.path != "/projects" {
		t.Errorf("path = %q", captured.path)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want one row per project, got:\n%s", out.String())
	}
	for i, want := range [][]string{{"alpha", "Alpha", "2026-01-02"}, {"beta", "Beta", "2026-02-03"}} {
		for _, w := range want {
			if !strings.Contains(lines[i], w) {
				t.Errorf("row %d %q missing %q", i, lines[i], w)
			}
		}
	}
}

func TestList_FailureModes(t *testing.T) {
	t.Run("client unavailable", func(t *testing.T) {
		io, out, _ := newTestIO(false)
		err := runList(context.Background(), &ListOptions{IO: io, Org: "acme", Scope: baseScope(),
			Client: func(context.Context) (*amsvc.ClientWithResponses, error) { return nil, errors.New("boom") }})
		if err == nil || decodeEnvelope(t, out.String())["error"] == nil {
			t.Fatalf("err=%v out=%s", err, out.String())
		}
	})
	t.Run("transport", func(t *testing.T) {
		clientFn, _, closeFn := newTestClient(t, 200, nil)
		closeFn()
		io, out, _ := newTestIO(false)
		if err := runList(context.Background(), &ListOptions{IO: io, Client: clientFn, Org: "acme", Scope: baseScope()}); err == nil {
			t.Fatal("expected error")
		}
		if code := decodeEnvelope(t, out.String())["error"].(map[string]any)["code"]; code != clierr.Transport {
			t.Errorf("code = %v", code)
		}
	})
	t.Run("unknown org is a 404 with the server's code", func(t *testing.T) {
		clientFn, closeFn := serverError(t, http.StatusNotFound, "ORG_NOT_FOUND", "org missing")
		defer closeFn()
		io, out, _ := newTestIO(false)
		if err := runList(context.Background(), &ListOptions{IO: io, Client: clientFn, Org: "nope", Scope: baseScope()}); err == nil {
			t.Fatal("expected error")
		}
		e := decodeEnvelope(t, out.String())["error"].(map[string]any)
		if e["code"] != "ORG_NOT_FOUND" || e["status"] != float64(404) {
			t.Errorf("error = %v", e)
		}
	})
	t.Run("non-JSON error body", func(t *testing.T) {
		clientFn, _, closeFn := newTestClient(t, http.StatusBadGateway, nil)
		defer closeFn()
		io, out, _ := newTestIO(false)
		_ = runList(context.Background(), &ListOptions{IO: io, Client: clientFn, Org: "acme", Scope: baseScope()})
		e := decodeEnvelope(t, out.String())["error"].(map[string]any)
		if e["code"] != clierr.ServerInvalid || !strings.Contains(e["message"].(string), "502") {
			t.Errorf("error = %v", e)
		}
	})
}

func TestCreate_TextOutputAndFailures(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		io, _, errOut := newTestIO(false)
		io.JSON = false
		clientFn, captured, closeFn := newTestClient(t, http.StatusAccepted, projectDoc("alpha", "Alpha"))
		defer closeFn()
		if err := runCreate(context.Background(), &CreateOptions{IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
			Name: "alpha", DisplayName: "Alpha"}); err != nil {
			t.Fatal(err)
		}
		if captured.method != "POST" || captured.path != "/projects" {
			t.Errorf("request = %s %s", captured.method, captured.path)
		}
		if !strings.Contains(errOut.String(), "Created project alpha") {
			t.Errorf("stderr = %q", errOut.String())
		}
	})
	t.Run("client unavailable", func(t *testing.T) {
		io, out, _ := newTestIO(false)
		err := runCreate(context.Background(), &CreateOptions{IO: io, Org: "acme", Scope: baseScope(), Name: "a", DisplayName: "A",
			Client: func(context.Context) (*amsvc.ClientWithResponses, error) { return nil, errors.New("no client") }})
		if err == nil || !strings.Contains(out.String(), "no client") {
			t.Fatalf("err=%v out=%s", err, out.String())
		}
	})
	t.Run("transport", func(t *testing.T) {
		clientFn, _, closeFn := newTestClient(t, 200, nil)
		closeFn()
		io, out, _ := newTestIO(false)
		if err := runCreate(context.Background(), &CreateOptions{IO: io, Client: clientFn, Org: "acme", Scope: baseScope(), Name: "a", DisplayName: "A"}); err == nil {
			t.Fatal("expected error")
		}
		if code := decodeEnvelope(t, out.String())["error"].(map[string]any)["code"]; code != clierr.Transport {
			t.Errorf("code = %v", code)
		}
	})
	t.Run("server rejects", func(t *testing.T) {
		clientFn, closeFn := serverError(t, http.StatusInternalServerError, "INTERNAL", "db down")
		defer closeFn()
		io, out, _ := newTestIO(false)
		_ = runCreate(context.Background(), &CreateOptions{IO: io, Client: clientFn, Org: "acme", Scope: baseScope(), Name: "a", DisplayName: "A"})
		e := decodeEnvelope(t, out.String())["error"].(map[string]any)
		if e["code"] != "INTERNAL" || e["message"] != "db down" {
			t.Errorf("error = %v", e)
		}
	})
}

func TestDelete_TextOutputAndFailures(t *testing.T) {
	t.Run("text with --yes", func(t *testing.T) {
		io, _, errOut := newTestIO(false)
		io.JSON = false
		clientFn, _, closeFn := newTestClient(t, http.StatusNoContent, nil)
		defer closeFn()
		if err := runDelete(context.Background(), &DeleteOptions{IO: io, Client: clientFn, Org: "acme", Scope: baseScope(), ProjectName: "alpha", Yes: true}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(errOut.String(), "Deleted project alpha") {
			t.Errorf("stderr = %q", errOut.String())
		}
	})
	t.Run("no request is made when confirmation is refused", func(t *testing.T) {
		clientFn, captured, closeFn := newTestClient(t, http.StatusNoContent, nil)
		defer closeFn()
		io, _, _ := newTestIO(true)
		err := runDelete(context.Background(), &DeleteOptions{IO: io, Client: clientFn, Org: "acme", Scope: baseScope(), ProjectName: "alpha",
			Prompter: &fakePrompter{confirmDeletionErr: errors.New("typed the wrong name")}})
		if err == nil || captured.called {
			t.Fatalf("err=%v called=%v", err, captured.called)
		}
	})
	t.Run("client unavailable", func(t *testing.T) {
		io, out, _ := newTestIO(false)
		err := runDelete(context.Background(), &DeleteOptions{IO: io, Org: "acme", Scope: baseScope(), ProjectName: "a", Yes: true,
			Client: func(context.Context) (*amsvc.ClientWithResponses, error) { return nil, errors.New("no client") }})
		if err == nil || !strings.Contains(out.String(), "no client") {
			t.Fatalf("err=%v out=%s", err, out.String())
		}
	})
	t.Run("transport", func(t *testing.T) {
		clientFn, _, closeFn := newTestClient(t, 204, nil)
		closeFn()
		io, out, _ := newTestIO(false)
		_ = runDelete(context.Background(), &DeleteOptions{IO: io, Client: clientFn, Org: "acme", Scope: baseScope(), ProjectName: "a", Yes: true})
		if code := decodeEnvelope(t, out.String())["error"].(map[string]any)["code"]; code != clierr.Transport {
			t.Errorf("code = %v", code)
		}
	})
	t.Run("invalid name", func(t *testing.T) {
		io, out, _ := newTestIO(false)
		err := runDelete(context.Background(), &DeleteOptions{IO: io, Client: unreachableClient, Org: "acme", Scope: baseScope(), ProjectName: "a/b", Yes: true})
		if err == nil || decodeEnvelope(t, out.String())["error"].(map[string]any)["code"] != clierr.InvalidFlag {
			t.Fatalf("err=%v out=%s", err, out.String())
		}
	})
}
