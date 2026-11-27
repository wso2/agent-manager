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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

// fakeOrgManager serves the two org lookups; any other InfraResourceManager
// method panics on the nil embedded interface, which these tests never reach.
type fakeOrgManager struct {
	services.InfraResourceManager
	orgs []*models.OrganizationResponse
}

func (f *fakeOrgManager) ListOrganizations(context.Context, int, int) ([]*models.OrganizationResponse, int32, error) {
	return f.orgs, int32(len(f.orgs)), nil
}

func (f *fakeOrgManager) GetOrganization(context.Context, string) (*models.OrganizationResponse, error) {
	return f.orgs[0], nil
}

func namedOrg(name string) *models.OrganizationResponse {
	return &models.OrganizationResponse{Name: name, DisplayName: name}
}

func listOrgs(t *testing.T, orgs []*models.OrganizationResponse, resolved *middleware.ResolvedOrg) []map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/orgs", nil)
	if resolved != nil {
		req = req.WithContext(middleware.WithResolvedOrg(req.Context(), *resolved))
	}
	rec := httptest.NewRecorder()
	NewInfraResourceController(&fakeOrgManager{orgs: orgs}, nil).ListOrganizations(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Organizations []map[string]any `json:"organizations"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body.Organizations
}

// The organization is the request's: its handle, with no creation date, as nothing records one.
func TestListOrganizations_ReturnsRequestOrg(t *testing.T) {
	got := listOrgs(t,
		[]*models.OrganizationResponse{namedOrg("acme")},
		&middleware.ResolvedOrg{OUID: "ou-1", OuHandle: "acme"})

	require.Len(t, got, 1)
	assert.Equal(t, "acme", got[0]["name"])
	assert.Equal(t, "acme", got[0]["handle"])
	assert.NotContains(t, got[0], "createdAt")
}

// A trusted caller acting for another organization gets that organization's handle, not its token's.
func TestListOrganizations_UsesOverriddenOrgHandle(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/orgs", nil)
	ctx := jwtassertion.ContextWithTokenClaims(req.Context(), &jwtassertion.TokenClaims{OuId: "ou-admin", OuHandle: "platform"})
	ctx = middleware.WithResolvedOrg(ctx, middleware.ResolvedOrg{OUID: "ou-beta", OuHandle: "beta"})
	rec := httptest.NewRecorder()
	NewInfraResourceController(&fakeOrgManager{orgs: []*models.OrganizationResponse{namedOrg("beta")}}, nil).
		ListOrganizations(rec, req.WithContext(ctx))
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Organizations []map[string]any `json:"organizations"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Organizations, 1)
	assert.Equal(t, "beta", body.Organizations[0]["handle"])
}

func TestListOrganizations_NoResolvedOrgOmitsHandle(t *testing.T) {
	got := listOrgs(t, []*models.OrganizationResponse{namedOrg("acme")}, nil)
	require.Len(t, got, 1)
	assert.NotContains(t, got[0], "handle")
}

func TestGetOrganization_CarriesResolvedOrgHandle(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/orgs/acme", nil)
	req = req.WithContext(middleware.WithResolvedOrg(req.Context(),
		middleware.ResolvedOrg{OUID: "ou-1", OuHandle: "acme"}))
	rec := httptest.NewRecorder()

	NewInfraResourceController(&fakeOrgManager{
		orgs: []*models.OrganizationResponse{namedOrg("acme")},
	}, nil).GetOrganization(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "acme", body["handle"])
	assert.Equal(t, "acme", body["displayName"])
	assert.NotContains(t, body, "createdAt")
}
