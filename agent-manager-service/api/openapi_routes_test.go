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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/webconsole"
)

// fakeModules stands in for the module host; its active set can change.
type fakeModules struct {
	mu     sync.Mutex
	active []string
}

func (f *fakeModules) Active() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.active...)
}

func (f *fakeModules) set(active ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.active = active
}

// catalogMux is the root mux as the server builds it: the catalog routes,
// then the console at "/".
func catalogMux(modules ActiveModules) *http.ServeMux {
	mux := http.NewServeMux()
	registerAPICatalogRoutes(mux, modules)
	mux.Handle("/", webconsole.Handler())
	return mux
}

func get(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

type servedSpec struct {
	Info  map[string]any            `json:"info"`
	Paths map[string]map[string]any `json:"paths"`
}

func fetchSpec(t *testing.T, h http.Handler) servedSpec {
	t.Helper()
	rec := get(t, h, http.MethodGet, "/api/v1/openapi.json")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var s servedSpec
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &s))
	return s
}

func modulesOf(s servedSpec) map[string]bool {
	out := map[string]bool{}
	for _, item := range s.Paths {
		for _, op := range item {
			if m, ok := op.(map[string]any); ok {
				if mod, ok := m["x-amp-module"].(string); ok {
					out[mod] = true
				}
			}
		}
	}
	return out
}

func TestOpenAPISpecFollowsTheActiveModules(t *testing.T) {
	mods := &fakeModules{}
	mux := catalogMux(mods)

	core := fetchSpec(t, mux)
	assert.Equal(t, map[string]bool{"core": true}, modulesOf(core))
	assert.Equal(t, []any{"core"}, core.Info["x-amp-modules"])
	assert.Contains(t, core.Paths, "/orgs/{orgName}/projects")
	assert.NotContains(t, core.Paths, "/orgs/{orgName}/gateways")

	mods.set("gateway", "legacy")
	full := fetchSpec(t, mux)
	assert.Equal(t, map[string]bool{"core": true, "gateway": true, "legacy": true}, modulesOf(full))
	assert.Contains(t, full.Paths, "/orgs/{orgName}/gateways")
	assert.Contains(t, full.Paths, "/orgs/{orgName}/evaluators", "legacy's served routes are listed")
	assert.NotContains(t, full.Paths, "/orgs/{orgName}/data-planes", "routes answering 501 are not")
}

func TestOpenAPISpecWithoutAModuleHost(t *testing.T) {
	assert.Equal(t, map[string]bool{"core": true}, modulesOf(fetchSpec(t, catalogMux(nil))))
}

func TestOpenAPISpecAsYAML(t *testing.T) {
	rec := get(t, catalogMux(&fakeModules{active: []string{"gateway"}}), http.MethodGet, "/api/v1/openapi.yaml")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/yaml", rec.Header().Get("Content-Type"))
	var doc struct {
		OpenAPI string         `yaml:"openapi"`
		Paths   map[string]any `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(rec.Body.Bytes(), &doc))
	assert.Equal(t, "3.0.3", doc.OpenAPI)
	assert.Contains(t, doc.Paths, "/orgs/{orgName}/gateways")
}

func TestOpenAPISpecRefusesWrites(t *testing.T) {
	assert.Equal(t, http.StatusMethodNotAllowed, get(t, catalogMux(nil), http.MethodPost, "/api/v1/openapi.json").Code)
}

func TestAPIDocsIsNotTheConsole(t *testing.T) {
	mux := catalogMux(nil)
	rec := get(t, mux, http.MethodGet, "/api/docs")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<redoc ")
	assert.NotEmpty(t, rec.Header().Get("Content-Security-Policy"))

	assert.Equal(t, http.StatusNotFound, get(t, mux, http.MethodGet, "/api/docs/nosuch.js").Code,
		"an unknown viewer file is a 404, not the console's index page")
	body := get(t, mux, http.MethodGet, "/api/docs/nosuch.js").Body.String()
	assert.False(t, strings.Contains(body, "<redoc") || strings.Contains(strings.ToLower(body), "<html"))
}

// snapshotModules is a module host that hands out its active set and its
// fragments together, and counts how often it is asked.
type snapshotModules struct {
	fakeModules
	frags map[string][]byte
	calls int
}

func (s *snapshotModules) API() ([]string, map[string][]byte) {
	s.calls++
	return s.Active(), s.frags
}

func TestOpenAPISpecReadsModulesAndFragmentsTogether(t *testing.T) {
	mods := &snapshotModules{
		fakeModules: fakeModules{active: []string{"widgets"}},
		frags:       map[string][]byte{"widgets": []byte("paths:\n  /tags:\n    get:\n      responses:\n        \"200\":\n          description: ok\n")},
	}
	spec := fetchSpec(t, catalogMux(mods))
	assert.Contains(t, spec.Paths, "/m/widgets/tags")
	assert.Equal(t, 1, mods.calls, "one snapshot per request")
}
