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
import { createAgentMCPConfig, listAgentMCPConfigs } from "../../agent-runtime/apis/agent-mcp-configs";
import { addAgentMCPProxy, listAgentMCPProxies } from "./agent-mcp-proxies";

vi.mock("../../agent-runtime/apis/agent-mcp-configs", () => ({
  createAgentMCPConfig: vi.fn(),
  listAgentMCPConfigs: vi.fn(),
}));

const agent = { orgName: "acme", projName: "p", agentName: "bot" };
const getToken = async () => "jwt";

// mcpDoc is an MCP config document as the server returns it.
function mcpDoc(uid: string, name: string, spec: object, status?: object) {
  return {
    apiVersion: "ai-control-plane.choreolabs.com/v1", kind: "AgentMcpConfig",
    metadata: { name, project: "p", uid, creationTimestamp: "2026-11-01" }, spec, status,
  };
}

beforeEach(() => vi.clearAllMocks());

describe("listAgentMCPProxies", () => {
  it("flattens each attached config into a proxy row", async () => {
    vi.mocked(listAgentMCPConfigs).mockResolvedValue({
      count: 3,
      pagination: { count: 3, offset: 0, limit: 100 },
      list: [
        mcpDoc("c1", "orders", { description: "Orders MCP", proxyName: "orders-mcp", environmentVariables: [{ key: "K", name: "K" }] },
          { url: "https://gw.example/orders/mcp?x=1" }),
        mcpDoc("c2", "raw", { proxyName: "raw-mcp" }, { url: "not a url" }),
        mcpDoc("c3", "empty", {}),
      ],
    } as never);

    const res = await listAgentMCPProxies(agent, getToken);

    expect(listAgentMCPConfigs).toHaveBeenCalledWith(agent, { limit: 100, offset: 0 }, getToken);
    expect(res.count).toBe(3);
    expect(res.list[0]).toMatchObject({
      id: "c1", name: "orders", description: "Orders MCP", proxyId: "orders-mcp", proxyName: "orders-mcp",
      proxyUrl: "https://gw.example/orders/mcp?x=1", context: "/orders/mcp", status: "active",
      environmentVariables: [{ key: "K", name: "K" }], createdAt: "2026-11-01",
    });
    // An unparsable URL is kept as the context rather than dropped.
    expect(res.list[1]).toMatchObject({ proxyId: "raw-mcp", context: "not a url", status: "active" });
    // A config with nothing in the default environment has no proxy and no status.
    expect(res.list[2]).toMatchObject({ id: "c3", proxyId: undefined, context: undefined, status: undefined });
  });

  it("propagates a failure to list the configs", async () => {
    vi.mocked(listAgentMCPConfigs).mockRejectedValue({ message: "forbidden" });
    await expect(listAgentMCPProxies(agent, getToken)).rejects.toEqual({ message: "forbidden" });
  });
});

describe("addAgentMCPProxy", () => {
  it("binds the proxy as an MCP config document", async () => {
    vi.mocked(createAgentMCPConfig).mockResolvedValue(mcpDoc("new", "orders-mcp", { proxyName: "orders-mcp" }) as never);
    const vars = [{ key: "TOKEN", name: "TOKEN" }];

    const res = await addAgentMCPProxy(agent, { proxyId: "orders-mcp", environmentVariables: vars } as never, getToken);

    expect(createAgentMCPConfig).toHaveBeenCalledWith(agent, {
      apiVersion: "ai-control-plane.choreolabs.com/v1",
      kind: "AgentMcpConfig",
      metadata: { name: "orders-mcp", project: "p" },
      spec: { description: undefined, proxyName: "orders-mcp", configuration: {}, environmentVariables: vars },
    }, getToken);
    expect(res).toMatchObject({ uuid: "new", type: "mcp" });
  });
});
