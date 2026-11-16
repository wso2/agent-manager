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

export interface ShadowAccess {
  /** Whether the scope sets are known yet. */
  resolved: boolean;
  /** May see platforms (without secrets), scans and unregistered agents. */
  canRead: boolean;
  /** May add platforms, run scans and review agents. */
  canManage: boolean;
}

/**
 * What the caller may do in Shadow Agent Discovery (organization admins, by
 * default). The service enforces it; the console only hides what would fail.
 */
export function useShadowAccess(): ShadowAccess {
  const { enforced, resolved, scopes } = useTokenScopes();
  return useMemo(() => {
    if (!enforced) return { resolved: true, canRead: true, canManage: true };
    const canManage = scopes.has("amp:shadow-agent:manage");
    return { resolved, canRead: canManage || scopes.has("amp:shadow-agent:read"), canManage };
  }, [enforced, resolved, scopes]);
}
