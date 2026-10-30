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
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// The types package reads window.__RUNTIME_CONFIG__ when it loads, and nothing
// injects it under Vitest.
vi.hoisted(() => {
  (window as unknown as { __RUNTIME_CONFIG__: Record<string, unknown> }).__RUNTIME_CONFIG__ ??= {};
});

vi.mock("@agent-management-platform/api-client", () => ({
  useAgentIdentityBinding: vi.fn(),
  useGetProject: vi.fn(),
  useListAgentIdentityAgents: vi.fn(),
  useListDeploymentPipelines: vi.fn(),
  useListEnvironments: vi.fn(),
  useProvisionAgentIdentity: vi.fn(),
  useRetryAgentIdentityProvisioning: vi.fn(),
}));

vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@agent-management-platform/shared-component")>()),
  useAgentRolesAndGroups: vi.fn(),
}));

import {
  useAgentIdentityBinding,
  useGetProject,
  useListAgentIdentityAgents,
  useListDeploymentPipelines,
  useListEnvironments,
  useProvisionAgentIdentity,
  useRetryAgentIdentityProvisioning,
} from "@agent-management-platform/api-client";
import { useAgentRolesAndGroups } from "@agent-management-platform/shared-component";
import { EnvAgentRolesGroupsSection } from "./EnvAgentRolesGroupsSection";

const config = (window as unknown as { __RUNTIME_CONFIG__: { featureFlags?: unknown } })
  .__RUNTIME_CONFIG__;

const retry = vi.fn();

const mockBinding = (
  binding: { status: string; lastError?: string } | undefined,
  provisioned: boolean,
) =>
  vi.mocked(useAgentIdentityBinding).mockReturnValue({
    binding, provisioned, isLoading: false, isError: false,
  } as unknown as ReturnType<typeof useAgentIdentityBinding>);

