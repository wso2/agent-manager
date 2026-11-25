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

import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

const state = vi.hoisted(() => ({
  agent: undefined as unknown,
  proxies: { data: undefined as unknown, isLoading: false },
  envs: { environments: [] as unknown[], isLoading: false },
  security: {} as Record<string, unknown>,
  create: {
    mutate: (() => undefined) as unknown as ReturnType<typeof vi.fn>,
    isError: false,
    isPending: false,
    error: null as unknown,
    reset: () => undefined,
  },
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useListMCPProxies: vi.fn(() => state.proxies),
  useGetAgent: () => ({ data: state.agent }),
  useListAgentMCPConfigs: () => ({ data: { configs: [{ name: "weather-mcp" }] } }),
  useCreateAgentMCPConfig: () => state.create,
}));
vi.mock("@agent-management-platform/shared-component", () => ({
  AGENTID_ENV_VAR_ROWS: [],
  EnvironmentVariablesReference: ({ title }: { title: string }) => <div>{title}</div>,
  useOrgEnvironmentsState: () => state.envs,
  useMCPProxySecurity: () => state.security,
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
}));
vi.mock("./MCPServerDisplay", () => ({
  MCPServerDisplay: ({ server }: { server: { name?: string } | null }) => (
    <span>{server?.name}</span>
  ),
}));

import { useListMCPProxies } from "@agent-management-platform/api-client";
import { AddMCPToolConfigPanel } from "./AddMCPToolConfigPanel";

const apiKeySecurity = {
  authenticationType: "apiKey",
  usesIdentitySecurity: false,
  spec: { editableKeys: ["url", "apikey"] },
  isLoading: false,
  isResolved: true,
};

function LocationProbe() {
  const location = useLocation();
  return (
    <>
      <div data-testid="location">{location.pathname}</div>
      <div data-testid="state">{JSON.stringify(location.state)}</div>
    </>
  );
}

const renderPanel = (onClose = vi.fn()) => {
  render(
    <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }} initialEntries={["/start"]}>
      <Routes>
        <Route
          path="/start"
          element={<AddMCPToolConfigPanel open orgId="org" projectId="p1" agentId="a1" onClose={onClose} />}
        />
        <Route path="*" element={<LocationProbe />} />
      </Routes>
    </MemoryRouter>,
  );
  return onClose;
};

beforeEach(() => {
  state.agent = { provisioning: { type: "internal" } };
  state.proxies = {
    data: {
      list: [
        { id: "weather", name: "Weather MCP", description: "Forecasts" },
        { id: "github", name: "GitHub", context: "/gh" },
      ],
    },
    isLoading: false,
  };
  state.envs = { environments: [{ name: "dev" }, { name: "prod" }], isLoading: false };
  state.security = apiKeySecurity;
  state.create = { mutate: vi.fn(), isError: false, isPending: false, error: null, reset: vi.fn() };
});

