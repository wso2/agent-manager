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

import { UserCreatePage } from "./UserCreatePage";

const renderPage = () =>
  renderRoute(
    "/org/acme/settings/identities/users/create",
    "/org/:orgId/settings/identities/users/create",
    <UserCreatePage />,
  );

const type = (label: string, value: string) =>
  fireEvent.change(screen.getByLabelText(label), { target: { value } });

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.isPending = false;
  hoisted.error = null;
});

describe("UserCreatePage", () => {
  it("enables create only once username and password are set", () => {
    renderPage();
    const create = screen.getByRole("button", { name: "Create user" });

    expect(create).toBeDisabled();
    type("Username", "jane");
    type("Password", "pw");
    expect(create).toBeEnabled();
  });

  it("creates the user with trimmed names and returns to the list", async () => {
    hoisted.createUser.mockResolvedValue({ id: "u1" });
    renderPage();

    type("Username", " jane ");
    type("Password", "pw");
    type("First Name (optional)", " Jane ");
    type("Last Name (optional)", " Doe ");
    fireEvent.click(screen.getByRole("button", { name: "Create user" }));

    expect(await screen.findByText("at /org/acme/settings/identities/users")).toBeInTheDocument();
    expect(hoisted.createUser).toHaveBeenCalledWith({
      params: { orgName: "acme" },
      body: {
        type: "engineer",
        attributes: { username: "jane", password: "pw", given_name: "Jane", family_name: "Doe" },
      },
    });
  });

  it("stays on the page when creation fails", async () => {
    hoisted.createUser.mockRejectedValue(new Error("taken"));
    renderPage();

    type("Username", "jane");
    type("Password", "pw");
    fireEvent.click(screen.getByRole("button", { name: "Create user" }));

    await vi.waitFor(() => expect(hoisted.createUser).toHaveBeenCalled());
    expect(screen.queryByText(/^at /)).not.toBeInTheDocument();
  });

  it("shows the server error and a pending label", () => {
    hoisted.error = new Error("Username taken");
    hoisted.isPending = true;
    renderPage();

    expect(screen.getByText("Username taken")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Creating..." })).toBeDisabled();
  });

  it("cancels back to the users list", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByText("at /org/acme/settings/identities/users")).toBeInTheDocument();
  });
});
