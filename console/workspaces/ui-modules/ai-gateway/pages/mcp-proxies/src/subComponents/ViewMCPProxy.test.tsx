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
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

// These pages render real MUI forms, which can be slow on a busy CI runner.
vi.setConfig({ testTimeout: 30000 });

const hoisted = vi.hoisted(() => ({
  canWrite: true,
  proxy: { data: undefined as unknown, isLoading: false, error: null as unknown },
  environments: { data: [] as unknown[] },
  gateways: { data: { gateways: [] as unknown[] } },
  update: { mutateAsync: vi.fn(), isPending: false },
  pushSnackBar: vi.fn(),
  copyToClipboard: vi.fn(),
  securityProps: [] as Record<string, unknown>[],
  aihub: false,
}));

vi.mock("@agent-management-platform/api-client", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useGetMCPProxy: () => hoisted.proxy,
  useListEnvironments: () => hoisted.environments,
  useListGateways: () => hoisted.gateways,
  useUpdateMCPProxy: () => hoisted.update,
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useResourceAccess: () => ({ canRead: true, canWrite: hoisted.canWrite }),
}));
vi.mock("@agent-management-platform/views", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useSnackBar: () => ({ pushSnackBar: hoisted.pushSnackBar }),
  useIsUIModuleInstalled: (name: string) => (name === "ai-hub" ? hoisted.aihub : false),
}));
vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  copyToClipboard: hoisted.copyToClipboard,
}));

type TabProps = {
  config?: { id?: string; name?: string };
  envChips?: { label: string; status?: string }[];
  onUpdate?: (fields: unknown) => Promise<unknown>;
  onDirtyChange?: (dirty: boolean) => void;
  isOnSelfHostedGateway?: boolean;
  otherEndpointTools?: readonly string[];
  environments?: { name: string }[];
};

vi.mock("./MCPProxyOverviewTab", () => ({
  MCPProxyOverviewTab: ({ config, envChips }: TabProps) => (
    <div>
      overview of {config?.id}:{" "}
      {envChips?.map((chip) => `${chip.label} ${chip.status ?? ""}`).join(", ")}
    </div>
  ),
}));
vi.mock("./MCPProxyConnectionTab", () => ({
  MCPProxyConnectionTab: ({ onUpdate }: TabProps) => (
    <button onClick={() => void onUpdate?.({ resilience: { timeout: "5s" } })}>
      save connection
    </button>
  ),
}));
vi.mock("./MCPProxyManageToolsTab", () => ({
  MCPProxyManageToolsTab: () => <div>manage tools tab</div>,
}));
vi.mock("./MCPProxySecurityTab", () => ({
  MCPProxySecurityTab: (props: TabProps) => {
    hoisted.securityProps.push(props as Record<string, unknown>);
    return (
      <button onClick={() => props.onDirtyChange?.(true)}>edit security</button>
    );
  },
}));
vi.mock("./MCPProxyRewriteTab", () => ({
  MCPProxyRewriteTab: () => <div>rewrite tab</div>,
}));
vi.mock("./MCPProxySubscriptionPlansTab", () => ({
  MCPProxySubscriptionPlansTab: ({
    onSave,
  }: {
    onSave: (plans: string[], endpoints?: unknown[]) => Promise<unknown>;
  }) => (
    <div>
      <button onClick={() => void onSave(["gold"])}>save plans</button>
      <button onClick={() => void onSave(["gold"], [{ id: "primary", policies: [] }])}>
        save plans and policies
      </button>
    </div>
  ),
}));
vi.mock("./MCPProxyPoliciesTab", () => ({
  MCPProxyPoliciesTab: () => <div>policies tab</div>,
}));

import { ViewMCPProxy } from "./ViewMCPProxy";

const Location = () => {
  const location = useLocation();
  return <div data-testid="location">{location.pathname + location.search}</div>;
};

