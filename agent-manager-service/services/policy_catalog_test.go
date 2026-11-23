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

package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

func TestBuildPolicyCatalog(t *testing.T) {
	gwA := gatewayRef{ID: "a", Name: "A"}
	gwB := gatewayRef{ID: "b", Name: "B"}
	manifest := func(name, version string) llmPolicyManifestItem {
		return llmPolicyManifestItem{
			Name: name, Version: version, DisplayName: name + " (gw)", Description: "gw desc",
			Parameters: map[string]interface{}{"p": 1},
		}
	}
	hubMCP := HubPolicy{Name: "mcp-auth", Version: "v1.0", DisplayName: "MCP Auth", Categories: []string{"MCP"}, IsLatest: true}
	hubAI := HubPolicy{Name: "pii-mask", Version: "v1.2", DisplayName: "PII", Categories: []string{"AI"}, IsLatest: true}
	hubCors := HubPolicy{Name: "cors", Version: "v1.0", Categories: []string{"Security", "AI", "MCP"}, IsLatest: true}

	type want struct {
		Name      string
		Status    models.PolicyCatalogStatus
		Source    models.PolicyCatalogSource
		Version   string
		MissingOn []gatewayRef
	}
	tests := []struct {
		name      string
		kind      models.PolicyCatalogKind
		installed installedPolicies
		hub       []HubPolicy
		hubOK     bool
		want      []want
	}{
		{
			name: "hub MCP policy installed → installed, source hub, in mcp",
			kind: models.PolicyCatalogKindMCP,
			installed: installedPolicies{
				ByName:    map[string]llmPolicyManifestItem{"mcp-auth": manifest("mcp-auth", "v1.0.3")},
				Reporting: []gatewayRef{gwA},
			},
			hub: []HubPolicy{hubMCP}, hubOK: true,
			want: []want{{"mcp-auth", models.PolicyCatalogStatusInstalled, models.PolicyCatalogSourceHub, "v1.0.3", nil}},
		},
		{
			name: "hub MCP policy installed → absent in llm",
			kind: models.PolicyCatalogKindLLM,
			installed: installedPolicies{
				ByName:    map[string]llmPolicyManifestItem{"mcp-auth": manifest("mcp-auth", "v1.0.3")},
				Reporting: []gatewayRef{gwA},
			},
			hub: []HubPolicy{hubMCP}, hubOK: true,
			want: []want{},
		},
		{
			name: "hub AI policy not installed → not_installed, missingOn empty (installed on none)",
			kind: models.PolicyCatalogKindLLM,
			installed: installedPolicies{
				Reporting: []gatewayRef{gwA, gwB},
			},
			hub: []HubPolicy{hubAI, hubMCP}, hubOK: true,
			want: []want{{"pii-mask", models.PolicyCatalogStatusNotInstalled, models.PolicyCatalogSourceHub, "v1.2", nil}},
		},
		{
			name:      "no reporting gateways → hub policies not_installed, missingOn empty",
			kind:      models.PolicyCatalogKindLLM,
			installed: installedPolicies{},
			hub:       []HubPolicy{hubAI}, hubOK: true,
			want: []want{{"pii-mask", models.PolicyCatalogStatusNotInstalled, models.PolicyCatalogSourceHub, "v1.2", nil}},
		},
		{
			name:      "cors tagged Security,AI,MCP → present in llm",
			kind:      models.PolicyCatalogKindLLM,
			installed: installedPolicies{Reporting: []gatewayRef{gwA}},
			hub:       []HubPolicy{hubCors}, hubOK: true,
			want: []want{{"cors", models.PolicyCatalogStatusNotInstalled, models.PolicyCatalogSourceHub, "v1.0", nil}},
		},
		{
			name:      "cors tagged Security,AI,MCP → present in mcp",
			kind:      models.PolicyCatalogKindMCP,
			installed: installedPolicies{Reporting: []gatewayRef{gwA}},
			hub:       []HubPolicy{hubCors}, hubOK: true,
			want: []want{{"cors", models.PolicyCatalogStatusNotInstalled, models.PolicyCatalogSourceHub, "v1.0", nil}},
		},
		{
			name: "custom installed → in both kinds (llm), source custom",
			kind: models.PolicyCatalogKindLLM,
			installed: installedPolicies{
				ByName:    map[string]llmPolicyManifestItem{"my-policy": manifest("my-policy", "v2.0.0")},
				Custom:    map[string]bool{"my-policy": true},
				Reporting: []gatewayRef{gwA},
			},
			hub: []HubPolicy{hubAI}, hubOK: true,
			want: []want{
				{"my-policy", models.PolicyCatalogStatusInstalled, models.PolicyCatalogSourceCustom, "v2.0.0", nil},
				{"pii-mask", models.PolicyCatalogStatusNotInstalled, models.PolicyCatalogSourceHub, "v1.2", nil},
			},
		},
		{
			name: "custom installed → in both kinds (mcp), source custom",
			kind: models.PolicyCatalogKindMCP,
			installed: installedPolicies{
				ByName:    map[string]llmPolicyManifestItem{"my-policy": manifest("my-policy", "v2.0.0")},
				Custom:    map[string]bool{"my-policy": true},
				Reporting: []gatewayRef{gwA},
			},
			hub: []HubPolicy{hubAI}, hubOK: true,
			want: []want{{"my-policy", models.PolicyCatalogStatusInstalled, models.PolicyCatalogSourceCustom, "v2.0.0", nil}},
		},
		{
			name: "installed wso2 policy unknown to hub → absent in mcp",
			kind: models.PolicyCatalogKindMCP,
			installed: installedPolicies{
				ByName:    map[string]llmPolicyManifestItem{"mystery": manifest("mystery", "v1.0.0")},
				Reporting: []gatewayRef{gwA},
			},
			hub: []HubPolicy{hubAI}, hubOK: true,
			want: []want{},
		},
		{
			name: "installed wso2 policy unknown to hub → in llm, source hub",
			kind: models.PolicyCatalogKindLLM,
			installed: installedPolicies{
				ByName:    map[string]llmPolicyManifestItem{"mystery": manifest("mystery", "v1.0.0")},
				Reporting: []gatewayRef{gwA},
			},
			hub: []HubPolicy{hubMCP}, hubOK: true,
			want: []want{{"mystery", models.PolicyCatalogStatusInstalled, models.PolicyCatalogSourceHub, "v1.0.0", nil}},
		},
		{
			name: "hubOK=false → installed non-custom policies in llm",
			kind: models.PolicyCatalogKindLLM,
			installed: installedPolicies{
				ByName:    map[string]llmPolicyManifestItem{"x": manifest("x", "v1.0.0")},
				Reporting: []gatewayRef{gwA},
			},
			hub: []HubPolicy{hubMCP, hubAI}, hubOK: false,
			want: []want{{"x", models.PolicyCatalogStatusInstalled, models.PolicyCatalogSourceHub, "v1.0.0", nil}},
		},
		{
			name: "hubOK=false → installed non-custom policies also in mcp",
			kind: models.PolicyCatalogKindMCP,
			installed: installedPolicies{
				ByName:    map[string]llmPolicyManifestItem{"mcp-rewrite": manifest("mcp-rewrite", "v1.0.0")},
				Reporting: []gatewayRef{gwA},
			},
			hub: []HubPolicy{hubMCP, hubAI}, hubOK: false,
			want: []want{{"mcp-rewrite", models.PolicyCatalogStatusInstalled, models.PolicyCatalogSourceHub, "v1.0.0", nil}},
		},
		{
			name: "hubOK=false → installed custom policy still in mcp",
			kind: models.PolicyCatalogKindMCP,
			installed: installedPolicies{
				ByName:    map[string]llmPolicyManifestItem{"x": manifest("x", "v1.0.0")},
				Custom:    map[string]bool{"x": true},
				Reporting: []gatewayRef{gwA},
			},
			hub: nil, hubOK: false,
			want: []want{{"x", models.PolicyCatalogStatusInstalled, models.PolicyCatalogSourceCustom, "v1.0.0", nil}},
		},
		{
			name: "custom installed, hub lists the name with llm-only categories → still in mcp",
			kind: models.PolicyCatalogKindMCP,
			installed: installedPolicies{
				ByName:    map[string]llmPolicyManifestItem{"pii-mask": manifest("pii-mask", "v2.0.0")},
				Custom:    map[string]bool{"pii-mask": true},
				Reporting: []gatewayRef{gwA},
			},
			hub: []HubPolicy{hubAI}, hubOK: true,
			want: []want{{"pii-mask", models.PolicyCatalogStatusInstalled, models.PolicyCatalogSourceCustom, "v2.0.0", nil}},
		},
		{
			name: "custom installed, hub lists the name with mcp-only categories → still in llm",
			kind: models.PolicyCatalogKindLLM,
			installed: installedPolicies{
				ByName:    map[string]llmPolicyManifestItem{"mcp-auth": manifest("mcp-auth", "v2.0.0")},
				Custom:    map[string]bool{"mcp-auth": true},
				Reporting: []gatewayRef{gwA},
			},
			hub: []HubPolicy{hubMCP}, hubOK: true,
			want: []want{{"mcp-auth", models.PolicyCatalogStatusInstalled, models.PolicyCatalogSourceCustom, "v2.0.0", nil}},
		},
		{
			name: "non-custom installed, hub lists it with llm-only categories → absent in mcp",
			kind: models.PolicyCatalogKindMCP,
			installed: installedPolicies{
				ByName:    map[string]llmPolicyManifestItem{"pii-mask": manifest("pii-mask", "v2.0.0")},
				Reporting: []gatewayRef{gwA},
			},
			hub: []HubPolicy{hubAI}, hubOK: true,
			want: []want{},
		},
		{
			name: "non-custom partial, unknown to hub → disabled row in llm",
			kind: models.PolicyCatalogKindLLM,
			installed: installedPolicies{
				MissingOn: map[string][]gatewayRef{"mystery": {gwB}},
				Sighted:   map[string]llmPolicyManifestItem{"mystery": {Name: "mystery", Version: "v1.4.0"}},
				Reporting: []gatewayRef{gwA, gwB},
			},
			hub: []HubPolicy{{Name: "other", Version: "v1", Categories: []string{"MCP"}}}, hubOK: true,
			want: []want{{"mystery", models.PolicyCatalogStatusNotInstalled, models.PolicyCatalogSourceHub, "v1.4.0", []gatewayRef{gwB}}},
		},
		{
			name: "non-custom partial, hub unavailable → disabled row in mcp too",
			kind: models.PolicyCatalogKindMCP,
			installed: installedPolicies{
				MissingOn: map[string][]gatewayRef{"mystery": {gwB}},
				Sighted:   map[string]llmPolicyManifestItem{"mystery": {Name: "mystery", Version: "v1.4.0"}},
				Reporting: []gatewayRef{gwA, gwB},
			},
			hubOK: false,
			want:  []want{{"mystery", models.PolicyCatalogStatusNotInstalled, models.PolicyCatalogSourceHub, "v1.4.0", []gatewayRef{gwB}}},
		},
		{
			name: "non-custom partial, hub unavailable → disabled row in llm",
			kind: models.PolicyCatalogKindLLM,
			installed: installedPolicies{
				MissingOn: map[string][]gatewayRef{"mystery": {gwB}},
				Sighted:   map[string]llmPolicyManifestItem{"mystery": {Name: "mystery", Version: "v1.4.0"}},
				Reporting: []gatewayRef{gwA, gwB},
			},
			hubOK: false,
			want:  []want{{"mystery", models.PolicyCatalogStatusNotInstalled, models.PolicyCatalogSourceHub, "v1.4.0", []gatewayRef{gwB}}},
		},
		{
			name: "non-custom partial, unknown to hub → absent in mcp",
			kind: models.PolicyCatalogKindMCP,
			installed: installedPolicies{
				MissingOn: map[string][]gatewayRef{"mystery": {gwB}},
				Sighted:   map[string]llmPolicyManifestItem{"mystery": {Name: "mystery", Version: "v1.4.0"}},
				Reporting: []gatewayRef{gwA, gwB},
			},
			hub: []HubPolicy{{Name: "other", Version: "v1", Categories: []string{"AI"}}}, hubOK: true,
			want: []want{},
		},
		{
			name: "policy on 1 of 2 gateways → not_installed, missingOn=[B]",
			kind: models.PolicyCatalogKindLLM,
			installed: installedPolicies{
				MissingOn: map[string][]gatewayRef{"pii-mask": {gwB}},
				Reporting: []gatewayRef{gwA, gwB},
			},
			hub: []HubPolicy{hubAI}, hubOK: true,
			want: []want{{"pii-mask", models.PolicyCatalogStatusNotInstalled, models.PolicyCatalogSourceHub, "v1.2", []gatewayRef{gwB}}},
		},
		{
			name: "rolling upgrade → installed at v1.0.1",
			kind: models.PolicyCatalogKindLLM,
			installed: installedPolicies{
				ByName:    map[string]llmPolicyManifestItem{"pii-mask": manifest("pii-mask", "v1.0.1")},
				Reporting: []gatewayRef{gwA, gwB},
			},
			hub: []HubPolicy{hubAI}, hubOK: true,
			want: []want{{"pii-mask", models.PolicyCatalogStatusInstalled, models.PolicyCatalogSourceHub, "v1.0.1", nil}},
		},
		{
			name: "custom name only in MissingOn → not_installed, source custom",
			kind: models.PolicyCatalogKindMCP,
			installed: installedPolicies{
				Custom:    map[string]bool{"my-policy": true},
				MissingOn: map[string][]gatewayRef{"my-policy": {gwB}},
				Reporting: []gatewayRef{gwA, gwB},
			},
			hub: []HubPolicy{hubAI}, hubOK: true,
			want: []want{{"my-policy", models.PolicyCatalogStatusNotInstalled, models.PolicyCatalogSourceCustom, "", []gatewayRef{gwB}}},
		},
		{
			name: "custom name in neither ByName nor MissingOn → absent",
			kind: models.PolicyCatalogKindMCP,
			installed: installedPolicies{
				Custom:    map[string]bool{"ghost": true},
				Reporting: []gatewayRef{gwA},
			},
			hub: nil, hubOK: true,
			want: []want{},
		},
		{
			name: "duplicate hub entries → latest wins",
			kind: models.PolicyCatalogKindLLM,
			installed: installedPolicies{
				Reporting: []gatewayRef{gwA},
			},
			hub: []HubPolicy{
				{Name: "pii-mask", Version: "v1.2", Categories: []string{"AI"}, IsLatest: false},
				{Name: "pii-mask", Version: "v1.1", Categories: []string{"AI"}, IsLatest: true},
			}, hubOK: true,
			want: []want{{"pii-mask", models.PolicyCatalogStatusNotInstalled, models.PolicyCatalogSourceHub, "v1.1", nil}},
		},
		{
			name: "duplicate hub entries, none latest → highest version wins",
			kind: models.PolicyCatalogKindLLM,
			installed: installedPolicies{
				Reporting: []gatewayRef{gwA},
			},
			hub: []HubPolicy{
				{Name: "pii-mask", Version: "v1.1", Categories: []string{"AI"}},
				{Name: "pii-mask", Version: "v1.10", Categories: []string{"AI"}},
				{Name: "pii-mask", Version: "v1.2", Categories: []string{"AI"}},
			}, hubOK: true,
			want: []want{{"pii-mask", models.PolicyCatalogStatusNotInstalled, models.PolicyCatalogSourceHub, "v1.10", nil}},
		},
		{
			name: "ordering → installed before not_installed, then by name",
			kind: models.PolicyCatalogKindLLM,
			installed: installedPolicies{
				ByName: map[string]llmPolicyManifestItem{
					"zeta": manifest("zeta", "v1.0.0"), "alpha": manifest("alpha", "v1.0.0"),
				},
				Reporting: []gatewayRef{gwA},
			},
			hub: []HubPolicy{
				{Name: "beta", Version: "v1.0", Categories: []string{"Guardrails"}},
				{Name: "aaa", Version: "v1.0", Categories: []string{"AI"}},
			}, hubOK: true,
			want: []want{
				{"alpha", models.PolicyCatalogStatusInstalled, models.PolicyCatalogSourceHub, "v1.0.0", nil},
				{"zeta", models.PolicyCatalogStatusInstalled, models.PolicyCatalogSourceHub, "v1.0.0", nil},
				{"aaa", models.PolicyCatalogStatusNotInstalled, models.PolicyCatalogSourceHub, "v1.0", nil},
				{"beta", models.PolicyCatalogStatusNotInstalled, models.PolicyCatalogSourceHub, "v1.0", nil},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPolicyCatalog(tt.kind, tt.installed, tt.hub, tt.hubOK)
			gotWant := make([]want, 0, len(got))
			for _, item := range got {
				gotWant = append(gotWant, want{item.Name, item.Status, item.Source, item.Version, item.MissingOn})
				assert.NotNil(t, item.Categories, "categories must never be nil: %s", item.Name)
			}
			assert.Equal(t, tt.want, gotWant)
		})
	}
}

