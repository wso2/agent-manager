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
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const pushSnackBar = vi.fn();
vi.mock("@agent-management-platform/views", () => ({ useSnackBar: () => ({ pushSnackBar }) }));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: async () => "t", isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("./telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../utils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../utils")>()),
  httpGET: vi.fn(async () => ({ ok: true, json: async () => ({}) })),
}));

import { httpGET, SERVICE_BASE } from "../utils";
import {
  a2aCatalogEntryQueryKey,
  a2aConnectQueryKey,
  useGetA2ACatalogEntry,
  useGetA2AConnect,
  useListA2ACatalog,
} from "./a2a-catalog";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const listPath = `${SERVICE_BASE}/orgs/acme/a2a-agents`;
const entry = { orgName: "acme", projName: "p1", agentName: "orders" };
const failure = (status: number) => Object.assign(new Error("nf"), { status });

let unmount: (() => void) | undefined;
beforeEach(() => {
  vi.mocked(httpGET).mockClear();
  pushSnackBar.mockClear();
});
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

const settle = () => act(async () => { await new Promise((r) => setTimeout(r, 20)); });

describe("useListA2ACatalog", () => {
  it("sends only the defined, non-empty query values as strings", async () => {
    await mount(() =>
      useListA2ACatalog({ orgName: "acme" }, { q: "refund", kind: undefined, limit: 24, offset: 0 }));
    await settle();
    expect(httpGET).toHaveBeenCalledWith(listPath,
      { searchParams: { q: "refund", limit: "24", offset: "0" }, token: "t" });
  });

  it("forwards the four filters", async () => {
    await mount(() => useListA2ACatalog({ orgName: "acme" },
      { kind: "proxied", environment: "dev", project: "p", status: "failed" }));
    await settle();
    expect(httpGET).toHaveBeenCalledWith(listPath, {
      searchParams: { kind: "proxied", environment: "dev", project: "p", status: "failed" },
      token: "t",
    });
  });

  it("passes no searchParams when the query is empty", async () => {
    await mount(() => useListA2ACatalog({ orgName: "acme" }, {}));
    await settle();
    expect(httpGET).toHaveBeenCalledWith(listPath, { searchParams: undefined, token: "t" });
  });

  it("does not request without an organisation", async () => {
    await mount(() => useListA2ACatalog({ orgName: "" }, {}));
    await settle();
    expect(httpGET).not.toHaveBeenCalled();
  });
});

describe("useGetA2ACatalogEntry", () => {
  it("resolves a missing entry to null without an error", async () => {
    vi.mocked(httpGET).mockRejectedValueOnce(failure(404));
    const { qc } = await mount(() => useGetA2ACatalogEntry(entry));
    await settle();
    expect(httpGET).toHaveBeenCalledWith(
      `${SERVICE_BASE}/orgs/acme/projects/p1/agents/orders/a2a-catalog-entry`, { token: "t" });
    expect(qc.getQueryData(a2aCatalogEntryQueryKey(entry))).toBeNull();
    expect(qc.getQueryState(a2aCatalogEntryQueryKey(entry))?.status).toBe("success");
  });

  it("reports other entry failures silently", async () => {
    vi.mocked(httpGET).mockRejectedValueOnce(failure(500));
    const { qc } = await mount(() => useGetA2ACatalogEntry(entry));
    await settle();
    expect(qc.getQueryState(a2aCatalogEntryQueryKey(entry))?.status).toBe("error");
    expect(pushSnackBar).not.toHaveBeenCalled();
  });

  it("does not request an entry with a missing segment", async () => {
    await mount(() => useGetA2ACatalogEntry({ ...entry, projName: "" }));
    await settle();
    expect(httpGET).not.toHaveBeenCalled();
  });
});

describe("useGetA2AConnect", () => {
  const connect = { orgName: "acme", projName: "p 1", agentName: "a/b", envId: "dev" };

  it("encodes every segment", async () => {
    await mount(() => useGetA2AConnect(connect));
    await settle();
    expect(httpGET).toHaveBeenCalledWith(
      `${SERVICE_BASE}/orgs/acme/projects/p%201/agents/a%2Fb/environments/dev/a2a-connect`,
      { token: "t" });
  });

  it("does not request without an environment", async () => {
    await mount(() => useGetA2AConnect({ ...connect, envId: "" }));
    await settle();
    expect(httpGET).not.toHaveBeenCalled();
  });

  it("caches under the a2a-connect key", async () => {
    vi.mocked(httpGET).mockResolvedValueOnce(
      { ok: true, json: async () => ({ kind: "platform" }) } as Response);
    const { qc } = await mount(() => useGetA2AConnect(connect));
    await settle();
    expect(qc.getQueryData(a2aConnectQueryKey(connect))).toEqual({ kind: "platform" });
  });
});
