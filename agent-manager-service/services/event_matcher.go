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
	"slices"
	"sort"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// subscriptionTarget is where a subscription sits, for events.Reaches.
func subscriptionTarget(s *models.EventSubscription) events.Target {
	return events.Target{Scope: events.Scope(s.Scope), Project: s.ProjectName, Agent: s.AgentName}
}

// environmentSelected reports whether an environment filter lets an event in
// env through: an empty filter selects every environment, and an event tied
// to no environment (a build, say) passes any filter.
func environmentSelected(filter []string, env string) bool {
	return len(filter) == 0 || env == "" || slices.Contains(filter, env)
}

// subscriptionWants reports whether an enabled subscription receives e, by
// every rule except its destination's availability: e reaches the
// subscription's scope and location, is a type it names (or one AllEvents
// covers), and is in an environment it selects.
func subscriptionWants(s *models.EventSubscription, e events.Event) bool {
	return s.Enabled &&
		events.Reaches(subscriptionTarget(s), e) &&
		events.Wants(events.Scope(s.Scope), s.EventTypes, e.Name()) &&
		environmentSelected(s.Environments, e.Environment)
}

// deliveryGroup is one destination an event goes to and the subscriptions
// that send it there.
type deliveryGroup struct {
	Destination     *models.EventDestination
	SubscriptionIDs []string
}

// groupByDestination returns the destinations e goes to, each once however
// many subscriptions send it there. A subscription is skipped when it does
// not want e, or when its destination is gone or no longer available to it.
// The result is ordered by destination and subscription ID, so a repeated
// fan-out queues the same work.
func groupByDestination(
	subs []models.EventSubscription, destinations map[uuid.UUID]*models.EventDestination, e events.Event,
) []deliveryGroup {
	byDestination := map[uuid.UUID]*deliveryGroup{}
	for i := range subs {
		s := &subs[i]
		if !subscriptionWants(s, e) {
			continue
		}
		d, ok := destinations[s.DestinationID]
		if !ok || !d.UsableBy(s.Scope, s.ProjectName) {
			continue
		}
		g, ok := byDestination[d.ID]
		if !ok {
			g = &deliveryGroup{Destination: d}
			byDestination[d.ID] = g
		}
		g.SubscriptionIDs = append(g.SubscriptionIDs, s.ID.String())
	}
	out := make([]deliveryGroup, 0, len(byDestination))
	for _, g := range byDestination {
		sort.Strings(g.SubscriptionIDs)
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Destination.ID.String() < out[j].Destination.ID.String()
	})
	return out
}

// typesReaching returns the event types of an inherited subscription that
// can reach a narrower level: of an org subscription seen on an agent, only
// the agent types it names. A subscription naming AllEvents keeps it. Empty
// when none can, and the subscription then has nothing to show there.
func typesReaching(eventTypes []string, level events.Scope) []string {
	if len(eventTypes) == 1 && eventTypes[0] == events.AllEvents {
		return eventTypes
	}
	out := make([]string, 0, len(eventTypes))
	for _, t := range eventTypes {
		if events.CanSubscribe(level, t) {
			out = append(out, t)
		}
	}
	return out
}
