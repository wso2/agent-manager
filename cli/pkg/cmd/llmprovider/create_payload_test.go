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

package llmprovider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
)

// accessControlMode digs out accessControl.mode from a captured create body.
func accessControlMode(t *testing.T, raw []byte) (string, bool) {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal body: %v\nbody=%s", err, raw)
	}
	ac, ok := body["accessControl"].(map[string]any)
	if !ok {
		return "", false
	}
	mode, ok := ac["mode"].(string)
	return mode, ok
}

// A deployed proxy defaults to deny_all server-side, so a create that omits the
// access control block yields a provider that is configured correctly and still
// unreachable.
func TestCreate_SendsAllowAllAccessControlByDefault(t *testing.T) {
	ios, _, _ := newTestIO(true)
	clientFn, captured, closeFn := newTestClient(t, http.StatusCreated, sampleProviderResponse())
	defer closeFn()

	err := runCreate(context.Background(), &CreateOptions{
		IO: ios, Client: clientFn, Org: "acme", Scope: baseScope(),
		ID: "openai", DisplayName: "OpenAI", Version: defaultVersion, Context: "/",
		Template: "openai", AuthType: "api-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mode, ok := accessControlMode(t, captured.body)
	if !ok {
		t.Fatalf("no accessControl block in body=%s", captured.body)
	}
	if mode != "allow_all" {
		t.Errorf("accessControl.mode = %q, want allow_all", mode)
	}
}

func TestCreate_HonoursExplicitAccessMode(t *testing.T) {
	ios, _, _ := newTestIO(true)
	clientFn, captured, closeFn := newTestClient(t, http.StatusCreated, sampleProviderResponse())
	defer closeFn()

	err := runCreate(context.Background(), &CreateOptions{
		IO: ios, Client: clientFn, Org: "acme", Scope: baseScope(),
		ID: "openai", DisplayName: "OpenAI", Version: defaultVersion, Context: "/",
		Template: "openai", AuthType: "api-key", AccessMode: "deny_all",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mode, _ := accessControlMode(t, captured.body)
	if mode != "deny_all" {
		t.Errorf("accessControl.mode = %q, want deny_all", mode)
	}
}

// The spec pins version to ^v\d+\.\d+$. The old default "v1" cannot satisfy it and
// was accepted only because the server did not validate the pattern.
func TestDefaultVersionMatchesTheSpecPattern(t *testing.T) {
	if !versionRegex.MatchString(defaultVersion) {
		t.Errorf("defaultVersion %q does not match %s", defaultVersion, versionRegex)
	}
}

func TestGatewayNames_KeepsOnlyNonUUIDs(t *testing.T) {
	got := gatewayNames([]string{
		"11111111-1111-1111-1111-111111111111",
		"edge",
		" edge ",
		"  ",
		"inbound",
	})
	want := []string{"edge", "inbound"}
	if len(got) != len(want) {
		t.Fatalf("gatewayNames = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("gatewayNames[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseGateways_RejectsUnknownName(t *testing.T) {
	_, err := parseGateways([]string{"ghost"}, map[string]openapi_types.UUID{})
	if err == nil {
		t.Fatal("expected an error for an unresolvable gateway name")
	}
	if !strings.Contains(err.Error(), "unknown gateway") {
		t.Errorf("error = %v, want it to name the unknown gateway", err)
	}
}

func TestParseGateways_MixesNamesAndUUIDs(t *testing.T) {
	edge := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	explicit := "11111111-1111-1111-1111-111111111111"

	got, err := parseGateways([]string{explicit, "edge"}, map[string]openapi_types.UUID{"edge": edge})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].String() != explicit || got[1] != edge {
		t.Errorf("parseGateways = %v, want [%s %s]", got, explicit, edge)
	}
}

// One gateway named twice is one placement. Sending it twice drew a rejection from
// the server's "no two gateways may share an environment" check, which reads as a
// conflict between two gateways rather than as a repeated value.
func TestParseGateways_DropsDuplicatesKeepingFirstOccurrence(t *testing.T) {
	edge := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	other := "11111111-1111-1111-1111-111111111111"

	got, err := parseGateways(
		[]string{"edge", other, " edge ", edge.String(), other},
		map[string]openapi_types.UUID{"edge": edge},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "edge", " edge " and the bare UUID are all the same gateway.
	if len(got) != 2 || got[0] != edge || got[1].String() != other {
		t.Errorf("parseGateways = %v, want [%s %s]", got, edge, other)
	}
}

// newRoutingTestClient answers each path with its own body, so a test can exercise
// the gateway lookup followed by the create call. Any request to an undeclared path
// fails the test, which is how the "no extra round trip" case is asserted.
func newRoutingTestClient(
	t *testing.T, routes map[string]any,
) (func(context.Context) (*aigateway.ClientWithResponses, error), *capturedRequest, func()) {
	t.Helper()
	captured := &capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := routes[r.URL.Path]
		if !ok {
			t.Errorf("unexpected request to %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		status := http.StatusOK
		if r.Method == http.MethodPost {
			captured.called = true
			captured.method = r.Method
			captured.path = r.URL.Path
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read request body: %v", err)
			}
			captured.body = raw
			status = http.StatusCreated
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	client, err := aigateway.NewClientWithResponses(server.URL)
	if err != nil {
		server.Close()
		t.Fatalf("new client: %v", err)
	}
	return func(context.Context) (*aigateway.ClientWithResponses, error) { return client, nil }, captured, server.Close
}

func TestCreate_ResolvesGatewayNameToUUID(t *testing.T) {
	ios, _, _ := newTestIO(true)
	const edgeUUID = "22222222-2222-2222-2222-222222222222"
	clientFn, captured, closeFn := newRoutingTestClient(t, map[string]any{
		"/orgs/acme/gateways": aigateway.GatewayListResponse{
			Gateways: []aigateway.GatewayResponse{{Name: "edge", Uuid: edgeUUID}},
			Total:    1,
		},
		"/orgs/acme/llm-providers": sampleProviderResponse(),
	})
	defer closeFn()

	err := runCreate(context.Background(), &CreateOptions{
		IO: ios, Client: clientFn, Org: "acme", Scope: baseScope(),
		ID: "openai", DisplayName: "OpenAI", Version: defaultVersion, Context: "/",
		Template: "openai", AuthType: "api-key", Gateways: []string{"edge"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(captured.body), edgeUUID) {
		t.Errorf("create body does not carry the resolved gateway UUID; body=%s", captured.body)
	}
}

func TestCreate_UnknownGatewayNameFailsBeforeCreating(t *testing.T) {
	ios, _, _ := newTestIO(true)
	clientFn, captured, closeFn := newRoutingTestClient(t, map[string]any{
		"/orgs/acme/gateways": aigateway.GatewayListResponse{Gateways: []aigateway.GatewayResponse{}},
	})
	defer closeFn()

	err := runCreate(context.Background(), &CreateOptions{
		IO: ios, Client: clientFn, Org: "acme", Scope: baseScope(),
		ID: "openai", DisplayName: "OpenAI", Version: defaultVersion, Context: "/",
		Template: "openai", AuthType: "api-key", Gateways: []string{"ghost"},
	})
	if err == nil {
		t.Fatal("expected an error for an unresolvable gateway name")
	}
	if captured.called {
		t.Error("provider was created despite an unresolvable gateway name")
	}
}

// A UUID-only --gateways must not cost an extra round trip: the gateway list route
// is absent here, so reaching it fails the test.
func TestCreate_UUIDGatewaysSkipTheLookup(t *testing.T) {
	ios, _, _ := newTestIO(true)
	const explicit = "11111111-1111-1111-1111-111111111111"
	clientFn, captured, closeFn := newRoutingTestClient(t, map[string]any{
		"/orgs/acme/llm-providers": sampleProviderResponse(),
	})
	defer closeFn()

	err := runCreate(context.Background(), &CreateOptions{
		IO: ios, Client: clientFn, Org: "acme", Scope: baseScope(),
		ID: "openai", DisplayName: "OpenAI", Version: defaultVersion, Context: "/",
		Template: "openai", AuthType: "api-key", Gateways: []string{explicit},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(captured.body), explicit) {
		t.Errorf("create body missing the gateway UUID; body=%s", captured.body)
	}
}