func TestBuildPolicyCatalog_InstalledItemDetails(t *testing.T) {
	installed := installedPolicies{
		ByName: map[string]llmPolicyManifestItem{
			"pii-mask": {
				Name: "pii-mask", Version: "v1.0.1", DisplayName: "", Description: "gw desc",
				Parameters:       map[string]interface{}{"p": 1},
				SystemParameters: map[string]interface{}{"s": 2},
			},
		},
	}
	hub := []HubPolicy{{Name: "pii-mask", Version: "v1.0", DisplayName: "PII Hub", Description: "hub desc", Categories: []string{"AI"}}}

	got := buildPolicyCatalog(models.PolicyCatalogKindLLM, installed, hub, true)

	assert.Len(t, got, 1)
	assert.Equal(t, "PII Hub", got[0].DisplayName, "falls back to hub display name")
	assert.Equal(t, "gw desc", got[0].Description)
	assert.Equal(t, []string{"AI"}, got[0].Categories)
	assert.Equal(t, map[string]interface{}{"p": 1}, got[0].Parameters)
	assert.Equal(t, map[string]interface{}{"s": 2}, got[0].SystemParameters)
}

// A custom policy on only some gateways is disabled, but still shows the name, version and description the gateways
// that have it reported, instead of the raw policy name with no version.
func TestBuildPolicyCatalog_PartialCustomUsesSightedDefinition(t *testing.T) {
	gwB := gatewayRef{ID: "b", Name: "gw-b"}
	installed := installedPolicies{
		ByName:    map[string]llmPolicyManifestItem{},
		Custom:    map[string]bool{"my-guard": true},
		MissingOn: map[string][]gatewayRef{"my-guard": {gwB}},
		Sighted: map[string]llmPolicyManifestItem{
			"my-guard": {
				Name: "my-guard", Version: "v1.2.0", DisplayName: "My Guard", Description: "gw desc",
				Parameters: map[string]interface{}{"p": 1},
			},
		},
	}

	for _, kind := range []models.PolicyCatalogKind{models.PolicyCatalogKindLLM, models.PolicyCatalogKindMCP} {
		got := buildPolicyCatalog(kind, installed, nil, true)

		assert.Len(t, got, 1, kind)
		assert.Equal(t, models.PolicyCatalogStatusNotInstalled, got[0].Status)
		assert.Equal(t, models.PolicyCatalogSourceCustom, got[0].Source)
		assert.Equal(t, "v1.2.0", got[0].Version)
		assert.Equal(t, "My Guard", got[0].DisplayName)
		assert.Equal(t, "gw desc", got[0].Description)
		assert.Equal(t, []gatewayRef{gwB}, got[0].MissingOn)
		assert.Nil(t, got[0].Parameters, "a disabled row carries no settings form")
	}
}

