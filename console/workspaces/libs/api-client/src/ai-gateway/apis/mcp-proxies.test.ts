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
  proxyFromDocument,
  proxyListFromDocuments,
  documentFromProxy,
  updateDocumentFromProxy,
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
const PROXIES = "/m/ai-gateway/mcp-proxies";
const proxy = { orgName: "acme corp", proxyId: "orders/v1" };
const body = { id: "orders", name: "Orders MCP", version: "v1.0" };
const doc = documentFromProxy(body as never);
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
    method: "GET", url: PROXIES,
  },
  {
    name: "listAvailableMCPPolicies",
    call: () => listAvailableMCPPolicies({ orgName: "acme corp" }, getToken),
    method: "GET", url: `${PROXIES}/policies`,
  },
  {
    name: "createMCPProxy posts a document",
    call: () => createMCPProxy({ orgName: "acme corp" }, doc, getToken),
    method: "POST", url: PROXIES, body: doc,
  },
  { name: "getMCPProxy", call: () => getMCPProxy(proxy, getToken), method: "GET", url: `${PROXIES}/orders%2Fv1` },
  {
    name: "getProjectMCPProxy",
    call: () => getProjectMCPProxy({ ...proxy, projName: "p 1" }, getToken),
    method: "GET", url: `${ORG}/projects/p%201/mcp-proxies/orders%2Fv1`,
  },
  {
    name: "updateMCPProxy puts a document",
    call: () => updateMCPProxy(proxy, doc, getToken),
    method: "PUT", url: `${PROXIES}/orders%2Fv1`, body: doc,
  },
  {
    name: "deleteMCPProxy", call: () => deleteMCPProxy(proxy, getToken),
    method: "DELETE", url: `${PROXIES}/orders%2Fv1`, returnsBody: false,
  },
  {
    name: "fetchMCPProxyServerInfo",
    call: () => fetchMCPProxyServerInfo({ orgName: "acme corp" }, { url: "https://mcp.example" } as never, getToken),
    method: "POST", url: `${PROXIES}/fetch-server-info`, body: { url: "https://mcp.example" },
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

// The UI still works on the flat proxy, so the conversions must carry every field across.
describe("MCP proxy document conversions", () => {
  const full = {
    id: "orders",
    name: "Orders MCP",
    version: "v1.0",
    description: "order tools",
    context: "/orders",
    vhost: "api.example.com",
    mcpSpecVersion: "2025-03-26",
    inCatalog: true,
    endpoints: [
      {
        id: "primary",
        upstream: { main: { url: "https://mcp.example" } },
        environments: [{ environmentUuid: "env-1", deploymentStatus: "Deployed" as const }],
      },
    ],
    subscriptionPlans: ["gold", "silver"],
  };

  it("maps the handle to metadata.name and the label to spec.displayName", () => {
    const document = documentFromProxy(full as never);
    expect(document.apiVersion).toBe("ai-control-plane.choreolabs.com/v1");
    expect(document.kind).toBe("McpProxy");
    expect(document.metadata.name).toBe("orders");
    expect(document.spec.displayName).toBe("Orders MCP");
    expect(document.spec).not.toHaveProperty("id");
    expect(document.spec).not.toHaveProperty("name");
  });

  it("leaves the offered plans out when the body has none, so an update keeps them", () => {
    const withoutPlans = { ...full, subscriptionPlans: undefined };
    expect(documentFromProxy(withoutPlans as never).spec).not.toHaveProperty("subscriptionPlans");
    const none = documentFromProxy({ ...full, subscriptionPlans: [] } as never);
    expect(none.spec.subscriptionPlans).toEqual([]);
  });

  it("carries every spec key back through a round trip", () => {
    expect(proxyFromDocument(documentFromProxy(full as never))).toEqual(full);
  });

  it("names the update document after the path's proxy, not the body", () => {
    const document = updateDocumentFromProxy("orders-v2", full as never);
    expect(document.metadata.name).toBe("orders-v2");
    expect(document.spec.displayName).toBe("Orders MCP");
  });

  it("keeps an absent endpoints key absent rather than sending an empty list", () => {
    const withoutEndpoints = { ...full, endpoints: undefined };
    const document = documentFromProxy(withoutEndpoints as never);
    expect(document.spec).not.toHaveProperty("endpoints");
    expect(JSON.stringify(document)).not.toContain("endpoints");

    const cleared = documentFromProxy({ ...full, endpoints: [] } as never);
    expect(cleared.spec.endpoints).toEqual([]);
  });

  it("reads a page of sparse documents as the flat list the table renders", () => {
    const list = proxyListFromDocuments({
      count: 1,
      list: [documentFromProxy(full as never)],
      pagination: { limit: 20, offset: 0, total: 7 },
    });
    expect(list.count).toBe(1);
    expect(list.pagination).toEqual({ count: 7, limit: 20, offset: 0 });
    expect(list.list[0]).toEqual({
      id: "orders",
      name: "Orders MCP",
      version: "v1.0",
      description: "order tools",
      context: "/orders",
      mcpSpecVersion: "2025-03-26",
      projectName: undefined,
      createdBy: undefined,
      createdAt: undefined,
      updatedAt: undefined,
    });
  });
});
