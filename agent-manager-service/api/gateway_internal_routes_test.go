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

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
)

// routingStub records which handler a request reached. It embeds a real
// controller so route registration, which takes every method value, still works.
type routingStub struct {
	controllers.GatewayInternalController
	called string
}

func (s *routingStub) GetAgentAPIKeys(http.ResponseWriter, *http.Request) {
	s.called = "GetAgentAPIKeys"
}
func (s *routingStub) GetAgent(http.ResponseWriter, *http.Request) { s.called = "GetAgent" }

func serveInternal(t *testing.T, path string) string {
	t.Helper()
	stub := &routingStub{GatewayInternalController: controllers.NewGatewayInternalController(nil, nil, nil, nil)}
	mux := http.NewServeMux()
	RegisterGatewayInternalRoutes(middleware.NewInternalRouteRegistrar(mux, audit.NewNoopRecorder()), stub)

	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	return stub.called
}

func TestAgentAPIKeysRouteIsNotTheAgentFetch(t *testing.T) {
	assert.Equal(t, "GetAgentAPIKeys", serveInternal(t, "/agents/api-keys"))
}

func TestAgentFetchRouteStillServesAnAgentID(t *testing.T) {
	assert.Equal(t, "GetAgent", serveInternal(t, "/agents/0190a7c2-1111-7222-8333-444455556666"))
}
