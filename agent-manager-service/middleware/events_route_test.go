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
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
)

type capturePublisher struct{ got []events.Event }

func (c *capturePublisher) Publish(_ context.Context, e events.Event) error {
	c.got = append(c.got, e)
	return nil
}

func serveEvent(t *testing.T, pattern string, pathValues map[string]string, handler http.HandlerFunc) *capturePublisher {
	t.Helper()
	pub := &capturePublisher{}
	pattern, query, _ := strings.Cut(pattern, "?")
	method, path, _ := strings.Cut(pattern, " ")
	meta := audit.RouteMeta{Pattern: pattern, Method: method, Path: path, Params: extractPathParams(pattern)}
	h := WithEvents(pub, meta)(handler)
	target := "/x"
	if query != "" {
		target += "?" + query
	}
	req := httptest.NewRequest(meta.Method, target, nil)
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	req = req.WithContext(orgctx.WithResolvedOrg(req.Context(), orgctx.ResolvedOrg{OUID: "ou-1", OuHandle: "acme"}))
	h(httptest.NewRecorder(), req)
	return pub
}

func TestWithEvents_PublishesAgentEvent(t *testing.T) {
	pub := serveEvent(t, "PUT /agents/{agentName}?project=p1",
		map[string]string{"agentName": "a1"},
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"name":"a1","environment":"dev","imageId":"img","apiKey":"sk-1"}`))
		})

	require.Len(t, pub.got, 1)
	e := pub.got[0]
	assert.Equal(t, "com.wso2.agentmanager.agent.updated", e.Type)
	assert.Equal(t, "agent.updated", e.Name())
	assert.Equal(t, "1.0", e.SpecVersion)
	assert.Equal(t, "application/json", e.DataContentType)
	assert.Equal(t, "/agent-manager/orgs/ou-1/projects/p1/agents/a1", e.Source)
	assert.Equal(t, events.ScopeAgent, e.Scope)
	assert.Equal(t, "ou-1", e.OrgID)
	assert.Equal(t, "acme", e.OrgHandle)
	assert.Equal(t, "p1", e.Project)
	assert.Equal(t, "a1", e.Agent)
	assert.Empty(t, e.Environment, "the environment comes from the path or a handler annotation, never the response")
	data := e.Data.(map[string]any)
	assert.Equal(t, "img", data["imageId"])
	assert.NotContains(t, data, "apiKey", "credential fields are removed")
}

func TestWithEvents_SkipsFailuresAndUncataloguedRoutes(t *testing.T) {
	pub := serveEvent(t, "POST /orgs/{orgName}/projects", map[string]string{"orgName": "acme"},
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadRequest) })
	assert.Empty(t, pub.got, "a failed request emits nothing")

	pub = serveEvent(t, "POST /orgs/{orgName}/repositories/branches", map[string]string{"orgName": "acme"},
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	assert.Empty(t, pub.got, "a route outside the catalog emits nothing")
}

// serveRouteEvent serves one request through a module route's event, as
// HandleModuleRoute installs it.
func serveRouteEvent(t *testing.T, pattern string, omitData bool, pathValues map[string]string, handler http.HandlerFunc) *capturePublisher {
	t.Helper()
	pub := &capturePublisher{}
	method, path, _ := strings.Cut(pattern, " ")
	meta := audit.RouteMeta{Pattern: pattern, Method: method, Path: path, Params: extractPathParams(pattern)}
	h := withRouteEvent(pub, meta, "testmod.key_created", events.ScopeAgent, omitData)(handler)
	req := httptest.NewRequest(method, "/x", nil)
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	req = req.WithContext(orgctx.WithResolvedOrg(req.Context(), orgctx.ResolvedOrg{OUID: "ou-1", OuHandle: "acme"}))
	h(httptest.NewRecorder(), req)
	return pub
}

func TestWithEvents_OmitDataRoutesCarryNoBody(t *testing.T) {
	pub := serveRouteEvent(t, "POST /m/testmod/orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys", true,
		map[string]string{"orgName": "acme", "projName": "p1", "agentName": "a1", "envID": "prod"},
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"name":"k1","value":"the-key"}`))
		})
	require.Len(t, pub.got, 1)
	assert.Nil(t, pub.got[0].Data, "a route returning a credential sends no data")
	assert.Equal(t, "prod", pub.got[0].Environment, "the environment comes from the path")
}

