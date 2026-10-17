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
import type { AgentCardResponse } from "@agent-management-platform/types";

vi.mock("@agent-management-platform/views", () => ({ useSnackBar: () => ({ pushSnackBar: vi.fn() }) }));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: async () => "t", isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("./telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/agent-card", () => ({
  getAgentCard: vi.fn(),
  setAgentCardSource: vi.fn(async () => undefined),
  refreshAgentCard: vi.fn(async () => undefined),
  deleteAgentCardSource: vi.fn(async () => undefined),
}));
vi.mock("../apis/a2a-proxy", () => ({ setA2AProxy: vi.fn(async () => undefined) }));

import { getAgentCard } from "../apis/agent-card";
import {
  useDeleteAgentCardSource, useGetAgentCard, useRefreshAgentCard, useSetAgentCardSource,
} from "./agent-card";
import { useSetA2AProxy } from "./a2a-proxy";
import { a2aProxyQueryKey, agentCardQueryKey } from "./a2a-query-keys";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const params = { orgName: "o", projName: "p", agentName: "a", envId: "dev" };
const card = (sourceUrl: string): AgentCardResponse =>
  ({ source: "external", status: "fetched", sourceUrl, lastError: "" });

let unmount: (() => void) | undefined;
afterEach(() => unmount?.());

/** Mounts the card query beside a mutation hook, as the panel does. */
async function mountWith<T>(useMutationHook: () => T): Promise<{ current: T }> {
  return (await mountWithClient(useMutationHook)).result;
}

async function mountWithClient<T>(useMutationHook: () => T) {
  const result = {} as { current: T };
  const Probe = () => {
    useGetAgentCard(params);
    result.current = useMutationHook();
    return null;
  };
  const qc = new QueryClient();
  const root = createRoot(document.createElement("div"));
  const tree = createElement(QueryClientProvider, { client: qc }, createElement(Probe));
  await act(async () => root.render(tree));
  unmount = () => act(() => root.unmount());
  return { result, qc };
}

/** The next card read stays in flight until the returned function is called. */
function holdNextCardRead(next: AgentCardResponse) {
  let release!: () => void;
  vi.mocked(getAgentCard).mockImplementationOnce(
    () => new Promise((resolve) => { release = () => resolve(next); }),
  );
  return () => release();
}

async function settlesBeforeTheRefetch(mutate: () => Promise<unknown>, next: AgentCardResponse) {
  const release = holdNextCardRead(next);
  let settled = false;
  await act(async () => {
    void mutate().then(() => { settled = true; });
    await new Promise((r) => setTimeout(r, 20));
  });
  const early = settled;
  await act(async () => { release(); await new Promise((r) => setTimeout(r, 20)); });
  return { early, late: settled };
}

describe("agent card mutations", () => {
  it("resolve a source change only after the card is re-read", async () => {
    vi.mocked(getAgentCard).mockResolvedValueOnce(card("https://old.example/c"));
    const hook = await mountWith(useSetAgentCardSource);

    const { early, late } = await settlesBeforeTheRefetch(
      () => hook.current.mutateAsync({ params, body: { url: "https://new.example/c" } }),
      card("https://new.example/c"),
    );

    expect(early).toBe(false);
    expect(late).toBe(true);
  });

  it("resolve a refetch request only after the card is re-read", async () => {
    vi.mocked(getAgentCard).mockResolvedValueOnce(card("https://a.example/c"));
    const hook = await mountWith(useRefreshAgentCard);

    const { early, late } = await settlesBeforeTheRefetch(
      () => hook.current.mutateAsync(params),
      card("https://a.example/c"),
    );

    expect(early).toBe(false);
    expect(late).toBe(true);
  });

  // A mode switch changes the card too; the form must not close on the old card.
  it("resolve a gateway routing change only after the card is re-read", async () => {
    vi.mocked(getAgentCard).mockResolvedValueOnce(card("https://a.example/c"));
    const hook = await mountWith(useSetA2AProxy);

    const { early, late } = await settlesBeforeTheRefetch(
      () => hook.current.mutateAsync({ params, body: {} }),
      card("https://a.example/c"),
    );

    expect(early).toBe(false);
    expect(late).toBe(true);
  });

  it("leave the refetch to the routing save that follows a source change", async () => {
    vi.mocked(getAgentCard).mockResolvedValue(card("https://a.example/c"));
    const { result, qc } = await mountWithClient(useSetAgentCardSource);
    qc.setQueryData(a2aProxyQueryKey(params), null);

    await act(async () => {
      await result.current.mutateAsync({ params, body: { url: "https://n.example/c" }, routingFollows: true });
    });

    expect(qc.getQueryState(a2aProxyQueryKey(params))?.isInvalidated).toBe(false);
  });

  it("read a missing card source as null", async () => {
    vi.mocked(getAgentCard).mockRejectedValue(Object.assign(new Error("nf"), { status: 404 }));
    const { qc } = await mountWithClient(() => null);
    await act(async () => { await new Promise((r) => setTimeout(r, 20)); });

    expect(qc.getQueryData(agentCardQueryKey(params))).toBeNull();
    expect(qc.getQueryState(agentCardQueryKey(params))?.status).toBe("success");
  });

  it.each([
    ["a source change", useSetAgentCardSource,
      (h: { mutateAsync: (v: never) => Promise<unknown> }) =>
        h.mutateAsync({ params, body: { url: "https://n.example/c" } } as never)],
    ["a refetch request", useRefreshAgentCard,
      (h: { mutateAsync: (v: never) => Promise<unknown> }) => h.mutateAsync(params as never)],
  ])("invalidate the gateway routing before %s resolves", async (_n, hook, run) => {
    vi.mocked(getAgentCard).mockResolvedValue(card("https://a.example/c"));
    const { result, qc } = await mountWithClient(hook as () => never);
    qc.setQueryData(a2aProxyQueryKey(params), { status: "published" });

    await act(async () => { await run(result.current as never); });

    expect(qc.getQueryState(a2aProxyQueryKey(params))?.isInvalidated).toBe(true);
  });

  it("read the card and gateway routing as gone once the source is removed", async () => {
    vi.mocked(getAgentCard).mockResolvedValue(card("https://a.example/c"));
    const { result, qc } = await mountWithClient(useDeleteAgentCardSource);
    qc.setQueryData(a2aProxyQueryKey(params), { status: "published" });
    expect(qc.getQueryData(agentCardQueryKey(params))).toBeDefined();
    // The mounted card query re-reads after removal; the server now answers 404.
    vi.mocked(getAgentCard).mockRejectedValue(Object.assign(new Error("not found"), { status: 404 }));

    await act(async () => { await result.current.mutateAsync(params); });

    expect(qc.getQueryData(agentCardQueryKey(params))).toBeNull();
    expect(qc.getQueryData(a2aProxyQueryKey(params))).toBeNull();
  });
});
