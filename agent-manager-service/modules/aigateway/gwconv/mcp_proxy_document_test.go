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

package gwconv_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconv"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
)

func mcpProxy() *gwmodels.MCPProxyDTO {
	created := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	updated := created.Add(time.Hour)
	return &gwmodels.MCPProxyDTO{
		ID:             "github-mcp",
		Name:           "GitHub MCP",
		Version:        "v1.0",
		Description:    ptr("GitHub MCP server proxy"),
		Context:        ptr("/github"),
		Vhost:          ptr("api.example.com"),
		McpSpecVersion: ptr("2025-03-26"),
		InCatalog:      true,
		CreatedBy:      ptr("chamila"),
		CreatedAt:      &created,
		UpdatedAt:      &updated,
		Endpoints: []gwmodels.MCPProxyEndpointDTO{{
			ID:       "primary",
			Name:     "Primary",
			Upstream: gwmodels.UpstreamConfig{Main: &gwmodels.UpstreamEndpoint{URL: "https://mcp.example.com"}},
			Environments: []gwmodels.MCPEndpointEnvironmentDTO{{
				EnvironmentUUID:  "11111111-1111-1111-1111-111111111111",
				DeploymentStatus: "Deployed",
				GatewayID:        ptr("22222222-2222-2222-2222-222222222222"),
			}},
		}},
		SubscriptionPlans: &[]string{"gold"},
	}
}

// mcpProxyDocJSON is a posted document; spec overrides the defaults below.
func mcpProxyDocJSON(t *testing.T, apiVersion, kind, name string, spec map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]any{"name": name},
		"spec":       spec,
	})
	require.NoError(t, err)
	return raw
}

func TestMCPProxyDocumentEnvelope(t *testing.T) {
	doc, err := gwconv.MCPProxyDocument(mcpProxy())
	require.NoError(t, err)

	assert.Equal(t, gwconv.GroupVersion, doc.ApiVersion)
	assert.Equal(t, "McpProxy", doc.Kind)
}

// The §5.2 rename: v1's id is the handle in metadata.name, and its name the label in the spec.
func TestMCPProxyDocumentRename(t *testing.T) {
	doc, err := gwconv.MCPProxyDocument(mcpProxy())
	require.NoError(t, err)

	assert.Equal(t, "github-mcp", doc.Metadata.Name)
	assert.Equal(t, "GitHub MCP", doc.Spec.DisplayName)
}

func TestMCPProxyDocumentMetadata(t *testing.T) {
	doc, err := gwconv.MCPProxyDocument(mcpProxy())
	require.NoError(t, err)

	require.NotNil(t, doc.Metadata.CreatedBy)
	assert.Equal(t, "chamila", *doc.Metadata.CreatedBy)
	require.NotNil(t, doc.Metadata.CreationTimestamp)
	assert.Equal(t, time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC), *doc.Metadata.CreationTimestamp)
	require.NotNil(t, doc.Metadata.UpdateTimestamp)
	assert.Equal(t, time.Date(2026, 1, 15, 11, 30, 0, 0, time.UTC), *doc.Metadata.UpdateTimestamp)
}

// Every spec key the document declares must arrive, or the bridge dropped one silently.
func TestMCPProxyDocumentCarriesEverySpecKey(t *testing.T) {
	doc, err := gwconv.MCPProxyDocument(mcpProxy())
	require.NoError(t, err)

	raw, err := json.Marshal(doc.Spec)
	require.NoError(t, err)
	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &got))

	// projectName is absent on an org-level proxy, which this one is.
	for _, key := range specKeysOf(gwspec.McpProxySpec{}) {
		if key == "projectName" {
			continue
		}
		assert.Contains(t, got, key, "the bridge dropped spec.%s", key)
	}
	assert.Equal(t, "https://mcp.example.com",
		*doc.Spec.Endpoints[0].Upstream.Main.Url, "the nested endpoint survived the bridge")
	assert.Equal(t, "Deployed", *doc.Spec.Endpoints[0].Environments[0].DeploymentStatus)
}

// specKeysOf lists a generated spec type's JSON keys.
func specKeysOf(v any) []string {
	t := reflect.TypeOf(v)
	keys := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		tag := t.Field(i).Tag.Get("json")
		if name, _, _ := strings.Cut(tag, ","); name != "" && name != "-" {
			keys = append(keys, name)
		}
	}
	return keys
}

