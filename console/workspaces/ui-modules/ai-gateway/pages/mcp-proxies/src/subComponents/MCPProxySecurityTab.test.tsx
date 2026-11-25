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
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

// These pages render real MUI forms, which can be slow on a busy CI runner.
vi.setConfig({ testTimeout: 30000 });
import type { MCPEndpointConfig } from "@agent-management-platform/types";

const hoisted = vi.hoisted(() => ({
  canWrite: true,
  scopes: { data: undefined as unknown, isPending: false },
  updateScope: { mutateAsync: vi.fn(), isPending: false },
  deleteScope: { mutateAsync: vi.fn(), isPending: false },
  addConfirmation: vi.fn(),
  drawerProps: [] as Record<string, unknown>[],
}));

vi.mock("@agent-management-platform/api-client", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useListMCPProxyScopes: () => hoisted.scopes,
  useUpdateMCPProxyScope: () => hoisted.updateScope,
  useDeleteMCPProxyScope: () => hoisted.deleteScope,
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useResourceAccess: () => ({ canRead: true, canWrite: hoisted.canWrite }),
}));
vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useConfirmationDialog: () => ({ addConfirmation: hoisted.addConfirmation }),
}));
vi.mock("./CreateScopeDrawer", () => ({
  CreateScopeDrawer: (props: Record<string, unknown>) => {
    hoisted.drawerProps.push(props);
    return props.open ? <div>create scope drawer</div> : null;
  },
}));

import { MCPProxySecurityTab } from "./MCPProxySecurityTab";

const onUpdate = vi.fn();
const onDirtyChange = vi.fn();

const apiKeyConfig = {
  id: "primary",
  security: { enabled: true, apiKey: { enabled: true, key: "X-Custom", in: "header" } },
  capabilities: { tools: [{ name: "search" }, { name: "delete_repo" }] },
} as unknown as MCPEndpointConfig;

const identityConfig = {
  id: "primary",
  security: { enabled: true, identity: { enabled: true } },
  capabilities: { tools: [{ name: "search" }, { name: "delete_repo" }] },
  policies: [
    {
      name: "mcp-acl-list",
      version: "v1",
      params: { tools: { mode: "allow", exceptions: ["delete_repo"] } },
    },
  ],
} as unknown as MCPEndpointConfig;

const scopes = [
  { action: "repo.read", description: "Read repos", tools: ["search"], scope: "s1" },
  { action: "repo.admin", description: "", tools: [], scope: "s2" },
];

const renderTab = (
  config: MCPEndpointConfig | undefined,
  extra: Partial<Parameters<typeof MCPProxySecurityTab>[0]> = {},
) =>
  render(
    <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <MCPProxySecurityTab
        config={config}
        selectedEndpointId="primary"
        orgName="acme"
        proxyId="github"
        environments={[]}
        onUpdate={onUpdate}
        isUpdating={false}
        onDirtyChange={onDirtyChange}
        {...extra}
      />
    </MemoryRouter>,
  );

const chooseOption = (comboboxIndex: number, label: string) => {
  fireEvent.mouseDown(screen.getAllByRole("combobox")[comboboxIndex]);
  fireEvent.click(within(screen.getByRole("listbox")).getByText(label));
};

beforeEach(() => {
  hoisted.canWrite = true;
  hoisted.scopes = { data: { scopes }, isPending: false };
  hoisted.updateScope.mutateAsync.mockReset().mockResolvedValue({});
  hoisted.deleteScope.mutateAsync.mockReset().mockResolvedValue({});
  hoisted.addConfirmation.mockReset();
  hoisted.drawerProps = [];
  onUpdate.mockReset().mockResolvedValue({});
  onDirtyChange.mockReset();
});

