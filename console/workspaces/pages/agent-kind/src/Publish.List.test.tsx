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

import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { SnackBarProvider } from "@agent-management-platform/views";
import { ConfirmationDialogProvider } from "@agent-management-platform/shared-component";
import { vi } from "vitest";
import type {
  AgentKindResponse,
  AgentKindVersionResponse,
  BuildResponse,
} from "@agent-management-platform/types";

// PublishedList calls several real TanStack Query hooks (list versions, get
// agent, get kind, get builds, delete/publish mutations), which need a
// QueryClientProvider the real app only wires up at the shell level. Stub
// the api-client module boundary instead, matching the { data, isLoading }
// / { mutate(Async), isPending, isSuccess } shapes destructured in the
// component.
vi.mock("@agent-management-platform/api-client", () => ({
  useDeleteAgentKind: vi.fn(),
  useListAgentKindVersions: vi.fn(),
  useGetAgent: vi.fn(),
  useGetAgentKind: vi.fn(),
  usePublishAgentKind: vi.fn(),
  useGetAgentBuilds: vi.fn(),
}));

import {
  useDeleteAgentKind,
  useListAgentKindVersions,
  useGetAgent,
  useGetAgentKind,
  usePublishAgentKind,
  useGetAgentBuilds,
} from "@agent-management-platform/api-client";
import { PublishedList } from "./Publish.List";

const mockUseDeleteAgentKind = vi.mocked(useDeleteAgentKind);
const mockUseListAgentKindVersions = vi.mocked(useListAgentKindVersions);
const mockUseGetAgent = vi.mocked(useGetAgent);
const mockUseGetAgentKind = vi.mocked(useGetAgentKind);
const mockUsePublishAgentKind = vi.mocked(usePublishAgentKind);
const mockUseGetAgentBuilds = vi.mocked(useGetAgentBuilds);

const makeVersion = (
  version: string,
  createdAt: string,
  buildName = "build-1",
): AgentKindVersionResponse => ({
  version,
  buildName,
  imageId: `image-${version}`,
  sourceAgentName: "agent1",
  sourceProjectName: "proj1",
  configSchema: [],
  createdAt,
});

const makeKind = (overrides: Partial<AgentKindResponse> = {}): AgentKindResponse => ({
  uuid: "kind-1",
  name: "agent1",
  displayName: "Agent One",
  description: "An agent kind",
  organizationName: "org1",
  kind: "AgentKind",
  versions: [],
  createdAt: "2026-01-01T00:00:00Z",
  ...overrides,
});

const makeBuild = (buildName: string): BuildResponse => ({
  buildName,
  projectName: "proj1",
  agentName: "agent1",
  startedAt: "2026-01-01T00:00:00Z",
  status: "Succeeded",
  buildParameters: {
    repoUrl: "https://example.com/repo.git",
    appPath: "/",
    branch: "main",
    commitId: "abcdef1234567890",
    language: "python",
    languageVersion: "3.11",
    runCommand: "python main.py",
  },
});

const unpublishMutate = vi.fn();
const publishMutateAsync = vi.fn();

const seedHooks = ({
  versions = [] as AgentKindVersionResponse[],
  isVersionsLoading = false,
  existingKind = null as AgentKindResponse | null,
  builds = [] as BuildResponse[],
} = {}) => {
  mockUseDeleteAgentKind.mockReturnValue({
    mutate: unpublishMutate,
    isPending: false,
    isSuccess: false,
  } as unknown as ReturnType<typeof useDeleteAgentKind>);

  mockUseListAgentKindVersions.mockReturnValue({
    data: versions,
    isLoading: isVersionsLoading,
  } as unknown as ReturnType<typeof useListAgentKindVersions>);

  mockUseGetAgent.mockReturnValue({
    data: { displayName: "Agent One", description: "An agent" },
  } as unknown as ReturnType<typeof useGetAgent>);

  mockUseGetAgentKind.mockReturnValue({
    data: existingKind,
  } as unknown as ReturnType<typeof useGetAgentKind>);

  mockUsePublishAgentKind.mockReturnValue({
    mutateAsync: publishMutateAsync,
    isPending: false,
  } as unknown as ReturnType<typeof usePublishAgentKind>);

  mockUseGetAgentBuilds.mockReturnValue({
    data: { builds },
    isLoading: false,
  } as unknown as ReturnType<typeof useGetAgentBuilds>);
};

