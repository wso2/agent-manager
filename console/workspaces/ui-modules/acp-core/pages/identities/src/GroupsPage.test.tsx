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
import type { ThunderGroup } from "@agent-management-platform/types";
import { renderRoute } from "./__test__/renderRoute";

interface Confirmation {
  title: string;
  description: string;
  onConfirm: () => void;
}

const hoisted = vi.hoisted(() => ({
  list: {
    data: undefined as { groups: ThunderGroup[]; total: number } | undefined,
    isLoading: false,
    error: null as unknown,
  },
  listArgs: [] as unknown[][],
  deleteGroup: vi.fn(),
  addConfirmation: vi.fn(),
  flagOn: true,
  canWrite: true,
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useListGroups: (...args: unknown[]) => {
    hoisted.listArgs.push(args);
    return hoisted.list;
  },
  useDeleteGroup: () => ({ mutateAsync: hoisted.deleteGroup }),
}));
vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useConfirmationDialog: () => ({ addConfirmation: hoisted.addConfirmation }),
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureFlag: () => hoisted.flagOn,
  useResourceAccess: () => ({ canRead: true, canWrite: hoisted.canWrite }),
}));

import { GroupsPage } from "./GroupsPage";

const groups: ThunderGroup[] = [
  { id: "g1", name: "Engineering", description: "Builders" },
  { id: "g2", name: "Sales" },
];

const renderPage = () =>
  renderRoute(
    "/org/acme/settings/identities/groups",
    "/org/:orgId/settings/identities/groups",
    <GroupsPage />,
  );

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.list = { data: undefined, isLoading: false, error: null };
  hoisted.listArgs = [];
  hoisted.flagOn = true;
  hoisted.canWrite = true;
});

describe("GroupsPage", () => {
  it("lists groups and deletes one after confirmation", () => {
    hoisted.list.data = { groups, total: 2 };
    renderPage();

    expect(screen.getByText("Engineering")).toBeInTheDocument();
    expect(screen.getByText("Builders")).toBeInTheDocument();

    fireEvent.click(screen.getAllByRole("button", { name: "Delete group" })[1]);
    const confirmation = hoisted.addConfirmation.mock.calls[0][0] as Confirmation;
    expect(confirmation.title).toBe("Delete Group");
    expect(confirmation.description).toContain('"Sales"');
    confirmation.onConfirm();
    expect(hoisted.deleteGroup).toHaveBeenCalledWith({ orgName: "acme", groupId: "g2" });
  });

  it("opens a group when the row is clicked", () => {
    hoisted.list.data = { groups, total: 2 };
    renderPage();

    fireEvent.click(screen.getByText("Engineering"));
    expect(screen.getByText("at /org/acme/settings/identities/groups/g1")).toBeInTheDocument();
  });

  it("filters groups by name or description", () => {
    hoisted.list.data = { groups, total: 2 };
    renderPage();

    fireEvent.change(screen.getByPlaceholderText("Search groups"), {
      target: { value: "build" },
    });
    expect(screen.getByText("Engineering")).toBeInTheDocument();
    expect(screen.queryByText("Sales")).not.toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("Search groups"), {
      target: { value: "xyz" },
    });
    expect(screen.getByText('No groups match "xyz". Try a different search term.')).toBeInTheDocument();
  });

  it("leads to the create page from the toolbar", () => {
    hoisted.list.data = { groups, total: 2 };
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Create group" }));
    expect(screen.getByText("at /org/acme/settings/identities/groups/create")).toBeInTheDocument();
  });

  it("shows the empty state with a create action", () => {
    hoisted.list.data = { groups: [], total: 0 };
    renderPage();

    expect(screen.getByText("No groups yet")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create group" })).toBeEnabled();
  });

  it("hides create and delete without group write access", () => {
    hoisted.canWrite = false;
    hoisted.list.data = { groups, total: 2 };
    renderPage();

    expect(screen.queryByRole("button", { name: "Create group" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete group" })).not.toBeInTheDocument();
  });

  it("shows a load error", () => {
    hoisted.list.error = new Error("boom");
    renderPage();

    expect(screen.getByText("Failed to load groups")).toBeInTheDocument();
  });

  it("keeps the table while loading", () => {
    hoisted.list.isLoading = true;
    renderPage();

    expect(screen.queryByText("No groups yet")).not.toBeInTheDocument();
    expect(screen.getByText("Name")).toBeInTheDocument();
  });

  it("pages forward with the right offset", () => {
    hoisted.list.data = {
      groups: Array.from({ length: 10 }, (_, i) => ({ id: `g${i}`, name: `group${i}` })),
      total: 15,
    };
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: /next page/i }));
    expect(hoisted.listArgs[hoisted.listArgs.length - 1]).toEqual([{ orgName: "acme" }, { offset: 10, limit: 10 }]);
  });
});
