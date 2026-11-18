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
export const ALL = "__all__";

/**
 * Filter value for "has none": no owner, no LLM provider, no MCP server.
 * Short, as it ends up in shared links.
 */
export const NONE = "none";
export const NO_OWNER = NONE;

/**
 * Which agents to list by registration. Shadow agents (unregistered) are left out
 * by default; both boxes unticked is not allowed.
 */
export type RegistrationFilter = "registered" | "unregistered" | "both";

/** The registry's filters. Each one is a single value; ALL, or the default, narrows nothing. */
export interface AgentRegistryFilters {
  search: string;
  /** Set in the page header. Also decides where the LLM provider and MCP server filters look. */
  environment: string;
  /** Set with the quick toggle. */
  lifecycleState: string;
  /** The rest are set in the Filters panel. */
  registration: RegistrationFilter;
  project: string;
  owner: string;
  tag: string;
  llmProvider: string;
  mcpServer: string;
}

/** The filters the Filters panel edits. */
export const PANEL_FILTER_KEYS = ["project", "owner", "tag", "llmProvider", "mcpServer"] as const;
export type PanelFilterKey = (typeof PANEL_FILTER_KEYS)[number];
/** What the Filters panel edits: the select filters and the registration checkboxes. */
export type PanelFilters = Pick<AgentRegistryFilters, PanelFilterKey | "registration">;

/** The panel's unapplied edits. Registration is null while both boxes are unticked. */
export type PanelDraft = Omit<PanelFilters, "registration"> & {
  registration: RegistrationFilter | null;
};

export const EMPTY_FILTERS: AgentRegistryFilters = {
  search: "",
  environment: ALL,
  lifecycleState: ALL,
  registration: "registered",
  project: ALL,
  owner: ALL,
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

/** Name shown on a panel filter's chip and field. */
export const PANEL_FILTER_NAMES: Record<PanelFilterKey, string> = {
  project: "Project",
  owner: "Owner",
  tag: "Tag",
  llmProvider: "LLM provider",
  mcpServer: "MCP server",
};

/** Whether the registration checkboxes are ticked, and back. */
export function registrationTicks(value: RegistrationFilter) {
  return {
    registered: value !== "unregistered",
    unregistered: value !== "registered",
  };
}

/** Null when both boxes are unticked, which would list nothing. */
export function registrationFromTicks(
  registered: boolean,
  unregistered: boolean,
): RegistrationFilter | null {
  if (registered && unregistered) return "both";
  if (registered) return "registered";
  if (unregistered) return "unregistered";
  return null;
}

/** True when a filter is at its default and so narrows nothing. */
function isDefault(key: keyof AgentRegistryFilters, value: string): boolean {
  return key === "search" ? value.trim() === "" : value === EMPTY_FILTERS[key];
}

// ---- URL ----------------------------------------------------------------

/** Query parameter for each filter. Filters at their default are left out of the URL. */
const URL_PARAMS: Record<keyof AgentRegistryFilters, string> = {
  search: "q",
  environment: "env",
  lifecycleState: "lifecycle",
  registration: "registration",
  project: "project",
  owner: "owner",
  tag: "tag",
  llmProvider: "llm",
  mcpServer: "mcp",
};

export function filtersFromSearchParams(params: URLSearchParams): AgentRegistryFilters {
  const filters = { ...EMPTY_FILTERS };
  for (const [key, param] of Object.entries(URL_PARAMS) as [keyof AgentRegistryFilters, string][]) {
    const value = params.get(param);
    if (!value) continue;
    if (key === "registration") {
      // Anything else in the link falls back to the default.
      if (value === "unregistered" || value === "both") filters.registration = value;
    } else {
      filters[key] = value;
    }
  }
  return filters;
}

/** Writes the filters into the URL, keeping any other query parameters (such as the view). */
export function filtersToSearchParams(
  filters: AgentRegistryFilters,
  current: URLSearchParams,
): URLSearchParams {
  const next = new URLSearchParams(current);
  for (const [key, param] of Object.entries(URL_PARAMS) as [keyof AgentRegistryFilters, string][]) {
    const value = filters[key];
    if (isDefault(key, value)) next.delete(param);
    else next.set(param, value);
  }
  return next;
}

// ---- Options ------------------------------------------------------------

/** One option of a filter dropdown. */
export interface FilterOption {
  value: string;
  label: string;
}

/** The options each filter offers, built from the agents in the registry. */
export interface AgentRegistryFilterOptions {
  environments: FilterOption[];
  lifecycleStates: FilterOption[];
  project: FilterOption[];
  owner: FilterOption[];
  tag: FilterOption[];
  llmProvider: FilterOption[];
  mcpServer: FilterOption[];
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
    // An unregistered agent has no project, owner or tags; it only adds to "No owner".
    if (agent.registration === "unregistered") continue;
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
    environments: sortedOptions(environments),
    lifecycleStates: Object.entries(LIFECYCLE_LABELS).map(([value, label]) => ({ value, label })),
    project: sortedOptions(projects),
    owner: ownerOptions,
    tag: sortedOptions(tags),
    // "None" goes first, as with owners: an agent with no LLM provider in Agent Manager
    // may be calling a model directly, outside the gateway.
    llmProvider: [{ value: NONE, label: "No LLM provider" }, ...sortedOptions(llmProviders)],
    mcpServer: [{ value: NONE, label: "No MCP server" }, ...sortedOptions(mcpServers)],
  };
}

