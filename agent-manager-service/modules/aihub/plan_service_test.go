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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The search matches names case-insensitively beyond ASCII, as stored in display_name_folded, and takes LIKE
// wildcards literally.
func TestSearchFoldsUnicodeAndEscapesWildcards(t *testing.T) {
	assert.Equal(t, foldName("équipe"), foldName("ÉQUIPE"))
	assert.Equal(t, foldName("équipe"), foldName("équipe"), "one normal form")
	clause, args := searchClause(" ÉQUIPE_1% ")
	assert.Contains(t, clause, "display_name_folded LIKE ?")
	assert.Equal(t, []any{"%équipe\\_1\\%%", "%équipe\\_1\\%%"}, args)
}

// A plan in use keeps its expiry: setting another one or clearing it is a change, sending the same one back is not.
func TestChangesExpiry(t *testing.T) {
	at := time.Date(2027, 1, 31, 23, 59, 59, 0, time.UTC)
	same := at.Add(400 * time.Millisecond)
	later := at.Add(24 * time.Hour)

	assert.False(t, changesExpiry(&at, &planUpdate{}), "left out")
	assert.False(t, changesExpiry(&at, &planUpdate{ExpiryTime: &same}), "the same second")
	assert.True(t, changesExpiry(&at, &planUpdate{ExpiryTime: &later}))
	assert.True(t, changesExpiry(&at, &planUpdate{ClearExpiryTime: true}))
	assert.True(t, changesExpiry(nil, &planUpdate{ExpiryTime: &at}), "set where there was none")
	assert.False(t, changesExpiry(nil, &planUpdate{ClearExpiryTime: true}), "cleared where there was none")
}

// The older "limits" field changes only the request-count condition; an empty one removes it.
func TestUpdatedConditions_FromTheOlderLimitsField(t *testing.T) {
	existing := &plan{Conditions: []Condition{
		{ID: ConditionIDToolCalls, Type: ConditionTypeToolCallCount, Params: []byte(`{"limits":[{"count":3,"timeAmount":1,"timeUnit":"MINUTE"}],"onExceed":"reject"}`)},
	}}
	limit := Condition{
		ID: ConditionIDRequests, Type: ConditionTypeRequestCount,
		Params: []byte(`{"limits":[{"count":10,"timeAmount":1,"timeUnit":"MINUTE"}]}`),
	}
	got, err := updatedConditions(existing, &planUpdate{RequestLimit: &limit})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, ConditionTypeRequestCount, got[0].Type)

	existing.Conditions = got
	got, err = updatedConditions(existing, &planUpdate{ClearLimit: true})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, ConditionTypeToolCallCount, got[0].Type)

	got, err = updatedConditions(existing, &planUpdate{})
	require.NoError(t, err)
	assert.Equal(t, existing.Conditions, got, "nothing sent keeps them")
}

func TestPlanEditInUseSaysWhy(t *testing.T) {
	err := planEditInUse("Gold")
	assert.Equal(t, "The plan Gold has subscriptions. To change its name, conditions or expiry, create a new plan.", err.Error())
}
