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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

const testDefaultEnvID = "3fa85f64-5717-4562-b3fc-2c963f66afa6"

// gatewayDoc builds a Gateway document body with the given name and raw spec.
func gatewayDoc(name, rawSpec string) string {
	return `{"apiVersion":"ai-control-plane.choreolabs.com/v1","kind":"Gateway","metadata":{"name":"` + name + `"},"spec":` + rawSpec + `}`
}

func gatewayRequest(method, target, body, gatewayID string) (*http.Request, context.Context) {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if gatewayID != "" {
		req.SetPathValue("gatewayID", gatewayID)
	}
	ctx := modkit.WithOrgID(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-1"}), "ou-1")
	ctx, _ = audit.NewRequestScope(ctx)
	return req.WithContext(ctx), ctx
}

// registerRepo accepts one new gateway and keeps it in *created.
func registerRepo(created **gwmodels.Gateway, mapped *string) *gwrepomocks.GatewayRepositoryMock {
	return &gwrepomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(string, string) (*gwmodels.Gateway, error) { return nil, gwerr.ErrGatewayNotFound },
		TransactionFunc:       func(fn func(tx *gorm.DB) error) error { return fn(nil) },
		CreateTxFunc: func(_ *gorm.DB, gw *gwmodels.Gateway) error {
			*created = gw
			return nil
		},
		AcquireGatewayLockFunc:               func(*gorm.DB, string) error { return nil },
		EnvironmentMappingExistsFunc:         func(string, string) (bool, error) { return false, nil },
		AcquireEnvironmentLockFunc:           func(*gorm.DB, string) error { return nil },
		CountIngressCapableInEnvironmentFunc: func(*gorm.DB, string) (int64, error) { return 0, nil },
		CreateEnvironmentMappingTxFunc: func(_ *gorm.DB, m *gwmodels.GatewayEnvironmentMapping) error {
			*mapped = m.EnvironmentUUID.String()
			return nil
		},
		GetEnvironmentMappingsByGatewayIDFunc: func(string) ([]gwmodels.GatewayEnvironmentMapping, error) { return nil, nil },
	}
}

func defaultEnvCore() *gwtest.Core {
	return &gwtest.Core{
		GetEnvironmentFunc: func(_ context.Context, _, envName string) (*modkit.Environment, error) {
			if envName != gwmodels.DefaultEnvironmentName {
				return nil, modkit.ErrNotFound
			}
			return &modkit.Environment{ID: testDefaultEnvID, Name: envName}, nil
		},
	}
}

// A posted document registers a self-hosted egress gateway in the default environment.
func TestRegisterGateway_SelfHostedInTheDefaultEnvironment(t *testing.T) {
	var created *gwmodels.Gateway
	var mapped string
	ctrl := NewGatewayController(gwsvc.NewPlatformGatewayService(registerRepo(&created, &mapped)), defaultEnvCore(), nil)
	body := gatewayDoc("edge", `{"displayName":"Edge","description":"rack 4","endpoint":"  https://edge.example.com ","properties":{"site":"berlin"}}`)
	req, ctx := gatewayRequest(http.MethodPost, "/gateways", body, "")
	w := httptest.NewRecorder()

	ctrl.RegisterGateway(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.NotNil(t, created)
	assert.Equal(t, gwmodels.GatewayHostingSelfHosted, created.Hosting)
	assert.Equal(t, gwmodels.GatewayRoleEgress, created.GatewayFunctionalityType)
	assert.Equal(t, "https://edge.example.com", created.Vhost)
	assert.Equal(t, testDefaultEnvID, mapped, "the gateway is mapped to the default environment")

	var doc gwspec.GatewayDocument
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &doc))
	assert.Equal(t, kindGateway, doc.Kind)
	assert.Equal(t, "edge", doc.Metadata.Name)
	assert.Equal(t, gwspec.GATEWAYHOSTING_SELF_HOSTED, *doc.Spec.Hosting)
	assert.Equal(t, gwspec.GATEWAYTYPE_EGRESS, *doc.Spec.Type)
	assert.Equal(t, "https://edge.example.com", *doc.Spec.Endpoint)
	assert.Equal(t, "rack 4", *doc.Spec.Description)
	assert.Equal(t, map[string]interface{}{"site": "berlin"}, doc.Spec.Properties)

	event := audit.BuildEvent(ctx, "ai-gateway:gateways.create")
	assert.Equal(t, "ai", event.Details["gatewayType"])
	assert.Equal(t, "https://edge.example.com", event.Details["vhost"], "the record carries the stored vhost")
}

