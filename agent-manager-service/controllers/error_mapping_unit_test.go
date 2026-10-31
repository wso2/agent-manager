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
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
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

// An environment whose identity is created by promotion refuses identity
// management with a 409 and a fixed reason, never a 500 or the wrapped chain.
func TestHandleCommonErrors_IdentityEnvironmentUnsupported_Is409WithFixedReason(t *testing.T) {
	rr := httptest.NewRecorder()

	handleCommonErrors(rr, fmt.Errorf("resolve staging: %w", thundersvc.ErrIdentityEnvironmentUnsupported), "Failed")

	assert.Equal(t, http.StatusConflict, rr.Code)
	var body spec.ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, utils.ErrCodeConflict, body.Code)
	require.NotNil(t, body.Reason)
	assert.NotContains(t, *body.Reason, "resolve staging")
}

// Each typed identity promotion refusal answers its own status and names the
// resources involved, while the wrapped cause (internal address, upstream body,
// secret) never reaches the wire.
func TestHandleCommonErrors_IdentityPromotionError_MapsEachKind(t *testing.T) {
	const leaked = "https://thunder.internal.svc:8090/oauth2/token?client_secret=hunter2hunter2hunter2hunter2hunter2"
	cases := []struct {
		kind     services.IdentityPromotionErrorKind
		status   int
		code     string
		contains string
	}{
		{services.IdentityPromotionUnavailable, http.StatusServiceUnavailable, utils.ErrCodeServiceUnavailable, "role booker"},
		{services.IdentityPromotionForbidden, http.StatusForbidden, utils.ErrCodeForbidden, "role booker"},
		{services.IdentityPromotionConflict, http.StatusConflict, utils.ErrCodeConflict, "role booker"},
		{services.IdentityPromotionMissingValues, http.StatusBadRequest, utils.ErrCodeValidation, "2 required value(s)"},
		{services.IdentityPromotionBlocked, http.StatusBadRequest, utils.ErrCodeValidation, "role booker"},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			rr := httptest.NewRecorder()
			err := fmt.Errorf("failed to promote agent identity to environment %q: %w", "staging", &services.IdentityPromotionError{
				Kind:          tc.kind,
				Message:       "role booker is affected; upstream said " + leaked,
				MissingValues: []string{"CLIENT_URL", "ISSUER"},
				Cause:         fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, leaked),
			})

			handleCommonErrors(rr, err, "Failed to promote agent")

			assert.Equal(t, tc.status, rr.Code)
			var body spec.ErrorResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
			assert.Equal(t, tc.code, body.Code, "the kind decides the answer, not a sentinel in its cause")
			wire := rr.Body.String()
			assert.Contains(t, wire, tc.contains)
			for _, secret := range []string{"thunder.internal", "hunter2", "client_secret", "https://", "CLIENT_URL", "ISSUER"} {
				assert.NotContains(t, wire, secret)
			}
		})
	}
}

func TestHandleCommonErrors_IdentityPromotionError_MissingValuesAreCountedAndNeverNamed(t *testing.T) {
	names := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		names = append(names, fmt.Sprintf("REQUIRED_VALUE_%02d", i))
	}
	rr := httptest.NewRecorder()

	handleCommonErrors(rr, &services.IdentityPromotionError{Kind: services.IdentityPromotionMissingValues, MissingValues: names}, "Failed")

	require.Equal(t, http.StatusBadRequest, rr.Code)
	var body spec.ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	require.NotNil(t, body.Reason)
	assert.LessOrEqual(t, len(body.Message)+len(*body.Reason), 200, "the rendered UI string stays inside the promotion budget")
	assert.Contains(t, *body.Reason, "30 required value(s)")
	assert.NotContains(t, rr.Body.String(), "REQUIRED_VALUE", "a value name can reveal how the environment is configured")
}

// A typed refusal wrapping the unsupported-environment sentinel is still decided
// by its kind; the untyped sentinel keeps its fixed 409.
func TestHandleCommonErrors_IdentityPromotionError_KindWinsOverWrappedCapabilitySentinel(t *testing.T) {
	rr := httptest.NewRecorder()
	handleCommonErrors(rr, &services.IdentityPromotionError{
		Kind: services.IdentityPromotionUnavailable, Message: "identity service is down", Cause: thundersvc.ErrIdentityEnvironmentUnsupported,
	}, "Failed")
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)

	rr = httptest.NewRecorder()
	handleCommonErrors(rr, thundersvc.ErrIdentityEnvironmentUnsupported, "Failed")
	assert.Equal(t, http.StatusConflict, rr.Code)
}

// A workload promotion into an environment whose agent identity is not completed
// answers a 409 whose message and reason are the user safe ones the service wrote.
func TestHandleCommonErrors_AgentIdentityNotReady_Is409WithTheServicesTexts(t *testing.T) {
	for _, failed := range []bool{false, true} {
		rr := httptest.NewRecorder()
		notReady := utils.NewAgentIdentityNotReadyError("staging", failed, "Identity provisioning failed.")

		handleCommonErrors(rr, fmt.Errorf("promote: %w", notReady), "Failed to promote agent")

		assert.Equal(t, http.StatusConflict, rr.Code)
		var body spec.ErrorResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
		assert.Equal(t, utils.ErrCodeConflict, body.Code)
		assert.Equal(t, notReady.Message, body.Message)
		assert.Contains(t, body.Message, `"staging"`)
		require.NotNil(t, body.Reason)
		assert.Equal(t, notReady.Reason, *body.Reason)
		assert.Contains(t, *body.Reason, "Identity provisioning failed.")
	}
}
