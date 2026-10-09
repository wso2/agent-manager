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
import type { TraceFilters, TraceInclude } from "@agent-management-platform/types";
import { httpGETObserver } from "../utils";
import { exportTraces, getTraceList, type ObserverTraceListParams } from "./traces";

vi.mock("../utils", () => ({ httpGETObserver: vi.fn() }));

const mockGET = vi.mocked(httpGETObserver);

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
  expect(path).toBe("/api/v1/traces");
  return opts.searchParams ?? {};
}

beforeEach(() => {
  mockGET.mockReset();
  mockGET.mockImplementation(
    async () => new Response(JSON.stringify({ traces: [], totalCount: 0 })),
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
    expect(path).toBe("/api/v1/traces/export");
    return opts.searchParams ?? {};
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

describe("getTraceList Milestone 2 filters", () => {
  it.each([
    [{ tool: "search_web" }, { tool: "search_web" }],
    [{ toolError: true }, { toolError: "true" }],
    [{ mcpServer: "github" }, { mcpServer: "github" }],
    [{ minScore: 0.25 }, { minScore: "0.25" }],
    [{ maxScore: 0.5 }, { maxScore: "0.5" }],
    [{ maxScore: 0.5, evaluator: "Accuracy" }, { maxScore: "0.5", evaluator: "Accuracy" }],
  ] satisfies [TraceFilters, Record<string, string>][])("sends %o", async (filters, extra) => {
    expect(await sentQuery({ ...BASE, filters })).toEqual({ ...BASE_QUERY, ...extra });
  });

  it("sends tool, toolError and both score bounds together", async () => {
    const query = await sentQuery({
      ...BASE,
      filters: { tool: "search_web", toolError: true, minScore: 0.1, maxScore: 0.9 },
    });
    expect(query).toEqual({
      ...BASE_QUERY, tool: "search_web", toolError: "true", minScore: "0.1", maxScore: "0.9",
    });
  });

  it.each([false, undefined])("sends nothing for toolError=%s", async (toolError) => {
    expect(await sentQuery({ ...BASE, filters: { toolError } })).toEqual(BASE_QUERY);
  });

  it("omits empty strings but keeps a zero score", async () => {
    const query = await sentQuery({
      ...BASE,
      filters: { tool: "", mcpServer: "", evaluator: "", minScore: 0, maxScore: 0 },
    });
    expect(query).toEqual({ ...BASE_QUERY, minScore: "0", maxScore: "0" });
  });

  it("exports with the same filters", async () => {
    await exportTraces({
      ...BASE,
      filters: { tool: "search_web", toolError: true, mcpServer: "github", maxScore: 0.5 },
    });
    const [path, opts] = mockGET.mock.calls[0];
    expect(path).toBe("/api/v1/traces/export");
    expect(opts.searchParams).toEqual({
      ...BASE_QUERY, tool: "search_web", toolError: "true", mcpServer: "github", maxScore: "0.5",
    });
  });
});

describe("getTraceList include", () => {
  it.each([
    [undefined, true, "models"],
    [["tools"], undefined, "tools"],
    [["mcpServers"], false, "mcpServers"],
    [["mcpServers", "tools"], undefined, "tools,mcpServers"],
    [["tools"], true, "models,tools"],
    [["models"], true, "models"],
    [["tools", "models", "tools"], undefined, "models,tools"],
  ] satisfies [TraceInclude[] | undefined, boolean | undefined, string][])(
    "include=%o with includeModels=%s sends include=%s",
    async (include, includeModels, expected) => {
      expect(await sentQuery({ ...BASE, include, includeModels }))
        .toEqual({ ...BASE_QUERY, include: expected });
    },
  );

  it.each([[[]], [undefined]])("sends no include for include=%o", async (include) => {
    expect(await sentQuery({ ...BASE, include })).toEqual(BASE_QUERY);
  });
});
