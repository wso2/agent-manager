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

package thundersvc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// indexOf returns the position of s in ss, or -1 if absent.
func indexOf(ss []string, s string) int {
	for i, v := range ss {
		if v == s {
			return i
		}
	}
	return -1
}

func TestAddGroupMemberEntries_SendsAgentType(t *testing.T) {
	var body GroupMembersRequest
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/groups/g1/members/add", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	c := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	err := c.AddGroupMemberEntries(context.Background(), "g1", []GroupMember{{ID: "a1", Type: "agent"}})

	require.NoError(t, err)
	require.Len(t, body.Members, 1)
	assert.Equal(t, "a1", body.Members[0].ID)
	assert.Equal(t, "agent", body.Members[0].Type)
}

func TestRemoveGroupMemberEntries_SendsAgentType(t *testing.T) {
	var body GroupMembersRequest
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/groups/g1/members/remove", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	c := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	err := c.RemoveGroupMemberEntries(context.Background(), "g1", []GroupMember{{ID: "a1", Type: "agent"}})

	require.NoError(t, err)
	require.Len(t, body.Members, 1)
	assert.Equal(t, "a1", body.Members[0].ID)
	assert.Equal(t, "agent", body.Members[0].Type)
}

func TestListGroupMemberEntries_ReturnsTypedEntries(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/groups/g1/members", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totalResults": 2,
			"members": []map[string]any{
				{"id": "a1", "type": "agent"},
				{"id": "u1", "type": "user"},
			},
		})
	})
	defer srv.Close()

	c := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	members, total, err := c.ListGroupMemberEntries(context.Background(), "g1", 0, 20)

	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, members, 2)
	assert.Equal(t, GroupMember{ID: "a1", Type: "agent"}, members[0])
	assert.Equal(t, GroupMember{ID: "u1", Type: "user"}, members[1])
}

func TestCanonicalMCPResourceIdentifier(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{name: "already canonical", raw: "https://gw.example.com/github/mcp", want: "https://gw.example.com/github/mcp"},
		{name: "scheme and host lowercased", raw: "HTTPS://GW.Example.com/x/mcp", want: "https://gw.example.com/x/mcp"},
		{name: "path case preserved", raw: "https://gw.example.com/GitHub/mcp", want: "https://gw.example.com/GitHub/mcp"},
		{name: "default https port dropped", raw: "https://gw.example.com:443/x/mcp", want: "https://gw.example.com/x/mcp"},
		{name: "default http port dropped", raw: "http://gw.example.com:80/x/mcp", want: "http://gw.example.com/x/mcp"},
		{name: "non-default port kept", raw: "https://gw.example.com:8443/x/mcp", want: "https://gw.example.com:8443/x/mcp"},
		{name: "trailing slash dropped", raw: "https://gw.example.com/x/mcp/", want: "https://gw.example.com/x/mcp"},
		{name: "scheme-less rejected", raw: "gw.example.com/x/mcp", wantErr: "scheme"},
		{name: "non-http scheme rejected", raw: "ftp://gw.example.com/x/mcp", wantErr: "http"},
		{name: "port-only authority rejected", raw: "https://:443/mcp", wantErr: "host"},
		{name: "port-only authority with non-default port rejected", raw: "https://:8443/mcp", wantErr: "host"},
		{name: "userinfo rejected", raw: "https://u:p@gw.example.com/x/mcp", wantErr: "userinfo"},
		{name: "query rejected", raw: "https://gw.example.com/x/mcp?a=1", wantErr: "query"},
		{name: "fragment rejected", raw: "https://gw.example.com/x/mcp#f", wantErr: "fragment"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := canonicalMCPResourceIdentifier(tc.raw)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestEnsureProxyResourceServer_CreatesRSWithAnchorResourceAndActions guards
// against a regression where actions were registered directly at the resource
// server root: ThunderID's derivePermission only prefixes an item's handle
// with its resource *parent* chain, never with the resource server's own
// handle — resource servers carry no handle of their own at all — so a root
// action's stored permission ended up as the bare action handle (e.g. "read")
// instead of "gh-proxy:read". Anchoring every action under an explicit
// resource whose own handle equals the proxy handle is what makes the
// composed permission match the "<proxy-handle>:<action>" scope string.
func TestEnsureProxyResourceServer_CreatesRSWithAnchorResourceAndActions(t *testing.T) {
	rsCreated, resCreated, actCreated := 0, 0, 0
	var createRSBody map[string]string
	var createResBody map[string]string
	var createActionBodies []map[string]string
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceServers": []any{}, "totalResults": 0})
		case r.Method == http.MethodGet && r.URL.Path == "/organization-units/tree/default":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "ou-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/resource-servers":
			rsCreated++
			_ = json.NewDecoder(r.Body).Decode(&createRSBody)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "rs-1", "handle": "gh-proxy", "identifier": "https://gw.example.com/github/mcp"})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources":
			_ = json.NewEncoder(w).Encode(map[string]any{"resources": []any{}, "totalResults": 0})
		case r.Method == http.MethodPost && r.URL.Path == "/resource-servers/rs-1/resources":
			resCreated++
			_ = json.NewDecoder(r.Body).Decode(&createResBody)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "res-1", "handle": createResBody["handle"]})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources/res-1/actions":
			_ = json.NewEncoder(w).Encode(map[string]any{"actions": []any{}, "totalResults": 0})
		case r.Method == http.MethodPost && r.URL.Path == "/resource-servers/rs-1/resources/res-1/actions":
			actCreated++
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			createActionBodies = append(createActionBodies, body)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": fmt.Sprintf("act-%d", actCreated), "handle": body["handle"], "permission": "gh-proxy:" + body["handle"]})
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	rsID, err := client.EnsureProxyResourceServer(context.Background(), "gh-proxy", "GitHub Proxy", "https://gw.example.com/github/mcp", []string{"read", "write"})
	assert.NoError(t, err)
	assert.Equal(t, "rs-1", rsID)
	assert.Equal(t, 1, rsCreated)
	assert.Equal(t, 1, resCreated)
	assert.Equal(t, 2, actCreated)
	assert.Equal(t, "gh-proxy", createRSBody["handle"], "RS handle must be the proxy handle — it prefixes derived permissions")
	assert.Equal(t, "https://gw.example.com/github/mcp", createRSBody["identifier"], "identifier must be the env invocation URI in RFC 8707 canonical form, not the bare handle")
	assert.Equal(t, ":", createRSBody["delimiter"])
	assert.Equal(t, "MCP", createRSBody["type"])
	assert.Equal(t, "ou-1", createRSBody["ouId"])
	assert.Equal(t, "gh-proxy", createResBody["handle"], "anchor resource's handle must equal the proxy handle so its permission composes to the proxy handle")
	assert.Len(t, createActionBodies, 2)
}

