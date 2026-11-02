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
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

var (
	orgTarget     = repositories.EventTarget{OUID: "ou-1", Scope: models.EventScopeOrg}
	projectTarget = repositories.EventTarget{OUID: "ou-1", Scope: models.EventScopeProject, ProjectName: "payments"}
	agentTarget   = repositories.EventTarget{
		OUID: "ou-1", Scope: models.EventScopeAgent, ProjectName: "payments", AgentName: "refund-agent",
	}
)

// putDestination stores a destination of ou-1 with the given availability.
func putDestination(store *memEventStore, name, availability string, projects ...string) models.EventDestination {
	if projects == nil {
		projects = []string{}
	}
	d := models.EventDestination{
		ID: uuid.New(), OUID: "ou-1", Name: name, Kind: "fake", Availability: availability, Projects: projects,
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.destinations[d.ID] = d
	return d
}

func newTestSubscriptionService() (EventSubscriptionService, *memEventStore, *repomocks.EventRepositoryMock) {
	store := newMemEventStore()
	repo := store.mock()
	return NewEventSubscriptionService(repo, slog.New(slog.DiscardHandler)), store, repo
}

func subscriptionInput(destination uuid.UUID, types ...string) EventSubscriptionInput {
	return EventSubscriptionInput{
		Name:          "  Deploys to team channel ",
		DestinationID: destination,
		EventTypes:    types,
		Environments:  []string{"production", " production", ""},
		Enabled:       true,
	}
}

func TestEventSubscriptionService_CreateNormalisesAndAudits(t *testing.T) {
	svc, store, _ := newTestSubscriptionService()
	team := putDestination(store, "#payments-team", models.DestinationAvailableProjects, "payments")
	rec := &recordingAuditor{}

	sub, err := svc.Create(auditCtx(rec), agentTarget, subscriptionInput(team.ID, "agent.deployed", "agent.deployed"))
	require.NoError(t, err)
	assert.Equal(t, "Deploys to team channel", sub.Name)
	assert.Equal(t, []string{"agent.deployed"}, sub.EventTypes)
	assert.Equal(t, []string{"production"}, sub.Environments)
	assert.Equal(t, models.EventScopeAgent, sub.Scope)
	assert.Equal(t, "refund-agent", sub.AgentName)
	assert.Equal(t, []audit.Action{audit.ActionEventSubscriptionCreate, audit.ActionEventSubscriptionCreate}, rec.actions())
}

func TestEventSubscriptionService_EventTypesFollowTheScope(t *testing.T) {
	svc, store, _ := newTestSubscriptionService()
	ctx := auditCtx(&recordingAuditor{})
	all := putDestination(store, "Everyone", models.DestinationAvailableAll)

	cases := []struct {
		target repositories.EventTarget
		types  []string
		ok     bool
	}{
		{orgTarget, []string{"project.created", "agent.deployed", "agent.created"}, true},
		{projectTarget, []string{"agent.created", "agent.deployed"}, true},
		{projectTarget, []string{"project.deleted"}, false}, // an org event
		{agentTarget, []string{"agent.deployed", events.TypeMonitorRunFailed}, true},
		{agentTarget, []string{"agent.created"}, false}, // a project event
		{agentTarget, []string{"agent.teleported"}, false},
		{agentTarget, []string{events.TypeDestinationTest}, false},
		{agentTarget, []string{events.AllEvents}, true},
		{agentTarget, []string{events.AllEvents, "agent.deployed"}, false},
		{agentTarget, []string{}, false},
		{agentTarget, []string{" "}, false},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s %v", tc.target.Scope, tc.types), func(t *testing.T) {
			_, err := svc.Create(ctx, tc.target, subscriptionInput(all.ID, tc.types...))
			if tc.ok {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, utils.ErrInvalidInput)
			}
		})
	}
}

