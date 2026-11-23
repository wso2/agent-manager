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

package observability

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store/memory"
)

// agentsCore knows agent-1; looking up "broken" fails.
type agentsCore struct {
	fakeCore
	lookups atomic.Int32
}

func (c *agentsCore) GetAgent(_ context.Context, _, agentID string) (modkit.Agent, error) {
	c.lookups.Add(1)
	switch agentID {
	case "agent-1":
		return modkit.Agent{ID: agentID}, nil
	case "broken":
		return modkit.Agent{}, errors.New("database down")
	}
	return modkit.Agent{}, modkit.ErrNotFound
}

// ingestRuntime answers Config from a map and Core from core.
type ingestRuntime struct {
	configRuntime
	core modkit.CoreAPI
}

func (r ingestRuntime) Core() modkit.CoreAPI { return r.core }

// collected is what the fake collector received.
type collected struct {
	header http.Header
	body   []byte
}

// fakeCollector records each export and answers with status and body.
func fakeCollector(t *testing.T, status int, body string) (*httptest.Server, chan collected) {
	t.Helper()
	got := make(chan collected, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/traces", r.URL.Path)
		b, _ := io.ReadAll(r.Body)
		got <- collected{header: r.Header.Clone(), body: b}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func newTestIngest(t *testing.T, cfg map[string]string, core modkit.CoreAPI) *ingest {
	t.Helper()
	in, err := newIngest(ingestRuntime{configRuntime: configRuntime{cfg: cfg}, core: core})
	require.NoError(t, err)
	return in
}

var testKey = modkit.AgentKey{
	KeyID: "jti-1", OrgID: "org-a", OrgName: "acme", ProjectID: "proj-1", ProjectName: "shop",
	AgentID: "agent-1", AgentName: "bot", EnvironmentID: "env-1",
}

// export sends body as the agent with key, with extra headers as name/value pairs.
func export(in *ingest, key *modkit.AgentKey, contentType string, body []byte, header ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/m/observability"+IngestPath, bytes.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	for i := 0; i+1 < len(header); i += 2 {
		r.Header.Set(header[i], header[i+1])
	}
	if key != nil {
		r = r.WithContext(modkit.WithAgentKey(r.Context(), *key))
	}
	w := httptest.NewRecorder()
	in.serve(w, r)
	return w
}

func TestIngestForwardsWithIdentityFromTheKey(t *testing.T) {
	srv, got := fakeCollector(t, http.StatusOK, `{"partialSuccess":{}}`)
	in := newTestIngest(t, map[string]string{"collector.url": srv.URL + "/"}, &agentsCore{})
	body := []byte{0x0a, 0x00, 0xff, 0x10, 'o', 't', 'l', 'p'}

	w := export(in, &testKey, "application/x-protobuf", body,
		"X-Org-Id", "org-b", "X-User-Agent-Name", "spoofed", "X-User-Component", "agent-b",
		"Authorization", "Bearer user-token", "X-Forwarded-For", "10.0.0.9", "X-Custom", "anything")

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"partialSuccess":{}}`, w.Body.String(), "the collector's answer is passed back")
	c := <-got
	assert.Equal(t, body, c.body, "the body is forwarded byte for byte")
	assert.Equal(t, "application/x-protobuf", c.header.Get("Content-Type"))
	want := map[string]string{
		"X-Org-Id": "org-a", "X-User-Project": "proj-1", "X-User-Component": "agent-1", "X-User-Environment": "env-1",
		"X-User-Organization-Name": "acme", "X-User-Project-Name": "shop", "X-User-Agent-Name": "bot",
	}
	for h, v := range want {
		assert.Equal(t, []string{v}, c.header.Values(h), "%s is the key's, whatever the agent sent", h)
	}
	for _, h := range []string{"Authorization", "X-Forwarded-For", "X-Custom", "X-Amp-Api-Key"} {
		assert.Empty(t, c.header.Get(h), "%s does not reach the collector", h)
	}
}

func TestIngestForwardsGzipUnchanged(t *testing.T) {
	srv, got := fakeCollector(t, http.StatusOK, `{}`)
	in := newTestIngest(t, map[string]string{"collector.url": srv.URL}, &agentsCore{})
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte(`{"resourceSpans":[]}`))
	require.NoError(t, zw.Close())

	w := export(in, &testKey, "application/json; charset=utf-8", buf.Bytes(), "Content-Encoding", "gzip")

	require.Equal(t, http.StatusOK, w.Code)
	c := <-got
	assert.Equal(t, buf.Bytes(), c.body, "a gzipped body is forwarded still compressed")
	assert.Equal(t, "gzip", c.header.Get("Content-Encoding"))
	assert.Equal(t, "application/json; charset=utf-8", c.header.Get("Content-Type"))
}

func TestIngestPassesTheCollectorsStatusBack(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		srv, _ := fakeCollector(t, status, `{"code":3}`)
		in := newTestIngest(t, map[string]string{"collector.url": srv.URL}, &agentsCore{})
		w := export(in, &testKey, "application/json", []byte(`{}`))
		assert.Equal(t, status, w.Code)
		assert.JSONEq(t, `{"code":3}`, w.Body.String())
	}
}

func TestIngestRefusals(t *testing.T) {
	srv, got := fakeCollector(t, http.StatusOK, `{}`)
	down := httptest.NewServer(http.NotFoundHandler())
	down.Close()
	small := map[string]string{"collector.url": srv.URL, "ingest.max-body-bytes": "16"}
	deleted, broken := testKey, testKey
	deleted.AgentID, broken.AgentID = "deleted", "broken"
	big := bytes.Repeat([]byte("x"), 17)

	cases := []struct {
		name   string
		cfg    map[string]string
		key    *modkit.AgentKey
		ctype  string
		body   []byte
		header []string
		chunk  bool
		want   int
	}{
		{name: "no collector configured", cfg: map[string]string{}, key: &testKey, ctype: "application/json", want: http.StatusServiceUnavailable},
		{name: "collector unreachable", cfg: map[string]string{"collector.url": down.URL}, key: &testKey, ctype: "application/json", want: http.StatusServiceUnavailable},
		{name: "no agent key", cfg: small, ctype: "application/json", want: http.StatusUnauthorized},
		{name: "a deleted agent", cfg: small, key: &deleted, ctype: "application/json", want: http.StatusUnauthorized},
		{name: "the agent lookup fails", cfg: small, key: &broken, ctype: "application/json", want: http.StatusServiceUnavailable},
		{name: "another content type", cfg: small, key: &testKey, ctype: "text/plain", want: http.StatusUnsupportedMediaType},
		{name: "another encoding", cfg: small, key: &testKey, ctype: "application/json", header: []string{"Content-Encoding", "br"}, want: http.StatusUnsupportedMediaType},
		{name: "a declared length over the limit", cfg: small, key: &testKey, ctype: "application/json", body: big, want: http.StatusRequestEntityTooLarge},
		{name: "a chunked body over the limit", cfg: small, key: &testKey, ctype: "application/json", body: big, chunk: true, want: http.StatusRequestEntityTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := newTestIngest(t, tc.cfg, &agentsCore{})
			r := httptest.NewRequest(http.MethodPost, "/m/observability"+IngestPath, bytes.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.ctype)
			for i := 0; i+1 < len(tc.header); i += 2 {
				r.Header.Set(tc.header[i], tc.header[i+1])
			}
			if tc.chunk {
				r.ContentLength = -1
			}
			if tc.key != nil {
				r = r.WithContext(modkit.WithAgentKey(r.Context(), *tc.key))
			}
			w := httptest.NewRecorder()
			in.serve(w, r)
			assert.Equal(t, tc.want, w.Code, w.Body.String())
			assert.NotEqual(t, http.StatusOK, w.Code, "a refusal is never a silent 200")
		})
	}
	assert.Empty(t, got, "nothing refused reaches the collector")
}

func TestIngestRemembersTheAgent(t *testing.T) {
	srv, _ := fakeCollector(t, http.StatusOK, `{}`)
	core := &agentsCore{}
	in := newTestIngest(t, map[string]string{"collector.url": srv.URL}, core)
	for range 3 {
		require.Equal(t, http.StatusOK, export(in, &testKey, "application/json", []byte(`{}`)).Code)
	}
	assert.Equal(t, int32(1), core.lookups.Load(), "the core is asked once a minute, not per export")
}

func TestIngestConfig(t *testing.T) {
	for _, cfg := range []map[string]string{
		{"collector.url": "otel-collector:4318"},
		{"collector.url": "ftp://otel-collector"},
		{"collector.url": "http://c", "ingest.max-body-bytes": "-1"},
		{"collector.url": "http://c", "ingest.max-body-bytes": "lots"},
	} {
		_, err := newIngest(ingestRuntime{configRuntime: configRuntime{cfg: cfg}, core: &agentsCore{}})
		assert.Error(t, err, "%v", cfg)
	}
	in := newTestIngest(t, map[string]string{"collector.url": "http://otel-collector:4318/", "ingest.max-body-bytes": strconv.Itoa(1 << 20)}, &agentsCore{})
	assert.Equal(t, "http://otel-collector:4318/v1/traces", in.traces)
	assert.Equal(t, int64(1<<20), in.maxBody)
	assert.True(t, strings.HasSuffix(IngestPath, "/v1/traces"), "the agent's OTLP endpoint is <amp>/m/observability/otel")
}

// recordingRuntime records the routes Activate registers.
type recordingRuntime struct {
	ingestRuntime
	routes map[string][]modkit.RouteOption
	agent  map[string]bool
}

func (r *recordingRuntime) Handle(method, path, _ string, _ http.HandlerFunc, opts ...modkit.RouteOption) {
	r.routes[method+" "+path] = opts
}

func (r *recordingRuntime) HandleAgent(method, path string, _ http.HandlerFunc, opts ...modkit.RouteOption) {
	r.routes[method+" "+path] = opts
	r.agent[method+" "+path] = true
}

func (*recordingRuntime) Subscribe(string, modkit.EventHandler) {}
func (*recordingRuntime) Provide(string, any)                   {}

func activated(t *testing.T) *recordingRuntime {
	t.Helper()
	rt := &recordingRuntime{
		ingestRuntime: ingestRuntime{configRuntime: configRuntime{cfg: map[string]string{}}, core: &agentsCore{}},
		routes:        map[string][]modkit.RouteOption{}, agent: map[string]bool{},
	}
	require.NoError(t, New(memory.New()).Activate(context.Background(), rt))
	return rt
}

func TestIngestRouteIsAnAuditedAgentRoute(t *testing.T) {
	rt := activated(t)
	route := http.MethodPost + " " + IngestPath
	require.True(t, rt.agent[route], "agents call it with their key")
	o := modkit.Options(rt.routes[route])
	assert.Equal(t, ActionTracesIngest, o.AuditAction)
	assert.Equal(t, time.Hour, o.AuditCoalesce, "recorded once per agent per hour")
	var declared []string
	for _, a := range New(nil).Manifest().AuditActions {
		declared = append(declared, a.Name)
	}
	assert.Contains(t, declared, ActionTracesIngest)
}

func TestOpenAPIDescribesEveryRoute(t *testing.T) {
	var doc struct {
		Paths map[string]map[string]struct {
			Security []map[string][]string `yaml:"security"`
		} `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(openAPI, &doc))
	var documented, mounted []string
	for path, ops := range doc.Paths {
		for method, op := range ops {
			if method == "parameters" {
				continue
			}
			documented = append(documented, strings.ToUpper(method)+" "+path)
			if path == IngestPath {
				assert.Equal(t, []map[string][]string{{"agentKey": {}}}, op.Security)
			}
		}
	}
	for route := range activated(t).routes {
		mounted = append(mounted, route)
	}
	assert.ElementsMatch(t, mounted, documented)
}
