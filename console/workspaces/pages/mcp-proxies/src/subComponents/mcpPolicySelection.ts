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

import {
  normalizePolicyVersionToMajor,
  type MCPPoliciesCatalogResponse,
} from "@agent-management-platform/api-client";
import type { MCPProxyPolicy } from "@agent-management-platform/types";
import type { PolicySelection } from "@agent-management-platform/shared-component";

/**
 * Applies an add or edit from the picker. The picker returns the exact catalog
 * version; the proxy stores its major (e.g. "v1"). A policy already saved under
 * that name and major (stored entries may hold a full version such as
 * "v1.0.2") is replaced in place, otherwise the policy is appended.
 */
export function applyPolicySelection(
  policies: MCPProxyPolicy[],
  selection: PolicySelection,
): MCPProxyPolicy[] {
  const next: MCPProxyPolicy = {
    name: selection.name,
    version: normalizePolicyVersionToMajor(selection.version),
    displayName: selection.displayName,
    params: selection.settings,
  };
  const index = policies.findIndex(
    (policy) =>
      policy.name === next.name &&
      normalizePolicyVersionToMajor(policy.version) === next.version,
  );
  if (index < 0) return [...policies, next];
  const result = [...policies];
  result[index] = next;
  return result;
}

export interface RequiredMCPPolicy {
  /** Version to store on the proxy policy. */
  version: string;
  displayName?: string;
}

/**
 * Finds the policy a tab needs (mcp-rewrite, mcp-acl-list) in the catalog.
 * Normally it must be installed on the gateway. While availability is unknown
 * (no gateway has reported yet) installation is not checked: the catalog entry
 * of that name is used whatever its status, else the version already saved on
 * the proxy. Returns undefined when saving must stay disabled.
 */
export function resolveRequiredMCPPolicy(
  catalog: MCPPoliciesCatalogResponse | undefined,
  name: string,
  saved: MCPProxyPolicy[] | undefined,
): RequiredMCPPolicy | undefined {
  const entries = (catalog?.data ?? []).filter((p) => p.name === name);
  const availabilityKnown = catalog?.availabilityKnown ?? true;
  const found = availabilityKnown
    ? entries.find((p) => p.status === "installed")
    : (entries.find((p) => p.status === "installed") ?? entries[0]);
  if (found) {
    return { version: found.savedVersion, displayName: found.displayName };
  }
  if (!catalog || availabilityKnown) return undefined;
  const existing = (saved ?? []).find((p) => p.name === name);
  return existing
    ? {
        version: normalizePolicyVersionToMajor(existing.version),
        displayName: existing.displayName,
      }
    : undefined;
}
