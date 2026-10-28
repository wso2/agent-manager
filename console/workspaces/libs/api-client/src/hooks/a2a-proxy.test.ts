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
import { afterEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("@agent-management-platform/views", () => ({ useSnackBar: () => ({ pushSnackBar: vi.fn() }) }));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: async () => "t", isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("./telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/a2a-proxy", () => ({
  getA2AProxy: vi.fn(),
  setA2AProxy: vi.fn(async () => undefined),
  deleteA2AProxy: vi.fn(async () => undefined),
  refreshA2AProxy: vi.fn(async () => undefined),
}));

import { getA2AProxy, setA2AProxy } from "../apis/a2a-proxy";
import { useGetA2AProxy, useIsSavingA2AProxy, useSetA2AProxy } from "./a2a-proxy";
import { a2aProxyQueryKey, agentCardQueryKey } from "./a2a-query-keys";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const params = { orgName: "o", projName: "p", agentName: "a", envId: "dev" };
const notFound = () => Object.assign(new Error("nf"), { status: 404 });

let unmount: (() => void) | undefined;
afterEach(() => unmount?.());

async function mount<T>(useHook: () => T) {
  const result = {} as { current: T };
  const Probe = () => {
    result.current = useHook();
    return null;
  };
  const qc = new QueryClient();
  const root = createRoot(document.createElement("div"));
  await act(async () =>
    root.render(createElement(QueryClientProvider, { client: qc }, createElement(Probe))),
  );
  unmount = () => act(() => root.unmount());
  return { result, qc };
}

describe("useGetA2AProxy", () => {
  it("reads a direct environment as null", async () => {
    vi.mocked(getA2AProxy).mockRejectedValueOnce(notFound());
    const { qc } = await mount(() => useGetA2AProxy(params));
    await act(async () => { await new Promise((r) => setTimeout(r, 20)); });
    expect(qc.getQueryData(a2aProxyQueryKey(params))).toBeNull();
    expect(qc.getQueryState(a2aProxyQueryKey(params))?.status).toBe("success");
  });
});

describe("useSetA2AProxy", () => {
  it("refetches the card and routing even when the save fails", async () => {
    vi.mocked(setA2AProxy).mockRejectedValueOnce(new Error("boom"));
    const { result, qc } = await mount(useSetA2AProxy);
    qc.setQueryData(a2aProxyQueryKey(params), null);
    qc.setQueryData(agentCardQueryKey(params), { status: "fetched" });

    await act(async () => {
      await result.current.mutateAsync({ params, body: {} }).catch(() => undefined);
    });

    expect(qc.getQueryState(a2aProxyQueryKey(params))?.isInvalidated).toBe(true);
    expect(qc.getQueryState(agentCardQueryKey(params))?.isInvalidated).toBe(true);
  });
});

describe("useIsSavingA2AProxy", () => {
  it("is true only while a save for the same environment is pending", async () => {
    let finish!: () => void;
    vi.mocked(setA2AProxy).mockImplementationOnce(
      () => new Promise<undefined>((resolve) => { finish = () => resolve(undefined); }),
    );
    const { result: seen } = await mount(() => ({
      save: useSetA2AProxy(),
      same: useIsSavingA2AProxy(params),
      other: useIsSavingA2AProxy({ ...params, envId: "prod" }),
    }));
    expect(seen.current.same).toBe(false);

    await act(async () => {
      void seen.current.save.mutateAsync({ params, body: {} });
      await new Promise((r) => setTimeout(r, 20));
    });
    expect(seen.current.same).toBe(true);
    expect(seen.current.other).toBe(false);

    await act(async () => { finish(); await new Promise((r) => setTimeout(r, 20)); });
    expect(seen.current.same).toBe(false);
  });
});
