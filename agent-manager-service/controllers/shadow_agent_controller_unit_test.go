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
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
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
	w := postRunnerScan(t, svc, `{"aiReview":true,"llmJudge":false,"directUpload":true}`)
	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Equal(t, models.ShadowScanOptions{AIReview: true, DirectUpload: true}, svc.opts)

	var resp spec.ShadowRunnerScanResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, svc.result.ScanID.String(), resp.ScanId)
	require.NotNil(t, resp.Token)
	assert.Equal(t, "tok", *resp.Token)
	require.NotNil(t, resp.ExpiresAt)
	assert.True(t, exp.Equal(*resp.ExpiresAt))
	assert.Equal(t, "https://amp.example.com", resp.AmpUrl)
	assert.Equal(t, "img", resp.Image)
}

// Without direct upload (the default) the response has no token or expiry.
func TestShadowCreateRunnerScan_WithoutDirectUploadHasNoToken(t *testing.T) {
	svc := &runnerScanService{result: &services.ShadowRunnerScan{
		ScanID: uuid.New(), AMPURL: "https://amp.example.com", Image: "img",
	}}
	w := postRunnerScan(t, svc, `{"aiReview":false,"llmJudge":false}`)
	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Equal(t, models.ShadowScanOptions{}, svc.opts)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.NotContains(t, body, "token")
	assert.NotContains(t, body, "expiresAt")
	assert.Equal(t, svc.result.ScanID.String(), body["scanId"])
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
		{"scan token outlived", utils.ErrShadowScanTokenExpired, http.StatusConflict},
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

// uploadScanService answers UploadRunnerScanResults and CancelRunnerScan only.
type uploadScanService struct {
	services.ShadowAgentService
	err    error
	ouID   string
	req    *models.PublishShadowScanRequest
	called bool
}

func (s *uploadScanService) UploadRunnerScanResults(
	_ context.Context, ouID string, scanID uuid.UUID, req *models.PublishShadowScanRequest,
) (*models.PublishShadowScanResponse, error) {
	s.ouID, s.req, s.called = ouID, req, true
	if s.err != nil {
		return nil, s.err
	}
	return &models.PublishShadowScanResponse{ScanID: scanID.String(), Status: models.ShadowScanPartial, FindingsCount: 2}, nil
}

func (s *uploadScanService) CancelRunnerScan(_ context.Context, ouID string, scanID uuid.UUID) (*models.ShadowScan, error) {
	s.ouID, s.called = ouID, true
	if s.err != nil {
		return nil, s.err
	}
	return &models.ShadowScan{ID: scanID, OUID: ouID, Status: models.ShadowScanFailed, Results: []models.ShadowScanConnectionResult{
		{ConnectionID: uuid.NewString(), Outcome: "failed", Message: models.ShadowScanCancelledMessage},
	}}, nil
}

func orgScanRequest(path, scanID, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/orgs/acme/shadow-agents/scans/"+scanID+path, strings.NewReader(body))
	r.SetPathValue(PathParamShadowScanID, scanID)
	return r.WithContext(middleware.WithResolvedOrg(r.Context(), middleware.ResolvedOrg{OUID: "ou-acme"}))
}

func uploadScanResults(svc services.ShadowAgentService, scanID, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	NewShadowAgentController(svc).UploadScanResults(w, orgScanRequest("/results", scanID, body))
	return w
}

// The upload takes the org from the signed-in user's request, and the body
// of the scan publisher endpoint.
func TestShadowUploadScanResults_DecodesTheBody(t *testing.T) {
	svc := &uploadScanService{}
	body := `{"complete":true,"scannedRepositories":["a"],"failedRepositories":[],
		"agents":[{"externalId":"e","name":"n","repository":"a","attributes":{"reviewedBy":"rules"},
		"evidence":[{"code":"github.d1_agent_dependency","source":"requirements.txt","observed":{"n":1}}]}]}`
	w := uploadScanResults(svc, uuid.NewString(), body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.True(t, svc.called)
	assert.Equal(t, "ou-acme", svc.ouID)
	require.Len(t, svc.req.Agents, 1)
	var resp models.PublishShadowScanResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, models.ShadowScanPartial, resp.Status)
	assert.Equal(t, 2, resp.FindingsCount)
}

func TestShadowUploadScanResults_RejectsBadBodies(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"malformed", `{"complete":`, http.StatusBadRequest},
		{"trailing data", `{"complete":true} {}`, http.StatusBadRequest},
		{
			"too large", `{"scannedRepositories":["` + strings.Repeat("a", maxShadowPublishBodyBytes) + `"]}`,
			http.StatusRequestEntityTooLarge,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &uploadScanService{}
			w := uploadScanResults(svc, uuid.NewString(), tc.body)
			assert.Equal(t, tc.want, w.Code)
			assert.False(t, svc.called, "a bad body never reaches the service")
		})
	}
	svc := &uploadScanService{}
	assert.Equal(t, http.StatusNotFound, uploadScanResults(svc, "not-a-uuid", `{}`).Code)
	assert.False(t, svc.called)
}

