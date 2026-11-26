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

import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TraceFilters } from "@agent-management-platform/types";
import { httpGET } from "../../shared/utils";
import {
  exportTraces,
  getTraceList,
  toSpanList,
  toTraceExport,
  toTraceList,
  type ObserverTraceListParams,
} from "./traces";

vi.mock("../../shared/utils", () => ({
  httpGET: vi.fn(),
  moduleBase: (module: string) => `/m/${module}`,
}));

const mockGET = vi.mocked(httpGET);

const BASE: ObserverTraceListParams = {
  organization: "org",
  project: "proj",
  component: "agent",
  environment: "dev",
  startTime: "2026-10-02T09:00:00Z",
  endTime: "2026-10-02T10:00:00Z",
  limit: 10,
  sortOrder: "desc",
};

const BASE_QUERY = {
  organization: "org",
  project: "proj",
  agent: "agent",
  environment: "dev",
  startTime: "2026-10-02T09:00:00Z",
  endTime: "2026-10-02T10:00:00Z",
  limit: "10",
  sortOrder: "desc",
};

/** Calls getTraceList and returns the query params it sent. */
async function sentQuery(params: ObserverTraceListParams): Promise<Record<string, string>> {
  await getTraceList(params);
  const [path, opts] = mockGET.mock.calls[0];
  expect(path).toBe("/m/observability/traces");
  return (opts.searchParams ?? {}) as Record<string, string>;
}

beforeEach(() => {
  mockGET.mockReset();
  mockGET.mockImplementation(
    async () =>
      new Response(JSON.stringify({ traces: [], pagination: { limit: 10, total: 0 } })),
  );
});

describe("getTraceList query string", () => {
  it("sends today's params when no filters, includeModels or cursor are set", async () => {
    expect(await sentQuery(BASE)).toEqual(BASE_QUERY);
  });

  it("sends every set filter", async () => {
    const query = await sentQuery({
      ...BASE,
      filters: {
        status: "error",
        minDurationMs: 5000,
        minTokens: 1000,
        minSpanCount: 20,
        model: "gpt-4o",
        conversationId: "conv-1",
      },
    });
    expect(query).toEqual({
      ...BASE_QUERY,
      status: "error",
      minDurationMs: "5000",
      minTokens: "1000",
      minSpanCount: "20",
      model: "gpt-4o",
      conversationId: "conv-1",
    });
  });

  it("omits unset and empty filters but keeps a zero threshold", async () => {
    const query = await sentQuery({
      ...BASE,
      filters: { minTokens: 0, model: "", conversationId: undefined },
    });
    expect(query).toEqual({ ...BASE_QUERY, minTokens: "0" });
  });

  it.each([
    [true, { include: "models" }],
    [false, {}],
    [undefined, {}],
  ])("includeModels=%s", async (includeModels, extra) => {
    expect(await sentQuery({ ...BASE, includeModels })).toEqual({ ...BASE_QUERY, ...extra });
  });

  it("sends the cursor only when set", async () => {
    expect(await sentQuery({ ...BASE, cursor: "abc" })).toEqual({ ...BASE_QUERY, cursor: "abc" });
    mockGET.mockClear();
    expect(await sentQuery({ ...BASE, cursor: "" })).toEqual(BASE_QUERY);
  });
});

describe("exportTraces query string", () => {
  /** Calls exportTraces and returns the query params it sent. */
  async function sentExportQuery(params: ObserverTraceListParams): Promise<Record<string, string>> {
    await exportTraces(params);
    const [path, opts] = mockGET.mock.calls[0];
    expect(path).toBe("/m/observability/traces/export");
    return (opts.searchParams ?? {}) as Record<string, string>;
  }

  it("sends today's params when no filters are set", async () => {
    expect(await sentExportQuery(BASE)).toEqual(BASE_QUERY);
  });

  it("sends every set filter", async () => {
    const query = await sentExportQuery({
      ...BASE,
      filters: {
        status: "ok",
        minDurationMs: 5000,
        minTokens: 1000,
        minSpanCount: 20,
        model: "gpt-4o",
        conversationId: "conv-1",
      },
    });
    expect(query).toEqual({
      ...BASE_QUERY,
      status: "ok",
      minDurationMs: "5000",
      minTokens: "1000",
      minSpanCount: "20",
      model: "gpt-4o",
      conversationId: "conv-1",
    });
  });

  it("omits unset and empty filters but keeps a zero threshold", async () => {
    const query = await sentExportQuery({
      ...BASE,
      filters: { status: undefined, minSpanCount: 0, model: "" },
    });
    expect(query).toEqual({ ...BASE_QUERY, minSpanCount: "0" });
  });
});

