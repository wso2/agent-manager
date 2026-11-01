/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// The types package reads window.__RUNTIME_CONFIG__ when it loads, and nothing
// injects it under Vitest.
vi.hoisted(() => {
  (window as unknown as { __RUNTIME_CONFIG__: Record<string, unknown> }).__RUNTIME_CONFIG__ ??= {};
});

vi.mock("@agent-management-platform/api-client", () => ({
  useAddAgentIdentityGroupMembers: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useAddAgentIdentityRoleAssignees: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useRemoveAgentIdentityGroupMembers: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useRemoveAgentIdentityRoleAssignees: vi.fn(() => ({ mutateAsync: vi.fn() })),
  useGetAgent: vi.fn(),
  useGetAgentGroups: vi.fn(),
  useGetAgentRoles: vi.fn(),
  useGetProject: vi.fn(),
  useListAgentIdentityAgents: vi.fn(),
  useListAgentIdentityGroups: vi.fn(),
  useListAgentIdentityRoles: vi.fn(),
  useListDeploymentPipelines: vi.fn(),
  useListEnvironments: vi.fn(),
}));

vi.mock("@agent-management-platform/views", () => ({
  PageLayout: ({ children }: { children?: React.ReactNode }) => <div>{children}</div>,
}));

vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@agent-management-platform/shared-component")>()),
  useUnsavedChangesGuard: vi.fn(() => ({ allowNavigation: vi.fn() })),
}));

import {
  useGetAgent,
  useGetAgentGroups,
  useGetAgentRoles,
  useGetProject,
  useListAgentIdentityAgents,
  useListAgentIdentityGroups,
  useListAgentIdentityRoles,
  useListDeploymentPipelines,
  useListEnvironments,
} from "@agent-management-platform/api-client";
import { AgentDetailPage } from "./AgentDetailPage";

const config = (window as unknown as { __RUNTIME_CONFIG__: { featureFlags?: unknown } })
  .__RUNTIME_CONFIG__;

// The first environment of the project's pipeline is where roles and groups are
// managed; every environment after it receives them with the identity.
const mockPipeline = (environments: string[]) => {
  vi.mocked(useListEnvironments).mockReturnValue({
    data: environments.map((name) => ({ name, displayName: name === "development" ? "Development" : "Staging" })),
  } as unknown as ReturnType<typeof useListEnvironments>);
  vi.mocked(useListDeploymentPipelines).mockReturnValue({
    data: {
      deploymentPipelines: [{
        name: "default",
        promotionPaths: environments.slice(0, -1).map((name, index) => ({
          sourceEnvironmentRef: name, targetEnvironmentRefs: [{ name: environments[index + 1] }],
        })),
      }],
    },
  } as unknown as ReturnType<typeof useListDeploymentPipelines>);
};

const renderPage = () =>
  render(
    <MemoryRouter initialEntries={["/org/acme/project/proj/agent/agent?envName=staging"]}>
      <Routes>
        <Route path="/org/:orgId/project/:projectName/agent/:agentName" element={<AgentDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(useGetAgent).mockReturnValue({
    data: { displayName: "Agent", provisioning: { type: "external" } }, isLoading: false,
  } as unknown as ReturnType<typeof useGetAgent>);
  vi.mocked(useGetProject).mockReturnValue({
    data: { displayName: "Project", deploymentPipeline: "default" }, isLoading: false,
  } as unknown as ReturnType<typeof useGetProject>);
  vi.mocked(useGetAgentRoles).mockReturnValue({
    data: { roles: [{ id: "r1", name: "booker" }] }, isLoading: false, error: null,
  } as unknown as ReturnType<typeof useGetAgentRoles>);
  vi.mocked(useGetAgentGroups).mockReturnValue({
    data: { groups: [] }, isLoading: false, error: null,
  } as unknown as ReturnType<typeof useGetAgentGroups>);
  vi.mocked(useListAgentIdentityAgents).mockReturnValue({
    data: { agents: [{ agentName: "agent", projectName: "proj", thunderAgentId: "thunder-1" }] }, isLoading: false,
  } as unknown as ReturnType<typeof useListAgentIdentityAgents>);
  vi.mocked(useListAgentIdentityRoles).mockReturnValue({
    data: { roles: [], total: 0 }, isLoading: false,
  } as unknown as ReturnType<typeof useListAgentIdentityRoles>);
  vi.mocked(useListAgentIdentityGroups).mockReturnValue({
    data: { groups: [], total: 0 }, isLoading: false,
  } as unknown as ReturnType<typeof useListAgentIdentityGroups>);
  mockPipeline(["development", "staging"]);
});

afterEach(() => {
  delete config.featureFlags;
});

describe("AgentDetailPage", () => {
  it("keeps roles and groups editable with the promotion UI off, and looks up no pipeline", () => {
    config.featureFlags = { enableAgentIdentity: true };
    renderPage();
    expect(screen.getByText("booker")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Search roles...")).toBeInTheDocument();
    expect(screen.queryByText(/read only here/)).not.toBeInTheDocument();
    // The page's own project read stays; the pipeline lookups get no ids.
    const pipelineCalls = vi.mocked(useListDeploymentPipelines).mock.calls;
    const environmentCalls = vi.mocked(useListEnvironments).mock.calls;
    expect(pipelineCalls.every(([params]) => !params.orgName)).toBe(true);
    expect(environmentCalls.every(([params]) => !params.orgName)).toBe(true);
  });

  it("shows roles and groups by name but read only in an environment after the first, naming the first", () => {
    config.featureFlags = { enableAgentIdentity: true, enableIdentityPromotionUi: true };
    renderPage();
    expect(screen.getByText("booker")).toBeInTheDocument();
    expect(screen.getByText(/managed in the Development environment/)).toHaveTextContent("read only here");
    expect(screen.queryByPlaceholderText("Search roles...")).not.toBeInTheDocument();
    expect(screen.queryByText(/no active identity binding/)).not.toBeInTheDocument();
  });

  it("keeps assignments editable with the promotion UI on in the first environment", () => {
    config.featureFlags = { enableAgentIdentity: true, enableIdentityPromotionUi: true };
    mockPipeline(["staging", "development"]);
    renderPage();
    expect(screen.getByPlaceholderText("Search roles...")).toBeInTheDocument();
    expect(screen.queryByText(/read only here/)).not.toBeInTheDocument();
  });
});
