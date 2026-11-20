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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// runnerScanService answers CreateRunnerScan only.
type runnerScanService struct {
	services.ShadowAgentService
	result *services.ShadowRunnerScan
	err    error
	opts   models.ShadowScanOptions
	called bool
}

func (s *runnerScanService) CreateRunnerScan(
	_ context.Context, _ string, _ uuid.UUID, opts models.ShadowScanOptions,
) (*services.ShadowRunnerScan, error) {
	s.opts, s.called = opts, true
	return s.result, s.err
}

func postRunnerScan(t *testing.T, svc services.ShadowAgentService, body string) *httptest.ResponseRecorder {
	t.Helper()
	id := uuid.New().String()
	r := httptest.NewRequest(http.MethodPost, "/orgs/acme/shadow-agents/connections/"+id+"/runner-scans",
		strings.NewReader(body))
	r.SetPathValue(PathParamShadowConnectionID, id)
	w := httptest.NewRecorder()
	NewShadowAgentController(svc).CreateRunnerScan(w, r)
	return w
}

func TestShadowCreateRunnerScan_ResponseIsNeverCached(t *testing.T) {
	exp := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)
	svc := &runnerScanService{result: &services.ShadowRunnerScan{
		ScanID: uuid.New(), Token: "tok", ExpiresAt: exp, AMPURL: "https://amp.example.com", Image: "img",
	}}
	w := postRunnerScan(t, svc, `{"aiReview":true,"llmJudge":false}`)
	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Equal(t, models.ShadowScanOptions{AIReview: true}, svc.opts)

	var resp spec.ShadowRunnerScanResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, svc.result.ScanID.String(), resp.ScanId)
	assert.Equal(t, "tok", resp.Token)
	assert.True(t, exp.Equal(resp.ExpiresAt))
	assert.Equal(t, "https://amp.example.com", resp.AmpUrl)
	assert.Equal(t, "img", resp.Image)
}

func TestShadowCreateRunnerScan_ErrorStatuses(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"invalid input", fmt.Errorf("%w: bad", utils.ErrInvalidInput), http.StatusBadRequest},
		{"not found", utils.ErrShadowConnectionNotFound, http.StatusNotFound},
		{"not set up", fmt.Errorf("%w: set it", utils.ErrShadowScanCommandUnavailable), http.StatusServiceUnavailable},
		{"identity provider down", fmt.Errorf("%w: down", utils.ErrServiceUnavailable), http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := postRunnerScan(t, &runnerScanService{err: tc.err}, `{"aiReview":false,"llmJudge":false}`)
			assert.Equal(t, tc.want, w.Code)
			assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		})
	}
}

func TestShadowCreateRunnerScan_MalformedBody(t *testing.T) {
	svc := &runnerScanService{}
	w := postRunnerScan(t, svc, `{"aiReview":`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, svc.called, "a malformed body never reaches the service")
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
}

// publishScanService answers PublishRunnerScan only.
type publishScanService struct {
	services.ShadowAgentService
	err    error
	ouID   string
	req    *models.PublishShadowScanRequest
	called bool
}

func (s *publishScanService) PublishRunnerScan(
	_ context.Context, ouID string, scanID uuid.UUID, req *models.PublishShadowScanRequest,
) (*models.PublishShadowScanResponse, error) {
	s.ouID, s.req, s.called = ouID, req, true
	if s.err != nil {
		return nil, s.err
	}
	return &models.PublishShadowScanResponse{ScanID: scanID.String(), Status: models.ShadowScanSucceeded, FindingsCount: 1}, nil
}

func postScanResults(svc services.ShadowAgentService, scanID, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/shadow-agent-publisher/scans/"+scanID+"/results", strings.NewReader(body))
	r.SetPathValue(PathParamShadowScanID, scanID)
	w := httptest.NewRecorder()
	NewShadowAgentController(svc).PublishScanResults(w, r)
	return w
}

func TestShadowPublishScanResults_DecodesTheBody(t *testing.T) {
	svc := &publishScanService{}
	body := `{"complete":true,"scannedRepositories":["a"],"failedRepositories":[{"name":"b","reason":"x"}],
		"agents":[{"externalId":"e","name":"n","repository":"a","attributes":{"reviewedBy":"rules"},
		"evidence":[{"code":"github.d1_agent_dependency","source":"requirements.txt","observed":{"n":1}}]}]}`
	w := postScanResults(svc, uuid.NewString(), body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.True(t, svc.called)
	assert.Empty(t, svc.ouID, "the org comes only from the scan token")
	assert.True(t, svc.req.Complete)
	require.Len(t, svc.req.Agents, 1)
	assert.Equal(t, "github.d1_agent_dependency", svc.req.Agents[0].Evidence[0].Code)
	var resp models.PublishShadowScanResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.FindingsCount)
}

func TestShadowPublishScanResults_RejectsBadBodies(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"malformed", `{"complete":`, http.StatusBadRequest},
		{"trailing data", `{"complete":true} {"complete":false}`, http.StatusBadRequest},
		{
			"too large", `{"scannedRepositories":["` + strings.Repeat("a", maxShadowPublishBodyBytes) + `"]}`,
			http.StatusRequestEntityTooLarge,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &publishScanService{}
			w := postScanResults(svc, uuid.NewString(), tc.body)
			assert.Equal(t, tc.want, w.Code)
			assert.False(t, svc.called, "a bad body never reaches the service")
		})
	}

	svc := &publishScanService{}
	w := postScanResults(svc, "not-a-uuid", `{}`)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.False(t, svc.called)
}

