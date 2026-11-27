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

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const modules = vi.hoisted(() => ({ installed: new Set(["ai-gateway"]) }));
vi.mock("@agent-management-platform/views", () => ({
  useSnackBar: () => ({ pushSnackBar: vi.fn() }),
  useIsUIModuleInstalled: (id: string) => modules.installed.has(id),
}));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: async () => "t", isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("../../acp-core/hooks/telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/mcp-proxies", () => ({ listAvailableMCPPolicies: vi.fn() }));

import { globalConfig } from "@agent-management-platform/types";
import type { UseQueryResult } from "@tanstack/react-query";
import { listAvailableMCPPolicies } from "../apis/mcp-proxies";
import { cleanupHooks, flush, renderHook } from "../../test-utils/query-hooks";
import {
  normalizePolicyVersionToPolicyHubMinor,
  useMCPPoliciesCatalog,
  useMCPPolicyDefinition,
  type MCPPoliciesCatalogResponse,
} from "./mcp-policies";

const HUB = "https://hub.example/policies?categories=Guardrails";
const MCP_URL = "https://hub.example/policies?categories=MCP&limit=50";
const DEFS = "https://hub.example/defs";
const fetchMock = vi.fn();
const ok = (body: unknown) => new Response(typeof body === "string" ? body : JSON.stringify(body), { status: 200 });

beforeEach(() => {
  vi.stubGlobal("fetch", fetchMock);
  fetchMock.mockReset();
  vi.mocked(listAvailableMCPPolicies).mockReset();
  globalConfig.guardrailsCatalogUrl = HUB;
  globalConfig.guardrailsDefinitionBaseUrl = DEFS;
});

afterEach(() => {
  cleanupHooks();
  vi.unstubAllGlobals();
  modules.installed.add("ai-gateway");
  globalConfig.guardrailsCatalogUrl = "";
  globalConfig.guardrailsDefinitionBaseUrl = "";
});

async function settle<T>(use: () => T) {
  const { result, qc } = await renderHook(use);
  await flush();
  return { query: result.current as unknown as UseQueryResult, qc };
}

const hubPolicy = (name: string, version: string) => ({
  name, version, displayName: name, description: "", provider: "hub", categories: ["MCP"], isLatest: true,
});

describe("normalizePolicyVersionToPolicyHubMinor", () => {
  it.each([
    ["v1.2.3", "1.2"],
    [" V2.0 ", "2.0"],
    ["1", "1.0"],
    ["v3", "3.0"],
    ["", ""],
    ["v", "v"],
    ["latest", "latest"],
    ["1.x", "1.x"],
    ["a.1", "a.1"],
  ])("normalizes %j to %j", (input, expected) => {
    expect(normalizePolicyVersionToPolicyHubMinor(input)).toBe(expected);
  });
});

