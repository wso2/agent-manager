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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func projectionOf(t *testing.T, card string) models.A2ACardProjection {
	t.Helper()
	p, err := a2aCardProjection(json.RawMessage(card))
	require.NoError(t, err)
	return p
}

func TestA2ACardProjectionIgnoresNestedKeyOrder(t *testing.T) {
	a := projectionOf(t, `{"a":{"x":1,"y":2},"name":"n"}`)
	b := projectionOf(t, `{"name":"n","a":{"y":2,"x":1}}`)
	assert.Equal(t, a.Fingerprint, b.Fingerprint)
	assert.Len(t, a.Fingerprint, 64)
}

func TestA2ACardProjectionIgnoresInterfaceURLButNotBinding(t *testing.T) {
	card := func(url, binding string) string {
		return `{"name":"n","supportedInterfaces":[{"url":"` + url + `","protocolBinding":"` + binding + `"}]}`
	}
	base := projectionOf(t, card("http://a", "JSONRPC"))
	assert.Equal(t, base.Fingerprint, projectionOf(t, card("http://b", "JSONRPC")).Fingerprint)
	assert.NotEqual(t, base.Fingerprint, projectionOf(t, card("http://a", "GRPC")).Fingerprint)
}

func TestA2ACardProjectionTracksSkillChanges(t *testing.T) {
	skill := func(id, modes string) string {
		return `{"id":"` + id + `","inputModes":["` + modes + `"]}`
	}
	card := func(skills ...string) string {
		return `{"name":"n","skills":[` + skills[0] + `,` + skills[1] + `]}`
	}
	base := projectionOf(t, card(skill("a", "text"), skill("b", "text")))
	assert.NotEqual(t, base.Fingerprint, projectionOf(t, card(skill("a", "image"), skill("b", "text"))).Fingerprint)
	assert.NotEqual(t, base.Fingerprint, projectionOf(t, card(skill("b", "text"), skill("a", "text"))).Fingerprint)
}

func TestA2ACardProjectionSearchTextIsPlainText(t *testing.T) {
	p, err := a2aCardProjection(json.RawMessage(`{"name":"Orders","description":"d","skills":[{"name":"s","description":"sd","tags":["refunds"],"examples":["say \"hi\" \\ now"]}]}`))
	require.NoError(t, err)
	assert.Equal(t, "Orders\nd\ns\nsd\nrefunds\nsay \"hi\" \\ now", p.SearchText)
	for _, bad := range []string{"[", "{", `\"`} {
		assert.NotContains(t, p.SearchText, bad)
	}
}

func TestA2ACardProjectionOfNilIsEmpty(t *testing.T) {
	p, err := a2aCardProjection(nil)
	require.NoError(t, err)
	assert.Equal(t, models.A2ACardProjection{}, p)
}

func TestA2ACardProjectionRejectsInvalidJSON(t *testing.T) {
	_, err := a2aCardProjection(json.RawMessage(`{`))
	assert.Error(t, err)
}

func TestA2ACardProjectionKeepsNumberPrecision(t *testing.T) {
	a := projectionOf(t, `{"v":12345678901234567890123}`)
	b := projectionOf(t, `{"v":12345678901234567890123}`)
	c := projectionOf(t, `{"v":1.2345678901234568e+22}`)
	assert.Equal(t, a.Fingerprint, b.Fingerprint)
	assert.NotEqual(t, a.Fingerprint, c.Fingerprint)
}

func TestA2ACardProjectionOfANonObjectUsesTheRawHash(t *testing.T) {
	raw := json.RawMessage(`[1,2]`)
	want, err := a2aCardHash(raw)
	require.NoError(t, err)
	p, err := a2aCardProjection(raw)
	require.NoError(t, err)
	assert.Equal(t, models.A2ACardProjection{Fingerprint: want, SearchText: ""}, p)
}
