/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useSearchParams } from "react-router-dom";
import { vi } from "vitest";
import { ThunderInstancesOrganization } from "./ThunderInstances.Organization";

// ThunderInstancesOrganization's own job is routing: it dispatches
// agents/roles/groups sub-paths to their Organization components and
// resolves the legacy `view/:envName/*` redirect. Those sub-organizations
// pull in AgentIdentityEnvironmentGate -> useAgentIdentityEnvName ->
// useListEnvironments (a real TanStack Query hook) several layers down, so
// stub them out at the module boundary to keep this test focused on the
// routing behavior that actually lives in this file.
vi.mock("./AgentsOrganization", () => ({
  AgentsOrganization: () => {
    const [searchParams] = useSearchParams();
    return <div>AGENTS ORGANIZATION envName={searchParams.get("envName") ?? "none"}</div>;
  },
}));
vi.mock("./RolesOrganization", () => ({
  RolesOrganization: () => <div>ROLES ORGANIZATION</div>,
}));
vi.mock("./GroupsOrganization", () => ({
  GroupsOrganization: () => <div>GROUPS ORGANIZATION</div>,
}));

function renderAt(initialEntry: string) {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route
          path="/org/:orgId/thunder-instances/*"
          element={<ThunderInstancesOrganization />}
        />
        <Route path="/org/:orgId/environments" element={<div>ENVIRONMENTS LIST PAGE</div>} />
        <Route
          path="/org/:orgId/environments/:envName"
          element={<div>ENVIRONMENT VIEW PAGE</div>}
        />
      </Routes>
    </MemoryRouter>
  );
}

describe("ThunderInstancesOrganization", () => {
  it("routes agents/* to the Agents sub-organization", () => {
    renderAt("/org/acme/thunder-instances/agents");
    expect(screen.getByText("AGENTS ORGANIZATION envName=none")).toBeInTheDocument();
  });

  it("routes roles/* to the Roles sub-organization", () => {
    renderAt("/org/acme/thunder-instances/roles");
    expect(screen.getByText("ROLES ORGANIZATION")).toBeInTheDocument();
  });

  it("routes groups/* to the Groups sub-organization", () => {
    renderAt("/org/acme/thunder-instances/groups");
    expect(screen.getByText("GROUPS ORGANIZATION")).toBeInTheDocument();
  });

  it("redirects a bare legacy view/:envName URL to the per-environment overview page", () => {
    renderAt("/org/acme/thunder-instances/view/prod");
    expect(screen.getByText("ENVIRONMENT VIEW PAGE")).toBeInTheDocument();
  });

  it("redirects a legacy view/:envName/agents URL to the top-level Agents page, preserving envName as a query param", () => {
    renderAt("/org/acme/thunder-instances/view/prod/agents");
    expect(screen.getByText("AGENTS ORGANIZATION envName=prod")).toBeInTheDocument();
  });

  it("redirects unrecognized sub-paths to the org's environments list", () => {
    renderAt("/org/acme/thunder-instances/nonsense");
    expect(screen.getByText("ENVIRONMENTS LIST PAGE")).toBeInTheDocument();
  });
});
