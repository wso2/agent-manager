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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type fakeBuildPublisherService struct {
	gotOUID  string
	gotBuild string
	gotReq   *models.PublishBuildWorkloadRequest
	err      error
}

func (f *fakeBuildPublisherService) PublishWorkload(_ context.Context, ouID, buildName string, req *models.PublishBuildWorkloadRequest) error {
	f.gotOUID, f.gotBuild, f.gotReq = ouID, buildName, req
	return f.err
}

func buildPublishRequest(body string, ouID string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/build-publisher/builds/my-agent-1700/workload", strings.NewReader(body))
	req.SetPathValue(utils.PathParamBuildName, "my-agent-1700")
	return req.WithContext(jwtassertion.ContextWithTokenClaims(req.Context(), &jwtassertion.TokenClaims{
		OuId:             ouID,
		RegisteredClaims: jwt.RegisteredClaims{Audience: jwt.ClaimStrings{"amp-build-" + ouID}},
	}))
}

func TestBuildPublisherController_TakesTheOrganizationFromTheToken(t *testing.T) {
	svc := &fakeBuildPublisherService{}
	rec := httptest.NewRecorder()
	NewBuildPublisherController(svc).PublishBuildWorkload(rec,
		buildPublishRequest(`{"workload":{"spec":{"container":{"image":"i"}}},"workloadOverrides":null}`, "org-1"))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "org-1", svc.gotOUID)
	assert.Equal(t, "my-agent-1700", svc.gotBuild)
	assert.JSONEq(t, `{"spec":{"container":{"image":"i"}}}`, string(svc.gotReq.Workload))
}

func TestBuildPublisherController_MapsServiceErrors(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{fmt.Errorf("%w: x", utils.ErrBadRequest), http.StatusBadRequest},
		{fmt.Errorf("%w: x", utils.ErrNotFound), http.StatusNotFound},
		{fmt.Errorf("%w: x", utils.ErrForbidden), http.StatusForbidden},
		{fmt.Errorf("%w: x", utils.ErrConflict), http.StatusConflict},
		{fmt.Errorf("%w: x", services.ErrBuildWorkloadAlreadyPublished), http.StatusConflict},
		{fmt.Errorf("refused: %w", audit.ErrRecorderUnavailable), http.StatusServiceUnavailable},
		{errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.err.Error(), func(t *testing.T) {
			rec := httptest.NewRecorder()
			NewBuildPublisherController(&fakeBuildPublisherService{err: tc.err}).PublishBuildWorkload(rec,
				buildPublishRequest(`{"workload":{}}`, "org-1"))
			assert.Equal(t, tc.want, rec.Code)
		})
	}
}

func TestBuildPublisherController_RejectsMalformedAndOversizedBodies(t *testing.T) {
	svc := &fakeBuildPublisherService{}
	rec := httptest.NewRecorder()
	NewBuildPublisherController(svc).PublishBuildWorkload(rec, buildPublishRequest(`{not json`, "org-1"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Nil(t, svc.gotReq, "a malformed body must not reach the service")

	for _, body := range []string{`{"workload":{},"extra":1}`, `{"workload":{}} {"workload":{}}`} {
		rec = httptest.NewRecorder()
		NewBuildPublisherController(svc).PublishBuildWorkload(rec, buildPublishRequest(body, "org-1"))
		assert.Equal(t, http.StatusBadRequest, rec.Code, body)
		assert.Nil(t, svc.gotReq)
	}

	rec = httptest.NewRecorder()
	big := `{"workload":"` + strings.Repeat("a", maxBuildWorkloadBodyBytes) + `"}`
	NewBuildPublisherController(svc).PublishBuildWorkload(rec, buildPublishRequest(big, "org-1"))
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Nil(t, svc.gotReq)
}
