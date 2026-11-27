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
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  invalidatedKeys, renderHook, seed, testQueryClient, waitFor, type RenderedHook,
} from "../../test-utils/render-hook";

const ui = vi.hoisted(() => ({ pushSnackBar: vi.fn(), getToken: async () => "tok" }));
const api = vi.hoisted(() => ({
  listProjects: vi.fn(),
  getProject: vi.fn(),
  createProject: vi.fn(),
  updateProject: vi.fn(),
  deleteProject: vi.fn(),
}));

vi.mock("@agent-management-platform/views", () => ({
  useSnackBar: () => ({ pushSnackBar: ui.pushSnackBar }),
}));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: ui.getToken, isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("./telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/projects", () => api);

import * as hooks from "./projects";

let rendered: RenderedHook<unknown> | undefined;
async function render<T>(useHook: () => T) {
  const hook = await renderHook(useHook);
  rendered = hook as RenderedHook<unknown>;
  return hook;
}

afterEach(() => {
  rendered?.unmount();
  rendered = undefined;
  vi.resetAllMocks();
});

const org = { orgName: "o" };
const proj = { orgName: "o", projName: "p" };

describe("project queries", () => {
  it("useListProjects lists one page under its key", async () => {
    api.listProjects.mockResolvedValue({ list: [{ metadata: { name: "p" } }], pagination: { total: 1 } });
    const hook = await render(() => hooks.useListProjects(org, { limit: 10 } as never));
    await waitFor(() => hook.result.current.data !== undefined);
    expect(api.listProjects).toHaveBeenCalledWith(org, { limit: 10 }, ui.getToken);
    expect(hook.client.getQueryData(["projects", org, { limit: 10 }])).toEqual({
      list: [{ metadata: { name: "p" } }], pagination: { total: 1 },
    });
  });

  it("useListAllProjects gathers every page", async () => {
    api.listProjects
      .mockResolvedValueOnce({ list: Array(100).fill({ metadata: { name: "x" } }), pagination: { total: 101, limit: 100, offset: 0 } })
      .mockResolvedValueOnce({ list: [{ metadata: { name: "last" } }], pagination: { total: 101, limit: 100, offset: 100 } });
    const hook = await render(() => hooks.useListAllProjects(org));
    await waitFor(() => hook.result.current.data !== undefined);
    expect(hook.result.current.data!.list).toHaveLength(101);
    expect(hook.result.current.data!.pagination?.limit).toBe(101);
    expect(api.listProjects.mock.calls[1]).toEqual([org, { limit: 100, offset: 100 }, ui.getToken]);
    expect(hook.client.getQueryData(["projects", org, "all"])).toBeDefined();
  });

  it("useGetProject reads one project", async () => {
    api.getProject.mockResolvedValue({ name: "p" });
    const hook = await render(() => hooks.useGetProject(proj));
    await waitFor(() => hook.result.current.data !== undefined);
    expect(api.getProject).toHaveBeenCalledWith(proj, ui.getToken);
    expect(hook.client.getQueryData(["project", proj])).toEqual({ name: "p" });
  });

  it.each([
    ["useListProjects", () => hooks.useListProjects({} as never)],
    ["useListAllProjects", () => hooks.useListAllProjects({} as never)],
    ["useGetProject", () => hooks.useGetProject(org as never)],
  ])("%s stays idle without its names", async (_name, useHook) => {
    const hook = await render(useHook as () => { fetchStatus: string });
    expect(hook.result.current.fetchStatus).toBe("idle");
    expect(api.listProjects).not.toHaveBeenCalled();
    expect(api.getProject).not.toHaveBeenCalled();
  });
});

const SEEDED = [["projects", org], ["project", proj], ["agents", {}]] as const;
const body = { displayName: "P" };

describe.each([
  ["useCreateProject", () => hooks.useCreateProject(org), body, "createProject", [org, body],
    "Project created successfully", [["projects", org]]],
  ["useUpdateProject", () => hooks.useUpdateProject(proj), body, "updateProject", [proj, body],
    "Project updated successfully", [["projects", org], ["project", proj]]],
  ["useDeleteProject", hooks.useDeleteProject, proj, "deleteProject", [proj],
    "Project deleted successfully", [["projects", org]]],
] as [string, () => { mutateAsync: (v: never) => Promise<unknown> }, unknown, keyof typeof api,
  unknown[], string, unknown[][]][])("%s", (_name, useHook, variables, fn, args, message, invalidated) => {
  it("calls the API, announces success and refreshes the project queries", async () => {
    api[fn].mockResolvedValue({});
    const client = testQueryClient();
    seed(client, SEEDED);
    const hook = await renderHook(useHook, { client });
    rendered = hook as RenderedHook<unknown>;
    await act(async () => { await hook.result.current.mutateAsync(variables as never); });
    expect(api[fn]).toHaveBeenCalledWith(...args, ui.getToken);
    expect(ui.pushSnackBar).toHaveBeenCalledWith({ message, type: "success" });
    expect(invalidatedKeys(client, SEEDED)).toEqual(invalidated);
  });

  it("shows the server's message and refreshes nothing on failure", async () => {
    api[fn].mockRejectedValue(Object.assign(new Error("Project has agents"), { status: 409 }));
    const client = testQueryClient();
    seed(client, SEEDED);
    const hook = await renderHook(useHook, { client });
    rendered = hook as RenderedHook<unknown>;
    await act(async () => {
      await hook.result.current.mutateAsync(variables as never).catch(() => undefined);
    });
    expect(ui.pushSnackBar).toHaveBeenCalledWith({ message: "Project has agents", type: "error" });
    expect(invalidatedKeys(client, SEEDED)).toEqual([]);
  });
});
