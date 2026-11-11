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
	"time"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
)

func sampleGateway(name, uuid string, gwType amsvc.GatewayType, envs ...string) amsvc.GatewayResponse {
	g := amsvc.GatewayResponse{
		Name:        name,
		DisplayName: strings.ToUpper(name),
		Uuid:        uuid,
		GatewayType: gwType,
		Status:      amsvc.GatewayStatus("ACTIVE"),
		Vhost:       name + ".example.com",
		CreatedAt:   time.Unix(0, 0).UTC(),
		UpdatedAt:   time.Unix(0, 0).UTC(),
	}
	if len(envs) > 0 {
		bindings := make([]amsvc.GatewayEnvironmentResponse, 0, len(envs))
		for _, env := range envs {
			bindings = append(bindings, amsvc.GatewayEnvironmentResponse{Name: env})
		}
		g.Environments = &bindings
	}
	return g
}

func sampleListResponse(gateways ...amsvc.GatewayResponse) amsvc.GatewayListResponse {
	return amsvc.GatewayListResponse{
		Gateways: gateways,
		Total:    int32(len(gateways)),
		Limit:    100,
		Offset:   0,
	}
}

func TestList_SuccessJSON(t *testing.T) {
	io, out, _ := newTestIO()
	clientFn, captured, closeFn := newTestClient(t, http.StatusOK, sampleListResponse(
		sampleGateway("edge", "11111111-1111-1111-1111-111111111111", amsvc.GatewayTypeBOTH, "default"),
	))
	defer closeFn()

	err := runList(context.Background(), &ListOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.method != "GET" {
		t.Errorf("method = %q, want GET", captured.method)
	}
	if captured.path != "/orgs/acme/gateways" {
		t.Errorf("path = %q, want /orgs/acme/gateways", captured.path)
	}
	env := decodeEnvelope(t, out.String())
	gateways := env["data"].(map[string]any)["gateways"].([]any)
	if len(gateways) != 1 {
		t.Fatalf("gateways len = %d, want 1", len(gateways))
	}
	if gateways[0].(map[string]any)["uuid"] != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("uuid = %v", gateways[0].(map[string]any)["uuid"])
	}
}

