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

package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/agentmanager"
	"github.com/wso2/agent-manager/agent-manager-observer/controllers"
	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

// fakeScoreClient returns err, or every trace at score, and counts its calls.
type fakeScoreClient struct {
	score float64
	err   error
	calls int
}

// TraceScores answers from the fake's score or error.
func (f *fakeScoreClient) TraceScores(_ context.Context, _, _, _ string, _, _ time.Time, ids []string, _ string) (map[string]*float64, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	scores := make(map[string]*float64, len(ids))
	for _, id := range ids {
		scores[id] = &f.score
	}
	return scores, nil
}

// scoreTraceFake lists one trace inside baseParams' window.
func scoreTraceFake() *fakeObserverClient {
	return &fakeObserverClient{traces: []observer.TraceInfo{{
		TraceID: "trace-1", RootSpanID: "root-1",
		StartTime: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC), EndTime: time.Date(2026, 4, 2, 0, 0, 1, 0, time.UTC),
	}}}
}

// traceEndpoints are the two routes that take the trace filters.
var traceEndpoints = map[string]func(*Handler) http.HandlerFunc{
	"/api/v1/traces":        func(h *Handler) http.HandlerFunc { return h.GetTraceOverviews },
	"/api/v1/traces/export": func(h *Handler) http.HandlerFunc { return h.ExportTraces },
}

func f64(v float64) *float64 { return &v }

