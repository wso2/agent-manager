/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

// Tools listing, environment switching and the "Environment Variables" /
// "Connect to MCP Server" drawers of the MCP tool configuration page.

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { UIModuleStatusProvider } from "@agent-management-platform/views";

const state = vi.hoisted(() => ({
  agent: undefined as unknown,
  config: { data: undefined as unknown, isLoading: false, isError: false, error: null as unknown },
  proxy: { data: undefined as unknown, isLoading: false, isError: false },
  scopes: { data: undefined as unknown, isLoading: false, isError: false },
  envs: [] as { name: string; displayName: string; id: string }[],
  update: {
    mutate: (() => undefined) as unknown as ReturnType<typeof vi.fn>,
    isPending: false,
    isError: false,
    error: null as unknown,
  },
  credentials: {} as Record<string, unknown>,
  thunder: {} as Record<string, unknown>,
  copy: (() => Promise.resolve(true)) as unknown as ReturnType<typeof vi.fn>,
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useGetAgent: () => ({ data: state.agent }),
  useGetAgentMCPConfig: () => state.config,
  useGetMCPProxy: () => state.proxy,
  useGetProjectMCPProxy: () => ({ data: undefined, isLoading: false, isError: false }),
  useListEnvironments: () => ({ data: state.envs, isError: false }),
  useListMCPProxies: () => ({
    data: { list: [{ id: "weather", name: "Weather MCP", version: "v1", context: "/weather" }] },
    isLoading: false,
  }),
  useListMCPProxyScopes: () => state.scopes,
  useUpdateAgentMCPConfig: () => state.update,
  useListMCPConfigAPIKeys: () => ({ data: { keys: [] }, isLoading: false, isError: false }),
  useCreateMCPConfigAPIKey: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useRotateMCPConfigAPIKey: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useRevokeMCPConfigAPIKey: () => ({ mutate: vi.fn(), isPending: false }),
  useTrack: () => ({ track: vi.fn() }),
  ConsoleAction: {},
}));

vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useOrgEnvironmentsState: () => ({ environments: state.envs, isLoading: false }),
  useAgentIdentityCredentials: () => state.credentials,
  useThunderInstanceForEnv: () => state.thunder,
  useConfirmIfUnsaved: () => (fn: () => void) => fn(),
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
  CodeBlock: ({ code }: { code: string }) => <pre data-testid="code">{code}</pre>,
  copyToClipboard: (text: string) => state.copy(text),
  MCPLogo: () => null,
}));

import { ViewMCPServerComponent } from "./ViewMCPServer.Component";

const apiKeySecurity = { enabled: true, apiKey: { enabled: true, key: "X-MCP-Key" } };
const identitySecurity = { enabled: true, apiKey: { enabled: false }, identity: { enabled: true } };
const noSecurity = { enabled: true, apiKey: { enabled: false }, identity: { enabled: false } };

const endpoint = (envUuid: string, security: unknown, tools: string[] = ["forecast", "alerts"]) => ({
  environments: [{ environmentUuid: envUuid }],
  security,
  capabilities: { tools: tools.map((name) => ({ name })) },
  policies: [{ name: "mcp-acl-list", params: { tools: { mode: "allow", exceptions: ["alerts"] } } }],
});

const mcpConfig = (envVars = [
  { key: "url", name: "WEATHER_URL" },
  { key: "apikey", name: "WEATHER_API_KEY" },
]) => ({
  name: "",
  type: "mcp",
  environmentVariables: envVars,
  envMappings: {
    dev: { configuration: { proxyName: "weather", url: "https://gw.example/weather/mcp" } },
    prod: { configuration: { proxyName: "weather", url: "https://gw.example/prod/mcp" } },
  },
});

