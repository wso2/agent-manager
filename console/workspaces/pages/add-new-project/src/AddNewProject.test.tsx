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

import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { SnackBarProvider } from "@agent-management-platform/views";
import { vi } from "vitest";
import { AddNewProject } from "./index";

// AddNewProject (and the ProjectForm it renders) call real TanStack Query
// hooks, which need a QueryClientProvider the real app only supplies at the
// shell level. Stub the api-client module boundary instead.
const createProjectMutate = vi.fn();
const generateNameMutate = vi.fn();

vi.mock("@agent-management-platform/api-client", () => ({
  useCreateProject: vi.fn(() => ({ mutate: createProjectMutate, isPending: false, error: null })),
  useGenerateResourceName: vi.fn(() => ({ mutate: generateNameMutate, isPending: false })),
  useListDeploymentPipelines: vi.fn(() => ({
    data: {
      deploymentPipelines: [{ name: "default", displayName: "Default", promotionPaths: [] }],
    },
    isLoading: false,
  })),
  useListEnvironments: vi.fn(() => ({ data: [], isLoading: false, isError: false })),
}));

const route = "/org/org1/newProject";

function renderWithRouter() {
  return render(
    <SnackBarProvider>
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }} initialEntries={[route]} initialIndex={0}>
        <Routes>
          <Route path="/org/:orgId/newProject" element={<AddNewProject />} />
        </Routes>
      </MemoryRouter>
    </SnackBarProvider>,
  );
}

describe("AddNewProject", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders the page title and description", () => {
    renderWithRouter();

    expect(screen.getByText("Create a New Project")).toBeInTheDocument();
    expect(
      screen.getByText("Create a new project to organize and manage your agents."),
    ).toBeInTheDocument();
  });

  it("disables Create until a resource name has been generated from the display name", () => {
    renderWithRouter();

    expect(screen.getByRole("button", { name: "Create" })).toBeDisabled();
  });

  it("generates a resource name from the display name and enables Create once it resolves", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    generateNameMutate.mockImplementation((_body, { onSuccess }) => {
      onSuccess({ name: "customer-support-platform" });
    });

    renderWithRouter();

    fireEvent.change(screen.getByPlaceholderText("e.g., Customer Support Platform"), {
      target: { value: "Customer Support Platform" },
    });

    await act(async () => { await vi.advanceTimersByTimeAsync(500); });
    vi.useRealTimers();

    expect(generateNameMutate).toHaveBeenCalledWith(
      { displayName: "Customer Support Platform", resourceType: "project" },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
    await waitFor(() => expect(screen.getByRole("button", { name: "Create" })).toBeEnabled());
  });

  it("submits the generated name and form fields when Create is clicked", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    generateNameMutate.mockImplementation((_body, { onSuccess }) => {
      onSuccess({ name: "my-project" });
    });

    renderWithRouter();

    fireEvent.change(screen.getByPlaceholderText("e.g., Customer Support Platform"), {
      target: { value: "My Project" },
    });
    await act(async () => { await vi.advanceTimersByTimeAsync(500); });
    vi.useRealTimers();

    await waitFor(() => expect(screen.getByRole("button", { name: "Create" })).toBeEnabled());
    fireEvent.click(screen.getByRole("button", { name: "Create" }));

    expect(createProjectMutate).toHaveBeenCalledWith(
      expect.objectContaining({ name: "my-project", displayName: "My Project" }),
      expect.objectContaining({ onSuccess: expect.any(Function), onError: expect.any(Function) }),
    );
  });
});
