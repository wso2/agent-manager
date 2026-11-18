//go:build integration

// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckHealthHTTP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ready", http.StatusFound)
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	redirect := httptest.NewServer(mux)
	defer redirect.Close()

	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer healthy.Close()

	healthyAddr := healthy.Listener.Addr().(*net.TCPAddr)
	redirectAddr := redirect.Listener.Addr().(*net.TCPAddr)
	require.NoError(t, checkHealth(t.Context(), healthyAddr.IP.String(), healthyAddr.Port))
	require.ErrorContains(t, checkHealth(t.Context(), redirectAddr.IP.String(), redirectAddr.Port), "redirects are disabled")
}

func TestCheckHealthIPv6WildcardReachesIPv4Listener(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	server := &httptest.Server{
		Listener: listener,
		Config: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})},
	}
	server.Start()
	defer server.Close()

	require.NoError(t, checkHealth(t.Context(), "[::]", listener.Addr().(*net.TCPAddr).Port))
}
