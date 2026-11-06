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

import { absoluteRouteMap } from "@agent-management-platform/types";
import { useFeatureManagement } from "@agent-management-platform/feature-management";

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
}

/** Which identity settings tabs the user can open, from the shared feature access. */
export function useIdentityVisibility(): IdentityVisibility {
  const { hasAccess } = useFeatureManagement();
  return {
    users: hasAccess("member_read"),
    roles: hasAccess("role_read"),
    groups: hasAccess("group_read"),
  };
}
