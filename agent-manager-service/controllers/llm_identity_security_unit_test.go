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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestCreateLLMProviderRejectsIdentitySecurity(t *testing.T) {
	// Any repository call would panic because this config must be rejected before persistence.
	repo := &repomocks.LLMProviderRepositoryMock{}
	svc := services.NewLLMProviderService(nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	controller := NewLLMController(nil, svc, nil, nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodPost, "/orgs/acme/llm-providers", strings.NewReader(`{
		"id":"provider","name":"Provider","version":"v1.0","context":"/provider","template":"openai",
		"security":{"enabled":true,"identity":{"enabled":true}}
	}`))
	request = request.WithContext(middleware.WithResolvedOrg(request.Context(), middleware.ResolvedOrg{OUID: "ou-acme"}))
	recorder := httptest.NewRecorder()

	controller.CreateLLMProvider(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var response spec.ErrorResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, utils.ErrLLMIdentityUnsupported.Error(), response.GetMessage())
}
