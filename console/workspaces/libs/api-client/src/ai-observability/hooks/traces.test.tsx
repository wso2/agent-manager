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
import type { TraceListResponse, TraceOverview } from "@agent-management-platform/types";
import type * as TracesApi from "../apis/traces";
import { getTraceList } from "../apis/traces";
import { useTraceList, type TraceListOptions } from "./traces";

const { getToken } = vi.hoisted(() => ({ getToken: async () => "token" }));

vi.mock("../apis/traces", async (importOriginal) => ({
  ...(await importOriginal<typeof TracesApi>()),
  getTraceList: vi.fn(),
}));
vi.mock("@agent-management-platform/auth", () => ({ useAuthHooks: () => ({ getToken }) }));
vi.mock("../../shared/react-query-notifications", async () => {
  const rq = await import("@tanstack/react-query");
  return { useApiQuery: rq.useQuery, useApiMutation: rq.useMutation };
});

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const mockList = vi.mocked(getTraceList);

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

/** A trace-list response holding the whole window. */
function page(traces: TraceOverview[]): TraceListResponse {
  return { traces, totalCount: traces.length };
}

/** A page of a window holding `total` traces, with lookedBackTo set as the module sets it. */
function partial(traces: TraceOverview[], total: number): TraceListResponse {
  const lookedBackTo = traces[traces.length - 1].startTime;
  return { traces, totalCount: total, lookedBackTo };
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
});

afterEach(() => {
  act(() => root?.unmount());
  root = undefined;
  focusManager.setFocused(undefined);
});

