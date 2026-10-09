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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func newTestWebhook(ouID, scope, project, agent string, envs []string, enabled bool) *models.WebhookEndpoint {
	return &models.WebhookEndpoint{
		ID: uuid.New(), OUID: ouID, Scope: scope, ProjectName: project, AgentName: agent,
		Environments: envs, Name: "hook", URL: "https://h.example.com", EventTypes: []string{"agent.deployed"},
		Enabled: enabled, SecretEncrypted: []byte{1, 2, 3},
	}
}

func TestWebhookRepository_ScopesAndMatching(t *testing.T) {
	repo := NewWebhookRepository(db.GetDB())
	ctx := context.Background()
	ou := "ou-" + uuid.NewString()[:8]
	t.Cleanup(func() { db.GetDB().Where("ou_id = ?", ou).Delete(&models.WebhookEndpoint{}) })

	orgHook := newTestWebhook(ou, models.WebhookScopeOrg, "", "", []string{}, true)
	projHook := newTestWebhook(ou, models.WebhookScopeProject, "p1", "", []string{}, true)
	devHook := newTestWebhook(ou, models.WebhookScopeAgent, "p1", "a1", []string{"dev"}, true)
	prodHook := newTestWebhook(ou, models.WebhookScopeAgent, "p1", "a1", []string{"prod"}, true)
	bothHook := newTestWebhook(ou, models.WebhookScopeAgent, "p1", "a1", []string{"dev", "prod"}, true)
	offHook := newTestWebhook(ou, models.WebhookScopeAgent, "p1", "a1", []string{"dev"}, false)
	otherAgent := newTestWebhook(ou, models.WebhookScopeAgent, "p1", "a2", []string{"dev"}, true)
	for _, h := range []*models.WebhookEndpoint{orgHook, projHook, devHook, prodHook, bothHook, offHook, otherAgent} {
		require.NoError(t, repo.CreateEndpoint(ctx, h))
	}

	list, err := repo.ListEndpoints(ctx, WebhookTarget{OUID: ou, Scope: models.WebhookScopeAgent, ProjectName: "p1", AgentName: "a1"})
	require.NoError(t, err)
	assert.Len(t, list, 4)

	// Updates write the event list as JSON and persist enabled=false.
	updated := *orgHook
	updated.EventTypes = []string{"project.created", "project.deleted"}
	updated.Enabled = false
	require.NoError(t, repo.UpdateEndpoint(ctx, &updated))
	got, err := repo.GetEndpointByID(ctx, orgHook.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"project.created", "project.deleted"}, got.EventTypes)
	assert.False(t, got.Enabled)
	updated.Enabled = true
	require.NoError(t, repo.UpdateEndpoint(ctx, &updated))

	ids := func(es []models.WebhookEndpoint) []uuid.UUID {
		out := []uuid.UUID{}
		for _, e := range es {
			out = append(out, e.ID)
		}
		return out
	}
	m, err := repo.MatchingEndpoints(ctx, ou, models.WebhookScopeAgent, "p1", "a1", "dev")
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{devHook.ID, bothHook.ID}, ids(m), "enabled endpoints that selected the environment")

	m, err = repo.MatchingEndpoints(ctx, ou, models.WebhookScopeAgent, "p1", "a1", "prod")
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{prodHook.ID, bothHook.ID}, ids(m))

	m, err = repo.MatchingEndpoints(ctx, ou, models.WebhookScopeAgent, "p1", "a1", "")
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{devHook.ID, prodHook.ID, bothHook.ID}, ids(m),
		"an agent event with no environment, such as a build, reaches every agent webhook")

	m, err = repo.MatchingEndpoints(ctx, ou, models.WebhookScopeOrg, "", "", "")
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{orgHook.ID}, ids(m))

	// Deliveries upsert per (endpoint, event).
	code := 503
	require.NoError(t, repo.RecordDelivery(ctx, &models.WebhookDelivery{
		EndpointID: devHook.ID, OUID: ou, EventID: "evt_1", EventType: "agent.deployed",
		Status: models.WebhookDeliveryPending, Attempts: 1, ResponseCode: &code, LastError: "HTTP 503",
	}))
	now := time.Now()
	require.NoError(t, repo.RecordDelivery(ctx, &models.WebhookDelivery{
		EndpointID: devHook.ID, OUID: ou, EventID: "evt_1", EventType: "agent.deployed",
		Status: models.WebhookDeliveryDelivered, Attempts: 2, DeliveredAt: &now,
	}))
	deliveries, err := repo.ListDeliveries(ctx, devHook.ID, 10)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	assert.Equal(t, models.WebhookDeliveryDelivered, deliveries[0].Status)
	assert.Equal(t, 2, deliveries[0].Attempts)

	// Deleting an agent removes its endpoints and their deliveries.
	require.NoError(t, repo.DeleteEndpointsUnder(ctx, ou, "p1", "a1"))
	list, err = repo.ListEndpoints(ctx, WebhookTarget{OUID: ou, Scope: models.WebhookScopeAgent, ProjectName: "p1", AgentName: "a1"})
	require.NoError(t, err)
	assert.Empty(t, list)
	deliveries, err = repo.ListDeliveries(ctx, devHook.ID, 10)
	require.NoError(t, err)
	assert.Empty(t, deliveries)
	_, err = repo.GetEndpointByID(ctx, otherAgent.ID)
	assert.NoError(t, err, "other agents are untouched")
}
