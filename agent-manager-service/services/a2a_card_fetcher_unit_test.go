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

package services

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validTestCard = `{"name":"Trip Planner","description":"d","supportedInterfaces":[{"url":"https://a.example/rpc","protocolBinding":"JSONRPC"}],"skills":[{"name":"plan"}],"x-extra":{"kept":true}}`

func cardServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestA2ACardFetcherReturnsTheCardAsServed(t *testing.T) {
	srv := cardServer(t, http.StatusOK, validTestCard)
	got, err := newA2ACardFetcher(time.Second, "").Fetch(context.Background(), srv.URL+a2aAgentCardPath, false)
	require.NoError(t, err)
	assert.JSONEq(t, validTestCard, string(got), "unknown fields pass through")
}

func TestA2ACardFetcherPinnedDialKeepsTheURLHost(t *testing.T) {
	var gotHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		_, _ = w.Write([]byte(validTestCard))
	}))
	t.Cleanup(srv.Close)

	url := "http://default-default.gateway.invalid:19080" + a2aAgentCardPath
	_, err := newA2ACardFetcher(time.Second, srv.Listener.Addr().String()).Fetch(context.Background(), url, false)
	require.NoError(t, err)
	assert.Equal(t, "default-default.gateway.invalid:19080", gotHost)
}

