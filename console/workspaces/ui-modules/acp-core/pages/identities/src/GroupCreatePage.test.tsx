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
import { renderRoute } from "./__test__/renderRoute";

const hoisted = vi.hoisted(() => ({
  createGroup: vi.fn(),
  isPending: false,
  error: null as unknown,
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useCreateGroup: () => ({
    mutateAsync: hoisted.createGroup,
    isPending: hoisted.isPending,
    error: hoisted.error,
  }),
}));
vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
}));

import { GroupCreatePage } from "./GroupCreatePage";

const renderPage = () =>
  renderRoute(
    "/org/acme/settings/identities/groups/create",
    "/org/:orgId/settings/identities/groups/create",
    <GroupCreatePage />,
  );

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.isPending = false;
  hoisted.error = null;
});

describe("GroupCreatePage", () => {
  it("keeps create disabled until a name is entered", () => {
    renderPage();

    expect(screen.getByRole("button", { name: "Create group" })).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "platform" } });
    expect(screen.getByRole("button", { name: "Create group" })).toBeEnabled();
  });

  it("says a name is required once it is cleared", () => {
    renderPage();

    const name = screen.getByLabelText("Name");
    fireEvent.change(name, { target: { value: "a" } });
    fireEvent.change(name, { target: { value: " " } });
    expect(screen.getByText("Name is required")).toBeInTheDocument();
  });

  it("creates the group with trimmed values and opens it", async () => {
    hoisted.createGroup.mockResolvedValue({ id: "group-9" });
    renderPage();

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "  platform " } });
    fireEvent.change(screen.getByLabelText("Description (optional)"), {
      target: { value: " runs infra " },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create group" }));

    expect(
      await screen.findByText("at /org/acme/settings/identities/groups/group-9"),
    ).toBeInTheDocument();
    expect(hoisted.createGroup).toHaveBeenCalledWith({
      params: { orgName: "acme" },
      body: { name: "platform", description: "runs infra" },
    });
  });

  it("sends no description when it is blank", async () => {
    hoisted.createGroup.mockResolvedValue({ id: "group-1" });
    renderPage();

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "platform" } });
    fireEvent.click(screen.getByRole("button", { name: "Create group" }));

    await screen.findByText(/^at /);
    expect(hoisted.createGroup).toHaveBeenCalledWith({
      params: { orgName: "acme" },
      body: { name: "platform", description: undefined },
    });
  });

  it("lists validation errors and does not submit an over-long name", () => {
    renderPage();

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "x".repeat(51) } });
    fireEvent.click(screen.getByRole("button", { name: "Create group" }));

    expect(screen.getAllByText("Name must be at most 50 characters").length).toBeGreaterThan(0);
    expect(hoisted.createGroup).not.toHaveBeenCalled();
  });

  it("stays on the page when creation fails", async () => {
    hoisted.createGroup.mockRejectedValue(new Error("conflict"));
    renderPage();

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "platform" } });
    fireEvent.click(screen.getByRole("button", { name: "Create group" }));

    await vi.waitFor(() => expect(hoisted.createGroup).toHaveBeenCalled());
    expect(screen.queryByText(/^at /)).not.toBeInTheDocument();
  });

  it("shows the server error message", () => {
    hoisted.error = new Error("Group already exists");
    renderPage();

    expect(screen.getByText("Group already exists")).toBeInTheDocument();
  });

  it("goes back to the groups list on cancel", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByText("at /org/acme/settings/identities/groups")).toBeInTheDocument();
  });

  it("disables create while the request is in flight", () => {
    hoisted.isPending = true;
    renderPage();

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "platform" } });
    expect(screen.getByRole("button", { name: "Create group" })).toBeDisabled();
  });
});
