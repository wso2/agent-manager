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
  renderHook, testQueryClient, waitFor, type RenderedHook,
} from "../../test-utils/render-hook";

const ui = vi.hoisted(() => ({
  pushSnackBar: vi.fn(),
  getToken: async () => "tok",
  auth: { isAuthenticated: true, userInfo: { sub: "user-1" } as { sub?: string } | undefined },
}));
const api = vi.hoisted(() => ({
  listOrganizations: vi.fn(),
  getOrganization: vi.fn(),
  generateResourceName: vi.fn(),
}));

vi.mock("@agent-management-platform/views", () => ({
  useSnackBar: () => ({ pushSnackBar: ui.pushSnackBar }),
}));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: ui.getToken, logout: vi.fn(), ...ui.auth }),
}));
vi.mock("./telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/organizations", () => api);

import * as hooks from "./organizations";

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
  ui.auth = { isAuthenticated: true, userInfo: { sub: "user-1" } };
});

describe("useListOrganizations", () => {
  it("lists the signed-in user's orgs, keyed by who is signed in", async () => {
    api.listOrganizations.mockResolvedValue({ organizations: [{ name: "acme" }] });
    const hook = await render(() => hooks.useListOrganizations({ limit: 5 } as never));
    await waitFor(() => hook.result.current.data !== undefined);
    expect(api.listOrganizations).toHaveBeenCalledWith({ limit: 5 }, ui.getToken);
    expect(hook.client.getQueryData(["organizations", { limit: 5 }, true, "user-1"])).toEqual({
      organizations: [{ name: "acme" }],
    });
  });

  it("does not retry a failure", async () => {
    api.listOrganizations.mockRejectedValue(Object.assign(new Error("x"), { status: 503 }));
    const client = testQueryClient();
    client.setDefaultOptions({ queries: { retry: 3, retryDelay: 0 } });
    const hook = await renderHook(() => hooks.useListOrganizations(), { client });
    rendered = hook as RenderedHook<unknown>;
    await waitFor(() => hook.result.current.isError);
    expect(api.listOrganizations).toHaveBeenCalledTimes(1);
  });
});

describe("useGetOrganization", () => {
  it("reads one org", async () => {
    api.getOrganization.mockResolvedValue({ name: "acme", handle: "acme-co" });
    const hook = await render(() => hooks.useGetOrganization({ orgName: "acme" }));
    await waitFor(() => hook.result.current.data !== undefined);
    expect(api.getOrganization).toHaveBeenCalledWith({ orgName: "acme" }, ui.getToken);
    expect(hook.client.getQueryData(["organization", { orgName: "acme" }])).toBeDefined();
  });

  it("stays idle without an org name", async () => {
    const hook = await render(() => hooks.useGetOrganization({} as never));
    expect(hook.result.current.fetchStatus).toBe("idle");
    expect(api.getOrganization).not.toHaveBeenCalled();
  });
});

describe("useGenerateResourceName", () => {
  it("returns a name without announcing it", async () => {
    api.generateResourceName.mockResolvedValue({ name: "my-agent" });
    const hook = await render(() => hooks.useGenerateResourceName({ orgName: "o" }));
    let result: unknown;
    await act(async () => {
      result = await hook.result.current.mutateAsync({ displayName: "My Agent" } as never);
    });
    expect(result).toEqual({ name: "my-agent" });
    expect(api.generateResourceName).toHaveBeenCalledWith(
      { orgName: "o" }, { displayName: "My Agent" }, ui.getToken,
    );
    expect(ui.pushSnackBar).not.toHaveBeenCalled();
  });

  it("still reports a failure", async () => {
    api.generateResourceName.mockRejectedValue(Object.assign(new Error("HTTP error! status: 500"), { status: 500 }));
    const hook = await render(() => hooks.useGenerateResourceName({ orgName: "o" }));
    await act(async () => {
      await hook.result.current.mutateAsync({} as never).catch(() => undefined);
    });
    expect(ui.pushSnackBar).toHaveBeenCalledWith({ message: "Failed to submit resource name", type: "error" });
  });
});
