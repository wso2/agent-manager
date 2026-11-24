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
import { renderHook, testQueryClient, waitFor, type RenderedHook } from "../../test-utils/render-hook";

const getRuntimeConfig = vi.hoisted(() => vi.fn());
vi.mock("@agent-management-platform/views", () => ({ useSnackBar: () => ({ pushSnackBar: vi.fn() }) }));
vi.mock("@agent-management-platform/auth", () => ({ useAuthHooks: () => ({ isAuthenticated: false }) }));
vi.mock("./telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/runtime-config", () => ({ getRuntimeConfig }));

import { useRuntimeConfig } from "./runtime-config";

let rendered: RenderedHook<unknown> | undefined;
afterEach(() => {
  rendered?.unmount();
  rendered = undefined;
  vi.resetAllMocks();
});

describe("useRuntimeConfig", () => {
  it("reads the config once and keeps it for the session", async () => {
    getRuntimeConfig.mockResolvedValue({ eventsEnabled: true });
    const hook = await renderHook(useRuntimeConfig);
    rendered = hook as RenderedHook<unknown>;
    await waitFor(() => hook.result.current.data !== undefined);
    expect(hook.result.current.data).toEqual({ eventsEnabled: true });
    expect(hook.client.getQueryState(["runtime-config"])?.isInvalidated).toBe(false);
    await hook.rerender();
    expect(getRuntimeConfig).toHaveBeenCalledTimes(1);
    expect(hook.result.current.isStale).toBe(false);
  });

  it("retries a failed read once before giving up", async () => {
    getRuntimeConfig.mockRejectedValue(new Error("down"));
    const client = testQueryClient();
    client.setDefaultOptions({ queries: { retryDelay: 0 } });
    const hook = await renderHook(useRuntimeConfig, { client });
    rendered = hook as RenderedHook<unknown>;
    await waitFor(() => hook.result.current.isError);
    expect(getRuntimeConfig).toHaveBeenCalledTimes(2);
  });
});
