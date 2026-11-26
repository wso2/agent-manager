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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/tracesv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store"
)

const (
	defaultLimit = 10
	maxLimit     = 1000
	// defaultExportLimit and maxExportSpans match the observer's export.
	defaultExportLimit = 100
	maxExportSpans     = 1000
	// exportBatch is how many traces' spans one export query reads: the
	// default export in one query, the largest in ten.
	exportBatch = 100
	// resourceAgent is the resource attribute naming a span's agent.
	resourceAgent = "agent-manager/agent"
)

type handlers struct {
	core   modkit.CoreAPI
	traces store.TraceStore
	log    *slog.Logger
}

// statusError is a request error with its HTTP status.
type statusError struct {
	status int
	msg    string
}

func (e *statusError) Error() string { return e.msg }

func badRequest(format string, args ...any) error {
	return &statusError{status: http.StatusBadRequest, msg: fmt.Sprintf(format, args...)}
}

func notFound(what string) error {
	return &statusError{status: http.StatusNotFound, msg: what + " not found"}
}

// scope resolves the caller's organization and the named project and agent.
// The organization always comes from the request's sign-in, never its query.
func (h handlers) scope(r *http.Request, project, agent string) (store.Scope, error) {
	ctx := r.Context()
	orgID := modkit.OrgID(ctx)
	if orgID == "" {
		return store.Scope{}, &statusError{status: http.StatusUnauthorized, msg: "the request has no organization"}
	}
	org, err := h.core.GetOrg(ctx, orgID)
	if err != nil {
		if errors.Is(err, modkit.ErrNotFound) {
			return store.Scope{}, notFound("organization")
		}
		return store.Scope{}, fmt.Errorf("get organization: %w", err)
	}
	s := store.Scope{OrgID: orgID, Names: store.Names{Org: org.Handle}}
	if project == "" {
		if agent != "" {
			return store.Scope{}, badRequest("agent needs project")
		}
		return s, nil
	}
	p, err := h.core.GetProjectByName(ctx, orgID, project)
	if err != nil {
		if errors.Is(err, modkit.ErrNotFound) {
			return store.Scope{}, notFound("project")
		}
		return store.Scope{}, fmt.Errorf("get project: %w", err)
	}
	s.ProjectID, s.Names.Project = p.ID, p.Name
	if agent == "" {
		return s, nil
	}
	a, err := h.core.GetAgentByName(ctx, orgID, project, agent)
	if err != nil {
		if errors.Is(err, modkit.ErrNotFound) {
			return store.Scope{}, notFound("agent")
		}
		return store.Scope{}, fmt.Errorf("get agent: %w", err)
	}
	s.AgentID, s.Names.Agent = a.ID, a.Name
	return s, nil
}

// traceQuery parses a trace list or export request: its agent's scope, and
// its window, filters and paging as a query.
func (h handlers) traceQuery(r *http.Request, limit int) (store.Scope, window, tracesv1.TraceQuery, error) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return store.Scope{}, window{}, tracesv1.TraceQuery{}, badRequest("invalid query: %v", err)
	}
	project, agent := query.Get("project"), query.Get("agent")
	if project == "" || agent == "" {
		return store.Scope{}, window{}, tracesv1.TraceQuery{}, badRequest("project and agent are required")
	}
	win, err := parseWindow(query, "desc", limit)
	if err != nil {
		return store.Scope{}, window{}, tracesv1.TraceQuery{}, err
	}
	filters, err := parseTraceFilters(query)
	if err != nil {
		return store.Scope{}, window{}, tracesv1.TraceQuery{}, err
	}
	s, err := h.scope(r, project, agent)
	if err != nil {
		return store.Scope{}, window{}, tracesv1.TraceQuery{}, err
	}
	q := tracesv1.TraceQuery{
		StartTime: win.start, EndTime: win.end, Limit: win.limit, SortOrder: win.sort,
		Filters: filters, Cursor: query.Get("cursor"),
	}
	return s, win, q, nil
}

