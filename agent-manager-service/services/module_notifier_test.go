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

package services

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/notifyv1"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// Notify queues one event naming the usable channels, in the caller's transaction;
// Unusable reports names that are missing or not available to the project.
func TestModuleNotifier(t *testing.T) {
	ctx := context.Background()
	gdb := db.GetDB()
	org := dbtest.Org(t, gdb)
	dbtest.Project(t, gdb, org, "shop")
	repo := repositories.NewEventRepository(gdb, nil)
	add := func(name, availability string, projects ...string) uuid.UUID {
		d := &models.EventDestination{
			ID: uuid.New(), OUID: org, Name: name, Kind: models.DestinationKindWebhook,
			Config: json.RawMessage(`{"url":"https://hooks.example.com"}`), Availability: availability, Projects: projects,
		}
		if projects == nil {
			d.Projects = []string{}
		}
		require.NoError(t, repo.CreateDestination(ctx, d))
		t.Cleanup(func() { _, _ = repo.DeleteDestination(ctx, org, d.ID) })
		return d.ID
	}
	finance := add("finance", models.DestinationAvailableOrg)
	shop := add("shop-hook", models.DestinationAvailableProjects, "shop")
	n := NewModuleNotifier(gdb)

	unusable, err := n.Unusable(ctx, org, "", []string{"finance", "shop-hook", "nope"})
	require.NoError(t, err)
	assert.Equal(t, []notifyv1.Unusable{{Name: "shop-hook"}, {Name: "nope", Missing: true}}, unusable)
	unusable, err = n.Unusable(ctx, org, "shop", []string{"finance", "shop-hook"})
	require.NoError(t, err)
	assert.Empty(t, unusable)

	outbox := func(id string) *events.Event {
		var bodies []string
		require.NoError(t, gdb.Raw("SELECT body FROM event_outbox WHERE org_id = ?", org).Scan(&bodies).Error)
		for _, b := range bodies {
			var e events.Event
			require.NoError(t, json.Unmarshal([]byte(b), &e))
			if e.ID == id {
				return &e
			}
		}
		return nil
	}
	in := notifyv1.Notification{
		OrgID: org, OrgName: "acme", Channels: []string{"finance", "shop-hook", "nope"}, Type: "t-mod.crossed",
		ID: "t-mod:" + uuid.NewString(), Subject: "b1", Data: map[string]any{"message": "hi"},
	}
	rolledBack := in
	rolledBack.ID = "t-mod:" + uuid.NewString()
	_ = gdb.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, n.Notify(ctx, tx, rolledBack))
		return assert.AnError
	})
	assert.Nil(t, outbox(rolledBack.ID), "nothing is queued when the caller rolls back")

	require.NoError(t, n.Notify(ctx, nil, in))
	e := outbox(in.ID)
	require.NotNil(t, e)
	assert.Equal(t, []string{finance.String()}, e.Destinations, "an org-wide notice skips project channels and unknown names")
	assert.Equal(t, "t-mod.crossed", e.Name())
	assert.Equal(t, "acme", e.OrgHandle)
	assert.Equal(t, "b1", e.Subject)

	inShop := in
	inShop.ID, inShop.Project = "t-mod:"+uuid.NewString(), "shop"
	require.NoError(t, n.Notify(ctx, nil, inShop))
	e = outbox(inShop.ID)
	require.NotNil(t, e)
	assert.ElementsMatch(t, []string{finance.String(), shop.String()}, e.Destinations)
	assert.Equal(t, events.ScopeProject, e.Scope)

	none := in
	none.ID, none.Channels = "t-mod:"+uuid.NewString(), []string{"nope"}
	require.NoError(t, n.Notify(ctx, nil, none))
	assert.Nil(t, outbox(none.ID), "no usable channel, nothing queued")
	require.NoError(t, gdb.Exec("DELETE FROM event_outbox WHERE org_id = ?", org).Error)
}
