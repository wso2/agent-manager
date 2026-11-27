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
package gwsvc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/subplanv1"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

func TestOfferedPlans(t *testing.T) {
	ctx := context.Background()
	plans := NewSubscriptionPlans()
	plans.Use(func() (subplanv1.Source, bool) {
		return planList{
			{ID: "1", Handle: "gold", Status: "ACTIVE"},
			{ID: "2", Handle: "silver", Status: "ACTIVE"},
			{ID: "3", Handle: "old", Status: "INACTIVE"},
			{ID: "4", Handle: "retired", Status: "ACTIVE", Deprecated: true},
		}, true
	})
	svc := &MCPProxyService{plans: plans}
	list := func(h ...string) *[]string { return &h }

	t.Run("left out keeps the stored list; empty offers none", func(t *testing.T) {
		got, err := svc.offeredPlans(ctx, "org", nil, []string{"gold"})
		require.NoError(t, err)
		assert.Nil(t, got)
		got, err = svc.offeredPlans(ctx, "org", list(), []string{"gold"})
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("trimmed, without repeats, in the order given", func(t *testing.T) {
		got, err := svc.offeredPlans(ctx, "org", list(" silver", "gold", "silver"), nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"silver", "gold"}, got)
	})

	t.Run("an added plan must exist and be active; one already offered stays", func(t *testing.T) {
		_, err := svc.offeredPlans(ctx, "org", list("nope"), nil)
		assert.True(t, errors.Is(err, httpx.ErrInvalidInput), "unknown plan: %v", err)
		_, err = svc.offeredPlans(ctx, "org", list("old"), nil)
		assert.True(t, errors.Is(err, httpx.ErrInvalidInput), "inactive plan: %v", err)
		got, err := svc.offeredPlans(ctx, "org", list("old", "gold"), []string{"old"})
		require.NoError(t, err)
		assert.Equal(t, []string{"old", "gold"}, got)
	})

	t.Run("a deprecated plan can't be added, and blocks other changes until it is removed", func(t *testing.T) {
		_, err := svc.offeredPlans(ctx, "org", list("retired"), nil)
		require.True(t, errors.Is(err, httpx.ErrInvalidInput), "deprecated plan: %v", err)
		assert.Contains(t, err.Error(), "can't be offered")

		got, err := svc.offeredPlans(ctx, "org", list("retired"), []string{"retired"})
		require.NoError(t, err, "the same list sent back is kept, so other edits of the proxy save")
		assert.Equal(t, []string{"retired"}, got)

		_, err = svc.offeredPlans(ctx, "org", list("retired", "gold"), []string{"retired"})
		require.True(t, errors.Is(err, httpx.ErrInvalidInput), "adding beside it: %v", err)
		assert.Contains(t, err.Error(), "remove it before changing the offered plans")

		_, err = svc.offeredPlans(ctx, "org", list("retired"), []string{"retired", "gold"})
		require.True(t, errors.Is(err, httpx.ErrInvalidInput), "removing another plan while keeping it: %v", err)

		got, err = svc.offeredPlans(ctx, "org", list("gold"), []string{"retired"})
		require.NoError(t, err, "removing it is allowed")
		assert.Equal(t, []string{"gold"}, got)
	})

	t.Run("an inactive plan already offered stays beside other changes", func(t *testing.T) {
		got, err := svc.offeredPlans(ctx, "org", list("old", "gold"), []string{"old"})
		require.NoError(t, err)
		assert.Equal(t, []string{"old", "gold"}, got)
	})

	t.Run("without the ai-hub module plans can still be removed", func(t *testing.T) {
		off := &MCPProxyService{plans: NewSubscriptionPlans()}
		got, err := off.offeredPlans(ctx, "org", list("gold"), []string{"gold", "silver"})
		require.NoError(t, err)
		assert.Equal(t, []string{"gold"}, got)
	})

	t.Run("a failed plan read is an error, not a refusal", func(t *testing.T) {
		broken := NewSubscriptionPlans()
		broken.Use(func() (subplanv1.Source, bool) { return failingPlans{}, true })
		_, err := (&MCPProxyService{plans: broken}).offeredPlans(ctx, "org", list("gold"), nil)
		require.Error(t, err)
		assert.False(t, errors.Is(err, httpx.ErrInvalidInput), "a server fault: %v", err)
	})

	t.Run("a proxy built without the plans holder can still drop its plans", func(t *testing.T) {
		got, err := (&MCPProxyService{}).offeredPlans(ctx, "org", list("gold"), []string{"gold", "silver"})
		require.NoError(t, err)
		assert.Equal(t, []string{"gold"}, got)
	})

	t.Run("a list of blanks is refused", func(t *testing.T) {
		_, err := svc.offeredPlans(ctx, "org", list(" ", ""), nil)
		assert.True(t, errors.Is(err, httpx.ErrInvalidInput))
	})

	t.Run("without the ai-hub module no plan can be added, but offered ones stay", func(t *testing.T) {
		off := &MCPProxyService{plans: NewSubscriptionPlans()}
		_, err := off.offeredPlans(ctx, "org", list("gold"), nil)
		assert.True(t, errors.Is(err, httpx.ErrInvalidInput))
		got, err := off.offeredPlans(ctx, "org", list("gold"), []string{"gold"})
		require.NoError(t, err)
		assert.Equal(t, []string{"gold"}, got)
	})
}

// failingPlans is a plans module whose list read fails.
type failingPlans struct{ planList }

func (failingPlans) ListPlans(context.Context, string) ([]subplanv1.Plan, error) {
	return nil, errors.New("database unavailable")
}
