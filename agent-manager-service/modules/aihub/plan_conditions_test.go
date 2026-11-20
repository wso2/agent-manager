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
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cond(t, id, params string) Condition {
	return Condition{Type: t, ID: id, Params: json.RawMessage(params)}
}

func TestNormalizeConditions_FillsDefaults(t *testing.T) {
	got, err := normalizeConditions([]Condition{
		cond(ConditionTypeRequestCount, "", `{"limits":[{"count":100,"timeUnit":"minute"}]}`),
		cond(ConditionTypeToolCallCount, "tool-calls", `{"limits":[{"count":3,"timeAmount":1,"timeUnit":"MINUTE"}],"onExceed":"allow"}`),
		cond(ConditionTypeTokenCount, "", `{"limits":[{"count":5000,"timeAmount":1,"timeUnit":"DAY"}]}`),
		cond(ConditionTypeCustom, "region", `{"b":2,"a":"eu"}`),
	})
	require.NoError(t, err)
	require.Len(t, got, 4)
	assert.Equal(t, ConditionIDRequests, got[0].ID)
	assert.JSONEq(t, `{"limits":[{"count":100,"timeAmount":1,"timeUnit":"MINUTE"}],"onExceed":"reject"}`, string(got[0].Params))
	assert.JSONEq(t, `{"limits":[{"count":3,"timeAmount":1,"timeUnit":"MINUTE"}],"onExceed":"allow"}`, string(got[1].Params))
	assert.JSONEq(t, `{"limits":[{"count":5000,"timeAmount":1,"timeUnit":"DAY"}],"onExceed":"reject","tokenType":"total"}`, string(got[2].Params))
	assert.Equal(t, `{"a":"eu","b":2}`, string(got[3].Params), "custom params are written one way (sorted keys)")
}

func TestNormalizeConditions_NoneIsUnlimited(t *testing.T) {
	got, err := normalizeConditions(nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestNormalizeConditions_Rejects(t *testing.T) {
	limit := `{"limits":[{"count":1,"timeAmount":1,"timeUnit":"MINUTE"}]}`
	tooMany := make([]Condition, 0, maxPlanConditions+1)
	for i := 0; i <= maxPlanConditions; i++ {
		tooMany = append(tooMany, cond(ConditionTypeCustom, fmt.Sprintf("c%d", i), `{}`))
	}
	windows := make([]string, 0, maxConditionWindows+1)
	for i := 1; i <= maxConditionWindows+1; i++ {
		windows = append(windows, fmt.Sprintf(`{"count":1,"timeAmount":%d,"timeUnit":"MINUTE"}`, i))
	}
	cases := []struct {
		name       string
		conditions []Condition
		want       string
	}{
		{"too many conditions", tooMany, "at most 20 conditions"},
		{"missing type", []Condition{cond("", "", limit)}, "type is required"},
		{"unknown type", []Condition{cond("event-count", "", limit)}, "type must be one of"},
		{"wrong built-in id", []Condition{cond(ConditionTypeRequestCount, "reqs", limit)}, "the id of a request-count condition is requests"},
		{"two of one built-in type", []Condition{cond(ConditionTypeRequestCount, "", limit), cond(ConditionTypeRequestCount, "", limit)}, "only one request-count"},
		{"no params", []Condition{cond(ConditionTypeRequestCount, "", ``)}, "params with at least one limit is required"},
		{"unknown param", []Condition{cond(ConditionTypeRequestCount, "", `{"limits":[{"count":1,"timeUnit":"MINUTE"}],"burst":2}`)}, "invalid params"},
		{"no limits", []Condition{cond(ConditionTypeRequestCount, "", `{"limits":[]}`)}, "needs at least one limit"},
		{"too many limits", []Condition{cond(ConditionTypeRequestCount, "", `{"limits":[`+strings.Join(windows, ",")+`]}`)}, "at most 10 limits"},
		{"count zero", []Condition{cond(ConditionTypeRequestCount, "", `{"limits":[{"count":0,"timeUnit":"MINUTE"}]}`)}, "count must be at least 1"},
		{"negative time amount", []Condition{cond(ConditionTypeRequestCount, "", `{"limits":[{"count":1,"timeAmount":-1,"timeUnit":"MINUTE"}]}`)}, "timeAmount must be at least 1"},
		{"bad unit", []Condition{cond(ConditionTypeRequestCount, "", `{"limits":[{"count":1,"timeUnit":"WEEK"}]}`)}, "timeUnit must be one of"},
		{"same window length", []Condition{cond(ConditionTypeRequestCount, "", `{"limits":[{"count":1,"timeAmount":60,"timeUnit":"MINUTE"},{"count":2,"timeAmount":1,"timeUnit":"HOUR"}]}`)}, "window of its own length"},
		{"bad onExceed", []Condition{cond(ConditionTypeRequestCount, "", `{"limits":[{"count":1,"timeUnit":"MINUTE"}],"onExceed":"queue"}`)}, "onExceed must be reject or allow"},
		{"tokenType off token-count", []Condition{cond(ConditionTypeToolCallCount, "", `{"limits":[{"count":1,"timeUnit":"MINUTE"}],"tokenType":"total"}`)}, "for token-count conditions only"},
		{"bad tokenType", []Condition{cond(ConditionTypeTokenCount, "", `{"limits":[{"count":1,"timeUnit":"MINUTE"}],"tokenType":"cached"}`)}, "tokenType must be total, prompt or completion"},
		{"custom without id", []Condition{cond(ConditionTypeCustom, "", `{}`)}, "a custom condition needs an id"},
		{"custom with built-in id", []Condition{cond(ConditionTypeCustom, "tokens", `{}`)}, "is the id of a built-in condition"},
		{"custom twice", []Condition{cond(ConditionTypeCustom, "x", `{}`), cond(ConditionTypeCustom, "x", `{}`)}, "already has a condition x"},
		{"custom without params", []Condition{cond(ConditionTypeCustom, "x", ``)}, "send {} for a custom condition"},
		{"custom params not an object", []Condition{cond(ConditionTypeCustom, "x", `[1]`)}, "must be a JSON object"},
		{"custom empty key", []Condition{cond(ConditionTypeCustom, "x", `{" ":1}`)}, "empty key"},
		{"custom params too big", []Condition{cond(ConditionTypeCustom, "x", `{"v":"`+strings.Repeat("a", maxCustomConditionParamsBytes)+`"}`)}, "at most 4096 bytes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := normalizeConditions(tc.conditions)
			require.Error(t, err)
			var v *validationError
			require.True(t, errors.As(err, &v), "a bad condition is a validation error (400), got %T", err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestSameConditions_IgnoresStoredFormatting(t *testing.T) {
	a, err := normalizeConditions([]Condition{cond(ConditionTypeCustom, "x", `{"b":1,"a":2}`)})
	require.NoError(t, err)
	// What PostgreSQL's JSONB hands back: other key order, spaces.
	b, err := normalizeConditions([]Condition{cond(ConditionTypeCustom, "x", `{"a": 2, "b": 1}`)})
	require.NoError(t, err)
	assert.True(t, sameConditions(a, b))
}

func TestWindowKey(t *testing.T) {
	assert.Equal(t, windowKey(60, "MINUTE"), windowKey(1, "HOUR"))
	assert.Equal(t, windowKey(24, "HOUR"), windowKey(1, "DAY"))
	assert.NotEqual(t, windowKey(30, "DAY"), windowKey(1, "MONTH"), "months match only months")
}