const basePath = "/org/org1/project/proj1/agents/agent1/publish";
const routePath = "/org/:orgId/project/:projectId/agents/:agentId/publish";

const renderList = (initialEntry = basePath) =>
  render(
    <SnackBarProvider>
      <ConfirmationDialogProvider>
        <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }} initialEntries={[initialEntry]}>
          <Routes>
            <Route path={routePath} element={<PublishedList />} />
            <Route path={`${routePath}/create-new-version`} element={<PublishedList />} />
            <Route
              path={`${routePath}/version-details/:versionId`}
              element={<div>Version Details Page</div>}
            />
          </Routes>
        </MemoryRouter>
      </ConfirmationDialogProvider>
    </SnackBarProvider>,
  );

describe("PublishedList", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows the empty state and no Unpublish button when the kind has never been published", () => {
    seedHooks({ versions: [], existingKind: null });
    renderList();

    expect(screen.getByText("No versions published yet")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /unpublish kind/i }),
    ).not.toBeInTheDocument();
  });

  it("lists published versions newest-first and marks the newest as Latest", () => {
    seedHooks({
      versions: [
        makeVersion("1.0.0", "2026-01-01T00:00:00Z", "build-old"),
        makeVersion("2.0.0", "2026-03-01T00:00:00Z", "build-new"),
      ],
      existingKind: makeKind(),
    });
    renderList();

    const rows = screen.getAllByRole("row").slice(1); // drop header row
    expect(within(rows[0]).getByText("2.0.0")).toBeInTheDocument();
    expect(within(rows[0]).getByText("Latest")).toBeInTheDocument();
    expect(within(rows[1]).getByText("1.0.0")).toBeInTheDocument();
    expect(within(rows[1]).queryByText("Latest")).not.toBeInTheDocument();
  });

  it("navigates to the version details page when a version row is clicked", () => {
    seedHooks({
      versions: [makeVersion("1.0.0", "2026-01-01T00:00:00Z")],
      existingKind: makeKind(),
    });
    renderList();

    fireEvent.click(screen.getByText("1.0.0"));

    expect(screen.getByText("Version Details Page")).toBeInTheDocument();
  });

  it("confirms before unpublishing and calls the delete mutation with the org/kind names", () => {
    seedHooks({
      versions: [makeVersion("1.0.0", "2026-01-01T00:00:00Z")],
      existingKind: makeKind(),
    });
    renderList();

    fireEvent.click(screen.getByRole("button", { name: /unpublish kind/i }));
    expect(screen.getByText(/cannot be undone/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Unpublish" }));

    expect(unpublishMutate).toHaveBeenCalledWith({ orgName: "org1", kindName: "agent1" });
  });

  it("publishes a new version with the filled-in name and selected build, then closes the drawer", async () => {
    publishMutateAsync.mockResolvedValue(makeVersion("1.2.0", "2026-05-01T00:00:00Z"));
    seedHooks({
      versions: [],
      existingKind: makeKind(),
      builds: [makeBuild("build-42")],
    });
    renderList(`${basePath}/create-new-version`);

    expect(screen.getByText("Create New Version")).toBeInTheDocument();
    // existingKind is set, so the Kind Details section (display name/description) is skipped.
    expect(screen.queryByLabelText("Display Name")).not.toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("e.g. 1.2.0"), {
      target: { value: "1.2.0" },
    });

    fireEvent.mouseDown(screen.getByText("Select a build"));
    fireEvent.click(await screen.findByRole("option", { name: /build-42/ }));

    const createButton = screen.getByRole("button", { name: "Create Version" });
    expect(createButton).toBeEnabled();
    fireEvent.click(createButton);

    await waitFor(() =>
      expect(publishMutateAsync).toHaveBeenCalledWith({
        params: { orgName: "org1", projName: "proj1", agentName: "agent1" },
        body: expect.objectContaining({
          kindName: "agent1",
          version: "1.2.0",
          buildName: "build-42",
          configSchema: [],
        }),
      }),
    );

    await waitFor(() =>
      expect(screen.queryByText("Create New Version")).not.toBeInTheDocument(),
    );
  });
});
