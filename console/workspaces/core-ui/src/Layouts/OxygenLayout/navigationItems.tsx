/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { useParams } from "react-router-dom";
import { useGetAgent } from "@agent-management-platform/api-client";
import { usePipelineEnvironmentsState } from "@agent-management-platform/shared-component";
import type { NavigationItem, NavigationSection } from "./LeftNavigation";
import { NAV_SECTIONS, useActiveNavEntry, useNavEntries } from "./navigation/useNavEntries";
import {
  currentNavLevel,
  isNavEntryShownAt,
  navEntryHref,
} from "./navigation/navModel";

/**
 * The left menu. It lists the same entries at every level (org, project,
 * agent); entries whose page lives at another level link to that level's gate
 * page instead. See navigation/navModel.ts.
 */
export function useNavigationItems(): Array<NavigationSection | NavigationItem> {
  const { orgId, projectId, agentId, envId } = useParams();
  const entries = useNavEntries();
  const active = useActiveNavEntry();
  const { data: agent } = useGetAgent({
    agentName: agentId,
    orgName: orgId,
    projName: projectId,
  });
  const { environments, isLoading: isLoadingEnvironments } =
    usePipelineEnvironmentsState(orgId, projectId);

  const level = currentNavLevel({ orgId, projectId, agentId });
  if (!level) {
    return [];
  }
  // Agent pages outside an environment open the first one in the pipeline,
  // once the pipeline has loaded (its order isn't final before). Until then the
  // entry links to the gate, which waits and picks it.
  const fallbackEnv =
    agentId && !isLoadingEnvironments ? environments[0]?.name : undefined;
  const params = { orgId, projectId, agentId, envId: envId ?? fallbackEnv };

  const toItem = (entry: (typeof entries)[number]): NavigationItem => ({
    label: entry.label,
    type: "item",
    icon: entry.icon,
    href: navEntryHref(entry, level, params, agent),
    isActive: entry.id === active?.id,
    pinBottom: entry.pinBottom,
    afterSections: entry.afterSections,
    dividerBefore: entry.dividerBefore,
  });

  const shown = entries.filter((entry) => isNavEntryShownAt(entry, level, agent));
  const items: NavigationItem[] = shown
    .filter((entry) => !entry.section)
    .map(toItem);
  const sections: NavigationSection[] = NAV_SECTIONS.map((section) => ({
    title: section.title,
    type: "section" as const,
    icon: section.icon,
    collapsible: true,
    items: shown.filter((entry) => entry.section === section.id).map(toItem),
  })).filter((section) => section.items.length > 0);

  return [...items, ...sections];
}
