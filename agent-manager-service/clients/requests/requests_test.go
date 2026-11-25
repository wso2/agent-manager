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
package requests

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fastRetry keeps retry tests quick; the backoff stays real but tiny.
var fastRetry = RequestRetryConfig{
	RetryWaitMin:     time.Millisecond,
	RetryWaitMax:     2 * time.Millisecond,
	RetryAttemptsMax: 2,
	AttemptTimeout:   time.Second,
}

type thing struct {
	Name string `json:"name"`
}

func TestSendRequestBuildsTheRequest(t *testing.T) {
	var got *http.Request
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("X-Trace", "abc")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"name":"created"}`))
	}))
	defer srv.Close()

	req := (&HttpRequest{Name: "test.create", URL: srv.URL + "/things?keep=1", Method: http.MethodPost}).
		SetHeader("Authorization", "Bearer t").
		SetQuery("q", "a b").
		SetJson(thing{Name: "x"}).
		SetHost("virtual.example")
	res := SendRequest(context.Background(), srv.Client(), req)

	var out thing
	require.NoError(t, res.ScanResponse(&out, http.StatusCreated))
	assert.Equal(t, "created", out.Name)
	assert.Equal(t, "abc", res.GetHeader("X-Trace"))
	assert.Equal(t, http.StatusCreated, res.StatusCode())
	require.NoError(t, res.Err())

	assert.Equal(t, http.MethodPost, got.Method)
	assert.Equal(t, "/things", got.URL.Path)
	assert.Equal(t, "1", got.URL.Query().Get("keep"), "query already in the URL is kept")
	assert.Equal(t, "a b", got.URL.Query().Get("q"))
	assert.Equal(t, "Bearer t", got.Header.Get("Authorization"))
	assert.Equal(t, "application/json", got.Header.Get("Content-Type"))
	assert.Equal(t, "virtual.example", got.Host, "Host override reaches the wire")
	assert.JSONEq(t, `{"name":"x"}`, gotBody)
}

func TestSendRequestFormData(t *testing.T) {
	var form url.Values
	var ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		form, ctype = r.PostForm, r.Header.Get("Content-Type")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	req := (&HttpRequest{URL: srv.URL, Method: http.MethodPost}).
		SetFormData(map[string]string{"grant_type": "client_credentials", "scope": "a b"})
	require.NoError(t, SendRequest(context.Background(), srv.Client(), req).Err())
	assert.Equal(t, "application/x-www-form-urlencoded", ctype)
	assert.Equal(t, "client_credentials", form.Get("grant_type"))
	assert.Equal(t, "a b", form.Get("scope"))
}

func TestScanResponseErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/conflict":
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"exists"}`))
		case "/empty":
			w.WriteHeader(http.StatusNotFound)
		default:
			_, _ = w.Write([]byte(`not json`))
		}
	}))
	defer srv.Close()
	send := func(path string) *Result {
		return SendRequest(context.Background(), srv.Client(), &HttpRequest{URL: srv.URL + path, Method: http.MethodGet})
	}

	var out thing
	err := send("/conflict").ScanResponse(&out, http.StatusOK)
	var httpErr *HttpError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusConflict, httpErr.StatusCode)
	assert.Equal(t, `{"error":"exists"}`, httpErr.Body)
	assert.Equal(t, `failed with status code 409 [{"error":"exists"}]`, err.Error())

	err = send("/empty").ScanResponse(&out, http.StatusOK)
	assert.EqualError(t, err, "failed with status code 404")

	err = send("/bad").ScanResponse(&out, http.StatusOK)
	require.ErrorContains(t, err, "failed to decode response body for status 200")

	assert.ErrorContains(t, send("/x").ScanResponse(out, http.StatusOK), "non-nil pointer expected")
	assert.ErrorContains(t, send("/x").ScanResponse(nil, http.StatusOK), "non-nil pointer expected")
	assert.ErrorContains(t, (&Result{}).ScanResponse(&out, http.StatusOK), "unexpected nil response")

	wrapped := &HttpError{StatusCode: 502, err: errors.New("eof")}
	assert.Equal(t, "failed with status code 502 and internal error eof", wrapped.Error())
}

