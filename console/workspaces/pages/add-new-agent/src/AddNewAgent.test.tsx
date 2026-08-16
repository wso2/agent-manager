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
import { AddNewAgent } from "./AddNewAgent";

// AddNewAgent's steps (NewAgentOptions, ExternalAgentFlow, ...) call real
// TanStack Query hooks, which need a QueryClientProvider the real app only
// supplies at the shell level. Stub the api-client module boundary instead.
//
// generateNameMutate must be a stable module-scope reference, not created
// fresh inside the vi.fn() factory below: ExternalAgentForm feeds it into a
// useMemo/useEffect dependency chain that debounces name generation, and a
// factory returning a new mock function on every call makes that effect
// re-fire (and update state) on every render, forever.
const generateNameMutate = vi.fn();

vi.mock("@agent-management-platform/api-client", () => ({
  useListAgents: vi.fn(() => ({ data: { agents: [] } })),
  useCreateAgent: vi.fn(() => ({ mutate: vi.fn(), isPending: false, error: null })),
  useGenerateResourceName: vi.fn(() => ({ mutate: generateNameMutate, isPending: false })),
}));

const route = "/org/org1/project/proj1/newAgent";

function renderWithRouter(initialEntry = route) {
  return render(
    <SnackBarProvider>
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }} initialEntries={[initialEntry]}>
        <Routes>
          <Route path="/org/:orgId/project/:projectId/newAgent/*" element={<AddNewAgent />} />
        </Routes>
      </MemoryRouter>
    </SnackBarProvider>,
  );
}

describe("AddNewAgent", () => {
  it("renders the two agent-hosting options", () => {
    renderWithRouter();

    expect(screen.getByText("Add a New Agent")).toBeInTheDocument();
    expect(screen.getByText("Externally-Hosted Agent")).toBeInTheDocument();
    expect(screen.getByText("Platform-Hosted Agent")).toBeInTheDocument();
  });

  it("navigates to the create-source step when Platform-Hosted Agent is chosen", () => {
    renderWithRouter();

    fireEvent.click(screen.getByText("Platform-Hosted Agent"));

    expect(screen.getByText("Create a Platform-Hosted Agent")).toBeInTheDocument();
  });

  it("navigates to the connect flow when Externally-Hosted Agent is chosen", () => {
    renderWithRouter();

    fireEvent.click(screen.getByText("Externally-Hosted Agent"));

    expect(screen.getByText("Register an Externally-Hosted Agent")).toBeInTheDocument();
  });
});
