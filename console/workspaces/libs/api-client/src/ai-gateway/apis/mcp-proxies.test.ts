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
import { httpDELETE, httpGET, httpPOST, httpPUT } from "../../shared/utils";
import { describeHttpCases, getToken, type HttpMocks } from "../../test-utils/http-cases";
import {
  createMCPProxy,
  deleteMCPProxy,
  fetchMCPProxyServerInfo,
  getMCPProxy,
  getProjectMCPProxy,
  listAvailableMCPPolicies,
  listMCPProxies,
  updateMCPProxy,
} from "./mcp-proxies";

vi.mock("../../shared/utils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../shared/utils")>()),
  httpGET: vi.fn(),
  httpPOST: vi.fn(),
  httpPUT: vi.fn(),
  httpDELETE: vi.fn(),
}));

const mockGET = vi.mocked(httpGET);

beforeEach(() => {
  mockGET.mockReset();
  mockGET.mockResolvedValue({ ok: true, json: async () => ({ list: [] }) } as Response);
});

describe("listMCPProxies", () => {
  it("sends the project query param when provided", async () => {
    await listMCPProxies({ orgName: "org" }, { limit: 50, offset: 0, project: "p1" });
    expect(mockGET.mock.calls[0][1]?.searchParams).toEqual({
      limit: "50",
      offset: "0",
      project: "p1",
    });
  });

  it("omits project when not provided (org-only list)", async () => {
    await listMCPProxies({ orgName: "org" }, { limit: 50, offset: 0 });
    expect(mockGET.mock.calls[0][1]?.searchParams).toEqual({ limit: "50", offset: "0" });
  });
});

describe("getProjectMCPProxy", () => {
  it("reads the proxy from its project route", async () => {
    await getProjectMCPProxy({ orgName: "org", projName: "p 1", proxyId: "orders" });
    expect(String(mockGET.mock.calls[0][0])).toMatch(
      /\/orgs\/org\/projects\/p%201\/mcp-proxies\/orders$/,
    );
  });

  it("refuses a missing project rather than calling the org route", async () => {
    await expect(
      getProjectMCPProxy({ orgName: "org", projName: undefined, proxyId: "orders" }),
    ).rejects.toThrow();
    expect(mockGET).not.toHaveBeenCalled();
  });
});

const ORG = "/m/ai-gateway/orgs/acme%20corp";
const proxy = { orgName: "acme corp", proxyId: "orders/v1" };
const body = { name: "orders", upstream: { url: "https://mcp.example" } };
const httpMocks = {
  GET: vi.mocked(httpGET),
  POST: vi.mocked(httpPOST),
  PUT: vi.mocked(httpPUT),
  DELETE: vi.mocked(httpDELETE),
} as unknown as HttpMocks;

describeHttpCases("MCP proxy API", httpMocks, [
  {
    name: "listMCPProxies without a query",
    call: () => listMCPProxies({ orgName: "acme corp" }, undefined, getToken),
    method: "GET", url: `${ORG}/mcp-proxies`,
  },
  {
    name: "listAvailableMCPPolicies",
    call: () => listAvailableMCPPolicies({ orgName: "acme corp" }, getToken),
    method: "GET", url: `${ORG}/mcp-proxies/policies`,
  },
  {
    name: "createMCPProxy",
    call: () => createMCPProxy({ orgName: "acme corp" }, body as never, getToken),
    method: "POST", url: `${ORG}/mcp-proxies`, body,
  },
  { name: "getMCPProxy", call: () => getMCPProxy(proxy, getToken), method: "GET", url: `${ORG}/mcp-proxies/orders%2Fv1` },
  {
    name: "getProjectMCPProxy",
    call: () => getProjectMCPProxy({ ...proxy, projName: "p 1" }, getToken),
    method: "GET", url: `${ORG}/projects/p%201/mcp-proxies/orders%2Fv1`,
  },
  {
    name: "updateMCPProxy",
    call: () => updateMCPProxy(proxy, body as never, getToken),
    method: "PUT", url: `${ORG}/mcp-proxies/orders%2Fv1`, body,
  },
  {
    name: "deleteMCPProxy", call: () => deleteMCPProxy(proxy, getToken),
    method: "DELETE", url: `${ORG}/mcp-proxies/orders%2Fv1`, returnsBody: false,
  },
  {
    name: "fetchMCPProxyServerInfo",
    call: () => fetchMCPProxyServerInfo({ orgName: "acme corp" }, { url: "https://mcp.example" } as never, getToken),
    method: "POST", url: `${ORG}/mcp-proxies/fetch-server-info`, body: { url: "https://mcp.example" },
  },
]);

describe("fetchMCPProxyServerInfo", () => {
  it("passes the caller's abort signal to the request", async () => {
    vi.mocked(httpPOST).mockReset().mockResolvedValue(new Response("{}"));
    const controller = new AbortController();
    await fetchMCPProxyServerInfo({ orgName: "o" }, { url: "u" } as never, getToken, controller.signal);
    expect(vi.mocked(httpPOST).mock.calls[0][2]).toEqual({ token: "jwt", signal: controller.signal });
  });
});
