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

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { Link, MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  accessState,
  apiState,
  resetAccessState,
  resetApiState,
  ROUTER_FUTURE,
} from "./__test__/mocks";
import { ThunderInstancesOrganization } from "./ThunderInstances.Organization";

vi.mock(
  "@agent-management-platform/api-client",
  async () => (await import("./__test__/mocks")).apiClientMock,
);
vi.mock(
  "@agent-management-platform/feature-management",
  async () => (await import("./__test__/mocks")).featureManagementMock,
);
vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  ...(await import("./__test__/mocks")).sharedComponentOverrides,
}));

// Oxygen UI pages are slow to render under a loaded CI runner.
vi.setConfig({ testTimeout: 60_000 });

function Location() {
  const location = useLocation();
  return (
    <>
      <output>{`${location.pathname}${location.search}`}</output>
      <Link to="/org/acme/thunder-instances/roles">go to roles</Link>
    </>
  );
}

const renderModule = (url: string, future: Partial<typeof ROUTER_FUTURE> = ROUTER_FUTURE) =>
  render(
    <MemoryRouter initialEntries={[url]} future={future}>
      <Routes>
        <Route
          path="/org/:orgId/thunder-instances/*"
          element={
            <>
              <ThunderInstancesOrganization />
              <Location />
            </>
          }
        />
      </Routes>
    </MemoryRouter>,
  );

const currentUrl = () => screen.getByRole("status").textContent;

beforeEach(() => {
  vi.clearAllMocks();
  resetApiState();
  resetAccessState();
  apiState.queries.useListEnvironments = {
    data: [
      { name: "dev", displayName: "Development" },
      { name: "prod", displayName: "Production" },
    ],
  };
  apiState.queries.useListAgentIdentityGroups = {
    data: { groups: [{ id: "g1", name: "on-call" }], total: 1 },
  };
  apiState.queries.useListAgentIdentityRoles = {
    data: { roles: [{ id: "r1", name: "admin" }], total: 1 },
  };
  apiState.queries.useListAgentIdentityAgents = {
    data: {
      agents: [
        {
          agentName: "support-bot",
          projectName: "shop",
          status: "completed",
          thunderAgentId: "tid-1",
        },
      ],
    },
  };
});

