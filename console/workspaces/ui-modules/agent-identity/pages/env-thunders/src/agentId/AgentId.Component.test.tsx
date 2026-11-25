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

import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { apiState, mutation, resetApiState, ROUTER_FUTURE, last } from "../__test__/mocks";
import { AgentIdComponent } from "./AgentId.Component";

vi.mock(
  "@agent-management-platform/api-client",
  async () => (await import("../__test__/mocks")).apiClientMock,
);

const identity = vi.hoisted(() => ({
  envs: { environments: [] as { name: string }[], isLoading: false, isError: false },
  credentials: {} as Record<string, unknown>,
  thunder: {} as Record<string, unknown>,
  rolesAndGroups: {} as Record<string, unknown>,
  regenerate: vi.fn(),
}));

vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  ...(await import("../__test__/mocks")).sharedComponentOverrides,
  useOrgEnvironmentsState: () => identity.envs,
  useAgentIdentityCredentials: () => identity.credentials,
  useThunderInstanceForEnv: () => identity.thunder,
  useAgentRolesAndGroups: () => identity.rolesAndGroups,
}));

// Oxygen UI pages are slow to render under a loaded CI runner.
vi.setConfig({ testTimeout: 60_000 });

const ENV = { orgName: "acme", envName: "dev" };

function Location() {
  const location = useLocation();
  return <output>{location.search}</output>;
}

const renderPage = (search = "") =>
  render(
    <MemoryRouter
      initialEntries={[`/org/acme/project/shop/agents/support-bot/agent-id${search}`]}
      future={ROUTER_FUTURE}
    >
      <Routes>
        <Route
          path="/org/:orgId/project/:projectId/agents/:agentId/agent-id"
          element={
            <>
              <AgentIdComponent />
              <Location />
            </>
          }
        />
      </Routes>
    </MemoryRouter>,
  );

const pickOption = async (placeholder: string, option: string) => {
  fireEvent.mouseDown(screen.getByPlaceholderText(placeholder));
  fireEvent.click(await screen.findByRole("option", { name: option }));
};
const rowOf = (text: string) => screen.getByText(text).closest("tr") as HTMLElement;

