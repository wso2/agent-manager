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
import { useFeatureManagement } from "@agent-management-platform/feature-management";
import { MCPLogo } from "@agent-management-platform/shared-component";
import {
  absoluteRouteMap,
  PLUGIN_API_VERSION,
  type PluginManifest,
} from "@agent-management-platform/types";

// Route-map shorthands (absolute paths, used for both routes and links).
const orgNav = absoluteRouteMap.children.org.children;

/** The Resources section shows when the user can read any of its resources. */
const useResourcesNav = () => {
  const { hasAccess } = useFeatureManagement();
  return (
    hasAccess("llm_provider_read") ||
    hasAccess("llm_provider_template_read") ||
    hasAccess("mcp_server_read") ||
    hasAccess("llm_proxy_read")
  );
};

export const mcpProxiesPlugin: PluginManifest = {
  id: "mcp-proxies",
  name: "MCP servers",
  apiVersion: PLUGIN_API_VERSION,
  contributions: [
    {
      type: "route",
      id: "page",
      level: "org",
      path: orgNav.mcpProxies.path + "/*",
      load: () =>
        import("./index").then((m) => ({
          default: m.metaData.levels!.organization as ComponentType,
        })),
    },
    {
      type: "nav-item",
      id: "nav",
      level: "org",
      section: "resources",
      order: 20,
      label: "MCP servers",
      icon: MCPLogo,
      route: orgNav.mcpProxies,
      useActivation: useResourcesNav,
    },
  ],
};
