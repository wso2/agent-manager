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
vi.mock("../apis/llm-providers", () => ({ listAvailableLLMPolicies: vi.fn() }));

import { globalConfig } from "@agent-management-platform/types";
import type { UseQueryResult } from "@tanstack/react-query";
import { listAvailableLLMPolicies } from "../apis/llm-providers";
import { cleanupHooks, flush, renderHook } from "../../test-utils/query-hooks";
import {
  filterGuardrailPolicies,
  useGuardrailPolicyDefinition,
  useGuardrailsCatalog,
  useLLMPoliciesCatalog,
  type GuardrailDefinition,
} from "./guardrails";

const CATALOG = "https://hub.example/policies";
const DEFS = "https://hub.example/defs";
const fetchMock = vi.fn();

const policy = (name: string, extra: Partial<GuardrailDefinition> = {}): GuardrailDefinition => ({
  name, version: "v1", displayName: "", description: "", provider: "hub", categories: [], isLatest: true, ...extra,
});
const ok = (body: unknown) => new Response(typeof body === "string" ? body : JSON.stringify(body), { status: 200 });

beforeEach(() => {
  vi.stubGlobal("fetch", fetchMock);
  fetchMock.mockReset();
  globalConfig.guardrailsCatalogUrl = CATALOG;
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

describe("filterGuardrailPolicies", () => {
  const all = [
    policy("pii-masking"),
    policy("jwt-auth"),
    policy("set-headers"),
    policy("aws-bedrock-guardrail"),
    policy("semantic-cache"),
    policy("semantic-prompt-guard"),
  ];

  it("always hides infra/auth policies and hides capability-gated ones by default", () => {
    expect(filterGuardrailPolicies(all).map((p) => p.name)).toEqual(["pii-masking"]);
  });

  it("shows a capability-gated policy only when its capability is on", () => {
    const capabilities = { awsBedrock: true, semanticGuardrails: false } as never;
    const names = filterGuardrailPolicies(all, capabilities)
      .map((p) => p.name);
    expect(names).toEqual(["pii-masking", "aws-bedrock-guardrail"]);
  });

  it("shows every policy a capability unlocks", () => {
    const capabilities = { semanticGuardrails: true } as never;
    const names = filterGuardrailPolicies(all, capabilities).map((p) => p.name);
    expect(names).toEqual(["pii-masking", "semantic-cache", "semantic-prompt-guard"]);
  });
});

describe("useGuardrailsCatalog", () => {
  it("fetches the catalog with the user's token", async () => {
    fetchMock.mockResolvedValue(ok({ count: 1, data: [policy("pii-masking")] }));
    const { qc } = await settle(() => useGuardrailsCatalog());
    expect(fetchMock).toHaveBeenCalledWith(CATALOG, { headers: { Authorization: "Bearer t" } });
    expect(qc.getQueryData(["Guardrails catalog", CATALOG])).toEqual({ count: 1, data: [policy("pii-masking")] });
  });

  it.each([
    ["the server's message", new Response("hub down", { status: 503 }), "hub down"],
    ["a status fallback", new Response("", { status: 502 }), "Failed to fetch guardrails catalog: 502"],
  ])("fails with %s on a non-OK response", async (_n, response, message) => {
    fetchMock.mockResolvedValue(response);
    const { query } = await settle(() => useGuardrailsCatalog());
    expect(query.isError).toBe(true);
    expect((query.error as Error).message).toBe(message);
  });

  it.each([
    ["the caller disables it", () => useGuardrailsCatalog(false), () => undefined],
    ["no catalog URL is configured", () => useGuardrailsCatalog(), () => { globalConfig.guardrailsCatalogUrl = ""; }],
    ["the gateway module is absent", () => useGuardrailsCatalog(), () => modules.installed.clear()],
  ])("stays idle when %s", async (_n, use, arrange) => {
    arrange();
    const { query } = await settle(use);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(query.fetchStatus).toBe("idle");
  });
});

describe("useGuardrailPolicyDefinition", () => {
  it("fetches the YAML definition from an encoded name/version path", async () => {
    fetchMock.mockResolvedValue(ok("kind: Policy"));
    const { qc } = await settle(() => useGuardrailPolicyDefinition("pii masking", "v1/0"));
    expect(fetchMock).toHaveBeenCalledWith(`${DEFS}/pii%20masking/versions/v1%2F0/definition`, {
      headers: { Authorization: "Bearer t" },
    });
    expect(qc.getQueryData(["Guardrail policy definition", DEFS, "pii masking", "v1/0"])).toBe("kind: Policy");
  });

  it.each([
    [new Response("no such policy", { status: 404 }), "no such policy"],
    [new Response("", { status: 500 }), "Failed to fetch policy definition: 500"],
  ])("fails with the server text or a status fallback", async (response, message) => {
    fetchMock.mockResolvedValue(response);
    const { query } = await settle(() => useGuardrailPolicyDefinition("p", "v1"));
    expect((query.error as Error).message).toBe(message);
  });

  it.each([
    ["no name", () => useGuardrailPolicyDefinition(undefined, "v1")],
    ["no version", () => useGuardrailPolicyDefinition("p", undefined)],
  ])("stays idle with %s", async (_n, use) => {
    const { query } = await settle(use);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(query.fetchStatus).toBe("idle");
  });

  it("stays idle when the gateway module is absent", async () => {
    modules.installed.clear();
    await settle(() => useGuardrailPolicyDefinition("p", "v1"));
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe("useLLMPoliciesCatalog", () => {
  const gatewayList = {
    list: [
      { name: "custom-guard", version: "1.0.3", displayName: "My Guard", description: "mine", parameters: { a: 1 } },
      { name: "pii-masking", version: "1.2.0", displayName: "" },
      { name: "word-count", version: "0.9.1" },
    ],
  };

  it("lists the gateway's policies, naming built-ins from the hub", async () => {
    vi.mocked(listAvailableLLMPolicies).mockResolvedValue(gatewayList as never);
    fetchMock.mockResolvedValue(ok({
      data: [policy("pii-masking", { displayName: "PII Masking" }), policy("custom-guard", { displayName: "Hub name" })],
    }));
    const { qc } = await settle(() => useLLMPoliciesCatalog("o", true, "openai"));

    expect(listAvailableLLMPolicies).toHaveBeenCalledWith({ orgName: "o" }, { providerId: "openai" }, expect.any(Function));
    await expect(vi.mocked(listAvailableLLMPolicies).mock.calls[0][2]!()).resolves.toBe("t");
    const data = qc.getQueryData<{ count: number; data: GuardrailDefinition[] }>(
      ["LLM gateway policies catalog", "o", "openai", CATALOG],
    );
    expect(data?.count).toBe(3);
    expect(data?.data).toEqual([
      {
        name: "custom-guard", version: "1.0.3", displayName: "My Guard", description: "mine", provider: "gateway",
        categories: [], isLatest: true, parameters: { a: 1 }, systemParameters: {},
      },
      expect.objectContaining({ name: "pii-masking", version: "1.2.0", displayName: "PII Masking", parameters: {} }),
      expect.objectContaining({ name: "word-count", displayName: "word-count", description: "" }),
    ]);
  });

  it.each([
    ["the hub fails", () => fetchMock.mockResolvedValue(new Response("", { status: 500 }))],
    ["the hub is unreachable", () => fetchMock.mockRejectedValue(new TypeError("network"))],
    ["no hub is configured", () => { globalConfig.guardrailsCatalogUrl = ""; }],
  ])("falls back to raw names when %s", async (_n, arrange) => {
    arrange();
    vi.mocked(listAvailableLLMPolicies).mockResolvedValue(gatewayList as never);
    const { qc } = await settle(() => useLLMPoliciesCatalog("o"));
    const data = qc.getQueryData<{ data: GuardrailDefinition[] }>(
      ["LLM gateway policies catalog", "o", undefined, globalConfig.guardrailsCatalogUrl],
    );
    expect(data?.data.map((p) => p.displayName)).toEqual(["My Guard", "pii-masking", "word-count"]);
  });

  it("returns an empty catalog when the gateway reports no list", async () => {
    vi.mocked(listAvailableLLMPolicies).mockResolvedValue({} as never);
    fetchMock.mockResolvedValue(ok({}));
    const { qc } = await settle(() => useLLMPoliciesCatalog("o"));
    expect(qc.getQueryData(["LLM gateway policies catalog", "o", undefined, CATALOG])).toEqual({ count: 0, data: [] });
  });

  it.each([
    ["without an org", () => useLLMPoliciesCatalog(undefined)],
    ["when disabled", () => useLLMPoliciesCatalog("o", false)],
  ])("stays idle %s", async (_n, use) => {
    vi.mocked(listAvailableLLMPolicies).mockClear();
    const { query } = await settle(use);
    expect(listAvailableLLMPolicies).not.toHaveBeenCalled();
    expect(query.fetchStatus).toBe("idle");
  });
});
