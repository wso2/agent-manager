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


import { fireEvent, render, screen } from "@testing-library/react";
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
  useRetryAgentIdentityProvisioning: vi.fn(),
  useListAgentIdentityAgents: vi.fn(),
  useListAgentIdentityGroups: vi.fn(() => ({ data: { groups: [], total: 0 }, isLoading: false })),
  useListAgentIdentityRoles: vi.fn(() => ({ data: { roles: [], total: 0 }, isLoading: false })),
  useListEnvironments: vi.fn(),
  useGetProject: vi.fn(),
  useListDeploymentPipelines: vi.fn(),
}));

vi.mock("@agent-management-platform/views", () => ({
  PageLayout: ({ children }: { children?: React.ReactNode }) => <div>{children}</div>,
  TextInput: ({ label, value }: { label: string; value?: string }) => (
    <div>{label}: {value}</div>
  ),
}));

vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@agent-management-platform/shared-component")>()),
  useAgentIdentityCredentials: vi.fn(),
  useAgentRolesAndGroups: vi.fn(),
  usePipelineEnvironmentsState: vi.fn(),
  useThunderInstanceForEnv: vi.fn(),
  useUnsavedChangesGuard: vi.fn(),
}));

import {
  useGetProject,
  useListAgentIdentityAgents,
  useListDeploymentPipelines,
  useListEnvironments,
  useRetryAgentIdentityProvisioning,
} from "@agent-management-platform/api-client";
import {
  useAgentIdentityCredentials,
  useAgentRolesAndGroups,
  usePipelineEnvironmentsState,
  useThunderInstanceForEnv,
} from "@agent-management-platform/shared-component";
import { AgentIdComponent } from "./AgentId.Component";

const config = (window as unknown as { __RUNTIME_CONFIG__: { featureFlags?: unknown } })
  .__RUNTIME_CONFIG__;

const ENVIRONMENTS = [
  { name: "development", displayName: "Development" },
  { name: "staging", displayName: "Staging" },
];

const credentials = (binding: unknown) => ({
  binding,
  provisioned: (binding as { status?: string } | undefined)?.status === "completed",
  isLoading: false,
  isError: false,
  error: null,
  revealed: null,
  isRegenerating: false,
  regenerate: vi.fn(),
});

const mockBinding = (binding: unknown) =>
  vi.mocked(useAgentIdentityCredentials).mockReturnValue(
    credentials(binding) as unknown as ReturnType<typeof useAgentIdentityCredentials>,
  );

const completed = { status: "completed", provisioningType: "external", clientId: "client-1" };
const retry = vi.fn();

const renderPage = (envName = "staging") =>
  render(
    <MemoryRouter initialEntries={[`/org/acme/project/proj/agent/agent?identityEnv=${envName}`]}>
      <Routes>
        <Route path="/org/:orgId/project/:projectId/agent/:agentId" element={<AgentIdComponent />} />
      </Routes>
    </MemoryRouter>,
  );

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(useListEnvironments).mockReturnValue({
    data: ENVIRONMENTS,
  } as unknown as ReturnType<typeof useListEnvironments>);
  vi.mocked(useGetProject).mockReturnValue({
    data: { deploymentPipeline: "default" },
  } as unknown as ReturnType<typeof useGetProject>);
  vi.mocked(useListDeploymentPipelines).mockReturnValue({
    data: {
      deploymentPipelines: [{
        name: "default",
        promotionPaths: [{
          sourceEnvironmentRef: "development", targetEnvironmentRefs: [{ name: "staging" }],
        }],
      }],
    },
  } as unknown as ReturnType<typeof useListDeploymentPipelines>);
  vi.mocked(usePipelineEnvironmentsState).mockReturnValue({
    environments: ENVIRONMENTS, isLoading: false, isError: false,
  } as unknown as ReturnType<typeof usePipelineEnvironmentsState>);
  vi.mocked(useThunderInstanceForEnv).mockReturnValue({
    thunderInstance: undefined, isLoading: false, isError: false, error: null,
  } as unknown as ReturnType<typeof useThunderInstanceForEnv>);
  vi.mocked(useAgentRolesAndGroups).mockReturnValue({
    roles: [], groups: [], isLoading: false, isError: false, error: null,
  } as unknown as ReturnType<typeof useAgentRolesAndGroups>);
  vi.mocked(useListAgentIdentityAgents).mockReturnValue({
    data: { agents: [{ agentName: "agent", projectName: "proj", thunderAgentId: "thunder-1", status: "completed" }] },
    isLoading: false,
  } as unknown as ReturnType<typeof useListAgentIdentityAgents>);
  vi.mocked(useRetryAgentIdentityProvisioning).mockReturnValue({
    mutate: retry, isPending: false,
  } as unknown as ReturnType<typeof useRetryAgentIdentityProvisioning>);
  mockBinding(undefined);
});

