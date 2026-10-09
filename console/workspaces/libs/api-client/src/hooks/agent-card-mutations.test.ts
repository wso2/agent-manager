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

import { getAgentCard } from "../apis/agent-card";
import { useGetAgentCard, useRefreshAgentCard, useSetAgentCardSource } from "./agent-card";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const params = { orgName: "o", projName: "p", agentName: "a", envId: "dev" };
const card = (sourceUrl: string): AgentCardResponse =>
  ({ source: "external", status: "fetched", sourceUrl, lastError: "" });

let unmount: (() => void) | undefined;
afterEach(() => unmount?.());

/** Mounts the card query beside a mutation hook, as the panel does. */
async function mountWith<T>(useMutationHook: () => T): Promise<{ current: T }> {
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
  return result;
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
});
