/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

const hoisted = vi.hoisted(() => ({
  createKey: vi.fn(),
  revokeKey: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useCreateAgentAPIKey: vi.fn(() => ({ mutateAsync: hoisted.createKey, isPending: false })),
  useGetA2AProxy: vi.fn(),
  useGetAgent: vi.fn(),
  useGetAgentConfigurations: vi.fn(() => ({
    data: { enableApiKeySecurity: true },
    isLoading: false,
  })),
  useListAgentAPIKeys: vi.fn(() => ({ data: [], isLoading: false, isError: false })),
  useRevokeAgentAPIKey: vi.fn(() => ({ mutate: hoisted.revokeKey, isPending: false })),
}));
vi.mock("@agent-management-platform/shared-component", () => ({
  APIKeysManager: ({
    keys,
    onCreate,
    onRevoke,
  }: {
    keys?: Array<{ name: string }>;
    onCreate: (input: { displayName: string; expiresAt?: string }) => Promise<string>;
    onRevoke: (keyName: string) => void;
  }) => (
    <div>
      api keys manager
      {keys?.map((k) => <span key={k.name}>key {k.name}</span>)}
      <button
        onClick={() =>
          void onCreate({ displayName: "CI key", expiresAt: "2027-01-01T00:00:00Z" }).then(
            (apiKey) => {
              document.body.dataset.createdKey = apiKey;
            },
          )
        }
      >
        create key
      </button>
      <button onClick={() => onRevoke("old-key")}>revoke key</button>
    </div>
  ),
  EnvironmentSelector: () => null,
}));
vi.mock("@agent-management-platform/views", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@agent-management-platform/views")>()),
  PageLayout: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

import { FeatureManagementProvider } from "@agent-management-platform/feature-management";
import {
  useGetA2AProxy,
  useGetAgent,
  useGetAgentConfigurations,
  useListAgentAPIKeys,
} from "@agent-management-platform/api-client";
import { SecurityComponent } from "./Security.Component";

const renderPage = (canManageKeys = true) =>
  render(
    <FeatureManagementProvider featureAccess={{ agent_api_key_manage: canManageKeys }}>
      <MemoryRouter
        initialEntries={["/org/o/project/p/agents/a/environment/dev/security"]}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <Routes>
          <Route path="/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/security" element={<SecurityComponent />} />
        </Routes>
      </MemoryRouter>
    </FeatureManagementProvider>,
  );

const routedProxy = () =>
  vi.mocked(useGetA2AProxy).mockReturnValue({
    data: { status: "DEPLOYED", statusReason: "", gatewayUrl: "https://gw.example/agent", upstreamAuthHeader: "" }, isLoading: false, isError: false, error: null,
  } as unknown as ReturnType<typeof useGetA2AProxy>);

const externalA2A = () =>
  vi.mocked(useGetAgent).mockReturnValue({
    data: { provisioning: { type: "external" }, agentType: { type: "external-agent-api" } },
    isLoading: false,
  } as unknown as ReturnType<typeof useGetAgent>);

const A2A_SETTINGS_HREF = "/org/o/project/p/agents/a/configure?tab=a2a&env=dev";

beforeEach(() => {
  vi.clearAllMocks();
  delete document.body.dataset.createdKey;
  vi.mocked(useGetAgentConfigurations).mockReturnValue({
    data: { enableApiKeySecurity: true }, isLoading: false,
  } as unknown as ReturnType<typeof useGetAgentConfigurations>);
  vi.mocked(useListAgentAPIKeys).mockReturnValue({
    data: [], isLoading: false, isError: false,
  } as unknown as ReturnType<typeof useListAgentAPIKeys>);
});

describe("SecurityComponent", () => {
  it("manages API keys of an external A2A agent routed through the gateway", () => {
    externalA2A();
    vi.mocked(useGetA2AProxy).mockReturnValue({
      data: { status: "DEPLOYED", statusReason: "", gatewayUrl: "https://gw.example/agent", upstreamAuthHeader: "" }, isLoading: false, isError: false, error: null,
    } as unknown as ReturnType<typeof useGetA2AProxy>);
    renderPage();
    expect(screen.getByText("api keys manager")).toBeInTheDocument();
  });

  it("explains why a direct external A2A agent has no API keys", () => {
    externalA2A();
    vi.mocked(useGetA2AProxy).mockReturnValue({
      data: null, isLoading: false, isError: false, error: null,
    } as unknown as ReturnType<typeof useGetA2AProxy>);
    renderPage();
    expect(screen.getByText(/not routed through the gateway/i)).toBeInTheDocument();
    expect(screen.getByText(/under Configure → A2A/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Go to A2A settings" })).toHaveAttribute("href", A2A_SETTINGS_HREF);
  });

  it("reports a failed routing lookup instead of calling the agent direct", () => {
    externalA2A();
    vi.mocked(useGetA2AProxy).mockReturnValue({
      data: undefined, isLoading: false, isError: true, error: new Error("boom"),
    } as unknown as ReturnType<typeof useGetA2AProxy>);
    renderPage();
    expect(screen.getByText(/could not load gateway routing/i)).toBeInTheDocument();
    expect(screen.queryByText(/not routed through the gateway/i)).not.toBeInTheDocument();
  });

  it("reports a failed agent lookup instead of saying the agent is not routed", () => {
    vi.mocked(useGetAgent).mockReturnValue({
      data: undefined, isLoading: false, isError: true, error: new Error("boom"),
    } as unknown as ReturnType<typeof useGetAgent>);
    vi.mocked(useGetA2AProxy).mockReturnValue({
      data: undefined, isLoading: false, isError: false, error: null,
    } as unknown as ReturnType<typeof useGetA2AProxy>);
    renderPage();
    expect(screen.getByText(/could not load the agent/i)).toBeInTheDocument();
    expect(screen.queryByText(/not routed through the gateway/i)).not.toBeInTheDocument();
    expect(vi.mocked(useListAgentAPIKeys)).toHaveBeenCalledWith(
      expect.objectContaining({ orgName: undefined }),
    );
  });

  const securityDisabled = () =>
    vi.mocked(useGetAgentConfigurations).mockReturnValue({
      data: { enableApiKeySecurity: false, enableOAuthSecurity: false }, isLoading: false,
    } as unknown as ReturnType<typeof useGetAgentConfigurations>);

  it("points an external A2A agent with security off to its gateway security settings", () => {
    externalA2A();
    securityDisabled();
    vi.mocked(useGetA2AProxy).mockReturnValue({
      data: { status: "DEPLOYED", statusReason: "", gatewayUrl: "https://gw.example/agent", upstreamAuthHeader: "" }, isLoading: false, isError: false, error: null,
    } as unknown as ReturnType<typeof useGetA2AProxy>);
    renderPage();
    expect(screen.getByText(/gateway security settings under Configure → A2A/)).toBeInTheDocument();
    expect(screen.queryByText(/redeploy/i)).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Go to A2A settings" })).toHaveAttribute("href", A2A_SETTINGS_HREF);
  });

  it("loads the keys of the route's agent and environment", () => {
    externalA2A();
    routedProxy();
    vi.mocked(useListAgentAPIKeys).mockReturnValue({
      data: [{ name: "first-key" }], isLoading: false, isError: false,
    } as unknown as ReturnType<typeof useListAgentAPIKeys>);
    renderPage();
    expect(screen.getByText("key first-key")).toBeInTheDocument();
    expect(vi.mocked(useListAgentAPIKeys)).toHaveBeenLastCalledWith({
      orgName: "o", projName: "p", agentName: "a", envId: "dev",
    });
  });

  it("shows a placeholder instead of keys or notices while loading", () => {
    vi.mocked(useGetAgent).mockReturnValue({
      data: undefined, isLoading: true, isError: false,
    } as unknown as ReturnType<typeof useGetAgent>);
    vi.mocked(useGetA2AProxy).mockReturnValue({
      data: undefined, isLoading: false, isError: false, error: null,
    } as unknown as ReturnType<typeof useGetA2AProxy>);
    renderPage();
    expect(screen.queryByText("api keys manager")).not.toBeInTheDocument();
    expect(screen.queryByText(/not routed through the gateway/i)).not.toBeInTheDocument();
  });

  it("explains that an OAuth-secured agent has no API keys", () => {
    externalA2A();
    routedProxy();
    vi.mocked(useGetAgentConfigurations).mockReturnValue({
      data: { enableApiKeySecurity: false, enableOAuthSecurity: true }, isLoading: false,
    } as unknown as ReturnType<typeof useGetAgentConfigurations>);
    renderPage();
    expect(screen.getByText("This agent uses OAuth")).toBeInTheDocument();
    expect(screen.queryByText("api keys manager")).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Go to A2A settings" })).not.toBeInTheDocument();
  });

  it("does not list keys for a user without the API key permission", () => {
    externalA2A();
    routedProxy();
    renderPage(false);
    expect(screen.getByText("You don't have access to API keys")).toBeInTheDocument();
    expect(screen.queryByText("api keys manager")).not.toBeInTheDocument();
    expect(vi.mocked(useListAgentAPIKeys)).toHaveBeenLastCalledWith({
      orgName: undefined, projName: undefined, agentName: undefined, envId: undefined,
    });
  });

  it("creates a key for the route's agent and environment and returns it", async () => {
    externalA2A();
    routedProxy();
    hoisted.createKey.mockResolvedValue({ apiKey: "secret-value", gatewayConnected: true });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "create key" }));
    await waitFor(() => expect(document.body.dataset.createdKey).toBe("secret-value"));
    expect(hoisted.createKey).toHaveBeenCalledWith({
      params: { orgName: "o", projName: "p", agentName: "a", envId: "dev" },
      body: { displayName: "CI key", expiresAt: "2027-01-01T00:00:00Z" },
    });
    expect(screen.queryByText(/gateway is not connected/i)).not.toBeInTheDocument();
  });

  it("warns that a key created while the gateway is offline works only after it reconnects", async () => {
    externalA2A();
    routedProxy();
    hoisted.createKey.mockResolvedValue({ apiKey: "secret-value", gatewayConnected: false });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "create key" }));
    expect(
      await screen.findByText(/gateway is not connected to the control plane/i),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /close/i }));
    await waitFor(() =>
      expect(screen.queryByText(/gateway is not connected/i)).not.toBeInTheDocument(),
    );
  });

  it("revokes a key in the route's agent and environment", () => {
    externalA2A();
    routedProxy();
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "revoke key" }));
    expect(hoisted.revokeKey).toHaveBeenCalledWith({
      orgName: "o", projName: "p", agentName: "a", envId: "dev", keyName: "old-key",
    });
  });
});
