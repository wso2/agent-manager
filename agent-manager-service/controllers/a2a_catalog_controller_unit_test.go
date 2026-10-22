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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type catalogServiceStub struct {
	ListFunc    func(ctx context.Context, filter models.A2ACatalogFilter) (*models.A2ACatalogPage, error)
	EntryFunc   func(ctx context.Context, ouID, projectName, agentName string) (*models.A2ACatalogEntry, error)
	ConnectFunc func(ctx context.Context, ouID, projectName, agentName, envName string) (*models.A2AConnectDetails, error)
}

func (s *catalogServiceStub) ListA2ACatalog(ctx context.Context, filter models.A2ACatalogFilter) (*models.A2ACatalogPage, error) {
	return s.ListFunc(ctx, filter)
}

func (s *catalogServiceStub) GetA2ACatalogEntry(ctx context.Context, ouID, projectName, agentName string) (*models.A2ACatalogEntry, error) {
	return s.EntryFunc(ctx, ouID, projectName, agentName)
}

func (s *catalogServiceStub) GetA2AConnectDetails(
	ctx context.Context, ouID, projectName, agentName, envName string,
) (*models.A2AConnectDetails, error) {
	return s.ConnectFunc(ctx, ouID, projectName, agentName, envName)
}

// catalogListRequest lists the catalogue of a resolved org with the given raw query string.
func catalogListRequest(rawQuery string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/orgs/default/a2a-agents?"+rawQuery, nil)
	return req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-org"}))
}

// catalogEntryRequest targets the checkout/weather agent's catalogue entry in a resolved org.
func catalogEntryRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/orgs/default/projects/checkout/agents/weather/a2a-catalog-entry", nil)
	req.SetPathValue(utils.PathParamProjName, "checkout")
	req.SetPathValue(utils.PathParamAgentName, "weather")
	return req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-org"}))
}

// capturingList records the filter it receives and answers with page.
func capturingList(got *models.A2ACatalogFilter, page models.A2ACatalogPage) *catalogServiceStub {
	return &catalogServiceStub{ListFunc: func(_ context.Context, filter models.A2ACatalogFilter) (*models.A2ACatalogPage, error) {
		*got = filter
		return &page, nil
	}}
}

func TestListA2AAgentsParsesTheQuery(t *testing.T) {
	fetchedAt := time.Date(2026, 10, 21, 9, 30, 0, 0, time.UTC)
	var got models.A2ACatalogFilter
	stub := capturingList(&got, models.A2ACatalogPage{
		Entries: []models.A2ACatalogEntry{{
			ProjectName: "p1", AgentName: "refunds", Kind: models.A2AAgentCardSourceExternal, Name: "Refunds",
			Description: "Handles refunds", Version: "1.0", Skills: []string{"refund"}, LeadEnvironment: "dev",
			Environments: []models.A2ACatalogEnvironment{{
				Name: "dev", Status: models.A2AAgentCardStatusFetched, FetchedAt: &fetchedAt,
				Proxied: true, CardDiffers: true, Matched: true,
			}},
		}},
		Total: 42, Limit: 24, Offset: 48,
	})
	rec := httptest.NewRecorder()

	NewA2ACatalogController(stub).ListA2AAgents(rec, catalogListRequest(
		"q=%20refund%20&kind=proxied&environment=dev&project=p1&status=fetched&limit=24&offset=48"))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, models.A2ACatalogFilter{
		OUID: "ou-org", Query: "refund", Project: "p1", Environment: "dev",
		Kind: models.A2ACatalogKindProxied, Status: models.A2AAgentCardStatusFetched, Limit: 24, Offset: 48,
	}, got)
	assert.JSONEq(t, `{
		"agents": [{
			"projectName": "p1", "agentName": "refunds", "kind": "external", "name": "Refunds",
			"description": "Handles refunds", "version": "1.0", "skills": ["refund"], "leadEnvironment": "dev",
			"environments": [{
				"name": "dev", "status": "fetched", "fetchedAt": "2026-10-21T09:30:00Z",
				"proxied": true, "cardDiffers": true, "matched": true
			}]
		}],
		"total": 42, "limit": 24, "offset": 48
	}`, rec.Body.String())
}

