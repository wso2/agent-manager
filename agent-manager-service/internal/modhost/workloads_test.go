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

package modhost_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// fakeTokens signs workload tokens as plain JSON, which is enough to see
// what a heartbeat put in them.
type fakeTokens struct{}

func (fakeTokens) IssueWorkload(w modkit.Workload, ttl time.Duration) (string, time.Time, error) {
	buf, err := json.Marshal(w)
	return base64.RawURLEncoding.EncodeToString(buf), time.Now().Add(ttl), err
}

func (fakeTokens) VerifyWorkload(token string) (modkit.Workload, error) {
	var w modkit.Workload
	buf, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return w, err
	}
	return w, json.Unmarshal(buf, &w)
}

const (
	runnerV1 = "t-run/v1"
	runnerV2 = "t-run/v2"
)

var wsUpgrader = websocket.Upgrader{}

// runnerModule declares a required "runner" service that uses a WebSocket
// endpoint, a "side" service with no endpoints, and a job.
func runnerModule(name string) *testModule {
	mf := manifest(name, nil, nil)
	mf.Workloads = []modkit.WorkloadSpec{
		{
			Name: "runner", Kind: modkit.WorkloadService, Trusted: true, Required: true,
			Scopes: []string{"run"}, Protocols: []string{runnerV2, runnerV1},
			Endpoints: []modkit.Endpoint{{Name: "events", Path: "/events", WebSocket: true}, {Name: "jobs", Path: "/jobs/{id}"}},
		},
		{Name: "side", Kind: modkit.WorkloadService, Scopes: []string{"side"}, Protocols: []string{"t-side/v1"}},
		{Name: "task", Kind: modkit.WorkloadJob, Scopes: []string{"run"}},
	}
	return &testModule{manifest: mf, activate: func(rt modkit.Runtime) error {
		rt.HandleInternal(http.MethodGet, "/whoami", "run", func(w http.ResponseWriter, r *http.Request) {
			wl, _ := modkit.WorkloadFrom(r.Context())
			_ = json.NewEncoder(w).Encode(map[string]string{"instance": wl.Instance, "host": r.Host})
		})
		rt.HandleInternal(http.MethodPost, "/jobs/{id}", "run", func(http.ResponseWriter, *http.Request) {})
		rt.HandleInternal(http.MethodGet, "/events", "run", func(w http.ResponseWriter, r *http.Request) {
			conn, err := wsUpgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer func() { _ = conn.Close() }()
			_ = conn.WriteMessage(websocket.TextMessage, []byte("hello from "+r.Host))
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		})
		return nil
	}}
}

// internalServer serves the internal API as the core mounts it, with an
// audit recorder on every request.
func internalServer(h *modhost.Host) http.Handler {
	inner := http.NewServeMux()
	inner.Handle("POST "+modhost.HeartbeatPath, h.ServeHeartbeat())
	outer := http.NewServeMux()
	outer.Handle("/api/internal/v1/", http.StripPrefix("/api/internal/v1", inner))
	outer.Handle("/m/", h.ServeInternal())
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		outer.ServeHTTP(w, r.WithContext(audit.WithRecorder(r.Context(), audit.NewNoopRecorder())))
	})
}

func audited() context.Context {
	return audit.WithRecorder(context.Background(), audit.NewNoopRecorder())
}

type heartbeatReply struct {
	code  int
	ok    modhost.HeartbeatResponse
	error modhost.WorkloadError
}

func heartbeat(t *testing.T, srv http.Handler, body any, header ...string) heartbeatReply {
	t.Helper()
	buf, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/internal/v1"+modhost.HeartbeatPath, bytes.NewReader(buf))
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	out := heartbeatReply{code: rec.Code}
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out.ok))
	} else {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out.error), rec.Body.String())
	}
	return out
}

func beat(credential, protocol, instance string) map[string]string {
	return map[string]string{"credential": credential, "protocol": protocol, "instance": instance, "version": "1.2.3"}
}