describe("useMCPPoliciesCatalog", () => {
  it("keeps the hub's MCP policies the gateway runs, at the gateway's major version", async () => {
    vi.mocked(listAvailableMCPPolicies).mockResolvedValue({
      list: [
        { name: "mcp-auth", version: "1.2.7" },
        { name: "mcp-acl", version: "v2.0.0-rc1" },
        { name: "weird", version: "next" },
      ],
    } as never);
    fetchMock.mockResolvedValue(ok({
      count: 4,
      data: [
        hubPolicy("mcp-auth", "v1.2"),
        hubPolicy("mcp-auth", "v1.1"),
        hubPolicy("mcp-acl", "2.0"),
        hubPolicy("weird", "next"),
        hubPolicy("not-on-gateway", "v1.0"),
      ],
    }));

    const { qc } = await settle(() => useMCPPoliciesCatalog("o"));

    expect(listAvailableMCPPolicies).toHaveBeenCalledWith({ orgName: "o" }, expect.any(Function));
    expect(fetchMock).toHaveBeenCalledWith(MCP_URL, { headers: { Authorization: "Bearer t" } });
    const data = qc.getQueryData<MCPPoliciesCatalogResponse>(["MCP policies catalog", MCP_URL, "o"]);
    expect(data?.count).toBe(3);
    expect(data?.data.map((p) => [p.name, p.version, p.policyHubVersion])).toEqual([
      ["mcp-auth", "v1", "v1.2"],
      ["mcp-acl", "v2", "2.0"],
      // A version that is not numeric is passed through untouched.
      ["weird", "next", "next"],
    ]);
  });

  it("adds the subscription policies the gateways run that the hub doesn't list", async () => {
    vi.mocked(listAvailableMCPPolicies).mockResolvedValue({
      list: [
        { name: "mcp-auth", version: "1.2.7" },
        { name: "subscription-validation", version: "v2.0.1" },
        { name: "subscription-tool-ratelimit", version: "1.0.0" },
      ],
    } as never);
    fetchMock.mockResolvedValue(ok({ count: 1, data: [hubPolicy("mcp-auth", "v1.2")] }));

    const { qc } = await settle(() => useMCPPoliciesCatalog("o"));

    const data = qc.getQueryData<MCPPoliciesCatalogResponse>(["MCP policies catalog", MCP_URL, "o"]);
    expect(data?.count).toBe(3);
    expect(data?.data.map((p) => [p.name, p.version, p.policyHubVersion, p.displayName])).toEqual([
      ["mcp-auth", "v1", "v1.2", "mcp-auth"],
      ["subscription-validation", "v2", undefined, "Subscription Validation"],
      ["subscription-tool-ratelimit", "v1", undefined, "Subscription Tool Rate Limit"],
    ]);
  });

  it("adds a subscription policy only at the major version it is described for", async () => {
    // v1 of subscription-validation records no plan, so the rate-limit policy can't use it.
    vi.mocked(listAvailableMCPPolicies).mockResolvedValue({
      list: [{ name: "subscription-validation", version: "1.0.0" }],
    } as never);
    fetchMock.mockResolvedValue(ok({ count: 0, data: [] }));

    const { qc } = await settle(() => useMCPPoliciesCatalog("o"));

    expect(qc.getQueryData(["MCP policies catalog", MCP_URL, "o"])).toEqual({ count: 0, data: [] });
  });

  it("uses the hub's entry for a subscription policy once the hub lists it", async () => {
    vi.mocked(listAvailableMCPPolicies).mockResolvedValue({
      list: [{ name: "subscription-validation", version: "2.0.0" }],
    } as never);
    fetchMock.mockResolvedValue(ok({ count: 1, data: [hubPolicy("subscription-validation", "v2.0")] }));

    const { qc } = await settle(() => useMCPPoliciesCatalog("o"));

    const data = qc.getQueryData<MCPPoliciesCatalogResponse>(["MCP policies catalog", MCP_URL, "o"]);
    expect(data?.data).toHaveLength(1);
    expect(data?.data[0]).toMatchObject({ provider: "hub", version: "v2", policyHubVersion: "v2.0" });
  });

  it("keeps a limit already present in the configured hub URL", async () => {
    globalConfig.guardrailsCatalogUrl = "https://hub.example/policies?limit=5";
    vi.mocked(listAvailableMCPPolicies).mockResolvedValue({ list: [{ name: "a", version: "1.0" }] } as never);
    fetchMock.mockResolvedValue(ok({ data: [] }));
    await settle(() => useMCPPoliciesCatalog("o"));
    expect(fetchMock).toHaveBeenCalledWith(
      "https://hub.example/policies?limit=5&categories=MCP", expect.anything(),
    );
  });

  it("skips the hub when the gateway runs no MCP policies", async () => {
    vi.mocked(listAvailableMCPPolicies).mockResolvedValue({} as never);
    const { qc } = await settle(() => useMCPPoliciesCatalog("o"));
    expect(fetchMock).not.toHaveBeenCalled();
    expect(qc.getQueryData(["MCP policies catalog", MCP_URL, "o"])).toEqual({ count: 0, data: [] });
  });

  it.each([
    [new Response("hub down", { status: 503 }), "hub down"],
    [new Response("", { status: 500 }), "Failed to fetch MCP policies catalog: 500"],
  ])("fails with the hub's text or a status fallback", async (response, message) => {
    vi.mocked(listAvailableMCPPolicies).mockResolvedValue({ list: [{ name: "a", version: "1.0" }] } as never);
    fetchMock.mockResolvedValue(response);
    const { query } = await settle(() => useMCPPoliciesCatalog("o"));
    expect((query.error as Error).message).toBe(message);
  });

  it.each([
    ["no org", () => undefined, () => useMCPPoliciesCatalog(undefined)],
    ["no hub URL", () => { globalConfig.guardrailsCatalogUrl = ""; }, () => useMCPPoliciesCatalog("o")],
    ["an unparsable hub URL", () => { globalConfig.guardrailsCatalogUrl = "not a url"; }, () => useMCPPoliciesCatalog("o")],
    ["the gateway module absent", () => modules.installed.clear(), () => useMCPPoliciesCatalog("o")],
  ])("stays idle with %s", async (_n, arrange, use) => {
    arrange();
    const { query } = await settle(use);
    expect(listAvailableMCPPolicies).not.toHaveBeenCalled();
    expect(query.fetchStatus).toBe("idle");
  });
});

describe("useMCPPolicyDefinition", () => {
  it("fetches the YAML definition from an encoded name/version path", async () => {
    fetchMock.mockResolvedValue(ok("kind: Policy"));
    const { qc } = await settle(() => useMCPPolicyDefinition("mcp auth", "v1"));
    expect(fetchMock).toHaveBeenCalledWith(`${DEFS}/mcp%20auth/versions/v1/definition`, {
      headers: { Authorization: "Bearer t" },
    });
    expect(qc.getQueryData(["MCP policy definition", DEFS, "mcp auth", "v1"])).toBe("kind: Policy");
  });

  it.each([
    [new Response("gone", { status: 404 }), "gone"],
    [new Response("", { status: 500 }), "Failed to fetch MCP policy definition: 500"],
  ])("fails with the server text or a status fallback", async (response, message) => {
    fetchMock.mockResolvedValue(response);
    const { query } = await settle(() => useMCPPolicyDefinition("p", "v1"));
    expect((query.error as Error).message).toBe(message);
  });

  it.each([
    ["no version", () => useMCPPolicyDefinition("p", undefined), () => undefined],
    ["the gateway module absent", () => useMCPPolicyDefinition("p", "v1"), () => modules.installed.clear()],
  ])("stays idle with %s", async (_n, use, arrange) => {
    arrange();
    const { query } = await settle(use);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(query.fetchStatus).toBe("idle");
  });
});
