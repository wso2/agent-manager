/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  TraceFilters,
  TraceListResponse,
  TraceOverview,
} from "@agent-management-platform/types";
import type * as TracesApi from "../apis/traces";
import { getTraceList } from "../apis/traces";
import { getAgentTraceScores } from "../apis/monitors";
import { useTraceList } from "./traces";

const { getToken } = vi.hoisted(() => ({ getToken: async () => "token" }));

vi.mock("../apis/traces", async (importOriginal) => ({
  ...(await importOriginal<typeof TracesApi>()),
  getTraceList: vi.fn(),
}));
vi.mock("../apis/monitors", () => ({ getAgentTraceScores: vi.fn() }));
vi.mock("@agent-management-platform/auth", () => ({ useAuthHooks: () => ({ getToken }) }));
vi.mock("./react-query-notifications", async () => {
  const rq = await import("@tanstack/react-query");
  return { useApiQuery: rq.useQuery, useApiMutation: rq.useMutation };
});

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const mockList = vi.mocked(getTraceList);
const mockScores = vi.mocked(getAgentTraceScores);

const START = "2026-10-02T09:00:00Z";
const END = "2026-10-02T10:00:00Z";

function trace(id: string, startTime: string): TraceOverview {
  return {
    traceId: id,
    rootSpanId: `${id}-root`,
    rootSpanName: "agent",
    startTime,
    endTime: startTime,
    durationInNanos: 0,
    spanCount: 1,
  };
}

function page(traces: TraceOverview[], extra: Partial<TraceListResponse> = {}): TraceListResponse {
  return { traces, totalCount: traces.length, ...extra };
}

type HookResult = ReturnType<typeof useTraceList>;

let root: Root | undefined;

function renderTraceList(options?: { filters?: TraceFilters; includeModels?: boolean }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const result = {} as { current: HookResult };
  function Probe() {
    result.current = useTraceList(
      "org", "proj", "agent", "dev", undefined, 10, "desc", START, END, options,
    );
    return null;
  }
  root = createRoot(document.createElement("div"));
  act(() => {
    root!.render(
      <QueryClientProvider client={client}>
        <Probe />
      </QueryClientProvider>,
    );
  });
  return result;
}

async function waitFor(check: () => boolean) {
  for (let i = 0; i < 50 && !check(); i += 1) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
  }
  expect(check()).toBe(true);
}

beforeEach(() => {
  mockList.mockReset();
  mockScores.mockReset();
  mockScores.mockResolvedValue({ traces: [], totalCount: 0 });
});

afterEach(() => {
  act(() => root?.unmount());
  root = undefined;
});