beforeEach(() => {
  vi.clearAllMocks();
  resetApiState();
  identity.envs = { environments: [{ name: "dev" }], isLoading: false, isError: false };
  identity.credentials = {
    binding: { status: "completed", clientId: "client-123", provisioningType: "internal" },
    provisioned: true,
    isLoading: false,
    isError: false,
    error: null,
    revealed: undefined,
    isRegenerating: false,
    regenerate: identity.regenerate,
  };
  identity.thunder = {
    thunderInstance: {
      issuerUrl: "https://idp.example/issuer",
      tokenUrl: "https://idp.example/token",
      jwksUrl: "https://idp.example/jwks",
    },
    isLoading: false,
    isError: false,
    error: null,
  };
  identity.rolesAndGroups = {
    roles: [{ id: "r1", name: "admin", description: "Full access" }],
    groups: [{ id: "g1", name: "on-call" }],
    isLoading: false,
  };
  apiState.queries.useListEnvironments = {
    data: [
      { name: "dev", displayName: "Development" },
      { name: "prod", displayName: "Production" },
    ],
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
  apiState.queries.useListAgentIdentityRoles = {
    data: {
      roles: [
        { id: "r1", name: "admin" },
        { id: "r2", name: "viewer" },
      ],
      total: 2,
    },
  };
  apiState.queries.useListAgentIdentityGroups = {
    data: {
      groups: [
        { id: "g1", name: "on-call" },
        { id: "g2", name: "readers" },
      ],
      total: 2,
    },
  };
});

describe("AgentIdComponent", () => {
  it("shows the client credentials, agent ID and OAuth2 endpoints", () => {
    renderPage();

    expect(screen.getByDisplayValue("client-123")).toBeInTheDocument();
    expect(screen.getByDisplayValue("••••••••")).toBeInTheDocument();
    expect(
      screen.getByText(
        "This secret was already generated and can't be shown again — regenerate to get a new one.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByDisplayValue("tid-1")).toBeInTheDocument();
    expect(screen.getByDisplayValue("https://idp.example/issuer")).toBeInTheDocument();
    expect(screen.getByDisplayValue("https://idp.example/token")).toBeInTheDocument();
    expect(screen.getByDisplayValue("https://idp.example/jwks")).toBeInTheDocument();
    // One environment: no selector. Internal agents get no "configure your agent" hint.
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    expect(screen.queryByText(/Configure your agent to request a JWT/)).not.toBeInTheDocument();
  });

  it("shows a freshly regenerated secret once, and tells external agents how to use it", () => {
    identity.credentials = {
      ...identity.credentials,
      binding: { status: "completed", provisioningType: "external" },
      revealed: { clientId: "client-new", clientSecret: "s3cret" },
    };
    renderPage();

    expect(screen.getByDisplayValue("client-new")).toBeInTheDocument();
    expect(screen.getByDisplayValue("s3cret")).toBeInTheDocument();
    expect(
      screen.getByText("This secret will not be shown again — copy it now."),
    ).toBeInTheDocument();
    expect(screen.getByText(/Configure your agent to request a JWT/)).toBeInTheDocument();
  });

  it("regenerates the secret", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Regenerate secret" }));

    expect(identity.regenerate).toHaveBeenCalledTimes(1);
  });

  it("disables regenerate while it is running", () => {
    identity.credentials = { ...identity.credentials, isRegenerating: true };
    renderPage();

    expect(screen.getByRole("button", { name: "Regenerating..." })).toBeDisabled();
  });

  it.each([
    [{ status: "failed" }, "Provisioning failed — check the identity settings for details."],
    [{ status: "pending" }, "Provisioning in progress…"],
  ])("explains a binding without credentials (%o)", (binding, message) => {
    identity.credentials = { ...identity.credentials, binding, provisioned: false };
    renderPage();

    expect(screen.getByText(message)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Regenerate secret" })).not.toBeInTheDocument();
  });

  it("shows identity load errors and a missing identity", () => {
    identity.credentials = {
      ...identity.credentials,
      isError: true,
      error: new Error("identity down"),
    };
    const { unmount } = renderPage();
    expect(screen.getByText("Failed to load agent identity")).toBeInTheDocument();
    expect(screen.getByText("identity down")).toBeInTheDocument();
    unmount();

    identity.credentials = { ...identity.credentials, isError: false, binding: undefined };
    renderPage();
    expect(screen.getByText("No agent identity")).toBeInTheDocument();
  });

  it("shows identity provider errors and a missing provider", () => {
    identity.thunder = {
      thunderInstance: undefined,
      isLoading: false,
      isError: true,
      error: new Error("idp down"),
    };
    const { unmount } = renderPage();
    expect(screen.getByText("Failed to load identity provider")).toBeInTheDocument();
    unmount();

    identity.thunder = {
      thunderInstance: undefined,
      isLoading: false,
      isError: false,
      error: null,
    };
    renderPage();
    expect(
      screen.getByText("No identity provider found for this environment."),
    ).toBeInTheDocument();
  });

  it("shows environment load states", () => {
    identity.envs = { environments: [], isLoading: false, isError: true };
    const { unmount } = renderPage();
    expect(screen.getByText("Failed to load environments")).toBeInTheDocument();
    unmount();

    identity.envs = { environments: [], isLoading: false, isError: false };
    renderPage();
    expect(
      screen.getByText("This agent isn't deployed to any environment yet."),
    ).toBeInTheDocument();
  });

  it("lists group memberships and saves changes", async () => {
    renderPage();
    fireEvent.click(screen.getByRole("tab", { name: "Groups" }));

    expect(screen.getByText("on-call")).toBeInTheDocument();
    await pickOption("Search groups...", "readers");
    fireEvent.click(within(rowOf("on-call")).getByRole("button"));
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument(),
    );
    expect(mutation("useAddAgentIdentityGroupMembers")).toHaveBeenCalledWith({
      params: { ...ENV, groupId: "g2" },
      body: { agentIds: ["tid-1"] },
    });
    expect(mutation("useRemoveAgentIdentityGroupMembers")).toHaveBeenCalledWith({
      params: { ...ENV, groupId: "g1" },
      body: { agentIds: ["tid-1"] },
    });
  });

  it("keeps failed role changes queued and names them", async () => {
    mutation("useAddAgentIdentityRoleAssignees").mockRejectedValue(new Error("nope"));
    renderPage();
    fireEvent.click(screen.getByRole("tab", { name: "Roles" }));

    expect(screen.getByText("Full access")).toBeInTheDocument();
    await pickOption("Search roles...", "viewer");
    fireEvent.click(within(rowOf("admin")).getByRole("button"));
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(
      await screen.findByText(
        "Failed to update: viewer. The rest were saved — retry to apply the remaining changes.",
      ),
    ).toBeInTheDocument();
    expect(mutation("useRemoveAgentIdentityRoleAssignees")).toHaveBeenCalledWith({
      params: { ...ENV, roleId: "r1" },
      body: { assignments: [{ id: "tid-1", type: "agent" }] },
    });
    // The failed add stays queued for a retry.
    expect(screen.getByText("viewer")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save changes" })).toBeInTheDocument();
  });

  it("discards pending changes on Cancel", async () => {
    renderPage();
    fireEvent.click(screen.getByRole("tab", { name: "Roles" }));

    await pickOption("Search roles...", "viewer");
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.queryByText("viewer")).not.toBeInTheDocument();
    expect(mutation("useAddAgentIdentityRoleAssignees")).not.toHaveBeenCalled();
  });

  it("warns when the role catalog is truncated", () => {
    apiState.queries.useListAgentIdentityRoles = {
      data: { roles: [{ id: "r2", name: "viewer" }], total: 150 },
    };
    renderPage();
    fireEvent.click(screen.getByRole("tab", { name: "Roles" }));

    expect(screen.getByText(/Showing the first 100 roles/)).toBeInTheDocument();
  });

  it("blocks assignment changes without an identity binding", () => {
    apiState.queries.useListAgentIdentityAgents = { data: { agents: [] } };
    identity.rolesAndGroups = { roles: [], groups: [], isLoading: false };
    renderPage();

    expect(screen.getByText(/no active identity binding in this environment/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: "Groups" }));
    expect(screen.queryByPlaceholderText("Search groups...")).not.toBeInTheDocument();
    expect(screen.getByText("No groups assigned")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: "Roles" }));
    expect(screen.getByText("No roles assigned")).toBeInTheDocument();
  });

  it("switches environment from the selector", async () => {
    identity.envs = {
      environments: [{ name: "dev" }, { name: "prod" }],
      isLoading: false,
      isError: false,
    };
    renderPage("?identityEnv=prod");

    expect(screen.getByText("Production Environment")).toBeInTheDocument();
    expect(last(apiState.calls.useListAgentIdentityAgents)).toEqual([
      { orgName: "acme", envName: "prod" },
    ]);

    fireEvent.mouseDown(screen.getByRole("combobox"));
    fireEvent.click(await screen.findByRole("option", { name: "Development" }));

    await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("?identityEnv=dev"));
  });

  it("corrects a link to an environment the agent isn't in", async () => {
    identity.envs = {
      environments: [{ name: "dev" }, { name: "prod" }],
      isLoading: false,
      isError: false,
    };
    renderPage("?identityEnv=gone");

    await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("?identityEnv=dev"));
    expect(screen.getByText("Development Environment")).toBeInTheDocument();
  });
});
