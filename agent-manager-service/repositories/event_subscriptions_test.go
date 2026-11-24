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
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// newDestination stores a destination available to all of the org's projects.
func newDestination(t *testing.T, repo EventRepository, ou, name string) *models.EventDestination {
	t.Helper()
	d := &models.EventDestination{
		OUID: ou, Name: name, Kind: models.DestinationKindWebhook,
		Config:       json.RawMessage(`{"url":"https://hooks.example.com"}`),
		Availability: models.DestinationAvailableAll, Projects: []string{},
	}
	require.NoError(t, repo.CreateDestination(context.Background(), d))
	return d
}

func subscription(ou string, d *models.EventDestination, scope, project, agent, name string, types, envs []string) *models.EventSubscription {
	return &models.EventSubscription{
		OUID: ou, Scope: scope, ProjectName: project, AgentName: agent, Name: name, DestinationID: d.ID,
		EventTypes: types, Environments: envs, Enabled: true,
	}
}

func subNames(subs []models.EventSubscription) []string {
	out := []string{}
	for _, s := range subs {
		out = append(out, s.Name)
	}
	return out
}

func TestEventSubscriptionsByScope(t *testing.T) {
	ctx := context.Background()
	gdb := db.GetDB()
	repo := NewEventRepository(gdb, testSecrets(t))
	ou := newOrg(t)
	seedAgent(t, ou, "proj", "bot", "staging")
	ensureAgent(t, ou, "other", "bot2")
	dest := newDestination(t, repo, ou, "hook")

	orgSub := subscription(ou, dest, models.EventScopeOrg, "", "", "org-wide", []string{"agent.deleted", "agent.created"}, []string{"staging", models.DefaultEnvironmentName})
	projSub := subscription(ou, dest, models.EventScopeProject, "proj", "", "project", []string{"*"}, nil)
	agentSub := subscription(ou, dest, models.EventScopeAgent, "proj", "bot", "agent", []string{"agent.updated"}, nil)
	for _, s := range []*models.EventSubscription{orgSub, projSub, agentSub} {
		require.NoError(t, repo.CreateSubscription(ctx, s, nil))
	}
	assert.NotNil(t, agentSub.AgentUUID)
	assert.NotNil(t, agentSub.ProjectID)

	org := EventTarget{OUID: ou, Scope: models.EventScopeOrg}
	subs, err := repo.ListSubscriptions(ctx, org)
	require.NoError(t, err)
	require.Equal(t, []string{"org-wide"}, subNames(subs), "an org target lists only org-scoped subscriptions")
	assert.Equal(t, []string{"agent.created", "agent.deleted"}, subs[0].EventTypes, "event types sorted")
	assert.Equal(t, []string{models.DefaultEnvironmentName, "staging"}, subs[0].Environments, "environments sorted")

	agentTarget := EventTarget{OUID: ou, Scope: models.EventScopeAgent, ProjectName: "proj", AgentName: "bot"}
	got, err := repo.GetSubscription(ctx, agentTarget, agentSub.ID)
	require.NoError(t, err)
	assert.Equal(t, "proj", got.ProjectName)
	assert.Equal(t, "bot", got.AgentName)
	assert.Empty(t, got.Environments)

	// Another owner's subscription is not found, and not deleted.
	_, err = repo.GetSubscription(ctx, EventTarget{OUID: ou, Scope: models.EventScopeProject, ProjectName: "other"}, projSub.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	deleted, err := repo.DeleteSubscription(ctx, org, projSub.ID)
	require.NoError(t, err)
	assert.False(t, deleted)

	byID, err := repo.SubscriptionsByID(ctx, ou, []uuid.UUID{orgSub.ID, agentSub.ID, uuid.New()})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"org-wide", "agent"}, subNames(byID))
	other, err := repo.SubscriptionsByID(ctx, newOrg(t), []uuid.UUID{orgSub.ID})
	require.NoError(t, err)
	assert.Empty(t, other, "ids of another org are not returned")

	using, err := repo.SubscriptionsUsing(ctx, ou, dest.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"org-wide", "project", "agent"}, subNames(using), "oldest first")

	// Candidates for an event of proj/bot: the org one, the project one and the agent one, while enabled.
	cands, err := repo.CandidateSubscriptions(ctx, ou, "proj", "bot")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"org-wide", "project", "agent"}, subNames(cands))
	cands, err = repo.CandidateSubscriptions(ctx, ou, "other", "bot2")
	require.NoError(t, err)
	assert.Equal(t, []string{"org-wide"}, subNames(cands))

	projSub.Enabled = false
	projSub.EventTypes = []string{"agent.created"}
	projSub.Environments = []string{"staging"}
	require.NoError(t, repo.UpdateSubscription(ctx, projSub, nil))
	cands, err = repo.CandidateSubscriptions(ctx, ou, "proj", "bot")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"org-wide", "agent"}, subNames(cands), "a disabled subscription is no candidate")
	reread, err := repo.GetSubscription(ctx, EventTarget{OUID: ou, Scope: models.EventScopeProject, ProjectName: "proj"}, projSub.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"agent.created"}, reread.EventTypes, "the update replaces the event types")
	assert.Equal(t, []string{"staging"}, reread.Environments)

	// Removing the agent's subscriptions keeps the project's; removing the project's takes both.
	require.NoError(t, repo.DeleteSubscriptionsUnder(ctx, ou, "proj", "bot"))
	using, err = repo.SubscriptionsUsing(ctx, ou, dest.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"org-wide", "project"}, subNames(using))
	require.NoError(t, repo.DeleteSubscriptionsUnder(ctx, ou, "proj", ""))
	using, err = repo.SubscriptionsUsing(ctx, ou, dest.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"org-wide"}, subNames(using))

	// A destination still in use cannot be deleted; once free it can.
	_, err = repo.DeleteDestination(ctx, ou, dest.ID)
	require.ErrorIs(t, err, utils.ErrEventDestinationInUse)
	deleted, err = repo.DeleteSubscription(ctx, org, orgSub.ID)
	require.NoError(t, err)
	assert.True(t, deleted)
	deleted, err = repo.DeleteDestination(ctx, ou, dest.ID)
	require.NoError(t, err)
	assert.True(t, deleted)
	deleted, err = repo.DeleteDestination(ctx, ou, dest.ID)
	require.NoError(t, err)
	assert.False(t, deleted, "deleting twice reports nothing deleted")
}