func TestWorkloadDeclarationsThatFailActivation(t *testing.T) {
	ok := func(http.ResponseWriter, *http.Request) {}
	service := func(mut func(*modkit.WorkloadSpec)) modkit.WorkloadSpec {
		wl := modkit.WorkloadSpec{
			Name: "w", Kind: modkit.WorkloadService, Scopes: []string{"s"}, Protocols: []string{"p/v1"},
			Endpoints: []modkit.Endpoint{{Name: "feed", Path: "/feed", WebSocket: true}},
		}
		mut(&wl)
		return wl
	}
	cases := map[string][]modkit.WorkloadSpec{
		"bad name":                 {service(func(w *modkit.WorkloadSpec) { w.Name = "W" })},
		"declared twice":           {service(func(*modkit.WorkloadSpec) {}), service(func(*modkit.WorkloadSpec) {})},
		"no scope":                 {service(func(w *modkit.WorkloadSpec) { w.Scopes = nil })},
		"service without protocol": {service(func(w *modkit.WorkloadSpec) { w.Protocols = nil })},
		"unknown kind":             {service(func(w *modkit.WorkloadSpec) { w.Kind = "daemon" })},
		"job with endpoints":       {service(func(w *modkit.WorkloadSpec) { w.Kind, w.Protocols = modkit.WorkloadJob, nil })},
		"reserved endpoint name":   {service(func(w *modkit.WorkloadSpec) { w.Endpoints[0].Name = modhost.RestEndpoint })},
		"unregistered endpoint":    {service(func(w *modkit.WorkloadSpec) { w.Endpoints[0].Path = "/nowhere" })},
		"websocket on a post":      {service(func(w *modkit.WorkloadSpec) { w.Endpoints[0].Path = "/submit" })},
		"endpoint with a query":    {service(func(w *modkit.WorkloadSpec) { w.Endpoints[0].Path = "/feed?x=1" })},
	}
	for name, workloads := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			mf := manifest("t-wl", nil, nil)
			mf.Workloads = workloads
			m := &testModule{manifest: mf, activate: func(rt modkit.Runtime) error {
				rt.HandleInternal(http.MethodGet, "/feed", "s", ok)
				rt.HandleInternal(http.MethodPost, "/submit", "s", ok)
				return nil
			}}
			h, _ := replica(t, ctx, databases(t)[0], []modkit.Module{m})
			_, err := h.Install(ctx, "t-wl", false)
			require.Error(t, err)
			assert.Empty(t, h.Active())
		})
	}
}

func TestWorkloadPlane(t *testing.T) { eachDatabase(t, workloadPlane) }

