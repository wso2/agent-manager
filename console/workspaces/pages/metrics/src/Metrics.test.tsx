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
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";

// api-client reads the runtime config at import time, which a test has no
// app shell to provide, and EnvironmentSelector pulls it in too; stub both at
// the module boundary (same approach as the traces tests).
vi.mock("@agent-management-platform/api-client", () => ({
  useGetAgentMetrics: vi.fn(() => ({
    data: undefined,
    error: null,
    isLoading: false,
    isRefetching: false,
    refetch: vi.fn(),
  })),
  useListAgentDeployments: vi.fn(() => ({
    data: { env1: { status: "active" } },
  })),
  isObserverConfigured: vi.fn(() => true),
  useTrack: vi.fn(() => ({ track: vi.fn() })),
  ConsoleAction: { MetricsRangeChanged: "metrics_range_changed" },
}));
vi.mock("@agent-management-platform/shared-component", () => ({
  EnvironmentSelector: () => null,
}));

import {
  isObserverConfigured,
  useListAgentDeployments,
} from "@agent-management-platform/api-client";
import { MetricsComponent } from "./index";

const route =
  "/org/org1/project/proj1/agents/agent1/observability/metrics";

function renderWithRouter(initialEntry = route) {
  return render(
    <ThemeProvider theme={createTheme()}>
      <MemoryRouter initialEntries={[initialEntry]} initialIndex={0}>
        <Routes>
          <Route
            path="/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/observability/metrics"
            element={<MetricsComponent />}
          />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );
}

describe("MetricsComponent", () => {
  beforeEach(() => {
    vi.mocked(isObserverConfigured).mockReturnValue(true);
    vi.mocked(useListAgentDeployments).mockReturnValue({
      data: { env1: { status: "active" } },
    } as unknown as ReturnType<typeof useListAgentDeployments>);
  });

  it("renders the page title", () => {
    renderWithRouter();
    expect(screen.getByText("System Metrics")).toBeInTheDocument();
  });

  it("renders refresh control", () => {
    renderWithRouter();
    expect(screen.getByLabelText("Refresh")).toBeInTheDocument();
  });

  it("shows the suspended state for a suspended environment", () => {
    vi.mocked(useListAgentDeployments).mockReturnValue({
      data: { env1: { status: "suspended" } },
    } as unknown as ReturnType<typeof useListAgentDeployments>);
    renderWithRouter();
    expect(screen.getByText("Environment Suspended")).toBeInTheDocument();
  });

  it("explains a missing observer instead of rendering controls", () => {
    vi.mocked(isObserverConfigured).mockReturnValue(false);
    renderWithRouter();
    expect(screen.getByText("Observer not configured.")).toBeInTheDocument();
    expect(screen.queryByLabelText("Refresh")).not.toBeInTheDocument();
  });
});
