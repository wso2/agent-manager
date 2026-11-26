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

vi.mock("@agent-management-platform/views", () => ({ useSnackBar: () => ({ pushSnackBar: vi.fn() }) }));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: async () => "t", isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("../../acp-core/hooks/telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/agent-mcp-configs", () => ({
  createAgentMCPConfig: vi.fn(),
  createMCPConfigAPIKey: vi.fn(),
  deleteAgentMCPConfig: vi.fn(),
  getAgentMCPConfig: vi.fn(),
  listAgentMCPConfigs: vi.fn(),
  listMCPConfigAPIKeys: vi.fn(),
  revokeMCPConfigAPIKey: vi.fn(),
  rotateMCPConfigAPIKey: vi.fn(),
  updateAgentMCPConfig: vi.fn(),
}));

import * as api from "../apis/agent-mcp-configs";
import { describeMutationHooks, describeQueryHooks } from "../../test-utils/hook-cases";
import {
  agentConfigDocumentFromCreateRequest,
  agentConfigDocumentFromUpdateRequest,
  agentConfigFromDocument,
  agentConfigListFromDocuments,
} from "../apis/agent-config-documents";
import {
  useCreateAgentMCPConfig,
  useCreateMCPConfigAPIKey,
  useDeleteAgentMCPConfig,
  useGetAgentMCPConfig,
  useListAgentMCPConfigs,
  useListMCPConfigAPIKeys,
  useRevokeMCPConfigAPIKey,
  useRotateMCPConfigAPIKey,
  useUpdateAgentMCPConfig,
} from "./agent-mcp-configs";

const m = vi.mocked(api);
const doc = {
  apiVersion: "ai-control-plane.choreolabs.com/v1", kind: "AgentMcpConfig" as const,
  metadata: { name: "cfg", project: "p", uid: "c1" }, spec: { description: "d" },
};
const docPage = { count: 1, list: [doc], pagination: { count: 1, offset: 0, limit: 20 } };

// useUpdating reads the current document first, so the update case serves it.
function useUpdating() {
  m.getAgentMCPConfig.mockResolvedValue(doc as never);
  return useUpdateAgentMCPConfig();
}
const agent = { orgName: "o", projName: "p", agentName: "a" };
const otherAgent = { ...agent, agentName: "b" };
const config = { ...agent, configId: "c1" };
const keys = { ...config, envName: "dev" };

describeQueryHooks("agent MCP config queries", [
  {
    name: "useListAgentMCPConfigs",
    use: () => useListAgentMCPConfigs(agent),
    api: m.listAgentMCPConfigs, args: [agent, undefined], key: ["agent-mcp-configs", "list", agent, undefined],
    data: docPage, expected: agentConfigListFromDocuments(docPage, "a"),
    idle: [() => useListAgentMCPConfigs({ ...agent, projName: "" })],
  },
  {
    name: "useGetAgentMCPConfig",
    use: () => useGetAgentMCPConfig(config),
    api: m.getAgentMCPConfig, args: [config], key: ["agent-mcp-configs", config],
    data: doc, expected: agentConfigFromDocument(doc, "a"),
    idle: [() => useGetAgentMCPConfig({ ...agent, configId: undefined } as never)],
  },
  {
    name: "useListMCPConfigAPIKeys",
    use: () => useListMCPConfigAPIKeys(keys),
    api: m.listMCPConfigAPIKeys, args: [keys], key: ["mcp-config-api-keys", "o", "p", "a", "c1", "dev"],
    idle: [() => useListMCPConfigAPIKeys({ ...keys, configId: "" })],
  },
]);

// The agent's MCP proxies and its configuration (AgentID scopes) derive from its MCP configs.
const SEEDED = {
  configs: ["agent-mcp-configs", "list", agent],
  proxies: ["agent-mcp-proxies", agent],
  otherProxies: ["agent-mcp-proxies", otherAgent],
  configurations: ["agent-configurations", agent, { environment: "dev" }],
  otherConfigurations: ["agent-configurations", otherAgent, { environment: "dev" }],
  keys: ["mcp-config-api-keys", "o", "p", "a", "c1", "dev"],
} as const;

const body = { name: "orders" };
const keyParams = { ...keys, keyName: "k" };

describeMutationHooks("agent MCP config mutations", SEEDED, [
  {
    name: "useCreateAgentMCPConfig", use: useCreateAgentMCPConfig, api: m.createAgentMCPConfig,
    variables: { params: agent, body },
    args: [agent, agentConfigDocumentFromCreateRequest("AgentMcpConfig", "p", body as never)],
    response: doc, expected: agentConfigFromDocument(doc, "a"), invalidates: ["configs", "proxies", "configurations"],
  },
  {
    name: "useUpdateAgentMCPConfig", use: useUpdating, api: m.updateAgentMCPConfig,
    variables: { params: config, body },
    args: [config, agentConfigDocumentFromUpdateRequest("AgentMcpConfig", "p", body, doc)],
    response: doc, expected: agentConfigFromDocument(doc, "a"), invalidates: ["configs", "proxies", "configurations"],
  },
  {
    name: "useDeleteAgentMCPConfig", use: useDeleteAgentMCPConfig, api: m.deleteAgentMCPConfig,
    variables: config, args: [config], invalidates: ["configs", "proxies", "configurations"],
  },
  {
    name: "useCreateMCPConfigAPIKey", use: useCreateMCPConfigAPIKey, api: m.createMCPConfigAPIKey,
    variables: { params: keys, body: { name: "ci" } }, args: [keys, { name: "ci" }], invalidates: ["keys"],
  },
  {
    name: "useRotateMCPConfigAPIKey", use: useRotateMCPConfigAPIKey, api: m.rotateMCPConfigAPIKey,
    variables: { params: keyParams, body: {} }, args: [keyParams, {}], invalidates: ["keys"],
  },
  {
    name: "useRevokeMCPConfigAPIKey", use: useRevokeMCPConfigAPIKey, api: m.revokeMCPConfigAPIKey,
    variables: keyParams, args: [keyParams], invalidates: ["keys"],
  },
]);
