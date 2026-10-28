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

import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@agent-management-platform/api-client", () => ({
  useCreateAgentAPIKey: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useGetA2AProxy: vi.fn(),
  useGetAgent: vi.fn(),
  useGetAgentConfigurations: vi.fn(() => ({
    data: { enableApiKeySecurity: true },
    isLoading: false,
  })),
  useListAgentDeployments: vi.fn(() => ({ data: undefined, isLoading: false })),
  useListAgentAPIKeys: vi.fn(() => ({ data: [], isLoading: false, isError: false })),
  useRevokeAgentAPIKey: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
}));
vi.mock("@agent-management-platform/shared-component", () => ({
  APIKeysManager: () => <div>api keys manager</div>,
  EnvironmentSelector: () => null,
}));
vi.mock("@agent-management-platform/views", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@agent-management-platform/views")>()),
  PageLayout: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

import {
  useGetA2AProxy,
  useGetAgent,
  useGetAgentConfigurations,
  useListAgentDeployments,
  useListAgentAPIKeys,
} from "@agent-management-platform/api-client";
import { SecurityComponent } from "./Security.Component";

const renderPage = () =>
  render(
    <MemoryRouter initialEntries={["/org/o/project/p/agents/a/environment/dev/security"]}>
      <Routes>
        <Route path="/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/security" element={<SecurityComponent />} />
      </Routes>
    </MemoryRouter>,
  );

const externalA2A = () =>
  vi.mocked(useGetAgent).mockReturnValue({
    data: { provisioning: { type: "external" }, agentType: { type: "external-agent-api", subType: "a2a-agent" } },
    isLoading: false,
  } as unknown as ReturnType<typeof useGetAgent>);

const A2A_SETTINGS_HREF = "/org/o/project/p/agents/a/configure?tab=a2a&env=dev";

beforeEach(() => vi.clearAllMocks());

describe("SecurityComponent", () => {
  it("manages API keys of an external A2A agent routed through the gateway", () => {
    externalA2A();
    vi.mocked(useGetA2AProxy).mockReturnValue({
      data: { status: "published", lastError: "", gatewayUrl: "https://gw.example/agent", upstreamAuthHeader: "" }, isLoading: false, isError: false, error: null,
    } as unknown as ReturnType<typeof useGetA2AProxy>);
    renderPage();
    expect(screen.getByText("api keys manager")).toBeInTheDocument();
    expect(vi.mocked(useListAgentDeployments)).toHaveBeenCalledWith(
      expect.objectContaining({ orgName: undefined }),
    );
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

  it("reports a failed agent lookup instead of saying the agent is not deployed", () => {
    vi.mocked(useGetAgent).mockReturnValue({
      data: undefined, isLoading: false, isError: true, error: new Error("boom"),
    } as unknown as ReturnType<typeof useGetAgent>);
    vi.mocked(useGetA2AProxy).mockReturnValue({
      data: undefined, isLoading: false, isError: false, error: null,
    } as unknown as ReturnType<typeof useGetA2AProxy>);
    renderPage();
    expect(screen.getByText(/could not load the agent/i)).toBeInTheDocument();
    expect(screen.queryByText(/not deployed/i)).not.toBeInTheDocument();
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
      data: { status: "published", lastError: "", gatewayUrl: "https://gw.example/agent", upstreamAuthHeader: "" }, isLoading: false, isError: false, error: null,
    } as unknown as ReturnType<typeof useGetA2AProxy>);
    renderPage();
    expect(screen.getByText(/gateway security settings under Configure → A2A/)).toBeInTheDocument();
    expect(screen.queryByText(/redeploy/i)).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Go to A2A settings" })).toHaveAttribute("href", A2A_SETTINGS_HREF);
  });

  it("points a deployed agent with security off to its deployment settings", () => {
    vi.mocked(useGetAgent).mockReturnValue({
      data: { provisioning: { type: "internal" }, agentType: { type: "agent-api", subType: "chat-api" } },
      isLoading: false,
    } as unknown as ReturnType<typeof useGetAgent>);
    vi.mocked(useGetA2AProxy).mockReturnValue({
      data: undefined, isLoading: false, isError: false, error: null,
    } as unknown as ReturnType<typeof useGetA2AProxy>);
    vi.mocked(useListAgentDeployments).mockReturnValue({
      data: { dev: { status: "active" } }, isLoading: false,
    } as unknown as ReturnType<typeof useListAgentDeployments>);
    securityDisabled();
    renderPage();
    expect(screen.getByText(/deployment settings and redeploy/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Go to Deployment Settings" })).toBeInTheDocument();
  });
});
