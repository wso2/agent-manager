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

package aihub

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentFor(t *testing.T) {
	created := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	p := &plan{
		UUID: "6c1f3c1e-6a43-4f0e-9d8f-0e3a0b9c2d11", Handle: "gold", Name: "Gold", Description: "For teams",
		Status: StatusActive, CreatedBy: "alice", CreatedAt: created, UpdatedAt: created.Add(time.Hour),
		Origin: OriginControlPlane,
	}

	doc := documentFor(p, nil)
	assert.Equal(t, "ai-control-plane.choreolabs.com/v1", doc.APIVersion)
	assert.Equal(t, "SubscriptionPlan", doc.Kind)
	assert.Equal(t, documentMetadata{
		Name: "gold", UID: p.UUID, CreationTimestamp: &p.CreatedAt, UpdateTimestamp: &p.UpdatedAt, CreatedBy: "alice",
		Origin: OriginControlPlane,
	}, doc.Metadata)
	assert.Equal(t, planSpec{DisplayName: "Gold", Description: "For teams", Status: StatusActive, Conditions: []Condition{}}, doc.Spec)
	assert.Equal(t, planStatus{}, doc.Status, "a current plan, and usage only in GET of one plan")

	raw, err := json.Marshal(documentFor(p, &usage{MCPProxies: 1}))
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"status":{"deprecated":false,"usage":{"inUse":false,"mcpProxies":1,"subscriptions":0}}`,
		"offering a plan doesn't put it in use")
	raw, err = json.Marshal(documentFor(p, &usage{Subscriptions: 2}))
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"usage":{"inUse":true,"mcpProxies":0,"subscriptions":2}`, "subscriptions do")

	deprecatedAt := created.Add(2 * time.Hour)
	p.DeprecatedAt = &deprecatedAt
	doc = documentFor(p, nil)
	assert.True(t, doc.Status.Deprecated)
	assert.Equal(t, &deprecatedAt, doc.Status.DeprecationTimestamp)
}

func TestYAMLDocuments(t *testing.T) {
	t.Run("YAML in reads as the same JSON, timestamps as written", func(t *testing.T) {
		got, err := yamlToJSON([]byte(`
apiVersion: ai-control-plane.choreolabs.com/v1
kind: SubscriptionPlan
metadata:
  name: gold
spec:
  displayName: Gold
  status: ACTIVE
  expiryTime: 2027-01-01T00:00:00Z
  conditions:
    - type: tool-call-count
      params:
        onExceed: reject
        limits: [{count: 30, timeAmount: 1, timeUnit: MINUTE}]
`))
		require.NoError(t, err)
		assert.JSONEq(t, `{"apiVersion":"ai-control-plane.choreolabs.com/v1","kind":"SubscriptionPlan",
			"metadata":{"name":"gold"},"spec":{"displayName":"Gold","status":"ACTIVE","expiryTime":"2027-01-01T00:00:00Z",
			"conditions":[{"type":"tool-call-count","params":{"onExceed":"reject",
			"limits":[{"count":30,"timeAmount":1,"timeUnit":"MINUTE"}]}}]}}`, string(got))
	})

	t.Run("bad YAML, an empty body and a key that isn't a string are refused", func(t *testing.T) {
		for _, in := range []string{"spec: [", "", "? [a]\n: b\n"} {
			_, err := yamlToJSON([]byte(in))
			assert.Error(t, err, in)
		}
	})

	t.Run("YAML out has the document's fields in order", func(t *testing.T) {
		out, err := jsonToYAML(documentFor(&plan{Handle: "gold", Name: "Gold", Status: StatusActive}, nil))
		require.NoError(t, err)
		text := string(out)
		order := []string{"apiVersion:", "kind:", "metadata:", "spec:"}
		for i := 1; i < len(order); i++ {
			assert.Less(t, strings.Index(text, order[i-1]), strings.Index(text, order[i]), text)
		}
		assert.Contains(t, text, "kind: SubscriptionPlan")
	})

	t.Run("media types", func(t *testing.T) {
		assert.True(t, isYAML("application/yaml; charset=utf-8"))
		assert.True(t, isYAML("application/x-yaml"))
		assert.False(t, isYAML("application/json"))
		assert.True(t, prefersYAML("application/yaml"))
		assert.True(t, prefersYAML("*/*, application/yaml;q=0.9"))
		assert.False(t, prefersYAML("application/json, application/yaml"))
		assert.False(t, prefersYAML(""))
	})
}
