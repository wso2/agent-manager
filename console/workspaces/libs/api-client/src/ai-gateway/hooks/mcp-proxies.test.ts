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

import { vi } from "vitest";

const modules = vi.hoisted(() => ({ installed: new Set(["ai-gateway"]) }));
vi.mock("@agent-management-platform/views", () => ({
  useSnackBar: () => ({ pushSnackBar: vi.fn() }),
  useIsUIModuleInstalled: (id: string) => modules.installed.has(id),
}));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: async () => "t", isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("../../acp-core/hooks/telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/mcp-proxies", async (importOriginal) => ({
  // The document conversions stay real; only the requests are mocked.
  ...(await importOriginal<object>()),
  createMCPProxy: vi.fn(),
  deleteMCPProxy: vi.fn(),
  fetchMCPProxyServerInfo: vi.fn(),
  getMCPProxy: vi.fn(),
  getProjectMCPProxy: vi.fn(),
  listMCPProxies: vi.fn(),
  updateMCPProxy: vi.fn(),
  listAvailableMCPPolicies: vi.fn(),
}));
vi.mock("../apis/mcp-proxy-scopes", async (importOriginal) => ({
  // The document conversions stay real; only the requests are mocked.
  ...(await importOriginal<object>()),
  listMCPProxyScopes: vi.fn(),
  createMCPProxyScope: vi.fn(),
  updateMCPProxyScope: vi.fn(),
  deleteMCPProxyScope: vi.fn(),
}));

import * as proxiesApi from "../apis/mcp-proxies";
import * as scopesApi from "../apis/mcp-proxy-scopes";
import { describeMutationHooks, describeQueryHooks } from "../../test-utils/hook-cases";
import {
  useCreateMCPProxy,
  useDeleteMCPProxy,
  useFetchMCPProxyServerInfo,
  useGetMCPProxy,
  useGetProjectMCPProxy,
  useListMCPProxies,
  useUpdateMCPProxy,
} from "./mcp-proxies";
import {
  useCreateMCPProxyScope, useDeleteMCPProxyScope, useListMCPProxyScopes, useUpdateMCPProxyScope,
} from "./mcp-proxy-scopes";

const setModulePresent = (present: boolean) => {
  if (present) modules.installed.add("ai-gateway");
  else modules.installed.clear();
};

const m = vi.mocked(proxiesApi);
const s = vi.mocked(scopesApi);
const org = { orgName: "o" };
const proxy = { orgName: "o", proxyId: "px" };
const projectProxy = { ...proxy, projName: "p" };

// The API serves documents; the hooks hand the UI the flat shapes it still works on.
const doc = {
  apiVersion: "ai-control-plane.choreolabs.com/v1",
  kind: "McpProxy",
  metadata: { name: "orders", createdBy: "chamila" },
  spec: { displayName: "Orders MCP", version: "v1.0", context: "/orders" },
};
const docList = { count: 1, list: [doc], pagination: { limit: 20, offset: 0, total: 1 } };
const flat = proxiesApi.proxyFromDocument(doc as never);
const flatList = proxiesApi.proxyListFromDocuments(docList as never);

const scopeDoc = {
  apiVersion: "ai-control-plane.choreolabs.com/v1",
  kind: "McpProxyScope",
  metadata: { name: "read" },
  spec: { scope: "orders:read", tools: ["list_orders"] },
};
const scopeDocList = { count: 1, list: [scopeDoc] };
const flatScope = scopesApi.scopeFromDocument(scopeDoc as never);
const flatScopeList = scopesApi.scopeListFromDocuments(scopeDocList as never);

