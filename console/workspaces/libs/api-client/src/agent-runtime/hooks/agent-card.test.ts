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

vi.mock("@agent-management-platform/views", () => ({ useSnackBar: () => ({ pushSnackBar: vi.fn() }) }));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: async () => "t", isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("../../acp-core/hooks/telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/agent-card", () => ({
  getAgentCard: vi.fn(),
  setAgentCardSource: vi.fn(),
  refreshAgentCard: vi.fn(),
  deleteAgentCardSource: vi.fn(),
}));

import { getAgentCard } from "../apis/agent-card";
import { useGetAgentCard } from "./agent-card";
import { agentCardQueryKey } from "./a2a-query-keys";
import { cleanupHooks, flush, renderHook } from "../../test-utils/query-hooks";
import { POLL_INTERVAL } from "../../shared/utils";

const params = { orgName: "o", projName: "p", agentName: "a", envId: "dev" };

afterEach(() => cleanupHooks());

async function pollIntervalFor(status: string, poll?: boolean) {
  vi.mocked(getAgentCard).mockReset().mockResolvedValue({ status } as never);
  const { qc } = await renderHook(() => useGetAgentCard(params, { poll }));
  await flush();
  const query = qc.getQueryCache().find({ queryKey: agentCardQueryKey(params) })!;
  const interval = query.observers[0].options.refetchInterval;
  return typeof interval === "function" ? interval(query) : interval;
}

describe("useGetAgentCard", () => {
  it("caches the card under the agent environment's card key", async () => {
    vi.mocked(getAgentCard).mockReset().mockResolvedValue({ status: "fetched", sourceUrl: "u" } as never);
    const { qc } = await renderHook(() => useGetAgentCard(params));
    await flush();
    expect(getAgentCard).toHaveBeenCalledWith(params, expect.any(Function));
    expect(qc.getQueryData(agentCardQueryKey(params))).toEqual({ status: "fetched", sourceUrl: "u" });
  });

  it("re-polls while the card fetch is pending", async () => {
    expect(await pollIntervalFor("pending")).toBe(POLL_INTERVAL);
  });

  it("stops polling once the card is fetched", async () => {
    expect(await pollIntervalFor("fetched")).toBe(false);
  });

  it("never polls when the caller turns polling off", async () => {
    expect(await pollIntervalFor("pending", false)).toBe(false);
  });

  it.each([
    ["the caller disables it", () => useGetAgentCard(params, { enabled: false })],
    ["the environment is unknown", () => useGetAgentCard({ ...params, envId: "" })],
  ])("does not read the card when %s", async (_n, use) => {
    vi.mocked(getAgentCard).mockClear();
    await renderHook(use);
    await flush();
    expect(getAgentCard).not.toHaveBeenCalled();
  });
});
