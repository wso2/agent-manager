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
import { AgentDetailPage } from "./AgentDetailPage";

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

const ENV = { orgName: "acme", envName: "dev" };
const AGENT_ASSIGNMENT = { assignments: [{ id: "tid-1", type: "agent" }] };

const renderPage = () =>
  renderAt(
    <AgentDetailPage />,
    "/org/:orgId/thunder-instances/agents/:projectName/:agentName",
    "/org/acme/thunder-instances/agents/shop/support-bot?envName=dev",
  );

const pickOption = async (placeholder: string, option: string) => {
  fireEvent.mouseDown(screen.getByPlaceholderText(placeholder));
  fireEvent.click(await screen.findByRole("option", { name: option }));
};

const rowOf = (text: string) => screen.getByText(text).closest("tr") as HTMLElement;
const save = () => fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

beforeEach(() => {
  vi.clearAllMocks();
  resetApiState();
  resetAccessState();
  apiState.queries.useGetAgent = { data: { name: "support-bot", displayName: "Support Bot" } };
  apiState.queries.useGetProject = { data: { metadata: { name: "shop" }, spec: { displayName: "Shop" } } };
  apiState.queries.useGetAgentRoles = {
    data: { roles: [{ id: "r1", name: "admin", description: "Full access" }] },
  };
  apiState.queries.useGetAgentGroups = { data: { groups: [{ id: "g1", name: "on-call" }] } };
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
          agentName: "support-bot",
          projectName: "other",
          status: "completed",
          thunderAgentId: "tid-x",
        },
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

describe("AgentDetailPage", () => {
  it("shows a skeleton while the agent loads", () => {
    apiState.queries.useGetAgent = { isLoading: true };
    renderPage();

    expect(screen.getByText("support-bot")).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Roles" })).not.toBeInTheDocument();
  });

  it("shows the agent's roles for the environment", () => {
    renderPage();

    expect(screen.getByText("Support Bot")).toBeInTheDocument();
    expect(screen.getByText("Shop")).toBeInTheDocument();
    expect(screen.getByText("Roles assigned to this agent's identity in dev.")).toBeInTheDocument();
    expect(screen.getByText("Full access")).toBeInTheDocument();
    expect(apiState.calls.useGetAgentRoles?.[0]).toEqual([
      { orgName: "acme", projName: "shop", agentName: "support-bot" },
      { environment: "dev" },
    ]);
  });

  it("adds and removes roles using the agent's bound ID", async () => {
    renderPage();

    await pickOption("Search roles...", "viewer");
    fireEvent.click(within(rowOf("admin")).getByRole("button"));
    save();

    await screen.findByText("Agent updated successfully.");
    expect(mutation("useAddAgentIdentityRoleAssignees")).toHaveBeenCalledWith({
      params: { ...ENV, roleId: "r2" },
      body: AGENT_ASSIGNMENT,
    });
    expect(mutation("useRemoveAgentIdentityRoleAssignees")).toHaveBeenCalledWith({
      params: { ...ENV, roleId: "r1" },
      body: AGENT_ASSIGNMENT,
    });
    expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
  });

  it("joins and leaves groups", async () => {
    renderPage();
    fireEvent.click(screen.getByRole("tab", { name: "Groups" }));

    expect(screen.getByText("Groups this agent's identity belongs to in dev.")).toBeInTheDocument();
    await pickOption("Search groups...", "readers");
    fireEvent.click(within(rowOf("on-call")).getByRole("button"));
    save();

    await screen.findByText("Agent updated successfully.");
    expect(mutation("useAddAgentIdentityGroupMembers")).toHaveBeenCalledWith({
      params: { ...ENV, groupId: "g2" },
      body: { agentIds: ["tid-1"] },
    });
    expect(mutation("useRemoveAgentIdentityGroupMembers")).toHaveBeenCalledWith({
      params: { ...ENV, groupId: "g1" },
      body: { agentIds: ["tid-1"] },
    });
  });

  it("reports a failed save", async () => {
    mutation("useAddAgentIdentityGroupMembers").mockRejectedValue(new Error("nope"));
    renderPage();
    fireEvent.click(screen.getByRole("tab", { name: "Groups" }));

    await pickOption("Search groups...", "readers");
    save();

    expect(
      await screen.findByText("Failed to update this agent's roles/groups. Please try again."),
    ).toBeInTheDocument();
  });

  it("cannot assign anything until the agent has an identity binding", () => {
    apiState.queries.useListAgentIdentityAgents = {
      data: { agents: [{ agentName: "support-bot", projectName: "shop", status: "pending" }] },
    };
    renderPage();

    expect(
      screen.getByText(/This agent has no active identity binding in dev/),
    ).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Search roles...")).not.toBeInTheDocument();
    expect(within(rowOf("admin")).queryByRole("button")).not.toBeInTheDocument();
  });

  it("shows load errors, empty tabs and a truncated catalog warning", () => {
    apiState.queries.useGetAgentRoles = { error: new Error("boom"), data: { roles: [] } };
    apiState.queries.useGetAgentGroups = { data: { groups: [] } };
    apiState.queries.useListAgentIdentityRoles = { data: { roles: [], total: 101 } };
    renderPage();

    expect(
      screen.getByText("Failed to load this agent's roles/groups. Please try again."),
    ).toBeInTheDocument();
    expect(screen.getByText("No roles assigned to this agent.")).toBeInTheDocument();
    expect(screen.getByText(/Showing the first 100 of 101 in this/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: "Groups" }));
    expect(screen.getByText("This agent is not a member of any groups.")).toBeInTheDocument();
  });

  it("hides the picker while the role catalog loads", () => {
    apiState.queries.useListAgentIdentityRoles = { isLoading: true };
    renderPage();

    expect(screen.queryByPlaceholderText("Search roles...")).not.toBeInTheDocument();
    expect(screen.getByRole("progressbar")).toBeInTheDocument();
  });

  it("is read-only without write access", () => {
    accessState.canWrite = false;
    apiState.queries.useListAgentIdentityAgents = { data: { agents: [] } };
    renderPage();

    // No binding notice for read-only users, and nothing editable.
    expect(screen.queryByText(/no active identity binding/)).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Search roles...")).not.toBeInTheDocument();
  });

  it("returns to the agents list on Cancel", async () => {
    renderPage();

    await pickOption("Search roles...", "viewer");
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(
      screen.getByText("at /org/acme/thunder-instances/agents?envName=dev"),
    ).toBeInTheDocument();
  });
});
