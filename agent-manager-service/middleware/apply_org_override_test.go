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

package middleware

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// These tests install the process-wide OrgOverrideResolver, so none of them
// runs in parallel.

const (
	overrideTokenOUID    = "a5000000-0000-0000-0000-adminroot001"
	overrideTokenHandle  = "admin-root"
	overrideTargetOUID   = "01900000-0000-7000-8000-0000000000b2"
	overrideTargetHandle = "org-b"
	overrideClientID     = "IPAAS_SERVICE"
)

type overrideOutcome struct {
	status     int
	handlerRan bool
	resolved   ResolvedOrg
	events     []audit.Event
	logs       string
}

// serveOverride runs a request through the chain the project MCP routes use
// (audit → RequireOrgMatch → ApplyOrgOverride → handler).
func serveOverride(t *testing.T, header string, lookup OUHandleLookup) overrideOutcome {
	t.Helper()

	var logBuf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logBuf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	var out overrideOutcome
	next := func(w http.ResponseWriter, r *http.Request) {
		out.handlerRan = true
		out.resolved, _ = GetResolvedOrg(r.Context())
		w.WriteHeader(http.StatusOK)
	}

	pattern := "PUT /orgs/{orgName}/projects/{projName}/mcp-proxies/{proxyId}"
	meta := audit.NewRouteMeta(pattern, audit.ExtractPathParams(pattern), []rbac.Permission{rbac.MCPServerUpdate})
	rec := &captureRecorder{}
	handler := WithAudit(rec, meta)(RequireOrgMatch(stubResolver{})(ApplyOrgOverride(lookup)(next)))

	req := httptest.NewRequest(http.MethodPut, "/orgs/x/projects/p1/mcp-proxies/h1", nil)
	if header != "" {
		req.Header.Set(occlient.HeaderImpersonateOrg, header)
	}
	claims := &jwtassertion.TokenClaims{
		Sub: overrideClientID, ClientID: overrideClientID,
		OuId: overrideTokenOUID, OuHandle: overrideTokenHandle,
	}
	req = req.WithContext(jwtassertion.ContextWithTokenClaims(req.Context(), claims))

	w := httptest.NewRecorder()
	handler(w, req)

	out.status = w.Code
	out.events = rec.events
	out.logs = logBuf.String()
	return out
}

func staticLookup(handle string, err error) (OUHandleLookup, *int) {
	calls := 0
	return func(_ context.Context, ouID string) (string, error) {
		calls++
		if ouID != overrideTargetOUID {
			return "", fmt.Errorf("unexpected lookup of %q", ouID)
		}
		return handle, err
	}, &calls
}

func trustingResolver(r *http.Request) (string, bool) { return ImpersonatedOrgHeader(r) }

func findEvent(events []audit.Event, action audit.Action) *audit.Event {
	for i := range events {
		if events[i].Action == action {
			return &events[i]
		}
	}
	return nil
}

func assertDenied(t *testing.T, out overrideOutcome, wantReason string) {
	t.Helper()
	assert.Equal(t, http.StatusForbidden, out.status)
	assert.False(t, out.handlerRan, "a refused override must not reach the handler")
	deny := findEvent(out.events, audit.ActionAuthzDeny)
	require.NotNil(t, deny, "the refusal is recorded as an authz deny")
	assert.Equal(t, wantReason, deny.Details["reason"])
}

func TestApplyOrgOverride_NoResolver_Header403(t *testing.T) {
	installOrgOverrideResolver(t, nil)
	lookup, calls := staticLookup(overrideTargetHandle, nil)

	out := serveOverride(t, overrideTargetOUID, lookup)

	assertDenied(t, out, "org-override-not-permitted")
	assert.Zero(t, *calls)
}

// A malformed header is still an attempt to impersonate: it is refused rather
// than silently ignored, so a caller never acts on its own org by mistake.
func TestApplyOrgOverride_NoResolver_MalformedHeader403(t *testing.T) {
	installOrgOverrideResolver(t, nil)
	lookup, _ := staticLookup(overrideTargetHandle, nil)

	out := serveOverride(t, "not-a-uuid", lookup)

	assertDenied(t, out, "org-override-not-permitted")
}