func TestRegisterGateway_RefusesBadDocuments(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want string
	}{
		"not json":      {`{`, "Invalid request body"},
		"api version":   {`{"apiVersion":"x/v1","kind":"Gateway","metadata":{"name":"g"},"spec":{"displayName":"G"}}`, "apiVersion must be"},
		"kind":          {`{"apiVersion":"ai-control-plane.choreolabs.com/v1","kind":"LlmProvider","metadata":{"name":"g"},"spec":{"displayName":"G"}}`, "kind must be"},
		"no name":       {gatewayDoc(" ", `{"displayName":"G","endpoint":"https://g.example.com"}`), "metadata.name is required"},
		"no label":      {gatewayDoc("g", `{"endpoint":"https://g.example.com"}`), "spec.displayName is required"},
		"managed":       {gatewayDoc("g", `{"displayName":"G","hosting":"MANAGED","endpoint":"https://g.example.com"}`), "provisioned by the platform"},
		"ingress":       {gatewayDoc("g", `{"displayName":"G","type":"INGRESS","endpoint":"https://g.example.com"}`), "spec.type of a self-hosted gateway"},
		"no endpoint":   {gatewayDoc("g", `{"displayName":"G"}`), "spec.endpoint is required"},
		"endpoint path": {gatewayDoc("g", `{"displayName":"G","endpoint":"https://g.example.com/api"}`), "base http or https URL"},
	} {
		var created *gwmodels.Gateway
		var mapped string
		ctrl := NewGatewayController(gwsvc.NewPlatformGatewayService(registerRepo(&created, &mapped)), defaultEnvCore(), nil)
		req, _ := gatewayRequest(http.MethodPost, "/gateways", tc.body, "")
		w := httptest.NewRecorder()

		ctrl.RegisterGateway(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, name)
		assert.Contains(t, w.Body.String(), tc.want, name)
		assert.Nil(t, created, "%s: nothing is stored", name)
	}
}

