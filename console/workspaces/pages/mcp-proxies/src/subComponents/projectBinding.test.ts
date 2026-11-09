/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { describe, expect, it } from "vitest";
import type { ProjectMCPProxyEnvironment } from "@agent-management-platform/types";
import {
  applyBindingConfigChange,
  bindingEndpointConfig,
  bindingRequestFrom,
  bindingRequiresAPIKey,
  DEFAULT_API_KEY_HEADER,
  proxyRequestFrom,
  toProjectProxyId,
} from "./projectBinding";

const base: ProjectMCPProxyEnvironment = {
  environmentName: "development",
  environmentUuid: "env-1",
  context: "/weather",
};

describe("bindingRequiresAPIKey", () => {
  it("defaults to an API key, as the service does", () => {
    expect(bindingRequiresAPIKey(undefined)).toBe(true);
    expect(bindingRequiresAPIKey({ enabled: true })).toBe(true);
  });

  it("is off when security or its API key is off", () => {
    expect(bindingRequiresAPIKey({ enabled: false })).toBe(false);
    expect(
      bindingRequiresAPIKey({ enabled: true, apiKey: { enabled: false } }),
    ).toBe(false);
  });
});

describe("bindingRequestFrom", () => {
  it("keeps every field the binding has, so a save changes only its own", () => {
    const request = bindingRequestFrom({
      ...base,
      gatewayId: "gw-1",
      releaseBindingName: "weather-dev",
      security: {
        enabled: true,
        apiKey: { enabled: true, key: "X-Weather-Key", in: "query" },
      },
      policies: [
        { name: "basic-ratelimit", version: "v1", params: { limit: 10 } },
      ],
      upstreamAuth: { type: "api-key", header: "Authorization" },
    });
    expect(request).toEqual({
      gatewayId: "gw-1",
      releaseBindingName: "weather-dev",
      security: {
        enabled: true,
        apiKey: { enabled: true, key: "X-Weather-Key", in: "query" },
      },
      policies: [
        { name: "basic-ratelimit", version: "v1", params: { limit: 10 } },
      ],
      upstreamAuth: { type: "api-key", header: "Authorization" },
    });
  });

  it("sends the upstream credential without its value, which keeps the stored one", () => {
    const request = bindingRequestFrom({
      ...base,
      upstreamAuth: { type: "api-key", header: "Authorization" },
    });
    expect(request.upstreamAuth).toEqual({
      type: "api-key",
      header: "Authorization",
    });
    expect(request.upstreamAuth && "value" in request.upstreamAuth).toBe(false);
  });

  it("leaves out what the binding does not have", () => {
    expect(bindingRequestFrom(base)).toEqual({ policies: [] });
    expect(
      bindingRequestFrom({ ...base, upstreamAuth: { type: "none" } })
        .upstreamAuth,
    ).toBeUndefined();
  });

  it("keeps security off, and fills the default key name when on", () => {
    expect(
      bindingRequestFrom({ ...base, security: { enabled: false } }).security,
    ).toEqual({
      enabled: false,
    });
    expect(
      bindingRequestFrom({ ...base, security: { enabled: true } }).security,
    ).toEqual({
      enabled: true,
      apiKey: { enabled: true, key: DEFAULT_API_KEY_HEADER, in: "header" },
    });
  });
});

describe("toProjectProxyId", () => {
  it("makes a kebab-case id the service takes", () => {
    expect(toProjectProxyId("  Docs MCP Server! ")).toBe("docs-mcp-server");
    expect(toProjectProxyId("a".repeat(120))).toHaveLength(100);
    expect(toProjectProxyId("***")).toBe("");
  });
});

describe("proxyRequestFrom", () => {
  it("keeps what a URL proxy has, with its upstream and no source", () => {
    expect(
      proxyRequestFrom({
        id: "docs",
        name: "Docs",
        version: "v1",
        context: "/docs",
        description: "d",
        projectName: "p1",
        upstream: { url: "https://mcp.example.com/mcp" },
        environments: [],
      }),
    ).toEqual({
      name: "Docs",
      version: "v1",
      context: "/docs",
      description: "d",
      upstream: { url: "https://mcp.example.com/mcp" },
    });
  });
});

describe("binding as an org endpoint config", () => {
  const urlProxy = {
    id: "docs",
    name: "Docs",
    version: "v1",
    context: "/docs",
    projectName: "p1",
    upstream: { url: "https://mcp.example.com/mcp" },
    environments: [],
  };
  const env = {
    ...base,
    upstreamAuth: { type: "api-key" as const, header: "Authorization" },
    policies: [{ name: "basic-ratelimit", version: "v1" }],
    capabilities: { tools: [{ name: "search" }] },
  };

  it("shows the proxy's URL with the binding's credential, tools and policies", () => {
    const config = bindingEndpointConfig(urlProxy, env);
    expect(config.upstream?.main).toEqual({
      url: "https://mcp.example.com/mcp",
      auth: { type: "api-key", header: "Authorization" },
    });
    expect(config.capabilities).toEqual({ tools: [{ name: "search" }] });
    expect(config.policies).toEqual([
      { name: "basic-ratelimit", version: "v1" },
    ]);
  });

  it("keeps the tools when a tab changes only the policies", () => {
    const { binding, proxyURL } = applyBindingConfigChange(urlProxy, env, {
      policies: [],
    });
    expect(binding.policies).toEqual([]);
    expect(binding.capabilities).toEqual({ tools: [{ name: "search" }] });
    expect(binding.upstreamAuth).toEqual({
      type: "api-key",
      header: "Authorization",
    });
    expect(proxyURL).toBeUndefined();
  });

  it("sends a new credential value, and a changed URL to the proxy", () => {
    const { binding, proxyURL } = applyBindingConfigChange(urlProxy, env, {
      upstream: {
        main: {
          url: "https://mcp2.example.com/mcp",
          auth: { type: "api-key", header: "X-Key", value: "s3cret" },
        },
      },
    });
    expect(binding.upstreamAuth).toEqual({
      type: "api-key",
      header: "X-Key",
      value: "s3cret",
    });
    expect(proxyURL).toBe("https://mcp2.example.com/mcp");
  });

  it("clears the credential when the tab removes it, and never moves a sourced proxy's URL", () => {
    const sourced = {
      ...urlProxy,
      upstream: undefined,
      source: { componentName: "c", endpointName: "e" },
    };
    const { binding, proxyURL } = applyBindingConfigChange(sourced, env, {
      upstream: { main: { url: "https://elsewhere/mcp" } },
    });
    expect(binding.upstreamAuth).toBeUndefined();
    expect(proxyURL).toBeUndefined();
  });
});
