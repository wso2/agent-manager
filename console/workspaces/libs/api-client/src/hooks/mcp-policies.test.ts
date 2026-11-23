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
import { beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("@agent-management-platform/views", () => ({
  useSnackBar: () => ({ pushSnackBar: vi.fn() }),
}));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: async () => "t", isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("./telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis", () => ({
  listAvailableMCPPolicies: vi.fn(),
}));

import { listAvailableMCPPolicies } from "../apis";
import { useMCPPoliciesCatalog } from "./mcp-policies";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const mockList = vi.mocked(listAvailableMCPPolicies);

async function renderHook(
  args: [string?, string?, string?],
  qc = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
) {
  const Probe = () => {
    useMCPPoliciesCatalog(...args);
    return null;
  };
  const root = createRoot(document.createElement("div"));
  await act(async () =>
    root.render(createElement(QueryClientProvider, { client: qc }, createElement(Probe))),
  );
  await act(async () => { await new Promise((r) => setTimeout(r, 0)); });
  return qc;
}

describe("useMCPPoliciesCatalog", () => {
  beforeEach(() => {
    mockList.mockReset();
    mockList.mockResolvedValue({
      count: 0, list: [], hubAvailable: true, availabilityScope: "deployed",
    } as never);
  });

  it("sends the environment uuid with the proxy id and keys the cache by it", async () => {
    const qc = await renderHook(["org", "orders", "env-1"]);
    expect(mockList.mock.calls[0][1]).toEqual({
      proxyId: "orders",
      environmentUuid: "env-1",
    });
    expect(
      qc.getQueryCache().getAll().map((q) => q.queryKey),
    ).toEqual([["MCP policies catalog", "org", "orders", "env-1"]]);
  });

  it("treats an empty environment uuid as unset", async () => {
    const qc = await renderHook(["org", "orders", ""]);
    expect(mockList.mock.calls[0][1]).toEqual({
      proxyId: "orders",
      environmentUuid: undefined,
    });
    expect(
      qc.getQueryCache().getAll().map((q) => q.queryKey),
    ).toEqual([["MCP policies catalog", "org", "orders", undefined]]);
  });
});
