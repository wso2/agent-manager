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
 * The vocabulary every access rule in this directory speaks: the token state a
 * rule reads, and the verdict it returns. Kept apart from any one rule so that
 * {@link RestrictedAction} and a new rule can share them without either
 * depending on the environment-tier logic.
 */

/** The scopes the current token carries, and whether they are enforced at all. */
export interface ScopeState {
  /**
   * False when auth is disabled outright, so no token exists to carry scopes.
   * Every check then passes, exactly as the service gates nothing.
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

export interface AccessDecision {
  allowed: boolean;
  /** The scope whose absence decided it. Undefined when allowed. */
  missingScope?: string;
  /** A sentence for the disabled control's tooltip. Empty when allowed. */
  reason: string;
}

export const ALLOWED: AccessDecision = { allowed: true, reason: "" };

/** Builds the denial, naming the scope so tooltip and eventual 403 agree. */
export function deny(missingScope: string, what: string): AccessDecision {
  return {
    allowed: false,
    missingScope,
    reason: `You do not have permission to ${what}. This requires the ${missingScope} scope.`,
  };
}