// A list must not hide rows by default: the whole point of the command is showing
// which gateways exist, and the placement rules are explained by the type column.
func TestList_SendsNoFiltersByDefault(t *testing.T) {
	io, _, _ := newTestIO()
	clientFn, captured, closeFn := newTestClient(t, http.StatusOK, sampleListResponse())
	defer closeFn()

	if err := runList(context.Background(), &ListOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.rawQuery != "" {
		t.Errorf("rawQuery = %q, want empty (no implicit type or status filter)", captured.rawQuery)
	}
}

func TestList_SendsTypeFilter(t *testing.T) {
	io, _, _ := newTestIO()
	clientFn, captured, closeFn := newTestClient(t, http.StatusOK, sampleListResponse())
	defer closeFn()

	if err := runList(context.Background(), &ListOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
		Type: "egress",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(captured.rawQuery, "type=EGRESS") {
		t.Errorf("rawQuery = %q, want type=EGRESS (upper-cased)", captured.rawQuery)
	}
}

func TestList_SendsPagination(t *testing.T) {
	io, _, _ := newTestIO()
	clientFn, captured, closeFn := newTestClient(t, http.StatusOK, sampleListResponse())
	defer closeFn()

	limit, offset := int32(5), int32(10)
	if err := runList(context.Background(), &ListOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
		Limit: &limit, Offset: &offset,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(captured.rawQuery, "limit=5") || !strings.Contains(captured.rawQuery, "offset=10") {
		t.Errorf("rawQuery = %q, want limit=5 and offset=10", captured.rawQuery)
	}
}

func TestList_RejectsUnknownTypeWithoutCallingServer(t *testing.T) {
	io, _, _ := newTestIO()
	clientFn, captured, closeFn := newTestClient(t, http.StatusOK, sampleListResponse())
	defer closeFn()

	err := runList(context.Background(), &ListOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(), Type: "AI-GATEWAY",
	})
	if err == nil {
		t.Fatal("expected an error for an unknown --type")
	}
	if captured.called {
		t.Error("server was called despite an invalid --type")
	}
}

func TestList_EmptyPrintsNoticeOnTTY(t *testing.T) {
	io, out, errOut := newTextTestIO(true)
	clientFn, _, closeFn := newTestClient(t, http.StatusOK, sampleListResponse())
	defer closeFn()

	if err := runList(context.Background(), &ListOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(errOut.String(), "No gateways found") {
		t.Errorf("errOut = %q, want an empty-list notice", errOut.String())
	}
	if out.String() != "" {
		t.Errorf("stdout = %q, want empty so the notice cannot be piped into a parser", out.String())
	}
}

// The notice names the filters, so a filter that matches nothing does not read as
// "this org has no gateways".
func TestList_EmptyNoticeNamesActiveFilters(t *testing.T) {
	io, _, errOut := newTextTestIO(true)
	clientFn, _, closeFn := newTestClient(t, http.StatusOK, sampleListResponse())
	defer closeFn()

	if err := runList(context.Background(), &ListOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
		Type: "egress",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(errOut.String(), "type EGRESS") {
		t.Errorf("errOut = %q, want the filter named", errOut.String())
	}
}

func TestList_EmptyStaysSilentWhenPiped(t *testing.T) {
	io, out, errOut := newTextTestIO(false)
	clientFn, _, closeFn := newTestClient(t, http.StatusOK, sampleListResponse())
	defer closeFn()

	if err := runList(context.Background(), &ListOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.String() != "" || errOut.String() != "" {
		t.Errorf("piped output must stay empty; stdout=%q stderr=%q", out.String(), errOut.String())
	}
}

func TestList_TableShowsUUID(t *testing.T) {
	io, out, _ := newTextTestIO(true)
	clientFn, _, closeFn := newTestClient(t, http.StatusOK, sampleListResponse(
		sampleGateway("edge", "11111111-1111-1111-1111-111111111111", amsvc.GatewayTypeBOTH, "prod", "default"),
		sampleGateway("inbound", "22222222-2222-2222-2222-222222222222", amsvc.GatewayTypeINGRESS),
	))
	defer closeFn()

	if err := runList(context.Background(), &ListOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rendered := out.String()
	for _, want := range []string{"UUID", "11111111-1111-1111-1111-111111111111", "BOTH", "INGRESS"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("table missing %q; got:\n%s", want, rendered)
		}
	}
}

func TestList_ServerErrorIsSurfaced(t *testing.T) {
	io, out, _ := newTestIO()
	clientFn, _, closeFn := newTestClient(t, http.StatusInternalServerError,
		map[string]string{"code": "INTERNAL_ERROR", "message": "boom"})
	defer closeFn()

	err := runList(context.Background(), &ListOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
	})
	if err == nil {
		t.Fatal("expected an error on 500")
	}
	if !strings.Contains(out.String(), "boom") {
		t.Errorf("envelope = %q, want the server message", out.String())
	}
}

func TestListCmd_RejectsZeroLimit(t *testing.T) {
	io, _, _ := newTestIO()
	clientFn, captured, closeFn := newTestClient(t, http.StatusOK, sampleListResponse())
	defer closeFn()

	root := testGatewayCmd(t, io, clientFn)
	root.SetArgs([]string{"gateway", "list", "--limit", "0"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected an error for --limit 0")
	}
	if captured.called {
		t.Error("server was called despite an invalid --limit")
	}
}

// The flags are ints narrowed to the wire's int32. Without an upper bound
// `--limit 4294967297` truncated to 1 and `--offset 4294967296` to 0, so the command
// quietly requested a page nobody asked for instead of rejecting the value.
func TestListCmd_RejectsOutOfRangePagination(t *testing.T) {
	for _, tc := range []struct{ flag, value string }{
		{flag: "--limit", value: "4294967297"},  // truncates to 1
		{flag: "--limit", value: "2147483648"},  // MaxInt32 + 1
		{flag: "--offset", value: "4294967296"}, // truncates to 0
	} {
		t.Run(tc.flag+"="+tc.value, func(t *testing.T) {
			io, _, _ := newTestIO()
			clientFn, captured, closeFn := newTestClient(t, http.StatusOK, sampleListResponse())
			defer closeFn()

			root := testGatewayCmd(t, io, clientFn)
			root.SetArgs([]string{"gateway", "list", tc.flag, tc.value})
			if err := root.Execute(); err == nil {
				t.Fatalf("expected an error for %s %s", tc.flag, tc.value)
			}
			if captured.called {
				t.Errorf("server was called despite an invalid %s", tc.flag)
			}
		})
	}
}

func TestNormalizeGatewayType(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    amsvc.GatewayTypeInput
		wantErr bool
	}{
		{in: "EGRESS", want: amsvc.GatewayTypeInputEGRESS},
		{in: "egress", want: amsvc.GatewayTypeInputEGRESS},
		{in: " both ", want: amsvc.GatewayTypeInputBOTH},
		{in: "INGRESS", want: amsvc.GatewayTypeInputINGRESS},
		// Deprecated server-side aliases are deliberately not advertised.
		{in: "REGULAR", wantErr: true},
		{in: "AI", wantErr: true},
		{in: "nonsense", wantErr: true},
	} {
		got, err := normalizeGatewayType(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("normalizeGatewayType(%q) = %q, want an error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeGatewayType(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("normalizeGatewayType(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
