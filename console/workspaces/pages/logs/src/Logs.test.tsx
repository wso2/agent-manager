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
import { LogsComponent } from "./index";

const route =
  "/org/org1/project/proj1/agents/agent1/environment/env1/observability/logs";

function renderWithRouter(initialEntry = route) {
  return render(
    <MemoryRouter initialEntries={[initialEntry]} initialIndex={0}>
      <Routes>
        <Route
          path="/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/observability/logs"
          element={<LogsComponent />}
        />
      </Routes>
    </MemoryRouter>
  );
}

describe("LogsComponent", () => {
  it("renders without crashing", () => {
    renderWithRouter();
    expect(screen.getByText("Logs")).toBeInTheDocument();
  });

  it("renders time range and sort summary", () => {
    renderWithRouter();
    expect(
      screen.getByText(/Time range:.*· Sort:/)
    ).toBeInTheDocument();
  });

  it("renders Export button", () => {
    renderWithRouter();
    expect(screen.getByRole("button", { name: /export/i })).toBeInTheDocument();
  });

  it("renders refresh and sort controls", () => {
    renderWithRouter();
    expect(screen.getByLabelText("Refresh")).toBeInTheDocument();
    expect(
      screen.getByLabelText(/Sort (ascending|descending)/)
    ).toBeInTheDocument();
  });
});