// When the hub happens to know a custom policy's name, the gateway's own definition still wins for the row.
func TestBuildPolicyCatalog_PartialCustomPrefersGatewayOverHub(t *testing.T) {
	installed := installedPolicies{
		ByName:    map[string]llmPolicyManifestItem{},
		Custom:    map[string]bool{"my-guard": true},
		MissingOn: map[string][]gatewayRef{"my-guard": {{ID: "b", Name: "gw-b"}}},
		Sighted: map[string]llmPolicyManifestItem{
			"my-guard": {Name: "my-guard", Version: "v1.2.0", DisplayName: "My Guard", Description: "gw desc"},
		},
	}
	hub := []HubPolicy{{Name: "my-guard", Version: "1.0", DisplayName: "Hub Guard", Description: "hub desc", Categories: []string{"AI"}}}

	got := buildPolicyCatalog(models.PolicyCatalogKindMCP, installed, hub, true)

	assert.Len(t, got, 1)
	assert.Equal(t, "v1.2.0", got[0].Version)
	assert.Equal(t, "My Guard", got[0].DisplayName)
	assert.Equal(t, "gw desc", got[0].Description)
	assert.Equal(t, []string{"AI"}, got[0].Categories)
}

// An installed row with no description of its own shows the hub's.
func TestBuildPolicyCatalog_InstalledDescriptionFallsBackToHub(t *testing.T) {
	installed := installedPolicies{
		ByName: map[string]llmPolicyManifestItem{"pii-mask": {Name: "pii-mask", Version: "v1.0.1"}},
	}
	hub := []HubPolicy{{Name: "pii-mask", Version: "v1.0", Description: "hub desc", Categories: []string{"AI"}}}

	got := buildPolicyCatalog(models.PolicyCatalogKindLLM, installed, hub, true)

	assert.Len(t, got, 1)
	assert.Equal(t, "hub desc", got[0].Description)
}

