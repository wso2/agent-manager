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

import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  accessState,
  apiState,
  mutation,
  resetAccessState,
  resetApiState,
  ROUTER_FUTURE,
  last,
} from "../__test__/mocks";
import { EnvAgentRolesGroupsSection } from "./EnvAgentRolesGroupsSection";

vi.mock(
  "@agent-management-platform/api-client",
  async () => (await import("../__test__/mocks")).apiClientMock,
);
vi.mock(
  "@agent-management-platform/feature-management",
  async () => (await import("../__test__/mocks")).featureManagementMock,
);

const rolesAndGroups = vi.hoisted(() => ({
  current: {} as Record<string, unknown>,
  calls: [] as unknown[],
}));

vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useAgentRolesAndGroups: (params: unknown) => {
    rolesAndGroups.calls.push(params);
    return rolesAndGroups.current;
  },
}));

// Oxygen UI pages are slow to render under a loaded CI runner.
vi.setConfig({ testTimeout: 60_000 });

const IDS = { orgId: "acme", projectId: "shop", agentId: "support-bot", envId: "dev" };

const renderCard = () =>
  render(
    <MemoryRouter future={ROUTER_FUTURE}>
      <EnvAgentRolesGroupsSection {...IDS} />
    </MemoryRouter>,
  );

const writeText = vi.fn();

beforeEach(() => {
  vi.clearAllMocks();
  resetApiState();
  resetAccessState();
  rolesAndGroups.calls = [];
  rolesAndGroups.current = { roles: [], groups: [], isLoading: false, isError: false };
  apiState.binding = {
    binding: { status: "completed" },
    provisioned: true,
    isLoading: false,
    isError: false,
  };
  apiState.queries.useListAgentIdentityAgents = {
    data: {
      agents: [
        {
          agentName: "support-bot",
          projectName: "shop",
          status: "completed",
          thunderAgentId: "tid-1",
        },
      ],
    },
  };
  Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
});

afterEach(() => {
  vi.useRealTimers();
});

describe("EnvAgentRolesGroupsSection", () => {
  it("shows the agent ID with its roles and groups, linking to the Agent ID page", () => {
    rolesAndGroups.current = {
      roles: [
        { id: "r1", name: "admin" },
        { id: "r2", name: "viewer" },
      ],
      groups: [{ id: "g1", name: "on-call" }],
      isLoading: false,
      isError: false,
    };
    renderCard();

    expect(screen.getByText("Agent ID")).toBeInTheDocument();
    expect(screen.getByText("tid-1")).toBeInTheDocument();
    expect(screen.getByText("Roles: admin, viewer")).toBeInTheDocument();
    expect(screen.getByText("Groups: on-call")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "View all" })).toHaveAttribute(
      "href",
      "/org/acme/project/shop/agents/support-bot/agent-id?identityEnv=dev",
    );
    expect(last(rolesAndGroups.calls)).toEqual({ ...IDS, enabled: true });
  });

  it("copies the agent ID", () => {
    vi.useFakeTimers();
    renderCard();

    fireEvent.click(screen.getByRole("button", { name: "Copy Agent ID" }));
    expect(writeText).toHaveBeenCalledWith("tid-1");
    expect(screen.getByRole("button", { name: "Copied" })).toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(1500);
    });
    expect(screen.getByRole("button", { name: "Copy Agent ID" })).toBeInTheDocument();
  });

  it("says when nothing is assigned", () => {
    renderCard();

    expect(screen.getByText("No roles or groups assigned")).toBeInTheDocument();
  });

  it("reports a roles/groups load failure", () => {
    rolesAndGroups.current = { roles: [], groups: [], isLoading: false, isError: true };
    renderCard();

    expect(screen.getByText("Unable to load roles/groups. Try again later.")).toBeInTheDocument();
  });

  it("renders nothing and requests nothing when Agent ID is disabled", () => {
    accessState.flags.enableAgentIdentity = false;
    const { container } = renderCard();

    expect(container).toBeEmptyDOMElement();
    expect(last(apiState.calls.useAgentIdentityBinding)).toEqual([
      { orgId: "", projectId: "", agentId: "", envId: "" },
    ]);
    expect(last(apiState.calls.useListAgentIdentityAgents)).toEqual([{ orgName: "", envName: "" }]);
  });

  it("shows identity load and error states", () => {
    apiState.binding = { binding: undefined, provisioned: false, isLoading: false, isError: true };
    renderCard();

    expect(screen.getByText("Unable to load Agent ID")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Couldn't load the Agent ID for this environment. Refresh the page to try again.",
      ),
    ).toBeInTheDocument();
  });

  it("retries a failed provisioning", () => {
    apiState.binding = {
      binding: { status: "failed", lastError: "IdP unreachable" },
      provisioned: false,
      isLoading: false,
      isError: false,
    };
    renderCard();

    expect(screen.getByText("Provisioning Status : Failed")).toBeInTheDocument();
    expect(screen.getByText("Provisioning failed: IdP unreachable")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(mutation("useRetryAgentIdentityProvisioning")).toHaveBeenCalledWith({
      params: { orgName: "acme", projName: "shop", agentName: "support-bot" },
      body: { environment: "dev" },
    });
  });

  it("shows Retrying while a retry runs, and no Retry for read-only users", () => {
    apiState.binding = {
      binding: { status: "failed" },
      provisioned: false,
      isLoading: false,
      isError: false,
    };
    apiState.mutations.useRetryAgentIdentityProvisioning.isPending = true;
    const { unmount } = renderCard();
    expect(screen.getByRole("button", { name: "Retrying..." })).toBeDisabled();
    unmount();

    accessState.canWrite = false;
    renderCard();
    expect(screen.getByText("Provisioning failed")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Retry/ })).not.toBeInTheDocument();
  });

  it("offers to create a missing agent ID", () => {
    apiState.binding = { binding: undefined, provisioned: false, isLoading: false, isError: false };
    apiState.queries.useListAgentIdentityAgents = { data: { agents: [] } };
    renderCard();

    expect(screen.getByText("Not created for this environment yet")).toBeInTheDocument();
    expect(
      screen.getByText("This agent has no Agent ID for this environment yet."),
    ).toBeInTheDocument();
    expect(screen.queryByText("No roles or groups assigned")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Create agent ID" }));
    expect(mutation("useProvisionAgentIdentity")).toHaveBeenCalledWith({
      params: { orgName: "acme", projName: "shop", agentName: "support-bot" },
      query: { environment: "dev" },
    });
  });

  it("explains a failed create attempt", () => {
    apiState.binding = { binding: undefined, provisioned: false, isLoading: false, isError: false };
    apiState.queries.useListAgentIdentityAgents = { data: { agents: [] } };
    apiState.mutations.useProvisionAgentIdentity.isError = true;
    renderCard();

    expect(
      screen.getByText(/Couldn't create the Agent ID for this environment/),
    ).toBeInTheDocument();
  });

  it.each([
    [
      { binding: { status: "in_progress" }, provisioned: true },
      undefined,
      "Provisioning identity…",
    ],
    [{ binding: { status: "pending" }, provisioned: false }, undefined, "Agent ID not available"],
    [
      { binding: { status: "completed" }, provisioned: true },
      true,
      "Unable to load Agent ID. Try again later.",
    ],
  ])("explains a missing agent ID (%o)", (binding, agentsError, message) => {
    apiState.binding = { ...binding, isLoading: false, isError: false };
    apiState.queries.useListAgentIdentityAgents = { data: { agents: [] }, isError: agentsError };
    renderCard();

    expect(screen.getByText(message)).toBeInTheDocument();
  });
});
