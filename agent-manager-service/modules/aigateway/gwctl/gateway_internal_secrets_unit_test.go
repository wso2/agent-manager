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

package gwctl

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// secretsFixture is a gateway that takes secrets by reference, authenticated by token, with the deployments
// syncDeployments reports.
type secretsFixture struct {
	controller *gatewayInternalController
	token      string
}

func newSecretsFixture(t *testing.T, syncDeployments func() ([]gwmodels.GatewaySyncRow, error)) secretsFixture {
	t.Helper()
	ouID, gatewayID := "ou-1", uuid.New()
	gateway := &gwmodels.Gateway{
		UUID: gatewayID, OUID: ouID,
		Properties: map[string]interface{}{gwmodels.SecretDeliveryProperty: gwmodels.SecretDeliveryReference},
	}
	var issued *gwmodels.GatewayToken
	gatewayRepo := &gwrepomocks.GatewayRepositoryMock{
		GetByUUIDFunc:         func(string) (*gwmodels.Gateway, error) { return gateway, nil },
		CountActiveTokensFunc: func(string) (int, error) { return 0, nil },
		CreateTokenFunc: func(token *gwmodels.GatewayToken) error {
			issued = token
			return nil
		},
		GetActiveTokenByPrefixFunc: func(context.Context, string) (*gwmodels.GatewayToken, error) {
			return issued, nil
		},
	}
	deploymentRepo := &gwrepomocks.DeploymentRepositoryMock{
		ListForGatewaySyncFunc: func(context.Context, string, string) ([]gwmodels.GatewaySyncRow, error) {
			return syncDeployments()
		},
	}
	gatewayService := gwsvc.NewPlatformGatewayService(gatewayRepo)
	token, err := gatewayService.RotateToken(gatewayID.String(), ouID)
	require.NoError(t, err)
	return secretsFixture{
		controller: &gatewayInternalController{
			gatewayService: gatewayService,
			gatewayInternalService: gwsvc.NewGatewayInternalAPIService(
				nil, nil, deploymentRepo, nil, gatewayRepo, nil, gwtest.NewSecrets(), nil,
			),
		},
		token: token.Token,
	}
}

func noDeployments() ([]gwmodels.GatewaySyncRow, error) { return nil, nil }

func failingDeployments() ([]gwmodels.GatewaySyncRow, error) {
	return nil, errors.New("database unavailable")
}

func (f secretsFixture) list(token, query string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/secrets"+query, nil)
	if token != "" {
		request.Header.Set("api-key", token)
	}
	recorder := httptest.NewRecorder()
	f.controller.GetSecrets(recorder, request)
	return recorder
}

func (f secretsFixture) value(token, handle string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/secrets/"+handle+"/value", nil)
	request.SetPathValue("handle", handle)
	if token != "" {
		request.Header.Set("api-key", token)
	}
	recorder := httptest.NewRecorder()
	f.controller.GetSecretValue(recorder, request)
	return recorder
}

func TestGetSecretsListsTheGatewaysSecretsUncached(t *testing.T) {
	f := newSecretsFixture(t, noDeployments)

	for _, query := range []string{"", "?includeValues=true"} {
		recorder := f.list(f.token, query)

		require.Equal(t, http.StatusOK, recorder.Code, query)
		assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
		assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
		var body gatewaySecretList
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
		assert.Empty(t, body.List)
		assert.Zero(t, body.Count)
	}
}

func TestGetSecretsFailsWhenTheSecretsCannotBeListed(t *testing.T) {
	f := newSecretsFixture(t, failingDeployments)

	recorder := f.list(f.token, "?includeValues=true")

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "database unavailable")
}

func TestGetSecretValueOfAnUnknownHandleIsNotFound(t *testing.T) {
	f := newSecretsFixture(t, noDeployments)

	recorder := f.value(f.token, "amp-unknown")

	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.Empty(t, recorder.Header().Get("Cache-Control"))
}

func TestGetSecretValueFailsWhenTheSecretCannotBeRead(t *testing.T) {
	f := newSecretsFixture(t, failingDeployments)

	recorder := f.value(f.token, "amp-unknown")

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "database unavailable")
}

func TestSecretEndpointsNeedTheGatewaysToken(t *testing.T) {
	f := newSecretsFixture(t, noDeployments)

	assert.Equal(t, http.StatusUnauthorized, f.list("", "").Code)
	assert.Equal(t, http.StatusUnauthorized, f.value("", "amp-unknown").Code)
	assert.Equal(t, http.StatusUnauthorized, f.value(uuid.NewString()+"-wrong", "amp-unknown").Code)
}
