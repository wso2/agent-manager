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

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

// These pages render real MUI forms, which can be slow on a busy CI runner.
vi.setConfig({ testTimeout: 30000 });


const hoisted = vi.hoisted(() => ({
  proxy: {
    data: undefined as unknown,
    isLoading: false,
    error: null as unknown,
    refetch: vi.fn(),
  },
  environments: { data: [] as unknown[], isLoading: false },
  update: {
    mutateAsync: vi.fn(),
    isPending: false,
    error: null as unknown,
  },
  fetchInfo: { mutateAsync: vi.fn(), isPending: false },
  gateways: { data: { gateways: [] as unknown[] }, isLoading: false, error: null },
}));

vi.mock("@agent-management-platform/api-client", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useGetMCPProxy: () => hoisted.proxy,
  useListEnvironments: () => hoisted.environments,
  useUpdateMCPProxy: () => hoisted.update,
  useFetchMCPProxyServerInfo: () => hoisted.fetchInfo,
  useListGateways: () => hoisted.gateways,
}));
vi.mock("@agent-management-platform/feature-management", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useFeatureFlag: () => false,
}));
vi.mock("@agent-management-platform/views", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useSnackBar: () => ({ pushSnackBar: vi.fn() }),
}));
vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
}));

import { EditMCPProxy } from "./EditMCPProxy";

const Location = () => <div data-testid="location">{useLocation().pathname}</div>;