func TestA2ACardFetcherRejectsInvalidCards(t *testing.T) {
	cases := map[string]string{
		"missing name":               `{"supportedInterfaces":[{"url":"u"}],"skills":[]}`,
		"empty name":                 `{"name":"","supportedInterfaces":[{"url":"u"}],"skills":[]}`,
		"missing interfaces":         `{"name":"n","skills":[]}`,
		"empty interfaces":           `{"name":"n","supportedInterfaces":[],"skills":[]}`,
		"interface without url":      `{"name":"n","supportedInterfaces":[{"protocolBinding":"JSONRPC"}],"skills":[]}`,
		"missing skills":             `{"name":"n","supportedInterfaces":[{"url":"u"}]}`,
		"null skills":                `{"name":"n","supportedInterfaces":[{"url":"u"}],"skills":null}`,
		"skills not an array":        `{"name":"n","supportedInterfaces":[{"url":"u"}],"skills":"x"}`,
		"not json":                   `<html>hi</html>`,
		"trailing garbage after obj": `{"name":"n","supportedInterfaces":[{"url":"u"}],"skills":[]} x`,
		"capitalised keys":           `{"Name":"n","SupportedInterfaces":[{"URL":"u"}],"Skills":[]}`,
		"capitalised interface url":  `{"name":"n","supportedInterfaces":[{"URL":"u"}],"skills":[]}`,
		"capitalised skills":         `{"name":"n","supportedInterfaces":[{"url":"u"}],"Skills":[]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			srv := cardServer(t, http.StatusOK, body)
			_, err := newA2ACardFetcher(time.Second, "").Fetch(context.Background(), srv.URL, false)
			require.Error(t, err)
		})
	}
}

func TestA2ACardFetcherRejectsNonObjects(t *testing.T) {
	for _, body := range []string{`[]`, `"card"`, `null`, `42`} {
		srv := cardServer(t, http.StatusOK, body)
		_, err := newA2ACardFetcher(time.Second, "").Fetch(context.Background(), srv.URL, false)
		require.Error(t, err, body)
	}
}

func TestA2ACardFetcherReportsTheStatusCode(t *testing.T) {
	srv := cardServer(t, http.StatusNotFound, `{"name":"n"}`)
	_, err := newA2ACardFetcher(time.Second, "").Fetch(context.Background(), srv.URL, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestA2ACardFetcherRejectsAnOversizedBody(t *testing.T) {
	padding := strings.Repeat("a", a2aCardMaxBytes)
	body := `{"name":"n","supportedInterfaces":[{"url":"u"}],"skills":[],"pad":"` + padding + `"}`
	srv := cardServer(t, http.StatusOK, body)
	_, err := newA2ACardFetcher(time.Second, "").Fetch(context.Background(), srv.URL, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1 MiB")
}

func TestA2ACardFetcherTimesOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, _ = w.Write([]byte(validTestCard))
	}))
	t.Cleanup(srv.Close)
	_, err := newA2ACardFetcher(50*time.Millisecond, "").Fetch(context.Background(), srv.URL, false)
	require.Error(t, err)
}

// External URLs are user-supplied: the guarded client refuses hosts that resolve to loopback.
func TestA2ACardFetcherGuardedModeRejectsLoopback(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(validTestCard))
	}))
	t.Cleanup(srv.Close)
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)

	for _, url := range []string{srv.URL, "http://localhost:" + port} {
		_, err := newA2ACardFetcher(time.Second, "").Fetch(context.Background(), url, true)
		require.Error(t, err, url)
		assert.Contains(t, err.Error(), "agent card URL is not allowed", url)
	}
	assert.Equal(t, int32(0), hits.Load())
}

type fakeFetchCall struct {
	URL     string
	Guarded bool
}

// fakeA2ACardFetcher is the hand-written stub for the in-package A2ACardFetcher.
type fakeA2ACardFetcher struct {
	FetchFunc func(ctx context.Context, url string, guarded bool) (json.RawMessage, error)
	calls     []fakeFetchCall
}

func (f *fakeA2ACardFetcher) Fetch(ctx context.Context, url string, guarded bool) (json.RawMessage, error) {
	f.calls = append(f.calls, fakeFetchCall{URL: url, Guarded: guarded})
	return f.FetchFunc(ctx, url, guarded)
}

func TestA2ACardFetcherPlatformDoesNotFollowRedirectsToOtherHosts(t *testing.T) {
	var hit atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit.Store(true)
		_, _ = w.Write([]byte(validTestCard))
	}))
	t.Cleanup(other.Close)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusFound)
	}))
	t.Cleanup(redirector.Close)

	_, err := newA2ACardFetcher(time.Second, "").Fetch(context.Background(), redirector.URL+a2aAgentCardPath, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 302")
	assert.False(t, hit.Load())
}

// The error becomes last_error, which any project reader can see.
func TestA2ACardFetcherGuardedModeHidesWhyAHostIsRefused(t *testing.T) {
	f := newA2ACardFetcher(time.Second, "")
	_, internal := f.Fetch(context.Background(), "https://10.0.0.1/c", true)
	_, unresolvable := f.Fetch(context.Background(), "https://nope.invalid/c", true)

	require.Error(t, internal)
	require.Error(t, unresolvable)
	assert.Equal(t, internal.Error(), unresolvable.Error())
	assert.NotContains(t, unresolvable.Error(), "lookup")
}

type recordingTransport struct{ hosts []string }

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.hosts = append(rt.hosts, req.URL.Host)
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(validTestCard)),
		Request:    req,
	}, nil
}

func TestA2ACardFetcherSendsEachModeThroughItsOwnClient(t *testing.T) {
	plain, guarded := &recordingTransport{}, &recordingTransport{}
	f := &a2aCardFetcher{plain: &http.Client{Transport: plain}, guarded: &http.Client{Transport: guarded}}

	_, err := f.Fetch(context.Background(), "https://93.184.215.14/card", true)
	require.NoError(t, err)
	_, err = f.Fetch(context.Background(), "http://gw.example/card", false)
	require.NoError(t, err)

	assert.Equal(t, []string{"93.184.215.14"}, guarded.hosts, "tenant URLs use the guarded client")
	assert.Equal(t, []string{"gw.example"}, plain.hosts, "platform URLs use the plain client")
}

func redirectAllowed(t *testing.T, from, to string) bool {
	t.Helper()
	origin, err := http.NewRequest(http.MethodGet, from, nil)
	require.NoError(t, err)
	next, err := http.NewRequest(http.MethodGet, to, nil)
	require.NoError(t, err)
	return sameHostRedirectOnly(next, []*http.Request{origin}) == nil
}

// Gateways commonly force HTTPS; that must not fail the platform fetch.
func TestA2ACardFetcherPlatformRedirectPolicy(t *testing.T) {
	assert.True(t, redirectAllowed(t, "http://gw.example/a", "http://gw.example/b"))
	assert.True(t, redirectAllowed(t, "http://gw.example/a", "https://gw.example/a"), "same-host upgrade")
	assert.True(t, redirectAllowed(t, "http://gw.example:80/a", "https://gw.example/a"), "default ports")
	assert.False(t, redirectAllowed(t, "https://gw.example/a", "http://gw.example/a"), "downgrade")
	assert.False(t, redirectAllowed(t, "http://gw.example/a", "https://other.example/a"))
	assert.False(t, redirectAllowed(t, "http://gw.example:8080/a", "https://gw.example:9443/a"), "upgrade to another port")
}
