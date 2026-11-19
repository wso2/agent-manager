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
import { generatePath, useParams } from "react-router-dom";
import {
  useAgentTypesState,
  useExternalComponentModulesByMountPoint,
  type AgentTypeOption,
} from "@agent-management-platform/views";
import { absoluteRouteMap, PLUGIN_MOUNT_POINTS } from "@agent-management-platform/types";
import { useAgentWizardReturn } from "./useAgentWizardReturn";

/**
 * What the "Add a New Agent" page offers: the active agent types and any
 * module content below them. When there is a single agent type and nothing
 * else to show, the page has no choice to offer: it goes straight to that
 * type's form (`soleAgentType`), and the form's back link skips the page.
 */
export function useAgentTypePicker() {
  const { agentTypes, isPending } = useAgentTypesState();
  const extraContent = useExternalComponentModulesByMountPoint(
    PLUGIN_MOUNT_POINTS.addNewAgentOptions,
  );
  // Only once every type has settled: one still activating could be a second choice.
  const soleAgentType: AgentTypeOption | undefined =
    !isPending && agentTypes.length === 1 && extraContent.length === 0
      ? agentTypes[0]
      : undefined;
  return { agentTypes, isPending, extraContent, soleAgentType };
}

/**
 * The back link for an agent type's first form page: the "Add a New Agent"
 * page when it offers a choice, else straight out of the wizard. Going to the
 * page when it has a single type would only redirect back to the form, and
 * while its content is unsettled it might, so both cases leave the wizard.
 */
export function useAgentPickerBack(): { href: string; label: string } {
  const { orgId = "", projectId = "default" } = useParams<{
    orgId: string;
    projectId: string;
  }>();
  const { returnTo } = useAgentWizardReturn();
  const { isPending, soleAgentType } = useAgentTypePicker();
  const skipsPicker = isPending || !!soleAgentType;
  return useMemo(() => {
    if (skipsPicker) return { href: returnTo, label: "Back" };
    const newAgentPath = absoluteRouteMap.children.org.children.projects.children.newAgent.path;
    return {
      href: generatePath(newAgentPath, { orgId, projectId }),
      label: "Back to add new agent",
    };
  }, [skipsPicker, returnTo, orgId, projectId]);
}
