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
  create: { mutateAsync: vi.fn(), isPending: false },
  fetchInfo: { mutateAsync: vi.fn(), isPending: false },
  environments: { data: [] as unknown[] },
  gateways: { data: { gateways: [] as unknown[] }, isLoading: false, error: null },
  pushSnackBar: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useCreateMCPProxy: () => hoisted.create,
  useListEnvironments: () => hoisted.environments,
  useFetchMCPProxyServerInfo: () => hoisted.fetchInfo,
  useListGateways: () => hoisted.gateways,
}));
vi.mock("@agent-management-platform/feature-management", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useFeatureFlag: () => false,
}));
vi.mock("@agent-management-platform/views", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useSnackBar: () => ({ pushSnackBar: hoisted.pushSnackBar }),
}));
vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
}));

import { AddMCPProxyForm } from "./AddMCPProxyForm";

const Location = () => <div data-testid="location">{useLocation().pathname}</div>;

const onCancel = vi.fn();

const renderForm = () =>
  render(
    <MemoryRouter
      initialEntries={["/org/acme/mcp-proxies/add"]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route
          path="/org/:orgId/mcp-proxies/add"
          element={<AddMCPProxyForm onCancel={onCancel} />}
        />
        <Route path="*" element={<Location />} />
      </Routes>
    </MemoryRouter>,
  );

const ENDPOINT_URL = "https://mcp.example.com/mcp";
const serverInfo = {
  serverInfo: { name: "GitHub Server", version: "2.0.0" },
  tools: [{ name: "search_repos", description: "Search repositories" }],
  resources: [],
  prompts: [],
  supportedVersions: ["2025-06-18"],
};

const typeEndpointUrl = (url: string) =>
  fireEvent.change(screen.getByLabelText("MCP Server Endpoint URL"), {
    target: { value: url },
  });

beforeEach(() => {
  onCancel.mockReset();
  hoisted.create.mutateAsync.mockReset().mockResolvedValue({});
  hoisted.fetchInfo.mutateAsync.mockReset().mockResolvedValue(serverInfo);
  hoisted.pushSnackBar.mockReset();
  hoisted.environments = {
    data: [{ id: "env-dev", name: "dev", displayName: "Development" }],
  };
  hoisted.gateways = {
    data: {
      gateways: [
        {
          uuid: "gw-dev",
          name: "dev-gw",
          displayName: "Dev Gateway",
          gatewayType: "EGRESS",
          environments: [{ id: "env-dev", name: "dev" }],
        },
      ],
    },
    isLoading: false,
    error: null,
  };
});

describe("AddMCPProxyForm", () => {
  it("keeps Create disabled until an endpoint is added and checks the version", () => {
    renderForm();

    expect(screen.getByText("No endpoints added yet.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create" })).toBeDisabled();

    fireEvent.change(screen.getByLabelText("Version"), {
      target: { value: "v-one" },
    });
    expect(
      screen.getByText("Use a semantic version, e.g. 0.0.1"),
    ).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Version"), { target: { value: "" } });
    expect(screen.getByText("Version is required")).toBeInTheDocument();
  });

  it("derives the handle and context from the name", () => {
    renderForm();

    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "My Docs Server" },
    });

    expect(screen.getByLabelText("Handle")).toHaveValue("my-docs-server");
    expect(screen.getByLabelText("Context (optional)")).toHaveValue(
      "/default/my-docs-server",
    );
  });

  it("flags a handle that is too long", () => {
    renderForm();

    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "a".repeat(101) },
    });

    expect(
      screen.getByText("Handle must be at most 100 characters (currently 101)."),
    ).toBeInTheDocument();
  });

  it("cancels through the onCancel callback", () => {
    renderForm();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it("fetches the server, adds the endpoint and creates the proxy", async () => {
    renderForm();

    fireEvent.click(screen.getByRole("button", { name: "Add endpoint" }));
    typeEndpointUrl(ENDPOINT_URL);

    await waitFor(
      () =>
        expect(hoisted.fetchInfo.mutateAsync).toHaveBeenCalledWith(
          expect.objectContaining({
            params: { orgName: "acme" },
            body: { url: ENDPOINT_URL },
          }),
        ),
      { timeout: 8000 },
    );
    expect(await screen.findByText("GitHub Server")).toBeInTheDocument();
    expect(screen.getByText("v2.0.0")).toBeInTheDocument();

    const addButton = screen.getByRole("button", { name: "Add endpoint" });
    await waitFor(() => expect(addButton).toBeEnabled());
    fireEvent.click(addButton);

    // The new endpoint row, and the proxy fields seeded from the server info.
    expect(await screen.findByText(ENDPOINT_URL)).toBeInTheDocument();
    expect(screen.getByText("1 tools · 0 resources · 0 prompts")).toBeInTheDocument();
    expect(screen.getByLabelText("Name")).toHaveValue("GitHub Server");
    expect(screen.getByLabelText("Version")).toHaveValue("2.0.0");
    expect(screen.getByLabelText("Handle")).toHaveValue("github-server");

    fireEvent.change(screen.getByLabelText("Description (optional)"), {
      target: { value: "  Code tools  " },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() =>
      expect(hoisted.create.mutateAsync).toHaveBeenCalledTimes(1),
    );
    expect(hoisted.create.mutateAsync).toHaveBeenCalledWith({
      params: { orgName: "acme" },
      body: {
        id: "github-server",
        name: "GitHub Server",
        version: "2.0.0",
        description: "Code tools",
        context: "/default/github-server",
        endpoints: [
          expect.objectContaining({
            id: "github-server",
            name: undefined,
            upstream: { main: { url: ENDPOINT_URL, auth: undefined } },
            capabilities: {
              tools: serverInfo.tools,
              resources: [],
              prompts: [],
            },
            mcpSpecVersions: ["2025-06-18"],
            security: {
              enabled: true,
              apiKey: { enabled: true, key: "X-API-Key", in: "header" },
            },
            environments: [
              { environmentUuid: "env-dev", gatewayId: "gw-dev" },
            ],
          }),
        ],
      },
    });
    expect(await screen.findByTestId("location")).toHaveTextContent(
      "/org/acme/mcp-proxies",
    );
  });

  it("removes an added endpoint", async () => {
    renderForm();

    fireEvent.click(screen.getByRole("button", { name: "Add endpoint" }));
    typeEndpointUrl(ENDPOINT_URL);
    const addButton = await screen.findByRole("button", { name: "Add endpoint" });
    await waitFor(() => expect(addButton).toBeEnabled(), { timeout: 8000 });
    fireEvent.click(addButton);

    fireEvent.click(await screen.findByRole("button", { name: "Remove endpoint" }));

    expect(screen.getByText("No endpoints added yet.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create" })).toBeDisabled();
  });

  it("rejects an invalid endpoint URL without fetching", async () => {
    renderForm();

    fireEvent.click(screen.getByRole("button", { name: "Add endpoint" }));
    typeEndpointUrl("not a url");

    expect(
      await screen.findByText(
        "Enter a valid MCP Server endpoint URL.",
        undefined,
        { timeout: 8000 },
      ),
    ).toBeInTheDocument();
    expect(hoisted.fetchInfo.mutateAsync).not.toHaveBeenCalled();
  });

  it("asks for credentials when the server answers unauthorized", async () => {
    hoisted.fetchInfo.mutateAsync.mockRejectedValue({ code: "UNAUTHORIZED" });
    renderForm();

    fireEvent.click(screen.getByRole("button", { name: "Add endpoint" }));
    typeEndpointUrl(ENDPOINT_URL);

    expect(
      await screen.findByText(
        "This server requires authentication. Enter the credentials above.",
        undefined,
        { timeout: 8000 },
      ),
    ).toBeInTheDocument();
  });

  it("reports other fetch failures in a snackbar", async () => {
    hoisted.fetchInfo.mutateAsync.mockRejectedValue(new Error("upstream down"));
    renderForm();

    fireEvent.click(screen.getByRole("button", { name: "Add endpoint" }));
    typeEndpointUrl(ENDPOINT_URL);

    await waitFor(
      () =>
        expect(hoisted.pushSnackBar).toHaveBeenCalledWith({
          message: "upstream down",
          type: "error",
        }),
      { timeout: 8000 },
    );
  });

  it("sends the auth header with the fetch and requires both key and value", async () => {
    renderForm();

    fireEvent.click(screen.getByRole("button", { name: "Add endpoint" }));
    fireEvent.click(screen.getByText("Advanced Configurations"));
    fireEvent.change(screen.getByPlaceholderText("Key"), {
      target: { value: "Authorization" },
    });
    typeEndpointUrl(ENDPOINT_URL);

    expect(
      await screen.findByText(
        "Enter both an authentication header and value.",
        undefined,
        { timeout: 8000 },
      ),
    ).toBeInTheDocument();
    expect(hoisted.fetchInfo.mutateAsync).not.toHaveBeenCalled();

    fireEvent.change(screen.getByPlaceholderText("Value"), {
      target: { value: "Bearer abc" },
    });

    await waitFor(
      () =>
        expect(hoisted.fetchInfo.mutateAsync).toHaveBeenCalledWith(
          expect.objectContaining({
            body: {
              url: ENDPOINT_URL,
              auth: { type: "api-key", header: "Authorization", value: "Bearer abc" },
            },
          }),
        ),
      { timeout: 8000 },
    );
  });

  it("disables Add endpoint when no environment exists", () => {
    hoisted.environments = { data: [] };
    renderForm();

    expect(screen.getByRole("button", { name: "Add endpoint" })).toBeDisabled();
  });
});