func TestEventSubscriptionWriteFailuresLeaveNothing(t *testing.T) {
	ctx := context.Background()
	gdb := db.GetDB()
	repo := NewEventRepository(gdb, testSecrets(t))
	ou := newOrg(t)
	seedAgent(t, ou, "proj", "bot")
	dest := newDestination(t, repo, ou, "hook")
	count := func() int64 {
		var n int64
		require.NoError(t, gdb.Model(&models.EventSubscription{}).Where("org_id = ?", ou).Count(&n).Error)
		return n
	}

	err := repo.CreateSubscription(ctx, subscription(ou, dest, models.EventScopeOrg, "", "", "bad-env", []string{"*"}, []string{"nowhere"}), nil)
	require.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
	assert.Zero(t, count(), "the subscription row is rolled back with its children")

	err = repo.CreateSubscription(ctx, subscription(ou, dest, models.EventScopeAgent, "proj", "ghost", "a", []string{"*"}, nil), nil)
	require.ErrorIs(t, err, utils.ErrAgentNotFound)
	err = repo.CreateSubscription(ctx, subscription(ou, dest, models.EventScopeProject, "ghost", "", "p", []string{"*"}, nil), nil)
	require.ErrorIs(t, err, utils.ErrProjectNotFound)

	errVeto := errors.New("destination not available to this project")
	var seen *models.EventDestination
	err = repo.CreateSubscription(ctx, subscription(ou, dest, models.EventScopeOrg, "", "", "vetoed", []string{"*"}, nil),
		func(d *models.EventDestination) error { seen = d; return errVeto })
	require.ErrorIs(t, err, errVeto, "the check's error is returned as is")
	require.NotNil(t, seen)
	assert.Equal(t, "hook", seen.Name)

	missing := *dest
	missing.ID = uuid.New()
	err = repo.CreateSubscription(ctx, subscription(ou, &missing, models.EventScopeOrg, "", "", "x", []string{"*"}, nil), nil)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound, "a missing destination")
	assert.Zero(t, count())
}

