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

import { describe, expect, it } from "vitest";
import {
  agentConfigDocumentFromCreateRequest,
  agentConfigDocumentFromUpdateRequest,
  agentConfigFromDocument,
  agentConfigListFromDocuments,
} from "./agent-config-documents";

const policies = [{ name: "retry", version: "v1", paths: [] }];
const modelDoc = {
  apiVersion: "ai-control-plane.choreolabs.com/v1",
  kind: "AgentModelConfig" as const,
  metadata: { name: "chat", project: "p1", uid: "u1", creationTimestamp: "2026-01-01T00:00:00Z" },
  spec: { description: "d", providerName: "openai", configuration: { policies }, environmentVariables: [{ key: "url", name: "URL" }] },
  status: {
    url: "https://gw/x", proxyUuid: "px", proxyName: "chat-proxy", providerUuid: "pv",
    authInfo: { type: "api-key", in: "header", name: "X-Key" },
  },
};

describe("agentConfigDocumentFromCreateRequest", () => {
  it("binds the default environment's provider", () => {
    const doc = agentConfigDocumentFromCreateRequest("AgentModelConfig", "p1", {
      name: "chat", type: "llm", description: "d",
      envMappings: { dev: { providerName: "other", configuration: {} }, default: { providerName: "openai", configuration: { policies } } },
    } as never);
    expect(doc).toEqual({
      apiVersion: "ai-control-plane.choreolabs.com/v1",
      kind: "AgentModelConfig",
      metadata: { name: "chat", project: "p1" },
      spec: { description: "d", providerName: "openai", configuration: { policies }, environmentVariables: undefined },
    });
  });

  it.each([
    [{ proxyName: "a" }], [{ proxyId: "a" }], [{ mcpProxyName: "a" }], [{ mcpProxyId: "a" }], [{ providerName: "a" }],
  ])("names an MCP config's proxy from %j", (mapping) => {
    const doc = agentConfigDocumentFromCreateRequest("AgentMcpConfig", "p1", {
      name: "tools", type: "mcp", envMappings: { only: { ...mapping, configuration: {} } },
    } as never);
    expect(doc.spec.proxyName).toBe("a");
    expect(doc.spec.providerName).toBeUndefined();
  });
});

describe("agentConfigDocumentFromUpdateRequest", () => {
  it("keeps what the update omits", () => {
    const doc = agentConfigDocumentFromUpdateRequest("AgentModelConfig", "p1", { description: "new" }, modelDoc);
    expect(doc.metadata).toEqual({ name: "chat", project: "p1" });
    expect(doc.spec).toEqual({ ...modelDoc.spec, description: "new" });
    expect(doc).not.toHaveProperty("status");
  });

  it("renames and re-points the provider", () => {
    const doc = agentConfigDocumentFromUpdateRequest("AgentModelConfig", "p1", {
      name: "chat2", envMappings: { default: { providerName: "anthropic", configuration: {} } },
    } as never, modelDoc);
    expect(doc.metadata.name).toBe("chat2");
    expect(doc.spec.providerName).toBe("anthropic");
    expect(doc.spec.configuration).toEqual({});
  });
});

describe("agentConfigFromDocument", () => {
  it("maps the document to the default environment", () => {
    const c = agentConfigFromDocument(modelDoc, "bot");
    expect(c).toMatchObject({ uuid: "u1", name: "chat", agentId: "bot", type: "llm", projectName: "p1", description: "d" });
    expect(c.envMappings.default.configuration).toMatchObject({
      providerName: "openai", providerUuid: "pv", proxyUuid: "px", proxyName: "chat-proxy", url: "https://gw/x",
      authInfo: { type: "api-key", in: "header", name: "X-Key" }, policies,
    });
  });

  it("names an MCP config's proxy under every key the pages read", () => {
    const c = agentConfigFromDocument({ ...modelDoc, kind: "AgentMcpConfig", spec: { proxyName: "tools" } }, "bot");
    expect(c.type).toBe("mcp");
    expect(c.envMappings.default.configuration).toMatchObject({
      providerName: "tools", proxyName: "tools", proxyId: "tools", mcpProxyName: "tools", mcpProxyId: "tools",
    });
  });

  it("renders a page as list items", () => {
    const page = agentConfigListFromDocuments({ count: 1, list: [modelDoc], pagination: { count: 1, offset: 0, limit: 20 } }, "bot");
    expect(page.pagination).toEqual({ count: 1, offset: 0, limit: 20 });
    expect(page.configs).toEqual([expect.objectContaining({ uuid: "u1", name: "chat", type: "llm", agentId: "bot" })]);
  });
});
