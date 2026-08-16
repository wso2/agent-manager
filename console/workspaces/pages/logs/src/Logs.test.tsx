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
import { LogsComponent } from "./index";

// LogsComponent (and the EnvironmentSelector it renders in its actions bar)
// call real TanStack Query hooks, which need a QueryClientProvider the real
// app supplies at the shell level. Stub the api-client module boundary
// instead of wiring up react-query here.
vi.mock("@agent-management-platform/api-client", () => ({
  useAgentRuntimeLogs: vi.fn(() => ({
    logs: [],
    error: null,
    isLoading: false,
    isRefetching: false,
    refetch: vi.fn(),
    isLoadingOlder: false,
    isLoadingNewer: false,
    loadOlder: vi.fn(),
    loadNewer: vi.fn(),
    hasMoreOlder: false,
    hasMoreNewer: false,
  })),
  isObserverConfigured: vi.fn(() => true),
  // Consumed by EnvironmentSelector via usePipelineEnvironments.
  useListEnvironments: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
  useGetProject: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
  useListDeploymentPipelines: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
}));

const route =
  "/org/org1/project/proj1/agents/agent1/environment/env1/observability/logs";

function renderWithRouter(initialEntry = route) {
  return render(
    <SnackBarProvider>
      <MemoryRouter initialEntries={[initialEntry]} initialIndex={0}>
        <Routes>
          <Route
            path="/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/observability/logs"
            element={<LogsComponent />}
          />
        </Routes>
      </MemoryRouter>
    </SnackBarProvider>
  );
}

describe("LogsComponent", () => {
  it("renders without crashing", () => {
    renderWithRouter();
    expect(screen.getByText("Runtime Logs")).toBeInTheDocument();
  });

  it("renders the log level filter", () => {
    renderWithRouter();
    expect(screen.getByText("All Levels")).toBeInTheDocument();
  });

  it("renders refresh and sort controls", () => {
    renderWithRouter();
    expect(screen.getByLabelText("Refresh")).toBeInTheDocument();
    expect(
      screen.getByLabelText(/Sort (ascending|descending)/)
    ).toBeInTheDocument();
  });
});