const renderPage = (routeState?: unknown, moduleStatus?: Record<string, "installed" | "not-installed">) => {
  const page = (
    <MemoryRouter
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      initialEntries={[{ pathname: "/org/o/project/p/agents/a/mcp/cfg%201", state: routeState }]}
    >
      <Routes>
        <Route path="/org/:orgId/project/:projectId/agents/:agentId/mcp/:proxyId" element={<ViewMCPServerComponent />} />
      </Routes>
    </MemoryRouter>
  );
  return render(
    moduleStatus
      ? <UIModuleStatusProvider value={moduleStatus}>{page}</UIModuleStatusProvider>
      : page,
  );
};

const setSecurity = (security: unknown, tools?: string[]) => {
  state.proxy = {
    data: { id: "weather", endpoints: [endpoint("env-dev", security, tools), endpoint("env-prod", security, [])] },
    isLoading: false,
    isError: false,
  };
};

beforeEach(() => {
  state.agent = { provisioning: { type: "internal" } };
  state.config = { data: mcpConfig(), isLoading: false, isError: false, error: null };
  state.envs = [
    { name: "dev", displayName: "Development", id: "env-dev" },
    { name: "prod", displayName: "Production", id: "env-prod" },
  ];
  setSecurity(apiKeySecurity);
  state.scopes = { data: { scopes: [{ scope: "weather:read", tools: ["forecast"] }] }, isLoading: false, isError: false };
  state.update = { mutate: vi.fn(), isPending: false, isError: false, error: null };
  state.credentials = {
    binding: { status: "active", clientId: "client-123" },
    provisioned: true,
    isLoading: false,
    isError: false,
    error: null,
    revealed: undefined,
    isRegenerating: false,
    regenerate: vi.fn(),
  };
  state.thunder = {
    thunderInstance: { issuerUrl: "https://idp/iss", tokenUrl: "https://idp/token", jwksUrl: "https://idp/jwks" },
    isLoading: false,
    isError: false,
    error: null,
  };
  state.copy = vi.fn(() => Promise.resolve(true));
});

