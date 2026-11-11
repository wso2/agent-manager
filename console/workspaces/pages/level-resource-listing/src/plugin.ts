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
// each `load`, which keeps them in the package's own chunk.
import type { ComponentType } from "react";
import { Bot, Box } from "@wso2/oxygen-ui-icons-react";
import {
  absoluteRouteMap,
  relativeRouteMap,
  PLUGIN_API_VERSION,
  type PluginManifest,
} from "@agent-management-platform/types";

// Route-map shorthands: relative paths for routes, absolute ones for links.
const orgRoutes = relativeRouteMap.children.org.children;
const orgNav = absoluteRouteMap.children.org.children;

export const resourceListingPlugin: PluginManifest = {
  id: "resource-listing",
  name: "Project and agent listings",
  apiVersion: PLUGIN_API_VERSION,
  contributions: [
    {
      type: "route",
      id: "projects-page",
      level: "org",
      path: orgRoutes.projectListing.path,
      load: () =>
        import("./index").then((m) => ({
          default: m.metaData.levels!.organization as ComponentType,
        })),
    },
    {
      type: "route",
      id: "agents-page",
      level: "project",
      path: orgRoutes.projects.children.agentListing.path,
      load: () =>
        import("./index").then((m) => ({
          default: m.metaData.levels!.project as ComponentType,
        })),
    },
    {
      type: "nav-item",
      id: "projects-nav",
      level: "org",
      section: "main",
      order: 20,
      label: "Projects",
      icon: Box,
      route: {
        path: orgNav.projectListing.path,
        wildPath: orgNav.projectListing.path,
      },
    },
    {
      type: "nav-item",
      id: "agents-nav",
      level: "project",
      section: "main",
      order: 20,
      label: "Agents",
      icon: Bot,
      route: {
        path: orgNav.projects.children.agentListing.path,
        wildPath: orgNav.projects.children.agentListing.path,
      },
      // Stays highlighted inside any agent of the project.
      activePaths: [orgNav.projects.children.agents.wildPath],
    },
  ],
};