func TestEventDestinationNamesAndAvailability(t *testing.T) {
	ctx := context.Background()
	gdb := db.GetDB()
	repo := NewEventRepository(gdb, testSecrets(t))
	ou := newOrg(t)
	ensureAgent(t, ou, "p1", "a1")
	ensureAgent(t, ou, "p2", "a2")

	first := newDestination(t, repo, ou, "Alerts")
	dup := &models.EventDestination{
		OUID: ou, Name: "aLERTS", Kind: models.DestinationKindWebhook,
		Config: json.RawMessage(`{"url":"https://x.example.com"}`), Availability: models.DestinationAvailableAll, Projects: []string{},
	}
	require.ErrorIs(t, repo.CreateDestination(ctx, dup), utils.ErrEventDestinationNameTaken, "names are unique in any case")
	newDestination(t, repo, newOrg(t), "alerts") // another org may reuse it

	shared := &models.EventDestination{
		OUID: ou, Name: "shared", Kind: models.DestinationKindWebhook,
		Config:       json.RawMessage(`{"url":"https://x.example.com"}`),
		Availability: models.DestinationAvailableProjects, Projects: []string{"p1", "p2"},
	}
	require.NoError(t, repo.CreateDestination(ctx, shared))
	single := &models.EventDestination{
		OUID: ou, Name: "single", Kind: models.DestinationKindWebhook,
		Config:       json.RawMessage(`{"url":"https://x.example.com"}`),
		Availability: models.DestinationAvailableProjects, Projects: []string{"p1"},
	}
	require.NoError(t, repo.CreateDestination(ctx, single))

	got, err := repo.DestinationsByID(ctx, ou, []uuid.UUID{first.ID, shared.ID, uuid.New()})
	require.NoError(t, err)
	require.Len(t, got, 2)

	// Losing p1, "shared" keeps p2; "single" is left with no project and becomes org-only, not available to all.
	require.NoError(t, repo.RemoveProjectFromDestinations(ctx, ou, "p1"))
	s, err := repo.GetDestination(ctx, ou, shared.ID)
	require.NoError(t, err)
	assert.Equal(t, models.DestinationAvailableProjects, s.Availability)
	assert.Equal(t, []string{"p2"}, s.Projects)
	s, err = repo.GetDestination(ctx, ou, single.ID)
	require.NoError(t, err)
	assert.Equal(t, models.DestinationAvailableOrg, s.Availability)
	assert.Empty(t, s.Projects)
	require.NoError(t, repo.RemoveProjectFromDestinations(ctx, ou, "no-such-project"))
}

func queueDelivery(t *testing.T, repo EventRepository, d *models.EventDelivery) *models.EventDelivery {
	t.Helper()
	if d.EventID == "" {
		d.EventID = "evt-" + uuid.NewString()
	}
	if d.EventType == "" {
		d.EventType = "agent.updated"
	}
	if d.SubscriptionIDs == nil {
		d.SubscriptionIDs = []string{}
	}
	require.NoError(t, repo.RecordQueuedDelivery(context.Background(), d))
	return d
}

