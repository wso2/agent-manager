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
  createAgentModelConfig,
  createLLMConfigAPIKey,
  deleteAgentModelConfig,
  getAgentModelConfig,
  listAgentModelConfigs,
  listLLMConfigAPIKeys,
  normalizeAgentModelConfigResponse,
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
const BASE = "/m/ai-gateway/orgs/acme%20corp/projects/p%2F1/agents/bot/model-configs";
const KEYS = `${BASE}/cfg%201/environments/dev/api-keys`;
const body = { name: "gpt", envMappings: { dev: { providerName: "openai" } } };
const normalized = { uuid: "u1", name: "gpt", envMappings: { dev: { environmentName: "dev" } } };

describeHttpCases("agent model config API", mocks, [
  {
    name: "listAgentModelConfigs with paging",
    call: () => listAgentModelConfigs(agent, { limit: 10, offset: 30 }, getToken),
    method: "GET", url: BASE, searchParams: { limit: "10", offset: "30" },
  },
  {
    name: "listAgentModelConfigs without paging",
    call: () => listAgentModelConfigs(agent, undefined, getToken),
    method: "GET", url: BASE, searchParams: {},
  },
  {
    name: "createAgentModelConfig",
    call: () => createAgentModelConfig(agent, body as never, getToken),
    method: "POST", url: BASE, body, response: normalized,
  },
  {
    name: "getAgentModelConfig",
    call: () => getAgentModelConfig(config, getToken),
    method: "GET", url: `${BASE}/cfg%201`, response: normalized,
  },
  {
    name: "updateAgentModelConfig",
    call: () => updateAgentModelConfig(config, body as never, getToken),
    method: "PUT", url: `${BASE}/cfg%201`, body, response: normalized,
  },
  {
    name: "deleteAgentModelConfig",
    call: () => deleteAgentModelConfig(config, getToken),
    method: "DELETE", url: `${BASE}/cfg%201`, returnsBody: false,
  },
  {
    name: "listLLMConfigAPIKeys",
    call: () => listLLMConfigAPIKeys(keyParams, getToken),
    method: "GET", url: KEYS,
  },
  {
    name: "createLLMConfigAPIKey",
    call: () => createLLMConfigAPIKey(keyParams, { name: "ci" } as never, getToken),
    method: "POST", url: KEYS, body: { name: "ci" },
  },
  {
    name: "rotateLLMConfigAPIKey",
    call: () => rotateLLMConfigAPIKey(keyParams, { expiresIn: "30d" } as never, getToken),
    method: "PUT", url: `${KEYS}/k%2F1`, body: { expiresIn: "30d" },
  },
  {
    name: "revokeLLMConfigAPIKey",
    call: () => revokeLLMConfigAPIKey(keyParams, getToken),
    method: "DELETE", url: `${KEYS}/k%2F1`, returnsBody: false,
  },
]);

describe("agent model config API inputs", () => {
  it("sends a copy of the body, never the caller's object", async () => {
    mocks.POST.mockReset().mockResolvedValue(new Response(JSON.stringify(normalized)));
    await createAgentModelConfig(agent, body as never, getToken);
    expect(mocks.POST.mock.calls[0][1]).toEqual(body);
    expect(mocks.POST.mock.calls[0][1]).not.toBe(body);
  });

  it.each([
    ["configId", () => getAgentModelConfig({ ...agent, configId: "" }, getToken)],
    ["envName", () => listLLMConfigAPIKeys({ ...config, envName: "" }, getToken)],
    ["keyName", () => revokeLLMConfigAPIKey({ ...keyParams, keyName: "" }, getToken)],
  ])("refuses a missing %s", async (label, call) => {
    await expect(call()).rejects.toThrow(`Missing required parameter: ${label}`);
  });
});

describe("normalizeAgentModelConfigResponse", () => {
  it("returns a response that already has envMappings unchanged", () => {
    const raw = { uuid: "u", envMappings: { dev: { environmentName: "dev" } } };
    expect(normalizeAgentModelConfigResponse(raw as never)).toBe(raw);
  });

  it("builds envMappings from the legacy llmProxy shape", () => {
    const raw = {
      uuid: "u",
      envModelConfig: {
        dev: {
          environmentName: "Development",
          llmProxy: {
            proxyUrl: "https://gw/openai", proxyUuid: "px", proxyName: "openai-proxy",
            providerName: "openai", policies: [{ name: "pii" }],
            apiKey: "secret", authHeaderName: "X-API-Key",
          },
        },
        prod: {
          environmentName: "",
          llmProxy: { proxyUrl: "https://gw/p", authHeaderName: "Authorization", authIn: "query" },
        },
      },
    };
    const out = normalizeAgentModelConfigResponse(raw as never);
    expect(out.envMappings.dev).toEqual({
      environmentName: "Development",
      configuration: {
        providerName: "openai", proxyUuid: "px", proxyName: "openai-proxy", url: "https://gw/openai",
        authInfo: { type: "apikey", in: "header", name: "X-API-Key", value: "secret" },
        policies: [{ name: "pii" }],
      },
    });
    // A non-header authIn is kept.
    expect(out.envMappings.prod.configuration?.authInfo).toEqual({
      type: "apikey", in: "query", name: "Authorization", value: undefined,
    });
  });

  it("leaves authInfo unset when the proxy reports no auth header", () => {
    const raw = { envModelConfig: { dev: { environmentName: "dev", llmProxy: { proxyUrl: "u" } } } };
    expect(normalizeAgentModelConfigResponse(raw as never).envMappings.dev.configuration?.authInfo)
      .toBeUndefined();
  });

  it("prefers an explicit configuration and tolerates a missing proxy", () => {
    const raw = {
      envModelConfig: {
        a: { environmentName: undefined, configuration: { url: "explicit" }, llmProxy: { proxyUrl: "ignored" } },
        b: { environmentName: "b" },
      },
    };
    const out = normalizeAgentModelConfigResponse(raw as never);
    expect(out.envMappings.a).toEqual({ environmentName: "a", configuration: { url: "explicit" } });
    expect(out.envMappings.b).toEqual({ environmentName: "b", configuration: undefined });
  });

  it("yields empty envMappings when the response has neither shape", () => {
    expect(normalizeAgentModelConfigResponse({ uuid: "u" } as never).envMappings).toEqual({});
  });
});
