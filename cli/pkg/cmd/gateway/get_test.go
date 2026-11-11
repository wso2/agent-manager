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

package gateway

import (
	"context"
	"net/http"
	"strings"
	"testing"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
)

func TestGet_SuccessJSON(t *testing.T) {
	io, out, _ := newTestIO()
	gw := sampleGateway("edge", "11111111-1111-1111-1111-111111111111", amsvc.GatewayTypeBOTH, "default")
	clientFn, captured, closeFn := newTestClient(t, http.StatusOK, gw)
	defer closeFn()

	err := runGet(context.Background(), &GetOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(), Gateway: "edge",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.path != "/orgs/acme/gateways/edge" {
		t.Errorf("path = %q, want /orgs/acme/gateways/edge", captured.path)
	}
	env := decodeEnvelope(t, out.String())
	if env["data"].(map[string]any)["uuid"] != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("uuid = %v", env["data"].(map[string]any)["uuid"])
	}
}

// The command takes a name or a UUID and sends it as-is; resolving either form is
// the server's job, and it must not answer a bad identifier with a 500.
func TestGet_AcceptsUUIDAndName(t *testing.T) {
	for _, identifier := range []string{"edge", "11111111-1111-1111-1111-111111111111"} {
		io, _, _ := newTestIO()
		clientFn, captured, closeFn := newTestClient(t, http.StatusOK,
			sampleGateway("edge", "11111111-1111-1111-1111-111111111111", amsvc.GatewayTypeBOTH))

		err := runGet(context.Background(), &GetOptions{
			IO: io, Client: clientFn, Org: "acme", Scope: baseScope(), Gateway: identifier,
		})
		closeFn()
		if err != nil {
			t.Fatalf("runGet(%q): %v", identifier, err)
		}
		if want := "/orgs/acme/gateways/" + identifier; captured.path != want {
			t.Errorf("path = %q, want %q", captured.path, want)
		}
	}
}

func TestGet_TextOutputShowsUUIDAndType(t *testing.T) {
	io, out, _ := newTextTestIO(true)
	clientFn, _, closeFn := newTestClient(t, http.StatusOK,
		sampleGateway("edge", "11111111-1111-1111-1111-111111111111", amsvc.GatewayTypeEGRESS, "default"))
	defer closeFn()

	if err := runGet(context.Background(), &GetOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(), Gateway: "edge",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"11111111-1111-1111-1111-111111111111", "EGRESS", "edge.example.com"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q; got:\n%s", want, out.String())
		}
	}
}

func TestGet_RejectsEmptyIdentifierWithoutCallingServer(t *testing.T) {
	io, _, _ := newTestIO()
	clientFn, captured, closeFn := newTestClient(t, http.StatusOK, nil)
	defer closeFn()

	err := runGet(context.Background(), &GetOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(), Gateway: "  ",
	})
	if err == nil {
		t.Fatal("expected an error for an empty gateway identifier")
	}
	if captured.called {
		t.Error("server was called despite an empty identifier")
	}
}

func TestGet_NotFoundSurfacesServerMessage(t *testing.T) {
	io, out, _ := newTestIO()
	clientFn, _, closeFn := newTestClient(t, http.StatusNotFound,
		map[string]string{"code": "NOT_FOUND", "message": "gateway not found"})
	defer closeFn()

	err := runGet(context.Background(), &GetOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(), Gateway: "ghost",
	})
	if err == nil {
		t.Fatal("expected an error on 404")
	}
	if !strings.Contains(out.String(), "gateway not found") {
		t.Errorf("envelope = %q, want the server message", out.String())
	}
}
