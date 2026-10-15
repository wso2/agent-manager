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
import type { GuardrailDefinition } from "@agent-management-platform/api-client";
import { normalizeRootSchema } from "../../utils/policyParameterEditor/schemaUtils";
import { resolvePolicyDetail } from "./resolvePolicyDetail";

function definition(overrides: Partial<GuardrailDefinition> = {}): GuardrailDefinition {
  return {
    name: "mcp-ratelimit",
    version: "v1",
    displayName: "MCP Rate Limit",
    description: "Rate limits MCP traffic.",
    provider: "gateway",
    categories: ["MCP", "Security"],
    isLatest: true,
    parameters: {
      type: "object",
      properties: { limit: { type: "integer", minimum: 1 } },
      required: ["limit"],
    },
    systemParameters: {},
    requiredSystemConfig: [],
    ...overrides,
  };
}

describe("resolvePolicyDetail", () => {
  it("builds the editor definition from the inline gateway schema", () => {
    const policy = definition();

    const detail = resolvePolicyDetail(policy);

    expect(detail.name).toBe("mcp-ratelimit");
    expect(detail.version).toBe("v1");
    expect(detail.description).toBe("Rate limits MCP traffic.");
    expect(detail.parameters).toEqual(normalizeRootSchema(policy.parameters));
    expect(detail.systemParameters).toEqual(normalizeRootSchema(policy.systemParameters));
  });

  it("yields a definition even for a policy with no parameters", () => {
    const detail = resolvePolicyDetail(definition({ parameters: {}, description: "" }));

    expect(detail.parameters).toEqual(normalizeRootSchema({}));
    expect(detail.description).toBe("");
  });
});
