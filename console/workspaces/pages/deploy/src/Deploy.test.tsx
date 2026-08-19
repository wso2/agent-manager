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
import { DeployComponent } from "./index";

// DeployComponent (and the BuildCard/DeployCard it renders) call real
// TanStack Query hooks, which need a QueryClientProvider the real app only
// wires up at the shell level. Those hooks also route through a shared
// react-query notifications utility that calls useSnackBar(), and two
// subComponents (EditDeployConfigDrawer, EditResourceConfigsDrawer) call
// useSnackBar() directly. Stub the api-client module boundary instead of
// wiring up react-query, and wrap renders in SnackBarProvider.
//
// Only the hooks actually reached by the scenarios below are stubbed.
// BuildCard's "no builds" branch and DeployCard's "no deployment" branch
// both return early, before EditDeployConfigDrawer / BuildSelectorDrawer /
// EditResourceConfigsDrawer / PromoteAgentDrawer ever mount, so those
// components' own hooks (useAgentBuildOptions, useDeployAgent,
// usePromoteAgent, useUpdateAgentResourceConfigs, etc.) are never invoked
// and don't need stubs here.
vi.mock("@agent-management-platform/api-client", () => ({
  useGetProject: vi.fn(() => ({ data: undefined, isLoading: false })),
  useListDeploymentPipelines: vi.fn(() => ({ data: undefined, isLoading: false })),
  useListEnvironments: vi.fn(() => ({ data: undefined, isLoading: false })),
  useGetAgent: vi.fn(() => ({ data: undefined, isLoading: false })),
  useGetAgentBuilds: vi.fn(() => ({ data: undefined, isLoading: false })),
  useGetAgentKind: vi.fn(() => ({ data: undefined, isLoading: false })),
  useListAgentDeployments: vi.fn(() => ({ data: undefined, isLoading: false })),
  useGetDeploymentPipeline: vi.fn(() => ({ data: undefined, isLoading: false })),
  useUpdateDeploymentState: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  useGetAgentResourceConfigs: vi.fn(() => ({ data: undefined, isLoading: false })),
  useGetAgentMetrics: vi.fn(() => ({ data: undefined, isLoading: false })),
  useGetAgentConfigurations: vi.fn(() => ({ data: undefined, isLoading: false })),
  useListAgentKindVersions: vi.fn(() => ({ data: undefined, isLoading: false })),
}));

import {
  useGetProject,
  useListDeploymentPipelines,
  useListEnvironments,
  useGetAgent,
  useGetAgentBuilds,
  useListAgentDeployments,
} from "@agent-management-platform/api-client";

const mockUseGetProject = vi.mocked(useGetProject);
const mockUseListDeploymentPipelines = vi.mocked(useListDeploymentPipelines);
const mockUseListEnvironments = vi.mocked(useListEnvironments);
const mockUseGetAgent = vi.mocked(useGetAgent);
const mockUseGetAgentBuilds = vi.mocked(useGetAgentBuilds);
const mockUseListAgentDeployments = vi.mocked(useListAgentDeployments);

const route = "/org/org1/project/proj1/agents/agent1/deploy";

function renderDeployComponent() {
  return render(
    <SnackBarProvider>
      <MemoryRouter
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
        initialEntries={[route]}
        initialIndex={0}
      >
        <Routes>
          <Route
            path="/org/:orgId/project/:projectId/agents/:agentId/deploy"
            element={<DeployComponent />}
          />
        </Routes>
      </MemoryRouter>
    </SnackBarProvider>
  );
}

describe("DeployComponent", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseGetProject.mockReturnValue({ data: undefined, isLoading: false } as ReturnType<
      typeof useGetProject
    >);
    mockUseListDeploymentPipelines.mockReturnValue({
      data: undefined,
      isLoading: false,
    } as ReturnType<typeof useListDeploymentPipelines>);
    mockUseListEnvironments.mockReturnValue({ data: undefined, isLoading: false } as ReturnType<
      typeof useListEnvironments
    >);
    mockUseGetAgent.mockReturnValue({ data: undefined, isLoading: false } as ReturnType<
      typeof useGetAgent
    >);
    mockUseGetAgentBuilds.mockReturnValue({ data: undefined, isLoading: false } as ReturnType<
      typeof useGetAgentBuilds
    >);
    mockUseListAgentDeployments.mockReturnValue({
      data: undefined,
      isLoading: false,
    } as ReturnType<typeof useListAgentDeployments>);
  });

  it("renders the Deploy page title", () => {
    renderDeployComponent();
    expect(screen.getByText("Deploy")).toBeInTheDocument();
  });

  it("shows the BuildCard empty state when the agent has no builds", () => {
    // No environments configured either, so only BuildCard renders.
    renderDeployComponent();
    expect(screen.getByText("No builds available")).toBeInTheDocument();
    expect(
      screen.getByText("build your agent first to deploy it to an environment.")
    ).toBeInTheDocument();
  });

  it("shows the BuildCard setup panel with the selected build once builds exist", async () => {
    mockUseGetAgentBuilds.mockReturnValue({
      data: {
        builds: [
          {
            buildName: "build-42",
            projectName: "proj1",
            agentName: "agent1",
            startedAt: "2026-01-01T00:00:00Z",
            status: "Completed",
            buildParameters: {
              repoUrl: "https://example.com/repo.git",
              appPath: "/",
              branch: "main",
              commitId: "abcdef1234567890",
              language: "python",
              languageVersion: "3.11",
              runCommand: "python main.py",
            },
          },
        ],
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useGetAgentBuilds>);

    renderDeployComponent();

    expect(await screen.findByText("Select Build")).toBeInTheDocument();
    expect(await screen.findByText("build-42")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /configure & deploy/i })
    ).toBeInTheDocument();
  });

  it("shows the DeployCard no-deployment state for a pipeline environment", () => {
    mockUseListEnvironments.mockReturnValue({
      data: [
        {
          name: "development",
          displayName: "Development",
          dataplaneRef: "dp-1",
          isProduction: false,
          createdAt: "2026-01-01T00:00:00Z",
        },
      ],
      isLoading: false,
    } as unknown as ReturnType<typeof useListEnvironments>);
    mockUseListAgentDeployments.mockReturnValue({
      data: {},
      isLoading: false,
    } as unknown as ReturnType<typeof useListAgentDeployments>);

    renderDeployComponent();

    expect(screen.getByText("No Deployment found")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Build your agent first to deploy it to Development environment."
      )
    ).toBeInTheDocument();
  });

  it("renders both BuildCard and DeployCard together for a configured environment", () => {
    mockUseListEnvironments.mockReturnValue({
      data: [
        {
          name: "development",
          displayName: "Development",
          dataplaneRef: "dp-1",
          isProduction: false,
          createdAt: "2026-01-01T00:00:00Z",
        },
      ],
      isLoading: false,
    } as unknown as ReturnType<typeof useListEnvironments>);
    mockUseListAgentDeployments.mockReturnValue({
      data: {},
      isLoading: false,
    } as unknown as ReturnType<typeof useListAgentDeployments>);

    renderDeployComponent();

    // BuildCard (no builds) and DeployCard (no deployment) both render as
    // side-by-side cards under the single "Deploy" page.
    expect(screen.getByText("Deploy")).toBeInTheDocument();
    expect(screen.getByText("No builds available")).toBeInTheDocument();
    expect(screen.getByText("No Deployment found")).toBeInTheDocument();
  });
});