describe("useTraceList window paging", () => {
  it("loadMore moves the window's end to the last trace shown, in desc", async () => {
    mockList
      .mockResolvedValueOnce(partial(
        [trace("t1", "2026-10-02T09:50:00Z"), trace("t2", "2026-10-02T09:40:00Z")], 4,
      ))
      .mockResolvedValueOnce(partial(
        [trace("t2", "2026-10-02T09:40:00Z"), trace("t3", "2026-10-02T09:30:00Z")], 3,
      ))
      .mockResolvedValueOnce(page(
        [trace("t3", "2026-10-02T09:30:00Z"), trace("t4", "2026-10-02T09:20:00Z")],
      ));

    const result = renderTraceList();
    await waitFor(() => result.current.traceList?.traces.length === 2);
    expect(result.current.hasMore).toBe(true);

    await act(() => result.current.loadMore());
    await act(() => result.current.loadMore());

    expect(mockList.mock.calls[1][0]).toMatchObject({
      startTime: START, endTime: "2026-10-02T09:40:00Z", sortOrder: "desc",
    });
    expect(mockList.mock.calls[2][0]).toMatchObject({
      startTime: START, endTime: "2026-10-02T09:30:00Z", sortOrder: "desc",
    });
    for (const [params] of mockList.mock.calls) expect(params).not.toHaveProperty("cursor");
    expect(result.current.traceList?.traces.map((t) => t.traceId)).toEqual(["t1", "t2", "t3", "t4"]);
    expect(result.current.hasMore).toBe(false);
  });

  it("loadMore moves the window's start to the last trace shown, in asc", async () => {
    mockList
      .mockResolvedValueOnce(partial(
        [trace("t1", "2026-10-02T09:10:00Z"), trace("t2", "2026-10-02T09:20:00Z")], 4,
      ))
      .mockResolvedValueOnce(partial(
        [trace("t2", "2026-10-02T09:20:00Z"), trace("t3", "2026-10-02T09:30:00Z")], 3,
      ))
      .mockResolvedValueOnce(page(
        [trace("t3", "2026-10-02T09:30:00Z"), trace("t4", "2026-10-02T09:40:00Z")],
      ));

    const result = renderTraceList(undefined, "asc");
    await waitFor(() => result.current.traceList?.traces.length === 2);
    expect(mockList.mock.calls[0][0]).toMatchObject({ startTime: START, endTime: END, sortOrder: "asc" });
    expect(result.current.hasMore).toBe(true);

    await act(() => result.current.loadMore());
    await act(() => result.current.loadMore());

    expect(mockList.mock.calls[1][0]).toMatchObject({
      startTime: "2026-10-02T09:20:00Z", endTime: END, sortOrder: "asc",
    });
    expect(mockList.mock.calls[2][0]).toMatchObject({
      startTime: "2026-10-02T09:30:00Z", endTime: END, sortOrder: "asc",
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
      .mockResolvedValueOnce(partial([trace("t1", "2026-10-02T09:50:00Z")], 2))
      .mockResolvedValueOnce(page([trace("t1", "2026-10-02T09:50:00Z"), trace("t2", "2026-10-02T09:40:00Z")]));

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

  it("reports hasMore false and skips loadMore when the page holds the whole window", async () => {
    mockList.mockResolvedValueOnce(page([trace("t1", "2026-10-02T09:50:00Z")]));

    const result = renderTraceList();
    await waitFor(() => result.current.traceList?.traces.length === 1);
    expect(result.current.hasMore).toBe(false);

    await act(() => result.current.loadMore());
    expect(mockList).toHaveBeenCalledTimes(1);
  });

  it("stops paging when a page adds no rows", async () => {
    // More traces than the page size share the edge's start time.
    mockList
      .mockResolvedValueOnce(partial(
        [trace("t1", "2026-10-02T09:50:00Z"), trace("t2", "2026-10-02T09:40:00Z")], 5,
      ))
      .mockResolvedValueOnce(partial([trace("t2", "2026-10-02T09:40:00Z")], 4));

    const result = renderTraceList();
    await waitFor(() => result.current.traceList?.traces.length === 2);

    let added: unknown;
    await act(async () => { added = await result.current.loadMore(); });
    expect(added).toBe(false);
    expect(result.current.hasMore).toBe(false);
  });

  it("shows the edge trace, which both windows hold, once", async () => {
    mockList
      .mockResolvedValueOnce(partial(
        [trace("t1", "2026-10-02T09:50:00Z"), trace("t2", "2026-10-02T09:40:00Z")], 3,
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
      .mockResolvedValueOnce(partial(
        [trace("t1", "2026-10-02T09:50:00Z"), trace("t2", "2026-10-02T09:40:00Z")], 5,
      ))
      .mockResolvedValueOnce(partial(
        [trace("t2", "2026-10-02T09:40:00Z"), trace("t3", "2026-10-02T09:30:00Z")], 4,
      ))
      .mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve; }))
      .mockResolvedValueOnce(page([trace("t9", "2026-10-02T09:55:00Z")]));

    const result = renderTraceList();
    await waitFor(() => result.current.traceList?.traces.length === 2);
    const added: unknown[] = [];
    await act(async () => { added.push(await result.current.loadMore()); });

    let oldLoad: Promise<unknown> = Promise.resolve();
    act(() => { oldLoad = result.current.loadMore(); });
    act(() => result.setOptions({ includeModels: true }));
    await waitFor(() => result.current.traceList?.traces[0]?.traceId === "t9");
    await act(async () => {
      resolveOld(page([trace("t4", "2026-10-02T09:20:00Z")]));
      added.push(await oldLoad);
    });

    expect(added).toEqual([true, undefined]);
    expect(result.current.traceList?.traces.map((t) => t.traceId)).toEqual(["t9"]);
  });

  it("sets loadError on a failed loadMore and clears it when the next loadMore starts", async () => {
    let resolveRetry: (res: TraceListResponse) => void = () => undefined;
    mockList
      .mockResolvedValueOnce(partial([trace("t1", "2026-10-02T09:50:00Z")], 2))
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
    expect(mockList.mock.calls[2][0].endTime).toBe("2026-10-02T09:50:00Z");

    await act(async () => {
      resolveRetry(page([trace("t1", "2026-10-02T09:50:00Z"), trace("t2", "2026-10-02T09:40:00Z")]));
      await retry;
    });
    expect(result.current.loadError).toBeNull();
    expect(result.current.traceList?.traces.map((t) => t.traceId)).toEqual(["t1", "t2"]);
  });

  const listResets: [string, (result: ReturnType<typeof renderTraceList>) => unknown][] = [
    ["a scope change", (result) => {
      act(() => result.setOptions({ includeModels: true }));
    }],
    ["Refresh", async (result) => {
      await act(() => result.current.refetch());
    }],
  ];

  it.each(listResets)("clears loadError when %s resets the list", async (_, reset) => {
    mockList
      .mockResolvedValueOnce(partial([trace("t1", "2026-10-02T09:50:00Z")], 2))
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
      .mockResolvedValueOnce(partial([trace("t1", "2026-10-02T09:50:00Z")], 2))
      .mockImplementationOnce(() => new Promise((_res, reject) => { rejectOld = reject; }))
      .mockResolvedValueOnce(partial([trace("t9", "2026-10-02T09:55:00Z")], 2))
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
    expect(mockList.mock.calls[3][0].endTime).toBe("2026-10-02T09:55:00Z");

    await act(async () => {
      rejectOld(new Error("upstream down"));
      await oldLoad;
    });
    expect(result.current.loadError).toBeNull();
    expect(result.current.isLoadingMore).toBe(true);

    await act(async () => {
      resolveNew(page([trace("t9", "2026-10-02T09:55:00Z"), trace("t8", "2026-10-02T09:45:00Z")]));
      await newLoad;
    });
    expect(result.current.isLoadingMore).toBe(false);
    expect(result.current.loadError).toBeNull();
    expect(result.current.traceList?.traces.map((t) => t.traceId)).toEqual(["t9", "t8"]);
  });

  it("passes includeModels and filters to the client, which decides what it sends", async () => {
    mockList.mockResolvedValueOnce(page([]));

    renderTraceList({ filters: { conversationId: "conv-1" }, includeModels: true });
    await waitFor(() => mockList.mock.calls.length === 1);

    expect(mockList.mock.calls[0][0]).toMatchObject({
      filters: { conversationId: "conv-1" },
      includeModels: true,
    });
  });
});
