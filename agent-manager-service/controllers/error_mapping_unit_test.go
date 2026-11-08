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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// A service that returns its own short user-facing message (a ValidationError)
// must have that message reach the wire — the generic "Invalid input provided"
// bucket label belongs to errors that carry their whole story in the reason.
// The ordering inside handleCommonErrors is what guarantees it: the sentinel
// case below would otherwise win and relabel the response.
func TestHandleCommonErrors_ValidationErrorWithSentinel_KeepsItsOwnMessage(t *testing.T) {
	rr := httptest.NewRecorder()

	handleCommonErrors(rr, utils.NewInvalidInputError(
		"Promotion blocked: the agent identity for \"staging\" is still being provisioned",
		"retry once provisioning completes",
	), "Failed to promote agent")

	require.Equal(t, http.StatusBadRequest, rr.Code)
	var body spec.ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, "Promotion blocked: the agent identity for \"staging\" is still being provisioned", body.Message)
	require.NotNil(t, body.Reason)
	assert.Equal(t, "retry once provisioning completes", *body.Reason)
	assert.Equal(t, utils.ErrCodeValidation, body.Code)
}

func TestHandleCommonErrors_AgentIdentityRequiresPromotion_Is409(t *testing.T) {
	rr := httptest.NewRecorder()

	handleCommonErrors(rr, fmt.Errorf("%w: staging", services.ErrProjectAgentIdentityRequiresPromotion),
		"Failed to provision agent identity")

	assert.Equal(t, http.StatusConflict, rr.Code)
	var body spec.ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, utils.ErrCodeConflict, body.Code)
}

func TestHandleCommonErrors_PromotionNeedsRedeploy_Is409WithInstructions(t *testing.T) {
	rr := httptest.NewRecorder()

	handleCommonErrors(rr, fmt.Errorf("%w: deploy the agent again and promote it through to \"staging\"", utils.ErrPromotionNeedsRedeploy),
		"Failed to promote agent")

	require.Equal(t, http.StatusConflict, rr.Code)
	var body spec.ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, utils.ErrCodeConflict, body.Code)
	assert.Contains(t, body.Message, "deploy the agent again",
		"the message alone tells the user how to proceed, since the console shows only the message")
	assert.Contains(t, body.Message, "promote it up to the source environment")
	require.NotNil(t, body.Reason)
	assert.Contains(t, *body.Reason, "deploy the agent again", "the reason tells the user how to proceed")
}

// A fail-closed service that refuses to act unrecorded answers 503 on every route, not only the proxy's.
func TestHandleCommonErrors_AuditRecorderUnavailable_Is503(t *testing.T) {
	rr := httptest.NewRecorder()

	handleCommonErrors(rr, fmt.Errorf("wrapped: %w", audit.ErrRecorderUnavailable), "Failed to remove the card source")

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	var body spec.ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, utils.ErrCodeServiceUnavailable, body.Code)
	assert.Nil(t, body.Reason, "the sink's error stays in the log")
}

func TestHandleCommonErrors_DeploymentPipelineAlreadyExists_IsActionable(t *testing.T) {
	rr := httptest.NewRecorder()
	handleCommonErrors(rr, fmt.Errorf("%w: %w", utils.ErrDeploymentPipelineAlreadyExists, utils.ErrConflict), "Failed to create deployment pipeline")
	require.Equal(t, http.StatusConflict, rr.Code)
	var body spec.ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, "A deployment pipeline with this name already exists. Choose a different name.", body.Message)
	assert.Equal(t, utils.ErrCodeConflict, body.Code)
	assert.Nil(t, body.Reason, "the actionable message needs no internal OpenChoreo details")
}

// TestHandleCommonErrors_WorkloadSecretSyncUnconfirmed checks the partial-success response.
func TestHandleCommonErrors_WorkloadSecretSyncUnconfirmed(t *testing.T) {
	rr := httptest.NewRecorder()
	err := fmt.Errorf("%w: backend diagnostic", utils.ErrWorkloadSecretSyncUnconfirmed)
	handleCommonErrors(rr, err, "Failed to update agent configurations")
	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.Contains(t, rr.Body.String(), "Secret values were saved")
	assert.Contains(t, rr.Body.String(), "rollout stopped")
	assert.NotContains(t, rr.Body.String(), "backend diagnostic")
}