describe("getTraceList tool and MCP server filters", () => {
  it.each([
    [{ tool: "search_web" }, { tool: "search_web" }],
    [{ toolError: true }, { toolError: "true" }],
    [{ mcpServer: "github" }, { mcpServer: "github" }],
  ] satisfies [TraceFilters, Record<string, string>][])("sends %o", async (filters, extra) => {
    expect(await sentQuery({ ...BASE, filters })).toEqual({ ...BASE_QUERY, ...extra });
  });

  it("sends tool and toolError together", async () => {
    const query = await sentQuery({ ...BASE, filters: { tool: "search_web", toolError: true } });
    expect(query).toEqual({ ...BASE_QUERY, tool: "search_web", toolError: "true" });
  });

  it.each([false, undefined])("sends nothing for toolError=%s", async (toolError) => {
    expect(await sentQuery({ ...BASE, filters: { toolError } })).toEqual(BASE_QUERY);
  });

  it("omits empty strings", async () => {
    const query = await sentQuery({ ...BASE, filters: { tool: "", mcpServer: "" } });
    expect(query).toEqual(BASE_QUERY);
  });

  it("exports with the same filters", async () => {
    await exportTraces({
      ...BASE,
      filters: { tool: "search_web", toolError: true, mcpServer: "github" },
    });
    const [path, opts] = mockGET.mock.calls[0];
    expect(path).toBe("/m/observability/traces/export");
    expect(opts.searchParams).toEqual({
      ...BASE_QUERY, tool: "search_web", toolError: "true", mcpServer: "github",
    });
  });
});

describe("getTraceList missing filter", () => {
  it.each([
    [{ missing: ["conversationId"] }, { missing: "conversationId" }],
    [{ missing: [] }, {}],
  ] satisfies [TraceFilters, Record<string, string>][])("sends %o", async (filters, extra) => {
    expect(await sentQuery({ ...BASE, filters })).toEqual({ ...BASE_QUERY, ...extra });
  });
});

describe("module list responses", () => {
  const trace = {
    traceId: "a",
    rootSpanId: "root",
    rootSpanName: "invoke_agent",
    startTime: "2026-10-02T09:40:00Z",
    endTime: "2026-10-02T09:40:01Z",
    durationInNanos: 1,
    spanCount: 1,
  };

  it("reads the trace list's paging state from pagination", () => {
    const got = toTraceList({
      traces: [trace],
      pagination: {
        limit: 1,
        total: 5,
        nextCursor: "c",
        lookedBackTo: "2026-10-02T09:40:00Z",
        truncated: true,
      },
    });
    expect(got).toEqual({
      traces: [trace],
      totalCount: 5,
      nextCursor: "c",
      lookedBackTo: "2026-10-02T09:40:00Z",
      truncated: true,
    });
  });

  it("counts the page when the total is unknown", () => {
    expect(toTraceList({ traces: [trace], pagination: { limit: 10 } }).totalCount).toBe(1);
  });

  it.each([
    [false, false, false],
    [true, false, true],
    [false, true, true],
  ])("marks an export truncated=%s, spansTruncated=%s as %s", (truncated, spansTruncated, want) => {
    const got = toTraceExport({
      traces: [],
      pagination: { limit: 100, total: 3, lookedBackTo: "2026-10-02T09:00:00Z", truncated },
      spansTruncated,
      failedTraceIds: ["x"],
    });
    expect(got).toEqual({
      traces: [],
      totalCount: 3,
      lookedBackTo: "2026-10-02T09:00:00Z",
      truncated: want,
      spansTruncated,
      failedTraceIds: ["x"],
    });
  });

  it("reads the span list", () => {
    const span = {
      spanId: "s",
      spanName: "chat",
      startTime: "2026-10-02T09:00:00Z",
      endTime: "2026-10-02T10:00:00Z",
      durationNs: 1,
    };
    expect(toSpanList({ spans: [span], pagination: { limit: 10, total: 4 } })).toEqual({
      spans: [span],
      totalCount: 4,
    });
  });
});
