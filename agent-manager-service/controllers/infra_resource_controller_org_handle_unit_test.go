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

func namespaceOrg(name string) *models.OrganizationResponse {
	return &models.OrganizationResponse{Name: name, DisplayName: name, Namespace: name}
}

func listOrgs(t *testing.T, orgs []*models.OrganizationResponse, claims *jwtassertion.TokenClaims) []map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/orgs", nil)
	if claims != nil {
		req = req.WithContext(jwtassertion.ContextWithTokenClaims(req.Context(), claims))
	}
	rec := httptest.NewRecorder()
	NewInfraResourceController(&fakeOrgManager{orgs: orgs}).ListOrganizations(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Organizations []map[string]any `json:"organizations"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body.Organizations
}

func TestListOrganizations_SingleOrgCarriesTokenHandle(t *testing.T) {
	got := listOrgs(t,
		[]*models.OrganizationResponse{namespaceOrg("wc-01a0c318-7a4ef447")},
		&jwtassertion.TokenClaims{OuId: "ou-1", OuHandle: "agent-manager-team"})

	require.Len(t, got, 1)
	assert.Equal(t, "agent-manager-team", got[0]["handle"])
	// The namespace-backed name is unchanged: the CLI and old links rely on it.
	assert.Equal(t, "wc-01a0c318-7a4ef447", got[0]["name"])
}

func TestListOrganizations_MultipleOrgsOmitHandle(t *testing.T) {
	got := listOrgs(t,
		[]*models.OrganizationResponse{namespaceOrg("default"), namespaceOrg("other")},
		&jwtassertion.TokenClaims{OuId: "ou-1", OuHandle: "acme"})

	require.Len(t, got, 2)
	for _, org := range got {
		assert.NotContains(t, org, "handle", "a token handle can't be attributed to one of several orgs")
	}
}

func TestListOrganizations_NoHandleInTokenOmitsField(t *testing.T) {
	for name, claims := range map[string]*jwtassertion.TokenClaims{
		"no claims":    nil,
		"empty handle": {OuId: "ou-1"},
	} {
		t.Run(name, func(t *testing.T) {
			got := listOrgs(t, []*models.OrganizationResponse{namespaceOrg("wc-01a0c318-7a4ef447")}, claims)
			require.Len(t, got, 1)
			assert.NotContains(t, got[0], "handle")
		})
	}
}

func TestGetOrganization_CarriesResolvedOrgHandle(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/orgs/wc-01a0c318-7a4ef447", nil)
	req = req.WithContext(middleware.WithResolvedOrg(req.Context(),
		middleware.ResolvedOrg{OUID: "ou-1", OuHandle: "agent-manager-team"}))
	rec := httptest.NewRecorder()

	NewInfraResourceController(&fakeOrgManager{
		orgs: []*models.OrganizationResponse{namespaceOrg("wc-01a0c318-7a4ef447")},
	}).GetOrganization(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "agent-manager-team", body["handle"])
	// The traces page reads the namespace from this response; it must not change.
	assert.Equal(t, "wc-01a0c318-7a4ef447", body["namespace"])
}
