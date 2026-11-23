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

// Plugin manifest. The shell imports this module eagerly, so it must not import
// the page components: they are reached only through the dynamic import() in
// `load`, which keeps them in their own chunk.
import { ChartColumnStacked, Wallet } from "@wso2/oxygen-ui-icons-react";
import { useFeatureManagement } from "@agent-management-platform/feature-management";
import {
  absoluteRouteMap,
  PLUGIN_API_VERSION,
  type PluginManifest,
} from "@agent-management-platform/types";

const orgNav = absoluteRouteMap.children.org.children;
const agentNav = orgNav.projects.children.agents.children;

/** Cost Management shows cost and budgets; reading either opens it. */
const useCostAccess = () => {
  const { hasAccess } = useFeatureManagement();
  return hasAccess("ai_cost_read") || hasAccess("budget_read");
};

/** The Dashboard and the cost panels need cost read; budgets have their own access. */
const useCostRead = () => useFeatureManagement().hasAccess("ai_cost_read");
const useBudgetRead = () => useFeatureManagement().hasAccess("budget_read");

const loadPage = () =>
  import("./AICostManagement/AICostManagement").then((m) => ({
    default: m.AICostManagement,
  }));

export const aiCostManagementPlugin: PluginManifest = {
  id: "ai-cost-management",
  name: "Cost Management",
  apiVersion: PLUGIN_API_VERSION,
  useActivation: useCostAccess,
  contributions: [
    {
      type: "route",
      id: "org-page",
      level: "org",
      path: orgNav.aiCostManagement.path + "/*",
      load: loadPage,
    },
    {
      type: "route",
      id: "project-page",
      level: "project",
      path: orgNav.projects.children.aiCostManagement.path + "/*",
      load: loadPage,
    },
    {
      type: "route",
      id: "agent-page",
      level: "agent",
      path: agentNav.aiCostManagement.path + "/*",
      load: loadPage,
    },
    ...(["org", "project", "agent"] as const).flatMap((level) => {
      const page =
        level === "org"
          ? orgNav.aiCostManagement
          : level === "project"
            ? orgNav.projects.children.aiCostManagement
            : agentNav.aiCostManagement;
      return [
        {
          type: "nav-item" as const,
          id: `${level}-overview-nav`,
          useActivation: useCostRead,
          level,
          section: "cost-management",
          order: 10,
          label: "Dashboard",
          icon: ChartColumnStacked,
          // Only the overview itself; Budgets has its own item.
          route: { path: page.path, wildPath: page.path },
        },
        {
          type: "nav-item" as const,
          id: `${level}-budgets-nav`,
          useActivation: useBudgetRead,
          level,
          section: "cost-management",
          order: 20,
          label: "Budgets",
          icon: Wallet,
          route: page.children.budgets,
        },
      ];
    }),
  ],
};
