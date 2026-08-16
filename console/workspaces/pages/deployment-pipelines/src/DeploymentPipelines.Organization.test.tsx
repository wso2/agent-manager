/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { render, screen, fireEvent } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { SnackBarProvider } from "@agent-management-platform/views";
import type {
  DeploymentPipelineListResponse,
  Environment,
} from "@agent-management-platform/types";

// DeploymentPipelinesOrganization (and the table/drawers it renders) call
// real TanStack Query hooks, which need a QueryClientProvider the real app
// only wires up at the shell level. Stub the api-client module boundary
// instead, matching the { data, isLoading, ... } shape each hook destructures.
// Mock function identities below must stay stable across renders (module
// scope, not created inside the vi.fn() factories): EditDeploymentPipelineDrawer
// and CreateDeploymentPipelineDrawer depend on `reset` inside a useEffect dep
// array, and a factory that returns a fresh `vi.fn()` on every call makes
// that effect re-fire (and call setState) on every render, forever.
const deleteMutate = vi.fn();
const createMutateAsync = vi.fn();
const resetCreateMutation = vi.fn();
const updateMutateAsync = vi.fn();
const resetUpdateMutation = vi.fn();

vi.mock("@agent-management-platform/api-client", () => ({
  useListDeploymentPipelines: vi.fn(),
  useListEnvironments: vi.fn(),
  useDeleteDeploymentPipeline: vi.fn(() => ({ mutate: deleteMutate })),
  useCreateDeploymentPipeline: vi.fn(() => ({
    mutateAsync: createMutateAsync,
    isPending: false,
    error: null,
    reset: resetCreateMutation,
  })),
  useUpdateOrgDeploymentPipeline: vi.fn(() => ({
    mutateAsync: updateMutateAsync,
    isPending: false,
    error: null,
    reset: resetUpdateMutation,
  })),
}));

import {
  useListDeploymentPipelines,
  useListEnvironments,
} from "@agent-management-platform/api-client";
import { DeploymentPipelinesOrganization } from "./DeploymentPipelines.Organization";

const mockUseListDeploymentPipelines = vi.mocked(useListDeploymentPipelines);
const mockUseListEnvironments = vi.mocked(useListEnvironments);

const makeEnvironment = (name: string, displayName: string): Environment => ({
  name,
  displayName,
  dataplaneRef: "dp-1",
  isProduction: false,
  createdAt: "2026-01-01T00:00:00Z",
});

const pipelinesResponse = (
  overrides: Partial<DeploymentPipelineListResponse> = {},
): DeploymentPipelineListResponse => ({
  deploymentPipelines: [
    {
      name: "pipeline-a",
      displayName: "Pipeline A",
      description: "",
      orgName: "org1",
      createdAt: "2026-01-01T00:00:00Z",
      promotionPaths: [
        { sourceEnvironmentRef: "dev", targetEnvironmentRefs: [{ name: "prod" }] },
      ],
    },
  ],
  ...overrides,
});

/** Renders the component the way the app shell mounts it: behind a catch-all
 * sub-route so the component's own index/`*` routes resolve relative to it. */
function renderAt(initialPath: string) {
  return render(
    <SnackBarProvider>
      <MemoryRouter initialEntries={[initialPath]}>
        <Routes>
          <Route
            path="/org/:orgId/deployment-pipelines/*"
            element={<DeploymentPipelinesOrganization />}
          />
        </Routes>
      </MemoryRouter>
    </SnackBarProvider>,
  );
}

