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
  AgentRegistryDeploymentStatus,
  AgentRegistryEntry,
  AgentRegistryEnvironment,
  AgentRegistryInterface,
  AgentRegistryOwnerHint,
  AgentRegistryRegistration,
} from "@agent-management-platform/types";

/** Value of a filter that is not narrowing anything. */
export const ALL = "__all__";

/**
 * Filter value for "has none": owner unknown, no LLM provider, no MCP server.
 * Short, as it ends up in shared links.
 */
export const NONE = "none";

/**
 * The registry's filters. Each one is a single value; ALL (or, for registration,
 * "registered") narrows nothing.
 */
export interface AgentRegistryFilters {
  search: string;
  /** The toggle at the top: registered agents, or shadow agents found outside. */
  registration: AgentRegistryRegistration;
  // Filters panel, registered agents.
  hosting: string;
  interface: string;
  project: string;
  owner: string;
  tag: string;
  /** Narrows to agents deployed there, and is where status, LLM and MCP look. */
  environment: string;
  status: string;
  llmProvider: string;
  mcpServer: string;
  // Filters panel, unregistered agents.
  foundOn: string;
}

export type PanelFilterKey = Exclude<keyof AgentRegistryFilters, "search" | "registration">;

/** The panel filters each registration shows. Unregistered: only where it was found. */
export const PANEL_FILTER_KEYS: Record<AgentRegistryRegistration, PanelFilterKey[]> = {
  registered: [
    "hosting",
    "interface",
    "project",
    "owner",
    "tag",
    "environment",
    "status",
    "llmProvider",
    "mcpServer",
  ],
  unregistered: ["foundOn"],
};

export type PanelFilters = Pick<AgentRegistryFilters, PanelFilterKey>;

export const EMPTY_FILTERS: AgentRegistryFilters = {
  search: "",
  registration: "registered",
  hosting: ALL,
  interface: ALL,
  project: ALL,
  owner: ALL,
  tag: ALL,
  environment: ALL,
  status: ALL,
  llmProvider: ALL,
  mcpServer: ALL,
  foundOn: ALL,
};

/** Name shown on a panel filter's field and chip. */
export const PANEL_FILTER_NAMES: Record<PanelFilterKey, string> = {
  hosting: "Hosting",
  interface: "Type",
  project: "Project",
  owner: "Owner",
  tag: "Tag",
  environment: "Environment",
  status: "Status",
  llmProvider: "LLM provider",
  mcpServer: "MCP server",
  foundOn: "Platform",
};

/** The option that filters nothing. */
export const ALL_LABELS: Record<PanelFilterKey, string> = {
  hosting: "All hosting",
  interface: "All types",
  project: "All projects",
  owner: "All owners",
  tag: "All tags",
  environment: "All environments",
  status: "All statuses",
  llmProvider: "All LLM providers",
  mcpServer: "All MCP servers",
  foundOn: "All platforms",
};

/**
 * The project pages' labels for each deployment status (EnvironmentCard), so a status
 * reads the same in the registry; "Not deployed" in sentence case, as the UX guidelines ask.
 */
export const DEPLOYMENT_STATUS_LABELS: Record<AgentRegistryDeploymentStatus, string> = {
  active: "Deployed",
  suspended: "Suspended",
  failed: "Error",
  "in-progress": "Deploying",
  "not-deployed": "Not deployed",
};

const HOSTING_LABELS = { platform: "Platform", external: "External" };

const INTERFACE_LABELS: Record<AgentRegistryInterface, string> = {
  "chat-api": "Chat",
  "custom-api": "Custom API",
  "a2a-agent": "A2A",
};

/**
 * Whether the registry lists the agent at all. Every registered agent is listed. A shadow
 * agent is not once its platform stops listing it (gone), or once it is onboarded: it is
 * then a registered external agent and would otherwise appear twice.
 */
export function isListed(agent: AgentRegistryEntry): boolean {
  if (agent.registration === "registered") return true;
  return !!agent.discovery && agent.discovery.present && !agent.discovery.onboarded;
}

/** A shadow agent's creator as one string: the name, else the email; undefined when neither. */
export function ownerHintText(hint: AgentRegistryOwnerHint | undefined): string | undefined {
  return hint?.name || hint?.email || undefined;
}

/** True when a filter is at its default and so narrows nothing. */
function isDefault(key: keyof AgentRegistryFilters, value: string): boolean {
  return key === "search" ? value.trim() === "" : value === EMPTY_FILTERS[key];
}

/** The filters with every panel filter of the other registration set back to ALL. */
export function withRegistration(
  filters: AgentRegistryFilters,
  registration: AgentRegistryRegistration,
): AgentRegistryFilters {
  const next = { ...filters, registration };
  for (const key of Object.keys(PANEL_FILTER_NAMES) as PanelFilterKey[]) {
    if (!PANEL_FILTER_KEYS[registration].includes(key)) next[key] = ALL;
  }
  return next;
}