func TestEnsureProxyResourceServer_IdempotentSkipsExistingActions(t *testing.T) {
	actCreated := 0
	var createActionBodies []map[string]string
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			// Real Thunder resource servers have no "handle" field of their own
			// (only child resources/actions do) — this must be found by its
			// identifier alone, matching production.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resourceServers": []any{map[string]string{"id": "rs-1", "handle": "gh-proxy", "identifier": "https://gw.example.com/github/mcp"}},
				"total":           1,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/resource-servers":
			t.Fatalf("no RS create expected when the resource server already exists")
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resources":    []any{map[string]string{"id": "res-1", "handle": "gh-proxy"}},
				"totalResults": 1,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/resource-servers/rs-1/resources":
			t.Fatalf("no anchor resource create expected when it already exists")
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources/res-1/actions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"actions":      []any{map[string]string{"id": "act-1", "handle": "read"}},
				"totalResults": 1,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/resource-servers/rs-1/resources/res-1/actions":
			actCreated++
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			createActionBodies = append(createActionBodies, body)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "act-2", "handle": body["handle"]})
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	rsID, err := client.EnsureProxyResourceServer(context.Background(), "gh-proxy", "GitHub Proxy", "https://gw.example.com/github/mcp", []string{"read", "write"})
	assert.NoError(t, err)
	assert.Equal(t, "rs-1", rsID)
	require.Len(t, createActionBodies, 1, "only the missing action must be created")
	assert.Equal(t, "write", createActionBodies[0]["handle"])
}

func TestEnsureProxyResourceServer_ReconcilesDriftedIdentifier(t *testing.T) {
	// Legacy RS row from before identifiers switched to invocation URIs:
	// identifier still equals the bare handle, and (as in real Thunder) the row
	// has no handle field of its own. Must be found via the bare-identifier
	// fallback and PUT with the URI identifier, not recreated.
	rsUpdated := 0
	var updateBody map[string]string
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resourceServers": []any{map[string]string{"id": "rs-1", "name": "GitHub Proxy", "identifier": "gh-proxy"}},
				"totalResults":    1,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/organization-units/tree/default":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "ou-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/resource-servers":
			t.Fatalf("drifted identifier must be updated in place, not recreated")
		case r.Method == http.MethodPut && r.URL.Path == "/resource-servers/rs-1":
			rsUpdated++
			_ = json.NewDecoder(r.Body).Decode(&updateBody)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "rs-1"})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resources":    []any{map[string]string{"id": "res-1", "handle": "gh-proxy"}},
				"totalResults": 1,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources/res-1/actions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"actions":      []any{map[string]string{"id": "act-1", "handle": "read"}},
				"totalResults": 1,
			})
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	rsID, err := client.EnsureProxyResourceServer(context.Background(), "gh-proxy", "GitHub Proxy", "https://gw.example.com/github/mcp", []string{"read"})
	assert.NoError(t, err)
	assert.Equal(t, "rs-1", rsID)
	assert.Equal(t, 1, rsUpdated)
	assert.Equal(t, "https://gw.example.com/github/mcp", updateBody["identifier"])
	assert.Equal(t, "gh-proxy", updateBody["handle"], "handle must never change — permissions derive from it")
	assert.Equal(t, ":", updateBody["delimiter"])
	assert.Equal(t, "MCP", updateBody["type"])
}

func TestEnsureProxyResourceServer_DistinctProxiesDoNotSerialize(t *testing.T) {
	// proxy-a's ensure is held mid-flight at the list call; proxy-b's ensure must
	// still complete — only same-handle ensures may serialize.
	firstListArrived := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
	var listCalls atomic.Int32
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			if listCalls.Add(1) == 1 {
				close(firstListArrived)
				<-releaseFirst
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceServers": []any{}, "totalResults": 0})
		case r.Method == http.MethodGet && r.URL.Path == "/organization-units/tree/default":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "ou-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/resource-servers":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "rs-" + body["handle"]})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/resources"):
			_ = json.NewEncoder(w).Encode(map[string]any{"resources": []any{}, "totalResults": 0})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/resources"):
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "res-" + body["handle"], "handle": body["handle"]})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/actions"):
			_ = json.NewEncoder(w).Encode(map[string]any{"actions": []any{}, "totalResults": 0})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/actions"):
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "act-1"})
		default:
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	defer release()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")

	firstErr := make(chan error, 1)
	go func() {
		_, err := client.EnsureProxyResourceServer(context.Background(), "proxy-a", "Proxy A", "https://gw.example.com/a/mcp", []string{"read"})
		firstErr <- err
	}()
	<-firstListArrived

	secondErr := make(chan error, 1)
	go func() {
		_, err := client.EnsureProxyResourceServer(context.Background(), "proxy-b", "Proxy B", "https://gw.example.com/b/mcp", []string{"read"})
		secondErr <- err
	}()
	select {
	case err := <-secondErr:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("ensure for proxy-b blocked behind proxy-a's in-flight ensure")
	}

	release()
	require.NoError(t, <-firstErr)
}

