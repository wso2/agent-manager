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
import { ChartColumnStacked, Coins, Wallet } from "@wso2/oxygen-ui-icons-react";
import { useFeatureManagement } from "@agent-management-platform/feature-management";
import {
  absoluteRouteMap,
  PLUGIN_API_VERSION,
  type PluginManifest,
} from "@agent-management-platform/types";

const orgNav = absoluteRouteMap.children.org.children;
const agentNav = orgNav.projects.children.agents.children;

/** Reading cost management (costs.read) opens the dashboard, the cost panels and budgets. */
const useCostRead = () => useFeatureManagement().hasAccess("cost_read");

const loadPage = () =>
  import("./AICostManagement/AICostManagement").then((m) => ({
    default: m.AICostManagement,
  }));

export const aiCostManagementPlugin: PluginManifest = {
  id: "ai-cost-management",
  name: "Cost management",
  apiVersion: PLUGIN_API_VERSION,
  useActivation: useCostRead,
  contributions: [
    {
      type: "route",
      id: "org-page",
      level: "org",
      path: orgNav.costManagement.path + "/*",
      load: loadPage,
    },
    {
      type: "route",
      id: "project-page",
      level: "project",
      path: orgNav.projects.children.costManagement.path + "/*",
      load: loadPage,
    },
    {
      type: "route",
      id: "agent-page",
      level: "agent",
      path: agentNav.costManagement.path + "/*",
      load: loadPage,
    },
    ...(["org", "project", "agent"] as const).flatMap((level) => {
      const page =
        level === "org"
          ? orgNav.costManagement
          : level === "project"
            ? orgNav.projects.children.costManagement
            : agentNav.costManagement;
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
          useActivation: useCostRead,
          level,
          section: "cost-management",
          order: 20,
          label: "Budgets",
          icon: Wallet,
          route: page.children.budgets,
        },
        // The price catalog is the organization's, so only its page has Pricing.
        ...(level === "org"
          ? [
              {
                type: "nav-item" as const,
                id: "org-pricing-nav",
                useActivation: useCostRead,
                level,
                section: "cost-management",
                order: 30,
                label: "Pricing Catalog",
                icon: Coins,
                route: orgNav.costManagement.children.pricing,
              },
            ]
          : []),
      ];
    }),
  ],
};