func workloadPlane(t *testing.T, d database) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	plain := manifest("t-plain", nil, nil)
	plain.Workloads = []modkit.WorkloadSpec{{Name: "w", Kind: modkit.WorkloadService, Scopes: []string{"s"}, Protocols: []string{"p/v1"}}}
	mods := []modkit.Module{runnerModule("t-run"), &testModule{manifest: plain}}
	h1, g := replica(t, ctx, d, mods)
	h2, _ := replica(t, ctx, d, mods) // health must come from the shared state
	for _, h := range []*modhost.Host{h1, h2} {
		h.Attach(modhost.Deps{Tokens: fakeTokens{}})
	}
	srv := internalServer(h1)
	actx := audited()

	t.Run("credentials are for declared services of an active module", func(t *testing.T) {
		_, err := h1.IssueCredential(actx, "t-run", "runner", "r1")
		assert.True(t, modhost.IsResolveError(err), "an uninstalled module: %v", err)
		installAll(t, h1, "t-run")
		for _, wl := range []string{"task", "nope"} {
			_, err := h1.IssueCredential(actx, "t-run", wl, "r1")
			assert.True(t, modhost.IsResolveError(err), "%s: %v", wl, err)
		}
		_, err = h1.IssueCredential(context.Background(), "t-run", "runner", "r1")
		assert.ErrorIs(t, err, audit.ErrRecorderUnavailable, "a credential nobody can account for is never issued")
		assert.EqualValues(t, 0, count(t, g, `SELECT count(*) FROM workload_credentials`))
	})

	waitFor(t, "the second replica to load t-run", func() bool { return len(h2.Active()) == 1 })
	cred, err := h1.IssueCredential(actx, "t-run", "runner", "r1")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(cred.Secret, cred.ID+"."))
	assert.EqualValues(t, 0, count(t, g, `SELECT count(*) FROM workload_credentials WHERE secret_hash LIKE ?`, "%"+cred.Secret+"%"),
		"only a hash of the secret is stored")

	t.Run("a required service with no instance degrades the module", func(t *testing.T) {
		hl, err := h2.Health(ctx, "t-run")
		require.NoError(t, err)
		assert.Equal(t, modhost.HealthDegraded, hl.Status)
		assert.Equal(t, []string{"no runner instance is connected"}, hl.Reasons)
	})

	t.Run("refusals", func(t *testing.T) {
		r := heartbeat(t, srv, map[string]string{"credential": cred.Secret})
		assert.Equal(t, http.StatusBadRequest, r.code)
		assert.Equal(t, modhost.ErrInvalidRequest, r.error.Error)
		r = heartbeat(t, srv, beat(cred.Secret, runnerV1, "two words"))
		assert.Equal(t, http.StatusBadRequest, r.code)
		for _, bad := range []string{"garbage", cred.ID + ".wrong", "01900000-0000-7000-8000-000000000000." + strings.SplitN(cred.Secret, ".", 2)[1]} {
			r = heartbeat(t, srv, beat(bad, runnerV1, "i1"))
			assert.Equal(t, http.StatusUnauthorized, r.code, bad)
			assert.Equal(t, modhost.ErrInvalidCredential, r.error.Error, bad)
		}
		r = heartbeat(t, srv, beat(cred.Secret, "t-run/v3", "i1"))
		assert.Equal(t, http.StatusConflict, r.code)
		assert.Equal(t, modhost.ErrUnsupportedProtocol, r.error.Error)
		assert.Equal(t, []string{runnerV2, runnerV1}, r.error.Protocols)
		assert.EqualValues(t, 0, count(t, g, `SELECT count(*) FROM workload_instances`))
	})

	t.Run("a heartbeat registers the instance and returns a token and the endpoints", func(t *testing.T) {
		r := heartbeat(t, srv, beat(cred.Secret, runnerV1, "i1"))
		require.Equal(t, http.StatusOK, r.code, r.error.Message)
		assert.Equal(t, "t-run", r.ok.Module)
		assert.Equal(t, "runner", r.ok.Workload)
		assert.Equal(t, "Bearer", r.ok.TokenType)
		assert.WithinDuration(t, time.Now().Add(modhost.DefaultWorkloadTokenTTL), r.ok.ExpiresAt, time.Minute)
		assert.Equal(t, map[string]string{
			"rest":   "http://example.com/m/t-run/internal",
			"events": "ws://example.com/m/t-run/internal/events",
			"jobs":   "http://example.com/m/t-run/internal/jobs/{id}",
		}, r.ok.Endpoints)
		wl, err := fakeTokens{}.VerifyWorkload(r.ok.Token)
		require.NoError(t, err)
		assert.Equal(t, modkit.Workload{Module: "t-run", Name: "runner", Instance: "i1", Subject: "r1", Scopes: []string{"run"}}, wl)

		// The second replica sees it.
		hl, err := h2.Health(ctx, "t-run")
		require.NoError(t, err)
		assert.Equal(t, modhost.HealthOK, hl.Status, hl.Reasons)
		require.Len(t, hl.Workloads, 3)
		require.Len(t, hl.Workloads[0].Instances, 1)
		assert.Equal(t, "i1", hl.Workloads[0].Instances[0].Instance)
		assert.Equal(t, runnerV1, hl.Workloads[0].Instances[0].Protocol)

		// Heartbeats refresh the row; a new protocol is picked up.
		r = heartbeat(t, srv, beat(cred.Secret, runnerV2, "i1"))
		require.Equal(t, http.StatusOK, r.code)
		assert.EqualValues(t, 1, count(t, g, `SELECT count(*) FROM workload_instances WHERE protocol = ?`, runnerV2))

		req := httptest.NewRequest(http.MethodGet, "/m/t-run/internal/whoami", nil)
		req.Header.Set("Authorization", "Bearer "+r.ok.Token)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"instance":"i1"`)
	})

	t.Run("endpoints follow the configured URL, then the forwarded origin", func(t *testing.T) {
		r := heartbeat(t, srv, beat(cred.Secret, runnerV1, "i1"), "X-Forwarded-Proto", "https", "X-Forwarded-Host", "amp.example.com:9243")
		require.Equal(t, http.StatusOK, r.code)
		assert.Equal(t, "https://amp.example.com:9243/m/t-run/internal", r.ok.Endpoints["rest"])
		assert.Equal(t, "wss://amp.example.com:9243/m/t-run/internal/events", r.ok.Endpoints["events"])

		r = heartbeat(t, srv, beat(cred.Secret, runnerV1, "i1"), "X-Forwarded-Proto", "gopher", "X-Forwarded-Host", "evil.example/x")
		require.Equal(t, http.StatusOK, r.code)
		assert.Equal(t, "http://example.com/m/t-run/internal", r.ok.Endpoints["rest"], "invalid forwarded values are ignored")

		h1.Attach(modhost.Deps{Tokens: fakeTokens{}, InternalURL: "https://amp-api:9243/"})
		defer h1.Attach(modhost.Deps{Tokens: fakeTokens{}})
		r = heartbeat(t, srv, beat(cred.Secret, runnerV1, "i1"), "X-Forwarded-Host", "ignored.example")
		require.Equal(t, http.StatusOK, r.code)
		assert.Equal(t, "https://amp-api:9243/m/t-run/internal", r.ok.Endpoints["rest"])
		assert.Equal(t, "wss://amp-api:9243/m/t-run/internal/events", r.ok.Endpoints["events"])
	})

	t.Run("a workload gets only the endpoints it declares", func(t *testing.T) {
		side, err := h1.IssueCredential(actx, "t-run", "side", "s1")
		require.NoError(t, err)
		r := heartbeat(t, srv, beat(side.Secret, "t-side/v1", ""))
		require.Equal(t, http.StatusOK, r.code)
		assert.Equal(t, map[string]string{"rest": "http://example.com/m/t-run/internal"}, r.ok.Endpoints)
		assert.EqualValues(t, 1, count(t, g, `SELECT count(*) FROM workload_instances WHERE instance = 'default'`))

		// A module with no internal routes has no rest endpoint either.
		installAll(t, h1, "t-plain")
		pc, err := h1.IssueCredential(actx, "t-plain", "w", "p1")
		require.NoError(t, err)
		r = heartbeat(t, srv, beat(pc.Secret, "p/v1", "p"))
		require.Equal(t, http.StatusOK, r.code)
		assert.Empty(t, r.ok.Endpoints)
		_, err = h1.RevokeAllCredentials(actx, "t-plain")
		require.NoError(t, err)
	})

	t.Run("a disabled module refuses heartbeats and internal routes with module_inactive", func(t *testing.T) {
		require.NoError(t, h1.Disable(ctx, "t-run"))
		r := heartbeat(t, srv, beat(cred.Secret, runnerV1, "i1"))
		assert.Equal(t, http.StatusUnauthorized, r.code)
		assert.Equal(t, modhost.ErrModuleInactive, r.error.Error)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/m/t-run/internal/whoami", nil))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Contains(t, rec.Body.String(), modhost.ErrModuleInactive)
		rec = httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/m/not-compiled/internal/whoami", nil))
		assert.Equal(t, http.StatusNotFound, rec.Code)
		hl, err := h1.Health(ctx, "t-run")
		require.NoError(t, err)
		assert.Equal(t, modhost.HealthInactive, hl.Status)

		require.NoError(t, h1.Enable(ctx, "t-run"))
		r = heartbeat(t, srv, beat(cred.Secret, runnerV1, "i1"))
		assert.Equal(t, http.StatusOK, r.code, "a workload resumes once the module is enabled")
	})

	t.Run("revoking a credential refuses it and forgets its instances", func(t *testing.T) {
		rt, err := h1.IssueCredential(actx, "t-run", "runner", "r2")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, heartbeat(t, srv, beat(rt.Secret, runnerV1, "i2")).code)
		require.NoError(t, h1.RevokeCredential(actx, "t-run", rt.ID))
		assert.ErrorIs(t, h1.RevokeCredential(actx, "t-run", rt.ID), modkit.ErrNotFound)
		r := heartbeat(t, srv, beat(rt.Secret, runnerV1, "i2"))
		assert.Equal(t, http.StatusUnauthorized, r.code)
		assert.Equal(t, modhost.ErrInvalidCredential, r.error.Error)
		assert.EqualValues(t, 0, count(t, g, `SELECT count(*) FROM workload_instances WHERE credential_id = ?`, rt.ID))
	})

	t.Run("revoking every credential of a module, as a purge does", func(t *testing.T) {
		n, err := h1.RevokeAllCredentials(actx, "t-run")
		require.NoError(t, err)
		assert.Equal(t, 2, n, "runner r1 and side s1")
		r := heartbeat(t, srv, beat(cred.Secret, runnerV1, "i1"))
		assert.Equal(t, modhost.ErrInvalidCredential, r.error.Error)
		assert.EqualValues(t, 0, count(t, g, `SELECT count(*) FROM workload_instances`))
		// The second replica follows the earlier re-enable within a poll.
		waitFor(t, "the second replica to report t-run degraded", func() bool {
			hl, err := h2.Health(ctx, "t-run")
			return err == nil && hl.Status == modhost.HealthDegraded
		})
	})
}

func TestModuleHealthReport(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := &reporting{testModule: runnerModule("t-rep")}
	h, _ := replica(t, ctx, databases(t)[0], []modkit.Module{m})
	installAll(t, h, "t-rep")
	hl, err := h.Health(ctx, "t-rep")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"queue": 3}, hl.Detail)
	_, err = h.Health(ctx, "nope")
	assert.True(t, modhost.IsResolveError(err))
}

type reporting struct{ *testModule }

func (reporting) Health(_ context.Context, rt modkit.Runtime) map[string]any {
	if rt.DB() == nil {
		return nil
	}
	return map[string]any{"queue": 3}
}

// fakeWorkload is configured with only a heartbeat URL and a credential, as
// every workload is. It builds every URL from the endpoints the heartbeat
// returns, and reconnects its WebSocket when they change.
type fakeWorkload struct {
	heartbeatURL, credential string

	mu        sync.Mutex
	endpoints map[string]string
	conn      *websocket.Conn
	greetings []string
}

func (f *fakeWorkload) beat(t *testing.T) error {
	t.Helper()
	buf, _ := json.Marshal(beat(f.credential, runnerV2, "fake-0"))
	resp, err := http.Post(f.heartbeatURL, "application/json", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	var hb modhost.HeartbeatResponse
	if err := json.NewDecoder(resp.Body).Decode(&hb); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.conn != nil && f.endpoints["events"] == hb.Endpoints["events"] {
		return nil
	}
	if f.conn != nil {
		_ = f.conn.Close()
	}
	conn, resp2, err := websocket.DefaultDialer.Dial(hb.Endpoints["events"], http.Header{"Authorization": {"Bearer " + hb.Token}})
	if resp2 != nil {
		_ = resp2.Body.Close()
	}
	if err != nil {
		return err
	}
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return err
	}
	f.endpoints, f.conn = hb.Endpoints, conn
	f.greetings = append(f.greetings, string(msg))
	return nil
}

func TestFakeWorkloadFollowsChangedEndpoints(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h, _ := replica(t, ctx, databases(t)[0], []modkit.Module{runnerModule("t-run")})
	h.Attach(modhost.Deps{Tokens: fakeTokens{}})
	installAll(t, h, "t-run")
	first := httptest.NewServer(internalServer(h))
	defer first.Close()
	second := httptest.NewServer(internalServer(h))
	defer second.Close()
	cred, err := h.IssueCredential(audited(), "t-run", "runner", "fake")
	require.NoError(t, err)

	f := &fakeWorkload{heartbeatURL: first.URL + "/api/internal/v1" + modhost.HeartbeatPath, credential: cred.Secret}
	defer func() {
		if f.conn != nil {
			_ = f.conn.Close()
		}
	}()
	require.NoError(t, f.beat(t))
	require.NoError(t, f.beat(t))
	assert.Equal(t, []string{"hello from " + strings.TrimPrefix(first.URL, "http://")}, f.greetings, "an unchanged endpoint keeps the connection")
	assert.True(t, strings.HasPrefix(f.endpoints["events"], "ws://"))

	// The operator moves the internal server: the next heartbeat says so.
	h.Attach(modhost.Deps{Tokens: fakeTokens{}, InternalURL: second.URL})
	require.NoError(t, f.beat(t))
	assert.Equal(t, []string{
		"hello from " + strings.TrimPrefix(first.URL, "http://"),
		"hello from " + strings.TrimPrefix(second.URL, "http://"),
	}, f.greetings)

	require.NoError(t, h.Disable(ctx, "t-run"))
	assert.EqualError(t, f.beat(t), "401 Unauthorized", "the workload stands by")
}
