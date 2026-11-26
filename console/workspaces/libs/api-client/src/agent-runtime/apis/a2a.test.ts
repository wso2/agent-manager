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

import { describe, expect, it, vi } from "vitest";
import { httpDELETE, httpGET, httpPOST, httpPUT } from "../../shared/utils";
import { describeHttpCases, getToken, type HttpMocks } from "../../test-utils/http-cases";
import { deleteA2AProxy, getA2AProxy, refreshA2AProxy, setA2AProxy } from "./a2a-proxy";
import {
  deleteAgentCardSource, getAgentCard, refreshAgentCard, setAgentCardSource,
} from "./agent-card";

vi.mock("../../shared/utils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../shared/utils")>()),
  httpGET: vi.fn(),
  httpPOST: vi.fn(),
  httpPUT: vi.fn(),
  httpDELETE: vi.fn(),
}));

const mocks = {
  GET: vi.mocked(httpGET),
  POST: vi.mocked(httpPOST),
  PUT: vi.mocked(httpPUT),
  DELETE: vi.mocked(httpDELETE),
} as unknown as HttpMocks;

const params = { orgName: "acme corp", projName: "p/1", agentName: "bot", envId: "dev" };
const ENV = "/m/ai-gateway/agents/bot";
const IN_PROJECT = { project: "p/1" };
const proxy = { status: "DEPLOYED", gatewayUrl: "https://gw.example/a2a" };
const card = { source: "external", status: "fetched", sourceUrl: "https://a.example/card" };

// These functions leave non-OK handling to the http helpers, which throw.
describeHttpCases("A2A proxy API", mocks, [
  {
    name: "getA2AProxy", call: () => getA2AProxy(params, getToken),
    method: "GET", url: `${ENV}/a2a-proxy`, searchParams: IN_PROJECT, response: proxy, checksOk: false,
  },
  {
    name: "setA2AProxy", call: () => setA2AProxy(params, { mode: "gateway" } as never, getToken),
    method: "PUT", url: `${ENV}/a2a-proxy`, searchParams: IN_PROJECT, body: { mode: "gateway" }, returnsBody: false, checksOk: false,
  },
  {
    name: "deleteA2AProxy", call: () => deleteA2AProxy(params, getToken),
    method: "DELETE", url: `${ENV}/a2a-proxy`, searchParams: IN_PROJECT, returnsBody: false, checksOk: false,
  },
  {
    name: "refreshA2AProxy", call: () => refreshA2AProxy(params, getToken),
    method: "POST", url: `${ENV}/a2a-proxy/refresh`, searchParams: IN_PROJECT, body: {}, returnsBody: false, checksOk: false,
  },
]);

describeHttpCases("agent card API", mocks, [
  {
    name: "getAgentCard", call: () => getAgentCard(params, getToken),
    method: "GET", url: `${ENV}/agent-card`, searchParams: IN_PROJECT, response: card, checksOk: false,
  },
  {
    name: "refreshAgentCard", call: () => refreshAgentCard(params, getToken),
    method: "POST", url: `${ENV}/agent-card/refresh`, searchParams: IN_PROJECT, body: {}, returnsBody: false, checksOk: false,
  },
  {
    name: "setAgentCardSource",
    call: () => setAgentCardSource(params, { url: "https://a.example/card" }, getToken),
    method: "PUT", url: `${ENV}/agent-card/source`, searchParams: IN_PROJECT, body: { url: "https://a.example/card" },
    returnsBody: false, checksOk: false,
  },
  {
    name: "deleteAgentCardSource", call: () => deleteAgentCardSource(params, getToken),
    method: "DELETE", url: `${ENV}/agent-card/source`, searchParams: IN_PROJECT, returnsBody: false, checksOk: false,
  },
]);

describe("A2A path params", () => {
  it.each(["projName", "agentName"] as const)(
    "refuses a missing %s before any request",
    async (missing) => {
      vi.mocked(httpGET).mockClear();
      const partial = { ...params, [missing]: undefined };
      await expect(getA2AProxy(partial as never, getToken)).rejects.toThrow(missing);
      await expect(getAgentCard(partial as never, getToken)).rejects.toThrow(missing);
      expect(httpGET).not.toHaveBeenCalled();
    },
  );
});