func TestMCPProxyDocumentNeedsAHandle(t *testing.T) {
	_, err := gwconv.MCPProxyDocument(nil)
	require.Error(t, err)

	proxy := mcpProxy()
	proxy.ID = ""
	_, err = gwconv.MCPProxyDocument(proxy)
	require.ErrorContains(t, err, "cannot name the document")
}

// A project-owned proxy is read only here; the document says so on spec.projectName.
func TestMCPProxyDocumentCarriesTheOwningProject(t *testing.T) {
	proxy := mcpProxy()
	proxy.ProjectName = ptr("shop")

	doc, err := gwconv.MCPProxyDocument(proxy)
	require.NoError(t, err)
	require.NotNil(t, doc.Spec.ProjectName)
	assert.Equal(t, "shop", *doc.Spec.ProjectName)
}

func TestMCPProxyListEnvelope(t *testing.T) {
	out, err := gwconv.MCPProxyList(&gwmodels.MCPProxyListResponse{
		Count: 1,
		List: []gwmodels.MCPProxyListItem{{
			ID: ptr("github-mcp"), Name: ptr("GitHub MCP"), Version: ptr("v1.0"),
			Context: ptr("/github"), Status: ptr("deployed"), CreatedBy: ptr("chamila"),
		}},
		// v1's pagination.count is the collection total.
		Pagination: gwmodels.PaginationInfo{Count: 7, Limit: 20, Offset: 0},
	})
	require.NoError(t, err)

	require.NotNil(t, out.Count)
	assert.EqualValues(t, 1, *out.Count, "count is the page size")
	require.NotNil(t, out.Pagination)
	assert.EqualValues(t, 7, *out.Pagination.Total, "total is the collection size")
	assert.EqualValues(t, 20, *out.Pagination.Limit)
	assert.EqualValues(t, 0, *out.Pagination.Offset)

	require.Len(t, out.List, 1)
	row := out.List[0]
	assert.Equal(t, gwconv.GroupVersion, row.ApiVersion)
	assert.Equal(t, "McpProxy", row.Kind)
	assert.Equal(t, "github-mcp", row.Metadata.Name)
	assert.Equal(t, "GitHub MCP", row.Spec.DisplayName, "a list row carries the required label")
	assert.Equal(t, "v1.0", row.Spec.Version, "a list row carries the required version")
	assert.Nil(t, row.Spec.Endpoints, "a list row leaves the heavy block out")
}

func TestMCPProxyListEmptyIsAnArray(t *testing.T) {
	out, err := gwconv.MCPProxyList(&gwmodels.MCPProxyListResponse{})
	require.NoError(t, err)

	raw, err := json.Marshal(out)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"list":[]`, "an empty page is an array, never null")
}

func TestMCPProxyListNeedsAHandlePerRow(t *testing.T) {
	_, err := gwconv.MCPProxyList(&gwmodels.MCPProxyListResponse{
		List: []gwmodels.MCPProxyListItem{{Name: ptr("GitHub MCP")}},
	})
	require.ErrorContains(t, err, "cannot name the document")
}

func TestMCPProxyFromDocumentMapsTheRename(t *testing.T) {
	body := mcpProxyDocJSON(t, gwconv.GroupVersion, "McpProxy", "github-mcp", map[string]any{
		"displayName": "GitHub MCP", "version": "v1.0", "context": "/github",
	})

	doc, req, err := gwconv.MCPProxyFromDocument(body)
	require.NoError(t, err)
	assert.Equal(t, "github-mcp", doc.Metadata.Name)
	assert.Equal(t, "github-mcp", req.ID, "metadata.name is the handle the service takes")
	assert.Equal(t, "GitHub MCP", req.Name, "spec.displayName is the label the service takes")
	assert.Equal(t, "v1.0", req.Version)
	assert.Equal(t, "/github", *req.Context)
}

func TestMCPProxyFromDocumentChecksApiVersionAndKind(t *testing.T) {
	spec := map[string]any{"displayName": "GitHub MCP", "version": "v1.0"}
	tests := []struct {
		name       string
		apiVersion string
		kind       string
		contains   string
	}{
		{"wrong apiVersion", "v1", "McpProxy", "apiVersion must be"},
		{"wrong kind", gwconv.GroupVersion, "LlmProvider", `kind must be "McpProxy"`},
		{"no kind", gwconv.GroupVersion, "", `kind must be "McpProxy"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := gwconv.MCPProxyFromDocument(mcpProxyDocJSON(t, tc.apiVersion, tc.kind, "github-mcp", spec))
			require.ErrorIs(t, err, gwconv.ErrDocument)
			assert.ErrorContains(t, err, tc.contains)
		})
	}
}

