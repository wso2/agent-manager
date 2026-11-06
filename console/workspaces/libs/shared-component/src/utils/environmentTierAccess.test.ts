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
import type { FeatureAccessKey } from "@agent-management-platform/feature-management";
import { evaluateAgentEnvironmentAccess, type AccessState } from "./environmentTierAccess";

const withAccess = (granted: FeatureAccessKey[]): AccessState => ({
  hasAccess: (key) => granted.includes(key),
});

// A Developer holds the non-production floor but not production.
const developer = withAccess(["agent_deploy", "agent_suspend"]);
// A Platform Engineer holds both tiers.
const platformEngineer = withAccess(["agent_deploy", "agent_deploy_production", "agent_suspend"]);

const staging = { isProduction: false };
const production = { isProduction: true };

describe("evaluateAgentEnvironmentAccess", () => {
  it("allows the floor holder into a non-production environment", () => {
    const decision = evaluateAgentEnvironmentAccess(developer, staging);
    expect(decision.allowed).toBe(true);
    expect(decision.missingAccess).toBeUndefined();
    expect(decision.reason).toBe("");
  });

  it("denies the floor holder a production environment, naming what it lacks", () => {
    const decision = evaluateAgentEnvironmentAccess(developer, production);
    expect(decision.allowed).toBe(false);
    expect(decision.missingAccess).toBe("agent_deploy_production");
    expect(decision.reason).toContain("production");
  });

  it("allows a holder of both tiers into a production environment", () => {
    expect(evaluateAgentEnvironmentAccess(platformEngineer, production).allowed).toBe(true);
  });

  // Production access never suffices without the floor, as on the server.
  it("denies a user holding only production access", () => {
    const decision = evaluateAgentEnvironmentAccess(
      withAccess(["agent_deploy_production"]),
      production,
    );
    expect(decision.allowed).toBe(false);
    expect(decision.missingAccess).toBe("agent_deploy");
  });

  it("requires the capability alongside the tier", () => {
    const decision = evaluateAgentEnvironmentAccess(
      withAccess(["agent_deploy"]),
      staging,
      "agent_suspend",
    );
    expect(decision.allowed).toBe(false);
    expect(decision.missingAccess).toBe("agent_suspend");
  });

  it("reports the missing capability before the missing tier", () => {
    const decision = evaluateAgentEnvironmentAccess(withAccess([]), production, "agent_suspend");
    expect(decision.missingAccess).toBe("agent_suspend");
  });

  // An unloaded environment has an unknown tier, so only the floor is checked.
  it("checks only the floor when the environment is unknown", () => {
    expect(evaluateAgentEnvironmentAccess(developer, undefined).allowed).toBe(true);
    expect(evaluateAgentEnvironmentAccess(withAccess([]), undefined).allowed).toBe(false);
  });

  it("denies when nothing is granted", () => {
    const decision = evaluateAgentEnvironmentAccess(withAccess([]), staging);
    expect(decision.allowed).toBe(false);
    expect(decision.missingAccess).toBe("agent_deploy");
  });
});
