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
import { BookOpenText, Megaphone } from "@wso2/oxygen-ui-icons-react";
import { useRouteAgent } from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  relativeRouteMap,
  PLUGIN_API_VERSION,
  type PluginActivationState,
  type PluginManifest,
} from "@agent-management-platform/types";

// Route-map shorthands: relative paths for routes, absolute ones for links.
const orgRoutes = relativeRouteMap.children.org.children;
const orgNav = absoluteRouteMap.children.org.children;
const agentRoutes = orgRoutes.projects.children.agents.children;
const agentNav = orgNav.projects.children.agents.children;

/** Internal agents built from source, i.e. not created from a catalog kind. */
const useSourceBuiltAgent = (): PluginActivationState => {
  const { data: agent, isLoading } = useRouteAgent();
  if (isLoading) return undefined;
  return !!agent && agent.provisioning?.type !== "external" && !agent.kindName;
};

export const agentKindPlugin: PluginManifest = {
  id: "agent-kind",
  name: "Agent catalog and publishing",
  module: "catalog",
  apiVersion: PLUGIN_API_VERSION,
  contributions: [
    {
      type: "route",
      id: "catalog-page",
      level: "org",
      path: orgRoutes.catalog.path + "/*",
      load: () =>
        import("./index").then((m) => ({
          default: m.metaData.levels!.organization as ComponentType,
        })),
    },
    {
      type: "route",
      id: "publish-page",
      level: "agent",
      path: agentRoutes.publish.path + "/*",
      load: () =>
        import("./index").then((m) => ({
          default: m.metaData.levels!.publishOrganization as ComponentType,
        })),
    },
    {
      type: "nav-item",
      id: "catalog-nav",
      level: "org",
      section: "main",
      order: 30,
      label: "Agent catalog",
      icon: BookOpenText,
      route: orgNav.catalog,
    },
    {
      type: "nav-item",
      id: "publish-nav",
      level: "agent",
      section: "lifecycle",
      order: 50,
      label: "Publish",
      icon: Megaphone,
      route: agentNav.publish,
      useActivation: useSourceBuiltAgent,
    },
  ],
};
