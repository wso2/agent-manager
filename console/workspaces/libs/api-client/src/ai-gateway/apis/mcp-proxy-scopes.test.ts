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

const SCOPES = "/m/ai-gateway/orgs/acme%20corp/mcp-proxies/orders%2Fv1/scopes";
const proxy = { orgName: "acme corp", proxyId: "orders/v1" };
const scope = { ...proxy, scopeAction: "read:orders" };

describeHttpCases("MCP proxy scope API", mocks, [
  { name: "listMCPProxyScopes", call: () => listMCPProxyScopes(proxy, getToken), method: "GET", url: SCOPES },
  {
    name: "createMCPProxyScope",
    call: () => createMCPProxyScope(proxy, { action: "read:orders", tools: ["list"] } as never, getToken),
    method: "POST", url: SCOPES, body: { action: "read:orders", tools: ["list"] },
  },
  {
    name: "updateMCPProxyScope",
    call: () => updateMCPProxyScope(scope, { tools: ["get"] } as never, getToken),
    method: "PUT", url: `${SCOPES}/read%3Aorders`, body: { tools: ["get"] },
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

describe("MCP proxy scope API refuses a missing org or proxy", () => {
  it.each([
    ["org", () => listMCPProxyScopes({ proxyId: "p" } as never, getToken), "orgName"],
    ["proxy", () => listMCPProxyScopes({ orgName: "acme" } as never, getToken), "proxyId"],
    ["proxy on create", () => createMCPProxyScope({ orgName: "acme" } as never, {} as never, getToken), "proxyId"],
  ])("without the %s", async (_n, call, label) => {
    mocks.GET.mockReset();
    mocks.POST.mockReset();
    await expect(call()).rejects.toThrow(label);
    expect(mocks.GET).not.toHaveBeenCalled();
    expect(mocks.POST).not.toHaveBeenCalled();
  });
});
