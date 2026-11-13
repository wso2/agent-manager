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

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

const defaultEnvUUID = "3fa85f64-5717-4562-b3fc-2c963f66afa6"

func TestGetGatewayEnvironments_Pagination(t *testing.T) {
	gatewayID := uuid.NewString()
	var mappings []models.GatewayEnvironmentMapping
	var environments []*models.EnvironmentResponse
	for i := maxGatewayEnvironments + 1; i > 0; i-- {
		id := uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", i))
		mappings = append(mappings, models.GatewayEnvironmentMapping{EnvironmentUUID: id})
		environments = append(environments, &models.EnvironmentResponse{UUID: id.String()})
	}
	repo := &repomocks.GatewayRepositoryMock{
		GetEnvironmentMappingsByGatewayIDFunc: func(_ string) ([]models.GatewayEnvironmentMapping, error) {
			return mappings, nil
		},
	}
	ctrl := &gatewayController{
		gatewayService: services.NewPlatformGatewayService(repo, nil),
		ocClient: &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return environments, nil
			},
		},
	}

	for _, tc := range []struct {
		query      string
		wantCount  int
		wantFirst  string
		wantOffset int32
	}{
		{"", maxGatewayEnvironments, environments[len(environments)-1].UUID, 0},
		{fmt.Sprintf("?offset=%d", maxGatewayEnvironments), 1, environments[0].UUID, maxGatewayEnvironments},
		{fmt.Sprintf("?offset=%d", len(environments)+5), 0, "", int32(len(environments) + 5)},
	} {
		t.Run(tc.query, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/orgs/acme/gateways/"+gatewayID+"/environments"+tc.query, nil)
			req.SetPathValue("gatewayID", gatewayID)
			ctrl.GetGatewayEnvironments(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
			}
			var response spec.GetGatewayEnvironments200Response
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if len(response.Environments) != tc.wantCount {
				t.Fatalf("got %d environments, want %d", len(response.Environments), tc.wantCount)
			}
			if tc.wantCount > 0 && response.Environments[0].Id != tc.wantFirst {
				t.Errorf("first environment = %q, want %q", response.Environments[0].Id, tc.wantFirst)
			}
			if response.Total != int32(len(environments)) || response.Limit != maxGatewayEnvironments || response.Offset != tc.wantOffset {
				t.Errorf("envelope = total %d, limit %d, offset %d; want %d, %d, %d",
					response.Total, response.Limit, response.Offset, len(environments), maxGatewayEnvironments, tc.wantOffset)
			}
		})
	}

	for _, offset := range []string{"-1", "2147483648", "abc"} {
		req := httptest.NewRequest(http.MethodGet, "/orgs/acme/gateways/"+gatewayID+"/environments?offset="+offset, nil)
		w := httptest.NewRecorder()
		(&gatewayController{}).GetGatewayEnvironments(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("offset=%s status = %d, want 400", offset, w.Code)
		}
	}
}

func listGatewaysRequest(
	t *testing.T, query string, repo *repomocks.GatewayRepositoryMock,
) (*gatewayController, *httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	ocClient := &clientmocks.OpenChoreoClientMock{
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
