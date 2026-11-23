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

import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  apiState,
  mutation,
  renderAt,
  resetApiState,
  sharedState,
  last,
} from "../../__test__/mocks";
import { RoleCreatePage } from "./RoleCreatePage";

vi.mock(
  "@agent-management-platform/api-client",
  async () => (await import("../../__test__/mocks")).apiClientMock,
);
vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  ...(await import("../../__test__/mocks")).sharedComponentOverrides,
}));

// Oxygen UI pages are slow to render under a loaded CI runner.
vi.setConfig({ testTimeout: 60_000 });

const renderPage = () =>
  renderAt(
    <RoleCreatePage />,
    "/org/:orgId/thunder-instances/roles/create",
    "/org/acme/thunder-instances/roles/create?envName=dev",
  );

const nameInput = () => screen.getByPlaceholderText("agent-admin");
const createButton = () => screen.getByRole("button", { name: "Create role" });

// The scope picker stays open after a pick (disableCloseOnSelect), so only
// open it when it is closed.
const pickScope = async (scope: string) => {
  if (!screen.queryByRole("listbox")) {
    fireEvent.mouseDown(screen.getByPlaceholderText("Search scopes..."));
  }
  fireEvent.click(await screen.findByRole("option", { name: scope }));
};

beforeEach(() => {
  vi.clearAllMocks();
  resetApiState();
  sharedState.guardDirty = [];
  apiState.queries.useListAgentIdentityScopes = {
    data: {
      scopes: [{ scope: "orders:read" }, { scope: "orders:write", description: "Write orders" }],
    },
  };
});

describe("RoleCreatePage", () => {
  it("creates the role with the picked scopes and opens it", async () => {
    mutation("useCreateAgentIdentityRole").mockResolvedValue({ id: "r-new" });
    renderPage();

    fireEvent.change(nameInput(), { target: { value: " order-admin " } });
    fireEvent.change(screen.getByPlaceholderText("Describe the role's purpose and permissions"), {
      target: { value: "Manages orders" },
    });
    await pickScope("orders:read");
    await pickScope("orders:write");
    expect(screen.getByRole("button", { name: "orders:read" })).toBeInTheDocument();

    fireEvent.click(createButton());

    await waitFor(() =>
      expect(mutation("useCreateAgentIdentityRole")).toHaveBeenCalledWith({
        params: { orgName: "acme", envName: "dev" },
        body: {
          name: "order-admin",
          description: "Manages orders",
          scopes: ["orders:read", "orders:write"],
        },
      }),
    );
    expect(
      await screen.findByText("at /org/acme/thunder-instances/roles/r-new?envName=dev"),
    ).toBeInTheDocument();
    expect(apiState.calls.useListAgentIdentityScopes?.[0]).toEqual([
      { orgName: "acme", envName: "dev" },
    ]);
  });

  it("guards leaving once a scope is picked, even without a name", async () => {
    renderPage();
    expect(last(sharedState.guardDirty)).toBe(false);

    await pickScope("orders:read");

    expect(last(sharedState.guardDirty)).toBe(true);
    expect(createButton()).toBeDisabled();
  });

  it("lets a picked scope be removed again", async () => {
    mutation("useCreateAgentIdentityRole").mockResolvedValue({ id: "r-new" });
    renderPage();
    await pickScope("orders:read");

    const chip = screen.getByRole("button", { name: "orders:read" });
    fireEvent.click(within(chip).getByTestId("CancelIcon"));
    fireEvent.change(nameInput(), { target: { value: "reader" } });
    fireEvent.click(createButton());

    await waitFor(() =>
      expect(mutation("useCreateAgentIdentityRole")).toHaveBeenCalledWith(
        expect.objectContaining({ body: { name: "reader", description: undefined, scopes: [] } }),
      ),
    );
  });

  it("explains an empty scope catalog", async () => {
    apiState.queries.useListAgentIdentityScopes = { data: { scopes: [] } };
    renderPage();

    fireEvent.mouseDown(screen.getByPlaceholderText("Search scopes..."));

    expect(await screen.findByText("No scopes in the catalog")).toBeInTheDocument();
  });

  it("shows a validation error for an over-long description and does not submit", async () => {
    renderPage();

    fireEvent.change(nameInput(), { target: { value: "reader" } });
    fireEvent.change(screen.getByPlaceholderText("Describe the role's purpose and permissions"), {
      target: { value: "d".repeat(2001) },
    });
    expect(screen.getAllByText(/Description must be at most/).length).toBeGreaterThan(0);

    fireEvent.click(createButton());
    await waitFor(() => expect(screen.getAllByText(/Description must be at most/)).toHaveLength(2));
    expect(mutation("useCreateAgentIdentityRole")).not.toHaveBeenCalled();
  });

  it("shows the API error when creation fails", async () => {
    apiState.mutations.useCreateAgentIdentityRole.error = new Error("Role name taken");
    mutation("useCreateAgentIdentityRole").mockRejectedValue(new Error("Role name taken"));
    renderPage();

    expect(screen.getByText("Role name taken")).toBeInTheDocument();
    fireEvent.change(nameInput(), { target: { value: "reader" } });
    fireEvent.click(createButton());
    await waitFor(() => expect(mutation("useCreateAgentIdentityRole")).toHaveBeenCalled());
    expect(screen.queryByText(/^at /)).not.toBeInTheDocument();
  });

  it("goes back to the roles list on Cancel", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(
      screen.getByText("at /org/acme/thunder-instances/roles?envName=dev"),
    ).toBeInTheDocument();
  });
});
