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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

const defaultEnvUUID = "3fa85f64-5717-4562-b3fc-2c963f66afa6"

func listGatewaysRequest(
	t *testing.T, query string, repo *repomocks.GatewayRepositoryMock,
) (*gatewayController, *httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	ocClient := &repomocks.PlatformRepositoryMock{
		ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{UUID: defaultEnvUUID, Name: "default"}}, nil
		},
	}
	svc := services.NewPlatformGatewayService(repo, nil)
	ctrl := &gatewayController{gatewayService: svc, ocClient: ocClient}

	req := httptest.NewRequest(http.MethodGet, "/orgs/acme/gateways?"+query, nil)
	return ctrl, httptest.NewRecorder(), req
}

// emptyGatewayRepo answers the two list queries with nothing, so a test can assert
// on the filters the controller built rather than on any rows.
func emptyGatewayRepo(recordFilters *repositories.GatewayFilterOptions) *repomocks.GatewayRepositoryMock {
	return &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(filters repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			*recordFilters = filters
			return []*models.Gateway{}, nil
		},
		CountWithFiltersFunc: func(_ repositories.GatewayFilterOptions) (int64, error) {
			return 0, nil
		},
	}
}

// The controller used to log a warning and drop an unresolvable environment filter,
// so `--env typo` returned every gateway in the org — a filter that fails open reads
// as "everything matches".
func TestListGateways_UnknownEnvironmentFilterIsRejected(t *testing.T) {
	// Every repository func is nil: the filter must be rejected before the service
	// is ever asked for gateways.
	ctrl, w, req := listGatewaysRequest(t, "environment=nonexistent", &repomocks.GatewayRepositoryMock{})

	ctrl.ListGateways(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "nonexistent") {
		t.Errorf("body = %s, want the rejected environment named", w.Body.String())
	}
}

func TestListGateways_KnownEnvironmentNameResolvesToItsUUID(t *testing.T) {
	var filters repositories.GatewayFilterOptions
	ctrl, w, req := listGatewaysRequest(t, "environment=default", emptyGatewayRepo(&filters))

	ctrl.ListGateways(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if filters.EnvironmentID == nil {
		t.Fatal("environment filter was dropped")
	}
	if *filters.EnvironmentID != defaultEnvUUID {
		t.Errorf("EnvironmentID = %q, want %q", *filters.EnvironmentID, defaultEnvUUID)
	}
}

func TestListGateways_KnownEnvironmentUUIDResolvesToItself(t *testing.T) {
	var filters repositories.GatewayFilterOptions
	ctrl, w, req := listGatewaysRequest(t, "environment="+defaultEnvUUID, emptyGatewayRepo(&filters))

	ctrl.ListGateways(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if filters.EnvironmentID == nil {
		t.Fatal("environment filter was dropped")
	}
	if *filters.EnvironmentID != defaultEnvUUID {
		t.Errorf("EnvironmentID = %q, want %q", *filters.EnvironmentID, defaultEnvUUID)
	}
}

// A UUID used to be accepted on syntactic validity alone, so an environment from
// another organization passed straight through as a filter and the caller saw an
// empty list instead of being told the value is unknown to them.
func TestListGateways_ForeignEnvironmentUUIDIsRejected(t *testing.T) {
	const foreignEnvUUID = "11111111-2222-3333-4444-555555555555"
	// Every repository func is nil: the filter must be rejected before the service
	// is ever asked for gateways.
	ctrl, w, req := listGatewaysRequest(t, "environment="+foreignEnvUUID, &repomocks.GatewayRepositoryMock{})

	ctrl.ListGateways(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), foreignEnvUUID) {
		t.Errorf("body = %s, want the rejected environment named", w.Body.String())
	}
}

func TestListGateways_NoEnvironmentFilterLeavesItUnset(t *testing.T) {
	var filters repositories.GatewayFilterOptions
	ctrl, w, req := listGatewaysRequest(t, "", emptyGatewayRepo(&filters))

	ctrl.ListGateways(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if filters.EnvironmentID != nil {
		t.Errorf("EnvironmentID = %q, want unset", *filters.EnvironmentID)
	}
}
