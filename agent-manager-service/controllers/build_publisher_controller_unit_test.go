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
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/workloadpolicy"
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
	// A build token: issued to this service's own client, for the organization.
	clientID := config.GetConfig().IDP.ClientID
	return req.WithContext(jwtassertion.ContextWithTokenClaims(req.Context(), &jwtassertion.TokenClaims{
		OuId:             ouID,
		ClientID:         clientID,
		RegisteredClaims: jwt.RegisteredClaims{Audience: jwt.ClaimStrings{clientID}},
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
		{workloadpolicy.ErrSuspended, http.StatusForbidden},
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

func TestBuildPublisherController_SaysWhyABuildIsRefused(t *testing.T) {
	tests := []struct {
		err      error
		want     int
		wantCode string
		wantText string
	}{
		{
			services.ErrBuildPublisherDisabled, http.StatusNotFound, utils.ErrCodeBuildPublisherDisabled,
			"Build publishing through Agent Manager is turned off for this environment",
		},
		{
			fmt.Errorf("%w: build \"b\"", services.ErrBuildNotAgent), http.StatusForbidden, utils.ErrCodeBuildNotAgent,
			"This build does not belong to an agent",
		},
		{
			fmt.Errorf("%w: build \"b\" runs workflow \"w\"", services.ErrBuildWorkflowNotEnabled), http.StatusForbidden,
			utils.ErrCodeWorkflowNotEnabled, "This build's workflow is not set up to publish through Agent Manager",
		},
		{
			fmt.Errorf("%w: build \"b\"", services.ErrAgentNotSourceBuilt), http.StatusForbidden, utils.ErrCodeAgentNotSourceBuilt,
			"This agent is not built from source",
		},
	}
	for _, tc := range tests {
		t.Run(tc.wantCode, func(t *testing.T) {
			rec := httptest.NewRecorder()
			NewBuildPublisherController(&fakeBuildPublisherService{err: tc.err}).PublishBuildWorkload(rec,
				buildPublishRequest(`{"workload":{}}`, "org-1"))
			// The status is unchanged (404 / 403, both final for the build step);
			// only the message and code now name the reason.
			assert.Equal(t, tc.want, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.wantCode)
			assert.Contains(t, rec.Body.String(), tc.wantText)
			assert.NotContains(t, rec.Body.String(), "\"w\"", "the workflow name stays in the logs")
		})
	}

	t.Run("a plain not-found is still Build not found", func(t *testing.T) {
		rec := httptest.NewRecorder()
		NewBuildPublisherController(&fakeBuildPublisherService{err: fmt.Errorf("%w: build", utils.ErrNotFound)}).PublishBuildWorkload(rec,
			buildPublishRequest(`{"workload":{}}`, "org-1"))
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "Build not found")
		assert.NotContains(t, rec.Body.String(), utils.ErrCodeBuildPublisherDisabled)
	})
}

// Both controllers answer a suspension through writeSubscriptionSuspended, so a
// user sees the same status, message and code wherever the request was refused.
func TestSubscriptionSuspendedIsAnsweredTheSameEverywhere(t *testing.T) {
	agentRec := httptest.NewRecorder()
	handleCommonErrors(agentRec, fmt.Errorf("deploy refused: %w", workloadpolicy.ErrSuspended), "Failed to deploy agent")

	publishRec := httptest.NewRecorder()
	NewBuildPublisherController(&fakeBuildPublisherService{err: workloadpolicy.ErrSuspended}).PublishBuildWorkload(publishRec,
		buildPublishRequest(`{"workload":{}}`, "org-1"))

	assert.Equal(t, http.StatusForbidden, agentRec.Code)
	assert.Equal(t, agentRec.Code, publishRec.Code)
	assert.JSONEq(t, agentRec.Body.String(), publishRec.Body.String())
	assert.Contains(t, agentRec.Body.String(), "SUBSCRIPTION_SUSPENDED")
	assert.Contains(t, agentRec.Body.String(), "The Agent Manager subscription is suspended")
}

func TestBuildPublisherController_AnswersASuspensionAsFinal(t *testing.T) {
	rec := httptest.NewRecorder()
	NewBuildPublisherController(&fakeBuildPublisherService{err: workloadpolicy.ErrSuspended}).PublishBuildWorkload(rec,
		buildPublishRequest(`{"workload":{}}`, "org-1"))
	// 403, not a 5xx: the build step retries only a missing response or a 5xx.
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "SUBSCRIPTION_SUSPENDED")
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
