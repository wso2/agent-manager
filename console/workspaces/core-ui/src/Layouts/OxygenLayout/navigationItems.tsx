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

import type { ReactNode } from "react";
import {
  Activity,
  Boxes,
  ClipboardCheck as EvaluationOutline,
  GitBranch,
  IdCard,
  Lock,
  Server,
} from "@wso2/oxygen-ui-icons-react";
import { generatePath, matchPath, useLocation, useParams } from "react-router-dom";
import { useRouteAgent } from "@agent-management-platform/api-client";
import { usePipelineEnvironmentsState } from "@agent-management-platform/shared-component";
import { useExternalNavItems } from "@agent-management-platform/views";
import type { PluginLevel } from "@agent-management-platform/types";
import type { NavigationItem, NavigationSection } from "./LeftNavigation";
import {
  usePluginNavItemsForLevel,
  type ActivePluginNavItem,
} from "../../plugins";

// The left navigation is a fixed layout per level that plugins fill in. The
// shell decides which slots exist and in what order; each plugin's nav items
// name a slot (`section`) and an `order`, and their activation hooks decide
// whether they show. A section with no active items is left out entirely.
//
// Flat slots ("main", "footer") render their items directly; the rest render
// as collapsible sections. Items naming a slot the level does not have are
// appended at the end, so a third-party plugin cannot get lost.
interface NavSlot {
  key: string;
  title?: string;
  icon?: ReactNode;
}

const NAV_LAYOUT: Record<PluginLevel, NavSlot[]> = {
  org: [
    { key: "main" },
    { key: "footer" },
    { key: "resources", title: "Resources", icon: <Boxes size={20} /> },
    { key: "evaluation", title: "Evaluation", icon: <EvaluationOutline /> },
    { key: "infrastructure", title: "Infrastructure", icon: <Server /> },
    { key: "agent-identities", title: "Agent identities", icon: <IdCard /> },
  ],
  project: [{ key: "main" }, { key: "footer" }],
  agent: [
    { key: "main" },
    { key: "lifecycle", title: "Agent lifecycle", icon: <GitBranch /> },
    { key: "security", title: "Security", icon: <Lock /> },
    { key: "observability", title: "Observability", icon: <Activity /> },
    { key: "evaluation", title: "Evaluation", icon: <EvaluationOutline /> },
    { key: "footer" },
  ],
};

const LEGACY_LEVEL: Record<PluginLevel, string> = {
  org: "org",
  project: "project",
  agent: "component",
};

export function useNavigationItems(): Array<
  NavigationSection | NavigationItem
> {
  const { orgId, projectId, agentId, envId } = useParams();
  const { data: agent, isLoading: isLoadingAgent } = useRouteAgent();
  const { environments, isLoading: isLoadingEnvironments } =
    usePipelineEnvironmentsState(orgId, projectId);
  const { pathname } = useLocation();
  const externalNavItems = useExternalNavItems();
  const pluginItems: Record<PluginLevel, ActivePluginNavItem[]> = {
    org: usePluginNavItemsForLevel("org"),
    project: usePluginNavItemsForLevel("project"),
    agent: usePluginNavItemsForLevel("agent"),
  };

  const defaultEnv =
    envId ?? (environments.length > 0 ? environments[0]?.name : "");

  // Access is settled here: FeatureAccessBridge shows a loader until it resolves.
  if (isLoadingAgent) {
    return [];
  }

  // Which level's menu to show. External agents always get the agent menu;
  // internal ones need an environment for their links, and fall back to the
  // project menu until there is one.
  let level: PluginLevel | null = null;
  if (orgId && projectId && agentId && (agent?.provisioning.type === "external" || defaultEnv)) {
    level = "agent";
  } else if (orgId && projectId) {
    level = "project";
  } else if (orgId) {
    level = "org";
  }
  if (!level) {
    return [];
  }

  // generatePath throws on a missing param, and every param the level needs is
  // present by the checks above.
  const params = { orgId, projectId, agentId, envId: defaultEnv };
  const toNavItem = (item: ActivePluginNavItem): NavigationItem => ({
    label: item.label,
    type: "item",
    icon: <item.icon size={20} />,
    href: generatePath(item.route.path, params),
    isActive: [item.route.wildPath, ...(item.activePaths ?? [])].some(
      (pattern) => !!matchPath(pattern, pathname),
    ),
    ...(item.dividerBefore ? { dividerBefore: true } : {}),
    ...(item.pinBottom ? { pinBottom: true } : {}),
  });

  const items = pluginItems[level];
  const layout = NAV_LAYOUT[level];
  const navigation: Array<NavigationSection | NavigationItem> = [];
  for (const slot of layout) {
    const slotItems = items.filter((item) => item.section === slot.key).map(toNavItem);
    if (!slotItems.length) continue;
    if (slot.title) {
      navigation.push({
        title: slot.title,
        type: "section",
        icon: slot.icon,
        collapsible: true,
        items: slotItems,
      });
    } else {
      navigation.push(...slotItems);
    }
  }
  const knownSlots = new Set(layout.map((slot) => slot.key));
  navigation.push(
    ...items
      .filter((item) => !item.section || !knownSlots.has(item.section))
      .map(toNavItem),
  );

  // Nav items passed to CoreUI the legacy way (externalPageModules).
  navigation.push(
    ...externalNavItems
      .filter((item) => item.level === LEGACY_LEVEL[level])
      .map((item) => ({
        label: item.title,
        type: "item" as const,
        icon: item.icon,
        isActive: !!matchPath(item.route, pathname),
        href: generatePath(item.route, params),
      })),
  );
  return navigation;
}
