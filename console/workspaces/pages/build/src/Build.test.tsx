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

import { render, screen, fireEvent } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { SnackBarProvider } from "@agent-management-platform/views";
import { vi } from "vitest";
import { BuildComponent } from "./index";

// BuildComponent (and AgentBuild/TopCards/BuildTable/BuildPanel/
// ConfigureBuildDrawer beneath it) call real TanStack Query hooks, which
// need a QueryClientProvider the real app only supplies at the shell level.
// Stub the api-client module boundary instead of wiring up react-query here.
vi.mock("@agent-management-platform/api-client", () => ({
  useGetAgent: vi.fn(),
  useGetAgentBuilds: vi.fn(() => ({ data: { builds: [] }, isLoading: false })),
  useBuildAgent: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  useListCommits: vi.fn(() => ({ data: { commits: [] }, isLoading: false, isError: false })),
  useUpdateAgentBuildParameters: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  useListGitSecrets: vi.fn(() => ({ data: { secrets: [] }, isLoading: false })),
}));

import { useGetAgent } from "@agent-management-platform/api-client";

const mockUseGetAgent = vi.mocked(useGetAgent);

const makeAgent = (provisioningType: string) => ({
  displayName: "Agent One",
  provisioning: {
    type: provisioningType,
    repository: { url: "https://github.com/acme/agent-one", branch: "main" },
  },
});

const route = "/org/org1/project/proj1/agents/agent1/build";

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
            path="/org/:orgId/project/:projectId/agents/:agentId/build"
            element={<BuildComponent />}
          />
        </Routes>
      </MemoryRouter>
    </SnackBarProvider>,
  );
}

describe("BuildComponent", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders the page title and the Trigger a Build action", () => {
    mockUseGetAgent.mockReturnValue({ data: makeAgent("external") } as unknown as ReturnType<
      typeof useGetAgent
    >);

    renderWithRouter();

    expect(screen.getByText("Build")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Trigger a Build" })).toBeInTheDocument();
  });

  it("hides the Configure Build action for externally-provisioned agents", () => {
    mockUseGetAgent.mockReturnValue({ data: makeAgent("external") } as unknown as ReturnType<
      typeof useGetAgent
    >);

    renderWithRouter();

    expect(screen.queryByRole("button", { name: "Configure Build" })).not.toBeInTheDocument();
  });

  it("shows the Configure Build action for internally-provisioned agents", () => {
    mockUseGetAgent.mockReturnValue({ data: makeAgent("internal") } as unknown as ReturnType<
      typeof useGetAgent
    >);

    renderWithRouter();

    expect(screen.getByRole("button", { name: "Configure Build" })).toBeInTheDocument();
  });

  it("opens the trigger-build drawer when Trigger a Build is clicked", () => {
    mockUseGetAgent.mockReturnValue({ data: makeAgent("external") } as unknown as ReturnType<
      typeof useGetAgent
    >);

    renderWithRouter();
    fireEvent.click(screen.getByRole("button", { name: "Trigger a Build" }));

    expect(screen.getByRole("heading", { name: "Trigger Build" })).toBeInTheDocument();
    expect(screen.getByText(/Build Agent One from a specific commit/)).toBeInTheDocument();
  });

  it("opens the configure-build drawer when Configure Build is clicked", () => {
    mockUseGetAgent.mockReturnValue({ data: makeAgent("internal") } as unknown as ReturnType<
      typeof useGetAgent
    >);

    renderWithRouter();
    fireEvent.click(screen.getByRole("button", { name: "Configure Build" }));

    expect(screen.getByRole("heading", { name: "Configure Build" })).toBeInTheDocument();
  });
});