afterEach(() => {
  delete config.featureFlags;
});

// The identity promotion card and drawer were removed: the platform promotes the
// identity by itself, so nothing on this page asks the user to promote.
const expectNoPromoteAction = () => {
  expect(screen.queryByRole("button", { name: /Promote|Review and promote|View details/ })).not.toBeInTheDocument();
  expect(screen.queryByText(/Promote identity|Needs promotion|Up to date/)).not.toBeInTheDocument();
};

describe("AgentIdComponent", () => {
  describe("with the promotion UI off", () => {
    beforeEach(() => {
      config.featureFlags = { enableAgentIdentity: true };
    });

    it("keeps the empty state when there is no binding, with no promoted-environment line", () => {
      renderPage();
      expect(screen.getByText("No agent identity")).toBeInTheDocument();
      expect(screen.queryByText(/Identity is being set up/)).not.toBeInTheDocument();
      expectNoPromoteAction();
    });

    it("makes no request for the project's pipeline beyond what the page already makes", () => {
      renderPage();
      // The environments list is the page's own call; the project and pipeline
      // lookups of the promoted-environment check get no ids.
      const projectCalls = vi.mocked(useGetProject).mock.calls;
      const pipelineCalls = vi.mocked(useListDeploymentPipelines).mock.calls;
      expect(projectCalls.every(([params]) => !params.orgName && !params.projName)).toBe(true);
      expect(pipelineCalls.every(([params]) => !params.orgName)).toBe(true);
    });

    it("leaves a completed identity and its editable assignments as they are", () => {
      mockBinding(completed);
      renderPage();
      expect(screen.getByRole("button", { name: /Regenerate Secret/ })).toBeInTheDocument();
      expect(screen.queryByText(/Setting up identity/)).not.toBeInTheDocument();
      fireEvent.click(screen.getByRole("tab", { name: "Roles" }));
      expect(screen.getByText("Add Role")).toBeInTheDocument();
      expect(screen.queryByText(/read only here/)).not.toBeInTheDocument();
      expectNoPromoteAction();
    });

    it("still shows a binding that is not completed as in progress or failed", () => {
      mockBinding({ status: "in_progress", provisioningType: "external" });
      renderPage();
      expect(screen.getByText("Setting up identity in Staging…")).toBeInTheDocument();
    });
  });

  describe("with the promotion UI off, failed binding", () => {
    it("shows the failure, its reason and Retry even when a client ID exists", () => {
      config.featureFlags = { enableAgentIdentity: true };
      mockBinding({ status: "failed", provisioningType: "external", clientId: "c", lastError: "boom" });
      renderPage();
      expect(screen.getByText("Setting up identity in Staging failed: boom")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
    });
  });

  describe("with the promotion UI on", () => {
    beforeEach(() => {
      config.featureFlags = { enableAgentIdentity: true, enableIdentityPromotionUi: true };
    });

    it("asks for the project's pipeline so it can tell which environment is the first", () => {
      renderPage();
      expect(useGetProject).toHaveBeenCalledWith({ orgName: "acme", projName: "proj" });
    });

    it("says identity is being set up, quietly and with no action, when a later environment has no binding yet", () => {
      renderPage();
      expect(screen.getByText("Identity is being set up for this environment.")).toBeInTheDocument();
      expect(screen.queryByText("No agent identity")).not.toBeInTheDocument();
      expect(screen.queryByRole("button")).not.toBeInTheDocument();
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });

    it("keeps the empty state for the first environment, where the identity is created", () => {
      renderPage("development");
      expect(screen.getByText("No agent identity")).toBeInTheDocument();
      expect(screen.queryByText(/Identity is being set up/)).not.toBeInTheDocument();
    });

    it.each(["pending", "in_progress"])("shows %s as setting up in the stage, with no error", (status) => {
      mockBinding({ status, provisioningType: "external" });
      renderPage();
      expect(screen.getByText("Setting up identity in Staging…")).toBeInTheDocument();
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Retry" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /Regenerate Secret/ })).not.toBeInTheDocument();
    });

    it("shows a failed binding with its short reason and a Retry action that re-runs that environment", () => {
      mockBinding({ status: "failed", provisioningType: "external", lastError: "token endpoint unreachable" });
      renderPage();
      expect(screen.getByRole("alert")).toHaveTextContent(
        "Setting up identity in Staging failed: token endpoint unreachable",
      );
      fireEvent.click(screen.getByRole("button", { name: "Retry" }));
      expect(retry).toHaveBeenCalledWith({
        params: { orgName: "acme", projName: "proj", agentName: "agent" },
        body: { environment: "staging" },
      });
    });

    it("shows a failed binding without a reason as a plain failure", () => {
      mockBinding({ status: "failed", provisioningType: "external" });
      renderPage();
      expect(screen.getByRole("alert")).toHaveTextContent("Setting up identity in Staging failed.");
      expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
    });

    it("cuts a very long failure reason", () => {
      mockBinding({ status: "failed", provisioningType: "external", lastError: "x".repeat(500) });
      renderPage();
      expect(screen.getByRole("alert").textContent).toContain(`${"x".repeat(200)}…`);
      expect(screen.getByRole("alert").textContent).not.toContain("x".repeat(201));
    });

    it("shows the failure even when a client ID already exists", () => {
      mockBinding({ status: "failed", provisioningType: "external", clientId: "client-1", lastError: "boom" });
      renderPage();
      expect(screen.getByRole("alert")).toHaveTextContent("Setting up identity in Staging failed: boom");
      expect(screen.getByText("Client ID: client-1")).toBeInTheDocument();
    });

    it("shows roles and groups read only in a later environment, naming the first environment", () => {
      mockBinding(completed);
      renderPage();
      fireEvent.click(screen.getByRole("tab", { name: "Roles" }));
      expect(screen.getByText(/managed in the Development environment/)).toHaveTextContent("read only here");
      expect(screen.queryByText("Add Role")).not.toBeInTheDocument();
      fireEvent.click(screen.getByRole("tab", { name: "Groups" }));
      expect(screen.getByText(/managed in the Development environment/)).toBeInTheDocument();
      expect(screen.queryByText("Add Group")).not.toBeInTheDocument();
      expectNoPromoteAction();
    });

    it("keeps assignments editable in the first environment", () => {
      mockBinding(completed);
      renderPage("development");
      fireEvent.click(screen.getByRole("tab", { name: "Roles" }));
      expect(screen.getByText("Add Role")).toBeInTheDocument();
      expect(screen.queryByText(/read only here/)).not.toBeInTheDocument();
    });

    it("takes the first environment from the pipeline order, not from the order of the environment list", () => {
      vi.mocked(useListEnvironments).mockReturnValue({
        data: [ENVIRONMENTS[1], ENVIRONMENTS[0]],
      } as unknown as ReturnType<typeof useListEnvironments>);
      mockBinding(completed);
      renderPage();
      fireEvent.click(screen.getByRole("tab", { name: "Roles" }));
      expect(screen.getByText(/managed in the Development environment/)).toBeInTheDocument();
    });
  });
});
