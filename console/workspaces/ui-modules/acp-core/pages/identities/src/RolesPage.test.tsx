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

import { fireEvent, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ThunderRole } from "@agent-management-platform/types";
import { renderRoute } from "./__test__/renderRoute";

interface Confirmation {
  title: string;
  description: string;
  onConfirm: () => void;
}

const hoisted = vi.hoisted(() => ({
  list: {
    data: undefined as { roles: ThunderRole[]; total: number } | undefined,
    isLoading: false,
    error: null as unknown,
  },
  listArgs: [] as unknown[][],
  deleteRole: vi.fn(),
  addConfirmation: vi.fn(),
  flagOn: true,
  canWrite: true,
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useListRoles: (...args: unknown[]) => {
    hoisted.listArgs.push(args);
    return hoisted.list;
  },
  useDeleteRole: () => ({ mutateAsync: hoisted.deleteRole }),
}));
vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useConfirmationDialog: () => ({ addConfirmation: hoisted.addConfirmation }),
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureFlag: () => hoisted.flagOn,
  useResourceAccess: () => ({ canRead: true, canWrite: hoisted.canWrite }),
}));

import { RolesPage } from "./RolesPage";

const roles: ThunderRole[] = [
  { id: "r1", name: "Admin", description: "Full control", isReadOnly: true },
  { id: "r2", name: "Viewer", description: "Read only access" },
];

const renderPage = () =>
  renderRoute(
    "/org/acme/settings/identities/roles",
    "/org/:orgId/settings/identities/roles",
    <RolesPage />,
  );

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.list = { data: undefined, isLoading: false, error: null };
  hoisted.listArgs = [];
  hoisted.flagOn = true;
  hoisted.canWrite = true;
});

describe("RolesPage", () => {
  it("lists roles with their descriptions", () => {
    hoisted.list.data = { roles, total: 2 };
    renderPage();

    expect(screen.getByText("Admin")).toBeInTheDocument();
    expect(screen.getByText("Read only access")).toBeInTheDocument();
    expect(hoisted.listArgs[0]).toEqual([{ orgName: "acme" }, { offset: 0, limit: 10 }]);
  });

  it("only offers delete on roles that are not read-only", () => {
    hoisted.list.data = { roles, total: 2 };
    renderPage();

    expect(screen.getAllByRole("button", { name: "Delete role" })).toHaveLength(1);
  });

  it("deletes a role after confirmation", () => {
    hoisted.list.data = { roles, total: 2 };
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Delete role" }));
    const confirmation = hoisted.addConfirmation.mock.calls[0][0] as Confirmation;
    expect(confirmation.title).toBe("Delete Role");
    expect(confirmation.description).toContain('"Viewer"');
    confirmation.onConfirm();
    expect(hoisted.deleteRole).toHaveBeenCalledWith({ orgName: "acme", roleId: "r2" });
  });

  it("opens a role when the row is clicked", () => {
    hoisted.list.data = { roles, total: 2 };
    renderPage();

    fireEvent.click(screen.getByText("Viewer"));
    expect(screen.getByText("at /org/acme/settings/identities/roles/r2")).toBeInTheDocument();
  });

  it("filters by name or description", () => {
    hoisted.list.data = { roles, total: 2 };
    renderPage();

    fireEvent.change(screen.getByPlaceholderText("Search roles"), {
      target: { value: "full" },
    });
    expect(screen.getByText("Admin")).toBeInTheDocument();
    expect(screen.queryByText("Viewer")).not.toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("Search roles"), {
      target: { value: "nothing" },
    });
    expect(screen.getByText("No roles found")).toBeInTheDocument();
  });

  it("leads to the create page from the empty state", () => {
    hoisted.list.data = { roles: [], total: 0 };
    renderPage();

    expect(screen.getByText("No roles yet")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Create role" }));
    expect(screen.getByText("at /org/acme/settings/identities/roles/create")).toBeInTheDocument();
  });

  it("disables create while user management is off", () => {
    hoisted.flagOn = false;
    hoisted.list.data = { roles, total: 2 };
    renderPage();

    expect(screen.getByRole("button", { name: "Create role" })).toBeDisabled();
  });

  it("hides create and delete without role write access", () => {
    hoisted.canWrite = false;
    hoisted.list.data = { roles, total: 2 };
    renderPage();

    expect(screen.queryByRole("button", { name: "Create role" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete role" })).not.toBeInTheDocument();
  });

  it("shows a load error", () => {
    hoisted.list.error = new Error("boom");
    renderPage();

    expect(screen.getByText("Failed to load roles")).toBeInTheDocument();
  });

  it("does not show the empty state while loading", () => {
    hoisted.list.isLoading = true;
    renderPage();

    expect(screen.queryByText("No roles yet")).not.toBeInTheDocument();
  });

  it("changes the page size and restarts from the first page", () => {
    hoisted.list.data = {
      roles: Array.from({ length: 10 }, (_, i) => ({ id: `r${i}`, name: `role${i}` })),
      total: 30,
    };
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: /next page/i }));
    expect(hoisted.listArgs[hoisted.listArgs.length - 1]).toEqual([{ orgName: "acme" }, { offset: 10, limit: 10 }]);

    fireEvent.mouseDown(screen.getByRole("combobox"));
    fireEvent.click(screen.getByRole("option", { name: "25" }));
    expect(hoisted.listArgs[hoisted.listArgs.length - 1]).toEqual([{ orgName: "acme" }, { offset: 0, limit: 25 }]);
  });
});