describe("ThunderInstancesOrganization", () => {
  it("lands on the groups page in the first environment", async () => {
    renderModule("/org/acme/thunder-instances");

    await waitFor(() =>
      expect(currentUrl()).toBe("/org/acme/thunder-instances/groups?envName=dev"),
    );
    expect(
      screen.getByText("Groups available in the selected environment's identity provider."),
    ).toBeInTheDocument();
    expect(screen.getByText("on-call")).toBeInTheDocument();
  });

  it("links writers to the create page, keeping the environment", async () => {
    renderModule("/org/acme/thunder-instances/groups?envName=prod");

    const create = await screen.findByRole("link", { name: "Create group" });
    expect(create).toHaveAttribute(
      "href",
      "/org/acme/thunder-instances/groups/create?envName=prod",
    );
  });

  it("hides the create action from read-only users", () => {
    accessState.canWrite = false;
    renderModule("/org/acme/thunder-instances/roles?envName=dev");

    expect(screen.getByText("admin")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Create role" })).not.toBeInTheDocument();
  });

  it("shows the roles list with its create action", () => {
    renderModule("/org/acme/thunder-instances/roles?envName=dev");

    expect(
      screen.getByText("Roles available in the selected environment's identity provider."),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Create role" })).toHaveAttribute(
      "href",
      "/org/acme/thunder-instances/roles/create?envName=dev",
    );
  });

  it("opens the create pages for writers", () => {
    const { unmount } = renderModule("/org/acme/thunder-instances/groups/create?envName=dev");
    expect(screen.getByText("Create Group")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("on-call-agents")).toBeInTheDocument();
    unmount();

    renderModule("/org/acme/thunder-instances/roles/create?envName=dev");
    expect(screen.getByText("Create Role")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("agent-admin")).toBeInTheDocument();
  });

  it("sends read-only users away from the create pages", async () => {
    accessState.canWrite = false;
    const { unmount } = renderModule("/org/acme/thunder-instances/groups/create?envName=dev");
    await waitFor(() =>
      expect(currentUrl()).toBe("/org/acme/thunder-instances/groups?envName=dev"),
    );
    unmount();

    renderModule("/org/acme/thunder-instances/roles/create?envName=prod");
    await waitFor(() =>
      expect(currentUrl()).toBe("/org/acme/thunder-instances/roles?envName=prod"),
    );
  });

  it("opens group and role detail pages", () => {
    apiState.queries.useGetAgentIdentityGroup = { data: { id: "g1", name: "on-call group" } };
    const { unmount } = renderModule("/org/acme/thunder-instances/groups/g1?envName=dev");
    expect(screen.getByText("on-call group")).toBeInTheDocument();
    unmount();

    apiState.queries.useGetAgentIdentityRole = { data: { id: "r1", name: "admin role" } };
    renderModule("/org/acme/thunder-instances/roles/r1?envName=dev");
    expect(screen.getByText("admin role")).toBeInTheDocument();
  });

  it("lists agents and switches environment from the selector", async () => {
    renderModule("/org/acme/thunder-instances/agents?envName=dev");

    expect(screen.getByText("support-bot label")).toBeInTheDocument();
    expect(screen.getByText("Development Environment")).toBeInTheDocument();

    fireEvent.mouseDown(screen.getByRole("combobox"));
    fireEvent.click(await screen.findByRole("option", { name: "Production" }));

    await waitFor(() =>
      expect(currentUrl()).toBe("/org/acme/thunder-instances/agents?envName=prod"),
    );
  });

  it("opens an agent's detail page", () => {
    apiState.queries.useGetAgent = { data: { displayName: "Support Bot" } };
    renderModule("/org/acme/thunder-instances/agents/shop/support-bot?envName=dev");

    expect(screen.getByText("Support Bot")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Roles" })).toBeInTheDocument();
  });

  it("remembers the last environment when moving between pages", async () => {
    renderModule("/org/acme/thunder-instances/groups?envName=prod");

    fireEvent.click(screen.getByRole("link", { name: "go to roles" }));

    await waitFor(() =>
      expect(currentUrl()).toBe("/org/acme/thunder-instances/roles?envName=prod"),
    );
  });

  it("replaces an unknown environment with the default", async () => {
    renderModule("/org/acme/thunder-instances/groups?envName=gone");

    await waitFor(() =>
      expect(currentUrl()).toBe("/org/acme/thunder-instances/groups?envName=dev"),
    );
  });

  it("redirects unknown sub-paths back to the section", async () => {
    // The console's BrowserRouter resolves "." against the parent route (no
    // v7_relativeSplatPath), which is what this redirect relies on.
    renderModule("/org/acme/thunder-instances/agents/a/b/c?envName=dev", {
      v7_startTransition: true,
    });

    await waitFor(() =>
      expect(currentUrl()).toBe("/org/acme/thunder-instances/agents?envName=dev"),
    );
  });

  it.each([
    [
      "/org/acme/thunder-instances/view/prod/roles/r1",
      "/org/acme/thunder-instances/roles/r1?envName=prod",
    ],
    [
      "/org/acme/thunder-instances/view/prod/agents",
      "/org/acme/thunder-instances/agents?envName=prod",
    ],
    [
      "/org/acme/thunder-instances/view/prod/settings",
      "/org/acme/thunder-instances/groups?envName=prod",
    ],
  ])("redirects the legacy link %s", async (legacy, target) => {
    renderModule(legacy);

    await waitFor(() => expect(currentUrl()).toBe(target));
  });

  it("offers a retry when environments fail to load", () => {
    const refetch = vi.fn();
    apiState.queries.useListEnvironments = { isError: true, refetch };
    renderModule("/org/acme/thunder-instances/groups");

    expect(screen.getByText("Failed to load environments")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(refetch).toHaveBeenCalled();
  });

  it("explains when the organization has no environments", () => {
    apiState.queries.useListEnvironments = { data: [] };
    renderModule("/org/acme/thunder-instances/roles");

    expect(screen.getByText("No environments yet")).toBeInTheDocument();
    expect(
      screen.getByText("Add an environment first to manage agent identity."),
    ).toBeInTheDocument();
  });

  it("waits for environments before choosing one", () => {
    apiState.queries.useListEnvironments = { isLoading: true };
    renderModule("/org/acme/thunder-instances/agents");

    expect(currentUrl()).toBe("/org/acme/thunder-instances/agents");
    expect(screen.queryByText("No environments yet")).not.toBeInTheDocument();
    expect(screen.queryByText("support-bot label")).not.toBeInTheDocument();
  });
});
