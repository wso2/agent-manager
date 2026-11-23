/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { describe, expect, it } from "vitest";
import type { MCPProxyEndpoint, MCPProxyPolicy } from "@agent-management-platform/types";
import type { EndpointDraft } from "./EndpointFormFields";
import {
  DEFAULT_ENDPOINT_SECURITY,
  deriveEndpointHandle,
  draftToEndpoint,
  endpointToDraft,
  prunePoliciesForCapabilities,
} from "./mcpEndpoints";

const draft = (overrides: Partial<EndpointDraft> = {}): EndpointDraft => ({
  id: "tmp-1",
  name: "",
  url: "https://mcp.example.com/mcp",
  authHeader: "",
  authValue: "",
  resilienceTimeout: "",
  resilienceIdleTimeout: "",
  environments: ["env-dev"],
  fetchedInfo: { tools: [{ name: "search" }], resources: [], prompts: [] },
  ...overrides,
});

describe("deriveEndpointHandle", () => {
  it("prefers the name, then the server name, then the URL host", () => {
    expect(deriveEndpointHandle({ name: "Primary EU", url: "", serverName: "x" }, 0)).toBe(
      "primary-eu",
    );
    expect(deriveEndpointHandle({ name: "", url: "", serverName: "GitHub MCP" }, 0)).toBe(
      "github-mcp",
    );
    expect(
      deriveEndpointHandle({ name: "", url: "https://api.example.com/mcp" }, 0),
    ).toBe("api-example-com");
    expect(deriveEndpointHandle({ name: "", url: "not a url" }, 2)).toBe("endpoint-3");
  });

  it("caps the handle at 30 characters without a trailing hyphen", () => {
    const handle = deriveEndpointHandle({ name: `${"a".repeat(29)} b`, url: "" }, 0);
    expect(handle).toBe("a".repeat(29));
  });

  it("de-duplicates against handles already used in the batch", () => {
    const used = new Set<string>();
    expect(deriveEndpointHandle({ name: "Primary", url: "" }, 0, used)).toBe("primary");
    expect(deriveEndpointHandle({ name: "Primary", url: "" }, 1, used)).toBe("primary-2");
    expect(deriveEndpointHandle({ name: "Primary", url: "" }, 2, used)).toBe("primary-3");

    const long = "b".repeat(30);
    used.add(long);
    const suffixed = deriveEndpointHandle({ name: long, url: "" }, 3, used);
    expect(suffixed).toBe(`${"b".repeat(28)}-2`);
    expect(suffixed.length).toBeLessThanOrEqual(30);
  });
});

describe("draftToEndpoint", () => {
  it("builds a new endpoint with default security and per-environment gateways", () => {
    const endpoint = draftToEndpoint(
      draft({
        name: " Primary ",
        authHeader: " Authorization ",
        authValue: " secret ",
        resilienceTimeout: " 5s ",
        gatewayByEnv: { "env-dev": "gw-1" },
        fetchedInfo: {
          tools: [{ name: "search" }],
          resources: [],
          prompts: [],
          supportedVersions: ["2025-06-18"],
        },
      }),
      0,
    );

    expect(endpoint).toEqual({
      id: "primary",
      name: "Primary",
      upstream: {
        main: {
          url: "https://mcp.example.com/mcp",
          auth: { type: "api-key", header: "Authorization", value: "secret" },
        },
      },
      resilience: { timeout: "5s", idleTimeout: undefined },
      capabilities: { tools: [{ name: "search" }], resources: [], prompts: [] },
      mcpSpecVersions: ["2025-06-18"],
      policies: undefined,
      security: DEFAULT_ENDPOINT_SECURITY,
      environments: [{ environmentUuid: "env-dev", gatewayId: "gw-1" }],
    });
  });

  it("keeps an existing endpoint's handle, security and stored credential", () => {
    const existing = {
      id: "legacy",
      upstream: { main: { url: "https://old.example.com" }, backup: { url: "x" } },
      security: { enabled: false },
      policies: [{ name: "rate-limit", version: "v1", params: {} }],
    } as unknown as MCPProxyEndpoint;
    const used = new Set<string>();

    const endpoint = draftToEndpoint(
      draft({ name: "Legacy", authHeader: "X-Key", authValue: "" }),
      0,
      existing,
      used,
    );

    expect(endpoint.id).toBe("legacy");
    expect(used.has("legacy")).toBe(true);
    expect(endpoint.upstream).toEqual({
      main: { url: "https://mcp.example.com/mcp", auth: { type: "api-key", header: "X-Key" } },
      backup: { url: "x" },
    });
    expect(endpoint.security).toEqual({ enabled: false });
    expect(endpoint.policies).toEqual(existing.policies);
    expect(endpoint.resilience).toBeUndefined();
    expect(endpoint.mcpSpecVersions).toBeUndefined();
  });
});

