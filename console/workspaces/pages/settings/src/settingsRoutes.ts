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

import { useMemo } from "react";
import { absoluteRouteMap, globalConfig } from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";

/**
 * Accessor for the `settings` subtree of the generated absolute route map.
 * The generated map is fully typed (every path/wildPath is preserved in its
 * declaration), so this needs no cast.
 */
export const settingsRoute = absoluteRouteMap.children.org.children.settings;

export interface IdentityVisibility {
  users: boolean;
  roles: boolean;
  groups: boolean;
  alerting: boolean;
}

/**
 * Mirror of the identity-related scope checks used by the main left navigation
 * (`navigationItems.tsx`). Kept local to avoid a circular dependency on core-ui.
 */
export function useIdentityVisibility(): IdentityVisibility {
  const { userInfo } = useAuthHooks();
  return useMemo(() => {
    if (globalConfig.disableAuth) {
      return { users: true, roles: true, groups: true, alerting: true };
    }
    const scopeStr = userInfo?.scope;
    if (!scopeStr) {
      return { users: false, roles: false, groups: false, alerting: false };
    }
    const s = new Set(scopeStr.split(" ").filter(Boolean));
    return {
      users: s.has("amp:org:invite-member") || s.has("amp:org:remove-member"),
      roles:
        s.has("amp:role:read") ||
        s.has("amp:role:create") ||
        s.has("amp:role:update") ||
        s.has("amp:role:delete"),
      groups:
        s.has("amp:group:read") ||
        s.has("amp:group:create") ||
        s.has("amp:group:update") ||
        s.has("amp:group:delete"),
      alerting: s.has("amp:alerting:read") || s.has("amp:alerting:manage"),
    };
  }, [userInfo?.scope]);
}

export interface AlertingAccess {
  canRead: boolean;
  canManage: boolean;
}

/**
 * Alerting scopes are granted to the Admin role only. Manage implies read:
 * the manage-only routes (save, test, delete) are useless without seeing the
 * endpoint.
 */
export function useAlertingAccess(): AlertingAccess {
  const { userInfo } = useAuthHooks();
  return useMemo(() => {
    if (globalConfig.disableAuth) {
      return { canRead: true, canManage: true };
    }
    const s = new Set((userInfo?.scope ?? "").split(" ").filter(Boolean));
    const canManage = s.has("amp:alerting:manage");
    return { canRead: canManage || s.has("amp:alerting:read"), canManage };
  }, [userInfo?.scope]);
}