func TestListA2AAgentsDefaults(t *testing.T) {
	var got models.A2ACatalogFilter
	stub := capturingList(&got, models.A2ACatalogPage{})
	rec := httptest.NewRecorder()

	NewA2ACatalogController(stub).ListA2AAgents(rec, catalogListRequest(""))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, models.A2ACatalogFilter{OUID: "ou-org", Limit: 50, Offset: 0}, got)
	assert.JSONEq(t, `{"agents":[],"total":0,"limit":0,"offset":0}`, rec.Body.String())
}

func TestListA2AAgentsBlankQueryMeansNoSearch(t *testing.T) {
	var got models.A2ACatalogFilter
	stub := capturingList(&got, models.A2ACatalogPage{})
	rec := httptest.NewRecorder()

	NewA2ACatalogController(stub).ListA2AAgents(rec, catalogListRequest("q=%20%20"))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, got.Query)
}

func TestListA2AAgentsRejectsBadParameters(t *testing.T) {
	const (
		limitMsg  = "Invalid limit parameter: must be between 1 and 100"
		offsetMsg = "Invalid offset parameter: must be 0 or greater"
	)
	cases := map[string]struct {
		query   string
		wantMsg string
	}{
		"limit zero":          {"limit=0", limitMsg},
		"limit above max":     {"limit=101", limitMsg},
		"limit not a number":  {"limit=x", limitMsg},
		"negative offset":     {"offset=-1", offsetMsg},
		"q too long":          {"q=" + strings.Repeat("a", 256), "Invalid q parameter: must be at most 255 characters"},
		"unknown kind":        {"kind=bogus", "Invalid kind parameter: must be platform, external or proxied"},
		"unknown status":      {"status=bogus", "Invalid status parameter: must be pending, fetched or failed"},
		"limit checked first": {"limit=0&offset=-1&kind=bogus", limitMsg},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			stub := &catalogServiceStub{} // ListFunc nil: the service must not be called on a 400
			rec := httptest.NewRecorder()

			NewA2ACatalogController(stub).ListA2AAgents(rec, catalogListRequest(tc.query))

			require.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), `"`+tc.wantMsg+`"`)
		})
	}
	for _, tc := range []struct{ name, query string }{
		{"smallest limit", "limit=1"},
		{"largest limit", "limit=100"},
		{"longest multibyte query", "q=" + strings.Repeat("é", 255)},
	} {
		t.Run("accepts "+tc.name, func(t *testing.T) {
			var got models.A2ACatalogFilter
			rec := httptest.NewRecorder()

			NewA2ACatalogController(capturingList(&got, models.A2ACatalogPage{})).ListA2AAgents(rec, catalogListRequest(tc.query))

			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

func connectStub(details models.A2AConnectDetails) *catalogServiceStub {
	return &catalogServiceStub{ConnectFunc: func(context.Context, string, string, string, string) (*models.A2AConnectDetails, error) {
		return &details, nil
	}}
}

func TestGetA2AConnectSerialisesAPIKeySecurity(t *testing.T) {
	stub := connectStub(models.A2AConnectDetails{
		Kind: models.A2AAgentCardSourceExternal, Proxied: true,
		GatewayURL: "https://gw/agent", CardURL: "https://gw/agent/.well-known/agent-card.json", JSONRPCURL: "https://gw/agent/rpc",
		Security:    models.A2AConnectSecurity{Mode: models.A2AConnectSecurityAPIKey, APIKeyHeader: "X-API-Key"},
		Publication: models.A2AConnectPublication{Status: string(models.A2APublicationStatusPublished)},
	})
	rec := httptest.NewRecorder()

	NewA2ACatalogController(stub).GetA2AConnect(rec, agentEnvRequest(http.MethodGet, "a2a-connect", ""))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"kind":"external","proxied":true,"isProduction":false,"gatewayUrl":"https://gw/agent",`+
		`"cardUrl":"https://gw/agent/.well-known/agent-card.json","jsonRpcUrl":"https://gw/agent/rpc",`+
		`"security":{"mode":"apiKey","apiKeyHeader":"X-API-Key"},"publication":{"status":"published","lastError":""}}`,
		rec.Body.String())
}

func TestGetA2AConnectSerialisesOAuthSecurity(t *testing.T) {
	stub := connectStub(models.A2AConnectDetails{
		Kind: models.A2AAgentCardSourcePlatform, IsProduction: true,
		Security: models.A2AConnectSecurity{Mode: models.A2AConnectSecurityOAuth, OAuth: &models.A2AConnectOAuth{
			Issuers:    []models.A2AConnectIssuer{{Name: "thunder", Issuer: "https://idp.example.com"}},
			Audiences:  []string{"aud"},
			HeaderName: "Authorization", AuthHeaderPrefix: "Bearer ",
		}},
		Publication: models.A2AConnectPublication{Status: string(models.A2APublicationStatusPending)},
	})
	rec := httptest.NewRecorder()

	NewA2ACatalogController(stub).GetA2AConnect(rec, agentEnvRequest(http.MethodGet, "a2a-connect", ""))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"kind":"platform","proxied":false,"isProduction":true,"gatewayUrl":"","cardUrl":"","jsonRpcUrl":"",`+
		`"security":{"mode":"oauth","oauth":{"issuers":[{"name":"thunder","issuer":"https://idp.example.com"}],`+
		`"audiences":["aud"],"headerName":"Authorization","authHeaderPrefix":"Bearer "}},`+
		`"publication":{"status":"pending","lastError":""}}`,
		rec.Body.String())
}

func TestGetA2AConnectSerialisesEmptyOAuthListsAsArrays(t *testing.T) {
	stub := connectStub(models.A2AConnectDetails{
		Kind:     models.A2AAgentCardSourcePlatform,
		Security: models.A2AConnectSecurity{Mode: models.A2AConnectSecurityOAuth, OAuth: &models.A2AConnectOAuth{}},
	})
	rec := httptest.NewRecorder()

	NewA2ACatalogController(stub).GetA2AConnect(rec, agentEnvRequest(http.MethodGet, "a2a-connect", ""))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"issuers":[]`)
	assert.Contains(t, rec.Body.String(), `"audiences":[]`)
}

func TestGetA2AConnectPassesThePathAndOrg(t *testing.T) {
	var got []string
	stub := &catalogServiceStub{ConnectFunc: func(_ context.Context, ouID, projectName, agentName, envName string) (*models.A2AConnectDetails, error) {
		got = []string{ouID, projectName, agentName, envName}
		return &models.A2AConnectDetails{}, nil
	}}

	NewA2ACatalogController(stub).GetA2AConnect(httptest.NewRecorder(), agentEnvRequest(http.MethodGet, "a2a-connect", ""))

	assert.Equal(t, []string{"ou-org", "checkout", "weather", "dev"}, got)
}

func TestGetA2AConnectMapsCardNotFound(t *testing.T) {
	stub := &catalogServiceStub{ConnectFunc: func(context.Context, string, string, string, string) (*models.A2AConnectDetails, error) {
		return nil, utils.ErrAgentCardNotFound
	}}
	rec := httptest.NewRecorder()

	NewA2ACatalogController(stub).GetA2AConnect(rec, agentEnvRequest(http.MethodGet, "a2a-connect", ""))

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetA2ACatalogEntrySerialisesAPendingAgent(t *testing.T) {
	var got []string
	stub := &catalogServiceStub{EntryFunc: func(_ context.Context, ouID, projectName, agentName string) (*models.A2ACatalogEntry, error) {
		got = []string{ouID, projectName, agentName}
		return &models.A2ACatalogEntry{
			ProjectName: "checkout", AgentName: "weather", Kind: models.A2AAgentCardSourcePlatform, Name: "weather",
			Environments: []models.A2ACatalogEnvironment{{Name: "dev", Status: models.A2AAgentCardStatusPending}},
		}, nil
	}}
	rec := httptest.NewRecorder()

	NewA2ACatalogController(stub).GetA2ACatalogEntry(rec, catalogEntryRequest())

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"ou-org", "checkout", "weather"}, got)
	assert.JSONEq(t, `{
		"projectName": "checkout", "agentName": "weather", "kind": "platform", "name": "weather",
		"description": "", "version": "", "skills": [], "leadEnvironment": "",
		"environments": [{"name": "dev", "status": "pending", "proxied": false, "cardDiffers": false, "matched": false}]
	}`, rec.Body.String())
}

func TestGetA2ACatalogEntryNotInCatalogue(t *testing.T) {
	stub := &catalogServiceStub{EntryFunc: func(context.Context, string, string, string) (*models.A2ACatalogEntry, error) {
		return nil, utils.ErrAgentNotInA2ACatalog
	}}
	rec := httptest.NewRecorder()

	NewA2ACatalogController(stub).GetA2ACatalogEntry(rec, catalogEntryRequest())

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), `"The agent is not in the A2A catalogue"`)
}
