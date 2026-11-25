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
  createRole: vi.fn(),
  isPending: false,
  error: null as unknown,
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useCreateRole: () => ({
    mutateAsync: hoisted.createRole,
    isPending: hoisted.isPending,
    error: hoisted.error,
  }),
}));
vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
}));

import { RoleCreatePage } from "./RoleCreatePage";

const renderPage = () =>
  renderRoute(
    "/org/acme/settings/identities/roles/create",
    "/org/:orgId/settings/identities/roles/create",
    <RoleCreatePage />,
  );

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.isPending = false;
  hoisted.error = null;
});

describe("RoleCreatePage", () => {
  it("keeps create disabled until a name is entered", () => {
    renderPage();

    expect(screen.getByRole("button", { name: "Create role" })).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "auditor" } });
    expect(screen.getByRole("button", { name: "Create role" })).toBeEnabled();
  });

  it("says a name is required once it is cleared", () => {
    renderPage();

    const name = screen.getByLabelText("Name");
    fireEvent.change(name, { target: { value: "a" } });
    fireEvent.change(name, { target: { value: " " } });
    expect(screen.getByText("Name is required")).toBeInTheDocument();
  });

  it("creates the role with trimmed values and opens it", async () => {
    hoisted.createRole.mockResolvedValue({ id: "role-9" });
    renderPage();

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "  auditor " } });
    fireEvent.change(screen.getByLabelText("Description (optional)"), {
      target: { value: " reads logs " },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create role" }));

    expect(
      await screen.findByText("at /org/acme/settings/identities/roles/role-9"),
    ).toBeInTheDocument();
    expect(hoisted.createRole).toHaveBeenCalledWith({
      params: { orgName: "acme" },
      body: { name: "auditor", description: "reads logs" },
    });
  });

  it("sends no description when it is blank", async () => {
    hoisted.createRole.mockResolvedValue({ id: "role-1" });
    renderPage();

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "auditor" } });
    fireEvent.click(screen.getByRole("button", { name: "Create role" }));

    await screen.findByText(/^at /);
    expect(hoisted.createRole).toHaveBeenCalledWith({
      params: { orgName: "acme" },
      body: { name: "auditor", description: undefined },
    });
  });

  it("lists validation errors and does not submit an over-long name", () => {
    renderPage();

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "x".repeat(51) } });
    fireEvent.click(screen.getByRole("button", { name: "Create role" }));

    expect(screen.getAllByText("Name must be at most 50 characters").length).toBeGreaterThan(0);
    expect(hoisted.createRole).not.toHaveBeenCalled();
  });

  it("stays on the page when creation fails", async () => {
    hoisted.createRole.mockRejectedValue(new Error("conflict"));
    renderPage();

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "auditor" } });
    fireEvent.click(screen.getByRole("button", { name: "Create role" }));

    await vi.waitFor(() => expect(hoisted.createRole).toHaveBeenCalled());
    expect(screen.queryByText(/^at /)).not.toBeInTheDocument();
  });

  it("shows the server error message", () => {
    hoisted.error = new Error("Role already exists");
    renderPage();

    expect(screen.getByText("Role already exists")).toBeInTheDocument();
  });

  it("goes back to the roles list on cancel", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByText("at /org/acme/settings/identities/roles")).toBeInTheDocument();
  });

  it("disables create while the request is in flight", () => {
    hoisted.isPending = true;
    renderPage();

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "auditor" } });
    expect(screen.getByRole("button", { name: "Create role" })).toBeDisabled();
  });
});
