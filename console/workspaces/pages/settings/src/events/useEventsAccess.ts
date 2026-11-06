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
import type { EventScope } from "@agent-management-platform/types";

export interface EventsAccess {
  /** Whether the scope sets are known yet. */
  resolved: boolean;
  /** May see the organization's destinations, subscriptions and delivery log. */
  canReadOrg: boolean;
  /** May add, change and test destinations and the organization's subscriptions. */
  canManageDestinations: boolean;
  /** May change this level's subscriptions. */
  canManageSubscriptions: boolean;
}

/**
 * What the caller may do on a level's Events page. Destinations and org
 * subscriptions need the events scopes (Admin only); a project's or an
 * agent's subscriptions are part of it and follow its update permission.
 * The service enforces all of this; the console only hides what would fail.
 */
export function useEventsAccess(scope: EventScope): EventsAccess {
  const { enforced, resolved, scopes } = useTokenScopes();
  return useMemo(() => {
    if (!enforced) {
      return {
        resolved: true,
        canReadOrg: true,
        canManageDestinations: true,
        canManageSubscriptions: true,
      };
    }
    const canManageDestinations = scopes.has("amp:events:manage");
    const canReadOrg = canManageDestinations || scopes.has("amp:events:read");
    const canManageSubscriptions =
      scope === "org"
        ? canManageDestinations
        : scope === "project"
          ? scopes.has("amp:project:update")
          : scopes.has("amp:agent:update");
    return { resolved, canReadOrg, canManageDestinations, canManageSubscriptions };
  }, [enforced, resolved, scopes, scope]);
}
