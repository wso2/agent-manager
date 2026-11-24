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
  createUser: vi.fn(),
  isPending: false,
  error: null as unknown,
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useCreateUser: () => ({
    mutateAsync: hoisted.createUser,
    isPending: hoisted.isPending,
    error: hoisted.error,
  }),
}));
vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
}));

import { UserAddPage } from "./UserAddPage";

const renderPage = () =>
  renderRoute(
    "/org/acme/settings/identities/users/add",
    "/org/:orgId/settings/identities/users/add",
    <UserAddPage />,
  );

const type = (label: string, value: string) =>
  fireEvent.change(screen.getByLabelText(label), { target: { value } });

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.isPending = false;
  hoisted.error = null;
});

describe("UserAddPage", () => {
  it("needs a username and password before create is enabled", () => {
    renderPage();
    const create = screen.getByRole("button", { name: "Create user" });

    expect(create).toBeDisabled();
    type("Username", "jane");
    expect(create).toBeDisabled();
    type("Password", "secret123");
    expect(create).toBeEnabled();
  });

  it("validates fields as the user types", () => {
    renderPage();

    type("Username", "jane doe");
    type("Password", "short");
    type("Email Address (optional)", "not-an-email");

    expect(screen.getByText("Username cannot contain spaces")).toBeInTheDocument();
    expect(screen.getByText("Password must be at least 8 characters")).toBeInTheDocument();
    expect(screen.getByText("Enter a valid email address")).toBeInTheDocument();
  });

  it("does not submit an invalid form", () => {
    renderPage();

    type("Username", "jane");
    type("Password", "short");
    fireEvent.click(screen.getByRole("button", { name: "Create user" }));

    expect(hoisted.createUser).not.toHaveBeenCalled();
    expect(screen.getAllByText("Password must be at least 8 characters").length).toBeGreaterThan(1);
  });

  it("creates the user with every filled attribute and returns to the list", async () => {
    hoisted.createUser.mockResolvedValue({ id: "u1" });
    renderPage();

    type("Username", "jane");
    type("Password", "secret123");
    type("First Name (optional)", "Jane");
    type("Last Name (optional)", "Doe");
    type("Email Address (optional)", "jane@example.com");
    fireEvent.click(screen.getByRole("button", { name: "Create user" }));

    expect(await screen.findByText("at /org/acme/settings/identities/users")).toBeInTheDocument();
    expect(hoisted.createUser).toHaveBeenCalledWith({
      params: { orgName: "acme" },
      body: {
        type: "engineer",
        attributes: {
          username: "jane",
          password: "secret123",
          given_name: "Jane",
          family_name: "Doe",
          email: "jane@example.com",
        },
      },
    });
  });

  it("leaves optional attributes out when they are empty", async () => {
    hoisted.createUser.mockResolvedValue({ id: "u1" });
    renderPage();

    type("Username", "jane");
    type("Password", "secret123");
    fireEvent.click(screen.getByRole("button", { name: "Create user" }));

    await screen.findByText(/^at /);
    expect(hoisted.createUser).toHaveBeenCalledWith({
      params: { orgName: "acme" },
      body: { type: "engineer", attributes: { username: "jane", password: "secret123" } },
    });
  });

  it("stays on the form when creation fails", async () => {
    hoisted.createUser.mockRejectedValue(new Error("taken"));
    renderPage();

    type("Username", "jane");
    type("Password", "secret123");
    fireEvent.click(screen.getByRole("button", { name: "Create user" }));

    await vi.waitFor(() => expect(hoisted.createUser).toHaveBeenCalled());
    expect(screen.queryByText(/^at /)).not.toBeInTheDocument();
  });

  it("shows the server error and locks the form while pending", () => {
    hoisted.error = new Error("Username taken");
    hoisted.isPending = true;
    renderPage();

    expect(screen.getByText("Username taken")).toBeInTheDocument();
    expect(screen.getByLabelText("Username")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
  });

  it("goes back to the users list on cancel", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByText("at /org/acme/settings/identities/users")).toBeInTheDocument();
  });
});
