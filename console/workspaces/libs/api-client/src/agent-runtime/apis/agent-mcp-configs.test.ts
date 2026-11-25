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
import { describeHttpCases, getToken, jsonResponse, type HttpMocks } from "../../test-utils/http-cases";
import {
  createAgentMCPConfig,
  createMCPConfigAPIKey,
  deleteAgentMCPConfig,
  getAgentMCPConfig,
  listAgentMCPConfigs,
  listMCPConfigAPIKeys,
  revokeMCPConfigAPIKey,
  rotateMCPConfigAPIKey,
  updateAgentMCPConfig,
} from "./agent-mcp-configs";

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
const BASE = "/m/ai-gateway/orgs/acme%20corp/projects/p%2F1/agents/bot/mcp-configs";
const KEYS = `${BASE}/cfg%201/environments/dev/api-keys`;

// A response already in envMappings form whose proxy is named only by proxyId.
const served = {
  uuid: "u1",
  name: "orders",
  envMappings: {
    dev: { environmentName: "dev", configuration: { proxyId: "orders-mcp", url: "https://gw/mcp" } },
    prod: { environmentName: "prod" },
  },
};
const normalized = {
  ...served,
  envMappings: {
    dev: {
      environmentName: "dev",
      configuration: {
        proxyId: "orders-mcp", url: "https://gw/mcp",
        proxyName: "orders-mcp", mcpProxyName: "orders-mcp", mcpProxyId: "orders-mcp",
      },
    },
    prod: { environmentName: "prod", configuration: undefined },
  },
};

describeHttpCases("agent MCP config API", mocks, [
  {
    name: "listAgentMCPConfigs tags every config as mcp",
    call: () => listAgentMCPConfigs(agent, { limit: 5, offset: 0 }, getToken),
    method: "GET", url: BASE, searchParams: { limit: "5", offset: "0" },
    response: { configs: [{ uuid: "a" }, { uuid: "b", type: "llm" }], total: 2 },
    expected: { configs: [{ uuid: "a", type: "mcp" }, { uuid: "b", type: "mcp" }], total: 2 },
  },
  {
    name: "listAgentMCPConfigs without paging",
    call: () => listAgentMCPConfigs(agent, undefined, getToken),
    method: "GET", url: BASE, searchParams: {}, response: { configs: [] }, expected: { configs: [] },
  },
  {
    name: "createAgentMCPConfig",
    call: () => createAgentMCPConfig(agent, { name: "orders" } as never, getToken),
    method: "POST", url: BASE, body: { name: "orders", type: "mcp" },
    response: served, expected: normalized,
  },
  {
    name: "getAgentMCPConfig",
    call: () => getAgentMCPConfig(config, getToken),
    method: "GET", url: `${BASE}/cfg%201`, response: served, expected: normalized,
  },
  {
    name: "updateAgentMCPConfig",
    call: () => updateAgentMCPConfig(config, { description: "d" } as never, getToken),
    method: "PUT", url: `${BASE}/cfg%201`, body: { description: "d" },
    response: served, expected: normalized,
  },
  {
    name: "deleteAgentMCPConfig",
    call: () => deleteAgentMCPConfig(config, getToken),
    method: "DELETE", url: `${BASE}/cfg%201`, returnsBody: false,
  },
  {
    name: "listMCPConfigAPIKeys",
    call: () => listMCPConfigAPIKeys(keyParams, getToken),
    method: "GET", url: KEYS,
  },
  {
    name: "createMCPConfigAPIKey",
    call: () => createMCPConfigAPIKey(keyParams, { name: "ci" } as never, getToken),
    method: "POST", url: KEYS, body: { name: "ci" },
  },
  {
    name: "rotateMCPConfigAPIKey",
    call: () => rotateMCPConfigAPIKey(keyParams, { expiresIn: "7d" } as never, getToken),
    method: "PUT", url: `${KEYS}/k%2F1`, body: { expiresIn: "7d" },
  },
  {
    name: "revokeMCPConfigAPIKey",
    call: () => revokeMCPConfigAPIKey(keyParams, getToken),
    method: "DELETE", url: `${KEYS}/k%2F1`, returnsBody: false,
  },
]);

describe("MCP config request bodies", () => {
  it.each([
    [{ proxyName: "a", proxyId: "b" }, "a"],
    [{ proxyId: "b", mcpProxyName: "c" }, "b"],
    [{ mcpProxyName: "c", mcpProxyId: "d" }, "c"],
    [{ mcpProxyId: "d", providerName: "e" }, "d"],
    [{ providerName: "e" }, "e"],
  ])("names the provider from the first proxy field set in %j", async (mapping, expected) => {
    mocks.POST.mockReset().mockResolvedValue(jsonResponse(served));
    const body = { name: "orders", envMappings: { dev: mapping } };
    await createAgentMCPConfig(agent, body as never, getToken);
    const sent = mocks.POST.mock.calls[0][1] as typeof body & { type: string };
    expect(sent.type).toBe("mcp");
    expect(sent.envMappings.dev).toEqual({ ...mapping, providerName: expected });
    // The caller's body is left untouched.
    expect(body.envMappings.dev).toEqual(mapping);
  });

  it("maps the proxy name on update without forcing the type", async () => {
    mocks.PUT.mockReset().mockResolvedValue(jsonResponse(served));
    await updateAgentMCPConfig(config, { envMappings: { dev: { proxyId: "x" } } } as never, getToken);
    expect(mocks.PUT.mock.calls[0][1]).toEqual({ envMappings: { dev: { proxyId: "x", providerName: "x" } } });
  });

  it("normalizes a legacy envModelConfig response into envMappings", async () => {
    mocks.GET.mockReset().mockResolvedValue(jsonResponse({
      uuid: "u", envModelConfig: { dev: { environmentName: "dev", configuration: { providerName: "p" } } },
    }));
    const res = await getAgentMCPConfig(config, getToken);
    expect(res.envMappings.dev.configuration).toMatchObject({
      providerName: "p", proxyName: "p", proxyId: "p", mcpProxyName: "p", mcpProxyId: "p",
    });
  });

  it("refuses a missing configId before any request", async () => {
    mocks.GET.mockReset();
    await expect(getAgentMCPConfig({ ...agent, configId: "" }, getToken)).rejects.toThrow("configId");
    expect(mocks.GET).not.toHaveBeenCalled();
  });
});
