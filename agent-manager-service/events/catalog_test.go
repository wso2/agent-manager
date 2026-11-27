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

package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSubscribableWidensWithTheScope(t *testing.T) {
	scopesOf := func(types []Type) map[Scope]bool {
		out := map[Scope]bool{}
		for _, ty := range types {
			out[ty.Scope] = true
		}
		return out
	}
	assert.Equal(t, map[Scope]bool{ScopeAgent: true}, scopesOf(Subscribable(ScopeAgent)))
	assert.Equal(t, map[Scope]bool{ScopeProject: true, ScopeAgent: true}, scopesOf(Subscribable(ScopeProject)))
	assert.Len(t, Subscribable(ScopeOrg), len(Types("")), "an org subscription can name every type")
	assert.Empty(t, Subscribable("galaxy"))
}

func TestCanSubscribe(t *testing.T) {
	assert.True(t, CanSubscribe(ScopeOrg, "agent.updated"))
	assert.True(t, CanSubscribe(ScopeProject, "agent.updated"))
	assert.True(t, CanSubscribe(ScopeAgent, "agent.updated"))
	assert.True(t, CanSubscribe(ScopeProject, "agent.created"))
	assert.False(t, CanSubscribe(ScopeAgent, "agent.created"), "a project event")
	assert.False(t, CanSubscribe(ScopeProject, "project.deleted"), "an org event")
	assert.False(t, CanSubscribe(ScopeOrg, TypeDestinationTest), "the test type is not in the catalog")
	assert.False(t, CanSubscribe(ScopeOrg, AllEvents), "AllEvents is not a type")
	assert.False(t, CanSubscribe(ScopeOrg, "agent.teleported"))
}

func TestReaches(t *testing.T) {
	at := func(name string, scope Scope, project, agent string) Event {
		return New("evt_1", name, scope, "ou-1", project, agent, time.Now())
	}
	orgEvent := at("project.deleted", ScopeOrg, "payments", "")
	projectEvent := at("agent.created", ScopeProject, "payments", "refund-agent")
	agentEvent := at("agent.updated", ScopeAgent, "payments", "refund-agent")

	org := Target{Scope: ScopeOrg}
	project := Target{Scope: ScopeProject, Project: "payments"}
	otherProject := Target{Scope: ScopeProject, Project: "claims"}
	agent := Target{Scope: ScopeAgent, Project: "payments", Agent: "refund-agent"}
	otherAgent := Target{Scope: ScopeAgent, Project: "payments", Agent: "support-agent"}

	cases := []struct {
		name   string
		target Target
		event  Event
		want   bool
	}{
		{"org gets org events", org, orgEvent, true},
		{"org gets project events", org, projectEvent, true},
		{"org gets agent events", org, agentEvent, true},
		{"project skips the org event naming it", project, orgEvent, false},
		{"project gets its events", project, projectEvent, true},
		{"project gets its agents' events", project, agentEvent, true},
		{"another project gets nothing", otherProject, agentEvent, false},
		{"agent skips the project event naming it", agent, projectEvent, false},
		{"agent gets its events", agent, agentEvent, true},
		{"another agent gets nothing", otherAgent, agentEvent, false},
		{"an unknown scope gets nothing", Target{Scope: "galaxy"}, agentEvent, false},
		{"an event without a scope reaches nobody", org, Event{Type: TypePrefix + "agent.updated"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Reaches(tc.target, tc.event))
		})
	}
}

func TestWants(t *testing.T) {
	const configured = "testmod.agent.configured"
	SetModuleTypes([]Type{{configured, ScopeAgent, "Test", "A module's agent event."}})
	t.Cleanup(func() { SetModuleTypes(nil) })

	assert.True(t, Wants(ScopeAgent, []string{configured}, configured))
	assert.False(t, Wants(ScopeAgent, []string{"agent.updated"}, configured))
	assert.True(t, Wants(ScopeAgent, []string{AllEvents}, configured))
	assert.True(t, Wants(ScopeAgent, []string{AllEvents}, "agent.updated"))
	assert.False(t, Wants(ScopeAgent, []string{AllEvents}, "agent.created"), "outside the agent's catalog")
	assert.False(t, Wants(ScopeOrg, []string{AllEvents}, TypeDestinationTest))
	assert.False(t, Wants(ScopeOrg, []string{AllEvents, configured}, "project.created"),
		"AllEvents only counts as the only entry")
	assert.False(t, Wants(ScopeProject, []string{"project.deleted"}, "project.deleted"),
		"a type that moved to a wider scope is ignored")
}

func TestLookupModuleAndNarrowerThan(t *testing.T) {
	SetModuleTypes([]Type{{Name: "testmod.thing_done", Scope: ScopeAgent}})
	t.Cleanup(func() { SetModuleTypes(nil) })
	if _, ok := LookupModule("testmod.thing_done"); !ok {
		t.Error("a module type is found")
	}
	if _, ok := LookupModule("agent.created"); ok {
		t.Error("a core type is not a module type")
	}
	if !ScopeAgent.NarrowerThan(ScopeOrg) || !ScopeProject.NarrowerThan(ScopeOrg) || ScopeOrg.NarrowerThan(ScopeAgent) || ScopeAgent.NarrowerThan(ScopeAgent) {
		t.Error("scopes narrow from org to project to agent")
	}
}