// ---- URL ----------------------------------------------------------------

/** Query parameter for each filter. Filters at their default are left out of the URL. */
const URL_PARAMS: Record<keyof AgentRegistryFilters, string> = {
  search: "q",
  registration: "registration",
  hosting: "hosting",
  interface: "type",
  project: "project",
  owner: "owner",
  tag: "tag",
  environment: "env",
  status: "status",
  llmProvider: "llm",
  mcpServer: "mcp",
  foundOn: "foundOn",
};

export function filtersFromSearchParams(params: URLSearchParams): AgentRegistryFilters {
  const filters = { ...EMPTY_FILTERS };
  for (const [key, param] of Object.entries(URL_PARAMS) as [keyof AgentRegistryFilters, string][]) {
    const value = params.get(param);
    if (!value) continue;
    if (key === "registration") {
      // Anything else in the link falls back to the default.
      if (value === "unregistered") filters.registration = value;
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

export type AgentRegistryFilterOptions = Record<PanelFilterKey, FilterOption[]>;

/** Labels are shown and matched as `key:value`. */
export function tagText(key: string, value: string): string {
  return `${key}:${value}`;
}

function sortedOptions(byValue: Map<string, string>): FilterOption[] {
  return [...byValue.entries()]
    .map(([value, label]) => ({ value, label }))
    .sort((a, b) => a.label.localeCompare(b.label));
}

function fixedOptions(labels: Record<string, string>): FilterOption[] {
  return Object.entries(labels).map(([value, label]) => ({ value, label }));
}

export function buildFilterOptions(agents: AgentRegistryEntry[]): AgentRegistryFilterOptions {
  const projects = new Map<string, string>();
  const owners = new Map<string, string>();
  const tags = new Map<string, string>();
  const environments = new Map<string, string>();
  const llmProviders = new Map<string, string>();
  const mcpServers = new Map<string, string>();
  const platforms = new Map<string, string>();
  let hasUnknownOwner = false;

  for (const agent of agents) {
    if (!isListed(agent)) continue;
    if (agent.registration === "unregistered") {
      if (agent.discovery) {
        platforms.set(agent.discovery.provider, agent.discovery.providerDisplayName);
      }
      continue;
    }
    projects.set(agent.projectName, agent.projectDisplayName);
    if (agent.createdBy) {
      owners.set(agent.createdBy.id, agent.createdBy.displayName);
    } else {
      hasUnknownOwner = true;
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

  // "None" options go first: finding gaps is a main reason to open the registry.
  const ownerOptions = sortedOptions(owners);
  if (hasUnknownOwner) {
    ownerOptions.unshift({ value: NONE, label: "Unknown" });
  }

  return {
    hosting: fixedOptions(HOSTING_LABELS),
    interface: fixedOptions(INTERFACE_LABELS),
    project: sortedOptions(projects),
    owner: ownerOptions,
    tag: sortedOptions(tags),
    environment: sortedOptions(environments),
    status: fixedOptions(DEPLOYMENT_STATUS_LABELS),
    // An agent with no LLM provider in Agent Manager may be calling a model directly.
    llmProvider: [{ value: NONE, label: "No LLM provider" }, ...sortedOptions(llmProviders)],
    mcpServer: [{ value: NONE, label: "No MCP server" }, ...sortedOptions(mcpServers)],
    foundOn: sortedOptions(platforms),
  };
}

// ---- Applied-filter chips -----------------------------------------------

/** One applied panel filter, shown as a removable chip. */
export interface FilterChip {
  key: PanelFilterKey;
  name: string;
  value: string;
}

/** A chip for each panel filter of the current registration that is set, in panel order. */
export function filterChips(
  filters: AgentRegistryFilters,
  options: AgentRegistryFilterOptions,
): FilterChip[] {
  return PANEL_FILTER_KEYS[filters.registration]
    .filter((key) => filters[key] !== ALL)
    .map((key) => ({
      key,
      name: PANEL_FILTER_NAMES[key],
      // A value no agent has any more (from an old link) still shows, as its raw id.
      value: options[key].find((o) => o.value === filters[key])?.label ?? filters[key],
    }));
}

// ---- Matching: registered agents ----------------------------------------

function matchesRegisteredSearch(agent: AgentRegistryEntry, term: string): boolean {
  const searchable = [
    agent.displayName,
    agent.name,
    agent.description ?? "",
    agent.projectDisplayName,
    agent.createdBy?.displayName ?? "",
    ...Object.entries(agent.labels).map(([k, v]) => tagText(k, v)),
  ];
  return searchable.some((text) => text.toLowerCase().includes(term));
}

function matchesOwner(agent: AgentRegistryEntry, owner: string): boolean {
  if (owner === ALL) return true;
  if (owner === NONE) return agent.createdBy === null;
  return agent.createdBy?.id === owner;
}

function matchesTag(agent: AgentRegistryEntry, tag: string): boolean {
  if (tag === ALL) return true;
  return Object.entries(agent.labels).some(([k, v]) => tagText(k, v) === tag);
}

/**
 * The environments the status, LLM provider and MCP server filters look at: the
 * chosen environment only, or every environment when none is chosen.
 */
function environmentsInScope(
  agent: AgentRegistryEntry,
  environment: string,
): AgentRegistryEnvironment[] {
  if (environment === ALL) return agent.environments;
  return agent.environments.filter((env) => env.name === environment);
}

/**
 * With a status, the agent must have it in the chosen environment (or in any). Without
 * one, a chosen environment keeps the agents deployed there at least once.
 */
function matchesDeployment(agent: AgentRegistryEntry, environment: string, status: string) {
  if (status !== ALL) {
    return environmentsInScope(agent, environment).some((env) => env.deploymentStatus === status);
  }
  if (environment === ALL) return true;
  return agent.environments.some(
    (env) => env.name === environment && env.deploymentStatus !== "not-deployed",
  );
}

function canReachLlmProvider(agent: AgentRegistryEntry, environment: string, llmProvider: string) {
  if (llmProvider === ALL) return true;
  const envs = environmentsInScope(agent, environment);
  if (llmProvider === NONE) return envs.every((env) => env.llmProviders.length === 0);
  return envs.some((env) => env.llmProviders.some((p) => p.id === llmProvider));
}

function canReachMcpServer(agent: AgentRegistryEntry, environment: string, mcpServer: string) {
  if (mcpServer === ALL) return true;
  const envs = environmentsInScope(agent, environment);
  if (mcpServer === NONE) return envs.every((env) => env.mcpServers.length === 0);
  return envs.some((env) => env.mcpServers.some((s) => s.id === mcpServer));
}

function matchesRegistered(agent: AgentRegistryEntry, filters: AgentRegistryFilters): boolean {
  const term = filters.search.trim().toLowerCase();
  return (
    (!term || matchesRegisteredSearch(agent, term)) &&
    (filters.hosting === ALL || agent.hosting === filters.hosting) &&
    (filters.interface === ALL || agent.interface === filters.interface) &&
    (filters.project === ALL || agent.projectName === filters.project) &&
    matchesOwner(agent, filters.owner) &&
    matchesTag(agent, filters.tag) &&
    matchesDeployment(agent, filters.environment, filters.status) &&
    canReachLlmProvider(agent, filters.environment, filters.llmProvider) &&
    canReachMcpServer(agent, filters.environment, filters.mcpServer)
  );
}

// ---- Matching: unregistered agents --------------------------------------

function matchesUnregistered(agent: AgentRegistryEntry, filters: AgentRegistryFilters): boolean {
  const term = filters.search.trim().toLowerCase();
  const searchable = [
    agent.displayName,
    agent.name,
    agent.discovery?.providerDisplayName ?? "",
    agent.discovery?.scope ?? "",
    ...(agent.discovery?.foundBy.map((s) => s.connectionName) ?? []),
  ];
  return (
    (!term || searchable.some((text) => text.toLowerCase().includes(term))) &&
    (filters.foundOn === ALL || agent.discovery?.provider === filters.foundOn)
  );
}

/** Keeps the agents of the chosen registration that match every filter. */
export function filterAgents(
  agents: AgentRegistryEntry[],
  filters: AgentRegistryFilters,
): AgentRegistryEntry[] {
  return agents.filter((agent) => {
    if (!isListed(agent) || agent.registration !== filters.registration) return false;
    return agent.registration === "registered"
      ? matchesRegistered(agent, filters)
      : matchesUnregistered(agent, filters);
  });
}

/** How many agents of each registration there are, for the toggle's labels. */
export function countByRegistration(
  agents: AgentRegistryEntry[],
): Record<AgentRegistryRegistration, number> {
  const counts = { registered: 0, unregistered: 0 };
  for (const agent of agents) {
    if (isListed(agent)) counts[agent.registration] += 1;
  }
  return counts;
}

/** The latest deployment across the agent's environments; null when never deployed. */
export function lastDeployedAt(agent: AgentRegistryEntry): string | null {
  const times = agent.environments
    .map((env) => env.lastDeployedAt)
    .filter((t): t is string => t !== null);
  if (times.length === 0) return null;
  return times.reduce((latest, t) => (t > latest ? t : latest));
}

/** True when the search or any panel filter of the current registration is narrowing the list. */
export function isFiltered(filters: AgentRegistryFilters): boolean {
  if (filters.search.trim() !== "") return true;
  return PANEL_FILTER_KEYS[filters.registration].some((key) => filters[key] !== ALL);
}