// listTraces serves GET /traces.
func (h handlers) listTraces(w http.ResponseWriter, r *http.Request) {
	s, window, q, err := h.traceQuery(r, defaultLimit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	page, err := h.traces.QueryTraces(r.Context(), s, q)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	resp := TraceListResponse{
		Traces:     make([]TraceOverview, 0, len(page.Traces)),
		Pagination: tracePagination(window, page),
	}
	for _, t := range page.Traces {
		resp.Traces = append(resp.Traces, overview(t))
	}
	h.ok(w, r, resp)
}

// exportTraces serves GET /traces/export: the window's traces, each
// with its spans and their attributes.
func (h handlers) exportTraces(w http.ResponseWriter, r *http.Request) {
	s, window, q, err := h.traceQuery(r, defaultExportLimit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if q.Cursor != "" {
		h.fail(w, r, badRequest("cursor is not supported by the trace export"))
		return
	}
	ctx := r.Context()
	page, err := h.traces.QueryTraces(ctx, s, q)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	resp := TraceExportResponse{
		Traces:     make([]FullTrace, 0, len(page.Traces)),
		Pagination: tracePagination(window, page),
	}
	// The spans are read as the list counted them, exportBatch traces a query.
	for batch := range slices.Chunk(page.Traces, exportBatch) {
		ids := make([]string, 0, len(batch))
		for _, t := range batch {
			ids = append(ids, t.TraceID)
		}
		pages, err := h.traces.QuerySpansByTrace(ctx, s, ids, tracesv1.SpanQuery{
			StartTime: window.start, EndTime: window.end, Limit: maxExportSpans, SortOrder: "asc", IncludeAttributes: true,
		})
		if err != nil {
			h.log.WarnContext(ctx, "observability: export left out traces",
				"org", modkit.OrgID(ctx), "traces", len(ids), "error", err)
			resp.FailedTraceIDs = append(resp.FailedTraceIDs, ids...)
			continue
		}
		for _, t := range batch {
			spans := pages[t.TraceID]
			if spans == nil {
				spans = &tracesv1.SpanPage{}
			}
			full := FullTrace{TraceOverview: overview(t), Spans: make([]Span, 0, len(spans.Spans))}
			for _, sp := range spans.Spans {
				full.Spans = append(full.Spans, toSpan(t.TraceID, tracesv1.SpanDetails(sp)))
			}
			resp.SpansTruncated = resp.SpansTruncated || len(spans.Spans) < spans.Total
			resp.Traces = append(resp.Traces, full)
		}
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q",
		"traces-export-"+time.Now().UTC().Format("20060102-150405")+".json"))
	w.Header().Set("Cache-Control", "no-store")
	h.ok(w, r, resp)
}

// listSpans serves GET /traces/{traceId}/spans.
func (h handlers) listSpans(w http.ResponseWriter, r *http.Request) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		h.fail(w, r, badRequest("invalid query: %v", err))
		return
	}
	window, err := parseWindow(query, "asc", defaultLimit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	s, err := h.scope(r, query.Get("project"), query.Get("agent"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	page, err := h.traces.QueryTraceSpans(r.Context(), s, r.PathValue("traceId"), tracesv1.SpanQuery{
		StartTime: window.start, EndTime: window.end, Limit: window.limit, SortOrder: window.sort,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	resp := SpanListResponse{
		Spans:      make([]SpanSummary, 0, len(page.Spans)),
		Pagination: Pagination{Limit: window.limit, Total: page.Total},
	}
	for _, sp := range page.Spans {
		sum := SpanSummary{
			SpanID: sp.SpanID, SpanName: sp.SpanName, SpanKind: store.SpanKind(sp.SpanName), ParentSpanID: sp.ParentSpanID,
			StartTime: sp.StartTime, EndTime: sp.EndTime, DurationNs: sp.DurationNs, Error: isError(sp.Status),
		}
		if sp.Status != nil {
			sum.StatusMessage = sp.Status.Message
		}
		resp.Spans = append(resp.Spans, sum)
	}
	h.ok(w, r, resp)
}

// getSpan serves GET /traces/{traceId}/spans/{spanId}.
func (h handlers) getSpan(w http.ResponseWriter, r *http.Request) {
	s, err := h.scope(r, "", "")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	traceID := r.PathValue("traceId")
	d, err := h.traces.GetSpanDetails(r.Context(), s, traceID, r.PathValue("spanId"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.ok(w, r, toSpan(traceID, *d))
}

// tracePagination is a trace page's paging state, as the store reports it.
func tracePagination(w window, page *tracesv1.TracePage) TracePagination {
	p := TracePagination{Limit: w.limit, Total: page.Total, NextCursor: page.NextCursor}
	if page.Truncated {
		p.Truncated, p.LookedBackTo = true, formatTime(page.LookedBackTo)
	}
	return p
}

// overview is a trace's list entry.
func overview(t tracesv1.Trace) TraceOverview {
	return TraceOverview{
		TraceID: t.TraceID, RootSpanID: t.RootSpanID, RootSpanName: t.RootSpanName,
		RootSpanKind: store.SpanKind(t.RootSpanName),
		StartTime:    formatTime(t.StartTime), EndTime: formatTime(t.EndTime),
		DurationInNanos: t.DurationNs, SpanCount: t.SpanCount,
	}
}

// toSpan is a span with its attributes, in the observer's shape.
func toSpan(traceID string, d tracesv1.SpanDetails) Span {
	span := Span{
		TraceID: traceID, SpanID: d.SpanID, ParentSpanID: d.ParentSpanID, Name: d.SpanName,
		StartTime: d.StartTime, EndTime: d.EndTime, DurationInNanos: d.DurationNs, Kind: d.Kind,
		Attributes: d.Attributes, Resource: d.ResourceAttributes,
		AmpAttributes: &AmpAttributes{Kind: store.SpanKind(d.SpanName), Status: &SpanStatus{Error: isError(d.Status)}},
	}
	span.Service, _ = d.ResourceAttributes[resourceAgent].(string)
	if d.Status != nil {
		span.Status, span.StatusMessage = strings.ToLower(d.Status.Code), d.Status.Message
		span.AmpAttributes.Status.Message = d.Status.Message
	}
	return span
}

// fail writes err with its status: a request error as is, a store error by kind.
func (h handlers) fail(w http.ResponseWriter, r *http.Request, err error) {
	var se *statusError
	switch {
	case errors.As(err, &se):
		writeError(w, se.status, se.msg)
	case errors.Is(err, tracesv1.ErrInvalidCursor):
		writeError(w, http.StatusBadRequest, "invalid cursor")
	case errors.Is(err, tracesv1.ErrNotFound):
		writeError(w, http.StatusNotFound, "trace or span not found")
	case errors.Is(err, tracesv1.ErrMissingOrg):
		writeError(w, http.StatusUnauthorized, "the request has no organization")
	case errors.Is(err, tracesv1.ErrForeignOrg):
		writeError(w, http.StatusForbidden, "the scope is not the caller's organization")
	default:
		h.log.ErrorContext(r.Context(), "observability: trace read failed",
			"org", modkit.OrgID(r.Context()), "path", r.URL.Path, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to read traces")
	}
}

// window is a request's time window, page size and order.
type window struct {
	start, end time.Time
	limit      int
	sort       string
}

func parseWindow(query url.Values, defaultSort string, defaultLimit int) (window, error) {
	var w window
	var err error
	if w.start, err = parseTime(query.Get("startTime")); err != nil {
		return w, badRequest("invalid startTime: %v", err)
	}
	if w.end, err = parseTime(query.Get("endTime")); err != nil {
		return w, badRequest("invalid endTime: %v", err)
	}
	w.limit = defaultLimit
	if s := query.Get("limit"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v <= 0 {
			return w, badRequest("limit must be a positive integer")
		}
		w.limit = min(v, maxLimit)
	}
	switch w.sort = query.Get("sortOrder"); w.sort {
	case "":
		w.sort = defaultSort
	case "asc", "desc":
	default:
		return w, badRequest("sortOrder must be 'asc' or 'desc'")
	}
	return w, nil
}

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("value is required")
	}
	return time.Parse(time.RFC3339, s)
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func isError(s *tracesv1.SpanStatus) bool {
	return s != nil && strings.EqualFold(s.Code, "error")
}

// ok writes v as a 200 JSON response, or a 500 if v can't be encoded.
func (h handlers) ok(w http.ResponseWriter, r *http.Request, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		h.log.ErrorContext(r.Context(), "observability: encode response failed",
			"org", modkit.OrgID(r.Context()), "path", r.URL.Path, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to encode the response")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(append(b, '\n'))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: "error", Message: msg})
}
