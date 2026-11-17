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

	"github.com/wso2/agent-manager/agent-manager-service/models"
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
