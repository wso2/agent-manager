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
import type { MCPProxyPolicy, SubscriptionPlan } from "@agent-management-platform/types";
import {
  missingPolicies,
  planEnforcement,
  hasRequestLimits,
  requiredPolicies,
  withPoliciesAdded,
} from "./subscriptionPolicies";

const planWith = (...types: string[]) =>
  ({ conditions: types.map((type) => ({ type, params: {} })) }) as unknown as SubscriptionPlan;
const names = (policies: MCPProxyPolicy[]) => policies.map((p) => `${p.name} ${p.version}`);

describe("requiredPolicies", () => {
  it("needs nothing without plans", () => {
    expect(requiredPolicies([])).toEqual([]);
  });
  it("needs validation v2, then the rate limit of each condition an MCP server counts", () => {
    expect(names(requiredPolicies([planWith("tool-call-count")]))).toEqual([
      "subscription-validation v2",
      "subscription-tool-ratelimit v1",
    ]);
    // An MCP server doesn't enforce request or token limits: only the validation is needed.
    expect(names(requiredPolicies([planWith("request-count", "token-count")]))).toEqual([
      "subscription-validation v2",
    ]);
    expect(hasRequestLimits([planWith("tool-call-count"), planWith("request-count")])).toBe(true);
    expect(hasRequestLimits([planWith("tool-call-count")])).toBe(false);
    expect(names(requiredPolicies([planWith()]))).toEqual(["subscription-validation v2"]);
  });
});

describe("missingPolicies and withPoliciesAdded", () => {
  const required = requiredPolicies([planWith("tool-call-count")]);
  it("lists a policy that any endpoint lacks", () => {
    const full = withPoliciesAdded([], required);
    expect(missingPolicies(required, [{ policies: full }, { policies: full }])).toEqual([]);
    const missing = missingPolicies(required, [{ policies: full }, { policies: [] }]);
    expect(names(missing)).toHaveLength(2);
  });
  it("puts validation first, replaces v1 in place, and keeps other policies", () => {
    const next = withPoliciesAdded(
      [
        { name: "mcp-auth", version: "v1" },
        { name: "subscription-validation", version: "v1.1.0" },
      ],
      required,
    );
    expect(names(next)).toEqual([
      "mcp-auth v1",
      "subscription-validation v2",
      "subscription-tool-ratelimit v1",
    ]);
    expect(names(withPoliciesAdded([{ name: "mcp-auth", version: "v1" }], required))[0]).toBe(
      "subscription-validation v2",
    );
  });
});

describe("planEnforcement", () => {
  it("marks a condition enforced only with validation v2 and its rate limit", () => {
    const plans = [planWith("tool-call-count", "token-count")];
    const none = planEnforcement(plans, []);
    expect(none.validationMissing).toBe(true);
    expect(none.rows.map((r) => r.status)).toEqual(["notEnforced", "notApplicable"]);
    const all = planEnforcement(plans, withPoliciesAdded([], requiredPolicies(plans)));
    expect(all.validationMissing).toBe(false);
    expect([all.enforced, all.applicable]).toEqual([1, 1]);
  });
});
