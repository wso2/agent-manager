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

package apicatalog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/docs"
)

const sample = `openapi: 3.0.3
info:
  title: t
  version: "1"
paths:
  /things:
    parameters:
      - $ref: "#/components/parameters/Org"
    get:
      x-amp-module: core
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Thing"
    post:
      x-amp-module: gateway
      requestBody:
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/GatewayOnly"
      responses:
        "201":
          description: created
  /gw:
    get:
      x-amp-module: gateway
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/GatewayList"
  /hosted:
    get:
      x-amp-module: core
      x-amp-not-served: true
      responses:
        "501":
          $ref: "#/components/responses/NotImplemented"
components:
  responses:
    NotImplemented:
      description: not here
  parameters:
    Org:
      name: org
      in: query
      schema:
        type: string
  schemas:
    Thing:
      type: object
      properties:
        n:
          type: integer
          example: 3
        shared:
          $ref: "#/components/schemas/Shared"
    GatewayOnly:
      type: object
      properties:
        s:
          $ref: "#/components/schemas/Shared"
    GatewayList:
      type: array
      items:
        $ref: "#/components/schemas/GatewayOnly"
    Shared:
      type: string
    Orphan:
      type: object
      properties:
        o:
          $ref: "#/components/schemas/OrphanOnly"
    OrphanOnly:
      type: boolean
`

func paths(t *testing.T, s *Spec) map[string]map[string]any {
	t.Helper()
	var doc struct {
		Paths      map[string]map[string]any `json:"paths"`
		Components map[string]map[string]any `json:"components"`
	}
	require.NoError(t, json.Unmarshal(s.JSON, &doc))
	return doc.Paths
}

func components(t *testing.T, s *Spec, kind string) []string {
	t.Helper()
	var doc struct {
		Components map[string]map[string]any `json:"components"`
	}
	require.NoError(t, json.Unmarshal(s.JSON, &doc))
	var names []string
	for n := range doc.Components[kind] {
		names = append(names, n)
	}
	return names
}

func TestCoreOnlyDropsModuleOperationsAndTheirComponents(t *testing.T) {
	s, err := New([]byte(sample)).For(nil, nil)
	require.NoError(t, err)

	p := paths(t, s)
	assert.Contains(t, p, "/things")
	assert.Contains(t, p["/things"], "get")
	assert.Contains(t, p["/things"], "parameters", "path-level parameters stay with the remaining operation")
	assert.NotContains(t, p["/things"], "post")
	assert.NotContains(t, p, "/gw", "a path with no operation left is removed")
	assert.NotContains(t, p, "/hosted", "not-served operations are left out")

	assert.ElementsMatch(t, []string{"Thing", "Shared", "Orphan", "OrphanOnly"}, components(t, s, "schemas"),
		"only components no remaining operation references go; one the spec never referenced stays, with what it references")
	assert.Empty(t, components(t, s, "responses"), "a section left empty is removed")
	assert.ElementsMatch(t, []string{"Org"}, components(t, s, "parameters"))
	assert.Contains(t, string(s.YAML), "x-amp-modules: [core]")
}

func TestActiveModuleKeepsItsOperations(t *testing.T) {
	s, err := New([]byte(sample)).For([]string{"gateway", "core", "gateway"}, nil)
	require.NoError(t, err)

	p := paths(t, s)
	assert.Contains(t, p["/things"], "post")
	assert.Contains(t, p, "/gw")
	assert.NotContains(t, p, "/hosted")
	assert.ElementsMatch(t, []string{"Thing", "Shared", "GatewayOnly", "GatewayList", "Orphan", "OrphanOnly"}, components(t, s, "schemas"))

	var info struct {
		Info map[string]any `json:"info"`
	}
	require.NoError(t, json.Unmarshal(s.JSON, &info))
	assert.Equal(t, []any{"core", "gateway"}, info.Info[ModulesKey])
}

func TestJSONKeepsTypesAndOrder(t *testing.T) {
	s, err := New([]byte(sample)).For(nil, nil)
	require.NoError(t, err)
	js := string(s.JSON)
	assert.Contains(t, js, `"example":3`, "integers stay numbers")
	assert.Contains(t, js, `"version":"1"`, "quoted scalars stay strings")
	assert.Less(t, strings.Index(js, `"info"`), strings.Index(js, `"paths"`), "key order follows the YAML")
}

func TestForCachesPerModuleSet(t *testing.T) {
	c := New([]byte(sample))
	a, err := c.For([]string{"gateway"}, nil)
	require.NoError(t, err)
	b, err := c.For([]string{"core", "gateway"}, nil)
	require.NoError(t, err)
	assert.Same(t, a, b)
}