func TestEnsureProxyResourceServer_ConcurrentSameHandleCreatesOnce(t *testing.T) {
	// The per-handle TOCTOU guard: concurrent ensures for one proxy must yield a
	// single RS create; late callers find the row the first caller created.
	var mu sync.Mutex
	created := 0
	exists := false
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			mu.Lock()
			servers := []any{}
			if exists {
				servers = append(servers, map[string]string{"id": "rs-1", "handle": "gh-proxy", "identifier": "https://gw.example.com/github/mcp"})
			}
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceServers": servers, "totalResults": len(servers)})
		case r.Method == http.MethodGet && r.URL.Path == "/organization-units/tree/default":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "ou-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/resource-servers":
			mu.Lock()
			created++
			exists = true
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "rs-1"})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resources":    []any{map[string]string{"id": "res-1", "handle": "gh-proxy"}},
				"totalResults": 1,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources/res-1/actions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"actions":      []any{map[string]string{"id": "act-1", "handle": "read"}},
				"totalResults": 1,
			})
		default:
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")

	var wg sync.WaitGroup
	errs := make([]error, 4)
	rsIDs := make([]string, 4)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rsIDs[i], errs[i] = client.EnsureProxyResourceServer(context.Background(), "gh-proxy", "GitHub Proxy", "https://gw.example.com/github/mcp", []string{"read"})
		}()
	}
	wg.Wait()

	for i := range errs {
		require.NoError(t, errs[i])
		assert.Equal(t, "rs-1", rsIDs[i])
	}
	assert.Equal(t, 1, created, "concurrent same-handle ensures must create exactly one resource server")
}

// TestFindProxyResourceServer_IdentifierFallbackOnlyMatchesHandleLessRows
// covers the Delete* paths, which only ever have a bare proxyHandle to search
// with (no computed identifier) — so a foreign RS whose identifier happens to
// equal the proxy handle must not be matched (delete would remove the wrong
// resource server); only a legacy handle-less row may match by identifier.
func TestFindProxyResourceServer_IdentifierFallbackOnlyMatchesHandleLessRows(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/resource-servers", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceServers": []any{
				map[string]string{"id": "rs-foreign", "handle": "billing-api", "identifier": "gh-proxy"},
				map[string]string{"id": "rs-legacy", "identifier": "gh-proxy"},
			},
			"totalResults": 2,
		})
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret").(*thunderClient)

	// No identifier known (mirrors DeleteProxyResourceServerAction/DeleteProxyResourceServer).
	rs, err := client.findProxyResourceServer(context.Background(), "test-system-token", "gh-proxy", "")

	require.NoError(t, err)
	require.NotNil(t, rs)
	assert.Equal(t, "rs-legacy", rs.ID, "identifier fallback must skip rows that carry a different handle")
}

// TestFindProxyResourceServer_MatchesByExactIdentifierRegardlessOfHandle covers
// the Ensure path: EnsureProxyResourceServer creates a proxy's RS with a
// computed invocation-URI identifier, but Thunder resource servers carry no
// handle of their own — so without an exact-identifier match, every later
// call would fail to find the RS it just created, retry the create, and get
// rejected with a name conflict (RES-1004) forever after.
func TestFindProxyResourceServer_MatchesByExactIdentifierRegardlessOfHandle(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/resource-servers", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resourceServers": []any{
				map[string]string{"id": "rs-1", "identifier": "gw.example.com/github/mcp"},
			},
			"totalResults": 1,
		})
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret").(*thunderClient)

	rs, err := client.findProxyResourceServer(context.Background(), "test-system-token", "gh-proxy", "gw.example.com/github/mcp")

	require.NoError(t, err)
	require.NotNil(t, rs, "a resource server created with this identifier must be found by it, even with no handle field to match on")
	assert.Equal(t, "rs-1", rs.ID)
}

