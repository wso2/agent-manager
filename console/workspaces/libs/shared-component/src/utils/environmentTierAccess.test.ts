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
import {
  AGENT_ENV_NON_PRODUCTION_SCOPE,
  AGENT_ENV_PRODUCTION_SCOPE,
  AGENT_SUSPEND_SCOPE,
  evaluateAgentEnvironmentAccess,
  type ScopeState,
} from "./environmentTierAccess";

// The scope set a Developer carries after the role rebalance: the tier floor but
// not the production grant.
const developer: ScopeState = {
  enforced: true,
  resolved: true,
  scopes: new Set([AGENT_ENV_NON_PRODUCTION_SCOPE, AGENT_SUSPEND_SCOPE]),
};

// A Platform Engineer holds both tiers.
const platformEngineer: ScopeState = {
  enforced: true,
  resolved: true,
  scopes: new Set([
    AGENT_ENV_NON_PRODUCTION_SCOPE,
    AGENT_ENV_PRODUCTION_SCOPE,
    AGENT_SUSPEND_SCOPE,
  ]),
};

const staging = { isProduction: false };
const production = { isProduction: true };

describe("evaluateAgentEnvironmentAccess", () => {
  it("allows the floor holder into a non-production environment", () => {
    const decision = evaluateAgentEnvironmentAccess(developer, staging);
    expect(decision.allowed).toBe(true);
    expect(decision.missingScope).toBeUndefined();
    expect(decision.reason).toBe("");
  });

  it("denies the floor holder a production environment, naming the grant it lacks", () => {
    const decision = evaluateAgentEnvironmentAccess(developer, production);
    expect(decision.allowed).toBe(false);
    expect(decision.missingScope).toBe(AGENT_ENV_PRODUCTION_SCOPE);
    expect(decision.reason).toContain(AGENT_ENV_PRODUCTION_SCOPE);
  });

  it("allows a holder of both tiers into a production environment", () => {
    expect(evaluateAgentEnvironmentAccess(platformEngineer, production).allowed).toBe(true);
  });

  // The production grant is never sufficient on its own — the floor is what says
  // "may act on environments at all", and the service layer denies without it
  // whatever else the token carries.
  it("denies a token holding only the production grant", () => {
    const decision = evaluateAgentEnvironmentAccess(
      { enforced: true, resolved: true, scopes: new Set([AGENT_ENV_PRODUCTION_SCOPE]) },
      production,
    );
    expect(decision.allowed).toBe(false);
    expect(decision.missingScope).toBe(AGENT_ENV_NON_PRODUCTION_SCOPE);
  });

  it("requires the capability scope alongside the tier", () => {
    const decision = evaluateAgentEnvironmentAccess(
      { enforced: true, resolved: true, scopes: new Set([AGENT_ENV_NON_PRODUCTION_SCOPE]) },
      staging,
      AGENT_SUSPEND_SCOPE,
    );
    expect(decision.allowed).toBe(false);
    expect(decision.missingScope).toBe(AGENT_SUSPEND_SCOPE);
  });

  it("reports the missing capability before the missing tier", () => {
    const decision = evaluateAgentEnvironmentAccess(
      { enforced: true, resolved: true, scopes: new Set<string>() },
      production,
      AGENT_SUSPEND_SCOPE,
    );
    expect(decision.missingScope).toBe(AGENT_SUSPEND_SCOPE);
  });

  // An environment the console has not loaded yet has an unknown tier. Denying
  // on a guess would flash a disabled control on every page load, so only the
  // floor is checked and the server settles the rest.
  it("checks only the floor when the environment is unknown", () => {
    expect(evaluateAgentEnvironmentAccess(developer, undefined).allowed).toBe(true);
    const noScopes = { enforced: true, resolved: true, scopes: new Set<string>() };
    expect(evaluateAgentEnvironmentAccess(noScopes, undefined).allowed).toBe(false);
  });

  // The scope set is empty before the access token is decoded, which is not the
  // same fact as a token that carries nothing. Denying on it disables every
  // gated control for as long as the decode takes and explains the absence with
  // a scope the caller may well hold.
  it("allows everything while the token is still being read", () => {
    const unread: ScopeState = {
      enforced: true,
      resolved: false,
      scopes: new Set<string>(),
    };
    const decision = evaluateAgentEnvironmentAccess(unread, production, AGENT_SUSPEND_SCOPE);
    expect(decision.allowed).toBe(true);
    expect(decision.reason).toBe("");
  });

  // Resolving with nothing is a real denial, not a pending one.
  it("denies once a token resolves carrying no scopes", () => {
    const decision = evaluateAgentEnvironmentAccess(
      { enforced: true, resolved: true, scopes: new Set<string>() },
      staging,
    );
    expect(decision.allowed).toBe(false);
    expect(decision.missingScope).toBe(AGENT_ENV_NON_PRODUCTION_SCOPE);
  });

  // Mirrors RBAC_ENABLED=false on the service: nothing is enforced, so the
  // console must not gate anything either.
  it("allows everything when RBAC is not enforced", () => {
    const decision = evaluateAgentEnvironmentAccess(
      { enforced: false, resolved: true, scopes: new Set<string>() },
      production,
      AGENT_SUSPEND_SCOPE,
    );
    expect(decision.allowed).toBe(true);
  });
});
