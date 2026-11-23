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

package aigateway

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// A channel budgets send warnings to keeps its name and is not deleted; any other goes.
func TestChannelInUseHook(t *testing.T) {
	g := migrated(t)
	ctx := context.Background()
	now := time.Now().UTC()
	org := uuid.Must(uuid.NewV7()).String()
	require.NoError(t, g.Exec(`INSERT INTO organizations (id, handle, display_name, created_at, updated_at) VALUES (?, 'o', 'O', ?, ?)`,
		org, now, now).Error)
	for _, b := range []struct{ name, channel string }{{"b-two", "finance"}, {"b-one", "finance"}, {"b-other", "ops"}} {
		require.NoError(t, g.Create(&gwmodels.Budget{
			ID: uuid.New(), OrgID: org, Name: b.name, Origin: gwmodels.BudgetOriginControlPlane, CreatedAt: now, UpdatedAt: now,
			Configuration: gwmodels.BudgetConfig{
				Status: "active", Limit: gwmodels.BudgetLimit{Amount: "10", Period: "day", Action: "allow"},
				Warnings: &gwmodels.BudgetWarnings{Thresholds: []int32{80}, Channels: []string{b.channel}},
			},
		}).Error)
	}
	hook := channelInUse(func() *gorm.DB { return g })
	ask := func(action, name string) error {
		return hook(ctx, modkit.HookRequest{
			Resource: modkit.ResourceEventDestination, Action: action, OrgID: org,
			Object: map[string]any{"name": name},
		})
	}
	var refused *modkit.Error
	require.True(t, errors.As(ask(modkit.ActionDelete, "finance"), &refused))
	assert.Equal(t, http.StatusConflict, refused.Status)
	assert.Equal(t, "budget(s) b-one, b-two send warnings to it; remove it from them first", refused.Message)
	require.True(t, errors.As(ask(modkit.ActionUpdate, "ops"), &refused))
	assert.Contains(t, refused.Message, "b-other")
	assert.Contains(t, refused.Message, "before renaming it")
	assert.NoError(t, ask(modkit.ActionDelete, "unused"))
}
