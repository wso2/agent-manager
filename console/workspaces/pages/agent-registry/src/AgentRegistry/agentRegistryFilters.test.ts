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

import type { AgentRegistryEntry } from "@agent-management-platform/types";
import {
  EMPTY_FILTERS,
  NO_OWNER,
  buildFilterOptions,
  filterAgents,
  filterChips,
  filtersFromSearchParams,
  filtersToSearchParams,
  isFiltered,
} from "./agentRegistryFilters";

const payments = { id: "payments-mcp", displayName: "Payments" };

function agent(overrides: Partial<AgentRegistryEntry>): AgentRegistryEntry {
  return {
    name: "a",
    displayName: "A",
    projectName: "p",
    projectDisplayName: "P",
    hosting: "platform",
    interface: "custom-api",
    owner: null,
    lifecycleState: "active",
    visibility: "project",
    labels: {},
    environments: [],
    lastActivityAt: null,
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

// Payments is reachable in development only.
const devOnly = agent({
  name: "dev-only",
  environments: [
    {
      name: "development",
      displayName: "Development",
      isProduction: false,
      deploymentStatus: "active",
      llmProviders: [],
      mcpServers: [payments],
    },
    {
      name: "production",
      displayName: "Production",
      isProduction: true,
      deploymentStatus: "active",
      llmProviders: [],
      mcpServers: [],
    },
  ],
});

const owned = agent({
  name: "owned",
  owner: { type: "user", id: "u1", displayName: "Owner One" },
  labels: { team: "hr" },
});

describe("filterAgents", () => {
  it("finds agents that can reach an MCP server in any environment", () => {
    const result = filterAgents([devOnly, owned], { ...EMPTY_FILTERS, mcpServer: payments.id });
    expect(result.map((a) => a.name)).toEqual(["dev-only"]);
  });

  it("checks the MCP server only in the chosen environment", () => {
    const result = filterAgents([devOnly], {
      ...EMPTY_FILTERS,
      environment: "production",
      mcpServer: payments.id,
    });
    expect(result).toEqual([]);
  });

  it("finds agents with no owner", () => {
    const result = filterAgents([devOnly, owned], { ...EMPTY_FILTERS, owner: NO_OWNER });
    expect(result.map((a) => a.name)).toEqual(["dev-only"]);
  });

  it("searches tags as key:value", () => {
    const result = filterAgents([devOnly, owned], { ...EMPTY_FILTERS, search: "team:hr" });
    expect(result.map((a) => a.name)).toEqual(["owned"]);
  });
});

describe("buildFilterOptions", () => {
  it("offers 'No owner' first when an agent has no owner", () => {
    const { owner } = buildFilterOptions([owned, devOnly]);
    expect(owner.map((o) => o.label)).toEqual(["No owner", "Owner One"]);
  });
});

describe("isFiltered", () => {
  it("ignores a blank search", () => {
    expect(isFiltered({ ...EMPTY_FILTERS, search: "  " })).toBe(false);
    expect(isFiltered({ ...EMPTY_FILTERS, tag: "team:hr" })).toBe(true);
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

  it("reads back what it wrote", () => {
    const filters = { ...EMPTY_FILTERS, search: "bot", owner: NO_OWNER, tag: "team:hr" };
    expect(filtersFromSearchParams(filtersToSearchParams(filters, new URLSearchParams()))).toEqual(
      filters,
    );
  });
});

describe("filterChips", () => {
  it("names each applied panel filter with its label", () => {
    const options = buildFilterOptions([devOnly, owned]);
    const chips = filterChips(
      { ...EMPTY_FILTERS, owner: NO_OWNER, mcpServer: payments.id, lifecycleState: "active" },
      options,
    );
    expect(chips).toEqual([
      { key: "owner", name: "Owner", value: "No owner" },
      { key: "mcpServer", name: "MCP server", value: "Payments" },
    ]);
  });
});
