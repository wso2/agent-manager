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
  bindingRequestFrom,
  bindingRequiresAPIKey,
  DEFAULT_API_KEY_HEADER,
  proxyRequestFrom,
  proxySourceLabel,
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
    expect(bindingRequiresAPIKey({ enabled: true, apiKey: { enabled: false } })).toBe(false);
  });
});

describe("bindingRequestFrom", () => {
  it("keeps every field the binding has, so a save changes only its own", () => {
    const request = bindingRequestFrom({
      ...base,
      gatewayId: "gw-1",
      releaseBindingName: "weather-dev",
      security: { enabled: true, apiKey: { enabled: true, key: "X-Weather-Key", in: "query" } },
      policies: [{ name: "basic-ratelimit", version: "v1", params: { limit: 10 } }],
      upstreamAuth: { type: "api-key", header: "Authorization" },
    });
    expect(request).toEqual({
      gatewayId: "gw-1",
      releaseBindingName: "weather-dev",
      security: { enabled: true, apiKey: { enabled: true, key: "X-Weather-Key", in: "query" } },
      policies: [{ name: "basic-ratelimit", version: "v1", params: { limit: 10 } }],
      upstreamAuth: { type: "api-key", header: "Authorization" },
    });
  });

  it("sends the upstream credential without its value, which keeps the stored one", () => {
    const request = bindingRequestFrom({
      ...base,
      upstreamAuth: { type: "api-key", header: "Authorization" },
    });
    expect(request.upstreamAuth).toEqual({ type: "api-key", header: "Authorization" });
    expect(request.upstreamAuth && "value" in request.upstreamAuth).toBe(false);
  });

  it("leaves out what the binding does not have", () => {
    expect(bindingRequestFrom(base)).toEqual({ policies: [] });
    expect(
      bindingRequestFrom({ ...base, upstreamAuth: { type: "none" } }).upstreamAuth,
    ).toBeUndefined();
  });

  it("keeps security off, and fills the default key name when on", () => {
    expect(bindingRequestFrom({ ...base, security: { enabled: false } }).security).toEqual({
      enabled: false,
    });
    expect(bindingRequestFrom({ ...base, security: { enabled: true } }).security).toEqual({
      enabled: true,
      apiKey: { enabled: true, key: DEFAULT_API_KEY_HEADER, in: "header" },
    });
  });
});

describe("proxySourceLabel", () => {
  it("names an org server external and a project server by its component endpoint", () => {
    expect(proxySourceLabel({ id: "github" })).toBe("External");
    expect(
      proxySourceLabel({
        id: "weather",
        projectName: "p1",
        source: { componentName: "weather", endpointName: "mcp" },
      }),
    ).toBe("weather / mcp");
    expect(proxySourceLabel({ id: "docs", projectName: "p1" })).toBe("External");
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