type failingClient struct{ err error }

func (f failingClient) Do(*http.Request) (*http.Response, error) { return nil, f.err }

func TestSendRequestFailures(t *testing.T) {
	errDial := errors.New("dial refused")
	res := SendRequest(context.Background(), failingClient{err: errDial}, &HttpRequest{URL: "http://x", Method: http.MethodGet})
	require.ErrorIs(t, res.Err(), errDial)
	require.ErrorIs(t, res.ScanResponse(&thing{}, http.StatusOK), errDial)
	assert.Equal(t, 0, res.StatusCode())
	assert.Empty(t, res.GetHeader("X"))

	// An unencodable body fails before anything is sent.
	req := (&HttpRequest{URL: "http://x", Method: http.MethodPost}).SetJson(make(chan int))
	res = SendRequest(context.Background(), failingClient{err: errors.New("must not be called")}, req)
	require.ErrorContains(t, res.Err(), "failed to encode request body")

	res = SendRequest(context.Background(), failingClient{}, &HttpRequest{URL: "http://x", Method: "BAD METHOD"})
	require.ErrorContains(t, res.Err(), "failed to build http request")
}

type statusSequence struct {
	codes []int
	calls atomic.Int32
	t     *testing.T
	body  string
}

func (s *statusSequence) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		i := int(s.calls.Add(1)) - 1
		b, _ := io.ReadAll(r.Body)
		if s.body != "" {
			assert.Equal(s.t, s.body, string(b), "body is replayed on attempt %d", i+1)
		}
		code := s.codes[min(i, len(s.codes)-1)]
		w.WriteHeader(code)
		_, _ = w.Write([]byte("attempt body"))
	}
}

func TestRetryableClientRetriesTransientStatus(t *testing.T) {
	seq := &statusSequence{codes: []int{http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusOK}, t: t, body: "payload"}
	srv := httptest.NewServer(seq.handler())
	defer srv.Close()

	c := NewRetryableHTTPClient(srv.Client(), fastRetry)
	req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader("payload"))
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(3), seq.calls.Load())
	b, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "attempt body", string(b), "body stays readable after the attempt context ends")
}

func TestRetryableClientReturnsLastRetryableResponse(t *testing.T) {
	seq := &statusSequence{codes: []int{http.StatusTooManyRequests}, t: t}
	srv := httptest.NewServer(seq.handler())
	defer srv.Close()

	resp, err := NewRetryableHTTPClient(srv.Client(), fastRetry).Do(mustReq(t, http.MethodPost, srv.URL))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	assert.Equal(t, int32(fastRetry.RetryAttemptsMax+1), seq.calls.Load())
	b, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "attempt body", string(b))
}

// 500 is retried only for idempotent methods; a POST that failed may have been applied.
func TestRetryableClientDefaultRetryPolicy(t *testing.T) {
	for _, tc := range []struct {
		method    string
		code      int
		wantCalls int32
	}{
		{http.MethodGet, http.StatusInternalServerError, 3},
		{http.MethodDelete, http.StatusInternalServerError, 3},
		{http.MethodPost, http.StatusInternalServerError, 1},
		{http.MethodPost, http.StatusServiceUnavailable, 3},
		{http.MethodGet, http.StatusNotFound, 1},
	} {
		t.Run(tc.method+" "+http.StatusText(tc.code), func(t *testing.T) {
			seq := &statusSequence{codes: []int{tc.code}, t: t}
			srv := httptest.NewServer(seq.handler())
			defer srv.Close()
			cfg := fastRetry
			resp, err := NewRetryableHTTPClient(srv.Client(), cfg).Do(mustReq(t, tc.method, srv.URL))
			require.NoError(t, err)
			_ = resp.Body.Close()
			assert.Equal(t, tc.code, resp.StatusCode)
			assert.Equal(t, tc.wantCalls, seq.calls.Load())
		})
	}
}

type flakyClient struct {
	calls atomic.Int32
	fails int32
	err   error
}

func (f *flakyClient) Do(r *http.Request) (*http.Response, error) {
	if f.calls.Add(1) <= f.fails {
		return nil, f.err
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Request: r}, nil
}

