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

/** The scope claim on the current access token. Used by Events, which is gated on scopes. */
export function useTokenScopes(): {
  /** False when auth is disabled: the server gates nothing, so the console must not either. */
  enforced: boolean;
  /** False while the token is still being decoded (`scopes` is empty only because unread). */
  resolved: boolean;
  scopes: ReadonlySet<string>;
} {
  const { userInfo, isLoadingAccessToken } = useAuthHooks();
  const scopeStr = userInfo?.scope;
  return useMemo(
    () => ({
      enforced: !globalConfig.disableAuth,
      resolved: !isLoadingAccessToken,
      scopes: new Set((scopeStr ?? "").split(" ").filter(Boolean)),
    }),
    [scopeStr, isLoadingAccessToken],
  );
}
