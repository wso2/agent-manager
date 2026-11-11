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
import { Settings } from "@wso2/oxygen-ui-icons-react";
import {
  absoluteRouteMap,
  relativeRouteMap,
  PLUGIN_API_VERSION,
  type PluginManifest,
} from "@agent-management-platform/types";

// Route-map shorthands: relative paths for routes, absolute ones for links.
const orgRoutes = relativeRouteMap.children.org.children;
const orgNav = absoluteRouteMap.children.org.children;
const agentRoutes = orgRoutes.projects.children.agents.children;
const agentNav = orgNav.projects.children.agents.children;

const configure = agentRoutes.configure;
const llmProviders = configure.children.llmProviders;

export const configureAgentPlugin: PluginManifest = {
  id: "configure-agent",
  name: "Configure agent",
  apiVersion: PLUGIN_API_VERSION,
  contributions: [
    {
      type: "route",
      id: "page",
      level: "agent",
      path: configure.path,
      load: () =>
        import("./index").then((m) => ({
          default: m.metaData.component as ComponentType,
        })),
    },
    {
      type: "route",
      id: "add-llm-provider-page",
      level: "agent",
      path: configure.path + "/" + llmProviders.path + "/" + llmProviders.children.add.path,
      load: () =>
        import("./index").then((m) => ({
          default: m.AddLLMProviderComponent as ComponentType,
        })),
    },
    {
      type: "route",
      id: "edit-llm-provider-page",
      level: "agent",
      path: configure.path + "/" + llmProviders.path + "/" + llmProviders.children.edit.path,
      load: () =>
        import("./index").then((m) => ({
          default: m.AddLLMProviderComponent as ComponentType,
        })),
    },
    {
      type: "route",
      id: "view-llm-provider-page",
      level: "agent",
      path: configure.path + "/" + llmProviders.path + "/" + llmProviders.children.view.path,
      load: () =>
        import("./index").then((m) => ({
          default: m.ViewLLMProviderComponent as ComponentType,
        })),
    },
    {
      type: "route",
      id: "view-mcp-server-page",
      level: "agent",
      path:
        configure.path +
        "/" +
        configure.children.mcpProxies.path +
        "/" +
        configure.children.mcpProxies.children.view.path,
      load: () =>
        import("./index").then((m) => ({
          default: m.ViewMCPServerComponent as ComponentType,
        })),
    },
    {
      type: "nav-item",
      id: "nav",
      level: "agent",
      section: "lifecycle",
      order: 10,
      label: "Configure",
      icon: Settings,
      route: agentNav.configure,
    },
  ],
};
