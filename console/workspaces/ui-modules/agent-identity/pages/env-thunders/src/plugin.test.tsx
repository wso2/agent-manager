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

import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import type { ResourceSummaryState } from "@agent-management-platform/types";
import { ROUTER_FUTURE } from "./__test__/mocks";
import { agentIdentityPlugin } from "./plugin";

// The page modules themselves are tested on their own; here only the wiring
// from each contribution to its export matters.
vi.mock("./index", () => ({
  thunderInstancesMetaData: { levels: { organization: "ThunderInstancesOrganization" } },
  agentIdMetaData: { component: "AgentIdComponent" },
  EnvAgentRolesGroupsSection: "EnvAgentRolesGroupsSection",
}));

vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureAccess: (key: string) => key === "agent_identity_read",
}));

const contribution = (id: string) => {
  const found = agentIdentityPlugin.contributions.find((c) => c.id === id);
  if (!found) throw new Error(`no contribution ${id}`);
  return found as unknown as Record<string, unknown>;
};

const inOrg = ({ children }: { children: ReactNode }) => (
  <MemoryRouter initialEntries={["/org/acme"]} future={ROUTER_FUTURE}>
    <Routes>
      <Route path="/org/:orgId" element={children} />
    </Routes>
  </MemoryRouter>
);

describe("agentIdentityPlugin", () => {
  it.each([
    ["identity-groups-summary", "Manage groups", "/org/acme/thunder-instances/groups"],
    ["identity-roles-summary", "Manage roles", "/org/acme/thunder-instances/roles"],
  ])("points the %s card at the org's page", (id, actionLabel, to) => {
    const useSummary = contribution(id).useSummary as () => ResourceSummaryState;
    const { result } = renderHook(() => useSummary(), { wrapper: inOrg });

    expect(result.current).toEqual({ countless: true, actionLabel, to });
  });

  it("shows the groups and roles nav items only to users who can read identities", () => {
    for (const id of ["groups-nav", "roles-nav"]) {
      const useActivation = contribution(id).useActivation as () => boolean;
      const { result } = renderHook(() => useActivation());
      expect(result.current).toBe(true);
    }
  });

  it("mounts the org pages under thunder-instances and the agent page at agent-id", () => {
    expect(contribution("org-page").path).toBe("/org/:orgId/thunder-instances/*");
    expect(contribution("agent-id-page").path).toBe(
      "/org/:orgId/project/:projectId/agents/:agentId/agent-id",
    );
  });

  it("lazy-loads the page components", async () => {
    const load = (id: string) => (contribution(id).load as () => Promise<{ default: unknown }>)();

    expect((await load("org-page")).default).toBe("ThunderInstancesOrganization");
    expect((await load("agent-id-page")).default).toBe("AgentIdComponent");
    expect((await load("overview-roles-groups")).default).toBe("EnvAgentRolesGroupsSection");
  });
});
