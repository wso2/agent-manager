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
import { TracesComponent } from "./index";

// TracesComponent (and the EnvironmentSelector it renders in its actions bar)
// call real TanStack Query hooks, which need a QueryClientProvider the real
// app supplies at the shell level. Stub the api-client module boundary
// instead of wiring up react-query here.
vi.mock("@agent-management-platform/api-client", () => ({
  useTraceList: vi.fn(() => ({
    data: { traces: [], totalCount: 0 },
    isLoading: false,
    isRefetching: false,
    refetch: vi.fn(),
    loadOlder: vi.fn(),
    loadNewer: vi.fn(),
    isLoadingOlder: false,
    isLoadingNewer: false,
  })),
  useExportTraces: vi.fn(() => ({
    mutateAsync: vi.fn(),
    isPending: false,
  })),
  useGetAgent: vi.fn(() => ({
    data: { uuid: "agent-uuid" },
    isPending: false,
    isSuccess: true,
  })),
  useGetOrganization: vi.fn(() => ({
    data: { namespace: "org1" },
    isPending: false,
    isSuccess: true,
  })),
  isObserverConfigured: vi.fn(() => true),
  // Consumed by TracesComponent directly and by EnvironmentSelector via
  // usePipelineEnvironments.
  useListEnvironments: vi.fn(() => ({
    data: [{ name: "env1", dataplaneRef: "dp1", isProduction: false, createdAt: "2026-01-01" }],
    isLoading: false,
    isPending: false,
    isSuccess: true,
    isError: false,
  })),
  useGetProject: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
  useListDeploymentPipelines: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
}));

const route =
  "/org/org1/project/proj1/agents/agent1/environment/env1/observability/traces";

function renderWithRouter(initialEntry = route) {
  return render(
    <SnackBarProvider>
      <MemoryRouter
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
        initialEntries={[initialEntry]}
        initialIndex={0}
      >
        <Routes>
          <Route
            path="/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/observability/traces"
            element={<TracesComponent />}
          />
        </Routes>
      </MemoryRouter>
    </SnackBarProvider>
  );
}

describe("TracesComponent", () => {
  it("renders the page title", () => {
    renderWithRouter();
    expect(screen.getByText("Traces")).toBeInTheDocument();
  });

  it("renders the empty state when there are no traces", () => {
    renderWithRouter();
    expect(screen.getByText("No traces found!")).toBeInTheDocument();
  });

  it("renders refresh, sort and export controls", () => {
    renderWithRouter();
    expect(screen.getByLabelText("Refresh")).toBeInTheDocument();
    expect(
      screen.getByLabelText(/Sort (ascending|descending)/)
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Export/i })).toBeInTheDocument();
  });

  it("disables the export button when there are no traces to export", () => {
    renderWithRouter();
    expect(screen.getByRole("button", { name: /Export/i })).toBeDisabled();
  });

  it("shows an observer-not-configured message when the observer URL is unset", async () => {
    const apiClient = await import("@agent-management-platform/api-client");
    vi.mocked(apiClient.isObserverConfigured).mockReturnValueOnce(false);
    renderWithRouter();
    expect(screen.getByText("Observer not configured.")).toBeInTheDocument();
  });
});
