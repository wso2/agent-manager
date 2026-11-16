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

import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("@agent-management-platform/views", () => ({
  useSnackBar: () => ({ pushSnackBar: vi.fn() }),
}));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: async () => "t", isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("./telemetry", () => ({
  ConsoleAction: {},
  markSessionExpired: vi.fn(),
  useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/shadowAgents", () => ({ deleteShadowConnection: vi.fn(async () => undefined) }));

import { deleteShadowConnection } from "../apis/shadowAgents";
import { useDeleteShadowConnection } from "./shadowAgents";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

describe("useDeleteShadowConnection", () => {
  it("drops the deleted connection from the cached list before the refetch", async () => {
    const key = ["shadow-agents", "connections", "o"];
    const qc = new QueryClient();
    qc.setQueryData(key, { connections: [{ id: "a" }, { id: "b" }] });
    // Hold the refetch so the assertion sees the cache before it completes.
    const invalidate = vi.spyOn(qc, "invalidateQueries").mockImplementation(async () => undefined);
    const result = {} as { current: ReturnType<typeof useDeleteShadowConnection> };
    const Probe = () => {
      result.current = useDeleteShadowConnection("o");
      return null;
    };
    const root = createRoot(document.createElement("div"));
    await act(async () =>
      root.render(createElement(QueryClientProvider, { client: qc }, createElement(Probe))),
    );

    await act(async () => {
      await result.current.mutateAsync("a");
    });

    expect(deleteShadowConnection).toHaveBeenCalledWith("o", "a", expect.any(Function));
    expect(qc.getQueryData(key)).toEqual({ connections: [{ id: "b" }] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["shadow-agents"] });
    act(() => root.unmount());
  });
});
