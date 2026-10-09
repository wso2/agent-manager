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

package controllers

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

// agentTraceScoresRequest builds a valid agent trace scores request plus extraQuery, for the org ou-org.
func agentTraceScoresRequest(extraQuery string) *http.Request {
	req := httptest.NewRequest(http.MethodGet,
		"/orgs/acme/projects/p1/agents/a1/scores?startTime=2026-10-01T00:00:00Z&endTime=2026-10-02T00:00:00Z"+extraQuery, nil)
	return req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-org"}))
}

func newAgentTraceScoresController(repo *repomocks.ScoreRepositoryMock) MonitorScoresController {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewMonitorScoresController(services.NewMonitorScoresService(repo, &repomocks.MonitorRepositoryMock{}, logger))
}

func traceIDList(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("trace-%d", i)
	}
	return ids
}

func TestGetAgentTraceScoresRejectsInvalidTraceIDs(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "more than 100 IDs", query: "&traceIds=" + strings.Join(traceIDList(101), ",")},
		{name: "empty list", query: "&traceIds="},
		{name: "empty ID", query: "&traceIds=a,,b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// GetAgentTraceScoresFunc is nil, so reaching the repository fails the test.
			ctrl := newAgentTraceScoresController(&repomocks.ScoreRepositoryMock{})
			rec := httptest.NewRecorder()

			ctrl.GetAgentTraceScores(rec, agentTraceScoresRequest(tt.query))

			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestGetAgentTraceScoresPassesFilters(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  repositories.AgentTraceScoreFilters
	}{
		{name: "no filters", query: "", want: repositories.AgentTraceScoreFilters{}},
		{
			name:  "trace IDs trimmed and deduplicated, with evaluator",
			query: "&traceIds=a,%20b%20,a&evaluator=Latency%20Check",
			want:  repositories.AgentTraceScoreFilters{TraceIDs: []string{"a", "b"}, EvaluatorName: "Latency Check"},
		},
		{
			name:  "exactly 100 IDs",
			query: "&traceIds=" + strings.Join(traceIDList(100), ","),
			want:  repositories.AgentTraceScoreFilters{TraceIDs: traceIDList(100)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &repomocks.ScoreRepositoryMock{
				GetAgentTraceScoresFunc: func(_, _, _ string, _, _ time.Time, _, _ int, _ string,
					_ repositories.AgentTraceScoreFilters,
				) ([]repositories.TraceAggregation, int, error) {
					return []repositories.TraceAggregation{}, 0, nil
				},
			}
			rec := httptest.NewRecorder()

			newAgentTraceScoresController(repo).GetAgentTraceScores(rec, agentTraceScoresRequest(tt.query))

			require.Equal(t, http.StatusOK, rec.Code)
			calls := repo.GetAgentTraceScoresCalls()
			require.Len(t, calls, 1)
			assert.Equal(t, "ou-org", calls[0].OuID)
			assert.Equal(t, tt.want, calls[0].Filters)
			assert.Equal(t, MaxScoresPerRequest, calls[0].Limit)
		})
	}
}
