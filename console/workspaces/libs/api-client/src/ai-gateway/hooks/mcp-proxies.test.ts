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
vi.mock("../apis/mcp-proxies", () => ({
  createMCPProxy: vi.fn(),
  deleteMCPProxy: vi.fn(),
  fetchMCPProxyServerInfo: vi.fn(),
  getMCPProxy: vi.fn(),
  getProjectMCPProxy: vi.fn(),
  listMCPProxies: vi.fn(),
  updateMCPProxy: vi.fn(),
  listAvailableMCPPolicies: vi.fn(),
}));
vi.mock("../apis/mcp-proxy-scopes", () => ({
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

describeQueryHooks("MCP proxy queries", [
  {
    name: "useListMCPProxies",
    use: () => useListMCPProxies(org, { project: "p" }),
    api: m.listMCPProxies, args: [org, { project: "p" }], key: ["mcp-proxies", org, { project: "p" }],
    idle: [() => useListMCPProxies({ orgName: "" })],
  },
  {
    name: "useGetMCPProxy",
    use: () => useGetMCPProxy(proxy),
    api: m.getMCPProxy, args: [proxy], key: ["mcp-proxy", proxy],
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
    idle: [() => useListMCPProxyScopes(proxy, { enabled: false }), () => useListMCPProxyScopes({ ...proxy, proxyId: "" })],
  },
], { setModulePresent });

const SEEDED = {
  proxies: ["mcp-proxies", org],
  proxy: ["mcp-proxy", proxy],
  scopes: ["mcp-proxy-scopes", proxy],
  otherScopes: ["mcp-proxy-scopes", { orgName: "o", proxyId: "other" }],
} as const;

const body = { name: "orders" };
const scope = { ...proxy, scopeAction: "read" };
const probe = new AbortController();

describeMutationHooks("MCP proxy mutations", SEEDED, [
  {
    name: "useCreateMCPProxy", use: useCreateMCPProxy, api: m.createMCPProxy,
    variables: { params: org, body }, args: [org, body], invalidates: ["proxies"],
  },
  {
    name: "useUpdateMCPProxy", use: useUpdateMCPProxy, api: m.updateMCPProxy,
    variables: { params: proxy, body }, args: [proxy, body], invalidates: ["proxies", "proxy"],
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
    name: "useCreateMCPProxyScope", use: useCreateMCPProxyScope, api: s.createMCPProxyScope,
    variables: { params: proxy, body: { action: "read" } }, args: [proxy, { action: "read" }], invalidates: ["scopes"],
  },
  {
    name: "useUpdateMCPProxyScope", use: useUpdateMCPProxyScope, api: s.updateMCPProxyScope,
    variables: { params: scope, body: { tools: [] } }, args: [scope, { tools: [] }], invalidates: ["scopes"],
  },
  {
    name: "useDeleteMCPProxyScope", use: useDeleteMCPProxyScope, api: s.deleteMCPProxyScope,
    variables: scope, args: [scope], invalidates: ["scopes"],
  },
]);
