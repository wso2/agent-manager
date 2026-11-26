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
	"slices"
	"sort"
	"sync/atomic"
)

// Type describes one event type subscriptions can name.
type Type struct {
	Name        string `json:"type"`
	Scope       Scope  `json:"scope"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

// Event types not tied to an API route.
const (
	// TypeDestinationTest is sent by the "test" action of a destination. It
	// is not in the catalog: subscriptions cannot name it, and AllEvents does
	// not match it.
	TypeDestinationTest = "destination.test"
)

// AllEvents, as a subscription's only event type, subscribes to every
// catalog type the subscription's scope can receive, including types added
// later.
const AllEvents = "*"

// routeEvent maps one API route to the event it emits on success.
type routeEvent struct {
	Type string
	// OmitData drops the response body: the route returns a credential
	// (an API key, a token, a client secret) that must never leave the
	// platform in an event.
	OmitData bool
}

// routeEvents is keyed by the registrar pattern. A route not listed here
// emits nothing. An agent event acting on an environment carries it (from
// the path or a handler annotation) and reaches the
// subscriptions whose environment filter selects it; one with no
// environment, such as agent.updated, reaches them whatever their filter.
var routeEvents = map[string]routeEvent{
	// --- Org ------------------------------------------------------------
	"POST /projects":                                     {Type: "project.created"},
	"DELETE /projects/{projName}":                        {Type: "project.deleted"},
	"POST /orgs/{orgName}/identities/users/invite":       {Type: "user.invited", OmitData: true},
	"POST /orgs/{orgName}/identities/users":              {Type: "user.created", OmitData: true},
	"DELETE /orgs/{orgName}/identities/users/{userID}":   {Type: "user.deleted"},
	"POST /orgs/{orgName}/identities/roles":              {Type: "role.created"},
	"PUT /orgs/{orgName}/identities/roles/{roleID}":      {Type: "role.updated"},
	"DELETE /orgs/{orgName}/identities/roles/{roleID}":   {Type: "role.deleted"},
	"POST /orgs/{orgName}/identities/groups":             {Type: "group.created"},
	"PUT /orgs/{orgName}/identities/groups/{groupID}":    {Type: "group.updated"},
	"DELETE /orgs/{orgName}/identities/groups/{groupID}": {Type: "group.deleted"},

	// --- Project --------------------------------------------------------
	"PUT /projects/{projName}":   {Type: "project.updated"},
	"POST /agents":               {Type: "agent.created"},
	"DELETE /agents/{agentName}": {Type: "agent.deleted"},

	// --- Agent ----------------------------------------------------------
	"PUT /agents/{agentName}": {Type: "agent.updated"},
}

// types is the catalog of every event subscriptions can name.
var types = []Type{
	// Org
	{"project.created", ScopeOrg, "Projects", "A project was created."},
	{"project.deleted", ScopeOrg, "Projects", "A project was deleted."},
	{"user.invited", ScopeOrg, "Members", "A user was invited to the organization."},
	{"user.created", ScopeOrg, "Members", "A user was added to the organization."},
	{"user.deleted", ScopeOrg, "Members", "A user was removed from the organization."},
	{"role.created", ScopeOrg, "Access control", "A role was created."},
	{"role.updated", ScopeOrg, "Access control", "A role was updated."},
	{"role.deleted", ScopeOrg, "Access control", "A role was deleted."},
	{"group.created", ScopeOrg, "Access control", "A group was created."},
	{"group.updated", ScopeOrg, "Access control", "A group was updated."},
	{"group.deleted", ScopeOrg, "Access control", "A group was deleted."},

	// Project
	{"project.updated", ScopeProject, "Project", "The project was updated."},
	{"agent.created", ScopeProject, "Agents", "An agent was created in the project."},
	{"agent.deleted", ScopeProject, "Agents", "An agent was deleted from the project."},

	// Agent
	{"agent.updated", ScopeAgent, "Agent", "The agent's details were updated. Sent to every environment."},
}

var typesByName = func() map[string]Type {
	m := make(map[string]Type, len(types))
	for _, t := range types {
		m[t.Name] = t
	}
	return m
}()

// moduleTypes are the webhook event types the active modules declare. The
// module host replaces them whenever the set of active modules changes.
var moduleTypes atomic.Pointer[map[string]Type]

// SetModuleTypes replaces the event types the active modules declare. A type
// that clashes with a core type is ignored.
func SetModuleTypes(ts []Type) {
	m := make(map[string]Type, len(ts))
	for _, t := range ts {
		if _, core := typesByName[t.Name]; !core {
			m[t.Name] = t
		}
	}
	moduleTypes.Store(&m)
}

// all is the core catalog and the active modules' types.
func all() []Type {
	m := moduleTypes.Load()
	if m == nil || len(*m) == 0 {
		return types
	}
	out := slices.Clone(types)
	for _, t := range *m {
		out = append(out, t)
	}
	return out
}

// Types returns the catalog for a scope, or every scope when scope is empty,
// ordered by category then name.
func Types(scope Scope) []Type {
	cat := all()
	out := make([]Type, 0, len(cat))
	for _, t := range cat {
		if scope == "" || t.Scope == scope {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Scope != out[j].Scope {
			return out[i].Scope < out[j].Scope
		}
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Lookup returns the catalog entry for an event type.
func Lookup(name string) (Type, bool) {
	if t, ok := typesByName[name]; ok {
		return t, true
	}
	if m := moduleTypes.Load(); m != nil {
		t, ok := (*m)[name]
		return t, ok
	}
	return Type{}, false
}

// rank orders scopes from the widest to the narrowest: an org holds
// projects, a project holds agents. An unknown scope ranks -1.
func (s Scope) rank() int {
	switch s {
	case ScopeOrg:
		return 0
	case ScopeProject:
		return 1
	case ScopeAgent:
		return 2
	default:
		return -1
	}
}

// Subscribable lists the catalog types a subscription at scope can name:
// those of its own scope and of every narrower one. An org subscription can
// name any type, a project subscription project and agent types, an agent
// subscription agent types only. Ordered like Types.
func Subscribable(scope Scope) []Type {
	if !scope.Valid() {
		return []Type{}
	}
	out := []Type{}
	for _, t := range Types("") {
		if t.Scope.rank() >= scope.rank() {
			out = append(out, t)
		}
	}
	return out
}

// CanSubscribe reports whether a subscription at scope can name the event
// type name. Types outside the catalog, such as TypeDestinationTest, never
// qualify.
func CanSubscribe(scope Scope, name string) bool {
	t, ok := Lookup(name)
	return ok && scope.Valid() && t.Scope.rank() >= scope.rank()
}

// Target is where a subscription sits: an org, a project of it, or an agent
// of a project.
type Target struct {
	Scope   Scope
	Project string
	Agent   string
}

// Reaches reports whether e can reach a subscription at t, by scope and
// location alone; the subscription's event types and environment filter are
// checked separately. An event reaches subscriptions at its own scope and the
// wider ones that hold it: an agent event reaches its agent's, project's and
// org's subscriptions. Never a narrower one, even when the event names a
// project or agent: project.deleted is an org event that names the project,
// so it does not reach the project's subscriptions.
func Reaches(t Target, e Event) bool {
	if !t.Scope.Valid() || !e.Scope.Valid() || e.Scope.rank() < t.Scope.rank() {
		return false
	}
	switch t.Scope {
	case ScopeProject:
		return e.Project != "" && e.Project == t.Project
	case ScopeAgent:
		return e.Project != "" && e.Agent != "" && e.Project == t.Project && e.Agent == t.Agent
	default:
		return true
	}
}

// Wants reports whether a subscription at scope naming eventTypes wants the
// event type name: it names it, or names only AllEvents, and its scope can
// receive it. A type named before it left the catalog, or before it moved to
// a wider scope, is ignored rather than delivered.
func Wants(scope Scope, eventTypes []string, name string) bool {
	if !CanSubscribe(scope, name) {
		return false
	}
	if len(eventTypes) == 1 && eventTypes[0] == AllEvents {
		return true
	}
	return slices.Contains(eventTypes, name)
}

// RouteEventType returns the event a route emits, if any.
func RouteEventType(pattern string) (name string, omitData bool, ok bool) {
	re, ok := routeEvents[pattern]
	return re.Type, re.OmitData, ok
}

// RoutePatterns lists every route that emits an event, for tests.
func RoutePatterns() []string {
	out := make([]string, 0, len(routeEvents))
	for p := range routeEvents {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
