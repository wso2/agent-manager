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

import { beforeEach, describe, expect, it, vi } from "vitest";
import { listEnvironments } from "../../agent-runtime/apis/deployments";
import {
  createAgentMCPConfig, getAgentMCPConfig, listAgentMCPConfigs,
} from "../../agent-runtime/apis/agent-mcp-configs";
import { addAgentMCPProxy, listAgentMCPProxies } from "./agent-mcp-proxies";

vi.mock("../../agent-runtime/apis/deployments", () => ({ listEnvironments: vi.fn() }));
vi.mock("../../agent-runtime/apis/agent-mcp-configs", () => ({
  createAgentMCPConfig: vi.fn(),
  getAgentMCPConfig: vi.fn(),
  listAgentMCPConfigs: vi.fn(),
}));

const agent = { orgName: "acme", projName: "p", agentName: "bot" };
const getToken = async () => "jwt";

beforeEach(() => vi.clearAllMocks());

describe("listAgentMCPProxies", () => {
  it("reads every attached config in full and flattens it into a proxy row", async () => {
    vi.mocked(listAgentMCPConfigs).mockResolvedValue({ configs: [{ uuid: "c1" }, { uuid: "c2" }, { uuid: "c3" }] } as never);
    const details: Record<string, unknown> = {
      c1: {
        uuid: "c1", name: "orders", description: "Orders MCP", createdAt: "2026-11-01",
        environmentVariables: [{ key: "K" }],
        envMappings: { dev: { configuration: { mcpProxyName: "orders-mcp", url: "https://gw.example/orders/mcp?x=1" } } },
      },
      c2: { uuid: "c2", name: "raw", envMappings: { dev: { configuration: { providerName: "raw-mcp", url: "not a url" } } } },
      c3: { uuid: "c3", name: "empty", envMappings: {} },
    };
    vi.mocked(getAgentMCPConfig).mockImplementation(
      async (p) => details[(p as { configId: string }).configId] as never,
    );

    const res = await listAgentMCPProxies(agent, getToken);

    expect(listAgentMCPConfigs).toHaveBeenCalledWith(agent, { limit: 1000, offset: 0 }, getToken);
    expect(getAgentMCPConfig).toHaveBeenCalledWith({ ...agent, configId: "c2" }, getToken);
    expect(res.count).toBe(3);
    expect(res.list[0]).toMatchObject({
      id: "c1", name: "orders", description: "Orders MCP", proxyId: "orders-mcp", proxyName: "orders-mcp",
      proxyUrl: "https://gw.example/orders/mcp?x=1", context: "/orders/mcp", status: "active",
      environmentVariables: [{ key: "K" }], createdAt: "2026-11-01",
    });
    // An unparsable URL is kept as the context rather than dropped.
    expect(res.list[1]).toMatchObject({ proxyId: "raw-mcp", context: "not a url", status: "active" });
    // A config with no environment mapping has no proxy and no status.
    expect(res.list[2]).toMatchObject({ id: "c3", proxyId: undefined, context: undefined, status: undefined });
  });

  it("propagates a failure to list the configs", async () => {
    vi.mocked(listAgentMCPConfigs).mockRejectedValue({ message: "forbidden" });
    await expect(listAgentMCPProxies(agent, getToken)).rejects.toEqual({ message: "forbidden" });
    expect(getAgentMCPConfig).not.toHaveBeenCalled();
  });
});

describe("addAgentMCPProxy", () => {
  it("binds the proxy in every environment of the org", async () => {
    vi.mocked(listEnvironments).mockResolvedValue([{ name: "dev" }, { name: "prod" }] as never);
    vi.mocked(createAgentMCPConfig).mockResolvedValue({ uuid: "new" } as never);
    const vars = [{ key: "TOKEN", value: "x" }];

    const res = await addAgentMCPProxy(agent, { proxyId: "orders-mcp", environmentVariables: vars } as never, getToken);

    expect(listEnvironments).toHaveBeenCalledWith({ orgName: "acme" }, getToken);
    expect(createAgentMCPConfig).toHaveBeenCalledWith(agent, {
      name: "orders-mcp",
      type: "mcp",
      envMappings: {
        dev: { proxyId: "orders-mcp", configuration: {} },
        prod: { proxyId: "orders-mcp", configuration: {} },
      },
      environmentVariables: vars,
    }, getToken);
    expect(res).toEqual({ uuid: "new" });
  });

  it("does not create a config when the environments cannot be read", async () => {
    vi.mocked(listEnvironments).mockRejectedValue(new Error("down"));
    await expect(addAgentMCPProxy(agent, { proxyId: "x" } as never, getToken)).rejects.toThrow("down");
    expect(createAgentMCPConfig).not.toHaveBeenCalled();
  });
});