func TestEnsureProxyResourceServer_RejectsOverlongInputs(t *testing.T) {
	srv := newTestThunderServer(t, func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("no Thunder calls expected for over-long input, got %s %s", r.Method, r.URL.Path)
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")

	_, err := client.EnsureProxyResourceServer(context.Background(), strings.Repeat("h", 101), "Too Long", "https://gw.example.com/x/mcp", []string{"read"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "100", "over-long handle error should state the 100-character Thunder limit")

	_, err = client.EnsureProxyResourceServer(context.Background(), "gh-proxy", "GitHub Proxy", "https://gw.example.com/x/mcp", []string{strings.Repeat("a", 101)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "100", "over-long action error should state the 100-character Thunder limit")
}

func TestEnsureProxyResourceServer_RejectsNonCanonicalIdentifierBeforeAnyCall(t *testing.T) {
	srv := newTestThunderServer(t, func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("no Thunder calls expected for a non-canonical identifier, got %s %s", r.Method, r.URL.Path)
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")

	_, err := client.EnsureProxyResourceServer(context.Background(), "gh-proxy", "GitHub Proxy", "gw.example.com/github/mcp", []string{"read"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "scheme", "a scheme-less identifier must be rejected as non-absolute")
}

func TestEnsureProxyResourceServer_CanonicalizesIdentifierOnCreate(t *testing.T) {
	// The uppercase host and default port are normalized at the Thunder boundary,
	// so the identifier Thunder mints aud claims from is RFC 8707 canonical.
	var createRSBody map[string]string
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceServers": []any{}, "total": 0})
		case r.Method == http.MethodGet && r.URL.Path == "/organization-units/tree/default":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "ou-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/resource-servers":
			_ = json.NewDecoder(r.Body).Decode(&createRSBody)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "rs-1"})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources":
			_ = json.NewEncoder(w).Encode(map[string]any{"resources": []any{}, "totalResults": 0})
		case r.Method == http.MethodPost && r.URL.Path == "/resource-servers/rs-1/resources":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "res-1", "handle": "gh-proxy"})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources/res-1/actions":
			_ = json.NewEncoder(w).Encode(map[string]any{"actions": []any{}, "totalResults": 0})
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")

	_, err := client.EnsureProxyResourceServer(context.Background(), "gh-proxy", "GitHub Proxy", "HTTPS://GW.Example.com:443/github/mcp/", nil)

	require.NoError(t, err)
	assert.Equal(t, "https://gw.example.com/github/mcp", createRSBody["identifier"])
}

func TestEnsureProxyResourceServer_IdentifierLimitIsTheIdentifierColumn(t *testing.T) {
	// The identifier column is VARCHAR(2048); it was previously capped at the
	// 100-character handle limit, rejecting legitimate long URIs.
	longIdentifier := "https://gw.example.com/" + strings.Repeat("a", 123) + "/mcp"
	require.Len(t, longIdentifier, 150)

	var createRSBody map[string]string
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceServers": []any{}, "total": 0})
		case r.Method == http.MethodGet && r.URL.Path == "/organization-units/tree/default":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "ou-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/resource-servers":
			_ = json.NewDecoder(r.Body).Decode(&createRSBody)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "rs-1"})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources":
			_ = json.NewEncoder(w).Encode(map[string]any{"resources": []any{}, "totalResults": 0})
		case r.Method == http.MethodPost && r.URL.Path == "/resource-servers/rs-1/resources":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "res-1", "handle": "gh-proxy"})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources/res-1/actions":
			_ = json.NewEncoder(w).Encode(map[string]any{"actions": []any{}, "totalResults": 0})
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")

	_, err := client.EnsureProxyResourceServer(context.Background(), "gh-proxy", "GitHub Proxy", longIdentifier, nil)

	require.NoError(t, err)
	assert.Equal(t, longIdentifier, createRSBody["identifier"])
}

