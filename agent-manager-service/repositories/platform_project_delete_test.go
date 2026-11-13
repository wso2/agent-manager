//go:build integration

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

package repositories

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// A project's LLM proxies, like its agents, must be deleted first; the project stays.
func TestDeleteProjectRefusesWhileItHasLLMProxies(t *testing.T) {
	ou := newOrg(t)
	seedLLMProxy(t, ou)
	require.NoError(t, db.GetDB().Where("org_id = ? AND name = ?", ou, "proxy-owner").Delete(&models.Agent{}).Error)

	err := NewPlatformRepo(db.GetDB()).DeleteProject(context.Background(), ou, "checkout")

	require.ErrorIs(t, err, utils.ErrConflict)
	assert.Contains(t, err.Error(), "1 LLM proxy(ies)")
	var left int64
	require.NoError(t, db.GetDB().Model(&models.Project{}).Where("org_id = ? AND name = ?", ou, "checkout").Count(&left).Error)
	assert.EqualValues(t, 1, left)
}

// Event subscriptions scoped to the project, and its place in a destination's projects, go with it.
func TestDeleteProjectRemovesItsEventSettings(t *testing.T) {
	ctx := context.Background()
	gdb := db.GetDB()
	ou := newOrg(t)
	projectID := dbtest.Project(t, gdb, ou, "events")
	events := NewEventRepository(gdb)
	destination := &models.EventDestination{
		OUID: ou, Name: "hook", Kind: models.DestinationKindWebhook,
		Config:       json.RawMessage(`{"url":"https://hooks.example.com"}`),
		Availability: models.DestinationAvailableProjects, Projects: []string{"events"},
	}
	require.NoError(t, events.CreateDestination(ctx, destination))
	require.NoError(t, events.CreateSubscription(ctx, &models.EventSubscription{
		ID: uuid.New(), OUID: ou, Scope: models.EventScopeProject, ProjectID: &projectID, ProjectName: "events",
		Name: "sub", DestinationID: destination.ID, EventTypes: []string{"*"}, Enabled: true,
	}, nil))

	require.NoError(t, NewPlatformRepo(gdb).DeleteProject(ctx, ou, "events"))

	var subscriptions int64
	require.NoError(t, gdb.Model(&models.EventSubscription{}).Where("org_id = ?", ou).Count(&subscriptions).Error)
	assert.Zero(t, subscriptions)
	saved, err := events.GetDestination(ctx, ou, destination.ID)
	require.NoError(t, err)
	assert.Empty(t, saved.Projects)
	assert.Equal(t, models.DestinationAvailableOrg, saved.Availability, "a destination left with no project is org-only")
}
