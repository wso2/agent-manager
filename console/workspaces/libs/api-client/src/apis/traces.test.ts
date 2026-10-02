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
import { httpGETObserver } from "../utils";
import { getTraceList, type ObserverTraceListParams } from "./traces";

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
    [true, { includeModels: "true" }],
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
