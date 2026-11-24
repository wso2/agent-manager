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

import { fireEvent, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  accessState,
  apiState,
  mutation,
  renderAt,
  resetAccessState,
  resetApiState,
  sharedState,
  last,
} from "../../__test__/mocks";
import { RoleEditPage } from "./RoleEditPage";

vi.mock(
  "@agent-management-platform/api-client",
  async () => (await import("../../__test__/mocks")).apiClientMock,
);
vi.mock(
  "@agent-management-platform/feature-management",
  async () => (await import("../../__test__/mocks")).featureManagementMock,
);
vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  ...(await import("../../__test__/mocks")).sharedComponentOverrides,
}));

// Oxygen UI pages are slow to render under a loaded CI runner.
vi.setConfig({ testTimeout: 60_000 });

const PARAMS = { orgName: "acme", envName: "dev", roleId: "r1" };

const renderPage = () =>
  renderAt(
    <RoleEditPage />,
    "/org/:orgId/thunder-instances/roles/:roleId",
    "/org/acme/thunder-instances/roles/r1?envName=dev",
  );

const pickOption = async (placeholder: string, option: RegExp | string) => {
  fireEvent.mouseDown(screen.getByPlaceholderText(placeholder));
  fireEvent.click(await screen.findByRole("option", { name: option }));
};

const rowOf = (text: string) => screen.getByText(text).closest("tr") as HTMLElement;
const save = () => fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

beforeEach(() => {
  vi.clearAllMocks();
  resetApiState();
  resetAccessState();
  sharedState.guardDirty = [];
  apiState.queries.useGetAgentIdentityRole = {
    data: {
      id: "r1",
      name: "order-admin",
      description: "Manages orders",
      permissions: [{ permissions: ["orders:read", "legacy:scope"] }],
    },
  };
  apiState.queries.useListAgentIdentityScopes = {
    data: {
      scopes: [
        { scope: "orders:read" },
        { scope: "orders:write", description: "Create and change orders" },
      ],
    },
  };
  apiState.queries.useGetAgentIdentityRoleAssignments = {
    data: {
      agents: [{ id: "tid-1" }],
      groups: [{ id: "g1", name: "on-call", description: "Pager rotation" }],
    },
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
        {
          agentName: "pricing",
          projectName: "finance",
          status: "completed",
          thunderAgentId: "tid-2",
        },
      ],
    },
  };
  apiState.queries.useListAgentIdentityGroups = {
    data: {
      groups: [
        { id: "g1", name: "on-call" },
        { id: "g2", name: "readers" },
      ],
      total: 2,
    },
  };
});