describe("ViewMCPServerComponent page", () => {
  it("shows a skeleton while the configuration loads", () => {
    state.config = { data: undefined, isLoading: true, isError: false, error: null };
    renderPage();
    expect(screen.getByText("Tool Configuration")).toBeInTheDocument();
    expect(screen.queryByText("Tools")).not.toBeInTheDocument();
  });

  it("shows the load error", () => {
    state.config = { data: undefined, isLoading: false, isError: true, error: new Error("forbidden") };
    renderPage();
    expect(screen.getByText("forbidden")).toBeInTheDocument();
  });

  it("titles the page after the proxy and lists the environment's tools with their ACL status", () => {
    renderPage();
    expect(screen.getByRole("heading", { name: "Weather MCP", level: 1 })).toBeInTheDocument();
    expect(screen.getByText("Version: v1")).toBeInTheDocument();
    expect(screen.getByText("forecast")).toBeInTheDocument();
    // alerts is an exception to the allow-all ACL, so it is blocked.
    expect(screen.getByText("Blocked")).toBeInTheDocument();
    expect(screen.getByText("Allowed")).toBeInTheDocument();
    // Scopes column only for OAuth-secured tools.
    expect(screen.queryByText("weather:read")).not.toBeInTheDocument();
  });

  it("switches to another environment's endpoint", () => {
    renderPage();
    fireEvent.mouseDown(screen.getByRole("combobox"));
    fireEvent.click(screen.getByRole("option", { name: "Production" }));
    expect(screen.getByText("No tools available")).toBeInTheDocument();
  });

  it("says when the tools failed to load", () => {
    state.scopes = { data: undefined, isLoading: false, isError: true };
    renderPage();
    expect(screen.getByText("Failed to load tools")).toBeInTheDocument();
  });

  it("shows the scopes each tool needs when the proxy uses OAuth", () => {
    setSecurity(identitySecurity);
    renderPage();
    expect(screen.getByText("weather:read")).toBeInTheDocument();
    expect(screen.getByText("—")).toBeInTheDocument();
  });

  describe("internal agent guide", () => {
    it("offers an API-key snippet and saves renamed variables", async () => {
      renderPage();
      fireEvent.click(screen.getByRole("button", { name: "Environment Variables & Integration Guide" }));
      const code = screen.getByTestId("code").textContent ?? "";
      expect(code).toContain('raw_urls = os.environ.get("WEATHER_URL", "")');
      expect(code).toContain('mcp_api_key = os.environ.get("WEATHER_API_KEY", "").strip()');

      fireEvent.change(screen.getByDisplayValue("WEATHER_URL"), { target: { value: " TOOLS_URL " } });
      fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
      expect(state.update.mutate).toHaveBeenCalledWith(
        {
          params: { orgName: "o", projName: "p", agentName: "a", configId: "cfg 1" },
          body: {
            environmentVariables: [
              { key: "url", name: "TOOLS_URL" },
              { key: "apikey", name: "WEATHER_API_KEY" },
            ],
          },
        },
        expect.objectContaining({ onSuccess: expect.any(Function) }),
      );
    });

    it("blocks saving an empty variable name", () => {
      renderPage();
      fireEvent.click(screen.getByRole("button", { name: "Environment Variables & Integration Guide" }));
      fireEvent.change(screen.getByDisplayValue("WEATHER_URL"), { target: { value: "  " } });
      expect(screen.getByRole("button", { name: "Save changes" })).toBeDisabled();
    });

    it("discards renamed variables on cancel", () => {
      renderPage();
      fireEvent.click(screen.getByRole("button", { name: "Environment Variables & Integration Guide" }));
      fireEvent.change(screen.getByDisplayValue("WEATHER_URL"), { target: { value: "OTHER" } });
      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
      fireEvent.click(screen.getByRole("button", { name: "Environment Variables & Integration Guide" }));
      expect(screen.getByDisplayValue("WEATHER_URL")).toBeInTheDocument();
    });

    it("hides a stale API key variable for an unsecured proxy", () => {
      setSecurity(noSecurity);
      renderPage();
      fireEvent.click(screen.getByRole("button", { name: "Environment Variables & Integration Guide" }));
      expect(screen.queryByDisplayValue("WEATHER_API_KEY")).not.toBeInTheDocument();
      expect(screen.getByText(/This server requires no API key/)).toBeInTheDocument();
      expect(screen.getByTestId("code").textContent).not.toContain("mcp_api_key");
    });

    it("shows the AgentID token flow for an OAuth proxy", () => {
      setSecurity(identitySecurity);
      renderPage();
      fireEvent.click(screen.getByRole("button", { name: "Environment Variables & Integration Guide" }));
      expect(screen.getByText("AgentID Variables")).toBeInTheDocument();
      const code = screen.getByTestId("code").textContent ?? "";
      expect(code).toContain('mcp_server_url = os.environ.get("WEATHER_URL", "").strip()');
      expect(code).toContain('"resource": mcp_server_url');
    });

    it("warns when the proxy's security could not be loaded", () => {
      state.proxy = { data: undefined, isLoading: false, isError: true };
      renderPage({ openEnvPanel: true });
      expect(screen.getByText(/before saving/)).toBeInTheDocument();
    });
  });

  describe("external agent connect panel", () => {
    beforeEach(() => {
      state.agent = { provisioning: { type: "external" } };
    });

    it("shows the endpoint, header and the key handed over after creation", () => {
      renderPage({ authInfoByEnv: { dev: { type: "apikey", in: "header", name: "ignored", value: "mcp-secret" } } });
      expect(screen.getByText(/copy your API key now/)).toBeInTheDocument();
      expect(screen.getByDisplayValue("X-MCP-Key")).toBeInTheDocument();
      expect(screen.getByTestId("code").textContent).toBe(
        'curl -N https://gw.example/weather/mcp \\\n  --header "X-MCP-Key: mcp-secret"',
      );
      // The per-config API key manager sits on the page, behind the open drawer.
      expect(screen.getByRole("button", { name: "Generate", hidden: true })).toBeInTheDocument();
    });

    it("explains that the key was only shown at creation", () => {
      renderPage();
      fireEvent.click(screen.getByRole("button", { name: "Connect to MCP Server" }));
      expect(screen.getByText(/the key was only\s+displayed when this/)).toBeInTheDocument();
      expect(screen.getByTestId("code").textContent).toContain("X-MCP-Key: <api-key>");
    });

    it("needs no credential for an unsecured proxy", () => {
      setSecurity(noSecurity);
      renderPage({ openEnvPanel: true });
      expect(screen.getByText(/no header or API key is required/)).toBeInTheDocument();
      expect(screen.getByTestId("code").textContent).toBe("curl -N https://gw.example/weather/mcp");
    });

    // subscription-validation on the endpoint: every call carries the subscriber's key too.
    const withSubscriptions = (security: unknown, params: Record<string, unknown> = {}) => {
      setSecurity(security);
      const proxy = state.proxy as { data: { endpoints: { policies: unknown[] }[] } };
      for (const ep of proxy.data.endpoints) {
        ep.policies = [...ep.policies, { name: "subscription-validation", version: "v2", params }];
      }
    };

    it("asks an unsecured proxy that takes subscriptions for the subscription key", () => {
      withSubscriptions(noSecurity);
      renderPage({ openEnvPanel: true });
      expect(screen.getByText(/This MCP server takes subscriptions/)).toBeInTheDocument();
      expect(screen.queryByText(/no header or API key is required/)).not.toBeInTheDocument();
      expect(screen.getByDisplayValue("Subscription-Key")).toBeInTheDocument();
      expect(screen.getByTestId("code").textContent).toBe(
        'curl -N https://gw.example/weather/mcp \\\n  --header "Subscription-Key: <subscription-key>"',
      );
    });

    it("adds the subscription key, in the header the policy names, beside the API key", () => {
      withSubscriptions(apiKeySecurity, { subscriptionKeyHeader: "X-Sub" });
      renderPage({ authInfoByEnv: { dev: { type: "apikey", in: "header", name: "ignored", value: "mcp-secret" } } });
      expect(screen.getByDisplayValue("X-Sub")).toBeInTheDocument();
      expect(screen.getByTestId("code").textContent).toBe(
        'curl -N https://gw.example/weather/mcp \\\n  --header "X-MCP-Key: mcp-secret" \\\n  --header "X-Sub: <subscription-key>"',
      );
    });

    it("hands over client credentials, IdP endpoints and scopes for an OAuth proxy", async () => {
      setSecurity(identitySecurity);
      renderPage({ openEnvPanel: true });
      expect(screen.getByDisplayValue("client-123")).toBeInTheDocument();
      expect(screen.getByDisplayValue("https://idp/token")).toBeInTheDocument();
      expect(screen.getByDisplayValue("https://idp/jwks")).toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: "Regenerate secret" }));
      expect(state.credentials.regenerate).toHaveBeenCalled();
      fireEvent.click(screen.getByRole("button", { name: "Copy scopes" }));
      await waitFor(() => expect(state.copy).toHaveBeenCalledWith("weather:read"));
    });

    it("reports missing identity details", () => {
      setSecurity(identitySecurity);
      state.credentials = { ...state.credentials, binding: { status: "failed" } };
      state.thunder = { thunderInstance: undefined, isLoading: false, isError: false };
      renderPage({ openEnvPanel: true });
      expect(screen.getByText(/Provisioning failed/)).toBeInTheDocument();
      expect(screen.getByText("No identity provider found for this environment.")).toBeInTheDocument();
    });

    it("explains when the Agent Identity module is off", () => {
      setSecurity(identitySecurity);
      renderPage({ openEnvPanel: true }, { "agent-identity": "not-installed" });
      expect(screen.getByText(/The Agent Identity module is off/)).toBeInTheDocument();
      expect(screen.queryByDisplayValue("client-123")).not.toBeInTheDocument();
    });

    it("warns when the proxy's security could not be loaded", () => {
      state.proxy = { data: undefined, isLoading: false, isError: true };
      renderPage({ openEnvPanel: true });
      expect(screen.getByText(/Couldn't load this MCP server's security/)).toBeInTheDocument();
    });
  });
});