func TestEventDeliveryLog(t *testing.T) {
	ctx := context.Background()
	gdb := db.GetDB()
	repo := NewEventRepository(gdb, testSecrets(t))
	ou := newOrg(t)
	seedAgent(t, ou, "proj", "bot", "staging")
	ensureAgent(t, ou, "other", "bot2")
	dest := newDestination(t, repo, ou, "hook")
	dest2 := newDestination(t, repo, ou, "hook2")
	sub := subscription(ou, dest, models.EventScopeOrg, "", "", "s", []string{"*"}, nil)
	require.NoError(t, repo.CreateSubscription(ctx, sub, nil))

	// Far in the past: PruneDeliveries is global, so its cutoff must not reach rows other tests write.
	base := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	at := func(m int) time.Time { return base.Add(time.Duration(m) * time.Minute) }
	inAgent := queueDelivery(t, repo, &models.EventDelivery{
		OUID: ou, DestinationID: dest.ID, SubscriptionIDs: []string{sub.ID.String(), "not-a-uuid"},
		EventScope: models.EventScopeAgent, ProjectName: "proj", AgentName: "bot", Environment: "staging",
		Status: models.EventDeliveryDelivered, CreatedAt: at(1),
	})
	queueDelivery(t, repo, &models.EventDelivery{
		OUID: ou, DestinationID: dest.ID, SubscriptionIDs: []string{sub.ID.String()},
		EventScope: models.EventScopeProject, ProjectName: "proj", Status: models.EventDeliveryFailed, Attempts: 3, CreatedAt: at(2),
	})
	queueDelivery(t, repo, &models.EventDelivery{
		OUID: ou, DestinationID: dest.ID, EventScope: models.EventScopeAgent, ProjectName: "other", AgentName: "bot2",
		Status: models.EventDeliveryPending, Attempts: 1, CreatedAt: at(3),
	})
	queueDelivery(t, repo, &models.EventDelivery{
		OUID: ou, DestinationID: dest.ID, EventScope: models.EventScopeOrg, Status: models.EventDeliverySkipped, CreatedAt: at(4),
	})
	gone := queueDelivery(t, repo, &models.EventDelivery{
		OUID: ou, DestinationID: dest2.ID, EventScope: models.EventScopeAgent, ProjectName: "deleted-proj", AgentName: "x",
		Environment: "nowhere", Status: models.EventDeliveryPending, CreatedAt: at(5),
	})

	saved, err := repo.GetDelivery(ctx, dest.ID, inAgent.EventID)
	require.NoError(t, err)
	assert.Equal(t, "proj", saved.ProjectName)
	assert.Equal(t, "bot", saved.AgentName)
	assert.Equal(t, "staging", saved.Environment)
	assert.Equal(t, []string{sub.ID.String()}, saved.SubscriptionIDs, "an unparsable subscription id is dropped")

	// A location that no longer exists is recorded without it.
	saved, err = repo.GetDelivery(ctx, dest2.ID, gone.EventID)
	require.NoError(t, err)
	assert.Empty(t, saved.ProjectName)
	assert.Empty(t, saved.AgentName)
	assert.Empty(t, saved.Environment)

	list := func(f DeliveryFilter) ([]string, int64) {
		f.OUID = ou
		if f.Limit == 0 {
			f.Limit = 50
		}
		rows, total, err := repo.ListDeliveries(ctx, f)
		require.NoError(t, err)
		statuses := []string{}
		for _, r := range rows {
			statuses = append(statuses, r.Status)
		}
		return statuses, total
	}
	all, total := list(DeliveryFilter{})
	assert.EqualValues(t, 5, total)
	assert.Equal(t, []string{"pending", "skipped", "pending", "failed", "delivered"}, all, "newest first")

	page, total := list(DeliveryFilter{Limit: 2, Offset: 1})
	assert.EqualValues(t, 5, total, "the total ignores paging")
	assert.Equal(t, []string{"skipped", "pending"}, page)

	got, _ := list(DeliveryFilter{DestinationID: &dest2.ID})
	assert.Equal(t, []string{"pending"}, got)
	got, _ = list(DeliveryFilter{SubscriptionID: &sub.ID})
	assert.Equal(t, []string{"failed", "delivered"}, got)
	got, _ = list(DeliveryFilter{Status: models.EventDeliveryPending})
	assert.Len(t, got, 2)
	got, _ = list(DeliveryFilter{Location: DeliveryLocation{ProjectName: "proj"}})
	assert.Equal(t, []string{"failed", "delivered"}, got)
	got, _ = list(DeliveryFilter{Location: DeliveryLocation{ProjectName: "proj", AgentName: "bot"}})
	assert.Equal(t, []string{"delivered"}, got)
	got, _ = list(DeliveryFilter{Location: DeliveryLocation{EventScopes: []string{models.EventScopeOrg}}})
	assert.Equal(t, []string{"skipped"}, got)
	_, total, err = repo.ListDeliveries(ctx, DeliveryFilter{OUID: newOrg(t), Limit: 10})
	require.NoError(t, err)
	assert.Zero(t, total, "another org sees none")

	counts, err := repo.SubscriptionCounts(ctx, ou, []uuid.UUID{sub.ID}, base, DeliveryLocation{})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"delivered": 1, "failed": 1}, counts[sub.ID])
	counts, err = repo.SubscriptionCounts(ctx, ou, []uuid.UUID{sub.ID}, at(2), DeliveryLocation{})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"failed": 1}, counts[sub.ID], "since bounds the window")
	counts, err = repo.SubscriptionCounts(ctx, ou, []uuid.UUID{sub.ID}, base, DeliveryLocation{ProjectName: "proj", AgentName: "bot"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"delivered": 1}, counts[sub.ID])

	// Stats: the latest non-skipped delivery, and failures or retried pendings since the last delivered one.
	stats, err := repo.DestinationStats(ctx, ou)
	require.NoError(t, err)
	s := stats[dest.ID]
	assert.Equal(t, 1, s.UsedBy)
	assert.Equal(t, models.EventDeliveryPending, s.LastStatus, "a skipped delivery is not the last status")
	require.NotNil(t, s.LastDeliveryAt)
	assert.Equal(t, at(3).Unix(), s.LastDeliveryAt.Unix())
	assert.Equal(t, 2, s.ConsecutiveFailures)
	one, err := repo.DestinationStatsOf(ctx, ou, dest2.ID)
	require.NoError(t, err)
	assert.Zero(t, one.UsedBy)
	assert.Zero(t, one.ConsecutiveFailures, "a pending delivery never attempted is not a failure")
	none, err := repo.DestinationStatsOf(ctx, ou, uuid.New())
	require.NoError(t, err)
	assert.Equal(t, DestinationStats{}, none)

	n, err := repo.PruneDeliveries(ctx, at(3))
	require.NoError(t, err)
	assert.EqualValues(t, 2, n)
	_, total = list(DeliveryFilter{})
	assert.EqualValues(t, 3, total)
	var links int64
	require.NoError(t, gdb.Table("event_delivery_subscriptions").Where("delivery_id = ?", inAgent.ID).Count(&links).Error)
	assert.Zero(t, links, "subscription links go with their delivery")
}
