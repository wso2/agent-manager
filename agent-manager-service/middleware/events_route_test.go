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
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
)

type capturePublisher struct{ got []events.Event }

func (c *capturePublisher) Publish(_ context.Context, e events.Event) { c.got = append(c.got, e) }

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

func TestWithEvents_PublishesDeployWithEnvironmentFromResponse(t *testing.T) {
	pub := serveEvent(t, "POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments",
		map[string]string{"orgName": "acme", "projName": "p1", "agentName": "a1"},
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"agentName":"a1","environment":"dev","imageId":"img","apiKey":"sk-1"}`))
		})

	require.Len(t, pub.got, 1)
	e := pub.got[0]
	assert.Equal(t, "com.wso2.agentmanager.agent.deployed", e.Type)
	assert.Equal(t, "agent.deployed", e.Name())
	assert.Equal(t, "1.0", e.SpecVersion)
	assert.Equal(t, "application/json", e.DataContentType)
	assert.Equal(t, "/agent-manager/orgs/ou-1/projects/p1/agents/a1", e.Source)
	assert.Equal(t, events.ScopeAgent, e.Scope)
	assert.Equal(t, "ou-1", e.OrgID)
	assert.Equal(t, "acme", e.OrgHandle)
	assert.Equal(t, "p1", e.Project)
	assert.Equal(t, "a1", e.Agent)
	assert.Equal(t, "dev", e.Environment, "the environment is read from the response")
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

func TestWithEvents_OmitDataRoutesCarryNoBody(t *testing.T) {
	pub := serveEvent(t, "POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys",
		map[string]string{"orgName": "acme", "projName": "p1", "agentName": "a1", "envID": "prod"},
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"name":"k1","value":"the-key"}`))
		})
	require.Len(t, pub.got, 1)
	assert.Nil(t, pub.got[0].Data, "a route returning a credential sends no data")
	assert.Equal(t, "prod", pub.got[0].Environment, "the environment comes from the path")
	assert.Equal(t, "prod", pub.got[0].Subject, "the subject is the last path parameter")
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

func TestWithEvents_AgentEventsAreEnvironmentSpecific(t *testing.T) {
	// The environment can come from the query string...
	pub := serveEvent(t, "PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/resource-configs?environment=prod",
		map[string]string{"orgName": "acme", "projName": "p1", "agentName": "a1"},
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	require.Len(t, pub.got, 1)
	assert.Equal(t, "prod", pub.got[0].Environment)

	// ...or from a handler annotation when it is in the request body.
	pub = serveEvent(t, "PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/configurations",
		map[string]string{"orgName": "acme", "projName": "p1", "agentName": "a1"},
		func(w http.ResponseWriter, r *http.Request) {
			events.SetEnvironment(r.Context(), "dev")
			w.WriteHeader(http.StatusNoContent)
		})
	require.Len(t, pub.got, 1)
	assert.Equal(t, "dev", pub.got[0].Environment)

	// A build is not tied to an environment: it is an agent event with none.
	pub = serveEvent(t, "POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/builds",
		map[string]string{"orgName": "acme", "projName": "p1", "agentName": "a1"},
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) })
	require.Len(t, pub.got, 1)
	assert.Equal(t, events.ScopeAgent, pub.got[0].Scope)
	assert.Empty(t, pub.got[0].Environment)
}
