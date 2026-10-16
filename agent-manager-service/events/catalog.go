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

import "sort"

// Type describes one event type endpoints can subscribe to.
type Type struct {
	Name        string `json:"type"`
	Scope       Scope  `json:"scope"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

// Event types not tied to an API route.
const (
	TypeMonitorRunSucceeded = "monitor_run.succeeded"
	TypeMonitorRunFailed    = "monitor_run.failed"
	TypeWebhookTest         = "webhook.test"
)

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
// the path, query, response or a handler annotation) and reaches the agent
// webhooks that selected that environment; one with no environment, such as
// a build, reaches all of the agent's webhooks.
var routeEvents = map[string]routeEvent{
	// --- Org ------------------------------------------------------------
	"POST /orgs/{orgName}/projects":                                        {Type: "project.created"},
	"DELETE /orgs/{orgName}/projects/{projName}":                           {Type: "project.deleted"},
	"POST /orgs/{orgName}/mcp-proxies":                                     {Type: "mcp_server.created"},
	"PUT /orgs/{orgName}/mcp-proxies/{proxyId}":                            {Type: "mcp_server.updated"},
	"DELETE /orgs/{orgName}/mcp-proxies/{proxyId}":                         {Type: "mcp_server.deleted"},
	"POST /orgs/{orgName}/llm-providers":                                   {Type: "llm_provider.created"},
	"PUT /orgs/{orgName}/llm-providers/{providerId}":                       {Type: "llm_provider.updated"},
	"DELETE /orgs/{orgName}/llm-providers/{providerId}":                    {Type: "llm_provider.deleted"},
	"POST /orgs/{orgName}/llm-providers/{providerId}/deployments":          {Type: "llm_provider.deployed"},
	"POST /orgs/{orgName}/llm-providers/{providerId}/deployments/undeploy": {Type: "llm_provider.undeployed"},
	"POST /orgs/{orgName}/environments":                                    {Type: "environment.created"},
	"PUT /orgs/{orgName}/environments/{envID}":                             {Type: "environment.updated"},
	"DELETE /orgs/{orgName}/environments/{envID}":                          {Type: "environment.deleted"},
	"POST /orgs/{orgName}/gateways":                                        {Type: "gateway.created", OmitData: true},
	"PUT /orgs/{orgName}/gateways/{gatewayID}":                             {Type: "gateway.updated"},
	"DELETE /orgs/{orgName}/gateways/{gatewayID}":                          {Type: "gateway.deleted"},
	"POST /orgs/{orgName}/deployment-pipelines":                            {Type: "deployment_pipeline.created"},
	"PUT /orgs/{orgName}/deployment-pipelines/{pipelineName}":              {Type: "deployment_pipeline.updated"},
	"DELETE /orgs/{orgName}/deployment-pipelines/{pipelineName}":           {Type: "deployment_pipeline.deleted"},
	"POST /orgs/{orgName}/evaluators/custom":                               {Type: "evaluator.created"},
	"PUT /orgs/{orgName}/evaluators/custom/{identifier}":                   {Type: "evaluator.updated"},
	"DELETE /orgs/{orgName}/evaluators/custom/{identifier}":                {Type: "evaluator.deleted"},
	"POST /orgs/{orgName}/agent-kinds/{kindName}/versions":                 {Type: "agent_kind.version_added"},
	"PUT /orgs/{orgName}/agent-kinds/{kindName}":                           {Type: "agent_kind.updated"},
	"DELETE /orgs/{orgName}/agent-kinds/{kindName}":                        {Type: "agent_kind.deleted"},
	"POST /orgs/{orgName}/git-secrets":                                     {Type: "git_secret.created", OmitData: true},
	"DELETE /orgs/{orgName}/git-secrets/{secretName}":                      {Type: "git_secret.deleted"},
	"POST /orgs/{orgName}/identities/users/invite":                         {Type: "user.invited", OmitData: true},
	"POST /orgs/{orgName}/identities/users":                                {Type: "user.created", OmitData: true},
	"DELETE /orgs/{orgName}/identities/users/{userID}":                     {Type: "user.deleted"},
	"POST /orgs/{orgName}/identities/roles":                                {Type: "role.created"},
	"PUT /orgs/{orgName}/identities/roles/{roleID}":                        {Type: "role.updated"},
	"DELETE /orgs/{orgName}/identities/roles/{roleID}":                     {Type: "role.deleted"},
	"POST /orgs/{orgName}/identities/groups":                               {Type: "group.created"},
	"PUT /orgs/{orgName}/identities/groups/{groupID}":                      {Type: "group.updated"},
	"DELETE /orgs/{orgName}/identities/groups/{groupID}":                   {Type: "group.deleted"},

	// --- Project --------------------------------------------------------
	"PUT /orgs/{orgName}/projects/{projName}":                                        {Type: "project.updated"},
	"POST /orgs/{orgName}/projects/{projName}/agents":                                {Type: "agent.created"},
	"DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}":                  {Type: "agent.deleted"},
	"POST /orgs/{orgName}/projects/{projName}/llm-proxies":                           {Type: "llm_proxy.created"},
	"PUT /orgs/{orgName}/projects/{projName}/llm-proxies/{proxyId}":                  {Type: "llm_proxy.updated"},
	"DELETE /orgs/{orgName}/projects/{projName}/llm-proxies/{proxyId}":               {Type: "llm_proxy.deleted"},
	"POST /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/deployments":          {Type: "llm_proxy.deployed"},
	"POST /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/deployments/undeploy": {Type: "llm_proxy.undeployed"},

	// --- Agent ----------------------------------------------------------
	"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}":                                            {Type: "agent.updated"},
	"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/build-parameters":                           {Type: "agent.build_parameters_updated"},
	"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/configurations":                             {Type: "agent.configuration_updated"},
	"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/deploy-settings":                            {Type: "agent.configuration_updated"},
	"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/resource-configs":                           {Type: "agent.configuration_updated"},
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/builds":                                    {Type: "agent.build_triggered"},
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments":                               {Type: "agent.deployed"},
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/promote":                                   {Type: "agent.promoted"},
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments/state":                         {Type: "agent.deployment_state_changed"},
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/publish-kind":                              {Type: "agent.kind_published"},
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys":             {Type: "agent.api_key_created", OmitData: true},
	"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys/{keyName}":    {Type: "agent.api_key_rotated", OmitData: true},
	"DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys/{keyName}": {Type: "agent.api_key_revoked"},
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors":                                  {Type: "monitor.created"},
	"PATCH /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}":                   {Type: "monitor.updated"},
	"DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}":                  {Type: "monitor.deleted"},
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/start":              {Type: "monitor.started"},
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/stop":               {Type: "monitor.stopped"},
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/runs/{runId}/rerun": {Type: "monitor.rerun_requested"},
}

// types is the catalog of every event endpoints can subscribe to.
var types = []Type{
	// Org
	{"project.created", ScopeOrg, "Projects", "A project was created."},
	{"project.deleted", ScopeOrg, "Projects", "A project was deleted."},
	{"mcp_server.created", ScopeOrg, "MCP servers", "An MCP server was added."},
	{"mcp_server.updated", ScopeOrg, "MCP servers", "An MCP server was updated."},
	{"mcp_server.deleted", ScopeOrg, "MCP servers", "An MCP server was deleted."},
	{"llm_provider.created", ScopeOrg, "LLM providers", "An LLM provider was added."},
	{"llm_provider.updated", ScopeOrg, "LLM providers", "An LLM provider was updated."},
	{"llm_provider.deleted", ScopeOrg, "LLM providers", "An LLM provider was deleted."},
	{"llm_provider.deployed", ScopeOrg, "LLM providers", "An LLM provider was deployed to a gateway."},
	{"llm_provider.undeployed", ScopeOrg, "LLM providers", "An LLM provider was undeployed from a gateway."},
	{"environment.created", ScopeOrg, "Environments", "An environment was created."},
	{"environment.updated", ScopeOrg, "Environments", "An environment was updated."},
	{"environment.deleted", ScopeOrg, "Environments", "An environment was deleted."},
	{"gateway.created", ScopeOrg, "Gateways", "A gateway was registered."},
	{"gateway.updated", ScopeOrg, "Gateways", "A gateway was updated."},
	{"gateway.deleted", ScopeOrg, "Gateways", "A gateway was deleted."},
	{"deployment_pipeline.created", ScopeOrg, "Deployment pipelines", "A deployment pipeline was created."},
	{"deployment_pipeline.updated", ScopeOrg, "Deployment pipelines", "A deployment pipeline was updated."},
	{"deployment_pipeline.deleted", ScopeOrg, "Deployment pipelines", "A deployment pipeline was deleted."},
	{"evaluator.created", ScopeOrg, "Evaluators", "A custom evaluator was created."},
	{"evaluator.updated", ScopeOrg, "Evaluators", "A custom evaluator was updated."},
	{"evaluator.deleted", ScopeOrg, "Evaluators", "A custom evaluator was deleted."},
	{"agent_kind.version_added", ScopeOrg, "Agent kinds", "A version was added to an agent kind."},
	{"agent_kind.updated", ScopeOrg, "Agent kinds", "An agent kind was updated."},
	{"agent_kind.deleted", ScopeOrg, "Agent kinds", "An agent kind was deleted."},
	{"git_secret.created", ScopeOrg, "Git secrets", "A git secret was created."},
	{"git_secret.deleted", ScopeOrg, "Git secrets", "A git secret was deleted."},
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
	{"llm_proxy.created", ScopeProject, "LLM proxies", "An LLM proxy was created."},
	{"llm_proxy.updated", ScopeProject, "LLM proxies", "An LLM proxy was updated."},
	{"llm_proxy.deleted", ScopeProject, "LLM proxies", "An LLM proxy was deleted."},
	{"llm_proxy.deployed", ScopeProject, "LLM proxies", "An LLM proxy was deployed."},
	{"llm_proxy.undeployed", ScopeProject, "LLM proxies", "An LLM proxy was undeployed."},

	// Agent
	{"agent.updated", ScopeAgent, "Agent", "The agent's details were updated. Sent to every environment."},
	{"agent.build_parameters_updated", ScopeAgent, "Agent", "The agent's build parameters changed. Sent to every environment."},
	{"agent.kind_published", ScopeAgent, "Agent", "The agent was published as an agent kind. Sent to every environment."},
	{"agent.build_triggered", ScopeAgent, "Builds and deployments", "A build was started. Builds are not tied to an environment, so this is sent to every environment."},
	{"agent.configuration_updated", ScopeAgent, "Configuration", "The agent's environment variables, deploy settings or resources changed in an environment."},
	{"agent.deployed", ScopeAgent, "Builds and deployments", "The agent was deployed to an environment."},
	{"agent.promoted", ScopeAgent, "Builds and deployments", "The agent was promoted into an environment."},
	{"agent.deployment_state_changed", ScopeAgent, "Builds and deployments", "A deployment was started, stopped or suspended."},
	{"agent.api_key_created", ScopeAgent, "API keys", "An API key was created for the agent."},
	{"agent.api_key_rotated", ScopeAgent, "API keys", "An API key was rotated."},
	{"agent.api_key_revoked", ScopeAgent, "API keys", "An API key was revoked."},
	{"monitor.created", ScopeAgent, "Monitors", "A monitor was created."},
	{"monitor.updated", ScopeAgent, "Monitors", "A monitor was updated."},
	{"monitor.deleted", ScopeAgent, "Monitors", "A monitor was deleted."},
	{"monitor.started", ScopeAgent, "Monitors", "A monitor was started."},
	{"monitor.stopped", ScopeAgent, "Monitors", "A monitor was stopped."},
	{"monitor.rerun_requested", ScopeAgent, "Monitors", "A monitor run was re-run."},
	{TypeMonitorRunSucceeded, ScopeAgent, "Monitor runs", "A monitor run finished, with its scores."},
	{TypeMonitorRunFailed, ScopeAgent, "Monitor runs", "A monitor run failed or could not start."},
}

var typesByName = func() map[string]Type {
	m := make(map[string]Type, len(types))
	for _, t := range types {
		m[t.Name] = t
	}
	return m
}()

// Types returns the catalog for a scope, or every scope when scope is empty,
// ordered by category then name.
func Types(scope Scope) []Type {
	out := make([]Type, 0, len(types))
	for _, t := range types {
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
	t, ok := typesByName[name]
	return t, ok
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