// A partial non-custom policy the hub does not know is a disabled row built from the gateways' own definition.
func TestBuildPolicyCatalog_PartialUnknownNonCustomRowDetails(t *testing.T) {
	gwB := gatewayRef{ID: "b", Name: "gw-b"}
	installed := installedPolicies{
		MissingOn: map[string][]gatewayRef{
			"mystery": {gwB},
			"bare":    {gwB},
		},
		Sighted: map[string]llmPolicyManifestItem{
			"mystery": {
				Name: "mystery", Version: "v1.4.0", DisplayName: "Mystery", Description: "gw desc",
				Parameters: map[string]interface{}{"p": 1},
			},
		},
	}

	got := buildPolicyCatalog(models.PolicyCatalogKindLLM, installed, nil, true)

	assert.Len(t, got, 2)
	assert.Equal(t, "bare", got[0].Name)
	assert.Equal(t, "bare", got[0].DisplayName, "falls back to the bare name")
	assert.Equal(t, "mystery", got[1].Name)
	assert.Equal(t, "Mystery", got[1].DisplayName)
	assert.Equal(t, "gw desc", got[1].Description)
	assert.Equal(t, "v1.4.0", got[1].Version)
	assert.Equal(t, []string{}, got[1].Categories)
	assert.Nil(t, got[1].Parameters)
	assert.Equal(t, []gatewayRef{gwB}, got[1].MissingOn)
	assert.Equal(t, models.PolicyCatalogSourceHub, got[1].Source)
	assert.Equal(t, models.PolicyCatalogStatusNotInstalled, got[1].Status)
}