func TestWithEvents_APIKeyEventNamesKeyAndEnvironment(t *testing.T) {
	// The API key service resolves envID, which may be a UUID, to the
	// environment name, and adds the created key's name, through modkit.
	pub := serveRouteEvent(t, "POST /m/testmod/orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys", true,
		map[string]string{"orgName": "acme", "projName": "p1", "agentName": "a1", "envID": "6f1c0c1e-env-uuid"},
		func(w http.ResponseWriter, r *http.Request) {
			modkit.EventEnvironment(r.Context(), "prod")
			modkit.EventData(r.Context(), "name", "k1")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"keyId":"k1","apiKey":"the-key"}`))
		})
	require.Len(t, pub.got, 1)
	e := pub.got[0]
	assert.Equal(t, "prod", e.Environment, "the resolved name wins over the path value")
	assert.Equal(t, "k1", e.Subject, "the subject is the created key")
	assert.Equal(t, map[string]any{"name": "k1"}, e.Data, "only the key name is sent, never the key")
}

func TestWithEvents_HandlerAnnotations(t *testing.T) {
	pub := serveEvent(t, "PUT /orgs/{orgName}/projects/{projName}", map[string]string{"orgName": "acme", "projName": "p1"},
		func(w http.ResponseWriter, r *http.Request) {
			events.AddData(r.Context(), "changed", []string{"description"})
			w.WriteHeader(http.StatusNoContent)
		})
	require.Len(t, pub.got, 1)
	assert.Equal(t, events.ScopeProject, pub.got[0].Scope)
	assert.Equal(t, []string{"description"}, pub.got[0].Data.(map[string]any)["changed"])
}

func TestWithEvents_CreateEventsNameTheCreatedResource(t *testing.T) {
	pub := serveEvent(t, "POST /agents?project=p1",
		map[string]string{},
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"name":"a1","projectName":"p1"}`))
		})
	require.Len(t, pub.got, 1)
	e := pub.got[0]
	assert.Equal(t, events.ScopeProject, e.Scope, "agent.created stays a project event")
	assert.Equal(t, "a1", e.Subject, "the subject is the created agent, not the project")
	assert.Equal(t, "p1", e.Project)
	assert.Equal(t, "a1", e.Agent)
	assert.Equal(t, "/agent-manager/orgs/ou-1/projects/p1/agents/a1", e.Source)

	pub = serveEvent(t, "POST /orgs/{orgName}/projects", map[string]string{"orgName": "acme"},
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"name":"p2"}`))
		})
	require.Len(t, pub.got, 1)
	e = pub.got[0]
	assert.Equal(t, "p2", e.Subject)
	assert.Equal(t, "p2", e.Project)
	assert.Empty(t, e.Agent)
	assert.Equal(t, "/agent-manager/orgs/ou-1/projects/p2", e.Source)
}

func TestWithEventsIgnoresEnvironmentQuery(t *testing.T) {
	pub := serveEvent(t, "PUT /agents/{agentName}?project=p1&environment=spoofed",
		map[string]string{"agentName": "a1"},
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	require.Len(t, pub.got, 1)
	assert.Empty(t, pub.got[0].Environment)
}

// flushCheckPublisher records whether the response had been sent when the
// event was persisted.
type flushCheckPublisher struct {
	w       *httptest.ResponseRecorder
	flushed bool
}

func (p *flushCheckPublisher) Publish(context.Context, events.Event) error {
	p.flushed = p.w.Flushed
	return nil
}

func TestWithEvents_SendsResponseBeforePersistingEvent(t *testing.T) {
	w := httptest.NewRecorder()
	pub := &flushCheckPublisher{w: w}
	meta := audit.RouteMeta{
		Pattern: "POST /orgs/{orgName}/projects", Method: http.MethodPost,
		Path: "/orgs/{orgName}/projects", Params: []string{"orgName"},
	}
	h := WithEvents(pub, meta)(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"name":"p1"}`))
	})
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.SetPathValue("orgName", "acme")
	req = req.WithContext(orgctx.WithResolvedOrg(req.Context(), orgctx.ResolvedOrg{OUID: "ou-1", OuHandle: "acme"}))
	h(w, req)
	assert.True(t, pub.flushed, "the client must not wait on the outbox insert")
}

// serveRouteEventAt is serveRouteEvent requested at target, so a test can add a query.
func serveRouteEventAt(t *testing.T, pattern, target string, pathValues map[string]string, handler http.HandlerFunc) *capturePublisher {
	t.Helper()
	pub := &capturePublisher{}
	method, path, _ := strings.Cut(pattern, " ")
	meta := audit.RouteMeta{Pattern: pattern, Method: method, Path: path, Params: extractPathParams(pattern)}
	h := withRouteEvent(pub, meta, "testmod.widget_updated", events.ScopeAgent, false)(handler)
	req := httptest.NewRequest(method, target, nil)
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	req = req.WithContext(orgctx.WithResolvedOrg(req.Context(), orgctx.ResolvedOrg{OUID: "ou-1", OuHandle: "acme"}))
	h(httptest.NewRecorder(), req)
	return pub
}

func TestWithEvents_ProjectFromQueryOrAnnotation(t *testing.T) {
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

	pub := serveRouteEventAt(t, "PUT /m/testmod/agents/{agentName}", "/x?project=p1",
		map[string]string{"agentName": "a1"}, ok)
	require.Len(t, pub.got, 1)
	assert.Equal(t, "p1", pub.got[0].Project, "a route without {projName} takes the project from the query")
	assert.Equal(t, "/agent-manager/orgs/ou-1/projects/p1/agents/a1", pub.got[0].Source)

	pub = serveRouteEventAt(t, "PUT /m/testmod/agents/{agentName}", "/x",
		map[string]string{"agentName": "a1"}, func(w http.ResponseWriter, r *http.Request) {
			modkit.EventProject(r.Context(), "p2")
			w.WriteHeader(http.StatusOK)
		})
	require.Len(t, pub.got, 1)
	assert.Equal(t, "p2", pub.got[0].Project, "a project read from the body is annotated by the handler")

	pub = serveRouteEventAt(t, "PUT /m/testmod/projects/{projName}/agents/{agentName}", "/x?project=other",
		map[string]string{"projName": "p1", "agentName": "a1"}, ok)
	require.Len(t, pub.got, 1)
	assert.Equal(t, "p1", pub.got[0].Project, "the path wins over the query")
}

func TestWithEvents_CreateEventsNameADocumentByMetadataName(t *testing.T) {
	pub := serveEvent(t, "POST /agents?project=p1",
		map[string]string{},
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"kind":"Agent","metadata":{"name":"a1"},"spec":{"displayName":"Agent One"}}`))
		})
	require.Len(t, pub.got, 1)
	assert.Equal(t, "a1", pub.got[0].Subject)
	assert.Equal(t, "a1", pub.got[0].Agent)
}