func TestRetryableClientTransportErrors(t *testing.T) {
	errNet := errors.New("connection reset")

	recovers := &flakyClient{fails: 2, err: errNet}
	resp, err := NewRetryableHTTPClient(recovers, fastRetry).Do(mustReq(t, http.MethodGet, "http://x"))
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, int32(3), recovers.calls.Load())

	never := &flakyClient{fails: 100, err: errNet}
	_, err = NewRetryableHTTPClient(never, fastRetry).Do(mustReq(t, http.MethodGet, "http://x"))
	require.ErrorIs(t, err, errNet)
	require.ErrorContains(t, err, "request failed after 3 attempts")
}

func TestRetryableClientAttemptTimeout(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		<-r.Context().Done()
	}))
	defer srv.Close()

	cfg := fastRetry
	cfg.AttemptTimeout = 20 * time.Millisecond
	cfg.RetryAttemptsMax = 1
	_, err := NewRetryableHTTPClient(srv.Client(), cfg).Do(mustReq(t, http.MethodGet, srv.URL))
	require.ErrorContains(t, err, "request timed out after 2 attempts")
	assert.Equal(t, int32(2), calls.Load())
}

func TestRetryableClientStopsWhenCallerContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	never := &flakyClient{fails: 100, err: errors.New("boom")}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://x", nil)
	_, err := NewRetryableHTTPClient(never, fastRetry).Do(req)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int32(1), never.calls.Load(), "no retry after the caller gives up")

	// Cancelled while waiting between attempts.
	ctx, cancel = context.WithCancel(context.Background())
	slow := fastRetry
	slow.RetryWaitMin, slow.RetryWaitMax = time.Hour, time.Hour
	failing := &flakyClient{fails: 100, err: errors.New("boom")}
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, "http://x", nil)
	time.AfterFunc(20*time.Millisecond, cancel)
	_, err = NewRetryableHTTPClient(failing, slow).Do(req)
	require.ErrorContains(t, err, "context cancelled during retry wait")
	require.ErrorIs(t, err, context.Canceled)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestRetryableClientUnreadableRequestBody(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://x", io.NopCloser(errReader{}))
	_, err := NewRetryableHTTPClient(&flakyClient{}, fastRetry).Do(req)
	require.ErrorContains(t, err, "failed to read request body")
}

func TestNewRetryableHTTPClientDefaults(t *testing.T) {
	c := NewRetryableHTTPClient(nil)
	assert.IsType(t, &http.Client{}, c.client)

	cfg := c.config.getRetryConfig(&HttpRequest{Method: http.MethodGet})
	assert.Equal(t, DefaultRetryWaitMin, cfg.RetryWaitMin)
	assert.Equal(t, DefaultRetryWaitMax, cfg.RetryWaitMax)
	assert.Equal(t, DefaultRetryAttemptsMax, cfg.RetryAttemptsMax)
	assert.Equal(t, DefaultAttemptTimeout, cfg.AttemptTimeout)
	assert.True(t, cfg.RetryOnStatus(http.StatusInternalServerError))

	custom := RequestRetryConfig{RetryOnStatus: func(s int) bool { return s == http.StatusTeapot }}
	got := custom.getRetryConfig(&HttpRequest{Method: http.MethodGet})
	assert.True(t, got.RetryOnStatus(http.StatusTeapot))
	assert.False(t, got.RetryOnStatus(http.StatusServiceUnavailable), "a custom policy replaces the default")
}

func TestCalculateBackoff(t *testing.T) {
	for attempt := 1; attempt <= 6; attempt++ {
		base := min(time.Second*time.Duration(1<<(attempt-1)), 10*time.Second)
		for range 20 {
			d := calculateBackoff(time.Second, 10*time.Second, attempt)
			assert.GreaterOrEqual(t, d, base/2)
			assert.Less(t, d, base+1)
		}
	}
	assert.Equal(t, time.Duration(1), calculateBackoff(1, 10, 1), "a base too small to halve is returned as is")
}

func mustReq(t *testing.T, method, u string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, u, nil)
	require.NoError(t, err)
	return req
}