describe("useTraceList cursor paging", () => {
  it.each([
    ["without filters", undefined],
    ["with filters", { status: "error", minTokens: 1000 } satisfies TraceFilters],
  ])("loadOlder sends the original window plus nextCursor %s", async (_, filters) => {
    mockList
      .mockResolvedValueOnce(page(
        [trace("t1", "2026-10-02T09:50:00Z"), trace("t2", "2026-10-02T09:40:00Z")],
        { nextCursor: "c1" },
      ))
      .mockResolvedValueOnce(page([trace("t3", "2026-10-02T09:30:00Z")], { nextCursor: "c2" }))
      .mockResolvedValueOnce(page([trace("t4", "2026-10-02T09:20:00Z")]));

    const result = renderTraceList({ filters });
    await waitFor(() => result.current.traceList?.traces.length === 2);
    expect(mockList.mock.calls[0][0].cursor).toBeUndefined();

    await act(() => result.current.loadOlder());
    await act(() => result.current.loadOlder());

    const expectedFilters = filters ?? {};
    expect(mockList.mock.calls[1][0]).toMatchObject({
      startTime: START, endTime: END, sortOrder: "desc", filters: expectedFilters, cursor: "c1",
    });
    expect(mockList.mock.calls[2][0]).toMatchObject({
      startTime: START, endTime: END, sortOrder: "desc", filters: expectedFilters, cursor: "c2",
    });
    expect(result.current.traceList?.traces.map((t) => t.traceId)).toEqual(["t1", "t2", "t3", "t4"]);
    expect(result.current.hasOlder).toBe(false);
  });

  it("reports hasOlder false and skips loadOlder when nextCursor is absent", async () => {
    mockList.mockResolvedValueOnce(page([trace("t1", "2026-10-02T09:50:00Z")]));

    const result = renderTraceList();
    await waitFor(() => result.current.traceList?.traces.length === 1);
    expect(result.current.hasOlder).toBe(false);

    await act(() => result.current.loadOlder());
    expect(mockList).toHaveBeenCalledTimes(1);
  });

  it("treats truncated without nextCursor as no older pages", async () => {
    mockList.mockResolvedValueOnce(page(
      [trace("t1", "2026-10-02T09:50:00Z")],
      { truncated: true, lookedBackTo: "2026-10-02T09:10:00.123456789Z" },
    ));

    const result = renderTraceList({ filters: { status: "error" } });
    await waitFor(() => result.current.traceList?.traces.length === 1);
    expect(result.current.hasOlder).toBe(false);
    expect(result.current.truncated).toBe(true);
    expect(result.current.lookedBackTo).toBe("2026-10-02T09:10:00.123456789Z");
  });

  it("can page on from an empty filtered first page", async () => {
    mockList
      .mockResolvedValueOnce(page([], { truncated: true, nextCursor: "c1" }))
      .mockResolvedValueOnce(page([trace("t9", "2026-10-02T09:05:00Z")]));

    const result = renderTraceList({ filters: { model: "gpt-4o" } });
    await waitFor(() => result.current.hasOlder);

    await act(() => result.current.loadOlder());
    expect(mockList.mock.calls[1][0].cursor).toBe("c1");
    expect(result.current.traceList?.traces.map((t) => t.traceId)).toEqual(["t9"]);
  });

  it("shows a trace repeated across pages once", async () => {
    mockList
      .mockResolvedValueOnce(page(
        [trace("t1", "2026-10-02T09:50:00Z"), trace("t2", "2026-10-02T09:40:00Z")],
        { nextCursor: "c1" },
      ))
      .mockResolvedValueOnce(page(
        [trace("t2", "2026-10-02T09:40:00Z"), trace("t3", "2026-10-02T09:30:00Z")],
      ));

    const result = renderTraceList();
    await waitFor(() => result.current.traceList?.traces.length === 2);
    await act(() => result.current.loadOlder());

    expect(result.current.traceList?.traces.map((t) => t.traceId)).toEqual(["t1", "t2", "t3"]);
  });

  it("fetches scores for the span of the returned page", async () => {
    mockList
      .mockResolvedValueOnce(page([trace("t1", "2026-10-02T09:50:00Z")], { nextCursor: "c1" }))
      .mockResolvedValueOnce(page(
        [trace("t2", "2026-10-02T09:40:00Z"), trace("t3", "2026-10-02T09:30:00Z")],
      ));
    mockScores.mockResolvedValue({
      traces: [{ traceId: "t3", score: 0.5, totalCount: 1, skippedCount: 0 }],
      totalCount: 1,
    });

    const result = renderTraceList();
    await waitFor(() => result.current.traceList?.traces.length === 1);
    await act(() => result.current.loadOlder());

    expect(mockScores.mock.calls[1][0]).toMatchObject({
      startTime: "2026-10-02T09:29:59.000Z",
      endTime: "2026-10-02T09:40:01.000Z",
      limit: 100,
    });
    const t3 = result.current.traceList?.traces.find((t) => t.traceId === "t3");
    expect(t3?.score).toEqual({ score: 0.5, totalCount: 1, skippedCount: 0 });
  });

  it("sends includeModels and filters on the first page", async () => {
    mockList.mockResolvedValueOnce(page([]));

    renderTraceList({ filters: { conversationId: "conv-1" }, includeModels: true });
    await waitFor(() => mockList.mock.calls.length === 1);

    expect(mockList.mock.calls[0][0]).toMatchObject({
      filters: { conversationId: "conv-1" },
      includeModels: true,
    });
  });
});
