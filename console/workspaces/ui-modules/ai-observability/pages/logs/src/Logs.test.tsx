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
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";

// api-client reads the runtime config at import time, which a test has no
// app shell to provide, and EnvironmentSelector pulls it in too; stub both at
// the module boundary (same approach as the traces tests).
vi.mock("@agent-management-platform/api-client", () => ({
  useAgentRuntimeLogs: vi.fn(() => ({
    logs: [],
    isLoading: false,
    error: null,
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
  useTrack: vi.fn(() => ({ track: vi.fn() })),
  ConsoleAction: { LogQuery: "log_query" },
}));
vi.mock("@agent-management-platform/shared-component", () => ({
  EnvironmentSelector: () => null,
}));

import { isObserverConfigured } from "@agent-management-platform/api-client";
import { LogsComponent } from "./index";

const route =
  "/org/org1/project/proj1/agents/agent1/environment/env1/observability/logs";

function renderWithRouter(initialEntry = route) {
  return render(
    <ThemeProvider theme={createTheme()}>
      <MemoryRouter initialEntries={[initialEntry]} initialIndex={0}>
        <Routes>
          <Route
            path="/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/observability/logs"
            element={<LogsComponent />}
          />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );
}

describe("LogsComponent", () => {
  beforeEach(() => {
    vi.mocked(isObserverConfigured).mockReturnValue(true);
  });

  it("renders the page title", () => {
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

  it("explains a missing observer instead of rendering controls", () => {
    vi.mocked(isObserverConfigured).mockReturnValue(false);
    renderWithRouter();
    expect(screen.getByText("Observer not configured.")).toBeInTheDocument();
    expect(screen.queryByLabelText("Refresh")).not.toBeInTheDocument();
  });
});
