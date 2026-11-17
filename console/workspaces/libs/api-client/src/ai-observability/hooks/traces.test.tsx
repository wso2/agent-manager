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

import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { QueryClient, QueryClientProvider, focusManager, onlineManager } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  TraceFilters,
  TraceListResponse,
  TraceOverview,
} from "@agent-management-platform/types";
import type * as TracesApi from "../apis/traces";
import { getTraceList } from "../apis/traces";
import { getAgentTraceScores } from "../apis/monitors";
import { useTraceList, type TraceListOptions } from "./traces";

const { getToken } = vi.hoisted(() => ({ getToken: async () => "token" }));

vi.mock("../apis/traces", async (importOriginal) => ({
  ...(await importOriginal<typeof TracesApi>()),
  getTraceList: vi.fn(),
}));
vi.mock("../apis/monitors", () => ({ getAgentTraceScores: vi.fn() }));
vi.mock("@agent-management-platform/auth", () => ({ useAuthHooks: () => ({ getToken }) }));
vi.mock("../../shared/react-query-notifications", async () => {
  const rq = await import("@tanstack/react-query");
  return { useApiQuery: rq.useQuery, useApiMutation: rq.useMutation };
});

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const mockList = vi.mocked(getTraceList);
const mockScores = vi.mocked(getAgentTraceScores);

const START = "2026-10-02T09:00:00Z";
const END = "2026-10-02T10:00:00Z";

/** A minimal trace overview. */
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

/** A trace-list response for the given traces. */
function page(traces: TraceOverview[], extra: Partial<TraceListResponse> = {}): TraceListResponse {
  return { traces, totalCount: traces.length, ...extra };
}

type HookResult = ReturnType<typeof useTraceList>;

let root: Root | undefined;

/** Renders useTraceList and returns a ref to its latest result. */
function renderTraceList(options?: TraceListOptions, sortOrder: "asc" | "desc" = "desc") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const result = {} as {
    current: HookResult;
    setOptions: (next: TraceListOptions | undefined) => void;
  };
  function Probe() {
    const [opts, setOpts] = useState(options);
    result.setOptions = setOpts;
    result.current = useTraceList(
      "org", "proj", "agent", "dev", undefined, 10, sortOrder, START, END, opts,
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

/** Flushes pending work until check passes, then asserts it. */
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
  focusManager.setFocused(undefined);
});

