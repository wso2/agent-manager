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
 * The environment tier is an authorization axis of its own: it is about where an
 * action lands rather than what the action is. The floor says "may act on
 * environments at all"; the production grant is held in addition to it to reach
 * the environments OpenChoreo flags isProduction, and is never sufficient alone.
 *
 * Deploy, promote and deployment-state changes are all decided on it by
 * agentManagerService.requireEnvTier, which resolves the target environment
 * server-side. What follows mirrors that rule so the console does not offer a
 * button that is certain to come back 403 — it is a UX hint, never the
 * enforcement point.
 *
 * The rule is kept free of React and of the app's runtime config so it can be
 * tested directly; useAgentEnvironmentAccess in the sibling file binds it to the
 * current token.
 */
export const AGENT_ENV_NON_PRODUCTION_SCOPE = "amp:agent:env-non-production";
export const AGENT_ENV_PRODUCTION_SCOPE = "amp:agent:env-production";

/** The capability scope the deployment-state route demands beside the tier. */
export const AGENT_SUSPEND_SCOPE = "amp:agent:suspend";

/** The scopes the current token carries, and whether they are enforced at all. */
export interface ScopeState {
  /**
   * False when this deployment does not enforce RBAC (RBAC_ENABLED=false, or
   * auth disabled outright). Every check then passes, exactly as the service's
   * own gates skip when the switch is off.
   */
  enforced: boolean;
  /**
   * False while the access token is still being decoded, when `scopes` is empty
   * only because nothing has been read yet. The rule needs it because an empty
   * set is otherwise indistinguishable from a token that genuinely carries no
   * scopes at all.
   */
  resolved: boolean;
  scopes: ReadonlySet<string>;
}

/** Only the part of an Environment that decides the tier. */
export interface EnvironmentTier {
  isProduction?: boolean;
}

export interface AccessDecision {
  allowed: boolean;
  /** The scope whose absence decided it. Undefined when allowed. */
  missingScope?: string;
  /** A sentence for the disabled control's tooltip. Empty when allowed. */
  reason: string;
}

export const ALLOWED: AccessDecision = { allowed: true, reason: "" };

function deny(missingScope: string, what: string): AccessDecision {
  return {
    allowed: false,
    missingScope,
    reason: `You do not have permission to ${what}. This requires the ${missingScope} scope.`,
  };
}

/**
 * Decides whether the caller may act on `environment`, given the capability
 * scope the operation also needs — deploy and promote declare none beside the
 * tier, the deployment-state route declares agent:suspend.
 *
 * Order matches the server's: the route's static capability check runs first
 * (middleware.RequireAllPermissions), then the service resolves the environment
 * and checks the floor, then the production grant. Reporting the same scope the
 * server would name keeps the tooltip and the eventual 403 consistent.
 *
 * An unresolved token allows everything, for the same reason and with the same
 * caveat as the unknown tier below: this is a hint, and the server decides.
 *
 * `environment` is undefined while the environment list is still loading. The
 * tier is then unknown, and denying on a guess would flash a disabled control on
 * every page load, so only the floor is checked and the server settles the rest.
 */
export function evaluateAgentEnvironmentAccess(
  state: ScopeState,
  environment: EnvironmentTier | undefined,
  capability?: string,
): AccessDecision {
  if (!state.enforced) return ALLOWED;
  // An unread token carries an empty scope set, which the checks below would
  // read as a token permitted nothing: every gated control disabled, each
  // naming a scope the caller may well hold, until the decode lands a moment
  // later. The server is the enforcement point, so the hint says nothing until
  // it has something to say.
  if (!state.resolved) return ALLOWED;

  if (capability && !state.scopes.has(capability)) {
    return deny(capability, "perform this action");
  }
  if (!state.scopes.has(AGENT_ENV_NON_PRODUCTION_SCOPE)) {
    return deny(AGENT_ENV_NON_PRODUCTION_SCOPE, "act on deployment environments");
  }
  if (environment?.isProduction && !state.scopes.has(AGENT_ENV_PRODUCTION_SCOPE)) {
    return deny(AGENT_ENV_PRODUCTION_SCOPE, "act on production environments");
  }
  return ALLOWED;
}
