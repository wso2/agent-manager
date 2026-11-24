/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Environment } from "@agent-management-platform/types";

vi.setConfig({ testTimeout: 30000 });

const hoisted = vi.hoisted(() => ({
  createScope: { mutateAsync: vi.fn(), isPending: false, reset: vi.fn() },
  updateRole: { mutateAsync: vi.fn(), isPending: false },
  listRoles: vi.fn(),
  markCompleted: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  listAgentIdentityRoles: hoisted.listRoles,
  useCreateMCPProxyScope: () => hoisted.createScope,
  useUpdateAgentIdentityRole: () => hoisted.updateRole,
  useDialogAnalytics: () => ({ markCompleted: hoisted.markCompleted }),
  useValidationErrorTracking: () => () => {},
}));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: async () => "token" }),
}));
vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
  useConfirmIfUnsaved: () => (fn: () => void) => fn(),
}));

import { CreateScopeDrawer } from "./CreateScopeDrawer";

const onClose = vi.fn();
const devEnv = { id: "env-dev", name: "dev", displayName: "Development" } as Environment;

const renderDrawer = (environments: Environment[] = [devEnv]) =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <CreateScopeDrawer
          open
          onClose={onClose}
          orgName="acme"
          proxyId="github"
          environments={environments}
          tools={["search", "delete_repo"]}
          blockedTools={new Set(["delete_repo"])}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );

const nameField = () => screen.getByPlaceholderText("e.g., read-only");

beforeEach(() => {
  onClose.mockReset();
  hoisted.createScope.mutateAsync.mockReset().mockResolvedValue({
    action: "repo.read",
    scope: "urn:repo.read",
    tools: ["search"],
  });
  hoisted.updateRole.mutateAsync.mockReset().mockResolvedValue({});
  hoisted.markCompleted.mockReset();
  hoisted.listRoles.mockReset().mockResolvedValue({
    roles: [
      { id: "r1", name: "Reader", permissions: [{ permissions: ["existing"] }] },
      { id: "r2", name: "Default" },
      { id: "r3", name: "Administrator", isReadOnly: true },
    ],
  });
});

describe("CreateScopeDrawer", () => {
  it("validates the scope name", () => {
    renderDrawer();

    const create = screen.getByRole("button", { name: "Create scope" });
    expect(create).toBeDisabled();

    fireEvent.change(nameField(), { target: { value: "read only!" } });
    expect(
      screen.getByText(
        "Only letters, numbers, '.', '_' and '-' are allowed (max 100 characters)",
      ),
    ).toBeInTheDocument();
    expect(create).toBeDisabled();

    fireEvent.change(nameField(), { target: { value: "read-only" } });
    expect(create).toBeEnabled();
  });

  it("lists blocked tools as unselectable", () => {
    renderDrawer();

    fireEvent.mouseDown(screen.getByPlaceholderText("Search tools..."));
    const listbox = screen.getByRole("listbox");
    expect(within(listbox).getByText("Blocked by Manage Tools")).toBeInTheDocument();
    expect(
      within(listbox).getByText("delete_repo").closest("li"),
    ).toHaveAttribute("aria-disabled", "true");
  });

  it("creates the scope and grants it to the chosen roles", async () => {
    renderDrawer();

    fireEvent.change(nameField(), { target: { value: " repo.read " } });
    // The description is the drawer's only multiline field.
    fireEvent.change(document.querySelector("textarea:not([aria-hidden])")!, {
      target: { value: "Read repositories" },
    });

    const toolsInput = screen.getByPlaceholderText("Search tools...");
    fireEvent.mouseDown(toolsInput);
    fireEvent.click(within(screen.getByRole("listbox")).getByText("search"));
    fireEvent.keyDown(toolsInput, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("listbox")).not.toBeInTheDocument());

    await waitFor(() => expect(hoisted.listRoles).toHaveBeenCalled());
    expect(hoisted.listRoles.mock.calls[0][0]).toEqual({ orgName: "acme", envName: "dev" });
    expect(screen.getByText("Development")).toBeInTheDocument();
    fireEvent.mouseDown(screen.getByPlaceholderText("Search roles..."));
    const reader = await screen.findByRole("option", { name: "Reader" });
    // Read-only and Thunder's native default role are not offered.
    expect(screen.queryByRole("option", { name: "Default" })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "Administrator" })).not.toBeInTheDocument();
    fireEvent.click(reader);

    fireEvent.click(screen.getByRole("button", { name: "Create scope" }));

    await waitFor(() =>
      expect(hoisted.createScope.mutateAsync).toHaveBeenCalledWith({
        params: { orgName: "acme", proxyId: "github" },
        body: { action: "repo.read", description: "Read repositories", tools: ["search"] },
      }),
    );
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(hoisted.markCompleted).toHaveBeenCalled();
    await waitFor(() =>
      expect(hoisted.updateRole.mutateAsync).toHaveBeenCalledWith({
        params: { orgName: "acme", envName: "dev", roleId: "r1" },
        body: { name: "Reader", description: undefined, scopes: ["existing", "urn:repo.read"] },
      }),
    );
  });

  it("keeps the drawer open and reports a failed create", async () => {
    hoisted.createScope.mutateAsync.mockRejectedValue(new Error("conflict"));
    renderDrawer([]);

    fireEvent.change(nameField(), { target: { value: "repo.read" } });
    fireEvent.click(screen.getByRole("button", { name: "Create scope" }));

    expect(
      await screen.findByText("Failed to create scope. Please try again."),
    ).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
    expect(hoisted.updateRole.mutateAsync).not.toHaveBeenCalled();
  });

  it("explains when the endpoint is not deployed anywhere", () => {
    renderDrawer([]);

    expect(
      screen.getByText("This endpoint isn't deployed to any environment yet."),
    ).toBeInTheDocument();
  });

  it("closes on cancel", () => {
    renderDrawer();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(onClose).toHaveBeenCalled();
  });
});
