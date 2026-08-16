/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { SnackBarProvider } from "@agent-management-platform/views";
import { vi } from "vitest";
import { TestComponent } from "./index";

// TestComponent (and the AgentChat/Swagger/EnvironmentSelector children it
// renders) call real TanStack Query hooks, which need a QueryClientProvider
// the real app only supplies at the shell level. Stub the api-client module
// boundary instead of wiring up react-query here.
vi.mock("@agent-management-platform/api-client", () => ({
  useGetAgent: vi.fn(),
  useGetAgentConfigurations: vi.fn(),
  useListAgentDeployments: vi.fn(),
  useGetAgentEndpoints: vi.fn(),
  useTestAgentAPIKey: vi.fn(),
  // Consumed by EnvironmentSelector (PageLayout's actions) via
  // usePipelineEnvironments.
  useListEnvironments: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
  useGetProject: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
  useListDeploymentPipelines: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
}));

import {
  useGetAgent,
  useGetAgentConfigurations,
  useListAgentDeployments,
  useGetAgentEndpoints,
  useTestAgentAPIKey,
} from "@agent-management-platform/api-client";

const mockUseGetAgent = vi.mocked(useGetAgent);
const mockUseGetAgentConfigurations = vi.mocked(useGetAgentConfigurations);
const mockUseListAgentDeployments = vi.mocked(useListAgentDeployments);
const mockUseGetAgentEndpoints = vi.mocked(useGetAgentEndpoints);
const mockUseTestAgentAPIKey = vi.mocked(useTestAgentAPIKey);

// Minimal stand-in for a react-query UseQueryResult, with just the fields
// the components under test actually destructure.
const asQueryResult = (overrides: Record<string, unknown>) =>
  ({
    data: undefined,
    isLoading: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
    ...overrides,
  }) as any;

const route = "/org/org1/project/proj1/agents/agent1/environment/env1/tryOut";

function renderTestPage() {
  return render(
    <SnackBarProvider>
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }} initialEntries={[route]} initialIndex={0}>
        <Routes>
          <Route
            path="/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/tryOut"
            element={<TestComponent />}
          />
        </Routes>
      </MemoryRouter>
    </SnackBarProvider>
  );
}

describe("TestComponent", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // AgentChat / Swagger both call these regardless of agent type; give
    // them an inert default and let individual tests override as needed.
    mockUseGetAgentEndpoints.mockReturnValue(asQueryResult({ data: {} }));
    mockUseTestAgentAPIKey.mockReturnValue(asQueryResult({}));
  });

  it("shows the not-deployed empty state when the environment has no active deployment", () => {
    mockUseGetAgent.mockReturnValue(
      asQueryResult({ data: { agentType: { subType: "chat-api" } } }),
    );
    mockUseListAgentDeployments.mockReturnValue(asQueryResult({ data: {} }));
    mockUseGetAgentConfigurations.mockReturnValue(asQueryResult({}));

    renderTestPage();

    expect(screen.getByText("Try your agent")).toBeInTheDocument();
    expect(screen.getByText("Agent is not deployed")).toBeInTheDocument();
    expect(
      screen.getByText(/Deploy your agent to try it out/),
    ).toBeInTheDocument();
  });

  it("renders a loading skeleton with no title while agent/deployment data is loading", () => {
    mockUseGetAgent.mockReturnValue(asQueryResult({ isLoading: true }));
    mockUseListAgentDeployments.mockReturnValue(
      asQueryResult({ isLoading: true }),
    );
    mockUseGetAgentConfigurations.mockReturnValue(
      asQueryResult({ isLoading: true }),
    );

    renderTestPage();

    // PageLayout swaps the title text for a skeleton entirely while loading.
    expect(screen.queryByText("Try your agent")).not.toBeInTheDocument();
    expect(screen.queryByText("Agent is not deployed")).not.toBeInTheDocument();
  });

  it("renders the chat UI for a chat-api agent with an active deployment", () => {
    mockUseGetAgent.mockReturnValue(
      asQueryResult({ data: { agentType: { subType: "chat-api" } } }),
    );
    mockUseListAgentDeployments.mockReturnValue(
      asQueryResult({ data: { env1: { status: "active" } } }),
    );
    mockUseGetAgentConfigurations.mockReturnValue(
      asQueryResult({ data: { enableApiKeySecurity: false } }),
    );

    renderTestPage();

    expect(screen.getByText("Try your agent")).toBeInTheDocument();
    expect(screen.getByText("Start a conversation")).toBeInTheDocument();
  });

  it("renders the Swagger tester's no-schema state for a non-chat agent with an active deployment", () => {
    mockUseGetAgent.mockReturnValue(
      asQueryResult({ data: { agentType: { subType: "rest-api" } } }),
    );
    mockUseListAgentDeployments.mockReturnValue(
      asQueryResult({ data: { env1: { status: "active" } } }),
    );
    mockUseGetAgentConfigurations.mockReturnValue(
      asQueryResult({ data: { enableApiKeySecurity: false } }),
    );
    mockUseGetAgentEndpoints.mockReturnValue(asQueryResult({ data: {} }));

    renderTestPage();

    expect(
      screen.getByText("No API schema available for this endpoint."),
    ).toBeInTheDocument();
  });

  it("shows an error alert when the environment's security configuration fails to load", () => {
    mockUseGetAgent.mockReturnValue(
      asQueryResult({ data: { agentType: { subType: "chat-api" } } }),
    );
    mockUseListAgentDeployments.mockReturnValue(
      asQueryResult({ data: { env1: { status: "active" } } }),
    );
    mockUseGetAgentConfigurations.mockReturnValue(
      asQueryResult({ isError: true }),
    );

    renderTestPage();

    expect(
      screen.getByText(/Could not load this environment.s security settings/),
    ).toBeInTheDocument();
  });
});
