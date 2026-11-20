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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// deleteAgentServiceStub answers only DeleteAgent; any other call panics on the nil embedded interface.
type deleteAgentServiceStub struct {
	services.AgentManagerService
	warnings []services.AgentDeleteWarning
	err      error
}

func (s deleteAgentServiceStub) DeleteAgent(context.Context, string, string, string) ([]services.AgentDeleteWarning, error) {
	return s.warnings, s.err
}

func deleteAgentRequest(stub deleteAgentServiceStub) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/orgs/org/projects/proj/agents/agent", nil)
	NewAgentController(stub, nil, nil).DeleteAgent(rr, req)
	return rr
}

func TestDeleteAgent_WithoutWarningsIsNoContent(t *testing.T) {
	rr := deleteAgentRequest(deleteAgentServiceStub{})
	require.Equal(t, http.StatusNoContent, rr.Code)
	require.Empty(t, rr.Body.String())
}

func TestDeleteAgent_WarningsAreReturnedAfterSuccessfulDelete(t *testing.T) {
	rr := deleteAgentRequest(deleteAgentServiceStub{warnings: []services.AgentDeleteWarning{{
		Code: utils.ErrCodeAgentIdentityOutsidePipeline, Message: "remove it in y", Environments: []string{"y"},
	}}})
	require.Equal(t, http.StatusOK, rr.Code)
	var body spec.AgentDeleteResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	require.Len(t, body.Warnings, 1)
	require.Equal(t, utils.ErrCodeAgentIdentityOutsidePipeline, body.Warnings[0].Code)
	require.Equal(t, []string{"y"}, body.Warnings[0].Environments)
	require.Equal(t, "remove it in y", body.Warnings[0].Message)
}

func TestDeleteAgent_FailureReturnsNoWarnings(t *testing.T) {
	rr := deleteAgentRequest(deleteAgentServiceStub{err: utils.ErrAgentNotFound})
	require.Equal(t, http.StatusNotFound, rr.Code)
}