// TestRealSpec filters the shipped spec: the core alone has no gateway
// operation, and every $ref in each result resolves.
func TestRealSpec(t *testing.T) {
	c := New(docs.OpenAPI)
	for _, mods := range [][]string{nil, {"gateway", "legacy"}} {
		s, err := c.For(mods, nil)
		require.NoError(t, err)

		var doc yaml.Node
		require.NoError(t, yaml.Unmarshal(s.YAML, &doc))
		root := doc.Content[0]
		defined := map[componentRef]bool{}
		comps := mappingValue(root, "components")
		for i := 0; i+1 < len(comps.Content); i += 2 {
			kind := comps.Content[i+1]
			for j := 0; j+1 < len(kind.Content); j += 2 {
				defined[componentRef{comps.Content[i].Value, kind.Content[j].Value}] = true
			}
		}
		collectRefs(root, func(r componentRef) {
			assert.True(t, defined[r], "modules %v: dangling $ref %s/%s", mods, r.kind, r.name)
		})

		gatewayOps, notServed := 0, 0
		ps := mappingValue(root, "paths")
		for i := 0; i+1 < len(ps.Content); i += 2 {
			item := ps.Content[i+1]
			for j := 0; j+1 < len(item.Content); j += 2 {
				if !httpMethods[item.Content[j].Value] {
					continue
				}
				if v := mappingValue(item.Content[j+1], ModuleKey); v != nil && v.Value == "gateway" {
					gatewayOps++
				}
				if mappingValue(item.Content[j+1], NotServedKey) != nil {
					notServed++
				}
			}
		}
		assert.Zero(t, notServed, "modules %v", mods)
		if mods == nil {
			assert.Zero(t, gatewayOps)
		} else {
			assert.Positive(t, gatewayOps)
		}
		var v any
		require.NoError(t, json.Unmarshal(s.JSON, &v), "the JSON form is valid JSON")
	}
}

const widgetsFragment = `paths:
  /tags:
    get:
      summary: List tags
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/WidgetsTags"
components:
  schemas:
    WidgetsTags:
      type: array
      items:
        $ref: "#/components/schemas/Shared"
`

func TestModuleFragmentIsMergedUnderItsPrefix(t *testing.T) {
	frags := map[string][]byte{"widgets": []byte(widgetsFragment)}
	s, err := New([]byte(sample)).For([]string{"widgets"}, frags)
	require.NoError(t, err)
	require.Empty(t, s.Skipped)

	p := paths(t, s)
	require.Contains(t, p, "/m/widgets/tags")
	get, ok := p["/m/widgets/tags"]["get"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "widgets", get[ModuleKey], "the operation is labelled with its module")
	assert.Contains(t, components(t, s, "schemas"), "WidgetsTags")

	inactive, err := New([]byte(sample)).For(nil, frags)
	require.NoError(t, err)
	assert.NotContains(t, paths(t, inactive), "/m/widgets/tags", "an inactive module's fragment is not merged")
	assert.NotContains(t, components(t, inactive, "schemas"), "WidgetsTags")
}

func TestBadModuleFragmentIsLeftOutWhole(t *testing.T) {
	for name, frag := range map[string]string{
		"component collision": "paths:\n  /x:\n    get:\n      responses: {}\ncomponents:\n  schemas:\n    Thing:\n      type: string\n",
		"other module":        "paths:\n  /x:\n    get:\n      x-amp-module: gateway\n      responses: {}\n",
		"relative path":       "paths:\n  x:\n    get:\n      responses: {}\n",
		"not yaml":            "paths: [",
	} {
		t.Run(name, func(t *testing.T) {
			s, err := New([]byte(sample)).For([]string{"widgets"}, map[string][]byte{"widgets": []byte(frag)})
			require.NoError(t, err, "a bad fragment does not take the spec down")
			require.Len(t, s.Skipped, 1)
			assert.NotContains(t, paths(t, s), "/m/widgets/x")
			assert.Contains(t, paths(t, s), "/things")
		})
	}
}

func TestCacheFollowsFragmentContent(t *testing.T) {
	c := New([]byte(sample))
	first, err := c.For([]string{"widgets"}, nil)
	require.NoError(t, err)
	assert.NotContains(t, paths(t, first), "/m/widgets/tags", "no fragment yet")

	second, err := c.For([]string{"widgets"}, map[string][]byte{"widgets": []byte(widgetsFragment)})
	require.NoError(t, err)
	assert.Contains(t, paths(t, second), "/m/widgets/tags",
		"the same module set with a fragment is not served the cached spec built without one")

	again, err := c.For([]string{"widgets"}, map[string][]byte{"widgets": []byte(widgetsFragment)})
	require.NoError(t, err)
	assert.Same(t, second, again)
}