func TestApplyOrgOverride_Allowed_RewritesOUIDAndHandle(t *testing.T) {
	installOrgOverrideResolver(t, trustingResolver)
	lookup, calls := staticLookup(overrideTargetHandle, nil)

	out := serveOverride(t, overrideTargetOUID, lookup)

	require.Equal(t, http.StatusOK, out.status)
	require.True(t, out.handlerRan)
	assert.Equal(t, ResolvedOrg{OUID: overrideTargetOUID, OuHandle: overrideTargetHandle}, out.resolved)
	assert.Equal(t, 1, *calls)

	assert.Contains(t, out.logs, `"msg":"org override applied"`)
	assert.Contains(t, out.logs, `"clientId":"`+overrideClientID+`"`)
	assert.Contains(t, out.logs, `"tokenOuID":"`+overrideTokenOUID+`"`)
	assert.Contains(t, out.logs, `"targetOuID":"`+overrideTargetOUID+`"`)

	ev := findEvent(out.events, audit.ActionAuthzOrgOverride)
	require.NotNil(t, ev, "an applied override is a first-class audit event")
	assert.Equal(t, overrideTargetOUID, ev.OUID, "the event is filed under the target org")
	assert.Equal(t, overrideTokenOUID, ev.ActorOUID)
	assert.Equal(t, overrideClientID, ev.Details["clientId"])
	assert.Equal(t, overrideTokenOUID, ev.Details["tokenOuID"])
	assert.Equal(t, overrideTargetOUID, ev.Details["targetOuID"])
}

func TestApplyOrgOverride_Refused_403(t *testing.T) {
	installOrgOverrideResolver(t, func(*http.Request) (string, bool) { return "", false })
	lookup, calls := staticLookup(overrideTargetHandle, nil)

	out := serveOverride(t, overrideTargetOUID, lookup)

	assertDenied(t, out, "org-override-not-permitted")
	assert.Zero(t, *calls)
}

// A resolver that says ok but names no organization is a bug; it must fail
// closed rather than leave the request on an empty org.
func TestApplyOrgOverride_ResolverReturnsEmpty_403(t *testing.T) {
	installOrgOverrideResolver(t, func(*http.Request) (string, bool) { return "", true })
	lookup, _ := staticLookup(overrideTargetHandle, nil)

	out := serveOverride(t, overrideTargetOUID, lookup)

	assertDenied(t, out, "org-override-not-permitted")
}

func TestApplyOrgOverride_NoHeader_Passthrough(t *testing.T) {
	// The resolver would allow an override; without the header it is not asked.
	installOrgOverrideResolver(t, func(*http.Request) (string, bool) { return overrideTargetOUID, true })
	lookup, calls := staticLookup(overrideTargetHandle, nil)

	out := serveOverride(t, "", lookup)

	require.Equal(t, http.StatusOK, out.status)
	assert.Equal(t, ResolvedOrg{OUID: overrideTokenOUID, OuHandle: overrideTokenHandle}, out.resolved)
	assert.Zero(t, *calls)
	assert.Nil(t, findEvent(out.events, audit.ActionAuthzOrgOverride))
	assert.NotContains(t, out.logs, "org override applied")
}

func TestApplyOrgOverride_LookupFails_502(t *testing.T) {
	installOrgOverrideResolver(t, trustingResolver)
	lookup, _ := staticLookup("", errors.New("thunder unavailable"))

	out := serveOverride(t, overrideTargetOUID, lookup)

	assert.Equal(t, http.StatusBadGateway, out.status)
	assert.False(t, out.handlerRan)
	assert.Nil(t, findEvent(out.events, audit.ActionAuthzOrgOverride))
}

func TestApplyOrgOverride_LookupNotFound_403(t *testing.T) {
	installOrgOverrideResolver(t, trustingResolver)
	lookup, _ := staticLookup("", fmt.Errorf("wrapped: %w", ErrOverrideOrgNotFound))

	out := serveOverride(t, overrideTargetOUID, lookup)

	assertDenied(t, out, "org-override-target-not-found")
}

func TestApplyOrgOverride_LookupReturnsEmptyHandle_502(t *testing.T) {
	installOrgOverrideResolver(t, trustingResolver)
	lookup, _ := staticLookup("", nil)

	out := serveOverride(t, overrideTargetOUID, lookup)

	assert.Equal(t, http.StatusBadGateway, out.status)
	assert.False(t, out.handlerRan)
}

func TestApplyOrgOverride_NilLookupPanics(t *testing.T) {
	assert.Panics(t, func() { ApplyOrgOverride(nil) })
}