describe("MCPProxySecurityTab with API key security", () => {
  it("shows the saved key and location", () => {
    renderTab(apiKeyConfig);

    expect(screen.getByText("API Key")).toBeInTheDocument();
    expect(screen.getByLabelText("Header Key")).toHaveValue("X-Custom");
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("saves a changed key location", async () => {
    renderTab(apiKeyConfig);

    chooseOption(1, "query");
    expect(screen.getByLabelText("Query Param Key")).toHaveValue("X-Custom");
    expect(screen.getByText("You have unsaved changes")).toBeInTheDocument();
    expect(onDirtyChange).toHaveBeenLastCalledWith(true);

    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(onUpdate).toHaveBeenCalledWith({
        security: {
          enabled: true,
          apiKey: { enabled: true, key: "X-Custom", in: "query" },
          identity: { enabled: false },
        },
      }),
    );
    expect(await screen.findByText("Updated security settings.")).toBeInTheDocument();
  });

  it("requires a key for API key security", () => {
    renderTab(apiKeyConfig);

    fireEvent.change(screen.getByLabelText("Header Key"), { target: { value: " " } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(
      screen.getAllByText("API Key is required when using API Key authentication"),
    ).not.toHaveLength(0);
    expect(onUpdate).not.toHaveBeenCalled();
  });

  it("reports a failed save", async () => {
    onUpdate.mockRejectedValue(new Error("boom"));
    renderTab(apiKeyConfig);

    fireEvent.change(screen.getByLabelText("Header Key"), { target: { value: "X-Key" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("Failed to update security.")).toBeInTheDocument();
  });

  it("discards unsaved edits", () => {
    renderTab(apiKeyConfig);

    fireEvent.change(screen.getByLabelText("Header Key"), { target: { value: "X-Other" } });
    fireEvent.click(screen.getByRole("button", { name: "Discard" }));

    expect(screen.getByLabelText("Header Key")).toHaveValue("X-Custom");
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("confirms before switching the authentication method", () => {
    renderTab(apiKeyConfig);

    chooseOption(0, "None");

    expect(hoisted.addConfirmation).toHaveBeenCalledTimes(1);
    const confirmation = hoisted.addConfirmation.mock.calls[0][0];
    expect(confirmation.title).toBe("Switch authentication method?");
    expect(confirmation.description).toContain("secured with API Key");
    // Nothing changes until the switch is confirmed.
    expect(screen.getByLabelText("Header Key")).toBeInTheDocument();

    confirmation.onConfirm();
    return waitFor(() =>
      expect(screen.queryByLabelText("Header Key")).not.toBeInTheDocument(),
    );
  });

  it("explains why OAuth is unavailable on a self-hosted gateway", () => {
    renderTab(apiKeyConfig, { isOnSelfHostedGateway: true });

    expect(
      screen.getByText(
        "OAuth isn't available on self-hosted gateways yet. Use API key security.",
      ),
    ).toBeInTheDocument();
  });

  it("is read-only without write access", () => {
    hoisted.canWrite = false;
    renderTab(apiKeyConfig);

    expect(screen.getByLabelText("Header Key")).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  });

  it("shows only the heading while loading", () => {
    renderTab(apiKeyConfig, { isLoading: true });

    expect(screen.getByText("Authentication")).toBeInTheDocument();
    expect(screen.queryByLabelText("Header Key")).not.toBeInTheDocument();
  });
});

describe("MCPProxySecurityTab with OAuth security", () => {
  it("lists each tool with its assigned scopes", () => {
    renderTab(identityConfig);

    expect(screen.getByText("Authorization")).toBeInTheDocument();
    expect(screen.getByText("search")).toBeInTheDocument();
    expect(screen.getByText("delete_repo")).toBeInTheDocument();
    expect(screen.getByText("repo.read")).toBeInTheDocument();
  });

  it("saves a new scope assignment through the scope API", async () => {
    renderTab(identityConfig, { otherEndpointTools: ["create_issue"] });

    const deleteRepoRow = screen.getByText("delete_repo").closest("tr")!;
    const input = within(deleteRepoRow).getByPlaceholderText("Assign scopes...");
    fireEvent.mouseDown(input);
    fireEvent.click(within(screen.getByRole("listbox")).getByText("repo.admin"));

    expect(screen.getByText("You have unsaved changes")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(hoisted.updateScope.mutateAsync).toHaveBeenCalledWith({
        params: { orgName: "acme", proxyId: "github", scopeAction: "repo.admin" },
        body: { tools: ["delete_repo"] },
      }),
    );
    expect(hoisted.updateScope.mutateAsync).toHaveBeenCalledTimes(1);
    expect(onUpdate).toHaveBeenCalledWith({
      security: {
        enabled: true,
        apiKey: { enabled: false, key: "", in: "header" },
        identity: { enabled: true },
      },
    });
    expect(await screen.findByText("Updated security settings.")).toBeInTheDocument();
  });

  it("lists the scope catalog and deletes a scope after confirmation", async () => {
    renderTab(identityConfig);

    fireEvent.click(screen.getByRole("tab", { name: "Scopes" }));
    expect(screen.getByText("Read repos")).toBeInTheDocument();

    const readRow = screen.getByText("Read repos").closest("tr")!;
    fireEvent.click(within(readRow).getByRole("button"));

    const confirmation = hoisted.addConfirmation.mock.calls[0][0];
    expect(confirmation.title).toBe("Delete Scope");
    await confirmation.onConfirm();
    expect(hoisted.deleteScope.mutateAsync).toHaveBeenCalledWith({
      orgName: "acme",
      proxyId: "github",
      scopeAction: "repo.read",
    });
  });

  it("shows an empty scope catalog", () => {
    hoisted.scopes = { data: { scopes: [] }, isPending: false };
    renderTab(identityConfig);

    fireEvent.click(screen.getByRole("tab", { name: "Scopes" }));

    expect(screen.getByText("No scopes yet")).toBeInTheDocument();
    expect(screen.getByText('Click "Create Scope" to add one.')).toBeInTheDocument();
  });

  it("explains when the endpoint has no tools to bind", () => {
    renderTab({ ...identityConfig, capabilities: { tools: [] } } as MCPEndpointConfig);

    expect(screen.getByText("No Tools Available")).toBeInTheDocument();
  });

  it("opens the Create Scope drawer with the endpoint's tools and the ACL-blocked ones", () => {
    renderTab(identityConfig);

    fireEvent.click(screen.getByRole("button", { name: "Create scope" }));

    expect(screen.getByText("create scope drawer")).toBeInTheDocument();
    const props = hoisted.drawerProps[hoisted.drawerProps.length - 1];
    expect(props.tools).toEqual(["search", "delete_repo"]);
    expect([...(props.blockedTools as Set<string>)]).toEqual(["delete_repo"]);
  });
});
