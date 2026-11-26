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

import { act } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  invalidatedKeys, renderHook, seed, testQueryClient, waitFor, type RenderedHook,
} from "../../test-utils/render-hook";

const ui = vi.hoisted(() => ({
  pushSnackBar: vi.fn(),
  identityPresent: true,
  getToken: async () => "tok",
}));
const api = vi.hoisted(() => ({
  listAgents: vi.fn(),
  listOrgAgents: vi.fn(),
  getAgent: vi.fn(),
  createAgent: vi.fn(),
  updateAgent: vi.fn(),
  deleteAgent: vi.fn(),
  generateAgentToken: vi.fn(),
  getAgentRoles: vi.fn(),
  getAgentGroups: vi.fn(),
  getAgentIdentity: vi.fn(),
  provisionAgentIdentity: vi.fn(),
  regenerateAgentIdentitySecret: vi.fn(),
  retryAgentIdentityProvisioning: vi.fn(),
  revokeAgentIdentitySecret: vi.fn(),
}));

vi.mock("@agent-management-platform/views", () => ({
  useSnackBar: () => ({ pushSnackBar: ui.pushSnackBar }),
  useIsUIModuleInstalled: (id: string) => (id === "agent-identity" ? ui.identityPresent : true),
}));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: ui.getToken, isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("./telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/agents", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../apis/agents")>()),
  ...api,
}));

import * as hooks from "./agents";

let rendered: RenderedHook<unknown> | undefined;
type Wrapper = (children: React.ReactNode) => React.ReactNode;
async function render<T>(useHook: () => T, wrapper?: Wrapper) {
  const hook = await renderHook(useHook, { wrapper });
  rendered = hook as RenderedHook<unknown>;
  return hook;
}

afterEach(() => {
  rendered?.unmount();
  rendered = undefined;
  vi.resetAllMocks();
  ui.identityPresent = true;
});

const p = { orgName: "o", projName: "p", agentName: "a" };
const env = { environment: "dev" };

// An Agent document as the API serves it, and the flat agent the hooks hand the UI.
const agentDoc = (name: string) => ({
  apiVersion: "ai-control-plane.choreolabs.com/v1",
  kind: "Agent",
  metadata: { name, project: "p" },
  spec: { displayName: name.toUpperCase(), provisioning: { type: "external" }, agentType: { type: "t" } },
});
const flatAgent = (name: string) => ({
  name, displayName: name.toUpperCase(), description: "", createdAt: "", projectName: "p",
  provisioning: { type: "external" }, agentType: { type: "t" },
  uuid: undefined, status: undefined, labels: undefined, createdBy: undefined,
});

type QueryRow = [
  string, () => { data?: unknown }, keyof typeof api, unknown[], unknown[], unknown?, unknown?,
];
const queries: QueryRow[] = [
  ["useListAgents", () => hooks.useListAgents({ orgName: "o", projName: "p" }, { limit: 5 } as never),
    "listAgents", [{ orgName: "o", projName: "p" }, { limit: 5 }],
    ["agents", { orgName: "o", projName: "p" }, { limit: 5 }],
    { list: [agentDoc("a")], pagination: { total: 1, limit: 5, offset: 0 } },
    { agents: [flatAgent("a")], total: 1, limit: 5, offset: 0 }],
  ["useListOrgAgents", () => hooks.useListOrgAgents({ orgName: "o" }), "listOrgAgents",
    [{ orgName: "o" }], ["org-agents", { orgName: "o" }]],
  ["useGetAgent", () => hooks.useGetAgent(p), "getAgent", [p], ["agent", p], agentDoc("a"), flatAgent("a")],
  ["useGetAgentRoles", () => hooks.useGetAgentRoles(p, env), "getAgentRoles", [p, env],
    ["agent-roles", p, env]],
  ["useGetAgentGroups", () => hooks.useGetAgentGroups(p, env), "getAgentGroups", [p, env],
    ["agent-groups", p, env]],
  ["useGetAgentIdentity", () => hooks.useGetAgentIdentity(p, env), "getAgentIdentity", [p, env],
    ["agent-identity", p, env]],
];

