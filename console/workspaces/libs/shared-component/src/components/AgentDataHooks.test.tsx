/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

// The small data hooks and components that sit directly on api-client hooks:
// roles/groups, AgentID credentials, the Thunder instance lookup, the
// environment card and the environment selector.

import { act, fireEvent, render, renderHook, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  useGetAgentRoles: vi.fn(),
  useGetAgentGroups: vi.fn(),
  useAgentIdentityBinding: vi.fn(),
  useRegenerateAgentIdentitySecret: vi.fn(),
  useListThunderInstances: vi.fn(),
  useGetAgent: vi.fn(),
  useListEnvironments: vi.fn(),
}));
vi.mock("@agent-management-platform/api-client", () => api);

import { RolesGroupsChips, useAgentRolesAndGroups } from "./AgentRolesGroups/AgentRolesGroups";
import { useAgentIdentityCredentials } from "./AgentIdentityCredentials/AgentIdentityCredentials";
import { useThunderInstanceForEnv } from "./ThunderInstanceForEnv/ThunderInstanceForEnv";
import { EnvironmentCard } from "./EnvironmentCard/EnvironmentCard";
import { EnvironmentSelector } from "./EnvironmentSelector/EnvironmentSelector";

const ids = { orgId: "org", projectId: "proj", agentId: "ag", envId: "dev" };

describe("useAgentRolesAndGroups", () => {
  it("queries both lists for the environment, gated by enabled", () => {
    api.useGetAgentRoles.mockReturnValue({
      data: { roles: [{ id: "r", name: "reader" }] },
      isLoading: false,
      isError: false,
    });
    api.useGetAgentGroups.mockReturnValue({ data: undefined, isLoading: true, isError: false });
    const { result } = renderHook(() => useAgentRolesAndGroups({ ...ids, enabled: false }));
    const path = { orgName: "org", projName: "proj", agentName: "ag" };
    expect(api.useGetAgentRoles).toHaveBeenCalledWith(path, { environment: "dev" }, { enabled: false });
    expect(api.useGetAgentGroups).toHaveBeenCalledWith(path, { environment: "dev" }, { enabled: false });
    expect(result.current).toEqual({
      roles: [{ id: "r", name: "reader" }],
      groups: [],
      isLoading: true,
      isError: false,
      error: undefined,
    });
  });

  it("reports an error from either list", () => {
    const err = new Error("403");
    const idle = { data: undefined, isLoading: false };
    api.useGetAgentRoles.mockReturnValue({ ...idle, isError: false, error: null });
    api.useGetAgentGroups.mockReturnValue({ ...idle, isError: true, error: err });
    const { result } = renderHook(() => useAgentRolesAndGroups({ ...ids, enabled: true }));
    expect(result.current.isError).toBe(true);
    expect(result.current.error).toBe(err);
  });
});

describe("RolesGroupsChips", () => {
  it("shows skeletons while loading", () => {
    const { container } = render(<RolesGroupsChips roles={[]} groups={[]} isLoading />);
    expect(container.querySelectorAll(".MuiSkeleton-root")).toHaveLength(2);
  });

  it("shows chips or empty text for each list", () => {
    render(
      <RolesGroupsChips roles={[{ id: "1", name: "admin" }, { id: "2", name: "viewer" }]} groups={[]} isLoading={false} />,
    );
    expect(screen.getByText("admin")).toBeInTheDocument();
    expect(screen.getByText("viewer")).toBeInTheDocument();
    expect(screen.getByText("Not a member of any groups.")).toBeInTheDocument();
    expect(screen.queryByText("No roles assigned.")).not.toBeInTheDocument();
  });
});

describe("useAgentIdentityCredentials", () => {
  const mutateAsync = vi.fn();
  beforeEach(() => {
    mutateAsync.mockReset();
    api.useAgentIdentityBinding.mockReturnValue({
      binding: { clientId: "cid" },
      provisioned: true,
      isLoading: false,
      isError: false,
      error: null,
    });
    api.useRegenerateAgentIdentitySecret.mockReturnValue({ mutateAsync, isPending: false });
  });

  it("reveals the new secret after a regenerate", async () => {
    mutateAsync.mockResolvedValue({ clientId: "cid", clientSecret: "shh" });
    const { result } = renderHook(() => useAgentIdentityCredentials(ids));
    expect(result.current.binding).toEqual({ clientId: "cid" });
    expect(result.current.provisioned).toBe(true);
    expect(result.current.revealed).toBeNull();
    await act(() => result.current.regenerate());
    expect(mutateAsync).toHaveBeenCalledWith({
      params: { orgName: "org", projName: "proj", agentName: "ag" },
      body: { environment: "dev" },
    });
    expect(result.current.revealed).toEqual({ clientId: "cid", clientSecret: "shh" });
  });

  it("reveals nothing when regenerate fails", async () => {
    mutateAsync.mockRejectedValue(new Error("nope"));
    const { result } = renderHook(() => useAgentIdentityCredentials(ids));
    await act(() => result.current.regenerate());
    expect(result.current.revealed).toBeNull();
  });
});

