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
import { OverviewComponent } from "./index";

// OverviewComponent (via AgentOverview/EditAgentDrawer) calls real TanStack
// Query hooks, which need a QueryClientProvider the real app only supplies
// at the shell level. Stub the api-client module boundary instead.
vi.mock("@agent-management-platform/api-client", () => ({
  useGetAgent: vi.fn(),
  useUserDisplayName: vi.fn(() => undefined),
  useUpdateAgent: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  // Consumed by ExternalAgentOverview via usePipelineEnvironmentsState.
  useListEnvironments: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
  useGetProject: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
  useListDeploymentPipelines: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
  useListGateways: vi.fn(() => ({ data: undefined, isLoading: false })),
}));

import { useGetAgent } from "@agent-management-platform/api-client";

const mockUseGetAgent = vi.mocked(useGetAgent);

const route = "/org/org1/project/proj1/agents/agent1";

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
            path="/org/:orgId/project/:projectId/agents/:agentId"
            element={<OverviewComponent />}
          />
        </Routes>
      </MemoryRouter>
    </SnackBarProvider>,
  );
}

describe("OverviewComponent", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders a loading skeleton instead of the agent's details while it's loading", () => {
    mockUseGetAgent.mockReturnValue({
      data: undefined,
      isLoading: true,
    } as unknown as ReturnType<typeof useGetAgent>);

    renderWithRouter();

    // PageLayout swaps the title and actions for skeletons entirely while loading.
    expect(screen.queryByText("Agent")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Edit Agent/i })).not.toBeInTheDocument();
  });

  it("renders the agent's display name and description once loaded", () => {
    mockUseGetAgent.mockReturnValue({
      data: {
        displayName: "My Agent",
        description: "Does useful things.",
        provisioning: { type: "external" },
        createdAt: "2026-01-01T00:00:00Z",
        createdBy: { id: "user-1", display: "Jane Doe" },
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useGetAgent>);

    renderWithRouter();

    expect(screen.getByText("My Agent")).toBeInTheDocument();
    expect(screen.getByText("Does useful things.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Edit Agent/i })).toBeEnabled();
  });

  it("shows the agent's custom tags as chips when it has labels", () => {
    mockUseGetAgent.mockReturnValue({
      data: {
        displayName: "My Agent",
        provisioning: { type: "external" },
        labels: { team: "platform" },
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useGetAgent>);

    renderWithRouter();

    // LabelChips renders an aria-hidden off-screen copy of every chip (to
    // measure natural width) alongside the visible one, so more than one
    // match for the same label text is expected here.
    expect(screen.getAllByText("team: platform").length).toBeGreaterThan(0);
  });

  it("omits the custom tags row when the agent has no labels", () => {
    mockUseGetAgent.mockReturnValue({
      data: {
        displayName: "My Agent",
        provisioning: { type: "external" },
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useGetAgent>);

    renderWithRouter();

    expect(screen.queryByText(/team/)).not.toBeInTheDocument();
  });

  it("opens the edit agent drawer when Edit Agent is clicked", () => {
    mockUseGetAgent.mockReturnValue({
      data: {
        displayName: "My Agent",
        provisioning: { type: "external" },
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useGetAgent>);

    renderWithRouter();
    fireEvent.click(screen.getByRole("button", { name: /Edit Agent/i }));

    expect(screen.getByRole("heading", { name: "Edit Agent" })).toBeInTheDocument();
  });
});