func TestMCPProxyFromDocumentChecksRequiredFields(t *testing.T) {
	_, _, err := gwconv.MCPProxyFromDocument(mcpProxyDocJSON(t, gwconv.GroupVersion, "McpProxy", " ",
		map[string]any{"displayName": "GitHub MCP", "version": "v1.0"}))
	require.ErrorIs(t, err, gwconv.ErrDocument)
	assert.ErrorContains(t, err, "metadata.name is required")

	_, _, err = gwconv.MCPProxyFromDocument(mcpProxyDocJSON(t, gwconv.GroupVersion, "McpProxy", "github-mcp",
		map[string]any{"version": "v1.0"}))
	require.ErrorIs(t, err, gwconv.ErrDocument)
	assert.ErrorContains(t, err, "spec.displayName is required")

	// A document with no spec at all is refused on the first required field inside it.
	_, _, err = gwconv.MCPProxyFromDocument([]byte(
		`{"apiVersion":"` + gwconv.GroupVersion + `","kind":"McpProxy","metadata":{"name":"github-mcp"}}`))
	require.ErrorIs(t, err, gwconv.ErrDocument)
	assert.ErrorContains(t, err, "spec.displayName is required")
}

// An absent endpoints key leaves the stored endpoints alone; a declared empty one is a change the
// service sees. Collapsing the two would silently undeploy every binding.
func TestMCPProxyFromDocumentKeepsEndpointsOmittedApartFromEmpty(t *testing.T) {
	spec := map[string]any{"displayName": "GitHub MCP", "version": "v1.0"}

	_, omitted, err := gwconv.MCPProxyFromDocument(mcpProxyDocJSON(t, gwconv.GroupVersion, "McpProxy", "github-mcp", spec))
	require.NoError(t, err)
	assert.Nil(t, omitted.Endpoints, "an absent key stays absent")

	spec["endpoints"] = []any{}
	_, declared, err := gwconv.MCPProxyFromDocument(mcpProxyDocJSON(t, gwconv.GroupVersion, "McpProxy", "github-mcp", spec))
	require.NoError(t, err)
	require.NotNil(t, declared.Endpoints, "a declared empty list is not an absent one")
	assert.Empty(t, declared.Endpoints)
}

// A flat body rendered as a document and read back must be the same body.
func TestMCPProxyRoundTripThroughTheDocument(t *testing.T) {
	proxy := mcpProxy()
	doc, err := gwconv.MCPProxyDocument(proxy)
	require.NoError(t, err)

	body, err := json.Marshal(doc)
	require.NoError(t, err)
	_, back, err := gwconv.MCPProxyFromDocument(body)
	require.NoError(t, err)

	assert.Equal(t, proxy.ID, back.ID)
	assert.Equal(t, proxy.Name, back.Name)
	assert.Equal(t, proxy.Version, back.Version)
	assert.Equal(t, proxy.Description, back.Description)
	assert.Equal(t, proxy.Context, back.Context)
	assert.Equal(t, proxy.Vhost, back.Vhost)
	assert.Equal(t, proxy.McpSpecVersion, back.McpSpecVersion)
	assert.Equal(t, proxy.InCatalog, back.InCatalog)
	assert.Equal(t, proxy.Endpoints, back.Endpoints)
}

// A body that is not JSON at all is not a document the server can name a field in; the handler
// answers it with v1's message rather than a parser's.
func TestMCPProxyFromDocumentSeparatesAMalformedBody(t *testing.T) {
	_, _, err := gwconv.MCPProxyFromDocument([]byte("{"))
	require.ErrorIs(t, err, gwconv.ErrMalformedBody)
	assert.NotErrorIs(t, err, gwconv.ErrDocument)
}

func TestMCPProxyListNeedsAPage(t *testing.T) {
	_, err := gwconv.MCPProxyList(nil)
	require.ErrorContains(t, err, "no MCP proxy page")
}

// A spec key the document ignores but the body types differently is refused as a document error.
func TestMCPProxyFromDocumentRejectsASpecTheBodyCannotTake(t *testing.T) {
	_, _, err := gwconv.MCPProxyFromDocument(mcpProxyDocJSON(t, gwconv.GroupVersion, "McpProxy", "github-mcp",
		map[string]any{"displayName": "GitHub MCP", "version": "v1.0", "gateways": "not-a-list"}))
	require.ErrorIs(t, err, gwconv.ErrDocument)
}
