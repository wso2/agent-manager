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

import { fireEvent, screen, waitFor } from "@testing-library/react";
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
import { AgentIdentityEnvironmentProvider } from "../../context/AgentIdentityEnvironmentContext";
import { GroupsPage } from "./GroupsPage";

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

const ENVS = [
  { name: "dev", displayName: "Development" },
  { name: "prod", displayName: "Production" },
];

const group = (id: string, name: string, description?: string) => ({ id, name, description });

const renderPage = (url = "/org/acme/thunder-instances/groups?envName=dev") =>
  renderAt(
    <AgentIdentityEnvironmentProvider>
      <GroupsPage createAction={<button type="button">Create group</button>} />
    </AgentIdentityEnvironmentProvider>,
    "/org/:orgId/thunder-instances/groups",
    url,
  );

beforeEach(() => {
  vi.clearAllMocks();
  resetApiState();
  resetAccessState();
  apiState.queries.useListEnvironments = { data: ENVS };
});

describe("GroupsPage", () => {
  it("lists the environment's groups with their descriptions", () => {
    apiState.queries.useListAgentIdentityGroups = {
      data: {
        groups: [group("g1", "on-call", "Pager rotation"), group("g2", "readers")],
        total: 2,
      },
    };
    renderPage();

    expect(screen.getByText("on-call")).toBeInTheDocument();
    expect(screen.getByText("Pager rotation")).toBeInTheDocument();
    expect(screen.getByText("readers")).toBeInTheDocument();
    expect(apiState.calls.useListAgentIdentityGroups?.[0]).toEqual([
      { orgName: "acme", envName: "dev" },
      { offset: 0, limit: 10 },
    ]);
    // Several environments: one tab each, plus the create action in the tab bar.
    expect(screen.getByRole("tab", { name: "Development" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Production" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create group" })).toBeInTheDocument();
  });

  it("switches environment from the tab bar", () => {
    apiState.queries.useListAgentIdentityGroups = {
      data: { groups: [group("g1", "on-call")], total: 1 },
    };
    renderPage();

    fireEvent.click(screen.getByRole("tab", { name: "Production" }));

    expect(screen.getByRole("tab", { name: "Production" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(last(apiState.calls.useListAgentIdentityGroups)?.[0]).toEqual({
      orgName: "acme",
      envName: "prod",
    });
  });

  it("shows the empty state with the create action when there are no groups", () => {
    apiState.queries.useListAgentIdentityGroups = { data: { groups: [], total: 0 } };
    renderPage();

    expect(screen.getByText("No groups yet")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create group" })).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Search groups")).not.toBeInTheDocument();
  });

  it("filters by name or description and explains an empty search", () => {
    apiState.queries.useListAgentIdentityGroups = {
      data: {
        groups: [group("g1", "on-call", "Pager rotation"), group("g2", "readers")],
        total: 2,
      },
    };
    renderPage();

    fireEvent.change(screen.getByPlaceholderText("Search groups"), { target: { value: "PAGER" } });
    expect(screen.getByText("on-call")).toBeInTheDocument();
    expect(screen.queryByText("readers")).not.toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("Search groups"), { target: { value: "zzz" } });
    expect(screen.getByText("No groups found")).toBeInTheDocument();
    expect(
      screen.getByText('No groups match "zzz". Try a different search term.'),
    ).toBeInTheDocument();
  });

  it("opens a group's page, keeping the environment in the URL", () => {
    apiState.queries.useListAgentIdentityGroups = {
      data: { groups: [group("g1", "on-call")], total: 1 },
    };
    renderPage();

    fireEvent.click(screen.getByText("on-call"));

    expect(
      screen.getByText("at /org/acme/thunder-instances/groups/g1?envName=dev"),
    ).toBeInTheDocument();
  });

  it("asks for confirmation before deleting and deletes the chosen group", async () => {
    apiState.queries.useListAgentIdentityGroups = {
      data: { groups: [group("g1", "on-call")], total: 1 },
    };
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Delete group" }));

    expect(sharedState.addConfirmation).toHaveBeenCalledTimes(1);
    const confirmation = sharedState.addConfirmation.mock.calls[0][0];
    expect(confirmation.title).toBe("Delete Group");
    expect(confirmation.description).toBe(
      'Are you sure you want to delete "on-call"? This action cannot be undone.',
    );
    // The row click must not also navigate.
    expect(screen.queryByText(/^at /)).not.toBeInTheDocument();

    await confirmation.onConfirm();
    expect(mutation("useDeleteAgentIdentityGroup")).toHaveBeenCalledWith({
      orgName: "acme",
      envName: "dev",
      groupId: "g1",
    });
  });

  it("hides the delete action from read-only users", () => {
    accessState.canWrite = false;
    apiState.queries.useListAgentIdentityGroups = {
      data: { groups: [group("g1", "on-call")], total: 1 },
    };
    renderPage();

    expect(screen.getByText("on-call")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete group" })).not.toBeInTheDocument();
  });

  it("reports a load failure", () => {
    apiState.queries.useListAgentIdentityGroups = { error: new Error("boom") };
    renderPage();

    expect(screen.getByText("Failed to load groups")).toBeInTheDocument();
  });

  it("does not show the empty state while loading", () => {
    apiState.queries.useListAgentIdentityGroups = { isLoading: true };
    renderPage();

    expect(screen.queryByText("No groups yet")).not.toBeInTheDocument();
    expect(screen.getByText("Name")).toBeInTheDocument();
  });

  it("pages through large lists", async () => {
    apiState.queries.useListAgentIdentityGroups = {
      data: { groups: [group("g1", "on-call")], total: 30 },
    };
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: /next page/i }));
    await waitFor(() =>
      expect(last(apiState.calls.useListAgentIdentityGroups)?.[1]).toEqual({
        offset: 10,
        limit: 10,
      }),
    );
  });

  it("jumps back to the last page when the current page comes back empty", async () => {
    apiState.queries.useListAgentIdentityGroups = { data: { groups: [], total: 25 } };
    renderPage();

    await waitFor(() =>
      expect(last(apiState.calls.useListAgentIdentityGroups)?.[1]).toEqual({
        offset: 20,
        limit: 10,
      }),
    );
  });
});