func TestEventSubscriptionService_DestinationAvailability(t *testing.T) {
	svc, store, _ := newTestSubscriptionService()
	ctx := auditCtx(&recordingAuditor{})
	adminOnly := putDestination(store, "Admins", models.DestinationAvailableOrg)
	claims := putDestination(store, "Claims", models.DestinationAvailableProjects, "claims")
	payments := putDestination(store, "Payments", models.DestinationAvailableProjects, "payments")
	elsewhere := models.EventDestination{ID: uuid.New(), OUID: "ou-2", Name: "Other org", Availability: models.DestinationAvailableAll}
	store.destinations[elsewhere.ID] = elsewhere

	_, err := svc.Create(ctx, orgTarget, subscriptionInput(adminOnly.ID, "project.created"))
	assert.NoError(t, err, "org subscriptions may use any destination")
	_, err = svc.Create(ctx, orgTarget, subscriptionInput(claims.ID, "project.created"))
	assert.NoError(t, err)

	_, err = svc.Create(ctx, projectTarget, subscriptionInput(adminOnly.ID, "agent.created"))
	assert.ErrorIs(t, err, utils.ErrInvalidInput, "org-only destinations are closed to projects")
	_, err = svc.Create(ctx, agentTarget, subscriptionInput(claims.ID, "agent.deployed"))
	assert.ErrorIs(t, err, utils.ErrInvalidInput, "an agent uses its project's availability")
	_, err = svc.Create(ctx, agentTarget, subscriptionInput(payments.ID, "agent.deployed"))
	assert.NoError(t, err)

	_, err = svc.Create(ctx, orgTarget, subscriptionInput(elsewhere.ID, "project.created"))
	assert.ErrorIs(t, err, utils.ErrInvalidInput, "another org's destination does not exist here")
	_, err = svc.Create(ctx, orgTarget, subscriptionInput(uuid.Nil, "project.created"))
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
}

func TestEventSubscriptionService_ScopeIsolation(t *testing.T) {
	svc, store, _ := newTestSubscriptionService()
	ctx := auditCtx(&recordingAuditor{})
	all := putDestination(store, "Everyone", models.DestinationAvailableAll)
	sub, err := svc.Create(ctx, agentTarget, subscriptionInput(all.ID, "agent.deployed"))
	require.NoError(t, err)

	other := agentTarget
	other.AgentName = "support-agent"
	_, err = svc.Get(ctx, other, sub.ID)
	assert.ErrorIs(t, err, utils.ErrEventSubscriptionNotFound)
	_, err = svc.Get(ctx, projectTarget, sub.ID)
	assert.ErrorIs(t, err, utils.ErrEventSubscriptionNotFound, "the project cannot edit its agent's subscription")
	_, err = svc.Update(ctx, other, sub.ID, subscriptionInput(all.ID, "agent.deployed"))
	assert.ErrorIs(t, err, utils.ErrEventSubscriptionNotFound)
	assert.ErrorIs(t, svc.Delete(ctx, other, sub.ID), utils.ErrEventSubscriptionNotFound)
	assert.NoError(t, svc.Delete(ctx, agentTarget, sub.ID))
}

func TestEventSubscriptionService_Update(t *testing.T) {
	svc, store, _ := newTestSubscriptionService()
	ctx := auditCtx(&recordingAuditor{})
	all := putDestination(store, "Everyone", models.DestinationAvailableAll)
	other := putDestination(store, "Payments", models.DestinationAvailableProjects, "payments")
	sub, err := svc.Create(ctx, agentTarget, subscriptionInput(all.ID, "agent.deployed"))
	require.NoError(t, err)

	in := subscriptionInput(other.ID, events.AllEvents)
	in.Enabled = false
	in.Environments = nil
	updated, err := svc.Update(ctx, agentTarget, sub.ID, in)
	require.NoError(t, err)
	assert.False(t, updated.Enabled)
	assert.Equal(t, other.ID, updated.DestinationID)
	assert.Equal(t, []string{events.AllEvents}, updated.EventTypes)
	assert.Empty(t, updated.Environments, "an empty filter selects every environment")
}

