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
import { useAuthHooks } from "@agent-management-platform/auth";
import { globalConfig } from "@agent-management-platform/types";

/**
 * Reads the scope claim off the current access token.
 *
 * It lives beside the query hooks because this package is where the token is
 * already read for every request — the scope set is a property of that same
 * token, not a separate resource to fetch.
 *
 * Callers gating a control on the environment tier should use
 * useAgentEnvironmentAccess from @agent-management-platform/shared-component
 * rather than testing scope strings by hand; it encodes the floor/production
 * rule in one place.
 *
 * The return shape is ScopeState from that module, spelled out here rather than
 * imported because this package cannot depend on shared-component without a
 * cycle. The two have to be kept in step: the rule reads every field below.
 */
export function useTokenScopes(): {
  /**
   * False when this deployment does not enforce RBAC. It mirrors the service's
   * RBAC_ENABLED switch (plus disableAuth, where there is no token at all): the
   * server gates nothing, so the console must not either.
   */
  enforced: boolean;
  /**
   * False while the access token is still being decoded, when `scopes` is empty
   * only because nothing has been read yet.
   *
   * A caller that hides what the scope set does not reach has to wait for this,
   * or it renders the shape of a token with no permissions and then rearranges
   * itself a moment later; evaluateAgentEnvironmentAccess does that waiting for
   * every gated control. It says nothing about how many scopes were found: a
   * resolved token carrying none is resolved, and every gated surface should
   * deny.
   */
  resolved: boolean;
  scopes: ReadonlySet<string>;
} {
  const { userInfo, isLoadingAccessToken } = useAuthHooks();
  const scopeStr = userInfo?.scope;
  return useMemo(
    () => ({
      enforced: !globalConfig.disableAuth && globalConfig.rbacEnabled,
      resolved: !isLoadingAccessToken,
      scopes: new Set((scopeStr ?? "").split(" ").filter(Boolean)),
    }),
    [scopeStr, isLoadingAccessToken],
  );
}