describe("DeploymentPipelinesOrganization", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseListEnvironments.mockReturnValue({
      data: [makeEnvironment("dev", "Development"), makeEnvironment("prod", "Production")],
      isLoading: false,
    } as ReturnType<typeof useListEnvironments>);
  });

  it("renders the pipeline list with its promotion chain once data loads", () => {
    mockUseListDeploymentPipelines.mockReturnValue({
      data: pipelinesResponse(),
      isLoading: false,
      error: null,
    } as ReturnType<typeof useListDeploymentPipelines>);

    renderAt("/org/org1/deployment-pipelines");

    expect(screen.getByText("Deployment Pipelines")).toBeInTheDocument();
    expect(screen.getByText("Pipeline A")).toBeInTheDocument();
    expect(screen.getByText("Development")).toBeInTheDocument();
    expect(screen.getByText("Production")).toBeInTheDocument();
  });

  it("shows the empty state and no Create button when there are no pipelines", () => {
    mockUseListDeploymentPipelines.mockReturnValue({
      data: pipelinesResponse({ deploymentPipelines: [] }),
      isLoading: false,
      error: null,
    } as ReturnType<typeof useListDeploymentPipelines>);

    renderAt("/org/org1/deployment-pipelines");

    expect(screen.getByText("No deployment pipelines yet")).toBeInTheDocument();
    // The toolbar's "Create Pipeline" button only shows once pipelines exist;
    // the empty-state action is the only Create entry point here.
    expect(screen.getAllByRole("button", { name: "Create Pipeline" })).toHaveLength(1);
  });

  it("surfaces a load error instead of the table", () => {
    mockUseListDeploymentPipelines.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("network down"),
    } as unknown as ReturnType<typeof useListDeploymentPipelines>);

    renderAt("/org/org1/deployment-pipelines");

    expect(screen.getByText(/Failed to load pipelines\./)).toBeInTheDocument();
    expect(screen.getByText(/network down/)).toBeInTheDocument();
    expect(screen.queryByText("Pipeline A")).not.toBeInTheDocument();
  });

  it("auto-opens the edit drawer for the pipeline named in ?edit= and strips the param", () => {
    mockUseListDeploymentPipelines.mockReturnValue({
      data: pipelinesResponse(),
      isLoading: false,
      error: null,
    } as ReturnType<typeof useListDeploymentPipelines>);

    function LocationProbe() {
      const location = useLocation();
      return <div data-testid="location">{location.pathname}{location.search}</div>;
    }

    render(
      <SnackBarProvider>
        <MemoryRouter
          initialEntries={[
            { pathname: "/org/org1/deployment-pipelines", search: "?edit=pipeline-a" },
          ]}
        >
          <LocationProbe />
          <Routes>
            <Route
              path="/org/:orgId/deployment-pipelines/*"
              element={<DeploymentPipelinesOrganization />}
            />
          </Routes>
        </MemoryRouter>
      </SnackBarProvider>,
    );

    expect(screen.getByRole("heading", { name: "Edit Deployment Pipeline" })).toBeInTheDocument();
    // The consumed ?edit= param is replaced out of the URL once the drawer opens.
    expect(screen.getByTestId("location")).toHaveTextContent("/org/org1/deployment-pipelines");
    expect(screen.getByTestId("location").textContent).not.toContain("edit=");
  });

  it("opens the create drawer from the empty state's Create Pipeline button", () => {
    mockUseListDeploymentPipelines.mockReturnValue({
      data: pipelinesResponse({ deploymentPipelines: [] }),
      isLoading: false,
      error: null,
    } as ReturnType<typeof useListDeploymentPipelines>);

    renderAt("/org/org1/deployment-pipelines");

    fireEvent.click(screen.getByRole("button", { name: "Create Pipeline" }));

    expect(screen.getByRole("heading", { name: "Create Deployment Pipeline" })).toBeInTheDocument();
  });

  it("redirects an unmatched sub-path back to the pipeline list", () => {
    mockUseListDeploymentPipelines.mockReturnValue({
      data: pipelinesResponse(),
      isLoading: false,
      error: null,
    } as ReturnType<typeof useListDeploymentPipelines>);

    renderAt("/org/org1/deployment-pipelines/does-not-exist");

    expect(screen.getByText("Deployment Pipelines")).toBeInTheDocument();
    expect(screen.getByText("Pipeline A")).toBeInTheDocument();
  });
});
