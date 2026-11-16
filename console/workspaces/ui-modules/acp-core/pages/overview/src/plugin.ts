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
import { Home, LayoutDashboard } from "@wso2/oxygen-ui-icons-react";
import {
  absoluteRouteMap,
  PLUGIN_API_VERSION,
  type PluginManifest,
} from "@agent-management-platform/types";

// Route-map shorthands (absolute paths, used for both routes and links).
const orgNav = absoluteRouteMap.children.org.children;
const agentNav = orgNav.projects.children.agents.children;

export const overviewPlugin: PluginManifest = {
  id: "overview",
  name: "Overview",
  apiVersion: PLUGIN_API_VERSION,
  contributions: [
    {
      type: "route",
      id: "org-page",
      level: "org",
      index: true,
      load: () =>
        import("./index").then((m) => ({
          default: m.metaData.levels!.organization as ComponentType,
        })),
    },
    {
      type: "route",
      id: "project-page",
      level: "project",
      index: true,
      load: () =>
        import("./index").then((m) => ({
          default: m.metaData.levels!.project as ComponentType,
        })),
    },
    {
      type: "route",
      id: "agent-page",
      level: "agent",
      index: true,
      load: () =>
        import("./index").then((m) => ({
          default: m.metaData.levels!.component as ComponentType,
        })),
    },
    {
      type: "route",
      id: "edit-project-page",
      level: "project",
      path: orgNav.projects.children.edit.path,
      load: () =>
        import("./index").then((m) => ({
          default: m.EditProject as ComponentType,
        })),
    },
    {
      type: "route",
      id: "edit-agent-page",
      level: "agent",
      path: agentNav.edit.path,
      load: () =>
        import("./index").then((m) => ({
          default: m.EditAgent as ComponentType,
        })),
    },
    {
      type: "nav-item",
      id: "org-nav",
      level: "org",
      section: "main",
      order: 10,
      label: "Overview",
      icon: Home,
      // Exact match: the org overview is the org's landing page only.
      route: {
        path: absoluteRouteMap.children.org.path,
        wildPath: absoluteRouteMap.children.org.path,
      },
    },
    {
      type: "nav-item",
      id: "project-nav",
      level: "project",
      section: "main",
      order: 10,
      label: "Overview",
      icon: Home,
      route: {
        path: orgNav.projects.path,
        wildPath: orgNav.projects.path,
      },
      activePaths: [orgNav.projects.children.edit.path],
    },
    {
      type: "nav-item",
      id: "agent-nav",
      level: "agent",
      section: "main",
      order: 10,
      label: "Overview",
      icon: LayoutDashboard,
      route: {
        path: orgNav.projects.children.agents.path,
        wildPath: orgNav.projects.children.agents.path,
      },
    },
  ],
};
