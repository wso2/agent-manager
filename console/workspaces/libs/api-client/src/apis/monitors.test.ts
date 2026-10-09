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
import type { AgentTraceScoresParams } from "@agent-management-platform/types";
import type * as Utils from "../utils";
import { httpGET } from "../utils";
import { getAgentTraceScores } from "./monitors";

vi.mock("../utils", async (importOriginal) => ({
  ...(await importOriginal<typeof Utils>()),
  httpGET: vi.fn(),
}));

const mockGET = vi.mocked(httpGET);

const BASE: AgentTraceScoresParams = {
  orgName: "org",
  projName: "proj",
  agentName: "agent",
  startTime: "2026-10-02T09:00:00Z",
  endTime: "2026-10-02T10:00:00Z",
  limit: 10,
  offset: 0,
  sortOrder: "desc",
};

const BASE_QUERY = {
  startTime: "2026-10-02T09:00:00Z",
  endTime: "2026-10-02T10:00:00Z",
  limit: "10",
  offset: "0",
  sortOrder: "desc",
};

/** Calls getAgentTraceScores and returns the query params it sent. */
async function sentQuery(params: AgentTraceScoresParams) {
  await getAgentTraceScores(params);
  const [path, opts] = mockGET.mock.calls[0];
  expect(path).toBe("/api/v1/orgs/org/projects/proj/agents/agent/scores");
  return opts.searchParams;
}

beforeEach(() => {
  mockGET.mockReset();
  mockGET.mockImplementation(
    async () => new Response(JSON.stringify({ traces: [], totalCount: 0 })),
  );
});

describe("getAgentTraceScores query string", () => {
  it("sends today's params when traceIds and evaluator are unset", async () => {
    expect(await sentQuery(BASE)).toEqual(BASE_QUERY);
  });

  it("sends traceIds comma-separated and the evaluator when set", async () => {
    expect(await sentQuery({ ...BASE, traceIds: ["t1", "t2", "t3"], evaluator: "Accuracy" }))
      .toEqual({ ...BASE_QUERY, traceIds: "t1,t2,t3", evaluator: "Accuracy" });
  });

  it("omits empty traceIds and an empty evaluator", async () => {
    expect(await sentQuery({ ...BASE, traceIds: [], evaluator: "" })).toEqual(BASE_QUERY);
  });
});