describe("AddMCPToolConfigPanel", () => {
  it("lists project proxies for the agent's project", () => {
    state.proxies = {
      data: { list: [{ id: "proj-proxy", name: "Project Proxy", projectName: "p1" }] },
      isLoading: false,
    };
    renderPanel();
    expect(useListMCPProxies).toHaveBeenCalledWith(
      { orgName: "org" },
      expect.objectContaining({ project: "p1" }),
    );
    expect(screen.getByText("Project Proxy")).toBeTruthy();
  });

  it("points to MCP server registration when there are none", () => {
    state.proxies = { data: { list: [] }, isLoading: false };
    renderPanel();
    expect(screen.getByText("No MCP servers available")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Register MCP server" }));
    expect(screen.getByTestId("location")).toHaveTextContent("/org/org/mcp-proxies/add");
  });

  it("filters servers by name, description or context", () => {
    renderPanel();
    const search = screen.getByPlaceholderText("Search MCP servers");
    fireEvent.change(search, { target: { value: "forecast" } });
    expect(screen.getByText("Weather MCP")).toBeInTheDocument();
    expect(screen.queryByText("GitHub")).not.toBeInTheDocument();
    fireEvent.change(search, { target: { value: "nothing" } });
    expect(screen.getByText("No MCP servers match your search")).toBeInTheDocument();
  });

  it("keeps Save disabled until a server is picked", () => {
    renderPanel();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("maps every environment to the picked server and opens the new configuration", () => {
    state.create.mutate.mockImplementation((_v, opts: { onSuccess: (d: unknown) => void }) =>
      opts.onSuccess({
        uuid: "cfg/1",
        envMappings: { dev: { configuration: { authInfo: { type: "apikey", in: "header", name: "K", value: "v" } } } },
      }),
    );
    const onClose = renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Weather MCP. Click to select" }));
    // "weather-mcp" already exists on the agent.
    expect(screen.getByLabelText("Name")).toHaveValue("weather-mcp-2");
    expect(screen.getByDisplayValue("WEATHER_URL")).toBeInTheDocument();
    fireEvent.change(screen.getByDisplayValue("WEATHER_API_KEY"), { target: { value: "WX_KEY" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(state.create.mutate).toHaveBeenCalledWith(
      {
        params: { orgName: "org", projName: "p1", agentName: "a1" },
        body: {
          name: "weather-mcp-2",
          type: "mcp",
          envMappings: {
            dev: { proxyId: "weather", configuration: {} },
            prod: { proxyId: "weather", configuration: {} },
          },
          environmentVariables: [
            { key: "url", name: "WEATHER_URL" },
            { key: "apikey", name: "WX_KEY" },
          ],
        },
      },
      expect.anything(),
    );
    expect(onClose).toHaveBeenCalled();
    expect(screen.getByTestId("location")).toHaveTextContent("cfg%2F1");
    expect(screen.getByTestId("state")).toHaveTextContent('"openEnvPanel":true');
  });

  it("lets the user go back and pick another server", () => {
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Weather MCP. Click to select" }));
    fireEvent.click(screen.getByRole("button", { name: "Selected: Weather MCP. Click to change." }));
    expect(screen.getByText("Choose the MCP server for this agent.")).toBeInTheDocument();
  });

  it("asks only for the URL on an unsecured server", () => {
    state.security = { ...apiKeySecurity, authenticationType: "", spec: { editableKeys: ["url"] } };
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "GitHub. Click to select" }));
    expect(screen.getByText(/This MCP server is unsecured/)).toBeInTheDocument();
    expect(screen.queryByDisplayValue("GITHUB_API_KEY")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(state.create.mutate.mock.calls[0][0].body.environmentVariables).toEqual([
      { key: "url", name: "GITHUB_URL" },
    ]);
  });

  it("lists the AgentID variables for an OAuth server", () => {
    state.security = {
      ...apiKeySecurity,
      authenticationType: "identity",
      usesIdentitySecurity: true,
      spec: { editableKeys: ["url"] },
    };
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "GitHub. Click to select" }));
    expect(screen.getByText(/even with OAuth/)).toBeInTheDocument();
    expect(screen.getByText("Injected at runtime")).toBeInTheDocument();
  });

  it("refuses to save while the server's security is unknown", () => {
    state.security = { ...apiKeySecurity, isResolved: false };
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "GitHub. Click to select" }));
    expect(screen.getByText(/Couldn't load this MCP server's security/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("saves an external agent's configuration without variables", () => {
    state.agent = { provisioning: { type: "external" } };
    state.security = { ...apiKeySecurity, isLoading: true, isResolved: false };
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "GitHub. Click to select" }));
    expect(screen.queryByText("Environment Variable Names")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(state.create.mutate.mock.calls[0][0].body.environmentVariables).toBeUndefined();
  });

  it("shows why creation failed", () => {
    state.create = { ...state.create, isError: true, error: new Error("duplicate name") };
    renderPanel();
    expect(screen.getByText("duplicate name")).toBeInTheDocument();
  });

  it("closes on cancel", () => {
    const onClose = renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalled();
  });
});