// ---- Applied-filter chips -----------------------------------------------

const REGISTRATION_CHIP_VALUES: Record<RegistrationFilter, string> = {
  registered: "Registered",
  unregistered: "Unregistered only",
  both: "Registered and unregistered",
};

/** One applied panel filter, shown as a removable chip. */
export interface FilterChip {
  key: PanelFilterKey | "registration";
  name: string;
  value: string;
}

/**
 * A chip for each panel filter that is set, in panel order. Registration gets one
 * only when it differs from the default (registered agents only).
 */
export function filterChips(
  filters: AgentRegistryFilters,
  options: AgentRegistryFilterOptions,
): FilterChip[] {
  const chips: FilterChip[] = PANEL_FILTER_KEYS.filter((key) => filters[key] !== ALL).map(
    (key) => ({
      key,
      name: PANEL_FILTER_NAMES[key],
      // A value no agent has any more (from an old link) still shows, as its raw id.
      value: options[key].find((o) => o.value === filters[key])?.label ?? filters[key],
    }),
  );
  if (filters.registration !== EMPTY_FILTERS.registration) {
    chips.push({
      key: "registration",
      name: "Registration",
      value: REGISTRATION_CHIP_VALUES[filters.registration],
    });
  }
  return chips;
}

/** How many unregistered agents the registration filter is hiding. */
export function hiddenUnregisteredCount(
  agents: AgentRegistryEntry[],
  filters: AgentRegistryFilters,
): number {
  if (filters.registration !== "registered") return 0;
  return agents.filter((agent) => agent.registration === "unregistered").length;
}

// ---- Matching -----------------------------------------------------------

function matchesSearch(agent: AgentRegistryEntry, search: string): boolean {
  const term = search.trim().toLowerCase();
  if (!term) return true;
  const searchable = [
    agent.displayName,
    agent.name,
    agent.description ?? "",
    agent.projectDisplayName,
    agent.owner?.displayName ?? "",
    agent.discovery?.providerDisplayName ?? "",
    agent.discovery?.scope ?? "",
    agent.discovery?.ownerHint ?? "",
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
  if (llmProvider === NONE) {
    // What a shadow agent uses is not known, so it never counts as having none.
    return (
      agent.registration === "registered" &&
      environmentsInScope(agent, environment).every((env) => env.llmProviders.length === 0)
    );
  }
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
  if (mcpServer === NONE) {
    return (
      agent.registration === "registered" &&
      environmentsInScope(agent, environment).every((env) => env.mcpServers.length === 0)
    );
  }
  return environmentsInScope(agent, environment).some((env) =>
    env.mcpServers.some((s) => s.id === mcpServer),
  );
}

function matchesRegistration(agent: AgentRegistryEntry, registration: RegistrationFilter): boolean {
  return registration === "both" || agent.registration === registration;
}

/** Keeps the agents that match every filter. */
export function filterAgents(
  agents: AgentRegistryEntry[],
  filters: AgentRegistryFilters,
): AgentRegistryEntry[] {
  return agents.filter(
    (agent) =>
      matchesSearch(agent, filters.search) &&
      matchesRegistration(agent, filters.registration) &&
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
  return (Object.entries(filters) as [keyof AgentRegistryFilters, string][]).some(
    ([key, value]) => !isDefault(key, value),
  );
}