func TestRegisterGateway_DefaultEnvironmentMissing(t *testing.T) {
	var created *gwmodels.Gateway
	var mapped string
	core := &gwtest.Core{GetEnvironmentFunc: func(context.Context, string, string) (*modkit.Environment, error) {
		return nil, modkit.ErrNotFound
	}}
	ctrl := NewGatewayController(gwsvc.NewPlatformGatewayService(registerRepo(&created, &mapped)), core, nil)
	req, _ := gatewayRequest(http.MethodPost, "/gateways", gatewayDoc("g", `{"displayName":"G","endpoint":"https://g.example.com"}`), "")
	w := httptest.NewRecorder()

	ctrl.RegisterGateway(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	assert.Nil(t, created)
}

func selfHostedGateway() *gwmodels.Gateway {
	return &gwmodels.Gateway{
		UUID: uuid.New(), OUID: "ou-1", Name: "edge", DisplayName: "Edge", Description: "old", Vhost: "https://edge.example.com",
		Hosting: gwmodels.GatewayHostingSelfHosted, GatewayFunctionalityType: gwmodels.GatewayRoleEgress,
		Properties: map[string]interface{}{"old": true},
	}
}

func TestUpdateGateway_ReplacesTheEditableFields(t *testing.T) {
	gateway := selfHostedGateway()
	var saved *gwmodels.Gateway
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(string, string) (*gwmodels.Gateway, error) { return gateway, nil },
		GetByUUIDFunc:         func(string) (*gwmodels.Gateway, error) { return gateway, nil },
		UpdateGatewayFunc: func(gw *gwmodels.Gateway) error {
			saved = gw
			return nil
		},
	}
	ctrl := NewGatewayController(gwsvc.NewPlatformGatewayService(repo), nil, nil)

	body := gatewayDoc("edge", `{"displayName":"Edge 2","isCritical":true,"hosting":"SELF_HOSTED","type":"EGRESS","endpoint":"https://edge.example.com/"}`)
	req, _ := gatewayRequest(http.MethodPut, "/gateways/edge", body, "edge")
	w := httptest.NewRecorder()
	ctrl.UpdateGateway(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotNil(t, saved)
	assert.Equal(t, "Edge 2", saved.DisplayName)
	assert.True(t, saved.IsCritical)
	assert.Empty(t, saved.Description, "an absent description is cleared")
	assert.Empty(t, saved.Properties, "absent properties are cleared")
	assert.Equal(t, "https://edge.example.com", saved.Vhost)
}

func TestUpdateGateway_RefusesFixedFieldChanges(t *testing.T) {
	for name, tc := range map[string]struct {
		docName, spec, want string
	}{
		"name":     {"other", `{"displayName":"E"}`, "metadata.name does not match"},
		"hosting":  {"edge", `{"displayName":"E","hosting":"MANAGED"}`, "spec.hosting cannot change"},
		"type":     {"edge", `{"displayName":"E","type":"BOTH"}`, "spec.type cannot change"},
		"endpoint": {"edge", `{"displayName":"E","endpoint":"https://other.example.com"}`, "spec.endpoint cannot change"},
		"label":    {"edge", `{}`, "spec.displayName is required"},
	} {
		gateway := selfHostedGateway()
		repo := &gwrepomocks.GatewayRepositoryMock{
			GetByNameAndOrgIDFunc: func(string, string) (*gwmodels.Gateway, error) { return gateway, nil },
		}
		ctrl := NewGatewayController(gwsvc.NewPlatformGatewayService(repo), nil, nil)
		req, _ := gatewayRequest(http.MethodPut, "/gateways/edge", gatewayDoc(tc.docName, tc.spec), "edge")
		w := httptest.NewRecorder()

		ctrl.UpdateGateway(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, name)
		assert.Contains(t, w.Body.String(), tc.want, name)
		assert.Empty(t, repo.UpdateGatewayCalls(), "%s: nothing is saved", name)
	}
}

func TestUpdateGateway_BadBody(t *testing.T) {
	ctrl := NewGatewayController(gwsvc.NewPlatformGatewayService(&gwrepomocks.GatewayRepositoryMock{}), nil, nil)
	req, _ := gatewayRequest(http.MethodPut, "/gateways/edge", `{`, "edge")
	w := httptest.NewRecorder()

	ctrl.UpdateGateway(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// A managed gateway is read-only: an update or a delete is refused with 403 and touches nothing.
func TestManagedGatewayIsReadOnly(t *testing.T) {
	managed := &gwmodels.Gateway{UUID: uuid.New(), OUID: "ou-1", Name: "platform", Hosting: gwmodels.GatewayHostingManaged}
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(string, string) (*gwmodels.Gateway, error) { return managed, nil },
	}
	ctrl := NewGatewayController(gwsvc.NewPlatformGatewayService(repo), nil, nil)

	req, _ := gatewayRequest(http.MethodPut, "/gateways/platform", gatewayDoc("platform", `{"displayName":"P"}`), "platform")
	w := httptest.NewRecorder()
	ctrl.UpdateGateway(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	req, _ = gatewayRequest(http.MethodDelete, "/gateways/platform", "", "platform")
	w = httptest.NewRecorder()
	ctrl.DeleteGateway(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	assert.Empty(t, repo.UpdateGatewayCalls())
	assert.Empty(t, repo.DeleteCalls())
}

// A self-hosted gateway is deleted by name or UUID; the service is handed its UUID.
func TestDeleteGateway_ByName(t *testing.T) {
	gateway := selfHostedGateway()
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc:     func(string, string) (*gwmodels.Gateway, error) { return gateway, nil },
		GetByUUIDFunc:             func(string) (*gwmodels.Gateway, error) { return gateway, nil },
		HasGatewayDeploymentsFunc: func(string, string) (bool, error) { return false, nil },
		DeleteFunc:                func(string, string) error { return nil },
	}
	ctrl := NewGatewayController(gwsvc.NewPlatformGatewayService(repo), nil, nil)
	req, _ := gatewayRequest(http.MethodDelete, "/gateways/edge", "", "edge")
	w := httptest.NewRecorder()

	ctrl.DeleteGateway(w, req)

	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	require.Len(t, repo.DeleteCalls(), 1)
	assert.Equal(t, gateway.UUID.String(), repo.DeleteCalls()[0].GatewayID)
}

func TestDeleteGateway_Unknown(t *testing.T) {
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(string, string) (*gwmodels.Gateway, error) { return nil, gwerr.ErrGatewayNotFound },
		GetByUUIDFunc:         func(string) (*gwmodels.Gateway, error) { return nil, gwerr.ErrGatewayNotFound },
	}
	ctrl := NewGatewayController(gwsvc.NewPlatformGatewayService(repo), nil, nil)
	req, _ := gatewayRequest(http.MethodDelete, "/gateways/nope", "", "nope")
	w := httptest.NewRecorder()

	ctrl.DeleteGateway(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// listRepo answers the list queries with gateways and records the filters it was asked for.
func listRepo(filters *gwrepo.GatewayFilterOptions, gateways ...*gwmodels.Gateway) *gwrepomocks.GatewayRepositoryMock {
	return &gwrepomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(f gwrepo.GatewayFilterOptions) ([]*gwmodels.Gateway, error) {
			*filters = f
			return gateways, nil
		},
		CountWithFiltersFunc: func(gwrepo.GatewayFilterOptions) (int64, error) { return int64(len(gateways)), nil },
	}
}

func TestListGateways_Documents(t *testing.T) {
	managed := &gwmodels.Gateway{
		UUID: uuid.New(), OUID: "ou-1", Name: "platform", DisplayName: "Platform", Vhost: "gw.example.com",
		RuntimeURL: "http://gw.svc:9090", Hosting: gwmodels.GatewayHostingManaged, GatewayFunctionalityType: gwmodels.GatewayRoleBoth, IsActive: true,
	}
	var filters gwrepo.GatewayFilterOptions
	ctrl := NewGatewayController(gwsvc.NewPlatformGatewayService(listRepo(&filters, managed, selfHostedGateway())), nil, nil)
	req, _ := gatewayRequest(http.MethodGet, "/gateways?limit=5&offset=0", "", "")
	w := httptest.NewRecorder()

	ctrl.ListGateways(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var list gwspec.GatewayListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Equal(t, int32(2), *list.Count)
	assert.Equal(t, int32(2), *list.Pagination.Total)
	assert.Equal(t, int32(5), *list.Pagination.Limit)
	first := list.List[0]
	assert.Equal(t, "platform", first.Metadata.Name)
	assert.Equal(t, managed.UUID.String(), *first.Metadata.Uid)
	assert.Equal(t, gwspec.GATEWAYHOSTING_MANAGED, *first.Spec.Hosting)
	assert.Equal(t, gwspec.GATEWAYTYPE_BOTH, *first.Spec.Type)
	assert.Equal(t, "gw.example.com", *first.Spec.Endpoint)
	assert.Equal(t, "http://gw.svc:9090", *first.Spec.RuntimeUrl)
	assert.Equal(t, gwspec.GATEWAYSTATUS_ACTIVE, *first.Status.State)
	assert.Nil(t, first.Spec.Description)
	assert.Equal(t, gwspec.GATEWAYSTATUS_INACTIVE, *list.List[1].Status.State)
	assert.Empty(t, filters.Hosting, "no hosting filter by default")
}

func TestListGateways_Filters(t *testing.T) {
	for query, want := range map[string]string{
		"hosting=MANAGED":     gwmodels.GatewayHostingManaged,
		"hosting=SELF_HOSTED": gwmodels.GatewayHostingSelfHosted,
	} {
		var filters gwrepo.GatewayFilterOptions
		ctrl := NewGatewayController(gwsvc.NewPlatformGatewayService(listRepo(&filters)), nil, nil)
		req, _ := gatewayRequest(http.MethodGet, "/gateways?"+query+"&status=ACTIVE&type=EGRESS", "", "")
		w := httptest.NewRecorder()

		ctrl.ListGateways(w, req)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, want, filters.Hosting, query)
		require.NotNil(t, filters.Status, query)
		assert.True(t, *filters.Status, query)
	}

	ctrl := NewGatewayController(gwsvc.NewPlatformGatewayService(&gwrepomocks.GatewayRepositoryMock{}), nil, nil)
	req, _ := gatewayRequest(http.MethodGet, "/gateways?hosting=elsewhere", "", "")
	w := httptest.NewRecorder()
	ctrl.ListGateways(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetGateway_Document(t *testing.T) {
	gateway := selfHostedGateway()
	repo := &gwrepomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(string, string) (*gwmodels.Gateway, error) { return gateway, nil },
		GetByUUIDFunc:         func(string) (*gwmodels.Gateway, error) { return gateway, nil },
	}
	ctrl := NewGatewayController(gwsvc.NewPlatformGatewayService(repo), nil, nil)
	req, _ := gatewayRequest(http.MethodGet, "/gateways/edge", "", "edge")
	w := httptest.NewRecorder()

	ctrl.GetGateway(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var doc gwspec.GatewayDocument
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &doc))
	assert.Equal(t, "edge", doc.Metadata.Name)
	assert.Equal(t, "old", *doc.Spec.Description)
	assert.Nil(t, doc.Spec.RuntimeUrl)
}
