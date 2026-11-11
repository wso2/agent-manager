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
import {
  useFeatureAccess,
  useFeatureFlag,
} from "@agent-management-platform/feature-management";
import { thunderInstancesMetadata } from "./metadata";
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

const thunderInstances = orgNav.thunderInstances;

/** Agent identities are listed for users who can read them. */
const useAgentIdentitiesNav = () => useFeatureAccess("agent_identity_read");

export const agentIdentityPlugin: PluginManifest = {
  id: "agent-identity",
  name: "Agent identity",
  module: "agent-identity",
  apiVersion: PLUGIN_API_VERSION,
  // Pages, not just menu items, stay unreachable while the feature is off.
  useActivation: () => useFeatureFlag("enableAgentIdentity"),
  contributions: [
    {
      type: "route",
      id: "org-page",
      level: "org",
      path: orgRoutes.thunderInstances.path + "/*",
      load: () =>
        import("./index").then((m) => ({
          default: m.thunderInstancesMetaData.levels!.organization as ComponentType,
        })),
    },
    {
      type: "route",
      id: "agent-id-page",
      level: "agent",
      path: agentRoutes.agentId.path,
      load: () =>
        import("./index").then((m) => ({
          default: m.agentIdMetaData.component as ComponentType,
        })),
    },
    {
      type: "nav-item",
      id: "groups-nav",
      level: "org",
      section: "agent-identities",
      order: 10,
      label: thunderInstancesMetadata.children.groups.title,
      icon: thunderInstancesMetadata.children.groups.icon,
      route: thunderInstances.children.groups,
      useActivation: useAgentIdentitiesNav,
    },
    {
      type: "nav-item",
      id: "roles-nav",
      level: "org",
      section: "agent-identities",
      order: 20,
      label: thunderInstancesMetadata.children.roles.title,
      icon: thunderInstancesMetadata.children.roles.icon,
      route: thunderInstances.children.roles,
      useActivation: useAgentIdentitiesNav,
    },
    {
      type: "nav-item",
      id: "agent-id-nav",
      level: "agent",
      section: "security",
      // Before Credentials (10).
      order: 0,
      label: "Agent ID",
      icon: thunderInstancesMetadata.icon,
      route: agentNav.agentId,
    },
  ],
};