const renderPage = (search = "") =>
  render(
    <MemoryRouter
      initialEntries={[`/org/acme/mcp-proxies/view/github${search}`]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route
          path="/org/:orgId/mcp-proxies/view/:proxyId"
          element={
            <>
              <ViewMCPProxy />
              <Location />
            </>
          }
        />
        <Route path="*" element={<Location />} />
      </Routes>
    </MemoryRouter>,
  );

const proxy = {
  id: "github",
  name: "GitHub",
  version: "1.0.0",
  context: "/default/github",
  description: "Repository tools",
  mcpSpecVersion: "2025-03-26",
  endpoints: [
    {
      id: "primary",
      name: "Primary",
      mcpSpecVersions: ["2025-06-18"],
      capabilities: { tools: [{ name: "search" }] },
      environments: [
        { environmentUuid: "env-dev", gatewayId: "gw-self", deploymentStatus: "Deployed" },
        { environmentUuid: "env-gone", deploymentStatus: "Undeployed" },
      ],
    },
    {
      id: "secondary",
      name: "Secondary",
      capabilities: { tools: [{ name: "create_issue" }] },
      environments: [],
    },
  ],
};

beforeEach(() => {
  hoisted.aihub = false;
  hoisted.canWrite = true;
  hoisted.proxy = { data: proxy, isLoading: false, error: null };
  hoisted.environments = {
    data: [{ id: "env-dev", name: "dev", displayName: "Development" }],
  };
  hoisted.gateways = {
    data: {
      gateways: [
        { uuid: "gw-self", name: "self", hosting: "SELF_HOSTED", environments: [] },
      ],
    },
  };
  hoisted.update = { mutateAsync: vi.fn().mockResolvedValue({}), isPending: false };
  hoisted.pushSnackBar.mockReset();
  hoisted.copyToClipboard.mockReset().mockResolvedValue(true);
  hoisted.securityProps = [];
});

describe("ViewMCPProxy", () => {
  it("shows the load error", () => {
    hoisted.proxy = { data: undefined, isLoading: false, error: new Error("not allowed") };
    renderPage();

    expect(screen.getByText("not allowed")).toBeInTheDocument();
    expect(screen.queryByText("Context")).not.toBeInTheDocument();
  });

  it("shows nothing but the skeleton while loading", () => {
    hoisted.proxy = { data: undefined, isLoading: true, error: null };
    renderPage();

    expect(screen.queryByText("Context")).not.toBeInTheDocument();
    expect(screen.queryByRole("tab")).not.toBeInTheDocument();
  });

  it("explains how to add an endpoint when there are none", () => {
    hoisted.proxy = {
      data: { ...proxy, endpoints: [], context: undefined },
      isLoading: false,
      error: null,
    };
    renderPage();

    expect(
      screen.getByText(/This MCP Server has no endpoints configured/),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Copy Context" })).not.toBeInTheDocument();
    // No endpoint of its own, so the proxy's spec version applies.
    expect(screen.getByText("2025-03-26")).toBeInTheDocument();
  });

  it("shows the proxy details and the first endpoint's overview", async () => {
    renderPage();

    expect(screen.getByText("GitHub")).toBeInTheDocument();
    expect(screen.getByText("Repository tools")).toBeInTheDocument();
    expect(screen.getByText("/default/github")).toBeInTheDocument();
    expect(await screen.findByText("2025-06-18")).toBeInTheDocument();
    expect(screen.getByText("Primary")).toBeInTheDocument();
    expect(
      screen.getByText(/overview of primary: Development Deployed, env-gone Undeployed/),
    ).toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent("endpoint=primary");
  });

  it("switches endpoints from the endpoint selector", async () => {
    renderPage();

    fireEvent.mouseDown(await screen.findByRole("combobox"));
    fireEvent.click(within(screen.getByRole("listbox")).getByText("Secondary"));

    expect(await screen.findByText(/overview of secondary/)).toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent("endpoint=secondary");
    // Secondary reports no versions of its own, so the proxy's applies.
    expect(screen.getByText("2025-03-26")).toBeInTheDocument();
  });

  it("opens the tab named in the URL and switches tabs", async () => {
    renderPage("?tab=policies");

    expect(await screen.findByText("policies tab")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("tab", { name: "Manage Tools" }));
    expect(screen.getByText("manage tools tab")).toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent("tab=manage-tools");

    fireEvent.click(screen.getByRole("tab", { name: "Rewrite" }));
    expect(screen.getByText("rewrite tab")).toBeInTheDocument();
  });

  it("falls back to the overview for an unknown tab", async () => {
    renderPage("?tab=unknown");

    expect(await screen.findByText(/overview of primary/)).toBeInTheDocument();
  });

  it("saves a tab's change into the selected endpoint of the whole proxy", async () => {
    renderPage("?tab=connection");

    fireEvent.click(await screen.findByRole("button", { name: "save connection" }));

    await waitFor(() => expect(hoisted.update.mutateAsync).toHaveBeenCalledTimes(1));
    const call = hoisted.update.mutateAsync.mock.calls[0][0];
    expect(call.params).toEqual({ orgName: "acme", proxyId: "github" });
    expect(call.body.endpoints[0]).toEqual({
      ...proxy.endpoints[0],
      resilience: { timeout: "5s" },
    });
    expect(call.body.endpoints[1]).toEqual(proxy.endpoints[1]);
  });

  it("gives the security tab the other endpoints' tools and the gateway hosting", async () => {
    renderPage("?tab=security");

    await screen.findByRole("button", { name: "edit security" });
    const props = hoisted.securityProps[hoisted.securityProps.length - 1];
    expect(props.otherEndpointTools).toEqual(["create_issue"]);
    expect(props.isOnSelfHostedGateway).toBe(true);
    expect(props.environments).toEqual([
      { id: "env-dev", name: "dev", displayName: "Development" },
    ]);
  });

  it("asks before leaving the security tab with unsaved changes", async () => {
    renderPage("?tab=security");

    fireEvent.click(await screen.findByRole("button", { name: "edit security" }));
    fireEvent.click(screen.getByRole("tab", { name: "Overview" }));

    expect(await screen.findByText("You have unsaved changes")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Stay" }));
    await waitFor(() =>
      expect(screen.queryByText("You have unsaved changes")).not.toBeInTheDocument(),
    );
    expect(screen.getByRole("button", { name: "edit security" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("tab", { name: "Overview" }));
    fireEvent.click(await screen.findByRole("button", { name: "Leave" }));

    expect(await screen.findByText(/overview of primary/)).toBeInTheDocument();
  });

  it("opens the edit page", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Edit MCP server" }));

    expect(screen.getByTestId("location")).toHaveTextContent(
      "/org/acme/mcp-proxies/view/github/edit",
    );
  });

  it("offers no edit action without write access", () => {
    hoisted.canWrite = false;
    renderPage();

    expect(
      screen.queryByRole("button", { name: "Edit MCP server" }),
    ).not.toBeInTheDocument();
  });

  it("copies the context and confirms it", async () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Copy Context" }));

    expect(hoisted.copyToClipboard).toHaveBeenCalledWith("/default/github");
    await waitFor(() =>
      expect(hoisted.pushSnackBar).toHaveBeenCalledWith({
        message: "Context copied to clipboard",
        type: "success",
      }),
    );
  });

  it("reports a failed copy", async () => {
    hoisted.copyToClipboard.mockResolvedValue(false);
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Copy Context" }));

    await waitFor(() =>
      expect(hoisted.pushSnackBar).toHaveBeenCalledWith({
        message: "Failed to copy Context",
        type: "error",
      }),
    );
  });

  it("has no Subscription Plans tab without the AI Hub", () => {
    renderPage("?tab=subscription-plans");
    expect(screen.queryByRole("tab", { name: "Subscription Plans" })).not.toBeInTheDocument();
  });

  it("saves the offered plans, and the endpoints when their policies changed", async () => {
    hoisted.aihub = true;
    hoisted.update.mutateAsync.mockResolvedValue(undefined);
    renderPage("?tab=subscription-plans");
    fireEvent.click(await screen.findByRole("button", { name: "save plans" }));
    await waitFor(() => expect(hoisted.update.mutateAsync).toHaveBeenCalled());
    const first = hoisted.update.mutateAsync.mock.lastCall?.[0];
    expect(first.params).toEqual({ orgName: "acme", proxyId: "github" });
    expect(first.body.subscriptionPlans).toEqual(["gold"]);
    expect(first.body.endpoints).toEqual(proxy.endpoints);

    fireEvent.click(screen.getByRole("button", { name: "save plans and policies" }));
    await waitFor(() =>
      expect(hoisted.update.mutateAsync.mock.lastCall?.[0].body.endpoints).toEqual([
        { id: "primary", policies: [] },
      ]),
    );
  });
});