func TestEnsureProxyResourceServer_RejectsIdentifierOverColumnLimit(t *testing.T) {
	srv := newTestThunderServer(t, func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("no Thunder calls expected for an over-long identifier, got %s %s", r.Method, r.URL.Path)
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	overLimit := "https://gw.example.com/" + strings.Repeat("a", 2048)

	_, err := client.EnsureProxyResourceServer(context.Background(), "gh-proxy", "GitHub Proxy", overLimit, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "2048", "over-long identifier error should state the 2048-character limit")
}

func TestDeleteProxyResourceServer_DeletesActionsThenResourceThenRS(t *testing.T) {
	var calls []string
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		// Delete only has a bare proxyHandle to search with (no computed
		// identifier), so — as in real Thunder, which has no resource-server
		// handle field — this can only find a legacy bare-identifier row.
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceServers": []any{map[string]string{"id": "rs-1", "identifier": "gh-proxy"}}, "totalResults": 1})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources":
			_ = json.NewEncoder(w).Encode(map[string]any{"resources": []any{map[string]string{"id": "res-1", "handle": "gh-proxy"}}, "totalResults": 1})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources/res-1/actions":
			_ = json.NewEncoder(w).Encode(map[string]any{"actions": []any{map[string]string{"id": "act-1", "handle": "read"}}, "totalResults": 1})
		case r.Method == http.MethodDelete && r.URL.Path == "/resource-servers/rs-1/resources/res-1/actions/act-1":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && r.URL.Path == "/resource-servers/rs-1/resources/res-1":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && r.URL.Path == "/resource-servers/rs-1":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	assert.NoError(t, client.DeleteProxyResourceServer(context.Background(), "gh-proxy"))
	// deletion must go bottom-up (Thunder 400-blocks a delete while children exist)
	actionIdx := indexOf(calls, "DELETE /resource-servers/rs-1/resources/res-1/actions/act-1")
	resourceIdx := indexOf(calls, "DELETE /resource-servers/rs-1/resources/res-1")
	rsIdx := indexOf(calls, "DELETE /resource-servers/rs-1")
	assert.Less(t, actionIdx, resourceIdx)
	assert.Less(t, resourceIdx, rsIdx)
}

// A resource server registered with a URL identifier carries no handle, as in real Thunder: the delete by bare handle
// finds nothing and reports success, while the delete by identifier finds and removes it.
func TestDeleteProxyResourceServerByIdentifier_FindsAURLRegisteredRS(t *testing.T) {
	const url = "https://gw.example.com/p1/weather"
	var deleted []string
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceServers": []any{map[string]string{"id": "rs-1", "identifier": url}}, "totalResults": 1})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources":
			_ = json.NewEncoder(w).Encode(map[string]any{"resources": []any{map[string]string{"id": "res-1", "handle": "weather"}}, "totalResults": 1})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources/res-1/actions":
			_ = json.NewEncoder(w).Encode(map[string]any{"actions": []any{}, "totalResults": 0})
		case r.Method == http.MethodDelete:
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")

	require.NoError(t, client.DeleteProxyResourceServer(context.Background(), "weather"))
	assert.Empty(t, deleted, "by bare handle the URL-registered resource server is not found")

	require.NoError(t, client.DeleteProxyResourceServerByIdentifier(context.Background(), "weather", url))
	assert.Equal(t, []string{"/resource-servers/rs-1/resources/res-1", "/resource-servers/rs-1"}, deleted)
}

// A rename finds the resource server by its old URL identifier, which has no handle in Thunder, and rewrites the
// identifier in place, so its resources, actions and grants carry over.
func TestRenameProxyResourceServerIdentifier_RewritesInPlace(t *testing.T) {
	const from, to = "https://gw.example.com/p1/old", "https://gw.example.com/p1/new"
	var put map[string]any
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceServers": []any{map[string]string{"id": "rs-1", "name": "Weather", "identifier": from}}, "totalResults": 1})
		case r.Method == http.MethodGet && r.URL.Path == "/organization-units/tree/default":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "ou-1"})
		case r.Method == http.MethodPut && r.URL.Path == "/resource-servers/rs-1":
			_ = json.NewDecoder(r.Body).Decode(&put)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"rs-1"}`))
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")

	require.NoError(t, client.RenameProxyResourceServerIdentifier(context.Background(), "weather", from, to))
	require.NotNil(t, put, "the resource server was updated in place")
	assert.Equal(t, to, put["identifier"])
}

// TestDeleteProxyResourceServerAction_DeletesFromAnchorResource guards against
// a regression where action deletion targeted the resource-server root
// (/resource-servers/{id}/actions/{actionId}) instead of the anchor resource's
// nested action path, which is where EnsureProxyResourceServer now creates them.
func TestDeleteProxyResourceServerAction_DeletesFromAnchorResource(t *testing.T) {
	var deleted string
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceServers": []any{map[string]string{"id": "rs-1", "identifier": "gh-proxy"}}, "totalResults": 1})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources":
			_ = json.NewEncoder(w).Encode(map[string]any{"resources": []any{map[string]string{"id": "res-1", "handle": "gh-proxy"}}, "totalResults": 1})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources/res-1/actions":
			_ = json.NewEncoder(w).Encode(map[string]any{"actions": []any{map[string]string{"id": "act-1", "handle": "read"}}, "totalResults": 1})
		case r.Method == http.MethodDelete && r.URL.Path == "/resource-servers/rs-1/resources/res-1/actions/act-1":
			deleted = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	rsID, err := client.DeleteProxyResourceServerAction(context.Background(), "gh-proxy", "read")
	assert.NoError(t, err)
	assert.Equal(t, "rs-1", rsID)
	assert.Equal(t, "/resource-servers/rs-1/resources/res-1/actions/act-1", deleted)
}

// TestDeleteProxyResourceServerAction_NoOpWhenActionAlreadyGone covers a
// double-delete (e.g. a retried request): the anchor resource exists but the
// target action handle is no longer in its actions list. No DELETE call
// should be attempted and no error returned.
func TestDeleteProxyResourceServerAction_NoOpWhenActionAlreadyGone(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceServers": []any{map[string]string{"id": "rs-1", "identifier": "gh-proxy"}}, "totalResults": 1})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources":
			_ = json.NewEncoder(w).Encode(map[string]any{"resources": []any{map[string]string{"id": "res-1", "handle": "gh-proxy"}}, "totalResults": 1})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources/res-1/actions":
			_ = json.NewEncoder(w).Encode(map[string]any{"actions": []any{}, "totalResults": 0})
		default:
			t.Fatalf("unexpected call %s %s, no delete should be attempted for an already-gone action", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	rsID, err := client.DeleteProxyResourceServerAction(context.Background(), "gh-proxy", "read")
	assert.NoError(t, err)
	assert.Equal(t, "rs-1", rsID)
}

// TestDeleteProxyResourceServerAction_NoOpWhenAnchorResourceMissing covers a
// resource server that exists (e.g. left over from a partial provisioning
// run) but never got its anchor resource created. Deleting an action from it
// must be a no-op, not an error.
func TestDeleteProxyResourceServerAction_NoOpWhenAnchorResourceMissing(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceServers": []any{map[string]string{"id": "rs-1", "identifier": "gh-proxy"}}, "totalResults": 1})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/resources":
			_ = json.NewEncoder(w).Encode(map[string]any{"resources": []any{}, "totalResults": 0})
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	rsID, err := client.DeleteProxyResourceServerAction(context.Background(), "gh-proxy", "read")
	assert.NoError(t, err)
	assert.Equal(t, "rs-1", rsID)
}

// TestDeleteProxyResourceServerAction_NoOpWhenResourceServerMissing and
// TestDeleteProxyResourceServer_NoOpWhenResourceServerMissing cover a proxy
// that was never provisioned (or already fully torn down): both delete paths
// must be silent no-ops, not errors — a caller cleaning up after a failed
// partial create should not be blocked by "resource server not found".
func TestDeleteProxyResourceServerAction_NoOpWhenResourceServerMissing(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceServers": []any{}, "totalResults": 0})
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	rsID, err := client.DeleteProxyResourceServerAction(context.Background(), "gh-proxy", "read")
	assert.NoError(t, err)
	assert.Equal(t, "", rsID)
}

func TestDeleteProxyResourceServer_NoOpWhenResourceServerMissing(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceServers": []any{}, "totalResults": 0})
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	assert.NoError(t, client.DeleteProxyResourceServer(context.Background(), "gh-proxy"))
}

// TestListAMPPermissions_DescendsIntoAnchorResourceChildren guards against a
// regression where the amp resource server's permission tree came back empty.
// GET /resource-servers/{id}/resources without parentId only returns
// top-level resources — every real AMP permission resource (org, profile,
// project, ...) lives one level below the "amp" anchor resource (see
// amp-thunder-bootstrap.yaml), so ListAMPPermissions must also fetch each
// top-level resource's children via parentId or the permission list is empty.
func TestListAMPPermissions_DescendsIntoAnchorResourceChildren(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resourceServers": []any{map[string]string{"id": "amp-rs", "identifier": "urn:wso2:amp"}},
				"totalResults":    1,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/amp-rs/resources" && r.URL.Query().Get("parentId") == "":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resources":    []any{map[string]string{"id": "anchor-1", "handle": "amp", "name": "Agent Manager"}},
				"totalResults": 1,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/amp-rs/resources" && r.URL.Query().Get("parentId") == "anchor-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resources":    []any{map[string]string{"id": "child-1", "handle": "org", "name": "Organization"}},
				"totalResults": 1,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/amp-rs/resources/anchor-1/actions":
			_ = json.NewEncoder(w).Encode(map[string]any{"actions": []any{}, "totalResults": 0})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/amp-rs/resources/child-1/actions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"actions": []any{map[string]string{
					"id": "act-1", "handle": "view", "name": "View", "permission": "amp:org:view",
				}},
				"totalResults": 1,
			})
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	perms, rsID, err := client.ListAMPPermissions(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "amp-rs", rsID)
	require.Len(t, perms, 1, "the anchor resource's own child permission must be included, not just the (action-less) anchor itself")
	assert.Equal(t, "amp:org:view", perms[0].Name)
	assert.Equal(t, "Organization", perms[0].ResourceName)
	assert.Equal(t, "View", perms[0].ActionName)
}

// TestListAMPPermissions_PaginatesMoreThan20ChildrenUnderAnchor mirrors the
// real amp resource server, which has more than one page (20) of children
// (org, profile, project, agent, ...) — proving the fix walks every page via
// parentId instead of only the first.
func TestListAMPPermissions_PaginatesMoreThan20ChildrenUnderAnchor(t *testing.T) {
	const childCount = 23 // matches the live amp resource server's actual child count
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceServers": []any{map[string]string{"id": "amp-rs", "identifier": "urn:wso2:amp"}}, "totalResults": 1})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/amp-rs/resources" && r.URL.Query().Get("parentId") == "":
			_ = json.NewEncoder(w).Encode(map[string]any{"resources": []any{map[string]string{"id": "anchor-1", "handle": "amp"}}, "totalResults": 1})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/amp-rs/resources" && r.URL.Query().Get("parentId") == "anchor-1":
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			var page []any
			for i := offset; i < offset+limit && i < childCount; i++ {
				page = append(page, map[string]string{"id": fmt.Sprintf("child-%d", i), "handle": fmt.Sprintf("res-%d", i), "name": fmt.Sprintf("Resource %d", i)})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"resources": page, "totalResults": childCount})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/amp-rs/resources/anchor-1/actions":
			_ = json.NewEncoder(w).Encode(map[string]any{"actions": []any{}, "totalResults": 0})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/resource-servers/amp-rs/resources/child-"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/resource-servers/amp-rs/resources/"), "/actions")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"actions":      []any{map[string]string{"id": id + "-act", "handle": "view", "name": "View", "permission": "amp:res-x:view"}},
				"totalResults": 1,
			})
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	perms, rsID, err := client.ListAMPPermissions(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "amp-rs", rsID)
	assert.Len(t, perms, childCount, "every child beyond the first page of 20 must still be walked for its actions")
}

// TestListAMPPermissions_ReturnsEmptySliceNotNilWhenAnchorHasNoChildren
// guards against a nil slice reaching JSON encoding as "permissions": null
// instead of "permissions": [] when the amp resource server exists but has no
// children yet (e.g. a fresh or partially-bootstrapped install) — a null
// where callers expect an array is a class of bug on its own.
func TestListAMPPermissions_ReturnsEmptySliceNotNilWhenAnchorHasNoChildren(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceServers": []any{map[string]string{"id": "amp-rs", "identifier": "urn:wso2:amp"}}, "totalResults": 1})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/amp-rs/resources":
			_ = json.NewEncoder(w).Encode(map[string]any{"resources": []any{}, "totalResults": 0})
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	perms, rsID, err := client.ListAMPPermissions(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "amp-rs", rsID)
	require.NotNil(t, perms, `permissions must serialize as "[]", not "null"`)
	assert.Empty(t, perms)
}

// TestListAMPPermissions_ResourceServerNotFound covers a deployment where the
// amp resource server was never bootstrapped: the catalog must come back
// empty (permissions can still be managed without it) rather than erroring.
func TestListAMPPermissions_ResourceServerNotFound(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceServers": []any{}, "totalResults": 0})
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	perms, rsID, err := client.ListAMPPermissions(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "", rsID)
	assert.Empty(t, perms)
}

// TestFindResourceServerID_PaginatesBeyondFirstPage proves the "total" vs
// "totalResults" JSON tag fix: before it, the pagination loop always read a
// zero total (Thunder's field is "totalResults", not "total"), so it stopped
// after the first page and any resource server sitting on page 2+ was
// silently never found. This puts the target on page 2 of 2.
func TestFindResourceServerID_PaginatesBeyondFirstPage(t *testing.T) {
	const totalCount = 25 // > one page (20); "urn:wso2:amp" is at index 22, on page 2
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			var page []any
			for i := offset; i < offset+limit && i < totalCount; i++ {
				identifier := fmt.Sprintf("proxy-%d", i)
				if i == totalCount-3 {
					identifier = "urn:wso2:amp"
				}
				page = append(page, map[string]string{"id": fmt.Sprintf("rs-%d", i), "identifier": identifier})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceServers": page, "totalResults": totalCount})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-22/resources":
			_ = json.NewEncoder(w).Encode(map[string]any{"resources": []any{}, "totalResults": 0})
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	perms, rsID, err := client.ListAMPPermissions(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "rs-22", rsID, "urn:wso2:amp sits on page 2 of the resource-server listing and must still be found")
	assert.Empty(t, perms)
}

// TestGetAgentRoleAssignments_ReturnsAgentEntriesAndResolvedGroups proves the
// agent-identity read path keeps agent assignees (as raw entries) and resolves
// group assignees, unlike the user-store GetRoleAssignments which drops agents.
func TestGetAgentRoleAssignments_ReturnsAgentEntriesAndResolvedGroups(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/roles/r1/assignments":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"assignments": []map[string]string{
					{"id": "a1", "type": "agent"},
					{"id": "g1", "type": "group"},
					{"id": "a2", "type": "agent"},
					{"id": "u1", "type": "user"},
				},
			})
		case "/groups/g1":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "g1", "name": "readers"})
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()

	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	assignments, err := client.GetAgentRoleAssignments(context.Background(), "r1")

	require.NoError(t, err)
	assert.Equal(t, []AssignmentEntry{{ID: "a1", Type: "agent"}, {ID: "a2", Type: "agent"}}, assignments.Agents)
	require.Len(t, assignments.Groups, 1)
	assert.Equal(t, "readers", assignments.Groups[0].Name)
}

// TestGetAgentRoleAssignments_SkipsDeletedGroup proves a group assignee that no
// longer exists in Thunder is skipped rather than failing the whole listing.
func TestGetAgentRoleAssignments_SkipsDeletedGroup(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/roles/r1/assignments":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"assignments": []map[string]string{
					{"id": "g-gone", "type": "group"},
					{"id": "a1", "type": "agent"},
				},
			})
		case "/groups/g-gone":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()

	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	assignments, err := client.GetAgentRoleAssignments(context.Background(), "r1")

	require.NoError(t, err)
	assert.Empty(t, assignments.Groups)
	assert.Equal(t, []AssignmentEntry{{ID: "a1", Type: "agent"}}, assignments.Agents)
}

// TestListRoles_OUFiltered_ExcludesNativeAdministrator proves the OU-scoped role
// listing hides Thunder's native Administrator role — it carries the built-in
// "system" scope and must never surface as an assignable agent role — and that
// the exclusion happens before client-side pagination so offset/limit/total stay
// consistent.
func TestListRoles_OUFiltered_ExcludesNativeAdministrator(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/roles", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totalResults": 3,
			"roles": []map[string]any{
				{"id": "r-admin", "ouId": "ou-1", "name": NativeAdministratorRoleName},
				{"id": "r-readers", "ouId": "ou-1", "name": "readers"},
				{"id": "r-elsewhere", "ouId": "ou-2", "name": "writers"},
			},
		})
	})
	defer srv.Close()

	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	roles, total, err := client.ListRoles(context.Background(), "ou-1", 0, 20)

	require.NoError(t, err)
	assert.Equal(t, 1, total, "Administrator must not count toward the OU total")
	require.Len(t, roles, 1)
	assert.Equal(t, "readers", roles[0].Name)
}

// TestListRoles_Unfiltered_KeepsNativeAdministrator pins the ouID="" contract:
// rolesForAssignee and the scope-cleanup sweep walk every role in the instance
// and must still see the native Administrator role (e.g. to report an agent
// already mis-assigned to it).
func TestListRoles_Unfiltered_KeepsNativeAdministrator(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totalResults": 2,
			"roles": []map[string]any{
				{"id": "r-admin", "ouId": "ou-1", "name": NativeAdministratorRoleName},
				{"id": "r-readers", "ouId": "ou-1", "name": "readers"},
			},
		})
	})
	defer srv.Close()

	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	roles, total, err := client.ListRoles(context.Background(), "", 0, 20)

	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, roles, 2)
}

// TestListRoles_OUFiltered_ExcludesAMPSystemClient mirrors
// TestListRoles_OUFiltered_ExcludesNativeAdministrator: env-Thunder's own
// bootstrap-seeded system-client role must be hidden from agent-identity role
// listings the same way as the native Administrator role.
func TestListRoles_OUFiltered_ExcludesAMPSystemClient(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/roles", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totalResults": 3,
			"roles": []map[string]any{
				{"id": "r-sysclient", "ouId": "ou-1", "name": AMPSystemClientRoleName},
				{"id": "r-readers", "ouId": "ou-1", "name": "readers"},
				{"id": "r-elsewhere", "ouId": "ou-2", "name": "writers"},
			},
		})
	})
	defer srv.Close()

	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	roles, total, err := client.ListRoles(context.Background(), "ou-1", 0, 20)

	require.NoError(t, err)
	assert.Equal(t, 1, total, "AMP System Client Thunder Admin must not count toward the OU total")
	require.Len(t, roles, 1)
	assert.Equal(t, "readers", roles[0].Name)
}

// ouGroupServer serves Thunder's OU-scoped group endpoint from a fixed group
// list, honouring the offset/limit query so the client's own pagination is
// exercised against realistic paging. reqs counts the group fetches.
func ouGroupServer(t *testing.T, ouID string, groups []map[string]any, reqs *int) *httptest.Server {
	t.Helper()
	return newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/organization-units/"+ouID+"/groups", r.URL.Path)
		if reqs != nil {
			*reqs++
		}
		offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
		require.NoError(t, err)
		limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
		require.NoError(t, err)

		page := []map[string]any{}
		if offset < len(groups) {
			end := offset + limit
			if end > len(groups) {
				end = len(groups)
			}
			page = groups[offset:end]
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"totalResults": len(groups), "groups": page})
	})
}

// TestListGroupsByOUId_ExcludesNativeAdministrators proves the OU-scoped group
// listing hides Thunder's native Administrators group. Its members inherit the
// native Administrator role and with it the built-in "system" scope, so the
// group must never surface as a joinable group, and it must not count toward
// the total either.
func TestListGroupsByOUId_ExcludesNativeAdministrators(t *testing.T) {
	srv := ouGroupServer(t, "ou-1", []map[string]any{
		{"id": "g-admin", "name": NativeAdministratorsGroupName},
		{"id": "g-readers", "name": "readers"},
		{"id": "g-writers", "name": "writers"},
	}, nil)
	defer srv.Close()

	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	groups, total, err := client.ListGroupsByOUId(context.Background(), "ou-1", 0, 20)

	require.NoError(t, err)
	assert.Equal(t, 2, total, "Administrators must not count toward the OU total")
	require.Len(t, groups, 2)
	assert.Equal(t, "readers", groups[0].Name)
	assert.Equal(t, "writers", groups[1].Name)
}

// TestListGroupsByOUId_PaginatesAfterExclusion pins the reason the exclusion
// lives below pagination: filtering a server-side page after the fact returned
// short pages and an inflated total. Walking limit=1 pages must yield every
// visible group exactly once and never a gap.
func TestListGroupsByOUId_PaginatesAfterExclusion(t *testing.T) {
	srv := ouGroupServer(t, "ou-1", []map[string]any{
		{"id": "g-admin", "name": NativeAdministratorsGroupName},
		{"id": "g-a", "name": "a"},
		{"id": "g-b", "name": "b"},
		{"id": "g-c", "name": "c"},
	}, nil)
	defer srv.Close()

	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")

	var names []string
	for offset := 0; ; offset++ {
		page, total, err := client.ListGroupsByOUId(context.Background(), "ou-1", offset, 1)
		require.NoError(t, err)
		assert.Equal(t, 3, total)
		if len(page) == 0 {
			break
		}
		require.Len(t, page, 1, "every page below the total must be full")
		names = append(names, page[0].Name)
	}
	assert.Equal(t, []string{"a", "b", "c"}, names)
}

// TestListGroupsByOUId_PagesPastFetchSize proves the all-pages fetch keeps
// walking past a single Thunder page, so a native group sitting beyond the
// first page is still excluded.
func TestListGroupsByOUId_PagesPastFetchSize(t *testing.T) {
	all := make([]map[string]any, 0, 150)
	for i := 0; i < 149; i++ {
		all = append(all, map[string]any{"id": fmt.Sprintf("g-%d", i), "name": fmt.Sprintf("group-%d", i)})
	}
	all = append(all, map[string]any{"id": "g-admin", "name": NativeAdministratorsGroupName})

	reqs := 0
	srv := ouGroupServer(t, "ou-1", all, &reqs)
	defer srv.Close()

	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	groups, total, err := client.ListGroupsByOUId(context.Background(), "ou-1", 0, 200)

	require.NoError(t, err)
	assert.Equal(t, 149, total)
	assert.Len(t, groups, 149)
	assert.Greater(t, reqs, 1, "must fetch beyond the first page")
	for _, g := range groups {
		assert.NotEqual(t, NativeAdministratorsGroupName, g.Name)
	}
}

// TestGetAgentGroups_ExcludesNativeAdministrators proves an agent mis-added to
// the native Administrators group is never reported as a member of it, so the
// group cannot leak through the agent's group list either.
func TestGetAgentGroups_ExcludesNativeAdministrators(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/organization-units/ou-1/groups":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"totalResults": 2,
				"groups": []map[string]any{
					{"id": "g-admin", "name": NativeAdministratorsGroupName},
					{"id": "g-readers", "name": "readers"},
				},
			})
		case "/groups/g-admin/members", "/groups/g-readers/members":
			// The agent is a member of both, so only the exclusion can keep the
			// native group out of the result.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"totalResults": 1,
				"members":      []map[string]any{{"id": "agent-1", "type": "agent"}},
			})
		default:
			t.Errorf("unexpected path %q — the native group must not be probed", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	groups, err := client.GetAgentGroups(context.Background(), "ou-1", "agent-1")

	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "readers", groups[0].Name)
}

// TestListGroups_Unfiltered_KeepsNativeAdministrators pins the ouID="" contract,
// mirroring ListRoles: the raw instance-wide sweep must still see the native
// group. The ouID-scoped branch of the same method does exclude it.
func TestListGroups_Unfiltered_KeepsNativeAdministrators(t *testing.T) {
	payload := map[string]any{
		"totalResults": 2,
		"groups": []map[string]any{
			{"id": "g-admin", "ouId": "ou-1", "name": NativeAdministratorsGroupName},
			{"id": "g-readers", "ouId": "ou-1", "name": "readers"},
		},
	}
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/groups", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})
	defer srv.Close()

	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")

	groups, total, err := client.ListGroups(context.Background(), "", 0, 20)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, groups, 2)

	scoped, scopedTotal, err := client.ListGroups(context.Background(), "ou-1", 0, 20)
	require.NoError(t, err)
	assert.Equal(t, 1, scopedTotal, "the OU-scoped branch must exclude the native group")
	require.Len(t, scoped, 1)
	assert.Equal(t, "readers", scoped[0].Name)
}

const testOUB = "01900000-0000-7000-8000-0000000000b2"

func TestGetOUHandleByID_ReturnsHandle(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/organization-units/"+testOUB, r.URL.Path)
		assert.Equal(t, "Bearer test-system-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": testOUB, "handle": "org-b", "name": "Org B"})
	})
	defer srv.Close()

	c := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	handle, err := c.GetOUHandleByID(context.Background(), testOUB)

	require.NoError(t, err)
	assert.Equal(t, "org-b", handle)
}

func TestGetOUHandleByID_NotFound(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	c := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	_, err := c.GetOUHandleByID(context.Background(), "01900000-0000-7000-8000-0000000000ff")

	require.Error(t, err)
	assert.True(t, IsNotFound(err), "a missing OU is reported as not found: %v", err)
}

func TestGetOUHandleByID_RejectsMismatchedOrEmptyBody(t *testing.T) {
	for name, body := range map[string]map[string]any{
		"no handle":  {"id": testOUB},
		"another ou": {"id": "01900000-0000-7000-8000-0000000000c3", "handle": "org-c"},
	} {
		t.Run(name, func(t *testing.T) {
			srv := newTestThunderServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(body)
			})
			defer srv.Close()

			c := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
			_, err := c.GetOUHandleByID(context.Background(), testOUB)

			require.Error(t, err)
			assert.False(t, IsNotFound(err))
		})
	}
}

func TestGetOUHandleByID_RejectsNonUUID(t *testing.T) {
	srv := newTestThunderServer(t, func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("no request expected, got %s", r.URL.Path)
	})
	defer srv.Close()

	c := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	_, err := c.GetOUHandleByID(context.Background(), "../users")

	require.Error(t, err)
}