describe.each(queries)("%s", (_name, useHook, fn, args, key, served, handed) => {
  it("loads through the API with the caller's token and caches under its key", async () => {
    const data = served ?? [{ status: "completed" }];
    api[fn].mockResolvedValue(data);

    const hook = await render(useHook);
    await waitFor(() => hook.result.current.data !== undefined);

    expect(hook.result.current.data).toEqual(handed ?? data);
    expect(api[fn]).toHaveBeenCalledWith(...args, ui.getToken);
    expect(hook.client.getQueryData(key)).toEqual(handed ?? data);
  });
});

describe("query gating", () => {
  it.each([
    ["useListAgents without a project", () => hooks.useListAgents({ orgName: "o" } as never), "listAgents"],
    ["useListOrgAgents without an org", () => hooks.useListOrgAgents({} as never), "listOrgAgents"],
    ["useGetAgent without an agent", () => hooks.useGetAgent({ orgName: "o", projName: "p" } as never), "getAgent"],
    ["useGetAgentRoles without an environment",
      () => hooks.useGetAgentRoles(p, { environment: "" }), "getAgentRoles"],
    ["useGetAgentRoles when switched off",
      () => hooks.useGetAgentRoles(p, env, { enabled: false }), "getAgentRoles"],
    ["useGetAgentGroups without an agent",
      () => hooks.useGetAgentGroups({ orgName: "o", projName: "p" } as never, env), "getAgentGroups"],
    ["useGetAgentIdentity without a project",
      () => hooks.useGetAgentIdentity({ orgName: "o", agentName: "a" } as never), "getAgentIdentity"],
  ] as [string, () => { fetchStatus: string }, keyof typeof api][])(
    "%s stays idle",
    async (_name, useHook, fn) => {
      const hook = await render(useHook);
      expect(hook.result.current.fetchStatus).toBe("idle");
      expect(api[fn]).not.toHaveBeenCalled();
    },
  );

  it("does not read an agent's identity when the Agent Identity module is absent", async () => {
    ui.identityPresent = false;
    const hook = await render(() => hooks.useGetAgentIdentity(p, env));
    expect(hook.result.current.fetchStatus).toBe("idle");
    expect(api.getAgentIdentity).not.toHaveBeenCalled();
  });
});

