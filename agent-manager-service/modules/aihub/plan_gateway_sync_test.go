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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requestCondition(params string) Condition {
	return Condition{ID: ConditionIDRequests, Type: ConditionTypeRequestCount, Params: json.RawMessage(params)}
}

func TestThrottleLimit(t *testing.T) {
	for name, tc := range map[string]struct {
		conditions []Condition
		count      int
		unit       string
		stop, ok   bool
	}{
		"the first window of one unit": {
			conditions: []Condition{requestCondition(
				`{"limits":[{"count":5,"timeAmount":10,"timeUnit":"MINUTE"},{"count":1000,"timeAmount":1,"timeUnit":"HOUR"},{"count":9000,"timeAmount":1,"timeUnit":"DAY"}],"onExceed":"reject"}`)},
			count: 1000, unit: "HOUR", stop: true, ok: true,
		},
		"allow on exceed does not stop": {
			conditions: []Condition{requestCondition(
				`{"limits":[{"count":60,"timeAmount":1,"timeUnit":"minute"}],"onExceed":"allow"}`)},
			count: 60, unit: "MINUTE", stop: false, ok: true,
		},
		"no window of one unit": {conditions: []Condition{requestCondition(
			`{"limits":[{"count":5,"timeAmount":10,"timeUnit":"MINUTE"}],"onExceed":"reject"}`)}},
		"only other kinds": {conditions: []Condition{{
			ID: ConditionIDToolCalls, Type: ConditionTypeToolCallCount,
			Params: json.RawMessage(`{"limits":[{"count":3,"timeAmount":1,"timeUnit":"MINUTE"}]}`),
		}}},
		"no conditions": {},
	} {
		t.Run(name, func(t *testing.T) {
			count, unit, stop, ok := throttleLimit(tc.conditions)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.count, count)
			assert.Equal(t, tc.unit, unit)
			assert.Equal(t, tc.stop, stop)
		})
	}
}

func TestGatewayPlan(t *testing.T) {
	limited := gatewayPlan("id-1", "gold", "Gold", "ACTIVE", []Condition{requestCondition(
		`{"limits":[{"count":1000,"timeAmount":1,"timeUnit":"HOUR"}],"onExceed":"allow"}`)}, nil)
	require.NotNil(t, limited.ThrottleLimitCount)
	assert.Equal(t, 1000, *limited.ThrottleLimitCount)
	assert.Equal(t, "HOUR", limited.ThrottleLimitUnit)
	assert.False(t, limited.StopOnQuotaReach, "the plan lets requests through")
	require.Len(t, limited.Conditions, 1)
	assert.Equal(t, "request-count", limited.Conditions[0].Type)

	unlimited := gatewayPlan("id-2", "free", "Free", "INACTIVE", nil, nil)
	assert.Nil(t, unlimited.ThrottleLimitCount)
	assert.Empty(t, unlimited.ThrottleLimitUnit)
	assert.True(t, unlimited.StopOnQuotaReach, "the default, as for plans made elsewhere")
	assert.Equal(t, "INACTIVE", unlimited.Status)
}
