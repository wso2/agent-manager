/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type {
  AgentRegistryEntry,
  AgentRegistryEnvironment,
  AgentRegistryLifecycleState,
} from "@agent-management-platform/types";

/** Value of a filter that is not narrowing anything. */
export const ALL = "";

/** Owner filter value for agents that have no owner. */
export const NO_OWNER = "__no-owner__";

/** The registry's filters. Each one is a single value, or ALL. */
export interface AgentRegistryFilters {
  search: string;
  project: string;
  environment: string;
  owner: string;
  lifecycleState: string;
  tag: string;
  llmProvider: string;
  mcpServer: string;
}

export const EMPTY_FILTERS: AgentRegistryFilters = {
  search: "",
  project: ALL,
  environment: ALL,
  owner: ALL,
  lifecycleState: ALL,
  tag: ALL,
  llmProvider: ALL,
  mcpServer: ALL,
};

export const LIFECYCLE_LABELS: Record<AgentRegistryLifecycleState, string> = {
  draft: "Draft",
  active: "Active",
  deprecated: "Deprecated",
  retired: "Retired",
};

/** One option of a filter dropdown. */
export interface FilterOption {
  value: string;
  label: string;
}

/** The options each filter dropdown offers, built from the agents in the registry. */
export interface AgentRegistryFilterOptions {
  projects: FilterOption[];
  environments: FilterOption[];
  owners: FilterOption[];
  lifecycleStates: FilterOption[];
  tags: FilterOption[];
  llmProviders: FilterOption[];
  mcpServers: FilterOption[];
}

/** Labels are shown and matched as `key:value`. */
export function tagText(key: string, value: string): string {
  return `${key}:${value}`;
}

function sortedOptions(byValue: Map<string, string>): FilterOption[] {
  return [...byValue.entries()]
    .map(([value, label]) => ({ value, label }))
    .sort((a, b) => a.label.localeCompare(b.label));
}

export function buildFilterOptions(agents: AgentRegistryEntry[]): AgentRegistryFilterOptions {
  const projects = new Map<string, string>();
  const environments = new Map<string, string>();
  const owners = new Map<string, string>();
  const tags = new Map<string, string>();
  const llmProviders = new Map<string, string>();
  const mcpServers = new Map<string, string>();
  let hasAgentWithoutOwner = false;

  for (const agent of agents) {
    projects.set(agent.projectName, agent.projectDisplayName);
    if (agent.owner) {
      owners.set(agent.owner.id, agent.owner.displayName);
    } else {
      hasAgentWithoutOwner = true;
    }
    for (const [key, value] of Object.entries(agent.labels)) {
      tags.set(tagText(key, value), tagText(key, value));
    }
    for (const env of agent.environments) {
      environments.set(env.name, env.displayName);
      env.llmProviders.forEach((p) => llmProviders.set(p.id, p.displayName));
      env.mcpServers.forEach((s) => mcpServers.set(s.id, s.displayName));
    }
  }

  // "No owner" goes first: finding unowned agents is a main reason to open the registry.
  const ownerOptions = sortedOptions(owners);
  if (hasAgentWithoutOwner) {
    ownerOptions.unshift({ value: NO_OWNER, label: "No owner" });
  }

  return {
    projects: sortedOptions(projects),
    environments: sortedOptions(environments),
    owners: ownerOptions,
    lifecycleStates: Object.entries(LIFECYCLE_LABELS).map(([value, label]) => ({ value, label })),
    tags: sortedOptions(tags),
    llmProviders: sortedOptions(llmProviders),
    mcpServers: sortedOptions(mcpServers),
  };
}

function matchesSearch(agent: AgentRegistryEntry, search: string): boolean {
  const term = search.trim().toLowerCase();
  if (!term) return true;
  const searchable = [
    agent.displayName,
    agent.name,
    agent.description ?? "",
    agent.projectDisplayName,
    agent.owner?.displayName ?? "",
    ...Object.entries(agent.labels).map(([k, v]) => tagText(k, v)),
  ];
  return searchable.some((text) => text.toLowerCase().includes(term));
}

function matchesOwner(agent: AgentRegistryEntry, owner: string): boolean {
  if (owner === ALL) return true;
  if (owner === NO_OWNER) return agent.owner === null;
  return agent.owner?.id === owner;
}

function matchesTag(agent: AgentRegistryEntry, tag: string): boolean {
  if (tag === ALL) return true;
  return Object.entries(agent.labels).some(([k, v]) => tagText(k, v) === tag);
}

/**
 * The environments the LLM provider and MCP server filters look at: the chosen
 * environment only, or every environment when none is chosen.
 */
function environmentsInScope(
  agent: AgentRegistryEntry,
  environment: string,
): AgentRegistryEnvironment[] {
  if (environment === ALL) return agent.environments;
  return agent.environments.filter((env) => env.name === environment);
}

/** An agent is "in" an environment when it has been deployed there at least once. */
function isInEnvironment(agent: AgentRegistryEntry, environment: string): boolean {
  if (environment === ALL) return true;
  return agent.environments.some(
    (env) => env.name === environment && env.deploymentStatus !== "not-deployed",
  );
}

function canReachLlmProvider(
  agent: AgentRegistryEntry,
  environment: string,
  llmProvider: string,
): boolean {
  if (llmProvider === ALL) return true;
  return environmentsInScope(agent, environment).some((env) =>
    env.llmProviders.some((p) => p.id === llmProvider),
  );
}

function canReachMcpServer(
  agent: AgentRegistryEntry,
  environment: string,
  mcpServer: string,
): boolean {
  if (mcpServer === ALL) return true;
  return environmentsInScope(agent, environment).some((env) =>
    env.mcpServers.some((s) => s.id === mcpServer),
  );
}

/** Keeps the agents that match every filter. */
export function filterAgents(
  agents: AgentRegistryEntry[],
  filters: AgentRegistryFilters,
): AgentRegistryEntry[] {
  return agents.filter(
    (agent) =>
      matchesSearch(agent, filters.search) &&
      (filters.project === ALL || agent.projectName === filters.project) &&
      isInEnvironment(agent, filters.environment) &&
      matchesOwner(agent, filters.owner) &&
      (filters.lifecycleState === ALL || agent.lifecycleState === filters.lifecycleState) &&
      matchesTag(agent, filters.tag) &&
      canReachLlmProvider(agent, filters.environment, filters.llmProvider) &&
      canReachMcpServer(agent, filters.environment, filters.mcpServer),
  );
}

/** True when any filter or the search is narrowing the list. */
export function isFiltered(filters: AgentRegistryFilters): boolean {
  return Object.entries(filters).some(([key, value]) =>
    key === "search" ? value.trim() !== "" : value !== ALL,
  );
}
