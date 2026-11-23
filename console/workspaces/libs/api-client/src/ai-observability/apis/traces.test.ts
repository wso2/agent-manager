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
  traceFilterSearchParams,
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
  expect(path).toBe("/m/observability/api/v1/traces");
  return (opts.searchParams ?? {}) as Record<string, string>;
}

beforeEach(() => {
  mockGET.mockReset();
  mockGET.mockImplementation(
    async () => new Response(JSON.stringify({ traces: [], totalCount: 0 })),
  );
});

// Every filter set, and the query params the observability module refuses with 400.
const ALL_FILTERS: TraceFilters = {
  status: "error",
  minDurationMs: 5000,
  minTokens: 1000,
  minSpanCount: 20,
  model: "gpt-4o",
  conversationId: "conv-1",
  tool: "search_web",
  toolError: true,
  mcpServer: "github",
  missing: ["conversationId"],
};
const REFUSED = [
  "status", "minDurationMs", "minTokens", "minSpanCount", "model",
  "conversationId", "tool", "toolError", "mcpServer", "missing", "cursor",
];

describe("getTraceList query string", () => {
  it("sends the module's params", async () => {
    expect(await sentQuery(BASE)).toEqual(BASE_QUERY);
  });

  it("sends no param the module refuses, even with every filter set", async () => {
    const query = await sentQuery({ ...BASE, filters: ALL_FILTERS });
    expect(query).toEqual(BASE_QUERY);
    for (const key of REFUSED) expect(query).not.toHaveProperty(key);
  });

  it.each([
    [true, { include: "models" }],
    [false, {}],
    [undefined, {}],
  ])("includeModels=%s", async (includeModels, extra) => {
    expect(await sentQuery({ ...BASE, includeModels })).toEqual({ ...BASE_QUERY, ...extra });
  });
});

describe("exportTraces query string", () => {
  /** Calls exportTraces and returns the query params it sent. */
  async function sentExportQuery(params: ObserverTraceListParams): Promise<Record<string, string>> {
    await exportTraces(params);
    const [path, opts] = mockGET.mock.calls[0];
    expect(path).toBe("/m/observability/api/v1/traces/export");
    return (opts.searchParams ?? {}) as Record<string, string>;
  }

  it("sends the module's params", async () => {
    expect(await sentExportQuery(BASE)).toEqual(BASE_QUERY);
  });

  it("sends no param the module refuses, even with every filter set", async () => {
    const query = await sentExportQuery({ ...BASE, filters: ALL_FILTERS });
    expect(query).toEqual(BASE_QUERY);
    for (const key of REFUSED) expect(query).not.toHaveProperty(key);
  });
});

describe("traceFilterSearchParams", () => {
  it("encodes every set filter", () => {
    expect(traceFilterSearchParams(ALL_FILTERS)).toEqual({
      status: "error",
      minDurationMs: "5000",
      minTokens: "1000",
      minSpanCount: "20",
      model: "gpt-4o",
      conversationId: "conv-1",
      tool: "search_web",
      toolError: "true",
      mcpServer: "github",
      missing: "conversationId",
    });
  });

  it("omits unset and empty filters but keeps a zero threshold", () => {
    expect(
      traceFilterSearchParams({ minTokens: 0, model: "", conversationId: undefined, tool: "" }),
    ).toEqual({ minTokens: "0" });
  });

  it.each([false, undefined])("omits toolError=%s", (toolError) => {
    expect(traceFilterSearchParams({ toolError })).toEqual({});
  });

  it("omits an empty missing list", () => {
    expect(traceFilterSearchParams({ missing: [] })).toEqual({});
  });
});