// A newer scan command may send fields this server does not know; they are
// ignored rather than failing a long scan.
func TestShadowPublishScanResults_IgnoresUnknownFields(t *testing.T) {
	svc := &publishScanService{}
	w := postScanResults(svc, uuid.NewString(),
		`{"complete":true,"runnerVersion":"2","agents":[{"externalId":"e","score":0.9,"repository":"a"}]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.True(t, svc.called)
	require.Len(t, svc.req.Agents, 1)
	assert.Equal(t, "e", svc.req.Agents[0].ExternalID)
}

func TestShadowPublishScanResults_ErrorStatuses(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"invalid agent", fmt.Errorf("%w: agent 0 has no evidence", utils.ErrInvalidInput), http.StatusBadRequest},
		{"scan not found", utils.ErrShadowScanNotFound, http.StatusNotFound},
		{"connection deleted", utils.ErrShadowConnectionNotFound, http.StatusNotFound},
		{"turned off", services.ErrShadowScanPublishDisabled, http.StatusNotFound},
		{"already published", fmt.Errorf("%w: already published", utils.ErrShadowScanNotPending), http.StatusConflict},
		{"newer scan published", utils.ErrShadowScanSuperseded, http.StatusConflict},
		{"audit trail down", audit.ErrRecorderUnavailable, http.StatusServiceUnavailable},
		{"other", fmt.Errorf("db down"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := postScanResults(&publishScanService{err: tc.err}, uuid.NewString(), `{"complete":true}`)
			assert.Equal(t, tc.want, w.Code)
			var body map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.NotEmpty(t, body["message"])
		})
	}
}

// scanReadService answers GetScan and ListScans with one source code scan.
type scanReadService struct {
	services.ShadowAgentService
	scan models.ShadowScan
}

func (s *scanReadService) GetScan(context.Context, string, uuid.UUID) (*models.ShadowScan, error) {
	sc := s.scan
	return &sc, nil
}

func (s *scanReadService) ListScans(context.Context, string, int) ([]models.ShadowScan, error) {
	return []models.ShadowScan{s.scan}, nil
}

// Only the scan detail carries the repository lists, which can run to
// thousands of names.
func TestShadowScanRepositoriesOnlyInTheDetail(t *testing.T) {
	svc := &scanReadService{scan: models.ShadowScan{
		ID: uuid.New(), Status: models.ShadowScanPartial, ConnectionIDs: []string{"c"},
		Results: []models.ShadowScanConnectionResult{{
			ConnectionID: "c", Outcome: "partial", Calls: []models.ShadowScanCallResult{},
			ScannedRepositories: []string{"a"},
			FailedRepositories:  []models.ShadowScanFailedRepository{{Name: "b", Message: "clone failed"}},
		}},
	}}
	ctrl := NewShadowAgentController(svc)

	r := httptest.NewRequest(http.MethodGet, "/orgs/acme/shadow-agents/scans/"+svc.scan.ID.String(), nil)
	r.SetPathValue(PathParamShadowScanID, svc.scan.ID.String())
	w := httptest.NewRecorder()
	ctrl.GetScan(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	detail := decodeJSON[spec.ShadowScanResponse](t, w.Body.Bytes())
	require.Len(t, detail.Results, 1)
	assert.Equal(t, []string{"a"}, detail.Results[0].ScannedRepositories)
	assert.Equal(t, []spec.ShadowScanFailedRepository{{Name: "b", Message: "clone failed"}}, detail.Results[0].FailedRepositories)

	w = httptest.NewRecorder()
	ctrl.ListScans(w, httptest.NewRequest(http.MethodGet, "/orgs/acme/shadow-agents/scans", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "scannedRepositories")
	assert.NotContains(t, w.Body.String(), "failedRepositories")
}

// findingListService answers ListFindings only.
type findingListService struct {
	services.ShadowAgentService
	rows []repositories.ShadowFindingRow
}

func (s *findingListService) ListFindings(
	context.Context, string, repositories.ShadowFindingFilter,
) ([]repositories.ShadowFindingRow, int, error) {
	return s.rows, len(s.rows), nil
}

// The list says who confirmed a code agent, so the console can mark agents
// confirmed by AI review; other kinds never carry it.
func TestShadowListFindings_ReviewedBy(t *testing.T) {
	row := func(kind string, attrs map[string]any) repositories.ShadowFindingRow {
		return repositories.ShadowFindingRow{ShadowFinding: models.ShadowFinding{
			ID: uuid.New(), ConnectionID: uuid.New(), Kind: kind, ExternalID: uuid.NewString(), Attributes: attrs,
		}}
	}
	svc := &findingListService{rows: []repositories.ShadowFindingRow{
		row(models.ShadowKindCodeAgent, map[string]any{"reviewedBy": "ai_review"}),
		row(models.ShadowKindCodeAgent, map[string]any{"reviewedBy": "rules"}),
		row(models.ShadowKindCodeAgent, map[string]any{"reviewedBy": ""}),
		row(models.ShadowKindCodeAgent, map[string]any{"reviewedBy": true}),
		row(models.ShadowKindPlatformAgent, map[string]any{"reviewedBy": "ai_review"}),
	}}
	w := httptest.NewRecorder()
	NewShadowAgentController(svc).ListFindings(w,
		httptest.NewRequest(http.MethodGet, "/orgs/acme/shadow-agents/findings", nil))
	require.Equal(t, http.StatusOK, w.Code)
	list := decodeJSON[spec.ShadowFindingListResponse](t, w.Body.Bytes())
	require.Len(t, list.Findings, 5)
	got := make([]*string, 0, len(list.Findings))
	for _, f := range list.Findings {
		got = append(got, f.ReviewedBy)
	}
	aiReview, rules := "ai_review", "rules"
	assert.Equal(t, []*string{&aiReview, &rules, nil, nil, nil}, got)
}

func decodeJSON[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(b, &v), string(b))
	return v
}
