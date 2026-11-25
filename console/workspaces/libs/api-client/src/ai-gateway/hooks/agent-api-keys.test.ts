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
vi.mock("../apis/agent-api-keys", () => ({
  createAgentAPIKey: vi.fn(),
  issueTestAgentAPIKey: vi.fn(),
  listAgentAPIKeys: vi.fn(),
  revokeAgentAPIKey: vi.fn(),
  rotateAgentAPIKey: vi.fn(),
}));
vi.mock("../apis/agent-mcp-proxies", () => ({ addAgentMCPProxy: vi.fn(), listAgentMCPProxies: vi.fn() }));

import * as keysApi from "../apis/agent-api-keys";
import * as proxiesApi from "../apis/agent-mcp-proxies";
import { describeMutationHooks, describeQueryHooks } from "../../test-utils/hook-cases";
import {
  useCreateAgentAPIKey,
  useListAgentAPIKeys,
  useRevokeAgentAPIKey,
  useRotateAgentAPIKey,
  useTestAgentAPIKey,
} from "./agent-api-keys";
import { useAddAgentMCPProxy, useListAgentMCPProxies } from "./agent-mcp-proxies";

const setModulePresent = (present: boolean) => {
  if (present) modules.installed.add("ai-gateway");
  else modules.installed.clear();
};

const k = vi.mocked(keysApi);
const p = vi.mocked(proxiesApi);
const agent = { orgName: "o", projName: "p", agentName: "a" };
const env = { ...agent, envId: "dev" };

describeQueryHooks("agent API key and MCP proxy queries", [
  {
    name: "useListAgentAPIKeys",
    use: () => useListAgentAPIKeys(env),
    api: k.listAgentAPIKeys, args: [env], key: ["agent-api-keys", "o", "p", "a", "dev"],
    idle: [() => useListAgentAPIKeys({ ...env, envId: "" })],
  },
  {
    name: "useListAgentMCPProxies",
    use: () => useListAgentMCPProxies(agent),
    api: p.listAgentMCPProxies, args: [agent], key: ["agent-mcp-proxies", agent],
    idle: [() => useListAgentMCPProxies({ ...agent, agentName: "" })],
  },
], { setModulePresent });

// The Try-It key is not gated on the module: the caller decides when to issue it.
describeQueryHooks("test API key", [
  {
    name: "useTestAgentAPIKey",
    use: () => useTestAgentAPIKey(env, { enabled: true }),
    api: k.issueTestAgentAPIKey, args: [env], key: ["agent-test-api-key", "o", "p", "a", "dev"],
    idle: [() => useTestAgentAPIKey(env, { enabled: false }), () => useTestAgentAPIKey({ ...env, envId: "" }, { enabled: true })],
    gated: false,
  },
]);

const SEEDED = {
  devKeys: ["agent-api-keys", "o", "p", "a", "dev"],
  prodKeys: ["agent-api-keys", "o", "p", "a", "prod"],
  proxies: ["agent-mcp-proxies", agent],
  otherProxies: ["agent-mcp-proxies", { ...agent, agentName: "b" }],
  mcpConfigs: ["agent-mcp-configs", "list", agent],
} as const;

describeMutationHooks("agent API key and MCP proxy mutations", SEEDED, [
  {
    name: "useCreateAgentAPIKey", use: useCreateAgentAPIKey, api: k.createAgentAPIKey,
    variables: { params: env, body: { name: "ci" } }, args: [env, { name: "ci" }], invalidates: ["devKeys"],
  },
  {
    name: "useRotateAgentAPIKey", use: useRotateAgentAPIKey, api: k.rotateAgentAPIKey,
    variables: { params: { ...env, keyName: "ci" }, body: {} }, args: [{ ...env, keyName: "ci" }, {}], invalidates: ["devKeys"],
  },
  {
    name: "useRevokeAgentAPIKey", use: useRevokeAgentAPIKey, api: k.revokeAgentAPIKey,
    variables: { ...env, keyName: "ci" }, args: [{ ...env, keyName: "ci" }], invalidates: ["devKeys"],
  },
  {
    name: "useAddAgentMCPProxy", use: useAddAgentMCPProxy, api: p.addAgentMCPProxy,
    variables: { params: agent, body: { proxyId: "orders" } }, args: [agent, { proxyId: "orders" }],
    invalidates: ["proxies", "mcpConfigs"],
  },
]);
