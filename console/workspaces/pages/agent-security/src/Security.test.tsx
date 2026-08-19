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
import { SnackBarProvider } from "@agent-management-platform/views";
import { vi } from "vitest";
import { SecurityComponent } from "./index";

// SecurityComponent (and the EnvironmentSelector it renders in its actions
// bar) call real TanStack Query hooks, which need a QueryClientProvider the
// real app only supplies at the shell level. Stub the api-client module
// boundary instead of wiring up react-query here.
vi.mock("@agent-management-platform/api-client", () => ({
  useGetAgentConfigurations: vi.fn(),
  useListAgentDeployments: vi.fn(),
  useListAgentAPIKeys: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
  useCreateAgentAPIKey: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useRevokeAgentAPIKey: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  // Consumed by EnvironmentSelector via usePipelineEnvironments.
  useListEnvironments: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
  useGetProject: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
  useListDeploymentPipelines: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
}));

import {
  useGetAgentConfigurations,
  useListAgentDeployments,
  useListAgentAPIKeys,
} from "@agent-management-platform/api-client";

const mockUseGetAgentConfigurations = vi.mocked(useGetAgentConfigurations);
const mockUseListAgentDeployments = vi.mocked(useListAgentDeployments);
const mockUseListAgentAPIKeys = vi.mocked(useListAgentAPIKeys);

const asQueryResult = (overrides: Record<string, unknown>) =>
  ({ data: undefined, isLoading: false, isError: false, ...overrides }) as any;

const route =
  "/org/org1/project/proj1/agents/agent1/environment/env1/security";

function renderWithRouter() {
  return render(
    <SnackBarProvider>
      <MemoryRouter
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
        initialEntries={[route]}
        initialIndex={0}
      >
        <Routes>
          <Route
            path="/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/security"
            element={<SecurityComponent />}
          />
        </Routes>
      </MemoryRouter>
    </SnackBarProvider>,
  );
}

describe("SecurityComponent", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows the not-deployed empty state when the environment has no active deployment", () => {
    mockUseGetAgentConfigurations.mockReturnValue(
      asQueryResult({ data: { enableApiKeySecurity: true } }),
    );
    mockUseListAgentDeployments.mockReturnValue(asQueryResult({ data: {} }));

    renderWithRouter();

    expect(screen.getByText("API Keys")).toBeInTheDocument();
    expect(screen.getByText("Agent is not deployed")).toBeInTheDocument();
  });

  it("shows the security-disabled empty state when the deployed environment has API key security off", () => {
    mockUseGetAgentConfigurations.mockReturnValue(
      asQueryResult({
        data: { enableApiKeySecurity: false, enableOAuthSecurity: false },
      }),
    );
    mockUseListAgentDeployments.mockReturnValue(
      asQueryResult({ data: { env1: { status: "active" } } }),
    );

    renderWithRouter();

    expect(screen.getByText("Agent security is disabled")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Go to Deployment Settings" }),
    ).toBeInTheDocument();
  });

  it("shows the OAuth empty state when the agent uses OAuth instead of API keys", () => {
    mockUseGetAgentConfigurations.mockReturnValue(
      asQueryResult({
        data: { enableApiKeySecurity: false, enableOAuthSecurity: true },
      }),
    );
    mockUseListAgentDeployments.mockReturnValue(
      asQueryResult({ data: { env1: { status: "active" } } }),
    );

    renderWithRouter();

    expect(screen.getByText("This agent uses OAuth")).toBeInTheDocument();
  });

  it("lists API keys for a deployed environment with API key security enabled", () => {
    mockUseGetAgentConfigurations.mockReturnValue(
      asQueryResult({ data: { enableApiKeySecurity: true } }),
    );
    mockUseListAgentDeployments.mockReturnValue(
      asQueryResult({ data: { env1: { status: "active" } } }),
    );
    mockUseListAgentAPIKeys.mockReturnValue(
      asQueryResult({
        data: [
          {
            name: "key-1",
            displayName: "CI Key",
            maskedApiKey: "sk-****1234",
            status: "active",
            expiresAt: undefined,
          },
        ],
      }),
    );

    renderWithRouter();

    expect(screen.getByText("CI Key")).toBeInTheDocument();
    expect(screen.getByText("sk-****1234")).toBeInTheDocument();
    expect(screen.getByText("Never expires")).toBeInTheDocument();
  });
});