describe("RoleEditPage", () => {
  it("shows a skeleton while the role loads", () => {
    apiState.queries.useGetAgentIdentityRole = { isLoading: true };
    renderPage();

    expect(screen.getByText("Role")).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Permissions" })).not.toBeInTheDocument();
  });

  it("shows the role's scopes checked in the catalog tree", () => {
    renderPage();

    expect(screen.getByText("order-admin")).toBeInTheDocument();
    expect(screen.getByText("Check the scopes to assign to this role.")).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Orders / Read" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Orders / Write" })).not.toBeChecked();
    expect(screen.getByText("Create and change orders")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
  });

  it("saves the full scope set, keeping a scope that left the catalog", async () => {
    renderPage();

    fireEvent.click(screen.getByRole("checkbox", { name: "Orders / Write" }));
    expect(last(sharedState.guardDirty)).toBe(true);
    save();

    await screen.findByText("Role updated successfully.");
    expect(mutation("useUpdateAgentIdentityRole")).toHaveBeenCalledWith({
      params: PARAMS,
      body: {
        name: "order-admin",
        description: "Manages orders",
        scopes: ["orders:read", "legacy:scope", "orders:write"],
      },
    });
    expect(mutation("useAddAgentIdentityRoleAssignees")).not.toHaveBeenCalled();
  });

  it("locks the permissions of a predefined role", () => {
    apiState.queries.useGetAgentIdentityRole = {
      data: {
        id: "r1",
        name: "system",
        isReadOnly: true,
        permissions: [{ permissions: ["orders:read"] }],
      },
    };
    renderPage();

    expect(screen.getByText("Read-only")).toBeInTheDocument();
    expect(
      screen.getByText("Permissions for predefined roles cannot be modified."),
    ).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Orders / Write" })).toBeDisabled();
  });

  it("explains an empty scope catalog", () => {
    apiState.queries.useListAgentIdentityScopes = { data: { scopes: [] } };
    renderPage();

    expect(screen.getByText("No scopes in the catalog.")).toBeInTheDocument();
  });

  it("assigns and unassigns agents", async () => {
    renderPage();
    fireEvent.click(screen.getByRole("tab", { name: "Agents" }));

    expect(within(rowOf("tid-1")).getByText("support-bot label")).toBeInTheDocument();
    await pickOption("Search agents...", /pricing label/);
    fireEvent.click(within(rowOf("tid-1")).getByRole("button"));
    save();

    await screen.findByText("Role updated successfully.");
    expect(mutation("useAddAgentIdentityRoleAssignees")).toHaveBeenCalledWith({
      params: PARAMS,
      body: { assignments: [{ id: "tid-2", type: "agent" }] },
    });
    expect(mutation("useRemoveAgentIdentityRoleAssignees")).toHaveBeenCalledWith({
      params: PARAMS,
      body: { assignments: [{ id: "tid-1", type: "agent" }] },
    });
    // Scopes were not touched, so the role itself is not updated.
    expect(mutation("useUpdateAgentIdentityRole")).not.toHaveBeenCalled();
  });

  it("assigns and unassigns groups", async () => {
    renderPage();
    fireEvent.click(screen.getByRole("tab", { name: "Groups" }));

    expect(screen.getByText("Pager rotation")).toBeInTheDocument();
    await pickOption("Search groups...", "readers");
    fireEvent.click(within(rowOf("on-call")).getByRole("button"));
    save();

    await screen.findByText("Role updated successfully.");
    expect(mutation("useAddAgentIdentityRoleAssignees")).toHaveBeenCalledWith({
      params: PARAMS,
      body: { assignments: [{ id: "g2", type: "group" }] },
    });
    expect(mutation("useRemoveAgentIdentityRoleAssignees")).toHaveBeenCalledWith({
      params: PARAMS,
      body: { assignments: [{ id: "g1", type: "group" }] },
    });
  });

  it("reports a failed save and keeps the changes", async () => {
    mutation("useAddAgentIdentityRoleAssignees").mockRejectedValue(new Error("nope"));
    renderPage();
    fireEvent.click(screen.getByRole("tab", { name: "Groups" }));

    await pickOption("Search groups...", "readers");
    save();

    expect(await screen.findByText("Failed to update role. Please try again.")).toBeInTheDocument();
    expect(screen.getByText("readers")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save changes" })).toBeInTheDocument();
  });

  it("shows empty assignments and the group-page warning", () => {
    apiState.queries.useGetAgentIdentityRoleAssignments = { data: { agents: [], groups: [] } };
    apiState.queries.useListAgentIdentityGroups = { data: { groups: [], total: 130 } };
    renderPage();

    fireEvent.click(screen.getByRole("tab", { name: "Agents" }));
    expect(screen.getByText("No agents assigned yet")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: "Groups" }));
    expect(screen.getByText("No groups assigned yet")).toBeInTheDocument();
    expect(screen.getByText(/Showing the first 100 of 130 groups/)).toBeInTheDocument();
  });

  it("is read-only without write access", () => {
    accessState.canWrite = false;
    renderPage();

    expect(screen.getByText("Scopes assigned to this role.")).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Orders / Write" })).toBeDisabled();
    fireEvent.click(screen.getByRole("tab", { name: "Agents" }));
    expect(screen.getByText("Agents assigned to this role.")).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Search agents...")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: "Groups" }));
    expect(screen.getByText("Groups assigned to this role.")).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Search groups...")).not.toBeInTheDocument();
  });

  it("returns to the roles list on Cancel", () => {
    renderPage();

    fireEvent.click(screen.getByRole("checkbox", { name: "Orders / Write" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(
      screen.getByText("at /org/acme/thunder-instances/roles?envName=dev"),
    ).toBeInTheDocument();
  });
});
