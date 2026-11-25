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

const ui = vi.hoisted(() => ({
  getToken: async () => "tok",
  userInfo: undefined as Record<string, string> | undefined,
}));
const getUIManifest = vi.hoisted(() => vi.fn());
vi.mock("@agent-management-platform/views", () => ({ useSnackBar: () => ({ pushSnackBar: vi.fn() }) }));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: ui.getToken, userInfo: ui.userInfo, isAuthenticated: true }),
}));
vi.mock("./telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/ui-manifest", () => ({ getUIManifest }));

import { useUIManifest } from "./ui-manifest";

let rendered: RenderedHook<unknown> | undefined;
afterEach(() => {
  rendered?.unmount();
  rendered = undefined;
  ui.userInfo = undefined;
  vi.resetAllMocks();
});

describe("useUIManifest", () => {
  it.each([
    [{ sub: "u1", username: "ada", sessionState: "s1" }, ["ui-manifest", "u1", "s1"]],
    [{ username: "ada" }, ["ui-manifest", "ada", ""]],
    [undefined, ["ui-manifest", "", ""]],
  ])("keys the manifest by user and session (%j)", async (userInfo, key) => {
    ui.userInfo = userInfo;
    getUIManifest.mockResolvedValue({ modules: [] });
    const hook = await renderHook(useUIManifest);
    rendered = hook as RenderedHook<unknown>;
    await waitFor(() => hook.result.current.data !== undefined);
    expect(getUIManifest).toHaveBeenCalledWith(ui.getToken);
    expect(hook.client.getQueryData(key)).toEqual({ modules: [] });
  });
});
