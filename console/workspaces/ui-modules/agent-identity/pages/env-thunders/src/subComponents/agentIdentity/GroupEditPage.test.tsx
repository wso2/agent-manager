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
} from "../../__test__/mocks";
import { GroupEditPage } from "./GroupEditPage";

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

const PARAMS = { orgName: "acme", envName: "dev", groupId: "g1" };

const renderPage = () =>
  renderAt(
    <GroupEditPage />,
    "/org/:orgId/thunder-instances/groups/:groupId",
    "/org/acme/thunder-instances/groups/g1?envName=dev",
  );

const pickOption = async (placeholder: string, option: RegExp | string) => {
  fireEvent.mouseDown(screen.getByPlaceholderText(placeholder));
  fireEvent.click(await screen.findByRole("option", { name: option }));
};

const rowOf = (text: string) => screen.getAllByText(text)[0].closest("tr") as HTMLElement;

beforeEach(() => {
  vi.clearAllMocks();
  resetApiState();
  resetAccessState();
  apiState.queries.useGetAgentIdentityGroup = {
    data: { id: "g1", name: "on-call", description: "Pager rotation" },
  };
  apiState.queries.useGetAgentIdentityGroupMembers = {
    data: { members: [{ id: "tid-1" }, { id: "tid-orphan" }], total: 2 },
  };
  apiState.queries.useGetAgentIdentityGroupRoles = {
    data: { roles: [{ id: "r1", name: "admin", description: "Full access" }] },
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
        { agentName: "unbound", projectName: "shop", status: "pending" },
      ],
    },
  };
  apiState.queries.useListAgentIdentityRoles = {
    data: {
      roles: [
        { id: "r1", name: "admin" },
        { id: "r2", name: "viewer" },
      ],
      total: 2,
    },
  };
});