type staticHub struct {
	policies []HubPolicy
	ok       bool
}

func (h staticHub) Snapshot(context.Context) ([]HubPolicy, bool) { return h.policies, h.ok }

// availabilityKnown tells the console whether installed/not_installed means anything: it is false only when none of
// the checked gateways has reported a manifest.
func TestBuildCatalogResponse_AvailabilityKnown(t *testing.T) {
	hub := staticHub{policies: []HubPolicy{{Name: "pii-mask", Version: "v1.0", Categories: []string{"AI"}, IsLatest: true}}, ok: true}
	reported := gatewayWithLLMPolicyManifest(map[string]interface{}{"name": "pii-mask", "version": "v1.0.0"})
	reported.UUID = uuid.New()
	silent := &models.Gateway{UUID: uuid.New()}

	for name, tc := range map[string]struct {
		gateways      []*models.Gateway
		wantKnown     bool
		wantInstalled bool
	}{
		"one gateway reported":  {[]*models.Gateway{reported}, true, true},
		"one reported, one not": {[]*models.Gateway{reported, silent}, true, true},
		"none reported":         {[]*models.Gateway{silent}, false, false},
		"no gateways at all":    {nil, false, false},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &repomocks.GatewayRepositoryMock{
				ListWithFiltersFunc: func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) { return tc.gateways, nil },
			}

			resp, err := buildCatalogResponse(context.Background(), models.PolicyCatalogKindLLM, repo, nil, hub, "org", nil)

			require.NoError(t, err)
			assert.Equal(t, tc.wantKnown, resp.AvailabilityKnown)
			require.Len(t, resp.List, 1)
			assert.Equal(t, tc.wantInstalled, resp.List[0].Status == models.PolicyCatalogStatusInstalled)
		})
	}
}
