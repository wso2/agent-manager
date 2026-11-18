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
    const { owners } = buildFilterOptions([owned, devOnly]);
    expect(owners.map((o) => o.label)).toEqual(["No owner", "Owner One"]);
  });
});

describe("isFiltered", () => {
  it("ignores a blank search", () => {
    expect(isFiltered({ ...EMPTY_FILTERS, search: "  " })).toBe(false);
    expect(isFiltered({ ...EMPTY_FILTERS, tag: "team:hr" })).toBe(true);
  });
});
