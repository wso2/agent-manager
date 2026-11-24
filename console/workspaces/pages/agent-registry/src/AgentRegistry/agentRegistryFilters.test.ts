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

import type { AgentRegistryEntry, AgentRegistryEnvironment } from "@agent-management-platform/types";
import {
  EMPTY_FILTERS,
  NONE,
  buildFilterOptions,
  countByRegistration,
  filterAgents,
  filterChips,
  filtersFromSearchParams,
  filtersToSearchParams,
  isFiltered,
  isListed,
  lastDeployedAt,
  withRegistration,
} from "./agentRegistryFilters";

const payments = { id: "payments-mcp", displayName: "Payments", tools: ["get_invoice"] };

function agent(overrides: Partial<AgentRegistryEntry>): AgentRegistryEntry {
  return {
    name: "a",
    displayName: "A",
    registration: "registered",
    projectName: "p",
    projectDisplayName: "P",
    hosting: "platform",
    interface: "custom-api",
    createdBy: null,
    labels: {},
    environments: [],
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function env(overrides: Partial<AgentRegistryEnvironment>): AgentRegistryEnvironment {
  return {
    name: "development",
    displayName: "Development",
    isProduction: false,
    deploymentStatus: "active",
    lastDeployedAt: null,
    llmProviders: [],
    mcpServers: [],
    ...overrides,
  };
}

// Payments in development only; suspended in production.
const devOnly = agent({
  name: "dev-only",
  interface: "a2a-agent",
  environments: [
    env({ mcpServers: [payments], lastDeployedAt: "2026-02-01T00:00:00Z" }),
    env({
      name: "production",
      displayName: "Production",
      isProduction: true,
      deploymentStatus: "suspended",
      lastDeployedAt: "2026-03-01T00:00:00Z",
    }),
  ],
});

const owned = agent({
  name: "owned",
  hosting: "external",
  createdBy: { id: "u1", displayName: "Owner One" },
  labels: { team: "hr" },
});

const shadow = agent({
  name: "shadow",
  registration: "unregistered",
  projectName: "",
  projectDisplayName: "",
  hosting: "external",
  interface: null,
  discovery: {
    findingId: "f1",
    provider: "openai",
    providerDisplayName: "OpenAI",
    scope: "proj_cs",
    ownerHint: { email: "dev@example.com" },
    firstSeenAt: "2026-01-01T00:00:00Z",
    lastSeenAt: "2026-01-02T00:00:00Z",
    present: true,
    onboarded: false,
    connectionName: "OpenAI prod",
    kind: "platform_agent",
    externalId: "agent_alpha",
    state: "active",
    foundBy: [{ findingId: "f1", connectionId: "c1", connectionName: "OpenAI prod", present: true }],
  },
});

const all = [devOnly, owned, shadow];
const names = (agents: AgentRegistryEntry[]) => agents.map((a) => a.name);

describe("registration toggle", () => {
  it("lists registered agents by default, and only unregistered ones when switched", () => {
    expect(names(filterAgents(all, EMPTY_FILTERS))).toEqual(["dev-only", "owned"]);
    const unregistered = withRegistration(EMPTY_FILTERS, "unregistered");
    expect(names(filterAgents(all, unregistered))).toEqual(["shadow"]);
  });

  it("drops the other registration's filters when switching", () => {
    const filters = { ...EMPTY_FILTERS, mcpServer: payments.id, search: "x" };
    const switched = withRegistration(filters, "unregistered");
    expect(switched.mcpServer).toBe(EMPTY_FILTERS.mcpServer);
    expect(switched.search).toBe("x");
  });

  it("counts both, for the toggle's labels", () => {
    expect(countByRegistration(all)).toEqual({ registered: 2, unregistered: 1 });
  });
});

describe("registered filters", () => {
  it("filters by hosting and by type", () => {
    expect(names(filterAgents(all, { ...EMPTY_FILTERS, hosting: "external" }))).toEqual([
      "owned",
    ]);
    expect(names(filterAgents(all, { ...EMPTY_FILTERS, interface: "a2a-agent" }))).toEqual([
      "dev-only",
    ]);
  });

  it("finds agents whose owner is unknown", () => {
    expect(names(filterAgents(all, { ...EMPTY_FILTERS, owner: NONE }))).toEqual(["dev-only"]);
  });

  it("checks the status in the chosen environment, or in any", () => {
    const suspended = { ...EMPTY_FILTERS, status: "suspended" };
    expect(names(filterAgents(all, suspended))).toEqual(["dev-only"]);
    expect(filterAgents(all, { ...suspended, environment: "development" })).toEqual([]);
  });

  it("checks the MCP server only in the chosen environment", () => {
    const filters = { ...EMPTY_FILTERS, mcpServer: payments.id };
    expect(names(filterAgents(all, filters))).toEqual(["dev-only"]);
    expect(filterAgents(all, { ...filters, environment: "production" })).toEqual([]);
  });

  it("finds agents with no MCP server", () => {
    expect(names(filterAgents(all, { ...EMPTY_FILTERS, mcpServer: NONE }))).toEqual(["owned"]);
  });

  it("searches tags as key:value", () => {
    expect(names(filterAgents(all, { ...EMPTY_FILTERS, search: "team:hr" }))).toEqual(["owned"]);
  });
});

describe("unregistered filters", () => {
  it("filters by the platform the agent was found on, and searches it", () => {
    const base = withRegistration(EMPTY_FILTERS, "unregistered");
    expect(names(filterAgents(all, { ...base, foundOn: "openai" }))).toEqual(["shadow"]);
    expect(filterAgents(all, { ...base, foundOn: "bedrock" })).toEqual([]);
    expect(names(filterAgents(all, { ...base, search: "proj_cs" }))).toEqual(["shadow"]);
  });
});

describe("options and chips", () => {
  it("offers Unknown first for owners, and None first for MCP servers", () => {
    const options = buildFilterOptions(all);
    expect(options.owner.map((o) => o.label)).toEqual(["Unknown", "Owner One"]);
    expect(options.mcpServer[0]).toEqual({ value: NONE, label: "No MCP server" });
    expect(options.foundOn).toEqual([{ value: "openai", label: "OpenAI" }]);
  });

  it("names only the current registration's applied filters", () => {
    const options = buildFilterOptions(all);
    const filters = { ...EMPTY_FILTERS, status: "suspended", foundOn: "openai" };
    expect(filterChips(filters, options)).toEqual([
      { key: "status", name: "Status", value: "Suspended" },
    ]);
  });

  it("is not filtered by the registration toggle alone", () => {
    expect(isFiltered(withRegistration(EMPTY_FILTERS, "unregistered"))).toBe(false);
    expect(isFiltered({ ...EMPTY_FILTERS, hosting: "platform" })).toBe(true);
  });
});

describe("URL", () => {
  it("keeps only the filters that are set, and other parameters", () => {
    const params = filtersToSearchParams(
      { ...EMPTY_FILTERS, mcpServer: payments.id, environment: "production" },
      new URLSearchParams("view=tiles"),
    );
    expect(params.toString()).toBe("view=tiles&env=production&mcp=payments-mcp");
  });

  it("reads back what it wrote, and ignores an unknown registration", () => {
    const filters = { ...EMPTY_FILTERS, search: "bot", owner: NONE, interface: "chat-api" };
    expect(filtersFromSearchParams(filtersToSearchParams(filters, new URLSearchParams()))).toEqual(
      filters,
    );
    const odd = new URLSearchParams("registration=both");
    expect(filtersFromSearchParams(odd).registration).toBe("registered");
  });
});

describe("lastDeployedAt", () => {
  it("is the latest deployment across environments, or null", () => {
    expect(lastDeployedAt(devOnly)).toBe("2026-03-01T00:00:00Z");
    expect(lastDeployedAt(owned)).toBeNull();
  });
});

describe("which shadow agents are listed", () => {
  const gone = agent({ ...shadow, name: "gone", discovery: { ...shadow.discovery!, present: false } });
  const onboarded = agent({
    ...shadow,
    name: "onboarded",
    discovery: { ...shadow.discovery!, onboarded: true },
  });

  it("leaves out the ones gone from their platform and the ones already onboarded", () => {
    expect([shadow, gone, onboarded].map(isListed)).toEqual([true, false, false]);
    const unregistered = withRegistration(EMPTY_FILTERS, "unregistered");
    expect(names(filterAgents([shadow, gone, onboarded], unregistered))).toEqual(["shadow"]);
    expect(countByRegistration([shadow, gone, onboarded]).unregistered).toBe(1);
  });

  it("searches the connections that found it (Source)", () => {
    const filters = { ...withRegistration(EMPTY_FILTERS, "unregistered"), search: "openai prod" };
    expect(names(filterAgents(all, filters))).toEqual(["shadow"]);
  });
});
