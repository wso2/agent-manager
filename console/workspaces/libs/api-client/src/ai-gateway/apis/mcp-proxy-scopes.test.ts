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
import {
  createMCPProxyScope, deleteMCPProxyScope, listMCPProxyScopes, updateMCPProxyScope,
  documentFromScopeRequest, documentFromScopeUpdate, scopeFromDocument, scopeListFromDocuments,
} from "./mcp-proxy-scopes";

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

const SCOPES = "/m/ai-gateway/mcp-proxies/orders%2Fv1/scopes";
const proxy = { orgName: "acme corp", proxyId: "orders/v1" };
const scope = { ...proxy, scopeAction: "read:orders" };
const createDoc = documentFromScopeRequest({ action: "read:orders", tools: ["list"] } as never);
const updateDoc = documentFromScopeUpdate("read:orders", { tools: ["get"] } as never);

describeHttpCases("MCP proxy scope API", mocks, [
  { name: "listMCPProxyScopes", call: () => listMCPProxyScopes(proxy, getToken), method: "GET", url: SCOPES },
  {
    name: "createMCPProxyScope posts a document",
    call: () => createMCPProxyScope(proxy, createDoc, getToken),
    method: "POST", url: SCOPES, body: createDoc,
  },
  {
    name: "updateMCPProxyScope puts a document",
    call: () => updateMCPProxyScope(scope, updateDoc, getToken),
    method: "PUT", url: `${SCOPES}/read%3Aorders`, body: updateDoc,
  },
  {
    name: "deleteMCPProxyScope",
    call: () => deleteMCPProxyScope(scope, getToken),
    method: "DELETE", url: `${SCOPES}/read%3Aorders`, returnsBody: false,
  },
]);

describe("MCP proxy scope API required params", () => {
  it.each([
    ["updateMCPProxyScope", () => updateMCPProxyScope({ ...scope, scopeAction: "" }, {} as never, getToken)],
    ["deleteMCPProxyScope", () => deleteMCPProxyScope({ ...scope, scopeAction: "" }, getToken)],
  ])("%s refuses a missing scope action", async (_n, call) => {
    mocks.PUT.mockReset();
    mocks.DELETE.mockReset();
    await expect(call()).rejects.toThrow("scopeAction");
    expect(mocks.PUT).not.toHaveBeenCalled();
    expect(mocks.DELETE).not.toHaveBeenCalled();
  });
});

describe("MCP proxy scope API refuses a missing proxy", () => {
  it.each([
    ["list", () => listMCPProxyScopes({ orgName: "acme" } as never, getToken), "proxyId"],
    ["create", () => createMCPProxyScope({ orgName: "acme" } as never, {} as never, getToken), "proxyId"],
  ])("on %s", async (_n, call, label) => {
    mocks.GET.mockReset();
    mocks.POST.mockReset();
    await expect(call()).rejects.toThrow(label);
    expect(mocks.GET).not.toHaveBeenCalled();
    expect(mocks.POST).not.toHaveBeenCalled();
  });
});

// The UI still works on the flat scope, so the conversions must carry every field across.
describe("MCP proxy scope document conversions", () => {
  it("maps the action to metadata.name and reads the derived token scope back", () => {
    const document = documentFromScopeRequest({
      action: "read:orders", description: "read orders", tools: ["list"],
    } as never);
    expect(document.apiVersion).toBe("ai-control-plane.choreolabs.com/v1");
    expect(document.kind).toBe("McpProxyScope");
    expect(document.metadata.name).toBe("read:orders");
    expect(document.spec).not.toHaveProperty("action");

    expect(scopeFromDocument({
      ...document,
      metadata: { ...document.metadata, creationTimestamp: "2026-01-15T10:30:00Z" },
      spec: { ...document.spec, scope: "orders:read:orders" },
    })).toEqual({
      action: "read:orders",
      scope: "orders:read:orders",
      description: "read orders",
      tools: ["list"],
      createdAt: "2026-01-15T10:30:00Z",
      updatedAt: undefined,
    });
  });

  it("keeps an absent tools key absent and a declared empty one empty", () => {
    const described = documentFromScopeUpdate("read", { description: "d" } as never);
    expect(described.spec).not.toHaveProperty("tools");
    expect(JSON.stringify(described)).not.toContain("tools");

    const unbound = documentFromScopeUpdate("read", { tools: [] } as never);
    expect(unbound.spec.tools).toEqual([]);
    expect(unbound.spec).not.toHaveProperty("description");
  });

  it("names the update document after the path's action, which is immutable", () => {
    expect(documentFromScopeUpdate("read", { tools: ["get"] } as never).metadata.name).toBe("read");
  });

  it("reads a page of documents as the flat list the UI renders", () => {
    const list = scopeListFromDocuments({
      count: 1,
      list: [{ ...documentFromScopeRequest({ action: "read" } as never), spec: { scope: "p:read" } }],
    } as never);
    expect(list.scopes).toEqual([
      { action: "read", scope: "p:read", description: undefined, tools: [], createdAt: undefined, updatedAt: undefined },
    ]);
  });
});