describe("endpointToDraft", () => {
  it("round-trips a saved endpoint into an editable draft", () => {
    const result = endpointToDraft({
      id: "primary",
      name: "Primary",
      upstream: { main: { url: "https://mcp.example.com", auth: { header: "X-Key" } } },
      resilience: { timeout: "5s", idleTimeout: "30s" },
      capabilities: { tools: [{ name: "search" }] },
      mcpSpecVersions: ["2025-06-18"],
      environments: [
        { environmentUuid: "env-dev", gatewayId: "gw-1" },
        { environmentUuid: "env-prod" },
      ],
    } as unknown as MCPProxyEndpoint);

    expect(result).toEqual({
      id: "primary",
      name: "Primary",
      url: "https://mcp.example.com",
      authHeader: "X-Key",
      authValue: "",
      resilienceTimeout: "5s",
      resilienceIdleTimeout: "30s",
      environments: ["env-dev", "env-prod"],
      gatewayByEnv: { "env-dev": "gw-1" },
      fetchedInfo: {
        tools: [{ name: "search" }],
        resources: [],
        prompts: [],
        supportedVersions: ["2025-06-18"],
      },
    });
  });

  it("fills blanks for a sparse endpoint", () => {
    const result = endpointToDraft({ id: "bare" } as MCPProxyEndpoint);

    expect(result).toMatchObject({
      name: "",
      url: "",
      authHeader: "",
      environments: [],
      gatewayByEnv: {},
      fetchedInfo: { tools: [], resources: [], prompts: [] },
    });
  });
});

describe("prunePoliciesForCapabilities", () => {
  const capabilities = {
    tools: [{ name: "search" }],
    resources: [{ uri: "file:///readme" }],
    prompts: [],
  };

  it("drops rewrite entries and ACL exceptions for capabilities that are gone", () => {
    const policies: MCPProxyPolicy[] = [
      { name: "rate-limit", version: "v1", params: { rps: 5 } },
      {
        name: "mcp-rewrite",
        version: "v1",
        params: {
          tools: [
            { name: "find", target: "search" },
            { name: "gone", target: "removed_tool" },
          ],
          resources: [{ uri: "file:///readme" }],
          prompts: [{ name: "old" }],
        },
      },
      {
        name: "mcp-acl-list",
        version: "v1",
        params: {
          tools: { mode: "deny", exceptions: ["search", "removed_tool", 3] },
          prompts: { mode: "deny", exceptions: ["old"] },
        },
      },
    ];

    expect(prunePoliciesForCapabilities(policies, capabilities)).toEqual([
      { name: "rate-limit", version: "v1", params: { rps: 5 } },
      {
        name: "mcp-rewrite",
        version: "v1",
        params: {
          tools: [{ name: "find", target: "search" }],
          resources: [{ uri: "file:///readme" }],
        },
      },
      {
        name: "mcp-acl-list",
        version: "v1",
        params: { tools: { mode: "deny", exceptions: ["search"] } },
      },
    ]);
  });

  it("removes policies left with nothing to apply to", () => {
    const policies: MCPProxyPolicy[] = [
      { name: "mcp-rewrite", version: "v1", params: { tools: [{ name: "gone" }] } },
      { name: "mcp-acl-list", version: "v1", params: { prompts: { mode: "deny" } } },
    ];

    expect(prunePoliciesForCapabilities(policies, capabilities)).toBeUndefined();
    expect(prunePoliciesForCapabilities([], capabilities)).toEqual([]);
    expect(prunePoliciesForCapabilities(undefined, capabilities)).toBeUndefined();
  });
});