describe("useThunderInstanceForEnv", () => {
  it("picks the instance bound to the environment", () => {
    api.useListThunderInstances.mockReturnValue({
      data: { thunderInstances: [{ envName: "prod", issuer: "p" }, { envName: "dev", issuer: "d" }] },
      isLoading: false,
      isError: false,
      error: null,
    });
    const { result } = renderHook(() => useThunderInstanceForEnv({ orgId: "org", envId: "dev" }));
    expect(api.useListThunderInstances).toHaveBeenCalledWith({ orgName: "org" });
    expect(result.current.thunderInstance).toEqual({ envName: "dev", issuer: "d" });
  });

  it("is undefined while the list is loading", () => {
    api.useListThunderInstances.mockReturnValue({ data: undefined, isLoading: true });
    const { result } = renderHook(() => useThunderInstanceForEnv({ orgId: "org", envId: "dev" }));
    expect(result.current.thunderInstance).toBeUndefined();
    expect(result.current.isLoading).toBe(true);
  });
});

describe("EnvironmentCard", () => {
  const base = { orgId: "org", projectId: "proj", agentId: "ag" };

  it("shows a skeleton while the agent loads", () => {
    api.useGetAgent.mockReturnValue({ isLoading: true });
    const { container } = render(<EnvironmentCard {...base} />);
    expect(container.querySelector(".MuiSkeleton-root")).toBeInTheDocument();
    expect(api.useGetAgent).toHaveBeenCalledWith({ orgName: "org", projName: "proj", agentName: "ag" });
  });

  it("shows the environment name, actions and body", () => {
    api.useGetAgent.mockReturnValue({ isLoading: false });
    render(
      <EnvironmentCard
        {...base}
        environment={{ name: "dev", displayName: "Development" } as never}
        actions={<button type="button">Deploy</button>}
        bottomContent={<span>capabilities</span>}
      />,
    );
    expect(screen.getByText("Development")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Deploy" })).toBeInTheDocument();
    expect(screen.getByText("capabilities")).toBeInTheDocument();
  });

  it.each([
    [{}, "Environment"],
    [{ environment: { name: "dev" } }, "dev"],
    [{ tabsHeader: <span>tabs</span> }, "tabs"],
  ])("titles the card from %j", (props, title) => {
    api.useGetAgent.mockReturnValue({ isLoading: false });
    render(<EnvironmentCard {...base} {...(props as object)} />);
    expect(screen.getByText(title)).toBeInTheDocument();
  });

  it("can hide the title entirely", () => {
    api.useGetAgent.mockReturnValue({ isLoading: false });
    render(<EnvironmentCard {...base} hideEnvTitle environment={{ name: "dev" } as never} />);
    expect(screen.queryByText("dev")).not.toBeInTheDocument();
  });
});

describe("EnvironmentSelector", () => {
  function Where() {
    return <span data-testid="where">{useLocation().pathname}</span>;
  }
  const renderAt = (path: string) =>
    render(
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route
            path="/org/:orgId/environment/:envId/*"
            element={
              <>
                <EnvironmentSelector />
                <Where />
              </>
            }
          />
          <Route path="/org/:orgId/*" element={<EnvironmentSelector />} />
        </Routes>
      </MemoryRouter>,
    );

  it("switches environments by rewriting the URL", () => {
    api.useListEnvironments.mockReturnValue({
      data: [
        { name: "dev", displayName: "Development" },
        { name: "prod" },
      ],
      isLoading: false,
      isError: false,
    });
    renderAt("/org/acme/environment/dev/gateways");
    expect(api.useListEnvironments).toHaveBeenCalledWith({ orgName: "acme" });
    expect(screen.getByText("Development Environment")).toBeInTheDocument();
    fireEvent.mouseDown(screen.getByRole("combobox"));
    const listbox = screen.getByRole("listbox");
    expect(within(listbox).getByText("Development")).toBeInTheDocument();
    fireEvent.click(within(listbox).getByText("prod"));
    expect(screen.getByTestId("where")).toHaveTextContent("/org/acme/environment/prod/gateways");
  });

  it("falls back to the env id when it is not in the list", () => {
    api.useListEnvironments.mockReturnValue({ data: [{ name: "a" }, { name: "b" }], isLoading: false, isError: false });
    renderAt("/org/acme/environment/zzz/x");
    expect(screen.getByText("zzz Environment")).toBeInTheDocument();
  });

  it.each([
    ["only one environment", "/org/acme/environment/dev/x", { data: [{ name: "dev" }], isError: false }],
    ["no env in the URL", "/org/acme/x", { data: [{ name: "a" }, { name: "b" }], isError: false }],
    ["the list failed", "/org/acme/environment/a/x", { data: [{ name: "a" }, { name: "b" }], isError: true }],
  ])("renders nothing with %s", (_label, path, state) => {
    api.useListEnvironments.mockReturnValue({ ...state, isLoading: false });
    renderAt(path);
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
  });
});