describeQueryHooks("MCP proxy queries", [
  {
    name: "useListMCPProxies",
    use: () => useListMCPProxies(org, { project: "p" }),
    api: m.listMCPProxies, args: [org, { project: "p" }], key: ["mcp-proxies", org, { project: "p" }],
    data: docList, expected: flatList,
    idle: [() => useListMCPProxies({ orgName: "" })],
  },
  {
    name: "useGetMCPProxy",
    use: () => useGetMCPProxy(proxy),
    api: m.getMCPProxy, args: [proxy], key: ["mcp-proxy", proxy],
    data: doc, expected: flat,
    idle: [() => useGetMCPProxy({ ...proxy, proxyId: "" })],
  },
  {
    name: "useGetProjectMCPProxy",
    use: () => useGetProjectMCPProxy(projectProxy),
    api: m.getProjectMCPProxy, args: [projectProxy], key: ["project-mcp-proxy", projectProxy],
    idle: [() => useGetProjectMCPProxy({ ...projectProxy, projName: undefined } as never)],
  },
  {
    name: "useListMCPProxyScopes",
    use: () => useListMCPProxyScopes(proxy),
    api: s.listMCPProxyScopes, args: [proxy], key: ["mcp-proxy-scopes", proxy],
    data: scopeDocList, expected: flatScopeList,
    idle: [() => useListMCPProxyScopes(proxy, { enabled: false }), () => useListMCPProxyScopes({ ...proxy, proxyId: "" })],
  },
], { setModulePresent });

const SEEDED = {
  proxies: ["mcp-proxies", org],
  proxy: ["mcp-proxy", proxy],
  scopes: ["mcp-proxy-scopes", proxy],
  otherScopes: ["mcp-proxy-scopes", { orgName: "o", proxyId: "other" }],
} as const;

const body = { id: "orders", name: "Orders MCP", version: "v1.0", context: "/orders" };
const scope = { ...proxy, scopeAction: "read" };
const probe = new AbortController();

describeMutationHooks("MCP proxy mutations", SEEDED, [
  {
    name: "useCreateMCPProxy posts a document", use: useCreateMCPProxy, api: m.createMCPProxy,
    variables: { params: org, body },
    args: [org, proxiesApi.documentFromProxy(body as never)],
    response: doc, expected: flat, invalidates: ["proxies"],
  },
  {
    name: "useUpdateMCPProxy puts a document named by the path's proxy", use: useUpdateMCPProxy,
    api: m.updateMCPProxy, variables: { params: proxy, body },
    args: [proxy, proxiesApi.updateDocumentFromProxy("px", body as never)],
    response: doc, expected: flat, invalidates: ["proxies", "proxy"],
  },
  {
    name: "useDeleteMCPProxy", use: useDeleteMCPProxy, api: m.deleteMCPProxy,
    variables: proxy, args: [proxy], invalidates: ["proxies"],
  },
  {
    name: "useFetchMCPProxyServerInfo", use: useFetchMCPProxyServerInfo, api: m.fetchMCPProxyServerInfo,
    variables: { params: org, body: { url: "u" }, signal: probe.signal },
    args: [org, { url: "u" }], after: [probe.signal], invalidates: [],
  },
  {
    name: "useCreateMCPProxyScope posts a document named by the action", use: useCreateMCPProxyScope,
    api: s.createMCPProxyScope, variables: { params: proxy, body: { action: "read" } },
    args: [proxy, scopesApi.documentFromScopeRequest({ action: "read" } as never)],
    response: scopeDoc, expected: flatScope, invalidates: ["scopes"],
  },
  {
    name: "useUpdateMCPProxyScope puts a document carrying the declared empty tool list",
    use: useUpdateMCPProxyScope, api: s.updateMCPProxyScope,
    variables: { params: scope, body: { tools: [] } },
    args: [scope, scopesApi.documentFromScopeUpdate("read", { tools: [] } as never)],
    response: scopeDoc, expected: flatScope, invalidates: ["scopes"],
  },
  {
    name: "useDeleteMCPProxyScope", use: useDeleteMCPProxyScope, api: s.deleteMCPProxyScope,
    variables: scope, args: [scope], invalidates: ["scopes"],
  },
]);