beforeEach(() => {
  vi.clearAllMocks();
  mockBinding({ status: "completed" }, true);
  vi.mocked(useListAgentIdentityAgents).mockReturnValue({
    data: { agents: [{ agentName: "agent", projectName: "proj", thunderAgentId: "thunder-1" }] }, isError: false,
  } as unknown as ReturnType<typeof useListAgentIdentityAgents>);
  vi.mocked(useListEnvironments).mockReturnValue({
    data: [{ name: "development", displayName: "Development" }, { name: "staging", displayName: "Staging" }],
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
  vi.mocked(useProvisionAgentIdentity).mockReturnValue({
    mutate: vi.fn(), isPending: false, isError: false,
  } as unknown as ReturnType<typeof useProvisionAgentIdentity>);
  vi.mocked(useRetryAgentIdentityProvisioning).mockReturnValue({
    mutate: retry, isPending: false,
  } as unknown as ReturnType<typeof useRetryAgentIdentityProvisioning>);
  vi.mocked(useAgentRolesAndGroups).mockReturnValue({
    roles: [{ id: "r1", name: "booker" }], groups: [{ id: "g1", name: "travel" }],
    isLoading: false, isError: false, error: null,
  } as unknown as ReturnType<typeof useAgentRolesAndGroups>);
});

afterEach(() => {
  delete config.featureFlags;
});

// A binding that has not completed has no Agent ID in the identity provider yet.
const mockNoAgentId = () =>
  vi.mocked(useListAgentIdentityAgents).mockReturnValue({
    data: { agents: [] }, isError: false,
  } as unknown as ReturnType<typeof useListAgentIdentityAgents>);

const renderSection = (envId = "staging") =>
  render(
    <MemoryRouter>
      <EnvAgentRolesGroupsSection orgId="org" projectId="proj" agentId="agent" envId={envId} />
    </MemoryRouter>,
  );

// The identity promotion card and drawer were removed: the platform promotes the
// identity by itself, so nothing on this card asks the user to promote.
const expectNoPromoteAction = () => {
  expect(screen.queryByRole("button", { name: /Promote|Review and promote|View details/ })).not.toBeInTheDocument();
  expect(screen.queryByText(/Needs promotion|Up to date|Not promoted/)).not.toBeInTheDocument();
};

describe("EnvAgentRolesGroupsSection", () => {
  describe("with the promotion UI off", () => {
    beforeEach(() => {
      config.featureFlags = { enableAgentIdentity: true };
    });

    it("lists the environment's roles and groups and looks up no pipeline", () => {
      renderSection();
      expect(screen.getByText("Roles: booker")).toBeInTheDocument();
      expect(screen.getByText("Groups: travel")).toBeInTheDocument();
      expect(screen.getByText("thunder-1")).toBeInTheDocument();
      expect(vi.mocked(useAgentRolesAndGroups)).toHaveBeenCalledWith(
        expect.objectContaining({ enabled: true }),
      );
      const projectCalls = vi.mocked(useGetProject).mock.calls;
      const pipelineCalls = vi.mocked(useListDeploymentPipelines).mock.calls;
      const environmentCalls = vi.mocked(useListEnvironments).mock.calls;
      expect(projectCalls.every(([params]) => !params.orgName && !params.projName)).toBe(true);
      expect(pipelineCalls.every(([params]) => !params.orgName)).toBe(true);
      expect(environmentCalls.every(([params]) => !params.orgName)).toBe(true);
      expectNoPromoteAction();
    });

    it("is unchanged when the flag is false", () => {
      config.featureFlags = { enableAgentIdentity: true, enableIdentityPromotionUi: false };
      renderSection();
      expect(screen.getByText("Roles: booker")).toBeInTheDocument();
      expect(screen.queryByText(/Identity is being set up/)).not.toBeInTheDocument();
    });

    it("still offers Create Agent ID when the environment has no identity", () => {
      mockBinding(undefined, false);
      renderSection();
      expect(screen.getByRole("button", { name: /Create Agent ID/ })).toBeInTheDocument();
      expect(screen.queryByText(/Identity is being set up/)).not.toBeInTheDocument();
    });

    it("keeps the failed alert with its reason and Retry", () => {
      mockBinding({ status: "failed", lastError: "boom" }, false);
      renderSection();
      expect(screen.getByRole("alert")).toHaveTextContent("Provisioning failed: boom");
      fireEvent.click(screen.getByRole("button", { name: "Retry" }));
      expect(retry).toHaveBeenCalledWith({
        params: { orgName: "org", projName: "proj", agentName: "agent" },
        body: { environment: "staging" },
      });
    });

    it("keeps the previous wording for a pending binding", () => {
      mockBinding({ status: "pending" }, false);
      mockNoAgentId();
      renderSection();
      expect(screen.getByText("Agent ID not available")).toBeInTheDocument();
      expect(screen.queryByText(/Setting up identity/)).not.toBeInTheDocument();
    });
  });

  describe("with the promotion UI on", () => {
    beforeEach(() => {
      config.featureFlags = { enableAgentIdentity: true, enableIdentityPromotionUi: true };
    });

    it("shows a completed identity in a later environment as the plain card, with no promote action", () => {
      renderSection();
      expect(screen.getByText("thunder-1")).toBeInTheDocument();
      expect(screen.getByText("Roles: booker")).toBeInTheDocument();
      expect(screen.getByText("Groups: travel")).toBeInTheDocument();
      expectNoPromoteAction();
    });

    it("says identity is being set up, with no Create Agent ID action, when a later environment has no binding", () => {
      mockBinding(undefined, false);
      mockNoAgentId();
      renderSection();
      expect(screen.getByText("Identity is being set up for this environment.")).toBeInTheDocument();
      expect(screen.queryByText("Not created for this environment yet")).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /Create Agent ID/ })).not.toBeInTheDocument();
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });

    it("keeps Create Agent ID for the first environment, where the identity is created", () => {
      mockBinding(undefined, false);
      renderSection("development");
      expect(screen.getByRole("button", { name: /Create Agent ID/ })).toBeInTheDocument();
      expect(screen.queryByText(/Identity is being set up/)).not.toBeInTheDocument();
    });

    it.each(["pending", "in_progress"])("shows a %s binding in a later environment as setting up in that stage", (status) => {
      mockBinding({ status }, false);
      mockNoAgentId();
      renderSection();
      expect(screen.getByText("Setting up identity in Staging…")).toBeInTheDocument();
      expect(screen.queryByText("Agent ID not available")).not.toBeInTheDocument();
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });

    it("shows a failed binding with its reason and Retry, as it always did", () => {
      mockBinding({ status: "failed", lastError: "boom" }, false);
      renderSection();
      expect(screen.getByRole("alert")).toHaveTextContent("Provisioning failed: boom");
      fireEvent.click(screen.getByRole("button", { name: "Retry" }));
      expect(retry).toHaveBeenCalledTimes(1);
    });

    it("does not call a later environment's identity being set up while the pipeline loads", () => {
      mockBinding(undefined, false);
      mockNoAgentId();
      vi.mocked(useListDeploymentPipelines).mockReturnValue({
        data: undefined, isLoading: true, isError: false,
      } as unknown as ReturnType<typeof useListDeploymentPipelines>);
      renderSection();
      expect(screen.queryByText(/Identity is being set up/)).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /Create Agent ID/ })).not.toBeInTheDocument();
    });

    it("treats a project whose pipeline has no promotion paths as having no promoted environment", () => {
      mockBinding(undefined, false);
      mockNoAgentId();
      vi.mocked(useListDeploymentPipelines).mockReturnValue({
        data: { deploymentPipelines: [{ name: "default", promotionPaths: [] }] },
      } as unknown as ReturnType<typeof useListDeploymentPipelines>);
      renderSection();
      expect(screen.getByRole("button", { name: /Create Agent ID/ })).toBeInTheDocument();
      expect(screen.queryByText(/Identity is being set up/)).not.toBeInTheDocument();
    });

    it("shows a failure to read roles and groups as an error", () => {
      vi.mocked(useAgentRolesAndGroups).mockReturnValue({
        roles: [], groups: [], isLoading: false, isError: true, error: new Error("x"),
      } as unknown as ReturnType<typeof useAgentRolesAndGroups>);
      renderSection();
      expect(screen.getByText(/Unable to load roles\/groups/)).toBeInTheDocument();
    });
  });
});
