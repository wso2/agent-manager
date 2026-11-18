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

import { useCallback, useMemo } from "react";
import {
  normalizePolicyVersionToMajor,
  useMCPPoliciesCatalog,
} from "@agent-management-platform/api-client";
import type {
  MCPEndpointConfig,
  MCPProxyPolicy,
} from "@agent-management-platform/types";
import {
  PolicyListSection,
  type PolicyListSectionProps,
  type PolicySelection,
} from "@agent-management-platform/shared-component";
import { useResourceAccess } from "@agent-management-platform/feature-management";
import { MANAGED_MCP_POLICY_NAMES } from "../constants";
import { applyPolicySelection } from "./mcpPolicySelection";

function isManagedMCPPolicy(name: string): boolean {
  return MANAGED_MCP_POLICY_NAMES.includes(name);
}

function mcpPoliciesToSelections(
  policies: MCPProxyPolicy[],
): PolicySelection[] {
  return policies.map((policy) => ({
    name: policy.name,
    version: policy.version,
    displayName: policy.displayName,
    settings: policy.params,
  }));
}

function selectionsToMCPPolicies(
  selections: PolicySelection[],
): MCPProxyPolicy[] {
  return selections.map((selection) => ({
    name: selection.name,
    version: selection.version,
    displayName: selection.displayName,
    params: selection.settings,
  }));
}

// MCP proxies save the major version (e.g. "v1") while the catalog lists the
// exact gateway version, so saved policies are matched by name and major.
const matchApplied: NonNullable<PolicyListSectionProps["matchApplied"]> = (
  applied,
  item,
) =>
  applied.name === item.name &&
  normalizePolicyVersionToMajor(applied.version) ===
    normalizePolicyVersionToMajor(item.version);

export type MCPProxyPoliciesTabProps = {
  config: MCPEndpointConfig | undefined;
  selectedEndpointId: string;
  orgName?: string;
  /** MCP proxy handle; scopes policy availability to the proxy's gateways. */
  proxyId?: string;
  onUpdate: (fields: Partial<MCPEndpointConfig>) => Promise<unknown>;
  isUpdating: boolean;
};

export function MCPProxyPoliciesTab({
  config,
  orgName,
  proxyId,
  onUpdate,
}: MCPProxyPoliciesTabProps) {
  const { canWrite } = useResourceAccess("mcp_server");
  const {
    data: mcpCatalog,
    isLoading: isLoadingCatalog,
    error: catalogError,
  } = useMCPPoliciesCatalog(orgName, proxyId);

  // The selected environment's policies (all of them, including managed ones the
  // other tabs own). Edits are persisted immediately to that environment's block.
  const policies = useMemo(() => config?.policies ?? [], [config?.policies]);

  const onPoliciesChange = useCallback(
    (nextPolicies: MCPProxyPolicy[]) => {
      void onUpdate({ policies: nextPolicies });
    },
    [onUpdate],
  );

  const visibleSelections = useMemo(
    () =>
      mcpPoliciesToSelections(
        policies.filter((policy) => !isManagedMCPPolicy(policy.name)),
      ),
    [policies],
  );

  const replacePolicy = useCallback(
    (nextSelection: PolicySelection) => {
      onPoliciesChange(applyPolicySelection(policies, nextSelection));
    },
    [onPoliciesChange, policies],
  );

  const removePolicy = useCallback(
    (name: string, version: string) => {
      onPoliciesChange(
        policies.filter(
          (policy) => policy.name !== name || policy.version !== version,
        ),
      );
    },
    [onPoliciesChange, policies],
  );

  const reorderPolicies = useCallback(
    (nextVisible: PolicySelection[]) => {
      const reorderedVisible = selectionsToMCPPolicies(nextVisible);
      let visibleIdx = 0;
      const merged = policies.map((policy) => {
        if (isManagedMCPPolicy(policy.name)) {
          return policy;
        }
        const next = reorderedVisible[visibleIdx++];
        return next ?? policy;
      });
      onPoliciesChange(merged);
    },
    [onPoliciesChange, policies],
  );

  return (
    <PolicyListSection
      policies={visibleSelections}
      onAdd={replacePolicy}
      onEdit={replacePolicy}
      onRemove={removePolicy}
      onReorder={reorderPolicies}
      readOnly={!canWrite}
      catalogData={mcpCatalog}
      matchApplied={matchApplied}
      isLoadingCatalog={isLoadingCatalog}
      catalogError={catalogError}
      filterPolicies={(items) =>
        items.filter((policy) => !isManagedMCPPolicy(policy.name))
      }
      title="Policies"
      description="Add policies to this server and drag to change their execution order."
      addButtonLabel="Add policy"
      drawerAddTitle="Add Policy"
      drawerEditTitle="Edit Policy"
      drawerAddSubtitle="Choose a policy to configure advanced options."
      drawerEditSubtitle="Update the policy configuration."
      policyNoun="policy"
      loadingLabel="Loading policies..."
      searchPlaceholder="Search policies"
      catalogErrorLabel="Failed to load policies."
      emptySearchTitle="No policies match your search"
      emptyCatalogTitle="No policies available"
      emptyCatalogDescription="No MCP policies reported by the active gateway are available in the policy hub."
    />
  );
}

export default MCPProxyPoliciesTab;
