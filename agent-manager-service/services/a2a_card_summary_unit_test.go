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
)

func TestA2ACardSummaryReadsTheTopLevelFields(t *testing.T) {
	card := json.RawMessage(`{"name":"Order Support","description":"Refunds","version":"2.1.0","skills":[{"name":"issue-refund"},{"name":"order-status"}]}`)

	got := a2aCardSummaryOf(card)

	assert.Equal(t, a2aCardSummary{
		Name:        "Order Support",
		Description: "Refunds",
		Version:     "2.1.0",
		Skills:      []string{"issue-refund", "order-status"},
	}, got)
}

func TestA2ACardSummaryOfUnusableInputIsZero(t *testing.T) {
	cases := []struct {
		name string
		in   json.RawMessage
	}{
		{"nil", nil},
		{"null", json.RawMessage(`null`)},
		{"wrong types", json.RawMessage(`{"name":5,"skills":"x"}`)},
		{"malformed", json.RawMessage(`{`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, a2aCardSummary{Skills: []string{}}, a2aCardSummaryOf(tc.in))
		})
	}
}

func TestA2ACardSummarySkipsUnusableSkills(t *testing.T) {
	card := json.RawMessage(`{"skills":[1,{"name":""},{"name":"ok"}]}`)

	assert.Equal(t, []string{"ok"}, a2aCardSummaryOf(card).Skills)
}
