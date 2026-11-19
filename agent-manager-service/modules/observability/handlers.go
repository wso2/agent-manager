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
	"strconv"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/tracesv1"
)

const (
	defaultLimit = 10
	maxLimit     = 1000
	// resourceAgent is the resource attribute naming a span's agent.
	resourceAgent = "agent-manager/agent"
)

// unsupportedParams are observer trace list parameters this module does not
// serve yet; a request with one is refused rather than answered unfiltered.
var unsupportedParams = []string{
	"status", "minDurationMs", "minTokens", "minSpanCount", "model",
	"conversationId", "tool", "toolError", "mcpServer", "missing", "cursor",
}

type handlers struct {
	core   modkit.CoreAPI
	traces tracesv1.Query
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
func (h handlers) scope(r *http.Request, project, agent string) (tracesv1.Scope, error) {
	ctx := r.Context()
	orgID := modkit.OrgID(ctx)
	if orgID == "" {
		return tracesv1.Scope{}, &statusError{status: http.StatusUnauthorized, msg: "the request has no organization"}
	}
	org, err := h.core.GetOrg(ctx, orgID)
	if err != nil {
		if errors.Is(err, modkit.ErrNotFound) {
			return tracesv1.Scope{}, notFound("organization")
		}
		return tracesv1.Scope{}, fmt.Errorf("get organization: %w", err)
	}
	s := tracesv1.Scope{OrgID: orgID, Names: tracesv1.Names{Org: org.Handle}}
	if project == "" {
		if agent != "" {
			return tracesv1.Scope{}, badRequest("agent needs project")
		}
		return s, nil
	}
	projects, err := h.core.ListProjects(ctx, orgID)
	if err != nil {
		return tracesv1.Scope{}, fmt.Errorf("list projects: %w", err)
	}
	for _, p := range projects {
		if p.Name == project {
			s.ProjectID, s.Names.Project = p.ID, p.Name
		}
	}
	if s.ProjectID == "" {
		return tracesv1.Scope{}, notFound("project")
	}
	if agent == "" {
		return s, nil
	}
	agents, err := h.core.ListAgents(ctx, orgID, s.ProjectID)
	if err != nil {
		return tracesv1.Scope{}, fmt.Errorf("list agents: %w", err)
	}
	for _, a := range agents {
		if a.Name == agent {
			s.AgentID, s.Names.Agent = a.ID, a.Name
		}
	}
	if s.AgentID == "" {
		return tracesv1.Scope{}, notFound("agent")
	}
	return s, nil
}

// listTraces serves GET /api/v1/traces.
func (h handlers) listTraces(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	for _, p := range unsupportedParams {
		if query.Has(p) {
			h.fail(w, r, badRequest("%s is not supported by this trace store", p))
			return
		}
	}
	project, agent := query.Get("project"), query.Get("agent")
	if project == "" || agent == "" {
		h.fail(w, r, badRequest("project and agent are required"))
		return
	}
	window, err := parseWindow(query, "desc")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	s, err := h.scope(r, project, agent)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	page, err := h.traces.QueryTraces(r.Context(), s, tracesv1.TraceQuery{
		StartTime: window.start, EndTime: window.end, Limit: window.limit, SortOrder: window.sort,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	resp := TraceOverviewResponse{Traces: make([]TraceOverview, 0, len(page.Traces)), TotalCount: page.Total}
	for _, t := range page.Traces {
		resp.Traces = append(resp.Traces, TraceOverview{
			TraceID: t.TraceID, RootSpanID: t.RootSpanID, RootSpanName: t.RootSpanName,
			RootSpanKind: spanKind(t.RootSpanName),
			StartTime:    formatTime(t.StartTime), EndTime: formatTime(t.EndTime),
			DurationInNanos: t.DurationNs, SpanCount: t.SpanCount,
		})
	}
	resp.LookedBackTo = formatTime(window.edge())
	if n := len(page.Traces); n > 0 && n < page.Total {
		resp.LookedBackTo = formatTime(page.Traces[n-1].StartTime)
	}
	writeJSON(w, http.StatusOK, resp)
}

// listSpans serves GET /api/v1/traces/{traceId}/spans.
func (h handlers) listSpans(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	window, err := parseWindow(query, "asc")
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
	resp := SpanListResponse{Spans: make([]SpanSummary, 0, len(page.Spans)), TotalCount: page.Total}
	for _, sp := range page.Spans {
		sum := SpanSummary{
			SpanID: sp.SpanID, SpanName: sp.SpanName, SpanKind: spanKind(sp.SpanName), ParentSpanID: sp.ParentSpanID,
			StartTime: sp.StartTime, EndTime: sp.EndTime, DurationNs: sp.DurationNs, Error: isError(sp.Status),
		}
		if sp.Status != nil {
			sum.StatusMessage = sp.Status.Message
		}
		resp.Spans = append(resp.Spans, sum)
	}
	writeJSON(w, http.StatusOK, resp)
}

// getSpan serves GET /api/v1/traces/{traceId}/spans/{spanId}.
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
	span := Span{
		TraceID: traceID, SpanID: d.SpanID, ParentSpanID: d.ParentSpanID, Name: d.SpanName,
		StartTime: d.StartTime, EndTime: d.EndTime, DurationInNanos: d.DurationNs, Kind: d.Kind,
		Attributes: d.Attributes, Resource: d.ResourceAttributes,
		AmpAttributes: &AmpAttributes{Kind: spanKind(d.SpanName), Status: &SpanStatus{Error: isError(d.Status)}},
	}
	span.Service, _ = d.ResourceAttributes[resourceAgent].(string)
	if d.Status != nil {
		span.Status, span.StatusMessage = strings.ToLower(d.Status.Code), d.Status.Message
		span.AmpAttributes.Status.Message = d.Status.Message
	}
	writeJSON(w, http.StatusOK, span)
}

// fail writes err with its status: a request error as is, a store error by kind.
func (h handlers) fail(w http.ResponseWriter, r *http.Request, err error) {
	var se *statusError
	switch {
	case errors.As(err, &se):
		writeError(w, se.status, se.msg)
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

// edge is where a walk in the window's order ends.
func (w window) edge() time.Time {
	if w.sort == "asc" {
		return w.end
	}
	return w.start
}

func parseWindow(query url.Values, defaultSort string) (window, error) {
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: "error", Message: msg})
}
