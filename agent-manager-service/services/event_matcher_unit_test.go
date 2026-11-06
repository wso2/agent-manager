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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func matcherEvent(name string, scope events.Scope, project, agent, env string) events.Event {
	e := events.New("evt_"+uuid.NewString(), name, scope, "ou-1", project, agent, time.Now())
	e.Environment = env
	return e
}

func matcherSubscription(scope, project, agent string, types []string, envs ...string) models.EventSubscription {
	return models.EventSubscription{
		ID: uuid.New(), OUID: "ou-1", Scope: scope, ProjectName: project, AgentName: agent,
		EventTypes: types, Environments: envs, Enabled: true,
	}
}

func TestSubscriptionWants(t *testing.T) {
	all := []string{events.AllEvents}
	deployed := matcherEvent("agent.deployed", events.ScopeAgent, "payments", "refund-agent", "production")
	build := matcherEvent("agent.build_triggered", events.ScopeAgent, "payments", "refund-agent", "")
	agentCreated := matcherEvent("agent.created", events.ScopeProject, "payments", "refund-agent", "")
	projectDeleted := matcherEvent("project.deleted", events.ScopeOrg, "payments", "", "")
	test := matcherEvent(events.TypeDestinationTest, events.ScopeOrg, "", "", "")

	cases := []struct {
		name  string
		sub   models.EventSubscription
		event events.Event
		want  bool
	}{
		{"org * gets agent events", matcherSubscription("org", "", "", all), deployed, true},
		{"org * gets project events", matcherSubscription("org", "", "", all), agentCreated, true},
		{"org * gets org events", matcherSubscription("org", "", "", all), projectDeleted, true},
		{"* never gets the test event", matcherSubscription("org", "", "", all), test, false},
		{"project * gets its agents' events", matcherSubscription("project", "payments", "", all), deployed, true},
		{"project * gets its project events", matcherSubscription("project", "payments", "", all), agentCreated, true},
		{"project * skips the org event naming it", matcherSubscription("project", "payments", "", all), projectDeleted, false},
		{"project subscriptions are isolated", matcherSubscription("project", "claims", "", all), deployed, false},
		{"agent * gets its events", matcherSubscription("agent", "payments", "refund-agent", all), deployed, true},
		{"agent * skips the project event naming it", matcherSubscription("agent", "payments", "refund-agent", all), agentCreated, false},
		{"agent subscriptions are isolated", matcherSubscription("agent", "payments", "support-agent", all), deployed, false},
		{"same agent name in another project", matcherSubscription("agent", "claims", "refund-agent", all), deployed, false},
		{"listed type", matcherSubscription("agent", "payments", "refund-agent", []string{"agent.deployed"}), deployed, true},
		{"unlisted type", matcherSubscription("agent", "payments", "refund-agent", []string{"agent.updated"}), deployed, false},
		{"environment listed", matcherSubscription("org", "", "", all, "production"), deployed, true},
		{"environment not listed", matcherSubscription("org", "", "", all, "development"), deployed, false},
		{"no environment passes any filter", matcherSubscription("agent", "payments", "refund-agent", all, "development"), build, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, subscriptionWants(&tc.sub, tc.event))
		})
	}

	disabled := matcherSubscription("org", "", "", all)
	disabled.Enabled = false
	assert.False(t, subscriptionWants(&disabled, deployed), "a disabled subscription wants nothing")
}

func TestGroupByDestination(t *testing.T) {
	everyone := &models.EventDestination{ID: uuid.New(), Availability: models.DestinationAvailableAll}
	adminOnly := &models.EventDestination{ID: uuid.New(), Availability: models.DestinationAvailableOrg}
	claims := &models.EventDestination{
		ID: uuid.New(), Availability: models.DestinationAvailableProjects, Projects: []string{"claims"},
	}
	destinations := map[uuid.UUID]*models.EventDestination{everyone.ID: everyone, adminOnly.ID: adminOnly, claims.ID: claims}
	all := []string{events.AllEvents}

	orgToEveryone := matcherSubscription("org", "", "", all)
	orgToEveryone.DestinationID = everyone.ID
	agentToEveryone := matcherSubscription("agent", "payments", "refund-agent", []string{"agent.deployed"})
	agentToEveryone.DestinationID = everyone.ID
	orgToAdminOnly := matcherSubscription("org", "", "", all)
	orgToAdminOnly.DestinationID = adminOnly.ID
	projectToClaims := matcherSubscription("project", "payments", "", all) // its destination was narrowed away
	projectToClaims.DestinationID = claims.ID
	toDeleted := matcherSubscription("org", "", "", all)
	toDeleted.DestinationID = uuid.New()
	unwanted := matcherSubscription("org", "", "", []string{"project.created"})
	unwanted.DestinationID = claims.ID

	subs := []models.EventSubscription{orgToEveryone, agentToEveryone, orgToAdminOnly, projectToClaims, toDeleted, unwanted}
	groups := groupByDestination(subs, destinations,
		matcherEvent("agent.deployed", events.ScopeAgent, "payments", "refund-agent", "production"))

	byDestination := map[uuid.UUID][]string{}
	for _, g := range groups {
		byDestination[g.Destination.ID] = g.SubscriptionIDs
	}
	require.Len(t, byDestination, 2, "one delivery per destination")
	assert.ElementsMatch(t, []string{orgToEveryone.ID.String(), agentToEveryone.ID.String()}, byDestination[everyone.ID],
		"both subscriptions are recorded on the single delivery")
	assert.Equal(t, []string{orgToAdminOnly.ID.String()}, byDestination[adminOnly.ID],
		"org subscriptions use org-only destinations")
	assert.NotContains(t, byDestination, claims.ID, "a destination no longer available to the project is skipped")

	again := groupByDestination(subs, destinations,
		matcherEvent("agent.deployed", events.ScopeAgent, "payments", "refund-agent", "production"))
	assert.Equal(t, groups, again, "a repeated fan-out queues the same work in the same order")
}

func TestTypesReaching(t *testing.T) {
	assert.Equal(t, []string{events.AllEvents}, typesReaching([]string{events.AllEvents}, events.ScopeAgent))
	assert.Equal(t, []string{"agent.deployed"},
		typesReaching([]string{"project.created", "agent.created", "agent.deployed"}, events.ScopeAgent))
	assert.Equal(t, []string{"agent.created", "agent.deployed"},
		typesReaching([]string{"project.created", "agent.created", "agent.deployed"}, events.ScopeProject))
	assert.Empty(t, typesReaching([]string{"project.created"}, events.ScopeProject))
}
