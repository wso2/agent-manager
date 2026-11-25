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

import { describe, expect, it, vi } from "vitest";
import { httpDELETE, httpGET, httpPOST, httpPUT } from "../../shared/utils";
import { describeHttpCases, getToken, type HttpMocks } from "../../test-utils/http-cases";
import {
  createAgentModelConfig,
  createLLMConfigAPIKey,
  deleteAgentModelConfig,
  getAgentModelConfig,
  listAgentModelConfigs,
  listLLMConfigAPIKeys,
  revokeLLMConfigAPIKey,
  rotateLLMConfigAPIKey,
  updateAgentModelConfig,
} from "./agent-model-configs";

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

const agent = { orgName: "acme corp", projName: "p/1", agentName: "bot" };
const config = { ...agent, configId: "cfg 1" };
const keyParams = { ...config, envName: "dev", keyName: "k/1" };
const BASE = "/m/ai-gateway/agents/bot/model-configs";
const KEYS = `${BASE}/cfg%201/api-keys`;
const IN_PROJECT = { project: "p/1" };
const doc = { apiVersion: "ai-control-plane.choreolabs.com/v1", kind: "X", metadata: { name: "c", project: "p/1" }, spec: {} };

describeHttpCases("agent model config API", mocks, [
  {
    name: "listAgentModelConfigs with paging",
    call: () => listAgentModelConfigs(agent, { limit: 10, offset: 30 }, getToken),
    method: "GET", url: BASE, searchParams: { ...IN_PROJECT, limit: "10", offset: "30" },
  },
  {
    name: "listAgentModelConfigs without paging",
    call: () => listAgentModelConfigs(agent, undefined, getToken),
    method: "GET", url: BASE, searchParams: IN_PROJECT,
  },
  {
    name: "createAgentModelConfig",
    call: () => createAgentModelConfig(agent, doc as never, getToken),
    method: "POST", url: BASE, searchParams: IN_PROJECT, body: doc, response: doc,
  },
  {
    name: "getAgentModelConfig",
    call: () => getAgentModelConfig(config, getToken),
    method: "GET", url: `${BASE}/cfg%201`, searchParams: IN_PROJECT, response: doc,
  },
  {
    name: "updateAgentModelConfig",
    call: () => updateAgentModelConfig(config, doc as never, getToken),
    method: "PUT", url: `${BASE}/cfg%201`, searchParams: IN_PROJECT, body: doc, response: doc,
  },
  {
    name: "deleteAgentModelConfig",
    call: () => deleteAgentModelConfig(config, getToken),
    method: "DELETE", url: `${BASE}/cfg%201`, searchParams: IN_PROJECT, returnsBody: false,
  },
  {
    name: "listLLMConfigAPIKeys",
    call: () => listLLMConfigAPIKeys(keyParams, getToken),
    method: "GET", url: KEYS, searchParams: IN_PROJECT,
  },
  {
    name: "createLLMConfigAPIKey",
    call: () => createLLMConfigAPIKey(keyParams, { name: "ci" } as never, getToken),
    method: "POST", url: KEYS, searchParams: IN_PROJECT, body: { name: "ci" },
  },
  {
    name: "rotateLLMConfigAPIKey",
    call: () => rotateLLMConfigAPIKey(keyParams, { expiresIn: "30d" } as never, getToken),
    method: "PUT", url: `${KEYS}/k%2F1`, searchParams: IN_PROJECT, body: { expiresIn: "30d" },
  },
  {
    name: "revokeLLMConfigAPIKey",
    call: () => revokeLLMConfigAPIKey(keyParams, getToken),
    method: "DELETE", url: `${KEYS}/k%2F1`, searchParams: IN_PROJECT, returnsBody: false,
  },
]);

describe("agent model config API inputs", () => {
  it.each([
    ["configId", () => getAgentModelConfig({ ...agent, configId: "" }, getToken)],
    ["projName", () => listAgentModelConfigs({ ...agent, projName: "" }, undefined, getToken)],
    ["keyName", () => revokeLLMConfigAPIKey({ ...keyParams, keyName: "" }, getToken)],
  ])("refuses a missing %s", async (label, call) => {
    await expect(call()).rejects.toThrow(`Missing required parameter: ${label}`);
  });
});
