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

package main

import (
	"context"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

func TestRunServesAndShutsDown(t *testing.T) {
	apiLn, ingestLn := listen(t), listen(t)
	var ingested atomic.Int32
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})
	ingest := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/traces" {
			ingested.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	})

	var closeCalls atomic.Int32
	var apiStoppedAtClose bool
	apiAddr := apiLn.Addr().String()
	closeStore := func() error {
		closeCalls.Add(1)
		_, err := net.DialTimeout("tcp", apiAddr, 200*time.Millisecond)
		apiStoppedAtClose = err != nil
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, apiLn, ingestLn, api, ingest, closeStore) }()

	resp, err := http.Get("http://" + apiAddr + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", resp.StatusCode)
	}

	resp, err = http.Post("http://"+ingestLn.Addr().String()+"/v1/traces", "application/x-protobuf", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if ingested.Load() != 1 {
		t.Fatalf("ingest handler calls = %d, want 1", ingested.Load())
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after cancel")
	}
	if closeCalls.Load() != 1 {
		t.Fatalf("closeStore calls = %d, want 1", closeCalls.Load())
	}
	if !apiStoppedAtClose {
		t.Fatal("closeStore ran before the API server stopped")
	}
}

func TestRunReportsServeError(t *testing.T) {
	apiLn, ingestLn := listen(t), listen(t)
	_ = ingestLn.Close()
	var closeCalls atomic.Int32
	closeStore := func() error { closeCalls.Add(1); return nil }
	h := http.NotFoundHandler()

	done := make(chan error, 1)
	go func() { done <- run(context.Background(), apiLn, ingestLn, h, h, closeStore) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("run returned nil, want serve error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after a serve error")
	}
	if closeCalls.Load() != 1 {
		t.Fatalf("closeStore calls = %d, want 1", closeCalls.Load())
	}
	if _, err := net.DialTimeout("tcp", apiLn.Addr().String(), 200*time.Millisecond); err == nil {
		t.Fatal("API server still accepting after run returned")
	}
}
