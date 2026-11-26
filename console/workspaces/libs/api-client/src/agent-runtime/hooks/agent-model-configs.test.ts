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
vi.mock("../apis/agent-model-configs", () => ({
  createAgentModelConfig: vi.fn(),
  createLLMConfigAPIKey: vi.fn(),
  deleteAgentModelConfig: vi.fn(),
  getAgentModelConfig: vi.fn(),
  listAgentModelConfigs: vi.fn(),
  listLLMConfigAPIKeys: vi.fn(),
  revokeLLMConfigAPIKey: vi.fn(),
  rotateLLMConfigAPIKey: vi.fn(),
  updateAgentModelConfig: vi.fn(),
}));

import * as api from "../apis/agent-model-configs";
import { describeMutationHooks, describeQueryHooks } from "../../test-utils/hook-cases";
import {
  agentConfigDocumentFromCreateRequest,
  agentConfigDocumentFromUpdateRequest,
  agentConfigFromDocument,
  agentConfigListFromDocuments,
} from "../apis/agent-config-documents";
import {
  useCreateAgentModelConfig,
  useCreateLLMConfigAPIKey,
  useDeleteAgentModelConfig,
  useGetAgentModelConfig,
  useListAgentModelConfigs,
  useListLLMConfigAPIKeys,
  useRevokeLLMConfigAPIKey,
  useRotateLLMConfigAPIKey,
  useUpdateAgentModelConfig,
} from "./agent-model-configs";

const m = vi.mocked(api);
const doc = {
  apiVersion: "ai-control-plane.choreolabs.com/v1", kind: "AgentModelConfig" as const,
  metadata: { name: "cfg", project: "p", uid: "c1" }, spec: { description: "d" },
};
const docPage = { count: 1, list: [doc], pagination: { count: 1, offset: 0, limit: 20 } };

// useUpdating reads the current document first, so the update case serves it.
function useUpdating() {
  m.getAgentModelConfig.mockResolvedValue(doc as never);
  return useUpdateAgentModelConfig();
}
const agent = { orgName: "o", projName: "p", agentName: "a" };
const config = { ...agent, configId: "c1" };
const keys = { ...config, envName: "dev" };
const page = { limit: 10, offset: 0 };

describeQueryHooks("agent model config queries", [
  {
    name: "useListAgentModelConfigs",
    use: () => useListAgentModelConfigs(agent, page),
    api: m.listAgentModelConfigs, args: [agent, page], key: ["agent-model-configs", "list", agent, page],
    data: docPage, expected: agentConfigListFromDocuments(docPage, "a"),
    idle: [() => useListAgentModelConfigs({ ...agent, agentName: "" })],
  },
  {
    name: "useGetAgentModelConfig",
    use: () => useGetAgentModelConfig(config),
    api: m.getAgentModelConfig, args: [config], key: ["agent-model-configs", config],
    data: doc, expected: agentConfigFromDocument(doc, "a"),
    idle: [() => useGetAgentModelConfig({ ...agent, configId: "" })],
  },
  {
    name: "useListLLMConfigAPIKeys",
    use: () => useListLLMConfigAPIKeys(keys),
    api: m.listLLMConfigAPIKeys, args: [keys], key: ["llm-config-api-keys", "o", "p", "a", "c1", "dev"],
    idle: [() => useListLLMConfigAPIKeys({ ...keys, envName: "" })],
  },
]);

const SEEDED = {
  configs: ["agent-model-configs", "list", agent],
  config: ["agent-model-configs", config],
  keys: ["llm-config-api-keys", "o", "p", "a", "c1", "dev"],
  mcpConfigs: ["agent-mcp-configs", "list", agent],
} as const;

const body = { name: "gpt" };
const keyBody = { name: "ci" };

describeMutationHooks("agent model config mutations", SEEDED, [
  {
    name: "useCreateAgentModelConfig", use: useCreateAgentModelConfig, api: m.createAgentModelConfig,
    variables: { params: agent, body },
    args: [agent, agentConfigDocumentFromCreateRequest("AgentModelConfig", "p", body as never)],
    response: doc, expected: agentConfigFromDocument(doc, "a"), invalidates: ["configs", "config"],
  },
  {
    name: "useUpdateAgentModelConfig", use: useUpdating, api: m.updateAgentModelConfig,
    variables: { params: config, body },
    args: [config, agentConfigDocumentFromUpdateRequest("AgentModelConfig", "p", body, doc)],
    response: doc, expected: agentConfigFromDocument(doc, "a"), invalidates: ["configs", "config"],
  },
  {
    name: "useDeleteAgentModelConfig", use: useDeleteAgentModelConfig, api: m.deleteAgentModelConfig,
    variables: config, args: [config], invalidates: ["configs", "config"],
  },
  {
    name: "useCreateLLMConfigAPIKey", use: useCreateLLMConfigAPIKey, api: m.createLLMConfigAPIKey,
    variables: { params: keys, body: keyBody }, args: [keys, keyBody], invalidates: ["keys"],
  },
  {
    name: "useRotateLLMConfigAPIKey", use: useRotateLLMConfigAPIKey, api: m.rotateLLMConfigAPIKey,
    variables: { params: { ...keys, keyName: "k" }, body: {} }, args: [{ ...keys, keyName: "k" }, {}], invalidates: ["keys"],
  },
  {
    name: "useRevokeLLMConfigAPIKey", use: useRevokeLLMConfigAPIKey, api: m.revokeLLMConfigAPIKey,
    variables: { ...keys, keyName: "k" }, args: [{ ...keys, keyName: "k" }], invalidates: ["keys"],
  },
]);
