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

import { describe, expect, it, vi } from "vitest";
import type { GuardrailCapabilities } from "@agent-management-platform/types";

vi.mock("@agent-management-platform/views", () => ({ useSnackBar: () => ({ pushSnackBar: vi.fn() }) }));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: async () => "t", isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("./telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));

import { filterGuardrailPolicies, type GuardrailDefinition } from "./guardrails";

function policy(name: string): GuardrailDefinition {
  return {
    name,
    version: "v1.0.0",
    displayName: name,
    description: "",
    provider: "WSO2",
    categories: [],
    isLatest: true,
  };
}

const allCapabilities: GuardrailCapabilities = {
  awsBedrock: true,
  azureContentSafety: true,
  graniteGuardian: true,
  nemoGuard: true,
  semanticGuardrails: true,
};

describe("filterGuardrailPolicies", () => {
  it("filterGuardrailPolicies hides policies the gateway bundles but AM cannot configure", () => {
    const policies = [
      "mtls-auth",
      "oauth2-token-exchange",
      "typesafe-jev-content-safety",
      "typesafe-jev-mcp-tool-intent-verification",
      "typesafe-jev-mcp-tool-result-screening",
      "typesafe-jev-model-routing",
      "typesafe-jev-tool-filtering",
      "regex-guardrail",
    ].map(policy);

    const visible = filterGuardrailPolicies(policies, allCapabilities);

    expect(visible.map((p) => p.name)).toEqual(["regex-guardrail"]);
  });
});
