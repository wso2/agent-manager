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

import { useMemo } from "react";
import { useTokenScopes } from "@agent-management-platform/api-client";

import { ALLOWED, deny, type AccessDecision, type ScopeState } from "./types";

/** Gates `PUT /orgs/{org}/projects/{proj}` (rbac.ProjectUpdate). */
export const PROJECT_UPDATE_SCOPE = "amp:project:update";

/** Gates `PUT /orgs/{org}/projects/{proj}/agents/{agent}` (rbac.AgentUpdate). */
export const AGENT_UPDATE_SCOPE = "amp:agent:update";

/**
 * Decides a single flat capability scope, for routes whose authorization is one
 * `RequireAllPermissions` check and nothing resource-dependent — the pattern
 * behind most write endpoints. Environment-tier routes need
 * {@link evaluateAgentEnvironmentAccess} instead, which layers the tier on top.
 *
 * Unenforced and unresolved tokens both allow, for the reason given there: this
 * is a UX hint that spares the user a certain 403, never the enforcement point,
 * and denying on a half-read token would flash disabled controls on every load.
 */
export function evaluateScopeAccess(
  state: ScopeState,
  scope: string,
  what: string,
): AccessDecision {
  if (!state.enforced || !state.resolved) return ALLOWED;
  return state.scopes.has(scope) ? ALLOWED : deny(scope, what);
}

/**
 * Hook form of {@link evaluateScopeAccess}, bound to the current token.
 *
 * `what` completes "You do not have permission to ..." in the tooltip, so it
 * reads as a verb phrase: "update this project".
 */
export function useScopeAccess(scope: string, what: string): AccessDecision {
  const state = useTokenScopes();
  return useMemo(
    () => evaluateScopeAccess(state, scope, what),
    [state, scope, what],
  );
}