describe("GroupEditPage", () => {
  it("shows a skeleton while the group loads", () => {
    apiState.queries.useGetAgentIdentityGroup = { isLoading: true };
    renderPage();

    expect(screen.getByText("Group")).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Agents" })).not.toBeInTheDocument();
  });

  it("shows the group's members, resolving names where the agent is known", () => {
    renderPage();

    expect(screen.getByText("on-call")).toBeInTheDocument();
    expect(screen.getByText("Pager rotation")).toBeInTheDocument();
    expect(within(rowOf("tid-1")).getByText("support-bot label")).toBeInTheDocument();
    expect(within(rowOf("tid-1")).getByText("shop project")).toBeInTheDocument();
    // A member the agents list doesn't know falls back to its raw ID.
    expect(screen.getAllByText("tid-orphan")).toHaveLength(2);
    expect(apiState.calls.useGetAgentIdentityGroupMembers?.[0]).toEqual([
      PARAMS,
      { offset: 0, limit: 100 },
    ]);
  });

  it("only offers bound agents that are not already members", async () => {
    renderPage();

    fireEvent.mouseDown(screen.getByPlaceholderText("Search agents..."));
    const options = await screen.findAllByRole("option");
    expect(options).toHaveLength(1);
    expect(options[0]).toHaveTextContent("pricing label");
    expect(options[0]).toHaveTextContent("finance project");
  });

  it("saves added and removed members in one batch each", async () => {
    renderPage();

    await pickOption("Search agents...", /pricing label/);
    fireEvent.click(within(rowOf("tid-orphan")).getByRole("button"));
    expect(screen.queryByText("tid-orphan")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await screen.findByText("Group updated successfully.");
    expect(mutation("useAddAgentIdentityGroupMembers")).toHaveBeenCalledWith({
      params: PARAMS,
      body: { agentIds: ["tid-2"] },
    });
    expect(mutation("useRemoveAgentIdentityGroupMembers")).toHaveBeenCalledWith({
      params: PARAMS,
      body: { agentIds: ["tid-orphan"] },
    });
    expect(mutation("useAddAgentIdentityRoleAssignees")).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
  });

  it("drops a pending add when it is removed before saving", async () => {
    renderPage();

    await pickOption("Search agents...", /pricing label/);
    fireEvent.click(within(rowOf("tid-2")).getByRole("button"));

    expect(screen.queryByText("tid-2")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
  });

  it("keeps a failed part queued and says what failed", async () => {
    mutation("useAddAgentIdentityGroupMembers").mockRejectedValue(new Error("nope"));
    renderPage();

    await pickOption("Search agents...", /pricing label/);
    fireEvent.click(within(rowOf("tid-orphan")).getByRole("button"));
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(
      await screen.findByText("Failed to update group (adding members). Please try again."),
    ).toBeInTheDocument();
    // The failed add is still pending; the successful remove is not.
    expect(screen.getByText("tid-2")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save changes" })).toBeInTheDocument();
  });

  it("assigns and unassigns roles one call per role", async () => {
    renderPage();
    fireEvent.click(screen.getByRole("tab", { name: "Roles" }));

    expect(screen.getByText("Full access")).toBeInTheDocument();
    await pickOption("Search roles...", "viewer");
    fireEvent.click(within(rowOf("admin")).getByRole("button"));
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await screen.findByText("Group updated successfully.");
    const assignment = { assignments: [{ id: "g1", type: "group" }] };
    expect(mutation("useAddAgentIdentityRoleAssignees")).toHaveBeenCalledWith({
      params: { orgName: "acme", envName: "dev", roleId: "r2" },
      body: assignment,
    });
    expect(mutation("useRemoveAgentIdentityRoleAssignees")).toHaveBeenCalledWith({
      params: { orgName: "acme", envName: "dev", roleId: "r1" },
      body: assignment,
    });
  });

  it("reports a failed role update", async () => {
    mutation("useAddAgentIdentityRoleAssignees").mockRejectedValue(new Error("nope"));
    renderPage();
    fireEvent.click(screen.getByRole("tab", { name: "Roles" }));

    await pickOption("Search roles...", "viewer");
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(
      await screen.findByText(
        "Failed to update group (updating role assignments). Please try again.",
      ),
    ).toBeInTheDocument();
  });

  it("shows empty, loading and error states on the roles tab", () => {
    apiState.queries.useGetAgentIdentityGroupRoles = { data: { roles: [] } };
    const { unmount } = renderPage();
    fireEvent.click(screen.getByRole("tab", { name: "Roles" }));
    expect(screen.getByText("No roles assigned yet")).toBeInTheDocument();
    unmount();

    apiState.queries.useGetAgentIdentityGroupRoles = { isError: true };
    renderPage();
    fireEvent.click(screen.getByRole("tab", { name: "Roles" }));
    expect(screen.getByText("Failed to load roles. Please try again.")).toBeInTheDocument();
  });

  it("warns when only the first page of members or roles is shown", () => {
    apiState.queries.useGetAgentIdentityGroupMembers = {
      data: { members: [{ id: "tid-1" }], total: 140 },
    };
    apiState.queries.useListAgentIdentityRoles = { data: { roles: [], total: 120 } };
    renderPage();

    expect(screen.getByText(/Showing the first 100 of 140 members/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: "Roles" }));
    expect(screen.getByText(/Showing the first 100 of 120 roles/)).toBeInTheDocument();
  });

  it("is read-only without write access", () => {
    accessState.canWrite = false;
    apiState.queries.useGetAgentIdentityGroupMembers = { data: { members: [], total: 0 } };
    renderPage();

    expect(screen.getByText("Agents assigned to this group.")).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Search agents...")).not.toBeInTheDocument();
    expect(screen.getByText("No members yet")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: "Roles" }));
    expect(screen.getByText("Roles assigned to this group.")).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Search roles...")).not.toBeInTheDocument();
    expect(within(rowOf("admin")).queryByRole("button")).not.toBeInTheDocument();
  });

  it("returns to the groups list on Cancel", async () => {
    renderPage();

    await pickOption("Search agents...", /pricing label/);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(
      screen.getByText("at /org/acme/thunder-instances/groups?envName=dev"),
    ).toBeInTheDocument();
  });
});
