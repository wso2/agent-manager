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

// Package api serves the OpenChoreo Observer HTTP API on top of an observer.Client.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/deployments/dev/observer-lite/store"
)

const maxBodyBytes = 1 << 20

// NewHandler serves the Observer API routes that amp-observer calls, backed by c.
func NewHandler(c observer.Client) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /api/v1alpha1/traces/query", postJSON(func(r *http.Request, req observer.TracesQueryRequest) (any, error) {
		return c.QueryTraces(r.Context(), req)
	}))
	mux.HandleFunc("POST /api/v1alpha1/traces/{traceId}/spans/query", postJSON(func(r *http.Request, req observer.TracesQueryRequest) (any, error) {
		return c.QueryTraceSpans(r.Context(), r.PathValue("traceId"), req)
	}))
	mux.HandleFunc("GET /api/v1alpha1/traces/{traceId}/spans/{spanId}", func(w http.ResponseWriter, r *http.Request) {
		respond(w, outcome(c.GetSpanDetails(r.Context(), r.PathValue("traceId"), r.PathValue("spanId"))))
	})
	mux.HandleFunc("POST /api/v1/logs/query", postJSON(func(r *http.Request, req observer.LogsQueryRequest) (any, error) {
		scope, err := typedLogsScope(req.SearchScope)
		if err != nil {
			return nil, badRequest{err}
		}
		req.SearchScope = scope
		return c.QueryLogs(r.Context(), req)
	}))
	mux.HandleFunc("POST /api/v1/metrics/query", postJSON(func(r *http.Request, req observer.MetricsQueryRequest) (any, error) {
		return c.QueryMetrics(r.Context(), req)
	}))
	return mux
}

type badRequest struct{ error }

type result struct {
	body any
	err  error
}

func outcome[T any](v *T, err error) result { return result{body: v, err: err} }

func postJSON[Req any](call func(r *http.Request, req Req) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req Req
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
			http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		body, err := call(r, req)
		respond(w, result{body: body, err: err})
	}
}

func respond(w http.ResponseWriter, res result) {
	var bad badRequest
	switch {
	case errors.As(res.err, &bad):
		http.Error(w, bad.Error(), http.StatusBadRequest)
	case errors.Is(res.err, store.ErrSpanNotFound):
		http.Error(w, res.err.Error(), http.StatusNotFound)
	case res.err != nil:
		http.Error(w, res.err.Error(), http.StatusInternalServerError)
	default:
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(res.body); err != nil {
			slog.Error("encode response", "error", err)
		}
	}
}

// typedLogsScope turns the raw scope object into the struct the observer client expects.
func typedLogsScope(raw any) (any, error) {
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("searchScope must be an object")
	}
	encoded, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	if _, isWorkflow := obj["workflowRunName"]; isWorkflow {
		var scope observer.WorkflowSearchScope
		return scope, json.Unmarshal(encoded, &scope)
	}
	var scope observer.ComponentSearchScope
	return scope, json.Unmarshal(encoded, &scope)
}
