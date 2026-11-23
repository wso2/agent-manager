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

import { afterEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor, type RenderedHook } from "../../test-utils/render-hook";

const ui = vi.hoisted(() => ({ pushSnackBar: vi.fn(), getToken: async () => "tok" }));
const listCatalogLLMProviders = vi.hoisted(() => vi.fn());

vi.mock("@agent-management-platform/views", () => ({
  useSnackBar: () => ({ pushSnackBar: ui.pushSnackBar }),
}));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: ui.getToken, isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("./telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/catalog", () => ({ listCatalogLLMProviders }));

import { useListCatalogLLMProviders } from "./catalog";

let rendered: RenderedHook<unknown> | undefined;
afterEach(() => {
  rendered?.unmount();
  rendered = undefined;
  vi.resetAllMocks();
});

describe("useListCatalogLLMProviders", () => {
  it("lists the org's catalog providers under its key", async () => {
    const data = { entries: [{ id: "p1" }], total: 1 };
    listCatalogLLMProviders.mockResolvedValue(data);
    const query = { environmentId: "e1" };
    const hook = await renderHook(() => useListCatalogLLMProviders({ orgName: "o" }, query));
    rendered = hook as RenderedHook<unknown>;
    await waitFor(() => hook.result.current.data !== undefined);
    expect(listCatalogLLMProviders).toHaveBeenCalledWith({ orgName: "o" }, query, ui.getToken);
    expect(hook.client.getQueryData(["catalog", "llm-providers", { orgName: "o" }, query])).toEqual(data);
  });

  it("names the catalog in a failed fetch", async () => {
    listCatalogLLMProviders.mockRejectedValue(Object.assign(new Error("x"), { status: 500 }));
    const hook = await renderHook(() => useListCatalogLLMProviders({ orgName: "o" }));
    rendered = hook as RenderedHook<unknown>;
    await waitFor(() => hook.result.current.isError);
    expect(ui.pushSnackBar).toHaveBeenCalledWith({ message: "Failed to fetch Catalog", type: "error" });
  });

  it.each([
    ["without an org", { orgName: "" }, undefined],
    ["when switched off", { orgName: "o" }, { enabled: false }],
  ])("stays idle %s", async (_name, params, options) => {
    const hook = await renderHook(() => useListCatalogLLMProviders(params, undefined, options));
    rendered = hook as RenderedHook<unknown>;
    expect(hook.result.current.fetchStatus).toBe("idle");
    expect(listCatalogLLMProviders).not.toHaveBeenCalled();
  });
});
