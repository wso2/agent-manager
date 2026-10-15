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

/**
 * Which policies apply to LLM vs MCP is decided by agent-manager-service; the console
 * only hides policies a dedicated tab owns, and lists policies that need gateway
 * configuration (with a warning) rather than hiding them. These tests pin that split
 * so the console does not grow a second applicability or capability list.
 */

import { describe, expect, it, vi } from "vitest";

// The functions under test are pure; these mocks only stop the module's hook-side
// imports (auth pulls in @wso2/oxygen-ui, whose prismjs import does not resolve
// under vitest) from loading.
vi.mock("@agent-management-platform/auth", () => ({ useAuthHooks: vi.fn() }));
vi.mock("../apis", () => ({ listAvailableLLMPolicies: vi.fn() }));
vi.mock("./react-query-notifications", () => ({ useApiQuery: vi.fn() }));

import {
  filterGuardrailPolicies,
  toPolicyDefinition,
  type GuardrailDefinition,
} from "./guardrails";

function policy(name: string): GuardrailDefinition {
  return toPolicyDefinition({ name, version: "v1.0.0" });
}

function names(policies: GuardrailDefinition[]): string[] {
  return policies.map((p) => p.name);
}

describe("toPolicyDefinition", () => {
  it("keeps the service's display-ready fields", () => {
    const def = toPolicyDefinition({
      name: "word-count-guardrail",
      version: "v1.0.0",
      displayName: "Word Count Guardrail",
      description: "Validates word count.",
      categories: ["Guardrails", "AI"],
      parameters: { type: "object" },
      systemParameters: { type: "object", properties: {} },
      requiredSystemConfig: ["azurecontentsafety_key"],
    });

    expect(def).toEqual({
      name: "word-count-guardrail",
      version: "v1.0.0",
      displayName: "Word Count Guardrail",
      description: "Validates word count.",
      provider: "gateway",
      categories: ["Guardrails", "AI"],
      isLatest: true,
      parameters: { type: "object" },
      systemParameters: { type: "object", properties: {} },
      requiredSystemConfig: ["azurecontentsafety_key"],
    });
  });

  it("never changes the version (MCP majors and LLM build versions pass through)", () => {
    expect(toPolicyDefinition({ name: "mcp-ratelimit", version: "v1" }).version).toBe("v1");
    expect(toPolicyDefinition({ name: "cors", version: "v1.0.2" }).version).toBe("v1.0.2");
  });

  it("fills safe defaults for an item with only name and version", () => {
    const def = toPolicyDefinition({ name: "acme-custom", version: "v2" });

    expect(def.displayName).toBe("acme-custom");
    expect(def.description).toBe("");
    expect(def.categories).toEqual([]);
    expect(def.parameters).toEqual({});
    expect(def.systemParameters).toEqual({});
    expect(def.requiredSystemConfig).toEqual([]);
  });

  it("falls back to the name for an empty display name", () => {
    expect(
      toPolicyDefinition({ name: "cors", version: "v1", displayName: "" }).displayName,
    ).toBe("cors");
  });
});

describe("filterGuardrailPolicies", () => {
  it("hides policies a dedicated LLM provider tab manages", () => {
    const managed = [
      "api-key-auth",
      "basic-auth",
      "jwt-auth",
      "advanced-ratelimit",
      "basic-ratelimit",
      "token-based-ratelimit",
      "llm-cost-based-ratelimit",
      "llm-cost",
      "subscription-validation",
    ];

    const visible = filterGuardrailPolicies([...managed.map(policy), policy("word-count-guardrail")]);

    expect(names(visible)).toEqual(["word-count-guardrail"]);
  });

  it("does not re-filter MCP or mediation policies the service already decided on", () => {
    const fromService = ["cors", "log-message", "set-headers", "mcp-custom-policy"].map(policy);

    expect(names(filterGuardrailPolicies(fromService))).toEqual([
      "cors",
      "log-message",
      "set-headers",
      "mcp-custom-policy",
    ]);
  });

  it("lists policies that need gateway configuration instead of hiding them", () => {
    const needsConfig = toPolicyDefinition({
      name: "azure-content-safety-content-moderation",
      version: "v1.0.0",
      requiredSystemConfig: ["azurecontentsafety_endpoint", "azurecontentsafety_key"],
    });

    expect(names(filterGuardrailPolicies([needsConfig, policy("regex-guardrail")]))).toEqual([
      "azure-content-safety-content-moderation",
      "regex-guardrail",
    ]);
  });
});
