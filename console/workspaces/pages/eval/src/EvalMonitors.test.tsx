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

import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { SnackBarProvider } from "@agent-management-platform/views";
import { ConfirmationDialogProvider } from "@agent-management-platform/shared-component";
import { vi } from "vitest";
import { EvalMonitorsComponent } from "./index";

// EvalMonitorsComponent (via MonitorTable and the EnvironmentSelector it
// renders in its actions bar) calls real TanStack Query hooks, which need a
// QueryClientProvider the real app only supplies at the shell level. Stub
// the api-client module boundary instead of wiring up react-query here.
vi.mock("@agent-management-platform/api-client", () => ({
  useListMonitors: vi.fn(() => ({ data: undefined, isLoading: false, error: null })),
  useDeleteMonitor: vi.fn(() => ({ mutate: vi.fn() })),
  // Rendered per-row by MonitorTable via MonitorStartStopButton.
  useStartMonitor: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  useStopMonitor: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  // Consumed by EnvironmentSelector via usePipelineEnvironments.
  useListEnvironments: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
  useGetProject: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
  useListDeploymentPipelines: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
}));

import { useListMonitors } from "@agent-management-platform/api-client";

const mockUseListMonitors = vi.mocked(useListMonitors);

const route =
  "/org/org1/project/proj1/agents/agent1/environment/env1/evaluation/monitor";

function renderWithRouter() {
  return render(
    <SnackBarProvider>
      <ConfirmationDialogProvider>
        <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }} initialEntries={[route]} initialIndex={0}>
          <Routes>
            <Route
              path="/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/evaluation/monitor"
              element={<EvalMonitorsComponent />}
            />
          </Routes>
        </MemoryRouter>
      </ConfirmationDialogProvider>
    </SnackBarProvider>,
  );
}

describe("EvalMonitorsComponent", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders the page title and the Add monitor action", () => {
    mockUseListMonitors.mockReturnValue({
      data: { monitors: [] },
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useListMonitors>);

    renderWithRouter();

    expect(screen.getByText("Eval Monitors")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Add monitor/i })).toBeInTheDocument();
  });

  it("shows the empty state when there are no monitors", () => {
    mockUseListMonitors.mockReturnValue({
      data: { monitors: [] },
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useListMonitors>);

    renderWithRouter();

    expect(screen.getByText("No monitors yet")).toBeInTheDocument();
  });

  it("lists monitors by display name once data loads", () => {
    mockUseListMonitors.mockReturnValue({
      data: {
        monitors: [
          {
            id: "mon-1",
            name: "prod-quality",
            displayName: "Production Quality",
            type: "future",
            status: "active",
            evaluators: ["accuracy"],
          },
        ],
      },
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useListMonitors>);

    renderWithRouter();

    expect(screen.getByText("Production Quality")).toBeInTheDocument();
  });
});