func TestEventSubscriptionService_Limit(t *testing.T) {
	svc, store, _ := newTestSubscriptionService()
	ctx := auditCtx(&recordingAuditor{})
	all := putDestination(store, "Everyone", models.DestinationAvailableAll)
	for i := 0; i < maxEventSubscriptionsPerOwner; i++ {
		putSubscription(store, agentTarget, fmt.Sprintf("s%d", i), all.ID)
	}
	_, err := svc.Create(ctx, agentTarget, subscriptionInput(all.ID, "agent.deployed"))
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
	_, err = svc.Create(ctx, projectTarget, subscriptionInput(all.ID, "agent.deployed"))
	assert.NoError(t, err, "the limit is per owner")
}

func TestEventSubscriptionService_AuditFailureBlocksTheChange(t *testing.T) {
	svc, store, repo := newTestSubscriptionService()
	all := putDestination(store, "Everyone", models.DestinationAvailableAll)
	rec := &recordingAuditor{fail: errors.New("sink down")}
	_, err := svc.Create(auditCtx(rec), agentTarget, subscriptionInput(all.ID, "agent.deployed"))
	require.Error(t, err)
	assert.Empty(t, repo.CreateSubscriptionCalls(), "nothing is stored without an audit record")
}

func TestEventSubscriptionService_ListInherited(t *testing.T) {
	svc, store, repo := newTestSubscriptionService()
	all := putDestination(store, "Everyone", models.DestinationAvailableAll)
	put := func(target repositories.EventTarget, name string, types ...string) models.EventSubscription {
		s := putSubscription(store, target, name, all.ID)
		s.EventTypes = types
		store.subscriptions[s.ID] = s
		return s
	}
	own := put(agentTarget, "Own", "agent.deployed")
	fromProject := put(projectTarget, "Project deploys", "agent.deployed", "agent.created")
	projectOnly := put(projectTarget, "Project agents", "agent.created")
	fromOrg := put(orgTarget, "Audit", events.AllEvents)
	orgMixed := put(orgTarget, "Mixed", "project.created", "agent.deployed")
	orgOnly := put(orgTarget, "Org only", "project.created")
	put(repositories.EventTarget{OUID: "ou-1", Scope: models.EventScopeProject, ProjectName: "claims"}, "Elsewhere", "agent.deployed")

	var countedFor []repositories.DeliveryLocation
	repo.SubscriptionCountsFunc = func(
		_ context.Context, _ string, ids []uuid.UUID, _ time.Time, loc repositories.DeliveryLocation,
	) (repositories.SubscriptionCounts, error) {
		countedFor = append(countedFor, loc)
		out := repositories.SubscriptionCounts{}
		for _, id := range ids {
			out[id] = map[string]int{models.EventDeliveryDelivered: 1}
		}
		return out, nil
	}

	list, err := svc.List(context.Background(), agentTarget)
	require.NoError(t, err)
	require.Len(t, list.Own, 1)
	assert.Equal(t, own.ID, list.Own[0].ID)

	inherited := map[uuid.UUID][]string{}
	for _, s := range list.Inherited {
		inherited[s.ID] = s.EventTypes
	}
	assert.Equal(t, map[uuid.UUID][]string{
		fromProject.ID: {"agent.deployed"},
		fromOrg.ID:     {events.AllEvents},
		orgMixed.ID:    {"agent.deployed"},
	}, inherited, "types narrowed to what reaches an agent; subscriptions that never do are hidden")
	assert.NotContains(t, inherited, projectOnly.ID)
	assert.NotContains(t, inherited, orgOnly.ID)

	assert.Contains(t, list.Destinations, all.ID)
	assert.Equal(t, 1, list.Counts[fromOrg.ID][models.EventDeliveryDelivered])
	require.Len(t, countedFor, 2)
	assert.Equal(t, repositories.DeliveryLocation{}, countedFor[0], "own subscriptions count everything")
	assert.Equal(t, repositories.DeliveryLocation{
		ProjectName: "payments", AgentName: "refund-agent", EventScopes: []string{models.EventScopeAgent},
	}, countedFor[1], "inherited ones count only this agent's events")

	orgList, err := svc.List(context.Background(), orgTarget)
	require.NoError(t, err)
	assert.Len(t, orgList.Own, 3)
	assert.Empty(t, orgList.Inherited, "nothing is above the org")
}