describe("useGetAgentIdentity polling", () => {
  it("re-reads while a binding is still provisioning, and stops once all settle", async () => {
    api.getAgentIdentity
      .mockResolvedValueOnce([{ status: "pending" }])
      .mockResolvedValue([{ status: "completed" }]);
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const hook = await render(() => hooks.useGetAgentIdentity(p, env));
      await waitFor(() => hook.result.current.data !== undefined);
      expect(api.getAgentIdentity).toHaveBeenCalledTimes(1);

      await act(async () => { await vi.advanceTimersByTimeAsync(15_000); });
      await waitFor(() => (hook.result.current.data as { status: string }[])[0].status === "completed");
      expect(api.getAgentIdentity).toHaveBeenCalledTimes(2);

      await act(async () => { await vi.advanceTimersByTimeAsync(60_000); });
      expect(api.getAgentIdentity).toHaveBeenCalledTimes(2);
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("useAgentIdentityBinding", () => {
  const ids = { orgId: "o", projectId: "p", agentId: "a", envId: "dev" };

  it("reports the environment's binding as provisioned once completed", async () => {
    api.getAgentIdentity.mockResolvedValue([{ status: "completed", clientId: "c1" }]);
    const hook = await render(() => hooks.useAgentIdentityBinding(ids));
    await waitFor(() => hook.result.current.binding !== undefined);
    expect(hook.result.current).toMatchObject({
      binding: { clientId: "c1" }, provisioned: true, isError: false,
    });
    expect(api.getAgentIdentity).toHaveBeenCalledWith(
      { orgName: "o", projName: "p", agentName: "a" }, { environment: "dev" }, ui.getToken,
    );
  });

  it("is not provisioned while failed, and surfaces a read error", async () => {
    api.getAgentIdentity.mockResolvedValue([{ status: "failed" }]);
    const failed = await render(() => hooks.useAgentIdentityBinding(ids));
    await waitFor(() => failed.result.current.binding !== undefined);
    expect(failed.result.current.provisioned).toBe(false);
    failed.unmount();

    api.getAgentIdentity.mockRejectedValue(Object.assign(new Error("x"), { status: 500 }));
    const broken = await render(() => hooks.useAgentIdentityBinding(ids));
    await waitFor(() => broken.result.current.isError);
    expect(broken.result.current).toMatchObject({ binding: undefined, provisioned: false });
  });
});

describe("useOrgAgentDisplayNames", () => {
  it("resolves display names per project, falling back to the raw names", async () => {
    api.listOrgAgents.mockResolvedValue({
      agents: [
        { name: "bot", projectName: "p1", displayName: "Support Bot", projectDisplayName: "Project One" },
        { name: "bot", projectName: "p2", displayName: "Sales Bot" },
      ],
    });
    const hook = await render(() => {
      const resolver = hooks.useOrgAgentDisplayNames({ orgName: "o" });
      return { agent: resolver.resolveAgentName, project: resolver.resolveProjectName };
    });
    await waitFor(() => hook.result.current.agent("p1", "bot") !== "bot");

    const { agent, project } = hook.result.current;
    expect(agent("p1", "bot")).toBe("Support Bot");
    expect(agent("p2", "bot")).toBe("Sales Bot");
    expect(agent("p3", "bot")).toBe("bot");
    expect(project("p1", "bot")).toBe("Project One");
    expect(project("p2", "bot")).toBe("p2");
  });
});

describe("useRouteAgent", () => {
  const at = (path: string) => (children: React.ReactNode) => (
    <MemoryRouter
      initialEntries={[path]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route path="/org/:orgId/project/:projectId/agents/:agentId" element={children} />
        <Route path="*" element={children} />
      </Routes>
    </MemoryRouter>
  );

  it("loads the agent named in the URL", async () => {
    api.getAgent.mockResolvedValue(agentDoc("a1"));
    const hook = await render(() => hooks.useRouteAgent(), at("/org/o1/project/p1/agents/a1"));
    await waitFor(() => hook.result.current.data !== undefined);
    expect(api.getAgent).toHaveBeenCalledWith(
      { orgName: "o1", projName: "p1", agentName: "a1" }, ui.getToken,
    );
  });

  it("stays idle on a page with no agent", async () => {
    const hook = await render(() => hooks.useRouteAgent(), at("/org/o1/catalog"));
    expect(hook.result.current.fetchStatus).toBe("idle");
    expect(api.getAgent).not.toHaveBeenCalled();
  });
});

describe("useGenerateAgentToken", () => {
  it("mints nothing until asked, then mints with the latest body and query", async () => {
    api.generateAgentToken.mockResolvedValue({ token: "jwt" });
    const idle = await render(() => hooks.useGenerateAgentToken(p, { expiresIn: "1h" } as never, env));
    expect(api.generateAgentToken).not.toHaveBeenCalled();
    idle.unmount();

    const hook = await render(() =>
      hooks.useGenerateAgentToken(p, { expiresIn: "1h" } as never, env, true),
    );
    await waitFor(() => hook.result.current.data !== undefined);
    expect(hook.result.current.data).toEqual({ token: "jwt" });
    expect(api.generateAgentToken).toHaveBeenCalledWith(p, { expiresIn: "1h" }, env, ui.getToken);
    expect(hook.client.getQueryData(["agent-token", "a", "p", "o", "dev"])).toEqual({ token: "jwt" });
  });
});

const SEEDED = [
  ["agents", {}],
  ["org-agents", {}],
  ["agent", p],
  ["agent-identity", p],
  ["agent-identity", { ...p, agentName: "other" }],
  ["projects", {}],
] as const;

type MutationRow = [
  string,
  () => { mutateAsync: (v: never) => Promise<unknown> },
  unknown,
  keyof typeof api,
  unknown[],
  string,
  unknown[][],
  unknown?,
];
const mutations: MutationRow[] = [
  ["useCreateAgent", hooks.useCreateAgent,
    { params: p, body: { name: "a", displayName: "A", provisioning: { type: "external" }, agentType: { type: "t" } } },
    "createAgent",
    [p, {
      apiVersion: "ai-control-plane.choreolabs.com/v1", kind: "Agent", metadata: { name: "a", project: "p" },
      spec: { displayName: "A", provisioning: { type: "external" }, agentType: { type: "t" } },
    }],
    "Agent created successfully", [["agents", {}], ["org-agents", {}]], agentDoc("a")],
  ["useUpdateAgent", hooks.useUpdateAgent, { params: p, body: { displayName: "A", description: "d" } }, "updateAgent",
    [p, {
      apiVersion: "ai-control-plane.choreolabs.com/v1", kind: "Agent", metadata: { name: "a", project: "p" },
      spec: { displayName: "A", description: "d" },
    }],
    "Agent updated successfully",
    [["agents", {}], ["org-agents", {}], ["agent", p]], agentDoc("a")],
  ["useDeleteAgent", hooks.useDeleteAgent, p, "deleteAgent", [p], "Agent deleted successfully",
    [["agents", {}], ["org-agents", {}]]],
  ["useProvisionAgentIdentity", hooks.useProvisionAgentIdentity, { params: p, query: env },
    "provisionAgentIdentity", [p, env], "Agent Identity created successfully",
    [["agent-identity", p]]],
  ["useRegenerateAgentIdentitySecret", hooks.useRegenerateAgentIdentitySecret,
    { params: p, body: env }, "regenerateAgentIdentitySecret", [p, env],
    "Agent Identity Secret rotated successfully", [["agent-identity", p]]],
  ["useRetryAgentIdentityProvisioning", hooks.useRetryAgentIdentityProvisioning,
    { params: p, body: env }, "retryAgentIdentityProvisioning", [p, env],
    "Agent Identity Provisioning triggered successfully", [["agent-identity", p]]],
  ["useRevokeAgentIdentitySecret", hooks.useRevokeAgentIdentitySecret, { params: p, query: env },
    "revokeAgentIdentitySecret", [p, env], "Agent Identity Secret revoked successfully",
    [["agent-identity", p]]],
];

describe.each(mutations)("%s", (_name, useHook, variables, fn, args, message, invalidated, served) => {
  it("calls the API, announces success and refreshes exactly the affected queries", async () => {
    api[fn].mockResolvedValue(served ?? { ok: true });
    const client = testQueryClient();
    seed(client, SEEDED);
    const hook = await renderHook(useHook, { client });
    rendered = hook as RenderedHook<unknown>;

    await act(async () => { await hook.result.current.mutateAsync(variables as never); });

    expect(api[fn]).toHaveBeenCalledWith(...args, ui.getToken);
    expect(ui.pushSnackBar).toHaveBeenCalledWith({ message, type: "success" });
    expect(invalidatedKeys(client, SEEDED)).toEqual(invalidated);
  });

  it("shows an error and refreshes nothing when the API fails", async () => {
    api[fn].mockRejectedValue(Object.assign(new Error("HTTP error! status: 500"), { status: 500 }));
    const client = testQueryClient();
    seed(client, SEEDED);
    const hook = await renderHook(useHook, { client });
    rendered = hook as RenderedHook<unknown>;

    await act(async () => {
      await hook.result.current.mutateAsync(variables as never).catch(() => undefined);
    });

    expect(ui.pushSnackBar).toHaveBeenCalledWith(expect.objectContaining({ type: "error" }));
    expect(invalidatedKeys(client, SEEDED)).toEqual([]);
  });
});

it("explains a failed Agent ID creation in plain words", async () => {
  api.provisionAgentIdentity.mockRejectedValue(new Error("dial tcp: i/o timeout"));
  const hook = await render(hooks.useProvisionAgentIdentity);
  await act(async () => {
    await hook.result.current.mutateAsync({ params: p, query: env }).catch(() => undefined);
  });
  expect(ui.pushSnackBar.mock.calls[0][0].message).toMatch(/^Couldn't create the Agent ID/);
});