const renderPage = () =>
  render(
    <MemoryRouter
      initialEntries={["/org/acme/mcp-proxies/view/github/edit"]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route
          path="/org/:orgId/mcp-proxies/view/:proxyId/edit"
          element={<EditMCPProxy />}
        />
        <Route path="*" element={<Location />} />
      </Routes>
    </MemoryRouter>,
  );

const aclPolicy = {
  name: "mcp-acl-list",
  version: "v1",
  params: { tools: { mode: "deny", exceptions: ["search"] } },
};

const proxy = {
  id: "github",
  name: "GitHub",
  version: "1.0.0",
  context: "/default/github",
  description: "Code tools",
  endpoints: [
    {
      id: "primary",
      name: "Primary",
      upstream: {
        main: {
          url: "https://mcp.example.com/mcp",
          auth: { type: "api-key", header: "Authorization" },
        },
      },
      capabilities: { tools: [{ name: "search" }], resources: [], prompts: [] },
      policies: [aclPolicy],
      security: { enabled: true, identity: { enabled: true } },
      environments: [{ environmentUuid: "env-dev", gatewayId: "gw-dev" }],
    },
  ],
};

beforeEach(() => {
  hoisted.proxy = {
    data: proxy,
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  };
  hoisted.environments = {
    data: [
      { id: "env-dev", name: "dev", displayName: "Development" },
      { id: "env-prod", name: "prod", displayName: "Production" },
    ],
    isLoading: false,
  };
  hoisted.update = {
    mutateAsync: vi.fn().mockResolvedValue({}),
    isPending: false,
    error: null,
  };
  hoisted.fetchInfo = { mutateAsync: vi.fn(), isPending: false };
  hoisted.gateways = {
    data: {
      gateways: [
        {
          uuid: "gw-dev",
          name: "dev-gw",
          gatewayType: "EGRESS",
          environments: [{ id: "env-dev", name: "dev" }],
        },
        {
          uuid: "gw-prod",
          name: "prod-gw",
          gatewayType: "EGRESS",
          environments: [{ id: "env-prod", name: "prod" }],
        },
      ],
    },
    isLoading: false,
    error: null,
  };
});

describe("EditMCPProxy", () => {
  it("shows a load error with retry when the proxy cannot be loaded", () => {
    hoisted.proxy = {
      data: undefined,
      isLoading: false,
      error: new Error("proxy not found"),
      refetch: vi.fn(),
    };
    renderPage();

    expect(screen.getByText("Couldn't load this MCP server")).toBeInTheDocument();
    expect(screen.getByText("proxy not found")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(hoisted.proxy.refetch).toHaveBeenCalled();
  });

  it("shows no form while loading", () => {
    hoisted.proxy = { ...hoisted.proxy, data: undefined, isLoading: true };
    renderPage();

    expect(screen.getByText("Edit MCP Server")).toBeInTheDocument();
    expect(screen.queryByLabelText("Name")).not.toBeInTheDocument();
  });

  it("prefills the form with the proxy and its endpoints", () => {
    renderPage();

    expect(screen.getByLabelText("Name")).toHaveValue("GitHub");
    expect(screen.getByLabelText("Handle")).toHaveValue("github");
    expect(screen.getByLabelText("Handle")).toBeDisabled();
    expect(screen.getByLabelText("Version")).toHaveValue("1.0.0");
    expect(screen.getByLabelText("Context (optional)")).toHaveValue("/default/github");
    expect(screen.getByText("Primary")).toBeInTheDocument();
    expect(screen.getByText("Development")).toBeInTheDocument();
    expect(screen.getByText("https://mcp.example.com/mcp")).toBeInTheDocument();
    expect(screen.getByText("1 of 2 environments have an endpoint.")).toBeInTheDocument();
  });

  it("requires a name before saving", () => {
    renderPage();

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "" } });

    expect(screen.getByText("Name is required")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("saves the edited fields and keeps the endpoint's policies and security", async () => {
    renderPage();

    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "  GitHub Tools  " },
    });
    fireEvent.change(screen.getByLabelText("Description (optional)"), {
      target: { value: "" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(hoisted.update.mutateAsync).toHaveBeenCalledTimes(1));
    const call = hoisted.update.mutateAsync.mock.calls[0][0];
    expect(call.params).toEqual({ orgName: "acme", proxyId: "github" });
    expect(call.body).toMatchObject({
      id: "github",
      name: "GitHub Tools",
      version: "1.0.0",
      context: "/default/github",
      description: undefined,
    });
    expect(call.body.endpoints).toHaveLength(1);
    expect(call.body.endpoints[0]).toMatchObject({
      id: "primary",
      name: "Primary",
      // The stored credential is kept by omitting its value.
      upstream: {
        main: {
          url: "https://mcp.example.com/mcp",
          auth: { type: "api-key", header: "Authorization" },
        },
      },
      policies: [aclPolicy],
      security: { enabled: true, identity: { enabled: true } },
      environments: [{ environmentUuid: "env-dev", gatewayId: "gw-dev" }],
    });
    expect(await screen.findByTestId("location")).toHaveTextContent(
      "/org/acme/mcp-proxies/view/github",
    );
  });

  it("shows the update error", () => {
    hoisted.update = { ...hoisted.update, error: new Error("version conflict") };
    renderPage();

    expect(screen.getByText("version conflict")).toBeInTheDocument();
  });

  it("returns to the view page on cancel", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.getByTestId("location")).toHaveTextContent(
      "/org/acme/mcp-proxies/view/github",
    );
  });

  it("removes an endpoint from the saved proxy", async () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Remove endpoint" }));
    expect(screen.getByText("No endpoints configured yet.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(hoisted.update.mutateAsync).toHaveBeenCalledTimes(1));
    expect(hoisted.update.mutateAsync.mock.calls[0][0].body.endpoints).toEqual([]);
  });

  it("edits an endpoint in place with its stored credential masked", async () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Edit endpoint" }));

    // Page-level actions hide while the endpoint form is open.
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
    expect(screen.getByLabelText("Endpoint Name (optional)")).toHaveValue("Primary");
    expect(screen.getByLabelText("MCP Server Endpoint URL")).toHaveValue(
      "https://mcp.example.com/mcp",
    );
    expect(
      screen.getByText("Leave unchanged to keep the stored value."),
    ).toBeInTheDocument();
    // Nothing changed yet, so Update stays disabled.
    expect(screen.getByRole("button", { name: "Update endpoint" })).toBeDisabled();

    fireEvent.change(screen.getByLabelText("Endpoint Name (optional)"), {
      target: { value: "Main" },
    });
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Update endpoint" })).toBeEnabled(),
    );
    fireEvent.click(screen.getByRole("button", { name: "Update endpoint" }));

    expect(await screen.findByText("Main")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(hoisted.update.mutateAsync).toHaveBeenCalledTimes(1));
    expect(hoisted.update.mutateAsync.mock.calls[0][0].body.endpoints[0]).toMatchObject({
      id: "primary",
      name: "Main",
    });
  });

  it("asks to re-enter the credential before re-fetching an edited URL", async () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Edit endpoint" }));
    fireEvent.change(screen.getByLabelText("MCP Server Endpoint URL"), {
      target: { value: "https://mcp.example.com/v2" },
    });

    expect(
      await screen.findByText(
        "Re-enter the authentication value to re-fetch server info.",
        undefined,
        { timeout: 8000 },
      ),
    ).toBeInTheDocument();
    expect(hoisted.fetchInfo.mutateAsync).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
  });
});
