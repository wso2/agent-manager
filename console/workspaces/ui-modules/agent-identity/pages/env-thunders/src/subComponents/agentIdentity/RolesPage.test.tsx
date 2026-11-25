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
import { RolesPage } from "./RolesPage";

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

const role = (
  id: string,
  name: string,
  extra: { description?: string; isReadOnly?: boolean } = {},
) => ({
  id,
  name,
  ...extra,
});

const renderPage = (createAction?: React.ReactNode) =>
  renderAt(
    <AgentIdentityEnvironmentProvider>
      <RolesPage createAction={createAction} />
    </AgentIdentityEnvironmentProvider>,
    "/org/:orgId/thunder-instances/roles",
    "/org/acme/thunder-instances/roles?envName=dev",
  );

beforeEach(() => {
  vi.clearAllMocks();
  resetApiState();
  resetAccessState();
  // A single environment: no tab bar.
  apiState.queries.useListEnvironments = { data: [{ name: "dev" }] };
});

describe("RolesPage", () => {
  it("lists roles and asks the API for the environment's first page", () => {
    apiState.queries.useListAgentIdentityRoles = {
      data: {
        roles: [role("r1", "admin", { description: "Full access" }), role("r2", "viewer")],
        total: 2,
      },
    };
    renderPage();

    expect(screen.getByText("admin")).toBeInTheDocument();
    expect(screen.getByText("Full access")).toBeInTheDocument();
    expect(screen.getByText("viewer")).toBeInTheDocument();
    expect(screen.queryByRole("tab")).not.toBeInTheDocument();
    expect(apiState.calls.useListAgentIdentityRoles?.[0]).toEqual([
      { orgName: "acme", envName: "dev" },
      { offset: 0, limit: 10 },
    ]);
  });

  it("offers the create action in the empty state", () => {
    apiState.queries.useListAgentIdentityRoles = { data: { roles: [], total: 0 } };
    renderPage(<button type="button">Create role</button>);

    expect(screen.getByText("No roles yet")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create role" })).toBeInTheDocument();
  });

  it("searches by name and description", () => {
    apiState.queries.useListAgentIdentityRoles = {
      data: {
        roles: [role("r1", "admin", { description: "Full access" }), role("r2", "viewer")],
        total: 2,
      },
    };
    renderPage();

    fireEvent.change(screen.getByPlaceholderText("Search roles"), { target: { value: "full" } });
    expect(screen.getByText("admin")).toBeInTheDocument();
    expect(screen.queryByText("viewer")).not.toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("Search roles"), { target: { value: "nobody" } });
    expect(screen.getByText("No roles found")).toBeInTheDocument();
  });

  it("opens a role's page", () => {
    apiState.queries.useListAgentIdentityRoles = {
      data: { roles: [role("r1", "admin")], total: 1 },
    };
    renderPage();

    fireEvent.click(screen.getByText("admin"));

    expect(
      screen.getByText("at /org/acme/thunder-instances/roles/r1?envName=dev"),
    ).toBeInTheDocument();
  });

  it("deletes a role after confirmation, but never offers delete on a read-only role", async () => {
    apiState.queries.useListAgentIdentityRoles = {
      data: { roles: [role("r1", "admin"), role("r2", "system", { isReadOnly: true })], total: 2 },
    };
    renderPage();

    const deleteButtons = screen.getAllByRole("button", { name: "Delete role" });
    expect(deleteButtons).toHaveLength(1);
    fireEvent.click(deleteButtons[0]);

    const confirmation = sharedState.addConfirmation.mock.calls[0][0];
    expect(confirmation.title).toBe("Delete Role");
    expect(confirmation.description).toContain('"admin"');
    await confirmation.onConfirm();
    expect(mutation("useDeleteAgentIdentityRole")).toHaveBeenCalledWith({
      orgName: "acme",
      envName: "dev",
      roleId: "r1",
    });
  });

  it("hides delete from read-only users", () => {
    accessState.canWrite = false;
    apiState.queries.useListAgentIdentityRoles = {
      data: { roles: [role("r1", "admin")], total: 1 },
    };
    renderPage();

    expect(screen.queryByRole("button", { name: "Delete role" })).not.toBeInTheDocument();
  });

  it("reports a load failure", () => {
    apiState.queries.useListAgentIdentityRoles = { error: new Error("boom") };
    renderPage();

    expect(screen.getByText("Failed to load roles")).toBeInTheDocument();
  });

  it("changes the page size from the paginator", async () => {
    apiState.queries.useListAgentIdentityRoles = {
      data: { roles: [role("r1", "admin")], total: 40 },
    };
    renderPage();

    fireEvent.mouseDown(screen.getByRole("combobox"));
    fireEvent.click(await screen.findByRole("option", { name: "25" }));

    await waitFor(() =>
      expect(last(apiState.calls.useListAgentIdentityRoles)?.[1]).toEqual({ offset: 0, limit: 25 }),
    );
  });

  it("jumps back to the last page when the current page comes back empty", async () => {
    apiState.queries.useListAgentIdentityRoles = { data: { roles: [], total: 15 } };
    renderPage();

    await waitFor(() =>
      expect(last(apiState.calls.useListAgentIdentityRoles)?.[1]).toEqual({
        offset: 10,
        limit: 10,
      }),
    );
  });
});