describe("useTraceList cursor paging", () => {
  it.each([
    ["without filters", undefined],
    ["with filters", { status: "error", minTokens: 1000 } satisfies TraceFilters],
  ])("loadMore sends the original window plus nextCursor %s", async (_, filters) => {
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

    await act(() => result.current.loadMore());
    await act(() => result.current.loadMore());

    const expectedFilters = filters ?? {};
    expect(mockList.mock.calls[1][0]).toMatchObject({
      startTime: START, endTime: END, sortOrder: "desc", filters: expectedFilters, cursor: "c1",
    });
    expect(mockList.mock.calls[2][0]).toMatchObject({
      startTime: START, endTime: END, sortOrder: "desc", filters: expectedFilters, cursor: "c2",
    });
    expect(result.current.traceList?.traces.map((t) => t.traceId)).toEqual(["t1", "t2", "t3", "t4"]);
    expect(result.current.hasMore).toBe(false);
  });

  it("loadMore follows the cursor forward in asc and appends later traces", async () => {
    mockList
      .mockResolvedValueOnce(page(
        [trace("t1", "2026-10-02T09:10:00Z"), trace("t2", "2026-10-02T09:20:00Z")],
        { nextCursor: "c1" },
      ))
      .mockResolvedValueOnce(page([trace("t3", "2026-10-02T09:30:00Z")], { nextCursor: "c2" }))
      .mockResolvedValueOnce(page([trace("t4", "2026-10-02T09:40:00Z")]));

    const result = renderTraceList(undefined, "asc");
    await waitFor(() => result.current.traceList?.traces.length === 2);
    expect(mockList.mock.calls[0][0]).toMatchObject({ startTime: START, endTime: END, sortOrder: "asc" });
    expect(mockList.mock.calls[0][0].cursor).toBeUndefined();
    expect(result.current.hasMore).toBe(true);

    await act(() => result.current.loadMore());
    await act(() => result.current.loadMore());

    expect(mockList.mock.calls[1][0]).toMatchObject({
      startTime: START, endTime: END, sortOrder: "asc", cursor: "c1",
    });
    expect(mockList.mock.calls[2][0]).toMatchObject({
      startTime: START, endTime: END, sortOrder: "asc", cursor: "c2",
    });
    expect(result.current.traceList?.traces.map((t) => t.traceId)).toEqual(["t1", "t2", "t3", "t4"]);
    expect(result.current.hasMore).toBe(false);
    expect(result.current).not.toHaveProperty("loadNewer");
    expect(result.current).not.toHaveProperty("isLoadingNewer");
  });

  const blips: [string, () => void][] = [
    ["the window loses and regains focus", () => {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
    }],
    ["the network drops and reconnects", () => {
      onlineManager.setOnline(false);
      onlineManager.setOnline(true);
    }],
  ];

  it.each(blips)("keeps loaded pages when %s", async (_, blip) => {
    mockList
      .mockResolvedValueOnce(page([trace("t1", "2026-10-02T09:50:00Z")], { nextCursor: "c1" }))
      .mockResolvedValueOnce(page([trace("t2", "2026-10-02T09:40:00Z")]));

    const result = renderTraceList({ paged: true });
    await waitFor(() => result.current.traceList?.traces.length === 1);
    await act(() => result.current.loadMore());

    await act(async () => {
      blip();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(mockList).toHaveBeenCalledTimes(2);
    expect(result.current.traceList?.traces.map((t) => t.traceId)).toEqual(["t1", "t2"]);
  });

  it.each(blips)("refetches when %s for a caller that doesn't page", async (_, blip) => {
    mockList
      .mockResolvedValueOnce(page([trace("t1", "2026-10-02T09:50:00Z")]))
      .mockResolvedValueOnce(page([trace("t9", "2026-10-02T09:55:00Z")]));

    const result = renderTraceList();
    await waitFor(() => result.current.traceList?.traces.length === 1);

    act(() => blip());

    await waitFor(() => result.current.traceList?.traces[0]?.traceId === "t9");
    expect(mockList).toHaveBeenCalledTimes(2);
  });

  it("reports hasMore false and skips loadMore when nextCursor is absent", async () => {
    mockList.mockResolvedValueOnce(page([trace("t1", "2026-10-02T09:50:00Z")]));

    const result = renderTraceList();
    await waitFor(() => result.current.traceList?.traces.length === 1);
    expect(result.current.hasMore).toBe(false);

    await act(() => result.current.loadMore());
    expect(mockList).toHaveBeenCalledTimes(1);
  });

  it("treats truncated without nextCursor as no more pages", async () => {
    mockList.mockResolvedValueOnce(page(
      [trace("t1", "2026-10-02T09:50:00Z")],
      { truncated: true, lookedBackTo: "2026-10-02T09:10:00.123456789Z" },
    ));

    const result = renderTraceList({ filters: { status: "error" } });
    await waitFor(() => result.current.traceList?.traces.length === 1);
    expect(result.current.hasMore).toBe(false);
    expect(result.current.truncated).toBe(true);
    expect(result.current.lookedBackTo).toBe("2026-10-02T09:10:00.123456789Z");
  });

  it("can page on from an empty filtered first page", async () => {
    mockList
      .mockResolvedValueOnce(page([], { truncated: true, nextCursor: "c1" }))
      .mockResolvedValueOnce(page([trace("t9", "2026-10-02T09:05:00Z")]));

    const result = renderTraceList({ filters: { model: "gpt-4o" } });
    await waitFor(() => result.current.hasMore);

    await act(() => result.current.loadMore());
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
    await act(() => result.current.loadMore());

    expect(result.current.traceList?.traces.map((t) => t.traceId)).toEqual(["t1", "t2", "t3"]);
  });

  it("resolves loadMore to whether the page added rows, and undefined after a reset", async () => {
    let resolveOld: (res: TraceListResponse) => void = () => undefined;
    mockList
      .mockResolvedValueOnce(page(
        [trace("t1", "2026-10-02T09:50:00Z"), trace("t2", "2026-10-02T09:40:00Z")],
        { nextCursor: "c1" },
      ))
      .mockResolvedValueOnce(page([trace("t2", "2026-10-02T09:40:00Z")], { nextCursor: "c2" }))
      .mockResolvedValueOnce(page([trace("t3", "2026-10-02T09:30:00Z")], { nextCursor: "c3" }))
      .mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve; }))
      .mockResolvedValueOnce(page([trace("t9", "2026-10-02T09:55:00Z")]));

    const result = renderTraceList();
    await waitFor(() => result.current.traceList?.traces.length === 2);
    const added: unknown[] = [];
    await act(async () => { added.push(await result.current.loadMore()); });
    await act(async () => { added.push(await result.current.loadMore()); });

    let oldLoad: Promise<unknown> = Promise.resolve();
    act(() => { oldLoad = result.current.loadMore(); });
    act(() => result.setOptions({ filters: { status: "error" } }));
    await waitFor(() => result.current.traceList?.traces[0]?.traceId === "t9");
    await act(async () => {
      resolveOld(page([trace("t4", "2026-10-02T09:20:00Z")]));
      added.push(await oldLoad);
    });

    expect(added).toEqual([false, true, undefined]);
    expect(result.current.traceList?.traces.map((t) => t.traceId)).toEqual(["t9"]);
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
    await act(() => result.current.loadMore());

    expect(mockScores.mock.calls[1][0]).toMatchObject({
      startTime: "2026-10-02T09:29:59.000Z",
      endTime: "2026-10-02T09:40:01.000Z",
      limit: 100,
    });
    const t3 = result.current.traceList?.traces.find((t) => t.traceId === "t3");
    expect(t3?.score).toEqual({ score: 0.5, totalCount: 1, skippedCount: 0 });
  });

  it("pages through scores until every trace on a filtered page is found", async () => {
    mockList.mockResolvedValueOnce(page(
      [trace("t1", "2026-10-02T09:50:00Z"), trace("t2", "2026-10-02T09:10:00Z")],
    ));
    const others = Array.from({ length: 100 }, (_, i) => (
      { traceId: `other${i}`, score: 0.1, totalCount: 1, skippedCount: 0 }
    ));
    mockScores
      .mockResolvedValueOnce({
        traces: [{ traceId: "t1", score: 0.9, totalCount: 1, skippedCount: 0 }, ...others.slice(1)],
        totalCount: 200,
      })
      .mockResolvedValueOnce({
        traces: [...others.slice(0, 99), { traceId: "t2", score: 0.2, totalCount: 1, skippedCount: 0 }],
        totalCount: 200,
      });

    const result = renderTraceList({ filters: { status: "error" } });
    await waitFor(() => result.current.traceList?.traces.length === 2);

    expect(mockScores).toHaveBeenCalledTimes(2);
    expect(mockScores.mock.calls[0][0]).toMatchObject({ limit: 100, offset: 0 });
    expect(mockScores.mock.calls[1][0]).toMatchObject({ limit: 100, offset: 100 });
    expect(result.current.traceList?.traces.map((t) => t.score?.score)).toEqual([0.9, 0.2]);
  });

  it("sets loadError on a failed loadMore and clears it when the next loadMore starts", async () => {
    let resolveRetry: (res: TraceListResponse) => void = () => undefined;
    mockList
      .mockResolvedValueOnce(page([trace("t1", "2026-10-02T09:50:00Z")], { nextCursor: "c1" }))
      .mockRejectedValueOnce(new Error("upstream down"))
      .mockImplementationOnce(() => new Promise((resolve) => { resolveRetry = resolve; }));

    const result = renderTraceList();
    await waitFor(() => result.current.traceList?.traces.length === 1);
    await act(() => result.current.loadMore());
    expect(result.current.loadError?.message).toBe("upstream down");
    expect(result.current.hasMore).toBe(true);

    let retry: Promise<unknown> = Promise.resolve();
    act(() => { retry = result.current.loadMore(); });
    expect(result.current.loadError).toBeNull();
    expect(mockList.mock.calls[2][0].cursor).toBe("c1");

    await act(async () => {
      resolveRetry(page([trace("t2", "2026-10-02T09:40:00Z")]));
      await retry;
    });
    expect(result.current.loadError).toBeNull();
    expect(result.current.traceList?.traces.map((t) => t.traceId)).toEqual(["t1", "t2"]);
  });

  const listResets: [string, (result: ReturnType<typeof renderTraceList>) => unknown][] = [
    ["a filter change", (result) => {
      act(() => result.setOptions({ filters: { status: "error" } }));
    }],
    ["Refresh", async (result) => {
      await act(() => result.current.refetch());
    }],
  ];

  it.each(listResets)("clears loadError when %s resets the list", async (_, reset) => {
    mockList
      .mockResolvedValueOnce(page([trace("t1", "2026-10-02T09:50:00Z")], { nextCursor: "c1" }))
      .mockRejectedValueOnce(new Error("upstream down"))
      .mockResolvedValueOnce(page([trace("t9", "2026-10-02T09:55:00Z")]));

    const result = renderTraceList();
    await waitFor(() => result.current.traceList?.traces.length === 1);
    await act(() => result.current.loadMore());
    expect(result.current.loadError).not.toBeNull();

    await reset(result);
    await waitFor(() => result.current.traceList?.traces[0]?.traceId === "t9");
    expect(result.current.loadError).toBeNull();
  });

  it.each(listResets)("lets the new list page when %s lands mid-load", async (_, reset) => {
    let rejectOld: (err: Error) => void = () => undefined;
    let resolveNew: (res: TraceListResponse) => void = () => undefined;
    mockList
      .mockResolvedValueOnce(page([trace("t1", "2026-10-02T09:50:00Z")], { nextCursor: "c1" }))
      .mockImplementationOnce(() => new Promise((_res, reject) => { rejectOld = reject; }))
      .mockResolvedValueOnce(page([trace("t9", "2026-10-02T09:55:00Z")], { nextCursor: "c9" }))
      .mockImplementationOnce(() => new Promise((resolve) => { resolveNew = resolve; }));

    const result = renderTraceList();
    await waitFor(() => result.current.traceList?.traces.length === 1);
    let oldLoad: Promise<unknown> = Promise.resolve();
    act(() => { oldLoad = result.current.loadMore(); });
    expect(result.current.isLoadingMore).toBe(true);

    await reset(result);
    await waitFor(() => result.current.traceList?.traces[0]?.traceId === "t9");
    expect(result.current.isLoadingMore).toBe(false);

    // The new list pages while the old request is still in flight.
    let newLoad: Promise<unknown> = Promise.resolve();
    act(() => { newLoad = result.current.loadMore(); });
    expect(mockList.mock.calls[3][0].cursor).toBe("c9");

    await act(async () => {
      rejectOld(new Error("upstream down"));
      await oldLoad;
    });
    expect(result.current.loadError).toBeNull();
    expect(result.current.isLoadingMore).toBe(true);

    await act(async () => {
      resolveNew(page([trace("t8", "2026-10-02T09:45:00Z")]));
      await newLoad;
    });
    expect(result.current.isLoadingMore).toBe(false);
    expect(result.current.loadError).toBeNull();
    expect(result.current.traceList?.traces.map((t) => t.traceId)).toEqual(["t9", "t8"]);
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