// minScore, maxScore and evaluator parse; bounds are in [0, 1], minScore is at
// most maxScore, and evaluator works alone.
func TestParseTraceFilters_Score(t *testing.T) {
	tests := []struct {
		query   string
		want    controllers.TraceFilters
		wantErr string
	}{
		{query: "maxScore=0.5", want: controllers.TraceFilters{MaxScore: f64(0.5)}},
		{query: "minScore=0", want: controllers.TraceFilters{MinScore: f64(0)}},
		{query: "minScore=1", want: controllers.TraceFilters{MinScore: f64(1)}},
		{query: "minScore=0.25&maxScore=0.75", want: controllers.TraceFilters{MinScore: f64(0.25), MaxScore: f64(0.75)}},
		{query: "minScore=0.5&maxScore=0.5", want: controllers.TraceFilters{MinScore: f64(0.5), MaxScore: f64(0.5)}},
		{query: "maxScore=0.5&evaluator=Helpfulness", want: controllers.TraceFilters{MaxScore: f64(0.5), Evaluator: "Helpfulness"}},
		{query: "minScore=", want: controllers.TraceFilters{}},
		{query: "minScore=-0.1", wantErr: "minScore must be a number between 0 and 1"},
		{query: "maxScore=1.01", wantErr: "maxScore must be a number between 0 and 1"},
		{query: "maxScore=abc", wantErr: "maxScore must be a number between 0 and 1"},
		{query: "maxScore=NaN", wantErr: "maxScore must be a number between 0 and 1"},
		{query: "minScore=Inf", wantErr: "minScore must be a number between 0 and 1"},
		{query: "minScore=0.8&maxScore=0.2", wantErr: "minScore must not be greater than maxScore"},
		{query: "evaluator=Helpfulness", want: controllers.TraceFilters{Evaluator: "Helpfulness"}},
		{query: "maxScore=0.5&evaluator=" + strings.Repeat("a", 257), wantErr: "evaluator must be at most 256 characters"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			q, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}
			got, err := parseTraceFilters(q)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filters = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// An invalid score filter is a 400 on the list and the export, before any call.
func TestTraceEndpoints_ScoreFiltersBadRequest(t *testing.T) {
	for _, query := range []string{"&maxScore=2", "&minScore=x", "&minScore=0.9&maxScore=0.1"} {
		for path, handler := range traceEndpoints {
			t.Run(path+query, func(t *testing.T) {
				scores := &fakeScoreClient{}
				h := NewHandler(controllers.NewTracingController(scoreTraceFake()).WithScoreClient(scores), nil)
				rec := httptest.NewRecorder()
				handler(h)(rec, httptest.NewRequest(http.MethodGet, path+"?"+baseParams()+query, nil))

				assertStatus(t, rec, http.StatusBadRequest)
				if scores.calls != 0 {
					t.Errorf("score client called %d times, want 0", scores.calls)
				}
			})
		}
	}
}

// Without a score client a score filter is a 503 with a generic message, and
// every other request still works.
func TestTraceEndpoints_ScoreFilterWithoutServiceURL(t *testing.T) {
	for path, handler := range traceEndpoints {
		t.Run(path, func(t *testing.T) {
			h := NewHandler(controllers.NewTracingController(&fakeObserverClient{}), nil)

			rec := httptest.NewRecorder()
			handler(h)(rec, httptest.NewRequest(http.MethodGet, path+"?"+baseParams()+"&maxScore=0.5", nil))
			assertStatus(t, rec, http.StatusServiceUnavailable)
			if body := rec.Body.String(); strings.Contains(body, "AGENT_MANAGER_SERVICE_URL") {
				t.Errorf("body names the config: %s", body)
			}

			for _, query := range []string{"", "&status=error", "&toolError=true", "&minDurationMs=0"} {
				rec := httptest.NewRecorder()
				handler(h)(rec, httptest.NewRequest(http.MethodGet, path+"?"+baseParams()+query, nil))
				assertStatus(t, rec, http.StatusOK)
			}
		})
	}
}

// A rejected token is a 403; any other lookup failure, a timeout included, is a 502.
func TestTraceEndpoints_ScoreLookupErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "forbidden", err: fmt.Errorf("agentmanager.TraceScores: status 403: %w", agentmanager.ErrForbidden), want: http.StatusForbidden},
		{name: "unauthorized", err: fmt.Errorf("agentmanager.TraceScores: status 401: %w", agentmanager.ErrForbidden), want: http.StatusForbidden},
		{name: "server error", err: errors.New("agentmanager.TraceScores: status 500"), want: http.StatusBadGateway},
		{name: "timeout", err: fmt.Errorf("agentmanager.TraceScores: %w", context.DeadlineExceeded), want: http.StatusBadGateway},
	}
	for _, tt := range tests {
		for path, handler := range traceEndpoints {
			t.Run(tt.name+" "+path, func(t *testing.T) {
				scores := &fakeScoreClient{err: tt.err}
				h := NewHandler(controllers.NewTracingController(scoreTraceFake()).WithScoreClient(scores), nil)
				rec := httptest.NewRecorder()
				handler(h)(rec, httptest.NewRequest(http.MethodGet, path+"?"+baseParams()+"&maxScore=0.5", nil))

				assertStatus(t, rec, tt.want)
				if scores.calls != 1 {
					t.Errorf("score client called %d times, want 1", scores.calls)
				}
				if strings.Contains(rec.Body.String(), "status") {
					t.Errorf("body echoes the service error: %s", rec.Body.String())
				}
			})
		}
	}
}

// A score in range keeps the trace, and one out of range drops it.
func TestGetTraceOverviews_ScoreFilterKeepsTraceInRange(t *testing.T) {
	for score, want := range map[float64]bool{0.4: true, 0.6: false} {
		t.Run(fmt.Sprint(score), func(t *testing.T) {
			h := NewHandler(controllers.NewTracingController(scoreTraceFake()).WithScoreClient(&fakeScoreClient{score: score}), nil)
			rec := httptest.NewRecorder()
			h.GetTraceOverviews(rec, httptest.NewRequest(http.MethodGet, "/api/v1/traces?"+baseParams()+"&maxScore=0.5", nil))

			assertStatus(t, rec, http.StatusOK)
			if got := strings.Contains(rec.Body.String(), "trace-1"); got != want {
				t.Errorf("trace-1 listed = %v, want %v (body: %s)", got, want, rec.Body.String())
			}
		})
	}
}