func TestEventSubscriptionService_DeliveriesAreScopedToTheLevel(t *testing.T) {
	svc, _, repo := newTestSubscriptionService()
	var filters []repositories.DeliveryFilter
	repo.ListDeliveriesFunc = func(_ context.Context, f repositories.DeliveryFilter) ([]models.EventDelivery, int64, error) {
		filters = append(filters, f)
		return []models.EventDelivery{}, 0, nil
	}
	for _, target := range []repositories.EventTarget{orgTarget, projectTarget, agentTarget} {
		_, err := svc.Deliveries(context.Background(), target, DeliveryQuery{Limit: 500})
		require.NoError(t, err)
	}
	require.Len(t, filters, 3)
	assert.Equal(t, repositories.DeliveryLocation{}, filters[0].Location)
	assert.Equal(t, repositories.DeliveryLocation{
		ProjectName: "payments", EventScopes: []string{models.EventScopeProject, models.EventScopeAgent},
	}, filters[1].Location, "an org event naming the project is not the project's")
	assert.Equal(t, repositories.DeliveryLocation{
		ProjectName: "payments", AgentName: "refund-agent", EventScopes: []string{models.EventScopeAgent},
	}, filters[2].Location)
	assert.Equal(t, maxEventDeliveryLimit, filters[0].Limit, "the page size is capped")

	_, err := svc.Deliveries(context.Background(), orgTarget, DeliveryQuery{Status: "lost"})
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
	_, err = svc.Deliveries(context.Background(), orgTarget, DeliveryQuery{Offset: -1})
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
}

func TestEventSubscriptionService_RemoveForProjectAndAgent(t *testing.T) {
	svc, store, _ := newTestSubscriptionService()
	payments := putDestination(store, "Payments", models.DestinationAvailableProjects, "payments", "claims")
	putSubscription(store, projectTarget, "Project", payments.ID)
	putSubscription(store, agentTarget, "Agent", payments.ID)
	putSubscription(store, orgTarget, "Org", payments.ID)
	other := agentTarget
	other.AgentName = "support-agent"
	putSubscription(store, other, "Other agent", payments.ID)

	svc.RemoveForAgent(context.Background(), "ou-1", "payments", "refund-agent")
	assert.Len(t, store.subscriptions, 3)

	svc.RemoveForProject(context.Background(), "ou-1", "payments")
	require.Len(t, store.subscriptions, 1, "the project's and its agents' subscriptions go; the org's stays")
	for _, s := range store.subscriptions {
		assert.Equal(t, models.EventScopeOrg, s.Scope)
	}
	assert.Equal(t, []string{"claims"}, store.destinations[payments.ID].Projects,
		"the deleted project is no longer listed, so a new one by that name gets no access")
}

func TestEventSubscriptionService_EventTypes(t *testing.T) {
	svc, _, _ := newTestSubscriptionService()
	agentTypes, err := svc.EventTypes(models.EventScopeAgent)
	require.NoError(t, err)
	require.NotEmpty(t, agentTypes)
	for _, ty := range agentTypes {
		assert.Equal(t, events.ScopeAgent, ty.Scope)
	}
	projectTypes, err := svc.EventTypes(models.EventScopeProject)
	require.NoError(t, err)
	assert.Greater(t, len(projectTypes), len(agentTypes), "a project subscription can also name its agents' events")
	all, err := svc.EventTypes("")
	require.NoError(t, err)
	orgTypes, err := svc.EventTypes(models.EventScopeOrg)
	require.NoError(t, err)
	assert.Len(t, orgTypes, len(all), "an org subscription can name every type")
	_, err = svc.EventTypes("galaxy")
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
}