func TestShadowUploadScanResults_ErrorStatuses(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"invalid agent", fmt.Errorf("%w: agent 0 has no evidence", utils.ErrInvalidInput), http.StatusBadRequest},
		{"scan of another org or not a source code scan", utils.ErrShadowScanNotFound, http.StatusNotFound},
		{"turned off", services.ErrShadowScanPublishDisabled, http.StatusNotFound},
		{"cancelled", fmt.Errorf("%w: %s", utils.ErrShadowScanNotPending, models.ShadowScanCancelledMessage), http.StatusConflict},
		{"newer scan published", utils.ErrShadowScanSuperseded, http.StatusConflict},
		{"audit trail down", audit.ErrRecorderUnavailable, http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := uploadScanResults(&uploadScanService{err: tc.err}, uuid.NewString(), `{"complete":true}`)
			assert.Equal(t, tc.want, w.Code)
		})
	}
}

func TestShadowCancelScan(t *testing.T) {
	cancel := func(svc services.ShadowAgentService, scanID string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		NewShadowAgentController(svc).CancelScan(w, orgScanRequest("/cancel", scanID, ""))
		return w
	}
	svc := &uploadScanService{}
	id := uuid.NewString()
	w := cancel(svc, id)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "ou-acme", svc.ouID)
	var resp spec.ShadowScanResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, id, resp.Id)
	assert.Equal(t, models.ShadowScanFailed, resp.Status)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, models.ShadowScanCancelledMessage, resp.Results[0].GetMessage())

	assert.Equal(t, http.StatusConflict,
		cancel(&uploadScanService{err: fmt.Errorf("%w: already published", utils.ErrShadowScanNotPending)}, id).Code)
	assert.Equal(t, http.StatusConflict,
		cancel(&uploadScanService{err: fmt.Errorf("%w: succeeded", utils.ErrShadowScanFinished)}, id).Code)
	assert.Equal(t, http.StatusNotFound, cancel(&uploadScanService{err: utils.ErrShadowScanNotFound}, id).Code)
	notCalled := &uploadScanService{}
	assert.Equal(t, http.StatusNotFound, cancel(notCalled, "nope").Code)
	assert.False(t, notCalled.called)
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

// A running scan shows each connection's progress, with when it started and
// ended.
func TestShadowScanShowsProgress(t *testing.T) {
	started := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	finished := started.Add(time.Minute)
	svc := &scanReadService{scan: models.ShadowScan{
		ID: uuid.New(), Status: models.ShadowScanRunning, ConnectionIDs: []string{"a", "b", "c"},
		Results: []models.ShadowScanConnectionResult{
			{
				ConnectionID: "a", ConnectionName: "A", Provider: "openai", Outcome: "ok", FindingsCount: 1,
				Calls: []models.ShadowScanCallResult{{Call: "GET /v1/agents", Outcome: "ok", Items: 1}}, StartedAt: &started, FinishedAt: &finished,
			},
			{
				ConnectionID: "b", ConnectionName: "B", Provider: "anthropic", Outcome: models.ShadowConnectionRunning,
				Calls: []models.ShadowScanCallResult{{Call: "GET /v1/agents", Outcome: "ok"}}, StartedAt: &started,
			},
			{ConnectionID: "c", Outcome: models.ShadowConnectionQueued, Calls: []models.ShadowScanCallResult{}},
		},
	}}
	r := httptest.NewRequest(http.MethodGet, "/orgs/acme/shadow-agents/scans/"+svc.scan.ID.String(), nil)
	r.SetPathValue(PathParamShadowScanID, svc.scan.ID.String())
	w := httptest.NewRecorder()
	NewShadowAgentController(svc).GetScan(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	resp := decodeJSON[spec.ShadowScanResponse](t, w.Body.Bytes())
	require.Len(t, resp.Results, 3)
	assert.Equal(t, started, resp.Results[0].GetStartedAt())
	assert.Equal(t, finished, resp.Results[0].GetFinishedAt())
	assert.Equal(t, models.ShadowConnectionRunning, resp.Results[1].Outcome)
	assert.Len(t, resp.Results[1].Calls, 1)
	assert.Nil(t, resp.Results[1].FinishedAt)
	assert.Equal(t, models.ShadowConnectionQueued, resp.Results[2].Outcome)
	assert.Nil(t, resp.Results[2].StartedAt)
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
